package session

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/domain"
)

// scheduleWakeAdvertised reports whether prompts should point agents at the
// schedule_wake MCP tool: its own flag row (ADR-008), which needs the
// dispatcher running. A var, not a const, so prompt tests can pin both states.
var scheduleWakeAdvertised = func() bool {
	types := domain.AllWakeTriggerTypes()
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return config.LoadConfig().WakeToolEnabled("schedule_wake", names)
}

// scheduleWakeGuidance replaces polling with the durable tool. The ban on the
// client-side wake tool and /loop already sits in the text it is appended to;
// repeating it here would add a Go reference the polling guard test forbids.
const scheduleWakeGuidance = "To wait for a later time or event (CI finishing, a PR merging, another session becoming idle, " +
	"a reply to a help request), call the schedule_wake MCP tool and end your turn: the app prompts you again when it " +
	"arrives, even across a restart. Do not poll."

// taskProtocol is the task protocol block, with the schedule_wake step when that tool is on.
func taskProtocol() string {
	if !scheduleWakeAdvertised() {
		return taskProtocolBlock
	}
	return taskProtocolBlock + "\n10. " + scheduleWakeGuidance
}

// withWakeGuidance appends the schedule_wake paragraph to a rendered command or
// prompt; flag off leaves text unchanged byte for byte.
func withWakeGuidance(text string) string {
	if !scheduleWakeAdvertised() {
		return text
	}
	return text + "\n" + scheduleWakeGuidance + "\n"
}
