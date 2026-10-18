# Validation: Parallel Fan-Out Plan

## Adversarial Review

### Finding 1 — Sequential creation hides latency behind a claim of "acceptable"

The plan says "N ≤ 5 so latency is acceptable" for sequential session creation, but each `CreateSession` call includes a `git worktree add` shell-out (or go-git equivalent). On a large repo, five sequential worktree adds can take 5–15 seconds. Users launching a fan-out from the UI will see a spinner for that duration with no incremental feedback.

**Mitigation:** Stream partial results from `FanOutSession` (server-streaming RPC) or create sessions in a background goroutine and return the `group_id` immediately, polling for readiness. The sequential-vs-parallel concern (worktree naming race) is real but can be addressed by including the variant index in the worktree directory name rather than relying on a pure UUID race.

**Verdict:** Plan needs a sentence clarifying the UX during creation (spinner with progress, not a blocking call) or a server-streaming variant.

### Finding 2 — "Pick winner" merge may silently succeed on an unfinished session

The plan says the server should "confirm winner session is in a terminal or review state." The existing session state machine has `Active`, `Stopped`, `Review`, `Archived`, `Error` states (inferred from the codebase). If "terminal or review state" is not precisely defined against the actual enum, an agent that paused mid-task could be mistakenly picked as a winner.

**Mitigation:** Define the allowed states explicitly in the plan as the enum values from `proto/session/v1/session.proto`'s `SessionStatus` (or `InstanceStatus`), not a prose description.

**Verdict:** Minor — add a table mapping allowed states for winner selection.

### Finding 3 — The omnibar detector pattern is under-specified

`detector.ts` is described as matching `"run N variants:"` or "similar prefix." Detectors in this codebase match specific patterns and must not false-positive on normal session creation prompts. An unspecified pattern is a future regression risk.

**Mitigation:** Specify the exact regex or prefix in the plan (e.g., `/^run (\d+) variants:/i`). This is a minor doc gap, not a blocker.

**Verdict:** Minor.

### Finding 4 — EstimateFanoutCost RPC is speculative

The plan introduces `EstimateFanoutCost` as a new RPC with no backing data. Stapler Squad's cost tracking (`token-cost-tracking` project plan, `server/services/quota_gate.go`) measures *actual* spend post-hoc, not pre-flight estimates. A per-session cost estimate would need model pricing data (input/output token rates) that the server does not currently have.

**Mitigation:** Simplify to a UI-side calculation: "N variants × your average session cost (shown from recent history)." Or drop the estimate to a static warning ("This will start N sessions simultaneously; monitor your usage in the Insights tab"). Do not ship a `EstimateFanoutCost` RPC without a clear data source for the estimate.

**Verdict:** Plan should remove the `EstimateFanoutCost` RPC and replace it with a simpler warning.

### Finding 5 — `noarchivedrevival` analyzer scope

When the `PickFanoutWinner` handler archives losers and later code auto-revives stopped sessions, the `noarchivedrevival` linter (`tools/lint/noarchivedrevival`) will flag any revival call in the six monitored files that lacks an archival check. The plan does not mention this constraint. The losing session archive action must use `ArchivedAt` correctly so the linter does not block CI.

**Mitigation:** Note in the implementation that loser archival must set `ArchivedAt` via the existing archive path (not just setting a status flag) and that the `noarchivedrevival` linter will enforce the guard at every revival site.

**Verdict:** Must-add note; not a plan-breaker but will cause CI failures if missed.

---

## Plan Adjustments After Adversarial Review

1. **FanOutSession RPC** should return a `group_id` immediately and create sessions asynchronously (or use server-streaming). Do not block the request until all worktrees are ready.
2. **Remove `EstimateFanoutCost` RPC.** Replace with a UI-side static warning: "Starting N variants will run N sessions simultaneously. Check the Insights tab for ongoing cost."
3. **Winner state validation** must reference the actual `SessionStatus` proto enum values, not prose.
4. **Detector pattern** must specify a concrete regex before implementation starts.
5. **Add a note** to the `PickFanoutWinner` implementation about `noarchivedrevival` compliance.

---

## Go / No-Go

**Go.** After the five adjustments above, this plan is sound. Core infrastructure (worktrees, ForkSession, CapacityMonitor, DiffViewer) is already in place; net-new surface area — two RPCs, one UI component, one route, omnibar 7-touchpoint registration — is well-bounded and consistent with established codebase patterns. Worktree naming race risk is mitigated by embedding the variant index in the directory name. No blocking unknowns remain.
