package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// RegisterRoutes registers all /auth/* endpoints on mux.
// primaryDomain is the hostname used in the CA download filename so clients
// know which server issued the cert (e.g. "myhost.local").
// remotePort is the HTTPS port used when building invite URLs.
func RegisterRoutes(mux *http.ServeMux, waHandler *Handler, sessions *SessionManager, store *CredentialStore, setup *SetupManager, invites *InviteManager, tlsCAPath, primaryDomain string, remotePort int) {
	h := &httpHandlers{
		wa:            waHandler,
		sessions:      sessions,
		store:         store,
		setup:         setup,
		invites:       invites,
		caPath:        tlsCAPath,
		primaryDomain: primaryDomain,
		remotePort:    remotePort,
	}

	mux.HandleFunc("/auth/status", h.status)
	mux.HandleFunc("/auth/register/begin", h.beginRegistration)
	mux.HandleFunc("/auth/register/finish", h.finishRegistration)
	mux.HandleFunc("/auth/login/begin", h.beginLogin)
	mux.HandleFunc("/auth/login/finish", h.finishLogin)
	mux.HandleFunc("/auth/logout", h.logout)
	mux.HandleFunc("/auth/ca.pem", h.serveCACert)
	mux.HandleFunc("POST /auth/invite/generate", h.generateInvite)
	mux.HandleFunc("GET /auth/credentials", h.listCredentials)
	mux.HandleFunc("POST /auth/credentials/{id}/revoke", h.revokeCredential)

	log.Info("auth: registered /auth/* routes")
}

type httpHandlers struct {
	wa            *Handler
	sessions      *SessionManager
	store         *CredentialStore
	setup         *SetupManager
	invites       *InviteManager
	caPath        string
	primaryDomain string
	remotePort    int
}

// isLocalhostRequest returns true when the request originates from the loopback
// interface (127.0.0.1 or ::1). Auth is not required for local access.
func isLocalhostRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// status returns the current auth configuration state.
// Used by the frontend to decide what to show (setup page, login, or nothing).
// Requests from localhost always get auth_enabled=false so the local UI never
// requires a passkey — only remote (HTTPS) clients need to authenticate.
func (h *httpHandlers) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Local clients bypass auth entirely.
	if isLocalhostRequest(r) {
		jsonResponse(w, map[string]interface{}{
			"auth_enabled":    false,
			"has_credentials": h.store.HasCredentials(),
			"authenticated":   true,
			"setup_active":    h.setup.IsActive(),
		})
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

	_, creation, ceremonyKey, err := h.wa.BeginRegistration(r)
	if err != nil {
		log.Error("auth: begin registration failed", "err", err)
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

	// Consume the setup/invite token BEFORE FinishRegistration to prevent two
	// concurrent callers with the same token from both registering a credential.
	// Consume is mutex-protected: only the first caller wins.
	//
	// Trade-off: if FinishRegistration subsequently fails (e.g., attestation
	// error, network interruption), the invite token is permanently burned and
	// the user must generate a new one. This is acceptable for the expected
	// LAN/Tailscale environment where WebAuthn ceremony failures are rare.
	var displayName string
	if setupToken := r.URL.Query().Get("setup_token"); setupToken != "" {
		consumed := h.setup.Consume(setupToken)
		if !consumed && h.invites != nil {
			var label string
			label, consumed = h.invites.Consume(setupToken)
			if consumed {
				displayName = label
			}
		}
		if !consumed {
			http.Error(w, "setup token invalid or already used", http.StatusUnauthorized)
			return
		}
	}

	token, err := h.wa.FinishRegistration(ceremonyKey, r, displayName)
	if err != nil {
		log.Error("auth: finish registration failed", "err", err)
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

	assertion, ceremonyKey, err := h.wa.BeginLogin(r)
	if err != nil {
		log.Error("auth: begin login failed", "err", err)
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
		log.Error("auth: finish login failed", "err", err)
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
// browser/OS trust store. The download filename encodes the issuing domain,
// the issue date, and a short content hash so clients can identify which
// server produced the cert and whether it has changed.
//
// Format: stapler-squad-ca-<domain>-<YYYY-MM-DD>-<hash8>.pem
func (h *httpHandlers) serveCACert(w http.ResponseWriter, r *http.Request) {
	if h.caPath == "" {
		http.Error(w, "CA cert not available (HTTP mode)", http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(h.caPath)
	if err != nil {
		log.Error("auth: read CA cert", "err", err)
		http.Error(w, "could not read CA cert", http.StatusInternalServerError)
		return
	}

	sum := sha256.Sum256(data)
	hash8 := hex.EncodeToString(sum[:])[:8]
	date := time.Now().UTC().Format("2006-01-02")
	domain := h.primaryDomain
	if domain == "" {
		domain = "localhost"
	}
	filename := fmt.Sprintf("stapler-squad-ca-%s-%s-%s.pem", domain, date, hash8)

	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// isAuthorised returns true if the request carries a valid auth session token,
// a valid setup token, or a valid invite token in the query string.
// Tokens are NOT consumed here — consume them explicitly after the ceremony.
func (h *httpHandlers) isAuthorised(r *http.Request) bool {
	if token, err := getAuthToken(r); err == nil {
		if h.sessions.ValidateAuthSession(token) {
			return true
		}
	}
	if setupToken := r.URL.Query().Get("setup_token"); setupToken != "" {
		if h.setup.IsValid(setupToken) {
			return true
		}
		if h.invites != nil && h.invites.IsValid(setupToken) {
			return true
		}
	}
	return false
}

// isAuthorisedBySession returns true only when the request has a valid
// long-lived auth session cookie or Bearer token (not a setup/invite token).
// Used for endpoints that require an already-authenticated user.
func (h *httpHandlers) isAuthorisedBySession(r *http.Request) bool {
	token, err := getAuthToken(r)
	if err != nil {
		return false
	}
	return h.sessions.ValidateAuthSession(token)
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

// generateInvite creates a new one-time invite token. Requires an authenticated
// session (not a setup/invite token). Includes CSRF protection via Origin header.
func (h *httpHandlers) generateInvite(w http.ResponseWriter, r *http.Request) {
	if h.caPath == "" {
		http.Error(w, "invite generation requires TLS (HTTP-only mode active)", http.StatusServiceUnavailable)
		return
	}
	if h.invites == nil {
		http.Error(w, "invite manager not configured", http.StatusServiceUnavailable)
		return
	}
	if !h.isAuthorisedBySession(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Defense-in-depth: verify Origin matches the expected HTTPS origin.
	// The primary CSRF defence is SameSite=Strict on the session cookie; this
	// Origin check is a secondary layer for non-browser clients.
	//
	// Browsers omit the port for the default HTTPS port (443), so both forms
	// are accepted. When primaryDomain is empty (e.g., localhost-only mode)
	// we skip the check entirely — SameSite=Strict remains in effect.
	if origin := r.Header.Get("Origin"); origin != "" && h.primaryDomain != "" {
		domain := h.primaryDomain
		withPort := fmt.Sprintf("https://%s:%d", domain, h.remotePort)
		withoutPort := fmt.Sprintf("https://%s", domain)
		if !strings.EqualFold(origin, withPort) && !strings.EqualFold(origin, withoutPort) {
			http.Error(w, "forbidden: origin mismatch", http.StatusForbidden)
			return
		}
	}

	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	token, expiresAt, err := h.invites.Generate(body.Label)
	if err != nil {
		log.Error("auth: generate invite", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	domain := h.primaryDomain
	if domain == "" {
		domain = "localhost"
	}
	registrationURL := fmt.Sprintf("https://%s:%d/login?setup_token=%s", domain, h.remotePort, token)
	caURL := fmt.Sprintf("https://%s:%d/auth/ca.pem", domain, h.remotePort)
	ttlSeconds := int(time.Until(expiresAt).Seconds())

	// Render QR PNGs inline as base64 data URIs to avoid a second round-trip.
	regQRPNG, err := GenerateQRPNG(registrationURL)
	if err != nil {
		log.Error("auth: generate registration QR", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	caQRPNG, err := GenerateQRPNG(caURL)
	if err != nil {
		log.Error("auth: generate CA QR", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"token":            token,
		"registration_url": registrationURL,
		"ca_url":           caURL,
		"reg_qr_data_url":  "data:image/png;base64," + base64.StdEncoding.EncodeToString(regQRPNG),
		"ca_qr_data_url":   "data:image/png;base64," + base64.StdEncoding.EncodeToString(caQRPNG),
		"expires_at":       expiresAt.UTC().Format(time.RFC3339),
		"ttl_seconds":      ttlSeconds,
	})
}

// listCredentials returns all registered passkeys for the authenticated user.
func (h *httpHandlers) listCredentials(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorisedBySession(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	creds := h.store.ListCredentials()
	jsonResponse(w, map[string]interface{}{"credentials": creds})
}

// revokeCredential removes a passkey by its hex-encoded ID.
// If the last credential is removed, all auth sessions are revoked.
func (h *httpHandlers) revokeCredential(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorisedBySession(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	idHex := r.PathValue("id")
	if idHex == "" {
		http.Error(w, "missing credential id", http.StatusBadRequest)
		return
	}
	credID, err := hex.DecodeString(idHex)
	if err != nil {
		http.Error(w, "invalid credential id: must be hex", http.StatusBadRequest)
		return
	}

	if err := h.store.RemoveCredential(credID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "credential not found", http.StatusNotFound)
			return
		}
		log.Error("auth: revoke credential", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	lastCredential := !h.store.HasCredentials()
	if lastCredential {
		h.sessions.RevokeAllSessions()
		log.Info("auth: last credential revoked — all sessions invalidated")
	}

	jsonResponse(w, map[string]interface{}{
		"ok":              true,
		"last_credential": lastCredential,
	})
}

func jsonResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error("auth: failed to write JSON response", "err", err)
	}
}
