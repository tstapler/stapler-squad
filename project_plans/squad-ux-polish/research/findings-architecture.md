# Findings: Architecture

**Subtopic**: Architecture — Data model design, storage, API shape, PR integration
**Date**: 2026-04-17
**Source**: Codebase analysis of `session/`, `server/`, `github/`, `proto/`, `session/ent/`

---

## Summary

The codebase has already migrated from flat JSON to SQLite via `ent` (entgo.io ORM). The storage layer has a `Repository` interface with concrete SQLite backing. Tags are a first-class many-to-many edge in the ent schema. The review queue is an in-memory reactive structure (not a persisted entity). GitHub operations are uniformly delegated to the `gh` CLI binary.

Five architectural questions are addressed below:

1. **Project entity**: First-class DB entity wins. Tags/Category alone are insufficient once project-level actions (aggregate stats, batch operations, project settings) are needed. The ent schema already has the machinery to add a `Project` entity with a `sessions` edge.

2. **Prompt library**: A dedicated JSON config file in `~/.stapler-squad/workspaces/{hash}/` is the right fit for 50–500 entries; it matches the `config.json` pattern already in place, avoids ent schema churn, and keeps prompt storage queryable without SQL. SQLite is also viable for larger sets.

3. **Review queue state machine**: The queue is already a filtered view of sessions with `AttentionReason` and `Priority` enums. "Ready for review" maps to adding `ReasonTaskComplete` items. "Reviewed → Merged" should remain session status fields (`GitHubPRState`), not new Status enum values — the existing `Stopped` terminal state plus `GitHubPRState = "merged"` covers it cleanly.

4. **Batch creation API**: A new `BatchCreateSessions` RPC with best-effort semantics is preferred over N client-side calls. No ACID atomicity is required or expected; partial success with per-item status is correct.

5. **PR creation**: Stay with `gh` CLI (`gh pr create`), following the established pattern across `github/client.go`. No direct REST API; no new OAuth scope.

---

## Options Surveyed

### Q1 — Project Entity Placement

**Option A: First-class `Project` entity (workspace → project → session)**
- New ent schema entity `Project` with fields: `ID`, `Name`, `Description`, `RepoPath`, `CreatedAt`, `UpdatedAt`
- Edge: `Project` → `[]Session` (one-to-many, sessions have at most one project)
- Migration: populate from existing `Category`/tags at startup if a matching group is detected; otherwise leave null

**Option B: Derived — tag-based (virtual grouping)**
- Designate a `project:<name>` tag convention; sessions belong to a project by holding this tag
- No new entity; no DB migration
- Project "entity" is synthesized on the server when listing sessions filtered by tag prefix

**Option C: Derived — same-repo detection**
- Auto-group sessions sharing a `MainRepoPath` as a "project"
- No new entity; grouping is a server-side view
- No user-controlled project name; relies on filesystem paths

---

### Q2 — Prompt Library Storage

**Option A: Extend `config.json`**
- Add `PromptLibrary []PromptEntry` to the `Config` struct in `config/config.go`
- Consistent with the session-defaults research (same pattern, same file)
- Atomic writes already implemented; file is small enough for 500 entries

**Option B: Separate `prompts.json` per workspace**
- New file `~/.stapler-squad/workspaces/{hash}/prompts.json`
- Keeps config.json focused; independent migration path
- Same Go struct → JSON pattern; negligible implementation difference from Option A

**Option C: ent/SQLite table**
- New `ent` schema entity `Prompt`
- Enables full-text search, frequency tracking, per-project scoping
- Heavier: requires ent schema extension + migration + go generate

---

### Q3 — Review Queue State Machine

**Option A: Filtered view (current pattern, extended)**
- "Ready for review" = session in queue with `ReasonTaskComplete`; no new Status value
- "Reviewed" = acked from queue (`LastAcknowledged` set); session status remains `Stopped`
- "Merged" = `GitHubPRState = "merged"`; detected by `PRStatusPoller`

**Option B: New Status values (`ReadyForReview`, `Merged`)**
- Extend the `Status` iota with `ReadyForReview = 7`, `Merged = 8`
- Surfaces cleanly in ListSessions filtering
- Breaking change to the status int mapping; DB migration required; frontend must handle new enum

**Option C: Separate `ReviewEntry` entity in DB**
- Decouple review lifecycle from session status
- Own entity with: `SessionID`, `State (pending|reviewed|merged)`, `PRNumber`, `CreatedAt`, `MergedAt`
- Most decoupled, but highest implementation cost; adds a join to every review queue query

---

### Q4 — Batch Session Creation API

**Option A: New `BatchCreateSessions` RPC**
```protobuf
rpc BatchCreateSessions(BatchCreateSessionsRequest)
    returns (BatchCreateSessionsResponse) {}

message BatchCreateSessionsRequest {
  repeated CreateSessionRequest sessions = 1;
  BatchOptions options = 2;
}

message BatchCreateSessionsResponse {
  repeated BatchCreateResult results = 1;
  int32 succeeded = 2;
  int32 failed = 3;
}

message BatchCreateResult {
  string title = 1;
  bool success = 2;
  string error = 3;
  Session session = 4; // populated on success
}
```

**Option B: Repeated client-side `CreateSession` calls**
- Client fires N independent RPCs in parallel
- No new backend code; just client orchestration
- No partial-success aggregation; UI must reconstruct state from N responses

**Option C: `CreateSession` with `batch_id` field + server fanout**
- Add optional `batch_id string` to existing `CreateSessionRequest`
- Server groups requests sharing a `batch_id` for coordinated status reporting
- Awkward: requires streaming or polling to observe batch progress

---

### Q5 — PR Creation Integration

**Option A: `gh` CLI via `exec.Command` (existing pattern)**
- `gh pr create --title ... --body ... --repo ...`
- Matches `github/client.go` style exactly
- Requires `gh` authenticated: already enforced by `CheckGHAuth()`

**Option B: GitHub REST API directly**
- `POST /repos/{owner}/{repo}/pulls`
- Requires OAuth token; `gh auth token` can extract it
- More portable (no `gh` binary dependency), but adds HTTP client + token management code

**Option C: GitHub GraphQL API**
- `createPullRequest` mutation
- Most powerful (can set reviewers, labels atomically)
- Highest complexity; GraphQL client needed

---

## Trade-off Matrix

### Q1 — Project Entity

| Axis | First-class Entity (A) | Tag Convention (B) | Repo-detection (C) |
|---|---|---|---|
| Backwards compat | Migration needed | Zero migration | Zero migration |
| Extensibility | High — add fields freely | Limited — conventions fragile | Very limited — no user labels |
| Implementation complexity | Medium (new ent schema + migration) | Low | Low |
| Storage overhead | Negligible | None | None |
| Project-level actions | Natural (aggregate queries) | Awkward (tag scan) | Awkward (path scan) |
| Handles multi-repo projects | Yes | Yes (tags are arbitrary) | No |

**Winner: A** — once project-level views and batch actions are needed, derived grouping breaks down. The ent ORM makes adding a `Project` entity a one-day task.

---

### Q2 — Prompt Library Storage

| Axis | Extend config.json (A) | Separate prompts.json (B) | SQLite table (C) |
|---|---|---|---|
| Backwards compat | Seamless | New file (no migration) | New ent migration |
| Extensibility | Limited — not indexed | Same | Full-text search, frequency |
| Implementation complexity | Lowest | Low | Medium |
| Storage overhead | config.json grows slightly | Separate small file | Trivial |
| Pattern match | Matches session-defaults ADR | Slightly cleaner separation | Matches review_queue approach |
| Supports 500 entries well | Yes (JSON is ~50KB) | Yes | Yes |

**Winner: B** — separate `prompts.json` keeps config.json focused on settings vs. content, and is the cleanest separation with minimal cost. SQLite is overkill until full-text search is needed.

---

### Q3 — Review Queue State Machine

| Axis | Filtered View (A) | New Status Values (B) | Separate Entity (C) |
|---|---|---|---|
| Backwards compat | Full | Breaking (int mapping) | Full |
| Extensibility | Medium | Low — status enum grows | High |
| Implementation complexity | Low | Medium | High |
| Storage overhead | None | None | New table |
| Fits existing patterns | Yes — queue is already reactive | Partial | No |
| PR merge tracking | Via GitHubPRState (already exists) | Via new Merged status | Via ReviewEntry.MergedAt |

**Winner: A** — the review queue is already a reactive filtered view. "Merged" is already tracked via `GitHubPRState` from the `PRStatusPoller`. Adding new Status enum values is a breaking migration with no concrete benefit over the existing fields.

---

### Q4 — Batch Create API

| Axis | New BatchCreateSessions RPC (A) | Client-side N calls (B) | Batch-id fanout (C) |
|---|---|---|---|
| Backwards compat | Additive | No backend changes | Additive |
| Atomicity | Best-effort, per-item status | No aggregation | Polling required |
| Implementation complexity | Medium (new RPC + handler) | Low (client only) | High (coordination logic) |
| Partial success visibility | First-class | DIY | Requires separate RPC |
| Throttling / back-pressure | Server-controlled | Client-controlled | N/A |

**Winner: A** — a `BatchCreateSessions` RPC keeps the backend in control of throttling (important: each session spawns a tmux process and creates a git worktree, both expensive). Partial success must be first-class at the protocol level.

---

### Q5 — PR Creation

| Axis | gh CLI (A) | REST API (B) | GraphQL (C) |
|---|---|---|---|
| Backwards compat | Full — existing pattern | No conflict | No conflict |
| Auth/token scope | `gh auth` already required | Need token extraction | Need token extraction |
| Implementation complexity | Lowest | Medium | Highest |
| Feature coverage | Sufficient (title, body, draft, base) | Full | Full + atomic multi-field |
| Dependency | `gh` binary required (already) | None extra | None extra |
| Failure transparency | stderr captured | HTTP status | GraphQL errors |

**Winner: A** — the existing `github/client.go` uses `gh` exclusively for all operations (GetPRInfo, MergePR, ClosePR, PostPRComment). There is no existing REST/GraphQL client. Adding `CreatePR` as a `gh pr create` call is 10 lines and zero new dependencies.

---

## Risk and Failure Modes

### Project Entity (Q1)

**Risk: Category → Project migration produces noise**
- Existing sessions have `Category` strings like "Work/Frontend" or "" (empty)
- Naive "one category = one project" mapping creates spurious projects
- Mitigation: Migration is optional/manual — only auto-migrate when user explicitly assigns; empty categories stay unassigned

**Risk: `ProjectID` FK introduced before all existing sessions are assigned**
- Sessions created before Project feature exists have null `ProjectID`
- Mitigation: `ProjectID` is nullable in schema; "no project" is a valid state; UI shows ungrouped sessions in a catch-all group

### Prompt Library (Q2)

**Risk: `prompts.json` gets large and read is slow**
- 500 entries at ~500 bytes each = ~250KB; trivially fast to read
- No risk at expected scale

**Risk: Concurrent writes from multiple stapler-squad instances**
- The workspace isolation (each workspace has its own directory) prevents this; prompts.json is per-workspace

### Review Queue State Machine (Q3)

**Risk: Session stuck in review queue after merge**
- `PRStatusPoller` updates `GitHubPRState` periodically; queue evaluator uses this field
- If poller is slow, queue item lingers briefly after merge
- Mitigation: On `MergePR` RPC success, immediately remove session from queue (already a queue.Remove call in the review queue manager)

**Risk: "TaskComplete" and "UncommittedChanges" both appear for the same session**
- A session can have code changes (diff present) but Claude has declared it done
- The queue prioritizes `ReasonApprovalPending > ReasonInputRequired > ReasonTaskComplete`
- This is acceptable — the review queue will surface it with the highest-priority reason

### Batch Session Creation (Q4)

**Risk: tmux name collision when creating N sessions simultaneously**
- Each session title must be unique; concurrent requests can race on the same title prefix
- Mitigation: The DB has a `UNIQUE` constraint on `title` (`field.String("title").Unique()`); the second concurrent create will fail with a clear error surfaced in `BatchCreateResult`

**Risk: Git worktree creation race (two sessions on same repo, same base branch)**
- `git worktree add` on the same repo from concurrent goroutines can fail
- Mitigation: The server should serialize worktree operations per repo path using a keyed mutex (or rely on ent upsert patterns already in place)

**Risk: Partial batch creates partial cleanup**
- 3 of 5 sessions succeed; then the user cancels — are the 3 tmux sessions and worktrees cleaned up?
- Mitigation: BatchCreateResult marks each item as succeeded/failed; client offers "cleanup partial batch" action using existing DeleteSession RPC

### PR Creation (Q5)

**Risk: Branch has no upstream remote**
- `gh pr create` requires the branch to be pushed to remote
- Mitigation: Server-side check: run `git push -u origin <branch>` before `gh pr create`; surface error in RPC response if push fails

**Risk: Branch protection rules block PR creation**
- Some repos require signed commits, specific base branches, or CI to pass before PR is created
- Mitigation: These are GitHub-enforced; the `gh pr create` will fail with a descriptive stderr; surface it in RPC error message

---

## Migration and Adoption Cost

### Q1 — Project Entity

1. Add `Project` entity to `session/ent/schema/project.go`:
   - Fields: `id` (uuid), `name`, `description`, `repo_path`, `created_at`, `updated_at`
   - Edge to `[]Session` (from session side: `project_id` nullable FK)
2. Add `project_id` nullable field to Session ent schema
3. Run `go generate ./session/ent/...`
4. Add `Project` to `storage.go` CRUD (thin wrapper around ent client)
5. Add proto message `Project` + RPCs: `CreateProject`, `ListProjects`, `UpdateProject`, `DeleteProject`, `AssignSessionsToProject`
6. Add UI: project picker in session card, project sidebar/view

**Estimated cost**: 2–3 days backend; 2–3 days UI. Migration of existing sessions is optional (null FK is valid).

### Q2 — Prompt Library

1. Define `PromptEntry` struct in `config/` or new `prompts/` package:
   ```go
   type PromptEntry struct {
     ID        string    `json:"id"`
     Text      string    `json:"text"`
     Label     string    `json:"label,omitempty"`
     Tags      []string  `json:"tags,omitempty"`
     UsedCount int       `json:"used_count"`
     LastUsed  time.Time `json:"last_used"`
     CreatedAt time.Time `json:"created_at"`
   }
   ```
2. New file `~/.stapler-squad/workspaces/{hash}/prompts.json`; atomic write via `os.Rename` (same pattern as `config.json`)
3. Add RPCs: `ListPrompts`, `UpsertPrompt`, `DeletePrompt`, `RecordPromptUsage`
4. UI: prompt picker in SessionWizard creation form

**Estimated cost**: 1 day backend; 1–2 days UI.

### Q3 — Review Queue (no migration needed)

The existing review queue already handles "task complete" sessions. The required changes are UI-only:
1. Add "Create PR" button to review queue items where `ReasonTaskComplete && no GitHubPRURL`
2. Add `CreatePR` RPC to `github_service.go`
3. Add "Mark as reviewed" ack that sets `LastAcknowledged` and transitions to next action

**Estimated cost**: 0.5 days backend (CreatePR RPC); 1 day UI.

### Q4 — Batch Create

1. Add `BatchCreateSessions` RPC to proto
2. Implement handler in `session_service.go` using a `sync.WaitGroup` / worker pool
3. UI: paste-tasks-to-sessions panel in new session modal

**Estimated cost**: 1 day backend; 2 days UI.

### Q5 — PR Creation

1. Add `CreatePR` function to `github/client.go`:
   ```go
   func CreatePR(owner, repo, head, base, title, body string, draft bool) (*PRInfo, error)
   // uses: gh pr create --repo {owner}/{repo} --head {head} --base {base} --title ... --body ... [--draft]
   ```
2. Add `CreatePR` RPC to proto; implement in `github_service.go`; call from review queue UI

**Estimated cost**: 0.5 days backend; included in Q3 UI estimate.

---

## Operational Concerns

**Concurrent batch creation and tmux process limits**
- Each session creates a tmux session; Linux default `max_proc` limits apply
- Recommendation: limit batch size to ≤20 sessions per request; server returns `ResourceExhausted` if limit exceeded

**prompts.json file ownership and permissions**
- File is per-workspace; same isolation guarantees as `sessions.json` and `config.json`
- No additional concern

**gh CLI version compatibility**
- `gh pr create` syntax has been stable since v1.0 (2020); `--json` output format used by `GetPRInfo` has been stable since v2.0
- Minimum required version: `gh` >= 2.x [TRAINING_ONLY — verify minimum version requirement]
- Recommendation: add `gh --version` check at startup alongside existing `CheckGHAuth()`

**ent schema migrations**
- `session/ent/migrate/schema.go` is auto-generated; adding `Project` requires `go generate ./session/ent/...` + running the ent atlas migration
- The project already uses `enttest` for tests; new schema changes will need test helpers updated

---

## Prior Art and Lessons Learned

### From session-defaults research (this codebase)
- The `Profile` concept in `session-defaults` research is structurally similar to "project" — a named preset. The distinction is that a Project accumulates sessions over time (mutable, relational), while a Profile is a template (immutable-ish, configuration). Both should coexist.
- The `ResolveDefaults` RPC pattern (merge global → directory → profile) is a good model for "apply project defaults at session creation time."

### From SQLite migration strategy doc (this codebase)
- The codebase explicitly migrated away from flat JSON for sessions due to 25MB file sizes and O(N) writes
- Prompt library at 50–500 entries is far below that threshold; JSON remains appropriate
- The `ent` ORM is already the canonical storage path; new persistent entities should use it

### From review_queue_manager.go (this codebase)
- The `ReactiveQueueManager` uses an event-bus + subscriber pattern; adding "PR created" and "session merged" as new event types follows naturally
- The `AttentionReason` enum has `ReasonTaskComplete` already; no new reason is needed for the review queue entry point

### From github/client.go (this codebase)
- All GitHub operations delegate to `gh` CLI; there is no direct REST/GraphQL client
- `CheckGHAuth()` is a synchronous check that runs before every operation
- The `gh pr create` pattern is well-tested in the approval classification suite (`pkg/classifier/classifier.go:819`)

### Industry pattern: batch RPCs
- Google API Design Guide recommends batch methods for performance over repeated calls [TRAINING_ONLY — verify]
- Stripe, GitHub REST v3, and similar APIs use partial-success batch semantics (return array of results with per-item success/error) rather than all-or-nothing
- ConnectRPC does not have a native "batch" streaming pattern; the standard Go approach is a request-response RPC with a `repeated` results field

---

## Open Questions

1. **Project scope**: Should `Project` be workspace-scoped (one per `~/.stapler-squad/workspaces/{hash}/`) or global? Most natural answer is workspace-scoped since each workspace corresponds to a running stapler-squad process. But users might want the same named project across workspaces if they switch directories.

2. **Prompt library scope**: Global (shared across all workspaces, stored in `~/.stapler-squad/prompts.json`) vs. per-workspace? Most prompts are likely task-specific; global sharing adds complexity for unclear gain. Recommendation: per-workspace, with a "copy to global" gesture as a future stretch.

3. **Batch create vs. session template**: The batch create feature and the template feature partially overlap. A template pre-fills fields; batch create creates multiple instances from a field set. Should the UI combine them (paste N task descriptions into a "template" form that creates N sessions), or keep them separate entry points?

4. **PR creation "push" precondition**: The server must push the branch before calling `gh pr create`. Should this be automatic (always push on "Create PR"), or should the UI warn the user that the branch will be pushed and ask for confirmation?

5. **Review queue "merged" cleanup**: After a session's PR is merged, should the session be automatically deleted (worktree removed) or kept until manually cleaned up? The current flow requires manual deletion. This could be a project-level setting.

---

## Recommendation

### Q1 — Project Entity Placement
**Use a first-class `Project` entity.** Add it to the ent schema as `session/ent/schema/project.go` with a nullable `project_id` FK on `Session`. Migrate existing sessions on a best-effort basis (auto-assign when a session's `Category` matches an existing project name; leave others unassigned). This approach gives project-level queries, settings, and batch actions without tag-convention fragility.

### Q2 — Prompt Library Storage
**Use a separate `prompts.json` file per workspace** at `~/.stapler-squad/workspaces/{hash}/prompts.json`. This matches the existing Go-struct-to-JSON pattern, avoids ent schema churn for a ~500-entry list, and keeps `config.json` focused on settings. Add `UsedCount` and `LastUsed` fields from day one to enable "sort by recent" and "sort by frequency" without a schema migration later.

### Q3 — Review Queue State Machine
**Keep the review queue as a filtered view; do not add new Status enum values.** "Ready for review" = `ReasonTaskComplete` in the existing queue. "Merged" = `GitHubPRState == "merged"` already tracked by `PRStatusPoller`. Add a `CreatePR` action in the review queue UI for sessions with `ReasonTaskComplete` and no `GitHubPRURL`. On `MergePR` success, call `queue.Remove(sessionID)` immediately (already the pattern in `handleApprovalResponse`).

### Q4 — Batch Session Creation API
**Add a new `BatchCreateSessions` RPC** with best-effort semantics (per-item success/error in the response). The server implementation uses a bounded worker pool (max 5 concurrent creates) to limit tmux and git worktree resource pressure. The existing `CreateSession` handler logic is reused per item. Partial failure is surfaced in `BatchCreateResult.Error` per item; the client shows which sessions succeeded and offers cleanup for failures.

### Q5 — PR Creation Integration
**Use `gh pr create` via `exec.Command`**, following the existing `github/client.go` pattern. Add `CreatePR(owner, repo, head, base, title, body string, draft bool) (*PRInfo, error)` to `github/client.go`. Add a `CreatePR` RPC to `SessionService`. The server ensures the branch is pushed to remote before calling `gh pr create` (add a `git push -u origin <branch>` step; surface push errors in the RPC response).

---

## Pending Web Searches

1. **"entgo.io ent ORM add entity migration existing SQLite database"** — verify the exact steps to add a new entity (Project) to an existing ent SQLite schema without wiping existing session data. Confirm `atlas` migration is the right tool here.

2. **"gh CLI minimum version pr create json output"** — confirm the minimum `gh` version that supports `gh pr create --json` output for parsing the created PR number. Current usage in `GetPRInfo` uses `gh pr view --json`; `CreatePR` will need similar output.

3. **"ConnectRPC batch RPC design pattern Go"** — verify whether ConnectRPC has any built-in support for batch request handling, or whether the correct pattern is a single RPC with `repeated` request/response fields.

4. **"GitHub API rate limits pr create"** — confirm rate limit considerations for batch PR creation if a user creates many sessions and tries to submit PRs for all of them at once. REST API limit is 5000 requests/hour for authenticated users [TRAINING_ONLY — verify].
