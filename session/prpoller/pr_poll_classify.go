package prpoller

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"

	"github.com/tstapler/stapler-squad/github"
)

// hostAuthWindow is how long a host may go without a successful fetch before a
// 401 or a missing token is read as token-wide (every PR on it failing at once).
const hostAuthWindow = 15 * time.Minute

type pollFailureClass int

const (
	pollFailTransient pollFailureClass = iota // 5xx, timeout, network: never counted
	pollFailRateLimit
	pollFailAuth // host-level, not counted
	pollFailRepoInaccessible
	pollFailCounted
)

// pollFailure is the classification of one failed fetch.
type pollFailure struct {
	Class pollFailureClass
	// PollError is the poll_error enum the row records.
	PollError string
	// Metric is the ssq_pr_poll_failures_total reason label.
	Metric string
}

// Counted reports whether the failure counts toward unknown/exhaustion.
func (f pollFailure) Counted() bool { return f.Class == pollFailCounted }

// hostHealth remembers the last successful fetch per host (in memory, per process).
type hostHealth struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func newHostHealth() *hostHealth { return &hostHealth{last: map[string]time.Time{}} }

func (h *hostHealth) markSuccess(host string, at time.Time) {
	h.mu.Lock()
	h.last[domain.NormalizeHost(host)] = at
	h.mu.Unlock()
}

func (h *hostHealth) healthy(host string, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	at, ok := h.last[domain.NormalizeHost(host)]
	return ok && now.Sub(at) < hostAuthWindow
}

// repoProbeFunc probes the repository once; it is only called for a counted-candidate 404 or 403.
type repoProbeFunc func() (github.RepoProbe, error)

// classifyPollError sorts a failed PR fetch into the plan's classes. The GitHub
// client reports failures as error text (status code and rate-limit kind), so the
// match is on that text; headers are not visible here, which is why the SSO case
// is read from the repository probe instead.
func classifyPollError(err error, host string, now time.Time, health *hostHealth, probe repoProbeFunc) pollFailure {
	msg := strings.ToLower(err.Error())
	switch {
	case containsAny(msg, "rate limit", "429", "retry-after", "abuse"):
		return pollFailure{Class: pollFailRateLimit, PollError: "rate_limited", Metric: "rate_limited"}
	case errors.Is(err, github.ErrNotAuthenticated) || containsAny(msg, "401", "unauthorized"):
		if !health.healthy(host, now) {
			return pollFailure{Class: pollFailAuth, PollError: "auth", Metric: "auth"}
		}
		return pollFailure{Class: pollFailCounted, PollError: "auth", Metric: "auth"}
	case containsAny(msg, "404", "not found", "403", "forbidden"):
		return classifyMissing(msg, probe)
	}
	return pollFailure{Class: pollFailTransient, PollError: "network", Metric: "network"}
}

func classifyMissing(msg string, probe repoProbeFunc) pollFailure {
	inaccessible := pollFailure{Class: pollFailRepoInaccessible, PollError: "repo_inaccessible", Metric: "repo_inaccessible"}
	if strings.Contains(msg, "sso") {
		return inaccessible
	}
	p, err := probe()
	switch {
	case err != nil:
		return pollFailure{Class: pollFailTransient, PollError: "network", Metric: "network"}
	case p.SSO || p.Status == 404 || p.Status == 403:
		return inaccessible
	}
	reason := "not_found"
	if containsAny(msg, "403", "forbidden") {
		reason = "auth"
	}
	return pollFailure{Class: pollFailCounted, PollError: reason, Metric: reason}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
