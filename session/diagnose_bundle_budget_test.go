package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateTokens_UsesBytesPerTokenRatio(t *testing.T) {
	t.Parallel()
	s := strings.Repeat("a", 4000)
	assert.Equal(t, 1000, EstimateTokens(s))
}

func TestEstimateTokens_EmptyString_ReturnsZero(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, EstimateTokens(""))
}

func TestExceedsDiagnosticBundleBudget_UnderBudget_ReturnsFalse(t *testing.T) {
	t.Parallel()
	s := strings.Repeat("a", DiagnosticBundleTokenBudget*bytesPerTokenEstimate-1)
	assert.False(t, ExceedsDiagnosticBundleBudget(s))
}

func TestExceedsDiagnosticBundleBudget_OverBudget_ReturnsTrue(t *testing.T) {
	t.Parallel()
	s := strings.Repeat("a", (DiagnosticBundleTokenBudget+1)*bytesPerTokenEstimate)
	assert.True(t, ExceedsDiagnosticBundleBudget(s))
}
