package services

// Story 5.1.3: runtime proofs for the stage and transition CRUD request fields.
// Each proof flips the field through the production RPC handler on a real
// BacklogService (enfService) and observes a different answer from either the
// operator transition handler or the graph validator the handlers run. Helpers
// carry the bkf prefix to stay clear of other test files.

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

const (
	bkfCustomStage session.BacklogStatus = "bkf_custom"
	bkfOtherStage  session.BacklogStatus = "bkf_other"
	bkfStageName                         = "display name"
)

// bkfCreateStage creates a stage through the CreateStage handler.
func bkfCreateStage(t *testing.T, svc *BacklogService, req *sessionv1.CreateStageRequest) string {
	t.Helper()
	resp, err := svc.CreateStage(t.Context(), connect.NewRequest(req))
	require.NoError(t, err)
	return resp.Msg.Item.Id
}

// bkfAddEdge creates an edge through CreateStageTransition and returns its error.
func bkfAddEdge(t *testing.T, svc *BacklogService, from, to session.BacklogStatus, enabled bool) error {
	t.Helper()
	_, err := svc.CreateStageTransition(t.Context(), connect.NewRequest(&sessionv1.CreateStageTransitionRequest{
		FromStageSlug: string(from), ToStageSlug: string(to), Enabled: enabled,
	}))
	return err
}

// bkfStageID returns the id of a seeded stage.
func bkfStageID(t *testing.T, repo *session.EntStageConfigRepository, slug session.BacklogStatus) string {
	t.Helper()
	stage, err := repo.GetStageBySlug(t.Context(), string(slug))
	require.NoError(t, err)
	return stage.ID.String()
}

// bkfOperatorCanMove reports whether the operator transition handler lets an
// item at `from` move to `to` after configure edited the stage config.
func bkfOperatorCanMove(t *testing.T, from, to session.BacklogStatus, configure func(*BacklogService, *session.EntStageConfigRepository)) bool {
	t.Helper()
	return enfBuiltTransition(t, from, to, configure)
}

// bkfUnrelatedEdgeAccepted reports whether the graph validator that
// CreateStageTransition runs accepts an unrelated edge, after configure edited
// the stage config.
func bkfUnrelatedEdgeAccepted(t *testing.T, configure func(*BacklogService, *session.EntStageConfigRepository)) bool {
	t.Helper()
	svc, _, repo, _ := enfService(t)
	configure(svc, repo)
	return bkfAddEdge(t, svc, session.BacklogStatusQueued, session.BacklogStatusRefining, true) == nil
}

func TestConfigSurfaceProof_CreateStage_ChangesHandlerAnswer(t *testing.T) {
	ready := session.BacklogStatusReady
	createWithEdge := func(slug session.BacklogStatus, enabled bool) func(*BacklogService, *session.EntStageConfigRepository) {
		return func(svc *BacklogService, _ *session.EntStageConfigRepository) {
			bkfCreateStage(t, svc, &sessionv1.CreateStageRequest{Slug: string(slug), Name: bkfStageName, IsEntry: true, IsTerminal: true, Enabled: enabled})
			require.NoError(t, bkfAddEdge(t, svc, slug, ready, true))
		}
	}
	custom := bkfCustomStage

	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfOperatorCanMove(t, custom, ready, createWithEdge(bkfCustomStage, setting.(bool)))
	}, false, true) // enabled
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfOperatorCanMove(t, custom, ready, createWithEdge(setting.(session.BacklogStatus), true))
	}, bkfOtherStage, bkfCustomStage) // slug
}

func TestConfigSurfaceProof_CreateStage_IsEntryAndIsTerminal_ChangeGraphValidation(t *testing.T) {
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfUnrelatedEdgeAccepted(t, func(svc *BacklogService, _ *session.EntStageConfigRepository) {
			bkfCreateStage(t, svc, &sessionv1.CreateStageRequest{Slug: string(bkfCustomStage), Name: bkfStageName, IsEntry: setting.(bool), IsTerminal: true, Enabled: true})
		})
	}, false, true) // is_entry: an unreachable stage fails validation
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfUnrelatedEdgeAccepted(t, func(svc *BacklogService, _ *session.EntStageConfigRepository) {
			bkfCreateStage(t, svc, &sessionv1.CreateStageRequest{Slug: string(bkfCustomStage), Name: bkfStageName, IsEntry: true, IsTerminal: setting.(bool), Enabled: true})
		})
	}, false, true) // is_terminal: a non-terminal stage with no way out fails validation
}

func TestConfigSurfaceProof_UpdateStage_ChangesHandlerAnswer(t *testing.T) {
	done, refining, review := session.BacklogStatusDone, session.BacklogStatusRefining, session.BacklogStatusReview
	disable := func(slug session.BacklogStatus, enabled bool) func(*BacklogService, *session.EntStageConfigRepository) {
		return func(svc *BacklogService, repo *session.EntStageConfigRepository) {
			_, err := svc.UpdateStage(t.Context(), connect.NewRequest(&sessionv1.UpdateStageRequest{Id: bkfStageID(t, repo, slug), Enabled: &enabled}))
			require.NoError(t, err)
		}
	}

	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfOperatorCanMove(t, done, refining, disable(refining, setting.(bool)))
	}, false, true) // enabled
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfOperatorCanMove(t, done, refining, disable(setting.(session.BacklogStatus), false))
	}, refining, review) // id: which stage the update targets
}

func TestConfigSurfaceProof_UpdateStage_IsEntryAndIsTerminal_ChangeGraphValidation(t *testing.T) {
	update := func(svc *BacklogService, entry, terminal *bool) {
		id := bkfCreateStage(t, svc, &sessionv1.CreateStageRequest{Slug: string(bkfCustomStage), Name: bkfStageName, IsEntry: true, IsTerminal: true, Enabled: true})
		_, err := svc.UpdateStage(t.Context(), connect.NewRequest(&sessionv1.UpdateStageRequest{Id: id, IsEntry: entry, IsTerminal: terminal}))
		require.NoError(t, err)
	}
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfUnrelatedEdgeAccepted(t, func(svc *BacklogService, _ *session.EntStageConfigRepository) {
			v := setting.(bool)
			update(svc, &v, nil)
		})
	}, false, true) // is_entry
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfUnrelatedEdgeAccepted(t, func(svc *BacklogService, _ *session.EntStageConfigRepository) {
			v := setting.(bool)
			update(svc, nil, &v)
		})
	}, false, true) // is_terminal
}

func TestConfigSurfaceProof_CreateStageTransition_ChangesHandlerAnswer(t *testing.T) {
	queued, refining, review, idea := session.BacklogStatusQueued, session.BacklogStatusRefining, session.BacklogStatusReview, session.BacklogStatusIdea
	create := func(from, to session.BacklogStatus, enabled bool) func(*BacklogService, *session.EntStageConfigRepository) {
		return func(svc *BacklogService, _ *session.EntStageConfigRepository) {
			require.NoError(t, bkfAddEdge(t, svc, from, to, enabled))
		}
	}

	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfOperatorCanMove(t, queued, refining, create(queued, refining, setting.(bool)))
	}, false, true) // enabled
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfOperatorCanMove(t, queued, refining, create(queued, setting.(session.BacklogStatus), true))
	}, review, refining) // to_stage_slug
	assertConfigChangesBehavior(t, func(setting any) any {
		return bkfOperatorCanMove(t, queued, review, create(setting.(session.BacklogStatus), review, true))
	}, idea, queued) // from_stage_slug
}
