package backlogsvc

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/queue"
)

// The fixtures below are the backlogsvc copies of server/services' in-package test helpers of the same names: those
// reach unexported fields of the core types, so they cannot be imported, and these rebuild them from exported API.

// bkfFlagWriter returns a setter that writes a flag through the production RPC into a config dir private to the test.
func bkfFlagWriter(t *testing.T) func(name string, on bool) {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	svc := services.NewFeatureFlagService()
	return func(name string, on bool) {
		t.Helper()
		_, err := svc.UpdateFeatureFlag(context.Background(),
			connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: name, Enabled: on}))
		require.NoError(t, err)
	}
}

// newIsolatedDefaultsService creates a DefaultsService backed by a fresh temporary state dir.
func newIsolatedDefaultsService(t *testing.T) *services.DefaultsService {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	return services.NewDefaultsService()
}

func updateQuota(t testing.TB, s *sessionv1.QuotaSettings) (*sessionv1.UpdateQuotaConfigResponse, error) {
	t.Helper()
	resp, err := services.NewQuotaSettingsService().UpdateQuotaConfig(context.Background(),
		connect.NewRequest(&sessionv1.UpdateQuotaConfigRequest{Settings: s}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

type forkTestFixture struct {
	svc     *services.SessionService
	bus     *events.EventBus
	storage *session.Storage
	poller  *session.ReviewQueuePoller
	cleanup func()
}

func setupForkTestFixture(t testing.TB) *forkTestFixture {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	svc := services.NewSessionService(storage, bus)
	// Cleanups run LIFO: the bus closes after Shutdown, which blocks on in-flight tracked goroutines.
	t.Cleanup(func() { bus.Close() })
	t.Cleanup(func() { svc.Shutdown() })

	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), session.NewInstanceStatusManager(), nil)
	svc.SetReviewQueuePoller(poller)
	// Registered after Shutdown's cleanup so it runs first: no created session may leave its SessionDriver polling.
	t.Cleanup(func() { svctest.StopPollerSessionDrivers(poller) })
	return &forkTestFixture{svc: svc, bus: bus, storage: storage, poller: poller, cleanup: func() {}}
}

// charFixture is one backlog-linked autonomous session with a subscribed event channel.
type charFixture struct {
	svc     *services.SessionService
	storage *session.Storage
	inst    *session.Instance
	title   string
	itemID  string
	itemSes string
	ch      <-chan *events.Event
}

func newCharFixture(t *testing.T, title string, role string, itemStatus session.BacklogStatus) *charFixture {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	bus := events.NewEventBus(100)
	svc := services.NewSessionService(storage, bus)
	t.Cleanup(func() { svc.Shutdown() })
	svc.SetLifecycleContext(ctx)

	inst := &session.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test",
		Status: domain.Paused, Program: "claude", AutonomousMode: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	svc.AutonomousSvc.SetInstanceFinder(func(_ string) *session.Instance { return inst })
	ch, _ := bus.Subscribe(ctx)
	f := &charFixture{svc: svc, storage: storage, inst: inst, title: title, ch: ch}

	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: title + " item", Status: string(itemStatus)})
	require.NoError(t, err)
	is, err := storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: inst.UUID, SessionRole: role})
	require.NoError(t, err)
	f.itemID, f.itemSes = item.ID, is.ID
	return f
}

func (f *charFixture) complete(outcome session.AutonomousDriverOutcome) {
	f.svc.AutonomousSvc.OnAutonomousDriverComplete(f.title, outcome)
}

// drain returns every event already published; the callback publishes synchronously.
func (f *charFixture) drain() []*events.Event {
	var out []*events.Event
	for {
		select {
		case ev := <-f.ch:
			out = append(out, ev)
		default:
			return out
		}
	}
}
