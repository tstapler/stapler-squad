package unfinished

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readBufferFixture writes n files of `lines` lines each and returns their paths.
func readBufferFixture(t *testing.T, n, lines int) []string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, n)
	for i := range paths {
		paths[i] = filepath.Join(dir, fmt.Sprintf("f%03d.txt", i))
		if err := os.WriteFile(paths[i], []byte(strings.Repeat("some untracked line\n", lines)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

// assertReadBufferCeiling requires newImpl to allocate at most 25% of the
// legacy readFileIfSmall+countBytesLines path over the fixture (Story 4.4.1,
// the read-buffer row: 75.9% of S1 scanner alloc_space).
func assertReadBufferCeiling(tb gateTB, paths []string, newImpl func(path string)) {
	tb.Helper()
	legacy := func(p string) {
		if data, ok := readFileIfSmall(p); ok {
			countBytesLines(data)
		}
	}
	each := func(f func(string)) func() {
		return func() {
			for _, p := range paths {
				f(p)
			}
		}
	}
	oldBytes, _ := allocDelta(20, each(legacy))
	newBytes, _ := allocDelta(20, each(newImpl))
	if newBytes > 0.25*oldBytes {
		tb.Errorf("4.4.1 read buffers: B/op = %.0f, ceiling = 25%% of legacy %.0f", newBytes, oldBytes)
	}
}

func TestResidualAllocator_ShouldMeetCeilingVsLegacy_WhenAssignedTo441(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	paths := readBufferFixture(t, 100, 400) // 8 KB each
	t.Run("untracked line count streams", func(t *testing.T) {
		assertReadBufferCeiling(t, paths, func(p string) { countFileLines(p) })
	})
	t.Run("tracked read uses the pool", func(t *testing.T) {
		assertReadBufferCeiling(t, paths, func(p string) {
			if bp, ok := readSmallFilePooled(p); ok {
				countBytesLines(*bp)
				releaseFileBuf(bp)
			}
		})
	})
}

func TestReadBufferGate_ShouldFail_WhenLegacyIsWiredAsNew(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	rec := &failRecorder{}
	assertReadBufferCeiling(rec, readBufferFixture(t, 20, 400), func(p string) {
		if data, ok := readFileIfSmall(p); ok {
			countBytesLines(data)
		}
	})
	if rec.failures == 0 {
		t.Error("gate passed with the legacy reader wired as the new implementation")
	}
}

func TestReadSmallFilePooled_ShouldMatchReadFileIfSmall_WhenContentAndLimitsVary(t *testing.T) {
	orig := maxUntrackedFileSize
	maxUntrackedFileSize = 100
	t.Cleanup(func() { maxUntrackedFileSize = orig })
	dir := t.TempDir()
	cases := map[string]string{
		"empty": "", "newline": "\n", "no-eol": "abc", "two": "a\nb\n",
		"at-limit": strings.Repeat("x", 100), "over-limit": strings.Repeat("x", 101),
	}
	for name, body := range cases {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		want, wantOK := readFileIfSmall(p)
		bp, gotOK := readSmallFilePooled(p)
		if gotOK != wantOK || (gotOK && string(*bp) != string(want)) {
			t.Errorf("%s: pooled = (%v), legacy = (%v)", name, gotOK, wantOK)
		}
		if gotOK {
			releaseFileBuf(bp)
		}
		n, nOK := countFileLines(p)
		if nOK != wantOK || (nOK && n != countBytesLines(want)) {
			t.Errorf("%s: countFileLines = (%d,%v), want (%d,%v)", name, n, nOK, countBytesLines(want), wantOK)
		}
	}
	for _, missing := range []string{filepath.Join(dir, "nope"), dir} {
		if _, ok := readSmallFilePooled(missing); ok {
			t.Errorf("readSmallFilePooled(%s) ok, want false", missing)
		}
		if _, ok := countFileLines(missing); ok {
			t.Errorf("countFileLines(%s) ok, want false", missing)
		}
	}
}
