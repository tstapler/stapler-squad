# Architecture Review: monitor-waiting-indicator
**Date**: 2026-09-09
**Verdict**: CLEAN

## Constitution Check

No `docs/adr/ADR-000-architecture-constitution.md` exists in this repo (checked
`docs/adr/` listing) — skipped, no constraints to apply.

## Grounding (verified against real code, not just plan.md prose)

- `session/detection/binaries/claude.go:342-381`, `session/detection/pattern_set.go:87-109`,
  `session/detection/detector.go:19-76` — read directly. `matchWaitingForAgent` today reads only
  `m[1]` exactly as ADR-001/plan.md claim; `footerAgentCount` already sums two groups exactly as
  cited; the "confirmed dead" comment block does sit above all three `WaitingForAgent` patterns as
  described.
- `server/adapters/instance_adapter.go:252-256`, `session/instance_status.go:22-24,133-137`,
  `web-app/src/components/sessions/SubStatusChip.tsx:21,34-61` — confirmed `SubagentCount` flows
  unconditionally from detector → proto → chip, matching AC2's "already satisfied" claim.
  `grep -n "STAPLER_SQUAD_USE_CONTROL_MODE"` near `GetStatusAndIdleInfo`/`GetRecentHash` in
  `session/claude_controller.go` returns nothing — confirms AC4's "no mode-conditional branch"
  claim.
- `research/build-vs-buy.md` independently reaches the same "extend the existing detector, no new
  package/library" conclusion as ADR-001 (4 of the 6 research docs agree; only
  `research/architecture.md` proposed the `monitorwait/` package).
- `kibitzer run session/detection --trigger batch` (`.claude/inspect.json` only configures the
  `go-primitive-obsession` advisory checker — no `architecture.components`/`dependency_rules`
  entries exist yet, so `component-deps`/`content-rules`/`naming-rules` don't run): the two
  functions this plan touches are **not** among kibitzer's existing findings —
  `matchWaitingForAgent` (`pattern_set.go:92-109`, 18 lines) isn't flagged, unlike the adjacent
  `MatchLines` (`pattern_set.go:114`, flagged `[long-function]` at 63 lines, untouched by this
  plan). `binaries/claude.go`'s pattern-table function is already flagged `[long-function]`
  (383 lines) pre-existing, but the plan's edit there is a same-sized regex-string swap inside an
  existing data literal, not new structure — consistent with the "one-line change to a
  large-but-stable file is fine" carve-out in `code-architecture-best-practices`, not a case of
  deepening a violation.

## Lens 1 — Structural Integrity

No SOLID/Clean-Architecture/DDD/testability findings. The change is confined to the detection
layer (`session/detection/`); no UI, proto, or persistence boundary is crossed. `matchWaitingForAgent`
and the regex table remain pure functions, testable in isolation exactly as today — the plan's own
test tasks (1.1.1c) exercise them directly via `pattern_set_test.go` and `bug_regression_test.go`
without needing new mocks/fixtures infrastructure.

## Lens 2 — Type-Level Design

- **Primitive obsession**: Plan explicitly considered and rejected a `ShellMonitorCount` value
  type (Pattern Decisions table), on the correct ground that the count is never compared against
  or mixed with a different kind of count anywhere in the codebase — the cross-entity-confusion
  risk `type-driven-design` newtypes exist to prevent doesn't apply here, and `SubagentCount`
  already plays this exact role for the short-form footer. Introducing a wrapper type here only
  would itself be the inconsistency. Correct call, not a violation.
- **Illegal states**: The zero-count guard added to `matchWaitingForAgent` (Task 1.1.1b) closes a
  real illegal-state gap — today's per-group `n > 0` check prevents a single zero group from
  contributing, but nothing stops a hypothetical future all-zero match from returning
  `ok=true, count=0` (a `StatusWaitingForAgent` with nothing to wait for). Mirroring
  `footerAgentCount`'s existing `total <= 0 → ok=false` guard is the right level of investment —
  this is a scoped runtime guard on a stateless parse function, not a candidate for a sum-type
  redesign.
- **Parse-at-boundary**: N/A at this scope — the boundary (`PatternSet.MatchLines` parsing raw PTY
  text into `DetectedStatus`/count) already exists and isn't restructured, only extended.

## Lens 3 — Pattern Selection

- **GoF/PoEAA fit**: The plan's central architectural decision (ADR-001) is explicitly *rejecting*
  `research/architecture.md`'s proposal to add a new `monitorwait/` package mirroring
  `ratelimit/`'s Detector/Manager/PTYConsumer machinery. This is the correct call, verified against
  actual code, not just asserted: the ratelimit package's structure earns its cost from real
  timer/lifecycle state (cooldown windows, recovery scheduling) that this signal has none of — it's
  a pure function of the current PTY tail, recomputed fresh every call, exactly like
  `footerAgentCount` already is for the short form. Building a second Detector/Manager/PTYConsumer
  surface here would be a GoF pattern applied where a two-line regex-and-summation fix does the
  job — precisely the anti-pattern `design-patterns`' decision guide warns against ("are patterns
  being added where a simple function would do").
- **API contract design**: `StatusPattern`, `matchWaitingForAgent`'s signature, and
  `InstanceStatusInfo.SubagentCount` are all unchanged — only the regex string and the internal
  summation loop change. Fully backward-compatible internal contract.
- **Build-vs-buy consistency**: Plan matches `research/build-vs-buy.md`'s recommendation exactly
  (minimal regex + table-driven tests, no new library, no new package) — verified above.

## Lens 4 — Tech Debt Trajectory

- Both entries in plan.md's Tech Debt Disposition table (stale "confirmed dead" comment;
  `matchWaitingForAgent`'s single-group read) are real, correctly identified, and sized
  appropriately as "Extend as-is" — each is a same-day, few-line fix folded into the story that
  already touches that code, not a deferred refactor. No hotspot from `research/architecture.md`
  or `kibitzer run` is silently left unaddressed: neither research doc uses churn/complexity
  hotspot framing for this area, and kibitzer's own findings on the touched files/functions were
  cross-checked above and are pre-existing, unrelated to, and not deepened by this diff.
- No case of "Extend as-is" deepening an existing violation was found — the one large pre-existing
  hotspot in the touched file (`claude.go`'s 383-line pattern-table function) gets a same-shape
  regex edit inside its existing data literal, not a new responsibility.

## Blockers

None.

## Concerns

None.

## Nitpicks

- `research/architecture.md`'s open question #3 (whether the indicator should coexist with a
  simultaneous `NEEDS_APPROVAL` chip, since a session could plausibly be both) is left unresolved
  by design — ADR-001/plan.md close it by pointing out `SubStatus` is a single enum slot today, so
  simultaneous display isn't possible without new UI work regardless. Requirements.md's AC2 only
  asks for "distinguishable from idle/needs-attention," which doesn't require simultaneity, and the
  plan's own Out of Scope section names this class of extension explicitly. Correctly scoped out,
  not a defect — flagging only so a future planner doesn't mistake the silence for an oversight.
