package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
)

func TestLocalValidator_should_AcceptTokenOrSession_And_RejectOthers(t *testing.T) {
	sessions := NewSessionManager("")
	t.Cleanup(sessions.Close)
	v := NewLocalValidator(sessions, "tok")
	sess, err := sessions.CreateAuthSession()
	require.NoError(t, err)

	assert.True(t, v.ValidateAuthSession("tok"))
	assert.True(t, v.ValidateAuthSession(sess))
	assert.False(t, v.ValidateAuthSession("nope"))
	assert.False(t, v.ValidateAuthSession(""))
	assert.False(t, NewLocalValidator(nil, "").ValidateAuthSession(""), "empty token must not match empty input")
}

func newLoginHarness(t *testing.T) (*LocalLogin, *SessionManager) {
	t.Helper()
	sessions := NewSessionManager("")
	t.Cleanup(sessions.Close)
	return NewLocalLogin(sessions, NewLocalValidator(sessions, "tok")), sessions
}

func mint(l *LocalLogin, bearer string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/auth/local-login/code", nil)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	l.mintCode(w, r)
	return w
}

func TestLocalLogin_should_Reject_MintWithoutLocalToken(t *testing.T) {
	l, sessions := newLoginHarness(t)
	sess, err := sessions.CreateAuthSession()
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, mint(l, "").Code)
	assert.Equal(t, http.StatusUnauthorized, mint(l, "wrong").Code)
	assert.Equal(t, http.StatusUnauthorized, mint(l, sess).Code, "a browser session must not mint codes")
}

func TestLocalLogin_should_ExchangeCodeOnce_ForSessionCookie(t *testing.T) {
	l, sessions := newLoginHarness(t)
	w := mint(l, "tok")
	require.Equal(t, http.StatusOK, w.Code)
	var body struct{ Code string }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	exchange := func(code, next string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/auth/local-login?code="+code+"&next="+next, nil)
		rec := httptest.NewRecorder()
		l.exchange(rec, r)
		return rec
	}
	rec := exchange(body.Code, "/backlog?item=1")
	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/backlog?item=1", rec.Header().Get("Location"))
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.True(t, cookies[0].HttpOnly)
	assert.True(t, sessions.ValidateAuthSession(cookies[0].Value))

	assert.Equal(t, http.StatusUnauthorized, exchange(body.Code, "/").Code, "code is single use")
}

func TestLocalLogin_should_Reject_ExpiredCode(t *testing.T) {
	l, _ := newLoginHarness(t)
	now := time.Now()
	l.now = func() time.Time { return now }
	var body struct{ Code string }
	require.NoError(t, json.Unmarshal(mint(l, "tok").Body.Bytes(), &body))
	l.now = func() time.Time { return now.Add(loginCodeTTL + time.Second) }
	rec := httptest.NewRecorder()
	l.exchange(rec, httptest.NewRequest(http.MethodGet, "/auth/local-login?code="+body.Code, nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestSafeNextPath_should_OnlyAllowSameOriginPaths(t *testing.T) {
	for _, bad := range []string{"", "//evil.example", "https://evil.example", "/\\evil.example", "evil", "/a\r\nSet-Cookie: x=1"} {
		assert.Equal(t, "/", safeNextPath(bad), bad)
	}
	assert.Equal(t, "/backlog?item=1", safeNextPath("/backlog?item=1"))
}

func TestLocalLogin_should_FallBackToRoot_When_NextIsOffOrigin(t *testing.T) {
	l, _ := newLoginHarness(t)
	var body struct{ Code string }
	require.NoError(t, json.Unmarshal(mint(l, "tok").Body.Bytes(), &body))
	rec := httptest.NewRecorder()
	l.exchange(rec, httptest.NewRequest(http.MethodGet, "/auth/local-login?code="+body.Code+"&next=//evil.example", nil))
	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/", rec.Header().Get("Location"))
}

func TestLocalLogin_should_NotConsumeCode_When_MethodIsHEAD(t *testing.T) {
	l, _ := newLoginHarness(t)
	var body struct{ Code string }
	require.NoError(t, json.Unmarshal(mint(l, "tok").Body.Bytes(), &body))
	rec := httptest.NewRecorder()
	l.exchange(rec, httptest.NewRequest(http.MethodHead, "/auth/local-login?code="+body.Code, nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	rec = httptest.NewRecorder()
	l.exchange(rec, httptest.NewRequest(http.MethodGet, "/auth/local-login?code="+body.Code, nil))
	assert.Equal(t, http.StatusFound, rec.Code)
}

func TestLocalLogin_should_Reject_Mint_When_ValidatorHasNoToken(t *testing.T) {
	sessions := NewSessionManager("")
	t.Cleanup(sessions.Close)
	l := NewLocalLogin(sessions, NewLocalValidator(sessions, ""))
	assert.Equal(t, http.StatusUnauthorized, mint(l, "").Code)
	r := httptest.NewRequest(http.MethodPost, "/auth/local-login/code", nil)
	r.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	l.mintCode(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLocalLogin_should_CapPendingCodes(t *testing.T) {
	l, _ := newLoginHarness(t)
	for i := 0; i < maxPendingLoginCodes; i++ {
		require.Equal(t, http.StatusOK, mint(l, "tok").Code)
	}
	assert.Equal(t, http.StatusTooManyRequests, mint(l, "tok").Code)
}

func TestStatus_should_NotTrustLoopbackRemoteAddr_When_RequireLocalAuth(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	store, err := NewCredentialStore()
	require.NoError(t, err)
	sessions := NewSessionManager("")
	t.Cleanup(sessions.Close)

	status := func(requireAuth bool) map[string]interface{} {
		h := &httpHandlers{store: store, setup: NewSetupManager(), sessions: sessions, requireLocalAuth: requireAuth}
		r := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
		r.RemoteAddr = "127.0.0.1:5555"
		w := httptest.NewRecorder()
		h.status(w, r)
		var out map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out
	}
	off := status(false)
	assert.Equal(t, false, off["auth_enabled"])
	assert.Equal(t, true, off["authenticated"])
	on := status(true)
	assert.Equal(t, true, on["auth_enabled"])
	assert.Equal(t, false, on["authenticated"], "loopback RemoteAddr must not imply authenticated")
}

func TestLocalLogin_should_ServeMinimalStatus_When_RegisteredWithoutPasskeySubsystem(t *testing.T) {
	l, sessions := newLoginHarness(t)
	mux := http.NewServeMux()
	RegisterLocalLoginRoutes(mux, l, true)

	get := func(cookie string) map[string]interface{} {
		r := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
		r.RemoteAddr = "127.0.0.1:1"
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: AuthCookieName, Value: cookie})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		require.Equal(t, http.StatusOK, w.Code)
		var out map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out
	}
	assert.Equal(t, false, get("")["authenticated"])
	assert.Equal(t, true, get("")["auth_enabled"])
	sess, err := sessions.CreateAuthSession()
	require.NoError(t, err)
	assert.Equal(t, true, get(sess)["authenticated"])

	// Without the flag the route is left to RegisterRoutes.
	bare := http.NewServeMux()
	RegisterLocalLoginRoutes(bare, l, false)
	w := httptest.NewRecorder()
	bare.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/status", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestLocalLogin_should_IssueShortLivedSessionCookie(t *testing.T) {
	l, sessions := newLoginHarness(t)
	var body struct{ Code string }
	require.NoError(t, json.Unmarshal(mint(l, "tok").Body.Bytes(), &body))
	rec := httptest.NewRecorder()
	l.exchange(rec, httptest.NewRequest(http.MethodGet, "/auth/local-login?code="+body.Code, nil))
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, int(localLoginSessionTTL.Seconds()), cookies[0].MaxAge)
	assert.True(t, sessions.ValidateAuthSession(cookies[0].Value))
}
