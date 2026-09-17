package session

import "encoding/json"

// StageRole identifies which pipeline stage a PipelineStageExecutor override
// applies to. It is a closed vocabulary — see the StageRole* consts — so
// ExecutorFor and the repository layer can't be handed a typo'd role string.
type StageRole string

const (
	StageRoleTriage StageRole = "triage"
	StageRoleReview StageRole = "review"
	StageRoleWork   StageRole = "work"
)

// PipelineStageExecutor is one stage's program/model override. An empty
// field means "use the default for that stage" rather than "unset".
type PipelineStageExecutor struct {
	Program string `json:"program"`
	Model   string `json:"model"`
}

// SerializeStageExecutors serializes a stage-role-to-executor override map
// to its JSON storage representation, mirroring domain.SerializeAcCriteria.
func SerializeStageExecutors(m map[StageRole]PipelineStageExecutor) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ParseStageExecutors deserializes a PipelineMode's stage_executors_json
// column. "" and "{}" are treated identically, both pre-migration rows and
// explicitly-empty overrides, returning an empty non-nil map with no error.
func ParseStageExecutors(raw string) (map[StageRole]PipelineStageExecutor, error) {
	if raw == "" || raw == "{}" {
		return map[StageRole]PipelineStageExecutor{}, nil
	}
	m := make(map[StageRole]PipelineStageExecutor)
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return m, nil
}
