package services

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/telemetry"
)

// MetricRetentionItemPRHoldOverMax counts item-direct PR holds that outlasted
// itemPRHoldMaxAge, once per (item, hold window).
const MetricRetentionItemPRHoldOverMax = "ssq_retention_item_pr_hold_over_max_total"

// MetricRetentionSessionPRHoldOverMax counts session-level PR holds on expired
// archived sessions that outlasted itemPRHoldMaxAge, once per (session, window).
const MetricRetentionSessionPRHoldOverMax = "ssq_retention_session_pr_hold_over_max_total"

// itemPRHoldMaxAge is when an item-direct hold is surfaced. The hold itself
// never expires: releasing would delete a worktree under a possibly open PR.
const itemPRHoldMaxAge = time.Hour

var (
	itemPRHoldOverMax    = newItemPRHoldCounter(MetricRetentionItemPRHoldOverMax)
	sessionPRHoldOverMax = newItemPRHoldCounter(MetricRetentionSessionPRHoldOverMax)
)

type itemPRHoldCounter struct {
	n    atomic.Int64
	otel metric.Int64Counter
}

func newItemPRHoldCounter(name string) *itemPRHoldCounter {
	c := &itemPRHoldCounter{}
	if oc, err := telemetry.GetMeter().Int64Counter(name); err == nil {
		c.otel = oc
	}
	return c
}

func (c *itemPRHoldCounter) inc() {
	c.n.Add(1)
	if c.otel != nil {
		c.otel.Add(context.Background(), 1)
	}
}

// ItemPRHoldOverMaxCount is ssq_retention_item_pr_hold_over_max_total in this process.
func ItemPRHoldOverMaxCount() int64 { return itemPRHoldOverMax.n.Load() }

// SessionPRHoldOverMaxCount is ssq_retention_session_pr_hold_over_max_total in this process.
func SessionPRHoldOverMaxCount() int64 { return sessionPRHoldOverMax.n.Load() }

// ItemPRHold is an item-direct PR hold on an expired archived session.
type ItemPRHold struct {
	ItemID      string
	SessionUUID string
	Since       time.Time
}

// itemHolds lists the item-direct holds on d. A hold began at the later of the
// session's retention expiry (before it the sweeper would not delete anyway)
// and the non-terminal link's attached_at, the only durable per-link timestamp.
// Items with no timestamp (no evidence, unrepresented legacy number) are held
// but cannot be aged, so they are not listed.
func (p *sweepPRData) itemHolds(d session.InstanceData, expiry time.Time) []ItemPRHold {
	var out []ItemPRHold
	for _, id := range p.itemIDs[d.UUID] {
		since := time.Time{}
		for _, v := range p.guards[id].Direct {
			if v.Terminal() {
				continue
			}
			at := v.AttachedAt
			if at.Before(expiry) {
				at = expiry
			}
			if since.IsZero() || at.Before(since) {
				since = at
			}
		}
		if !since.IsZero() {
			out = append(out, ItemPRHold{ItemID: id, SessionUUID: d.UUID, Since: since})
		}
	}
	return out
}

// itemHoldTracker reports each over-max hold once until the hold clears.
type itemHoldTracker struct {
	mu       sync.Mutex
	reported map[string]struct{}
	overMax  []ItemPRHold
	// reportedTotal is this tracker's own tally (the process counter is shared).
	reportedTotal int
}

// observe takes this cycle's holds, logs and counts the newly over-max ones, and
// forgets items whose hold is gone or back under the line so a new window re-reports.
func (t *itemHoldTracker) observe(holds []ItemPRHold, now time.Time) {
	t.observeWith(itemPRHoldOverMax, "session retention sweeper: item-direct PR hold exceeded max age; session kept (fail closed)", holds, now)
}

// observeWith is observe with the counter and log line of one hold kind. For a
// session-level hold ItemPRHold.ItemID carries the session uuid, the dedup key.
func (t *itemHoldTracker) observeWith(counter *itemPRHoldCounter, msg string, holds []ItemPRHold, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	current := make(map[string]struct{})
	var over []ItemPRHold
	for _, h := range holds {
		if now.Sub(h.Since) <= itemPRHoldMaxAge {
			continue
		}
		over = append(over, h)
		current[h.ItemID] = struct{}{}
		if _, done := t.reported[h.ItemID]; !done {
			counter.inc()
			t.reportedTotal++
			log.Warn(msg,
				"item", h.ItemID, "session", h.SessionUUID, "held_since", h.Since)
		}
	}
	sort.Slice(over, func(i, j int) bool { return over[i].ItemID < over[j].ItemID })
	t.reported, t.overMax = current, over
}

func (t *itemHoldTracker) snapshot() []ItemPRHold {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]ItemPRHold(nil), t.overMax...)
}

// OverMaxItemPRHolds lists the item-direct holds currently over the max age,
// for the backlog stuck report (T5.2.2).
func (s *SessionRetentionSweeper) OverMaxItemPRHolds() []ItemPRHold { return s.holds.snapshot() }
