package server

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services"
)

// wireHookProofsAndQuestions installs the process-wide hook proofs, feeds the
// approval handler's question registry and, once per boot, refreshes every live
// local session's proof file and stale hook entry in the background.
func wireHookProofsAndQuestions(h *services.ApprovalHandler, deps *ServerDependencies) {
	if dir, err := config.GetConfigDir(); err != nil {
		log.Warn("hook proofs disabled: config dir unavailable", "err", err)
	} else if proofs, perr := services.NewHookProofs(dir); perr != nil {
		log.Warn("hook proofs disabled: secret unavailable", "err", perr)
	} else {
		services.SetDefaultHookProofs(proofs)
		h.SetHookProofs(proofs)
	}
	var count func(cause string)
	if gate := deps.SessionService.DeliveryGate(); gate != nil {
		count = func(cause string) { gate.Metrics().Add(deliverygate.CounterReplyUnreplyable, cause) }
	}
	h.SetQuestionRegistry(deps.SessionService.PendingQuestions(), count)
	if deps.ReviewQueuePoller != nil && services.DefaultHookProofs() != nil {
		go func() {
			n := services.RefreshHookProofs(deps.ReviewQueuePoller.GetInstances())
			log.Info("[HookProof] refreshed live sessions", "sessions", n)
		}()
	}
}
