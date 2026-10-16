package services

// backlog_debug_mutate_handler.go — test-only debug endpoints that mutate a
// backlog item directly through the storage layer, bypassing
// TransitionBacklogItemStatus's engine/gate checks and the CreateSession/
// SpawnSessionFromItem RPC flow entirely.
//
// Added for project_plans/backlog-event-driven-updates's Playwright e2e
// layer (validation.md's UX Acceptance Tests row for criterion #1: "trigger
// a server-side transition via a test-only mutation endpoint"). The Happy
// Path Scenario at the top of validation.md is explicit that the reconciler
// itself calls storage.TransitionBacklogItemStatus/UpdateBacklogItem/
// ArchiveBacklogItem/DeleteBacklogItem directly — "no RPC handler involved"
// — which is exactly what these endpoints do, so a Playwright test can
// simulate a second, independent actor (reconciler, another tab, another
// operator) mutating an item without needing a real second browser context
// or without fighting TransitionBacklogItemStatus's CanTransition/
// ValidateGates business rules (plan approval, AC criteria, review verdict,
// etc.) that a legitimate multi-step UI flow would otherwise require.
//
// Mirrors backlog_debug_seed_handler.go's shape and gating exactly: these
// routes are registered by server.go ONLY when
// STAPLER_SQUAD_INSTANCE=e2e-local — never reachable in a normal deploy.

import (
	"encoding/json"
	"net/http"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// BacklogDebugMutateHandler mutates BacklogItem rows directly for the e2e suite.
type BacklogDebugMutateHandler struct {
	storage *session.Storage
}

// NewBacklogDebugMutateHandler constructs the handler. storage may be nil in
// which case every request 503s.
func NewBacklogDebugMutateHandler(storage *session.Storage) *BacklogDebugMutateHandler {
	return &BacklogDebugMutateHandler{storage: storage}
}

// RegisterRoutes registers the debug mutate endpoints on the given mux.
// Callers MUST only invoke this when running as the e2e-local instance.
func (h *BacklogDebugMutateHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/debug/backlog/mutate-create", h.handleCreate)
	mux.HandleFunc("/api/debug/backlog/mutate-transition", h.handleTransition)
	mux.HandleFunc("/api/debug/backlog/mutate-update", h.handleUpdate)
	mux.HandleFunc("/api/debug/backlog/mutate-archive", h.handleArchive)
	mux.HandleFunc("/api/debug/backlog/mutate-delete", h.handleDelete)
}

type mutateCreateRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	RepoPath    string   `json:"repoPath"`
	AcCriteria  []string `json:"acCriteria"`
}

type mutateCreateResponse struct {
	ItemID   string `json:"itemId"`
	PublicID string `json:"publicId,omitempty"`
}

// handleCreate creates a backlog item directly at an arbitrary status,
// bypassing every normal creation/transition gate — lets an e2e test seed an
// item already sitting in "in_progress"/"review"/etc. without walking it
// through the real idea -> ready -> in_progress lifecycle first.
func (h *BacklogDebugMutateHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req mutateCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	status := req.Status
	if status == "" {
		status = string(session.BacklogStatusIdea)
	}
	priority := req.Priority
	if priority == 0 {
		priority = 3
	}

	var acJSON session.AcCriteriaJSON
	if len(req.AcCriteria) > 0 {
		criteria := make([]session.AcCriterion, len(req.AcCriteria))
		for i, text := range req.AcCriteria {
			criteria[i] = session.AcCriterion{Index: i, Text: text, Status: "pending"}
		}
		raw, err := json.Marshal(criteria)
		if err != nil {
			http.Error(w, "failed to encode acCriteria: "+err.Error(), http.StatusInternalServerError)
			return
		}
		acJSON = session.AcCriteriaJSON(raw)
	}

	ctx := r.Context()
	item, err := h.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:              req.Title,
		Description:        req.Description,
		Status:             status,
		Priority:           priority,
		RepoPath:           req.RepoPath,
		AcceptanceCriteria: acJSON,
	})
	if err != nil {
		log.Error("backlog debug mutate: create item failed", "err", err)
		http.Error(w, "failed to create backlog item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := mutateCreateResponse{ItemID: item.ID}
	if publicID, ok := item.PublicID(); ok {
		resp.PublicID = publicID.String()
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Error("backlog debug mutate: encode response failed", "err", err)
	}
}

type mutateTransitionRequest struct {
	ItemID       string `json:"itemId"`
	TargetStatus string `json:"targetStatus"`
}

// handleTransition calls storage.TransitionBacklogItemStatus directly — no
// CanTransition/ValidateGates check, no precondition — exactly mirroring how
// a reconciler (ReconcilePRPending, etc.) drives a status change today, per
// validation.md's Happy Path Scenario. This is what a Playwright test uses
// to simulate "a status change made via one flow" arriving live while the
// UI has the item open in another (the browser tab under test).
func (h *BacklogDebugMutateHandler) handleTransition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req mutateTransitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ItemID == "" || req.TargetStatus == "" {
		http.Error(w, "itemId and targetStatus are required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if _, err := h.storage.TransitionBacklogItemStatus(ctx, req.ItemID, session.BacklogStatus(req.TargetStatus), nil, session.TriggeredBySystem); err != nil {
		log.Error("backlog debug mutate: transition failed", "err", err)
		http.Error(w, "failed to transition backlog item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type mutateUpdateRequest struct {
	ItemID      string  `json:"itemId"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

// handleUpdate calls storage.UpdateBacklogItem directly with only the
// fields the caller supplied — used to simulate a background field edit
// (e.g. description change) arriving live while the item is open for
// editing elsewhere (Story 5.3.2 buffered-update banner).
func (h *BacklogDebugMutateHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req mutateUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ItemID == "" {
		http.Error(w, "itemId is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if _, err := h.storage.UpdateBacklogItem(ctx, req.ItemID, session.BacklogItemUpdate{
		Title:       req.Title,
		Description: req.Description,
	}, nil); err != nil {
		log.Error("backlog debug mutate: update failed", "err", err)
		http.Error(w, "failed to update backlog item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type mutateItemIDRequest struct {
	ItemID string `json:"itemId"`
}

// handleArchive calls storage.ArchiveBacklogItem directly — used to
// simulate the item being archived from "another tab" while a detail
// panel/BacklogItemPanel for it is open in the browser under test (UX AC
// #13/#16).
func (h *BacklogDebugMutateHandler) handleArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req mutateItemIDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ItemID == "" {
		http.Error(w, "itemId is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if _, err := h.storage.ArchiveBacklogItem(ctx, req.ItemID, nil, session.TriggeredByUser, ""); err != nil {
		log.Error("backlog debug mutate: archive failed", "err", err)
		http.Error(w, "failed to archive backlog item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleDelete calls storage.DeleteBacklogItem directly — used to simulate
// the item being permanently removed from "another tab" while a detail
// panel for it is open (UX AC #13).
func (h *BacklogDebugMutateHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req mutateItemIDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ItemID == "" {
		http.Error(w, "itemId is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if err := h.storage.DeleteBacklogItem(ctx, req.ItemID); err != nil {
		log.Error("backlog debug mutate: delete failed", "err", err)
		http.Error(w, "failed to delete backlog item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
