package workflows

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// fakeSessionService is a test double for SessionServiceInterface that records
// whether CreateSession was invoked — used to assert that a rejected admission check
// (Task 1.3.1d) or a dual-registered mismatched trigger (Task 1.1.1f) never reaches
// session creation.
type fakeSessionService struct {
	called bool
	err    error
}

func (f *fakeSessionService) CreateSession(_ context.Context, _ *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	f.called = true
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&sessionv1.CreateSessionResponse{
		Session: &sessionv1.Session{Id: "fake-session-id"},
	}), nil
}

// fakeAdmissionGate is a test double for AdmissionGate.
type fakeAdmissionGate struct {
	admitted bool
	err      error
	called   bool
}

func (f *fakeAdmissionGate) Admit(_ context.Context) (bool, error) {
	f.called = true
	return f.admitted, f.err
}

// fakeFireEventRecorder captures TriggerFireEvent inputs recorded via recordFireEvent,
// so tests can assert an admission rejection is durably logged (Task 1.3.1d).
type fakeFireEventRecorder struct {
	events []session.TriggerFireEventInput
}

func (f *fakeFireEventRecorder) Create(_ context.Context, input session.TriggerFireEventInput) error {
	f.events = append(f.events, input)
	return nil
}

// newTestScheduler builds a Scheduler backed by a fresh in-memory ent-backed
// WorkflowRepository (same pattern as retention_test.go's newTestInfra).
func newTestScheduler(t *testing.T, sessionSvc SessionServiceInterface) (*Scheduler, session.WorkflowRepository, *session.EntRepository) {
	t.Helper()
	entRepo, err := session.NewEntRepository(session.WithDatabasePath(t.TempDir() + "/scheduler_test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { entRepo.Close() })

	wfRepo := session.NewEntWorkflowRepository(entRepo.GetEntClient())
	sched := NewScheduler(wfRepo, sessionSvc, events.NewEventBus(10))
	return sched, wfRepo, entRepo
}

// TestFireNow_AdmissionRejected_DoesNotCreateSession verifies Task 1.3.1d: when the
// admission gate rejects, CreateSession is never called, FireNow returns a non-nil
// error, and a fired_failed TriggerFireEvent is persisted.
func TestFireNow_AdmissionRejected_DoesNotCreateSession(t *testing.T) {
	fakeSess := &fakeSessionService{}
	sched, wfRepo, _ := newTestScheduler(t, fakeSess)

	gate := &fakeAdmissionGate{admitted: false}
	sched.SetAdmissionGate(gate)
	recorder := &fakeFireEventRecorder{}
	sched.SetTriggerFireEventRepo(recorder)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:            "admission-rejected-wf",
		Name:            "Admission Rejected",
		Command:         "do the thing",
		TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	_, fireErr := sched.FireNow(context.Background(), wf, "")
	require.Error(t, fireErr)
	assert.True(t, gate.called, "Admit should have been called")
	assert.False(t, fakeSess.called, "CreateSession must not be called when admission is rejected")

	require.Len(t, recorder.events, 1)
	assert.Equal(t, "fired_failed", recorder.events[0].Outcome)
	assert.NotNil(t, recorder.events[0].WorkflowID)
	assert.Equal(t, wf.ID, *recorder.events[0].WorkflowID)
}

// TestFireNow_AdmissionError_DoesNotCreateSession verifies FireNow treats an
// admission-gate error the same as an outright rejection — fail closed, not open.
func TestFireNow_AdmissionError_DoesNotCreateSession(t *testing.T) {
	fakeSess := &fakeSessionService{}
	sched, wfRepo, _ := newTestScheduler(t, fakeSess)

	gate := &fakeAdmissionGate{err: errors.New("boom")}
	sched.SetAdmissionGate(gate)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:            "admission-error-wf",
		Name:            "Admission Error",
		Command:         "do the thing",
		TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	_, fireErr := sched.FireNow(context.Background(), wf, "")
	require.Error(t, fireErr)
	assert.False(t, fakeSess.called, "CreateSession must not be called when the admission gate errors")
}

// TestFireNow_AdmissionAllowed_CreatesSession is the control case: FireNow proceeds to
// CreateSession when the admission gate allows it (or no gate is wired).
func TestFireNow_AdmissionAllowed_CreatesSession(t *testing.T) {
	fakeSess := &fakeSessionService{}
	sched, wfRepo, _ := newTestScheduler(t, fakeSess)

	gate := &fakeAdmissionGate{admitted: true}
	sched.SetAdmissionGate(gate)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:            "admission-allowed-wf",
		Name:            "Admission Allowed",
		Command:         "do the thing",
		TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	_, fireErr := sched.FireNow(context.Background(), wf, "")
	require.NoError(t, fireErr)
	assert.True(t, gate.called)
	assert.True(t, fakeSess.called, "CreateSession should be called once admission is granted")
}

// TestScheduler_Start_DoesNotRegisterMismatchedTriggerAsCron verifies Task 1.1.1f: a
// Workflow row with TriggerType="webhook" and CronEnabled=true — constructed directly
// via the repository, bypassing WorkflowService's save-time validation, to simulate a
// legacy/malformed row — is NOT registered as a cron entry by Scheduler.Start.
func TestScheduler_Start_DoesNotRegisterMismatchedTriggerAsCron(t *testing.T) {
	fakeSess := &fakeSessionService{}
	sched, wfRepo, _ := newTestScheduler(t, fakeSess)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:            "mismatched-cron-wf",
		Name:            "Mismatched",
		Command:         "do the thing",
		TargetDirectory: "/tmp/test",
		CronExpression:  "0 9 * * 1",
		CronEnabled:     true,
		TriggerType:     "webhook",
		WebhookSlug:     "some-slug",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)
	defer sched.Stop()

	sched.mu.Lock()
	_, registered := sched.entryMap[wf.ID.String()]
	sched.mu.Unlock()
	assert.False(t, registered, "a trigger_type=webhook row must not be registered as a cron entry even with cron_enabled=true")
}

// TestScheduler_Reload_DoesNotRegisterMismatchedTriggerAsCron is the Reload-path sibling
// of TestScheduler_Start_DoesNotRegisterMismatchedTriggerAsCron.
func TestScheduler_Reload_DoesNotRegisterMismatchedTriggerAsCron(t *testing.T) {
	fakeSess := &fakeSessionService{}
	sched, wfRepo, _ := newTestScheduler(t, fakeSess)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:            "mismatched-reload-wf",
		Name:            "Mismatched",
		Command:         "do the thing",
		TargetDirectory: "/tmp/test",
		CronExpression:  "0 9 * * 1",
		CronEnabled:     true,
		TriggerType:     "github_push",
		GitHubRepo:      "owner/repo",
	})
	require.NoError(t, err)

	require.NoError(t, sched.Reload(context.Background(), wf))

	sched.mu.Lock()
	_, registered := sched.entryMap[wf.ID.String()]
	sched.mu.Unlock()
	assert.False(t, registered, "a trigger_type=github_push row must not be registered as a cron entry even with cron_enabled=true")
}

// TestBackfillTriggerTypes_CorrectsLegacyRowsOnly verifies Task 1.1.1d: a row whose
// trigger_type is still at ent's migration-default ("manual") but has cron_enabled=true
// is corrected to "cron"; a row with an explicit, non-default mismatched trigger_type
// is left untouched (Task 1.1.1e's gate is responsible for refusing that one, not this
// backfill).
func TestBackfillTriggerTypes_CorrectsLegacyRowsOnly(t *testing.T) {
	_, wfRepo, _ := newTestScheduler(t, &fakeSessionService{})
	ctx := context.Background()

	legacyCron, err := wfRepo.Create(ctx, session.WorkflowCreateInput{
		Slug:            "legacy-cron",
		Name:            "Legacy Cron",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		CronExpression:  "0 9 * * 1",
		CronEnabled:     true,
		// TriggerType intentionally left unset — simulates ent's "manual" default
		// having already been applied to a pre-existing row by auto-migration.
	})
	require.NoError(t, err)

	mismatched, err := wfRepo.Create(ctx, session.WorkflowCreateInput{
		Slug:            "explicit-mismatch",
		Name:            "Explicit Mismatch",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		CronExpression:  "0 9 * * 1",
		CronEnabled:     true,
		TriggerType:     "webhook",
		WebhookSlug:     "explicit-slug",
	})
	require.NoError(t, err)

	backfillTriggerTypes(ctx, wfRepo)

	got, err := wfRepo.GetByID(ctx, legacyCron.ID)
	require.NoError(t, err)
	assert.Equal(t, "cron", got.TriggerType, "legacy row with no explicit trigger_type should be corrected to cron")

	got2, err := wfRepo.GetByID(ctx, mismatched.ID)
	require.NoError(t, err)
	assert.Equal(t, "webhook", got2.TriggerType, "explicit mismatched trigger_type must not be silently overwritten")
}

// TestScheduler_Start_RegistersValidCronTrigger is the control case proving the guard
// above doesn't just reject everything: a correctly self-consistent cron trigger is
// still registered.
func TestScheduler_Start_RegistersValidCronTrigger(t *testing.T) {
	fakeSess := &fakeSessionService{}
	sched, wfRepo, _ := newTestScheduler(t, fakeSess)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:            "valid-cron-wf",
		Name:            "Valid Cron",
		Command:         "do the thing",
		TargetDirectory: "/tmp/test",
		CronExpression:  "0 9 * * 1",
		CronEnabled:     true,
		TriggerType:     "cron",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)
	defer sched.Stop()

	sched.mu.Lock()
	_, registered := sched.entryMap[wf.ID.String()]
	sched.mu.Unlock()
	assert.True(t, registered, "a self-consistent trigger_type=cron row should still be registered")
}
