package prs

import (
	"testing"

	"github.com/stretchr/testify/assert"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services/access"
)

const prWatchTestPR = githubpkg.LinkKey("github.com/o/r#7")

func TestPRWatchGuardSignature_should_EqualNudgeAndPRFixSignatures_When_SamePRAndReason(t *testing.T) {
	assert.Equal(t, access.GuardSignature(prWatchTestPR, access.ReasonSetSignature("FAILING_CHECKS")),
		PRWatchGuardSignature(prWatchTestPR, PRWatchKindCI))
	assert.Equal(t, access.GuardSignature(prWatchTestPR, access.ReasonSetSignature("UNRESOLVED_THREADS")),
		PRWatchGuardSignature(prWatchTestPR, PRWatchKindComment))
	assert.Equal(t, access.GuardSignature(prWatchTestPR, access.ReasonSetSignature("CHANGES_REQUESTED")),
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
