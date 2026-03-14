package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"claude-squad/log"
)

// RegisterRoutes registers all /auth/* endpoints on mux.
func RegisterRoutes(mux *http.ServeMux, waHandler *Handler, sessions *SessionManager, store *CredentialStore, setup *SetupManager, tlsCAPath string) {
	h := &httpHandlers{
		wa:       waHandler,
		sessions: sessions,
		store:    store,
		setup:    setup,
		caPath:   tlsCAPath,
	}

	mux.HandleFunc("/auth/status", h.status)
	mux.HandleFunc("/auth/register/begin", h.beginRegistration)
	mux.HandleFunc("/auth/register/finish", h.finishRegistration)
	mux.HandleFunc("/auth/login/begin", h.beginLogin)
	mux.HandleFunc("/auth/login/finish", h.finishLogin)
	mux.HandleFunc("/auth/logout", h.logout)
	mux.HandleFunc("/auth/ca.pem", h.serveCACert)

	log.InfoLog.Printf("auth: registered /auth/* routes")
}

type httpHandlers struct {
	wa       *Handler
	sessions *SessionManager
	store    *CredentialStore
	setup    *SetupManager
	caPath   string
}

// status returns the current auth configuration state.
// Used by the frontend to decide what to show (setup page, login, or nothing).
func (h *httpHandlers) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if caller is already authenticated
	authenticated := false
	if token, err := getAuthToken(r); err == nil {
		authenticated = h.sessions.ValidateAuthSession(token)
	}

	jsonResponse(w, map[string]interface{}{
		"auth_enabled":    h.wa != nil,
		"has_credentials": h.store.HasCredentials(),
		"authenticated":   authenticated,
		"setup_active":    h.setup.IsActive(),
	})
}

// beginRegistration starts a WebAuthn registration ceremony.
// Requires either an active setup token (first passkey) or an existing auth session.
func (h *httpHandlers) beginRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.wa == nil {
		http.Error(w, "passkey auth not configured", http.StatusServiceUnavailable)
		return
	}

	// Gate: either authenticated, or setup token provided, or no passkeys yet
	if h.store.HasCredentials() && !h.isAuthorised(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	_, creation, ceremonyKey, err := h.wa.BeginRegistration()
	if err != nil {
		log.ErrorLog.Printf("auth: begin registration failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"ceremony_key": ceremonyKey,
		"options":      creation,
	})
}

// finishRegistration completes a WebAuthn registration ceremony.
func (h *httpHandlers) finishRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.wa == nil {
		http.Error(w, "passkey auth not configured", http.StatusServiceUnavailable)
		return
	}

	// Gate: same as begin
	if h.store.HasCredentials() && !h.isAuthorised(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ceremonyKey := r.URL.Query().Get("ceremony_key")
	if ceremonyKey == "" {
		http.Error(w, "missing ceremony_key", http.StatusBadRequest)
		return
	}

	token, err := h.wa.FinishRegistration(ceremonyKey, r)
	if err != nil {
		log.ErrorLog.Printf("auth: finish registration failed: %v", err)
		http.Error(w, fmt.Sprintf("registration failed: %v", err), http.StatusBadRequest)
		return
	}

	setAuthCookie(w, token)
	jsonResponse(w, map[string]interface{}{"ok": true})
}

// beginLogin starts a WebAuthn login ceremony.
func (h *httpHandlers) beginLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.wa == nil {
		http.Error(w, "passkey auth not configured", http.StatusServiceUnavailable)
		return
	}
	if !h.store.HasCredentials() {
		http.Error(w, "no passkeys registered", http.StatusPreconditionFailed)
		return
	}

	assertion, ceremonyKey, err := h.wa.BeginLogin()
	if err != nil {
		log.ErrorLog.Printf("auth: begin login failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"ceremony_key": ceremonyKey,
		"options":      assertion,
	})
}

// finishLogin completes a WebAuthn login ceremony.
func (h *httpHandlers) finishLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.wa == nil {
		http.Error(w, "passkey auth not configured", http.StatusServiceUnavailable)
		return
	}

	ceremonyKey := r.URL.Query().Get("ceremony_key")
	if ceremonyKey == "" {
		http.Error(w, "missing ceremony_key", http.StatusBadRequest)
		return
	}

	token, err := h.wa.FinishLogin(ceremonyKey, r)
	if err != nil {
		log.ErrorLog.Printf("auth: finish login failed: %v", err)
		http.Error(w, fmt.Sprintf("login failed: %v", err), http.StatusUnauthorized)
		return
	}

	setAuthCookie(w, token)
	jsonResponse(w, map[string]interface{}{"ok": true})
}

// logout revokes the current session.
func (h *httpHandlers) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if token, err := getAuthToken(r); err == nil {
		h.sessions.RevokeAuthSession(token)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	jsonResponse(w, map[string]interface{}{"ok": true})
}

// serveCACert serves the CA certificate PEM so users can import it into their
// browser/OS trust store.
func (h *httpHandlers) serveCACert(w http.ResponseWriter, r *http.Request) {
	if h.caPath == "" {
		http.Error(w, "CA cert not available (HTTP mode)", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="claude-squad-ca.pem"`)
	http.ServeFile(w, r, h.caPath)
}

// isAuthorised returns true if the request carries a valid auth session token
// OR a valid setup token in the query string.
func (h *httpHandlers) isAuthorised(r *http.Request) bool {
	if token, err := getAuthToken(r); err == nil {
		if h.sessions.ValidateAuthSession(token) {
			return true
		}
	}
	// Allow setup token via query param for first-time registration flow
	if setupToken := r.URL.Query().Get("setup_token"); setupToken != "" {
		return h.setup.Validate(setupToken)
	}
	return false
}

// getAuthToken extracts the auth token from the cookie or Authorization header.
func getAuthToken(r *http.Request) (string, error) {
	// Cookie (browser clients)
	if cookie, err := r.Cookie(AuthCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, nil
	}
	// Bearer token (API/headless clients)
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:], nil
	}
	return "", fmt.Errorf("no auth token")
}

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(AuthTokenTTL().Seconds()),
		Expires:  time.Now().Add(AuthTokenTTL()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func jsonResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.ErrorLog.Printf("auth: failed to write JSON response: %v", err)
	}
}
