package backlogdto

// PipelineMode identifies which PipelineMode definition (by slug) drives a
// backlog item's triage/work/review content.
type PipelineMode string

// PipelineModeDefault is the sentinel PipelineMode value meaning "no mode
// chosen — use the pre-existing hardcoded pipeline." Resolving this value is
// guaranteed, by construction, to never touch pipelineModeCache or
// PipelineModeRepository — this is the concrete mechanism that keeps "no
// uncached DB read on the hot path for the common case" true without needing
// runtime feature-flagging.
const PipelineModeDefault PipelineMode = ""

// ResolvedModeLabel renders a raw BacklogItemData.PipelineMode string for
// [PipelineEngine]-prefixed log lines: the empty string (PipelineModeDefault)
// becomes "default" for log readability, any other slug is passed through
// unchanged. Exported so both server/services (TriggerTriage) and
// session (ReviewGateRunner.Run) call sites use one shared rendering — see
// Story 1.7.2's observability acceptance criteria.
func ResolvedModeLabel(mode string) string {
	if mode == "" {
		return "default"
	}
	return mode
}
