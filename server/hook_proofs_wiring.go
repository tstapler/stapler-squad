package server

import (
	"context"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services/approvals"
)

// wireHookProofsAndQuestions installs the process-wide hook proofs, feeds the
// approval handler's question registry and, once per boot, refreshes every live
// local session's proof file and stale hook entry in the background.
func wireHookProofsAndQuestions(h *approvals.ApprovalHandler, deps *ServerDependencies, goBackground func(func()), serverCtx context.Context) {
	if dir, err := config.GetConfigDir(); err != nil {
		log.Warn("hook proofs disabled: config dir unavailable", "err", err)
	} else if proofs, perr := approvals.NewHookProofs(dir); perr != nil {
		log.Warn("hook proofs disabled: secret unavailable", "err", perr)
	} else {
		approvals.SetDefaultHookProofs(proofs)
		h.SetHookProofs(proofs)
	}
	var count func(cause string)
	if gate := deps.SessionService.DeliveryGate(); gate != nil {
		count = func(cause string) { gate.Metrics().Add(deliverygate.CounterReplyUnreplyable, cause) }
	}
	h.SetQuestionRegistry(deps.SessionService.PendingQuestions(), count)
	wireGuidanceMirror(h, deps, serverCtx, goBackground)
	if deps.ReviewQueuePoller != nil && approvals.DefaultHookProofs() != nil {
		goBackground(func() {
			n := approvals.RefreshHookProofs(deps.ReviewQueuePoller.GetInstances())
			log.Info("[HookProof] refreshed live sessions", "sessions", n)
		})
	}
}
