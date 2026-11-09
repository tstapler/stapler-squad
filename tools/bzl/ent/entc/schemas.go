package main

import (
	"entgo.io/ent"

	entschema "github.com/tstapler/stapler-squad/session/ent/schema"
)

// allSchemas lists every ent.Schema type in session/ent/schema; checkNames fails the action when
// it drifts from the sources.
func allSchemas() []ent.Interface {
	return []ent.Interface{
		entschema.AcAuditEvent{},
		entschema.AnalyticsEvent{},
		entschema.ApprovalRule{},
		entschema.BacklogActivityNote{},
		entschema.BacklogFlowMarker{},
		entschema.BacklogItem{},
		entschema.BacklogItemDependency{},
		entschema.BacklogProgressNote{},
		entschema.BacklogStage{},
		entschema.BacklogStatusEvent{},
		entschema.BacklogStuckEpisode{},
		entschema.BacklogStuckState{},
		entschema.ClassificationAnalytics{},
		entschema.ClaudeMetadata{},
		entschema.ClaudeSession{},
		entschema.CompactionEvent{},
		entschema.ContextSample{},
		entschema.DeletedItemSessionCost{},
		entschema.DiffStats{},
		entschema.DismissedFinding{},
		entschema.ErrorEvent{},
		entschema.EscapeEvent{},
		entschema.GateSatisfactionRecord{},
		entschema.GuidanceRequest{},
		entschema.HandoffSummary{},
		entschema.IntegrationCredential{},
		entschema.ItemSession{},
		entschema.ItemSource{},
		entschema.PipelineMode{},
		entschema.Project{},
		entschema.PullRequestState{},
		entschema.ReviewVerdict{},
		entschema.Session{},
		entschema.SessionGoal{},
		entschema.SessionPR{},
		entschema.SessionPRTombstone{},
		entschema.SessionSummary{},
		entschema.Shell{},
		entschema.SourceSyncEvent{},
		entschema.StageLivenessDefinition{},
		entschema.StageTransition{},
		entschema.Tag{},
		entschema.TaggingRule{},
		entschema.TaggingRuleFire{},
		entschema.TranscriptScanState{},
		entschema.TransitionGate{},
		entschema.TriggerFireEvent{},
		entschema.WakeTrigger{},
		entschema.WebhookRegistration{},
		entschema.WebhookRequestLedger{},
		entschema.Workflow{},
		entschema.Worktree{},
	}
}
