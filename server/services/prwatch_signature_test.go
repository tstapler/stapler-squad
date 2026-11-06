package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
)

const prWatchTestPR = githubpkg.LinkKey("github.com/o/r#7")

func TestPRWatchGuardSignature_should_EqualNudgeAndPRFixSignatures_When_SamePRAndReason(t *testing.T) {
	assert.Equal(t, guardSignature(prWatchTestPR, reasonSetSignature("FAILING_CHECKS")),
		PRWatchGuardSignature(prWatchTestPR, PRWatchKindCI))
	assert.Equal(t, guardSignature(prWatchTestPR, reasonSetSignature("UNRESOLVED_THREADS")),
		PRWatchGuardSignature(prWatchTestPR, PRWatchKindComment))
	assert.Equal(t, guardSignature(prWatchTestPR, reasonSetSignature("CHANGES_REQUESTED")),
		PRWatchGuardSignature(prWatchTestPR, PRWatchKindReview))
	// Subsumption matches the PR-fix path.
	assert.Equal(t, PRWatchGuardSignature(prWatchTestPR, PRWatchKindComment),
		PRWatchGuardSignature(prWatchTestPR, PRWatchKindReview, PRWatchKindComment))
}

func TestPRWatchGuardSignature_should_BeOrderIndependentAndPerPR(t *testing.T) {
	a := PRWatchGuardSignature(prWatchTestPR, PRWatchKindCI, PRWatchKindComment)
	assert.Equal(t, a, PRWatchGuardSignature(prWatchTestPR, PRWatchKindComment, PRWatchKindCI, PRWatchKindCI))
	assert.NotEqual(t, a, PRWatchGuardSignature(githubpkg.LinkKey("github.com/o/r#8"), PRWatchKindCI, PRWatchKindComment))
}

func TestSteerSessionGuarded_should_ReturnDuplicate_When_PRWatchSignatureRepeatsWithinWindow(t *testing.T) {
	svc, inst, rec, _ := newGuardedSteerFixture(t, notReadyNone)
	sig := PRWatchGuardSignature(prWatchTestPR, PRWatchKindCI)

	out, err := svc.SteerSessionGuarded(context.Background(), inst.GetStableID(), sig, "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerDelivered, out)

	out, err = svc.SteerSessionGuarded(context.Background(), inst.GetStableID(), sig, "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerDuplicate, out)
	assert.Equal(t, 1, rec.writes)

	// A manual-nudge style signature for the same PR and reason also dedupes.
	out, err = svc.SteerSessionGuarded(context.Background(), inst.GetStableID(),
		guardSignature(prWatchTestPR, reasonSetSignature("FAILING_CHECKS")), "msg")
	require.NoError(t, err)
	assert.Equal(t, SteerDuplicate, out)
}
