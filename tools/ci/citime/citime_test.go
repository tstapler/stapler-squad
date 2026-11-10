package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanTimesTargetsAndPackages(t *testing.T) {
	t.Parallel()
	in := strings.Join([]string{
		"Makefile:10: update target 'vet' due to: target is .PHONY",
		"go vet ./...",
		"Makefile:20: update target 'test' due to: target is .PHONY",
		"\x1b[32m✓\x1b[0m  session (12.5s)",
		"✓  config (cached)",
		"✓  tiny (3ms)",
		"✓  slow (1m2s)",
		"✖  server (3s)",
	}, "\n") + "\n"
	clock := []time.Duration{0, 5 * time.Second, 25 * time.Second, 30 * time.Second}
	i := 0
	now := func() time.Time { d := clock[min(i, len(clock)-1)]; i++; return time.Unix(0, 0).Add(d) }
	rep, err := scan(strings.NewReader(in), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Targets) != 2 || rep.Targets[0] != (Entry{"vet", 20}) || rep.Targets[1] != (Entry{"test", 5}) {
		t.Errorf("targets = %+v", rep.Targets)
	}
	if len(rep.Packages) != 4 || rep.Packages[0] != (Entry{"test: slow", 62}) || rep.Packages[1].Name != "test: session" || rep.Packages[3].Name != "test: tiny" {
		t.Errorf("packages = %+v (cached packages carry no time and are skipped)", rep.Packages)
	}
	if rep.TotalSeconds != 30 {
		t.Errorf("total = %v", rep.TotalSeconds)
	}
	md := rep.markdown(5)
	if !strings.Contains(md, "| `vet` | 20.0s | 66.7% |") {
		t.Errorf("markdown missing vet row:\n%s", md)
	}
}

func TestFoldShortTargets(t *testing.T) {
	t.Parallel()
	got := foldShort([]Entry{{"a.o", 0.2}, {"build", 3}, {"b.o", 0.3}})
	if len(got) != 2 || got[0].Name != "build" || got[1].Name != "(2 targets under 1s)" || got[1].Seconds != 0.5 {
		t.Errorf("got %+v", got)
	}
}

func TestFollowReaderStopsAfterDoneFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	log := filepath.Join(dir, "make.log")
	if err := os.WriteFile(log, []byte("Makefile:1: update target 'a' due to: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(log)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	done := filepath.Join(dir, "make.log.done")
	go func() {
		time.Sleep(300 * time.Millisecond)
		os.WriteFile(done, nil, 0o644) //nolint:errcheck // test helper
	}()
	rep, err := scan(followReader{f: f, done: done}, time.Now)
	if err != nil || len(rep.Targets) != 1 {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
}
