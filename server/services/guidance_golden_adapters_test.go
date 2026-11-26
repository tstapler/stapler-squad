package services

import (
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/notify"
	"github.com/tstapler/stapler-squad/session"
)

// The constructors below are the only lines of the guidance golden that may differ between builds.

const goldenAckTool = guidance.AskUserQuestionTool

func goldenGuidanceService(storage *session.Storage, bus *events.EventBus, respawner guidance.TriageRespawner) *guidance.GuidanceRequestService {
	return guidance.NewGuidanceRequestService(storage, &notify.EventBusNotifier{Bus: bus}, respawner)
}

func goldenSeedHandler(svc *SessionService, storage *session.Storage) *guidance.GuidanceDebugSeedHandler {
	return guidance.NewGuidanceDebugSeedHandler(svc.GuidanceSeedPorts(), storage)
}
