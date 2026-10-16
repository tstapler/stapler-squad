package session

// pipeline_mode_validation.go — structural validation for PipelineMode's
// slug and 9 content-template fields (Epic 2.3 of
// project_plans/backlog-configurable-pipeline). Called from
// server/services/backlog_service_pipeline_mode.go's CreatePipelineMode and
// UpdatePipelineMode handlers before any repository write, per Story 2.3.1's
// acceptance criteria and the plan's now-resolved Unresolved Questions entry
// on write-time placeholder allow-list rejection.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/session/tokens"
)

// pipelineModeSlugRe matches the allowed pipeline-mode slug character set:
// lowercase letters, digits, and hyphens, non-empty. Deliberately looser
// than session/workflow_slug.go's ValidateWorkflowSlug (which additionally
// forbids leading/trailing/consecutive hyphens and enforces a 2-64 char
// length) — Story 2.3.1's acceptance criteria only calls for "empty slug or
// characters outside [a-z0-9-]" to be rejected for pipeline-mode slugs.
// WorkflowRepository's create path is the precedent for the general shape
// (a lowercase-alphanumeric-plus-hyphen slug format-checked at the write
// boundary), not a requirement to reuse the exact same regex/length rules.
var pipelineModeSlugRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// shellMetacharacters is a minimal, defense-in-depth blocklist of
// characters/sequences that could enable command injection if a
// content-template field were ever read into a shell context. The design
// itself never does this (see the NFR on structural integrity, not access
// control) — this check exists purely to make that invariant enforced
// rather than only documented. Chosen to cover the most common shell
// metacharacter injection primitives: command substitution (backtick,
// `$(`) and command chaining/piping (`;`, `|`, `&&`). This is deliberately
// NOT a comprehensive shell-escaping blocklist (it does not reject `<`,
// `>`, `*`, `?`, or a bare `&`) — the goal is a minimal, defensible
// enforcement of "never shell-interpreted," not a general-purpose shell
// sanitizer.
var shellMetacharacters = []string{"`", "$(", ";", "|", "&&"}

// placeholderTokenRe extracts {{name}} tokens from a content-template field,
// the same token shape renderTemplate (session/pipeline_engine.go)
// substitutes — used here for scanning only, never substitution.
var placeholderTokenRe = regexp.MustCompile(`\{\{([a-zA-Z0-9_]+)\}\}`)

// PipelineModeContentFields groups the slug and the 9 content-template
// fields ValidatePipelineModeContent checks.
//
// ValidateSlug should be true only for a Create-style call: slug is
// required and format-checked there. Update never sets it, because
// UpdatePipelineModeRequest has no slug field at all — slug is immutable
// after creation (see proto/session/v1/backlog.proto) — so there is nothing
// for an Update call to validate.
//
// Any content-template field left as "" (e.g. a field a partial Update
// request didn't set) trivially passes both content checks below (an empty
// string contains no shell metacharacters and no placeholder tokens), so
// callers building this struct for an Update only need to populate whichever
// fields the request actually sets — omitted fields never fail validation
// they didn't ask for.
type PipelineModeContentFields struct {
	Slug         string
	ValidateSlug bool

	StatusCommandTemplate string
	DoneCommandTemplate   string
	FailCommandTemplate   string
	ReviewCommandTemplate string
	ShipCommandTemplate   string
	HelpCommandTemplate   string
	TriagePromptTemplate  string
	ReviewPromptTemplate  string
	InitialPromptTemplate string

	// StageExecutors and ForceUnknownModel are Epic 1.3's stage-executor
	// checks (see validateStageExecutors). A nil/empty StageExecutors map
	// trivially passes — an Update request that isn't touching stage
	// executors at all only needs to leave this unset, same convention as
	// the 9 content-template fields above.
	StageExecutors    map[StageRole]PipelineStageExecutor
	ForceUnknownModel bool
}

// namedTemplateFields returns the 9 content-template fields paired with
// their proto/JSON field names (used in error messages), in the same fixed
// order pipeline_engine.go's ComputeContentHash callers use.
func (f PipelineModeContentFields) namedTemplateFields() []struct {
	name  string
	value string
} {
	return []struct {
		name  string
		value string
	}{
		{"status_command_template", f.StatusCommandTemplate},
		{"done_command_template", f.DoneCommandTemplate},
		{"fail_command_template", f.FailCommandTemplate},
		{"review_command_template", f.ReviewCommandTemplate},
		{"ship_command_template", f.ShipCommandTemplate},
		{"help_command_template", f.HelpCommandTemplate},
		{"triage_prompt_template", f.TriagePromptTemplate},
		{"review_prompt_template", f.ReviewPromptTemplate},
		{"initial_prompt_template", f.InitialPromptTemplate},
	}
}

// ValidatePipelineModeContent enforces Story 2.3.1's structural-integrity
// invariants at the RPC write boundary, before any repository write occurs:
//
//  1. If fields.ValidateSlug, fields.Slug must be non-empty and contain only
//     characters in [a-z0-9-].
//  2. None of the 9 content-template fields may contain a raw shell
//     metacharacter from shellMetacharacters (defense in depth).
//  3. Every {{...}} token in every content-template field must name a
//     placeholder in the recognized allow-list (recognizedPlaceholders,
//     declared in pipeline_engine.go and also used by renderTemplate) — an
//     unrecognized token is rejected, naming both the offending field and
//     the unrecognized token.
//
// Returns nil if fields passes all checks.
func ValidatePipelineModeContent(fields PipelineModeContentFields) error {
	if fields.ValidateSlug {
		if err := validatePipelineModeSlug(fields.Slug); err != nil {
			return err
		}
	}

	for _, tf := range fields.namedTemplateFields() {
		if tf.value == "" {
			continue
		}
		if err := validateNoShellMetacharacters(tf.name, tf.value); err != nil {
			return err
		}
		if err := validateRecognizedPlaceholders(tf.name, tf.value); err != nil {
			return err
		}
	}

	if err := validateStageExecutors(fields.StageExecutors, fields.ForceUnknownModel); err != nil {
		return err
	}

	return nil
}

// validStageRoles is the closed set of map keys stage_executors may use —
// StageRole's 3 exported consts. A typo'd key (e.g. "wrok") would otherwise
// round-trip silently through JSON/proto (StageRole is a newtype, not a
// validated sum type) and simply never be consulted by ExecutorFor.
var validStageRoles = map[StageRole]bool{
	StageRoleTriage: true,
	StageRoleReview: true,
	StageRoleWork:   true,
}

// headlessOnlyPrograms is the allow-list ADR-002 permits for
// StageRoleTriage/StageRoleReview's Program field: both roles run
// exclusively through a headless.PoolClient (Strategy pattern registry),
// which currently has adapters for "claude" and "gemini" only. Aider has no
// headless mode, so it's rejected for these two roles but allowed for
// StageRoleWork, which spawns an interactive Instance instead and is not
// checked against this allow-list.
var headlessOnlyPrograms = map[string]bool{
	"claude": true,
	"gemini": true,
}

// validateStageExecutors enforces Story 1.3.1's per-stage program/model
// checks:
//  1. Every map key must be one of the 3 known StageRole consts.
//  2. A non-empty Program on StageRoleTriage/StageRoleReview must be
//     "claude" or "gemini" (ADR-002); StageRoleWork has no such allow-list.
//  3. A non-empty Model must pass the shared character-class guard
//     (ValidateModel, shared with server/workflows to catch
//     shell-metacharacter/whitespace injection) and, unless
//     forceUnknownModel is set, must resolve to a known pricing-table entry
//     when it is not a "family:"-prefixed alias — catching a BUG-062-style
//     typo (syntactically valid, but not a real model) at save time. A
//     "family:" alias skips the pricing cross-check entirely; it's already
//     validated by ResolveModel's own unknown-alias error.
func validateStageExecutors(executors map[StageRole]PipelineStageExecutor, forceUnknownModel bool) error {
	for role, executor := range executors {
		if !validStageRoles[role] {
			return fmt.Errorf("stage_executors: invalid role %q (must be one of %q, %q, %q)",
				role, StageRoleTriage, StageRoleReview, StageRoleWork)
		}

		if executor.Program != "" && (role == StageRoleTriage || role == StageRoleReview) {
			if !headlessOnlyPrograms[executor.Program] {
				return fmt.Errorf("stage_executors[%s].program: %q has no headless mode (ADR-002) — must be one of \"claude\", \"gemini\"",
					role, executor.Program)
			}
		}

		if executor.Model == "" {
			continue
		}
		if err := ValidateModel(executor.Model); err != nil {
			return fmt.Errorf("stage_executors[%s].model: %w", role, err)
		}
		if err := validateKnownModel(executor.Model, forceUnknownModel); err != nil {
			return fmt.Errorf("stage_executors[%s].model: %w", role, err)
		}
	}
	return nil
}

// validateKnownModel cross-checks a literal (non-"family:"-prefixed) model ID
// against tokens.DefaultPricingTable(). forceUnknownModel bypasses the check
// for a real-but-not-yet-priced model (a new model shipped before the
// pricing table was updated) without silently re-permitting a typo.
func validateKnownModel(model string, forceUnknownModel bool) error {
	if strings.HasPrefix(model, "family:") || forceUnknownModel {
		return nil
	}
	if _, ok := tokens.DefaultPricingTable().LookupByModel(model); !ok {
		return fmt.Errorf("%q is not a recognized model (set force_unknown_model to override)", model)
	}
	return nil
}

// validatePipelineModeSlug rejects an empty slug or one containing
// characters outside [a-z0-9-].
func validatePipelineModeSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug: must not be empty")
	}
	if !pipelineModeSlugRe.MatchString(slug) {
		return fmt.Errorf("slug: must contain only lowercase letters, digits, and hyphens (got %q)", slug)
	}
	return nil
}

// validateNoShellMetacharacters rejects value if it contains any of
// shellMetacharacters, naming both fieldName and the offending sequence.
func validateNoShellMetacharacters(fieldName, value string) error {
	for _, meta := range shellMetacharacters {
		if strings.Contains(value, meta) {
			return fmt.Errorf("%s: must not contain shell metacharacter %q", fieldName, meta)
		}
	}
	return nil
}

// validateRecognizedPlaceholders scans value for {{...}} tokens and rejects
// the first one whose name is not in recognizedPlaceholders (the same
// package-level allow-list renderTemplate substitutes against — see
// pipeline_engine.go), naming both fieldName and the unrecognized token.
func validateRecognizedPlaceholders(fieldName, value string) error {
	for _, match := range placeholderTokenRe.FindAllStringSubmatch(value, -1) {
		token := match[1]
		if !isRecognizedPlaceholder(token) {
			return fmt.Errorf("%s: unrecognized placeholder {{%s}}", fieldName, token)
		}
	}
	return nil
}

// isRecognizedPlaceholder reports whether name is in recognizedPlaceholders.
func isRecognizedPlaceholder(name string) bool {
	for _, recognized := range recognizedPlaceholders {
		if recognized == name {
			return true
		}
	}
	return false
}

// placeholderAllowList returns the exact allow-list slice
// ValidatePipelineModeContent scans against — the same package-level
// recognizedPlaceholders var renderTemplate (pipeline_engine.go) uses.
// Exposed for the drift-prevention test
// (TestPlaceholderAllowList_should_BeIdenticalBetweenRenderTemplateAndValidator_When_ComparedDirectly),
// which fails immediately if a future edit gives one function its own copy
// that drifts from the other.
func placeholderAllowList() []string {
	return recognizedPlaceholders
}
