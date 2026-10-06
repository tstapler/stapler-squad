package git

import (
	"fmt"
	"strings"
)

// ConflictMarkerStyle names which git.conflictStyle format renderConflictHunk emits.
//
// Confirmed in this environment (2026-09-06, per plan.md's Unresolved Questions):
// `git config --global --get merge.conflictStyle` is unset (real git's own default,
// "merge" style — no `|||||||` base section); this repo's local .git/config happens
// to have `merge.conflictstyle = diff3` set, which this project deliberately does
// NOT follow — matching the *global* default (what a fresh clone/CI checkout
// produces) is the byte-compatibility target, not this one checkout's local override.
type ConflictMarkerStyle int

const (
	// MergeStyleDefault renders 7-character <<<<<<</=======/>>>>>>> markers with no
	// ||||||| base section — git's default merge.conflictStyle ("merge").
	MergeStyleDefault ConflictMarkerStyle = iota
)

// renderConflictHunk renders hunk (which must be RegionConflict) as git-byte-compatible
// conflict-marker text: "<<<<<<< <oursLabel>\n<ours lines>\n=======\n<theirs
// lines>\n>>>>>>> <theirsLabel>\n" — MergeStyleDefault, the only style this project
// targets (see ConflictMarkerStyle).
func renderConflictHunk(hunk MergeHunk, oursLabel, theirsLabel string) (string, error) {
	if hunk.Kind != RegionConflict {
		return "", fmt.Errorf("renderConflictHunk: hunk kind must be RegionConflict, got %v", hunk.Kind)
	}

	var b strings.Builder
	b.WriteString("<<<<<<< ")
	b.WriteString(oursLabel)
	b.WriteString("\n")
	// Real git omits the trailing newline after an empty side (a delete-vs-modify
	// conflict) rather than emitting a blank line — join+"\n" unconditionally would
	// add one that never appears in real git's output.
	if len(hunk.Ours) > 0 {
		b.WriteString(strings.Join(hunk.Ours, "\n"))
		b.WriteString("\n")
	}
	b.WriteString("=======\n")
	if len(hunk.Theirs) > 0 {
		b.WriteString(strings.Join(hunk.Theirs, "\n"))
		b.WriteString("\n")
	}
	b.WriteString(">>>>>>> ")
	b.WriteString(theirsLabel)
	b.WriteString("\n")
	return b.String(), nil
}
