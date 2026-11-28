package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session/domain"
)

// T-IX-16 (index half): a hidden review/triage/diagnose spawn is resolvable by
// the gate, with its kind, as soon as CreateDirectorySession returns.
func TestCreateDirectorySession_ShouldUpsertHiddenKind_WhenDiagnoseTriageOrReviewSpawn(t *testing.T) {
	cases := []struct {
		tag  string
		want deliverygate.HiddenKind
	}{
		{"backlog:review", deliverygate.KindReview},
		{"backlog:triage", deliverygate.KindTriage},
		{"backlog:diagnose", deliverygate.KindDiagnose},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			svc, _, gate := newCountingGateService(t)
			title := "gate-index-" + string(c.want)
			inst, err := svc.CreateDirectorySession(context.Background(), t.TempDir(), domain.SessionSpawnOptions{
				Title: title, OneShot: true, Hidden: true, Tags: []string{c.tag},
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = inst.Destroy() })

			for _, key := range []string{title, inst.Snapshot().UUID} {
				res := gate.Resolver().Resolve(key, nil)
				assert.Equal(t, deliverygate.VisibilityHidden, res.Visibility, key)
				assert.Equal(t, c.want, res.Kind, key)
			}
		})
	}
}
