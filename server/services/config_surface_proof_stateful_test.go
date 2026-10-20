package services

// Story 5.1.4: runtime proofs for the transition-gate `stateful` request field.
// A structural gate with stateful=true latches once a transition commits through
// it, so a later regression of the item no longer denies; stateful=false keeps
// recomputing. A recorded kind is always stateful and rejects an explicit false.

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

const (
	stfACDone    = `[{"index":0,"text":"criterion","status":"done"}]`
	stfACPending = `[{"index":0,"text":"criterion","status":"pending"}]`
)

func stfCreateGate(t *testing.T, svc *BacklogService, edge string, g bkfGate, stateful bool) (*sessionv1.CreateTransitionGateResponse, error) {
	t.Helper()
	resp, err := svc.CreateTransitionGate(t.Context(), connect.NewRequest(&sessionv1.CreateTransitionGateRequest{
		TransitionId: edge, Kind: string(g.kind), Config: g.config(), Stateful: stateful, Enabled: true,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func stfUpdateStateful(t *testing.T, svc *BacklogService, id string, g bkfGate, stateful bool) error {
	t.Helper()
	_, err := svc.UpdateTransitionGate(t.Context(), connect.NewRequest(&sessionv1.UpdateTransitionGateRequest{
		Id: id, Kind: string(g.kind), Config: g.config(), Stateful: &stateful,
	}))
	return err
}

// stfRegressedAnswer commits idea->refining through an ac_complete gate whose
// stateful flag configure sets, moves the item back, regresses its acceptance
// criteria, and returns the second attempt's answer.
func stfRegressedAnswer(t *testing.T, configure func(svc *BacklogService, edge string)) string {
	t.Helper()
	svc, storage, repo, _ := enfService(t)
	configure(svc, enfEdgeID(t, repo, session.BacklogStatusIdea, session.BacklogStatusRefining))
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "stateful item", Status: string(session.BacklogStatusIdea), AcceptanceCriteria: session.AcCriteriaJSON(stfACDone),
	})
	require.NoError(t, err)

	require.NoError(t, enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, ""), "first attempt satisfies the gate")
	_, err = storage.Rollback(t.Context(), item.ID, session.BacklogStatusIdea,
		&session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusRefining)}, "test")
	require.NoError(t, err)
	pending := session.AcCriteriaJSON(stfACPending)
	_, err = storage.UpdateBacklogItem(t.Context(), item.ID, session.BacklogItemUpdate{AcceptanceCriteria: &pending}, nil)
	require.NoError(t, err)

	return bkfUUID.ReplaceAllString(errText(enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, "")), "<id>")
}

func TestConfigSurfaceProof_CreateTransitionGate_Stateful_ChangesHandlerAnswer(t *testing.T) {
	assertConfigChangesBehavior(t, func(setting any) any {
		return stfRegressedAnswer(t, func(svc *BacklogService, edge string) {
			_, err := stfCreateGate(t, svc, edge, bkfStructural(bkfCheckACDone, 0), setting.(bool))
			require.NoError(t, err)
		})
	}, false, true)
}

func TestConfigSurfaceProof_CreateTransitionGate_Stateful_RecordedKindIsAlwaysStateful(t *testing.T) {
	svc, _, repo, _ := enfService(t)
	edge := enfEdgeID(t, repo, session.BacklogStatusIdea, session.BacklogStatusRefining)
	resp, err := stfCreateGate(t, svc, edge, bkfApproval(), false)
	require.NoError(t, err)
	require.True(t, resp.Item.Stateful, "a recorded kind reports its effective statefulness")
}

func TestConfigSurfaceProof_UpdateTransitionGate_Stateful_ChangesHandlerAnswer(t *testing.T) {
	assertConfigChangesBehavior(t, func(setting any) any {
		return stfRegressedAnswer(t, func(svc *BacklogService, edge string) {
			resp, err := stfCreateGate(t, svc, edge, bkfStructural(bkfCheckACDone, 0), !setting.(bool))
			require.NoError(t, err)
			require.NoError(t, stfUpdateStateful(t, svc, resp.Item.Id, bkfStructural(bkfCheckACDone, 0), setting.(bool)))
		})
	}, false, true)
}

func TestConfigSurfaceProof_UpdateTransitionGate_Stateful_RejectsFalseOnRecordedKind(t *testing.T) {
	svc, _, repo, _ := enfService(t)
	edge := enfEdgeID(t, repo, session.BacklogStatusIdea, session.BacklogStatusRefining)
	resp, err := stfCreateGate(t, svc, edge, bkfApproval(), true)
	require.NoError(t, err)

	err = stfUpdateStateful(t, svc, resp.Item.Id, bkfApproval(), false)
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, enfConnectCode(t, err))
	require.Contains(t, err.Error(), "human_approval gates are always stateful")
	got, err := svc.GetTransitionGate(t.Context(), connect.NewRequest(&sessionv1.GetTransitionGateRequest{Id: resp.Item.Id}))
	require.NoError(t, err)
	require.True(t, got.Msg.Item.Stateful, "the rejected update changed nothing")
}
