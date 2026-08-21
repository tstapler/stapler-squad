package classifier

import "fmt"

// EscalationCategory classifies why a request was escalated for manual review,
// independent of which code path (classifier rule match, classifier no-match
// fallback, or the domain-age synthetic escalation) produced the escalation.
type EscalationCategory string

const (
	// EscalationNoMatch means no rule matched the request; the classifier's
	// default-escalate fallback applied.
	EscalationNoMatch EscalationCategory = "no-match"
	// EscalationExplicitRule means a named rule (seed, user, or claude-settings
	// sourced) explicitly matched and its Decision was Escalate.
	EscalationExplicitRule EscalationCategory = "explicit-rule"
	// EscalationDomainAge means the domain-age checker flagged a newly
	// registered domain referenced by the command.
	EscalationDomainAge EscalationCategory = "domain-age"
	// EscalationSecretScan means the plaintext secret scanner flagged the
	// command. (Secret scan results are normally AutoDeny, not Escalate, but
	// the RuleID is shared taxonomy so it is categorized here too.)
	EscalationSecretScan EscalationCategory = "secret-scan"
	// EscalationUnclassifiable means the command's actual executable could not
	// be statically determined (e.g. a shell-expansion program).
	EscalationUnclassifiable EscalationCategory = "unclassifiable"
	// EscalationUnexpected means an internal classifier bug produced a result
	// that doesn't fit any known escalation path — see RuleIDUnexpectedDecision.
	EscalationUnexpected EscalationCategory = "unexpected"
)

// Shared sentinel RuleID constants. These are the single source of truth for
// the RuleID literals emitted at the escalation/auto-deny call sites in
// approval_handler.go and classifier.go, so a future rename is a single edit
// instead of a silent categorization drift.
const (
	// RuleIDNewDomainCheck is emitted by the domain-age escalation branch in
	// approval_handler.go.
	RuleIDNewDomainCheck = "new-domain-check"
	// RuleIDSecretScan is emitted by the plaintext secret scan auto-deny
	// branch in approval_handler.go.
	RuleIDSecretScan = "secret-scan"
	// RuleIDShellExpansionProgram is emitted by classifier.go when a command's
	// program cannot be statically determined because it is a shell expansion.
	RuleIDShellExpansionProgram = "shell-expansion-program"
	// RuleIDUnexpectedDecision is synthetic — it is never emitted by the
	// classifier itself. It is set only by HandlePermissionRequest's default:
	// arm (Epic 2.1) when a ClassificationResult's Decision doesn't match any
	// expected value, to distinguish an internal classifier bug from a
	// genuine no-match coverage gap.
	RuleIDUnexpectedDecision = "internal-unexpected-decision"
)

// CategorizeEscalationRuleID maps a ClassificationResult's RuleID to its
// EscalationCategory. An empty RuleID is EscalationNoMatch. Any non-empty
// RuleID that isn't one of the known sentinel values falls back to
// EscalationExplicitRule — never a silent no-op — since named rules (seed,
// user, or claude-settings sourced) are the common case for an unrecognized
// RuleID.
func CategorizeEscalationRuleID(ruleID string) EscalationCategory {
	switch ruleID {
	case "":
		return EscalationNoMatch
	case RuleIDNewDomainCheck:
		return EscalationDomainAge
	case RuleIDSecretScan:
		return EscalationSecretScan
	case RuleIDShellExpansionProgram:
		return EscalationUnclassifiable
	case RuleIDUnexpectedDecision:
		return EscalationUnexpected
	default:
		return EscalationExplicitRule
	}
}

// EscalationReasonText returns the human-readable sentence explaining why a
// result was escalated. It returns result.Reason verbatim when present.
// When Reason is empty, it falls back to a category-aware sentence — an
// empty Reason on a real rule match (explicit-rule/domain-age/unclassifiable)
// must never render the no-match sentence, since that would falsely claim no
// rule matched.
func EscalationReasonText(result ClassificationResult) string {
	if result.Reason != "" {
		return result.Reason
	}
	switch CategorizeEscalationRuleID(result.RuleID) {
	case EscalationNoMatch:
		return "No approval rule matched this request — escalated to manual review by default."
	case EscalationUnexpected:
		return "An internal classification error occurred — review manually."
	default:
		// explicit-rule / domain-age / unclassifiable with a blank Reason: name the rule
		// rather than falsely claiming no rule matched.
		return fmt.Sprintf("Rule %q flagged this for review — no reason text was provided.", result.RuleID)
	}
}
