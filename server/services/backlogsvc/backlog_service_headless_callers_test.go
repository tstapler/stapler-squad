package backlogsvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/headless"
)

// Characterisation of the headless-caller wiring (SetHeadlessPool,
// SetBackendSelector, SetGeminiCaller) and the resolveHeadlessCaller branches
// not covered by the Story 2.3.1 tests in backlog_service_test.go.

// fakeSelectorBackend is a registrable headless.Backend for Selector tests.
type fakeSelectorBackend struct {
	fakeAvailabilityCaller
	name string
}

func (f *fakeSelectorBackend) Name() string                { return f.name }
func (f *fakeSelectorBackend) Capabilities() headless.Caps { return headless.Caps{} }

func newHeadlessCallersSvc(t *testing.T) (*BacklogService, *fakeHeadlessPool) {
	t.Helper()
	svc := NewBacklogService(svctest.CreateTestStorage(t), nil, nil, nil, nil, nil)
	pool := &fakeHeadlessPool{}
	svc.SetHeadlessPool(pool)
	return svc, pool
}

func TestSetHeadlessPool_should_RegisterPoolUnderClaude(t *testing.T) {
	t.Parallel()
	svc, pool := newHeadlessCallersSvc(t)
	assert.Same(t, headless.PoolClient(pool), svc.headlessPool)
	assert.Same(t, headless.PoolClient(pool), svc.headlessCallers["claude"])
}

func TestSetGeminiCaller_should_IgnoreNilAndRegisterNonNil(t *testing.T) {
	t.Parallel()
	svc, _ := newHeadlessCallersSvc(t)

	svc.SetGeminiCaller(nil)
	_, registered := svc.headlessCallers["gemini"]
	assert.False(t, registered, "a nil caller must not be registered")

	g := &headless.GeminiCaller{}
	svc.SetGeminiCaller(g)
	assert.Same(t, headless.PoolClient(g), svc.headlessCallers["gemini"])
}

func TestSetGeminiCaller_should_CreateMapWhenPoolNeverSet(t *testing.T) {
	t.Parallel()
	svc := NewBacklogService(svctest.CreateTestStorage(t), nil, nil, nil, nil, nil)
	g := &headless.GeminiCaller{}
	svc.SetGeminiCaller(g)
	assert.Same(t, headless.PoolClient(g), svc.headlessCallers["gemini"])
}

func TestSetBackendSelector_should_RouteDefaultClientButKeepRawClaude(t *testing.T) {
	t.Parallel()
	svc, pool := newHeadlessCallersSvc(t)
	sel := headless.NewSelector(nil)

	svc.SetBackendSelector(sel)

	sc, ok := svc.headlessPool.(*headless.SelectingClient)
	require.True(t, ok, "default client must become a SelectingClient")
	assert.Same(t, sel, sc.Selector)
	assert.Empty(t, sc.StageProgram)
	assert.Same(t, headless.PoolClient(pool), svc.headlessCallers["claude"], "raw pool stays registered under claude")
}

func TestSetBackendSelector_should_LeavePoolUntouchedForNilSelector(t *testing.T) {
	t.Parallel()
	svc, pool := newHeadlessCallersSvc(t)
	svc.SetBackendSelector(nil)
	assert.Same(t, headless.PoolClient(pool), svc.headlessPool)
}

func TestResolveHeadlessCaller_should_ReturnRawPoolForClaude_And_SelectingClientForEmpty_When_SelectorSet(t *testing.T) {
	t.Parallel()
	svc, pool := newHeadlessCallersSvc(t)
	sel := headless.NewSelector(nil)
	svc.SetBackendSelector(sel)

	caller, configured, reason := svc.resolveHeadlessCaller("claude", "item-1", "triage")
	assert.Same(t, headless.PoolClient(pool), caller, "explicit claude is never re-routed")
	assert.Empty(t, configured)
	assert.Empty(t, reason)

	caller, configured, reason = svc.resolveHeadlessCaller("", "item-1", "triage")
	sc, ok := caller.(*headless.SelectingClient)
	require.True(t, ok)
	assert.Same(t, sel, sc.Selector)
	assert.Empty(t, sc.StageProgram)
	assert.Empty(t, configured)
	assert.Empty(t, reason)
}

func TestResolveHeadlessCaller_should_PinStageProgram_When_SelectorServesKnownBackend(t *testing.T) {
	t.Parallel()
	svc, _ := newHeadlessCallersSvc(t)
	sel := headless.NewSelector(nil)
	sel.Register(&fakeSelectorBackend{fakeAvailabilityCaller: fakeAvailabilityCaller{available: true}, name: headless.BackendGemini})
	svc.SetBackendSelector(sel)

	caller, configured, reason := svc.resolveHeadlessCaller(headless.BackendGemini, "item-1", "triage")
	sc, ok := caller.(*headless.SelectingClient)
	require.True(t, ok)
	assert.Same(t, sel, sc.Selector)
	assert.Equal(t, headless.BackendGemini, sc.StageProgram)
	assert.Empty(t, configured)
	assert.Empty(t, reason)
}

func TestResolveHeadlessCaller_should_ReportSelectorFallback_When_KnownBackendUnregistered(t *testing.T) {
	t.Parallel()
	svc, _ := newHeadlessCallersSvc(t)
	sel := headless.NewSelector(nil)
	svc.SetBackendSelector(sel)

	caller, configured, reason := svc.resolveHeadlessCaller(headless.BackendGemini, "item-1", "triage")
	sc, ok := caller.(*headless.SelectingClient)
	require.True(t, ok)
	assert.Same(t, sel, sc.Selector)
	assert.Empty(t, sc.StageProgram, "fallback client must not pin the unavailable program")
	assert.Equal(t, headless.BackendGemini, configured)
	assert.Equal(t, "unsupported_program", reason)
}

func TestResolveHeadlessCaller_should_SkipSelector_When_ProgramIsNotKnownBackend(t *testing.T) {
	t.Parallel()
	svc, pool := newHeadlessCallersSvc(t)
	svc.SetBackendSelector(headless.NewSelector(nil))

	caller, configured, reason := svc.resolveHeadlessCaller("no-such-backend", "item-1", "triage")
	assert.Same(t, svc.headlessPool, caller)
	assert.NotSame(t, headless.PoolClient(pool), caller, "headlessPool is the selecting client once a selector is set")
	assert.Equal(t, "no-such-backend", configured)
	assert.Equal(t, "unsupported_program", reason)
}

func TestResolveHeadlessCaller_should_ReturnDefaultCallerWithoutFallback_When_PoolNeverSetAndProgramIsClaude(t *testing.T) {
	t.Parallel()
	svc := NewBacklogService(svctest.CreateTestStorage(t), nil, nil, nil, nil, nil)

	caller, configured, reason := svc.resolveHeadlessCaller("claude", "item-1", "triage")
	assert.Nil(t, caller, "no pool wired: the default claude caller is the (nil) headlessPool")
	assert.Empty(t, configured)
	assert.Empty(t, reason, "explicit claude is never reported as a fallback")
}

func TestResolveHeadlessCaller_should_PassPerStageFeatureKeyToSelector(t *testing.T) {
	t.Parallel()
	svc, _ := newHeadlessCallersSvc(t)
	sel := headless.NewSelector(nil)
	var seen []string
	sel.SetFallbackRecorder(func(r headless.Resolution) { seen = append(seen, r.Feature) })
	svc.SetBackendSelector(sel)

	for _, stage := range []string{"triage", "review"} {
		_, _, reason := svc.resolveHeadlessCaller(headless.BackendGemini, "item-1", stage)
		require.Equal(t, "unsupported_program", reason)
	}
	assert.Equal(t, []string{"triage", "review"}, seen, "selector must see each stage as its feature key")
}
