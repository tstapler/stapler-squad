package svctest

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/stagecfg"
)

// FakeTriggerSessionService is a test double for workflows.SessionServiceInterface
// shared by the GitHub and generic webhook handler tests. CallCount is atomic so the
// concurrency dedup tests (webhook-triggers Task 2.4.1a) can safely assert on it from
// the main goroutine while many request-handling goroutines race concurrently. lastReq
// captures the most recent CreateSessionRequest (Task 3.2.1c — asserting the rendered
// prompt/WorkflowId reached CreateSession) behind the same mutex so it's safe to read
// from the concurrency tests too, even though only the single-fire tests assert on it.
type FakeTriggerSessionService struct {
	CallCount atomic.Int32
	err       error

	mu      sync.Mutex
	lastReq *sessionv1.CreateSessionRequest
}

func (f *FakeTriggerSessionService) CreateSession(_ context.Context, req *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	f.CallCount.Add(1)
	f.mu.Lock()
	f.lastReq = req.Msg
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&sessionv1.CreateSessionResponse{
		Session: &sessionv1.Session{Id: "fake-session-id"},
	}), nil
}

// LastRequest returns the most recently captured CreateSessionRequest, safe for
// concurrent use.
func (f *FakeTriggerSessionService) LastRequest() *sessionv1.CreateSessionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq
}

// WebhookTestInfra bundles the real ent-backed repositories + Scheduler + config a
// webhook handler test needs. A real (temp-file sqlite) ent client is used —not a
// fake — specifically so the dedup tests exercise the actual DB unique-constraint
// arbitration (AC12), not application-level bookkeeping.
type WebhookTestInfra struct {
	EntRepo      *session.EntRepository
	WorkflowRepo stagecfg.WorkflowRepository
	FireEvents   session.TriggerFireEventRepository
	Scheduler    *workflows.Scheduler
	SessionSvc   *FakeTriggerSessionService
	Cfg          *config.Config
}

// NewWebhookTestInfra builds a fresh, isolated WebhookTestInfra. Each test gets its own
// temp-file DB and encryption key so dedup/rate-limit state never leaks across tests.
func NewWebhookTestInfra(t *testing.T) *WebhookTestInfra {
	t.Helper()
	// Isolate config so GetOrCreateEncryptionKey's SaveConfig call does not write to
	// the shared test-mode config dir (mirrors backlog_service_encryption_test.go).
	WithIsolatedStateDir(t)

	EntRepo := session.NewTestEntRepository(t)

	WorkflowRepo := stagecfg.NewEntWorkflowRepository(EntRepo.GetEntClient())
	FireEvents := session.NewEntTriggerFireEventRepository(EntRepo.GetEntClient())

	SessionSvc := &FakeTriggerSessionService{}
	Scheduler := workflows.NewScheduler(WorkflowRepo, SessionSvc, events.NewEventBus(10))

	cfg := &config.Config{FeatureFlags: map[string]bool{"webhook_triggers": true}}
	_, err := cfg.GetOrCreateEncryptionKey()
	require.NoError(t, err)

	return &WebhookTestInfra{
		EntRepo:      EntRepo,
		WorkflowRepo: WorkflowRepo,
		FireEvents:   FireEvents,
		Scheduler:    Scheduler,
		SessionSvc:   SessionSvc,
		Cfg:          cfg,
	}
}

// EncryptSecret encrypts secret using infra.Cfg's encryption key — used to build a test
// Workflow's WebhookSecretEncrypted field.
func (infra *WebhookTestInfra) EncryptSecret(t *testing.T, secret string) string {
	t.Helper()
	key, err := infra.Cfg.GetOrCreateEncryptionKey()
	require.NoError(t, err)
	enc, err := domain.EncryptToken(key, secret)
	require.NoError(t, err)
	return enc
}

// SignWebhook computes the "sha256=<hex>" signature header value for secret/body.
func SignWebhook(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// JiraTicketBody marshals a generic-webhook payload carrying event, labels and an
// issue key/summary.
func JiraTicketBody(t *testing.T, event string, labels []string, key, summary string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]interface{}{
		"event":  event,
		"labels": labels,
		"issue":  map[string]interface{}{"key": key, "summary": summary},
	})
	require.NoError(t, err)
	return b
}

// DoGenericWebhookRequest POSTs body to /webhooks/{slug} on h, setting the signature
// header only when sigHeader is non-empty.
func DoGenericWebhookRequest(t *testing.T, h http.Handler, slug string, body []byte, sigHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/"+slug, bytes.NewReader(body))
	if sigHeader != "" {
		req.Header.Set("X-Webhook-Signature", sigHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// NewGenericWebhookWorkflow creates an enabled webhook-type Workflow at slug, with
// secret encrypted under infra's key.
func NewGenericWebhookWorkflow(t *testing.T, infra *WebhookTestInfra, slug, secret, eventFilter, labelFilter, promptTemplate string) *ent.Workflow {
	t.Helper()
	wf, err := infra.WorkflowRepo.Create(context.Background(), stagecfg.WorkflowCreateInput{
		Slug:                   slug,
		Name:                   "Generic Webhook Trigger",
		Command:                "base instructions",
		TriggerType:            "webhook",
		WebhookSlug:            slug,
		WebhookSecretEncrypted: infra.EncryptSecret(t, secret),
		EventFilter:            eventFilter,
		LabelFilter:            labelFilter,
		PromptTemplate:         promptTemplate,
		Enabled:                BoolPtr(true),
	})
	require.NoError(t, err)
	return wf
}
