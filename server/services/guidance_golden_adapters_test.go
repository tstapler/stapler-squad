package services

import (
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// The constructors below are the only lines of the guidance golden that may differ between builds.

const goldenAckTool = askUserQuestion

func goldenGuidanceService(storage *session.Storage, bus *events.EventBus, respawner TriageRespawner) *GuidanceRequestService {
	return NewGuidanceRequestService(storage, bus, respawner)
}

func goldenSeedHandler(svc *SessionService, storage *session.Storage) *GuidanceDebugSeedHandler {
	return NewGuidanceDebugSeedHandler(svc, storage)
}
