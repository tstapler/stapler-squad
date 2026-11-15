package backlogsvc

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// spawnAdmission carries one spawn attempt through the admission steps.
// priorSessions and unconfirmedDeadUUIDs are filled by earlier steps for later ones.
type spawnAdmission struct {
	item                 *backlogdto.BacklogItemData
	isReopen             bool
	autonomous           bool
	priorSessions        []backlogdto.ItemSessionSummary
	unconfirmedDeadUUIDs map[string]bool
}

// spawnAdmissionStep either refuses the spawn (non-nil error) or lets it proceed.
// Steps may have side effects (tombstoning, pane kills), so their order is part of
// the contract; TestSpawnSessionAfterGates_should_ApplyAdmissionChecksInOrder pins it.
type spawnAdmissionStep func(s *BacklogService, ctx context.Context, a *spawnAdmission) error

// spawnAdmissionSteps is the ordered admission pipeline of spawnSessionAfterGates.
// A new admission check is one function plus one row here.
var spawnAdmissionSteps = []spawnAdmissionStep{
	(*BacklogService).admitPlanningGate,
	(*BacklogService).admitRepoPath,
	(*BacklogService).admitSessionCreator,
	(*BacklogService).loadPriorSessions,
	(*BacklogService).tombstoneOrphanWork,
	(*BacklogService).admitNoopLoopGate,
	(*BacklogService).killEndedRoundPanes,
	(*BacklogService).admitNoActiveWork,
	(*BacklogService).admitNoConfirmedLiveWork,
	(*BacklogService).admitNoActiveJules,
	(*BacklogService).admitReworkCap,
}

// admitSpawn runs spawnAdmissionSteps in order and stops at the first refusal.
func (s *BacklogService) admitSpawn(ctx context.Context, a *spawnAdmission) error {
	for _, step := range spawnAdmissionSteps {
		if err := step(s, ctx, a); err != nil {
			return err
		}
	}
	return nil
}

func (s *BacklogService) admitPlanningGate(ctx context.Context, a *spawnAdmission) error {
	item, autonomous := a.item, a.autonomous
	// 4b. Planning-gate defense-in-depth (PR #199 review F2/F3). SpawnSessionFromItem's
	// own planning gate (step 3b) only runs on that RPC's direct call path;
	// DequeueNextQueuedItems claims a queued item via transitionWithGuard (which itself
	// now enforces this — F4) and then calls this method directly, with no other gate in
	// between. Re-checking here means an unapproved-plan item can never reach a real
	// spawned session no matter which call site reaches this function, now or in the
	// future. Skipped when autonomous=true (the driver runs its own planning loop) —
	// this matches SpawnSessionFromItem's own gate and means it never fires for
	// AutoReopenAfterFailedReview/AutoReopenForPRFix, which always pass autonomous=true.
	if !item.SkipPlanning && !item.PlanApproved && !autonomous {
		log.Info("[spawnSessionAfterGates] planning gate blocked spawn", "item", item.ID, "status", item.Status, "autonomous", false)
		return connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("run TriggerTriage and approve the plan before spawning, or use 'Run Autonomously' to skip the planning gate"))
	}
	return nil
}

func (s *BacklogService) admitRepoPath(ctx context.Context, a *spawnAdmission) error {
	item := a.item
	// 5. Repo path required.
	if item.RepoPath == "" {
		return connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before spawning a session"))
	}
	return nil
}

func (s *BacklogService) admitSessionCreator(ctx context.Context, a *spawnAdmission) error {
	// 6. Require SessionCreator before doing any DB writes.
	// degraded: sessionCreator unavailable — return CodeUnimplemented so callers can detect the gap.
	if s.sessionCreator == nil {
		return connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("SessionCreator not wired — contact admin"))
	}
	return nil
}

func (s *BacklogService) loadPriorSessions(ctx context.Context, a *spawnAdmission) error {
	item := a.item
	// 8. Load prior sessions for context.
	priorSessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		log.Warn("[SpawnSessionFromItem] failed to load prior sessions", "item", item.ID, "error", err)
		priorSessions = nil
	}
	a.priorSessions = priorSessions
	return nil
}

func (s *BacklogService) tombstoneOrphanWork(ctx context.Context, a *spawnAdmission) error {
	item, priorSessions := a.item, a.priorSessions
	// 8a. Tombstone any work session that never reached its normal completion path
	// (crash, kill, server restart mid-session) before checking 8b below — otherwise a
	// single dead session blocks every future spawn attempt for this item forever. Found
	// live: AutoReopenForPRFix retried every ~60s against the same dead session for
	// hours, bouncing the item in_progress<->pr_pending with no progress (see
	// docs/tasks/backlog-feature-improvement.md).
	s.tombstoneOrphanWorkSessions(ctx, item.ID, priorSessions)
	return nil
}

func (s *BacklogService) admitNoopLoopGate(ctx context.Context, a *spawnAdmission) error {
	item := a.item
	// 8a'. No-op loop gate: refuse to dispatch another work session while a
	// repeated_noop_dispatch row is open (see domain.StuckReasonRepeatedNoopDispatch).
	// Autonomous respawns pass through here too, so this stops every auto path.
	if blocked, gateErr := s.storage.HasOpenStuckReason(ctx, item.ID, domain.StuckReasonRepeatedNoopDispatch); gateErr != nil {
		log.Warn("[spawnSessionAfterGates] no-op gate lookup failed, proceeding", "item", item.ID, "error", gateErr)
	} else if blocked {
		log.Warn("[spawnSessionAfterGates] refusing dispatch: repeated no-op sessions on this item", "item", item.ID)
		return connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item has repeated_noop_dispatch open: consecutive work sessions produced no commits; resolve the duplicate/shipped state (archive or reset the item) before dispatching again"))
	}
	return nil
}

func (s *BacklogService) killEndedRoundPanes(ctx context.Context, a *spawnAdmission) error {
	priorSessions := a.priorSessions
	// 8a2. Close the tmux pane of every already-ended work-session round before
	// spawning the next one. Each rework round gets its own "-rN" title (see
	// buildRevisionTitle) so the session list stays readable across rounds, but
	// nothing previously closed a finished round's tmux pane — it sat around
	// indefinitely as an idle "[exited]" pane, accumulating with every rework
	// cycle. KillTmuxPaneOnly (not StopSessionByUUID/Instance.Kill) leaves the
	// worktree alone, since rework rounds share one worktree/branch. Returns
	// the UUIDs whose kill attempt did NOT confirm dead — see its doc comment
	// and 8b2 below for why that set, not every ended session, is what 8b2
	// needs to re-check.
	a.unconfirmedDeadUUIDs = s.killEndedWorkSessionPanes(ctx, priorSessions)
	return nil
}

func (s *BacklogService) admitNoActiveWork(ctx context.Context, a *spawnAdmission) error {
	priorSessions := a.priorSessions
	// 8b. Guard against spawning a duplicate work session when one is already active.
	if active := findActiveWorkSession(priorSessions); active != nil {
		return connect.NewError(connect.CodeAlreadyExists, s.activeWorkSessionBlockedError(active))
	}
	return nil
}

func (s *BacklogService) admitNoConfirmedLiveWork(ctx context.Context, a *spawnAdmission) error {
	priorSessions, unconfirmedDeadUUIDs := a.priorSessions, a.unconfirmedDeadUUIDs
	// 8b2. Concurrent-liveness cap: independent of both reworkCap (a
	// historical-attempt-count cap, not a right-now-concurrency cap) and the
	// 8b guard above (which trusts priorSessions' EndedAt column). Re-asks the
	// OS/tmux truth question directly via sessionStopper.IsSessionLive rather
	// than DB bookkeeping, as a second, independent line of defense against
	// the exact failure mode fixed in findConfirmedLiveInstance: a work
	// session wrongly tombstoned by a transient liveness-check miss would
	// pass the 8b guard above (its EndedAt is now set) but still fail here if
	// IsSessionLive's OS-truth fallback confirms it's actually still running.
	// unconfirmedDeadUUIDs scopes the recheck to sessions 8a2 could not just
	// confirm dead itself — every other already-ended session was either
	// already gone or was just successfully killed (and deregistered, see
	// KillTmuxPaneOnly), so re-asking IsSessionLive for it would repeat the
	// exact same expensive tmux/storage round trip 8a2 already paid, once per
	// historical rework round, on every single spawn attempt.
	if live := findConfirmedLiveWorkSession(s.sessionStopper, priorSessions, unconfirmedDeadUUIDs); live != nil {
		return connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("a work session (%s) is already confirmed live for this item; refusing to spawn a concurrent one", live.SessionUUID))
	}
	return nil
}

func (s *BacklogService) admitNoActiveJules(ctx context.Context, a *spawnAdmission) error {
	priorSessions := a.priorSessions
	// 8b (Jules). Guard against starting a competing local session while a Jules
	// session for this item is already running on Google's infrastructure — see
	// session.HasActiveJulesSession's doc comment.
	if activeJules := findActiveJulesSession(priorSessions); activeJules != nil {
		return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("a Jules session (%s) is already running for this item; wait for it to finish before starting a local session", activeJules.SessionUUID))
	}
	return nil
}

func (s *BacklogService) admitReworkCap(ctx context.Context, a *spawnAdmission) error {
	item, isReopen, priorSessions := a.item, a.isReopen, a.priorSessions
	// 8b3. Rework-cap backstop, checked only once every "is something already running"
	// guard above (8b/8b2/8b Jules) has cleared — a currently-live session is the real
	// blocker and must be reported as such, not masked by a cap message, when both
	// conditions happen to be true at once. AutoReopenAfterFailedReview and
	// AutoRespawnAutonomousWork each already check effectiveReworkCap before ever
	// calling into this function — this re-check is their "now or in the future, no
	// matter which call site reaches this function" backstop, mirroring step 3b's
	// identical planning-gate re-check precedent above. Without it, the cap was
	// enforced per-caller rather than by this shared primitive: confirmed live on
	// item 9dd2793e, whose work-session count reached a 6th round (buildRevisionTitle's
	// "-r6" title suffix, one past the default cap of 5) with no rework_cap row ever
	// recorded — 11 review bounces and real session cost burned past the point the cap
	// exists to stop, through whichever call path reached here without its own check.
	// Scoped to isReopen only: a fresh spawn from "ready" has no prior work-session
	// history to exceed a cap against.
	if isReopen {
		workCount := 0
		for _, is := range priorSessions {
			if is.Role == instance.SessionRoleWork {
				workCount++
			}
		}
		if reworkCap := s.effectiveReworkCap(item); workCount >= reworkCap {
			log.Info("[spawnSessionAfterGates] hit work session cap; refusing spawn", "item", item.ID, "count", workCount, "cap", reworkCap)
			s.notifyReworkCapHit(ctx, item.ID, item.Title, domain.BacklogStatus(item.Status), "at the shared spawn gate", reworkCap)
			return connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("item has reached its %d-round rework cap; raise the cap in Settings or via the item's rework-cap override before spawning again", reworkCap))
		}
	}
	return nil
}
