package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// slackWebhookURLPrefix is the required prefix for a valid Slack Incoming
// Webhook URL. Both the client-side (SlackNotificationSettings.tsx) and this
// server-side backstop validate against the same literal shape.
const slackWebhookURLPrefix = "https://hooks.slack.com/services/"

// slackTestMessageText is the canned message body sent by TestSlackWebhook.
const slackTestMessageText = "stapler-squad test message — if you can see this, your webhook is configured correctly."

// SlackConfigService handles the GetSlackConfig/UpdateSlackConfig/
// TestSlackWebhook RPCs. Delegated to from SessionService exactly like
// DefaultsService/CallbackConfigService — a config-backed handler with no
// second implementation, so a concrete type per
// the `interface-pollution-checklist` skill.
type SlackConfigService struct {
	slackNotifier *SlackNotifier
}

// NewSlackConfigService creates a SlackConfigService backed by the given
// SlackNotifier (used for GetDeliveryStatus and, for TestSlackWebhook, the
// shared postToSlack helper — both types live in package services).
func NewSlackConfigService(n *SlackNotifier) *SlackConfigService {
	return &SlackConfigService{slackNotifier: n}
}

// GetSlackConfig returns the current Slack notification configuration.
// webhook_configured/signing_secret_configured reflect ciphertext (or env
// override) *presence*, not decrypt success — see plan.md's Task 1.4.2a
// decrypt-health note for why that's an accepted Phase 1 gap.
// +api: slack-config:get
func (s *SlackConfigService) GetSlackConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSlackConfigRequest],
) (*connect.Response[sessionv1.GetSlackConfigResponse], error) {
	cfg := config.LoadConfig()
	return connect.NewResponse(&sessionv1.GetSlackConfigResponse{
		Config: s.slackConfigToProto(cfg),
	}), nil
}

// UpdateSlackConfig updates the Slack notification configuration. See
// UpdateSlackConfigRequest's doc comment for the empty-string-vs-clear-bool
// precedence semantics applied to both secret fields.
// +api: slack-config:update
func (s *SlackConfigService) UpdateSlackConfig(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateSlackConfigRequest],
) (*connect.Response[sessionv1.UpdateSlackConfigResponse], error) {
	cfg := config.LoadConfig()

	switch {
	case req.Msg.ClearWebhookUrl:
		cfg.Slack.WebhookURLEncrypted = ""
	case req.Msg.WebhookUrl != "":
		if !strings.HasPrefix(req.Msg.WebhookUrl, slackWebhookURLPrefix) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("webhook_url must start with %q", slackWebhookURLPrefix))
		}
		key, err := cfg.GetOrCreateEncryptionKey()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get encryption key: %w", err))
		}
		encrypted, err := session.EncryptToken(key, req.Msg.WebhookUrl)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to encrypt webhook url: %w", err))
		}
		cfg.Slack.WebhookURLEncrypted = encrypted
	default:
		// Left blank, not cleared: leave the stored ciphertext untouched.
	}

	switch {
	case req.Msg.ClearSigningSecret:
		cfg.Slack.SigningSecretEncrypted = ""
	case req.Msg.SigningSecret != "":
		key, err := cfg.GetOrCreateEncryptionKey()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get encryption key: %w", err))
		}
		encrypted, err := session.EncryptToken(key, req.Msg.SigningSecret)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to encrypt signing secret: %w", err))
		}
		cfg.Slack.SigningSecretEncrypted = encrypted
	default:
		// Left blank, not cleared: leave the stored ciphertext untouched.
	}

	cfg.Slack.NotifyOnQueueItem = req.Msg.NotifyOnQueueItem
	cfg.Slack.QueueDepthThreshold = int(req.Msg.QueueDepthThreshold)
	cfg.Slack.ApprovalEnabled = req.Msg.ApprovalEnabled
	cfg.Slack.DashboardBaseURL = req.Msg.DashboardBaseUrl

	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	return connect.NewResponse(&sessionv1.UpdateSlackConfigResponse{
		Config: s.slackConfigToProto(cfg),
	}), nil
}

// TestSlackWebhook sends a canned test message synchronously (not via
// dispatchAsync — the whole point is the caller waits for a real result) and
// reports the outcome. If req.Msg.WebhookUrl is non-empty, it's used directly
// (already plaintext from the settings form, tested before any save); a
// blank webhook_url falls back to the currently-saved config. Testing an
// in-form URL never persists it.
// +api: slack-config:test-webhook
func (s *SlackConfigService) TestSlackWebhook(
	ctx context.Context,
	req *connect.Request[sessionv1.TestSlackWebhookRequest],
) (*connect.Response[sessionv1.TestSlackWebhookResponse], error) {
	webhookURL := req.Msg.WebhookUrl
	if webhookURL != "" {
		if !strings.HasPrefix(webhookURL, slackWebhookURLPrefix) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("webhook_url must start with %q", slackWebhookURLPrefix))
		}
	} else {
		cfg := config.LoadConfig()
		resolved, err := resolveSlackWebhookURL(cfg)
		if err != nil {
			return connect.NewResponse(&sessionv1.TestSlackWebhookResponse{
				Success: false,
				Error:   err.Error(),
			}), nil
		}
		if resolved == "" {
			return connect.NewResponse(&sessionv1.TestSlackWebhookResponse{
				Success: false,
				Error:   "no webhook configured",
			}), nil
		}
		webhookURL = resolved
	}

	payload := slackWebhookPayload{Text: slackTestMessageText}
	if err := s.slackNotifier.postToSlack(ctx, webhookURL, payload); err != nil {
		return connect.NewResponse(&sessionv1.TestSlackWebhookResponse{
			Success: false,
			Error:   err.Error(),
		}), nil
	}

	return connect.NewResponse(&sessionv1.TestSlackWebhookResponse{Success: true}), nil
}

// slackConfigToProto converts the config-layer SlackConfig plus the
// notifier's live delivery status into the masked (booleans-only-for-secrets)
// proto representation shared by GetSlackConfig and UpdateSlackConfig.
func (s *SlackConfigService) slackConfigToProto(cfg *config.Config) *sessionv1.SlackConfigProto {
	proto := &sessionv1.SlackConfigProto{
		WebhookConfigured:       cfg.Slack.WebhookURLEncrypted != "" || cfg.SlackWebhookURLOverride() != "",
		SigningSecretConfigured: cfg.Slack.SigningSecretEncrypted != "" || cfg.SlackSigningSecretOverride() != "",
		NotifyOnQueueItem:       cfg.Slack.NotifyOnQueueItem,
		QueueDepthThreshold:     int32(cfg.Slack.QueueDepthThreshold),
		ApprovalEnabled:         cfg.Slack.ApprovalEnabled,
		DashboardBaseUrl:        cfg.Slack.DashboardBaseURL,
	}

	if s.slackNotifier != nil {
		attempted, success, errMsg, at := s.slackNotifier.GetDeliveryStatus()
		status := &sessionv1.SlackDeliveryStatus{
			Attempted: attempted,
			Success:   success,
			Error:     errMsg,
		}
		if !at.IsZero() {
			status.AttemptedAt = timestamppb.New(at)
		}
		proto.LastDelivery = status
	}

	return proto
}
