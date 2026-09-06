package session

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseGateConfig_should_RejectUnknownKey_When_ConfigNamesKeyOutsideKindAllowlist
// (Task 2.7.2g4) proves every one of the 4 gate kinds rejects a config
// payload naming a key outside its own allowlist — the structural boundary
// architecture-review Concern 2 / adversarial-review Concern 1 call for.
func TestParseGateConfig_should_RejectUnknownKey_When_ConfigNamesKeyOutsideKindAllowlist(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		kind GateKind
		raw  string
	}{
		{"human_approval", GateKindHumanApproval, `{"extra_flag":"x"}`},
		{"automated_review", GateKindAutomatedReview, `{"pipeline_mode":"quick","extra_flag":"x"}`},
		{"structural", GateKindStructural, `{"check_id":"all_criteria_done","extra_flag":"x"}`},
		{"custom", GateKindCustom, `{"skill":"review-feasibility","extra_flag":"x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseGateConfig(tc.kind, json.RawMessage(tc.raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "extra_flag")
		})
	}
}

// TestParseGateConfig_should_RejectUnregisteredSkill_When_CustomConfigNamesSkillOutsideAllowlist
// (Task 2.7.2g4) proves a "custom" gate config naming a skill not in
// registeredCustomCheckSkills is rejected at parse/save time.
func TestParseGateConfig_should_RejectUnregisteredSkill_When_CustomConfigNamesSkillOutsideAllowlist(t *testing.T) {
	t.Parallel()
	_, err := ParseGateConfig(GateKindCustom, json.RawMessage(`{"skill":"delete-everything"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete-everything")
	assert.Contains(t, err.Error(), "not in the pre-registered allowlist")
}

// TestParseGateConfig_should_RoundTripValidConfig_When_KindIsHumanApproval
// (Task 2.7.2g4) proves all 4 kinds' valid configs parse into their typed
// GateConfig without error.
func TestParseGateConfig_should_RoundTripValidConfig_When_KindIsHumanApproval(t *testing.T) {
	t.Parallel()
	cfg, err := ParseGateConfig(GateKindHumanApproval, nil)
	require.NoError(t, err)
	assert.Equal(t, HumanApprovalConfig{}, cfg)
}

func TestParseGateConfig_should_RoundTripValidConfig_When_KindIsAutomatedReview(t *testing.T) {
	t.Parallel()
	cfg, err := ParseGateConfig(GateKindAutomatedReview, json.RawMessage(`{"pipeline_mode":"quick","requires_diff":true}`))
	require.NoError(t, err)
	assert.Equal(t, AutomatedReviewConfig{PipelineMode: "quick", RequiresDiff: true}, cfg)
}

// TestParseGateConfig_should_CoerceStringBool_When_ConfigArrivesFromProtoStringMap
// proves the wire path — a proto map<string,string> JSON-marshals every
// value as a quoted string — still parses correctly for a bool field, since
// the RPC handler's only way to hand ParseGateConfig a "requires_diff" value
// is as the JSON string "true"/"false", not a native JSON boolean.
func TestParseGateConfig_should_CoerceStringBool_When_ConfigArrivesFromProtoStringMap(t *testing.T) {
	t.Parallel()
	cfg, err := ParseGateConfig(GateKindAutomatedReview, json.RawMessage(`{"requires_diff":"true"}`))
	require.NoError(t, err)
	assert.Equal(t, AutomatedReviewConfig{RequiresDiff: true}, cfg)
}

func TestParseGateConfig_should_RoundTripValidConfig_When_KindIsStructural(t *testing.T) {
	t.Parallel()
	cfg, err := ParseGateConfig(GateKindStructural, json.RawMessage(`{"check_id":"all_criteria_done"}`))
	require.NoError(t, err)
	assert.Equal(t, StructuralConfig{CheckID: "all_criteria_done"}, cfg)
}

func TestParseGateConfig_should_RoundTripValidConfig_When_KindIsCustom(t *testing.T) {
	t.Parallel()
	cfg, err := ParseGateConfig(GateKindCustom, json.RawMessage(`{"skill":"review-feasibility"}`))
	require.NoError(t, err)
	assert.Equal(t, CustomCheckConfig{SkillID: "review-feasibility"}, cfg)
}

// TestParseGateConfig_should_RejectEmptyCheckID_When_StructuralConfigOmitsCheckID
// proves a structural gate can't be saved with an empty/missing check_id.
func TestParseGateConfig_should_RejectEmptyCheckID_When_StructuralConfigOmitsCheckID(t *testing.T) {
	t.Parallel()
	_, err := ParseGateConfig(GateKindStructural, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check_id")
}
