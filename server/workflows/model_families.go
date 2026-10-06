package workflows

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/session"
)

// DefaultModelFamilies returns the hardcoded family alias → concrete model ID
// map, e.g. "sonnet" → "claude-sonnet-4-6". Keep in sync with the frontend's
// MODEL_FAMILIES (web-app/src/lib/constants/programs.ts) so every alias the UI
// offers actually resolves.
func DefaultModelFamilies() map[string]string {
	return map[string]string{
		"opus":   "claude-opus-4-8",
		"sonnet": "claude-sonnet-4-6",
		"haiku":  "claude-haiku-4-5-20251001",
	}
}

// LoadModelFamilyOverride loads family→model overrides from a JSON file and
// merges them over DefaultModelFamilies(), mirroring
// session/tokens/pricing.go's LoadPricingOverride. This is what lets a new
// Anthropic model version become a family's "latest" without a frontend (or
// even backend) redeploy — only the override file needs to change.
func LoadModelFamilyOverride(configPath string) (map[string]string, error) {
	families := DefaultModelFamilies()

	// #nosec G304 -- configPath is always configDir+"model_family_overrides.json" (see
	// dependencies.go), built from the internal config dir and a literal filename; never
	// network/RPC input.
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var overrides map[string]string
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil, err
	}
	for family, modelID := range overrides {
		// Override values bypass CreateWorkflow/UpdateWorkflow's save-time
		// ValidateModel call (they never go through the RPC layer), but they
		// reach the same fire-time sink (FireNow concatenates the resolved
		// value into a `claude --model <value>` program string) — so they
		// must be validated here too, or a malformed override file value
		// becomes a shell-metacharacter injection vector.
		if err := ValidateModel(modelID); err != nil {
			return nil, fmt.Errorf("model family override %q has invalid model id %q: %w", family, modelID, err)
		}
		families[family] = modelID
	}
	return families, nil
}

// ResolveModel resolves a workflow's stored Model value to a concrete model ID
// using families.
//
// Delegates to session.ResolveModel — the pure alias-resolution logic moved
// there (Task 2.2.1a) so both FireNow and this project's new work-stage/
// headless spawn paths (which live in session, not server/workflows) resolve
// family aliases through one shared implementation instead of two
// independently-drifting copies.
func ResolveModel(families map[string]string, model string) (string, error) {
	return session.ResolveModel(families, model)
}

// ValidateModel validates a workflow's Model field at save time (CreateWorkflow/
// UpdateWorkflow), so a malformed value is rejected up front instead of
// silently breaking workflow launch later at fire time. Empty is always valid
// (means "use the program's default model").
//
// Delegates to session.ValidateModel — the pure character-class check moved
// there so session/pipeline_mode_validation.go (which cannot import
// server/workflows) can reuse the identical check for PipelineMode stage
// executors without a second, independently-drifting copy.
func ValidateModel(model string) error {
	return session.ValidateModel(model)
}
