package services

import (
	"context"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
)

// fakeTriggerSessionService is a test double for workflows.SessionServiceInterface
// shared by the GitHub and generic webhook handler tests. callCount is atomic so the
// concurrency dedup tests (webhook-triggers Task 2.4.1a) can safely assert on it from
// the main goroutine while many request-handling goroutines race concurrently.
type fakeTriggerSessionService struct {
	callCount atomic.Int32
	err       error
}

func (f *fakeTriggerSessionService) CreateSession(_ context.Context, _ *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	f.callCount.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&sessionv1.CreateSessionResponse{
		Session: &sessionv1.Session{Id: "fake-session-id"},
	}), nil
}

// webhookTestInfra bundles the real ent-backed repositories + Scheduler + config a
// webhook handler test needs. A real (temp-file sqlite) ent client is used —not a
// fake — specifically so the dedup tests exercise the actual DB unique-constraint
// arbitration (AC12), not application-level bookkeeping.
type webhookTestInfra struct {
	entRepo      *session.EntRepository
	workflowRepo session.WorkflowRepository
	fireEvents   session.TriggerFireEventRepository
	scheduler    *workflows.Scheduler
	sessionSvc   *fakeTriggerSessionService
	cfg          *config.Config
}

// newWebhookTestInfra builds a fresh, isolated webhookTestInfra. Each test gets its own
// temp-file DB and encryption key so dedup/rate-limit state never leaks across tests.
func newWebhookTestInfra(t *testing.T) *webhookTestInfra {
	t.Helper()
	// Isolate config so GetOrCreateEncryptionKey's SaveConfig call does not write to
	// the shared test-mode config dir (mirrors backlog_service_encryption_test.go).
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	entRepo, err := session.NewEntRepository(session.WithDatabasePath(t.TempDir() + "/webhook_test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = entRepo.Close() })

	workflowRepo := session.NewEntWorkflowRepository(entRepo.GetEntClient())
	fireEvents := session.NewEntTriggerFireEventRepository(entRepo.GetEntClient())

	sessionSvc := &fakeTriggerSessionService{}
	scheduler := workflows.NewScheduler(workflowRepo, sessionSvc, events.NewEventBus(10))

	cfg := &config.Config{FeatureFlags: map[string]bool{"webhook_triggers": true}}
	_, err = cfg.GetOrCreateEncryptionKey()
	require.NoError(t, err)

	return &webhookTestInfra{
		entRepo:      entRepo,
		workflowRepo: workflowRepo,
		fireEvents:   fireEvents,
		scheduler:    scheduler,
		sessionSvc:   sessionSvc,
		cfg:          cfg,
	}
}

// encryptSecret encrypts secret using infra.cfg's encryption key — used to build a test
// Workflow's WebhookSecretEncrypted field.
func (infra *webhookTestInfra) encryptSecret(t *testing.T, secret string) string {
	t.Helper()
	key, err := infra.cfg.GetOrCreateEncryptionKey()
	require.NoError(t, err)
	enc, err := session.EncryptToken(key, secret)
	require.NoError(t, err)
	return enc
}
