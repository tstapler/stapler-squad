package prwatch

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Lifecycle is a session's coarse liveness, as the adapter maps it from the
// session status.
type Lifecycle int

const (
	// LifeLive: the session has a running process.
	LifeLive Lifecycle = iota
	// LifeSuspended: paused, hibernated or crashed; may be resumed.
	LifeSuspended
	// LifeGone: stopped, failed, archived or removed.
	LifeGone
	// LifeTransitional: creating or restoring; reported live but never
	// changes the resume bookkeeping.
	LifeTransitional
)

// SessionFacts is what the adapter extracts from a session; ComputeWatchSet
// turns it into SessionRefs without touching session, github or config.
type SessionFacts struct {
	UUID      string
	Title     string // raw; sanitized here
	Lifecycle Lifecycle
	Hidden    bool
	Owner     string
	Repo      string
	Host      string // "" is github.com
	PR        int
	Fork      bool
	Terminal  bool // the session's PR is merged or closed
}

// BacklogLinks answers D4. Contract: Pending reports, for the given session
// UUIDs, which have a newest linked backlog item in pr_pending, and which
// lookups failed (fail closed). A session with no link appears in neither
// set. A non-nil error means every lookup failed.
type BacklogLinks interface {
	Pending(ctx context.Context, sessionUUIDs []string) (pending, failed map[string]bool, err error)
}

// WatchSetInput is one watch-set computation.
type WatchSetInput struct {
	Sessions    []SessionFacts
	IgnoreRepos []string // owner/name, any case
	Links       BacklogLinks
	Tracker     *ResumeTracker
	Now         time.Time
}

var repoPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// maxTitleRunes bounds the operator-surface title.
const maxTitleRunes = 60

// SanitizeTitle makes a session title safe for notifications and status:
// control and format characters are dropped, whitespace collapses to single
// spaces, and the result is cut to maxTitleRunes.
func SanitizeTitle(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = b.Len() > 0
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == utf8.RuneError:
		default:
			if space {
				b.WriteByte(' ')
				space = false
			}
			b.WriteRune(r)
		}
	}
	out := b.String()
	if utf8.RuneCountInString(out) <= maxTitleRunes {
		return out
	}
	return string([]rune(out)[:maxTitleRunes-1]) + "…"
}

// ComputeWatchSet classifies every session as watched (empty SkipReason) or
// skipped with a reason (D4, D8). Backlog state is read once, in one batch,
// and only for sessions that pass every cheaper check. It never returns an
// error: a failed backlog read skips the affected sessions (fail closed).
func ComputeWatchSet(ctx context.Context, in WatchSetInput) []SessionRef {
	ignore := make(map[string]bool, len(in.IgnoreRepos))
	for _, r := range in.IgnoreRepos {
		ignore[strings.ToLower(r)] = true
	}
	refs := make([]SessionRef, 0, len(in.Sessions))
	var candidates []int // indexes into refs still needing the backlog check
	seen := make(map[string]bool, len(in.Sessions))
	for _, f := range in.Sessions {
		seen[f.UUID] = true
		ref := SessionRef{UUID: f.UUID, Title: SanitizeTitle(f.Title), PR: f.PR, State: StateLive}
		if in.Tracker != nil {
			ref.ResumedAt = in.Tracker.Observe(f.UUID, f.Lifecycle, in.Now)
		}
		switch f.Lifecycle {
		case LifeSuspended:
			ref.State = StateSuspended
		case LifeGone:
			ref.State = StateGone
		}
		ref.SkipReason = skipReasonFor(f, ref.State, ignore)
		if repoPart.MatchString(f.Owner) && repoPart.MatchString(f.Repo) {
			ref.Repo = f.Owner + "/" + f.Repo
		}
		if ref.SkipReason == "" {
			candidates = append(candidates, len(refs))
		}
		refs = append(refs, ref)
	}
	if in.Tracker != nil {
		in.Tracker.Prune(seen)
	}
	if len(candidates) > 0 {
		applyBacklog(ctx, in.Links, refs, candidates)
	}
	return refs
}

// isGitHubDotCom: sessions created through MCP create_session_for_pr store the
// host explicitly as "github.com", while hand-made ones leave it empty.
func isGitHubDotCom(host string) bool {
	return host == "" || strings.EqualFold(host, "github.com")
}

func skipReasonFor(f SessionFacts, state SessionState, ignore map[string]bool) SkipReason {
	switch {
	case state == StateGone:
		return SkipNotLive
	case f.Hidden:
		return SkipHidden
	case !isGitHubDotCom(f.Host):
		return SkipGHEHost
	case f.Fork:
		return SkipFork
	case !repoPart.MatchString(f.Owner) || !repoPart.MatchString(f.Repo):
		return SkipNoOwnerRepo
	case f.PR <= 0:
		return SkipNoPR
	case f.Terminal:
		return SkipPRTerminal
	case ignore[strings.ToLower(f.Owner+"/"+f.Repo)]:
		return SkipIgnoreRepos
	}
	return ""
}

func applyBacklog(ctx context.Context, links BacklogLinks, refs []SessionRef, idx []int) {
	if links == nil {
		return
	}
	uuids := make([]string, len(idx))
	for n, i := range idx {
		uuids[n] = refs[i].UUID
	}
	pending, failed, err := links.Pending(ctx, uuids)
	for _, i := range idx {
		switch {
		case err != nil || failed[refs[i].UUID]:
			refs[i].SkipReason = SkipBacklogLookupFailed
		case pending[refs[i].UUID]:
			refs[i].SkipReason = SkipBacklogPRPending
		}
	}
}

// ResumeTracker remembers each session's last-seen lifecycle in memory and
// reports when a suspended session becomes live (VERIFIED: no persisted
// resume time exists). A session already live the first time it is seen is
// not resumed. Safe for concurrent use.
type ResumeTracker struct {
	mu        sync.Mutex
	suspended map[string]bool
	resumedAt map[string]time.Time
}

// NewResumeTracker returns an empty tracker.
func NewResumeTracker() *ResumeTracker {
	return &ResumeTracker{suspended: map[string]bool{}, resumedAt: map[string]time.Time{}}
}

// Observe records the lifecycle and returns the time the session last
// resumed in this process, zero if it has not. The value stays until the
// session is suspended again or disappears.
func (t *ResumeTracker) Observe(uuid string, l Lifecycle, now time.Time) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch l {
	case LifeSuspended:
		t.suspended[uuid] = true
		delete(t.resumedAt, uuid)
	case LifeLive:
		if t.suspended[uuid] {
			delete(t.suspended, uuid)
			t.resumedAt[uuid] = now
		}
	case LifeGone:
		delete(t.suspended, uuid)
		delete(t.resumedAt, uuid)
	}
	return t.resumedAt[uuid]
}

// Prune forgets sessions not in seen so the maps stay bounded.
func (t *ResumeTracker) Prune(seen map[string]bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for u := range t.suspended {
		if !seen[u] {
			delete(t.suspended, u)
		}
	}
	for u := range t.resumedAt {
		if !seen[u] {
			delete(t.resumedAt, u)
		}
	}
}
