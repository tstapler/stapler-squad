package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
)

func healthTestInstance(t *testing.T, uuid string) *session.Instance {
	t.Helper()
	li := session.NewLiveInstance(&session.Instance{Title: "s-" + uuid})
	t.Cleanup(li.Stop)
	if uuid != "" {
		li.SetClaudeConversationUUID(uuid)
	}
	return li.Instance
}

func loopSignals(repeats int) tokens.ContextHealthSignals {
	return tokens.ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: repeats, RepeatedToolName: "Bash"}
}

func TestApplyContextHealth_PublishesOnLevelChangeOnly(t *testing.T) {
	inst := healthTestInstance(t, "abc-123")
	current := &tokens.ParseResult{ContextHealth: loopSignals(3)}
	lookup := func(string) *tokens.ParseResult { return current }
	var published int
	publish := func(*session.Instance) { published++ }
	run := func() {
		applyContextHealth([]*session.Instance{inst}, lookup, config.ContextHealthConfig{}, publish)
	}

	run() // unknown -> amber
	assert.Equal(t, 1, published)
	assert.Equal(t, tokens.HealthAmber, inst.Snapshot().ContextHealth.Level)

	run() // unchanged
	assert.Equal(t, 1, published)

	current = &tokens.ParseResult{ContextHealth: loopSignals(4)} // same level, new reason
	run()
	assert.Equal(t, 1, published, "reason-only change must not publish")
	assert.Equal(t, "Repeated the same Bash call 4 times in a row", inst.Snapshot().ContextHealth.Reason)

	current = &tokens.ParseResult{ContextHealth: loopSignals(6)} // amber -> red
	run()
	assert.Equal(t, 2, published)
	assert.Equal(t, tokens.HealthRed, inst.Snapshot().ContextHealth.Level)
}

func TestApplyContextHealth_SkipsInstancesWithoutTranscript(t *testing.T) {
	noUUID := healthTestInstance(t, "")
	noParse := healthTestInstance(t, "missing")
	var published int
	applyContextHealth([]*session.Instance{noUUID, noParse},
		func(string) *tokens.ParseResult { return nil },
		config.ContextHealthConfig{},
		func(*session.Instance) { published++ })
	assert.Equal(t, 0, published)
	assert.Equal(t, tokens.HealthUnknown, noUUID.Snapshot().ContextHealth.Level)
	assert.Equal(t, tokens.HealthUnknown, noParse.Snapshot().ContextHealth.Level)
}
