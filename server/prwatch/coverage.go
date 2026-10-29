package prwatch

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

// CoverageState is how Squad knows a repository's webhook coverage (D14).
type CoverageState string

const (
	CoverageStateCovered     CoverageState = "covered"
	CoverageStateUnavailable CoverageState = "unavailable"
	// CoverageStateUnknown: the inbox predates /v1/watch (404), so coverage
	// cannot be reported and metrics 10 and 11 are not computable.
	CoverageStateUnknown CoverageState = "unknown"
)

const coverageReasonMax = 200

type repoCoverage struct {
	state     CoverageState
	pr        int       // the PR re-checked while unavailable
	reason    string    // P0's reason for the current gap
	lastCheck time.Time // last watch call for this repo
	notified  bool      // a CoverageGap is outstanding (key not forgotten)
}

// Coverage tracks per-repo webhook coverage, raises CoverageGap once per
// repo and re-checks unavailable repos until the gap clears (D9, D15).
type Coverage struct {
	in       Inbox
	notifier Notifier
	clock    Clock
	log      Logger
	settings Settings

	mu            sync.Mutex
	repos         map[string]*repoCoverage
	loggedNoWatch bool
}

// NewCoverage builds a tracker. Settings.CoverageRecheck defaults to 10
// minutes when zero.
func NewCoverage(in Inbox, notifier Notifier, clock Clock, log Logger, settings Settings) *Coverage {
	if settings.CoverageRecheck <= 0 {
		settings.CoverageRecheck = 10 * time.Minute
	}
	return &Coverage{in: in, notifier: notifier, clock: clock, log: log, settings: settings, repos: map[string]*repoCoverage{}}
}

// Watch registers interest in one PR. It calls /v1/watch and, when the inbox
// has no such route, falls back to Subscribe and records coverage unknown.
func (c *Coverage) Watch(ctx context.Context, repo string, pr int) (inbox.Subscription, CoverageState, error) {
	res, err := c.in.Watch(ctx, c.settings.ConsumerID, repo, pr)
	if errors.Is(err, inbox.ErrWatchUnsupported) {
		sub, serr := c.in.Subscribe(ctx, c.settings.ConsumerID, repo, pr)
		if serr != nil {
			return inbox.Subscription{}, CoverageStateUnknown, serr
		}
		c.markUnknown(repo, pr)
		return sub, CoverageStateUnknown, nil
	}
	if err != nil {
		return inbox.Subscription{}, "", err
	}
	state := c.apply(repo, pr, res)
	return res.Subscription, state, nil
}

// Recheck re-calls watch for one PR of every unavailable repo whose last
// check is at least CoverageRecheck old. Covered and unknown repos are
// never re-checked. Failed calls are retried at the next interval.
func (c *Coverage) Recheck(ctx context.Context) {
	now := c.clock.Now()
	type due struct {
		repo string
		pr   int
	}
	var dues []due
	c.mu.Lock()
	for repo, rc := range c.repos {
		if rc.state == CoverageStateUnavailable && now.Sub(rc.lastCheck) >= c.settings.CoverageRecheck {
			dues = append(dues, due{repo, rc.pr})
		}
	}
	c.mu.Unlock()

	for _, d := range dues {
		res, err := c.in.Watch(ctx, c.settings.ConsumerID, d.repo, d.pr)
		if err != nil {
			c.mu.Lock()
			if rc := c.repos[d.repo]; rc != nil {
				rc.lastCheck = now
			}
			c.mu.Unlock()
			c.log.Warn("prwatch.coverage_recheck_failed", "repo", d.repo, "pr", d.pr)
			continue
		}
		c.apply(d.repo, d.pr, res)
	}
}

// State reports the recorded coverage of a repo; ok is false when never seen.
func (c *Coverage) State(repo string) (CoverageState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rc, ok := c.repos[repo]
	if !ok {
		return "", false
	}
	return rc.state, true
}

func (c *Coverage) markUnknown(repo string, pr int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.repos[repo] = &repoCoverage{state: CoverageStateUnknown, pr: pr, lastCheck: c.clock.Now()}
	if !c.loggedNoWatch {
		c.loggedNoWatch = true
		c.log.Info("prwatch.coverage_unknown", "reason", "inbox has no /v1/watch")
	}
}

func (c *Coverage) apply(repo string, pr int, res inbox.WatchResult) CoverageState {
	now := c.clock.Now()
	c.mu.Lock()
	rc := c.repos[repo]
	if rc == nil {
		rc = &repoCoverage{}
		c.repos[repo] = rc
	}
	rc.lastCheck = now
	var raise bool
	if res.Coverage == inbox.CoverageUnavailable {
		rc.state = CoverageStateUnavailable
		rc.reason = res.Reason
		if rc.pr == 0 {
			rc.pr = pr
		}
		raise = !rc.notified
		rc.notified = true
	} else {
		rc.state = CoverageStateCovered
		rc.pr = pr
		rc.reason = ""
		rc.notified = false
	}
	state, reason, notifyPR := rc.state, rc.reason, rc.pr
	c.mu.Unlock()

	if raise {
		n := coverageGapNotification(repo, notifyPR, reason, c.settings)
		if err := c.notifier.Notify(n); err != nil {
			c.log.Warn("prwatch.notify_failed", "kind", string(n.Kind))
		}
	}
	return state
}

func coverageDedupeKey(repo string) string { return "prwatch-coverage:" + repo }

func coverageGapNotification(repo string, pr int, reason string, s Settings) Notification {
	reason = strings.TrimSpace(reason)
	if r := []rune(reason); len(r) > coverageReasonMax {
		reason = string(r[:coverageReasonMax])
	}
	var b strings.Builder
	b.WriteString("Squad cannot watch PR #" + strconv.Itoa(pr) + " in " + repo +
		" because no GitHub webhook reaches gh-signal for that repository, so sessions on its PRs will not be steered.")
	if reason != "" {
		b.WriteString(" gh-signal said: " + reason + ".")
	}
	b.WriteString(" " + coverageNextStep(repo, pr, reason, s.ConsumerID))
	b.WriteString(" To stop this notice instead, add " + repo + " to inbox_pr_watch.ignore_repos.")
	if s.StatusURL != "" {
		b.WriteString(" Status: " + s.StatusURL)
	}
	return Notification{
		Kind:      NotifyCoverageGap,
		DedupeKey: coverageDedupeKey(repo),
		Title:     strings.ReplaceAll(TitleCoverageGap, "<owner>/<repo>", repo),
		Message:   b.String(),
	}
}

// coverageNextStep varies the advice by P0's reason string.
func coverageNextStep(repo string, pr int, reason, consumerID string) string {
	cmd := "gh-signal inbox watch " + repo + " " + strconv.Itoa(pr) + " --consumer " + consumerID
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "admin") || strings.Contains(lower, "permission"):
		return "Next step: ask a repository admin to add the webhook (or grant your account admin access), then run " + cmd + "."
	case strings.Contains(lower, "auth") || strings.Contains(lower, "token") || strings.Contains(lower, "login"):
		return "Next step: fix gh authentication (gh auth status), then run " + cmd + "."
	default:
		return "Next step: run " + cmd + "."
	}
}
