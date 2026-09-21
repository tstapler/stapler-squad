package tokens

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestClassifyActivity_WhenSkillNameSignalPresent_ExpectSkillSignalOutranksToolRatio
// is the literal Story 1.2.3 AC example: a 90% Read-call ratio would alone
// suggest ActivityExploratory, but a "code-debugging" skill activation must
// win.
func TestClassifyActivity_WhenSkillNameSignalPresent_ExpectSkillSignalOutranksToolRatio(t *testing.T) {
	t.Parallel()
	r := &ParseResult{
		SkillActivations: []SkillActivation{{Name: "code-debugging"}},
		ToolUsage: map[string]ToolTokenStats{
			"Read": {ToolName: "Read", CallCount: 90},
			"Bash": {ToolName: "Bash", CallCount: 10},
		},
	}

	assert.Equal(t, ActivityDebugging, ClassifyActivity(r))
}

func TestClassifyActivity_WhenRefactorSkillPresent_ExpectActivityRefactoring(t *testing.T) {
	t.Parallel()
	r := &ParseResult{
		SkillActivations: []SkillActivation{{Name: "code-refactoring"}},
	}

	assert.Equal(t, ActivityRefactoring, ClassifyActivity(r))
}

// TestClassifyActivity_WhenDebugAndRefactorSkillsBothPresent_ExpectDebugCheckedFirst
// covers the plan's "checked in that order" tie-break rule.
func TestClassifyActivity_WhenDebugAndRefactorSkillsBothPresent_ExpectDebugCheckedFirst(t *testing.T) {
	t.Parallel()
	r := &ParseResult{
		SkillActivations: []SkillActivation{{Name: "debug-and-refactor-helper"}},
	}

	assert.Equal(t, ActivityDebugging, ClassifyActivity(r))
}

func TestClassifyActivity_WhenNoSkillMatchAndHighWriteRatio_ExpectActivityFeatureDev(t *testing.T) {
	t.Parallel()
	r := &ParseResult{
		ToolUsage: map[string]ToolTokenStats{
			"Edit": {ToolName: "Edit", CallCount: 4},
			"Read": {ToolName: "Read", CallCount: 6},
		},
	}

	assert.Equal(t, ActivityFeatureDev, ClassifyActivity(r))
}

func TestClassifyActivity_WhenNoSkillMatchAndHighReadRatio_ExpectActivityExploratory(t *testing.T) {
	t.Parallel()
	r := &ParseResult{
		ToolUsage: map[string]ToolTokenStats{
			"Read": {ToolName: "Read", CallCount: 7},
			"Bash": {ToolName: "Bash", CallCount: 3},
		},
	}

	assert.Equal(t, ActivityExploratory, ClassifyActivity(r))
}

func TestClassifyActivity_WhenNoSkillMatchAndNeitherRatioMet_ExpectActivityOther(t *testing.T) {
	t.Parallel()
	r := &ParseResult{
		ToolUsage: map[string]ToolTokenStats{
			"Bash": {ToolName: "Bash", CallCount: 10},
		},
	}

	assert.Equal(t, ActivityOther, ClassifyActivity(r))
}

// TestClassifyActivity_WhenNoToolUsageRecorded_ExpectActivityOther is the
// Story 1.2.3 zero-tool-call edge case.
func TestClassifyActivity_WhenNoToolUsageRecorded_ExpectActivityOther(t *testing.T) {
	t.Parallel()
	r := &ParseResult{}

	assert.Equal(t, ActivityOther, ClassifyActivity(r))
}

func TestClassifyActivity_WhenNilParseResult_ExpectActivityOther(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ActivityOther, ClassifyActivity(nil))
}
