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
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"unsafe"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The budget page ABI. Its major and minor are versioned on their own,
// independently of the CRD version: v1alpha1 can graduate without a page
// change, and a page major can move without a CRD change.
const (
	// PageABIMajor is the page ABI major. A new major is a new file name
	// (PageFileName), so a reader of an older major finds no page.
	PageABIMajor = 1
	// PageABIMinor is the page ABI minor. A minor only appends fields.
	PageABIMinor = 0
	// PageFileName is the page's file name, budget.v<PageABIMajor>.page.
	PageFileName = "budget.v1.page"
	// DefaultPagePath is where a native Darwin pod finds the page: a
	// top-level budget directory under the default data root (/var/lib/k3sm),
	// beside the server and agent work dirs and outside every protected tree.
	DefaultPagePath = "/var/lib/k3sm/budget/" + PageFileName
	// LinuxPagePath is where a Linux reader finds the page (a vm guest, or a
	// pod on a Linux producer's node), by a bind mount or a hostPath.
	LinuxPagePath = "/run/k3sm/budget/" + PageFileName
	// NodeBudgetAPIVersion is the NodeBudget apiVersion the page mirrors.
	NodeBudgetAPIVersion = "budget.k3sm.io/v1alpha1"

	// PageSize is the page file's fixed size in bytes. The file is never
	// shorter, renamed or truncated.
	PageSize = 4096
	// PageBodyLimit is the byte offset no field may end beyond: a quarter of
	// PageSize stays free for appended minors.
	PageBodyLimit = 3072
	// PageMagic is the eight bytes at offset 0.
	PageMagic = "K3SMBDGT"
	// PageSeqOffset is the offset of the seqlock word, alone on its own
	// 64-byte line.
	PageSeqOffset = 64
	// PageNotifyEpochOffset is the offset of notifyEpoch, a u64 alone on its
	// own 64-byte line outside the seqlock region. Readers that wait wait on
	// its low 32-bit word, which on this little-endian page is the four
	// bytes at this same offset.
	PageNotifyEpochOffset = 128
	// PageHeaderSize is the offset of the first body field: the header
	// line, the seq line and the notifyEpoch line.
	PageHeaderSize = 192
	// PageAbsent is the sentinel an i64 field holds when the producer could
	// not sample it. 0 is never absent.
	PageAbsent int64 = math.MinInt64
	// MaxPageSignals is the number of signal slots on the page.
	MaxPageSignals = MaxBudgetSignals
	// MaxPageCounters is the number of per-band counter slots on the page.
	MaxPageCounters = 3
	// MaxPageNodeNameBytes is the longest node name the page carries, the
	// Kubernetes object name limit.
	MaxPageNodeNameBytes = 253
	// PageNodeNameBytes is the size of the inline nodeName field.
	PageNodeNameBytes = 256
)

var (
	// ErrInvalidPageSnapshot is returned by EncodePage and
	// PageWriter.Publish for a snapshot the page cannot carry.
	ErrInvalidPageSnapshot = errors.New("budgetv1alpha1: invalid page snapshot")
	// ErrPageNodeNameTooLong is returned, wrapped in ErrInvalidPageSnapshot,
	// for a node name longer than MaxPageNodeNameBytes.
	ErrPageNodeNameTooLong = errors.New("budgetv1alpha1: page node name longer than 253 bytes")
	// ErrPageBuffer is returned for a destination or mapping the page cannot
	// be written to: too short, not 8-byte aligned, or a big-endian host.
	ErrPageBuffer = errors.New("budgetv1alpha1: unusable page buffer")
	// ErrPageWriterNotResumed is returned by Publish before Resume.
	ErrPageWriterNotResumed = errors.New("budgetv1alpha1: page writer not resumed")
)

// PageSnapshot is what one page publication carries. It mirrors
// NodeBudget: the same fields under the same names and in the same units
// (seconds, bytes, permille, counts), reusing the status types, so a value
// reads the same on the page and in the status. The page-only fields are
// the header, seq, notifyEpoch and WriterPeriodSeconds; the CRD-only fields
// are the conditions, the writer, the policy references (policy,
// inForce), wouldKill and the windowed counts, and with them every string
// but the node name.
type PageSnapshot struct {
	// Generation is the producer's generation, status.producerGeneration:
	// it bumps on a writer restart and on a policy change.
	Generation int64
	// WriterPeriodSeconds is the writer's publication period, at least 1. A
	// reader treats the page as stale when now - ObservedAt exceeds three
	// periods (the manifest's staleFactor).
	WriterPeriodSeconds int64
	// NodeName is the node's name, metadata.name, 1 to
	// MaxPageNodeNameBytes bytes.
	NodeName string
	// ObservedAt is when the producer took the observation; the page holds
	// it in whole Unix seconds.
	ObservedAt metav1.Time
	// Platform names the producer's platform block.
	Platform Platform
	// Capacity is the node's capacity and reserves.
	Capacity BudgetCapacity
	// Level is the derived pressure level.
	Level Level
	// Darwin is the macOS platform block. It is required on a darwin page
	// and refused on a linux page.
	Darwin *DarwinSignals
	// Linux is the Linux platform block. It is required on a linux page. On
	// a darwin page it is the vm guest section that guest-init fills, and
	// absent otherwise.
	Linux *LinuxSignals
	// Signals are the signal records, at most MaxPageSignals, one per slot
	// in order.
	Signals []BudgetSignal
	// Admit is true while every signal is below its admit-stop rung.
	Admit bool
	// AdmitReasonSignal names the signal that blocks admission.
	AdmitReasonSignal SignalID
	// EvictDeadline is when the evict-soft grace runs out; the page holds
	// it in whole Unix seconds.
	EvictDeadline *metav1.Time
	// KillMode is the published tier-1 kill mode.
	KillMode KillMode
	// Recommendation is how much more the node can safely take.
	Recommendation BudgetRecommendation
	// Counters are the per-band monotonic totals, at most MaxPageCounters,
	// one per slot in order.
	Counters []PageBandCounters
}

// PageBandCounters is BandCounters without the windowed counts, which stay
// on the NodeBudget (the page carries running totals only).
type PageBandCounters struct {
	// Band is the band counted.
	Band Band
	// EvictionsTotal counts evictions.
	EvictionsTotal int64
	// OOMKillsTotal counts out-of-memory kills.
	OOMKillsTotal int64
	// KernelKillsTotal counts kernel kills the producer did not initiate.
	KernelKillsTotal int64
	// RestartsTotal counts container restarts.
	RestartsTotal int64
	// MemHighEventsTotal counts memory.high events.
	MemHighEventsTotal int64
	// Tier1RefusedTotal counts refused tier-1 deliveries.
	Tier1RefusedTotal int64
}

// NewPageSnapshot returns the page snapshot of an observation: the fields
// the page mirrors, with Generation taken from o.ProducerGeneration. The
// snapshot shares the pointed-to values of o.
func NewPageSnapshot(nodeName string, writerPeriodSeconds int64, o *NodeBudgetObservation) PageSnapshot {
	s := PageSnapshot{
		Generation:          o.ProducerGeneration,
		WriterPeriodSeconds: writerPeriodSeconds,
		NodeName:            nodeName,
		ObservedAt:          o.ObservedAt,
		Platform:            o.Platform,
		Capacity:            o.Capacity,
		Level:               o.Level,
		Darwin:              o.Darwin,
		Linux:               o.Linux,
		Signals:             append([]BudgetSignal(nil), o.Signals...),
		Admit:               o.Admit,
		AdmitReasonSignal:   o.AdmitReasonSignal,
		EvictDeadline:       o.EvictDeadline,
		KillMode:            o.KillMode,
		Recommendation:      o.Recommendation,
	}
	for _, c := range o.Counters {
		s.Counters = append(s.Counters, PageBandCounters{
			Band: c.Band, EvictionsTotal: c.EvictionsTotal, OOMKillsTotal: c.OOMKillsTotal,
			KernelKillsTotal: c.KernelKillsTotal, RestartsTotal: c.RestartsTotal,
			MemHighEventsTotal: c.MemHighEventsTotal, Tier1RefusedTotal: c.Tier1RefusedTotal,
		})
	}
	return s
}

// pageAbsentWord is PageAbsent as a page word.
const pageAbsentWord uint64 = 1 << 63

// pageKind is the encoding of one page field.
type pageKind uint8

const (
	kindBytes pageKind = iota
	kindU16
	kindU32
	kindU64
	kindI64
)

func (k pageKind) String() string {
	return [...]string{"bytes", "u16", "u32", "u64", "i64"}[k]
}

// pageGetter returns a field's 8-byte word (or narrower value) for a
// snapshot, and false when the snapshot holds a value the field cannot
// carry.
type pageGetter func(s *PageSnapshot) (uint64, bool)

// pageField is one row of the layout table, the single definition the
// offsets, the descriptor, the byte budget and the encoder derive from.
type pageField struct {
	name   string     // the layout name, mirroring the status JSON path
	offset int        // computed by buildPageLayout, never written by hand
	kind   pageKind   // the encoding
	size   int        // bytes
	align  int        // a stricter alignment than the natural one, or 0
	enum   string     // a PageCodeTables name, "bool", or ""
	absent bool       // an i64 that may hold PageAbsent
	status string     // the NodeBudget JSON path mirrored, "" when page-only
	get    pageGetter // the value; nil for magic, nodeName and bodySize
}

// pageLayout is the ordered layout table; pageBodyEnd is the offset one past
// its last field.
var pageLayout, pageBodyEnd = buildPageLayout(pageFieldSpecs())

// pageOffset returns the offset of the named field.
func pageOffset(name string) int {
	for _, f := range pageLayout {
		if f.name == name {
			return f.offset
		}
	}
	panic("budgetv1alpha1: no page field " + name) // the table is static; a miss is a programming error
}

// pageBodySize is the bodySize header value of this minor.
func pageBodySize() int { return pageBodyEnd - PageHeaderSize }

func buildPageLayout(specs []pageField) ([]pageField, int) {
	off := 0
	for i := range specs {
		f := &specs[i]
		a := f.align
		if a == 0 {
			switch f.kind {
			case kindU16:
				a = 2
			case kindU32:
				a = 4
			default:
				a = 8
			}
		}
		off = (off + a - 1) / a * a
		f.offset = off
		off += f.size
	}
	return specs, off
}

func constGet(v uint64) pageGetter { return func(*PageSnapshot) (uint64, bool) { return v, true } }

// i64Word encodes an optional i64: nil is PageAbsent, and a present value
// equal to the sentinel cannot be carried.
func i64Word(p *int64) (uint64, bool) {
	if p == nil {
		return pageAbsentWord, true
	}
	return uint64(*p), *p != PageAbsent
}

func boolWord(b bool) (uint64, bool) {
	if b {
		return 1, true
	}
	return 0, true
}

// pageFieldSpecs is the layout in order. Offsets come from the order, the
// sizes and the alignments; nothing here is an offset.
func pageFieldSpecs() []pageField {
	var f []pageField
	add := func(p pageField) {
		if p.size == 0 {
			p.size = 8
		}
		f = append(f, p)
	}
	i64 := func(name, status string, absent bool, get pageGetter) {
		add(pageField{name: name, kind: kindI64, absent: absent, status: status, get: get})
	}
	opt := func(name, status string, get func(s *PageSnapshot) *int64) {
		i64(name, status, true, func(s *PageSnapshot) (uint64, bool) { return i64Word(get(s)) })
	}
	code := func(name, status, enum string, get pageGetter) {
		add(pageField{name: name, kind: kindU64, enum: enum, status: status, get: get})
	}

	// Header line: constant once the page is formatted.
	add(pageField{name: "magic", kind: kindBytes, size: len(PageMagic)})
	add(pageField{name: "abiMajor", kind: kindU16, size: 2, get: constGet(PageABIMajor)})
	add(pageField{name: "abiMinor", kind: kindU16, size: 2, get: constGet(PageABIMinor)})
	add(pageField{name: "headerSize", kind: kindU32, size: 4, get: constGet(PageHeaderSize)})
	// bodySize derives from the table itself, so EncodePage writes it.
	add(pageField{name: "bodySize", kind: kindU32, size: 4})
	// The seqlock word and the notify word, each alone on a cache line.
	add(pageField{name: "seq", kind: kindU64, align: 64, get: constGet(0)})
	add(pageField{name: "notifyEpoch", kind: kindU64, align: 64, get: constGet(0)})

	// Identity. The body, the seqlock region, starts on its own line.
	add(pageField{name: "generation", kind: kindI64, align: 64, status: "status.producerGeneration",
		get: func(s *PageSnapshot) (uint64, bool) { return uint64(s.Generation), true }})
	i64("writerPeriodSeconds", "", false, func(s *PageSnapshot) (uint64, bool) { return uint64(s.WriterPeriodSeconds), true })
	i64("observedAt", "status.observedAt", false, func(s *PageSnapshot) (uint64, bool) {
		return uint64(s.ObservedAt.Unix()), !s.ObservedAt.IsZero()
	})
	code("platform", "status.platform", "platform", func(s *PageSnapshot) (uint64, bool) {
		return pageCode(pagePlatformCodes[:], s.Platform, "")
	})
	add(pageField{name: "nodeNameLen", kind: kindU64, status: "metadata.name", get: func(s *PageSnapshot) (uint64, bool) {
		return uint64(len(s.NodeName)), true
	}})
	add(pageField{name: "nodeName", kind: kindBytes, size: PageNodeNameBytes, status: "metadata.name"})

	// Budget.
	for _, c := range []struct {
		name string
		get  func(*BudgetCapacity) *int64
	}{
		{"cpuCapacityMilli", func(c *BudgetCapacity) *int64 { return c.CPUCapacityMilli }},
		{"cpuSystemReserveMilli", func(c *BudgetCapacity) *int64 { return c.CPUSystemReserveMilli }},
		{"memCapacityBytes", func(c *BudgetCapacity) *int64 { return c.MemCapacityBytes }},
		{"memSystemReserveBytes", func(c *BudgetCapacity) *int64 { return c.MemSystemReserveBytes }},
		{"memEvictionReserveBytes", func(c *BudgetCapacity) *int64 { return c.MemEvictionReserveBytes }},
		{"memAllocatableBytes", func(c *BudgetCapacity) *int64 { return c.MemAllocatableBytes }},
		{"swapTotalBytes", func(c *BudgetCapacity) *int64 { return c.SwapTotalBytes }},
		{"swapUsedBytes", func(c *BudgetCapacity) *int64 { return c.SwapUsedBytes }},
	} {
		opt("capacity."+c.name, "status.capacity."+c.name, func(s *PageSnapshot) *int64 { return c.get(&s.Capacity) })
	}

	// Pressure: the derived level and the platform blocks.
	code("level", "status.level", "level", func(s *PageSnapshot) (uint64, bool) {
		return pageCode(pageLevelCodes[:], s.Level, "")
	})
	darwin := func(get func(*DarwinSignals) *int64) func(*PageSnapshot) *int64 {
		return func(s *PageSnapshot) *int64 {
			if s.Darwin == nil {
				return nil
			}
			return get(s.Darwin)
		}
	}
	compressor := func(get func(*DarwinCompressor) *int64) func(*PageSnapshot) *int64 {
		return darwin(func(d *DarwinSignals) *int64 {
			if d.Compressor == nil {
				return nil
			}
			return get(d.Compressor)
		})
	}
	opt("darwin.vmPressureLevel", "status.darwin.vmPressureLevel", darwin(func(d *DarwinSignals) *int64 { return d.VMPressureLevel }))
	opt("darwin.memorystatusLevelPermille", "status.darwin.memorystatusLevelPermille", darwin(func(d *DarwinSignals) *int64 { return d.MemorystatusLevelPermille }))
	opt("darwin.compressor.bytesUsed", "status.darwin.compressor.bytesUsed", compressor(func(c *DarwinCompressor) *int64 { return c.BytesUsed }))
	opt("darwin.compressor.bytesStored", "status.darwin.compressor.bytesStored", compressor(func(c *DarwinCompressor) *int64 { return c.BytesStored }))
	opt("darwin.compressor.segmentLimitBytes", "status.darwin.compressor.segmentLimitBytes", compressor(func(c *DarwinCompressor) *int64 { return c.SegmentLimitBytes }))
	opt("darwin.swapinsTotal", "status.darwin.swapinsTotal", darwin(func(d *DarwinSignals) *int64 { return d.SwapinsTotal }))
	opt("darwin.swapoutsTotal", "status.darwin.swapoutsTotal", darwin(func(d *DarwinSignals) *int64 { return d.SwapoutsTotal }))

	for _, r := range []struct {
		name string
		get  func(*LinuxPSI) *PSIResource
	}{
		{"cpu", func(p *LinuxPSI) *PSIResource { return p.CPU }},
		{"memory", func(p *LinuxPSI) *PSIResource { return p.Memory }},
		{"io", func(p *LinuxPSI) *PSIResource { return p.IO }},
	} {
		for _, l := range []struct {
			name string
			get  func(*PSIResource) *PSILine
		}{
			{"some", func(r *PSIResource) *PSILine { return r.Some }},
			{"full", func(r *PSIResource) *PSILine { return r.Full }},
		} {
			for _, v := range []struct {
				name string
				get  func(*PSILine) *int64
			}{
				{"avg10Permille", func(l *PSILine) *int64 { return l.Avg10Permille }},
				{"avg60Permille", func(l *PSILine) *int64 { return l.Avg60Permille }},
				{"totalUs", func(l *PSILine) *int64 { return l.TotalUs }},
			} {
				path := "linux.psi." + r.name + "." + l.name + "." + v.name
				opt(path, "status."+path, func(s *PageSnapshot) *int64 {
					if s.Linux == nil || s.Linux.PSI == nil {
						return nil
					}
					res := r.get(s.Linux.PSI)
					if res == nil {
						return nil
					}
					line := l.get(res)
					if line == nil {
						return nil
					}
					return v.get(line)
				})
			}
		}
	}
	for _, e := range []struct {
		name string
		get  func(*LinuxMemoryEvents) *int64
	}{
		{"high", func(e *LinuxMemoryEvents) *int64 { return e.High }},
		{"max", func(e *LinuxMemoryEvents) *int64 { return e.Max }},
		{"oom", func(e *LinuxMemoryEvents) *int64 { return e.OOM }},
		{"oomKill", func(e *LinuxMemoryEvents) *int64 { return e.OOMKill }},
	} {
		path := "linux.memoryEvents." + e.name
		opt(path, "status."+path, func(s *PageSnapshot) *int64 {
			if s.Linux == nil || s.Linux.MemoryEvents == nil {
				return nil
			}
			return e.get(s.Linux.MemoryEvents)
		})
	}

	// Signals: fixed slots. An empty slot (code 0 in id) is all zero bytes.
	for i := range MaxPageSignals {
		p := "signals[" + strconv.Itoa(i) + "]."
		sig := func(s *PageSnapshot) *BudgetSignal {
			if i < len(s.Signals) {
				return &s.Signals[i]
			}
			return nil
		}
		slotCode := func(name, enum string, get func(*BudgetSignal) (uint64, bool)) {
			code(p+name, "status.signals[]."+name, enum, func(s *PageSnapshot) (uint64, bool) {
				if g := sig(s); g != nil {
					return get(g)
				}
				return 0, true
			})
		}
		slotI64 := func(name string, get func(*BudgetSignal) *int64) {
			i64(p+name, "status.signals[]."+name, true, func(s *PageSnapshot) (uint64, bool) {
				if g := sig(s); g != nil {
					return i64Word(get(g))
				}
				return 0, true
			})
		}
		slotCode("id", "signal", func(g *BudgetSignal) (uint64, bool) {
			c := g.ID.PageCode()
			return c, c != 0
		})
		slotCode("unit", "unit", func(g *BudgetSignal) (uint64, bool) {
			c, ok := pageCode(pageUnitCodes[:], g.Unit, "")
			return c, ok && c != 0 && g.Unit == g.ID.Unit()
		})
		slotI64("value", func(g *BudgetSignal) *int64 { return g.Value })
		slotI64("warnThreshold", func(g *BudgetSignal) *int64 { return g.WarnThreshold })
		slotI64("admitStopThreshold", func(g *BudgetSignal) *int64 { return g.AdmitStopThreshold })
		slotI64("criticalThreshold", func(g *BudgetSignal) *int64 { return g.CriticalThreshold })
		slotI64("evictSoftThreshold", func(g *BudgetSignal) *int64 { return g.EvictSoftThreshold })
		slotI64("evictHardThreshold", func(g *BudgetSignal) *int64 { return g.EvictHardThreshold })
		slotI64("warnRelease", func(g *BudgetSignal) *int64 { return g.WarnRelease })
		slotI64("admitStopRelease", func(g *BudgetSignal) *int64 { return g.AdmitStopRelease })
		slotI64("criticalRelease", func(g *BudgetSignal) *int64 { return g.CriticalRelease })
		slotI64("evictSoftRelease", func(g *BudgetSignal) *int64 { return g.EvictSoftRelease })
		slotI64("evictHardRelease", func(g *BudgetSignal) *int64 { return g.EvictHardRelease })
		slotI64("graceSeconds", func(g *BudgetSignal) *int64 { return g.GraceSeconds })
		slotI64("minReclaimBytes", func(g *BudgetSignal) *int64 { return g.MinReclaimBytes })
		slotI64("windowSeconds", func(g *BudgetSignal) *int64 { return g.WindowSeconds })
		slotI64("etaToNextRungSeconds", func(g *BudgetSignal) *int64 { return g.EtaToNextRungSeconds })
		slotCode("trend", "trend", func(g *BudgetSignal) (uint64, bool) {
			return pageCode(pageTrendCodes[:], g.Trend, TrendUnknown)
		})
		slotCode("limitingResource", "limitingResource", func(g *BudgetSignal) (uint64, bool) {
			return pageCode(pageLimitingResourceCodes[:], g.LimitingResource, ResourceUnknown)
		})
		slotCode("sharedWith", "signal", func(g *BudgetSignal) (uint64, bool) {
			c := g.SharedWith.PageCode()
			return c, g.SharedWith == "" || c != 0
		})
	}

	// Action.
	add(pageField{name: "admit", kind: kindU64, enum: "bool", status: "status.admit",
		get: func(s *PageSnapshot) (uint64, bool) { return boolWord(s.Admit) }})
	code("admitReasonSignal", "status.admitReasonSignal", "signal", func(s *PageSnapshot) (uint64, bool) {
		c := s.AdmitReasonSignal.PageCode()
		return c, s.AdmitReasonSignal == "" || c != 0
	})
	i64("evictDeadline", "status.evictDeadline", true, func(s *PageSnapshot) (uint64, bool) {
		if s.EvictDeadline == nil {
			return pageAbsentWord, true
		}
		return uint64(s.EvictDeadline.Unix()), !s.EvictDeadline.IsZero()
	})
	code("killMode.mode", "status.killMode.mode", "killMode", func(s *PageSnapshot) (uint64, bool) {
		return pageCode(pageKillModeCodes[:], s.KillMode.Mode, "")
	})
	add(pageField{name: "killMode.armed", kind: kindU64, enum: "bool", status: "status.killMode.armed",
		get: func(s *PageSnapshot) (uint64, bool) { return boolWord(s.KillMode.Armed) }})
	i64("killMode.armingEpisodes", "status.killMode.armingEpisodes", false, func(s *PageSnapshot) (uint64, bool) {
		return uint64(s.KillMode.ArmingEpisodes), s.KillMode.ArmingEpisodes >= 0
	})
	code("killMode.reason", "status.killMode.reason", "killModeReason", func(s *PageSnapshot) (uint64, bool) {
		return pageCode(pageKillModeReasonCodes[:], s.KillMode.Reason, KillReasonUnknown)
	})

	// Recommendation.
	opt("recommendation.safeAdditionalBytes", "status.recommendation.safeAdditionalBytes",
		func(s *PageSnapshot) *int64 { return s.Recommendation.SafeAdditionalBytes })
	code("recommendation.basis", "status.recommendation.basis", "signal", func(s *PageSnapshot) (uint64, bool) {
		c := s.Recommendation.Basis.PageCode()
		return c, s.Recommendation.Basis == "" || c != 0
	})

	// Counters: fixed slots. An empty slot (code 0 in band) is all zero.
	for i := range MaxPageCounters {
		p := "counters[" + strconv.Itoa(i) + "]."
		ctr := func(s *PageSnapshot) *PageBandCounters {
			if i < len(s.Counters) {
				return &s.Counters[i]
			}
			return nil
		}
		code(p+"band", "status.counters[].band", "band", func(s *PageSnapshot) (uint64, bool) {
			if c := ctr(s); c != nil {
				code, ok := pageCode(pageBandCodes[:], c.Band, "")
				return code, ok && code != 0
			}
			return 0, true
		})
		for _, t := range []struct {
			name string
			get  func(*PageBandCounters) int64
		}{
			{"evictionsTotal", func(c *PageBandCounters) int64 { return c.EvictionsTotal }},
			{"oomKillsTotal", func(c *PageBandCounters) int64 { return c.OOMKillsTotal }},
			{"kernelKillsTotal", func(c *PageBandCounters) int64 { return c.KernelKillsTotal }},
			{"restartsTotal", func(c *PageBandCounters) int64 { return c.RestartsTotal }},
			{"memHighEventsTotal", func(c *PageBandCounters) int64 { return c.MemHighEventsTotal }},
			{"tier1RefusedTotal", func(c *PageBandCounters) int64 { return c.Tier1RefusedTotal }},
		} {
			i64(p+t.name, "status.counters[]."+t.name, false, func(s *PageSnapshot) (uint64, bool) {
				if c := ctr(s); c != nil {
					v := t.get(c)
					return uint64(v), v >= 0
				}
				return 0, true
			})
		}
	}
	return f
}

// pageLayoutHeader opens the descriptor. It is part of every frozen
// descriptor of this major, so it never changes within one.
const pageLayoutHeader = `# k3sm node budget page, ABI major 1: a 4096-byte little-endian file.
# One line per field: <offset> <size> <kind> <absent|-> <enum|-> <name>. A minor only appends lines.
# Fields mirror NodeBudget: the same names and units (seconds, bytes, permille, counts).
# Page-only: magic abiMajor abiMinor headerSize bodySize seq notifyEpoch writerPeriodSeconds.
# NodeBudget-only: conditions, writer, policy, inForce, wouldKill, windowed counts, every string but nodeName.
# Codes are u64, index+1 in the manifest tables, 0 unset or unknown; an absent i64 is -9223372036854775808.
# Signal and counter slots are filled in order; an empty slot (id or band 0) is all zero bytes.
# nodeName: 256 inline bytes after nodeNameLen; a length above 253 is refused by writers and readers.
# Host or guest: platform=darwin with every linux word absent is the host's page; with any linux word present, a vm guest's.
# Seqlock: store seq odd, every body word (headerSize to headerSize+bodySize), seq even, then notifyEpoch.
# notifyEpoch: alone on its line, outside the seqlock region; wait on its low 32-bit word, the 4 bytes at
# its offset; bumped only on a change of level, admit, a threshold or generation; it wraps, so compare
# for inequality only.
`

// PageLayout returns the layout descriptor: one line per field in offset
// order, "<offset> <size> <kind> <absent|-> <enum|-> <name>". A minor
// appends lines and never changes one, so the descriptor of every earlier
// minor of this major is a line prefix of it.
func PageLayout() string {
	var b strings.Builder
	b.WriteString(pageLayoutHeader)
	for _, f := range pageLayout {
		absent, enum := "-", "-"
		if f.absent {
			absent = "absent"
		}
		if f.enum != "" {
			enum = f.enum
		}
		fmt.Fprintf(&b, "%d %d %s %s %s %s\n", f.offset, f.size, f.kind, absent, enum, f.name)
	}
	return b.String()
}

// EncodePage writes the page of s into dst[:PageSize], deterministically:
// the header, seq 0 (even: no write in progress), notifyEpoch 0, the body,
// and zero bytes everywhere else. A live page's seq and notifyEpoch belong
// to PageWriter. dst is fully written only when EncodePage returns nil.
func EncodePage(dst []byte, s *PageSnapshot) error {
	if len(dst) < PageSize {
		return fmt.Errorf("%w: %d bytes, want at least %d", ErrPageBuffer, len(dst), PageSize)
	}
	if err := checkPageSnapshot(s); err != nil {
		return err
	}
	var page [PageSize]byte
	for _, f := range pageLayout {
		switch f.name {
		case "magic":
			copy(page[f.offset:], PageMagic)
			continue
		case "nodeName":
			copy(page[f.offset:f.offset+f.size], s.NodeName)
			continue
		case "bodySize":
			binary.LittleEndian.PutUint32(page[f.offset:], uint32(pageBodySize()))
			continue
		}
		v, ok := f.get(s)
		if !ok {
			return fmt.Errorf("%w: %s cannot carry the snapshot's value", ErrInvalidPageSnapshot, f.name)
		}
		switch f.kind {
		case kindU16:
			binary.LittleEndian.PutUint16(page[f.offset:], uint16(v))
		case kindU32:
			binary.LittleEndian.PutUint32(page[f.offset:], uint32(v))
		default:
			binary.LittleEndian.PutUint64(page[f.offset:], v)
		}
	}
	copy(dst, page[:])
	return nil
}

// checkPageSnapshot holds the snapshot to what the layout can carry; each
// field's getter checks its own value.
func checkPageSnapshot(s *PageSnapshot) error {
	switch {
	case len(s.NodeName) > MaxPageNodeNameBytes:
		return fmt.Errorf("%w: %w (%d bytes)", ErrInvalidPageSnapshot, ErrPageNodeNameTooLong, len(s.NodeName))
	case s.NodeName == "":
		return fmt.Errorf("%w: the node name is empty", ErrInvalidPageSnapshot)
	case s.WriterPeriodSeconds <= 0:
		return fmt.Errorf("%w: writer period %d s, want at least 1", ErrInvalidPageSnapshot, s.WriterPeriodSeconds)
	case !s.Platform.Valid():
		return fmt.Errorf("%w: platform %q is not a known platform", ErrInvalidPageSnapshot, s.Platform)
	case !s.Level.Valid():
		return fmt.Errorf("%w: level %q is not a known level", ErrInvalidPageSnapshot, s.Level)
	case len(s.Signals) > MaxPageSignals:
		return fmt.Errorf("%w: %d signals, at most %d", ErrInvalidPageSnapshot, len(s.Signals), MaxPageSignals)
	case len(s.Counters) > MaxPageCounters:
		return fmt.Errorf("%w: %d counter entries, at most %d", ErrInvalidPageSnapshot, len(s.Counters), MaxPageCounters)
	case s.Platform == PlatformDarwin && s.Darwin == nil:
		return fmt.Errorf("%w: a darwin page needs the darwin block", ErrInvalidPageSnapshot)
	case s.Platform == PlatformLinux && (s.Linux == nil || s.Darwin != nil):
		return fmt.Errorf("%w: a linux page needs the linux block and no darwin block", ErrInvalidPageSnapshot)
	}
	return nil
}

// pageNotifyKey is what PageNotifyDue compares: the level, admit, the
// generation and every rung and release value of every slot.
type pageNotifyKey struct {
	level      Level
	admit      bool
	generation int64
	ladder     [MaxPageSignals][10]int64
}

func notifyKeyOf(s *PageSnapshot) pageNotifyKey {
	k := pageNotifyKey{level: s.Level, admit: s.Admit, generation: s.Generation}
	for i := range k.ladder {
		for j := range k.ladder[i] {
			k.ladder[i][j] = PageAbsent
		}
		if i >= len(s.Signals) {
			continue
		}
		g := &s.Signals[i]
		rungs, releases := g.rungs(), g.releases()
		for j, p := range append(rungs[:], releases[:]...) {
			if p != nil {
				k.ladder[i][j] = *p
			}
		}
	}
	return k
}

// PageNotifyDue reports whether publishing next after prev bumps
// notifyEpoch: a change of level, of admit, of generation, or of any
// threshold (a rung or a release value of any signal slot, including a
// value becoming absent or present). Any other change, a signal's value or
// a counter included, does not; a reader that wants every sample polls. A
// nil prev (the first publication after Resume) is always due.
func PageNotifyDue(prev, next *PageSnapshot) bool {
	if prev == nil {
		return true
	}
	return notifyKeyOf(prev) != notifyKeyOf(next)
}

// PageWriter publishes snapshots into a mapped page under the seqlock.
// There is one writer per page; the caller holds the exclusive writer lock
// outside the page directory. A PageWriter is not safe for concurrent use.
//
// Every store is a sync/atomic 8-byte store, in this order: seq odd, every
// body word, seq even, then, only when PageNotifyDue, notifyEpoch + 1.
// notifyEpoch wraps freely; readers compare it for inequality only.
type PageWriter struct {
	page    []byte
	seq     uint64
	epoch   uint64
	resumed bool
	last    pageNotifyKey
	hasLast bool
}

// NewPageWriter returns a writer over page, the caller's mapping of the
// page file: at least PageSize bytes, 8-byte aligned (a mapping is page
// aligned), on a little-endian host.
func NewPageWriter(page []byte) (*PageWriter, error) {
	if len(page) < PageSize {
		return nil, fmt.Errorf("%w: %d bytes, want at least %d", ErrPageBuffer, len(page), PageSize)
	}
	if uintptr(unsafe.Pointer(&page[0]))%8 != 0 {
		return nil, fmt.Errorf("%w: the mapping is not 8-byte aligned", ErrPageBuffer)
	}
	probe := uint16(1)
	if *(*byte)(unsafe.Pointer(&probe)) != 1 {
		return nil, fmt.Errorf("%w: the host is not little-endian", ErrPageBuffer)
	}
	return &PageWriter{page: page[:PageSize]}, nil
}

func (w *PageWriter) word(off int) *uint64 { return (*uint64)(unsafe.Pointer(&w.page[off])) }

// Resume takes over the page. When it already holds this ABI major, the
// writer continues its seq and notifyEpoch (the page is never replaced, so
// an open mapping keeps working); otherwise seq and notifyEpoch start at
// 0. An odd seq, left by a writer that died mid-write, stays odd until the
// next Publish rewrites the body. Resume then writes this minor's header,
// the magic last.
func (w *PageWriter) Resume() error {
	var hdr [PageSeqOffset]byte
	for off := 0; off < len(hdr); off += 8 {
		binary.LittleEndian.PutUint64(hdr[off:], atomic.LoadUint64(w.word(off)))
	}
	w.seq, w.epoch = 0, 0
	if string(hdr[:len(PageMagic)]) == PageMagic &&
		binary.LittleEndian.Uint16(hdr[pageOffset("abiMajor"):]) == PageABIMajor &&
		binary.LittleEndian.Uint32(hdr[pageOffset("headerSize"):]) == PageHeaderSize {
		w.seq = atomic.LoadUint64(w.word(PageSeqOffset))
		w.epoch = atomic.LoadUint64(w.word(PageNotifyEpochOffset))
		if w.seq%2 == 1 {
			w.seq++
		}
	}
	var fresh [PageSize]byte
	if err := encodeHeader(fresh[:]); err != nil {
		return err
	}
	for off := PageSeqOffset - 8; off >= 0; off -= 8 {
		atomic.StoreUint64(w.word(off), binary.LittleEndian.Uint64(fresh[off:]))
	}
	w.resumed, w.hasLast = true, false
	return nil
}

// encodeHeader writes the header line of this minor into dst.
func encodeHeader(dst []byte) error {
	s := PageSnapshot{NodeName: "-", WriterPeriodSeconds: 1, Platform: PlatformLinux, Level: LevelOK, Linux: &LinuxSignals{}}
	s.ObservedAt = metav1.Unix(1, 0)
	return EncodePage(dst, &s)
}

// Publish writes s to the page under the seqlock and bumps notifyEpoch when
// PageNotifyDue. An invalid snapshot is refused before any store.
func (w *PageWriter) Publish(s *PageSnapshot) error {
	if !w.resumed {
		return ErrPageWriterNotResumed
	}
	var scratch [PageSize]byte
	if err := EncodePage(scratch[:], s); err != nil {
		return err
	}
	key := notifyKeyOf(s)
	due := !w.hasLast || key != w.last
	atomic.StoreUint64(w.word(PageSeqOffset), w.seq+1)
	for off := PageHeaderSize; off < pageBodyEnd; off += 8 {
		atomic.StoreUint64(w.word(off), binary.LittleEndian.Uint64(scratch[off:]))
	}
	w.seq += 2
	atomic.StoreUint64(w.word(PageSeqOffset), w.seq)
	if due {
		w.epoch++
		atomic.StoreUint64(w.word(PageNotifyEpochOffset), w.epoch)
	}
	w.last, w.hasLast = key, true
	return nil
}
