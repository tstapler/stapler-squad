package approvals

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spike S7 item 6: the injected PostToolUse groups carry no matcher, so they run
// for every tool including AskUserQuestion and the acknowledgement receipt works.
func TestInjectHooksConfig_ShouldNotRestrictPostToolUseToATool_SoAskUserQuestionReceiptsArrive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, InjectHooksConfig(dir, "s", []HookName{HookPostToolLogging}))

	var hooks map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(readSettings(t, dir)["hooks"], &hooks))
	var groups []HookMatcherGroup
	require.NoError(t, json.Unmarshal(hooks["PostToolUse"], &groups))

	require.NotEmpty(t, groups)
	for _, g := range groups {
		assert.Empty(t, g.Matcher, "a matcher would have to name AskUserQuestion")
	}
}
