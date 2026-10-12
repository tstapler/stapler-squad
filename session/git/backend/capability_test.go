package backend_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

type fakeProbe struct {
	caps  []backend.Capability
	err   error
	panic bool
	calls []backend.OperationName
}

func (p *fakeProbe) Capabilities(_ context.Context, _ backend.Local, op backend.OperationName) ([]backend.Capability, error) {
	p.calls = append(p.calls, op)
	if p.panic {
		panic("probe boom")
	}
	return p.caps, p.err
}

func withProbe(p backend.CapabilityProbe) func(*backend.RouterConfig) {
	return func(c *backend.RouterConfig) { c.Preflight, c.Capabilities = nil, p }
}

func commitRig(t *testing.T, p *fakeProbe) *rig {
	t.Helper()
	g := newRig(t, cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortLocalWrite: backend.BackendGoGit}), optedIn(false), withProbe(p))
	g.gg.commit = func(context.Context, backend.RepoLocation, backend.CommitRequest) error {
		g.log.add("gogit.Commit.ran")
		return nil
	}
	return g
}

func TestProbeCapabilityRoutesToCLIWithItsReason(t *testing.T) {
	for _, c := range []struct {
		kind   backend.CapabilityKind
		reason backend.FallbackReason
	}{
		{backend.CapHooks, backend.ReasonCapabilityHooks},
		{backend.CapGPGSign, backend.ReasonCapabilityGPGSign},
		{backend.CapLFS, backend.ReasonCapabilityLFS},
		{backend.CapUnsupportedIndex, backend.ReasonCapabilityUnsupportedIdx},
		{backend.CapSparse, backend.ReasonCapabilitySparse},
		{backend.CapSubmodule, backend.ReasonCapabilitySubmodule},
	} {
		t.Run(string(c.kind), func(t *testing.T) {
			p := &fakeProbe{caps: []backend.Capability{{Kind: c.kind, Detail: "x"}}}
			g := commitRig(t, p)
			require.NoError(t, g.r.Commit(ctxBG, localRepo, backend.CommitRequest{}))
			assert.Zero(t, g.log.count("gogit.Commit.ran"), "a capability must keep the call off gogit")
			assert.Equal(t, int64(1), g.fallbacks(backend.OpCommit, backend.CohortLocalWrite, c.reason))
			assert.Equal(t, int64(1), g.m.count(backend.MetricCapabilityDetectTotal, "outcome", string(c.kind)))
		})
	}
}

func TestProbeNoCapabilityRunsInProcessAndCountsOK(t *testing.T) {
	p := &fakeProbe{}
	g := commitRig(t, p)
	require.NoError(t, g.r.Commit(ctxBG, localRepo, backend.CommitRequest{}))
	assert.Equal(t, 1, g.log.count("gogit.Commit.ran"))
	assert.Equal(t, int64(1), g.m.count(backend.MetricCapabilityDetectTotal, "outcome", "ok"))
	assert.Zero(t, g.m.totalCount(backend.MetricFallbackTotal))
}

func TestProbeErrorAndPanicFailClosedWithSubReason(t *testing.T) {
	cases := map[string]struct {
		p   *fakeProbe
		sub backend.DetectSub
	}{
		"typed error":   {&fakeProbe{err: &backend.DetectError{Sub: backend.DetectParse, Err: errors.New("x")}}, backend.DetectParse},
		"untyped error": {&fakeProbe{err: errors.New("x")}, backend.DetectUnclassified},
		"unknown sub":   {&fakeProbe{err: &backend.DetectError{Sub: "free text"}}, backend.DetectUnclassified},
		"panic":         {&fakeProbe{panic: true}, backend.DetectPanic},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			g := commitRig(t, c.p)
			require.NoError(t, g.r.Commit(ctxBG, localRepo, backend.CommitRequest{}))
			assert.Zero(t, g.log.count("gogit.Commit.ran"))
			assert.Equal(t, int64(1), g.fallbacks(backend.OpCommit, backend.CohortLocalWrite, backend.ReasonCapabilityDetectError))
			assert.Equal(t, int64(1), g.m.count(backend.MetricCapabilityDetectTotal, "outcome", "detect_error", "sub_reason", string(c.sub)))
		})
	}
}

func TestProbeIsNotConsultedForRefsReads(t *testing.T) {
	p := &fakeProbe{err: errors.New("must not be called")}
	g := newRig(t, allCohorts(backend.BackendGoGit), withProbe(p))
	g.gg.resolveRef = func(context.Context, backend.RepoLocation, backend.RefName) (backend.CommitSHA, error) {
		return "sha", nil
	}
	got, err := g.r.ResolveRef(ctxBG, localRepo, "HEAD")
	require.NoError(t, err)
	assert.Equal(t, backend.CommitSHA("sha"), got)
	assert.Empty(t, p.calls)
	assert.Zero(t, g.m.totalCount(backend.MetricCapabilityDetectTotal))
}

func TestProbeOverReportingIsFilteredByOperation(t *testing.T) {
	// Status is a read: hooks and signing say nothing about it.
	p := &fakeProbe{caps: []backend.Capability{{Kind: backend.CapHooks}, {Kind: backend.CapGPGSign}}}
	g := newRig(t, cohortsOf(map[backend.Cohort]backend.BackendMode{backend.CohortDiffStatus: backend.BackendGoGit}), withProbe(p))
	g.gg.status = func(context.Context, backend.RepoLocation, backend.Intent) (backend.StatusResult, error) {
		g.log.add("gogit.Status.ran")
		return backend.StatusResult{}, nil
	}
	_, err := g.r.Status(ctxBG, localRepo, backend.IntentDisplay)
	require.NoError(t, err)
	assert.Equal(t, 1, g.log.count("gogit.Status.ran"))
}

func TestPreflightFuncStillWinsOverProbe(t *testing.T) {
	p := &fakeProbe{}
	g := newRig(t, allCohorts(backend.BackendGoGit), func(c *backend.RouterConfig) {
		c.Capabilities = p
		c.Preflight = func(context.Context, backend.Local, backend.OperationName) []backend.FallbackReason {
			return []backend.FallbackReason{backend.ReasonCapabilityLFS}
		}
	})
	_, err := g.r.ResolveRef(ctxBG, localRepo, "HEAD")
	require.NoError(t, err)
	assert.Empty(t, p.calls)
	assert.Equal(t, int64(1), g.fallbacks(backend.OpResolveRef, backend.CohortRefs, backend.ReasonCapabilityLFS))
}

func TestRelevantCapabilitiesTable(t *testing.T) {
	has := func(op backend.OperationName, k backend.CapabilityKind) bool {
		for _, got := range backend.RelevantCapabilities(op) {
			if got == k {
				return true
			}
		}
		return false
	}
	for _, op := range backend.AllOperations() {
		if op == backend.OpClone {
			continue // no repository exists yet; asserted below
		}
		assert.Equal(t, op.Mutating(), has(op, backend.CapHooks), "%s: hooks gate exactly the mutating operations", op)
	}
	for _, op := range []backend.OperationName{backend.OpCurrentBranch, backend.OpHeadRef, backend.OpResolveRef, backend.OpRefExists,
		backend.OpRepoRoot, backend.OpGitDir, backend.OpCommonDir, backend.OpListRefs, backend.OpGetConfig, backend.OpListBranches, backend.OpListWorktrees} {
		assert.Empty(t, backend.RelevantCapabilities(op), "%s must not consult the probe", op)
	}
	assert.True(t, has(backend.OpCommit, backend.CapGPGSign))
	assert.True(t, has(backend.OpPush, backend.CapGPGSign))
	assert.False(t, has(backend.OpAdd, backend.CapGPGSign))
	assert.True(t, has(backend.OpStatus, backend.CapLFS))
	assert.True(t, has(backend.OpAdd, backend.CapLFS))
	assert.True(t, has(backend.OpMergeBase, backend.CapShallow))
	assert.True(t, has(backend.OpFetch, backend.CapLocalTransport))
	assert.True(t, has(backend.OpFetch, backend.CapSubmodule), "fetch recurses into submodules")
	assert.False(t, has(backend.OpPush, backend.CapSubmodule))
	assert.True(t, has(backend.OpPush, backend.CapSSHProxy))
	assert.Empty(t, backend.RelevantCapabilities(backend.OpClone), "clone has no repository to inspect")
}

func TestEveryCapabilityKindMapsToAKnownReasonAndEveryDetectSubIsUnique(t *testing.T) {
	for _, k := range []backend.CapabilityKind{backend.CapHooks, backend.CapGPGSign, backend.CapLFS, backend.CapUnsupportedIndex,
		backend.CapSparse, backend.CapShallow, backend.CapSubmodule, backend.CapSSHProxy, backend.CapLocalTransport} {
		assert.True(t, k.Reason().Known(), k)
		assert.NotEqual(t, backend.ReasonCapabilityDetectError, k.Reason(), k)
	}
	assert.Equal(t, backend.ReasonCapabilityDetectError, backend.CapabilityKind("made_up").Reason())
	seen := map[backend.DetectSub]bool{}
	for _, s := range backend.AllDetectSubs() {
		assert.False(t, seen[s], s)
		seen[s] = true
	}
}
