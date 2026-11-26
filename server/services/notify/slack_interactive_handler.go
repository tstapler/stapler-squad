package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// maxSlackInteractiveBodyBytes caps the request body this now-internet-
// facing endpoint will read. Slack's interactive-component payloads are a
// few KB at most; this is generous headroom, not a tight fit, per
// research/pitfalls.md §5 item 7's "standard HTTP hardening" note.
const maxSlackInteractiveBodyBytes = 1 << 20 // 1 MiB

// SlackInteractivePayload is the subset of Slack's interactive-component
// callback payload this handler needs: which button was clicked and who
// clicked it (User, for audit logging -- see Handle's success-path log line).
type SlackInteractivePayload struct {
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
}

// approvalResolver is the narrow interface SlackInteractiveHandler needs —
// defined here, at the consumer, per
// the `interface-pollution-checklist` skill. Satisfied by
// *SessionService (session_service.go's ResolveApproval forwards to its
// private *ApprovalService).
type approvalResolver interface {
	ResolveApproval(ctx context.Context, req *connect.Request[sessionv1.ResolveApprovalRequest]) (*connect.Response[sessionv1.ResolveApprovalResponse], error)
}

// SlackInteractiveHandler handles POST /api/hooks/slack-interactive: Slack's
// interactive-component callback for the Approve/Deny buttons added to
// outbound approval-pending messages (Phase 2, Story 2.1.2). Only registered
// at all when cfg.Slack.ApprovalEnabled (server.go, Story 2.1.3).
type SlackInteractiveHandler struct {
	resolver approvalResolver
	replays  *slackReplayGuard
}

// NewSlackInteractiveHandler constructs a SlackInteractiveHandler. The
// signing secret is resolved live from config.LoadConfig() on every request
// (via resolveSlackSigningSecret) rather than captured once at construction
// time, matching this package's established "read live config, don't
// snapshot it" convention (see slack_notifier.go's resolveSlackWebhookURL
// call sites and hookBaseURLFn's doc comment in server/server.go).
func NewSlackInteractiveHandler(resolver approvalResolver) *SlackInteractiveHandler {
	return &SlackInteractiveHandler{
		resolver: resolver,
		replays:  newSlackReplayGuard(time.Now, slackReplayGuardMaxEntries),
	}
}

// Handle reads the raw body exactly once and uses those bytes for both
// signature verification and payload parsing. The URL query and Content-Type
// are not signed, so nothing is read from them (never r.FormValue/r.Form).
// Any verification failure gets a generic 401. A repeat of a handled request
// is acknowledged without acting again, one still in progress gets 409, and a
// request that failed to resolve may be retried.
func (h *SlackInteractiveHandler) Handle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSlackInteractiveBodyBytes)
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	cfg := config.LoadConfig()
	secret, err := resolveSlackSigningSecret(cfg)
	if err != nil {
		log.Warn("SlackInteractiveHandler: failed to resolve signing secret", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if secret == "" {
		// Fail closed (§5 item 6): ApprovalEnabled without a configured
		// signing secret must reject every request, never "verification
		// unavailable, allow by default."
		log.Warn("SlackInteractiveHandler: rejecting request, no signing secret configured")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := verifySlackSignature(secret, r.Header, rawBody); err != nil {
		log.Warn("SlackInteractiveHandler: signature verification failed", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	payload, err := parseSlackInteractivePayload(rawBody)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	key := slackRequestKey{
		timestamp: r.Header.Get("X-Slack-Request-Timestamp"),
		signature: r.Header.Get("X-Slack-Signature"),
	}
	if claim := h.replays.claim(key); claim != slackClaimGranted {
		answerUnclaimedSlackRequest(w, claim)
		return
	}
	succeeded := false
	defer func() {
		if succeeded {
			h.replays.commit(key)
		} else {
			h.replays.release(key)
		}
	}()
	succeeded = h.resolveActions(r.Context(), payload)

	w.WriteHeader(http.StatusOK)
}

// answerUnclaimedSlackRequest responds to a verified request that must not be
// acted on.
func answerUnclaimedSlackRequest(w http.ResponseWriter, claim slackClaim) {
	switch claim {
	case slackClaimProcessed:
		log.Info("SlackInteractiveHandler: ignoring repeat of an already processed request")
		w.WriteHeader(http.StatusOK)
	case slackClaimInFlight:
		http.Error(w, "request already in progress", http.StatusConflict)
	case slackClaimFull:
		log.Warn("SlackInteractiveHandler: rejecting request, replay cache is full")
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	default:
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
}

// parseSlackInteractivePayload decodes the form field "payload" from the
// signature-verified body bytes.
func parseSlackInteractivePayload(rawBody []byte) (SlackInteractivePayload, error) {
	var payload SlackInteractivePayload
	form, err := url.ParseQuery(string(rawBody))
	if err != nil {
		return payload, err
	}
	err = json.Unmarshal([]byte(form.Get("payload")), &payload)
	return payload, err
}

// resolveActions resolves every recognized button in payload and reports
// whether all of those resolutions succeeded.
func (h *SlackInteractiveHandler) resolveActions(ctx context.Context, payload SlackInteractivePayload) bool {
	succeeded := true
	for _, action := range payload.Actions {
		approvalID, decision, ok := parseSlackActionValue(action.Value)
		if !ok {
			continue
		}
		req := connect.NewRequest(&sessionv1.ResolveApprovalRequest{
			ApprovalId: approvalID,
			Decision:   decision,
		})
		if _, err := h.resolver.ResolveApproval(ctx, req); err != nil {
			succeeded = false
			log.Warn("SlackInteractiveHandler: ResolveApproval failed", "approval_id", approvalID, "slack_user", payload.User.Username, "error", err)
		} else {
			// Audit trail: this is an internet-facing endpoint granting/denying
			// tool-execution approval, so a successful outcome needs the same
			// visibility a failure already gets, including who did it.
			log.Info("SlackInteractiveHandler: approval resolved via Slack", "approval_id", approvalID, "decision", decision, "slack_user", payload.User.Username)
		}
	}
	return succeeded
}

// parseSlackActionValue splits a button's value ("<approvalID>:allow" or
// "<approvalID>:deny") into the approval ID and the decision string
// ApprovalService.ResolveApproval expects (those exact literals — no further
// mapping needed). ok is false for any value that doesn't parse to exactly
// one of the two recognized decisions.
func parseSlackActionValue(value string) (approvalID, decision string, ok bool) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	switch parts[1] {
	case "allow", "deny":
		return parts[0], parts[1], true
	default:
		return "", "", false
	}
}
