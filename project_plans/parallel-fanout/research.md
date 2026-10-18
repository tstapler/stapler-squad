# Research: Parallel Fan-Out

## Existing Infrastructure Inventory

### Session Forking (current capability)

`proto/session/v1/session.proto` (lines 284–287, 2266–2279) defines `ForkSession(ForkSessionRequest) → ForkSessionResponse`.  `ForkSessionRequest` takes `session_id`, `checkpoint_id`, and `title`. The implementation in `session/history_fork.go` (`ForkClaudeConversation`) copies the first N lines of a Claude conversation JSONL file into a new UUID-named file, then creates a new worktree for the forked session. Tests live in `session/instance_fork_test.go` and `server/services/session_service_fork_test.go`.

**Key observation:** `ForkSession` creates exactly one new session from an existing checkpoint. The fan-out feature is orthogonal: it creates N sessions *before* any agent has run, all sharing the same starting prompt.

### Session Creation

`CreateSession` in the session service accepts a `CreateSessionRequest` proto. Fields `fork_source_id` (field 19) and `fork_at_message` (field 20) are already part of it, showing prior art for "creation-time configuration". Adding variant metadata (variant index, group ID, model override) via new proto fields follows the same pattern.

### Worktree Isolation

Each session already creates a dedicated git worktree under `~/.stapler-squad/worktrees/`. Fan-out sessions each get their own worktree — no shared state, no new isolation mechanism needed.

### Quota and Capacity Guardrails

- `server/services/quota_gate.go` (`QuotaGate`) pauses the backlog controller when a rate-limit event is detected; its `NewQuotaGate` constructor takes a `config.QuotaConfig` getter.
- `server/services/capacity_monitor.go` (`CapacityMonitor`) polls provider limits and calls `stopForGuardrail` to terminate sessions that exceed cost/throughput budgets. Both act on sessions independently of how they were created, so fan-out sessions are automatically covered once they exist.

The pre-launch warning (AC-4) is an additional UX concern that needs a new "fan-out cost estimate" path: N × estimated_cost_per_session. The server can surface this through a new RPC or by extending the existing feature-flags endpoint.

### DiffViewer Component

`web-app/src/components/sessions/DiffViewer.tsx` provides per-session diff rendering and is already embedded in `SessionDetailView.tsx`. A comparison layout can render N `DiffViewer` instances in a side-by-side grid (CSS grid with auto columns) without duplicating the diff-fetching logic.

### Multi-Session Dashboard

The existing session list already renders many sessions simultaneously. The comparison view is a *filtered view* of the session list showing only the N sessions in a fan-out group, with a side-by-side layout and a "pick winner" action per column.

## Competitor Reference: Orca Parallel Worktrees

Orca (https://www.onorca.dev/docs/model/worktrees) fans a prompt across up to 5 agents simultaneously, each in its own worktree, with a comparison view and a merge action. The 5-variant cap is an explicit product decision. Stapler Squad's existing worktree isolation already matches Orca's isolation model.

## New Proto Fields Required

A fan-out group needs a shared identifier across all N sessions so the UI can fetch and group them:

```proto
// FanoutGroup links sessions created by the same parallel fan-out invocation.
message FanoutGroup {
  string group_id   = 1; // UUID, shared by all sessions in the fan-out
  int32  variant    = 2; // 0-based index within the group
  int32  total      = 3; // total variants in the group (≤ 5)
  string model_override = 4; // optional model/agent override for this variant
}
```

`CreateSessionRequest` gains an optional `FanoutGroup fanout_group = 21`.

A new `FanOutSession` RPC (or a batch extension of `CreateSession`) creates N sessions atomically from a single request:

```proto
rpc FanOutSession(FanOutSessionRequest) returns (FanOutSessionResponse) {}

message FanOutSessionRequest {
  string base_title   = 1;
  string prompt       = 2;
  int32  count        = 3;   // 2–5
  repeated string model_overrides = 4; // optional, len ≤ count
  // ... base session fields (repo path, program, etc.)
}
message FanOutSessionResponse {
  string group_id = 1;
  repeated string session_ids = 2;
}
```

## Winner Selection and Archive

"Pick the winner" = merge variant branch + archive the others.

Merge: the server already has `git merge` support (used in session creation flows). Archiving: sessions have `ArchivedAt` timestamps; setting it on losers is the existing archive path (see `noarchivedrevival.md` for the guard rules).

The merge action needs a new RPC: `PickFanoutWinner(group_id, winner_session_id)` that:
1. Merges `winner_session_id`'s branch into base.
2. Archives all other sessions in the group.
3. Deletes (or preserves per user preference) the losing worktrees.

## Omnibar / Session Creation Registry

Per `docs/reference/session-creation-registry.md`, new session creation modes require 7 touchpoints. The fan-out action is a new creation mode and must go through all 7.

Per `docs/reference/feature-testing-registry.md`, any new RPC needs a `// +api:` marker and a `// +feature:` marker in the relevant React file.

## Risk Areas

1. **Worktree naming collision**: N sessions created in rapid succession for the same repo may race on worktree directory name generation. Need to confirm `worktree_branch_exists_race` mitigations apply here.
2. **Quota gate interaction**: creating N sessions at once could spike the active session count and trip the capacity monitor's guardrail before any agent runs. The pre-launch warning must account for this.
3. **Merge conflicts**: if "pick the winner" tries to merge a variant whose base branch has diverged, the merge will fail. The UX needs a graceful error path.
4. **Omnibar registry touchpoints**: adding a fan-out creation mode means updating `types.ts`, `dispatch.ts`, `dispatch.test.ts`, `detector.ts`, `detector.test.ts`, and the 2 other touchpoints — easy to miss one under time pressure.
