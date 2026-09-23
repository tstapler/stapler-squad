package tokens

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// FindingType identifies which waste-pattern heuristic produced a Finding.
// Maps to session.v1.FindingType enum in Go — see
// proto/session/v1/types.proto:423's precedent for this alias pattern.
type FindingType = sessionv1.FindingType

// Severity classifies how urgently a Finding should be acted on.
// Maps to session.v1.Severity enum in Go, same alias pattern as FindingType.
type Severity = sessionv1.Severity

// FindingType constants — 4 non-zero values ship in v1. Two more heuristics
// (redundant/large-file-reads, tool-failure-rate) are deferred — see plan.md's
// "Detector scope cut" note — and are intentionally not aliased here yet.
const (
	FindingCacheHitFloorBreach   = sessionv1.FindingType_FINDING_TYPE_CACHE_HIT_FLOOR_BREACH
	FindingSessionTokenCeiling   = sessionv1.FindingType_FINDING_TYPE_SESSION_TOKEN_CEILING
	FindingModelSwitchCacheBust  = sessionv1.FindingType_FINDING_TYPE_MODEL_SWITCH_CACHE_BUST
	FindingOversizedStartContext = sessionv1.FindingType_FINDING_TYPE_OVERSIZED_START_CONTEXT
	// Reserved for future detectors, deferred per plan.md:
	// FindingRedundantFileReads = sessionv1.FindingType_FINDING_TYPE_REDUNDANT_FILE_READS
	// FindingToolFailureRate    = sessionv1.FindingType_FINDING_TYPE_TOOL_FAILURE_RATE
)

// Severity constants.
const (
	SeverityInfo     = sessionv1.Severity_SEVERITY_INFO
	SeverityWarn     = sessionv1.Severity_SEVERITY_WARN
	SeverityCritical = sessionv1.Severity_SEVERITY_CRITICAL
)

// DollarImpact is a modeled USD estimate for one finding's waste. There is no
// exported Sum/Add helper anywhere in this codebase — dollar impacts across
// findings are not summable (a session-token-ceiling breach and a cache-hit-floor
// breach on the same session double-count the same wasted tokens from different
// angles). See ADR-002-findings-non-summable-dollar-impact.md. This is a
// review-convention guardrail, not a compiler guarantee: Go cannot forbid `+`
// on a numeric newtype.
type DollarImpact float64

// WasteScore is a single sortable 0-100 badness number for a session, computed
// by ComputeWasteScore. It is NOT a sum of finding dollar impacts — it's a
// weighted blend of ratios (cache-hit shortfall, ceiling proximity, oversized
// start-context proximity). See ADR-002-findings-non-summable-dollar-impact.md.
type WasteScore float64

// Finding is one waste-pattern verdict for a single session, produced by a
// detector in findings.go and translated to the sessionv1.WasteFinding proto
// message by the caller (server/services/insights_service.go).
type Finding struct {
	Type         FindingType
	Severity     Severity
	DollarImpact DollarImpact
	Message      string
}
