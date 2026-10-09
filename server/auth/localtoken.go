package auth

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	loginCodeTTL = 60 * time.Second
	// maxPendingLoginCodes bounds memory if something spams the code endpoint.
	maxPendingLoginCodes = 64
)

// LocalValidator accepts passkey sessions and, for non-browser clients, the
// local API token. It implements middleware.AuthValidator.
type LocalValidator struct {
	Sessions *SessionManager
	token    []byte
}

// NewLocalValidator builds a validator for token (non-empty) and sessions (may be nil).
func NewLocalValidator(sessions *SessionManager, token string) *LocalValidator {
	return &LocalValidator{Sessions: sessions, token: []byte(token)}
}

// ValidateAuthSession reports whether t is the local API token or a live session.
func (v *LocalValidator) ValidateAuthSession(t string) bool {
	if len(v.token) > 0 && subtle.ConstantTimeCompare([]byte(t), v.token) == 1 {
		return true
	}
	return v.Sessions != nil && v.Sessions.ValidateAuthSession(t)
}

// LocalLogin lets a process holding the local API token open the web UI in a
// browser without ever putting the token (or a long-lived credential) in a URL:
// the token mints a single-use, short-lived code, and the browser exchanges that
// code for a normal session cookie.
type LocalLogin struct {
	validator *LocalValidator
	sessions  *SessionManager

	mu    sync.Mutex
	codes map[string]time.Time
	now   func() time.Time
}

// NewLocalLogin builds the code-exchange handlers. sessions must be non-nil.
func NewLocalLogin(sessions *SessionManager, validator *LocalValidator) *LocalLogin {
	return &LocalLogin{validator: validator, sessions: sessions, codes: map[string]time.Time{}, now: time.Now}
}

// RegisterLocalLoginRoutes registers the code mint and exchange endpoints.
func RegisterLocalLoginRoutes(mux *http.ServeMux, l *LocalLogin) {
	mux.HandleFunc("POST /auth/local-login/code", l.mintCode)
	mux.HandleFunc("GET /auth/local-login", l.exchange)
}

func (l *LocalLogin) mintCode(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || !l.validator.isLocalToken(auth[7:]) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	code, err := randomHex(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	now := l.now()
	l.mu.Lock()
	for c, exp := range l.codes {
		if now.After(exp) {
			delete(l.codes, c)
		}
	}
	if len(l.codes) >= maxPendingLoginCodes {
		l.mu.Unlock()
		http.Error(w, "too many pending codes", http.StatusTooManyRequests)
		return
	}
	l.codes[code] = now.Add(loginCodeTTL)
	l.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code}) //nolint:errcheck
}

func (l *LocalLogin) exchange(w http.ResponseWriter, r *http.Request) {
	// The "GET" mux pattern also routes HEAD; a prefetch must not burn the code.
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code := r.URL.Query().Get("code")
	l.mu.Lock()
	exp, ok := l.codes[code]
	delete(l.codes, code) // single use, valid or not
	l.mu.Unlock()
	if !ok || l.now().After(exp) {
		http.Error(w, "invalid or expired code", http.StatusUnauthorized)
		return
	}
	tok, err := l.sessions.CreateAuthSession()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    tok,
		Path:     "/",
		MaxAge:   int(authTokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil, // the local listener is plain HTTP on loopback
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, safeNextPath(r.URL.Query().Get("next")), http.StatusFound)
}

func (v *LocalValidator) isLocalToken(t string) bool {
	return len(v.token) > 0 && subtle.ConstantTimeCompare([]byte(t), v.token) == 1
}

// safeNextPath returns next only if it is a same-origin absolute path; anything
// else (scheme-relative "//evil", backslashes, full URLs) falls back to "/".
func safeNextPath(next string) string {
	const root = "/"
	if next == "" || next[0] != '/' || strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\\\r\n") {
		return root
	}
	if u, err := url.Parse(next); err != nil || u.Host != "" || u.Scheme != "" {
		return root
	}
	return next
}
