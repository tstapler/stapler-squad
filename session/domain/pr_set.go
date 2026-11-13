package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// PRRef identifies one pull request: canonical host, owner and repo plus the number.
// The fields are unexported so a literal cannot be built outside this package; use
// NewPRRef, which lowercases host, owner and repo (GitHub treats all three as
// case-insensitive, mirroring github.RepoRef's link keys).
type PRRef struct {
	host   string
	owner  string
	repo   string
	number int
}

// NewPRRef builds the canonical PRRef. An empty host means github.com.
func NewPRRef(host, owner, repo string, number int) (PRRef, error) {
	owner = strings.ToLower(strings.TrimSpace(owner))
	repo = strings.ToLower(strings.TrimSpace(repo))
	if owner == "" {
		return PRRef{}, errors.New("pr ref: owner must not be empty")
	}
	if repo == "" {
		return PRRef{}, errors.New("pr ref: repo must not be empty")
	}
	if number <= 0 {
		return PRRef{}, fmt.Errorf("pr ref: number must be positive, got %d", number)
	}
	return PRRef{host: NormalizeHost(host), owner: owner, repo: repo, number: number}, nil
}

func (r PRRef) Host() string { return r.host }

func (r PRRef) Owner() string { return r.owner }

func (r PRRef) Repo() string { return r.repo }

func (r PRRef) Number() int { return r.number }

// IsZero reports whether r is the unusable zero value.
func (r PRRef) IsZero() bool { return r == PRRef{} }

// String renders host/owner/repo#number ("" for the zero value).
func (r PRRef) String() string {
	if r.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s#%d", r.host, r.owner, r.repo, r.number)
}

// MarshalText makes encoding/json print the ref instead of dropping the unexported fields.
func (r PRRef) MarshalText() ([]byte, error) { return []byte(r.String()), nil }

// PRState is the lifecycle state of one pull request.
type PRState string

const (
	PRStateOpen    PRState = "open"
	PRStateClosed  PRState = "closed"
	PRStateMerged  PRState = "merged"
	PRStateUnknown PRState = "unknown" // never observed or unreachable; not terminal, not open
)

// ParsePRState rejects any string that is not one of the four states.
func ParsePRState(s string) (PRState, error) {
	switch st := PRState(s); st {
	case PRStateOpen, PRStateClosed, PRStateMerged, PRStateUnknown:
		return st, nil
	}
	return PRStateUnknown, fmt.Errorf("unrecognized PR state %q", s)
}

// Terminal reports whether the state is closed or merged. Unknown is not terminal.
func (s PRState) Terminal() bool { return s == PRStateClosed || s == PRStateMerged }

// PRRole is display-only metadata for a link.
type PRRole string

const (
	PRRolePrimary  PRRole = "primary"
	PRRoleFollowup PRRole = "followup"
	PRRoleStacked  PRRole = "stacked"
	PRRoleUnknown  PRRole = "unknown" // only for an unparseable stored value
)

// ParsePRRole returns PRRoleUnknown with an error for an unrecognized string.
func ParsePRRole(s string) (PRRole, error) {
	switch r := PRRole(s); r {
	case PRRolePrimary, PRRoleFollowup, PRRoleStacked, PRRoleUnknown:
		return r, nil
	}
	return PRRoleUnknown, fmt.Errorf("unrecognized PR role %q", s)
}

// AttachSource records how a link came to exist.
type AttachSource string

const (
	AttachSourceCreatedFromURL    AttachSource = "created_from_url"
	AttachSourcePushScan          AttachSource = "push_scan"
	AttachSourceBranchDiscovery   AttachSource = "branch_discovery"
	AttachSourcePRCreationService AttachSource = "pr_creation_service"
	AttachSourceReportPRCreated   AttachSource = "report_pr_created"
	AttachSourceJules             AttachSource = "jules"
	AttachSourceBackfill          AttachSource = "backfill"
	AttachSourceManual            AttachSource = "manual"
)

// ParseAttachSource rejects any string outside the eight sources.
func ParseAttachSource(s string) (AttachSource, error) {
	switch a := AttachSource(s); a {
	case AttachSourceCreatedFromURL, AttachSourcePushScan, AttachSourceBranchDiscovery,
		AttachSourcePRCreationService, AttachSourceReportPRCreated, AttachSourceJules,
		AttachSourceBackfill, AttachSourceManual:
		return a, nil
	}
	return "", fmt.Errorf("unrecognized attach source %q", s)
}

// Verified reports whether the source proves the PR exists and belongs to the owner.
func (a AttachSource) Verified() bool {
	return a == AttachSourceManual || a == AttachSourceReportPRCreated
}

// Unverified-terminal exhaustion: a seeded row becomes effectively terminal only
// after this many counted poll failures spanning at least verifyExhaustedSpan.
const (
	VerifyExhaustedFailures = 5
	VerifyExhaustedSpan     = 30 * time.Minute
)

// PRView is an immutable copy of one link joined to its PullRequestState row.
// Zero times mean "unset". Copy it by value; never share a pointer across goroutines.
type PRView struct {
	StateID int // PullRequestState row id; identical for every link to one PR
	Ref     PRRef

	State          PRState
	StoredTerminal bool // the persisted `terminal` column (see Terminal for the effective value)
	URL            string
	Title          string
	HeadBranch     string
	BaseBranch     string
	IsDraft        bool

	CheckConclusion       string
	ApprovedCount         int
	ChangesRequestedCount int

	OpenedAt         time.Time
	MergedAt         time.Time
	ClosedAt         time.Time
	LastCheckedAt    time.Time
	LastPolledAt     time.Time
	PollFailCount    int
	FirstFailAt      time.Time
	PollError        string
	RateLimitResetAt time.Time

	SessionUUID   string // exactly one of SessionUUID / BacklogItemID is set
	BacklogItemID string
	Role          PRRole
	Pinned        bool
	AttachSource  AttachSource
	AttachedAt    time.Time
}

// VerifyAttemptLimit is the counted-failure count at which an unverified-terminal
// row stops holding (see VerifyExhausted).
const VerifyAttemptLimit = VerifyExhaustedFailures

// UnverifiedTerminal reports a backfill-seeded row: state unknown with the terminal bit set.
func (v PRView) UnverifiedTerminal() bool { return v.State == PRStateUnknown && v.StoredTerminal }

// VerifyExhausted reports that polling an unverified-terminal row has failed enough
// (first to LAST counted failure, never to now) to stop holding the row.
func (v PRView) VerifyExhausted() bool {
	if v.PollFailCount < VerifyExhaustedFailures || v.FirstFailAt.IsZero() || v.LastPolledAt.IsZero() {
		return false
	}
	return v.LastPolledAt.Sub(v.FirstFailAt) >= VerifyExhaustedSpan
}

// Terminal is the effective terminal value used for ranking, the cleanup gate and
// the sweeper: closed/merged, or a stored terminal bit on a known state, or an
// unverified-terminal row whose verification is exhausted.
func (v PRView) Terminal() bool {
	switch {
	case v.State.Terminal():
		return true
	case v.State == PRStateUnknown:
		return v.StoredTerminal && v.VerifyExhausted()
	default:
		return v.StoredTerminal
	}
}

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
func NewestPR(prs []PRView, keep func(PRView) bool) *PRView {
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
		if pr := NewestPR(prs, keep); pr != nil {
			return pr
		}
	}
	return nil
}

// distinctPRs drops duplicate views of one PullRequestState (defensive: the
// schema keeps one state row per PRRef). Views without a state id fall back to
// the ref.
func DistinctPRs(prs []PRView) []PRView {
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
