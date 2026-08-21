package classifier

import (
	"strings"
	"testing"
)

func TestCategorizeEscalationRuleID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want EscalationCategory
	}{
		{"empty ruleID is no-match", "", EscalationNoMatch},
		{"named rule is explicit-rule", "seed-escalate-git-branch-safe-delete", EscalationExplicitRule},
		{"domain-age sentinel", RuleIDNewDomainCheck, EscalationDomainAge},
		{"secret-scan sentinel", RuleIDSecretScan, EscalationSecretScan},
		{"shell-expansion-program sentinel", RuleIDShellExpansionProgram, EscalationUnclassifiable},
		{"unexpected-decision sentinel", RuleIDUnexpectedDecision, EscalationUnexpected},
		{"unknown ruleID falls back to explicit-rule", "some-future-rule-id-nobody-has-seen", EscalationExplicitRule},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CategorizeEscalationRuleID(tt.in)
			if got != tt.want {
				t.Errorf("CategorizeEscalationRuleID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEscalationReasonText(t *testing.T) {
	t.Run("non-empty Reason passthrough", func(t *testing.T) {
		result := ClassificationResult{
			Decision: Escalate,
			RuleID:   "",
			Reason:   "No matching rule; escalated for manual review.",
		}
		got := EscalationReasonText(result)
		want := "No matching rule; escalated for manual review."
		if got != want {
			t.Errorf("EscalationReasonText(%+v) = %q, want %q", result, got, want)
		}
	})

	t.Run("zero-value result falls back to no-match sentence", func(t *testing.T) {
		result := ClassificationResult{}
		got := EscalationReasonText(result)
		want := "No approval rule matched this request — escalated to manual review by default."
		if got != want {
			t.Errorf("EscalationReasonText(%+v) = %q, want %q", result, got, want)
		}
	})

	t.Run("blank Reason on explicit-rule match names the rule, not no-match", func(t *testing.T) {
		result := ClassificationResult{
			Decision: Escalate,
			RuleID:   "custom-rule",
			Reason:   "",
		}
		got := EscalationReasonText(result)
		want := `Rule "custom-rule" flagged this for review — no reason text was provided.`
		if got != want {
			t.Errorf("EscalationReasonText(%+v) = %q, want %q", result, got, want)
		}
		if strings.Contains(got, "No approval rule matched") {
			t.Errorf("EscalationReasonText(%+v) = %q, must not contain %q (pre-mortem P1 regression)", result, got, "No approval rule matched")
		}
	})

	t.Run("blank Reason on unexpected-decision sentinel yields internal-error sentence", func(t *testing.T) {
		result := ClassificationResult{
			RuleID: RuleIDUnexpectedDecision,
			Reason: "",
		}
		got := EscalationReasonText(result)
		want := "An internal classification error occurred — review manually."
		if got != want {
			t.Errorf("EscalationReasonText(%+v) = %q, want %q", result, got, want)
		}
	})
}
