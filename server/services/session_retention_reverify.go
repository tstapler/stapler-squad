package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

const (
	// MetricRetentionClosedPRUnverifiable counts closed-PR re-verifications that
	// failed for a reason retrying will not fix (no token, deleted/private repo, 401/403).
	MetricRetentionClosedPRUnverifiable = "ssq_retention_closed_pr_unverifiable_total"

	// Per-sweep bounds on GitHub re-verification, so a bad token or an outage cannot
	// turn one sweep into N sequential 30s requests. PRs over budget hold their
	// session until the next sweep.
	reverifyMaxChecksPerSweep = 25
	reverifyBudgetPerSweep    = 60 * time.Second
	reverifyCallTimeout       = 10 * time.Second

	// A PR that stays unverifiable is released after this many consecutive failed
	// sweeps spanning this long, mirroring session.PRView's VerifyExhausted rule
	// (VerifyAttemptLimit failures over 30 minutes) so a session is not held forever.
	reverifyUnverifiableSpan = 30 * time.Minute
)

var closedPRUnverifiable = newItemPRHoldCounter(MetricRetentionClosedPRUnverifiable)

// ClosedPRUnverifiableCount is ssq_retention_closed_pr_unverifiable_total in this process.
func ClosedPRUnverifiableCount() int64 { return closedPRUnverifiable.n.Load() }

type reverifyOutcome int

const (
	reverifyStillClosed reverifyOutcome = iota
	reverifyReopened
	reverifyTransient    // rate limit, 5xx, network: retry next sweep, always holds
	reverifyUnverifiable // no token, 404, 401/403: holds until the count/age rule releases it
)

// classifyReverifyError sorts a GitHub lookup error. Unknown errors are
// transient: holding is the safe default.
func classifyReverifyError(err error) reverifyOutcome {
	if errors.Is(err, githubpkg.ErrNotAuthenticated) || errors.Is(err, githubpkg.ErrGitHubRefNotFound) {
		return reverifyUnverifiable
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "rate limit") || strings.Contains(msg, "(429)") {
		return reverifyTransient
	}
	for _, marker := range []string{"unauthorized (401)", "forbidden (403)", "(404)", "404", "not found", "could not resolve to"} {
		if strings.Contains(msg, marker) {
			return reverifyUnverifiable
		}
	}
	return reverifyTransient
}

type unverifiableState struct {
	failures int
	first    time.Time
}

// closedPRReverifier holds the per-sweep GitHub budget and, across sweeps, how long
// each PR has been unverifiable. Sweeps are single-goroutine; mu covers test access.
type closedPRReverifier struct {
	mu           sync.Mutex
	checks       int
	deadline     time.Time
	unverifiable map[string]unverifiableState
}

func (r *closedPRReverifier) startSweep(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks, r.deadline = 0, now.Add(reverifyBudgetPerSweep)
}

// take spends one check, or reports the budget exhausted.
func (r *closedPRReverifier) take(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.checks >= reverifyMaxChecksPerSweep || (!r.deadline.IsZero() && now.After(r.deadline)) {
		return false
	}
	r.checks++
	return true
}

// noteUnverifiable records one more unverifiable sweep for key and reports whether
// the PR has been unverifiable long enough to stop holding its session.
func (r *closedPRReverifier) noteUnverifiable(key string, now time.Time) (released bool, st unverifiableState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.unverifiable == nil {
		r.unverifiable = make(map[string]unverifiableState)
	}
	st = r.unverifiable[key]
	if st.failures == 0 {
		st.first = now
	}
	st.failures++
	r.unverifiable[key] = st
	return st.failures >= session.VerifyAttemptLimit && now.Sub(st.first) >= reverifyUnverifiableSpan, st
}

func (r *closedPRReverifier) clear(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.unverifiable, key)
}

// closedPRsStayClosed re-asks GitHub about every closed-unmerged PR the session
// or its items hold. The store keeps a closed PR terminal against anything but a
// reopened webhook, so a missed webhook would otherwise let a reopened PR's
// worktree be deleted. Reopened, rate-limited, network and over-budget outcomes
// hold the session; an unverifiable PR holds it until the count/age rule releases it.
func (s *SessionRetentionSweeper) closedPRsStayClosed(ctx context.Context, d session.InstanceData, p *sweepPRData) (bool, string) {
	if s.reverifyClosed == nil {
		return true, ""
	}
	var views []session.PRView
	views = append(views, p.views[d.UUID]...)
	for _, id := range p.itemIDs[d.UUID] {
		views = append(views, p.guards[id].Direct...)
	}
	for _, v := range views {
		if v.State != session.PRStateClosed {
			continue
		}
		if safe, reason := s.reverifyOne(ctx, d, v.Ref); !safe {
			return false, reason
		}
	}
	return true, ""
}

func (s *SessionRetentionSweeper) reverifyOne(ctx context.Context, d session.InstanceData, ref session.PRRef) (bool, string) {
	now := s.now()
	if !s.reverify.take(now) {
		return false, fmt.Sprintf("closed PR %s not re-verified: per-sweep GitHub budget used, retrying next sweep", ref)
	}
	callCtx, cancel := context.WithTimeout(ctx, reverifyCallTimeout)
	defer cancel()
	reopened, err := s.reverifyClosed(callCtx, ref)
	key := ref.String()
	switch {
	case err == nil && reopened:
		s.reverify.clear(key)
		return false, fmt.Sprintf("PR %s was reopened on GitHub", ref)
	case err == nil:
		s.reverify.clear(key)
		return true, ""
	case classifyReverifyError(err) == reverifyUnverifiable:
		released, st := s.reverify.noteUnverifiable(key, now)
		closedPRUnverifiable.inc()
		log.Warn("session retention sweeper: closed PR cannot be re-verified on GitHub",
			"pr", key, "session", d.Title, "failures", st.failures, "first_failure", st.first, "released", released, "err", err)
		if released {
			return true, ""
		}
		return false, fmt.Sprintf("closed PR %s unverifiable (%d/%d): %v", ref, st.failures, session.VerifyAttemptLimit, err)
	default:
		return false, fmt.Sprintf("could not re-verify closed PR %s, retrying next sweep: %v", ref, err)
	}
}

// newClosedPRReverifier is the production reverifyClosed: one conditional GET
// per PR, with a process-lifetime ETag cache so repeat sweeps cost a 304.
func newClosedPRReverifier() func(context.Context, session.PRRef) (bool, error) {
	cache := githubpkg.NewETagCache()
	return func(ctx context.Context, ref session.PRRef) (bool, error) {
		repo, err := githubpkg.NewRepoRefWithHost(ref.Owner(), ref.Repo(), ref.Host())
		if err != nil {
			return false, err
		}
		info, _, err := githubpkg.GetPRInfoConditional(ctx, repo, ref.Number(), cache)
		if err != nil {
			return false, err
		}
		if info == nil {
			return false, errors.New("GitHub returned no PR data (304 without a cached copy)")
		}
		switch strings.ToLower(info.State) {
		case "closed", "merged":
			return false, nil
		case "open":
			return true, nil
		}
		return false, fmt.Errorf("unrecognized PR state %q", info.State)
	}
}
