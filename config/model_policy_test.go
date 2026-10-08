package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelPolicyDefaults_NeverOpus(t *testing.T) {
	for _, k := range ModelPolicyKeys() {
		assert.False(t, strings.Contains(strings.ToLower(ModelPolicyDefault(k)), "opus"), k)
		assert.Empty(t, ValidateModelPolicyValue(k, ModelPolicyDefault(k)), k)
	}
	assert.Equal(t, "family:sonnet", (*Config)(nil).ModelPolicyValue(ModelPolicyWork))
	assert.Equal(t, "family:haiku", (&Config{}).ModelPolicyValue(ModelPolicyHandoffSummary))
}

func TestModelPolicyValue(t *testing.T) {
	cfg := &Config{ModelPolicy: map[string]string{
		ModelPolicyWork:             "family:opus",
		ModelPolicyReview:           "none",
		ModelPolicyBackgroundEffort: "bogus",
		ModelPolicyIntentParse:      "has space",
	}}
	assert.Equal(t, "family:opus", cfg.ModelPolicyValue(ModelPolicyWork), "explicit opt-in honored")
	assert.Empty(t, cfg.ModelPolicyValue(ModelPolicyReview), "none = account default")
	assert.Equal(t, "medium", cfg.ModelPolicyValue(ModelPolicyBackgroundEffort), "invalid effort falls back")
	assert.Equal(t, "family:haiku", cfg.ModelPolicyValue(ModelPolicyIntentParse), "invalid model falls back")
}

func TestValidateModelPolicyValue(t *testing.T) {
	assert.NotEmpty(t, ValidateModelPolicyValue("nope", "x"))
	assert.NotEmpty(t, ValidateModelPolicyValue(ModelPolicyBackgroundEffort, "huge"))
	assert.Empty(t, ValidateModelPolicyValue(ModelPolicyBackgroundEffort, "low"))
	assert.Empty(t, ValidateModelPolicyValue(ModelPolicyWork, ""))
}
