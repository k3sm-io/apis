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
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// pageDir holds the page fixtures, the layout descriptors, the manifest and
// SHA256SUMS.
var pageDir = filepath.Join("testdata", "page")

// pageWriterPeriodSeconds is the writer period every fixture is stamped
// with.
const pageWriterPeriodSeconds = 1

// pageSampleSnapshot is the page snapshot of one status golden sample.
func pageSampleSnapshot(p Platform) PageSnapshot {
	nb := samples()[p]
	return NewPageSnapshot(nb.Name, pageWriterPeriodSeconds, &nb.Status.NodeBudgetObservation)
}

// pageFixtureDef is one decodable fixture and the snapshot it encodes.
type pageFixtureDef struct {
	file, desc string
	snap       func() PageSnapshot
}

// pageFixtureDefs are the decodable fixtures, each built from the samples
// the status goldens are built from.
func pageFixtureDefs() []pageFixtureDef {
	return []pageFixtureDef{
		{"darwin.v1.page", "the Darwin status golden (sampleDarwin) as a native Darwin page", func() PageSnapshot {
			return pageSampleSnapshot(PlatformDarwin)
		}},
		{"linux-producer.v1.page", "the Linux status golden (sampleLinux) as a Linux producer's page", func() PageSnapshot {
			return pageSampleSnapshot(PlatformLinux)
		}},
		{"linux-guest.v1.page", "a vm guest page: the Darwin host's page relayed to the guest, with the linux block as the guest section (sampleLinux's)", func() PageSnapshot {
			s := pageSampleSnapshot(PlatformDarwin)
			s.Linux = samples()[PlatformLinux].Status.Linux
			return s
		}},
		{"absent.v1.page", "the Darwin page with every optional value absent (PageAbsent), no signals and no counters", func() PageSnapshot {
			s := pageSampleSnapshot(PlatformDarwin)
			s.Capacity = BudgetCapacity{}
			s.Darwin = &DarwinSignals{}
			s.Signals = nil
			s.Counters = nil
			s.Recommendation = BudgetRecommendation{}
			s.AdmitReasonSignal = ""
			s.EvictDeadline = nil
			return s
		}},
	}
}

// pagePatternWord is the word the pattern fixture holds at offset off.
func pagePatternWord(off int) uint64 { return 0x5041_5454_0000_0000 | uint64(off) }

// pagePattern is the raw fixture: a valid header, every u64 and i64 field
// (seq and notifyEpoch included) holding 0x50415454<<32 | its own offset,
// nodeName's bytes each holding the low byte of their own offset, and zero
// elsewhere. An SDK checks its offsets against it.
func pagePattern(t *testing.T) []byte {
	b := mustEncode(t, pageSampleSnapshot(PlatformDarwin))
	for _, f := range pageLayout {
		switch {
		case f.offset < PageSeqOffset:
		case f.kind == kindBytes:
			for p := f.offset; p < f.offset+f.size; p++ {
				b[p] = byte(p)
			}
		default:
			binary.LittleEndian.PutUint64(b[f.offset:], pagePatternWord(f.offset))
		}
	}
	return b
}

func mustEncode(t *testing.T, s PageSnapshot) []byte {
	t.Helper()
	b := make([]byte, PageSize)
	if err := EncodePage(b, &s); err != nil {
		t.Fatalf("EncodePage: %v", err)
	}
	return b
}

// pageNegatives forges each negative case from the Darwin page.
func pageNegatives(t *testing.T) map[string][]byte {
	base := mustEncode(t, pageSampleSnapshot(PlatformDarwin))
	forge := func(edit func(b []byte) []byte) []byte {
		return edit(append([]byte(nil), base...))
	}
	le := binary.LittleEndian
	return map[string][]byte{
		"short": forge(func(b []byte) []byte { return b[:1024] }),
		"bad-magic": forge(func(b []byte) []byte {
			copy(b, "K3SMBDGX")
			return b
		}),
		"unknown-major": forge(func(b []byte) []byte {
			le.PutUint16(b[pageOffset("abiMajor"):], PageABIMajor+1)
			return b
		}),
		"body-size-below-known": forge(func(b []byte) []byte {
			le.PutUint32(b[pageOffset("bodySize"):], uint32(pageBodySize()-8))
			return b
		}),
		"odd-seq": forge(func(b []byte) []byte {
			le.PutUint64(b[PageSeqOffset:], 1)
			return b
		}),
		"node-name-too-long": forge(func(b []byte) []byte {
			le.PutUint64(b[pageOffset("nodeNameLen"):], MaxPageNodeNameBytes+1)
			copy(b[pageOffset("nodeName"):], strings.Repeat("a", MaxPageNodeNameBytes+1))
			return b
		}),
	}
}

// pageNotifyVectors are PageNotifyDue's transitions from the Darwin page.
var pageNotifyVectors = []PageNotifyVector{
	{Field: "level", Value: 2, Due: true},
	{Field: "admit", Value: 0, Due: true},
	{Field: "generation", Value: 5, Due: true},
	{Field: "signals[0].warnThreshold", Value: 3 * gib, Due: true},
	{Field: "signals[1].criticalRelease", Value: 660, Due: true},
	{Field: "signals[3].evictSoftThreshold", Value: 14 * gib, Due: true},
	{Field: "signals[4].evictHardRelease", Value: PageAbsent, Due: true},
	{Field: "signals[0].value", Value: 5 * gib, Due: false},
	{Field: "signals[0].trend", Value: 3, Due: false},
	{Field: "observedAt", Value: sampleAt.Unix() + 1, Due: false},
	{Field: "capacity.swapUsedBytes", Value: 2 * gib, Due: false},
	{Field: "darwin.vmPressureLevel", Value: 2, Due: false},
	{Field: "counters[2].evictionsTotal", Value: 6, Due: false},
	{Field: "killMode.armingEpisodes", Value: 3, Due: false},
	{Field: "recommendation.safeAdditionalBytes", Value: 2 * gib, Due: false},
}

// pageFiles returns every generated file under testdata/page but the layout
// descriptors, the manifest and SHA256SUMS.
func pageFiles(t *testing.T) map[string][]byte {
	files := map[string][]byte{"pattern.v1.page": pagePattern(t)}
	for _, d := range pageFixtureDefs() {
		files[d.file] = mustEncode(t, d.snap())
	}
	for name, b := range pageNegatives(t) {
		files["negative/"+name+".v1.page"] = b
	}
	return files
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// pageManifestJSON is the manifest of files, as committed.
func pageManifestJSON(t *testing.T, files map[string][]byte) []byte {
	m := NewPageManifest()
	for _, d := range pageFixtureDefs() {
		m.Fixtures = append(m.Fixtures, PageFixture{File: d.file, SHA256: sha256Hex(files[d.file]), Decodes: true, Description: d.desc})
	}
	m.Fixtures = append(m.Fixtures, PageFixture{
		File: "pattern.v1.page", SHA256: sha256Hex(files["pattern.v1.page"]),
		Description: "raw: a valid header; every u64 and i64 word, seq and notifyEpoch included, holds 0x50415454<<32 | its own offset; each nodeName byte holds the low byte of its own offset; every other byte is zero",
	})
	for i := range m.Negative {
		m.Negative[i].SHA256 = sha256Hex(files[m.Negative[i].File])
	}
	for _, v := range pageNotifyVectors {
		v.Base = "darwin.v1.page"
		m.NotifyVectors = append(m.NotifyVectors, v)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

// pageSums is SHA256SUMS over every file under testdata/page but itself.
func pageSums(t *testing.T) []byte {
	var lines []string
	err := filepath.WalkDir(pageDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(pageDir, path)
		if err != nil || rel == "SHA256SUMS" {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines = append(lines, sha256Hex(b)+"  "+filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", pageDir, err)
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i][66:] < lines[j][66:] })
	return []byte(strings.Join(lines, "\n") + "\n")
}

func writePageFile(t *testing.T, rel string, b []byte) {
	t.Helper()
	path := filepath.Join(pageDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("update golden: %v", err)
	}
}

func readPageFile(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(pageDir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// TestPageLayoutGoldenFixture pins the page ABI: the fixture bytes, their
// digests, the offsets, the frozen layout descriptors, the byte budget, the
// negative set, the round trip, the content boundary, the parity with the
// NodeBudget status, and the notify predicate. Regenerate the fixtures
// deliberately with UPDATE_GOLDEN=1; it never rewrites an existing layout
// descriptor, so a layout change without an ABI bump stays red.
func TestPageLayoutGoldenFixture(t *testing.T) {
	files := pageFiles(t)
	layoutFile := pageLayoutFile(PageABIMinor)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		for rel, b := range files {
			writePageFile(t, rel, b)
		}
		if _, err := os.Stat(filepath.Join(pageDir, layoutFile)); errors.Is(err, fs.ErrNotExist) {
			writePageFile(t, layoutFile, []byte(PageLayout()))
		}
		writePageFile(t, "manifest.json", pageManifestJSON(t, files))
		writePageFile(t, "SHA256SUMS", pageSums(t))
	}

	t.Run("golden bytes exact per fixture", func(t *testing.T) {
		names := make([]string, 0, len(files))
		for rel := range files {
			names = append(names, rel)
		}
		sort.Strings(names)
		for _, rel := range names {
			t.Run(rel, func(t *testing.T) {
				if got := readPageFile(t, rel); !bytes.Equal(files[rel], got) {
					t.Fatalf("%s differs from its encoding (%d bytes on disk, %d encoded)", rel, len(got), len(files[rel]))
				}
			})
		}
	})

	t.Run("SHA256SUMS and manifest match", func(t *testing.T) {
		if got, want := readPageFile(t, "manifest.json"), pageManifestJSON(t, files); !bytes.Equal(got, want) {
			t.Fatalf("manifest.json is stale\n got = %s\nwant = %s", got, want)
		}
		if got, want := readPageFile(t, "SHA256SUMS"), pageSums(t); !bytes.Equal(got, want) {
			t.Fatalf("SHA256SUMS is stale\n got = %s\nwant = %s", got, want)
		}
		var m PageManifest
		if err := json.Unmarshal(readPageFile(t, "manifest.json"), &m); err != nil {
			t.Fatal(err)
		}
		listed := map[string]bool{m.Layout: true}
		for _, f := range m.Fixtures {
			listed[f.File] = true
		}
		for _, n := range m.Negative {
			listed[n.File] = true
		}
		for _, line := range strings.Split(strings.TrimSpace(string(readPageFile(t, "SHA256SUMS"))), "\n") {
			rel := line[66:]
			if strings.HasSuffix(rel, ".page") && !listed[rel] {
				t.Errorf("%s is not in the manifest", rel)
			}
		}
		if m.PageFileName != "budget.v"+strconv.Itoa(PageABIMajor)+".page" ||
			!strings.HasSuffix(m.DefaultPagePath, "/"+m.PageFileName) || !strings.HasSuffix(m.LinuxPagePath, "/"+m.PageFileName) {
			t.Errorf("the page file name and paths do not carry the ABI major: %+v", m)
		}
	})

	t.Run("offsets equal the literal list and alignment holds", func(t *testing.T) {
		want := map[string]int{
			"magic": 0, "abiMajor": 8, "abiMinor": 10, "headerSize": 12, "bodySize": 16,
			"seq": 64, "notifyEpoch": 128,
			"generation": 192, "writerPeriodSeconds": 200, "observedAt": 208, "platform": 216,
			"nodeNameLen": 224, "nodeName": 232,
			"capacity.cpuCapacityMilli": 488, "capacity.cpuSystemReserveMilli": 496,
			"capacity.memCapacityBytes": 504, "capacity.memSystemReserveBytes": 512,
			"capacity.memEvictionReserveBytes": 520, "capacity.memAllocatableBytes": 528,
			"capacity.swapTotalBytes": 536, "capacity.swapUsedBytes": 544,
			"level":                  552,
			"darwin.vmPressureLevel": 560, "darwin.memorystatusLevelPermille": 568,
			"darwin.compressor.bytesUsed": 576, "darwin.compressor.bytesStored": 584,
			"darwin.compressor.segmentLimitBytes": 592, "darwin.swapinsTotal": 600, "darwin.swapoutsTotal": 608,
			"linux.psi.cpu.some.avg10Permille": 616, "linux.psi.cpu.some.avg60Permille": 624, "linux.psi.cpu.some.totalUs": 632,
			"linux.psi.cpu.full.avg10Permille": 640, "linux.psi.cpu.full.avg60Permille": 648, "linux.psi.cpu.full.totalUs": 656,
			"linux.psi.memory.some.avg10Permille": 664, "linux.psi.memory.some.avg60Permille": 672, "linux.psi.memory.some.totalUs": 680,
			"linux.psi.memory.full.avg10Permille": 688, "linux.psi.memory.full.avg60Permille": 696, "linux.psi.memory.full.totalUs": 704,
			"linux.psi.io.some.avg10Permille": 712, "linux.psi.io.some.avg60Permille": 720, "linux.psi.io.some.totalUs": 728,
			"linux.psi.io.full.avg10Permille": 736, "linux.psi.io.full.avg60Permille": 744, "linux.psi.io.full.totalUs": 752,
			"linux.memoryEvents.high": 760, "linux.memoryEvents.max": 768,
			"linux.memoryEvents.oom": 776, "linux.memoryEvents.oomKill": 784,
			"admit": 2072, "admitReasonSignal": 2080, "evictDeadline": 2088,
			"killMode.mode": 2096, "killMode.armed": 2104, "killMode.armingEpisodes": 2112, "killMode.reason": 2120,
			"recommendation.safeAdditionalBytes": 2128, "recommendation.basis": 2136,
		}
		signal0 := map[string]int{
			"id": 792, "unit": 800, "value": 808,
			"warnThreshold": 816, "admitStopThreshold": 824, "criticalThreshold": 832,
			"evictSoftThreshold": 840, "evictHardThreshold": 848,
			"warnRelease": 856, "admitStopRelease": 864, "criticalRelease": 872,
			"evictSoftRelease": 880, "evictHardRelease": 888,
			"graceSeconds": 896, "minReclaimBytes": 904, "windowSeconds": 912, "etaToNextRungSeconds": 920,
			"trend": 928, "limitingResource": 936, "sharedWith": 944,
		}
		const signalStride = 160
		for i := range MaxPageSignals {
			for name, off := range signal0 {
				want["signals["+strconv.Itoa(i)+"]."+name] = off + i*signalStride
			}
		}
		counter0 := map[string]int{
			"band": 2144, "evictionsTotal": 2152, "oomKillsTotal": 2160, "kernelKillsTotal": 2168,
			"restartsTotal": 2176, "memHighEventsTotal": 2184, "tier1RefusedTotal": 2192,
		}
		const counterStride = 56
		for i := range MaxPageCounters {
			for name, off := range counter0 {
				want["counters["+strconv.Itoa(i)+"]."+name] = off + i*counterStride
			}
		}
		got := map[string]int{}
		for _, f := range pageLayout {
			got[f.name] = f.offset
		}
		if !reflect.DeepEqual(got, want) {
			for name, off := range want {
				if got[name] != off {
					t.Errorf("%s at %d, want %d", name, got[name], off)
				}
			}
			for name := range got {
				if _, ok := want[name]; !ok {
					t.Errorf("%s is not in the literal list", name)
				}
			}
		}
		if pageBodyEnd != 2312 {
			t.Errorf("the body ends at %d, want 2312", pageBodyEnd)
		}
		for name, c := range map[string]int{
			"seq": PageSeqOffset, "notifyEpoch": PageNotifyEpochOffset, "generation": PageHeaderSize,
		} {
			if pageOffset(name) != c {
				t.Errorf("%s at %d, its constant says %d", name, pageOffset(name), c)
			}
		}
		end := 0
		for _, f := range pageLayout {
			if f.offset < end {
				t.Errorf("%s at %d overlaps the field before it (ends %d)", f.name, f.offset, end)
			}
			end = f.offset + f.size
			if f.offset >= PageSeqOffset && (f.offset%8 != 0 || f.size%8 != 0) {
				t.Errorf("%s at %d, %d bytes: every word past the header line must be 8-byte aligned whole words", f.name, f.offset, f.size)
			}
			if f.absent && f.kind != kindI64 {
				t.Errorf("%s may be absent but is %s; only i64 fields carry PageAbsent", f.name, f.kind)
			}
			line := f.offset / 64
			if (line == 1 && f.name != "seq") || (line == 2 && f.name != "notifyEpoch") {
				t.Errorf("%s shares the line of seq or notifyEpoch", f.name)
			}
		}
	})

	t.Run("descriptor equals the frozen layout and frozen layouts are prefixes", func(t *testing.T) {
		cur := PageLayout()
		if got := string(readPageFile(t, layoutFile)); got != cur {
			t.Fatalf("the layout changed without an ABI bump: %s differs from PageLayout()\n got = %s\nwant = %s", layoutFile, got, cur)
		}
		frozen, err := filepath.Glob(filepath.Join(pageDir, "layout.v"+strconv.Itoa(PageABIMajor)+".*.txt"))
		if err != nil || len(frozen) == 0 {
			t.Fatalf("no frozen layout descriptors: %v", err)
		}
		for _, path := range frozen {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(cur, string(b)) || (len(b) > 0 && b[len(b)-1] != '\n') {
				t.Errorf("%s is not a line prefix of the current layout: a minor may only append fields", path)
			}
		}
	})

	t.Run("byte budget", func(t *testing.T) {
		if PageSize != 4096 || PageBodyLimit*4 != PageSize*3 {
			t.Fatalf("PageSize %d, PageBodyLimit %d: the limit keeps a quarter of the page free", PageSize, PageBodyLimit)
		}
		if pageBodyEnd > PageBodyLimit {
			t.Fatalf("the last field ends at %d, beyond the %d budget", pageBodyEnd, PageBodyLimit)
		}
		t.Logf("layout: %d fields, body ends at %d of %d (%d bytes of headroom)", len(pageLayout), pageBodyEnd, PageSize, PageSize-pageBodyEnd)
		covered := make([]bool, PageSize)
		for _, f := range pageLayout {
			for p := f.offset; p < f.offset+f.size; p++ {
				covered[p] = true
			}
		}
		for _, d := range pageFixtureDefs() {
			b := files[d.file]
			if len(b) != PageSize {
				t.Errorf("%s is %d bytes, want %d", d.file, len(b), PageSize)
			}
			for p, c := range b {
				if !covered[p] && c != 0 {
					t.Errorf("%s: reserved byte %d is %#x, want 0", d.file, p, c)
					break
				}
			}
		}
	})

	t.Run("negative set maps to manifest errors", func(t *testing.T) {
		m := NewPageManifest()
		if len(m.Negative) != len(pageNegatives(t)) {
			t.Fatalf("%d manifest cases, %d forged pages", len(m.Negative), len(pageNegatives(t)))
		}
		for _, c := range m.Negative {
			t.Run(c.Name, func(t *testing.T) {
				_, err := decodePage(readPageFile(t, c.File))
				var re *pageReadError
				if !errors.As(err, &re) || re.name != c.Error {
					t.Fatalf("decode %s: %v, want %s", c.File, err, c.Error)
				}
			})
		}
		t.Run("encoder refuses a node name over 253 bytes", func(t *testing.T) {
			s := pageSampleSnapshot(PlatformDarwin)
			s.NodeName = strings.Repeat("a", MaxPageNodeNameBytes+1)
			err := EncodePage(make([]byte, PageSize), &s)
			if !errors.Is(err, ErrPageNodeNameTooLong) || !errors.Is(err, ErrInvalidPageSnapshot) {
				t.Fatalf("EncodePage(254-byte name) = %v, want %v", err, ErrPageNodeNameTooLong)
			}
			s.NodeName = strings.Repeat("a", MaxPageNodeNameBytes)
			b := mustEncode(t, s)
			if got, err := decodePage(b); err != nil || got.NodeName != s.NodeName {
				t.Fatalf("a 253-byte name does not round-trip: %v", err)
			}
		})
		t.Run("encoder refuses what the page cannot carry", func(t *testing.T) {
			for name, edit := range map[string]func(*PageSnapshot){
				"empty node name":           func(s *PageSnapshot) { s.NodeName = "" },
				"unknown level":             func(s *PageSnapshot) { s.Level = "calm" },
				"unknown platform":          func(s *PageSnapshot) { s.Platform = "plan9" },
				"darwin page without block": func(s *PageSnapshot) { s.Darwin = nil },
				"linux page with darwin": func(s *PageSnapshot) {
					s.Platform, s.Linux = PlatformLinux, &LinuxSignals{}
				},
				"nine signals": func(s *PageSnapshot) {
					s.Signals = append(s.Signals, s.Signals[0], s.Signals[1])
				},
				"four counters": func(s *PageSnapshot) {
					s.Counters = append(s.Counters, s.Counters[0])
				},
				"value equal to the sentinel": func(s *PageSnapshot) { s.Signals[0].Value = i64(PageAbsent) },
				"unit not the signal's":       func(s *PageSnapshot) { s.Signals[0].Unit = UnitCount },
				"unknown signal":              func(s *PageSnapshot) { s.Signals[0].ID = "cpu.steal" },
				"unknown band":                func(s *PageSnapshot) { s.Counters[0].Band = "gold" },
				"negative counter":            func(s *PageSnapshot) { s.Counters[1].RestartsTotal = -1 },
				"zero observedAt":             func(s *PageSnapshot) { s.ObservedAt = metav1.Time{} },
			} {
				s := pageSampleSnapshot(PlatformDarwin)
				s.Signals = append([]BudgetSignal(nil), s.Signals...)
				s.Counters = append([]PageBandCounters(nil), s.Counters...)
				edit(&s)
				if err := EncodePage(make([]byte, PageSize), &s); !errors.Is(err, ErrInvalidPageSnapshot) {
					t.Errorf("%s: EncodePage = %v, want ErrInvalidPageSnapshot", name, err)
				}
			}
			if err := EncodePage(make([]byte, PageSize-1), &PageSnapshot{}); !errors.Is(err, ErrPageBuffer) {
				t.Errorf("a short dst: %v, want ErrPageBuffer", err)
			}
		})
	})

	t.Run("round trip", func(t *testing.T) {
		for _, d := range pageFixtureDefs() {
			t.Run(d.file, func(t *testing.T) {
				want := d.snap()
				got, err := decodePage(readPageFile(t, d.file))
				if err != nil {
					t.Fatalf("decode: %v", err)
				}
				if !reflect.DeepEqual(got, want) {
					gj, _ := json.Marshal(got)
					wj, _ := json.Marshal(want)
					t.Fatalf("decode(encode(s)) != s\n got = %s\nwant = %s", gj, wj)
				}
				if again := mustEncode(t, got); !bytes.Equal(again, files[d.file]) {
					t.Fatal("encode(decode(page)) != page")
				}
			})
		}
		t.Run("pattern.v1.page", func(t *testing.T) {
			b := readPageFile(t, "pattern.v1.page")
			for _, f := range pageLayout {
				if f.offset >= PageSeqOffset && f.kind != kindBytes {
					if got := binary.LittleEndian.Uint64(b[f.offset:]); got != pagePatternWord(f.offset) {
						t.Errorf("%s holds %#x, want %#x", f.name, got, pagePatternWord(f.offset))
					}
				}
			}
		})
	})

	t.Run("identity sensitivity", func(t *testing.T) {
		identityRE := regexp.MustCompile(`(?i)pod|namespace|uid|container|image|victim`)
		got := map[string]bool{}
		var freeStrings []string
		pageSnapshotPaths(reflect.TypeOf(PageSnapshot{}), "", got, &freeStrings)
		want := pageSnapshotAllowlist()
		if !reflect.DeepEqual(got, want) {
			for p := range got {
				if !want[p] {
					t.Errorf("snapshot field %s is not on the allowlist: a page field is node-level data only", p)
				}
			}
			for p := range want {
				if !got[p] {
					t.Errorf("allowlisted %s is gone from the snapshot", p)
				}
			}
		}
		if !reflect.DeepEqual(freeStrings, []string{"NodeName"}) {
			t.Errorf("free strings %v, want only NodeName", freeStrings)
		}
		for p := range got {
			if identityRE.MatchString(p) {
				t.Errorf("snapshot field %s suggests pod identity", p)
			}
		}
		for _, f := range pageLayout {
			if identityRE.MatchString(f.name) {
				t.Errorf("page field %s suggests pod identity", f.name)
			}
		}
		t.Run("the check is not vacuous", func(t *testing.T) {
			type leaky struct {
				PageSnapshot
				VictimPod string
			}
			got := map[string]bool{}
			var free []string
			pageSnapshotPaths(reflect.TypeOf(leaky{}), "", got, &free)
			if !got["VictimPod"] || !identityRE.MatchString("VictimPod") || len(free) != 2 {
				t.Fatalf("a leaky snapshot passes: paths %d, free %v", len(got), free)
			}
		})
	})

	t.Run("channel parity", func(t *testing.T) {
		statusPaths := map[string]bool{}
		jsonPaths(reflect.TypeOf(NodeBudget{}), "", statusPaths)
		has := func(p string) bool { return statusPaths["."+p] }
		pageOnly := map[string]bool{
			"magic": true, "abiMajor": true, "abiMinor": true, "headerSize": true, "bodySize": true,
			"seq": true, "notifyEpoch": true, "writerPeriodSeconds": true,
		}
		crdOnly := []string{
			"status.conditions", "status.writer", "status.policy", "status.inForce",
			"status.wouldKill", "status.counters[].windows",
		}
		mapped := map[string]bool{}
		for _, f := range pageLayout {
			switch {
			case f.status == "" && !pageOnly[f.name]:
				t.Errorf("%s maps to no status field and is not in the page-only set", f.name)
			case f.status != "" && pageOnly[f.name]:
				t.Errorf("%s is in the page-only set but maps to %s", f.name, f.status)
			case f.status != "" && !has(f.status):
				t.Errorf("%s maps to %s, which NodeBudget does not have", f.name, f.status)
			}
			mapped[f.status] = true
		}
		for name := range pageOnly {
			if !slicesContainsField(name) {
				t.Errorf("page-only %s is not a page field", name)
			}
		}
		for p := range statusPaths {
			p = strings.TrimPrefix(p, ".")
			if !strings.HasPrefix(p, "status.") || mapped[p] {
				continue
			}
			crd := false
			for _, c := range crdOnly {
				crd = crd || p == c || strings.HasPrefix(p, c+".") || strings.HasPrefix(p, c+"[]")
			}
			if !crd {
				t.Errorf("status field %s is neither on the page nor in the CRD-only set", p)
			}
		}
		// The fixtures carry exactly what the committed status goldens say.
		for p, file := range map[Platform]string{PlatformDarwin: "darwin.v1.page", PlatformLinux: "linux-producer.v1.page"} {
			var nb NodeBudget
			if err := json.Unmarshal(must(os.ReadFile(goldenStatusFiles[p])), &nb); err != nil {
				t.Fatal(err)
			}
			want := NewPageSnapshot(nb.Name, pageWriterPeriodSeconds, &nb.Status.NodeBudgetObservation)
			want.ObservedAt = metav1.NewTime(want.ObservedAt.UTC())
			if want.EvictDeadline != nil {
				d := metav1.NewTime(want.EvictDeadline.UTC())
				want.EvictDeadline = &d
			}
			got, err := decodePage(readPageFile(t, file))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s does not carry what %s says", file, goldenStatusFiles[p])
			}
		}
	})

	t.Run("PageNotifyDue vectors", func(t *testing.T) {
		base := readPageFile(t, "darwin.v1.page")
		prev, err := decodePage(base)
		if err != nil {
			t.Fatal(err)
		}
		if !PageNotifyDue(nil, &prev) || PageNotifyDue(&prev, &prev) {
			t.Fatal("the first publication must be due, and an unchanged one not")
		}
		for _, v := range pageNotifyVectors {
			t.Run(v.Field, func(t *testing.T) {
				b := append([]byte(nil), base...)
				binary.LittleEndian.PutUint64(b[pageOffset(v.Field):], uint64(v.Value))
				next, err := decodePage(b)
				if err != nil {
					t.Fatal(err)
				}
				if reflect.DeepEqual(prev, next) {
					t.Fatal("the vector changes nothing")
				}
				if got := PageNotifyDue(&prev, &next); got != v.Due {
					t.Fatalf("PageNotifyDue = %v, want %v", got, v.Due)
				}
			})
		}
	})
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func slicesContainsField(name string) bool {
	for _, f := range pageLayout {
		if f.name == name {
			return true
		}
	}
	return false
}

// pageSnapshotPaths lists the exported leaf field paths reachable from t
// and the paths of plain (non-enum) strings.
func pageSnapshotPaths(t reflect.Type, prefix string, out map[string]bool, free *[]string) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		if t.Kind() == reflect.Slice {
			prefix += "[]"
		}
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t == reflect.TypeOf(metav1.Time{}) {
		out[prefix] = true
		if t.Kind() == reflect.String && t.PkgPath() == "" {
			*free = append(*free, prefix)
		}
		return
	}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		p := f.Name
		if prefix != "" {
			p = prefix + "." + f.Name
		}
		if f.Anonymous {
			p = prefix
		}
		pageSnapshotPaths(f.Type, p, out, free)
	}
	sort.Strings(*free)
}

// pageSnapshotAllowlist is every exported field of PageSnapshot, written
// out: a new one fails until it is reviewed as node-level data.
func pageSnapshotAllowlist() map[string]bool {
	out := map[string]bool{}
	for _, p := range []string{
		"Generation", "WriterPeriodSeconds", "NodeName", "ObservedAt", "Platform",
		"Capacity.CPUCapacityMilli", "Capacity.CPUSystemReserveMilli", "Capacity.MemCapacityBytes",
		"Capacity.MemSystemReserveBytes", "Capacity.MemEvictionReserveBytes", "Capacity.MemAllocatableBytes",
		"Capacity.SwapTotalBytes", "Capacity.SwapUsedBytes",
		"Level",
		"Darwin.VMPressureLevel", "Darwin.MemorystatusLevelPermille", "Darwin.Compressor.BytesUsed",
		"Darwin.Compressor.BytesStored", "Darwin.Compressor.SegmentLimitBytes", "Darwin.SwapinsTotal", "Darwin.SwapoutsTotal",
		"Linux.PSI.CPU.Some.Avg10Permille", "Linux.PSI.CPU.Some.Avg60Permille", "Linux.PSI.CPU.Some.TotalUs",
		"Linux.PSI.CPU.Full.Avg10Permille", "Linux.PSI.CPU.Full.Avg60Permille", "Linux.PSI.CPU.Full.TotalUs",
		"Linux.PSI.Memory.Some.Avg10Permille", "Linux.PSI.Memory.Some.Avg60Permille", "Linux.PSI.Memory.Some.TotalUs",
		"Linux.PSI.Memory.Full.Avg10Permille", "Linux.PSI.Memory.Full.Avg60Permille", "Linux.PSI.Memory.Full.TotalUs",
		"Linux.PSI.IO.Some.Avg10Permille", "Linux.PSI.IO.Some.Avg60Permille", "Linux.PSI.IO.Some.TotalUs",
		"Linux.PSI.IO.Full.Avg10Permille", "Linux.PSI.IO.Full.Avg60Permille", "Linux.PSI.IO.Full.TotalUs",
		"Linux.MemoryEvents.High", "Linux.MemoryEvents.Max", "Linux.MemoryEvents.OOM", "Linux.MemoryEvents.OOMKill",
		"Signals[].ID", "Signals[].Unit", "Signals[].Value",
		"Signals[].WarnThreshold", "Signals[].AdmitStopThreshold", "Signals[].CriticalThreshold",
		"Signals[].EvictSoftThreshold", "Signals[].EvictHardThreshold",
		"Signals[].WarnRelease", "Signals[].AdmitStopRelease", "Signals[].CriticalRelease",
		"Signals[].EvictSoftRelease", "Signals[].EvictHardRelease",
		"Signals[].GraceSeconds", "Signals[].MinReclaimBytes", "Signals[].WindowSeconds", "Signals[].EtaToNextRungSeconds",
		"Signals[].Trend", "Signals[].LimitingResource", "Signals[].SharedWith",
		"Admit", "AdmitReasonSignal", "EvictDeadline",
		"KillMode.Mode", "KillMode.Armed", "KillMode.ArmingEpisodes", "KillMode.Reason",
		"Recommendation.SafeAdditionalBytes", "Recommendation.Basis",
		"Counters[].Band", "Counters[].EvictionsTotal", "Counters[].OOMKillsTotal", "Counters[].KernelKillsTotal",
		"Counters[].RestartsTotal", "Counters[].MemHighEventsTotal", "Counters[].Tier1RefusedTotal",
	} {
		out[p] = true
	}
	return out
}

// TestPageCodeTables asserts each page code table against the
// vocabulary: every coded value is valid, none is repeated, the "unknown"
// values (code 0) are not coded, and every other value is.
func TestPageCodeTables(t *testing.T) {
	t.Parallel()
	want := map[string][]string{
		"signal":           stringsOf(Signals()),
		"unit":             {string(UnitBytes), string(UnitPermille), string(UnitCount)},
		"level":            {string(LevelOK), string(LevelWarn), string(LevelCritical), string(LevelEvicting)},
		"band":             stringsOf(Bands()),
		"platform":         {string(PlatformDarwin), string(PlatformLinux)},
		"killMode":         {string(KillModeAuto), string(KillModeOn), string(KillModeOff)},
		"trend":            {string(TrendFalling), string(TrendSteady), string(TrendRising)},
		"limitingResource": {"memory", "swapfs", "nodefs", "imagefs", "pid", "cpu"},
		"killModeReason":   {"cold-start", "observing", "armed", "policy-off", "disarm-file"},
	}
	valid := map[string]func(string) bool{
		"signal":           func(v string) bool { return SignalID(v).Valid() },
		"unit":             func(v string) bool { return SignalUnit(v).Valid() },
		"level":            func(v string) bool { return Level(v).Valid() },
		"band":             func(v string) bool { return Band(v).Valid() },
		"platform":         func(v string) bool { return Platform(v).Valid() },
		"killMode":         func(v string) bool { return KillModeName(v).Valid() },
		"trend":            func(v string) bool { return Trend(v).Valid() && Trend(v) != TrendUnknown },
		"limitingResource": func(v string) bool { return LimitingResource(v).Valid() && LimitingResource(v) != ResourceUnknown },
		"killModeReason":   func(v string) bool { return KillModeReason(v).Valid() && KillModeReason(v) != KillReasonUnknown },
	}
	got := PageCodeTables()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("code tables\n got = %v\nwant = %v", got, want)
	}
	for name, table := range got {
		seen := map[string]bool{}
		for _, v := range table {
			if !valid[name](v) || seen[v] {
				t.Errorf("%s: %q is not a valid, unique, known value", name, v)
			}
			seen[v] = true
		}
	}
	for i, s := range Signals() {
		if s.PageCode() != uint64(i+1) {
			t.Errorf("%s: PageCode %d, want %d", s, s.PageCode(), i+1)
		}
	}
}

// alignedPage returns a zeroed, 8-byte aligned page buffer.
func alignedPage() []byte {
	words := make([]uint64, PageSize/8)
	return unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), PageSize)
}

// TestPageWriter checks the seqlock store order's results: the body equals
// EncodePage, seq moves by two per publication and stays even between
// them, notifyEpoch moves only when PageNotifyDue, and Resume continues
// both.
func TestPageWriter(t *testing.T) {
	t.Parallel()
	le := binary.LittleEndian
	page := alignedPage()
	if _, err := NewPageWriter(page[1:]); !errors.Is(err, ErrPageBuffer) {
		t.Fatalf("a short, misaligned mapping: %v", err)
	}
	w, err := NewPageWriter(page)
	if err != nil {
		t.Fatal(err)
	}
	s := pageSampleSnapshot(PlatformDarwin)
	if err := w.Publish(&s); !errors.Is(err, ErrPageWriterNotResumed) {
		t.Fatalf("Publish before Resume: %v", err)
	}
	if err := w.Resume(); err != nil {
		t.Fatal(err)
	}
	state := func() (uint64, uint64) {
		return le.Uint64(page[PageSeqOffset:]), le.Uint64(page[PageNotifyEpochOffset:])
	}
	publish := func(s PageSnapshot, wantSeq, wantEpoch uint64) {
		t.Helper()
		if err := w.Publish(&s); err != nil {
			t.Fatal(err)
		}
		if seq, epoch := state(); seq != wantSeq || epoch != wantEpoch {
			t.Fatalf("seq %d epoch %d, want %d %d", seq, epoch, wantSeq, wantEpoch)
		}
		enc := mustEncode(t, s)
		got := append([]byte(nil), page...)
		le.PutUint64(got[PageSeqOffset:], 0)
		le.PutUint64(got[PageNotifyEpochOffset:], 0)
		if !bytes.Equal(got, enc) {
			t.Fatal("the published page differs from EncodePage")
		}
	}
	publish(s, 2, 1)
	publish(s, 4, 1)
	s.Signals[0].Value = i64(5 * gib)
	publish(s, 6, 1)
	warn := s
	warn.Level = LevelWarn
	publish(warn, 8, 2)
	bad := s
	bad.Level = "calm"
	if err := w.Publish(&bad); err == nil {
		t.Fatal("an invalid snapshot was published")
	}
	if seq, _ := state(); seq != 8 {
		t.Fatalf("a refused publication moved seq to %d", seq)
	}

	w2, err := NewPageWriter(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := w2.Resume(); err != nil {
		t.Fatal(err)
	}
	w = w2
	publish(warn, 10, 3)
	le.PutUint64(page[PageSeqOffset:], 13)
	if err := w.Resume(); err != nil {
		t.Fatal(err)
	}
	publish(warn, 16, 4)
}

// TestPageWriterSeqlock runs a writer against a seqlock reader that loads
// every word atomically; every stable read must be one of the published
// pages, never a mix.
func TestPageWriterSeqlock(t *testing.T) {
	t.Parallel()
	page := alignedPage()
	w, err := NewPageWriter(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Resume(); err != nil {
		t.Fatal(err)
	}
	a := pageSampleSnapshot(PlatformDarwin)
	b := pageSampleSnapshot(PlatformDarwin)
	b.Generation, b.Level, b.Capacity.SwapUsedBytes = 99, LevelCritical, i64(3*gib)
	b.ObservedAt = metav1.NewTime(sampleAt.Add(time.Hour))
	bodies := map[string]bool{}
	for _, s := range []PageSnapshot{a, b} {
		bodies[string(mustEncode(t, s)[PageHeaderSize:pageBodyEnd])] = true
	}
	word := func(off int) uint64 { return atomic.LoadUint64((*uint64)(unsafe.Pointer(&page[off]))) }
	var stop atomic.Bool
	var wg sync.WaitGroup
	var stable atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		body := make([]byte, pageBodyEnd-PageHeaderSize)
		for !stop.Load() {
			s1 := word(PageSeqOffset)
			if s1%2 == 1 || s1 == 0 {
				continue
			}
			for off := PageHeaderSize; off < pageBodyEnd; off += 8 {
				binary.LittleEndian.PutUint64(body[off-PageHeaderSize:], word(off))
			}
			if word(PageSeqOffset) != s1 {
				continue
			}
			stable.Add(1)
			if !bodies[string(body)] {
				t.Errorf("a stable read at seq %d is no published page", s1)
				return
			}
		}
	}()
	for i := range 4000 {
		s := a
		if i%2 == 1 {
			s = b
		}
		if err := w.Publish(&s); err != nil {
			t.Fatal(err)
		}
	}
	stop.Store(true)
	wg.Wait()
	if stable.Load() == 0 {
		t.Log("the reader saw no stable read; the check ran vacuously this time")
	}
}
