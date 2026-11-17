// Package gatefixturetest holds test fixtures for workflow-engine gate tests
// that live in both session and session/workflowengine. It imports neither.
package gatefixturetest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
)

// ACCriteriaAllDone is a serialized AcCriteriaJSON with every criterion done.
func ACCriteriaAllDone(t *testing.T) domain.AcCriteriaJSON {
	t.Helper()
	return acCriteria(t, domain.AcStatusDone, domain.AcStatusDone)
}

// ACCriteriaOneUnchecked is a serialized AcCriteriaJSON with the second criterion pending.
func ACCriteriaOneUnchecked(t *testing.T) domain.AcCriteriaJSON {
	t.Helper()
	return acCriteria(t, domain.AcStatusDone, domain.AcStatusPending)
}

func acCriteria(t *testing.T, statuses ...domain.AcStatus) domain.AcCriteriaJSON {
	t.Helper()
	criteria := make([]domain.AcCriterion, len(statuses))
	for i, s := range statuses {
		criteria[i] = domain.AcCriterion{Index: i, Text: []string{"criterion one", "criterion two"}[i], Status: s}
	}
	raw, err := domain.SerializeAcCriteria(criteria)
	require.NoError(t, err)
	return raw
}

// NewCustomTransitionWithGates creates a custom stage pair ("gate-from" ->
// "gate-to") plus one StageTransition carrying one gate per kind (stateful
// flags and order_index follow the slices), then calls invalidate so the
// engine's cache sees them. It returns the (from, to) pair.
func NewCustomTransitionWithGates(t *testing.T, client *ent.Client, invalidate func(context.Context) error, kinds []domain.GateKind, stateful []bool) (from, to domain.BacklogStatus) {
	t.Helper()
	ctx := context.Background()
	require.Equal(t, len(kinds), len(stateful), "test fixture bug: kinds/stateful length mismatch")

	fromStage, err := client.BacklogStage.Create().SetSlug("gate-from").SetName("Gate From").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("gate-to").SetName("Gate To").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	transition, err := client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toStage.ID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)

	for i, kind := range kinds {
		c := client.TransitionGate.Create().
			SetTransitionID(transition.ID).
			SetKind(string(kind)).
			SetStateful(stateful[i]).
			SetOrderIndex(i).
			SetEnabled(true)
		// Structural and custom gates fail closed without a config (unknown check id,
		// unregistered skill), so default to the ac_complete check and the one
		// pre-registered skill, matching what every caller of this helper wants.
		switch kind {
		case domain.GateKindStructural:
			c = c.SetConfig(map[string]interface{}{"check_id": domain.StructuralCheckACComplete})
		case domain.GateKindCustom:
			c = c.SetConfig(map[string]interface{}{"skill": "review-feasibility"})
		}
		_, err := c.Save(ctx)
		require.NoError(t, err)
	}

	require.NoError(t, invalidate(ctx))
	return domain.BacklogStatus(fromStage.Slug), domain.BacklogStatus(toStage.Slug)
}
