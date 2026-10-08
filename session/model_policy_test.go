package session

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/config"
)

var testFamilies = map[string]string{"opus": "m-opus", "sonnet": "m-sonnet", "haiku": "m-haiku"}

func TestResolveFeatureModel(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		key     string
		program string
		want    string
	}{
		{"default work is sonnet", nil, config.ModelPolicyWork, "", "m-sonnet"},
		{"default headless is haiku", &config.Config{}, config.ModelPolicyPRDescription, "claude", "m-haiku"},
		{"opus only when explicit", &config.Config{ModelPolicy: map[string]string{config.ModelPolicyWork: "family:opus"}}, config.ModelPolicyWork, "", "m-opus"},
		{"non-claude program gets nothing", nil, config.ModelPolicyWork, "aider", ""},
		{"none opts out", &config.Config{ModelPolicy: map[string]string{config.ModelPolicyReview: "none"}}, config.ModelPolicyReview, "", ""},
		{"unknown family falls back to default", &config.Config{ModelPolicy: map[string]string{config.ModelPolicyWork: "family:nope"}}, config.ModelPolicyWork, "", "m-sonnet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveFeatureModel(tt.cfg, testFamilies, tt.key, tt.program))
		})
	}
}

func TestApplyModelPolicyToProgram(t *testing.T) {
	assert.Equal(t, "claude --foo --model m-sonnet --effort medium", ApplyModelPolicyToProgram(nil, testFamilies, config.ModelPolicyWork, "claude --foo"))
	assert.Empty(t, ApplyModelPolicyToProgram(nil, testFamilies, config.ModelPolicyWork, "aider --x"), "non-claude untouched")
	assert.Equal(t, "claude --model x --effort medium", ApplyModelPolicyToProgram(nil, testFamilies, config.ModelPolicyWork, "claude --model x"), "existing --model kept")
	assert.Empty(t, ApplyModelPolicyToProgram(nil, testFamilies, config.ModelPolicyWork, "claude --model x --effort high"))
	cfg := &config.Config{ModelPolicy: map[string]string{config.ModelPolicyBackgroundEffort: "none"}}
	assert.Equal(t, "claude --model m-sonnet", ApplyModelPolicyToProgram(cfg, testFamilies, config.ModelPolicyWork, "claude"))
}
