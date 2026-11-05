package session

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/github"
)

// ErrZeroPRRef is returned by every PR repository entry point handed a PRRef{}.
var ErrZeroPRRef = errors.New("session: zero PRRef")

// PRRef identifies one pull request: canonical host, owner and repo plus the number.
// The fields are unexported so a literal cannot be built outside this file; use
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
	return PRRef{host: github.NormalizeHost(host), owner: owner, repo: repo, number: number}, nil
}

func (r PRRef) Host() string  { return r.host }
func (r PRRef) Owner() string { return r.owner }
func (r PRRef) Repo() string  { return r.repo }
func (r PRRef) Number() int   { return r.number }

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
	verifyExhaustedFailures = 5
	verifyExhaustedSpan     = 30 * time.Minute
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
const VerifyAttemptLimit = verifyExhaustedFailures

// UnverifiedTerminal reports a backfill-seeded row: state unknown with the terminal bit set.
func (v PRView) UnverifiedTerminal() bool { return v.State == PRStateUnknown && v.StoredTerminal }

// VerifyExhausted reports that polling an unverified-terminal row has failed enough
// (first to LAST counted failure, never to now) to stop holding the row.
func (v PRView) VerifyExhausted() bool {
	if v.PollFailCount < verifyExhaustedFailures || v.FirstFailAt.IsZero() || v.LastPolledAt.IsZero() {
		return false
	}
	return v.LastPolledAt.Sub(v.FirstFailAt) >= verifyExhaustedSpan
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
