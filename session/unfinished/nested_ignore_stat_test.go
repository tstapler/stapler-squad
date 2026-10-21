package unfinished

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func nestedIgnoreFixture(tb testing.TB, n int) []nestedIgnore {
	tb.Helper()
	root := tb.TempDir()
	files := make([]nestedIgnore, 0, n)
	for i := 0; i < n; i++ {
		dir := filepath.Join(root, fmt.Sprintf("pkg%03d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatal(err)
		}
		p := filepath.Join(dir, ".gitignore")
		if err := os.WriteFile(p, []byte("*.log\n"), 0o644); err != nil {
			tb.Fatal(err)
		}
		sig, ok := statPathSig(p)
		if !ok {
			tb.Fatal("stat failed")
		}
		files = append(files, nestedIgnore{path: p, sig: sig})
	}
	return files
}

// Each recorded nested .gitignore costs exactly one stat per matcher-cache hit,
// and every one of them is counted.
func TestNestedIgnoresUnchanged_ShouldCountOneStatPerFile_When500Recorded(t *testing.T) {
	const n = 500
	files := nestedIgnoreFixture(t, n)
	g := &GoGitVCSReader{}
	before := g.StatCalls()
	if !g.nestedIgnoresUnchanged(files) {
		t.Fatal("unchanged files reported as changed")
	}
	if got := g.StatCalls() - before; got != n {
		t.Fatalf("stats per hit = %d, want exactly %d counted", got, n)
	}
	// The only allocation per hit is the syscall's path conversion, one per stat.
	if allocs := testing.AllocsPerRun(5, func() { g.nestedIgnoresUnchanged(files) }); allocs > n {
		t.Fatalf("allocs per hit = %v, want <= %d (one per stat)", allocs, n)
	}
	// An edit is still seen on the next hit (no DirtyGen shortcut).
	if err := os.WriteFile(files[n-1].path, []byte("*.log\n*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if g.nestedIgnoresUnchanged(files) {
		t.Fatal("in-place edit of a nested .gitignore went unseen")
	}
}

func BenchmarkNestedIgnoresUnchanged_500(b *testing.B) {
	files := nestedIgnoreFixture(b, 500)
	g := &GoGitVCSReader{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.nestedIgnoresUnchanged(files)
	}
}
