package services

// Story 5.1.3: runtime proofs for the transition-gate CRUD request fields. Each
// proof flips the field through CreateTransitionGate or UpdateTransitionGate on
// a real BacklogService and observes the operator transition handler's answer
// (allowed, or the text of the first unsatisfied gate it names).

import (
	"regexp"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

const (
	bkfCheckKey       = "check_id"
	bkfCheckNoBlocker = session.StructuralCheckNoOpenBlockers
	bkfCheckPRGreen   = session.StructuralCheckPRGreen
	bkfCheckACDone    = session.StructuralCheckACComplete
)

// Text of the two denials the order proofs tell apart.
const (
	bkfACDenial = "no acceptance criteria defined"
	bkfPRDenial = "pr_green structural check"
)

// bkfGate is the writable surface of one transition gate.
type bkfGate struct {
	kind    session.GateKind
	checkID string
	order   int32
	enabled bool
}

func bkfStructural(checkID string, order int32) bkfGate {
	return bkfGate{kind: session.GateKindStructural, checkID: checkID, order: order, enabled: true}
}

func bkfApproval() bkfGate {
	return bkfGate{kind: session.GateKindHumanApproval, enabled: true}
}

func (g bkfGate) config() map[string]string {
	if g.checkID == "" {
		return nil
	}
	return map[string]string{bkfCheckKey: g.checkID}
}

func bkfCreateGate(t *testing.T, svc *BacklogService, edgeID string, g bkfGate) string {
	t.Helper()
	resp, err := svc.CreateTransitionGate(t.Context(), connect.NewRequest(&sessionv1.CreateTransitionGateRequest{
		TransitionId: edgeID, Kind: string(g.kind), Config: g.config(), OrderIndex: g.order, Enabled: g.enabled,
	}))
	require.NoError(t, err)
	return resp.Msg.Item.Id
}

func bkfUpdateGate(t *testing.T, svc *BacklogService, id string, g bkfGate) {
	t.Helper()
	order, enabled := g.order, g.enabled
	_, err := svc.UpdateTransitionGate(t.Context(), connect.NewRequest(&sessionv1.UpdateTransitionGateRequest{
		Id: id, Kind: string(g.kind), Config: g.config(), OrderIndex: &order, Enabled: &enabled,
	}))
	require.NoError(t, err)
}

// bkfUUID strips the per-run gate ids from a denial so two runs that deny for the
// same reason compare equal.
var bkfUUID = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// bkfGatedAnswer builds an enforced service, lets configure edit the
// idea->refining edge, then returns the operator handler's answer for an item
// at idea: "ok", or the error text (which names the first unsatisfied gate).
func bkfGatedAnswer(t *testing.T, configure func(svc *BacklogService, edgeID string)) string {
	t.Helper()
	svc, storage, repo, _ := enfService(t)
	edge := enfEdgeID(t, repo, session.BacklogStatusIdea, session.BacklogStatusRefining)
	configure(svc, edge)
	item := enfItemAt(t, storage, session.BacklogStatusIdea)
	return bkfUUID.ReplaceAllString(errText(enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, "")), "<id>")
}

func TestConfigSurfaceProof_CreateTransitionGate_KindConfigAndOrder_ChangeHandlerAnswer(t *testing.T) {
	create := func(gates ...bkfGate) func(*BacklogService, string) {
		return func(svc *BacklogService, edge string) {
			for _, g := range gates {
				bkfCreateGate(t, svc, edge, g)
			}
		}
	}

	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfGatedAnswer(t, create(setting.(bkfGate)))
	}, bkfApproval(), bkfStructural(bkfCheckNoBlocker, 0)) // kind
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfGatedAnswer(t, create(bkfStructural(setting.(string), 0)))
	}, bkfCheckPRGreen, bkfCheckNoBlocker) // config
	byOrder := func(setting any) any {
		return bkfGatedAnswer(t, create(bkfStructural(bkfCheckACDone, setting.(int32)), bkfStructural(bkfCheckPRGreen, 1)))
	}
	assertConfigChangesBehavior(t, byOrder, int32(0), int32(2)) // order_index
	require.Contains(t, byOrder(int32(0)), bkfACDenial, "the lowest order_index gate is the one the denial names")
	require.Contains(t, byOrder(int32(2)), bkfPRDenial)
}

// bkfDisableGate creates the approval and no-blockers gates, then disables the
// one named by which through UpdateTransitionGate.
func bkfDisableGate(t *testing.T, svc *BacklogService, edge string, which bkfGateName) {
	t.Helper()
	gates := map[bkfGateName]bkfGate{bkfApprovalGate: bkfApproval(), bkfBlockersGate: bkfStructural(bkfCheckNoBlocker, 1)}
	ids := map[bkfGateName]string{}
	for name, g := range gates {
		ids[name] = bkfCreateGate(t, svc, edge, g)
	}
	disabled := gates[which]
	disabled.enabled = false
	bkfUpdateGate(t, svc, ids[which], disabled)
}

type bkfGateName string

const (
	bkfApprovalGate bkfGateName = "approval"
	bkfBlockersGate bkfGateName = "blockers"
)

func TestConfigSurfaceProof_UpdateTransitionGate_EnabledAndID_ChangeHandlerAnswer(t *testing.T) {
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfGatedAnswer(t, func(svc *BacklogService, edge string) {
			id := bkfCreateGate(t, svc, edge, bkfApproval())
			g := bkfApproval()
			g.enabled = setting.(bool)
			bkfUpdateGate(t, svc, id, g)
		})
	}, true, false) // enabled
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfGatedAnswer(t, func(svc *BacklogService, edge string) {
			bkfDisableGate(t, svc, edge, setting.(bkfGateName))
		})
	}, bkfApprovalGate, bkfBlockersGate) // id: which gate the update targets
}

func TestConfigSurfaceProof_UpdateTransitionGate_KindConfigAndOrder_ChangeHandlerAnswer(t *testing.T) {
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfGatedAnswer(t, func(svc *BacklogService, edge string) {
			id := bkfCreateGate(t, svc, edge, bkfStructural(bkfCheckNoBlocker, 0))
			bkfUpdateGate(t, svc, id, setting.(bkfGate))
		})
	}, bkfStructural(bkfCheckNoBlocker, 0), bkfApproval()) // kind
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfGatedAnswer(t, func(svc *BacklogService, edge string) {
			id := bkfCreateGate(t, svc, edge, bkfStructural(bkfCheckNoBlocker, 0))
			bkfUpdateGate(t, svc, id, bkfStructural(setting.(string), 0))
		})
	}, bkfCheckNoBlocker, bkfCheckPRGreen) // config
	byOrder := func(setting any) any {
		return bkfGatedAnswer(t, func(svc *BacklogService, edge string) {
			acGate := bkfCreateGate(t, svc, edge, bkfStructural(bkfCheckACDone, 0))
			bkfCreateGate(t, svc, edge, bkfStructural(bkfCheckPRGreen, 1))
			bkfUpdateGate(t, svc, acGate, bkfStructural(bkfCheckACDone, setting.(int32)))
		})
	}
	assertConfigChangesBehavior(t, byOrder, int32(0), int32(2)) // order_index
	require.Contains(t, byOrder(int32(0)), bkfACDenial, "the lowest order_index gate is the one the denial names")
	require.Contains(t, byOrder(int32(2)), bkfPRDenial)
}
