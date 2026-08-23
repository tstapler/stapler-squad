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

	out, err := CellsToSGR(grid)

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

	out, err := CellsToSGR(grid)
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

	out, err := CellsToSGR(grid)
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

	out, err := CellsToSGR(grid)
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

	out, err := CellsToSGR(grid)

	require.NoError(t, err)
	assert.Equal(t, "a\nb", out)
}

func TestCellSGRRenderer_ShouldReturnEmptyString_WhenGridIsEmpty(t *testing.T) {
	out, err := CellsToSGR(nil)

	require.NoError(t, err)
	assert.Equal(t, "", out)
}
