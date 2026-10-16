package services

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/internal/hookipc"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

type hookPolicyStub struct {
	result       classifier.ClassificationResult
	buildContext classifier.ClassificationContext
	calls        int
	lastContext  classifier.ClassificationContext
}

func (s *hookPolicyStub) BuildContext(string) classifier.ClassificationContext {
	return s.buildContext
}

func (s *hookPolicyStub) Classify(_ classifier.PermissionRequestPayload, ctx classifier.ClassificationContext) classifier.ClassificationResult {
	s.calls++
	s.lastContext = ctx
	return s.result
}

func TestHookClassifier_Classify_should_ReturnEquivalentPreToolUseOutput_When_PolicyAllows(t *testing.T) {
	policy := &hookPolicyStub{result: classifier.ClassificationResult{
		Decision: classifier.AutoAllow,
		RuleName: "Read-only Git",
		Reason:   "safe command",
	}}
	service := NewHookClassifier(policy, nil)
	request := hookRequest("Bash")
	request.Context.Env = map[string]string{"RUSTC": "rustc"}

	reply, err := service.Classify(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Output == nil {
		t.Fatal("allow classification returned defer")
	}
	output := reply.Output.HookSpecificOutput
	if output.PermissionDecision != "allow" || output.PermissionDecisionReason != "Read-only Git: safe command" {
		t.Fatalf("output = %+v", output)
	}
	if policy.lastContext.Env["RUSTC"] != "rustc" {
		t.Fatalf("invoking environment was not threaded: %+v", policy.lastContext.Env)
	}
}

func TestHookClassifier_Classify_should_ReturnDeferWithoutPolicySideEffects_When_AskUserQuestion(t *testing.T) {
	policy := &hookPolicyStub{result: classifier.ClassificationResult{Decision: classifier.AutoAllow}}
	service := NewHookClassifier(policy, nil)

	reply, err := service.Classify(context.Background(), hookRequest("AskUserQuestion"))
	if err != nil {
		t.Fatal(err)
	}
	if reply.Output != nil || reply.Source != hookipc.DecisionSourceDefer {
		t.Fatalf("reply = %+v, want defer", reply)
	}
	if policy.calls != 0 {
		t.Fatalf("policy calls = %d, want 0", policy.calls)
	}
}

func TestHookClassifier_Classify_should_AppendAlternative_When_PolicyDenies(t *testing.T) {
	policy := &hookPolicyStub{result: classifier.ClassificationResult{
		Decision:    classifier.AutoDeny,
		Reason:      "unsafe command.",
		Alternative: "Use a scoped path.",
	}}
	service := NewHookClassifier(policy, nil)

	reply, err := service.Classify(context.Background(), hookRequest("Bash"))
	if err != nil {
		t.Fatal(err)
	}
	if got := reply.Output.HookSpecificOutput.PermissionDecisionReason; got != "unsafe command. Use a scoped path." {
		t.Fatalf("reason = %q", got)
	}
}

func hookRequest(toolName string) hookipc.ClassificationEnvelope {
	return hookipc.ClassificationEnvelope{
		ProtocolVersion:     hookipc.CurrentProtocolVersion,
		InstanceFingerprint: hookipc.Fingerprint("test"),
		RequestID:           "request-1",
		Payload: classifier.PermissionRequestPayload{
			SessionID:     "session-1",
			HookEventName: "PreToolUse",
			ToolName:      toolName,
			Cwd:           "/repo",
			ToolInput:     map[string]interface{}{"command": "git status"},
		},
	}
}
