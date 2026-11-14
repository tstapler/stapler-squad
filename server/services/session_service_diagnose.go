package services

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/summaries"
)

// uniqueDispatchTitle builds a title for a repeatedly-dispatchable one-shot
// session (diagnose, review) from prefix and item.ID that stays unique across
// redispatches. A bare "prefix:item.ID[:8]" title is only safe for the FIRST
// dispatch: the session row from that dispatch is never deleted, so a second
// dispatch for the same item builds a brand-new Instance (fresh UUID) under
// the same title, and Storage.AddInstance rejects it with ErrTitleConflict
// because the UUIDs differ (session/storage.go's AddInstance doc comment) —
// surfaced to the operator as a raw "[internal] ... already exists" error
// with no indication the item itself is fine. The session-role identity
// check (session.Storage.IsDiagnoseCaller) keys off the ItemSession row's
// SessionUUID, not this title, so appending a per-dispatch suffix here is
// safe.
func uniqueDispatchTitle(prefix, itemID string) string {
	return fmt.Sprintf("%s:%s:%x", prefix, itemID[:8], time.Now().UnixNano())
}

// SessionSpawnOptions bundles the session-creation fields shared by
// CreateDirectorySession and CreateWorktreeSession (Fowler's Introduce
// Parameter Object), replacing their previous 8/9-parameter positional lists
// (syntax-rules-go long-parameter-list).
type SessionSpawnOptions struct {
	Title   string
	Prompt  string
	Tags    []string
	OneShot bool
	Hidden  bool
	// ProgramOverride, when non-empty, replaces config.ResolveDefaults' resolved.Program
	// as the spawned Instance's InstanceOptions.Program — set before session.NewInstance/
	// instance.Start(true), never via a post-hoc SwitchProgram/Restart (see Epic 2.4's
	// design note). Leave "" for callers unaffected by per-stage program overrides.
	ProgramOverride string
	// AllowedTools restricts the dispatched session's MCP tool surface (see
	// diagnosticSessionAllowedTools's doc comment); "" preserves the default,
	// unrestricted behavior.
	AllowedTools string
}

// reviewTarget is where a review session runs and the branch scoping it must be told.
type reviewTarget struct {
	Dir      string
	Preamble string
}

// resolveReviewTarget points a review at the work session's branch rather than
// whatever the shared checkout at repoPath has checked out. It prefers the work
// worktree as the working directory; when that is gone it falls back to repoPath
// but tells the reviewer to diff the named branch explicitly. With no recorded
// branch (directory-mode work) it returns repoPath and an empty preamble.
func resolveReviewTarget(repoPath string, wt domain.GitWorktreeData) reviewTarget {
	if wt.BranchName == "" {
		return reviewTarget{Dir: repoPath}
	}
	base := wt.BaseCommitSHA
	if base == "" {
		base = "the default branch"
	}
	if info, err := os.Stat(wt.WorktreePath); wt.WorktreePath != "" && err == nil && info.IsDir() {
		return reviewTarget{
			Dir: wt.WorktreePath,
			Preamble: fmt.Sprintf("## Work Branch (authoritative)\nThe change under review is on branch `%s`, checked out in your working directory (%s). "+
				"Review that branch only: `git diff %s..%s`. Do not review any other branch.\n\n", wt.BranchName, wt.WorktreePath, base, wt.BranchName),
		}
	}
	return reviewTarget{
		Dir: repoPath,
		Preamble: fmt.Sprintf("## Work Branch (authoritative)\nThe change under review is on branch `%s`. Your working directory is the shared repository checkout "+
			"and its HEAD is NOT that branch: never use HEAD or `<default>...HEAD`. Use `git diff %s..%s` (and `git show %s:<path>`) to read it.\n\n",
			wt.BranchName, base, wt.BranchName, wt.BranchName),
	}
}

// reviewTargetFor looks up the reviewed work session's worktree. Lookup failure
// degrades to the repo path with no preamble, as before.
func (s *SessionService) reviewTargetFor(ctx context.Context, item *backlogdto.BacklogItemData, itemSessionID string) reviewTarget {
	if s.concStorage == nil || itemSessionID == "" {
		return reviewTarget{Dir: item.RepoPath}
	}
	is, err := s.concStorage.GetItemSession(ctx, itemSessionID)
	if err != nil {
		log.Warn("[SpawnReviewSession] item session lookup failed; reviewing in repo path", "item", item.ID, "err", err)
		return reviewTarget{Dir: item.RepoPath}
	}
	wt, err := s.concStorage.GetWorktreeDataBySessionUUID(ctx, is.SessionUUID)
	if err != nil {
		log.Warn("[SpawnReviewSession] worktree lookup failed; reviewing in repo path", "item", item.ID, "err", err)
		return reviewTarget{Dir: item.RepoPath}
	}
	return resolveReviewTarget(item.RepoPath, wt)
}

// SpawnReviewSession satisfies the session.ReviewGateSpawner interface so that
// BacklogLifecycleListener can spawn one-shot review sessions automatically when
// a work session exits. The session is tagged "backlog:review", runs one-shot, and
// runs in the work session's worktree (itemSessionID is the work ItemSession), not
// the shared checkout, whose HEAD is arbitrary.
func (s *SessionService) SpawnReviewSession(ctx context.Context, item *backlogdto.BacklogItemData, itemSessionID string, prompt string) (*instance.Instance, error) {
	target := s.reviewTargetFor(ctx, item, itemSessionID)
	inst, err := s.CreateDirectorySession(ctx, target.Dir, SessionSpawnOptions{
		Title:   uniqueDispatchTitle("review", item.ID),
		Prompt:  target.Preamble + prompt,
		Tags:    []string{"backlog:review"},
		OneShot: true,
		Hidden:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("spawn review session: %w", err)
	}
	inst.SetCategory(instance.CategoryBacklog)
	return inst, nil
}

// diagnosticSessionAllowedTools restricts a dispatched Diagnose & Nudge
// session (server/mcp's diagnoseHandlers) to the minimal MCP tool surface its
// buildDiagnosePrompt action space actually offers: investigate the repo with
// Claude Code's own read/search tools, then conclude via exactly one of
// create_backlog_item (file a bug), post_backlog_update (a note),
// submit_diagnosis_result (always, to close out), or diagnose_nudge_session
// (redirect a stalled linked session). Deliberately excludes
// write_to_session/steer_session/send_control/run_command/resume_session —
// this session has no legitimate reason to touch another session's terminal
// except through the narrow, gated diagnose_nudge_session path.
//
// This is defense-in-depth, not the real enforcement: --allowedTools has been
// proven to provide no real technical enforcement in this codebase (see
// session/backlog_review.go's BuildReviewCallOptions doc comment and ADR-001's
// 2026-07-15 addendum). The actual gate is server-side — see
// server/mcp/diagnose_role_gate.go's denyIfDiagnoseCaller, which the
// restricted tools above check independent of what a client honors here.
const diagnosticSessionAllowedTools = "Read,Grep,Glob,Bash," +
	"mcp__stapler-squad__create_backlog_item," +
	"mcp__stapler-squad__post_backlog_update," +
	"mcp__stapler-squad__get_backlog_item," +
	"mcp__stapler-squad__get_backlog_stuck_report," +
	"mcp__stapler-squad__submit_diagnosis_result," +
	"mcp__stapler-squad__diagnose_nudge_session," +
	"mcp__stapler-squad__diagnose_reset_remediation," +
	"mcp__stapler-squad__suggest_workflow_action," +
	"mcp__stapler-squad__report_harness_gap"

// SpawnDiagnosticSession creates a hidden, one-shot Diagnose & Nudge session
// for item, carrying prompt (the assembled context bundle plus dispatch
// instructions — see server/services/diagnostic_service.go's
// buildDiagnosePrompt). Mirrors SpawnReviewSession's shape; satisfies
// services.DiagnosticSpawner. Unlike SpawnReviewSession, restricts the
// session's MCP tool surface via diagnosticSessionAllowedTools — see that
// constant's doc comment for why this alone isn't the real enforcement.
func (s *SessionService) SpawnDiagnosticSession(ctx context.Context, item *backlogdto.BacklogItemData, prompt string) (*instance.Instance, error) {
	inst, err := s.CreateDirectorySession(ctx, item.RepoPath, SessionSpawnOptions{
		Title:        uniqueDispatchTitle("diagnose", item.ID),
		Prompt:       prompt,
		Tags:         []string{"backlog:diagnose"},
		OneShot:      true,
		Hidden:       true,
		AllowedTools: diagnosticSessionAllowedTools,
	})
	if err != nil {
		return nil, fmt.Errorf("spawn diagnostic session: %w", err)
	}
	inst.SetCategory(instance.CategoryBacklog)
	return inst, nil
}

// CreateDirectorySession satisfies the services.SessionCreator interface so that
// BacklogService can spawn sessions without importing SessionService directly.
// It creates a directory-type session at path from opts, wires it into the live
// poller, and returns the Instance. opts.AllowedTools == "" (CreateDirectorySession's
// own callers, i.e. everyone but SpawnDiagnosticSession) preserves unrestricted
// tool access.
func (s *SessionService) CreateDirectorySession(ctx context.Context, path string, opts SessionSpawnOptions) (*instance.Instance, error) {
	cfg := config.LoadConfig()
	resolved := config.ResolveDefaults(cfg, path, "")
	program := resolved.Program
	if opts.ProgramOverride != "" {
		program = opts.ProgramOverride
	}
	instOpts := instance.InstanceOptions{
		Title:            opts.Title,
		Path:             path,
		Program:          program,
		PermissionMode:   instance.PermissionModeAuto, // automated sessions auto-approve tool uses without bypass prompt
		SessionType:      domain.SessionTypeDirectory,
		Prompt:           opts.Prompt,
		Tags:             opts.Tags,
		OneShot:          opts.OneShot,
		Hidden:           opts.Hidden,
		MCPServerURL:     s.resolveMCPServerURL(),
		CreateIfMissing:  true,
		TmuxServerSocket: s.testTmuxServerSocket,
		AllowedTools:     opts.AllowedTools,
		// Backend consults the session-name override map (tymux-bundled-integration
		// Epic 4.4.2) so a canary override applies through this entry point too;
		// there's no per-request override concept for this internal creator.
		Backend: instance.ResolveSessionBackendForTitle(cfg, opts.Title, ""),
	}
	instance, err := instance.NewInstance(instOpts)
	if err != nil {
		return nil, fmt.Errorf("CreateDirectorySession: %w", err)
	}
	// Index before Start: Start launches the agent, whose first hook must resolve
	// as hidden (AddInstance below is too late; see Spike 1.3e).
	s.indexSessionForDelivery(instance)
	// Wire callbacks (including the tagging engine/fire recorder, session-classifier-pipeline
	// Task 2.3.3d) before Start() so a first-time-setup session's ReclassifyTagsAfterCreate
	// call has a real engine to evaluate, matching the primary CreateSession pipeline's
	// wire-before-start ordering (session_creation_pipeline.go).
	s.wireCallbacks(instance)
	if err := instance.Start(true); err != nil {
		return nil, fmt.Errorf("CreateDirectorySession start: %w", err)
	}
	if s.statusManager != nil {
		instance.SetStatusManager(s.statusManager)
		if ctrlErr := instance.StartController(); ctrlErr != nil {
			log.Warn("[CreateDirectorySession] failed to start controller after wiring", "session", opts.Title, "err", ctrlErr)
		}
	}
	session.StartSessionDriver(instance, path)
	if err := s.storage.AddInstance(instance); err != nil {
		_ = instance.Destroy()
		return nil, fmt.Errorf("CreateDirectorySession save: %w", err)
	}
	if s.ReviewQueuePoller != nil {
		s.ReviewQueuePoller.AddInstance(instance)
	}
	if s.sessionTagPoller != nil {
		s.sessionTagPoller.AddInstance(instance)
	}
	s.eventBus.Publish(events.NewSessionCreatedEvent(instance))
	if s.backlogLifecycleListener != nil {
		s.backlogLifecycleListener.WireToInstance(instance)
	}
	if s.sessionSummaryGenerator != nil {
		summaries.WireSessionSummaryListener(s.sessionSummaryGenerator, instance)
	}
	return instance, nil
}

// CreateWorktreeSession satisfies the services.SessionCreator interface.
// It spawns a session that uses an already-created git worktree at worktreePath.
// repoPath is the parent repo (for program resolution). worktreePath must exist on disk.
func (s *SessionService) CreateWorktreeSession(ctx context.Context, repoPath, worktreePath string, opts SessionSpawnOptions) (*instance.Instance, error) {
	cfg := config.LoadConfig()
	resolved := config.ResolveDefaults(cfg, repoPath, "")
	program := resolved.Program
	if opts.ProgramOverride != "" {
		program = opts.ProgramOverride
	}
	instOpts := instance.InstanceOptions{
		Title:            opts.Title,
		Path:             repoPath,
		Program:          program,
		PermissionMode:   instance.PermissionModeAuto,
		SessionType:      domain.SessionTypeExistingWorktree,
		ExistingWorktree: worktreePath,
		Prompt:           opts.Prompt,
		Tags:             opts.Tags,
		OneShot:          opts.OneShot,
		Hidden:           opts.Hidden,
		MCPServerURL:     s.resolveMCPServerURL(),
		CreateIfMissing:  false,
		TmuxServerSocket: s.testTmuxServerSocket,
		// Backend consults the session-name override map (tymux-bundled-integration
		// Epic 4.4.2) so a canary override applies through this entry point too;
		// there's no per-request override concept for this internal creator.
		Backend: instance.ResolveSessionBackendForTitle(cfg, opts.Title, ""),
	}
	instance, err := instance.NewInstance(instOpts)
	if err != nil {
		return nil, fmt.Errorf("CreateWorktreeSession: %w", err)
	}
	s.indexSessionForDelivery(instance) // before Start, as in CreateDirectorySession
	// See CreateDirectorySession's matching comment: wire callbacks (tagging engine/fire
	// recorder included) before Start() so ReclassifyTagsAfterCreate has a real engine.
	s.wireCallbacks(instance)
	if err := instance.Start(true); err != nil {
		return nil, fmt.Errorf("CreateWorktreeSession start: %w", err)
	}
	if s.statusManager != nil {
		instance.SetStatusManager(s.statusManager)
		if ctrlErr := instance.StartController(); ctrlErr != nil {
			log.Warn("[CreateWorktreeSession] failed to start controller after wiring", "session", opts.Title, "err", ctrlErr)
		}
	}
	session.StartSessionDriver(instance, repoPath)
	if err := s.storage.AddInstance(instance); err != nil {
		_ = instance.Destroy()
		return nil, fmt.Errorf("CreateWorktreeSession save: %w", err)
	}
	if s.ReviewQueuePoller != nil {
		s.ReviewQueuePoller.AddInstance(instance)
	}
	if s.sessionTagPoller != nil {
		s.sessionTagPoller.AddInstance(instance)
	}
	s.eventBus.Publish(events.NewSessionCreatedEvent(instance))
	if s.backlogLifecycleListener != nil {
		s.backlogLifecycleListener.WireToInstance(instance)
	}
	if s.sessionSummaryGenerator != nil {
		summaries.WireSessionSummaryListener(s.sessionSummaryGenerator, instance)
	}
	return instance, nil
}
