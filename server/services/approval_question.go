package services

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// approval_question.go feeds the pending-question registry from the hook stream
// (ADR-010 decision 2). The hook only registers; which question is on screen is
// decided by session.DialogMatch when a reply claims it.

// Notification metadata keys of a question notification.
const (
	metaQuestionID = "question_id"
	// metaGuidanceRequestID is the durable GuidanceRequest row mirrored for the question.
	metaGuidanceRequestID = "guidance_request_id"
	metaQuestionShape     = "question_shape"
	metaQuestionOptions   = "question_options"
	metaReplyUnavailable  = "reply_unavailable"
	// metaQuestionPending is how many questions of the session were asked together
	// with this one (PendingQuestionStore.PendingCount): above 1 the card cannot
	// know which one is on screen. Unlike the notification's occurrence count it
	// does not accumulate over answered questions. The web client reads it as the
	// literal "question_pending_count" in web-app/src/lib/reply/replyQuestion.ts.
	metaQuestionPending = "question_pending_count"
)

// questionWiring is the optional Reply dependencies of an ApprovalHandler.
type questionWiring struct {
	store *PendingQuestionStore
	// proofs verifies sender proofs; nil falls back to DefaultHookProofs().
	proofs *HookProofs
	// unreplyable counts a question registered not replyable, by cause.
	unreplyable func(cause string)
	// mirror writes the durable GuidanceRequest row; nil disables mirroring.
	mirror *GuidanceHookMirror
	// sessionLive reports whether uuid is a known session; nil consults storage.
	sessionLive func(uuid string) bool
}

// SetQuestionRegistry wires the pending-question store and the unreplyable
// counter. Without it questions raise their toast and register nothing.
func (h *ApprovalHandler) SetQuestionRegistry(store *PendingQuestionStore, unreplyable func(cause string)) {
	h.questions.store, h.questions.unreplyable = store, unreplyable
}

// SetGuidanceMirror wires the durable hook-question mirror.
func (h *ApprovalHandler) SetGuidanceMirror(m *GuidanceHookMirror) { h.questions.mirror = m }

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
func (h *ApprovalHandler) registerQuestion(sessionID string, qa questionAttribution, asked AskedQuestion, hookCallID string) map[string]string {
	meta := map[string]string{metaQuestionShape: string(asked.Shape)}
	cause := qa.cause
	if asked.Shape != QuestionShapeSingle {
		cause = replyCauseShape
	}
	if cause == "" && h.questions.store == nil {
		cause = replyCauseNoProof
	}
	mirrorID := h.mirrorQuestion(sessionID, hookCallID, asked, cause == "")
	if mirrorID != "" {
		meta[metaGuidanceRequestID] = mirrorID
	}
	if cause != "" {
		meta[metaReplyUnavailable] = cause
		if h.questions.unreplyable != nil {
			h.questions.unreplyable(cause)
		}
		if h.questions.store != nil {
			// On screen but not registrable: it still counts toward "parallel".
			h.questions.store.NoteUnreplyable(qa.uuid)
		}
		return meta
	}
	labels, err := json.Marshal(asked.Labels)
	if err != nil {
		meta[metaReplyUnavailable] = replyCauseShape
		return meta
	}
	id := NewQuestionID()
	if mirrorID != "" {
		id = mirrorID // the durable row id survives a restart, so a rendered card keeps addressing it
	}
	pending := h.questions.store.Register(PendingQuestion{
		QuestionID:  id,
		SessionUUID: sessionID,
		Dialog:      session.QuestionDialog{Question: asked.Question, Header: asked.Header, Labels: asked.Labels},
		Token:       QuestionToken(asked.Question),
	})
	meta[metaQuestionID] = id
	meta[metaQuestionPending] = strconv.Itoa(pending)
	meta[metaQuestionOptions] = string(labels)
	return meta
}

// mirrorQuestion writes the durable row and returns its id, or "" when mirroring
// is off, the session has no UUID or the write failed (fail open).
func (h *ApprovalHandler) mirrorQuestion(sessionID, hookCallID string, asked AskedQuestion, replyable bool) string {
	if h.questions.mirror == nil {
		return ""
	}
	sessionUUID := h.sessionUUIDFor(sessionID)
	if sessionUUID == "" {
		return ""
	}
	id, _ := h.questions.mirror.Mirror(HookMirrorRequest{
		SessionUUID: sessionUUID, HookCallID: hookCallID, Asked: asked, Replyable: replyable,
	})
	return id
}

// sessionUUIDFor resolves a hook's session identifier (a UUID or, for an
// unverified sender, a title) to the instance UUID; rows are keyed by UUID only.
func (h *ApprovalHandler) sessionUUIDFor(id string) string {
	if id == "" || id == "unknown" {
		return ""
	}
	if h.knownSession(id) {
		return id
	}
	if h.storage == nil {
		return ""
	}
	instances, err := h.storage.ListInstanceData()
	if err != nil {
		return ""
	}
	for _, d := range instances {
		if d.UUID != "" && d.Title == id {
			return d.UUID
		}
	}
	return ""
}

// newQuestionNotificationID is the notification id of a question toast.
func newQuestionNotificationID() string { return uuid.New().String() }

func logQuestionRegistered(sessionID, shape, cause string) {
	log.ForSession(sessionID).Info("[ApprovalHandler] AskUserQuestion registered", "shape", shape, "unreplyable_cause", cause)
}
