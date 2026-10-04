package session

// pipeline_mode_validation_test.go — tests for ValidatePipelineModeContent
// (Epic 2.3 of project_plans/backlog-configurable-pipeline). Test names and
// scenarios for the "explicit focus area" rows are taken verbatim from
// project_plans/backlog-configurable-pipeline/implementation/validation.md's
// "Story 2.3.1" rows.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Drift-prevention: renderTemplate vs ValidatePipelineModeContent ───────

// TestPlaceholderAllowList_should_BeIdenticalBetweenRenderTemplateAndValidator_When_ComparedDirectly
// directly compares the allow-list slice ValidatePipelineModeContent scans
// against (via placeholderAllowList) to the package-level recognizedPlaceholders
// var renderTemplate substitutes against. Both currently resolve to the
// exact same shared var, so this is trivially true today — it exists purely
// to catch a FUTURE regression where someone gives one function its own
// copy that drifts from the other.
func TestPlaceholderAllowList_should_BeIdenticalBetweenRenderTemplateAndValidator_When_ComparedDirectly(t *testing.T) {
	t.Parallel()
	got := placeholderAllowList()

	require.Equal(t, recognizedPlaceholders, got,
		"ValidatePipelineModeContent's allow-list must equal renderTemplate's recognizedPlaceholders")

	// Also assert it's the literal SAME backing slice, not merely an
	// equal-valued copy — this is what actually catches a future "someone
	// gave one function its own copy" regression that require.Equal alone
	// would miss if the copy happened to start out identical.
	require.NotEmpty(t, got)
	require.Same(t, &recognizedPlaceholders[0], &got[0],
		"placeholderAllowList() must return the literal shared recognizedPlaceholders slice, not a copy")
}

// TestRenderTemplateAndValidatePipelineModeContent_should_AgreeOnEveryToken_When_TableDrivenOverAllPlaceholdersPlusOneBogusToken
// is table-driven over the 7 recognized placeholder names plus one synthetic
// unrecognized name (made_up_placeholder). For each: renderTemplate
// substitutes it (no literal {{name}} survives) if-and-only-if
// ValidatePipelineModeContent accepts a template using only that token. This
// is the behavioral cross-check that catches drift even if the two
// functions do NOT literally share a slice.
func TestRenderTemplateAndValidatePipelineModeContent_should_AgreeOnEveryToken_When_TableDrivenOverAllPlaceholdersPlusOneBogusToken(t *testing.T) {
	t.Parallel()
	tokens := make([]string, 0, len(recognizedPlaceholders)+1)
	tokens = append(tokens, recognizedPlaceholders...)
	tokens = append(tokens, "made_up_placeholder")

	item := &BacklogItemData{ID: "item-1", Title: "Title", Description: "Desc", RepoPath: "/repo"}
	placeholders := itemPlaceholders(item)
	placeholders["criteria_index"] = "1"
	placeholders["criteria_count"] = "3"
	placeholders["criteria_text"] = "criterion text"

	for _, token := range tokens {
		t.Run(token, func(t *testing.T) {
			t.Parallel()
			tmpl := "{{" + token + "}}"

			rendered := renderTemplate(tmpl, placeholders)
			substituted := !strings.Contains(rendered, "{{"+token+"}}")

			err := ValidatePipelineModeContent(PipelineModeContentFields{
				TriagePromptTemplate: tmpl,
			})
			accepted := err == nil

			assert.Equal(t, substituted, accepted,
				"token %q: renderTemplate substituted=%v but ValidatePipelineModeContent accepted=%v (err=%v)",
				token, substituted, accepted, err)
		})
	}
}

// ─── Placeholder allow-list ─────────────────────────────────────────────────

// TestValidatePipelineModeContent_should_Accept_When_AllPlaceholdersAreRecognized
// (validation.md row 59): a template using {{item_id}}: {{item_title}}
// (both recognized) passes validation.
func TestValidatePipelineModeContent_should_Accept_When_AllPlaceholdersAreRecognized(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		Slug:                 "quick",
		ValidateSlug:         true,
		TriagePromptTemplate: "Fix {{item_id}}: {{item_title}}.",
	})
	assert.NoError(t, err)
}

// TestValidatePipelineModeContent_should_RejectNamingFieldAndToken_When_UnrecognizedPlaceholderUsed
// (validation.md row 60): {{made_up_placeholder}} in triage_prompt_template
// is rejected with an error naming both the field and the token.
func TestValidatePipelineModeContent_should_RejectNamingFieldAndToken_When_UnrecognizedPlaceholderUsed(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		Slug:                 "quick",
		ValidateSlug:         true,
		TriagePromptTemplate: "Fix {{item_id}} using {{made_up_placeholder}}.",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "triage_prompt_template")
	assert.Contains(t, err.Error(), "made_up_placeholder")
}

// ─── Slug format ─────────────────────────────────────────────────────────

// TestValidatePipelineModeContent_should_RejectWithInvalidSlugMessage_When_SlugContainsUppercaseOrPunctuation
// (validation.md row 61): "Quick Fix!" is rejected; "quick-fix" is accepted.
func TestValidatePipelineModeContent_should_RejectWithInvalidSlugMessage_When_SlugContainsUppercaseOrPunctuation(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		Slug:         "Quick Fix!",
		ValidateSlug: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug")

	err = ValidatePipelineModeContent(PipelineModeContentFields{
		Slug:         "quick-fix",
		ValidateSlug: true,
	})
	assert.NoError(t, err)
}

// TestValidatePipelineModeContent_should_RejectEmptySlug_When_ValidateSlugTrue
// covers the "empty slug" half of Story 2.3.1's slug acceptance criteria.
func TestValidatePipelineModeContent_should_RejectEmptySlug_When_ValidateSlugTrue(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		Slug:         "",
		ValidateSlug: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slug")
}

// TestValidatePipelineModeContent_should_SkipSlugValidation_When_ValidateSlugFalse
// proves an Update-style call (ValidateSlug: false, Slug left as its zero
// value because UpdatePipelineModeRequest has no slug field) never fails
// slug validation it wasn't asked to perform.
func TestValidatePipelineModeContent_should_SkipSlugValidation_When_ValidateSlugFalse(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		ValidateSlug:         false,
		TriagePromptTemplate: "Fix {{item_id}}.",
	})
	assert.NoError(t, err)
}

// ─── Shell metacharacters (defense in depth) ────────────────────────────────

// TestValidatePipelineModeContent_should_RejectShellMetacharacters_When_ContentTemplateFieldContainsOne
// is table-driven over shellMetacharacters, proving each blocked sequence is
// rejected when present in a content-template field, naming the field.
func TestValidatePipelineModeContent_should_RejectShellMetacharacters_When_ContentTemplateFieldContainsOne(t *testing.T) {
	t.Parallel()
	for _, meta := range shellMetacharacters {
		t.Run(meta, func(t *testing.T) {
			t.Parallel()
			err := ValidatePipelineModeContent(PipelineModeContentFields{
				TriagePromptTemplate: "Run " + meta + " rm -rf /",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "triage_prompt_template")
		})
	}
}

// TestValidatePipelineModeContent_should_Accept_When_ContentTemplateFieldsContainNoMetacharacters
// is the zero-regression companion: ordinary content-template text with no
// shell metacharacters and only recognized placeholders passes.
func TestValidatePipelineModeContent_should_Accept_When_ContentTemplateFieldsContainNoMetacharacters(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		Slug:                  "quick",
		ValidateSlug:          true,
		StatusCommandTemplate: "Status for {{item_id}}: {{item_title}}",
		TriagePromptTemplate:  "Triage {{item_id}} — {{item_description}}",
		ReviewPromptTemplate:  "Review criterion {{criteria_index}}/{{criteria_count}}: {{criteria_text}}",
		InitialPromptTemplate: "Work on {{repo_path}} for {{item_id}}",
	})
	assert.NoError(t, err)
}

// ─── Stage executors (Epic 1.3 of backlog-stage-execution-costs) ───────────

// TestValidateStageExecutors_should_Accept_When_TriageProgramIsClaudeOrGemini
// (plan Task 1.3.1f): both headless-capable programs are accepted for the
// triage role.
func TestValidateStageExecutors_should_Accept_When_TriageProgramIsClaudeOrGemini(t *testing.T) {
	t.Parallel()
	for _, program := range []string{"claude", "gemini"} {
		t.Run(program, func(t *testing.T) {
			t.Parallel()
			err := ValidatePipelineModeContent(PipelineModeContentFields{
				StageExecutors: map[StageRole]PipelineStageExecutor{
					StageRoleTriage: {Program: program},
				},
			})
			assert.NoError(t, err)
		})
	}
}

// TestValidateStageExecutors_should_RejectNamingProgramAndStage_When_AiderConfiguredForTriage
// (plan.md Story 1.3.1 AC1): "aider" has no headless mode, so it's rejected
// for the triage role, naming both the rejected program and the stage.
func TestValidateStageExecutors_should_RejectNamingProgramAndStage_When_AiderConfiguredForTriage(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleTriage: {Program: "aider"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aider")
	assert.Contains(t, err.Error(), "triage")
}

// TestValidateStageExecutors_should_RejectNamingProgramAndStage_When_AiderConfiguredForReview
// mirrors the triage case for the review role — the headless-only allow-list
// applies to both non-work roles.
func TestValidateStageExecutors_should_RejectNamingProgramAndStage_When_AiderConfiguredForReview(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleReview: {Program: "aider"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aider")
	assert.Contains(t, err.Error(), "review")
}

// TestValidateStageExecutors_should_Accept_When_AiderConfiguredForWorkStage
// (plan.md Story 1.3.1 AC2): the work stage spawns an interactive Instance,
// not a headless call, so it has no program allow-list.
func TestValidateStageExecutors_should_Accept_When_AiderConfiguredForWorkStage(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleWork: {Program: "aider"},
		},
	})
	assert.NoError(t, err)
}

// TestValidateStageExecutors_should_Accept_When_ModelIsResolvableFamilyAlias
// (plan.md Story 1.3.1 AC3, family-alias branch): a "family:"-prefixed alias
// skips the pricing-table cross-check entirely.
func TestValidateStageExecutors_should_Accept_When_ModelIsResolvableFamilyAlias(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleReview: {Model: "family:opus"},
		},
	})
	assert.NoError(t, err)
}

// TestValidateStageExecutors_should_Reject_When_ModelContainsShellMetacharacters
// covers Task 1.3.1c's character-class guard (shared with
// server/workflows.ValidateModel) independent of the pricing cross-check.
func TestValidateStageExecutors_should_Reject_When_ModelContainsShellMetacharacters(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleTriage: {Model: "claude; rm -rf /"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "triage")
}

// TestValidateStageExecutors_should_RejectNamingModelAndOverride_When_ModelIsUnrecognized
// (plan.md Story 1.3.1 AC3): a syntactically valid but unrecognized model ID
// is rejected unless force_unknown_model is set — the whole point of this
// story is catching a BUG-062-style typo a character-class check alone
// cannot catch.
func TestValidateStageExecutors_should_RejectNamingModelAndOverride_When_ModelIsUnrecognized(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleReview: {Model: "claude-opus-9000"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claude-opus-9000")
	assert.Contains(t, err.Error(), "force_unknown_model")
}

// TestValidateStageExecutors_should_Accept_When_UnrecognizedModelHasForceUnknownModelSet
// is the override half of the same acceptance criterion.
func TestValidateStageExecutors_should_Accept_When_UnrecognizedModelHasForceUnknownModelSet(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleReview: {Model: "claude-opus-9000"},
		},
		ForceUnknownModel: true,
	})
	assert.NoError(t, err)
}

// TestValidateStageExecutors_should_Accept_When_ModelIsKnownPricingTableEntry
// is the zero-regression companion: a real, recognized model ID (normalizes
// to a known pricing-table family) is accepted with no override needed.
func TestValidateStageExecutors_should_Accept_When_ModelIsKnownPricingTableEntry(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleTriage: {Model: "claude-haiku-4-5"},
		},
	})
	assert.NoError(t, err)
}

// TestValidateStageExecutors_should_RejectNamingKeyAndValidRoles_When_MapKeyIsInvalid
// (plan.md Story 1.3.1 AC4): a typo'd map key ("wrok") is rejected rather
// than silently round-tripping through JSON/proto and never being consulted
// by ExecutorFor.
func TestValidateStageExecutors_should_RejectNamingKeyAndValidRoles_When_MapKeyIsInvalid(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRole("wrok"): {Model: "claude-haiku-4-5"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrok")
	assert.Contains(t, err.Error(), string(StageRoleTriage))
	assert.Contains(t, err.Error(), string(StageRoleReview))
	assert.Contains(t, err.Error(), string(StageRoleWork))
}

// TestValidateStageExecutors_should_Accept_When_ProgramAndModelAreBothEmpty
// covers the "inherit default" case every stage-executor entry allows.
func TestValidateStageExecutors_should_Accept_When_ProgramAndModelAreBothEmpty(t *testing.T) {
	t.Parallel()
	err := ValidatePipelineModeContent(PipelineModeContentFields{
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleWork: {},
		},
	})
	assert.NoError(t, err)
}
