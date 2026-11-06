package server

import (
	"context"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// contextHealthSource is the slice of TokenStore the publisher needs.
type contextHealthSource interface {
	Subscribe() <-chan *tokens.ParseResult
	GetByUUID(uuid string) *tokens.ParseResult
}

// applyContextHealth evaluates every instance's verdict from the token store and
// stores it. publish is called only when the Level changes; a Reason-only change
// (e.g. repeat count 3→4) is stored silently so the tooltip stays current.
func applyContextHealth(
	instances []*session.Instance,
	lookup func(uuid string) *tokens.ParseResult,
	cfg config.ContextHealthConfig,
	publish func(*session.Instance),
) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("[ContextHealth] recovered panic in applyContextHealth", "panic", r)
		}
	}()
	for _, inst := range instances {
		uuid := inst.GetClaudeConversationUUID()
		if uuid == "" {
			continue
		}
		pr := lookup(uuid)
		if pr == nil {
			continue
		}
		verdict := tokens.EvaluateContextHealth(pr.ContextHealth, cfg)
		cur := inst.Snapshot().ContextHealth
		if verdict == cur {
			continue
		}
		prev := cur.Level
		inst.SetContextHealth(verdict)
		if verdict.Level == prev {
			continue
		}
		log.Info("[ContextHealth] level transition",
			"session", inst.Title,
			"from", prev.String(),
			"to", verdict.Level.String(),
			"reason", verdict.Reason,
			"tool_calls_in_window", verdict.Signals.ToolCallsInWindow)
		publish(inst)
	}
}

// publishContextHealth re-evaluates all live instances on each TokenStore
// notification until ctx is cancelled. Per-notification cost is O(instances).
func publishContextHealth(
	ctx context.Context,
	src contextHealthSource,
	instances func() []*session.Instance,
	cfg func() config.ContextHealthConfig,
	publish func(*session.Instance),
) {
	ch := src.Subscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ch:
			if !ok {
				return
			}
			applyContextHealth(instances(), src.GetByUUID, cfg(), publish)
		}
	}
}
