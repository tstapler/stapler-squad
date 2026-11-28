package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The diagnose prompt offers diagnose_reset_remediation, so the spawned session's
// allow-list must carry it.
func TestDiagnosticSessionAllowedTools_CarriesResetRemediation(t *testing.T) {
	t.Parallel()
	assert.Contains(t, diagnosticSessionAllowedTools, "mcp__stapler-squad__diagnose_reset_remediation")
}
