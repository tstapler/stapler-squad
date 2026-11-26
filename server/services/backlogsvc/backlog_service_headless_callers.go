package backlogsvc

import (
	"slices"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/headless"
)

// SetHeadlessPool wires the headless pool for autonomous triage calls and
// registers it in headlessCallers under "claude" — the fallback target every
// resolveHeadlessCaller branch degrades to.
func (s *BacklogService) SetHeadlessPool(pool headless.PoolClient) {
	s.headlessPool = pool
	if s.headlessCallers == nil {
		s.headlessCallers = make(map[string]headless.PoolClient)
	}
	s.headlessCallers["claude"] = pool
}

// SetBackendSelector routes the default headless client (empty stage program)
// and every known backend name through sel, so per-feature settings and
// capability/availability fallback apply. headlessCallers["claude"] keeps the
// raw pool so a stage explicitly configured "claude" is never re-routed.
func (s *BacklogService) SetBackendSelector(sel *headless.Selector) {
	s.backendSelector = sel
	if sel != nil {
		s.headlessPool = &headless.SelectingClient{Selector: sel}
	}
}

// SetGeminiCaller registers caller in headlessCallers under "gemini" so a
// PipelineMode stage executor configured with program="gemini" resolves to it
// (Epic 3.1/2.3). Takes the concrete *headless.GeminiCaller type rather than
// the headless.PoolClient interface so a nil caller (gemini binary not found
// at startup — see server/dependencies.go) is caught by this ordinary nil
// check: wrapping a typed nil pointer in an interface first would make an
// interface-level `== nil` check pass a non-nil caller through by mistake.
func (s *BacklogService) SetGeminiCaller(caller *headless.GeminiCaller) {
	if caller == nil {
		return
	}
	if s.headlessCallers == nil {
		s.headlessCallers = make(map[string]headless.PoolClient)
	}
	s.headlessCallers["gemini"] = caller
}

// availabilityChecker is satisfied by a headless.PoolClient that can re-probe
// its own runtime availability at call time (e.g. GeminiCaller.Available(),
// Task 3.1.1g) — not part of the headless.PoolClient interface itself since
// Claude's Pool has no equivalent "goes missing mid-run" failure mode (see
// GeminiCaller.Available's doc comment).
type availabilityChecker interface {
	Available() bool
}

// resolveHeadlessCaller picks the headless.PoolClient to run a stage's
// configured program, failing closed to Claude — loudly, via a Warn log and a
// returned fallbackReason for persistence onto the ItemSession row — rather
// than silently no-op'ing, crashing, or trusting a startup-time-only
// availability check that can go stale (Story 2.3.1).
//
// program is the raw value ExecutorFor returned (empty or "claude" is the
// overwhelmingly common case and skips the map lookup and availability probe
// entirely). itemID/stage are for the log line and are not otherwise
// interpreted.
func (s *BacklogService) resolveHeadlessCaller(program, itemID, stage string) (caller headless.PoolClient, configuredProgram, fallbackReason string) {
	if program == "claude" {
		if raw, ok := s.headlessCallers["claude"]; ok {
			return raw, "", ""
		}
	}
	if program == "" || program == "claude" {
		return s.headlessPool, "", ""
	}
	if s.backendSelector != nil && slices.Contains(headless.KnownBackendNames(), program) {
		_, res := s.backendSelector.Resolve(headless.FeatureKey(stage), program, headless.Caps{})
		reason := res.FallbackReason
		if reason != "" {
			log.Warn("[PipelineEngine] headless program fell back", "program", program, "item", itemID, "stage", stage, "reason", reason)
			return &headless.SelectingClient{Selector: s.backendSelector}, program, reason
		}
		return &headless.SelectingClient{Selector: s.backendSelector, StageProgram: program}, "", ""
	}
	found, ok := s.headlessCallers[program]
	if !ok {
		log.Warn("[PipelineEngine] unsupported headless program", "program", program, "item", itemID, "stage", stage, "fallback", "claude")
		return s.headlessPool, program, "unsupported_program"
	}
	if ac, ok := found.(availabilityChecker); ok && !ac.Available() {
		log.Warn("[PipelineEngine] headless program unavailable", "program", program, "item", itemID, "stage", stage, "fallback", "claude")
		return s.headlessPool, program, program + "_unavailable"
	}
	return found, "", ""
}
