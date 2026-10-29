package tymux

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// sgrEscape is a real ANSI-aware tokenizer (not a substring/plain-text
// check): it finds every `\x1b[<params>m` SGR escape in s, in order, and
// parses each one's semicolon-separated parameters into ints — the same
// shape a real terminal emulator's parser would produce. Task
// 2.6.1c/validation.md's REQ-5 round-trip test asserts against this
// parsed structure, not raw byte-substring matching.
var sgrEscape = regexp.MustCompile("\x1b\\[([0-9;]*)m")

func parseSGRSequences(t *testing.T, s string) [][]int {
	t.Helper()
	matches := sgrEscape.FindAllStringSubmatch(s, -1)
	seqs := make([][]int, len(matches))
	for i, m := range matches {
		var params []int
		for _, p := range strings.Split(m[1], ";") {
			n, err := strconv.Atoi(p)
			require.NoErrorf(t, err, "SGR sequence %q has a non-numeric parameter %q", m[0], p)
			params = append(params, n)
		}
		seqs[i] = params
	}
	return seqs
}

func cell(text string, fg, bg, attrs uint32) *v1.Cell {
	return &v1.Cell{Text: text, Fg: fg, Bg: bg, Attrs: attrs}
}

func row(cells ...*v1.Cell) *v1.Row {
	return &v1.Row{Cells: cells}
}

func packIndexed(idx uint8) uint32 {
	return 0x0100_0000 | uint32(idx)
}

func packRGB(r, g, b uint8) uint32 {
	return 0x0200_0000 | (uint32(r) << 16) | (uint32(g) << 8) | uint32(b)
}

func TestCellSGRRenderer_ShouldEmitPlainText_WhenNoCellHasAttributesOrColor(t *testing.T) {
	grid := []*v1.Row{
		row(cell("h", 0, 0, 0), cell("i", 0, 0, 0)),
	}

	out, err := CellsToSGR(grid, nil)

	require.NoError(t, err)
	assert.Equal(t, "hi", out)
	assert.NotContains(t, out, "\x1b[", "plain text must not contain any SGR escape")
}

func TestCellSGRRenderer_ShouldEmitSingleSGRSequence_WhenAnAttributeRunSpansMultipleCells(t *testing.T) {
	// Cells 0-2 bold, cells 3-5 plain.
	grid := []*v1.Row{
		row(
			cell("a", 0, 0, attrBold), cell("b", 0, 0, attrBold), cell("c", 0, 0, attrBold),
			cell("d", 0, 0, 0), cell("e", 0, 0, 0), cell("f", 0, 0, 0),
		),
	}

	out, err := CellsToSGR(grid, nil)
	require.NoError(t, err)

	// Expect exactly one bold-on sequence before "abc" and one reset
	// before "def" — not six individual per-cell sequences.
	assert.Equal(t, "\x1b[1mabc\x1b[0mdef", out)

	seqs := parseSGRSequences(t, out)
	require.Len(t, seqs, 2, "expected exactly one bold-on and one reset sequence, got %v", seqs)
	assert.Equal(t, []int{1}, seqs[0])
	assert.Equal(t, []int{0}, seqs[1])
}

func TestCellSGRRenderer_ShouldEmitNoRedundantSGRCodes_WhenARowHasNoAttributeChanges(t *testing.T) {
	grid := []*v1.Row{
		row(
			cell("x", packRGB(1, 2, 3), 0, attrUnderline),
			cell("y", packRGB(1, 2, 3), 0, attrUnderline),
			cell("z", packRGB(1, 2, 3), 0, attrUnderline),
		),
	}

	out, err := CellsToSGR(grid, nil)
	require.NoError(t, err)

	seqs := parseSGRSequences(t, out)
	// One sequence to enter the run, one trailing reset at end-of-line —
	// never a sequence per identical cell.
	require.Len(t, seqs, 2)
	assert.Equal(t, []int{4, 38, 2, 1, 2, 3}, seqs[0])
	assert.Equal(t, []int{0}, seqs[1])
	require.True(t, strings.HasSuffix(out, "\x1b[0m"))
	assert.Contains(t, out, "xyz")
}

func TestCellSGRRenderer_ShouldRoundTripThroughARealANSIParser_WhenColorTransitionsIncludeTruecolor(t *testing.T) {
	// A single bold-red(truecolor) cell, followed by a plain-blue(256-color)
	// cell — a genuine color transition mid-row, not just an attribute
	// toggle.
	grid := []*v1.Row{
		row(
			cell("R", packRGB(255, 0, 0), 0, attrBold),
			cell("B", packIndexed(21), 0, 0),
		),
	}

	out, err := CellsToSGR(grid, nil)
	require.NoError(t, err)

	// Exact escape byte sequence for "bold, truecolor red": SGR 1 (bold)
	// plus 38;2;255;0;0 (truecolor foreground), combined into one escape.
	require.True(t, strings.HasPrefix(out, "\x1b[1;38;2;255;0;0mR"), "got %q", out)

	seqs := parseSGRSequences(t, out)
	require.Len(t, seqs, 3, "bold-red-truecolor, then a reset+256-color transition, then trailing reset: got %v", seqs)
	assert.Equal(t, []int{1, 38, 2, 255, 0, 0}, seqs[0])
	// Dropping bold forces a reset+reapply; the second cell's fg is a
	// 256-color index (38;5;21), not truecolor.
	assert.Equal(t, []int{0, 38, 5, 21}, seqs[1])
	assert.Equal(t, []int{0}, seqs[2])
}

func TestCellSGRRenderer_ShouldNewlineJoinRows_WhenGridHasMultipleRows(t *testing.T) {
	grid := []*v1.Row{
		row(cell("a", 0, 0, 0)),
		row(cell("b", 0, 0, 0)),
	}

	out, err := CellsToSGR(grid, nil)

	require.NoError(t, err)
	assert.Equal(t, "a\nb", out)
}

func TestCellSGRRenderer_ShouldReturnEmptyString_WhenGridIsEmpty(t *testing.T) {
	out, err := CellsToSGR(nil, nil)

	require.NoError(t, err)
	assert.Equal(t, "", out)
}

func TestCellSGRRenderer_ShouldPrintSpace_WhenCellIsEmptyFromCursorSkip(t *testing.T) {
	grid := []*v1.Row{row(cell("a", 0, 0, 0), cell("", 0, 0, 0), cell("b", 0, 0, 0))}

	out, err := CellsToSGR(grid, nil)

	require.NoError(t, err)
	assert.Equal(t, "a b", out)
}

func TestCellSGRRenderer_ShouldPrintNothing_WhenEmptyCellFollowsWideGlyph(t *testing.T) {
	grid := []*v1.Row{row(cell("世", 0, 0, 0), cell("", 0, 0, 0), cell("界", 0, 0, 0), cell("", 0, 0, 0), cell("", 0, 0, 0), cell("x", 0, 0, 0))}

	out, err := CellsToSGR(grid, nil)

	require.NoError(t, err)
	assert.Equal(t, "世界 x", out)
}

func TestCellSGRRenderer_ShouldEmitSGR2_WhenCellIsFaint(t *testing.T) {
	grid := []*v1.Row{row(cell("d", 0, 0, attrFaint), cell("p", 0, 0, 0))}

	out, err := CellsToSGR(grid, nil)

	require.NoError(t, err)
	assert.Equal(t, "\x1b[2md\x1b[0mp", out)
}

func TestCellSGRRenderer_ShouldResetBeforeBold_WhenFaintCellIsFollowedByBoldCell(t *testing.T) {
	grid := []*v1.Row{row(cell("d", 0, 0, attrFaint), cell("b", 0, 0, attrBold))}

	out, err := CellsToSGR(grid, nil)

	require.NoError(t, err)
	assert.Equal(t, "\x1b[2md\x1b[0;1mb\x1b[0m", out)
}

func styledCell(text string, attrs, ulStyle, ulColor, link uint32) *v1.Cell {
	return &v1.Cell{Text: text, Attrs: attrs, UnderlineStyle: ulStyle, UnderlineColor: ulColor, Hyperlink: link}
}

func TestCellSGRRenderer_ShouldEmitExactBytes_ForExtendedAttributes(t *testing.T) {
	links := []*v1.Hyperlink{{Uri: "https://example.com/a?x=1;y=2", Id: "k1"}, {Uri: "file:///tmp/b"}}
	tests := []struct {
		name  string
		cells []*v1.Cell
		want  string
	}{
		{"strikethrough", []*v1.Cell{styledCell("s", attrStrike, 0, 0, 0)}, "\x1b[9ms\x1b[0m"},
		{"blink", []*v1.Cell{styledCell("b", attrBlink, 0, 0, 0)}, "\x1b[5mb\x1b[0m"},
		{"conceal", []*v1.Cell{styledCell("c", attrConceal, 0, 0, 0)}, "\x1b[8mc\x1b[0m"},
		{"overline", []*v1.Cell{styledCell("o", attrOverline, 0, 0, 0)}, "\x1b[53mo\x1b[0m"},
		{"single underline stays plain 4", []*v1.Cell{styledCell("u", attrUnderline, 0, 0, 0)}, "\x1b[4mu\x1b[0m"},
		{"double underline", []*v1.Cell{styledCell("u", attrUnderline, 1, 0, 0)}, "\x1b[4:2mu\x1b[0m"},
		{"curly underline", []*v1.Cell{styledCell("u", attrUnderline, 2, 0, 0)}, "\x1b[4:3mu\x1b[0m"},
		{"dotted underline", []*v1.Cell{styledCell("u", attrUnderline, 3, 0, 0)}, "\x1b[4:4mu\x1b[0m"},
		{"dashed underline", []*v1.Cell{styledCell("u", attrUnderline, 4, 0, 0)}, "\x1b[4:5mu\x1b[0m"},
		{"underline color indexed", []*v1.Cell{styledCell("u", attrUnderline, 0, packIndexed(196), 0)}, "\x1b[4;58;5;196mu\x1b[0m"},
		{"underline color rgb with curly", []*v1.Cell{styledCell("u", attrUnderline, 2, packRGB(1, 2, 3), 0)}, "\x1b[4:3;58;2;1;2;3mu\x1b[0m"},
		{"underline color ignored without underline", []*v1.Cell{styledCell("u", 0, 2, packIndexed(9), 0)}, "u"},
		{"underline color dropped forces reset", []*v1.Cell{styledCell("a", attrUnderline, 0, packIndexed(9), 0), styledCell("b", attrUnderline, 0, 0, 0)}, "\x1b[4;58;5;9ma\x1b[0;4mb\x1b[0m"},
		{"underline style change re-emits", []*v1.Cell{styledCell("a", attrUnderline, 2, 0, 0), styledCell("b", attrUnderline, 0, 0, 0)}, "\x1b[4:3ma\x1b[4mb\x1b[0m"},
		{"bold and faint together", []*v1.Cell{styledCell("x", attrBold|attrFaint, 0, 0, 0)}, "\x1b[1;2mx\x1b[0m"},
		{"hyperlink with id", []*v1.Cell{styledCell("a", 0, 0, 0, 1), styledCell("b", 0, 0, 0, 1), styledCell("c", 0, 0, 0, 0)},
			"\x1b]8;id=k1;https://example.com/a?x=1;y=2\x1b\\ab\x1b]8;;\x1b\\c"},
		{"hyperlink without id closes at row end", []*v1.Cell{styledCell("a", 0, 0, 0, 2)}, "\x1b]8;;file:///tmp/b\x1b\\a\x1b]8;;\x1b\\"},
		{"adjacent hyperlinks switch directly", []*v1.Cell{styledCell("a", 0, 0, 0, 1), styledCell("b", 0, 0, 0, 2)},
			"\x1b]8;id=k1;https://example.com/a?x=1;y=2\x1b\\a\x1b]8;;file:///tmp/b\x1b\\b\x1b]8;;\x1b\\"},
		{"hyperlink survives SGR change", []*v1.Cell{styledCell("a", 0, 0, 0, 2), styledCell("b", attrBold, 0, 0, 2)},
			"\x1b]8;;file:///tmp/b\x1b\\a\x1b[1mb\x1b]8;;\x1b\\\x1b[0m"},
		{"out-of-range link index prints plain", []*v1.Cell{styledCell("a", 0, 0, 0, 9)}, "\x1b]8;;\x1b\\a\x1b]8;;\x1b\\"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := CellsToSGR([]*v1.Row{row(tt.cells...)}, links)
			require.NoError(t, err)
			assert.Equal(t, tt.want, out)
		})
	}
}

func TestCellSGRRenderer_ShouldNotLetDaemonStringsInjectEscapes_WhenHyperlinkHasControlBytes(t *testing.T) {
	links := []*v1.Hyperlink{{Uri: "http://x/\x1b[2Jevil\x07", Id: "a;b:c\x1b"}}

	out, err := CellsToSGR([]*v1.Row{row(styledCell("a", 0, 0, 0, 1))}, links)

	require.NoError(t, err)
	assert.Equal(t, "\x1b]8;id=abc;http://x/[2Jevil\x1b\\a\x1b]8;;\x1b\\", out)
}
