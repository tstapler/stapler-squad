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
	"github.com/tstapler/stapler-squad/session"
)

// bkfFlagWriter returns a setter that writes a flag through the production RPC
// into a config dir private to this test.
func bkfFlagWriter(t *testing.T) func(name string, on bool) {
	t.Helper()
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	return func(name string, on bool) {
		t.Helper()
		require.NoError(t, flagUpdate(svc, name, on))
	}
}

func TestConfigSurfaceProof_FilesFlags_ChangeServerSideGates(t *testing.T) {
	set := bkfFlagWriter(t)
	for name, reader := range map[string]func(context.Context) bool{
		filesEditingFlagName:    FilesEditingEnabled,
		filesViewerV2FlagName:   FilesViewerV2Enabled,
		filesMergePhoneFlagName: FilesMergePhoneEnabled,
		filesRangeReadFlagName:  FilesRangeReadEnabled,
		filesDeltaWriteFlagName: FilesDeltaWriteEnabled,
		filesEditLargeFlagName:  FilesEditLargeEnabled,
	} {
		t.Run(name, func(t *testing.T) {
			assertConfigChangesBehavior(t, func(setting any) any {
				set(name, setting.(bool))
				return reader(t.Context())
			}, false, true)
		})
	}
}

func TestConfigSurfaceProof_ProgramCLIFlagProbe_ChangesProbeGate(t *testing.T) {
	set := bkfFlagWriter(t)
	assertConfigChangesBehavior(t, func(setting any) any {
		set(programCLIFlagProbeFlagName, setting.(bool))
		return ProgramCLIFlagProbeEnabled()
	}, false, true)
}

func TestConfigSurfaceProof_GitHubIssueProgressComments_ChangesGate(t *testing.T) {
	set := bkfFlagWriter(t)
	assertConfigChangesBehavior(t, func(setting any) any {
		set(githubIssueProgressCommentsFlagName, setting.(bool))
		return GitHubIssueProgressCommentsEnabled()
	}, false, true)
}

func TestConfigSurfaceProof_SDDDefaultPipeline_ChangesNewItemDefault(t *testing.T) {
	set := bkfFlagWriter(t)
	assertConfigChangesBehavior(t, func(setting any) any {
		set(sddDefaultPipelineFlagName, setting.(bool))
		return sddDefaultPipelineEnabled()
	}, false, true)
}

func TestConfigSurfaceProof_CompletionReviewer_ChangesSubscriberGate(t *testing.T) {
	set := bkfFlagWriter(t)
	assertConfigChangesBehavior(t, func(setting any) any {
		set(completionReviewerFlagName, setting.(bool))
		return completionReviewerEnabled()
	}, false, true)
}

// bkfForeignClaims reports a foreign claim for every URL, so the flag gate is
// the only thing that can turn the answer off.
type bkfForeignClaims struct{}

func (bkfForeignClaims) ForeignClaim(string) (session.ClaimRecord, bool) {
	return session.ClaimRecord{}, true
}

func TestConfigSurfaceProof_CrossHostClaimDedup_ChangesClaimLookup(t *testing.T) {
	set := bkfFlagWriter(t)
	lookup := NewFlagGatedForeignClaimLookup(bkfForeignClaims{})
	assertConfigChangesBehavior(t, func(setting any) any {
		set(crossHostClaimDedupFlagName, setting.(bool))
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
			assertConfigChangesBehavior(t, func(setting any) any {
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
		backgroundActivityFeedFlagName:    "",
		notificationInlineActionsFlagName: scopePrefixAction + "APPROVE",
		askUserQuestionStructuredFlagName: scopeStageRender,
		agentWakeTriggersFlagName:         "dispatcher",
	} {
		t.Run(name, func(t *testing.T) {
			assertConfigChangesBehavior(t, func(setting any) any {
				set(name, setting.(bool))
				return effectiveFlag(name, decisionPoint)
			}, false, true)
		})
	}
}
