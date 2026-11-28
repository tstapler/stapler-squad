package services

// Pins for the lines that connect SessionService to the autonomysvc leaf.

import (
	"context"
	"io"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/autonomysvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/autonomous"
	"github.com/tstapler/stapler-squad/session/claudectl"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
)

type wiringRespawner struct{ called chan string }

func (r wiringRespawner) AutoRespawnAutonomousWork(_ context.Context, itemID string) error {
	r.called <- itemID
	return nil
}

// wiringPool and wiringRunner stand in for a headless backend no test reaches.
type wiringPool struct{}

func (wiringPool) CallBlocking(ctx context.Context, _ headless.FeatureKey, _, _ string, _ headless.CallOptions, _ headless.CostSink) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

type wiringRunner struct{}

func (wiringRunner) Run(ctx context.Context, _ []string, _ io.Reader) (io.ReadCloser, func() error, error) {
	<-ctx.Done()
	return nil, nil, ctx.Err()
}

type wiringReviewTrigger struct{ got []string }

func (w *wiringReviewTrigger) TriggerReviewForSession(uuid string) { w.got = append(w.got, uuid) }

func newAutonomyWiringService(t *testing.T) (*SessionService, *session.Storage) {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	svc := NewSessionService(storage, events.NewEventBus(100))
	t.Cleanup(func() { svc.Shutdown() })
	return svc, storage
}

func addWiringInstance(t *testing.T, storage *session.Storage, title string, status domain.Status) *instance.Instance {
	t.Helper()
	inst := &instance.Instance{
		Title: title, UUID: title + "-uuid", Path: "/tmp/test", Status: status, Program: "claude",
		AutonomousMode: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))
	return inst
}

func linkWiringWorkItem(t *testing.T, storage *session.Storage, inst *instance.Instance) string {
	t.Helper()
	item, err := storage.CreateBacklogItem(t.Context(), backlogdto.BacklogItemData{Title: inst.Title, Status: string(domain.BacklogStatusInProgress)})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(t.Context(), backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: inst.UUID, SessionRole: instance.SessionRoleWork})
	require.NoError(t, err)
	return item.ID
}

// registerIdleDriver registers a never-started driver, so a stop is observable.
func registerIdleDriver(t *testing.T, svc *SessionService, title string) {
	t.Helper()
	d := autonomous.NewAutonomousDriver(&instance.Instance{Title: title, UUID: title + "-d"}, wiringPool{}, "goal", 1)
	svc.AutonomousSvc.RegisterDriver(title, d)
	_, ok := svc.AutonomousSvc.DriverRunID(title)
	require.True(t, ok)
}

func assertNoDriver(t *testing.T, svc *SessionService, title string) {
	t.Helper()
	_, ok := svc.AutonomousSvc.DriverRunID(title)
	assert.False(t, ok, "the driver for %q must have been stopped", title)
}

func TestAutonomyWiring_ShouldRecoverAStall_When_PollerAndRespawnerAreWired(t *testing.T) {
	t.Parallel()
	svc, storage := newAutonomyWiringService(t)
	inst := addWiringInstance(t, storage, "wire-stall", domain.Paused)
	itemID := linkWiringWorkItem(t, storage, inst)
	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	poller.AddInstance(inst)
	svc.SetReviewQueuePoller(poller)
	respawner := wiringRespawner{called: make(chan string, 1)}
	svc.SetAutonomousStuckRespawner(respawner)

	svc.AutonomousSvc.OnAutonomousDriverComplete(inst.Title, autonomous.AutonomousDriverOutcome{Turns: 3, StopReason: domain.StopReasonMaxTurns})

	select {
	case got := <-respawner.called:
		assert.Equal(t, itemID, got)
	case <-time.After(5 * time.Second):
		t.Fatal("no respawn dispatched")
	}
}

func TestAutonomyWiring_ShouldReachTheLeaf_When_HubAccessorsAreUsed(t *testing.T) {
	t.Parallel()
	svc, storage := newAutonomyWiringService(t)

	trigger := &wiringReviewTrigger{}
	svc.SetReviewGateTrigger(trigger)
	svc.TriggerReviewForSession("work-uuid")
	assert.Equal(t, []string{"work-uuid"}, trigger.got)

	assert.Same(t, svc.AutonomousSvc, svc.StaleWorkInspector())

	inst := addWiringInstance(t, storage, "wire-deadpane", domain.Active)
	linkWiringWorkItem(t, storage, inst)
	snap := domain.StopSnapshot{PaneDead: true, ExitCode: 1}
	svc.DeadPaneRecorder()(inst.UUID, inst.Title, snap, domain.ClassifyObserved(snap))
	rows, err := storage.FindOpenStuckStates(t.Context())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0].Context, "stop_reason=dead_pane")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, svc.AutonomousSvc.DriverContext().Err())
	svc.SetLifecycleContext(ctx)
	assert.Error(t, svc.AutonomousSvc.DriverContext().Err())
}

// startableInstance has the controller a driver needs to start.
func startableInstance(title string) *instance.Instance {
	inst := &instance.Instance{Title: title, UUID: title + "-uuid", Status: domain.Active, Program: "claude", AutonomousMode: true}
	inst.SetControllerForTest(&claudectl.ClaudeController{})
	return inst
}

func TestAutonomyWiring_ShouldStartDrivers_When_AHeadlessBackendIsSet(t *testing.T) {
	t.Parallel()
	for name, set := range map[string]func(*SessionService){
		"pool": func(s *SessionService) {
			s.SetHeadlessPool(headless.NewPoolWithRunner(headless.PoolConfig{}, wiringRunner{}))
		},
		"client": func(s *SessionService) { s.SetHeadlessClient(wiringPool{}) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			svc, _ := newAutonomyWiringService(t)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			svc.SetLifecycleContext(ctx)
			set(svc)

			for _, tc := range []struct {
				title string
				start func(*instance.Instance)
			}{
				{"wire-start-" + name, svc.StartAutonomousDriverForInstance},
				{"wire-timeout-" + name, func(i *instance.Instance) { svc.StartAutonomousDriverWithTimeout(i, time.Minute) }},
			} {
				tc.start(startableInstance(tc.title))
				_, ok := svc.AutonomousSvc.DriverRunID(tc.title)
				require.True(t, ok, "%s: a driver must be registered", tc.title)
				svc.StopDriverForSession(tc.title)
				assertNoDriver(t, svc, tc.title)
			}
		})
	}
}

// The creation pipeline's driver is registered, bound to the service lifecycle
// and reports back through the completion callback.
func TestAutonomyWiring_ShouldRegisterBindAndComplete_When_ACreatedSessionIsAutonomous(t *testing.T) {
	t.Parallel()
	svc, _ := newAutonomyWiringService(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.SetLifecycleContext(ctx)
	svc.SetHeadlessPool(headless.NewPoolWithRunner(headless.PoolConfig{}, wiringRunner{}))
	const title = "wire-create"

	svc.creator().StartCreationAutonomousDriver(startableInstance(title), title)
	_, ok := svc.AutonomousSvc.DriverRunID(title)
	require.True(t, ok, "the pipeline must register its driver")

	cancel() // ends the driver run; only the completion callback deregisters it
	require.Eventually(t, func() bool {
		_, live := svc.AutonomousSvc.DriverRunID(title)
		return !live
	}, 10*time.Second, 10*time.Millisecond)
}

func TestAutonomyWiring_ShouldStopTheDriver_When_TheSessionIsHibernatedDeletedOrTurnedManual(t *testing.T) {
	t.Parallel()
	off := false
	for name, act := range map[string]func(*SessionService, string) error{
		"hibernate": func(s *SessionService, id string) error {
			_, err := s.HibernateSession(context.Background(), connect.NewRequest(&sessionv1.HibernateSessionRequest{Id: id}))
			return err
		},
		"delete": func(s *SessionService, id string) error {
			_, err := s.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: id}))
			return err
		},
		"manual": func(s *SessionService, id string) error {
			_, err := s.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{Id: id, AutonomousMode: &off}))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			svc, storage := newAutonomyWiringService(t)
			title := "wire-stop-" + name
			addWiringInstance(t, storage, title, domain.Paused)
			registerIdleDriver(t, svc, title)

			err := act(svc, title)

			// Hibernating a paused session is refused after the driver is stopped.
			if name != "hibernate" {
				require.NoError(t, err)
			}
			assertNoDriver(t, svc, title)
		})
	}
}

var _ autonomysvc.AutonomousStuckRespawner = wiringRespawner{}
