package autonomysvc

import (
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// newTestService builds the service the way SessionService wires it.
func newTestService(storage *session.Storage, bus *events.EventBus) *AutonomousOrchestrationService {
	a := NewAutonomousOrchestrationService(nil, bus)
	a.SetStorageGetter(func() *session.Storage { return storage })
	return a
}

// installOnGate puts a delivery gate (flag on) in front of bus and seeds the
// hidden entries: hidden-session suppression is the gate's job.
func installOnGate(bus *events.EventBus, entries ...deliverygate.Entry) {
	gate := deliverygate.NewGate(deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
		return deliverygate.FlagSettings{Global: true}, nil
	}))
	gate.Flags().Reload()
	gate.Index().Replace(entries)
	bus.SetPublishFilter(gate.PublishFilter())
}
