package jules

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestParseJulesSourceName_should_ReturnJulesSourceName_When_PrefixValid(t *testing.T) {
	got, err := ParseJulesSourceName("sources/github-tstapler-stapler-squad")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != JulesSourceName("sources/github-tstapler-stapler-squad") {
		t.Fatalf("got %q", got)
	}
}

func TestParseJulesSourceName_should_RejectMissingPrefix_When_SourcesPrefixAbsent(t *testing.T) {
	got, err := ParseJulesSourceName("github-tstapler-stapler-squad")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != "" {
		t.Fatalf("expected zero value, got %q", got)
	}
	if !strings.Contains(err.Error(), "sources/") {
		t.Fatalf("expected error to mention sources/, got %q", err.Error())
	}
}

func TestParseJulesSessionName_should_ReturnJulesSessionName_When_PrefixValid(t *testing.T) {
	got, err := ParseJulesSessionName("sessions/abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != JulesSessionName("sessions/abc") {
		t.Fatalf("got %q", got)
	}
}

func TestParseJulesSessionName_should_RejectMissingPrefix_When_SessionsPrefixAbsent(t *testing.T) {
	got, err := ParseJulesSessionName("abc")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != "" {
		t.Fatalf("expected zero value, got %q", got)
	}
	if !strings.Contains(err.Error(), "sessions/") {
		t.Fatalf("expected error to mention sessions/, got %q", err.Error())
	}
}

func TestParseGitHubBranchRef_should_RejectEmpty_When_StringEmpty(t *testing.T) {
	got, err := ParseGitHubBranchRef("")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != "" {
		t.Fatalf("expected zero value, got %q", got)
	}
}

func TestParseGitHubBranchRef_should_ReturnBranchRef_When_StringNonEmpty(t *testing.T) {
	got, err := ParseGitHubBranchRef("backlog/fix-flaky-poller")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != GitHubBranchRef("backlog/fix-flaky-poller") {
		t.Fatalf("got %q", got)
	}
}

func TestParseJulesAPIKey_should_RejectEmpty_When_StringEmpty(t *testing.T) {
	got, err := ParseJulesAPIKey("")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != "" {
		t.Fatalf("expected zero value, got %q", got)
	}
}

func TestJulesAPIKey_String_should_RedactValue_When_Formatted(t *testing.T) {
	const rawKey = "AIzaSyD-EXAMPLE-KEY-VALUE"
	k, err := ParseJulesAPIKey(rawKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := fmt.Sprintf("%v %s", k, k)
	want := "jules-api-key(redacted) jules-api-key(redacted)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if strings.Contains(got, rawKey) {
		t.Fatalf("formatted output leaked the raw key: %q", got)
	}
}

func TestJulesSessionState_UnmarshalJSON_should_ParseAsUnknown_When_WireValueUnrecognized(t *testing.T) {
	var session JulesSession
	err := json.Unmarshal([]byte(`{"state":"AWAITING_HUMAN_TEA_BREAK"}`), &session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.State.IsKnown() {
		t.Fatal("expected IsKnown() == false for an unrecognized wire value")
	}
	if session.State.Raw() != "AWAITING_HUMAN_TEA_BREAK" {
		t.Fatalf("Raw() = %q, want the original wire value preserved", session.State.Raw())
	}
}

func TestJulesSessionState_UnmarshalJSON_should_ParseKnownState_When_WireValueRecognized(t *testing.T) {
	var session JulesSession
	err := json.Unmarshal([]byte(`{"state":"COMPLETED"}`), &session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !session.State.IsKnown() {
		t.Fatal("expected IsKnown() == true for a recognized wire value")
	}
	if session.State != JulesStateCompleted {
		t.Fatalf("got %v, want JulesStateCompleted", session.State)
	}
}

func TestJulesSessionState_IsTerminal_should_ReturnTrueOnlyForCompletedAndFailed_When_AllStatesChecked(t *testing.T) {
	tests := []struct {
		name  string
		state JulesSessionState
		want  bool
	}{
		{"Queued", JulesStateQueued, false},
		{"Planning", JulesStatePlanning, false},
		{"AwaitingPlanApproval", JulesStateAwaitingPlanApproval, false},
		{"InProgress", JulesStateInProgress, false},
		{"Completed", JulesStateCompleted, true},
		{"Failed", JulesStateFailed, true},
		{"Unknown", JulesStateUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsTerminal(); got != tt.want {
				t.Fatalf("IsTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseJulesSessionState_should_NeverError_When_CalledWithArbitraryStrings(t *testing.T) {
	for _, raw := range []string{"", "QUEUED", "FAILED", "AWAITING_HUMAN_TEA_BREAK"} {
		got := ParseJulesSessionState(raw)
		if got.Raw() != raw {
			t.Fatalf("ParseJulesSessionState(%q).Raw() = %q", raw, got.Raw())
		}
	}
}
