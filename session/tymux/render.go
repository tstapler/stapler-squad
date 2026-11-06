package tymux

import (
	"strconv"
	"strings"

	"golang.org/x/text/width"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// Attribute bitflags mirrored from proto/tymux/v1/tymux.proto's Cell.attrs
// doc comment, which itself mirrors crates/tymux-core/src/pane.rs's
// ATTR_BOLD/ATTR_UNDERLINE/ATTR_REVERSE/ATTR_ITALIC/ATTR_FAINT constants exactly —
// keep all three in sync if any changes.
const (
	attrBold      uint32 = 1
	attrUnderline uint32 = 2
	attrReverse   uint32 = 4
	attrItalic    uint32 = 8
	attrFaint     uint32 = 16
	attrStrike    uint32 = 32
	attrBlink     uint32 = 64
	attrConceal   uint32 = 128
	attrOverline  uint32 = 256
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

// sgrState is the subset of a Cell that affects its SGR encoding. ulStyle/ulColor are
// zeroed when the underline bit is off, so a stale value never forces a reset.
// link is the cell's 1-based hyperlink index; it travels as OSC 8, not SGR, but lives
// here so the run-diffing in rowToSGR sees one state.
type sgrState struct {
	fg, bg, attrs    uint32
	ulStyle, ulColor uint32
	link             uint32
}

func cellState(c *v1.Cell) sgrState {
	s := sgrState{fg: c.GetFg(), bg: c.GetBg(), attrs: c.GetAttrs(), link: c.GetHyperlink()}
	if s.attrs&attrUnderline != 0 {
		s.ulStyle, s.ulColor = c.GetUnderlineStyle(), c.GetUnderlineColor()
	}
	return s
}

// sgr is s without its hyperlink, the part SGR 0 can reset.
func (s sgrState) sgr() sgrState {
	s.link = 0
	return s
}

// isZero reports whether s is the terminal's implicit default state (no
// attributes, default fg/bg) — the state a line starts in and the state a
// bare SGR reset (`\x1b[0m`) restores.
func (s sgrState) isZero() bool {
	return s.sgr() == sgrState{}
}

// CellsToSGR renders a PaneSnapshot's cell grid (PaneSnapshot.grid) into an
// ANSI/SGR-encoded string matching `tmux capture-pane -p -e`'s
// attribute-preserving output shape — the renderer CapturePaneContent()
// and CapturePaneContentRaw() share.
//
// Rows are newline-joined (mirroring rowsToPlainText's join, and matching
// the tmux backend's own capture-pane -p -e contract) -- this is a
// text-capture API, not a live terminal feed. A caller that replays this
// into a live vt100 parser (e.g. xterm.js) needs "\n" upgraded to "\r\n"
// first -- see resync.go's applySnapshotResync, the one call site that
// does that, rather than baking CRLF into every CellsToSGR consumer here.
// Within a row,
// an SGR escape sequence is emitted only when a cell's fg/bg/attrs differ
// from the previous cell's (Task 2.6.1a/Story 2.6.1) — an unbroken
// attribute run gets exactly one sequence before it, not one per cell.
//
// links is PaneSnapshot.hyperlinks, the table Cell.hyperlink indexes into (nil when
// the grid has no links). Each hyperlink run is opened and closed within its row.
func CellsToSGR(grid []*v1.Row, links []*v1.Hyperlink) (string, error) {
	lines := make([]string, len(grid))
	for i, row := range grid {
		lines[i] = rowToSGR(row, links)
	}
	return strings.Join(lines, "\n"), nil
}

// rowToSGR renders one row for text capture: trailing never-written cells are dropped so rows
// aren't padded to the pane width.
func rowToSGR(row *v1.Row, links []*v1.Hyperlink) string {
	return renderCells(row.GetCells(), links, true)
}

// renderCells renders cells, diffing each cell's SGR state against the previous cell's
// (implicitly the terminal's default state at the start of the row) and emitting a sequence only
// on change. With trimTrailing, trailing cells that are empty, unstyled and unlinked are dropped
// (a literal " " is kept: the daemon sent it); the pending-wrap re-print needs them untrimmed.
func renderCells(cells []*v1.Cell, links []*v1.Hyperlink, trimTrailing bool) string {
	if trimTrailing {
		for len(cells) > 0 {
			last := cells[len(cells)-1]
			if cellTextOf(last) != "" || cellState(last) != (sgrState{}) {
				break
			}
			cells = cells[:len(cells)-1]
		}
	}
	var b strings.Builder
	prev := sgrState{}
	afterWide := false
	for _, c := range cells {
		cur := cellState(c)
		if _, _, ok := linkTarget(cur.link, links); !ok {
			cur.link = 0 // an unusable link renders as plain text, not as an open/close pair
		}
		if cur.sgr() != prev.sgr() {
			b.WriteString(sgrSequence(prev.sgr(), cur.sgr()))
		}
		if cur.link != prev.link {
			b.WriteString(hyperlinkSequence(cur.link, links))
		}
		text := cellTextOf(c)
		b.WriteString(printedText(text, afterWide))
		afterWide = isWide(text)
		prev = cur
	}
	// Leave the line in the default state so whatever follows (the next
	// row, or the caller's own trailing output) never inherits a stale
	// attribute/color — matches capture-pane -e's per-line reset.
	if prev.link != 0 {
		b.WriteString(hyperlinkSequence(0, nil))
	}
	if !prev.isZero() {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

// maxHyperlinkURIBytes caps a rendered OSC 8 URI (matches common emulator limits); the URI is
// re-emitted on every link change, so an unbounded one amplifies the broadcast.
const maxHyperlinkURIBytes = 2048

// linkTarget resolves 1-based index idx of links to the sanitized OSC 8 parameters and URI.
// ok is false for no link, an out-of-range index, an empty URI, or one over the cap.
func linkTarget(idx uint32, links []*v1.Hyperlink) (params, uri string, ok bool) {
	if idx == 0 || int(idx) > len(links) || links[idx-1] == nil {
		return "", "", false
	}
	l := links[idx-1]
	uri = stripControl(l.GetUri(), "")
	if uri == "" || len(uri) > maxHyperlinkURIBytes {
		return "", "", false
	}
	if id := stripControl(l.GetId(), ";:"); id != "" {
		params = "id=" + id
	}
	return params, uri, true
}

// hyperlinkSequence opens the OSC 8 hyperlink at 1-based index idx of links, or closes the
// current one when idx doesn't resolve. SGR 0 doesn't end a hyperlink, so closing is explicit.
func hyperlinkSequence(idx uint32, links []*v1.Hyperlink) string {
	params, uri, ok := linkTarget(idx, links)
	if !ok {
		return "\x1b]8;;\x1b\\"
	}
	return "\x1b]8;" + params + ";" + uri + "\x1b\\"
}

// stripControl drops C0/DEL/C1 control bytes (so a daemon-supplied string can't end the OSC
// early or inject escapes) plus any rune in extra.
func stripControl(s, extra string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r < 0xa0) || strings.ContainsRune(extra, r) {
			return -1
		}
		return r
	}, s)
}

// cellTextOf is a cell's text with control bytes removed: cell text comes from the daemon's
// view of arbitrary program output and is replayed into browser terminals, so it must never
// carry escapes.
func cellTextOf(c *v1.Cell) string {
	return stripControl(c.GetText(), "")
}

// printedText returns the text to print for a cell whose sanitized text is text. The daemon
// leaves a cell's Text empty when the cursor skipped over it (CSI C, which Claude Code's renderer
// uses instead of printing spaces) — printing "" would glue the words on either side together.
// An empty cell is the 2nd half of a wide glyph only when the preceding cell is wide, and that
// one prints nothing.
func printedText(text string, afterWide bool) string {
	if text != "" {
		return text
	}
	if afterWide {
		return ""
	}
	return " "
}

func isWide(text string) bool {
	if text == "" {
		return false
	}
	p, _ := width.LookupString(text)
	k := p.Kind()
	return k == width.EastAsianWide || k == width.EastAsianFullwidth
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
	if prev.ulColor != 0 && cur.ulColor == 0 {
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
// reverse per Task 2.6.1b, then fg, then bg, then underline color) needed to set s's full state.
func sgrCodes(s sgrState) []string {
	var codes []string
	if s.attrs&attrBold != 0 {
		codes = append(codes, "1")
	}
	if s.attrs&attrFaint != 0 {
		codes = append(codes, "2")
	}
	if s.attrs&attrItalic != 0 {
		codes = append(codes, "3")
	}
	if s.attrs&attrUnderline != 0 {
		codes = append(codes, underlineCode(s.ulStyle))
	}
	if s.attrs&attrBlink != 0 {
		codes = append(codes, "5")
	}
	if s.attrs&attrReverse != 0 {
		codes = append(codes, "7")
	}
	if s.attrs&attrConceal != 0 {
		codes = append(codes, "8")
	}
	if s.attrs&attrStrike != 0 {
		codes = append(codes, "9")
	}
	if s.attrs&attrOverline != 0 {
		codes = append(codes, "53")
	}
	codes = append(codes, colorCodes(s.fg, "38")...)
	codes = append(codes, colorCodes(s.bg, "48")...)
	if s.attrs&attrUnderline != 0 {
		codes = append(codes, colorCodes(s.ulColor, "58")...)
	}
	return codes
}

// underlineCode renders Cell.underline_style: plain `4` for single, else the colon sub-parameter
// form `4:N` (2 double, 3 curly, 4 dotted, 5 dashed) that xterm.js, kitty, wezterm and iTerm2 accept.
func underlineCode(style uint32) string {
	if style == 0 || style > 4 {
		return "4"
	}
	return "4:" + strconv.Itoa(int(style)+1)
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
