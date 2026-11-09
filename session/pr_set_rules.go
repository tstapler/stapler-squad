package session

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/github"
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

// Live-work timing, owner-confirmed. The idle window is how long an unended
// session that stopped its turn still counts as live; the other three are hard
// ceilings so a wrongly-live session can never hold a worktree, pane or item forever.
const (
	LiveWorkIdleWindow   = 10 * time.Minute
	LiveWorkMaxHold      = time.Hour // merged-PR hold before the item may go done
	liveWorkWorkingCap   = time.Hour // Working && !TurnEnded with no change for this long is not live
	liveWorkErrorHoldCap = time.Hour // a probe failing this long stops counting as live
)

// LiveWorkFacts are the Snapshot() facts IsLiveWork needs. Working is the
// caller's "non-idle status" reading. TurnEnded is true once the agent recorded
// a turn end and nothing meaningful has changed since: Instance.TurnEnded.At is
// set and LastMeaningfulChangeAt is not after it (see LiveWorkFactsFromSnapshot).
type LiveWorkFacts struct {
	Unended                bool
	Working                bool
	TurnEnded              bool
	LastMeaningfulChangeAt time.Time
}

// IsLiveWork reports whether a work session is still active enough to hold an
// item in pr_pending: unended, and either mid-turn or changed within window.
// A zero LastMeaningfulChangeAt never counts as recent, and cannot cap a
// mid-turn session (callers substitute the creation time when they have it).
func IsLiveWork(facts LiveWorkFacts, now time.Time, window time.Duration) bool {
	live, _ := evaluateLiveWork(facts, now, window)
	return live
}

// evaluateLiveWork is IsLiveWork plus whether the working-cap decided it: a
// session that is Working and mid-turn but has had no meaningful change for
// liveWorkWorkingCap is reported not live (a hung status must not hold forever).
func evaluateLiveWork(facts LiveWorkFacts, now time.Time, window time.Duration) (live, capped bool) {
	if !facts.Unended {
		return false, false
	}
	if facts.Working && !facts.TurnEnded {
		if !facts.LastMeaningfulChangeAt.IsZero() && now.Sub(facts.LastMeaningfulChangeAt) >= liveWorkWorkingCap {
			return false, true
		}
		return true, false
	}
	if facts.LastMeaningfulChangeAt.IsZero() {
		return false, false
	}
	return now.Sub(facts.LastMeaningfulChangeAt) < window, false
}

// LiveWorkProbe is the consumer-side seam for the live-work hold. A nil probe
// means no hold; the real implementation reads the live instance Snapshot().
type LiveWorkProbe interface {
	IsLive(ctx context.Context, sessionUUID string, now time.Time) (bool, error)
}

// LiveWorkHoldExpired reports that the hold has lasted maxHold since holdStart
// (LiveWorkHoldStart). A zero holdStart never expires, so callers always pass a
// first-observation fallback.
func LiveWorkHoldExpired(now, holdStart time.Time, maxHold time.Duration) bool {
	if holdStart.IsZero() {
		return false
	}
	return now.Sub(holdStart) >= maxHold
}

// LiveWorkHoldStart is when the live-work hold began: the FIRST merge among the
// item's merged PRs (not the latest, so a chain of follow-up PRs cannot extend
// it). A merged row without merged_at uses last_polled_at (terminal rows are no
// longer polled, so it stays put); with neither, firstObserved, the first time the
// hold was seen, so a missing timestamp can never hold forever.
func LiveWorkHoldStart(prs []PRView, firstObserved time.Time) time.Time {
	var first time.Time
	for _, v := range distinctPRs(prs) {
		if v.State != PRStateMerged {
			continue
		}
		at := v.MergedAt
		if at.IsZero() {
			at = v.LastPolledAt
		}
		if !at.IsZero() && (first.IsZero() || at.Before(first)) {
			first = at
		}
	}
	if first.IsZero() {
		return firstObserved
	}
	return first
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

// LegacyPRMirror is the value the legacy scalar PR columns carry for a session
// that has links: its ActivePR, and whether every linked PR is terminal.
type LegacyPRMirror struct {
	Number   int
	URL      string
	Terminal bool
}

// ProjectLegacyPRMirror derives the legacy scalars from a non-empty link set.
// It reads no configuration, so the result is identical under every flag scope.
func ProjectLegacyPRMirror(prs []PRView, currentBranch string) (LegacyPRMirror, bool) {
	active := ActivePR(prs, currentBranch)
	if active == nil {
		return LegacyPRMirror{}, false
	}
	url := active.URL
	if url == "" {
		ref := github.ParsedGitHubRef{
			Type: github.RefTypePR, Host: active.Ref.Host(),
			Owner: active.Ref.Owner(), Repo: active.Ref.Repo(), PRNumber: active.Ref.Number(),
		}
		url = ref.HTMLURL()
	}
	return LegacyPRMirror{Number: active.Ref.Number(), URL: url, Terminal: PRsAllTerminal(prs, 0)}, true
}

// OpenPROnBranch returns the newest non-terminal PR whose head branch is branch.
// Create-PR short-circuits only on such a PR, so a session whose PRs are all
// terminal (or sit on other branches) may open another. An empty branch matches
// nothing: the caller then creates, and GitHub itself rejects a duplicate.
func OpenPROnBranch(prs []PRView, branch string) (PRView, bool) {
	if branch == "" {
		return PRView{}, false
	}
	best := newestPR(prs, func(v PRView) bool { return !v.Terminal() && v.HeadBranch == branch })
	if best == nil {
		return PRView{}, false
	}
	return *best, true
}

// WebURL is the stored URL, or the canonical github.com-style URL built from the ref.
func (v PRView) WebURL() string {
	if v.URL != "" {
		return v.URL
	}
	ref := github.ParsedGitHubRef{
		Type: github.RefTypePR, Host: v.Ref.Host(), Owner: v.Ref.Owner(), Repo: v.Ref.Repo(), PRNumber: v.Ref.Number(),
	}
	return ref.HTMLURL()
}

// ExistingPRForCreate is what create-PR treats as "this branch already has a PR".
// With links it is OpenPROnBranch; a session with no links keeps the legacy
// single-PR scalar check, so flag-off behavior is unchanged.
func ExistingPRForCreate(snap *InstanceSnapshot, branch string) (url string, number int, ok bool) {
	g := snap.GitHub
	if len(g.PRs) > 0 {
		v, found := OpenPROnBranch(g.PRs, branch)
		return v.WebURL(), v.Ref.Number(), found
	}
	return g.GitHubPRURL, g.GitHubPRNumber, g.GitHubPRURL != ""
}

// ActiveCheckConclusion is the CI conclusion of the session's active PR. The
// GitHubCheckConclusion scalar is not mirrored per PR, so with several PRs it can
// describe a merged one; it is used only when the session has no links.
func (s *InstanceSnapshot) ActiveCheckConclusion() string {
	if active := ActivePR(s.GitHub.PRs, s.Branch); active != nil {
		return active.CheckConclusion
	}
	return s.GitHub.GitHubCheckConclusion
}
