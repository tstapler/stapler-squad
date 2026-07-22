package services

// backlog_debug_seed_handler.go — test-only debug endpoint that seeds an open
// BacklogStuckState row directly, closing the e2e seeding gap flagged by
// Epic 4.1 (see tests/e2e/pages/StuckItemsPage.ts's seedStuckItem KNOWN GAP
// comment). Mirrors backlog_stuck_rpc_test.go's Go-test-only seedOpenStuckRow
// helper, exposed over HTTP so Playwright (an external process) can reach it.
//
// This bypasses MarkStuck/the reconciler's status precondition entirely — it
// exists only so the e2e suite can put a row in front of the UI without
// waiting out real detector thresholds (e.g. 30 minutes for
// pr_ready_unmerged). It is registered by server.go ONLY when
// STAPLER_SQUAD_INSTANCE=e2e-local, mirroring how the e2e test server itself
// is gated (see .claude/rules/e2e-test-conventions.md / CLAUDE.md's E2E Tests
// section) — never reachable in a normal deploy.

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// BacklogDebugSeedHandler seeds BacklogStuckState rows for the e2e suite.
type BacklogDebugSeedHandler struct {
	storage *session.Storage
}

// NewBacklogDebugSeedHandler constructs the handler. storage may be nil in
// which case every request 503s.
func NewBacklogDebugSeedHandler(storage *session.Storage) *BacklogDebugSeedHandler {
	return &BacklogDebugSeedHandler{storage: storage}
}

// RegisterRoutes registers the debug seed endpoints on the given mux. Callers
// MUST only invoke this when running as the e2e-local instance.
func (h *BacklogDebugSeedHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/debug/backlog/seed-stuck", h.handleSeed)
	mux.HandleFunc("/api/debug/backlog/seed-queued", h.handleSeedQueued)
}

type seedQueuedItemRequest struct {
	Title string `json:"title"`
}

type seedQueuedItemResponse struct {
	ItemID string `json:"itemId"`
}

// handleSeedQueued creates a backlog item directly in "queued" status, bypassing
// the real WIP-cap gate so the e2e suite can assert on the queued badge/section
// without first spawning enough real sessions to fill the concurrency cap.
func (h *BacklogDebugSeedHandler) handleSeedQueued(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req seedQueuedItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	now := time.Now()
	item, err := h.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    req.Title,
		Status:   string(session.BacklogStatusQueued),
		QueuedAt: &now,
	})
	if err != nil {
		log.Error("backlog debug seed: create queued item failed", "err", err)
		http.Error(w, "failed to create backlog item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(seedQueuedItemResponse{ItemID: item.ID}); err != nil {
		log.Error("backlog debug seed: encode response failed", "err", err)
	}
}

type seedStuckStateRequest struct {
	// ItemID is accepted for readability in test call sites but is NOT used as
	// the created BacklogItem's real ID (ent always generates the UUID) — the
	// e2e spec locates seeded cards by title, not this value.
	ItemID          string `json:"itemId"`
	Title           string `json:"title"`
	Reason          string `json:"reason"`
	FirstDetectedAt string `json:"firstDetectedAt"`
	PrNumber        int    `json:"prNumber"`
	PrUrl           string `json:"prUrl"`
	Context         string `json:"context"`
}

type seedStuckStateResponse struct {
	ItemID string `json:"itemId"`
}

// statusForSeedReason picks the BacklogStatus each real detector anchors its
// reason on, so a seeded item looks like what the reconciler would have
// produced (matters for anything that reads item.Status, e.g. self-heal).
func statusForSeedReason(reason domain.StuckReason) session.BacklogStatus {
	switch reason {
	case domain.StuckReasonPRReadyUnmerged:
		return session.BacklogStatusPRPending
	case domain.StuckReasonStaleWork, domain.StuckReasonBouncing, domain.StuckReasonAutonomousStuck:
		return session.BacklogStatusInProgress
	case domain.StuckReasonOrphanedTriage:
		return session.BacklogStatusIdea
	default: // abandoned_review, rework_cap, push_failed
		return session.BacklogStatusReview
	}
}

func (h *BacklogDebugSeedHandler) handleSeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req seedStuckStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	reason := domain.StuckReason(req.Reason)
	if !reason.IsValid() {
		http.Error(w, "invalid reason: "+req.Reason, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	status := statusForSeedReason(reason)
	item, err := h.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  req.Title,
		Status: string(status),
	})
	if err != nil {
		log.Error("backlog debug seed: create item failed", "err", err)
		http.Error(w, "failed to create backlog item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if req.PrNumber > 0 || req.PrUrl != "" {
		prNumber := req.PrNumber
		prURL := req.PrUrl
		if _, err := h.storage.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{
			PrNumber: &prNumber,
			PrURL:    &prURL,
		}, nil); err != nil {
			log.Error("backlog debug seed: set PR fields failed", "err", err)
			http.Error(w, "failed to set PR fields: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	firstDetectedAt := time.Now()
	if req.FirstDetectedAt != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, req.FirstDetectedAt); parseErr == nil {
			firstDetectedAt = parsed
		}
	}

	client := h.storage.GetEntClient()
	if client == nil {
		http.Error(w, "ent client not available (non-ent storage backend)", http.StatusServiceUnavailable)
		return
	}
	itemUUID, err := uuid.Parse(item.ID)
	if err != nil {
		http.Error(w, "created item has an unparseable id: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := client.BacklogStuckState.Create().
		SetItemID(itemUUID).
		SetReason(string(reason)).
		SetFirstDetectedAt(firstDetectedAt).
		SetLastCheckedAt(time.Now()).
		SetContext(req.Context).
		Exec(ctx); err != nil {
		log.Error("backlog debug seed: insert stuck row failed", "err", err)
		http.Error(w, "failed to seed stuck row: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(seedStuckStateResponse{ItemID: item.ID}); err != nil {
		log.Error("backlog debug seed: encode response failed", "err", err)
	}
}
