package tymux

import (
	"strconv"
	"strings"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

const (
	esc = "\x1b"
	csi = esc + "["
	st  = esc + `\`
)

// RenderSnapshotRedraw renders a full redraw of snap for a live vt100-style client (xterm.js):
// the cell grid plus the terminal state a bare grid paint would lose — alt screen, tab stops,
// charsets, scroll region, input/mouse modes, cursor shape/position/visibility, and title —
// so keys, mouse and paste keep working after a gap or reconnect.
//
// Order matters. The grid is painted in a neutral state (IRM, DECOM off, ASCII charsets,
// cursor hidden). DECSTBM and DECOM both home the cursor, so they come before the final CUP.
// Limits: DECSC/DECRC's saved cursor and the inactive screen's
// contents are not in the snapshot and are not restored.
func RenderSnapshotRedraw(snap *v1.PaneSnapshot) (string, error) {
	grid, err := CellsToSGR(snap.GetGrid(), snap.GetHyperlinks())
	if err != nil {
		return "", err
	}
	var b strings.Builder

	// Known base: leave any alt screen the client was in, then enter it if the snapshot is alt.
	b.WriteString(csi + "?1049l")
	if snap.GetAltScreen() {
		b.WriteString(csi + "?1049h")
	}
	// Neutral painting state, then clear.
	b.WriteString(csi + "0m" + csi + "4l" + csi + "?6l" + csi + "?7h" + csi + "?25l" + csi + "r")
	b.WriteString(esc + "(B" + esc + ")B" + esc + "*B" + esc + "+B\x0f")
	b.WriteString(csi + "2J" + csi + "H")

	b.WriteString(csi + "3g") // TBC: clear all tab stops
	for _, col := range snap.GetTabStops() {
		b.WriteString(csi + "1;" + strconv.Itoa(int(col)+1) + "H" + esc + "H") // HTS at that column
	}
	b.WriteString(csi + "H") // HTS positioning moved the cursor; paint from the top-left

	b.WriteString(strings.ReplaceAll(grid, "\n", "\r\n"))

	writeCharsets(&b, snap.GetCharsets())
	if top, bottom := snap.GetScrollTop(), snap.GetScrollBottom(); top != 0 || bottom != 0 {
		b.WriteString(csi + strconv.Itoa(int(top)) + ";" + strconv.Itoa(int(bottom)) + "r")
	}
	writeModes(&b, snap.GetModes())
	b.WriteString(csi + strconv.Itoa(int(snap.GetCursorStyle())) + " q")
	b.WriteString(esc + "]2;" + stripControl(snap.GetTitle(), "") + st)

	row := int(snap.GetCursorRow())
	if snap.GetModes().GetOrigin() { // CUP is relative to the scroll region's top under DECOM
		if top := int(snap.GetScrollTop()); top > 0 {
			row -= top - 1
		}
	}
	b.WriteString(csi + strconv.Itoa(row+1) + ";" + strconv.Itoa(int(snap.GetCursorCol())+1) + "H")
	if snap.GetCursorPendingWrap() {
		writePendingWrap(&b, snap)
	}
	b.WriteString(csi + "?25" + map[bool]string{true: "l", false: "h"}[snap.GetCursorHidden()])
	return b.String(), nil
}

// writePendingWrap re-prints the last-column cell under the cursor, which re-arms the terminal's
// deferred autowrap: CUP can only reach the last column, not the parked "past it" state.
// The cell keeps its own attributes (and ends with a reset), so the repaint changes nothing visible.
func writePendingWrap(b *strings.Builder, snap *v1.PaneSnapshot) {
	row := int(snap.GetCursorRow())
	grid := snap.GetGrid()
	if row < 0 || row >= len(grid) {
		return
	}
	cells := grid[row].GetCells()
	if len(cells) == 0 {
		return
	}
	last := cells[len(cells)-1]
	text, err := CellsToSGR([]*v1.Row{{Cells: []*v1.Cell{last}}}, snap.GetHyperlinks())
	if err != nil {
		return
	}
	b.WriteString(text)
}

// charsetFinal is the SCS final byte for a Charsets designation: 0 ASCII, 1 DEC graphics, 2 UK.
var charsetFinal = [...]string{"B", "0", "A"}

func writeCharsets(b *strings.Builder, cs *v1.Charsets) {
	final := func(kind uint32) string {
		if int(kind) < len(charsetFinal) {
			return charsetFinal[kind]
		}
		return "B"
	}
	b.WriteString(esc + "(" + final(cs.GetG0()) + esc + ")" + final(cs.GetG1()) +
		esc + "*" + final(cs.GetG2()) + esc + "+" + final(cs.GetG3()))
	switch cs.GetGl() {
	case 1:
		b.WriteString("\x0e") // SO
	case 2:
		b.WriteString(esc + "n") // LS2
	case 3:
		b.WriteString(esc + "o") // LS3
	default:
		b.WriteString("\x0f") // SI
	}
}

func writeModes(b *strings.Builder, m *v1.TerminalModes) {
	private := func(n int, on bool) {
		b.WriteString(csi + "?" + strconv.Itoa(n) + map[bool]string{true: "h", false: "l"}[on])
	}
	private(1, m.GetAppCursorKeys())
	if m.GetAppKeypad() {
		b.WriteString(esc + "=")
	} else {
		b.WriteString(esc + ">")
	}
	private(2004, m.GetBracketedPaste())
	private(1004, m.GetFocusEvents())

	// Reset every mouse protocol/encoding first so a stale one can't outlive the redraw.
	for _, n := range []int{9, 1000, 1002, 1003, 1005, 1006, 1015} {
		private(n, false)
	}
	if n := [...]int{0, 9, 1000, 1002, 1003}; int(m.GetMouseMode()) < len(n) && m.GetMouseMode() != 0 {
		private(n[m.GetMouseMode()], true)
	}
	if n := [...]int{0, 1005, 1006, 1015}; int(m.GetMouseEncoding()) < len(n) && m.GetMouseEncoding() != 0 {
		private(n[m.GetMouseEncoding()], true)
	}

	private(5, m.GetReverseVideo())
	private(6, m.GetOrigin())
	private(7, !m.GetNoAutowrap())
	b.WriteString(csi + "4" + map[bool]string{true: "h", false: "l"}[m.GetInsert()])
	b.WriteString(csi + "20" + map[bool]string{true: "h", false: "l"}[m.GetNewline()])
}
