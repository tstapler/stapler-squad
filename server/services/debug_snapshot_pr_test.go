package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
)

func TestDebugSnapshot_ShouldDumpEveryPRRow_WhenSessionHasSeveral(t *testing.T) {
	inst := multiPRInstance(t, "dbg", "",
		prViewFor(t, 11, session.PRStateMerged, "success"),
		prViewFor(t, 12, session.PRStateOpen, ciConclusionFailure))

	got, _ := collectSessionSnapshots(context.Background(), []*session.Instance{inst})

	if assert.Len(t, got, 1) {
		assert.ElementsMatch(t,
			[]string{"github.com/acme/app#11 merged", "github.com/acme/app#12 open"},
			got[0].PullRequests)
	}
}
