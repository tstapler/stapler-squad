// Package sessionstream serves the live terminal and event streams of a session:
// StreamTerminal, WatchSessions, the diff and detection reads beside them, the
// terminal snapshot and write RPCs, and the external-session approval endpoints.
// It never imports server/services; the host passes what it reads as Ports.
package sessionstream

import (
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/federation"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Ports are what the stream RPCs read from their host. Every member must be
// non-nil, and each is called per request, so wiring done after construction is
// seen. StatusManager and Federation may return nil.
type Ports struct {
	// FindPolled returns the review queue poller's instance for id, or nil when
	// there is no poller or it does not know id.
	FindPolled func(id string) *instance.Instance
	// Polled returns the poller's instances; ok is false when there is no poller.
	Polled func() (instances []*instance.Instance, ok bool)
	// LoadWired loads and wires the stored instances; the fallback when no poller
	// can answer.
	LoadWired func() ([]*instance.Instance, error)
	// FindInstance returns the live instance for id, or nil.
	FindInstance     func(id string) *instance.Instance
	ListInstanceData func() ([]domain.InstanceData, error)
	StatusManager    func() *instance.InstanceStatusManager
	Federation       func() *federation.FederatedSessions
}

// Handler implements the stream RPCs of SessionService.
type Handler struct {
	bus   *events.EventBus
	ports Ports
}

// New returns a Handler publishing to and subscribing on bus.
func New(bus *events.EventBus, ports Ports) *Handler {
	return &Handler{bus: bus, ports: ports}
}
