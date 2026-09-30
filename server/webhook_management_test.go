package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

const (
	mgmtCapabilityPath = "/api/integrations/webhooks/v1/capability"
	mgmtReconcilePath  = "/api/integrations/webhooks/v1/registrations/wiring-test/reconcile"
	wiringSecret       = "wiring-secret-0123456789abcdef0123456789"
)

type wiringHarness struct {
	srv   *Server
	deps  *ServerDependencies
	repo  *session.EntRepository
	cfg   *config.Config
	token string
}

// newWiringHarness builds the smallest Server registerWebhookManagement needs: a mux, a
// real ent-backed Storage and an event bus, with the given feature flags on a Config that
// has no persisted encryption key (the state that exposed the shared-Config bug).
func newWiringHarness(t *testing.T, flags map[string]bool) *wiringHarness {
	t.Helper()
	envtest.NewIsolatedStateDir(t)
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	h := &wiringHarness{
		srv:  &Server{mux: http.NewServeMux()},
		deps: &ServerDependencies{Storage: storage, EventBus: events.NewEventBus(10)},
		repo: repo,
		cfg:  &config.Config{FeatureFlags: flags},
	}
	configDir, err := config.GetConfigDir()
	require.NoError(t, err)
	h.token, err = repo.IssueIntegrationCredential(context.Background(), session.IntegrationCredentialInfo{
		PrincipalID: "wiring", WorkspaceID: services.WebhookWorkspaceID(configDir), AllowedDirRoot: t.TempDir(),
	})
	require.NoError(t, err)
	return h
}

func (h *wiringHarness) serve(method, path, token string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.srv.mux.ServeHTTP(rec, req)
	return rec
}

func TestRegisterWebhookManagement_should_LeaveRoutesUnregisteredAndKeyUnprovisioned_When_FlagIsOff(t *testing.T) {
	h := newWiringHarness(t, map[string]bool{"webhook_triggers": true})

	registerWebhookManagement(h.srv, h.deps, h.cfg)

	rec := h.serve(http.MethodGet, mgmtCapabilityPath, h.token, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code, "a disabled feature must be indistinguishable from a path that never existed")
	assert.Empty(t, h.cfg.MachineEncryptionKey, "a disabled feature must not provision key material")
}

func TestRegisterWebhookManagement_should_ServeAuthenticatedRoutesAndProvisionKey_When_FlagIsOn(t *testing.T) {
	h := newWiringHarness(t, map[string]bool{config.FeatureWebhookManagement: true, "webhook_triggers": true})

	registerWebhookManagement(h.srv, h.deps, h.cfg)

	assert.NotEmpty(t, h.cfg.MachineEncryptionKey, "the key must be provisioned at boot, on the shared Config")
	assert.Equal(t, http.StatusUnauthorized, h.serve(http.MethodGet, mgmtCapabilityPath, "", nil).Code)
	assert.Equal(t, http.StatusUnauthorized, h.serve(http.MethodGet, mgmtCapabilityPath, "sqi_not-a-real-token", nil).Code)
	rec := h.serve(http.MethodGet, mgmtCapabilityPath, h.token, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var capability services.WebhookCapabilityResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &capability))
	assert.True(t, capability.ReceiverEnabled)
	assert.Contains(t, capability.Operations, "emergency_cleanup")
}

func TestRegisterWebhookManagement_should_ReportReceiverDisabled_When_WebhookTriggersIsOff(t *testing.T) {
	h := newWiringHarness(t, map[string]bool{config.FeatureWebhookManagement: true})

	registerWebhookManagement(h.srv, h.deps, h.cfg)

	rec := h.serve(http.MethodGet, mgmtCapabilityPath, h.token, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var capability services.WebhookCapabilityResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &capability))
	assert.False(t, capability.ReceiverEnabled)
}

func TestRegisterWebhookManagement_should_NotPanicOrRegister_When_StorageIsMissing(t *testing.T) {
	h := newWiringHarness(t, map[string]bool{config.FeatureWebhookManagement: true})
	h.deps.Storage = nil

	assert.NotPanics(t, func() { registerWebhookManagement(h.srv, h.deps, h.cfg) })

	assert.Equal(t, http.StatusNotFound, h.serve(http.MethodGet, mgmtCapabilityPath, h.token, nil).Code)
}

// The receiver and the management service must decrypt with the same machine key. Each
// *config.Config lazily generates its own key when none is persisted, so a secret encrypted
// by one instance is unreadable to another; registerWebhookManagement avoids that by using the
// receiver's Config. This drives the real wiring, then a real signed delivery.
func TestRegisterWebhookManagement_should_ProduceRegistrationsTheReceiverCanVerify_When_ConfigHasNoPersistedKey(t *testing.T) {
	h := newWiringHarness(t, map[string]bool{config.FeatureWebhookManagement: true, "webhook_triggers": true})
	require.Empty(t, h.cfg.MachineEncryptionKey, "precondition: no key yet, as on a fresh instance")
	fireEvents := session.NewEntTriggerFireEventRepository(h.repo.GetEntClient())
	receiver := services.NewGenericWebhookHandler(session.NewEntWorkflowRepository(h.repo.GetEntClient()), nil, fireEvents, h.cfg)
	receiver.RegisterRoutes(h.srv.mux) // the receiver holds h.cfg, exactly as server.go wires it
	registerWebhookManagement(h.srv, h.deps, h.cfg)

	body := `{"request_id":"wiring-req-0001","expected_version":0,` +
		`"compat":{"contract_version":"v1","capability_revision":1,"manifest_sha256":"` + strings.Repeat("a", 64) + `",` +
		`"signer_key_id":"k1","client_name":"wiring","client_version":"0"},` +
		`"webhook":{"slug":"wiring-hook","name":"wiring","command":"noop","target_directory":"` + h.allowedDir(t) + `","event_filter":"only-this-event"},` +
		`"secret":"` + wiringSecret + `"}`
	create := h.serve(http.MethodPost, mgmtReconcilePath, h.token, []byte(body))
	require.Equal(t, http.StatusCreated, create.Code, create.Body.String())

	payload := []byte(`{"event":"something-else"}`)
	mac := hmac.New(sha256.New, []byte(wiringSecret))
	mac.Write(payload)
	signed := httptest.NewRequest(http.MethodPost, "/webhooks/wiring-hook", bytes.NewReader(payload))
	signed.Header.Set("X-Webhook-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	good := httptest.NewRecorder()
	h.srv.mux.ServeHTTP(good, signed)
	assert.Equal(t, http.StatusOK, good.Code, "a correctly signed delivery must verify against the registered secret")

	forged := httptest.NewRequest(http.MethodPost, "/webhooks/wiring-hook", bytes.NewReader(payload))
	forged.Header.Set("X-Webhook-Signature", "sha256=00")
	bad := httptest.NewRecorder()
	h.srv.mux.ServeHTTP(bad, forged)
	assert.Equal(t, http.StatusUnauthorized, bad.Code)
}

// allowedDir returns the directory root the harness credential was issued for.
func (h *wiringHarness) allowedDir(t *testing.T) string {
	t.Helper()
	cred, err := h.repo.AuthenticateIntegrationToken(context.Background(), h.token)
	require.NoError(t, err)
	return cred.AllowedDirRoot
}
