package classifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscalationCategory_should_BeHumanOnly_When_DomainAgeOrUnexpected(t *testing.T) {
	t.Parallel()
	for ruleID, want := range map[string]bool{
		RuleIDNewDomainCheck:        true,
		RuleIDUnexpectedDecision:    true,
		"":                          false,
		"seed-escalate-gh-api":      false,
		RuleIDSecretScan:            false,
		RuleIDShellExpansionProgram: false,
	} {
		assert.Equal(t, want, CategorizeEscalationRuleID(ruleID).HumanOnly(), "rule %q", ruleID)
	}
}
