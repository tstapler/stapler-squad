package tymux

import (
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// CellsToSGR renders a PaneSnapshot's cell grid (PaneSnapshot.grid) into an
// ANSI/SGR-encoded string matching `tmux capture-pane -p -e`'s
// attribute-preserving output shape — the renderer CapturePaneContent()
// (Task 2.2.2d) needs and CapturePaneContentRaw() deliberately doesn't
// (that method only joins Cell.text, no SGR).
//
// Epic 2.2 wires the call site so the package compiles end-to-end; Epic 2.6
// (Story 2.6.1, "CellSGRRenderer") implements the actual per-row cell-diff
// SGR walk — see plan.md's Epic 2.6 for the packed-color/attribute-bitflag
// unpacking this will need.
func CellsToSGR(grid []*v1.Row) (string, error) {
	return "", ErrNotImplemented
}
