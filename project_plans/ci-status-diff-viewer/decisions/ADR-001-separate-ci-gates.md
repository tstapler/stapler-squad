# ADR-001: AC5 and AC6 are two independent gates on the same CI-status data, not one mechanism

**Status**: Accepted
**Date**: 2026-08-02

## Context

`requirements.md`'s acceptance criteria 5 and 6 both talk about "CI status gating an
approval," which reads as if they describe one feature:

- AC5: "A configurable rule (default: off) blocks the manual 'Approve' action in the
  review queue when CI status for the session's branch is failing."
- AC6: "The auto-approve rule engine ... supports a `ci_passing` condition that can be
  combined with existing conditions ... before a rule auto-approves a session."

Research (`research/architecture.md` §1b, `research/stack.md`'s "Caveat" section) traced
both to the actual code and found they are **different code paths gating different
actions**, not two views onto one gate:

- AC5 gates `ApprovalService.ResolveApproval` (`server/services/approval_service.go:46`)
  — a human clicking "Approve" on an already-escalated pending tool-use request.
- AC6 gates `RuleBasedClassifier.matchesRule`/`classifySingle`
  (`pkg/classifier/classifier.go:679,506`) — automatic classification of every
  Bash/Edit/Write tool-use request *before* it ever reaches the review queue.

`session/approval_policy.go`'s `PolicyEngine` — which might have been a plausible single
home for both — is confirmed dead code (no non-test callers anywhere in the tree; see
`research/stack.md` and `research/architecture.md` §1a).

## Decision

Implement AC5 and AC6 as two independent, separately-toggled gates that both read
`Instance.GitHubCheckConclusion` (`session/instance.go:206`), rather than building a
shared `CIGate`/`ApprovalGate` abstraction that both call into:

- AC5: an inline guard clause in `ApprovalService.ResolveApproval`, controlled by a
  global feature flag (`review:block-approval-on-ci-failure`, default off, reusing the
  existing `Config.FeatureFlags` mechanism — see `implementation/plan.md` Phase 2).
- AC6: a new `RequireCIPassing bool` field on `pkg/classifier`'s `Rule`/`RuleSpec`,
  ANDed with existing conditions in `matchesRule` exactly like `FilePattern`/
  `CommandPattern` — opt-in per rule, not a global toggle (see `implementation/plan.md`
  Phase 1).

## Alternatives considered

1. **Single shared `ApprovalGate` interface** invoked from both `ResolveApproval` and
   `matchesRule`. Rejected: at the time of writing there is exactly one "gate" behavior
   (CI-red blocks) with no second concrete need for polymorphism — an interface with one
   implementation is the "speculative interface" smell this repo's
   `.claude/rules/interface-pollution-checklist.md` explicitly flags. `ResolveApproval`
   and `matchesRule` also have incompatible call shapes (one is a per-session RPC
   returning a `connect.Error`, the other is a per-tool-call boolean predicate evaluated
   under a read lock) — a shared interface would need to paper over that mismatch rather
   than express it.
2. **Route AC5 through the classifier too** (treat "Approve" as another classified
   request). Rejected: `RuleBasedClassifier` classifies `PermissionRequestPayload`
   (tool-name/tool-input shaped), not "approve this pending request" — forcing the
   Approve action through that shape would require synthesizing a fake payload, more
   complex than the two-gate design and contrary to `research/architecture.md`'s finding
   that these are structurally different actions in the existing code.

## Consequences

- Two feature-flag/config surfaces instead of one: a global `Config.FeatureFlags` toggle
  for AC5, and a per-rule `RequireCIPassing` checkbox for AC6. This is intentional — AC5
  is "should I ever let a human override red CI," a workspace-wide policy; AC6 is
  "should *this specific rule* additionally require green CI," a per-rule authoring
  choice. Collapsing them into one toggle would force every auto-approve rule to obey
  the same CI policy as manual approval, which is not what AC6's "combined with existing
  conditions" phrasing asks for.
- Both gates read the same `GitHubCheckConclusion`/staleness data, so they inherit the
  same ~60s+API-latency freshness bound (`research/architecture.md` §3) — a future fix
  to CI staleness (e.g. a synchronous re-fetch at decision time) would need to be applied
  to both call sites, not one.
