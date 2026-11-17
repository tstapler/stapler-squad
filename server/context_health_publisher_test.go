package server

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/tokens"
)

func healthTestInstance(t *testing.T, uuid string) *instance.Instance {
	t.Helper()
	li := instance.NewLiveInstance(&instance.Instance{Title: "s-" + uuid})
	t.Cleanup(li.Stop)
	if uuid != "" {
		li.SetClaudeConversationUUID(uuid)
	}
	return li.Instance
}

func loopSignals(repeats int) domain.ContextHealthSignals {
	return domain.ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: repeats, RepeatedToolName: "Bash"}
}

func TestApplyContextHealth_PublishesOnLevelChangeOnly(t *testing.T) {
	inst := healthTestInstance(t, "abc-123")
	current := &tokens.ParseResult{ContextHealth: loopSignals(3)}
	lookup := func(string) *tokens.ParseResult { return current }
	var published int
	publish := func(*instance.Instance) { published++ }
	run := func() {
		applyContextHealth([]*instance.Instance{inst}, lookup, config.ContextHealthConfig{}, publish)
	}

	run() // unknown -> amber
	assert.Equal(t, 1, published)
	assert.Equal(t, domain.HealthAmber, inst.Snapshot().ContextHealth.Level)

	run() // unchanged
	assert.Equal(t, 1, published)

	current = &tokens.ParseResult{ContextHealth: loopSignals(4)} // same level, new reason
	run()
	assert.Equal(t, 1, published, "reason-only change must not publish")
	assert.Equal(t, "Repeated the same Bash call 4 times in a row", inst.Snapshot().ContextHealth.Reason)

	current = &tokens.ParseResult{ContextHealth: loopSignals(6)} // amber -> red
	run()
	assert.Equal(t, 2, published)
	assert.Equal(t, domain.HealthRed, inst.Snapshot().ContextHealth.Level)
}

func TestApplyContextHealth_SkipsInstancesWithoutTranscript(t *testing.T) {
	noUUID := healthTestInstance(t, "")
	noParse := healthTestInstance(t, "missing")
	var published int
	applyContextHealth([]*instance.Instance{noUUID, noParse},
		func(string) *tokens.ParseResult { return nil },
		config.ContextHealthConfig{},
		func(*instance.Instance) { published++ })
	assert.Equal(t, 0, published)
	assert.Equal(t, domain.HealthUnknown, noUUID.Snapshot().ContextHealth.Level)
	assert.Equal(t, domain.HealthUnknown, noParse.Snapshot().ContextHealth.Level)
}
