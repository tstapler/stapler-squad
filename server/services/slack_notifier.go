package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// maxSlackBlockTextLen caps a single Slack Block Kit text block at 2900 runes,
// leaving headroom under Slack's hard 3000-character-per-block limit (mirrors
// approval_handler.go's maxNotificationMessageLen/maxEscalationReasonLen pattern).
const maxSlackBlockTextLen = 2900

// slackDispatchTimeout bounds each asynchronously-dispatched Slack send. It is
// intentionally short — Slack notifications are best-effort and must never
// become a source of goroutine buildup during an extended outage.
const slackDispatchTimeout = 5 * time.Second

// SlackNotifier formats Block Kit messages and delivers them to a configured
// Slack Incoming Webhook. It is the single implementation of this concern in
// the codebase (no interface — see .claude/rules/interface-pollution-checklist.md).
// All Notify*/MaybeNotify* methods are non-blocking: they dispatch the actual
// HTTP POST on an internal goroutine (dispatchAsync) and return immediately.
type SlackNotifier struct {
	httpClient *http.Client

	mu                  sync.Mutex
	thresholdCrossed    bool
	lastDeliveryAt      time.Time
	lastDeliverySuccess bool
	lastDeliveryError   string
	lastDeliveryAttempt bool
}

// NewSlackNotifier constructs a SlackNotifier with a 5-second-timeout HTTP
// client, mirroring domain_checker.go's http.Client{Timeout: 3 * time.Second}
// shape.
func NewSlackNotifier() *SlackNotifier {
	return &SlackNotifier{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// resolveSlackWebhookURL returns the effective Slack webhook URL: the
// SLACK_WEBHOOK_URL env override if set, else the decrypted stored
// ciphertext, else "" with no error (the "not configured" clean no-op case).
// Mirrors backlog_service_lifecycle.go:67-71's exact
// cfg.GetOrCreateEncryptionKey() -> session.DecryptToken(...) call shape.
func resolveSlackWebhookURL(cfg *config.Config) (string, error) {
	if v := cfg.SlackWebhookURLOverride(); v != "" {
		return v, nil
	}
	if cfg.Slack.WebhookURLEncrypted == "" {
		return "", nil
	}
	key, err := cfg.GetOrCreateEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("resolve slack webhook url: %w", err)
	}
	return session.DecryptToken(key, cfg.Slack.WebhookURLEncrypted)
}

// resolveSlackSigningSecret returns the effective Slack app signing secret,
// same precedence/shape as resolveSlackWebhookURL.
func resolveSlackSigningSecret(cfg *config.Config) (string, error) {
	if v := cfg.SlackSigningSecretOverride(); v != "" {
		return v, nil
	}
	if cfg.Slack.SigningSecretEncrypted == "" {
		return "", nil
	}
	key, err := cfg.GetOrCreateEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("resolve slack signing secret: %w", err)
	}
	return session.DecryptToken(key, cfg.Slack.SigningSecretEncrypted)
}

// slackWebhookPayload is a Slack Incoming Webhook Block Kit message body.
type slackWebhookPayload struct {
	Text   string       `json:"text"`
	Blocks []slackBlock `json:"blocks,omitempty"`
}

// slackBlock is a single Block Kit block. Two shapes are used by this
// notifier: "section" (Text set, Elements nil) for every message body block,
// and "actions" (Elements set, Text nil — Phase 2, Story 2.1.4) for the
// Approve/Deny button row NotifyApprovalPending appends when
// cfg.Slack.ApprovalEnabled. One struct rather than a second type so
// slackWebhookPayload.Blocks can stay a single, homogeneously-typed slice.
type slackBlock struct {
	Type     string               `json:"type"`
	Text     *slackBlockText      `json:"text,omitempty"`
	Elements []slackButtonElement `json:"elements,omitempty"`
}

// slackBlockText is a Block Kit text object.
type slackBlockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// slackButtonElement is a Block Kit button element. Value carries
// "<approvalID>:allow" or "<approvalID>:deny", which
// SlackInteractiveHandler.Handle (slack_interactive_handler.go) splits back
// apart to resolve the approval.
type slackButtonElement struct {
	Type     string          `json:"type"`
	Text     *slackBlockText `json:"text"`
	Value    string          `json:"value"`
	ActionID string          `json:"action_id"`
	Style    string          `json:"style,omitempty"`
}

// escapeSlackMrkdwn escapes Slack's three mrkdwn special characters so
// dynamic, potentially agent/user-influenced text (session names, review
// context, diff content, approval detail) can never be interpreted as
// mrkdwn/entity syntax when placed into a Block Kit text object. Per Slack's
// formatting reference, "&" MUST be escaped first — escaping "<"/">" before
// "&" would double-escape the "&" just introduced by their replacements
// (e.g. "<" -> "&lt;" -> "&amp;lt;" if done in the wrong order). Escaping
// stops literal injected sequences like "<!channel>" or "<@U12345>" from
// rendering as real Slack mention/broadcast directives, and keeps a
// dynamic label from corrupting the surrounding "<url|label>" link syntax.
func escapeSlackMrkdwn(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// truncateForSlackBlock caps s at maxRunes runes (rune-safe), appending
// "... truncated, see dashboard" when truncation occurs. Same shape as
// truncateString (approval_handler.go:588) but with a dashboard-pointing
// suffix instead of a bare "...".
func truncateForSlackBlock(s string, maxRunes int) string {
	const suffix = "... truncated, see dashboard"
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	// Leave room for the suffix itself so the total stays within maxRunes.
	suffixLen := len([]rune(suffix))
	cut := maxRunes - suffixLen
	if cut < 0 {
		cut = 0
	}
	return string(r[:cut]) + suffix
}

// postToSlack marshals payload and POSTs it to webhookURL. It never lets the
// raw error from httpClient.Do (which Go wraps in *url.Error embedding the
// full request URL) reach a log call or returned error — on any transport
// failure it returns a fresh, unwrapped sanitized error. On a non-2xx
// response it returns an error carrying only the status code, never the URL.
// Updates lastDeliveryAt/lastDeliverySuccess/lastDeliveryError under n.mu on
// every call (success or failure).
func (n *SlackNotifier) postToSlack(ctx context.Context, webhookURL string, payload slackWebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		n.recordDelivery(false, "failed to marshal slack payload")
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		n.recordDelivery(false, "failed to build slack request")
		return errors.New("slack webhook request failed: invalid request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		// Deliberately do not wrap err: http.Client.Do failures are *url.Error,
		// whose Error() string embeds the full request URL (the webhook
		// credential). Construct a fresh, unwrapped error instead.
		sanitized := errors.New("slack webhook request failed: network error")
		n.recordDelivery(false, sanitized.Error())
		log.Warn("SlackNotifier: send failed", "error", sanitized)
		return sanitized
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sanitized := fmt.Errorf("slack webhook request failed: status %d", resp.StatusCode)
		n.recordDelivery(false, sanitized.Error())
		log.Warn("SlackNotifier: send failed", "error", sanitized)
		return sanitized
	}

	n.recordDelivery(true, "")
	return nil
}

// recordDelivery updates the delivery-status snapshot under n.mu. Logs at
// Info only on a failure->success state transition, avoiding log spam on a
// working integration (per the plan's Observability Plan).
func (n *SlackNotifier) recordDelivery(success bool, errMsg string) {
	n.mu.Lock()
	wasFailing := n.lastDeliveryAttempt && !n.lastDeliverySuccess
	n.lastDeliveryAt = time.Now()
	n.lastDeliverySuccess = success
	n.lastDeliveryError = errMsg
	n.lastDeliveryAttempt = true
	n.mu.Unlock()

	if success && wasFailing {
		log.Info("SlackNotifier: send succeeded after prior failure")
	}
}

// dispatchAsync runs fn on a new goroutine with panic recovery (logged via
// log.Error) and a slackDispatchTimeout-bounded context derived from ctx.
// This is a private implementation detail of SlackNotifier: NotifyReviewQueueItem,
// NotifyApprovalPending, and MaybeNotifyQueueDepthThreshold each call this as
// the first line of their own body and return immediately — the non-blocking
// guarantee lives entirely inside SlackNotifier, never a caller's responsibility.
func (n *SlackNotifier) dispatchAsync(ctx context.Context, fn func(ctx context.Context)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("SlackNotifier: recovered from panic", "panic", r)
			}
		}()
		sendCtx, cancel := context.WithTimeout(ctx, slackDispatchTimeout)
		defer cancel()
		fn(sendCtx)
	}()
}

// NotifyReviewQueueItem sends a Slack notification for a new review-queue
// item. No-ops cleanly (no HTTP call) when no webhook is configured. The
// call itself is non-blocking; any send failure is logged and swallowed —
// this method never returns an error to the caller.
func (n *SlackNotifier) NotifyReviewQueueItem(ctx context.Context, cfg *config.Config, item *session.ReviewItem, dashboardURL string) {
	n.dispatchAsync(ctx, func(ctx context.Context) {
		webhookURL, err := resolveSlackWebhookURL(cfg)
		if err != nil {
			log.Warn("SlackNotifier: failed to resolve webhook url", "error", err)
			return
		}
		if webhookURL == "" {
			return
		}

		primary := fmt.Sprintf("*%s* needs attention: %s", escapeSlackMrkdwn(sanitizeNotificationText(item.SessionName)), item.Reason.String())
		blocks := []slackBlock{
			{Type: "section", Text: &slackBlockText{Type: "mrkdwn", Text: primary}},
		}
		if item.Context != "" {
			blocks = append(blocks, slackBlock{
				Type: "section",
				Text: &slackBlockText{Type: "mrkdwn", Text: truncateForSlackBlock(escapeSlackMrkdwn(sanitizeNotificationText(item.Context)), maxSlackBlockTextLen)},
			})
		}
		if item.DiffStats != nil && item.DiffStats.Content != "" {
			blocks = append(blocks, slackBlock{
				Type: "section",
				Text: &slackBlockText{Type: "mrkdwn", Text: truncateForSlackBlock(escapeSlackMrkdwn(item.DiffStats.Content), maxSlackBlockTextLen)},
			})
		}
		if dashboardURL != "" {
			link := dashboardURL + "/?session=" + item.SessionID
			linkText := fmt.Sprintf("<%s|View %s>", link, escapeSlackMrkdwn(sanitizeNotificationText(item.SessionName)))
			blocks = append(blocks, slackBlock{
				Type: "section",
				Text: &slackBlockText{Type: "mrkdwn", Text: linkText},
			})
		}

		payload := slackWebhookPayload{Text: primary, Blocks: blocks}
		if err := n.postToSlack(ctx, webhookURL, payload); err != nil {
			log.Warn("SlackNotifier: NotifyReviewQueueItem send failed", "error", err)
		}
	})
}

// NotifyApprovalPending sends a Slack notification for a new pending
// approval. No-ops cleanly when no webhook is configured. Non-blocking; any
// send failure is logged and swallowed.
func (n *SlackNotifier) NotifyApprovalPending(ctx context.Context, cfg *config.Config, approval *PendingApproval, sessionName, dashboardURL string) {
	n.dispatchAsync(ctx, func(ctx context.Context) {
		webhookURL, err := resolveSlackWebhookURL(cfg)
		if err != nil {
			log.Warn("SlackNotifier: failed to resolve webhook url", "error", err)
			return
		}
		if webhookURL == "" {
			return
		}

		displayName := sessionName
		if displayName == "" {
			displayName = approval.SessionID
		}
		primary := fmt.Sprintf("*%s* is waiting for approval to use *%s*", escapeSlackMrkdwn(sanitizeNotificationText(displayName)), escapeSlackMrkdwn(sanitizeNotificationText(approval.ToolName)))
		blocks := []slackBlock{
			{Type: "section", Text: &slackBlockText{Type: "mrkdwn", Text: primary}},
		}

		detail := buildApprovalMessage(approval)
		if detail != "" {
			blocks = append(blocks, slackBlock{
				Type: "section",
				Text: &slackBlockText{Type: "mrkdwn", Text: truncateForSlackBlock(escapeSlackMrkdwn(sanitizeNotificationText(detail)), maxSlackBlockTextLen)},
			})
		}

		if dashboardURL != "" {
			link := dashboardURL + "/?session=" + approval.SessionID
			linkText := fmt.Sprintf("<%s|View %s>", link, escapeSlackMrkdwn(sanitizeNotificationText(displayName)))
			blocks = append(blocks, slackBlock{
				Type: "section",
				Text: &slackBlockText{Type: "mrkdwn", Text: linkText},
			})
		}

		// Phase 2, Story 2.1.4: when interactive approvals are enabled, add an
		// actions block with Approve/Deny buttons whose values encode
		// "<approvalID>:allow"/"<approvalID>:deny" -- SlackInteractiveHandler
		// (slack_interactive_handler.go) splits them back apart on a verified
		// click. Omitted entirely when !ApprovalEnabled so Phase 1's payload
		// shape is unchanged.
		if cfg.Slack.ApprovalEnabled {
			blocks = append(blocks, slackBlock{
				Type: "actions",
				Elements: []slackButtonElement{
					{
						Type:     "button",
						Text:     &slackBlockText{Type: "plain_text", Text: "Approve"},
						Value:    approval.ID + ":allow",
						ActionID: "approve",
						Style:    "primary",
					},
					{
						Type:     "button",
						Text:     &slackBlockText{Type: "plain_text", Text: "Deny"},
						Value:    approval.ID + ":deny",
						ActionID: "deny",
						Style:    "danger",
					},
				},
			})
		}

		payload := slackWebhookPayload{Text: primary, Blocks: blocks}
		if err := n.postToSlack(ctx, webhookURL, payload); err != nil {
			log.Warn("SlackNotifier: NotifyApprovalPending send failed", "error", err)
		}
	})
}

// MaybeNotifyQueueDepthThreshold implements the edge-triggered digest latch:
// fires exactly one Slack digest message per crossing above threshold,
// resetting when depth drops back below it. threshold <= 0 always returns
// false (feature-flag-off state) with no dispatch. Returns fired=true iff a
// digest was dispatched by this call.
//
// Argument order (both plain ints — do not swap):
//   - depth is the CURRENT review-queue size (rqm.queue.GetStatistics().TotalItems).
//   - threshold is the CONFIGURED crossing point (cfg.Slack.QueueDepthThreshold).
//
// Swapping them silently inverts the crossing logic (e.g. a threshold of 0
// read as the depth would permanently disable notifications, since
// threshold <= 0 short-circuits above) with no compiler error — see
// .claude/rules/primitive-obsession-checklist.md.
func (n *SlackNotifier) MaybeNotifyQueueDepthThreshold(ctx context.Context, cfg *config.Config, depth, threshold int, dashboardURL string) (fired bool) {
	if threshold <= 0 {
		return false
	}

	n.mu.Lock()
	switch {
	case depth >= threshold && !n.thresholdCrossed:
		n.thresholdCrossed = true
		fired = true
	case depth < threshold:
		n.thresholdCrossed = false
	}
	n.mu.Unlock()

	if !fired {
		return false
	}

	n.dispatchAsync(ctx, func(ctx context.Context) {
		webhookURL, err := resolveSlackWebhookURL(cfg)
		if err != nil {
			log.Warn("SlackNotifier: failed to resolve webhook url", "error", err)
			return
		}
		if webhookURL == "" {
			return
		}

		primary := fmt.Sprintf("%d items pending in the review queue", depth)
		blocks := []slackBlock{
			{Type: "section", Text: &slackBlockText{Type: "mrkdwn", Text: primary}},
		}
		if dashboardURL != "" {
			link := dashboardURL + "/review-queue"
			linkText := fmt.Sprintf("<%s|View review queue>", link)
			blocks = append(blocks, slackBlock{
				Type: "section",
				Text: &slackBlockText{Type: "mrkdwn", Text: linkText},
			})
		}

		payload := slackWebhookPayload{Text: primary, Blocks: blocks}
		if err := n.postToSlack(ctx, webhookURL, payload); err != nil {
			log.Warn("SlackNotifier: queue-depth digest send failed", "error", err)
		}
	})

	return true
}

// GetDeliveryStatus returns a thread-safe snapshot of the notifier's most
// recent send outcome: attempted is false until the first postToSlack call
// ever completes.
func (n *SlackNotifier) GetDeliveryStatus() (attempted, success bool, errMsg string, at time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lastDeliveryAttempt, n.lastDeliverySuccess, n.lastDeliveryError, n.lastDeliveryAt
}
