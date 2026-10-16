package tymux

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// A bare "\n" (Line Feed) moves a live terminal's cursor down a row without
// returning it to column 0, so a multi-row redraw broadcast with only "\n"
// between rows staggers/overlaps every row after the first on screen. This
// is the regression test for exactly that bug in applySnapshotResync.
func TestApplySnapshotResync_BroadcastsCRLFBetweenRows(t *testing.T) {
	s := &tymuxGRPCSession{fanout: NewClientFanout()}
	_, ch := s.fanout.Subscribe()

	snap := &v1.PaneSnapshot{
		Grid: []*v1.Row{
			row(cell("a", 0, 0, 0)),
			row(cell("b", 0, 0, 0)),
		},
	}

	require.NoError(t, s.applySnapshotResync(snap))

	select {
	case data := <-ch:
		assert.Equal(t, resyncClearAndHomeSeq+"a\r\nb", string(data))
	default:
		t.Fatal("expected a broadcast frame, got none")
	}
}
