//go:build darwin

package fsevents

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type collector struct {
	mu  sync.Mutex
	evs []Event
}

func (c *collector) add(e []Event) { c.mu.Lock(); c.evs = append(c.evs, e...); c.mu.Unlock() }

func (c *collector) has(pred func(Event) bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.evs {
		if pred(e) {
			return true
		}
	}
	return false
}

func (c *collector) waitFor(t *testing.T, d time.Duration, what string, pred func(Event) bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if c.has(pred) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %s event within %v; got %+v", what, d, c.evs)
}

// resolved dodges /var -> /private/var, which FSEvents reports in paths.
func resolved(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func forPath(p string, f EventFlags) func(Event) bool {
	return func(e Event) bool { return e.Path == p && e.Flags&f != 0 }
}

func TestStreamCreateModifyRenameDelete(t *testing.T) {
	dir := resolved(t)
	var c collector
	s, err := NewStream(Config{Paths: []string{dir}, SinceWhen: SinceNow, OnEvents: c.add})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	f := filepath.Join(dir, "a.txt")
	const budget = 500 * time.Millisecond
	if err := os.WriteFile(f, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, budget, "create", forPath(f, ItemCreated))

	if err := os.WriteFile(f, []byte("22"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, budget, "modify", forPath(f, ItemModified))

	tmp := filepath.Join(dir, "a.txt.tmp")
	if err := os.WriteFile(tmp, []byte("3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, f); err != nil { // rename-over, the save pattern kqueue misses
		t.Fatal(err)
	}
	c.waitFor(t, budget, "rename-over", forPath(f, ItemRenamed))

	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, budget, "delete", forPath(f, ItemRemoved))
}

func TestStreamResumeReplaysAndSignalsHistoryDone(t *testing.T) {
	dir := resolved(t)
	var first collector
	s1, err := NewStream(Config{Paths: []string{dir}, SinceWhen: SinceNow, OnEvents: first.add})
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Start(); err != nil {
		t.Fatal(err)
	}
	// FlushSync makes the saved cursor cover everything delivered so far.
	s1.FlushSync()
	saved := CurrentEventID()
	s1.Stop()

	missed := filepath.Join(dir, "missed.txt")
	if err := os.WriteFile(missed, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var c collector
	s2, err := NewStream(Config{Paths: []string{dir}, SinceWhen: saved, OnEvents: c.add})
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.Start(); err != nil {
		t.Fatal(err)
	}
	defer s2.Stop()
	c.waitFor(t, 5*time.Second, "replayed create", func(e Event) bool {
		return strings.HasSuffix(e.Path, "missed.txt")
	})
	c.waitFor(t, 5*time.Second, "HistoryDone", func(e Event) bool { return e.Flags&HistoryDone != 0 })
}

func TestStreamStopIsIdempotent(t *testing.T) {
	s, err := NewStream(Config{Paths: []string{resolved(t)}, SinceWhen: SinceNow})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	s.Stop()
	s.Stop()
}

func TestVolumeUUID(t *testing.T) {
	u := VolumeUUID(t.TempDir())
	if len(u) != 36 || u == "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("unexpected UUID %q", u)
	}
	if VolumeUUID("/definitely/not/here") != "" {
		t.Fatal("expected empty UUID for missing path")
	}
}
