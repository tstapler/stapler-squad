package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/headless"
)

// gate_custom_check.go — Story 2.4.4's custom/pluggable check gate
// (ADR-003-custom-gate-check-execution-bound.md): InvokeCustomGateCheck runs a
// pre-registered skill/slash-command bounded by a LivenessDefinition, never
// arbitrary code. It reuses headless.Pool's existing bounded-call primitive
// (the same one TriggerTriage/review's headless paths use) rather than
// inventing a new execution mechanism.

// customCheckSystemPrompts maps each registeredCustomCheckSkills entry
// (session/gate_config.go, Epic 2.7) to the headless system prompt
// InvokeCustomGateCheck sends. This is the concrete realization of ADR-003's
// "invoke this named skill/slash-command" for a check with no interactive
// Claude Code session to run a real Skill-tool invocation inside — the
// headless call's system prompt IS the check's definition. Any key here must
// also be a key in registeredCustomCheckSkills; InvokeCustomGateCheck checks
// both maps (registeredCustomCheckSkills is the actual allowlist enforcement
// point — this map only supplies "what to ask," never "whether it's
// allowed").
var customCheckSystemPrompts = map[string]string{
	"review-feasibility": customCheckReviewFeasibilitySystemPrompt,
}

// customCheckReviewFeasibilitySystemPrompt backs the "review-feasibility"
// custom check — ADR-003's own running example (a idea -> ready transition
// requiring a feasibility-review PASS with no diff involved, since the item
// has no code yet to review).
const customCheckReviewFeasibilitySystemPrompt = `You are assessing whether a proposed backlog item is FEASIBLE to build as scoped. This is not a code review — no diff exists yet for this item. Read the item's title/description/acceptance criteria given in the user message and judge only: is this buildable, roughly scoped, and free of an obvious blocking unknown? Give a short rationale, then end your reply with exactly one line, verbatim: "VERDICT: PASS" or "VERDICT: FAIL".`

// CustomCheckOutcome is InvokeCustomGateCheck's completion result: the
// existing domain.ReviewOutcome PASS/FAIL/UNVERIFIABLE vocabulary (ADR-003 —
// "reported through the existing domain.ReviewOutcome... vocabulary, rendering
// through the same UI/verdict-aggregation code as an automated-review gate")
// plus the raw detail text recorded for audit/UI display.
type CustomCheckOutcome struct {
	Outcome ReviewOutcome
	Detail  string
}

// parseCustomCheckOutcome extracts a terminal "VERDICT: PASS/FAIL" line from a
// headless call's raw text output. Fail-closed: no recognized VERDICT line (a
// malformed, truncated, or off-script response) reports
// ReviewOutcomeUnverifiable, never a silent PASS.
func parseCustomCheckOutcome(output string) CustomCheckOutcome {
	upper := strings.ToUpper(output)
	switch {
	case strings.Contains(upper, "VERDICT: PASS"):
		return CustomCheckOutcome{Outcome: ReviewOutcomePass, Detail: strings.TrimSpace(output)}
	case strings.Contains(upper, "VERDICT: FAIL"):
		return CustomCheckOutcome{Outcome: ReviewOutcomeFail, Detail: strings.TrimSpace(output)}
	default:
		return CustomCheckOutcome{Outcome: ReviewOutcomeUnverifiable, Detail: strings.TrimSpace(output)}
	}
}

// CustomCheckCaller is the injectable seam InvokeCustomGateCheck uses to run a
// pre-registered skill via a bounded headless call — narrowed to
// *headless.Pool's CallBlocking signature so tests can substitute a fake
// without a real Pool/subprocess. Mirrors ReviewGateSpawner's role for the
// review gate (session/backlog_lifecycle.go).
type CustomCheckCaller interface {
	CallBlocking(ctx context.Context, key headless.FeatureKey, systemPrompt, userPrompt string, opts headless.CallOptions, sink headless.CostSink) (string, error)
}

var _ CustomCheckCaller = (*headless.Pool)(nil)

// InvokeCustomGateCheck runs the custom check named by cfg.SkillID for item,
// bounded by livenessDef (Epic 2.4.4, ADR-003). targetStage/mode identify the
// (stage, pipeline mode) pair reconcileCustomGateChecks (Task 2.4.4c) later
// re-resolves against the live LivenessEngine to detect an overdue
// invocation — captured here, at bind time, rather than re-derived from
// (item, gate) later, since neither the GateSatisfactionRecord schema nor
// TransitionGate carries a direct item/pipeline-mode join.
//
// Steps (Tasks 2.4.4a-b4, assembled in this order):
//  1. Reject cfg.SkillID if absent from registeredCustomCheckSkills
//     (session/gate_config.go) — fail-closed, defense in depth: ParseGateConfig
//     already enforces this at gate-save time (Task 2.7.2g), so reaching here
//     with an unregistered skill means a row predates that validation or the
//     allowlist has since shrunk. (Task 2.4.4a)
//  2. Record an in-flight GateSatisfactionRecord (Satisfied: false) BEFORE
//     starting the call, binding livenessDef's ExpectedDuration/
//     StalenessMargin plus targetStage/mode into OutcomeDetail. A Create
//     failure (e.g. ErrConflict — an invocation is already in flight for this
//     (item,gate) pair) is returned immediately, fail-closed, rather than
//     spawning a second concurrent call. (Task 2.4.4b2)
//  3. Runs the headless call bounded by a context.WithTimeout(ExpectedDuration).
//     A synchronous spawn/exec failure (non-zero exit, missing runtime
//     dependency, malformed environment) surfaces as this function's own
//     returned error — the caller blocks the transition and logs Warn — never
//     relying on the liveness sweep to detect it. (Task 2.4.4b1)
//  4. On completion (success, error, or the bounded timeout), records the
//     terminal verdict via Update, mirroring recordTerminalReviewVerdict's
//     PASS/FAIL/UNVERIFIABLE shape. (Task 2.4.4b3)
//
// The in-flight row from step 2 is also exactly what reconcileCustomGateChecks
// independently catches on its own periodic sweep tick if this call's own
// process never returns control to Go at all (e.g. a killed/orphaned server
// process outlives step 3's context) — the synchronous-failure path here and
// the sweep's liveness-timeout path are complementary, not mutually exclusive
// (see ADR-003's Decision section).
func InvokeCustomGateCheck(
	ctx context.Context,
	caller CustomCheckCaller,
	repo GateSatisfactionRepository,
	item *BacklogItemData,
	gateID uuid.UUID,
	cfg CustomCheckConfig,
	livenessDef LivenessDefinition,
	targetStage BacklogStatus,
	mode PipelineMode,
) (CustomCheckOutcome, error) {
	if !registeredCustomCheckSkills[cfg.SkillID] {
		return CustomCheckOutcome{}, fmt.Errorf("gate_custom_check: skill %q is not in the pre-registered allowlist", cfg.SkillID)
	}
	systemPrompt, ok := customCheckSystemPrompts[cfg.SkillID]
	if !ok {
		return CustomCheckOutcome{}, fmt.Errorf("gate_custom_check: skill %q has no configured system prompt", cfg.SkillID)
	}
	if caller == nil {
		return CustomCheckOutcome{}, fmt.Errorf("gate_custom_check: no CustomCheckCaller configured")
	}

	itemID, err := uuid.Parse(item.ID)
	if err != nil {
		return CustomCheckOutcome{}, fmt.Errorf("gate_custom_check: invalid item id %q: %w", item.ID, err)
	}

	if repo != nil {
		_, createErr := repo.Create(ctx, GateSatisfactionCreateInput{
			ItemID:    itemID,
			GateID:    gateID,
			Satisfied: false,
			OutcomeDetail: map[string]interface{}{
				"skill":                     cfg.SkillID,
				"expected_duration_seconds": livenessDef.ExpectedDuration.Seconds(),
				"staleness_margin_seconds":  livenessDef.StalenessMargin.Seconds(),
				"target_stage":              string(targetStage),
				"pipeline_mode":             string(mode),
			},
		})
		if createErr != nil {
			return CustomCheckOutcome{}, fmt.Errorf("gate_custom_check: record in-flight invocation: %w", createErr)
		}
	}

	callCtx := ctx
	if livenessDef.Kind == LivenessKindDurationBudget && livenessDef.ExpectedDuration > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, livenessDef.ExpectedDuration)
		defer cancel()
	}

	userPrompt := fmt.Sprintf("Title: %s\n\nDescription:\n%s", item.Title, item.Description)
	output, callErr := caller.CallBlocking(callCtx, headless.FeatureKey("gate-custom-check:"+cfg.SkillID), systemPrompt, userPrompt, headless.CallOptions{}, headless.DiscardCost)
	if callErr != nil {
		log.WarningLog().Printf("[GateCustomCheck] item=%s gate=%s skill=%s invocation failed: %v", item.ID, gateID, cfg.SkillID, callErr)
		recordCustomCheckTerminalOutcome(repo, itemID, gateID, false, map[string]interface{}{"error": callErr.Error()})
		return CustomCheckOutcome{}, fmt.Errorf("gate_custom_check: skill %q invocation failed: %w", cfg.SkillID, callErr)
	}

	outcome := parseCustomCheckOutcome(output)
	satisfied := outcome.Outcome == ReviewOutcomePass
	recordCustomCheckTerminalOutcome(repo, itemID, gateID, satisfied, map[string]interface{}{
		"outcome": string(outcome.Outcome),
		"detail":  outcome.Detail,
	})
	return outcome, nil
}

// recordCustomCheckTerminalOutcome updates the GateSatisfactionRecord for
// (itemID, gateID) with its final Satisfied value and detail, logging (never
// returning) an Update failure — mirrors every reconcile* sweep's
// best-effort write-failure handling in this package. Uses
// context.Background() rather than the (possibly already-timed-out) callCtx
// from the invocation itself, so a slow-but-completed call can still record
// its outcome even if its own bounded context expired first.
func recordCustomCheckTerminalOutcome(repo GateSatisfactionRepository, itemID, gateID uuid.UUID, satisfied bool, outcomeDetail map[string]interface{}) {
	if repo == nil {
		return
	}
	if _, err := repo.Update(context.Background(), itemID, gateID, GateSatisfactionUpdateInput{
		Satisfied:     &satisfied,
		OutcomeDetail: outcomeDetail,
	}); err != nil {
		log.ErrorLog().Printf("[GateCustomCheck] item=%s gate=%s: failed to record terminal outcome: %v", itemID, gateID, err)
	}
}

// customCheckInvocationAge is a small time.Since wrapper (test seam only in
// spirit — no override hook exists here since reconcileCustomGateChecks tests
// use real, deliberately-old CreatedAt fixture rows instead of mocking time).
func customCheckInvocationAge(createdAt time.Time) time.Duration {
	return time.Since(createdAt)
}
