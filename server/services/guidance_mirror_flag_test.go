package services

import (
	"context"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
)

func postVerifiedQuestion(q *questionHarness) (title, message string, meta map[string]string) {
	ev := q.post(askQuestionBody, map[string]string{hookProofHeader: q.proofs.HeaderValue("sess-uuid")})
	meta = maps.Clone(ev.NotificationMetadata)
	delete(meta, metaQuestionID) // a random id in the legacy path
	return ev.NotificationTitle, ev.NotificationMessage, meta
}

func TestGuidanceMirror_should_WriteNothingAndMatchLegacyOutput_When_StructuredFlagOff(t *testing.T) {
	withIsolatedStateDir(t) // no config: askuserquestion_structured defaults off

	legacy := newQuestionHarness(t) // no mirror wired at all
	lTitle, lMsg, lMeta := postVerifiedQuestion(legacy)

	mirrored := newQuestionHarness(t)
	storage := createTestStorage(t)
	m := NewGuidanceHookMirror(storage, mirrored.store, nil, nil) // production default gate
	mirrored.h.SetGuidanceMirror(m)
	title, msg, meta := postVerifiedQuestion(mirrored)

	assert.Equal(t, lTitle, title)
	assert.Equal(t, lMsg, msg)
	assert.Equal(t, lMeta, meta)
	assert.NotContains(t, meta, metaGuidanceRequestID)
	rows, err := storage.ListAllPendingGuidanceRequests(context.Background())
	require.NoError(t, err)
	assert.Empty(t, rows, "flag off writes no durable row")

	_, ok := m.Mirror(mirrorReq(t, "toolu_off", "Off?", true))
	assert.False(t, ok)
}

func TestGuidanceMirror_should_ReadTheFlagLivePerCall_When_ItFlipsAfterConstruction(t *testing.T) {
	withIsolatedStateDir(t)
	storage := createTestStorage(t)
	m := NewGuidanceHookMirror(storage, NewPendingQuestionStore(nil), nil, nil)
	_, ok := m.Mirror(mirrorReq(t, "toolu_a", "First?", true))
	require.False(t, ok)

	require.NoError(t, config.LoadConfig().SetFeatureFlagScope(askUserQuestionStructuredFlagName, scopeStageRender, true))

	id, ok := m.Mirror(mirrorReq(t, "toolu_b", "Second?", true))
	require.True(t, ok)
	rows, err := storage.ListAllPendingGuidanceRequests(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, id, rows[0].ID)
}
