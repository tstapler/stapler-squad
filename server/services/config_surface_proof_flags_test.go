package services

// Story 5.1.3: runtime proofs for feature flags whose production reader is a
// function this package can call. Every proof writes the flag through
// FeatureFlagService.UpdateFeatureFlag (the Features page's write path) into an
// isolated config, then asks the production reader and requires a different
// answer. Helpers carry the bkf prefix.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/completionreview"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/forwardsync"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/hostclaim"
)

// bkfFlagWriter returns a setter that writes a flag through the production RPC
// into a config dir private to this test.
func bkfFlagWriter(t *testing.T) func(name string, on bool) {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	svc := flagsvc.NewFeatureFlagService()
	return func(name string, on bool) {
		t.Helper()
		require.NoError(t, flagUpdate(svc, name, on))
	}
}

func TestConfigSurfaceProof_EscapeEventsDBMigrate_ChangesMigrationEnabled(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(config.EscapeEventsDBMigrateFeatureFlag, setting.(bool))
		return config.LoadConfig().EscapeEventsDBMigrateEnabled()
	}, false, true)
}

func TestConfigSurfaceProof_FilesFlags_ChangeServerSideGates(t *testing.T) {
	set := bkfFlagWriter(t)
	for name, reader := range map[string]func(context.Context) bool{
		flagsvc.FilesEditingFlagName:    testFilesFlags().Editing,
		flagsvc.FilesViewerV2FlagName:   testFilesFlags().ViewerV2,
		flagsvc.FilesMergePhoneFlagName: testFilesFlags().MergePhone,
		flagsvc.FilesRangeReadFlagName:  testFilesFlags().RangeRead,
		flagsvc.FilesDeltaWriteFlagName: testFilesFlags().DeltaWrite,
		flagsvc.FilesEditLargeFlagName:  testFilesFlags().EditLarge,
	} {
		t.Run(name, func(t *testing.T) {
			svctest.AssertConfigChangesBehavior(t, func(setting any) any {
				set(name, setting.(bool))
				return reader(t.Context())
			}, false, true)
		})
	}
}

func TestConfigSurfaceProof_ProgramCLIFlagProbe_ChangesProbeGate(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(flagsvc.ProgramCLIFlagProbeFlagName, setting.(bool))
		return flagsvc.ProgramCLIFlagProbeEnabled(testFlagRegistry())
	}, false, true)
}

func TestConfigSurfaceProof_GitHubIssueProgressComments_ChangesGate(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(featureflags.GithubIssueProgressCommentsFlagName, setting.(bool))
		return forwardsync.GitHubIssueProgressCommentsEnabled(testFlagRegistry())
	}, false, true)
}

func TestConfigSurfaceProof_SDDDefaultPipeline_ChangesNewItemDefault(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(featureflags.SddDefaultPipelineFlagName, setting.(bool))
		return SddDefaultPipelineEnabled(testFlagRegistry())
	}, false, true)
}

func TestConfigSurfaceProof_CompletionReviewer_ChangesSubscriberGate(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(featureflags.CompletionReviewerFlagName, setting.(bool))
		return completionreview.CompletionReviewerEnabled()
	}, false, true)
}

// bkfForeignClaims reports a foreign claim for every URL, so the flag gate is
// the only thing that can turn the answer off.
type bkfForeignClaims struct{}

func (bkfForeignClaims) ForeignClaim(string) (hostclaim.ClaimRecord, bool) {
	return hostclaim.ClaimRecord{}, true
}

func TestConfigSurfaceProof_CrossHostClaimDedup_ChangesClaimLookup(t *testing.T) {
	set := bkfFlagWriter(t)
	lookup := NewFlagGatedForeignClaimLookup(bkfForeignClaims{})
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(featureflags.CrossHostClaimDedupFlagName, setting.(bool))
		_, found := lookup.ForeignClaim("https://example.test/issues/1")
		return found
	}, false, true)
}

func TestConfigSurfaceProof_AutonomyNudgeFlags_ChangeEffectiveReaders(t *testing.T) {
	set := bkfFlagWriter(t)
	for name, reader := range map[string]func(*config.Config) bool{
		config.TriageGuidanceHaltFeatureFlag:   config.EffectiveTriageGuidanceHaltEnabled,
		config.DiagnoseNudgeFeatureFlag:        config.EffectiveDiagnoseNudgeEnabled,
		config.AutonomousStallNudgeFeatureFlag: config.EffectiveAutonomousStallNudgeEnabled,
	} {
		t.Run(name, func(t *testing.T) {
			svctest.AssertConfigChangesBehavior(t, func(setting any) any {
				set(name, setting.(bool))
				return reader(config.LoadConfig())
			}, false, true)
		})
	}
}

// The four notification-actions flags resolve through effectiveFlag, which
// applies the plan's dependency order; each is read at its first decision point.
func TestConfigSurfaceProof_NotificationActionFlags_ChangeEffectiveFlag(t *testing.T) {
	set := bkfFlagWriter(t)
	for name, decisionPoint := range map[string]string{
		flagsvc.BackgroundActivityFeedFlagName:    "",
		flagsvc.NotificationInlineActionsFlagName: flagsvc.ScopePrefixAction + "APPROVE",
		flagsvc.AskUserQuestionStructuredFlagName: flagsvc.ScopeStageRender,
		flagsvc.AgentWakeTriggersFlagName:         "dispatcher",
	} {
		t.Run(name, func(t *testing.T) {
			svctest.AssertConfigChangesBehavior(t, func(setting any) any {
				set(name, setting.(bool))
				return flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveFlag(name, decisionPoint)
			}, false, true)
		})
	}
}

func TestConfigSurfaceProof_Should_IncludeAllSixAttentionFlags_When_Run(t *testing.T) {
	set := bkfFlagWriter(t)
	for name, reader := range map[string]func(flagsvc.AttentionFlags) bool{
		flagsvc.AttentionProseQuestionDetectionFlagName:   func(f flagsvc.AttentionFlags) bool { return f.ProseQuestionDetection },
		flagsvc.SessionEventLogFlagName:                   func(f flagsvc.AttentionFlags) bool { return f.SessionEventLog },
		flagsvc.AttentionAckBypassFixFlagName:             func(f flagsvc.AttentionFlags) bool { return f.AckBypassFix },
		flagsvc.AttentionIdleClockOutputFlagName:          func(f flagsvc.AttentionFlags) bool { return f.IdleClockOutput },
		flagsvc.AttentionProseQuestionLLMTiebreakFlagName: func(f flagsvc.AttentionFlags) bool { return f.ProseQuestionLLMTiebreak },
		flagsvc.AttentionInboxConsolidationFlagName:       func(f flagsvc.AttentionFlags) bool { return f.InboxConsolidation },
	} {
		t.Run(name, func(t *testing.T) {
			svctest.AssertConfigChangesBehavior(t, func(setting any) any {
				set(name, setting.(bool))
				return reader(flagsvc.LoadAttentionFlags(testFlagRegistry()))
			}, false, true)
		})
	}
}
