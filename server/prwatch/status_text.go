package prwatch

import (
	"fmt"
	"strings"
	"time"
)

const statusTextReasonMax = 60

// NotRunningText is the answer when no status provider exists: the feature
// flag is off at startup or the module was never constructed.
func NotRunningText() string {
	return strings.Join([]string{
		"PR watch status",
		"Feature flag: off",
		"PR watch is not running in this process",
		"To turn it on, set feature_flags.inbox_pr_watch to true in the Squad config and restart Squad.",
		"See docs/how-to/inbox-pr-watch.md.",
		"",
	}, "\n")
}

// RenderText renders st as the D14 plain-text layout: ASCII only, one fact
// per line, meaning carried by words. now anchors relative ages and hostname
// names the consumer in the fix command. Codes stay in the JSON form.
func RenderText(st Status, now time.Time, hostname string) string {
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	p("PR watch status")
	p("Feature flag: %s", onOff(st.Enabled))
	p("This process polls: %s", yesNo(st.Polls))
	p("Inbox: %s", inboxPhrase(st))
	p("GitHub login: %s", map[bool]string{true: "resolved", false: "not resolved yet"}[st.LoginResolved])
	if st.FirstRunLine != "" {
		p("%s", st.FirstRunLine)
	}
	p("")

	p("Settings")
	p("  Comment and review steers: %s", modePhrase(st.Settings))
	if st.Settings.CIOnly {
		p("  CI-only mode is in effect: comments and reviews do not steer")
	}
	p("  Poll interval: %s; minimum time between comment steers: %s", st.Settings.PollInterval, st.Settings.MinSteerInterval)
	if len(st.Settings.IgnoreRepos) > 0 {
		p("  Ignored repositories: %s", strings.Join(st.Settings.IgnoreRepos, ", "))
	}
	for _, c := range st.Settings.Corrections {
		p("  Corrected: %s %s was replaced by %s", c.Key, c.Bad, c.Effective)
	}
	p("")

	p("Repositories")
	if len(st.Repos) == 0 {
		p("  none")
	}
	for _, r := range st.Repos {
		writeRepo(&b, st, r, hostname)
	}
	p("")

	p("Sessions")
	if len(st.Watched)+len(st.Skipped) == 0 {
		p("  none")
	}
	for _, w := range st.Watched {
		writeWatched(&b, st, w, now)
	}
	for _, s := range st.Skipped {
		p("  session %s %q   (not watched: %s)", s.ShortUUID, s.Title, skipPhrase(s.Reason))
	}
	p("")

	p("Waiting events")
	if len(st.Held) == 0 {
		p("  none")
	}
	for _, h := range st.Held {
		p("  %s  %s#%d  %s  waiting %s: %s", h.ShortUUID, h.Repo, h.PR, h.Kind.Rendered(),
			ageWords(h.AgeSeconds), heldPhrase(h))
	}
	p("")

	p("Recent events (newest first, up to 200, cleared on restart)")
	if len(st.Recent) == 0 {
		p("  none")
	}
	notSteered := 0
	for i := len(st.Recent) - 1; i >= 0; i-- {
		d := st.Recent[i]
		if d.Disposition == DispositionDropped {
			notSteered++
		}
		p("  %s  %-10s %s#%d  %s", clock(d.At), d.Kind.Rendered(), d.Repo, d.PR, dispositionPhrase(d))
	}
	p("")

	by := st.Counts.SteersByClass
	p("Steers by who triggered them (since start): you %d, bot %d, allowlisted %d, other %d, CI failure %d",
		by[string(ActorSelf)], by[string(ActorBot)], by[string(ActorAllowlisted)], by[string(ActorOther)], by[steerClassCI])
	p("Counts since start: steered %d, waiting %d, not steered %d", st.Counts.Steers, len(st.Held), notSteered)
	return b.String()
}

func writeRepo(b *strings.Builder, st Status, r RepoStatus, hostname string) {
	switch r.Coverage {
	case CoverageStateCovered:
		fmt.Fprintf(b, "  %s   webhook coverage: covered\n", r.Repo)
	case CoverageStateUnavailable:
		reason := r.Reason
		if reason == "" {
			reason = "no reason given"
		}
		reason = sanitizeStatusText(reason, statusTextReasonMax)
		fmt.Fprintf(b, "  %s   webhook coverage: NOT covered (gh-signal said: %s)\n", r.Repo, reason)
		pr := firstPR(st, r.Repo)
		if pr > 0 {
			fmt.Fprintf(b, "      Run: gh-signal inbox watch %s %d --consumer squad:%s\n", r.Repo, pr, hostname)
		}
		fmt.Fprintf(b, "      Or add %s to inbox_pr_watch.ignore_repos\n", r.Repo)
	default:
		fmt.Fprintf(b, "  %s   webhook coverage: unknown (not yet confirmed; events may not arrive)\n", r.Repo)
	}
}

func firstPR(st Status, repo string) int {
	for _, w := range st.Watched {
		if w.Repo == repo {
			return w.PR
		}
	}
	return 0
}

func writeWatched(b *strings.Builder, st Status, w WatchedStatus, now time.Time) {
	state := "watching"
	if w.State == StateSuspended {
		state = "watching (session paused)"
	}
	fmt.Fprintf(b, "  session %s %q   %s#%d   %s\n", w.ShortUUID, w.Title, w.Repo, w.PR, state)
	fmt.Fprintf(b, "    subscribed: %s; webhook coverage: %s\n", yesNo(w.Subscribed), coverageWord(w.Coverage))
	if w.LastEventAt != nil {
		fmt.Fprintf(b, "    last event: %s\n", clock(*w.LastEventAt))
	}
	if ls := st.LastSteer; ls != nil && ls.ShortUUID == w.ShortUUID {
		fmt.Fprintf(b, "    last steer: %s delivered\n", clock(ls.At))
	}
	switch w.NextSteer {
	case NextSteerAt:
		if w.NextSteerAt != nil {
			fmt.Fprintf(b, "    next steer for comments or reviews: %s\n", clock(*w.NextSteerAt))
		}
	case NextSteerWaiting:
		fmt.Fprintf(b, "    next steer for comments or reviews: when the waiting event is released (see Waiting events)\n")
	default:
		fmt.Fprintf(b, "    next steer for comments or reviews: now\n")
	}
	fmt.Fprintf(b, "    CI failure on a new commit: steers immediately\n")
}

func coverageWord(c CoverageState) string {
	switch c {
	case CoverageStateCovered:
		return "covered"
	case CoverageStateUnavailable:
		return "NOT covered"
	}
	return "unknown"
}

func inboxPhrase(st Status) string {
	at := ""
	if st.InboxErrorAt != nil {
		at = " since " + clock(*st.InboxErrorAt)
	}
	switch st.InboxState {
	case InboxStateOK:
		return "reachable"
	case InboxStateUnreachable:
		return "not reachable" + at + " (is gh-signal-inbox running?)"
	case InboxStateUnauthorized:
		return "rejected Squad's token" + at
	case InboxStateTokenMissing:
		return "token file is missing" + at
	case InboxStateTokenPerms:
		return "token file is too open; it must be mode 0600" + at
	case InboxStateNonLoopback:
		return "the configured URL is not a loopback address" + at
	case InboxStateLeaderNotHeld:
		return "idle: another Squad process is polling"
	}
	return "state not reported"
}

func modePhrase(s SettingsView) string {
	switch {
	case s.HumanFeedbackMode == ModeAll:
		return "all accounts except you (human_feedback_mode all)"
	case s.HumanFeedbackMode == ModeAllowlist && !s.CIOnly:
		return fmt.Sprintf("only allowlisted accounts, %d listed (human_feedback_mode allowlist)",
			s.ActorAllowlistCount+s.BotAllowlistCount)
	case s.HumanFeedbackMode == ModeAllowlist:
		return "no account, because the allowlist is empty (human_feedback_mode allowlist)"
	}
	return "no account (human_feedback_mode off)"
}

func skipPhrase(r SkipReason) string {
	switch r {
	case SkipNoPR:
		return "no pull request is linked yet"
	case SkipNoOwnerRepo:
		return "its GitHub owner and repository are unknown"
	case SkipGHEHost:
		return "it uses a GitHub host other than github.com"
	case SkipFork:
		return "its pull request comes from a fork"
	case SkipBacklogPRPending:
		return "its backlog item is waiting on this PR (the backlog path handles it)"
	case SkipBacklogLookupFailed:
		return "its backlog item could not be looked up"
	case SkipIgnoreRepos:
		return "repository is in ignore_repos"
	case SkipHidden:
		return "the session is hidden"
	case SkipNotLive:
		return "the session is not running"
	}
	return "reason not reported"
}

func heldPhrase(h HeldStatus) string {
	switch h.Reason {
	case HoldWindowBatching:
		if h.WindowClosesAt != nil {
			return "batching with an earlier steer; next steer at " + clockShort(*h.WindowClosesAt)
		}
		return "batching with an earlier steer in the same window"
	case HoldLoginUnresolved:
		return "your GitHub login is not resolved yet, so your own comments cannot be told apart"
	case HoldBusy:
		return "the session is busy"
	case HoldGuardBusy, HoldCoolingDown:
		return "another steer was just sent to this session; retrying shortly"
	case HoldNoStatusSource, HoldUnspecified, HoldNotTracked:
		return "the session is not ready to receive a steer"
	case HoldSteerFailed:
		return "the last attempt to steer failed; retrying"
	case HoldSessionSuspended:
		return "the session is paused; it is steered when it resumes"
	case HoldCIDuplicateNewSHA:
		return "a CI failure on a new commit is being checked against an earlier steer"
	}
	return "held for a later attempt"
}

func dispositionPhrase(d Disposition) string {
	switch d.Disposition {
	case DispositionSteered:
		return "steered"
	case DispositionHeld:
		return "waiting: " + heldPhrase(HeldStatus{Reason: HoldReason(d.Reason)})
	}
	return "not steered: " + dropPhrase(DropReason(d.Reason))
}

func dropPhrase(r DropReason) string {
	switch r {
	case DropApprovedReview:
		return "approvals do not steer"
	case DropPullRequestEvent:
		return "pull request activity other than feedback"
	case DropEditedOrDeleted:
		return "the comment was edited or deleted"
	case DropCINotFailure:
		return "the check did not fail"
	case DropCICheckRun:
		return "individual check runs do not steer"
	case DropSelfActor:
		return "you wrote it (own comment)"
	case DropBotActor:
		return "a bot wrote it"
	case DropHumanFeedbackOff:
		return "comments and reviews are turned off"
	case DropActorNotAllowlist:
		return "its author is not on the allowlist"
	case DropUnknownKind:
		return "an event type Squad does not handle"
	case DropRepoMismatch:
		return "it belongs to a different repository"
	case DropHiddenSession:
		return "the session is hidden"
	case DropSessionNotLive:
		return "the session is not running"
	case DropHeldAgeCeiling:
		return "it waited too long to be useful"
	case DropCoveredSameHeadSHA:
		return "same commit as an earlier CI steer"
	case DropFoldedOverCap:
		return "too many events were waiting; this one was folded into a summary"
	}
	return "reason not reported"
}

func ageWords(sec int64) string {
	switch {
	case sec < 60:
		return fmt.Sprintf("%d %s", sec, plural(int(sec), "second", "seconds"))
	case sec < 3600:
		m := int(sec / 60)
		return fmt.Sprintf("%d %s", m, plural(m, "minute", "minutes"))
	}
	h := int(sec / 3600)
	return fmt.Sprintf("%d %s", h, plural(h, "hour", "hours"))
}

func clock(t time.Time) string      { return t.Format("15:04:05") }
func clockShort(t time.Time) string { return t.Format("15:04") }

func onOff(b bool) string { return map[bool]string{true: "on", false: "off"}[b] }
func yesNo(b bool) string { return map[bool]string{true: "yes", false: "no"}[b] }
