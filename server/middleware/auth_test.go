package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

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
