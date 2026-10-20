package mcp

import (
	"context"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// WakeBackend is the running wake dispatcher as the wake tools use it;
// *wakes.Dispatcher satisfies it.
type WakeBackend interface {
	Schedule(ctx context.Context, req wakes.ScheduleRequest) (*session.WakeTriggerData, bool, error)
	ListByOwner(ctx context.Context, owner string) ([]*session.WakeTriggerData, error)
	Cancel(ctx context.Context, owner, id string) (*session.WakeTriggerData, bool, error)
	NextGeneration(ctx context.Context, owner string) (int, error)
	Limits() wakes.Limits
}

// HelpAnswerer answers a guidance request on behalf of a named answerer, so the
// answer listener (and with it the asker's wake) runs exactly as for a human
// answer; *services.GuidanceRequestService satisfies it.
type HelpAnswerer interface {
	AnswerAs(ctx context.Context, id, answer string, who services.Answerer) (*session.GuidanceRequestData, bool, error)
}

// WakeToolDeps are the late-bound collaborators of the five tools. The
// dispatcher starts after the MCP server is built, so the tools resolve them at
// call time; a nil result (dispatcher not running in this process, no flag) is
// reported as `unavailable`.
type WakeToolDeps struct {
	Backend  WakeBackend
	Owners   wakes.OwnerLookup
	Answerer HelpAnswerer
}

var wakeToolDepsProvider atomic.Pointer[func() *WakeToolDeps]

// SetWakeToolDeps installs the provider the wake tools resolve their
// collaborators from; wiring calls it once the dispatcher is running.
func SetWakeToolDeps(fn func() *WakeToolDeps) { wakeToolDepsProvider.Store(&fn) }

func currentWakeToolDeps() *WakeToolDeps {
	if fn := wakeToolDepsProvider.Load(); fn != nil && *fn != nil {
		return (*fn)()
	}
	return nil
}

// wakeFlags answers the agent_wake_triggers flag matrix at decision time
// (ADR-008): a stale cache can only mis-draw, never allow a write.
type wakeFlags struct{ cfg func() *config.Config }

func (f wakeFlags) config() *config.Config {
	if f.cfg != nil {
		return f.cfg()
	}
	return config.LoadConfig()
}

func triggerTypeNames() []string {
	types := domain.AllWakeTriggerTypes()
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return names
}

// toolOff returns the flag scope that blocks tool, or "" when it may run.
func (f wakeFlags) toolOff(tool string) string {
	if f.config().WakeToolEnabled(tool, triggerTypeNames()) {
		return ""
	}
	return config.WakeToolScope(tool)
}

// triggerOff returns the flag scope that blocks creating a trigger of type t.
func (f wakeFlags) triggerOff(t domain.WakeTriggerType) string {
	if f.config().WakeTriggerEnabled(string(t), triggerTypeNames()) {
		return ""
	}
	return config.WakeTriggerScope(string(t))
}
