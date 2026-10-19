package server

import (
	"context"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// wireGuidanceMirror gives the approval handler its durable hook-question mirror
// and runs the session reaper on the server's background group, so Shutdown
// joins it once serverCtx is cancelled.
func wireGuidanceMirror(h *services.ApprovalHandler, deps *ServerDependencies, serverCtx context.Context, goBackground func(func())) {
	if deps.Storage == nil || deps.EventBus == nil || deps.SessionService == nil {
		return
	}
	count := func(name string) func(string) {
		gate := deps.SessionService.DeliveryGate()
		if gate == nil {
			return nil
		}
		return func(reason string) { gate.Metrics().Add(name, reason) }
	}
	cleared := count(deliverygate.CounterGuidanceMirrorCleared)
	mirror := services.NewGuidanceHookMirror(deps.Storage, deps.SessionService.PendingQuestions(),
		count(deliverygate.CounterGuidanceMirrorFailed), cleared)
	h.SetGuidanceMirror(mirror)
	reaper := services.NewGuidanceSessionReaper(deps.Storage, storageSessionEnds{storage: deps.Storage}, mirror, cleared)
	goBackground(func() { reaper.Run(serverCtx, deps.EventBus) })
}

// storageSessionEnds answers the reaper's session questions from stored instances.
type storageSessionEnds struct{ storage *session.Storage }

func (s storageSessionEnds) find(match func(session.InstanceData) bool) (session.InstanceData, bool, bool) {
	instances, err := s.storage.ListInstanceData()
	if err != nil {
		log.Warn("[GuidanceReaper] list instances failed", "err", err)
		return session.InstanceData{}, false, false
	}
	for _, d := range instances {
		if match(d) {
			return d, true, true
		}
	}
	return session.InstanceData{}, false, true
}

// ResolveUUID maps a UUID or title to a UUID. A deleted session is no longer
// stored, so an id that is not a title is returned as given.
func (s storageSessionEnds) ResolveUUID(id string) string {
	if d, ok, _ := s.find(func(d session.InstanceData) bool { return d.UUID == id }); ok {
		return d.UUID
	}
	if d, ok, _ := s.find(func(d session.InstanceData) bool { return d.Title == id }); ok {
		return d.UUID
	}
	return id
}

// Ended reports archived, stopped or missing. A storage error is not "ended".
func (s storageSessionEnds) Ended(sessionUUID string) bool {
	d, ok, listed := s.find(func(d session.InstanceData) bool { return d.UUID == sessionUUID })
	if !listed {
		return false
	}
	return !ok || d.ArchivedAt != nil || d.Status == session.Stopped
}
