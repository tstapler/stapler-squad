package auth

import (
	"fmt"
	"net/http"

	"github.com/tstapler/stapler-squad/log"

	"github.com/go-webauthn/webauthn/webauthn"
)

// Handler wraps the go-webauthn/webauthn library and ties it to stapler-squad's
// CredentialStore and SessionManager.
type Handler struct {
	wa      *webauthn.WebAuthn
	store   *CredentialStore
	session *SessionManager
}

// NewHandler creates a new WebAuthn handler.
//
// rpID must be the effective domain of the origin clients will use
// (e.g. "192.168.1.42" or "myhost.local"). It must NOT include scheme or port.
//
// origins is the list of origins that are accepted (e.g. "https://192.168.1.42:8543").
func NewHandler(rpID string, origins []string, store *CredentialStore, session *SessionManager) (*Handler, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Claude Squad",
		RPID:          rpID,
		RPOrigins:     origins,
		Debug:         false,
	})
	if err != nil {
		return nil, fmt.Errorf("configure webauthn: %w", err)
	}

	log.InfoLog.Printf("auth: WebAuthn configured – rpID=%s origins=%v", rpID, origins)

	return &Handler{
		wa:      wa,
		store:   store,
		session: session,
	}, nil
}

// BeginRegistration starts a passkey registration ceremony.
// Returns the JSON-serialisable options and a ceremony session key the client
// must send back in FinishRegistration.
func (h *Handler) BeginRegistration() (*webauthn.SessionData, interface{}, string, error) {
	user := newLocalUser(h.store)

	creation, sessionData, err := h.wa.BeginRegistration(user)
	if err != nil {
		return nil, nil, "", fmt.Errorf("begin registration: %w", err)
	}

	key, err := h.session.StoreCeremony(ceremonyRegister, *sessionData)
	if err != nil {
		return nil, nil, "", fmt.Errorf("store ceremony: %w", err)
	}

	return sessionData, creation, key, nil
}

// FinishRegistration completes the registration ceremony using the HTTP request
// body. On success it persists the new credential and returns an auth token.
func (h *Handler) FinishRegistration(ceremonyKey string, r *http.Request) (string, error) {
	sessionData, ok := h.session.GetCeremony(ceremonyKey)
	if !ok {
		return "", fmt.Errorf("ceremony session not found or expired")
	}

	user := newLocalUser(h.store)
	cred, err := h.wa.FinishRegistration(user, sessionData, r)
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
// Returns the options and ceremony key.
func (h *Handler) BeginLogin() (interface{}, string, error) {
	user := newLocalUser(h.store)

	assertion, sessionData, err := h.wa.BeginLogin(user)
	if err != nil {
		return nil, "", fmt.Errorf("begin login: %w", err)
	}

	key, err := h.session.StoreCeremony(ceremonyLogin, *sessionData)
	if err != nil {
		return nil, "", fmt.Errorf("store ceremony: %w", err)
	}

	return assertion, key, nil
}

// FinishLogin completes the login ceremony. On success it returns a new auth token.
func (h *Handler) FinishLogin(ceremonyKey string, r *http.Request) (string, error) {
	sessionData, ok := h.session.GetCeremony(ceremonyKey)
	if !ok {
		return "", fmt.Errorf("ceremony session not found or expired")
	}

	user := newLocalUser(h.store)
	cred, err := h.wa.FinishLogin(user, sessionData, r)
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
