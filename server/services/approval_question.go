package services

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// approval_question.go feeds the pending-question registry from the hook stream
// (ADR-010 decision 2). The hook only registers; which question is on screen is
// decided by session.DialogMatch when a reply claims it.

// Notification metadata keys of a question notification.
const (
	metaQuestionID       = "question_id"
	metaQuestionShape    = "question_shape"
	metaQuestionOptions  = "question_options"
	metaReplyUnavailable = "reply_unavailable"
)

// questionWiring is the optional Reply dependencies of an ApprovalHandler.
type questionWiring struct {
	store *PendingQuestionStore
	// proofs verifies sender proofs; nil falls back to DefaultHookProofs().
	proofs *HookProofs
	// unreplyable counts a question registered not replyable, by cause.
	unreplyable func(cause string)
	// sessionLive reports whether uuid is a known session; nil consults storage.
	sessionLive func(uuid string) bool
}

// SetQuestionRegistry wires the pending-question store and the unreplyable
// counter. Without it questions raise their toast and register nothing.
func (h *ApprovalHandler) SetQuestionRegistry(store *PendingQuestionStore, unreplyable func(cause string)) {
	h.questions.store, h.questions.unreplyable = store, unreplyable
}

// SetHookProofs overrides the proofs used to verify the sender header.
func (h *ApprovalHandler) SetHookProofs(p *HookProofs) { h.questions.proofs = p }

func (h *ApprovalHandler) hookProofs() *HookProofs {
	if h.questions.proofs != nil {
		return h.questions.proofs
	}
	return DefaultHookProofs()
}

func (h *ApprovalHandler) knownSession(sessionUUID string) bool {
	if h.questions.sessionLive != nil {
		return h.questions.sessionLive(sessionUUID)
	}
	if h.storage == nil {
		return true
	}
	instances, err := h.storage.ListInstanceData()
	if err != nil {
		return false
	}
	for _, d := range instances {
		if d.UUID != "" && d.UUID == sessionUUID {
			return true
		}
	}
	return false
}

// questionAttribution is how a question's sender was identified.
type questionAttribution struct {
	// uuid is the session a verified proof vouches for ("" when none verified);
	// when set it is the attribution and the title header is ignored.
	uuid string
	// cause is why the question is not replyable ("" when the proof verified).
	cause string
}

func (h *ApprovalHandler) attributeQuestion(r *http.Request, headerResolvedID string) questionAttribution {
	value := r.Header.Get(hookProofHeader)
	proofs := h.hookProofs()
	switch {
	case value == "" || proofs == nil:
		if headerResolvedID == "" || headerResolvedID == "unknown" {
			return questionAttribution{cause: replyCausePathOnly}
		}
		return questionAttribution{cause: replyCauseNoProof}
	}
	id, ok := proofs.Verify(value)
	if !ok || !h.knownSession(id) {
		return questionAttribution{cause: replyCauseBadProof}
	}
	return questionAttribution{uuid: id}
}

// registerQuestion parses the payload, registers a replyable entry when the
// proof verified and the shape is single, and returns the notification metadata.
func (h *ApprovalHandler) registerQuestion(sessionID string, qa questionAttribution, asked AskedQuestion) map[string]string {
	meta := map[string]string{metaQuestionShape: string(asked.Shape)}
	cause := qa.cause
	if asked.Shape != QuestionShapeSingle {
		cause = replyCauseShape
	}
	if cause == "" && h.questions.store == nil {
		cause = replyCauseNoProof
	}
	if cause != "" {
		meta[metaReplyUnavailable] = cause
		if h.questions.unreplyable != nil {
			h.questions.unreplyable(cause)
		}
		return meta
	}
	labels, err := json.Marshal(asked.Labels)
	if err != nil {
		meta[metaReplyUnavailable] = replyCauseShape
		return meta
	}
	id := NewQuestionID()
	h.questions.store.Register(PendingQuestion{
		QuestionID:  id,
		SessionUUID: sessionID,
		Dialog:      session.QuestionDialog{Question: asked.Question, Header: asked.Header, Labels: asked.Labels},
		Token:       QuestionToken(asked.Question),
	})
	meta[metaQuestionID] = id
	meta[metaQuestionOptions] = string(labels)
	return meta
}

// newQuestionNotificationID is the notification id of a question toast.
func newQuestionNotificationID() string { return uuid.New().String() }

func logQuestionRegistered(sessionID, shape, cause string) {
	log.ForSession(sessionID).Info("[ApprovalHandler] AskUserQuestion registered", "shape", shape, "unreplyable_cause", cause)
}
