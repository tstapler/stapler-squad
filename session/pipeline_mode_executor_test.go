package session

import "testing"

func TestSerializeStageExecutors_should_RoundTripThroughJSON_When_MapHasAllThreeRoles(t *testing.T) {
	m := map[StageRole]PipelineStageExecutor{
		StageRoleTriage: {Model: "claude-haiku-4-5"},
		StageRoleReview: {Program: "gemini", Model: "gemini-2.5-pro"},
		StageRoleWork:   {Program: "aider"},
	}

	raw, err := SerializeStageExecutors(m)
	if err != nil {
		t.Fatalf("SerializeStageExecutors: %v", err)
	}

	got, err := ParseStageExecutors(raw)
	if err != nil {
		t.Fatalf("ParseStageExecutors: %v", err)
	}
	if len(got) != len(m) {
		t.Fatalf("round trip changed map size: got %d entries, want %d", len(got), len(m))
	}
	for role, want := range m {
		if got[role] != want {
			t.Errorf("role %q: got %+v, want %+v", role, got[role], want)
		}
	}
}

func TestSerializeStageExecutors_should_ProduceExpectedJSONShape_When_MapHasOneRole(t *testing.T) {
	m := map[StageRole]PipelineStageExecutor{
		StageRoleTriage: {Model: "claude-haiku-4-5"},
	}

	raw, err := SerializeStageExecutors(m)
	if err != nil {
		t.Fatalf("SerializeStageExecutors: %v", err)
	}

	const want = `{"triage":{"program":"","model":"claude-haiku-4-5"}}`
	if raw != want {
		t.Errorf("got %s, want %s", raw, want)
	}
}

func TestParseStageExecutors_should_ReturnEmptyNonNilMap_When_InputIsEmptyStringOrEmptyObject(t *testing.T) {
	for _, raw := range []string{"", "{}"} {
		got, err := ParseStageExecutors(raw)
		if err != nil {
			t.Fatalf("ParseStageExecutors(%q): unexpected error: %v", raw, err)
		}
		if got == nil {
			t.Fatalf("ParseStageExecutors(%q): got nil map, want empty non-nil map", raw)
		}
		if len(got) != 0 {
			t.Fatalf("ParseStageExecutors(%q): got %d entries, want 0", raw, len(got))
		}
	}
}

func TestParseStageExecutors_should_ReturnErrorNotPanic_When_JSONIsMalformed(t *testing.T) {
	_, err := ParseStageExecutors(`{"triage": not-json`)
	if err == nil {
		t.Fatal("ParseStageExecutors: expected error for malformed JSON, got nil")
	}
}
