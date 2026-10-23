/*
Copyright The k3sm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package budgetv1alpha1

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// pageReadError is a reader error by its contract name (PageErr*).
type pageReadError struct {
	name   string
	detail string
}

func (e *pageReadError) Error() string { return e.name + ": " + e.detail }

func readErr(name, format string, a ...any) error {
	return &pageReadError{name: name, detail: fmt.Sprintf(format, a...)}
}

// decodePage is the test-only reader of a static page: the checks a reader
// makes, in the order the contract states them, then every field. A live
// reader would retry an odd seq; a static page with one is torn.
func decodePage(b []byte) (PageSnapshot, error) {
	var s PageSnapshot
	if len(b) < PageSeqOffset {
		return s, readErr(PageErrTruncated, "%d bytes, shorter than the header line", len(b))
	}
	if string(b[:len(PageMagic)]) != PageMagic {
		return s, readErr(PageErrABIMismatch, "magic %q", b[:len(PageMagic)])
	}
	le := binary.LittleEndian
	if m := le.Uint16(b[pageOffset("abiMajor"):]); m != PageABIMajor {
		return s, readErr(PageErrABIMismatch, "abiMajor %d", m)
	}
	if h := le.Uint32(b[pageOffset("headerSize"):]); h != PageHeaderSize {
		return s, readErr(PageErrABIMismatch, "headerSize %d", h)
	}
	body := int(le.Uint32(b[pageOffset("bodySize"):]))
	if body < pageBodySize() {
		return s, readErr(PageErrTruncated, "bodySize %d below the known %d", body, pageBodySize())
	}
	if len(b) < PageHeaderSize+body {
		return s, readErr(PageErrTruncated, "%d bytes, want at least %d", len(b), PageHeaderSize+body)
	}
	if seq := le.Uint64(b[PageSeqOffset:]); seq%2 == 1 {
		return s, readErr(PageErrTorn, "seq %d is odd", seq)
	}

	word := func(name string) uint64 { return le.Uint64(b[pageOffset(name):]) }
	i64 := func(name string) int64 { return int64(word(name)) }
	opt := func(name string) *int64 {
		v := i64(name)
		if v == PageAbsent {
			return nil
		}
		return &v
	}
	var bad error
	code := func(name string, table []string, unknown string) string {
		c := word(name)
		switch {
		case c == 0:
			return ""
		case c <= uint64(len(table)):
			return table[c-1]
		case unknown != "":
			return unknown
		}
		if bad == nil {
			bad = readErr(PageErrABIMismatch, "%s holds unknown code %d", name, c)
		}
		return ""
	}
	codes := PageCodeTables()
	unixTime := func(v int64) metav1.Time { return metav1.NewTime(time.Unix(v, 0).UTC()) }

	n := word("nodeNameLen")
	if n > MaxPageNodeNameBytes {
		return s, readErr(PageErrABIMismatch, "nodeNameLen %d above %d", n, MaxPageNodeNameBytes)
	}
	off := pageOffset("nodeName")
	s.NodeName = string(b[off : off+int(n)])
	s.Generation = i64("generation")
	s.WriterPeriodSeconds = i64("writerPeriodSeconds")
	s.ObservedAt = unixTime(i64("observedAt"))
	s.Platform = Platform(code("platform", codes["platform"], ""))

	s.Capacity = BudgetCapacity{
		CPUCapacityMilli: opt("capacity.cpuCapacityMilli"), CPUSystemReserveMilli: opt("capacity.cpuSystemReserveMilli"),
		MemCapacityBytes: opt("capacity.memCapacityBytes"), MemSystemReserveBytes: opt("capacity.memSystemReserveBytes"),
		MemEvictionReserveBytes: opt("capacity.memEvictionReserveBytes"), MemAllocatableBytes: opt("capacity.memAllocatableBytes"),
		SwapTotalBytes: opt("capacity.swapTotalBytes"), SwapUsedBytes: opt("capacity.swapUsedBytes"),
	}
	s.Level = Level(code("level", codes["level"], ""))

	if s.Platform == PlatformDarwin {
		d := &DarwinSignals{
			VMPressureLevel: opt("darwin.vmPressureLevel"), MemorystatusLevelPermille: opt("darwin.memorystatusLevelPermille"),
			SwapinsTotal: opt("darwin.swapinsTotal"), SwapoutsTotal: opt("darwin.swapoutsTotal"),
		}
		c := &DarwinCompressor{
			BytesUsed: opt("darwin.compressor.bytesUsed"), BytesStored: opt("darwin.compressor.bytesStored"),
			SegmentLimitBytes: opt("darwin.compressor.segmentLimitBytes"),
		}
		if *c != (DarwinCompressor{}) {
			d.Compressor = c
		}
		s.Darwin = d
	}
	psi := &LinuxPSI{}
	for _, r := range []struct {
		name string
		dst  **PSIResource
	}{{"cpu", &psi.CPU}, {"memory", &psi.Memory}, {"io", &psi.IO}} {
		res := &PSIResource{}
		for _, l := range []struct {
			name string
			dst  **PSILine
		}{{"some", &res.Some}, {"full", &res.Full}} {
			p := "linux.psi." + r.name + "." + l.name + "."
			line := &PSILine{Avg10Permille: opt(p + "avg10Permille"), Avg60Permille: opt(p + "avg60Permille"), TotalUs: opt(p + "totalUs")}
			if *line != (PSILine{}) {
				*l.dst = line
			}
		}
		if *res != (PSIResource{}) {
			*r.dst = res
		}
	}
	ev := &LinuxMemoryEvents{
		High: opt("linux.memoryEvents.high"), Max: opt("linux.memoryEvents.max"),
		OOM: opt("linux.memoryEvents.oom"), OOMKill: opt("linux.memoryEvents.oomKill"),
	}
	lin := &LinuxSignals{}
	if *psi != (LinuxPSI{}) {
		lin.PSI = psi
	}
	if *ev != (LinuxMemoryEvents{}) {
		lin.MemoryEvents = ev
	}
	if s.Platform == PlatformLinux || *lin != (LinuxSignals{}) {
		s.Linux = lin
	}

	for i := range MaxPageSignals {
		p := "signals[" + strconv.Itoa(i) + "]."
		if word(p+"id") == 0 {
			continue
		}
		s.Signals = append(s.Signals, BudgetSignal{
			ID:                   SignalID(code(p+"id", codes["signal"], "")),
			Unit:                 SignalUnit(code(p+"unit", codes["unit"], "")),
			Value:                opt(p + "value"),
			WarnThreshold:        opt(p + "warnThreshold"),
			AdmitStopThreshold:   opt(p + "admitStopThreshold"),
			CriticalThreshold:    opt(p + "criticalThreshold"),
			EvictSoftThreshold:   opt(p + "evictSoftThreshold"),
			EvictHardThreshold:   opt(p + "evictHardThreshold"),
			WarnRelease:          opt(p + "warnRelease"),
			AdmitStopRelease:     opt(p + "admitStopRelease"),
			CriticalRelease:      opt(p + "criticalRelease"),
			EvictSoftRelease:     opt(p + "evictSoftRelease"),
			EvictHardRelease:     opt(p + "evictHardRelease"),
			GraceSeconds:         opt(p + "graceSeconds"),
			MinReclaimBytes:      opt(p + "minReclaimBytes"),
			WindowSeconds:        opt(p + "windowSeconds"),
			EtaToNextRungSeconds: opt(p + "etaToNextRungSeconds"),
			Trend:                Trend(code(p+"trend", codes["trend"], string(TrendUnknown))),
			LimitingResource:     LimitingResource(code(p+"limitingResource", codes["limitingResource"], string(ResourceUnknown))),
			SharedWith:           SignalID(code(p+"sharedWith", codes["signal"], "")),
		})
	}

	s.Admit = word("admit") == 1
	s.AdmitReasonSignal = SignalID(code("admitReasonSignal", codes["signal"], ""))
	if v := opt("evictDeadline"); v != nil {
		t := unixTime(*v)
		s.EvictDeadline = &t
	}
	s.KillMode = KillMode{
		Mode:           KillModeName(code("killMode.mode", codes["killMode"], "")),
		Armed:          word("killMode.armed") == 1,
		ArmingEpisodes: int32(i64("killMode.armingEpisodes")),
		Reason:         KillModeReason(code("killMode.reason", codes["killModeReason"], string(KillReasonUnknown))),
	}
	s.Recommendation = BudgetRecommendation{
		SafeAdditionalBytes: opt("recommendation.safeAdditionalBytes"),
		Basis:               SignalID(code("recommendation.basis", codes["signal"], "")),
	}
	for i := range MaxPageCounters {
		p := "counters[" + strconv.Itoa(i) + "]."
		if word(p+"band") == 0 {
			continue
		}
		s.Counters = append(s.Counters, PageBandCounters{
			Band:               Band(code(p+"band", codes["band"], "")),
			EvictionsTotal:     i64(p + "evictionsTotal"),
			OOMKillsTotal:      i64(p + "oomKillsTotal"),
			KernelKillsTotal:   i64(p + "kernelKillsTotal"),
			RestartsTotal:      i64(p + "restartsTotal"),
			MemHighEventsTotal: i64(p + "memHighEventsTotal"),
			Tier1RefusedTotal:  i64(p + "tier1RefusedTotal"),
		})
	}
	return s, bad
}
