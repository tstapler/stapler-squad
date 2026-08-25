package auth

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/log"

	"github.com/go-webauthn/webauthn/webauthn"
)

// Handler wraps the go-webauthn/webauthn library and provides dynamic RPID
// selection to support multiple hostnames.
type Handler struct {
	mu sync.RWMutex
	// A map of WebAuthn instances, keyed by RPID.
	webauthn map[string]*webauthn.WebAuthn
	// The list of allowed hostnames (RPIDs).
	rpIDs []string
	// The origins passed to every WebAuthn instance, including ones
	// registered later via hostnameValidator.
	origins []string
	// Optional: called with a hostname webauthnForHost couldn't match
	// against any known rpID. If it returns true, that hostname is
	// registered as a new rpID on the fly instead of being rejected. Left
	// nil to disable dynamic registration (e.g. in tests).
	hostnameValidator func(hostname string) bool
	// Guard rails around hostnameValidator: negativeCache avoids re-resolving
	// a hostname that already failed validation recently, and ipLimiter caps
	// how often a single source IP may trigger a validation attempt at all --
	// see hostname_guard.go.
	negativeCache *negativeHostnameCache
	ipLimiter     *sourceIPLimiter
	// The credential and session stores.
	store   *CredentialStore
	session *SessionManager
}

// NewHandler creates a new WebAuthn handler supporting multiple domains.
// hostnameValidator, if non-nil, is consulted by webauthnForHost to decide
// whether a hostname not present in rpIDs at construction time may still be
// registered as a new rpID at request time -- see webauthnForHost for why
// this exists.
func NewHandler(rpIDs []string, origins []string, store *CredentialStore, session *SessionManager, hostnameValidator func(string) bool) (*Handler, error) {
	if len(rpIDs) == 0 {
		return nil, fmt.Errorf("at least one RPID is required")
	}

	w := make(map[string]*webauthn.WebAuthn, len(rpIDs))
	for _, rpID := range rpIDs {
		wa, err := webauthn.New(&webauthn.Config{
			RPDisplayName: "Stapler Squad",
			RPID:          rpID,
			RPOrigins:     origins,
			Debug:         false,
		})
		if err != nil {
			return nil, fmt.Errorf("configure webauthn for rpID %s: %w", rpID, err)
		}
		w[rpID] = wa
	}

	log.Info("auth: WebAuthn configured", "rpIDs", rpIDs, "origins", origins)

	return &Handler{
		webauthn:          w,
		rpIDs:             rpIDs,
		origins:           origins,
		hostnameValidator: hostnameValidator,
		negativeCache:     newNegativeHostnameCache(),
		ipLimiter:         newSourceIPLimiter(),
		store:             store,
		session:           session,
	}, nil
}

// webauthnForHost selects the correct WebAuthn instance based on the request Host.
// It iterates through the configured RPIDs and returns the first one that is a
// suffix of the request's hostname. This allows a single server to handle
// requests for e.g. onyx.local and onyx.staplerhome.internal.
//
// rpIDs is otherwise fixed at NewHandler time, which goes stale if a LAN
// hostname wasn't resolvable yet when the server started (e.g. the service
// launched before DHCP/Wi-Fi association finished) -- the discovery logic
// itself can be correct and still miss a hostname because of *when* it ran.
// Rather than rejecting a request from a legitimate, now-resolvable LAN
// hostname for the rest of the process's life, fall back to hostnameValidator
// and register the hostname as a new rpID on demand.
func (h *Handler) webauthnForHost(r *http.Request) (*webauthn.WebAuthn, error) {
	hostname := r.Host
	if host, _, err := net.SplitHostPort(r.Host); err == nil {
		hostname = host
	}

	h.mu.RLock()
	for _, rpID := range h.rpIDs {
		if hostnameMatchesRPID(hostname, rpID) {
			wa := h.webauthn[rpID]
			h.mu.RUnlock()
			return wa, nil
		}
	}
	h.mu.RUnlock()

	if h.hostnameValidator == nil {
		return nil, fmt.Errorf("no valid rpID found for host %s", hostname)
	}

	if h.negativeCache.IsNegative(hostname) {
		return nil, fmt.Errorf("no valid rpID found for host %s", hostname)
	}

	if !h.ipLimiter.Allow(sourceIP(r)) {
		return nil, fmt.Errorf("too many rpID validation attempts from this source, try again later")
	}

	if !h.hostnameValidator(hostname) {
		h.negativeCache.MarkNegative(hostname)
		return nil, fmt.Errorf("no valid rpID found for host %s", hostname)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if wa, ok := h.webauthn[hostname]; ok {
		return wa, nil
	}
	// go-webauthn requires an exact origin match (no suffix/wildcard support --
	// see IsOriginInHaystack in go-webauthn/protocol/client.go), so a hostname
	// accepted as a new rpID also needs its own origin added here, or every
	// real ceremony from it will fail verification despite webauthnForHost
	// having selected an instance for it.
	newOrigin := originForHost(h.origins, hostname)
	if newOrigin == "" {
		return nil, fmt.Errorf("determine origin for dynamically registered rpID %s: no existing origin to derive scheme/port from", hostname)
	}
	origins := append(append([]string{}, h.origins...), newOrigin)
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Stapler Squad",
		RPID:          hostname,
		RPOrigins:     origins,
		Debug:         false,
	})
	if err != nil {
		return nil, fmt.Errorf("configure webauthn for rpID %s: %w", hostname, err)
	}
	h.webauthn[hostname] = wa
	h.rpIDs = append(h.rpIDs, hostname)
	h.origins = origins
	log.Info("auth: dynamically registered new rpID", "rpID", hostname, "origin", newOrigin)
	return wa, nil
}

// hostnameMatchesRPID reports whether hostname is rpID itself or a subdomain
// of it. A plain strings.HasSuffix check has no label boundary, so a
// registered rpID like "onyx.local" would also match an attacker-chosen Host
// header like "evil-onyx.local" -- requiring only a spoofed Host header, not
// any DNS control.
func hostnameMatchesRPID(hostname, rpID string) bool {
	return hostname == rpID || strings.HasSuffix(hostname, "."+rpID)
}

// originForHost builds the origin (scheme + hostname + port) that a
// dynamically-registered rpID should be allowed to authenticate from, reusing
// the scheme and port of an existing configured origin -- every origin in
// this handler is served by the same listener, so they share both. Returns
// "" if origins is empty or unparseable.
func originForHost(origins []string, hostname string) string {
	if len(origins) == 0 {
		return ""
	}
	u, err := url.Parse(origins[0])
	if err != nil || u.Scheme == "" {
		return ""
	}
	if port := u.Port(); port != "" {
		return fmt.Sprintf("%s://%s:%s", u.Scheme, hostname, port)
	}
	return fmt.Sprintf("%s://%s", u.Scheme, hostname)
}

// BeginRegistration starts a passkey registration ceremony.
func (h *Handler) BeginRegistration(r *http.Request) (*webauthn.SessionData, interface{}, string, error) {
	wa, err := h.webauthnForHost(r)
	if err != nil {
		return nil, nil, "", err
	}
	user := newLocalUser(h.store)

	creation, sessionData, err := wa.BeginRegistration(user)
	if err != nil {
		return nil, nil, "", fmt.Errorf("begin registration: %w", err)
	}

	key, err := h.session.StoreCeremony(ceremonyRegister, *sessionData)
	if err != nil {
		return nil, nil, "", fmt.Errorf("store ceremony: %w", err)
	}

	return sessionData, creation, key, nil
}

// FinishRegistration completes the registration ceremony.
// displayName is the label provided during invite generation; empty string is accepted.
func (h *Handler) FinishRegistration(ceremonyKey string, r *http.Request, displayName string) (string, error) {
	wa, err := h.webauthnForHost(r)
	if err != nil {
		return "", err
	}

	sessionData, ok := h.session.GetCeremony(ceremonyKey)
	if !ok {
		return "", fmt.Errorf("ceremony session not found or expired")
	}

	user := newLocalUser(h.store)
	cred, err := wa.FinishRegistration(user, sessionData, r)
	if err != nil {
		return "", fmt.Errorf("finish registration: %w", err)
	}

	if err := h.store.AddCredential(*cred, displayName); err != nil {
		return "", fmt.Errorf("persist credential: %w", err)
	}

	token, err := h.session.CreateAuthSession()
	if err != nil {
		return "", fmt.Errorf("create auth session: %w", err)
	}

	log.Info("auth: new passkey registered", "credential_id", fmt.Sprintf("%x", cred.ID))
	return token, nil
}

// BeginLogin starts a passkey login ceremony.
func (h *Handler) BeginLogin(r *http.Request) (interface{}, string, error) {
	wa, err := h.webauthnForHost(r)
	if err != nil {
		return nil, "", err
	}
	user := newLocalUser(h.store)

	assertion, sessionData, err := wa.BeginLogin(user)
	if err != nil {
		return nil, "", fmt.Errorf("begin login: %w", err)
	}

	key, err := h.session.StoreCeremony(ceremonyLogin, *sessionData)
	if err != nil {
		return nil, "", fmt.Errorf("store ceremony: %w", err)
	}

	return assertion, key, nil
}

// FinishLogin completes the login ceremony.
func (h *Handler) FinishLogin(ceremonyKey string, r *http.Request) (string, error) {
	wa, err := h.webauthnForHost(r)
	if err != nil {
		return "", err
	}
	sessionData, ok := h.session.GetCeremony(ceremonyKey)
	if !ok {
		return "", fmt.Errorf("ceremony session not found or expired")
	}

	user := newLocalUser(h.store)
	cred, err := wa.FinishLogin(user, sessionData, r)
	if err != nil {
		return "", fmt.Errorf("finish login: %w", err)
	}

	// Update sign count to detect cloned authenticators.
	if updateErr := h.store.UpdateCredential(*cred); updateErr != nil {
		log.Warn("auth: failed to update credential sign count", "err", updateErr)
	}

	token, err := h.session.CreateAuthSession()
	if err != nil {
		return "", fmt.Errorf("create auth session: %w", err)
	}

	log.Info("auth: login successful", "credential_id", fmt.Sprintf("%x", cred.ID))
	return token, nil
}
