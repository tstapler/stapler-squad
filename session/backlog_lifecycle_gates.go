package session

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// backlog_lifecycle_gates.go — Task 2.4.4c's reconcileCustomGateChecks: the
// stuck-detection sweep for an overdue InvokeCustomGateCheck invocation
// (session/gate_custom_check.go). Deliberately its own file rather than added
// to the already-large backlog_lifecycle.go — see plan.md's Task 2.4.4c file
// note (architecture-review/adversarial-review's file-growth concern) and the
// identical precedent already set for reconcileOrphanedTriageItems living in
// backlog_lifecycle_triage.go.
//
// Mirrors reconcileOrphanedTriageItems' LivenessEngine-consulting pattern
// (Epic 1.4, session/backlog_lifecycle_triage.go) exactly, applied to
// GateSatisfactionRecord rows instead of ItemSession rows: every unsatisfied
// (Satisfied: false) row IS an in-flight custom-check invocation — a
// human_approval gate never has a row until it IS satisfied
// (RecordGateApproval), so ListUnsatisfied's result set is exactly the
// custom-check population this sweep needs to scan (see
// GateSatisfactionRepository.ListUnsatisfied's own doc comment).

// reconcileCustomGateChecks marks StuckReasonGateTimeout (the Decision this
// Epic's plan.md section resolved) for any in-flight custom-check invocation
// whose age exceeds its bound LivenessDefinition's StalenessThreshold. The
// (ExpectedDuration, StalenessMargin, target stage, pipeline mode) bound at
// invocation time (InvokeCustomGateCheck's OutcomeDetail, Task 2.4.4b2) are
// used to re-resolve the definition live against l.livenessEngine — honoring
// a liveness override changed after the invocation started, exactly like
// reconcileOrphanedTriageItems' own live LivenessFor call. l.livenessEngine ==
// nil falls back to the bound ExpectedDuration+StalenessMargin values
// directly, unchanged from bind time.
//
// Best-effort: query/notify failures are logged, never returned. No resolve
// pass needed here for the same reason reconcileOrphanedTriageItems has none:
// selfHealStuck resolves this reason once the row is no longer unsatisfied
// (InvokeCustomGateCheck's own Update call records a terminal outcome) or the
// item leaves the gated status.
func (l *BacklogLifecycleListener) reconcileCustomGateChecks(ctx context.Context, er *EntRepository) {
	repo := l.getGateSatisfactionRepo()
	if repo == nil {
		return
	}

	records, err := repo.ListUnsatisfied(ctx)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcileCustomGateChecks ListUnsatisfied error: %v", err)
		return
	}

	for _, record := range records {
		threshold, ok := l.resolveCustomCheckStalenessThreshold(record)
		if !ok {
			continue // no bound duration budget recorded — nothing to compare against
		}
		age := customCheckInvocationAge(record.CreatedAt)
		if age <= threshold {
			continue // still within budget
		}

		itemIDStr := record.ItemID.String()
		item, itemErr := l.storage.GetBacklogItem(ctx, itemIDStr)
		if itemErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileCustomGateChecks GetBacklogItem item=%s: %v", itemIDStr, itemErr)
			continue
		}

		reasonDetail := fmt.Sprintf("custom gate check %s still open after %s (bound budget %s)", record.GateID, age.Round(time.Second), threshold)

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonGateTimeout, BacklogStatus(item.Status), reasonDetail)
		if markErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileCustomGateChecks MarkStuck item=%s: %v", item.ID, markErr)
			continue
		}
		if !applied {
			continue
		}

		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileCustomGateChecks FindOpenStuckStates item=%s: %v", item.ID, findErr)
			continue
		}
		row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonGateTimeout)
		if !ok || row.NotifiedAt != nil {
			continue
		}

		log.WarningLog().Printf("[BacklogLifecycle] item %s custom gate check %s timed out (%s)", item.ID, record.GateID, reasonDetail)
		l.notify(item.ID,
			"Custom gate check may be stuck",
			fmt.Sprintf("%s — a custom transition gate check has been running longer than expected. Investigate or re-run it.", item.Title),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonGateTimeout); notifyErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileCustomGateChecks MarkStuckNotified item=%s: %v", item.ID, notifyErr)
		}
	}
}

// resolveCustomCheckStalenessThreshold derives record's staleness threshold:
// preferentially the live l.livenessEngine.LivenessFor(targetStage, mode)
// resolution using the (target_stage, pipeline_mode) bound into
// record.OutcomeDetail at invocation time, falling back to the
// (expected_duration_seconds, staleness_margin_seconds) bound at that same
// time when no engine is wired or the live resolution reports IsNoTimeout.
// ok is false only when OutcomeDetail carries neither a resolvable engine
// value nor a bound fallback (a malformed/legacy row) — reconcileCustomGateChecks
// skips such a row rather than guessing a threshold.
func (l *BacklogLifecycleListener) resolveCustomCheckStalenessThreshold(record *GateSatisfactionData) (time.Duration, bool) {
	return resolveCustomCheckStalenessThresholdWithEngine(l.livenessEngine, record)
}

// resolveCustomCheckStalenessThresholdWithEngine implements
// resolveCustomCheckStalenessThreshold's logic as a free function so
// gate_custom_check_test.go can exercise it directly without constructing a
// full BacklogLifecycleListener.
func resolveCustomCheckStalenessThresholdWithEngine(engine LivenessEngine, record *GateSatisfactionData) (time.Duration, bool) {
	targetStage, hasStage := record.OutcomeDetail["target_stage"].(string)
	mode, _ := record.OutcomeDetail["pipeline_mode"].(string)
	if engine != nil && hasStage {
		if def, err := engine.LivenessFor(BacklogStatus(targetStage), PipelineMode(mode)); err == nil && !def.IsNoTimeout() {
			return def.StalenessThreshold(), true
		}
	}

	expectedSeconds, hasExpected := asFloat64(record.OutcomeDetail["expected_duration_seconds"])
	marginSeconds, hasMargin := asFloat64(record.OutcomeDetail["staleness_margin_seconds"])
	if !hasExpected || !hasMargin {
		return 0, false
	}
	return time.Duration(expectedSeconds+marginSeconds) * time.Second, true
}

// asFloat64 extracts a float64 from an interface{} value that may have
// round-tripped through JSON (ent's map[string]interface{} JSON column) as
// either a native float64 (an in-process, never-serialized value — e.g. a
// test fixture built as a Go literal) or json.Number/float64 after a real
// DB round-trip. ent's JSON field type decodes JSON numbers as float64 by
// default, so this covers the real-world case; the type switch exists so a
// malformed value degrades to "not present" (ok=false) rather than panicking.
func asFloat64(v interface{}) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}
