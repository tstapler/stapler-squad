package prwatch

// Every reason, class and title string lives here so the operator-doc test
// can derive its expected lists from one place. Add new ones here only.

// DropReason is why an event was acked without a steer.
type DropReason string

const (
	DropApprovedReview     DropReason = "approved_review"
	DropPullRequestEvent   DropReason = "pull_request_event"
	DropEditedOrDeleted    DropReason = "edited_or_deleted"
	DropCINotFailure       DropReason = "ci_not_failure"
	DropCICheckRun         DropReason = "ci_check_run"
	DropSelfActor          DropReason = "self_actor"
	DropBotActor           DropReason = "bot_actor"
	DropHumanFeedbackOff   DropReason = "human_feedback_off"
	DropActorNotAllowlist  DropReason = "actor_not_allowlisted"
	DropUnknownKind        DropReason = "unknown_kind"
	DropRepoMismatch       DropReason = "repo_mismatch"
	DropHiddenSession      DropReason = "hidden_session"
	DropSessionNotLive     DropReason = "session_not_live"
	DropHeldAgeCeiling     DropReason = "held_age_ceiling"
	DropCoveredSameHeadSHA DropReason = "covered_same_head_sha"
	DropFoldedOverCap      DropReason = "folded_over_cap"
)

// AllDropReasons lists every DropReason (D7).
func AllDropReasons() []DropReason {
	return []DropReason{
		DropApprovedReview, DropPullRequestEvent, DropEditedOrDeleted, DropCINotFailure,
		DropCICheckRun, DropSelfActor, DropBotActor, DropHumanFeedbackOff,
		DropActorNotAllowlist, DropUnknownKind, DropRepoMismatch, DropHiddenSession,
		DropSessionNotLive, DropHeldAgeCeiling, DropCoveredSameHeadSHA, DropFoldedOverCap,
	}
}

// HoldReason is why an event was left unacked for a later tick. Holds are
// not failures. The steer-outcome holds reuse the outcome words (D5).
type HoldReason string

const (
	HoldWindowBatching    HoldReason = "window_batching"
	HoldLoginUnresolved   HoldReason = "login_unresolved"
	HoldBusy              HoldReason = "busy"
	HoldGuardBusy         HoldReason = "guard_busy"
	HoldCoolingDown       HoldReason = "cooling_down"
	HoldNoStatusSource    HoldReason = "no_status_source"
	HoldUnspecified       HoldReason = "unspecified"
	HoldNotTracked        HoldReason = "not_tracked"
	HoldSteerFailed       HoldReason = "failed"
	HoldSessionSuspended  HoldReason = "session_suspended"
	HoldCIDuplicateNewSHA HoldReason = "ci_duplicate_new_sha"
)

// AllHoldReasons lists every HoldReason (D5).
func AllHoldReasons() []HoldReason {
	return []HoldReason{
		HoldWindowBatching, HoldLoginUnresolved, HoldBusy, HoldGuardBusy, HoldCoolingDown,
		HoldNoStatusSource, HoldUnspecified, HoldNotTracked, HoldSteerFailed,
		HoldSessionSuspended, HoldCIDuplicateNewSHA,
	}
}

// SkipReason is why a session is not watched (D8).
type SkipReason string

const (
	SkipNoPR                SkipReason = "no_pr"
	SkipNoOwnerRepo         SkipReason = "no_owner_repo"
	SkipGHEHost             SkipReason = "ghe_host"
	SkipFork                SkipReason = "fork"
	SkipBacklogPRPending    SkipReason = "backlog_pr_pending"
	SkipBacklogLookupFailed SkipReason = "backlog_lookup_failed"
	SkipIgnoreRepos         SkipReason = "ignore_repos"
	SkipHidden              SkipReason = "hidden"
	SkipNotLive             SkipReason = "not_live"
	SkipPRTerminal          SkipReason = "pr_terminal"
)

// AllSkipReasons lists every SkipReason (D8).
func AllSkipReasons() []SkipReason {
	return []SkipReason{
		SkipNoPR, SkipNoOwnerRepo, SkipGHEHost, SkipFork, SkipBacklogPRPending,
		SkipBacklogLookupFailed, SkipIgnoreRepos, SkipHidden, SkipNotLive, SkipPRTerminal,
	}
}

// ActorClass is the only form in which an actor appears in logs, counters,
// status and the ring buffer (D7). Precedence: self, bot, allowlisted, other.
type ActorClass string

const (
	ActorSelf        ActorClass = "self"
	ActorBot         ActorClass = "bot"
	ActorAllowlisted ActorClass = "allowlisted"
	ActorOther       ActorClass = "other"
)

// AllActorClasses lists every ActorClass.
func AllActorClasses() []ActorClass {
	return []ActorClass{ActorSelf, ActorBot, ActorAllowlisted, ActorOther}
}

// Notification titles (D15). <owner>/<repo> is substituted by the builder.
const (
	TitleCoverageGap       = "PR watch: no webhook coverage for <owner>/<repo>"
	TitleSteerFailed       = "PR watch: could not steer a session"
	TitleHeldLong          = "PR watch: an event is waiting for a session"
	TitleHeldDropped       = "PR watch: dropped an event that could not be delivered"
	TitleLoginUnresolved   = "PR watch: GitHub login unresolved"
	TitleStrangerActivity  = "PR watch: comments from unfamiliar accounts on <owner>/<repo>"
	TitleInboxUnreachable  = "PR watch: gh-signal inbox is not reachable"
	TitleInboxUnauthorized = "PR watch: gh-signal inbox rejected Squad's token"
	TitleTokenMissing      = "PR watch: inbox token file is missing"
	TitleTokenPermissions  = "PR watch: inbox token file is too open"
	TitleNonLoopbackURL    = "PR watch: inbox URL is not a loopback address"
	TitleLeaderNotHeld     = "PR watch: idle in this Squad process"
	TitleRecovered         = "PR watch: recovered"
)

// AllNotificationTitles lists every title above.
func AllNotificationTitles() []string {
	return []string{
		TitleCoverageGap, TitleSteerFailed, TitleHeldLong, TitleHeldDropped,
		TitleLoginUnresolved, TitleStrangerActivity, TitleInboxUnreachable,
		TitleInboxUnauthorized, TitleTokenMissing, TitleTokenPermissions,
		TitleNonLoopbackURL, TitleLeaderNotHeld, TitleRecovered,
	}
}
