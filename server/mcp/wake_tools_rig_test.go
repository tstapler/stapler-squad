package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

var wakeToolsNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// fakeWakeBackend records what the tools ask the dispatcher for.
type fakeWakeBackend struct {
	mu          sync.Mutex
	scheduled   []wakes.ScheduleRequest
	scheduleErr func(call int, req wakes.ScheduleRequest) error
	generation  int
	limits      wakes.Limits
	live        []*session.WakeTriggerData
	cancelErr   error
}

func newFakeWakeBackend() *fakeWakeBackend {
	return &fakeWakeBackend{limits: wakes.ResolveLimits(wakes.LimitSettings{})}
}

func (b *fakeWakeBackend) Schedule(_ context.Context, req wakes.ScheduleRequest) (*session.WakeTriggerData, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.scheduled = append(b.scheduled, req)
	if b.scheduleErr != nil {
		if err := b.scheduleErr(len(b.scheduled), req); err != nil {
			return nil, false, err
		}
	}
	return &session.WakeTriggerData{
		ID: uuid.NewString(), OwnerSessionUUID: req.Owner, Type: req.Spec.TriggerType(), Spec: req.Spec,
		Action: req.Action, Status: domain.WakeStatusPending, MaxFires: 1, Generation: req.Generation,
	}, true, nil
}

func (b *fakeWakeBackend) scheduleCalls() []wakes.ScheduleRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]wakes.ScheduleRequest(nil), b.scheduled...)
}

func (b *fakeWakeBackend) ListByOwner(_ context.Context, owner string) ([]*session.WakeTriggerData, error) {
	var out []*session.WakeTriggerData
	for _, r := range b.live {
		if r.OwnerSessionUUID == owner {
			out = append(out, r)
		}
	}
	return out, nil
}

func (b *fakeWakeBackend) Cancel(_ context.Context, owner, id string) (*session.WakeTriggerData, bool, error) {
	if b.cancelErr != nil {
		return nil, false, b.cancelErr
	}
	for _, r := range b.live {
		if r.ID == id && r.OwnerSessionUUID == owner {
			return r, true, nil
		}
	}
	return nil, false, wakes.ErrWakeNotFound
}

func (b *fakeWakeBackend) NextGeneration(context.Context, string) (int, error) {
	return b.generation, nil
}
func (b *fakeWakeBackend) Limits() wakes.Limits { return b.limits }

type fakeOwnerLookup struct{ facts map[string]wakes.OwnerFacts }

func (o fakeOwnerLookup) Owner(_ context.Context, id string) (wakes.OwnerFacts, error) {
	return o.facts[id], nil
}

const workspaceKeyA = "github.com/o/repo"

func liveOwner() wakes.OwnerFacts {
	return wakes.OwnerFacts{Exists: true, Live: true, WorkspaceKey: workspaceKeyA}
}

// toolRig registers the five tools over a real guidance store and a fake dispatcher.
type toolRig struct {
	t        *testing.T
	storage  *session.Storage
	backend  *fakeWakeBackend
	owners   fakeOwnerLookup
	svc      *services.GuidanceRequestService
	metrics  *wakes.Metrics
	server   *mcpserver.MCPServer
	cfg      *config.Config
	deps     *WakeToolDeps
	roles    map[string]callerRole
	mu       sync.Mutex
	answered []string
}

func newToolRig(t *testing.T) *toolRig {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	r := &toolRig{
		t: t, storage: storage, backend: newFakeWakeBackend(), metrics: wakes.NewMetrics(),
		owners: fakeOwnerLookup{facts: map[string]wakes.OwnerFacts{}}, roles: map[string]callerRole{},
		cfg: &config.Config{FeatureFlags: map[string]bool{config.WakeTriggersFlag: true}},
	}
	r.svc = services.NewGuidanceRequestService(storage, nil, nil)
	r.svc.SetAnswerListener(func(_ context.Context, id string) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.answered = append(r.answered, id)
	})
	r.deps = &WakeToolDeps{Backend: r.backend, Owners: r.owners, Answerer: r.svc}
	gate := &callerIdentityGate{metrics: r.metrics, roleOf: func(_ context.Context, id string) callerRole {
		if role, ok := r.roles[id]; ok {
			return role
		}
		return roleWork
	}}
	flags := wakeFlags{cfg: func() *config.Config { return r.cfg }}
	deps := func() *WakeToolDeps { return r.deps }
	r.server = mcpserver.NewMCPServer("test", "0.0.0")
	registerWakeTools(r.server, &wakeHandlers{gate: gate, flags: flags, deps: deps, now: func() time.Time { return wakeToolsNow }})
	registerPeerHelpTools(r.server, &peerHelpHandlers{gate: gate, flags: flags, deps: deps, storage: storage, now: func() time.Time { return wakeToolsNow }})
	return r
}

func (r *toolRig) answeredIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.answered...)
}

// as is the context of a request whose proof verified for id.
func as(id string) context.Context {
	return WithVerifiedSessionUUID(WithSessionUUID(context.Background(), id), id)
}

func (r *toolRig) call(ctx context.Context, tool string, args map[string]any) map[string]any {
	r.t.Helper()
	registered := r.server.GetTool(tool)
	require.NotNil(r.t, registered, "tool %s must be registered", tool)
	res, err := registered.Handler(ctx, makeToolReq(args))
	require.NoError(r.t, err)
	return parseResult(r.t, res)
}

func resultCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func resultErrField(m map[string]any, key string) string {
	e, _ := m["error"].(map[string]any)
	v, _ := e[key].(string)
	return v
}
