package services

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

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

// SlackInteractivePayload is the minimal subset of Slack's interactive-
// component callback payload this handler needs: which button was clicked.
type SlackInteractivePayload struct {
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
}

// approvalResolver is the narrow interface SlackInteractiveHandler needs —
// defined here, at the consumer, per
// .claude/rules/interface-pollution-checklist.md. Satisfied by
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
}

// NewSlackInteractiveHandler constructs a SlackInteractiveHandler. The
// signing secret is resolved live from config.LoadConfig() on every request
// (via resolveSlackSigningSecret) rather than captured once at construction
// time, matching this package's established "read live config, don't
// snapshot it" convention (see slack_notifier.go's resolveSlackWebhookURL
// call sites and hookBaseURLFn's doc comment in server/server.go).
func NewSlackInteractiveHandler(resolver approvalResolver) *SlackInteractiveHandler {
	return &SlackInteractiveHandler{resolver: resolver}
}

// Handle reads the raw request body via io.ReadAll exactly once — before any
// form/JSON parsing touches it — and reuses that same buffer for both
// signature verification and payload parsing. It never calls r.ParseForm()
// first: that is the named "read body before ParseForm" bug class from
// research/pitfalls.md §5, which would verify against an already-consumed
// (or differently-encoded) body instead of the exact bytes Slack signed.
//
// On any verification failure it responds 401 with a generic body — no
// internals leaked to this now-internet-facing surface (§5 item 7). On
// success it parses Slack's form-encoded "payload" field, extracts the
// clicked button's value ("<approvalID>:allow" or "<approvalID>:deny"), and
// resolves the approval in-process via approvalResolver.
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

	// Only now, after verification against the exact raw bytes above, restore
	// the body from that same already-verified buffer so ParseForm reads the
	// signed bytes rather than re-reading (an already-drained) r.Body.
	r.Body = io.NopCloser(bytes.NewReader(rawBody))
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var payload SlackInteractivePayload
	if err := json.Unmarshal([]byte(r.FormValue("payload")), &payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	for _, action := range payload.Actions {
		approvalID, decision, ok := parseSlackActionValue(action.Value)
		if !ok {
			continue
		}
		req := connect.NewRequest(&sessionv1.ResolveApprovalRequest{
			ApprovalId: approvalID,
			Decision:   decision,
		})
		if _, err := h.resolver.ResolveApproval(r.Context(), req); err != nil {
			log.Warn("SlackInteractiveHandler: ResolveApproval failed", "approval_id", approvalID, "error", err)
		}
	}

	w.WriteHeader(http.StatusOK)
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
