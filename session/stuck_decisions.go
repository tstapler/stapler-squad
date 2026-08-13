package session

import (
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/github"
)

// Pure, DB-independent decision functions backing the stuck-item detectors in
// backlog_lifecycle.go (Story 2.1.0). Each takes plain values (times, counts,
// bools) and returns a bool — no ent, no storage, no GitHub I/O — so the
// fuzziest threshold/cycle arithmetic (the false-positive risk root cause #4
// flagged as a rabbit hole) is exhaustively table-driven-testable without a
// DB. Reconcilers call these instead of inlining the comparisons.

// prReadyThreshold is how long a pr_pending item's PR must have been
// green-and-mergeable (per prReadyToMergeSolo) before it is flagged
// pr_ready_unmerged. See plan.md "Unresolved Questions" #1.
const prReadyThreshold = 30 * time.Minute

// abandonedReviewGrace is how long a review-status item's most recent
// to_status="review" BacklogStatusEvent must be in the past, with nothing
// active in flight, before it is flagged abandoned_review. Gives the 60s
// reconcile one or more ticks to re-spawn a review gate before flagging.
const abandonedReviewGrace = 15 * time.Minute

// bounceThreshold is the minimum number of in_progress->review round trips
// within bounceLookback (with no PASS verdict) that flags an item bouncing.
// Chosen to match server/services/backlog_service_triage.go's default rework
// cap (3), but is an independent constant, NOT read from config.Config — the
// rework cap is now user-configurable (Settings → Defaults) while this one
// isn't, so the two can drift. Session-package reconcilers have no config.Config
// plumbed in today; wire that through if this ever needs to move in lockstep.
const bounceThreshold = 3

// bounceLookback bounds the bouncing detector to *active* thrashing.
const bounceLookback = 24 * time.Hour

// bounceMainBranch is the branch reconcileBouncingItems' shipped-without-a-PR
// fallback checks a bouncing item's most recent work-session commit against.
// Matches server/services/backlog_service_triage.go's prFixMainBranch — kept
// as an independent constant since this package cannot import server/services.
const bounceMainBranch = "main"

// multiReasonThreshold is the minimum count of simultaneously open
// *non-escalation* stuck reasons on one item before it is escalated to
// domain.StuckReasonMultipleReasons. Fixed per plan.md's Pattern Decisions
// table (requirements.md's own live-data-informed default: "≥2 would have
// caught both" of this project's motivating live cases) — not a tunable
// scoring rubric.
const multiReasonThreshold = 2

// multiReasonNotifyDwell is how long domain.StuckReasonMultipleReasons' row
// must have been open (FirstDetectedAt) before its notification fires, so a
// single-tick threshold crossing doesn't notify immediately — the same
// one-reconcile-tick-width debounce shape as prReadyThreshold/
// abandonedReviewGrace, sized to the reconcile ticker's period. Independent
// constant, NOT read from server/dependencies.go's ticker constant: the
// session package cannot import server, matching bounceMainBranch's existing
// precedent for duplicating a cross-package constant.
const multiReasonNotifyDwell = 60 * time.Second

// stuckPRReady reports whether a pr_ready_unmerged condition first observed
// at firstDetected has held long enough (> prReadyThreshold) as of now to be
// notification-worthy. Exact-threshold and under-threshold both return false
// (no premature flag).
func stuckPRReady(firstDetected, now time.Time) bool {
	return now.Sub(firstDetected) > prReadyThreshold
}

// abandonedReview reports whether a review-status item's most recent
// to_status="review" transition at lastReviewAt is old enough (> 15m) as of
// now to be flagged abandoned_review.
func abandonedReview(lastReviewAt, now time.Time) bool {
	return now.Sub(lastReviewAt) > abandonedReviewGrace
}

// staleWork reports whether an in_progress item's active work session has
// gone quiet long enough (> maxWorkSessionStaleness, reused unchanged — no
// new threshold introduced) to be flagged stale_work.
func staleWork(lastProgress, now time.Time) bool {
	return now.Sub(lastProgress) > maxWorkSessionStaleness
}

// isBouncing reports whether cycleCount in_progress->review round trips
// within the lookback window, with no recorded PASS verdict, constitute a
// non-converging "bouncing" cycle (>= bounceThreshold and !hasPass).
func isBouncing(cycleCount int, hasPass bool) bool {
	return cycleCount >= bounceThreshold && !hasPass
}

// isMultiReasonEscalated reports whether openNonEscalationCount — the number
// of simultaneously open, non-escalation stuck reasons on one item — meets or
// exceeds multiReasonThreshold, i.e. the item should carry an open
// domain.StuckReasonMultipleReasons row.
func isMultiReasonEscalated(openNonEscalationCount int) bool {
	return openNonEscalationCount >= multiReasonThreshold
}

// multiReasonEscalationNotifyReady reports whether a
// domain.StuckReasonMultipleReasons row first observed at firstDetected has
// held long enough (>= multiReasonNotifyDwell) as of now to be
// notification-worthy — mirrors stuckPRReady/abandonedReview's "don't notify
// on the very tick that created the row" shape.
func multiReasonEscalationNotifyReady(firstDetected, now time.Time) bool {
	return now.Sub(firstDetected) >= multiReasonNotifyDwell
}

// IsRepeatedFailure reports whether the two most recent review verdicts (most
// recent first, as returned by Storage.GetRecentReviewVerdictSummaries) are a
// non-PASS outcome paired with an identical summary — i.e. the last rework
// attempt changed nothing about why the item failed. This catches a
// fast-looping non-converging cycle (e.g. an infrastructure error like a
// missing diff, reproduced on every attempt) well before bounceThreshold's
// 3-cycles-in-24h window would, since a broken-worktree or similar
// environment fault can otherwise burn through the entire rework cap in
// minutes without ever changing outcome. Exported: called from
// server/services across the package boundary (AutoReopenAfterFailedReview).
func IsRepeatedFailure(recent []ReviewVerdictSummary) bool {
	if len(recent) < 2 {
		return false
	}
	latest, prior := recent[0], recent[1]
	if latest.OverallOutcome == string(ReviewOutcomePass) {
		return false
	}
	return latest.OverallOutcome == prior.OverallOutcome && latest.Summary != "" && latest.Summary == prior.Summary
}

// consecutiveNoVerdictReviewThreshold is how many consecutive review-role
// ItemSessions with no ReviewVerdict row at all must be observed (most recent
// first) before IsRepeatedNoVerdictFailure trips. Matches IsRepeatedFailure's
// "two identical attempts in a row" threshold for consistency.
const consecutiveNoVerdictReviewThreshold = 2

// IsRepeatedNoVerdictFailure reports whether the most recent
// consecutiveNoVerdictReviewThreshold review-role ItemSessions for an item
// (ordered most-recent-first, one bool per session: true if that session ever
// had a ReviewVerdict row written) all exited without ever calling
// submit_review_verdict — a crash, kill, or turn-cap stop on the review side.
//
// IsRepeatedFailure alone is blind to this failure shape: it only ever sees
// sessions returned by Storage.GetRecentReviewVerdictSummaries, which queries
// itemsession.HasReviewVerdict() — a review session that never wrote a
// verdict is invisible to that query entirely, so two (or twenty) such
// sessions in a row never produce two comparable summaries and the breaker
// can never trip. That gap let a live item bounce 78 times in 24h — with the
// rework cap recently raised from 3 to 20, well out of reach — before catching
// it (see docs/tasks/backlog-feature-improvement.md, 2026-07-19 update). A run
// of verdict-less review exits carries the identical "nothing about this
// attempt changed" signal as two matching-summary failures, so it's treated
// the same way: stop the auto-reopen loop instead of burning through the
// rework cap.
func IsRepeatedNoVerdictFailure(hadVerdict []bool) bool {
	if len(hadVerdict) < consecutiveNoVerdictReviewThreshold {
		return false
	}
	for _, v := range hadVerdict[:consecutiveNoVerdictReviewThreshold] {
		if v {
			return false
		}
	}
	return true
}

// IsFlakyVerdictFlipFlop reports whether the two most recent review verdicts
// (most recent first, as returned by Storage.GetRecentReviewVerdictSummaries)
// share the same non-empty DiffHash but landed on a different OverallOutcome
// — i.e. the identical reviewed diff got two different answers, the
// signature of a flaky/non-deterministic review rather than a real fix or
// regression (the code under review never changed between attempts). An
// empty DiffHash is "unknown, not computed" and is never treated as a match
// — two unknowns are not evidence of anything. False on fewer than 2
// verdicts, and false when the outcomes agree (that repeated-same-outcome
// shape is IsRepeatedFailure's job, not this one).
//
// Known false-positive source (documented, not filtered — see
// validation.md): a manual OverrideBy="user" verdict interleaved with an
// automated one can look identical to a flip-flop but is actually a human
// correction, not model variance. OverrideBy isn't part of
// []ReviewVerdictSummary today, so it can't be excluded here.
func IsFlakyVerdictFlipFlop(recent []ReviewVerdictSummary) bool {
	if len(recent) < 2 {
		return false
	}
	latest, prior := recent[0], recent[1]
	if latest.DiffHash == "" || prior.DiffHash == "" {
		return false
	}
	if latest.DiffHash != prior.DiffHash {
		return false
	}
	return latest.OverallOutcome != prior.OverallOutcome
}

// TestOnlyReworkMinAttempts is how many consecutive rework attempts (most
// recent first) must have touched test-only files before
// IsTestOnlyReworkCycle trips. An unvalidated starting guess — no
// calibration corpus exists yet beyond the n≈3 items (ccbfe7a6, e271db3d,
// 92d679fd) that motivated this item; see validation.md. Exported: callers
// that build the []string attempt list to pass in (e.g.
// recentWorkSessionFileLists in server/services/backlog_service_triage.go)
// must request at least this many attempts, and must reference this
// constant rather than duplicating the literal — a hardcoded copy at the
// call site would silently go stale if this threshold is ever retuned.
const TestOnlyReworkMinAttempts = 2

// testFileSuffixes are the file-path suffixes IsTestOnlyReworkCycle treats as
// a test/spec file. Deliberately a fixed suffix list, not a regex/glob DSL —
// see plan.md's explicit scope guard against introducing a rules DSL for
// this predicate.
var testFileSuffixes = []string{
	"_test.go",
	".test.ts",
	".test.tsx",
	".spec.ts",
	".spec.tsx",
}

// isTestFile reports whether path ends in one of testFileSuffixes.
func isTestFile(path string) bool {
	for _, suffix := range testFileSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

// IsTestOnlyReworkCycle reports whether every file touched across the last
// TestOnlyReworkMinAttempts rework attempts (most recent first — one
// []string of changed file paths per attempt) is a test/spec file. A
// legitimate fix touches production code somewhere; a run of test-only
// diffs is suggestive of chasing a non-deterministic failure by editing the
// test rather than the underlying code. False on fewer than
// TestOnlyReworkMinAttempts attempts, on any attempt with no file data at
// all (no signal, don't guess), or on any attempt touching a non-test file.
//
// Known false-positive source (documented, not filtered — purely
// informational, never gates the reopen decision, so this rate is accepted
// rather than suppressed here; see validation.md): a legitimate
// test-coverage-improvement item also produces test-only rework cycles by
// design.
func IsTestOnlyReworkCycle(fileListsByAttempt [][]string) bool {
	if len(fileListsByAttempt) < TestOnlyReworkMinAttempts {
		return false
	}
	for _, files := range fileListsByAttempt[:TestOnlyReworkMinAttempts] {
		if len(files) == 0 {
			return false
		}
		for _, f := range files {
			if !isTestFile(f) {
				return false
			}
		}
	}
	return true
}

// MostRecentCompletedWorkSession returns the most recent completed
// (EndedAt != nil) work-role ItemSession from sessions, or nil if none
// exists. sessions must be ordered oldest-first, as Storage.ListItemSessions
// returns. Exported: called from server/services at review-verdict-save time
// to resolve which work session's diff a given verdict is reviewing (feeds
// the DiffHash computation IsFlakyVerdictFlipFlop consumes), and by the
// stuck-item reconciler to build IsTestOnlyReworkCycle's per-attempt file
// lists. Only considers completed sessions — reading a still-in-progress
// session's commit range risks racing an in-flight write (see validation.md).
func MostRecentCompletedWorkSession(sessions []ItemSessionSummary) *ItemSessionSummary {
	for i := len(sessions) - 1; i >= 0; i-- {
		if sessions[i].Role == SessionRoleWork && sessions[i].EndedAt != nil {
			return &sessions[i]
		}
	}
	return nil
}

// prReadyToMergeSolo is the solo-operator PR readiness predicate (ADR-001
// "Single-user readiness"). It applies every blocking-exclusion
// github.DerivePRPriority uses (draft, changes-requested, CI-failure, not
// mergeable, terminal state) but deliberately DROPS the ApprovedCount > 0
// gate DerivePRPriority requires (github/priority.go:50) — that gate is
// unreachable on a single-user repo (Tyler cannot self-approve his own PR),
// so PRPriorityReady never fires there and the flagship motivating case (PR
// #148) would never surface. Do NOT replace this with
// DerivePRPriority(info)==PRPriorityReady — see pre-mortem F1.
func prReadyToMergeSolo(info *github.PRInfo) bool {
	if info == nil {
		return false
	}
	state := strings.ToLower(info.State)
	if state == "merged" || state == "closed" {
		return false
	}
	if info.IsDraft {
		return false
	}
	if info.ChangesRequestedCount > 0 {
		return false
	}
	switch info.CheckConclusion {
	case "success", "":
		// green or no CI configured — proceed.
	default:
		return false
	}
	return strings.EqualFold(info.Mergeable, "MERGEABLE")
}
