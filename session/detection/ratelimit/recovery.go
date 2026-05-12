package ratelimit

import (
	"github.com/tstapler/stapler-squad/log"
)

type RecoveryHandler struct {
	sessionID string

	sendInput func([]byte) error
}

func NewRecoveryHandler(sessionID string, sendInput func([]byte) error) *RecoveryHandler {
	return &RecoveryHandler{
		sessionID: sessionID,
		sendInput: sendInput,
	}
}

func (h *RecoveryHandler) Execute(input []byte) error {
	log.Info("sending recovery input", "session", h.sessionID, "input", string(input))

	if err := h.sendInput(input); err != nil {
		log.Warn("failed to send recovery input", "session", h.sessionID, "err", err)
		return err
	}

	log.Info("successfully sent recovery input", "session", h.sessionID)
	return nil
}
