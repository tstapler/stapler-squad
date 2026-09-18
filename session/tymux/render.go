package tymux

import (
	"strconv"
	"strings"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// Attribute bitflags mirrored from proto/tymux/v1/tymux.proto's Cell.attrs
// doc comment, which itself mirrors crates/tymux-core/src/pane.rs's
// ATTR_BOLD/ATTR_UNDERLINE/ATTR_REVERSE/ATTR_ITALIC constants exactly —
// keep all three in sync if any changes.
const (
	attrBold      uint32 = 1
	attrUnderline uint32 = 2
	attrReverse   uint32 = 4
	attrItalic    uint32 = 8
)

// Color tag bits mirrored from pane.rs's pack_color doc comment: the top
// byte of a packed Cell.fg/Cell.bg value tags which variant the remaining
// bytes hold. 0x00 = default (no SGR color code needed), 0x01xxxxxx =
// 256-color index (low byte = index), 0x02rrggbb = truecolor RGB.
const (
	colorTagMask    uint32 = 0xFF00_0000
	colorTagIndexed uint32 = 0x0100_0000
	colorTagRGB     uint32 = 0x0200_0000
)

// sgrState is the subset of a Cell that affects its SGR encoding.
type sgrState struct {
	fg, bg, attrs uint32
}

func cellState(c *v1.Cell) sgrState {
	return sgrState{fg: c.GetFg(), bg: c.GetBg(), attrs: c.GetAttrs()}
}

// isZero reports whether s is the terminal's implicit default state (no
// attributes, default fg/bg) — the state a line starts in and the state a
// bare SGR reset (`\x1b[0m`) restores.
func (s sgrState) isZero() bool {
	return s.fg == 0 && s.bg == 0 && s.attrs == 0
}

// CellsToSGR renders a PaneSnapshot's cell grid (PaneSnapshot.grid) into an
// ANSI/SGR-encoded string matching `tmux capture-pane -p -e`'s
// attribute-preserving output shape — the renderer CapturePaneContent()
// (Task 2.2.2d) needs and CapturePaneContentRaw() deliberately doesn't
// (that method only joins Cell.text, no SGR).
//
// Rows are newline-joined (mirroring rowsToPlainText's join). Within a row,
// an SGR escape sequence is emitted only when a cell's fg/bg/attrs differ
// from the previous cell's (Task 2.6.1a/Story 2.6.1) — an unbroken
// attribute run gets exactly one sequence before it, not one per cell.
func CellsToSGR(grid []*v1.Row) (string, error) {
	lines := make([]string, len(grid))
	for i, row := range grid {
		lines[i] = rowToSGR(row)
	}
	return strings.Join(lines, "\n"), nil
}

// rowToSGR renders one row's cells, diffing each cell's SGR state against
// the previous cell's (implicitly the terminal's default state at the
// start of the row) and emitting a sequence only on change.
func rowToSGR(row *v1.Row) string {
	var b strings.Builder
	prev := sgrState{}
	for _, c := range row.GetCells() {
		cur := cellState(c)
		if cur != prev {
			b.WriteString(sgrSequence(prev, cur))
		}
		b.WriteString(c.GetText())
		prev = cur
	}
	// Leave the line in the default state so whatever follows (the next
	// row, or the caller's own trailing output) never inherits a stale
	// attribute/color — matches capture-pane -e's per-line reset.
	if !prev.isZero() {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// sgrSequence renders the one escape sequence needed to move from prev to
// cur. Moving to the zero state is always a bare reset. Otherwise: SGR has
// no clean way to turn off an individual bold/underline/reverse/italic
// flag or drop back to a default color except a full reset (this renderer
// doesn't bother tracking the dedicated 22/24/27/23/39/49 "off" codes), so
// any transition that *drops* an attribute or color forces a reset+reapply
// (Task 2.6.1a); a purely additive/overwriting transition (e.g. a color
// change, or turning on a further attribute) just emits the new state's
// set codes, since SGR "set" codes overwrite cleanly on their own.
func sgrSequence(prev, cur sgrState) string {
	if cur.isZero() {
		return "\x1b[0m"
	}
	codes := sgrCodes(cur)
	if isRemoval(prev, cur) {
		return "\x1b[0;" + strings.Join(codes, ";") + "m"
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// isRemoval reports whether moving from prev to cur drops any attribute
// bit or color that was previously set.
func isRemoval(prev, cur sgrState) bool {
	if prev.attrs&^cur.attrs != 0 {
		return true
	}
	if prev.fg != 0 && cur.fg == 0 {
		return true
	}
	if prev.bg != 0 && cur.bg == 0 {
		return true
	}
	return false
}

// sgrCodes returns the ordered SGR parameters (bold/italic/underline/
// reverse per Task 2.6.1b, then fg, then bg) needed to set s's full state.
func sgrCodes(s sgrState) []string {
	var codes []string
	if s.attrs&attrBold != 0 {
		codes = append(codes, "1")
	}
	if s.attrs&attrItalic != 0 {
		codes = append(codes, "3")
	}
	if s.attrs&attrUnderline != 0 {
		codes = append(codes, "4")
	}
	if s.attrs&attrReverse != 0 {
		codes = append(codes, "7")
	}
	codes = append(codes, colorCodes(s.fg, "38")...)
	codes = append(codes, colorCodes(s.bg, "48")...)
	return codes
}

// colorCodes unpacks one packed Cell.fg/Cell.bg value (pane.rs's
// pack_color doc comment: 0x00 = default, 0x01xxxxxx = 256-color index
// [low byte = index], 0x02rrggbb = truecolor RGB) into the SGR params for
// the given base ("38" foreground, "48" background): `<base>;5;n` for an
// indexed color, `<base>;2;r;g;b` for truecolor. The default tag emits no
// code at all — absence of a color code leaves whatever the terminal's
// current default is, which is what a reset (or never having set one)
// already gives us.
func colorCodes(packed uint32, base string) []string {
	switch packed & colorTagMask {
	case colorTagIndexed:
		return []string{base, "5", strconv.Itoa(int(packed & 0xFF))}
	case colorTagRGB:
		r := (packed >> 16) & 0xFF
		g := (packed >> 8) & 0xFF
		b := packed & 0xFF
		return []string{base, "2", strconv.Itoa(int(r)), strconv.Itoa(int(g)), strconv.Itoa(int(b))}
	default:
		return nil
	}
}
