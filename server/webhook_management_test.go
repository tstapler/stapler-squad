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
	"github.com/tstapler/stapler-squad/server/middleware"
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

// --- remote listener: passkey middleware in front of the integration API ---

type passkeyStub struct{}

func (passkeyStub) ValidateAuthSession(token string) bool { return token == passkeySessionToken }

const (
	passkeySessionToken = "passkey-session"
	sensitiveAPIPath    = "/api/session.v1.SessionService/ListSessions"
	middlewareDenied    = `{"error":"unauthorized"}`
)

// remoteHarness is a wiringHarness whose mux is fronted by the real remote chain, with a
// stand-in for an ordinary passkey-protected API route so reaching it can be observed.
type remoteHarness struct {
	*wiringHarness
	chain            http.Handler
	sensitiveReached bool
}

func newRemoteHarness(t *testing.T, flags map[string]bool) *remoteHarness {
	t.Helper()
	h := &remoteHarness{wiringHarness: newWiringHarness(t, flags)}
	h.srv.mux.HandleFunc(sensitiveAPIPath, func(w http.ResponseWriter, _ *http.Request) {
		h.sensitiveReached = true
		w.WriteHeader(http.StatusOK)
	})
	registerWebhookManagement(h.srv, h.deps, h.cfg)
	h.chain = h.srv.remoteChain(middleware.Auth(passkeyStub{}, middleware.WithSelfAuthenticatedPaths(h.srv.SelfAuthenticatedPath)))
	return h
}

func (h *remoteHarness) get(path, token string) *httptest.ResponseRecorder {
	return h.getFrom("198.51.100.5:40000", path, token)
}

func (h *remoteHarness) getFrom(peer, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = peer
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.chain.ServeHTTP(rec, req)
	return rec
}

func TestRemoteChain_should_AuthenticateIntegrationRoutesByCredentialNotPasskey_When_FeatureIsOn(t *testing.T) {
	h := newRemoteHarness(t, map[string]bool{config.FeatureWebhookManagement: true, "webhook_triggers": true})

	ok := h.get(mgmtCapabilityPath, h.token)
	assert.Equal(t, http.StatusOK, ok.Code, "an integration credential must work on the remote listener")

	anon := h.get(mgmtCapabilityPath, "")
	assert.Equal(t, http.StatusUnauthorized, anon.Code)
	assert.Contains(t, anon.Body.String(), "UNAUTHENTICATED", "the integration handler, not the passkey middleware, must have answered")

	passkeyOnly := h.get(mgmtCapabilityPath, passkeySessionToken)
	assert.Equal(t, http.StatusUnauthorized, passkeyOnly.Code)
	assert.Contains(t, passkeyOnly.Body.String(), "UNAUTHENTICATED", "a passkey session must not grant management access")
}

func TestRemoteChain_should_KeepPasskeyProtectionOnEverythingElse_When_IntegrationRoutesAreExempt(t *testing.T) {
	h := newRemoteHarness(t, map[string]bool{config.FeatureWebhookManagement: true, "webhook_triggers": true})

	withIntegrationToken := h.get(sensitiveAPIPath, h.token)
	assert.Equal(t, http.StatusUnauthorized, withIntegrationToken.Code)
	assert.JSONEq(t, middlewareDenied, withIntegrationToken.Body.String(), "the passkey middleware must still guard other APIs")
	assert.False(t, h.sensitiveReached, "an integration credential must never reach an ordinary API")

	withPasskey := h.get(sensitiveAPIPath, passkeySessionToken)
	assert.Equal(t, http.StatusOK, withPasskey.Code, "passkey access to ordinary APIs is unchanged")
	assert.True(t, h.sensitiveReached)

	unknownUnderPrefix := h.get("/api/integrations/webhooks/v1/admin", h.token)
	assert.JSONEq(t, middlewareDenied, unknownUnderPrefix.Body.String(),
		"only the real route shapes are exempt, not everything under the prefix")
}

func TestRemoteChain_should_NotLetPathTricksReachOrdinaryAPIs_When_PrefixIsExempt(t *testing.T) {
	h := newRemoteHarness(t, map[string]bool{config.FeatureWebhookManagement: true, "webhook_triggers": true})

	for name, path := range map[string]string{
		"parent traversal": "/api/integrations/webhooks/v1/capability/../../../session.v1.SessionService/ListSessions",
		"dot segment":      "/api/integrations/webhooks/v1/./capability",
		"double slash":     "/api/integrations/webhooks/v1//capability",
	} {
		req := httptest.NewRequest(http.MethodGet, "/placeholder", nil)
		req.URL.Path = path
		req.RemoteAddr = "198.51.100.6:40000"
		req.Header.Set("Authorization", "Bearer "+h.token)
		rec := httptest.NewRecorder()
		h.chain.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code, name)
		assert.JSONEq(t, middlewareDenied, rec.Body.String(), name)
	}
	assert.False(t, h.sensitiveReached)
}

func TestRemoteChain_should_LeaveIntegrationRoutesBehindPasskey_When_FeatureIsOff(t *testing.T) {
	h := newRemoteHarness(t, map[string]bool{"webhook_triggers": true})

	rec := h.get(mgmtCapabilityPath, h.token)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.JSONEq(t, middlewareDenied, rec.Body.String(), "a disabled feature must not loosen the passkey boundary")
	assert.False(t, h.srv.SelfAuthenticatedPath(mgmtCapabilityPath))
}

func TestRemoteChain_should_RateLimitByPeerAddress_When_ProbedOverTheRemoteListener(t *testing.T) {
	h := newRemoteHarness(t, map[string]bool{config.FeatureWebhookManagement: true, "webhook_triggers": true})
	const prober = "203.0.113.50:44444"

	for i := 0; i < 10; i++ {
		require.Equal(t, http.StatusUnauthorized, h.getFrom(prober, mgmtCapabilityPath, "guess").Code)
	}

	assert.Equal(t, http.StatusTooManyRequests, h.getFrom(prober, mgmtCapabilityPath, h.token).Code,
		"a valid token must not bypass a block on the remote listener either")
	assert.Equal(t, http.StatusOK, h.getFrom("203.0.113.51:44444", mgmtCapabilityPath, h.token).Code, "other peers are unaffected")
	assert.Equal(t, http.StatusOK, h.getFrom(prober, sensitiveAPIPath, passkeySessionToken).Code,
		"the block covers the integration API only; passkey access is unaffected")
}
