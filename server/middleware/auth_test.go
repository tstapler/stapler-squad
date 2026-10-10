package middleware

import (
	"net/http"
	"net/http/httptest"
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
		{http.MethodPost, "/webhooks/%2e%2e"},
		{http.MethodPost, "/webhooks//x"},
		{http.MethodPost, "/webhooks/x/"},
		{http.MethodPost, "/webhooks/GitHub"},
		{http.MethodGet, "/webhooks/stapler-squad"},
		{http.MethodPost, "/webhooks/github"},
		{http.MethodPost, "/api/session.v1.SessionService/CreateWorkflow"},
	} {
		code, reached = serveThroughAuth(request.method, request.path)
		assert.False(t, reached, "%s %s must remain protected", request.method, request.path)
		assert.NotEqual(t, http.StatusOK, code)
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
