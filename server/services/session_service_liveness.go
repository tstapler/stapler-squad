package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// FindLiveInstance returns the live in-memory instance held by the ReviewQueuePoller,
// or nil if the poller is not wired or the session is not found. Use this instead of
// LoadInstances() for read-only and mutation operations that need the live instance
// (with its PTY handles and controller state).
func (s *SessionService) FindLiveInstance(id string) *session.Instance {
	if s.reviewQueuePoller == nil {
		return nil
	}
	return s.reviewQueuePoller.FindInstance(id)
}

// findInstanceByID returns the first of instances whose MatchesID(id) is
// true, or nil — the shared linear-scan lookup behind every RPC handler
// below that resolves a request's Id (UUID or legacy title) against an
// already-loaded instance list (an already-live FindLiveInstance miss, a
// LoadInstances() call, or the poller's GetInstances() snapshot).
func findInstanceByID(instances []*session.Instance, id string) *session.Instance {
	for _, inst := range instances {
		if inst.MatchesID(id) {
			return inst
		}
	}
	return nil
}

// SessionProgram implements SessionSteerer. Returns ok=false if sessionUUID
// has no live instance tracked.
func (s *SessionService) SessionProgram(sessionUUID string) (string, bool) {
	inst := s.FindLiveInstance(sessionUUID)
	if inst == nil {
		return "", false
	}
	return inst.Program, true
}

// safeIdleStatusContexts re-exports session.SafeIdleStatusContexts under its
// original package-local name so this file's own
// TestSafeIdleStatusContexts_MatchClaudeIdlePatternDescriptions (which pins
// it against claude.go's actual pattern descriptions) needs no change. The
// allowlist itself now lives in the session package so
// session/nudge_gate.go's CheckNudgeEligible can share it instead of
// maintaining a second copy — see session.SafeIdleStatusContexts's doc
// comment.
var safeIdleStatusContexts = session.SafeIdleStatusContexts

// isSafeSteerStatus reports whether a detection result is safe for an
// unattended PTY write — see session.IsSafeSteerStatus's doc comment.
func isSafeSteerStatus(status detection.DetectedStatus, statusContext string) bool {
	return session.IsSafeSteerStatus(status, statusContext)
}

// IsReadyForSteer implements SessionSteerer. It gates an unattended PTY
// write (e.g. PR-fix steering) on isSafeSteerStatus — see that function's
// doc comment for the StatusIdle-vs-safe-description invariant. Any case
// where readiness can't be confirmed (no live instance, no status manager,
// no registered controller, or a queued/in-flight command) returns false
// rather than assuming ready.
func (s *SessionService) IsReadyForSteer(sessionUUID string) bool {
	inst := s.FindLiveInstance(sessionUUID)
	return inst != nil && s.instanceReadyForSteer(inst) == notReadyNone
}

// SteerActiveSession implements SessionSteerer. It is the internal steer: it
// takes no per-request access decision, its callers are pinned by the guard
// set (Story 5.1d), and a hidden target still succeeds (characterized).
func (s *SessionService) SteerActiveSession(ctx context.Context, sessionUUID, message string) error {
	inst := s.FindLiveInstance(sessionUUID)
	if inst == nil {
		return fmt.Errorf("steer session %q: not tracked live", sessionUUID)
	}
	return s.steerInternal(ctx, inst, message)
}

// ArchiveSessionByUUID satisfies the BacklogService.SessionStopper interface and the
// session.SessionArchiver interface (implemented here so both BacklogService and
// session.BacklogLifecycleListener can soft-archive backlog work sessions without
// reinventing the ArchiveSession RPC's logic — see ArchiveSession above).
// Falls back to a storage-only write when the session isn't in the live in-memory
// registry (FindLiveInstance only searches ReviewQueuePoller.instances, which does not
// necessarily contain every session ever persisted — e.g. after a server restart).
// Without this fallback, archival silently no-ops for such sessions and they never get
// hidden from the default session list, no matter how many times a sweep retries them
// (root cause of the 337/352-session pileup investigated 2026-08-24: a done backlog item
// still had 14 un-archived work sessions despite the done-transition hook and the
// periodic archive_terminal_sessions safety-net both firing correctly).
// No-op (not an error) if the session doesn't exist anywhere or is already archived, so
// callers can invoke this unconditionally from a sweep without extra existence checks.
func (s *SessionService) ArchiveSessionByUUID(ctx context.Context, sessionUUID string) error {
	inst := s.FindLiveInstance(sessionUUID)
	if inst == nil {
		if s.concStorage == nil {
			return nil // fake InstanceStore (tests) — no storage-only fallback available
		}
		archived, err := s.concStorage.ArchiveInstanceDataByID(sessionUUID, time.Now())
		if err != nil {
			return fmt.Errorf("failed to archive session %s via storage fallback: %w", sessionUUID, err)
		}
		// Re-check for a live instance: the session may have been resumed (added back
		// to the poller, e.g. via ResumeHibernatedSession) in the window between the
		// FindLiveInstance miss above and the storage write just now. If so, the write
		// above raced a genuinely live session and may have clobbered its storage row
		// back to a stale, ArchivedAt=now/Status=Stopped snapshot even though the
		// in-memory Instance is still active. Re-save the live instance's actual
		// (unmodified) current state to overwrite that stale write immediately, rather
		// than leaving storage inconsistent with the poller until whatever next
		// unrelated mutation happens to call SaveInstances for it.
		if archived {
			log.Info("ArchiveSessionByUUID: archived via storage-only fallback (session not in live poller)", "uuid", sessionUUID)
			if reInst := s.FindLiveInstance(sessionUUID); reInst != nil {
				if err := s.storage.SaveInstances([]*session.Instance{reInst}); err != nil {
					return fmt.Errorf("failed to re-save resumed session %s after storage fallback race: %w", sessionUUID, err)
				}
			}
			s.eventBus.Publish(events.NewSessionArchivedEvent(sessionUUID))
		}
		return nil
	}
	// SetArchivedAtIfNilAndStop also transitions Status to Stopped (see ArchiveSession's
	// comment) — safe to call unconditionally: no-ops the transition if already Stopped.
	if !inst.SetArchivedAtIfNilAndStop(time.Now()) {
		return nil // already archived
	}
	if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
		return fmt.Errorf("failed to save archived session %s: %w", sessionUUID, err)
	}
	// Notifies event-driven cleanup (ReactiveQueueManager evicting any stale
	// review-queue entry) that this session left the live/visible set, mirroring
	// DeleteSession's EventSessionDeleted publish below.
	s.eventBus.Publish(events.NewSessionArchivedEvent(sessionUUID))
	return nil
}

// stopGuardrailSession ends the backlog ItemSession with reason (so the
// orchestrator's reconcilers see why it stopped and can respawn or flag the
// item) and then kills the live session.
func stopGuardrailSession(ctx context.Context, st *session.Storage, inst *session.Instance, reason string) error {
	var endErr error
	is, err := st.GetItemSessionBySessionUUID(ctx, inst.Snapshot().UUID)
	switch {
	case err == nil:
		endErr = st.UpdateItemSessionEndedWithReason(ctx, is.ID, time.Now(), reason)
	case !errors.Is(err, session.ErrNotFound): // not-found = not backlog-linked, nothing to end
		endErr = err
	}
	// Kill even if the end-reason write failed; both errors reach the caller.
	return errors.Join(endErr, inst.Kill())
}

// StopSessionByUUID satisfies the BacklogService.SessionStopper interface.
// It kills the live tmux session identified by UUID (best-effort; errors are non-fatal).
func (s *SessionService) StopSessionByUUID(ctx context.Context, sessionUUID string) error {
	inst := s.findConfirmedLiveInstance(sessionUUID)
	if inst == nil {
		return nil // already gone
	}
	if err := inst.Kill(); err != nil {
		log.Warn("StopSessionByUUID: kill failed", "uuid", sessionUUID, "err", err)
		return err
	}
	return nil
}

// findConfirmedLiveInstance is the canonical liveness-truth check backing
// IsSessionLive, KillTmuxPaneOnly, and StopSessionByUUID. FindLiveInstance's
// map membership alone is not proof of death: a session can transiently drop
// out of the live poller's map during a reconciliation hiccup while its real
// tmux process keeps running (the backlog-orchestration-layer counterpart to
// the tmux-layer stale-liveness bugs #791/#799 fixed — a map-miss false
// negative instead of a stale-pointer false positive). Confirmed live 2026-09-12:
// AutoReopenAfterFailedReview's reuse check and spawnSessionAfterGates' step-8b
// guard both trusted this map alone, wrongly concluded a still-running work
// session was dead, and let a duplicate session spawn into the same shared
// worktree while the original kept writing to it.
//
// Fast path: the live poller (cheap, no subprocess). On a miss, reconstructs a
// read-only "shadow" Instance from persisted data
// (session.FromInstanceDataDeferred — no Start(), no PTY, no goroutines) bound
// to the same tmux session identity, and asks it directly via the canonical
// Instance.IsBackendProcessAlive() truth check before concluding dead. Only a
// genuine dead-on-both-signals result returns nil; a confirmed-alive shadow
// instance is returned so callers can act on the real underlying session
// (e.g. KillTmuxPaneOnly killing it) instead of just answering the liveness
// question. Caveat: FindInstanceDataByID loads with LoadMinimal, so a shadow
// instance's worktree metadata is empty — fine for KillTmuxPaneOnly (never
// touches the worktree) and StopSessionByUUID's existing best-effort,
// error-dropped cleanup call sites, but not a source of truth for worktree
// deletion.
func (s *SessionService) findConfirmedLiveInstance(sessionUUID string) *session.Instance {
	if inst := s.FindLiveInstance(sessionUUID); inst != nil {
		return inst
	}
	if s.concStorage == nil {
		return nil // fake InstanceStore (tests) — no direct-check fallback available
	}
	data, err := s.concStorage.FindInstanceDataByID(sessionUUID)
	if err != nil || data == nil {
		return nil
	}
	shadow, err := session.FromInstanceDataDeferred(*data)
	if err != nil {
		return nil
	}
	if !shadow.IsBackendProcessAlive() {
		return nil
	}
	log.Warn("findConfirmedLiveInstance: session missing from live poller map but tmux/process truth check confirms it is still alive", "uuid", sessionUUID)
	return shadow
}

// IsSessionLive satisfies the BacklogService.SessionStopper interface.
// It returns true if sessionUUID is confirmed live — see
// findConfirmedLiveInstance for why that is not simply map membership.
func (s *SessionService) IsSessionLive(sessionUUID string) bool {
	return s.findConfirmedLiveInstance(sessionUUID) != nil
}

// TimeSinceLastMeaningfulOutput satisfies the BacklogService.SessionStopper
// interface. It reports how long it has been since sessionUUID's live
// Instance last produced meaningful terminal output. ok is false when the
// session isn't currently tracked live (mirrors IsSessionLive's "not found"
// case) — callers must not use dur in that case.
func (s *SessionService) TimeSinceLastMeaningfulOutput(sessionUUID string) (time.Duration, bool) {
	inst := s.FindLiveInstance(sessionUUID)
	if inst == nil {
		return 0, false
	}
	return inst.GetTimeSinceLastMeaningfulOutput(), true
}

// canonicalizeAbsPath resolves path to an absolute, symlink-canonicalized form
// (e.g. macOS's /var -> /private/var) so path comparisons aren't fooled by two
// spellings of the same directory. Shared by
// OtherLiveSessionInsideWorktree, ConversationOwnedByOtherLiveSession, and
// wireCallbacks' spawn-reservation closure.
func canonicalizeAbsPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for %q: %w", path, err)
	}
	return git.CanonicalizeWorktreePath(abs), nil
}

// OtherLiveSessionInsideWorktree reports whether some OTHER currently-live
// session (any UUID besides excludeUUID) has its actual runtime working
// directory — Instance.GetCurrentWorkingDirectory(), a live pane/process
// introspection, not the persisted DB path field, which can report the
// canonical repo root rather than the worktree a session is really running
// in — resolving inside worktreePath. Defense-in-depth for stop_session/
// pause_session, which both delete the target session's git worktree:
// rework rounds of the same backlog item deliberately share one worktree
// (see spawnSessionAfterGates' backlogWorkBranchSlug), so destroying it out
// from under a still-running sibling round would corrupt its in-progress
// work — the exact situation the operator had to route around by killing
// tmux panes directly during the 2026-09-12 incident this guards against.
func (s *SessionService) OtherLiveSessionInsideWorktree(excludeUUID, worktreePath string) (blockingUUID string, blocked bool) {
	if s.reviewQueuePoller == nil || worktreePath == "" {
		return "", false
	}
	cleanTarget, err := canonicalizeAbsPath(worktreePath)
	if err != nil {
		return "", false
	}
	for _, inst := range s.reviewQueuePoller.GetInstances() {
		if inst == nil || inst.UUID == excludeUUID || !inst.IsBackendProcessAlive() {
			continue
		}
		cwd, cwdErr := inst.GetCurrentWorkingDirectory()
		if cwdErr != nil || cwd == "" {
			continue
		}
		cleanCwd, absErr := canonicalizeAbsPath(cwd)
		if absErr != nil {
			continue
		}
		if cleanCwd == cleanTarget || strings.HasPrefix(cleanCwd, cleanTarget+string(os.PathSeparator)) {
			return inst.UUID, true
		}
	}
	return "", false
}

// conversationOwnershipQuery is ConversationOwnedByOtherLiveSession's input —
// see that method's doc comment for why selfUUID and conversationUUID are
// named fields rather than positional params.
type conversationOwnershipQuery struct {
	selfUUID         string
	conversationUUID string
	path             string
}

// ConversationOwnedByOtherLiveSession reports whether conversationUUID is
// already the ConversationUUID of some OTHER currently-live,
// backend-process-alive Instance (any UUID besides selfUUID) whose own
// GetCurrentWorkingDirectory() resolves to path. Unlike
// OtherLiveSessionInsideWorktree, path overlap alone is not enough to block --
// the sibling must also already own this exact UUID -- so a session detecting
// its OWN first conversation is never blocked, only silently adopting a sibling's.
//
// Takes a conversationOwnershipQuery, not three positional strings: selfUUID
// and conversationUUID are both UUID-shaped, so an unnamed-positional
// signature makes them easy to transpose at a call site with no compiler
// signal (kibitzer primitive-obsession).
func (s *SessionService) ConversationOwnedByOtherLiveSession(q conversationOwnershipQuery) (ownerUUID string, ownedByOther bool) {
	if s.reviewQueuePoller == nil || q.conversationUUID == "" || q.path == "" {
		return "", false
	}
	cleanTarget, err := canonicalizeAbsPath(q.path)
	if err != nil {
		return "", false
	}
	for _, inst := range s.reviewQueuePoller.GetInstances() {
		if inst == nil || inst.UUID == q.selfUUID || !inst.IsBackendProcessAlive() {
			continue
		}
		if inst.GetClaudeConversationUUID() != q.conversationUUID {
			continue
		}
		cwd, cwdErr := inst.GetCurrentWorkingDirectory()
		if cwdErr != nil || cwd == "" {
			continue
		}
		cleanCwd, absErr := canonicalizeAbsPath(cwd)
		if absErr != nil {
			continue
		}
		if cleanCwd == cleanTarget {
			return inst.UUID, true
		}
	}
	return "", false
}

// RefuseIfWorktreeSharedWithOtherLiveSession resolves
// OtherLiveSessionInsideWorktree into a ready-to-return error, shared by
// every worktree-deleting path: MCP pause/stop, RPC UpdateSession, and
// DeleteSession. inst may be nil (no live Instance to inspect) — returns
// nil, same as a non-worktree session.
func (s *SessionService) RefuseIfWorktreeSharedWithOtherLiveSession(inst *session.Instance) error {
	if inst == nil || !inst.HasGitWorktree() {
		return nil
	}
	worktreePath := inst.GetEffectiveRootDir()
	blockingUUID, blocked := s.OtherLiveSessionInsideWorktree(inst.UUID, worktreePath)
	if !blocked {
		return nil
	}
	return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf(
		"cannot proceed: worktree %q is still in use by another active session (%s)",
		worktreePath, blockingUUID))
}

// IsRetryPending satisfies the BacklogService.SessionStopper interface. It
// reports whether sessionUUID's live Instance currently has a driver-managed
// automated retry claimed or scheduled (session-retry-backoff AC8). Returns
// false if the session isn't tracked live.
func (s *SessionService) IsRetryPending(sessionUUID string) bool {
	inst := s.FindLiveInstance(sessionUUID)
	if inst == nil {
		return false
	}
	return inst.IsRetryPending()
}

// KillTmuxPaneOnly satisfies the BacklogService.SessionStopper interface.
// It closes the tmux pane only (Instance.KillSession), leaving the worktree
// intact — unlike StopSessionByUUID (Instance.Kill/Destroy), which also runs
// CleanupWorktree and would delete a worktree still in use by the next rework
// round. Best-effort: errors are logged, not returned, since this runs as
// cleanup alongside a new spawn that should proceed regardless.
//
// Deregisters the instance from every poller on a successful kill —
// findConfirmedLiveInstance's fast path trusts FindLiveInstance's poller-map
// hit unconditionally as "live" (its IsBackendProcessAlive fallback check
// only runs on a map miss), so a killed-but-still-registered instance would
// otherwise be misreported live by every later IsSessionLive call, including
// spawnSessionAfterGates' 8b2 check moments after this exact kill.
func (s *SessionService) KillTmuxPaneOnly(ctx context.Context, sessionUUID string) error {
	inst := s.findConfirmedLiveInstance(sessionUUID)
	if inst == nil {
		return nil // already gone
	}
	if err := inst.KillSession(); err != nil {
		log.Warn("KillTmuxPaneOnly: kill failed", "uuid", sessionUUID, "err", err)
		return err
	}
	s.removeFromAllPollers(sessionUUID)
	return nil
}

// readSessionOwnerUUID is a seam so tests don't need a live tmux server.
var readSessionOwnerUUID = tmux.ReadSessionOwnerUUID

// ErrTmuxKillRefused is returned by KillTmuxSessionByTitle when the pane's
// owner couldn't be verified as one of the allowed UUIDs. Callers that treat
// a refused kill as non-fatal can check it with errors.Is.
var ErrTmuxKillRefused = errors.New("tmux kill refused: pane owner not verified")

// isTmuxSessionAbsentText reports whether tmux output/error text means the
// session (or its server) no longer exists.
func isTmuxSessionAbsentText(text string) bool {
	text = strings.ToLower(text)
	for _, marker := range []string{"can't find session", "no such session", "no server running", "error connecting to"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// KillTmuxSessionByTitle satisfies the BacklogService.SessionStopper interface.
// It kills the tmux session whose name is derived from title using the same sanitization
// as initTmuxSession (whitespace stripped, "." and ":" replaced with "_", "staplersquad_"
// prefix). This handles the case where the Instance is no longer tracked in memory
// but the underlying tmux session is still alive.
//
// The kill only proceeds if the pane's STAPLER_SESSION_UUID is in
// allowedOwnerUUIDs; a name match alone can hit an unrelated Instance's pane
// (ce71ad1a). Otherwise it returns ErrTmuxKillRefused.
func (s *SessionService) KillTmuxSessionByTitle(ctx context.Context, title string, allowedOwnerUUIDs ...string) error {
	return killTmuxSessionByTitle(ctx, title, false, allowedOwnerUUIDs)
}

// killTmuxSessionByTitle is KillTmuxSessionByTitle with an opt-in for panes
// that carry no owner marker at all (allowUnmarked). DeleteSession uses it:
// the user explicitly deleted this session, and a pre-marker pane left under
// its name would otherwise outlive it as an orphan. A marker naming a
// different owner is refused regardless.
func killTmuxSessionByTitle(ctx context.Context, title string, allowUnmarked bool, allowedOwnerUUIDs []string) error {
	name := stapleSquadTmuxName(title)

	if !tmuxSessionKillAllowed(ctx, name, allowedOwnerUUIDs, allowUnmarked) {
		return fmt.Errorf("%w: %q", ErrTmuxKillRefused, name)
	}

	killCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := runTmuxKillSession(killCtx, name)
	if err != nil {
		if isTmuxSessionAbsentText(string(out)) {
			return nil // session already gone — not an error
		}
		return fmt.Errorf("tmux kill-session %q: %w (output: %s)", name, err, out)
	}
	return nil
}

// runTmuxKillSession is a seam so tests can observe kills without a live tmux server.
var runTmuxKillSession = func(ctx context.Context, name string) ([]byte, error) {
	args := tmux.ResolveSocket("").Args("kill-session", "-t", name)
	return safeexec.CommandContext(ctx, tmux.Binary(), args...).CombinedOutput()
}

// tmuxSessionKillAllowed reports whether the tmux session named `name`
// either doesn't exist (nothing to protect -- kill-session will hit its own
// idempotent "already gone" handling), or its STAPLER_SESSION_UUID marker is
// in allowedOwnerUUIDs, or it has no marker and allowUnmarked is set.
func tmuxSessionKillAllowed(ctx context.Context, name string, allowedOwnerUUIDs []string, allowUnmarked bool) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	marker, err := readSessionOwnerUUID(checkCtx, tmux.ResolveSocket(""), name)
	if err != nil {
		if isTmuxSessionAbsentText(err.Error()) {
			return true
		}
		if allowUnmarked && strings.Contains(err.Error(), "unknown variable") {
			log.Warn("KillTmuxSessionByTitle: killing tmux session with no owner marker (explicit delete)", "session", name)
			return true
		}
		log.Warn("KillTmuxSessionByTitle: refusing to kill tmux session, could not verify owner",
			"session", name, "err", err)
		return false
	}
	for _, want := range allowedOwnerUUIDs {
		if want != "" && marker == want {
			return true
		}
	}
	log.Warn("KillTmuxSessionByTitle: refusing to kill tmux session, owner marker mismatch",
		"session", name, "marker", marker, "allowed", allowedOwnerUUIDs)
	return false
}

// stapleSquadTmuxName computes the sanitized tmux session name for a given title,
// matching the logic in session/tmux.toStaplerSquadTmuxNameWithPrefix.
func stapleSquadTmuxName(title string) string {
	var sb strings.Builder
	for _, r := range title {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			sb.WriteRune(r)
		}
	}
	sanitized := sb.String()
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")
	return "staplersquad_" + sanitized
}

// rejectRemoteTildePath rejects a `~`-prefixed path for a remote-host
// location (req.Msg.Remote set) instead of expanding it against
// os.UserHomeDir(), which would resolve to this server process's own home
// directory, not the remote host's -- found in pre-ship review. The remote
// host's real home directory isn't resolvable here without an extra SSH
// round trip, so callers targeting a remote host should pass an absolute
// path instead.
//
// Split from expandLocalTildePath below (Fowler's Remove Flag Argument):
// callers pick the function that matches their target instead of passing a
// remote bool flag.
func rejectRemoteTildePath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		return "", fmt.Errorf("path %q: ~ is not supported for remote sessions -- use an absolute path on the remote host", path)
	}
	return path, nil
}

// expandLocalTildePath expands a leading "~" or "~/" in path against this
// process's local home directory. Falls back to returning path unchanged if
// the home directory can't be resolved, or if the expanded result would
// escape the home directory via path traversal.
func expandLocalTildePath(path string) (string, error) {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home, nil
		} else {
			log.Warn("expandLocalTildePath: failed to resolve home directory", "err", err)
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded := filepath.Join(home, path[2:])
			// Guard against path traversal: reject any result that escapes the home directory.
			if !strings.HasPrefix(expanded, home+string(filepath.Separator)) && expanded != home {
				log.Warn("expandLocalTildePath: path traversal rejected", "input", path)
				return path, nil
			}
			return expanded, nil
		} else {
			log.Warn("expandLocalTildePath: failed to resolve home directory", "err", err)
		}
	}
	return path, nil
}
