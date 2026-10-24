package session

import (
	"time"

	"github.com/tstapler/stapler-squad/log"
)

const (
	// graceAfterAck is how long after an acknowledgment low-priority or controller-less
	// re-adds are dropped.
	graceAfterAck = 5 * time.Minute
	// minReAddInterval limits how soon an already-queued session may be re-added.
	minReAddInterval = 2 * time.Minute
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
	// episodeChanged marks an item whose episode differs from the queued one; always
	// false until episode ids exist.
	episodeChanged bool
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

// ackSnooze drops items the user acknowledged after the last output. Active high-priority
// adds bypass it: content-signature dedup leaves LastMeaningfulOutput unchanged when an
// identical prompt reappears, so the ack would otherwise hide a live blocking prompt.
func ackSnooze(c ruleCtx) (SuppressionDecision, bool) {
	activeHighPriority := c.shouldAdd() && c.priority <= PriorityHigh
	if !activeHighPriority && c.ackedAfterOutput {
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
// or a new episode passes. A restart empties the queue, so a missing item is never limited.
func readdLimit(c ruleCtx) (SuppressionDecision, bool) {
	if !c.shouldAdd() || c.lastAddedToQueue.IsZero() || c.now.Sub(c.lastAddedToQueue) >= minReAddInterval {
		return SuppressionDecision{}, false
	}
	existing, queued := c.existing()
	if !queued {
		return SuppressionDecision{}, false
	}
	// Lower priority number = higher priority (Urgent=1 > High=2 > Medium=3 > Low=4)
	if c.priority < existing.Priority || c.episodeChanged {
		log.Debug("escalation, bypassing rate limit", "from", existing.Priority.String(), "to", c.priority.String())
		return SuppressionDecision{}, false
	}
	return suppressDecision("readdLimit"), true
}
