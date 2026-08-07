package workflows

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// fakeSessionService is a test double for SessionServiceInterface that records
// whether CreateSession was invoked — used to assert that a rejected admission check
// (Task 1.3.1d) or a dual-registered mismatched trigger (Task 1.1.1f) never reaches
// session creation. lastReq is guarded by mu since some tests fire concurrently.
type fakeSessionService struct {
	mu      sync.Mutex
	called  bool
	err     error
	lastReq *sessionv1.CreateSessionRequest
}

func (f *fakeSessionService) CreateSession(_ context.Context, req *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	f.mu.Lock()
	f.called = true
	f.lastReq = req.Msg
	f.mu.Unlock()
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

// fakeRateLimiter is a test double for triggerRateLimiterGate backed by a single
// shared golang.org/x/time/rate.Limiter (ignoring workflowID) — sufficient for a
// single-workflow rate-limit test (Task 2.4.2b). Defined locally rather than importing
// *services.TriggerRateLimiter: server/services already imports server/workflows, so
// importing it back from this file (same `package workflows`) would be a cycle.
type fakeRateLimiter struct {
	lim *rate.Limiter
}

func (f *fakeRateLimiter) Allow(_ uuid.UUID) bool {
	return f.lim.Allow()
}

// TestFireNow_RateLimited_RejectsExcessFiresInTightLoop verifies Task 2.4.2b: firing
// the same trigger repeatedly in a tight loop only lets burst-sized calls through in
// that same instant; the rest are rejected (fired_failed, not silently dropped).
func TestFireNow_RateLimited_RejectsExcessFiresInTightLoop(t *testing.T) {
	fakeSess := &fakeSessionService{}
	sched, wfRepo, _ := newTestScheduler(t, fakeSess)

	const burst = 10
	sched.SetRateLimiter(&fakeRateLimiter{lim: rate.NewLimiter(rate.Limit(10.0/60.0), burst)})
	recorder := &fakeFireEventRecorder{}
	sched.SetTriggerFireEventRepo(recorder)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:            "rate-limited-wf",
		Name:            "Rate Limited",
		Command:         "do the thing",
		TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	const attempts = 15
	successCount := 0
	for i := 0; i < attempts; i++ {
		if _, fireErr := sched.FireNow(context.Background(), wf, ""); fireErr == nil {
			successCount++
		}
	}

	assert.Equal(t, burst, successCount, "exactly burst-sized fires should succeed in a tight loop")

	rejected := 0
	for _, ev := range recorder.events {
		if ev.Outcome == "fired_failed" && ev.ErrorMessage == "rate limit exceeded" {
			rejected++
		}
	}
	assert.Equal(t, attempts-burst, rejected, "excess fires must be recorded as fired_failed, not silently dropped")
}

// TestFireTrigger_Success_UpdatesLastFiredAt verifies Task 3.2.1a/4.1.1a: a successful
// FireTrigger call bumps the Workflow's last_fired_at to (approximately) now.
func TestFireTrigger_Success_UpdatesLastFiredAt(t *testing.T) {
	fakeSess := &fakeSessionService{}
	sched, wfRepo, _ := newTestScheduler(t, fakeSess)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:            "last-fired-wf",
		Name:            "Last Fired",
		Command:         "do the thing",
		TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)
	require.Nil(t, wf.LastFiredAt)

	before := time.Now()
	_, fireErr := sched.FireTrigger(context.Background(), wf, "rendered prompt", "")
	require.NoError(t, fireErr)

	got, err := wfRepo.GetByID(context.Background(), wf.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastFiredAt)
	assert.True(t, !got.LastFiredAt.Before(before), "last_fired_at should be set to (approximately) now")
}

// TestFireTrigger_WithDeliveryID_DoesNotDoubleClaimFireEvent verifies FireTrigger's
// deliveryID-scoped skip: when called with a non-empty deliveryID (the webhook-handler
// shape, where a "pending" TriggerFireEvent row is already claimed by the caller before
// FireTrigger runs), a rate-limit/admission rejection inside FireTrigger must not
// attempt its own Create for that same (workflow_id, delivery_id) — which would either
// collide with the pending row or (if the fake recorder doesn't enforce uniqueness)
// silently double-book the audit trail. deliveryID == "" (FireNow's shape) is the
// control case showing the audit row IS recorded there.
func TestFireTrigger_WithDeliveryID_DoesNotDoubleClaimFireEvent(t *testing.T) {
	fakeSess := &fakeSessionService{}
	sched, wfRepo, _ := newTestScheduler(t, fakeSess)

	gate := &fakeAdmissionGate{admitted: false}
	sched.SetAdmissionGate(gate)
	recorder := &fakeFireEventRecorder{}
	sched.SetTriggerFireEventRepo(recorder)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:            "webhook-shaped-wf",
		Name:            "Webhook Shaped",
		Command:         "do the thing",
		TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	_, fireErr := sched.FireTrigger(context.Background(), wf, "rendered prompt", "delivery-123")
	require.Error(t, fireErr)
	assert.Empty(t, recorder.events, "FireTrigger must not record its own audit row when deliveryID is non-empty — the caller already claimed one")

	_, fireErr = sched.FireTrigger(context.Background(), wf, "rendered prompt", "")
	require.Error(t, fireErr)
	require.Len(t, recorder.events, 1, "FireTrigger's own audit row IS expected when deliveryID is empty (FireNow's shape)")
	assert.Equal(t, "fired_failed", recorder.events[0].Outcome)
}

// TestFireTrigger_NeverSetsAutoApproveFlag is the Task 3.2.1d Goal-4 verification: a
// trigger-fired CreateSessionRequest must not differ from a manually-created equivalent
// in any bypass/auto-approve/elevated-permission field. WorkflowId (attribution) is the
// only intentional difference asserted here.
func TestFireTrigger_NeverSetsAutoApproveFlag(t *testing.T) {
	fakeSess := &fakeSessionService{}
	sched, wfRepo, _ := newTestScheduler(t, fakeSess)

	wf, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:            "no-bypass-wf",
		Name:            "No Bypass",
		Command:         "do the thing",
		TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	_, fireErr := sched.FireTrigger(context.Background(), wf, "rendered prompt", "")
	require.NoError(t, fireErr)

	fakeSess.mu.Lock()
	req := fakeSess.lastReq
	fakeSess.mu.Unlock()
	require.NotNil(t, req)

	// A manually-created session request never sets these — a trigger-fired one must
	// not either. auto_yes is the concrete "bypass approval" flag on
	// CreateSessionRequest today; skip_defaults is the other field that changes how the
	// session is provisioned relative to a plain manual create.
	assert.False(t, req.GetAutoYes(), "trigger-fired sessions must not auto-approve prompts")
	assert.False(t, req.GetSkipDefaults(), "trigger-fired sessions must not skip normal session defaults")
	assert.False(t, req.GetOneShot())
	assert.Equal(t, wf.ID.String(), req.GetWorkflowId(), "WorkflowId (attribution) is the intentional difference from a manual create")
}
