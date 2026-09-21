# Adversarial Review: app-scrollback-forwarding (iteration 2 — re-review of previously blocked items)

**Date**: 2026-09-14
**Verdict**: CONCERNS

## Re-reviewed items

- [x] Blocker 1 (client trigger contradiction) — RESOLVED — `plan.md` Story
  1.4.0 (lines 978-1148) adds an independent alt-screen trigger: a `wheel`
  listener (Task 1.4.0b) for desktop and a new outcome inside
  `useTerminalGestures`'s existing `SCROLLING` state (Task 1.4.0c) for mobile,
  explicitly justified against `research/architecture.md` §3's finding that
  `viewportY` never moves for an alt-screen pane. `design/ux.md`'s Surface 1
  interaction table (line 82) and its "Cross-cutting notes" section (lines
  529-544) now correctly distinguish plain-shell (`viewportY`-based, unchanged)
  from alt-screen (Story 1.4.0's wheel/gesture trigger) sessions, and
  explicitly flag the correction against the prior draft's stale unified-claim
  ("this superseded an earlier draft... per research/ux.md §4d's now-corrected
  claim"). Grepped `design/ux.md` for `viewportY`/"identical on both
  platforms" — no remaining stale reference found.

- [x] Blocker 2 (`StatusExecuting` treated as safe) — RESOLVED — Task 1.1.3a
  (`plan.md` lines 543-577) implements `AppScrollGate`'s status check as `==
  StatusIdle` exactly, an allowlist of one rather than a denylist, with an
  explicit statement that `StatusExecuting` is unsafe "same as
  StatusNeedsApproval/StatusInputRequired" and a note explaining why the
  session's own "gate unattended PTY writes on StatusIdle + StatusContext
  allowlist" precedent doesn't transfer to this different question. Story
  1.1.3's acceptance criteria (lines 532-537) add a dedicated
  Given/When/Then: `DetectedStatus=StatusExecuting` → `AppScrollGate` returns
  `(false, "unsafe detected status: executing")`, explicitly labeled a
  "regression guard for the architecture-review blocker." Citations to
  `research/pitfalls.md` §1 and `research/ux.md` §4c both check out — the
  latter (read directly, `research/ux.md:271-278`) states the fallback UX
  "should bias toward *not* forwarding while output is actively streaming...
  and only offering forwarded-scroll once the session is idle," matching the
  plan's implementation exactly.

- [x] Blocker 3 (`Blocked` outcome conflating three causes) — RESOLVED, with
  one residual narrow-window gap (see Concerns) — `ScrollBlockedReason`
  (proto enum, Task 1.3.3a, `plan.md` lines 897-906) genuinely threads end to
  end: `AppScrollGate`'s two distinguishable failure-reason strings →
  `Instance.ForwardScroll`'s three-way outcome mapping (Task 1.3.1c, lines
  751-761) → `AppScrollbackResponse.blocked_reason` on the wire → client
  `TerminalOutput.tsx` switches on `blocked_reason` to pick one of three
  distinct toast strings (Task 1.4.3a) → regression test asserting "a solo
  legacy-path user must never see the 'another viewer' message" (Task
  1.4.3b). `design/ux.md` Surface 3 (lines 139-249) carries the same three
  reasons through to concrete copy, an ambient/reactive signal pairing, and
  UX-AC-4's explicit non-regression criterion. This is not a declared-but-unused
  enum — it is load-bearing all the way to test assertions on literal copy
  strings.

## Blockers

None remaining. All three previously-blocked items are structurally resolved
with genuine wiring, not just renamed symptoms.

## Concerns

- [ ] **`ScrollBlockedReasonUnspecified` falls back to the `MULTIPLE_VIEWERS`
  copy, reintroducing a narrower version of Blocker 3's own defect.**
  `plan.md` Task 1.3.1c (lines 754-761) routes any gate failure that isn't
  the two viewer-count reasons (i.e. capability/alt-screen/status failures,
  including `StatusExecuting`) to `ScrollBlockedReasonUnspecified`, reasoning
  that "the client never surfaces those non-viewer gate failures as `Blocked`
  in practice" because the client's own eligibility check should prevent the
  attempt. Task 1.4.3a (lines 1268-1271) then has the client render this
  `UNSPECIFIED` case as the `MULTIPLE_VIEWERS` copy — "Can't browse Claude
  Code's history right now — another viewer is connected" — as "the
  least-wrong default." But this is exactly the shape of claim Blocker 3 was
  filed against: a `Blocked` toast asserting a cause that may not be true. The
  scenario is real, not hypothetical: the client's eligibility snapshot and
  the server's live gate evaluation are inherently racy (e.g. `DetectedStatus`
  flips from idle to `StatusExecuting` between the client deciding to attempt
  and the server evaluating `AppScrollGate`), which is precisely the class of
  check-then-act gap this same plan goes to considerable lengths to close
  elsewhere (`ScrollForwardAttachBarrier`). A user who hits this narrow
  window sees "another viewer is connected" when no second viewer exists —
  the same factually-false-toast problem, just with a smaller blast radius.
  The plan does document this as a deliberate tradeoff rather than hiding it,
  which is better than Blocker 3's original silence, but a generic
  "Scroll-forwarding is unavailable right now" fallback would close the gap
  entirely at effectively zero cost and would be more consistent with the
  principle the fix subagent just established. Recommend changing Task
  1.4.3a's `UNSPECIFIED` fallback copy before this ships, or explicitly
  accepting the residual risk with a one-line justification in the plan
  (currently justified only as "least-wrong," not compared against the
  cost-free generic alternative).

- [ ] (carried over, spot-checked, appears adequately addressed — lower
  confidence than the items above since verification was reading-only, not
  executed) Mid-forward PTY-write error path (Task 1.3.1c, lines 778-799):
  falls through to the unchanged tmux-native path on error, with `defer`-based
  lease/barrier cleanup — logic reads correctly on paper but has no test case
  cited in Task 1.3.1d beyond the three Given/When/Then already covering lease
  contention, gate short-circuit, and attach-barrier; the error-path branch
  itself doesn't have its own explicit acceptance-criterion/test pairing the
  way the other three do. Not a blocker, but worth a dedicated test case
  during implementation rather than relying on the prose description alone.

## Minors

- Task 1.4.4b's scope note (plan.md lines 1319-1328) explicitly flags itself
  as touching a path `requirements.md` names Out of Scope, and offers an
  escape hatch ("split this task into its own follow-up PR"). This is good
  practice — leaving it as a minor note rather than a concern since the plan
  already surfaces the tradeoff rather than burying it.
- The canary-to-alert wiring (Observability Plan, plan.md lines 133-145) reads
  as adequately addressed for the "unalerted canary" concern, but the
  described integration point (log-pattern-clustering tool threshold rule) was
  not independently verified to exist/support this shape of trend rule beyond
  the citation to `docs/how-to/debug-with-logs.md` — reasonable to trust given
  the specificity, but flagging as unverified-by-me rather than confirmed.
