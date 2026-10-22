package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session"
)

type rejectAllValidator struct{}

func (rejectAllValidator) ValidateAuthSession(string) bool { return false }

func serveThroughAuth(method, path string) (code int, reached bool) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	Auth(rejectAllValidator{})(next).ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w.Code, reached
}

func TestAuth_should_AllowOnlySingleSegmentGenericWebhookPost(t *testing.T) {
	code, reached := serveThroughAuth(http.MethodPost, "/webhooks/stapler-squad")
	assert.True(t, reached)
	assert.Equal(t, http.StatusOK, code)

	for _, request := range []struct{ method, path string }{
		{http.MethodPost, "/webhooks/"},
		{http.MethodPost, "/webhooks/stapler-squad/extra"},
		{http.MethodGet, "/webhooks/stapler-squad"},
		{http.MethodPost, "/webhooks/github"},
		{http.MethodPost, "/api/session.v1.SessionService/CreateWorkflow"},
	} {
		code, reached = serveThroughAuth(request.method, request.path)
		assert.False(t, reached, "%s %s must remain protected", request.method, request.path)
		assert.NotEqual(t, http.StatusOK, code)
	}
}
func TestAuth_should_ExemptOnlyPreLoginAssetPaths_When_NoSession(t *testing.T) {
	for _, tc := range []struct {
		path   string
		exempt bool
	}{
		{"/login", true},
		{"/_next/static/chunks/a-1.js", true},
		{"/auth/x", true},
		{"/health", true},
		{"/favicon.ico", true},
		{"/assets/a.js", false},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		assert.Equal(t, tc.exempt, isExempt(req), "isExempt(%s)", tc.path)
	}
}

func TestAuth_should_ExemptClaimGossipEndpoints_When_PeerHasNoPasskeySession(t *testing.T) {
	code, reached := serveThroughAuth(http.MethodPost, session.ClaimAdvertisementEndpointPath)
	assert.True(t, reached)
	assert.Equal(t, http.StatusOK, code)

	code, reached = serveThroughAuth(http.MethodGet, session.ClaimLookupEndpointPath+"?url=https://github.com/o/r/issues/1")
	assert.True(t, reached)
	assert.Equal(t, http.StatusOK, code)
}

func TestAuth_should_RedirectToLogin_When_UnauthenticatedRequestHitsOtherInternalPath(t *testing.T) {
	code, reached := serveThroughAuth(http.MethodPost, "/internal/claim-advertisement/extra")
	assert.False(t, reached, "exemption must be exact-match, not a prefix")
	assert.Equal(t, http.StatusFound, code)
}

// --- self-authenticated paths (integration API on the remote listener) ---

type passkeyStub struct{}

func (passkeyStub) ValidateAuthSession(token string) bool { return token == "passkey-session" }

const selfAuthPrefix = "/api/integrations/webhooks/v1/"

func serveWithSelfAuth(match func(string) bool, r *http.Request) (code int, reached bool) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	w := httptest.NewRecorder()
	Auth(passkeyStub{}, WithSelfAuthenticatedPaths(match))(next).ServeHTTP(w, r)
	return w.Code, reached
}

func TestAuth_should_SkipPasskeyOnlyForMatchedCanonicalPaths_When_SelfAuthenticatedPathsConfigured(t *testing.T) {
	loose := func(p string) bool { return strings.HasPrefix(p, selfAuthPrefix) } // deliberately sloppy

	code, reached := serveWithSelfAuth(loose, httptest.NewRequest(http.MethodGet, selfAuthPrefix+"capability", nil))
	assert.True(t, reached, "a matched canonical path must reach its own authentication")
	assert.Equal(t, http.StatusOK, code)

	code, reached = serveWithSelfAuth(loose, httptest.NewRequest(http.MethodGet, "/api/other.v1.Service/Method", nil))
	assert.False(t, reached, "an unmatched API path still requires a passkey session")
	assert.Equal(t, http.StatusUnauthorized, code)

	for name, path := range map[string]string{
		"parent traversal": selfAuthPrefix + "capability/../../../session.v1.SessionService/DeleteSession",
		"dot segment":      selfAuthPrefix + "./capability",
		"double slash":     selfAuthPrefix + "/capability",
		"trailing slash":   selfAuthPrefix + "capability/",
	} {
		req := httptest.NewRequest(http.MethodGet, "/placeholder", nil)
		req.URL.Path = path
		_, reached := serveWithSelfAuth(loose, req)
		assert.False(t, reached, "%s must not be treated as the exempt route even by a sloppy matcher", name)
	}

	encoded := httptest.NewRequest(http.MethodGet, "/placeholder", nil)
	encoded.URL.Path = selfAuthPrefix + "../x"
	encoded.URL.RawPath = selfAuthPrefix + "%2e%2e/x"
	_, reached = serveWithSelfAuth(loose, encoded)
	assert.False(t, reached, "a path carrying special escaping is never exempt")
}

func TestAuth_should_HonourRoutesRegisteredAfterConstruction_When_MatcherIsLate(t *testing.T) {
	registered := false
	match := func(p string) bool { return registered && p == selfAuthPrefix+"capability" }
	req := func() *http.Request { return httptest.NewRequest(http.MethodGet, selfAuthPrefix+"capability", nil) }

	_, before := serveWithSelfAuth(match, req())
	registered = true
	_, after := serveWithSelfAuth(match, req())

	assert.False(t, before)
	assert.True(t, after)
}

func TestAuth_should_BehaveAsBefore_When_NoSelfAuthenticatedPathsConfigured(t *testing.T) {
	code, reached := serveThroughAuth(http.MethodGet, selfAuthPrefix+"capability")

	assert.False(t, reached)
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestAuth_should_Return401WithoutCookieOrBearer_When_NudgeProcedurePathAndValidatorRejects(t *testing.T) {
	path := "/api" + sessionv1connect.GitHubUserServiceNudgeSessionForPRProcedure
	w := httptest.NewRecorder()
	reached := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })
	Auth(rejectAllValidator{})(next).ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))

	assert.False(t, reached, "handler must not run without a valid session")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.JSONEq(t, `{"error":"unauthorized"}`, w.Body.String())
}
