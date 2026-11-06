package services

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// T-OS-01: the Slack formatter and webhook sender are unchanged by the gate
// being compiled in. Their payload shape is pinned by the
// TestNotify*_Posts*Payload_ToHTTPTestServer and
// TestNotify*_UseDescriptiveLinkText tests in slack_notifier_test.go; this
// guard pins that neither Slack source file can see the gate, so no gate state
// can alter formatting or routing at this layer (gating happens upstream, in
// the review-queue path: server/slack_gate_test.go).
func TestSlackFormatter_ShouldBeUnchanged_WhenGateCompiledIn(t *testing.T) {
	for _, name := range []string{"slack_notifier.go", "slack_interactive_handler.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		require.NoError(t, err)
		for _, imp := range f.Imports {
			require.False(t, strings.Contains(imp.Path.Value, "deliverygate"),
				"%s must not import the delivery gate (found %s)", name, imp.Path.Value)
		}
	}
}
