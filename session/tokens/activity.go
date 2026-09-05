package tokens

import (
	"strings"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// ActivityType classifies the kind of work a session performed.
// Maps to session.v1.ActivityType enum in Go, same alias pattern as
// FindingType/Severity (session/tokens/finding_types.go).
type ActivityType = sessionv1.ActivityType

// ActivityType constants — the 5 non-zero values ActivityType ships in v1.
const (
	ActivityDebugging   = sessionv1.ActivityType_ACTIVITY_TYPE_DEBUGGING
	ActivityRefactoring = sessionv1.ActivityType_ACTIVITY_TYPE_REFACTORING
	ActivityFeatureDev  = sessionv1.ActivityType_ACTIVITY_TYPE_FEATURE_DEV
	ActivityExploratory = sessionv1.ActivityType_ACTIVITY_TYPE_EXPLORATORY
	ActivityOther       = sessionv1.ActivityType_ACTIVITY_TYPE_OTHER
)

// writeRatioThreshold and readRatioThreshold are the tool-call-ratio fallback
// cutoffs for ClassifyActivity's priority-2 heuristic — see plan.md's Story
// 1.2.3 acceptance criteria.
const (
	writeRatioThreshold = 0.3
	readRatioThreshold  = 0.6
)

// writeToolNames and readToolNames are the tool-name buckets ClassifyActivity
// sums call counts over to compute writeRatio/readRatio.
var (
	writeToolNames = map[string]bool{"Edit": true, "Write": true, "NotebookEdit": true}
	readToolNames  = map[string]bool{"Read": true, "Grep": true, "Glob": true}
)

// ClassifyActivity classifies a session's activity type: a skill-name
// substring match ("debug"/"refactor") takes priority, falling back to the
// writeRatio/readRatio tool-call-ratio heuristic above when no skill matched.
// See plan.md's Story 1.2.3 acceptance criteria for the full priority rules.
func ClassifyActivity(r *ParseResult) ActivityType {
	if r == nil {
		return ActivityOther
	}

	for _, sa := range r.SkillActivations {
		name := strings.ToLower(sa.Name)
		if strings.Contains(name, "debug") {
			return ActivityDebugging
		}
		if strings.Contains(name, "refactor") {
			return ActivityRefactoring
		}
	}

	var writeCalls, readCalls, totalCalls int
	for toolName, stat := range r.ToolUsage {
		totalCalls += stat.CallCount
		if writeToolNames[toolName] {
			writeCalls += stat.CallCount
		}
		if readToolNames[toolName] {
			readCalls += stat.CallCount
		}
	}
	if totalCalls == 0 {
		return ActivityOther
	}

	writeRatio := float64(writeCalls) / float64(totalCalls)
	if writeRatio >= writeRatioThreshold {
		return ActivityFeatureDev
	}
	readRatio := float64(readCalls) / float64(totalCalls)
	if readRatio >= readRatioThreshold {
		return ActivityExploratory
	}
	return ActivityOther
}
