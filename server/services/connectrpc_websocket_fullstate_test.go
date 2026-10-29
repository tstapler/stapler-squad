package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/streamhub"
)

// redrawPanePTY is a fakePanePTY whose backend also provides a full-state redraw (the tymux case).
type redrawPanePTY struct {
	fakePanePTY
	snap  streamhub.FullStateSnapshot
	ok    bool
	err   error
	calls int
}

func (r *redrawPanePTY) CapturePaneRedraw(context.Context) (streamhub.FullStateSnapshot, bool, error) {
	r.calls++
	return r.snap, r.ok, r.err
}

func resyncOnce(t *testing.T, target panePTY) *sessionv1.TerminalOutput {
	t.Helper()
	out, err := handleCurrentPaneRequest(context.Background(), "s", target,
		&sessionv1.CurrentPaneRequest{ResyncId: "r1"}, ResyncOptions{Writer: testWriter(), EchoResyncID: true})
	require.NoError(t, err)
	return out
}

func TestHandleCurrentPaneRequest_should_SendRedrawVerbatim_When_BackendProvidesOne(t *testing.T) {
	t.Parallel()
	const redraw = "\x1b[?1049h\x1b]0;title\x07\x1b[3;5Hhi\x1b[?25h"
	target := &redrawPanePTY{fakePanePTY: fakePanePTY{captureContent: "legacy", cols: 80, rows: 24},
		snap: streamhub.FullStateSnapshot{Data: redraw, AltScreen: true}, ok: true}

	out := resyncOnce(t, target)

	assert.Equal(t, redraw, string(out.Data), "no prefix, CRLF rewrite or cursor sync may wrap a full-state redraw")
	assert.True(t, out.GetAltScreenActive())
	assert.Equal(t, "r1", out.ResyncId)
	assert.Equal(t, 0, target.capturePaneCalled+target.capturePanePriorityCalled, "text capture must not run when the redraw is used")
}

func TestHandleCurrentPaneRequest_should_UseLegacyText_When_BackendHasNoRedraw(t *testing.T) {
	t.Parallel()
	out := resyncOnce(t, &fakePanePTY{captureContent: "legacy", cols: 80, rows: 24})

	assert.Contains(t, string(out.Data), "legacy")
	assert.True(t, len(out.Data) > len("legacy"), "legacy path keeps its ansiSnapshotPrefix/cursor sync")
}

func TestHandleCurrentPaneRequest_should_FallBackToLegacyText_When_RedrawUnavailable(t *testing.T) {
	t.Parallel()
	cases := map[string]*redrawPanePTY{
		"capability declined (older daemon)": {ok: false},
		"capture failed":                     {ok: true, err: errors.New("daemon gone")},
	}
	for name, target := range cases {
		target := target
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			target.fakePanePTY = fakePanePTY{captureContent: "legacy", cols: 80, rows: 24}
			out := resyncOnce(t, target)
			assert.Contains(t, string(out.Data), "legacy")
			assert.Equal(t, 1, target.calls)
		})
	}
}

func TestFullStateFrame_should_ReportNotOK_When_TargetLacksCapability(t *testing.T) {
	t.Parallel()
	_, ok := fullStateFrame(context.Background(), &fakePanePTY{})
	assert.False(t, ok)
}
