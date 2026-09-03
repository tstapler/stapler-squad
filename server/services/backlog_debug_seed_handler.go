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
// is gated (see the `e2e-test-conventions` skill / CLAUDE.md's E2E Tests
// section) — never reachable in a normal deploy.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	sessiongit "github.com/tstapler/stapler-squad/session/git"
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
	mux.HandleFunc("/api/debug/backlog/seed-headless-triage-session", h.handleSeedHeadlessTriageSession)
	mux.HandleFunc("/api/debug/backlog/seed-work-item-session", h.handleSeedWorkItemSession)
	mux.HandleFunc("/api/debug/backlog/seed-work-session-with-worktree", h.handleSeedWorkSessionWithWorktree)
	mux.HandleFunc("/api/debug/backlog/seed-jules-work-session", h.handleSeedJulesWorkSession)
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
	// HasPlan, when true, writes a real stub plan file to disk and sets it as
	// the created item's plan_artifacts_path — so ApprovePlan's os.Stat check
	// (server/services/backlog_service_lifecycle.go) succeeds, letting e2e
	// tests exercise the real approve-plan flow instead of only the
	// no-plan-yet gate.
	HasPlan bool `json:"hasPlan"`
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
	case domain.StuckReasonPlanNotApproved:
		return session.BacklogStatusQueued
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

	if req.HasPlan {
		// Rooted under this instance's own isolated config/state dir
		// (STAPLER_SQUAD_TEST_DIR, set by the e2e harness's global-setup.ts)
		// rather than the shared os.TempDir() — avoids a predictable,
		// world-writable temp path (CWE-377/CWE-59 symlink-race risk) and
		// gets cleaned up for free by the e2e harness's global-teardown.ts,
		// which deletes the whole test dir when the run ends.
		configDir, err := config.GetConfigDir()
		if err != nil {
			log.Error("backlog debug seed: resolve config dir failed", "err", err)
			http.Error(w, "failed to resolve config dir: "+err.Error(), http.StatusInternalServerError)
			return
		}
		planDir := filepath.Join(configDir, "e2e-plan-artifacts", item.ID)
		if err := os.MkdirAll(planDir, 0o750); err != nil {
			log.Error("backlog debug seed: create plan dir failed", "err", err)
			http.Error(w, "failed to create plan artifacts dir: "+err.Error(), http.StatusInternalServerError)
			return
		}
		planPath := filepath.Join(planDir, "plan.md")
		if err := os.WriteFile(planPath, []byte("# Seeded e2e plan\n"), 0o600); err != nil {
			log.Error("backlog debug seed: write plan file failed", "err", err)
			http.Error(w, "failed to write plan artifacts file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := h.storage.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{
			PlanArtifactsPath: &planPath,
		}, nil); err != nil {
			log.Error("backlog debug seed: set plan_artifacts_path failed", "err", err)
			http.Error(w, "failed to set plan_artifacts_path: "+err.Error(), http.StatusInternalServerError)
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

type seedHeadlessTriageSessionRequest struct {
	Title   string `json:"title"`
	Status  string `json:"status"`  // defaults to "review" if empty
	Summary string `json:"summary"` // defaults to a canned summary if empty
	// Ended, when true, also records an EndedAt on the seeded ItemSession so
	// mapBacklogItem's triageStatus derivation (web-app's useBacklogService.ts)
	// resolves to "completed" rather than "running" — needed to exercise the
	// item's own live (non-readOnly) TriageReviewPanel, which requires
	// status:"idea" + triageStatus:"completed" (BacklogItemDetail.tsx). The
	// default (false) preserves the original behavior for callers exercising
	// SessionDiagnosticPanel's readOnly branch, which doesn't consult EndedAt.
	Ended bool `json:"ended"`
}

type seedHeadlessTriageSessionResponse struct {
	ItemID    string `json:"itemId"`
	SessionID string `json:"sessionId"`
}

// handleSeedHeadlessTriageSession creates a backlog item with one linked
// ItemSession shaped exactly like a real headless-triage-* row (role
// "triage", SessionUUID prefixed headless-triage-, TriageResult populated) so
// the e2e suite (Story 6.1.2, backlog-item-detail-redesign.spec.ts) can
// exercise SessionDiagnosticPanel's TriageReviewPanel-readOnly branch without
// waiting on a real headless triage LLM call. Mirrors
// server/services/backlog_service_triage.go's real headless-triage session
// creation path (headlessTriageUUIDPrefix + uuid.New().String(), role
// session.SessionRoleTriage, TriageResult JSON) closely enough that
// classifySessionKind() (web-app/src/lib/backlog/sessionKind.ts) classifies
// the seeded row identically to a production one.
func (h *BacklogDebugSeedHandler) handleSeedHeadlessTriageSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req seedHeadlessTriageSessionRequest
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
		status = string(session.BacklogStatusReview)
	}
	summary := req.Summary
	if summary == "" {
		summary = "Triage complete: found 2 suggestions and 1 implementation task."
	}

	ctx := r.Context()
	item, err := h.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  req.Title,
		Status: status,
	})
	if err != nil {
		log.Error("backlog debug seed: create item failed", "err", err)
		http.Error(w, "failed to create backlog item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	triageResult := session.HeadlessTriageResult{
		Title:   req.Title,
		Summary: summary,
		Suggestions: []session.TriageSuggestion{
			{Text: "Add a regression test", Rationale: "Prevents this bug from recurring silently."},
			{Text: "Extract shared helper", Rationale: "Removes duplicated logic across call sites."},
		},
		Tasks: []session.TriageTask{
			{Text: "Implement the fix", Estimate: "30m", Category: "implementation"},
		},
	}
	triageResultJSON, err := json.Marshal(triageResult)
	if err != nil {
		log.Error("backlog debug seed: marshal triage result failed", "err", err)
		http.Error(w, "failed to marshal triage result: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sessionUUID := headlessTriageUUIDPrefix + uuid.New().String()
	itemSession, err := h.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:       item.ID,
		SessionUUID:  sessionUUID,
		SessionRole:  session.SessionRoleTriage,
		TriageResult: string(triageResultJSON),
	})
	if err != nil {
		log.Error("backlog debug seed: create headless triage item session failed", "err", err)
		http.Error(w, "failed to create item session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if req.Ended {
		if err := h.storage.UpdateItemSessionEnded(ctx, itemSession.ID, time.Now()); err != nil {
			log.Error("backlog debug seed: mark headless triage item session ended failed", "err", err)
			http.Error(w, "failed to mark item session ended: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(seedHeadlessTriageSessionResponse{
		ItemID:    item.ID,
		SessionID: itemSession.SessionUUID,
	}); err != nil {
		log.Error("backlog debug seed: encode response failed", "err", err)
	}
}

type seedWorkItemSessionRequest struct {
	Title  string `json:"title"`
	Status string `json:"status"` // defaults to "review" if empty
}

type seedWorkItemSessionResponse struct {
	ItemID    string `json:"itemId"`
	SessionID string `json:"sessionId"`
}

// handleSeedWorkItemSession creates a backlog item with a linked "work"
// ItemSession but no backing Session/Worktree row — enough for
// ReviewChangesModal's "View Changes" trigger (gated on a truthy work
// session only), without a real worktree/tmux spin-up.
func (h *BacklogDebugSeedHandler) handleSeedWorkItemSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req seedWorkItemSessionRequest
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
		status = string(session.BacklogStatusReview)
	}

	ctx := r.Context()
	item, err := h.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  req.Title,
		Status: status,
	})
	if err != nil {
		log.Error("backlog debug seed: create item failed", "err", err)
		http.Error(w, "failed to create backlog item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sessionUUID := "e2e-work-" + uuid.New().String()
	itemSession, err := h.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	})
	if err != nil {
		log.Error("backlog debug seed: create work item session failed", "err", err)
		http.Error(w, "failed to create item session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(seedWorkItemSessionResponse{
		ItemID:    item.ID,
		SessionID: itemSession.SessionUUID,
	}); err != nil {
		log.Error("backlog debug seed: encode response failed", "err", err)
	}
}

type seedWorkSessionWithWorktreeRequest struct {
	Title  string `json:"title"`
	Status string `json:"status"` // defaults to "review" if empty
	// RepoPath, when set, becomes the created item's RepoPath — so a test
	// can seed an item whose repo matches a ConfirmEgressConsent/
	// RevokeEgressConsent call's repo_path (jules-dispatch.spec.ts's §7.2/
	// §7.13 scenarios need both a tracked branch, only this endpoint
	// produces, and a known repo path to (un)acknowledge). Omitted defaults
	// to "", unchanged from before this field existed.
	RepoPath string `json:"repoPath"`
}

type seedWorkSessionWithWorktreeResponse struct {
	ItemID       string `json:"itemId"`
	SessionID    string `json:"sessionId"`
	WorktreePath string `json:"worktreePath"`
}

// handleSeedWorkSessionWithWorktree creates a backlog item with a "work"
// ItemSession backed by a real Session+Worktree row over a git-init'd temp
// dir — needed because BacklogFileBrowserModal's trigger is gated on a
// truthy worktreePath, which only a real ent.Worktree join produces.
func (h *BacklogDebugSeedHandler) handleSeedWorkSessionWithWorktree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req seedWorkSessionWithWorktreeRequest
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
		status = string(session.BacklogStatusReview)
	}

	// Rooted under this instance's own isolated config/state dir
	// (STAPLER_SQUAD_TEST_DIR, set by the e2e harness's global-setup.ts)
	// rather than the shared os.TempDir() — same rationale as the HasPlan
	// branch above: avoids a predictable, world-writable temp path
	// (CWE-377/CWE-59 symlink-race risk) and gets cleaned up for free by
	// the e2e harness's global-teardown.ts, which deletes the whole test
	// dir when the run ends (a bare os.MkdirTemp here would leak one
	// directory per test run on any long-lived dev/CI host, since nothing
	// else cleans up outside STAPLER_SQUAD_TEST_DIR).
	configDir, err := config.GetConfigDir()
	if err != nil {
		log.Error("backlog debug seed: resolve config dir failed", "err", err)
		http.Error(w, "failed to resolve config dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	worktreePath := filepath.Join(configDir, "e2e-worktree-artifacts", uuid.New().String())
	if err := os.MkdirAll(worktreePath, 0o750); err != nil {
		log.Error("backlog debug seed: create worktree dir failed", "err", err)
		http.Error(w, "failed to create worktree dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(filepath.Join(worktreePath, "README.md"), []byte("# e2e fixture\n"), 0o600); err != nil {
		log.Error("backlog debug seed: write worktree fixture file failed", "err", err)
		http.Error(w, "failed to write worktree fixture file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// A second file so the file tree has more than one row — needed for
	// tests that navigate between rows (modal-focus-trap AC5's tabindex-churn
	// e2e test requires react-arborist to move focus off one row and onto
	// another to exercise its roving-tabindex behavior).
	if err := os.WriteFile(filepath.Join(worktreePath, "NOTES.md"), []byte("# notes\n"), 0o600); err != nil {
		log.Error("backlog debug seed: write second worktree fixture file failed", "err", err)
		http.Error(w, "failed to write second worktree fixture file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// A real (tiny) git repo, not just a bare directory: GetVCSStatus (the
	// backend behind the VcsWidget the "Browse Files" trigger lives in) 404s
	// the widget entirely for a non-version-controlled directory.
	if err := sessiongit.InitializeProjectDirectory(worktreePath); err != nil {
		log.Error("backlog debug seed: git-init worktree dir failed", "err", err)
		http.Error(w, "failed to git-init worktree dir: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	item, err := h.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    req.Title,
		Status:   status,
		RepoPath: req.RepoPath,
	})
	if err != nil {
		log.Error("backlog debug seed: create item failed", "err", err)
		http.Error(w, "failed to create backlog item: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sessionUUID := "e2e-work-wt-" + uuid.New().String()
	now := time.Now()
	if err := h.storage.CreateInstanceData(ctx, session.InstanceData{
		Title:       sessionUUID,
		UUID:        sessionUUID,
		Path:        worktreePath,
		Program:     "claude",
		Status:      session.Stopped,
		SessionType: session.SessionTypeDirectory,
		CreatedAt:   now,
		UpdatedAt:   now,
		Worktree: session.GitWorktreeData{
			RepoPath:      worktreePath,
			WorktreePath:  worktreePath,
			SessionName:   sessionUUID,
			BranchName:    "main",
			BaseCommitSHA: "0000000000000000000000000000000000000000",
		},
	}); err != nil {
		log.Error("backlog debug seed: create work session instance failed", "err", err)
		http.Error(w, "failed to create work session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := h.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleWork,
	}); err != nil {
		log.Error("backlog debug seed: create work item session failed", "err", err)
		http.Error(w, "failed to create item session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(seedWorkSessionWithWorktreeResponse{
		ItemID:       item.ID,
		SessionID:    sessionUUID,
		WorktreePath: worktreePath,
	}); err != nil {
		log.Error("backlog debug seed: encode response failed", "err", err)
	}
}

type seedJulesWorkSessionRequest struct {
	Title  string `json:"title"`
	Status string `json:"status"` // defaults to "review" if empty
	// ItemID, when set, attaches the seeded jules_work ItemSession to an
	// already-existing backlog item instead of creating a new one via
	// Title/Status (both ignored when ItemID is set) — lets a test simulate
	// "a dispatch just completed for this exact item" after driving
	// JulesDispatchDialog's real UI through a DispatchToJules call that was
	// intercepted client-side to avoid a live billed Jules API round trip
	// (jules-dispatch.spec.ts's §7.2 scenario: DispatchToJules's own guard
	// chain calls the real Jules ListSources/CreateSession endpoints, which
	// no e2e-local test credential can pass). Everything downstream of the
	// seeded row — the storage write and the real WatchBacklogItems event it
	// emits — runs unmocked, so the UI observes the new row through the same
	// live-update path a genuine dispatch would use.
	ItemID string `json:"itemId"`
	// Ended, when true, closes the seeded jules_work ItemSession so
	// SessionsSection.tsx's computeJulesPhase resolves it to "done"/"failed"
	// (per EndReason) instead of "running" — the only two closed-row phases
	// that computation can produce (see JulesStatusBadge.tsx's phase union
	// vs. computeJulesPhase's binary open/closed branching: "queued" and
	// "needs-review" are never reachable through this real data path today).
	Ended     bool   `json:"ended"`
	EndReason string `json:"endReason"` // e.g. "jules_completed" or "jules_failed"; defaults to "jules_completed" when Ended is true and this is empty
	// PrNumber/PrUrl, when set, are written onto the created item (mirrors
	// handleSeed's PR-field seeding above) so a test can also exercise
	// PullRequestSection.tsx's "Opened by Jules" provenance marker, which
	// requires status "pr_pending" + a PR URL + the item's newest linked
	// session having role jules_work.
	PrNumber int    `json:"prNumber"`
	PrUrl    string `json:"prUrl"`
}

type seedJulesWorkSessionResponse struct {
	ItemID    string `json:"itemId"`
	SessionID string `json:"sessionId"`
}

// handleSeedJulesWorkSession creates a backlog item with a linked jules_work
// ItemSession (Story 3.3.2) directly through the storage layer — no real
// Jules API dispatch/poll involved — so the e2e suite can put
// JulesStatusBadge/PullRequestSection's Jules-provenance marker in front of
// the UI in a chosen phase without a live jules.google.com session. Mirrors
// handleSeedWorkItemSession's shape.
func (h *BacklogDebugSeedHandler) handleSeedJulesWorkSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.storage == nil {
		http.Error(w, "storage not available", http.StatusServiceUnavailable)
		return
	}

	var req seedJulesWorkSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var item *session.BacklogItemData
	if req.ItemID != "" {
		existing, err := h.storage.GetBacklogItem(ctx, req.ItemID)
		if err != nil {
			log.Error("backlog debug seed: get item failed", "err", err, "item_id", req.ItemID)
			http.Error(w, "failed to find backlog item: "+err.Error(), http.StatusNotFound)
			return
		}
		item = existing
	} else {
		if req.Title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		status := req.Status
		if status == "" {
			status = string(session.BacklogStatusReview)
		}
		created, err := h.storage.CreateBacklogItem(ctx, session.BacklogItemData{
			Title:  req.Title,
			Status: status,
		})
		if err != nil {
			log.Error("backlog debug seed: create item failed", "err", err)
			http.Error(w, "failed to create backlog item: "+err.Error(), http.StatusInternalServerError)
			return
		}
		item = created
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

	// julesSessionUUIDPrefix ("jules-", jules_dispatch_service.go) + the
	// "sessions/<id>" shape a real Jules session name takes, so the seeded
	// row reproduces the exact "jules-sessions/{id}" storage form ADR-004
	// documents — julesWebUrlFor (SessionsSection.tsx) strips this same
	// prefix to build the jules.google.com escape-hatch link.
	sessionUUID := julesSessionUUIDPrefix + "sessions/" + uuid.New().String()
	itemSession, err := h.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleJulesWork,
	})
	if err != nil {
		log.Error("backlog debug seed: create jules_work item session failed", "err", err)
		http.Error(w, "failed to create item session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if req.Ended {
		endReason := req.EndReason
		if endReason == "" {
			endReason = "jules_completed"
		}
		if err := h.storage.UpdateItemSessionEndedWithReason(ctx, itemSession.ID, time.Now(), endReason); err != nil {
			log.Error("backlog debug seed: end jules_work item session failed", "err", err)
			http.Error(w, "failed to mark item session ended: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(seedJulesWorkSessionResponse{
		ItemID:    item.ID,
		SessionID: sessionUUID,
	}); err != nil {
		log.Error("backlog debug seed: encode response failed", "err", err)
	}
}
