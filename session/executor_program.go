package session

import (
	"fmt"
	"regexp"
)

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
