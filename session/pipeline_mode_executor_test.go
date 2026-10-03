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

// TestSerializeStageExecutors_should_ProduceEmptyObjectNotNull_When_MapIsNil guards the
// "clear all stage-executor overrides" UI action, which passes a nil map — a bare `"null"`
// would violate the stage_executors_json schema invariant (see
// session/ent/schema/pipeline_mode.go's "no override configured" doc comment).
func TestSerializeStageExecutors_should_ProduceEmptyObjectNotNull_When_MapIsNil(t *testing.T) {
	raw, err := SerializeStageExecutors(nil)
	if err != nil {
		t.Fatalf("SerializeStageExecutors(nil): %v", err)
	}
	if raw != "{}" {
		t.Fatalf("SerializeStageExecutors(nil) = %q, want %q", raw, "{}")
	}

	got, err := ParseStageExecutors(raw)
	if err != nil {
		t.Fatalf("ParseStageExecutors(%q): %v", raw, err)
	}
	if got == nil {
		t.Fatal("ParseStageExecutors round trip of nil-map serialization: got nil map, want empty non-nil map")
	}
	if len(got) != 0 {
		t.Fatalf("ParseStageExecutors round trip of nil-map serialization: got %d entries, want 0", len(got))
	}
}
