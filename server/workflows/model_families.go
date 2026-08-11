package workflows

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// modelFamilyPrefix namespaces a family alias in a workflow's stored Model field
// (e.g. "family:sonnet"). Only this namespaced form is treated as an alias —
// a bare "sonnet" string is left as a literal model ID, so a workflow that
// historically stored that exact string (before family aliases existed) is
// never silently reinterpreted.
const modelFamilyPrefix = "family:"

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

	data, err := os.ReadFile(configPath) //nolint:gosec
	if err != nil {
		return nil, err
	}

	var overrides map[string]string
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil, err
	}
	for family, modelID := range overrides {
		families[family] = modelID
	}
	return families, nil
}

// ResolveModel resolves a workflow's stored Model value to a concrete model ID
// using families. Values without the "family:" prefix (including "") pass
// through unchanged. An unknown or retired family alias returns an error
// rather than passing the broken "family:xxx" string through to the CLI.
//
// Decision record (client vs. server-side family resolution): resolution
// happens here, server-side, at fire-time — not client-side at save-time —
// so that updating a family's "latest" model only requires editing this
// package's override file (LoadModelFamilyOverride), with no frontend
// redeploy needed to pick it up. It also means every fire (manual RunWorkflow
// and cron) always resolves against the current map, and a workflow that
// already stores a concrete model ID (pre-dating this feature) is never
// touched — ResolveModel is a no-op for any value without the "family:"
// prefix.
func ResolveModel(families map[string]string, model string) (string, error) {
	if !strings.HasPrefix(model, modelFamilyPrefix) {
		return model, nil
	}
	alias := strings.TrimPrefix(model, modelFamilyPrefix)
	resolved, ok := families[alias]
	if !ok {
		known := make([]string, 0, len(families))
		for k := range families {
			known = append(known, k)
		}
		sort.Strings(known)
		return "", fmt.Errorf("unknown model family %q (known families: %s)", alias, strings.Join(known, ", "))
	}
	return resolved, nil
}

// modelRe matches a bare model identifier or "family:" alias: letters, digits,
// hyphens, underscores, dots, and at most one ':' namespace separator. No
// whitespace or shell metacharacters, since the resolved value is concatenated
// directly into a `claude --model <value>` program string at fire time
// (FireNow).
var modelRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*(:[a-zA-Z0-9][a-zA-Z0-9_.-]*)?$`)

// ValidateModel validates a workflow's Model field at save time (CreateWorkflow/
// UpdateWorkflow), so a malformed value is rejected up front instead of
// silently breaking workflow launch later at fire time. Empty is always valid
// (means "use the program's default model").
func ValidateModel(model string) error {
	if model == "" {
		return nil
	}
	if !modelRe.MatchString(model) {
		return fmt.Errorf("model must contain only letters, digits, '-', '_', '.', and at most one ':' namespace separator (no whitespace or other characters)")
	}
	return nil
}
