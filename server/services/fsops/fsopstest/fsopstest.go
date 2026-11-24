// Package fsopstest holds fixtures shared by the fsops tests and the tests of the
// packages that consume it.
package fsopstest

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/analytics"
)

// MustMkdir creates p and its parents or fails the test.
func MustMkdir(tb testing.TB, p string) {
	tb.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		tb.Fatal(err)
	}
}

// WriteFixtureFile writes data to p or fails the test.
func WriteFixtureFile(tb testing.TB, p string, data []byte) {
	tb.Helper()
	if err := os.WriteFile(p, data, 0o644); err != nil {
		tb.Fatal(err)
	}
}

// NewResolverFixture builds a worktree with the symlink classes found in the
// owner's live worktrees (ADR-031 "Spike outcomes", Task 0.2.1d).
func NewResolverFixture(t *testing.T) (dir, outside string) {
	t.Helper()
	outside = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("TOPSECRET"), 0o644))
	dir = t.TempDir()
	MustMkdir(t, filepath.Join(dir, "src"))
	MustMkdir(t, filepath.Join(dir, "docs"))
	MustMkdir(t, filepath.Join(dir, "pkg", "shared"))
	MustMkdir(t, filepath.Join(dir, ".git", "hooks"))
	WriteFixtureFile(t, filepath.Join(dir, "src", "a.go"), []byte("package a\n"))
	WriteFixtureFile(t, filepath.Join(dir, "CLAUDE.md"), []byte("claude\n"))
	WriteFixtureFile(t, filepath.Join(dir, "README.md"), []byte("readme\n"))
	WriteFixtureFile(t, filepath.Join(dir, "pkg", "shared", "x.txt"), []byte("shared\n"))
	WriteFixtureFile(t, filepath.Join(dir, ".git", "config"), []byte("[core]\n"))
	links := map[string]string{
		"AGENTS.md":       "CLAUDE.md",                      // in-tree file link (257/335 in the live inventory)
		"docs/alias.md":   "../README.md",                   // in-tree file link via ..
		"shared":          "pkg/shared",                     // in-tree directory link
		"chain1":          "chain2",                         // chained in-tree links
		"chain2":          "CLAUDE.md",                      //
		"leak":            filepath.Join(outside, "secret"), // absolute target outside (AGENTS.md -> ~/.claude/CLAUDE.md class)
		"leak_rel":        "../" + filepath.Base(outside) + "/secret",
		"dir_leak":        outside,                  // directory link out of the root
		"dangling":        "../shared/slidev-theme", // dangling relative link above the root
		"dangling_inside": "nope.txt",               // dangling link that stays inside
		"gitlink":         ".git",
		"linkdir":         ".git/hooks",
		"loop1":           "loop2",
		"loop2":           "loop1",
	}
	for name, target := range links {
		require.NoError(t, os.Symlink(target, filepath.Join(dir, name)))
	}
	return dir, outside
}

// RecordingProvider is an analytics provider that records every event and returns Err.
type RecordingProvider struct {
	mu     sync.Mutex
	Events []analytics.Event
	Err    error
	CtxErr []error // ctx.Err() as seen inside Record
	Budget []time.Duration
}

func (p *RecordingProvider) Record(ctx context.Context, e analytics.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Events = append(p.Events, e)
	p.CtxErr = append(p.CtxErr, ctx.Err())
	if dl, ok := ctx.Deadline(); ok {
		p.Budget = append(p.Budget, time.Until(dl))
	}
	return p.Err
}
