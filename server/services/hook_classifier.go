package services

import (
	"context"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// HookClassifier is the classify-only service used by PreToolUse IPC. It does
// not perform PermissionRequest-only secret/domain/live-session enrichment and
// never creates a pending approval.
type HookClassifier struct {
	policy       classifier.Classifier
	analytics    *AnalyticsStore
	contextCache *HookContextCache
	deduper      *HookDeduper
	replier      HookReplier
}

// SetHookReplier lets the classifier answer AskUserQuestion hooks from stored
// answers (ADR-005 rung 1). Call it before serving.
func (h *HookClassifier) SetHookReplier(r HookReplier) { h.replier = r }

// SessionUUIDEnv is the variable a hook sends to name the Stapler Squad session
// it runs in; it is the only context an AskUserQuestion hook needs.
const SessionUUIDEnv = "STAPLER_SESSION_UUID"

func NewHookClassifier(policy classifier.Classifier, analytics *AnalyticsStore) *HookClassifier {
	service := &HookClassifier{
		policy:    policy,
		analytics: analytics,
		deduper:   NewHookDeduper(),
	}
	if policy != nil {
		service.contextCache = NewHookContextCache(policy.BuildContext)
	}
	return service
}

func (h *HookClassifier) Classify(ctx context.Context, request hookipc.ClassificationEnvelope) (hookipc.ClassificationReply, error) {
	if h == nil {
		return (&HookClassifier{}).classify(request)
	}
	stable := request.Payload.ToolUseID != ""
	return h.deduper.Do(ctx, request.RequestID, stable, func() (hookipc.ClassificationReply, error) {
		return h.classify(request)
	})
}

func (h *HookClassifier) classify(request hookipc.ClassificationEnvelope) (hookipc.ClassificationReply, error) {
	reply := hookipc.ClassificationReply{
		ProtocolVersion:     request.ProtocolVersion,
		InstanceFingerprint: request.InstanceFingerprint,
		RequestID:           request.RequestID,
		Source:              hookipc.DecisionSourcePrimary,
	}
	if h != nil && strings.EqualFold(request.Payload.ToolName, "AskUserQuestion") && h.replier != nil {
		if out := h.replier.Reply(context.Background(), request.Context.Env[SessionUUIDEnv], request.Payload.ToolInput); out != nil {
			reply.Output, reply.Source = out, hookipc.DecisionSourcePrimary
			return reply, nil
		}
	}
	if h == nil || h.policy == nil || strings.EqualFold(request.Payload.ToolName, "AskUserQuestion") {
		reply.Source = hookipc.DecisionSourceDefer
		return reply, nil
	}

	started := time.Now()
	classificationContext := h.contextCache.Get(request.Payload.Cwd)
	classificationContext.Cwd = request.Payload.Cwd
	classificationContext.Env = cloneEnvironment(request.Context.Env)
	result := h.policy.Classify(request.Payload, classificationContext)
	reply.Output = preToolUseOutput(result)
	if reply.Output == nil {
		reply.Source = hookipc.DecisionSourceDefer
	}
	if h.analytics != nil {
		source := request.Payload.Source
		if source == "" {
			source = "claude"
		}
		h.analytics.RecordFromResult(request.Payload, result, request.Payload.SessionID, "", time.Since(started).Milliseconds(), source)
	}
	return reply, nil
}

func cloneEnvironment(environment map[string]string) map[string]string {
	if len(environment) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(environment))
	for name, value := range environment {
		cloned[name] = value
	}
	return cloned
}

func preToolUseOutput(result classifier.ClassificationResult) *hookipc.HookOutput {
	switch result.Decision {
	case classifier.AutoAllow:
		reason := result.Reason
		if result.RuleName != "" {
			reason = result.RuleName + ": " + reason
		}
		return &hookipc.HookOutput{HookSpecificOutput: hookipc.HookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "allow",
			PermissionDecisionReason: reason,
		}}
	case classifier.AutoDeny:
		reason := result.Reason
		if result.Alternative != "" {
			reason += " " + result.Alternative
		}
		return &hookipc.HookOutput{HookSpecificOutput: hookipc.HookSpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "deny",
			PermissionDecisionReason: reason,
		}}
	default:
		return nil
	}
}
