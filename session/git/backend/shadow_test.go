package backend_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

var shadowDiff = cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortDiffStatus: backend.BackendShadow})

func diffReturning(vals ...string) func(context.Context, backend.RepoLocation, backend.DiffSpec) (string, error) {
	var n atomic.Int32
	return func(context.Context, backend.RepoLocation, backend.DiffSpec) (string, error) {
		i := int(n.Add(1)) - 1
		if i >= len(vals) {
			i = len(vals) - 1
		}
		return vals[i], nil
	}
}

func (g *rig) mismatches(op backend.OperationName, class backend.MismatchClass) int64 {
	return g.m.count(backend.MetricShadowMismatchTotal, "operation", string(op), "class", string(class))
}

func TestShadowReturnsCLIResultAndCountsRealMismatch(t *testing.T) {
	g := newRig(t, shadowDiff)
	g.cli.diff = diffReturning("cli-diff")
	g.gg.diff = diffReturning("gogit-diff")
	got, err := g.r.Diff(ctxBG, localRepo, backend.DiffSpec{})
	if err != nil || got != "cli-diff" {
		t.Fatalf("Diff = %q, %v; shadow must return the CLI result", got, err)
	}
	if g.mismatches(backend.OpDiff, backend.MismatchReal) != 1 || g.mismatches(backend.OpDiff, backend.MismatchRacy) != 0 {
		t.Fatal("want exactly one real mismatch")
	}
	if g.m.count(backend.MetricShadowCallsTotal, "operation", "Diff") != 1 {
		t.Fatal("shadow_calls_total is the window denominator and must count the call")
	}
	if recs := g.rec.all(); len(recs) != 1 || recs[0].Class != backend.MismatchReal || recs[0].Operation != backend.OpDiff {
		t.Fatalf("records = %+v", recs)
	}
}

func TestShadowMismatchRecordIsRedactedAndPathFree(t *testing.T) {
	g := newRig(t, shadowDiff)
	secret := "https://x-access-token:ghp_abc123SECRET@github.com/o/r.git"
	g.cli.diff = diffReturning("remote " + secret)
	g.gg.diff = diffReturning("other")
	if _, err := g.r.Diff(ctxBG, localRepo, backend.DiffSpec{}); err != nil {
		t.Fatal(err)
	}
	rec := g.rec.all()[0]
	blob, _ := json.Marshal(rec)
	if strings.Contains(string(blob), "ghp_") || strings.Contains(string(blob), "SECRET") {
		t.Fatalf("record leaks a token: %s", blob)
	}
	if strings.Contains(string(blob), "/repo") || rec.RepoRootHash == "" {
		t.Fatalf("record must identify the repo by hash only: %s", blob)
	}
	if rec.HeadSHA != "head0" || rec.IndexMTimeNs != 1 {
		t.Fatalf("record must carry the pinned HEAD and index mtime: %+v", rec)
	}
}

func TestShadowAgreementCountsNoMismatch(t *testing.T) {
	g := newRig(t, shadowDiff)
	g.cli.diff = diffReturning("same")
	g.gg.diff = diffReturning("same")
	if _, err := g.r.Diff(ctxBG, localRepo, backend.DiffSpec{}); err != nil {
		t.Fatal(err)
	}
	if g.m.totalCount(backend.MetricShadowMismatchTotal) != 0 || len(g.rec.all()) != 0 {
		t.Fatal("agreement is not a mismatch")
	}
}

func TestShadowDifferenceThatDisappearsIsRacy(t *testing.T) {
	g := newRig(t, shadowDiff)
	g.cli.diff = diffReturning("v2")
	g.gg.diff = diffReturning("v1", "v2") // stale on the first read only
	got, _ := g.r.Diff(ctxBG, localRepo, backend.DiffSpec{})
	if got != "v2" {
		t.Fatalf("got %q", got)
	}
	if g.mismatches(backend.OpDiff, backend.MismatchRacy) != 1 || g.mismatches(backend.OpDiff, backend.MismatchReal) != 0 {
		t.Fatal("a difference that disappears on the pinned re-read is racy")
	}
	if len(g.rec.all()) != 0 {
		t.Fatal("racy mismatches write no repro record")
	}
}

func TestShadowMovingRepositoryIsDiscardedRerunThenRacy(t *testing.T) {
	g := newRig(t, shadowDiff)
	g.cli.diff = diffReturning("cli")
	g.gg.diff = diffReturning("gogit")
	var n atomic.Int64
	g.pins.next = func() backend.StatePin { return backend.StatePin{Head: "h", IndexMTime: n.Add(1), Valid: true} } // always moving
	if _, err := g.r.Diff(ctxBG, localRepo, backend.DiffSpec{}); err != nil {
		t.Fatal(err)
	}
	if g.mismatches(backend.OpDiff, backend.MismatchRacy) != 1 || g.mismatches(backend.OpDiff, backend.MismatchReal) != 0 {
		t.Fatal("a pair that still changes after one re-run is racy")
	}
	if g.log.count("cli.Diff") != 3 {
		t.Fatalf("CLI reads = %d, want first read + re-read + one re-run = 3", g.log.count("cli.Diff"))
	}
}

func TestShadowStabilisesAfterOneRerunThenCountsReal(t *testing.T) {
	g := newRig(t, shadowDiff)
	g.cli.diff = diffReturning("cli")
	g.gg.diff = diffReturning("gogit")
	// pins: p0, then moved at the re-read bracket, then steady for the re-run
	seq := []backend.StatePin{
		{Head: "a", IndexMTime: 1, Valid: true}, // before first read
		{Head: "b", IndexMTime: 1, Valid: true}, // after re-read: HEAD moved
		{Head: "b", IndexMTime: 1, Valid: true}, // before re-run
		{Head: "b", IndexMTime: 1, Valid: true}, // after re-run: steady
	}
	var i atomic.Int32
	g.pins.next = func() backend.StatePin { return seq[min(int(i.Add(1))-1, len(seq)-1)] }
	if _, err := g.r.Diff(ctxBG, localRepo, backend.DiffSpec{}); err != nil {
		t.Fatal(err)
	}
	if g.mismatches(backend.OpDiff, backend.MismatchReal) != 1 {
		t.Fatal("a disagreement that persists on a stable re-run is real")
	}
	if rec := g.rec.all()[0]; rec.HeadSHA != "b" {
		t.Fatalf("the record must carry the pin of the stable pair, got %q", rec.HeadSHA)
	}
}

func TestShadowUnreadablePinIsNotHiddenAsRacy(t *testing.T) {
	g := newRig(t, shadowDiff)
	g.cli.diff = diffReturning("cli")
	g.gg.diff = diffReturning("gogit")
	g.pins.next = func() backend.StatePin { return backend.StatePin{} }
	if _, err := g.r.Diff(ctxBG, localRepo, backend.DiffSpec{}); err != nil {
		t.Fatal(err)
	}
	if g.mismatches(backend.OpDiff, backend.MismatchReal) != 1 {
		t.Fatal("when the state cannot be pinned a persistent disagreement must stay blocking")
	}
}

func TestShadowFalseCleanIsNeverRealOrRacyWhenStable(t *testing.T) {
	cases := []struct {
		name string
		op   backend.OperationName
		wire func(g *rig, cliDirty, ggDirty bool)
		call func(g *rig)
	}{
		{"IsDirty", backend.OpIsDirty, func(g *rig, c, gg bool) {
			g.cli.isDirty = func(context.Context, backend.RepoLocation, backend.Intent) (bool, error) { return c, nil }
			g.gg.isDirty = func(context.Context, backend.RepoLocation, backend.Intent) (bool, error) { return gg, nil }
		}, func(g *rig) { _, _ = g.r.IsDirty(ctxBG, localRepo, backend.IntentDisplay) }},
		{"Status", backend.OpStatus, func(g *rig, c, gg bool) {
			mk := func(d bool) backend.StatusResult {
				if d {
					return backend.StatusResult{Files: []backend.FileStatus{{Path: "a"}}}
				}
				return backend.StatusResult{}
			}
			g.cli.status = func(context.Context, backend.RepoLocation, backend.Intent) (backend.StatusResult, error) {
				return mk(c), nil
			}
			g.gg.status = func(context.Context, backend.RepoLocation, backend.Intent) (backend.StatusResult, error) {
				return mk(gg), nil
			}
		}, func(g *rig) { _, _ = g.r.Status(ctxBG, localRepo, backend.IntentDisplay) }},
		{"DiffNumstat", backend.OpDiffNumstat, func(g *rig, c, gg bool) {
			mk := func(d bool) []backend.NumstatRow {
				if d {
					return []backend.NumstatRow{{Path: "a", Added: 1}}
				}
				return nil
			}
			g.cli.numstat = func(context.Context, backend.RepoLocation, backend.DiffSpec) ([]backend.NumstatRow, error) {
				return mk(c), nil
			}
			g.gg.numstat = func(context.Context, backend.RepoLocation, backend.DiffSpec) ([]backend.NumstatRow, error) {
				return mk(gg), nil
			}
		}, func(g *rig) {
			_, _ = g.r.DiffNumstat(ctxBG, localRepo, backend.DiffSpec{Intent: backend.IntentDisplay})
		}},
	}
	for _, c := range cases {
		t.Run(c.name+" go-git clean, cli dirty", func(t *testing.T) {
			g := newRig(t, shadowDiff)
			c.wire(g, true, false)
			c.call(g)
			if g.mismatches(c.op, backend.MismatchFalseClean) != 1 || g.mismatches(c.op, backend.MismatchReal) != 0 {
				t.Fatal("want exactly one false_clean and no real")
			}
			if recs := g.rec.all(); len(recs) != 1 || recs[0].Class != backend.MismatchFalseClean {
				t.Fatalf("false_clean must write a repro record: %+v", recs)
			}
		})
		t.Run(c.name+" go-git dirty, cli clean is plain real", func(t *testing.T) {
			g := newRig(t, shadowDiff)
			c.wire(g, false, true)
			c.call(g)
			if g.mismatches(c.op, backend.MismatchReal) != 1 || g.mismatches(c.op, backend.MismatchFalseClean) != 0 {
				t.Fatal("want real, not false_clean")
			}
		})
	}
}

func TestShadowFalseCleanDecidedOnStablePairNotFirstRead(t *testing.T) {
	g := newRig(t, shadowDiff)
	g.cli.isDirty = func(context.Context, backend.RepoLocation, backend.Intent) (bool, error) { return true, nil }
	var n atomic.Int32
	g.gg.isDirty = func(context.Context, backend.RepoLocation, backend.Intent) (bool, error) {
		return n.Add(1) > 1, nil // clean once, then agrees
	}
	_, _ = g.r.IsDirty(ctxBG, localRepo, backend.IntentDisplay)
	if g.mismatches(backend.OpIsDirty, backend.MismatchRacy) != 1 || g.mismatches(backend.OpIsDirty, backend.MismatchFalseClean) != 0 {
		t.Fatal("a go-git clean that disappears on re-read is racy")
	}
}

func TestShadowGoGitPanicOrErrorNeverChangesTheCLIResult(t *testing.T) {
	g := newRig(t, shadowDiff)
	g.cli.diff = diffReturning("cli")
	g.gg.diff = func(context.Context, backend.RepoLocation, backend.DiffSpec) (string, error) { panic("go-git blew up") }
	got, err := g.r.Diff(ctxBG, localRepo, backend.DiffSpec{})
	if err != nil || got != "cli" {
		t.Fatalf("Diff = %q, %v", got, err)
	}
	if g.mismatches(backend.OpDiff, backend.MismatchReal) != 1 {
		t.Fatal("a failing in-process backend is a real mismatch")
	}
}

func TestShadowCLIErrorIsReturnedAndErrorsAgree(t *testing.T) {
	g := newRig(t, cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortRefs: backend.BackendShadow}))
	g.cli.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "", backend.ErrUnborn
	}
	g.gg.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "", backend.ErrUnborn
	}
	if _, err := g.r.ResolveRef(ctxBG, localRepo, "HEAD"); !errors.Is(err, backend.ErrUnborn) {
		t.Fatalf("err = %v", err)
	}
	if g.m.totalCount(backend.MetricShadowMismatchTotal) != 0 {
		t.Fatal("two ErrUnborn answers agree")
	}
	g.gg.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "", backend.ErrRefNotFound
	}
	_, _ = g.r.ResolveRef(ctxBG, localRepo, "HEAD")
	if g.mismatches(backend.OpResolveRef, backend.MismatchReal) != 1 {
		t.Fatal("ErrUnborn vs ErrRefNotFound disagree")
	}
}

func TestShadowTreatsNilAndEmptyCollectionsAsEqual(t *testing.T) {
	g := newRig(t, shadowDiff)
	g.cli.numstat = func(context.Context, backend.RepoLocation, backend.DiffSpec) ([]backend.NumstatRow, error) {
		return nil, nil
	}
	g.gg.numstat = func(context.Context, backend.RepoLocation, backend.DiffSpec) ([]backend.NumstatRow, error) {
		return []backend.NumstatRow{}, nil
	}
	_, _ = g.r.DiffNumstat(ctxBG, localRepo, backend.DiffSpec{Intent: backend.IntentDisplay})
	if g.m.totalCount(backend.MetricShadowMismatchTotal) != 0 {
		t.Fatal("nil and empty are both 'nothing'")
	}
}

func TestShadowPreflightVetoSkipsGoGit(t *testing.T) {
	g := newRig(t, shadowDiff, func(c *backend.RouterConfig) {
		c.Preflight = func(context.Context, backend.Local, backend.OperationName) []backend.FallbackReason {
			return []backend.FallbackReason{backend.ReasonCapabilitySparse}
		}
	})
	if _, err := g.r.Diff(ctxBG, localRepo, backend.DiffSpec{}); err != nil {
		t.Fatal(err)
	}
	if g.log.count("gogit.Diff") != 0 || g.fallbacks(backend.OpDiff, backend.CohortDiffStatus, backend.ReasonCapabilitySparse) != 1 {
		t.Fatal("a capability veto must keep shadow from reading in process")
	}
}

func TestDefaultPinReadsHeadAndIndexMTime(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(gitDir, "index")
	if err := os.WriteFile(index, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Unix(1_700_000_000, 5)
	if err := os.Chtimes(index, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	g := newRig(t, shadowDiff, func(c *backend.RouterConfig) { c.Pinner = nil }) // router uses its own default pin
	g.gg.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "pinnedhead", nil
	}
	g.gg.gitDir = func(context.Context, backend.RepoLocation) (backend.GitDir, error) { return ".git", nil } // relative, as git prints it
	g.cli.diff = diffReturning("a")
	g.gg.diff = diffReturning("b")
	if _, err := g.r.Diff(ctxBG, backend.Local{Root: backend.RepoRoot(dir)}, backend.DiffSpec{}); err != nil {
		t.Fatal(err)
	}
	rec := g.rec.all()[0]
	if rec.HeadSHA != "pinnedhead" || rec.IndexMTimeNs != mtime.UnixNano() {
		t.Fatalf("pin = %q/%d, want pinnedhead/%d", rec.HeadSHA, rec.IndexMTimeNs, mtime.UnixNano())
	}
}

func TestFileMismatchRecorderWritesPrivateRedactedJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shadow-mismatches")
	rec := &backend.FileMismatchRecorder{Dir: dir}
	rec.Record(backend.MismatchRecord{Operation: backend.OpDiff, Class: backend.MismatchReal, CLIResult: "x"})
	rec.Record(backend.MismatchRecord{Operation: backend.OpDiff, Class: backend.MismatchReal, CLIResult: "y"})
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %v, %v; two records must not collide", entries, err)
	}
	info, _ := entries[0].Info()
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("record mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestShadowPinThatBecomesUnreadableIsNotTreatedAsMovement(t *testing.T) {
	g := newRig(t, shadowDiff)
	g.cli.diff = diffReturning("cli")
	g.gg.diff = diffReturning("gogit")
	var n atomic.Int32
	g.pins.next = func() backend.StatePin {
		if n.Add(1) == 1 {
			return backend.StatePin{Head: "h", IndexMTime: 1, Valid: true}
		}
		return backend.StatePin{} // unreadable afterwards
	}
	_, _ = g.r.Diff(ctxBG, localRepo, backend.DiffSpec{})
	if g.mismatches(backend.OpDiff, backend.MismatchReal) != 1 {
		t.Fatal("an unreadable pin must not downgrade a persistent disagreement to racy")
	}
}
