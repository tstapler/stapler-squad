package tymux

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

func redrawOf(t *testing.T, snap *v1.PaneSnapshot) string {
	t.Helper()
	if snap.Modes == nil { // a snapshot without Modes is an old daemon's (see the legacy test)
		snap.Modes = &v1.TerminalModes{}
	}
	out, err := RenderSnapshotRedraw(snap)
	require.NoError(t, err)
	return out
}

func oneCell() []*v1.Row { return []*v1.Row{row(cell("a", 0, 0, 0))} }

// indexOrdered fails unless every part occurs in out, left to right.
func indexOrdered(t *testing.T, out string, parts ...string) {
	t.Helper()
	pos := 0
	for _, p := range parts {
		i := strings.Index(out[pos:], p)
		require.GreaterOrEqualf(t, i, 0, "%q missing (or out of order) after offset %d in %q", p, pos, out)
		pos += i + len(p)
	}
}

func TestRedraw_ShouldPlaceCursorAtSnapshotPosition_NotAtEndOfLastRow(t *testing.T) {
	snap := &v1.PaneSnapshot{Grid: []*v1.Row{row(cell("a", 0, 0, 0)), row(cell("b", 0, 0, 0))}, CursorRow: 0, CursorCol: 4}

	out := redrawOf(t, snap)

	indexOrdered(t, out, "a\r\nb", "\x1b[1;5H")
}

func TestRedraw_ShouldPaintInANeutralState_BeforeTheGrid(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell()})

	indexOrdered(t, out,
		"\x1b[?1049l", // known base
		"\x1b[0m\x1b[4l\x1b[?6l\x1b[?7h\x1b[?25l\x1b[r", // SGR, IRM, DECOM, DECAWM, cursor, region
		"\x1b(B\x1b)B\x1b*B\x1b+B\x0f",                  // ASCII charsets, SI
		"\x1b[2J\x1b[H",
		"a",
	)
}

func TestRedraw_ShouldEnterAltScreenBeforeClearing_WhenSnapshotIsAlt(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), AltScreen: true})

	indexOrdered(t, out, "\x1b[?1049l", "\x1b[?1049h", "\x1b[2J", "a")
}

func TestRedraw_ShouldLeaveAltScreen_WhenSnapshotIsPrimary(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell()})

	assert.Contains(t, out, "\x1b[?1049l")
	assert.NotContains(t, out, "\x1b[?1049h")
}

func TestRedraw_ShouldClearThenSetEachTabStop_BeforeTheGrid(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), TabStops: []uint32{4, 20}})

	indexOrdered(t, out, "\x1b[3g", "\x1b[1;5H\x1bH", "\x1b[1;21H\x1bH", "\x1b[H", "a")
}

func TestRedraw_ShouldDesignateCharsetsAndShift_AfterTheGrid(t *testing.T) {
	cases := map[string]struct {
		cs   *v1.Charsets
		want string
	}{
		"g0 dec graphics, SI":         {&v1.Charsets{G0: 1}, "\x1b(0\x1b)B\x1b*B\x1b+B\x0f"},
		"g1 dec graphics, SO":         {&v1.Charsets{G1: 1, Gl: 1}, "\x1b(B\x1b)0\x1b*B\x1b+B\x0e"},
		"g2 uk, LS2":                  {&v1.Charsets{G2: 2, Gl: 2}, "\x1b(B\x1b)B\x1b*A\x1b+B\x1bn"},
		"g3 dec graphics, LS3":        {&v1.Charsets{G3: 1, Gl: 3}, "\x1b(B\x1b)B\x1b*B\x1b+0\x1bo"},
		"unknown kind falls to ASCII": {&v1.Charsets{G0: 9}, "\x1b(B\x1b)B\x1b*B\x1b+B\x0f"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), Charsets: c.cs})

			indexOrdered(t, out, "a", c.want)
		})
	}
}

func TestRedraw_ShouldSetScrollRegionBeforeFinalCursorPosition(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), ScrollTop: 5, ScrollBottom: 10, CursorRow: 7, CursorCol: 2})

	indexOrdered(t, out, "a", "\x1b[5;10r", "\x1b[8;3H") // DECSTBM homes the cursor, so CUP comes after
}

func TestRedraw_ShouldOmitScrollRegion_WhenWholeScreen(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell()})

	assert.NotRegexp(t, `\x1b\[\d+;\d+r`, out)
}

func TestRedraw_ShouldMakeCursorRowRelativeToRegionTop_WhenOriginModeIsOn(t *testing.T) {
	snap := &v1.PaneSnapshot{Grid: oneCell(), ScrollTop: 5, ScrollBottom: 10, CursorRow: 7, CursorCol: 2, Modes: &v1.TerminalModes{Origin: true}}

	out := redrawOf(t, snap)

	indexOrdered(t, out, "\x1b[?6h", "\x1b[4;3H") // absolute row 7 (0-based) is row 4 of a region starting at 5
}

func TestRedraw_ShouldEmitEveryModeExplicitly_AfterTheGrid(t *testing.T) {
	on := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), Modes: &v1.TerminalModes{
		AppCursorKeys: true, AppKeypad: true, BracketedPaste: true, FocusEvents: true,
		ReverseVideo: true, Origin: true, NoAutowrap: true, Insert: true, Newline: true,
	}})
	indexOrdered(t, on, "a", "\x1b[?1h", "\x1b=", "\x1b[?2004h", "\x1b[?1004h", "\x1b[?5h", "\x1b[?6h", "\x1b[?7l", "\x1b[4h", "\x1b[20h")

	off := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell()})
	indexOrdered(t, off, "a", "\x1b[?1l", "\x1b>", "\x1b[?2004l", "\x1b[?1004l", "\x1b[?5l", "\x1b[?6l", "\x1b[?7h", "\x1b[4l", "\x1b[20l")
}

func TestRedraw_ShouldResetAllMouseModesThenSetTheActiveOnes(t *testing.T) {
	resets := "\x1b[?9l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1005l\x1b[?1006l\x1b[?1015l"
	cases := []struct {
		name         string
		mode, enc    uint32
		wantSetAfter string
	}{
		{"none", 0, 0, ""},
		{"x10", 1, 0, "\x1b[?9h"},
		{"normal + utf8", 2, 1, "\x1b[?1000h\x1b[?1005h"},
		{"button + sgr", 3, 2, "\x1b[?1002h\x1b[?1006h"},
		{"any + urxvt", 4, 3, "\x1b[?1003h\x1b[?1015h"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), Modes: &v1.TerminalModes{MouseMode: c.mode, MouseEncoding: c.enc}})

			indexOrdered(t, out, "a", resets+c.wantSetAfter)
		})
	}
}

func TestRedraw_ShouldIgnoreOutOfRangeMouseValues(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), Modes: &v1.TerminalModes{MouseMode: 99, MouseEncoding: 99}})

	assert.NotRegexp(t, `\x1b\[\?(9|1000|1002|1003|1005|1006|1015)h`, out)
}

func TestRedraw_ShouldEmitCursorStyle_ThenTitle_ThenPosition_ThenVisibility(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), CursorStyle: 5, Title: "vim", CursorRow: 1, CursorCol: 2})

	indexOrdered(t, out, "a", "\x1b[5 q", "\x1b]2;vim\x1b\\", "\x1b[2;3H", "\x1b[?25h")
}

func TestRedraw_ShouldHideCursor_WhenSnapshotCursorIsHidden(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), CursorHidden: true})

	assert.True(t, strings.HasSuffix(out, "\x1b[?25l"), "got %q", out)
}

func TestRedraw_ShouldShowCursorLast_WhenVisible(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell()})

	assert.True(t, strings.HasSuffix(out, "\x1b[?25h"), "got %q", out)
}

func TestRedraw_ShouldStripControlBytesFromTitle_SoItCannotEndTheOSCEarlyOrInjectEscapes(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), Title: "a\x07b\x1b]0;evil\x1b\\c\u009cd"})

	assert.Contains(t, out, "\x1b]2;ab]0;evil\\cd\x1b\\")
	assert.Equal(t, 1, strings.Count(out, "\x1b]2;"), "title must not smuggle a second OSC")
	assert.NotContains(t, out, "\x07")
}

func TestRedraw_ShouldEmitAnEmptyTitle_SoAStaleClientTitleIsCleared(t *testing.T) {
	out := redrawOf(t, &v1.PaneSnapshot{Grid: oneCell()})

	assert.Contains(t, out, "\x1b]2;\x1b\\")
}

func TestRedraw_ShouldTolerateMissingStateMessages(t *testing.T) {
	assert.NotPanics(t, func() { redrawOf(t, &v1.PaneSnapshot{Grid: oneCell(), Modes: nil, Charsets: nil}) })
}

func TestRedraw_ShouldReArmPendingWrap_WhenSnapshotCursorIsParkedPastTheLastColumn(t *testing.T) {
	snap := &v1.PaneSnapshot{
		Grid:              []*v1.Row{row(cell("a", 0, 0, 0), cell("z", 0, 0, attrBold))},
		CursorRow:         0,
		CursorCol:         1,
		CursorPendingWrap: true,
	}

	out := redrawOf(t, snap)

	// CUP to the last column, then the last cell again (with its own attributes, then a reset),
	// all before the cursor is made visible.
	indexOrdered(t, out, "\x1b[1;2H", "\x1b[1mz\x1b[0m", "\x1b[?25h")
}

func TestRedraw_ShouldNotRePrintLastCell_WhenWrapIsNotPending(t *testing.T) {
	snap := &v1.PaneSnapshot{Grid: []*v1.Row{row(cell("a", 0, 0, 0), cell("z", 0, 0, 0))}, CursorCol: 1}

	out := redrawOf(t, snap)

	assert.NotContains(t, out[strings.Index(out, "\x1b[1;2H"):], "z", "nothing is re-printed after the final CUP")
}

func TestRedraw_ShouldReprintTheWideGlyphItself_WhenPendingWrapFollowsAWideGlyphAtTheLastColumn(t *testing.T) {
	snap := &v1.PaneSnapshot{
		Grid:              []*v1.Row{row(cell("a", 0, 0, 0), cell("世", 0, 0, 0), cell("", 0, 0, 0))},
		CursorCol:         2,
		CursorPendingWrap: true,
		Modes:             &v1.TerminalModes{},
	}

	out := redrawOf(t, snap)

	// The continuation cell must not be overwritten with a space; the glyph is re-printed from its left half.
	indexOrdered(t, out, "\x1b[1;3H", "\x1b[1;2H世", "\x1b[?25h")
	assert.NotContains(t, out[strings.LastIndex(out, "\x1b[1;3H"):], " \x1b[?25h")
}

func TestRedraw_ShouldReprintPendingWrapCellUnderAscii_WhenCharsetsAreRestored(t *testing.T) {
	snap := &v1.PaneSnapshot{
		Grid:              []*v1.Row{row(cell("a", 0, 0, 0), cell("q", 0, 0, 0))},
		CursorCol:         1,
		CursorPendingWrap: true,
		Modes:             &v1.TerminalModes{},
		Charsets:          &v1.Charsets{G0: 1},
	}

	out := redrawOf(t, snap)

	reprint := strings.LastIndex(out, "\x1b[1;2Hq")
	require.GreaterOrEqual(t, reprint, 0, "pending-wrap re-print missing: %q", out)
	designate := strings.LastIndex(out, "\x1b(0")
	require.GreaterOrEqual(t, designate, 0, "DEC graphics designation missing: %q", out)
	assert.Greater(t, designate, reprint, "G0 must switch to DEC graphics only after the ASCII re-print, or 'q' becomes a line")
}

func TestRedraw_ShouldClampCursorRowAtOne_WhenOriginModeRegionStartsBelowTheCursor(t *testing.T) {
	snap := &v1.PaneSnapshot{
		Grid:         oneCell(),
		CursorRow:    0,
		ScrollTop:    5,
		ScrollBottom: 10,
		Modes:        &v1.TerminalModes{Origin: true},
	}

	out := redrawOf(t, snap)

	assert.NotContains(t, out, "\x1b[-", "a negative CUP row is an invalid CSI")
	assert.Contains(t, out, "\x1b[1;1H")
}

func TestRedraw_ShouldSkipStateResets_WhenSnapshotHasNoModes(t *testing.T) {
	// An older daemon doesn't send terminal state; zero values must not be applied as if real.
	snap := &v1.PaneSnapshot{Grid: oneCell(), CursorCol: 3}

	out, err := RenderSnapshotRedraw(snap)
	require.NoError(t, err)

	for _, resetOrState := range []string{"?1049", "?2004", "?1000", "?1006", "?1004", "\x1b]2;", " q", "\x1b(B", "\x1b[4l", "\x1b[20l", "?7h", "?6l"} {
		assert.NotContains(t, out, resetOrState, "no daemon state => no %q", resetOrState)
	}
	indexOrdered(t, out, "\x1b[2J", "a", "\x1b[1;4H", "\x1b[?25h")
}
