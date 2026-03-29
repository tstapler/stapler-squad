package auth

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/tstapler/stapler-squad/log"

	"github.com/go-webauthn/webauthn/webauthn"
)

// Handler wraps the go-webauthn/webauthn library and provides dynamic RPID
// selection to support multiple hostnames.
type Handler struct {
	// A map of WebAuthn instances, keyed by RPID.
	webauthn map[string]*webauthn.WebAuthn
	// The list of allowed hostnames (RPIDs).
	rpIDs []string
	// The credential and session stores.
	store   *CredentialStore
	session *SessionManager
}

// NewHandler creates a new WebAuthn handler supporting multiple domains.
func NewHandler(rpIDs []string, origins []string, store *CredentialStore, session *SessionManager) (*Handler, error) {
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

	log.InfoLog.Printf("auth: WebAuthn configured – rpIDs=%v origins=%v", rpIDs, origins)

	return &Handler{
		webauthn: w,
		rpIDs:    rpIDs,
		store:    store,
		session:  session,
	}, nil
}

// webauthnForHost selects the correct WebAuthn instance based on the request Host.
// It iterates through the configured RPIDs and returns the first one that is a
// suffix of the request's hostname. This allows a single server to handle
// requests for e.g. onyx.local and onyx.staplerhome.internal.
func (h *Handler) webauthnForHost(r *http.Request) (*webauthn.WebAuthn, error) {
	hostname := r.Host
	if host, _, err := net.SplitHostPort(r.Host); err == nil {
		hostname = host
	}

	for _, rpID := range h.rpIDs {
		if strings.HasSuffix(hostname, rpID) {
			return h.webauthn[rpID], nil
		}
	}
	return nil, fmt.Errorf("no valid rpID found for host %s", hostname)
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
func (h *Handler) FinishRegistration(ceremonyKey string, r *http.Request) (string, error) {
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

	if err := h.store.AddCredential(*cred); err != nil {
		return "", fmt.Errorf("persist credential: %w", err)
	}

	token, err := h.session.CreateAuthSession()
	if err != nil {
		return "", fmt.Errorf("create auth session: %w", err)
	}

	log.InfoLog.Printf("auth: new passkey registered (credential ID %x)", cred.ID)
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
		log.WarningLog.Printf("auth: failed to update credential sign count: %v", updateErr)
	}

	token, err := h.session.CreateAuthSession()
	if err != nil {
		return "", fmt.Errorf("create auth session: %w", err)
	}

	log.InfoLog.Printf("auth: login successful (credential ID %x)", cred.ID)
	return token, nil
}
