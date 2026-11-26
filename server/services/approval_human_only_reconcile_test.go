package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/testutil"
)

// reconcileWithBashRule runs one synchronous reconciliation pass over store, with a rule
// that decides every Bash request as decision.
func reconcileWithBashRule(t *testing.T, store *approvalkit.ApprovalStore, decision classifier.ClassificationDecision) {
	t.Helper()
	rules := newRulesService(t)
	rules.SetApprovalService(NewApprovalService(store))
	rules.classifier.ReplaceRules(append(rules.classifier.Rules(), classifier.Rule{
		RuleMeta: classifier.RuleMeta{ID: "all-bash", Name: "All Bash", Enabled: true, Source: "user", Priority: 999},
		ToolName: "Bash", Decision: decision,
	}))
	rules.reconcilePendingApprovalsSafe()
}

func categorizedApproval(category classifier.EscalationCategory) *approvalkit.PendingApproval {
	a := bashApproval("appr-"+string(category), "sess", "git status", time.Now())
	a.EscalationCategory = string(category)
	return a
}

func TestReconcilePendingApprovals_should_ResolveByCategory_When_ARuleNowDecides(t *testing.T) {
	t.Parallel()
	for category, tc := range map[classifier.EscalationCategory]struct {
		decision    classifier.ClassificationDecision
		wantPending bool
	}{
		classifier.EscalationDomainAge:    {decision: classifier.AutoAllow, wantPending: true},
		classifier.EscalationUnexpected:   {decision: classifier.AutoAllow, wantPending: true},
		classifier.EscalationNoMatch:      {decision: classifier.AutoAllow},
		classifier.EscalationExplicitRule: {decision: classifier.AutoAllow},
		"":                                {decision: classifier.AutoAllow},
	} {
		t.Run("allow rule, category "+string(category), func(t *testing.T) {
			t.Parallel()
			store := approvalkit.NewApprovalStore("")
			a := categorizedApproval(category)
			require.NoError(t, store.Create(a))

			reconcileWithBashRule(t, store, tc.decision)

			_, pending := store.Get(a.ID)
			assert.Equal(t, tc.wantPending, pending)
			assert.Equal(t, tc.wantPending, len(a.Decisions()) == 0, "a held approval receives no decision")
		})
	}
	for _, category := range []classifier.EscalationCategory{classifier.EscalationDomainAge, classifier.EscalationUnexpected} {
		t.Run("deny rule, category "+string(category), func(t *testing.T) {
			t.Parallel()
			store := approvalkit.NewApprovalStore("")
			a := categorizedApproval(category)
			require.NoError(t, store.Create(a))

			reconcileWithBashRule(t, store, classifier.AutoDeny)

			require.Len(t, a.Decisions(), 1)
			assert.Equal(t, "deny", (<-a.Decisions()).Behavior)
		})
	}
}

// What the handler queues is what reconciliation holds.
func TestReconcilePendingApprovals_should_HoldWhatTheHandlerQueuedForAHuman_When_ARuleNowAllows(t *testing.T) {
	t.Parallel()
	for name, tc := range humanOnlyCases() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, store := newTestHandler(5 * time.Second)
			h.SetClassifier(&countingClassifier{decision: tc.rules})
			h.SetDomainChecker(newDomainChecker(5 * 24 * time.Hour))
			answered := make(chan string, 1)
			go func() {
				answered <- decisionBehavior(t, postPermissionRequestWithCommand(t, h, "s", "Bash", tc.command))
			}()
			require.NoError(t, testutil.WaitForCondition(func() bool { return len(store.ListAll()) == 1 }, testutil.FastWaitConfig()))

			reconcileWithBashRule(t, store, classifier.AutoAllow)

			require.Len(t, store.ListAll(), 1)
			require.NoError(t, store.Resolve(store.ListAll()[0].ID, approvalkit.ApprovalDecision{Behavior: "deny"}))
			assert.Equal(t, "deny", <-answered)
		})
	}
}
