package services

// guidance_debug_seed_handler.go: test-only endpoint that raises a durable
// question for a live session without a real AskUserQuestion hook, so the e2e
// suite can put a hook-mirrored or agent-created question card in front of the
// UI. Registered by server.go ONLY when STAPLER_SQUAD_INSTANCE=e2e-local, like
// the backlog debug seed handlers; never reachable in a normal deploy.

import (
	"encoding/json"
	"net/http"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// GuidanceDebugSeedHandler seeds question rows for the e2e suite.
type GuidanceDebugSeedHandler struct {
	svc     *SessionService
	storage *session.Storage
}

// NewGuidanceDebugSeedHandler constructs the handler over the live service and its storage.
func NewGuidanceDebugSeedHandler(svc *SessionService, storage *session.Storage) *GuidanceDebugSeedHandler {
	return &GuidanceDebugSeedHandler{svc: svc, storage: storage}
}

// RegisterRoutes mounts the endpoint on mux.
func (h *GuidanceDebugSeedHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/debug/guidance/seed-question", h.seedQuestion)
}

type seedQuestion struct {
	Text        string   `json:"text"`
	Header      string   `json:"header"`
	MultiSelect bool     `json:"multiSelect"`
	Options     []string `json:"options"`
}

type seedQuestionRequest struct {
	// SessionID is a session title or UUID.
	SessionID string `json:"sessionId"`
	// Source is "ask_user_question_hook" (default, registered as a replyable dialog) or "agent_mcp".
	Source     string         `json:"source"`
	HookCallID string         `json:"hookCallId"`
	Questions  []seedQuestion `json:"questions"`
}

func (r seedQuestionRequest) payload() (domain.QuestionPayload, error) {
	qs := make([]domain.Question, len(r.Questions))
	for i, q := range r.Questions {
		qs[i] = domain.Question{Text: q.Text, Header: q.Header, MultiSelect: q.MultiSelect}
		for _, label := range q.Options {
			qs[i].Options = append(qs[i].Options, domain.Option{Label: label})
		}
	}
	return domain.NewQuestionPayload(qs)
}

func (h *GuidanceDebugSeedHandler) seedQuestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req seedQuestionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	inst := h.svc.findInstance(req.SessionID)
	payload, perr := req.payload()
	mirror := h.svc.guidanceMirror.Load()
	switch {
	case inst == nil:
		http.Error(w, "session not found", http.StatusNotFound)
		return
	case perr != nil:
		http.Error(w, "invalid questions: "+perr.Error(), http.StatusBadRequest)
		return
	case mirror == nil:
		http.Error(w, "guidance mirror not wired", http.StatusServiceUnavailable)
		return
	}
	id, err := h.create(r, req, inst.GetStableID(), payload, mirror)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "sessionUuid": inst.GetStableID()})
}

func (h *GuidanceDebugSeedHandler) create(r *http.Request, req seedQuestionRequest, sessionUUID string, payload domain.QuestionPayload, mirror *GuidanceHookMirror) (string, error) {
	ctx := r.Context()
	source := domain.QuestionSourceHook
	if req.Source == string(domain.QuestionSourceAgentMCP) {
		source = domain.QuestionSourceAgentMCP
	}
	params := domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: sessionUUID, Source: source, Payload: payload,
		QuestionText: req.Questions[0].Text, QuestionType: domain.QuestionTypeMultipleChoice, Options: encodeGuidanceOptions(req.Questions[0].Options), HookCallID: domain.HookCallID(req.HookCallID),
	}
	if source == domain.QuestionSourceHook {
		params.AskerSessionUUID = sessionUUID
		params.DeliveryRoute = domain.DeliveryRouteLiveKeystroke
	}
	gr, err := domain.NewGuidanceRequest(params)
	if err != nil {
		return "", err
	}
	row, err := h.storage.CreateMirroredGuidanceRequest(ctx, gr, 0)
	if err != nil {
		return "", err
	}
	if source == domain.QuestionSourceHook {
		mirror.EnsureRegistered(ctx, QuestionKey{SessionUUID: sessionUUID, QuestionID: row.ID})
	}
	return row.ID, nil
}
