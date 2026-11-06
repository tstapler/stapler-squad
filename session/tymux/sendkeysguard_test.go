package tymux

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/sendkeysguard"
)

// TestTymuxPackage_NoAppendCarriageReturnConcatenation is the session/tymux
// instance of the BUG-031 structural guard (see sendkeysguard's doc comment):
// SendPromptWithEnter here used to fold a prompt and its Enter keystroke into
// one append(p, 0x0D) []byte before a single AttachRequest_Input send —
// architecture.md §1's "parity risk" against TmuxProcessManager's two-step
// shape. This closed that gap; a revert must make this test fail.
func TestTymuxPackage_NoAppendCarriageReturnConcatenation(t *testing.T) {
	t.Parallel()
	sendkeysguard.CheckNoAppendCarriageReturnConcatenation(t, ".")
}
