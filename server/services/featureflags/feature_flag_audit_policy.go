package featureflags

import (
	"slices"
)

// FlagAuditPolicy says how a flag's flips are audited. Tightening flips never
// wait on the sink; loosening flips are audited durably before they persist.
type FlagAuditPolicy struct {
	// Loosening reports whether setting the flag to enabled reduces a protection.
	Loosening func(enabled bool) bool
	// EscapeHatch lets a loosening flip persist on the fallback log record alone
	// when the sink is down (the flag is the escape hatch for a sink fault).
	EscapeHatch bool
	// EscapeHatchScopes extends EscapeHatch to the named scopes only, for a flag
	// whose other scopes must stay refused while the sink is down.
	EscapeHatchScopes []string
}

func (p FlagAuditPolicy) EscapeHatchFor(scope string) bool {
	return p.EscapeHatch || slices.Contains(p.EscapeHatchScopes, scope)
}
