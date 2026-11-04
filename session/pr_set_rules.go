package session

import (
	"context"
	"time"
)

// newerPR orders by AttachedAt, then PR number, then ref text, so ActivePR is
// deterministic even when a backfill stamps every link in the same instant.
func newerPR(a, b PRView) bool {
	if !a.AttachedAt.Equal(b.AttachedAt) {
		return a.AttachedAt.After(b.AttachedAt)
	}
	if a.Ref.Number() != b.Ref.Number() {
		return a.Ref.Number() > b.Ref.Number()
	}
	return a.Ref.String() > b.Ref.String()
}

// newestPR returns a copy of the newest view satisfying keep, or nil.
func newestPR(prs []PRView, keep func(PRView) bool) *PRView {
	var best *PRView
	for i := range prs {
		if !keep(prs[i]) {
			continue
		}
		if best == nil || newerPR(prs[i], *best) {
			best = &prs[i]
		}
	}
	if best == nil {
		return nil
	}
	cp := *best
	return &cp
}

// ActivePR picks the PR a session's actions and legacy mirror should target:
// the pinned PR; else the open PR whose head is currentBranch; else the newest
// open PR; else the newest non-terminal unknown; else the newest effectively
// terminal PR; else nil. Ranking uses Terminal(), never the state string alone,
// and "newest" means the latest AttachedAt.
func ActivePR(prs []PRView, currentBranch string) *PRView {
	isOpen := func(v PRView) bool { return v.State == PRStateOpen && !v.Terminal() }
	steps := []func(PRView) bool{
		func(v PRView) bool { return v.Pinned },
		func(v PRView) bool { return currentBranch != "" && isOpen(v) && v.HeadBranch == currentBranch },
		isOpen,
		func(v PRView) bool { return v.State == PRStateUnknown && !v.Terminal() },
		PRView.Terminal,
	}
	for _, keep := range steps {
		if pr := newestPR(prs, keep); pr != nil {
			return pr
		}
	}
	return nil
}

// ItemVerdict is the done-rule outcome over an item's PR set.
type ItemVerdict string

const (
	ItemVerdictNoPRs             ItemVerdict = "no_prs"
	ItemVerdictOpen              ItemVerdict = "open"
	ItemVerdictDone              ItemVerdict = "done"
	ItemVerdictAllClosedUnmerged ItemVerdict = "all_closed_unmerged"
)

// distinctPRs drops duplicate views of one PullRequestState (defensive: the
// schema keeps one state row per PRRef). Views without a state id fall back to
// the ref.
func distinctPRs(prs []PRView) []PRView {
	type key struct {
		id  int
		ref PRRef
	}
	seen := make(map[key]struct{}, len(prs))
	out := make([]PRView, 0, len(prs))
	for _, v := range prs {
		k := key{id: v.StateID}
		if v.StateID == 0 {
			k.ref = v.Ref
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, v)
	}
	return out
}

// ItemPRVerdict implements the done rule over distinct PRs: any open or unknown
// PR (including every unverified-terminal row, exhausted or not) holds the item
// open; otherwise one merged PR is Done and only closed PRs are AllClosedUnmerged.
func ItemPRVerdict(prs []PRView) ItemVerdict {
	prs = distinctPRs(prs)
	if len(prs) == 0 {
		return ItemVerdictNoPRs
	}
	merged := false
	for _, v := range prs {
		switch v.State {
		case PRStateMerged:
			merged = true
		case PRStateClosed:
		default: // open, unknown, or anything unrecognized: fail closed
			return ItemVerdictOpen
		}
	}
	if merged {
		return ItemVerdictDone
	}
	return ItemVerdictAllClosedUnmerged
}

// PRsAllTerminal reports whether every linked PR is effectively terminal. An
// empty set is never terminal whatever the legacy number says (a legacy PR with
// no link must not read as safe); callers that accept "no PRs at all" ask
// HasNoPRs instead. The legacy number is part of the signature so call sites
// pass it explicitly, but only the empty-set rule exists today.
func PRsAllTerminal(prs []PRView, _ int) bool {
	prs = distinctPRs(prs)
	if len(prs) == 0 {
		return false
	}
	for _, v := range prs {
		if !v.Terminal() {
			return false
		}
	}
	return true
}

// HasNoPRs is the explicit empty-set path: no links and no legacy PR number.
func HasNoPRs(prs []PRView, legacyNumber int) bool {
	return len(prs) == 0 && legacyNumber == 0
}

// LiveWorkFacts are the Snapshot() facts IsLiveWork needs. Working is the
// caller's "non-idle status" reading; TurnEnded is true once the agent stopped
// its turn and is waiting.
type LiveWorkFacts struct {
	Unended                bool
	Working                bool
	TurnEnded              bool
	LastMeaningfulChangeAt time.Time
}

// IsLiveWork reports whether a work session is still active enough to hold an
// item in pr_pending: unended, and either mid-turn or changed within window.
// A zero LastMeaningfulChangeAt never counts as recent.
func IsLiveWork(facts LiveWorkFacts, now time.Time, window time.Duration) bool {
	if !facts.Unended {
		return false
	}
	if facts.Working && !facts.TurnEnded {
		return true
	}
	if facts.LastMeaningfulChangeAt.IsZero() {
		return false
	}
	return now.Sub(facts.LastMeaningfulChangeAt) < window
}

// LiveWorkProbe is the consumer-side seam for the live-work hold. A nil probe
// means no hold; the real implementation reads the live instance Snapshot().
type LiveWorkProbe interface {
	IsLive(ctx context.Context, sessionUUID string, now time.Time) (bool, error)
}

// LiveWorkHoldExpired reports that the hold has lasted maxHold since the latest
// merge. A zero latestMergedAt never expires.
func LiveWorkHoldExpired(now, latestMergedAt time.Time, maxHold time.Duration) bool {
	if latestMergedAt.IsZero() {
		return false
	}
	return now.Sub(latestMergedAt) >= maxHold
}

// AssignRole picks the display role for a PR about to be attached to a session
// that already holds existing links: primary when none of them is primary;
// otherwise stacked when baseBranch is another attached PR's head branch, else
// followup. How the PR arrived (attach source) does not change the role.
func AssignRole(existing []PRView, baseBranch string) PRRole {
	hasPrimary := false
	for _, v := range existing {
		hasPrimary = hasPrimary || v.Role == PRRolePrimary
	}
	if !hasPrimary {
		return PRRolePrimary
	}
	if baseBranch != "" {
		for _, v := range existing {
			if v.HeadBranch == baseBranch {
				return PRRoleStacked
			}
		}
	}
	return PRRoleFollowup
}
