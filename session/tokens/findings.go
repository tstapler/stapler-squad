package tokens

import (
	"fmt"

	"github.com/tstapler/stapler-squad/log"
)

// minTurnsForCacheFloor guards detectCacheHitFloorBreach against cold-start
// noise: a session's first few turns naturally have a low cache-hit rate
// before the cache warms up, so a session shorter than this never fires.
const minTurnsForCacheFloor = 5

// cacheHitFloor is the minimum acceptable cache-hit rate before a session is
// flagged as wasteful. Calibrated 2026-09-03 (validation.md's Threshold
// Calibration step) against the operator's real local ~/.claude/projects
// corpus (600 sessions sampled): the plan's original 0.40 fiat value fired on
// 0/600 sessions (below the 2% "reads as broken" line) because this
// operator's real cache-hit-rate distribution runs far hotter than a generic
// 40% floor assumes — p1=0.784, p5=0.934, p10=0.964, median=1.000 among
// eligible (>=5-turn, priced) sessions. Raised to 0.95 so the floor sits
// between this corpus's p5 and p10, firing on roughly 5-10% of sessions
// (within Failure #2's healthy 2-50% band) instead of 0%.
const cacheHitFloor = 0.95

// cacheHitCriticalFloor is the hit rate below which a cache-hit-floor breach
// is SeverityCritical rather than SeverityWarn. Calibrated alongside
// cacheHitFloor: the corpus's worst observed rate was 0.477, well below a
// generic "critical" line proportional to the new floor, so this stays a
// meaningfully lower bar than cacheHitFloor itself rather than scaling with it.
const cacheHitCriticalFloor = 0.85

// sessionTokenCeiling flags an unusually large single session at this
// project's current ~600-session scale. Calibrated 2026-09-03: fired on
// 185/600 (30.8%) of the real corpus at this value — within Failure #2's
// healthy 2-50% band, so left unchanged from the plan's original value.
const sessionTokenCeiling = 2_000_000

// oversizedContextFloor flags a session whose first turn's context is
// unusually large — a typical Claude Code system prompt plus a modest
// CLAUDE.md is a few thousand tokens, so a first turn well above that
// suggests an oversized CLAUDE.md or start-of-session file reads. Calibrated
// 2026-09-03: fired on 69/600 (11.5%) of the real corpus at this value —
// within Failure #2's healthy 2-50% band, so left unchanged from the plan's
// original value.
const oversizedContextFloor = 30_000

// sessionTokenCeilingCriticalCostUSD is the dollar-cost line above which a
// sessionTokenCeiling breach escalates from SeverityWarn to SeverityCritical.
const sessionTokenCeilingCriticalCostUSD = 20

// oversizedContextCriticalFloor is the first-turn context-token line above
// which an oversizedStartContext breach escalates from SeverityWarn to
// SeverityCritical.
const oversizedContextCriticalFloor = 80_000

// detectCacheHitFloorBreach flags a session whose cache-hit rate is below
// cacheHitFloor. Abstains (returns nil) rather than firing a misleading
// $0.00 finding when the session is too short to have a warmed-up cache, or
// when its model has no PricingTable entry — same "abstain rather than
// guess" discipline as ComputeCacheROI.
func detectCacheHitFloorBreach(r *ParseResult, pt *PricingTable) *Finding {
	if r == nil || pt == nil || len(r.TurnTimeline) < minTurnsForCacheFloor {
		return nil
	}
	pricing, priced := pt.LookupByModel(r.PrimaryModel)
	if !priced {
		return nil
	}
	hitRate := ComputeCacheHitRate(r.TotalInput, r.CacheRead)
	if hitRate >= cacheHitFloor {
		return nil
	}

	impact := (cacheHitFloor - hitRate) * float64(r.TotalInput+r.CacheRead) *
		(pricing.InputPricePerMTok - pricing.CacheReadPerMTok) / 1e6
	if impact < 0 {
		impact = 0
	}

	severity := SeverityWarn
	if hitRate < cacheHitCriticalFloor {
		severity = SeverityCritical
	}

	return &Finding{
		Type:         FindingCacheHitFloorBreach,
		Severity:     severity,
		DollarImpact: DollarImpact(impact),
		Message: fmt.Sprintf(
			"Cache hit rate %.0f%% is below the %.0f%% floor over %d turns — an estimated $%.2f in avoidable input-token cost.",
			hitRate*100, cacheHitFloor*100, len(r.TurnTimeline), impact,
		),
	}
}

// detectSessionTokenCeiling flags a session whose total token usage exceeds
// sessionTokenCeiling. Abstains when any model involved is unpriced, rather
// than firing a finding with an understated (partial) dollar impact.
func detectSessionTokenCeiling(r *ParseResult, pt *PricingTable) *Finding {
	if r == nil || pt == nil {
		return nil
	}
	totalTokens := r.TotalInput + r.TotalOutput + r.CacheCreation + r.CacheRead
	if totalTokens <= sessionTokenCeiling {
		return nil
	}

	costUSD, unpricedModels := pt.EstimateCost(r)
	if len(unpricedModels) > 0 {
		return nil
	}

	severity := SeverityWarn
	if costUSD > sessionTokenCeilingCriticalCostUSD {
		severity = SeverityCritical
	}

	return &Finding{
		Type:         FindingSessionTokenCeiling,
		Severity:     severity,
		DollarImpact: DollarImpact(costUSD),
		Message: fmt.Sprintf(
			"Session used %s tokens, over the %s ceiling — estimated cost $%.2f.",
			formatInt(totalTokens), formatInt(sessionTokenCeiling), costUSD,
		),
	}
}

// detectModelSwitchCacheBust flags a mid-session model switch immediately
// followed by a cache-bust turn (CacheRead == 0 && CacheCreation > 0),
// summing the avoidable cache-write cost across every such priced switch.
// Abstains entirely (nil) if no priced switch is found among however many
// were detected — never fires a zero-impact finding for an unpriced switch.
func detectModelSwitchCacheBust(r *ParseResult, pt *PricingTable) *Finding {
	if r == nil || pt == nil || len(r.Models) < 2 || len(r.TurnTimeline) < 2 {
		return nil
	}

	var (
		totalImpact  float64
		firedAny     bool
		firstTurnIdx int
		firstFrom    string
		firstTo      string
	)

	for i := 1; i < len(r.TurnTimeline); i++ {
		prev := r.TurnTimeline[i-1]
		cur := r.TurnTimeline[i]
		if prev.Model == "" || cur.Model == "" || prev.Model == cur.Model {
			continue
		}
		if !(cur.CacheRead == 0 && cur.CacheCreation > 0) {
			continue
		}

		pricing, priced := pt.LookupByModel(cur.Model)
		if !priced {
			continue
		}

		if !firedAny {
			firstTurnIdx = i
			firstFrom = prev.Model
			firstTo = cur.Model
		}
		firedAny = true
		totalImpact += float64(cur.CacheCreation) / 1e6 * pricing.CacheWritePerMTok
	}

	if !firedAny {
		return nil
	}

	return &Finding{
		Type:         FindingModelSwitchCacheBust,
		Severity:     SeverityWarn,
		DollarImpact: DollarImpact(totalImpact),
		Message: fmt.Sprintf(
			"Turn %d switched model from %s to %s, busting the cache — estimated $%.2f in avoidable cache-write cost.",
			firstTurnIdx+1, firstFrom, firstTo, totalImpact,
		),
	}
}

// detectOversizedStartContext flags a session whose first turn's context
// (input + cache-read, since the very first turn structurally has no cache
// read yet) exceeds oversizedContextFloor. Abstains when the first turn's
// model is unpriced.
func detectOversizedStartContext(r *ParseResult, pt *PricingTable) *Finding {
	if r == nil || pt == nil || len(r.TurnTimeline) == 0 {
		return nil
	}

	first := r.TurnTimeline[0]
	contextTokens := first.Input + first.CacheRead
	if contextTokens <= oversizedContextFloor {
		return nil
	}

	pricing, priced := pt.LookupByModel(first.Model)
	if !priced {
		return nil
	}

	impact := float64(contextTokens) / 1e6 * pricing.InputPricePerMTok

	severity := SeverityWarn
	if contextTokens > oversizedContextCriticalFloor {
		severity = SeverityCritical
	}

	return &Finding{
		Type:         FindingOversizedStartContext,
		Severity:     severity,
		DollarImpact: DollarImpact(impact),
		Message: fmt.Sprintf(
			"Session started with %s tokens of context (threshold: %s) — consider trimming CLAUDE.md or start-of-session file reads.",
			formatInt(contextTokens), formatInt(oversizedContextFloor),
		),
	}
}

// detectorFunc is the uniform signature every v1 detector shares.
type detectorFunc func(r *ParseResult, pt *PricingTable) *Finding

// ComputeFindings runs all 4 shipped detectors against r and returns whatever
// fires, in detector-declaration order (the caller, insights_service.go,
// sorts the request-wide accumulation by dollar impact separately). Each
// detector call is isolated with a recover() so one detector panicking on a
// malformed session can never take down the rest of the batch, or the
// caller's other sessions — matching the Observability Requirement that a
// computation error shows up as an empty/error state, not a page.
func ComputeFindings(r *ParseResult, pt *PricingTable) []Finding {
	findings := make([]Finding, 0, 4)

	detectors := []struct {
		name string
		fn   detectorFunc
	}{
		{"detectCacheHitFloorBreach", detectCacheHitFloorBreach},
		{"detectSessionTokenCeiling", detectSessionTokenCeiling},
		{"detectModelSwitchCacheBust", detectModelSwitchCacheBust},
		{"detectOversizedStartContext", detectOversizedStartContext},
	}

	for _, d := range detectors {
		if f := runDetectorSafely(d.name, d.fn, r, pt); f != nil {
			findings = append(findings, *f)
		}
	}

	return findings
}

// runDetectorSafely calls fn under a recover() guard, logging (rather than
// propagating) any panic. Factored out of ComputeFindings so
// findings_test.go's panic-isolation test can exercise the identical
// wrapping logic against a test-local double without an exported/mutable
// package-level seam.
func runDetectorSafely(name string, fn detectorFunc, r *ParseResult, pt *PricingTable) (result *Finding) {
	defer func() {
		if rec := recover(); rec != nil {
			sessionUUID := ""
			if r != nil {
				sessionUUID = r.SessionUUID
			}
			log.Warn("finding detector panicked", "detector", name, "session", sessionUUID, "recover", rec)
			result = nil
		}
	}()
	return fn(r, pt)
}

// ComputeWasteScore returns a single sortable 0-100 badness number for a
// session, or nil when the session is too sparse to evaluate meaningfully
// (fewer than minTurnsForCacheFloor turns) — nil, not 0, so "not evaluated"
// is never confused with "evaluated and clean". This is a weighted blend of
// ratios, NOT a sum of finding dollar impacts — see ADR-002.
func ComputeWasteScore(r *ParseResult, pt *PricingTable) *WasteScore {
	if r == nil || pt == nil || len(r.TurnTimeline) < minTurnsForCacheFloor {
		return nil
	}

	hitRate := ComputeCacheHitRate(r.TotalInput, r.CacheRead)
	cacheShortfall := clamp01(cacheHitFloor - hitRate)

	totalTokens := r.TotalInput + r.TotalOutput + r.CacheCreation + r.CacheRead
	ceilingPenalty := clamp01(float64(totalTokens) / float64(sessionTokenCeiling))

	first := r.TurnTimeline[0]
	contextTokens := first.Input + first.CacheRead
	contextPenalty := clamp01(float64(contextTokens) / float64(oversizedContextFloor))

	score := cacheShortfall*40 + ceilingPenalty*30 + contextPenalty*30
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	ws := WasteScore(score)
	return &ws
}

// clamp01 clamps v into [0, 1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// formatInt formats a token/threshold count with thousands separators, e.g.
// 3000000 -> "3,000,000", for finding messages.
func formatInt(n int64) string {
	if n < 0 {
		return "-" + formatInt(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	rem := len(s) % 3
	if rem == 0 {
		rem = 3
	}
	out = append(out, s[:rem]...)
	for i := rem; i < len(s); i += 3 {
		out = append(out, ',')
		out = append(out, s[i:i+3]...)
	}
	return string(out)
}
