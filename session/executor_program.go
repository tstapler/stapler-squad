package session

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// modelFamilyPrefix namespaces a family alias in a stage/workflow's stored
// Model value (e.g. "family:sonnet"). Only this namespaced form is treated as
// an alias — a bare "sonnet" string is left as a literal model ID, so a value
// that historically stored that exact string (before family aliases existed)
// is never silently reinterpreted.
const modelFamilyPrefix = "family:"

// modelRe matches a bare model identifier or "family:" alias: letters,
// digits, hyphens, underscores, dots, and at most one ':' namespace
// separator. No whitespace or shell metacharacters, since the resolved value
// is concatenated directly into a `claude --model <value>` program string at
// fire/spawn time (server/workflows' FireNow, and this project's work-stage
// spawn path).
var modelRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*(:[a-zA-Z0-9][a-zA-Z0-9_.-]*)?$`)

// ValidateModel validates a model identifier string at save time (a
// Workflow's Model field, or a PipelineMode stage executor's Model field), so
// a malformed value is rejected up front instead of silently breaking
// execution later at fire/spawn time. Empty is always valid (means "use the
// program's default model").
//
// Lives in session rather than server/workflows because session cannot
// import server/workflows (would create an import cycle) but must perform
// this same check for PipelineMode's stage-executor validation
// (pipeline_mode_validation.go) — server/workflows/model_families.go's
// ValidateModel now delegates here instead of keeping its own copy.
func ValidateModel(model string) error {
	if model == "" {
		return nil
	}
	if !modelRe.MatchString(model) {
		return fmt.Errorf("model must contain only letters, digits, '-', '_', '.', and at most one ':' namespace separator (no whitespace or other characters)")
	}
	return nil
}

// ResolveModel resolves a stored Model value (a Workflow's Model field, or a
// PipelineMode stage executor's Model field) to a concrete model ID using
// families. Values without the "family:" prefix (including "") pass through
// unchanged. An unknown or retired family alias returns an error rather than
// passing the broken "family:xxx" string through to the CLI.
//
// Decision record (client vs. server-side family resolution): resolution
// happens here, server-side, at fire/spawn time — not client-side at
// save-time — so that updating a family's "latest" model only requires
// editing server/workflows' override file (LoadModelFamilyOverride), with no
// frontend redeploy needed to pick it up. It also means every fire (manual
// RunWorkflow, cron, and this project's headless/work-stage spawn paths)
// always resolves against the current map, and a value that already stores a
// concrete model ID (pre-dating this feature) is never touched — ResolveModel
// is a no-op for any value without the "family:" prefix.
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

// ResolveExecutorProgram resolves a stage's configured (baseProgram,
// modelValue) pair to a concrete program string — the single place the
// "family:sonnet" → concrete-model-ID → "claude --model <id>" transform
// happens, shared by FireNow (server/workflows/scheduler.go) and the
// work-stage spawn path, so they cannot independently drift on alias
// resolution or shell-escaping.
//
// The resolved model is appended via "--model" only when baseProgram is
// empty or "claude" — matching FireNow's original isClaudeProgram guard. For
// any other base program, the resolved model is silently not appended: a
// documented v1 limitation, since a per-stage model override for a
// non-Claude program isn't expressible via this string-concatenation
// mechanism.
func ResolveExecutorProgram(baseProgram, modelValue string, families map[string]string) (string, error) {
	resolvedModel, err := ResolveModel(families, modelValue)
	if err != nil {
		return "", err
	}

	program := baseProgram
	if resolvedModel != "" {
		isClaudeProgram := program == "" || program == "claude"
		if isClaudeProgram {
			program = "claude --model " + resolvedModel
		}
	}
	return program, nil
}
