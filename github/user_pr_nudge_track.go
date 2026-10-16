package github

import (
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// NudgeReasonKind names why a PR needed a nudge. It mirrors the proto
// NudgeReason enum without importing generated code into this package.
type NudgeReasonKind string

const (
	NudgeReasonFailingChecks     NudgeReasonKind = "failing_checks"
	NudgeReasonUnresolvedThreads NudgeReasonKind = "unresolved_threads"
	NudgeReasonMergeConflict     NudgeReasonKind = "merge_conflict"
)

// NudgeFollowupState is how a delivered nudge turned out.
type NudgeFollowupState string

const (
	NudgeFollowupResolved NudgeFollowupState = "resolved"
	NudgeFollowupClosed   NudgeFollowupState = "closed"
	NudgeFollowupExpired  NudgeFollowupState = "expired"
)

const (
	maxRecordedNudges   = 256
	nudgeFollowupWindow = 24 * time.Hour
)

// NudgeFollowup is the outcome of one recorded nudge, produced by a later poll.
type NudgeFollowup struct {
	Key           PRKey
	State         NudgeFollowupState
	ResolvedAfter time.Duration // set for NudgeFollowupResolved
}

type recordedNudge struct {
	key     PRKey
	reasons []NudgeReasonKind
	at      time.Time
}

// nudgeTracker holds in-memory success-metric state. It is lost on restart, so
// follow-up counts and attention ages are lower bounds. The zero value is ready
// to use. A mutex (not atomics): both maps change together on each poll.
type nudgeTracker struct {
	mu        sync.Mutex
	firstSeen map[PRKey]time.Time
	recorded  []recordedNudge // oldest first
}

// RecordNudge remembers a delivered nudge so a later poll can log whether it
// worked. At most maxRecordedNudges are kept; the oldest is dropped first.
func (c *UserPRCache) RecordNudge(key PRKey, reasons []NudgeReasonKind, at time.Time) {
	t := &c.nudges
	t.mu.Lock()
	defer t.mu.Unlock()
	kept := t.recorded[:0:0]
	for _, r := range t.recorded {
		if r.key != key {
			kept = append(kept, r)
		}
	}
	kept = append(kept, recordedNudge{key: key, reasons: append([]NudgeReasonKind(nil), reasons...), at: at})
	if over := len(kept) - maxRecordedNudges; over > 0 {
		kept = kept[over:]
	}
	t.recorded = kept
}

// FirstSeenAttention reports when a poll first saw key needing attention.
func (c *UserPRCache) FirstSeenAttention(key PRKey) (time.Time, bool) {
	t := &c.nudges
	t.mu.Lock()
	defer t.mu.Unlock()
	at, ok := t.firstSeen[key]
	return at, ok
}

func (c *UserPRCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// trackNudges runs after each snapshot store and logs one nudge_followup line
// per finished nudge.
func (c *UserPRCache) trackNudges(prs []UserPR) {
	for _, ev := range c.nudges.evaluate(prs, c.clock()) {
		log.Info("nudge_followup",
			"pr", ev.Key.String(),
			"host", ev.Key.Host(),
			"state", string(ev.State),
			"resolved_after_s", int64(ev.ResolvedAfter/time.Second))
	}
}

// evaluate refreshes first-seen-attention times and settles recorded nudges
// against the latest open-PR list. A PR missing from the list counts as closed
// even if it vanished because its account's poll failed (a known lower-bound
// inaccuracy).
func (t *nudgeTracker) evaluate(prs []UserPR, now time.Time) []NudgeFollowup {
	t.mu.Lock()
	defer t.mu.Unlock()

	open := make(map[PRKey]*UserPR, len(prs))
	seen := make(map[PRKey]time.Time, len(prs))
	for i := range prs {
		key, err := NewPRKey(prs[i].Host, prs[i].Owner, prs[i].Repo, prs[i].Number)
		if err != nil {
			continue
		}
		open[key] = &prs[i]
		if prNeedsAttention(&prs[i]) {
			if at, ok := t.firstSeen[key]; ok {
				seen[key] = at
			} else {
				seen[key] = now
			}
		}
	}
	t.firstSeen = seen

	var out []NudgeFollowup
	pending := t.recorded[:0:0]
	for _, r := range t.recorded {
		pr, isOpen := open[r.key]
		switch {
		case !isOpen:
			out = append(out, NudgeFollowup{Key: r.key, State: NudgeFollowupClosed})
		case !anyReasonPresent(pr, r.reasons, true):
			out = append(out, NudgeFollowup{Key: r.key, State: NudgeFollowupResolved, ResolvedAfter: now.Sub(r.at)})
		case now.Sub(r.at) >= nudgeFollowupWindow:
			out = append(out, NudgeFollowup{Key: r.key, State: NudgeFollowupExpired})
		default:
			pending = append(pending, r)
		}
	}
	t.recorded = pending
	return out
}

// prNeedsAttention is true when a nudgeable reason is known to be present.
func prNeedsAttention(pr *UserPR) bool {
	return anyReasonPresent(pr, []NudgeReasonKind{NudgeReasonFailingChecks, NudgeReasonUnresolvedThreads, NudgeReasonMergeConflict}, false)
}

// anyReasonPresent reports whether any of reasons is present on pr. unknownIs
// is the answer for a reason whose state GitHub did not report (nil count or
// conflict flag): follow-up settlement passes true so unknown never reads as
// "resolved"; attention tracking passes false.
func anyReasonPresent(pr *UserPR, reasons []NudgeReasonKind, unknownIs bool) bool {
	for _, r := range reasons {
		switch r {
		case NudgeReasonFailingChecks:
			if len(pr.FailingChecks) > 0 {
				return true
			}
		case NudgeReasonUnresolvedThreads:
			if pr.UnresolvedThreadCount == nil && unknownIs || pr.UnresolvedThreadCount != nil && *pr.UnresolvedThreadCount > 0 {
				return true
			}
		case NudgeReasonMergeConflict:
			if pr.HasMergeConflict == nil && unknownIs || pr.HasMergeConflict != nil && *pr.HasMergeConflict {
				return true
			}
		}
	}
	return false
}
