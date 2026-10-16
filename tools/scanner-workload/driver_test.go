package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recordingClock struct{ waits []time.Duration }

func (c *recordingClock) WaitUntil(_ context.Context, off time.Duration) error {
	c.waits = append(c.waits, off)
	return nil
}

// driverCorpus builds a corpus of two small repos with W=2 from fresh sources;
// two calls yield byte-identical corpora at different paths.
func driverCorpus(t *testing.T) (string, Manifest) {
	t.Helper()
	parent := t.TempDir()
	srcs := []string{newSourceRepo(t, parent, "r1"), newSourceRepo(t, parent, "r2")}
	root := t.TempDir()
	return root, buildTestCorpus(t, srcs, root, 2)
}

func busyConfig(root string, m Manifest, seed uint64) DriverConfig {
	return DriverConfig{
		Seed: seed, Duration: 2 * time.Hour, Tick: 10 * time.Minute,
		Targets:       TargetsFromManifest(root, m),
		CommitsPerDay: map[string]float64{"r1": 48, "r2": 24}, DefaultCommitsPerDay: 5,
		Speedup: 1, FSOpsPerHour: 60,
	}
}

func TestDriver_SameSeedTwoIdenticalCorporaHashEqual(t *testing.T) {
	rootA, mA := driverCorpus(t)
	rootB, mB := driverCorpus(t)
	logA, err := Run(context.Background(), busyConfig(rootA, mA, 7))
	if err != nil {
		t.Fatal(err)
	}
	logB, err := Run(context.Background(), busyConfig(rootB, mB, 7))
	if err != nil {
		t.Fatal(err)
	}
	if logA.Hash() != logB.Hash() {
		t.Fatalf("hash mismatch for same seed:\nA=%s\nB=%s", logA.Hash(), logB.Hash())
	}

	rootC, mC := driverCorpus(t)
	logC, err := Run(context.Background(), busyConfig(rootC, mC, 8))
	if err != nil {
		t.Fatal(err)
	}
	if logC.Hash() == logA.Hash() {
		t.Fatal("different seeds produced the same op log")
	}
}

func TestDriver_AppliesEveryOpKindWithoutSkippingEverything(t *testing.T) {
	root, m := driverCorpus(t)
	l, err := Run(context.Background(), busyConfig(root, m, 3))
	if err != nil {
		t.Fatal(err)
	}
	applied := map[OpKind]int{}
	for _, op := range l.Ops {
		if !strings.HasPrefix(op.Detail, "skip:") {
			applied[op.Kind]++
		}
	}
	for _, k := range []OpKind{OpCommit, OpEdit, OpCreate, OpDelete, OpGitignore, OpMoveBase, OpIdle} {
		if applied[k] == 0 {
			t.Errorf("op kind %s never applied; applied=%v", k, applied)
		}
	}
}

func TestDriver_CommitAndMoveBaseTouchOnlyCorpusAndAdvanceHistory(t *testing.T) {
	root, m := driverCorpus(t)
	repoDir := m.Repos[0].Path
	before := headHash(t, repoDir)
	if _, err := Run(context.Background(), busyConfig(root, m, 5)); err != nil {
		t.Fatal(err)
	}
	if headHash(t, repoDir) == before && headHash(t, m.Repos[0].Worktrees[0]) == before {
		t.Fatal("no commit moved any HEAD")
	}
	if _, err := os.Stat(filepath.Join(m.Repos[0].Source, ".ssq-workload")); err == nil {
		t.Fatal("driver wrote into the source repo")
	}
}

func headHash(t *testing.T, dir string) string {
	t.Helper()
	repo, err := openRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	h, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	return h.Hash().String()
}

func TestSchedule_PureAndCadenceDriven(t *testing.T) {
	cfg := DriverConfig{
		Seed: 1, Duration: 24 * time.Hour,
		Targets:       []Target{{Name: "repos/a", Repo: "a", Dir: "x", Main: true}, {Name: "worktrees/a-w1", Repo: "a", Dir: "y"}},
		CommitsPerDay: map[string]float64{"a": 200},
	}
	count := func(c DriverConfig) int {
		n := 0
		for _, op := range Schedule(c) {
			if op.Kind == OpCommit {
				n++
			}
		}
		return n
	}
	a, b := Schedule(cfg), Schedule(cfg)
	if len(a) != len(b) {
		t.Fatal("schedule not repeatable")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("op %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
	if n := count(cfg); n < 150 || n > 250 {
		t.Fatalf("200 commits/day over 24h produced %d commit ops", n)
	}
	cfg.CommitsPerDay["a"] = 0
	if n := count(cfg); n != 0 {
		t.Fatalf("zero cadence produced %d commit ops", n)
	}
}

func TestRun_PacesThroughInjectedClockWithNoWaiting(t *testing.T) {
	root, m := driverCorpus(t)
	cfg := busyConfig(root, m, 2)
	clk := &recordingClock{}
	cfg.Clock = clk
	start := time.Now()
	l, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(clk.waits) != len(l.Ops) {
		t.Fatalf("clock saw %d waits for %d ops", len(clk.waits), len(l.Ops))
	}
	for i := 1; i < len(clk.waits); i++ {
		if clk.waits[i] < clk.waits[i-1] {
			t.Fatal("offsets not monotonic")
		}
	}
	if time.Since(start) > time.Minute {
		t.Fatal("run appears to have waited on wall clock")
	}
}
