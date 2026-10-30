package session

import (
	"context"
	"slices"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

const (
	// graceAfterAck is how long after an acknowledgment low-priority or controller-less
	// re-adds are dropped.
	graceAfterAck = 5 * time.Minute
	// minReAddInterval limits how soon an already-queued session may be re-added.
	minReAddInterval = 2 * time.Minute
	// snoozeTTL is how long an output-based ack may hide an item (with AckBypassFix on).
	snoozeTTL = 4 * time.Hour
	// episodeMetadataKey carries a queued item's EpisodeID so the limiter can tell a new episode.
	episodeMetadataKey = "episode_id"
)

// RuleOptions are the flags consumed by the poller's suppression rules.
type RuleOptions struct {
	AckBypassFix bool
}

type suppressionKind uint8

const (
	suppressionKeep     suppressionKind = iota // no rule objected; continue to the queue update
	suppressionRemove                          // drop the session's queue item
	suppressionSuppress                        // leave the queue unchanged and stop
)

// SuppressionDecision is the typed outcome of the post-Determine rule chain.
type SuppressionDecision struct {
	kind suppressionKind
	rule string
}

func keepDecision() SuppressionDecision { return SuppressionDecision{kind: suppressionKeep} }

func removeDecision(rule string) SuppressionDecision {
	return SuppressionDecision{kind: suppressionRemove, rule: rule}
}

func suppressDecision(rule string) SuppressionDecision {
	return SuppressionDecision{kind: suppressionSuppress, rule: rule}
}

// ruleCtx is everything a suppression rule may read; rules are pure functions of it.
type ruleCtx struct {
	action           DetectionAction
	priority         Priority
	reason           AttentionReason
	controllerActive bool
	ackedAfterOutput bool
	lastAcknowledged time.Time
	lastAddedToQueue time.Time
	now              time.Time
	// episodeChanged marks a result whose episode is not the acked one: an unacked
	// episode is an escalation, never a re-add shortly after an ack (grace).
	episodeChanged bool
	episode        EpisodeID // the result's turn-end episode; empty for non-episode reasons
	ackedEpisode   EpisodeID
	existing       func() (*ReviewItem, bool)
	opts           RuleOptions
}

func (c ruleCtx) shouldAdd() bool { return c.action == DetectionActionAdd }

// suppressionRule returns (decision, true) when it decides; (_, false) defers to the next rule.
type suppressionRule struct {
	name  string
	apply func(ruleCtx) (SuppressionDecision, bool)
}

// suppressionRules is an ordered contract: the first rule that decides wins.
var suppressionRules = []suppressionRule{
	{"ackSnooze", ackSnooze},
	{"grace", grace},
	{"readdLimit", readdLimit},
	{"episodeAck", episodeAck},
}

// suppressionPolicy runs the rule chain for one session's detection result.
func suppressionPolicy(snap *InstanceSnapshot, result DetectionResult, statusInfo InstanceStatusInfo, queue *ReviewQueue, opts RuleOptions) SuppressionDecision {
	rc := ruleCtx{
		action:           result.Action,
		priority:         result.Priority,
		reason:           result.Reason,
		controllerActive: statusInfo.IsControllerActive,
		ackedAfterOutput: snap.IsAcknowledgedAfterOutput(),
		lastAcknowledged: snap.LastAcknowledged,
		lastAddedToQueue: snap.LastAddedToQueue,
		now:              time.Now(),
		episode:          result.Episode,
		ackedEpisode:     snap.AckedEpisodeID,
		episodeChanged:   result.Episode != "" && result.Episode != snap.AckedEpisodeID,
		existing:         func() (*ReviewItem, bool) { return queue.Get(snap.Title) },
		opts:             opts,
	}
	return evaluateRules(suppressionRules, rc)
}

func evaluateRules(rules []suppressionRule, rc ruleCtx) SuppressionDecision {
	for _, r := range rules {
		if d, decided := r.apply(rc); decided {
			return d
		}
	}
	return keepDecision()
}

// needsHumanReasons are the reasons that block the agent on a person; with
// RuleOptions.AckBypassFix they escape the output-based ack snooze.
var needsHumanReasons = []AttentionReason{ReasonApprovalPending, ReasonInputRequired, ReasonErrorState, ReasonProseQuestion}

// NeedsHumanReasons returns a copy of the needs-human reason set.
func NeedsHumanReasons() []AttentionReason { return slices.Clone(needsHumanReasons) }

func isNeedsHumanReason(r AttentionReason) bool { return slices.Contains(needsHumanReasons, r) }

// isEpisodeKeyedReason marks the reasons whose ack is per turn-end episode, not per output.
func isEpisodeKeyedReason(r AttentionReason) bool {
	return r == ReasonProseQuestion || r == ReasonAwaitingDirection
}

// setEpisodeMetadata records the item's episode so the limiter can recognise a new one.
func setEpisodeMetadata(item *ReviewItem, ep EpisodeID) {
	if ep == "" {
		return
	}
	if item.Metadata == nil {
		item.Metadata = make(map[string]string)
	}
	item.Metadata[episodeMetadataKey] = string(ep)
}

// episodeAcked reports whether a result for reason/episode was already acknowledged.
func episodeAcked(reason AttentionReason, episode, acked EpisodeID) bool {
	return isEpisodeKeyedReason(reason) && episode != "" && episode == acked
}

// ackSnooze drops items the user acknowledged after the last output. Active high-priority
// adds bypass it: content-signature dedup leaves LastMeaningfulOutput unchanged when an
// identical prompt reappears, so the ack would otherwise hide a live blocking prompt.
// With AckBypassFix the same holds for every needs-human reason, whatever its priority,
// and a snooze expires after snoozeTTL. Episode-keyed reasons never use this snooze: their
// ack is the episodeAck rule, regardless of AckBypassFix.
func ackSnooze(c ruleCtx) (SuppressionDecision, bool) {
	bypass := c.shouldAdd() && (c.priority <= PriorityHigh || (c.opts.AckBypassFix && isNeedsHumanReason(c.reason)))
	if c.shouldAdd() && isEpisodeKeyedReason(c.reason) {
		bypass = true
	}
	expired := c.opts.AckBypassFix && !c.lastAcknowledged.IsZero() && c.now.Sub(c.lastAcknowledged) >= snoozeTTL
	if !bypass && !expired && c.ackedAfterOutput {
		return removeDecision("ackSnooze"), true
	}
	return SuppressionDecision{}, false
}

// grace drops low-priority or controller-less re-adds shortly after an acknowledgment.
func grace(c ruleCtx) (SuppressionDecision, bool) {
	if !c.shouldAdd() || c.episodeChanged {
		return SuppressionDecision{}, false
	}
	if c.priority != PriorityLow && c.controllerActive {
		return SuppressionDecision{}, false
	}
	if !c.lastAcknowledged.IsZero() && c.now.Sub(c.lastAcknowledged) < graceAfterAck {
		return removeDecision("grace"), true
	}
	return SuppressionDecision{}, false
}

// readdLimit stops notification spam for an item that is already queued; an escalation
// or a new episode (different from the queued item's) passes. A restart empties the queue, so a missing item is never limited.
func readdLimit(c ruleCtx) (SuppressionDecision, bool) {
	if !c.shouldAdd() || c.lastAddedToQueue.IsZero() || c.now.Sub(c.lastAddedToQueue) >= minReAddInterval {
		return SuppressionDecision{}, false
	}
	existing, queued := c.existing()
	if !queued {
		return SuppressionDecision{}, false
	}
	// Lower priority number = higher priority (Urgent=1 > High=2 > Medium=3 > Low=4)
	newEpisode := c.episode != "" && c.episode != EpisodeID(existing.Metadata[episodeMetadataKey])
	if c.priority < existing.Priority || newEpisode {
		log.Debug("escalation, bypassing rate limit", "from", existing.Priority.String(), "to", c.priority.String())
		return SuppressionDecision{}, false
	}
	return suppressDecision("readdLimit"), true
}

// episodeAck drops a re-add of an episode-keyed item whose episode the operator already
// acknowledged; a new episode is a new item. It applies whatever AckBypassFix says.
func episodeAck(c ruleCtx) (SuppressionDecision, bool) {
	if c.shouldAdd() && episodeAcked(c.reason, c.episode, c.ackedEpisode) {
		return removeDecision("episodeAck"), true
	}
	return SuppressionDecision{}, false
}

// SkipReason says why shouldSkipSession excluded a session from attention detection.
type SkipReason uint8

const (
	SkipNone SkipReason = iota
	SkipHidden
	SkipSuspended
	SkipArchived
	SkipNotStarted
)

func (r SkipReason) String() string {
	switch r {
	case SkipHidden:
		return "hidden"
	case SkipSuspended:
		return "suspended"
	case SkipArchived:
		return "archived"
	case SkipNotStarted:
		return "not_started"
	}
	return "none"
}

var attentionSkipTotal = mustAttentionSkipCounter()

func mustAttentionSkipCounter() metric.Int64Counter {
	c, err := telemetry.GetMeter().Int64Counter("attention_skip_total",
		metric.WithDescription("Sessions the review-queue poller skipped by design, by skip_reason"))
	if err != nil {
		panic(err) // malformed instrument name: a build-time programmer error
	}
	return c
}

func countAttentionSkip(r SkipReason) {
	attentionSkipTotal.Add(context.Background(), 1, metric.WithAttributes(attribute.String("skip_reason", r.String())))
}

// suppressionExplainer makes the poller's silent decisions visible: one Debug line per
// suppression change, and one log line plus one attention_skip_total increment per first skip
// (or change of skip reason). The zero value logs and counts for real;
// tests replace the funcs.
type suppressionExplainer struct {
	debug     func(msg string, args ...any)
	info      func(msg string, args ...any)
	countSkip func(SkipReason)

	lastRule sync.Map // title -> rule name of the last decision ("" = keep)
	lastSkip sync.Map // title -> SkipReason of the last skip
}

func (e *suppressionExplainer) noteDecision(title string, d SuppressionDecision) {
	if prev, ok := e.lastRule.Swap(title, d.rule); (ok && prev == d.rule) || (!ok && d.rule == "") {
		return
	}
	if d.rule == "" {
		return
	}
	debug := e.debug
	if debug == nil {
		debug = log.Debug
	}
	debug("suppressed", "session", title, "rule", d.rule)
}

func (e *suppressionExplainer) noteSkip(title string, r SkipReason) {
	if r == SkipNone {
		e.lastSkip.Delete(title)
		return
	}
	if prev, ok := e.lastSkip.Swap(title, r); ok && prev == r {
		return
	}
	count := e.countSkip
	if count == nil {
		count = countAttentionSkip
	}
	count(r)
	info := e.info
	if info == nil {
		info = log.Info
	}
	info("skipping session", "session", title, "skip_reason", r.String())
}

func (e *suppressionExplainer) forget(title string) {
	e.lastRule.Delete(title)
	e.lastSkip.Delete(title)
}
