package healthnotify

import (
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/instance"
)

func newGoldenStale(bus *events.EventBus, insts ...*instance.Instance) *StaleSessionNotifier {
	return NewStaleSessionNotifier(newTestNotifierPoller(insts...), bus, testPorts())
}

func newGoldenMemory(bus *events.EventBus, insts ...*instance.Instance) *MemoryPressureNotifier {
	return NewMemoryPressureNotifier(newTestNotifierPoller(insts...), bus, testPorts())
}
