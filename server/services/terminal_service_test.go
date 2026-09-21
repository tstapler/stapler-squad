package services

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/sendkeysguard"
)

// TestServicesPackage_NoDirectSendKeysPlusEnterConcatenation is the
// server/services instance of the BUG-031 structural guard (see
// sendkeysguard's doc comment and session/pane_submit_test.go's sibling
// test) — TerminalService.WriteToSession and SessionService.steerInstance
// both used to build the single-write pattern this guards against.
func TestServicesPackage_NoDirectSendKeysPlusEnterConcatenation(t *testing.T) {
	t.Parallel()
	sendkeysguard.CheckNoSingleWriteEnterConcatenation(t, ".")
}
