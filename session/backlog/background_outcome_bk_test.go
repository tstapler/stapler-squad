package backlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
)

// knownEndReasonProducers lists every end_reason value passed to
// UpdateItemSessionEndedWithReason / UpdateItemSessionEndedWithDetail
// (directly or via failJulesSession / classifyHeadlessCallError /
// stopGuardrailSession), found with:
//
//	git grep -n "UpdateItemSessionEndedWith\|failJulesSession(" -- '*.go' ':!*_test.go'
var knownEndReasonProducers = []string{
	"", "shutdown", "pool_saturated", "idle", "timeout", "claude_not_found",
	"subprocess_start_error", TriageEndReasonFanoutCeiling, TriageEndReasonCostCeiling, TriageEndReasonOther,
	"parse_error", "dispatch_failed", "dispatch_incomplete", "jules_timed_out",
	"jules_session_missing", "jules_failed", "jules_completed", "jules_completed_no_pr",
	"cost_budget_exceeded", "idle_wait_loop_escalation",
}

func TestEndReasonClassification_ShouldCoverEveryKnownProducer(t *testing.T) {
	t.Parallel()
	for _, r := range knownEndReasonProducers {
		_, fail := session.FailureEndReasons[r]
		_, non := session.NonFailureEndReasons[r]
		assert.Truef(t, fail != non, "end_reason %q must be in exactly one of the failure/non-failure maps", r)
	}
	for r := range session.FailureEndReasons {
		assert.NotContains(t, session.NonFailureEndReasons, r)
	}
}
