package services

import (
	"net/http"

	"github.com/tstapler/stapler-squad/log"
)

const ghActionReopened = "reopened"

// handlePullRequestEvent routes a verified pull_request.reopened delivery to the
// poller so a terminal row can move back to open (plan Story 3.3 / K1). Other
// pull_request actions are accepted and ignored. No audit row is written: this
// path only refreshes poller state.
func (h *GitHubWebhookHandler) handlePullRequestEvent(w http.ResponseWriter, r *http.Request, payload map[string]interface{}, body []byte) {
	action, _ := payload["action"].(string)
	fullName, ok := payloadRepoFullName(payload)
	pr, _ := payload["pull_request"].(map[string]interface{})
	number, hasNumber := pr["number"].(float64)
	if action != ghActionReopened || !ok || !hasNumber || h.prPollerInvalidator == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	ctx := r.Context()
	verified, err := h.verifySignatureForRepo(ctx, fullName, body, r.Header.Get("X-Hub-Signature-256"))
	if err != nil {
		log.Error("[GitHubWebhookHandler] failed to verify pull_request.reopened signature", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !verified {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	h.prPollerInvalidator.InvalidateForReopened(ctx, fullName, int(number))
	w.WriteHeader(http.StatusOK)
}
