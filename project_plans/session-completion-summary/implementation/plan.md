# Implementation Plan: session-completion-summary

**Feature**: Auto-generate a persisted, structured markdown "completion summary"
(proof-of-work document) when a session naturally finishes — narrative, changed
files, approval breakdown, timeline, and token cost — viewable and copyable
from a new `Summary` tab in the session detail view.
**Date**: 2026-08-02
**Status**: Ready for implementation
**ADRs**: [ADR-001: Completion Summary — Generation Scope and Storage
Independence from the Session Row](../decisions/ADR-001-completion-summary-generation-scope-and-storage.md)

---

## Step 0.5 — CREATIVE pass: alternatives considered

**A. Lifecycle-listener-driven async snapshot+render, scoped to natural exit
(chosen).** A new `session.LifecycleListener` reacts only to `EventExited`
(session process exited on its own); it does a cheap in-memory snapshot then
spawns a goroutine for markdown + optional LLM narrative + DB persist.
*Strength*: satisfies AC-5 (non-blocking) by construction — the listener does
zero blocking work inline — and avoids the worktree-deletion race entirely,
because `EventExited` never runs `CleanupWorktree()`. *Weakness*: requires
treating the explicit `DeleteSession`/`stop_session` path as a structurally
different, best-effort, deterministic-only secondary path (see ADR-001) rather
than one uniform code path — more surface area to reason about.

**B. Fully synchronous inline generation in the stop/delete path (rejected).**
Generate the complete document — including the LLM narrative — synchronously,
inline, wherever a session is torn down (`Destroy()` or its callers), before
returning to the caller. *Strength*: one code path, no "generating" status, no
partial/failed states to design for. *Weakness*: directly violates AC-5 — the
`claude -p` narrative call already carries a documented 30s timeout in its
existing reference implementation (`GetWorktreeAISummary`,
`server/services/unfinished_work_service.go:288-369`); putting that on the
teardown critical path would delay every session's stop/delete RPC by up to
30s whenever the narrative call is slow or the CLI is missing.

**C. Decoupled background reconciliation poller (rejected).** A periodic
poller (like `ReviewQueuePoller`/`PRStatusPoller`) scans for recently-stopped
sessions with no summary yet and generates one, entirely decoupled from the
lifecycle event. *Strength*: no listener wiring, no timing coordination with
`Destroy()` — architecturally the simplest to bolt on, and it's a pattern
already proven in this codebase for other post-session background work.
*Weakness*: by the time a poller runs, the worktree may already be gone
(`CleanupWorktree()` runs synchronously inside `Destroy()`, before any poller
could intervene), so it can't satisfy AC-2's real diff-stat requirement
without inventing a second always-on diff cache; it also makes AC-1's "when a
session transitions to its terminal state, a document is generated" not truly
immediate, since generation now lags by a full poll interval.

**Chosen: A.** Rejected alternatives are also recorded per-component in the
Pattern Decisions table below.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `CompletionSummary` (ent entity) | The persisted row: session UUID, status, markdown, narrative flag, generation error, timestamps. | New file `session/ent/schema/completionsummary.go`. |
| `CompletionSummaryStatus` (Go string type + proto enum) | One of `not_generated` / `generating` / `ready` / `failed`. | Go consts in `session/completion_summary.go`; proto enum `CompletionSummaryStatus` in `types.proto`. |
| `CompletionSummarySnapshot` (Go struct) | The cheap, synchronously-captured facts (diff stats, changed files, approval counts, rule coverage, token totals, timeline) that feed markdown generation. | `session/completion_summary.go`. Captured before any blocking work. |
| `ApprovalDecisionBreakdown` (Go struct) | Counts: `AutoApproved`, `ManualApproved`, `Denied`, `ReviewQueueResolved`, `StillOpen`. | `StillOpen` is a distinct category per `research/pitfalls.md` §7 — never folded into approved/denied. |
| `ApprovalRuleCoverage` (Go struct) | `RulesFired []RuleCoverageEntry` + `UnmatchedCount int`. | Answers "which rules fired, how many requests were unmatched" (AC-2). |
| `RuleCoverageEntry` (Go struct) | One rule's `RuleID`, `RuleName`, and trigger `Count` for this session. | |
| `TokenSummary` (Go struct) | `TotalInput`, `TotalOutput`, `CacheCreation`, `CacheRead`, `EstimatedCostUSD`, `UnpricedModels`, `PricingStale`. | Mirrors `PricingTable.EstimateCost`/`ModelFamilyCost` output shape — never independently recomputed. |
| `GenerateCompletionSummaryMarkdown` (Go func) | `func(snap *CompletionSummarySnapshot, narrative string) string` — pure, deterministic markdown builder. | `session/completion_summary.go`. No I/O, fully unit-testable. |
| `BuildCompletionSummarySnapshot` (Go func) | `func(inst *session.Instance) (*session.CompletionSummarySnapshot, error)` — pulls from `DiffStats`, `AnalyticsStore`, `RulesStore`, `TokenStore`, `PricingTable`. | `server/services/completion_summary_service.go`. |
| `GenerateCompletionSummaryNarrative` (Go func) | `func(ctx, snap) (string, error)` — the `claude -p` CLI shellout for the "what was done" prose. | Same file; adapted from `GetWorktreeAISummary`. |
| `CompletionSummaryListener` (Go struct) | Implements `session.LifecycleListener`; reacts only to `EventExited`. | `server/services/completion_summary_service.go`, mirrors `autoArchiveListener`. |
| `snapshotCompletionSummaryBeforeTeardown` (Go func) | Cheap, synchronous, deterministic-only snapshot+persist called from `DeleteSession`/`stop_session` before the async `Destroy()` teardown. | Best-effort per ADR-001 — no narrative, no regenerate affordance. |
| `UpsertCompletionSummary` / `GetCompletionSummaryBySessionUUID` (Go methods) | `EntRepository` methods; upsert requires `--feature sql/upsert` codegen. | `session/ent_repository.go`. |
| `CompletionSummaryData` (Go struct) | Plain serializable mirror of the ent entity, following the `DiffStatsData` precedent. | `session/storage.go`. |
| `GetSessionCompletionSummary` / `RegenerateSessionCompletionSummary` (RPCs) | Read the persisted summary / trigger a fresh async regeneration for a live, `Stopped` session. | `proto/session/v1/session.proto`, handlers in `server/services/session_service.go`. |
| `SummaryTab` (React component) | New session-detail tab rendering the markdown + copy action, following `ArtifactsTab.tsx`'s shape. | `web-app/src/components/sessions/SummaryTab.tsx` + `.css.ts`. |
| `useCompletionSummary` (React hook) | Fetches/polls/regenerates the summary for a session, mirrors `useExportRules.ts`. | `web-app/src/lib/hooks/useCompletionSummary.ts`. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall generation architecture | Lifecycle-listener-driven async snapshot+render, scoped to `EventExited` only | Approach A (Step 0.5) | B: fully synchronous inline generation; C: decoupled background poller | B blocks teardown up to 30s (violates AC-5); C can't get fresh diff data once the worktree is gone and delays AC-1's "immediate" framing |
| Completion-summary persistence | Standalone ent entity keyed by plain indexed `session_uuid` string field, no FK edge to `Session` | PoEAA-adjacent (independent record, not a child of an aggregate root that may outlive it) | Pattern B (TEXT column on `Session`, as `research/architecture.md` recommended); Pattern A-with-required-edge (like `DiffStats`/`ReviewVerdict`) | `DeleteSession` (`session_service.go:2023`) deletes the `Session` row synchronously before/racing the async `Destroy()` teardown — a same-row column or required-FK child would be unwritable or cascade-deleted exactly when the explicit-stop path needs to persist (see ADR-001) |
| Generation trigger point | New `session.LifecycleListener` reacting only to `EventExited` where `Status` becomes `Stopped` | Existing extension point (`session/instance.go:93`), used today by `BacklogLifecycleListener` | New `SESSION_STATUS_COMPLETED` proto enum value; uniformly reacting to `EventExited, EventStopped` (like `instanceBacklogListener`) | A new enum value triggers the 7-touchpoint session-creation-mode registry churn for zero behavioral gain (`Stopped` is already fully terminal); uniform handling reintroduces the worktree/storage-row race documented in ADR-001 and `pitfalls.md` §1–2 |
| Snapshot vs. render split | Synchronous cheap snapshot (in-memory reads only) → `go` goroutine for markdown+narrative+persist | Concurrency-shape decision from `research/pitfalls.md` §2 | Do the whole pipeline (including the LLM call) inside the synchronous listener callback | `RegisterLifecycleListener`'s own doc comment (`session/instance_controller.go:110`) requires listeners to "return quickly (no long blocking operations)" — an inline LLM call violates that contract directly |
| Markdown generation (Go) | Plain `strings.Builder`/`fmt.Fprintf` per section | Existing precedent: `session/backlog_review.go`'s section-writer functions, `github/client.go`'s `GeneratePRPrompt` | `text/template` | `session/pipeline_engine.go:254-259` has an explicit in-repo precedent *against* templating engines for this exact problem shape ("no conditionals, no loops... to resist the 'templating engine' rabbit hole") |
| "What was done" narrative | Reuse the `claude -p` CLI shellout pattern (30s timeout, semaphore-bounded) from `GetWorktreeAISummary` | `server/services/unfinished_work_service.go:288-369` | New direct Anthropic HTTP API client | `anthropic_limits_client.go` only queries rate-limit headers, has no prompt/completion plumbing; the CLI shellout pattern already solves timeout/failure-isolation/concurrency-bounding, which a fresh HTTP client would have to re-solve |
| Approval-decision breakdown source | Query `AnalyticsStore`/`ClassificationAnalytics` by `session_id` (new `LoadForSession` method) | `server/services/analytics_store.go`, `session/ent/schema/classificationanalytics.go` (already has an indexed `session_id` field) | New per-session decision-event log ent entity (like `escape_event.go`'s pattern) | The data already exists, indexed, per-session — a new event log would duplicate already-collected data for no gain |
| Cost figure | Call `PricingTable.EstimateCost`/`ModelFamilyCost` directly; propagate `unpriced`/`IsStale()` signals into the markdown as a caveat | `session/tokens/pricing.go` | Compute cost independently from raw token counts | `research/pitfalls.md` §6: this codebase has already been bitten once (`95ed72d34`) by an independent cost-computation path silently showing `$0.00` for an unpriced model — a fourth independent path repeats that exact bug class |
| Frontend placement | New `summary` tab in `SessionDetailView.tsx`, disabled until `session.status === SessionStatus.STOPPED` | `ArtifactsTab.tsx` (session-derived, auto-populated, two-tier empty state) | Modal dialog | AC-3 requires persistent, revisitable state; modals in this codebase (`ResumeSessionModal`, `WorkspaceSwitchModal`) are for transient confirm/configure flows and require extra focus-trap work a tab avoids |
| Export mechanism | `copyToClipboard()` (execCommand-fallback-aware) + a visible "Copied" label flip | `web-app/src/lib/clipboard.ts`, `DetectionEventsPanel.tsx` | New "Insert into PR description"/"Post to issue" backend action | PR creation (#42) and issue-comment posting don't exist in this codebase yet — explicitly out of scope per requirements.md |
| Diff per-file list | `session/git/ops.go`'s `FileStatsBetween(repoPath, baseSHA, headSHA)` (go-git, typed, no shell-out) | `.claude/rules/prefer-go-git-over-subshells.md` | Parse `DiffStats.Content`'s `diff --git a/... b/...` headers | `FileStatsBetween` already exists, is already go-git-typed (per repo convention), and returns exactly the `FileStat{Path, Status, Additions, Deletions}` shape needed — parsing raw diff text would be strictly worse and duplicate logic |

---

## Migration Plan

- **Migration file**: none — this repo auto-migrates its SQLite schema via
  `client.Schema.Create(context.Background())` (`session/ent_repository.go:93`)
  at startup; there is no hand-written SQL migration directory for `session/ent`.
  The new `session/ent/schema/completionsummary.go` file is picked up
  automatically the next time `go run -mod=mod entgo.io/ent/cmd/ent generate
  --feature sql/upsert ./session/ent/schema` is run and the server restarts.
- **Reversibility**: additive-only (a brand-new table, `completion_summaries`,
  with a unique index on `session_uuid`) — no existing table or column is
  touched. Ent's auto-migrate never drops tables, so reverting the code change
  leaves an unused `completion_summaries` table behind harmlessly; no down
  script is needed.
- **Zero-downtime strategy**: N/A beyond "it's a new table" — no backfill, no
  dual-write period, no existing reads/writes are affected until the new code
  paths are wired in.
- **Rollback procedure**: revert the PR. The orphaned `completion_summaries`
  table can be left in place (harmless) or dropped manually via `sqlite3
  <db> "DROP TABLE completion_summaries;"` if desired — not required.

## Observability Plan

- **Logs**: `log.Info("completion summary generated", "session", inst.UUID,
  "status", status, "narrative_generated", ok)` on success;
  `log.Warn("completion summary generation failed", "session", inst.UUID,
  "err", err)` on failure — both in `generateAndPersistCompletionSummary`
  (mirrors the existing `log.Warn("failed to update diff stats", ...)` style
  in `GetSessionDiff`). `log.Warn` (not `Error`) for narrative-call failures
  specifically, since those degrade gracefully to the deterministic fallback
  and are not user-facing failures.
- **Metrics**: none new required — this is a low-frequency, per-session-stop
  operation (not a hot path); no existing metrics pipeline was found wired to
  session lifecycle events in the research, and adding one is disproportionate
  to a handful of stop events per session per day.
- **Alerts**: no new alerts required — failures are visible per-session via
  the `Summary` tab's error state (AC-5), not an operational/paging concern.

## Risk Control

- **Feature flag**: not gated — this is additive (new tab, new RPCs, new
  listener that no-ops unless `EventExited` fires with `Status == Stopped`);
  no existing behavior changes. Consistent with how `ArtifactsTab`/`VcsPanel`
  shipped.
- **Rollback procedure**: standard revert via PR close + revert commit. The
  `CompletionSummaryListener` registration in `wireCallbacks` is the only
  behavior-affecting change to an existing code path — removing it fully
  disables generation with no other side effects.
- **Staged rollout**: full rollout on merge — this is a personal, self-hosted,
  single-tenant tool (per `research/build-vs-buy.md` §4); there is no cohort
  or staged-rollout mechanism in this codebase to use.

## Unresolved Questions

None. All four Open Questions from `requirements.md` are resolved explicitly
below (see "Resolution of requirements.md's Open Questions").

### Resolution of requirements.md's Open Questions

1. **Stopped vs. Hibernated/Paused** — Generation triggers **only** on
   `EventExited` where `Status` transitions to `Stopped` (natural process
   exit). `Hibernated`/`Paused`/`Restoring` never trigger generation. The
   explicit `DeleteSession`/`stop_session` teardown path gets a separate,
   best-effort, deterministic-only synchronous snapshot (no narrative, no
   regenerate affordance) — see ADR-001. Justification:
   `session/instance.go:34`'s own doc comment calls `Stopped` "terminal state,
   cannot transition further," and `Resume()` (`session/instance.go:1364`)
   only accepts `Status == Paused`, never `Stopped` — so `Stopped` is
   genuinely irreversible under current code.
2. **Staleness after resume** — Not applicable under current code: since
   generation never fires for `Paused`/`Hibernated` sessions (only for
   genuinely-`Stopped` ones, which cannot resume today), there is no code path
   that can produce a stale summary. If a future feature adds resume-from-
   `Stopped`, ADR-001 flags that it must add explicit invalidation/
   regeneration at that time. No staleness banner/flag is built in this pass.
3. **Duration definition** — Wall-clock: `snapshot.Duration =
   snapshot.StoppedAt.Sub(snapshot.StartedAt)`, where `StartedAt` is the
   session's ent `created_at` field (`session/ent/schema/session.go:40`) —
   the session record's *original* creation time, not the current tmux
   process's start time, so a session that was restarted mid-life still
   reports its full logical lifespan (per `research/pitfalls.md` §11's
   explicit warning against using the current tmux process's start time).
   Active-time-only tracking (excluding paused/hibernated intervals) does not
   exist in this codebase and is out of scope — the markdown labels this
   field "Duration (wall-clock)" to avoid implying precision it doesn't have.
4. **Append-only vs. overwrite** — Overwrite: `CompletionSummary` is a
   single row per `session_uuid` (unique index), upserted on every
   generation/regeneration via ent's `OnConflictColumns("session_uuid")`
   (requiring `--feature sql/upsert` per `.claude/rules/ent-schema-generation.md`).
   No version history is kept. Justification: nothing in requirements.md asks
   for multiple versions, the UX research's "Regenerate" affordance (mirroring
   `VcsPanel.tsx`'s retry pattern) implies replacing, not appending, and
   keeping a single row avoids the storage-bloat concern already flagged for
   per-session blobs in `research/pitfalls.md` §9.

---

## Dependency Visualization

```
Phase 1: Data Model & Persistence
  1.1.1 ent schema (completionsummary.go)
     │
     ▼
  1.1.2 ent codegen (--feature sql/upsert) + go build
     │
     ▼
  1.1.3 EntRepository Upsert/Get methods
     │
     ▼
  1.1.4 Storage wrappers + CompletionSummaryData

Phase 2: Deterministic Snapshot & Markdown (depends on 1.1.4)
  2.1.1 Domain types (session/completion_summary.go)
     │
     ├──► 2.1.2 GenerateCompletionSummaryMarkdown (pure, unit-testable)
     │
     ├──► 2.2.1 AnalyticsStore.LoadForSession
     │        │
     │        ▼
     │    2.2.2 ApprovalRuleCoverage aggregation (needs RulesStore.All())
     │
     └──► 2.3.1 BuildCompletionSummarySnapshot (wires diff/tokens/approvals together)

Phase 3: Narrative, Lifecycle Wiring, RPCs (depends on Phase 2)
  3.1.1 GenerateCompletionSummaryNarrative (claude -p shellout)
     │
     ▼
  3.2.1 CompletionSummaryListener + generateAndPersistCompletionSummary
     │
     ▼
  3.2.2 Wire into wireCallbacks + server/dependencies.go startup loop
     │
  3.3.1 snapshotCompletionSummaryBeforeTeardown (DeleteSession + stop_session)
     │
  3.4.1 proto additions (types.proto enum/message, session.proto RPCs) → make proto-gen
     │
     ▼
  3.4.2 GetSessionCompletionSummary / RegenerateSessionCompletionSummary handlers

Phase 4: Frontend (depends on 3.4.2 + make proto-gen)
  4.1.1 useCompletionSummary hook
     │
     ▼
  4.2.1 SummaryTab.tsx + .css.ts
     │
     ▼
  4.2.2 Wire into SessionDetail.tsx tab union + SessionDetailView.tsx tab strip
     │
     ▼
  4.3.1 Feature registry entries + make registry-generate
     │
     ▼
  4.3.2 e2e test

Phase 5: Edge-case hardening (depends on all above)
  5.1.1 Empty-session markdown test (AC-6)
  5.1.2 Narrative-failure fallback test (AC-5)
  5.1.3 CompletionSummary-survives-Session-deletion test (ADR-001 proof)
```

---

## Phase 1: Data Model & Persistence

### Epic 1.1: `CompletionSummary` ent entity and repository access
**Goal**: A durable, independently-keyed store for the generated document that
survives both server restarts (AC-3) and `Session` row deletion (ADR-001).

#### Story 1.1.1: Add the `CompletionSummary` ent schema
**As a** backend developer, **I want** a dedicated ent entity for completion
summaries, **so that** the document persists independently of the `Session`
row's lifecycle.
**Acceptance Criteria**:
- A new ent schema compiles and generates a `completion_summaries` table with
  a unique index on `session_uuid` and no FK edge to `Session`.
  - *Given* the new schema file `session/ent/schema/completionsummary.go`,
    *When* `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert
    ./session/ent/schema` runs, *Then* `session/ent/completionsummary/` and
    `session/ent/completionsummary_create.go` (with `OnConflictColumns`
    support) are generated and `go build ./...` succeeds.
**Files**: `session/ent/schema/completionsummary.go` (new)

##### Task 1.1.1a: Write the schema file (~4 min)
- Create `session/ent/schema/completionsummary.go` with fields:
  `id` (`field.UUID("id", uuid.UUID{}).Default(uuid.New)`),
  `session_uuid` (`field.String("session_uuid").NotEmpty()`),
  `status` (`field.String("status").Default("not_generated")`),
  `markdown` (`field.Text("markdown").Optional()`),
  `narrative_generated` (`field.Bool("narrative_generated").Default(false)`),
  `generation_error` (`field.String("generation_error").Optional()`),
  `generated_at` (`field.Time("generated_at").Optional().Nillable()`),
  `created_at` (`field.Time("created_at").Default(time.Now).Immutable()`),
  `updated_at` (`field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now)`).
- `Edges()` returns `nil` — add a doc comment: `// Deliberately no edge back to
  Session — see ADR-001 (project_plans/session-completion-summary/decisions/ADR-001-...).
  A CompletionSummary must survive DeleteSession's synchronous deletion of the
  Session row.`
- `Indexes()`: `index.Fields("session_uuid").Unique()`.
- Files: `session/ent/schema/completionsummary.go`

##### Task 1.1.1b: Regenerate ent code and build (~3 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert
  ./session/ent/schema` (per `.claude/rules/ent-schema-generation.md` — the
  flag-less form silently breaks upsert methods needed in Task 1.1.3a).
- Run `go build ./...`; commit all generated files under `session/ent/`
  together with the schema file.
- Files: `session/ent/*generated*` (many, generated), `session/ent/schema/completionsummary.go`

#### Story 1.1.2: Repository read/write methods
**As a** backend developer, **I want** `EntRepository` methods to upsert and
fetch a completion summary by session UUID, **so that** callers never touch
the ent client directly.
**Acceptance Criteria**:
- Upserting twice for the same `session_uuid` updates the existing row rather
  than creating a duplicate.
  - *Given* a `CompletionSummary` row already exists with
    `session_uuid="abc-123"` and `status="ready"`, *When*
    `UpsertCompletionSummary(ctx, "abc-123", "failed", "", false, "claude CLI not found", nil)`
    is called, *Then* exactly one row still exists for `"abc-123"` and its
    `status` is now `"failed"`.
**Files**: `session/ent_repository.go`

##### Task 1.1.2a: `UpsertCompletionSummary` (~5 min)
- Add `func (r *EntRepository) UpsertCompletionSummary(ctx context.Context,
  sessionUUID, status, markdown string, narrativeGenerated bool,
  generationError string, generatedAt *time.Time) error` using
  `r.client.CompletionSummary.Create().SetSessionUUID(sessionUUID).
  SetStatus(status).SetMarkdown(markdown).SetNarrativeGenerated(narrativeGenerated).
  SetGenerationError(generationError).SetNillableGeneratedAt(generatedAt).
  OnConflictColumns(completionsummary.FieldSessionUUID).UpdateNewValues().Exec(ctx)`
  — mirrors `UpdateSessionArtifacts`'s error-wrapping style
  (`fmt.Errorf("UpsertCompletionSummary: %w", err)`).
- Files: `session/ent_repository.go`

##### Task 1.1.2b: `GetCompletionSummaryBySessionUUID` (~3 min)
- Add `func (r *EntRepository) GetCompletionSummaryBySessionUUID(ctx
  context.Context, sessionUUID string) (*ent.CompletionSummary, error)` using
  `r.client.CompletionSummary.Query().Where(completionsummary.SessionUUID(sessionUUID)).Only(ctx)`;
  return `(nil, nil)` (not an error) when `ent.IsNotFound(err)` — callers treat
  a nil result as `status = not_generated`.
- Files: `session/ent_repository.go`

#### Story 1.1.3: `Storage` wrappers and serializable data type
**As a** backend developer, **I want** `Storage`-level wrappers, **so that**
non-ent callers (`session/completion_summary.go`, `server/services/`) never
import `session/ent` directly — matching the existing `UpdateInstanceArtifacts`
pattern.
**Acceptance Criteria**:
- Calling `Storage.GetCompletionSummary` for a session with no summary yet
  returns a zero-value `CompletionSummaryData{Status: "not_generated"}`, not
  an error.
  - *Given* no `CompletionSummary` row exists for `session_uuid="xyz-999"`,
    *When* `storage.GetCompletionSummary("xyz-999")` is called, *Then* it
    returns `(&CompletionSummaryData{Status: "not_generated"}, nil)`.
**Files**: `session/storage.go`

##### Task 1.1.3a: `CompletionSummaryData` struct + wrappers (~5 min)
- Add `type CompletionSummaryData struct { SessionUUID, Status, Markdown,
  GenerationError string; NarrativeGenerated bool; GeneratedAt *time.Time }`
  near `DiffStatsData` (`session/storage.go:152`).
- Add `func (s *Storage) UpsertCompletionSummary(sessionUUID, status,
  markdown string, narrativeGenerated bool, generationError string,
  generatedAt *time.Time) error` and `func (s *Storage)
  GetCompletionSummary(sessionUUID string) (*CompletionSummaryData, error)`,
  delegating to the `EntRepository` methods from Story 1.1.2, converting
  `nil, nil` (not-found) into the zero-value default described above.
- Files: `session/storage.go`

---

## Phase 2: Deterministic Snapshot & Markdown Generation

### Epic 2.1: Domain types and pure markdown builder
**Goal**: A fully deterministic, LLM-free path from structured facts to a
valid markdown document — the AC-6 minimal-session guarantee and the AC-2
factual sections both depend on this being correct and hallucination-free.

#### Story 2.1.1: Domain types
**As a** backend developer, **I want** the snapshot/breakdown/coverage/token
types defined once, **so that** generation and tests share one vocabulary.
**Acceptance Criteria**:
- The package compiles with zero external (non-stdlib, non-`session/git`)
  imports in this file — it must stay a pure data/formatting module.
  - *Given* `session/completion_summary.go` after this task, *When* `go vet
    ./session/...` runs, *Then* the file imports only `strings`, `fmt`,
    `time`, and `github.com/tstapler/stapler-squad/session/git`.
**Files**: `session/completion_summary.go` (new)

##### Task 2.1.1a: Status consts + snapshot/breakdown structs (~5 min)
- Create `session/completion_summary.go` with `CompletionSummaryStatus`
  (string type + 4 consts: `CompletionSummaryNotGenerated`,
  `CompletionSummaryGenerating`, `CompletionSummaryReady`,
  `CompletionSummaryFailed`), `ApprovalDecisionBreakdown`,
  `RuleCoverageEntry`, `ApprovalRuleCoverage`, `TokenSummary`,
  `CompletionSummarySnapshot` (fields: `SessionUUID, SessionTitle string`;
  `StartedAt, StoppedAt time.Time`; `Duration time.Duration`; `Reason string`;
  `DiffAdded, DiffRemoved int`; `ChangedFiles []git.FileStat`; `DiffTruncated
  bool`; `Approvals ApprovalDecisionBreakdown`; `RuleCoverage
  ApprovalRuleCoverage`; `Tokens TokenSummary`) exactly as defined in the
  Domain Glossary above.
- Files: `session/completion_summary.go`

#### Story 2.1.2: `GenerateCompletionSummaryMarkdown`
**As a** backend developer, **I want** a pure function that renders a
snapshot into markdown, **so that** the same section-writer pattern used by
`backlog_review.go` produces a testable, hallucination-free document.
**Acceptance Criteria**:
- Given a fully-populated snapshot, produces sections in the order: status
  line → narrative → approval breakdown → files/diff-stat (linked, not
  inlined) → timeline → token cost, per `research/ux.md` §2's progressive-
  disclosure ordering.
  - *Given* `snap := &CompletionSummarySnapshot{SessionUUID: "abc-123",
    SessionTitle: "fix-login-bug", DiffAdded: 42, DiffRemoved: 8,
    ChangedFiles: []git.FileStat{{Path: "server/auth.go", Status: "modified",
    Additions: 30, Deletions: 5}}, Approvals:
    ApprovalDecisionBreakdown{AutoApproved: 3, ManualApproved: 1, Denied: 1},
    Tokens: TokenSummary{TotalInput: 12000, TotalOutput: 3400,
    EstimatedCostUSD: 0.18}}`, *When*
    `GenerateCompletionSummaryMarkdown(snap, "Fixed a login race condition.")`
    is called, *Then* the returned string contains, in order, a `## Approvals`
    section with `Auto-approved: 3`, a `## Files Changed` section listing
    `server/auth.go` with `+30/-5`, and a `## Token Usage` section with
    `$0.18`.
- Given a minimal/empty snapshot (AC-6), every section still renders with an
  explicit empty-state line rather than being omitted.
  - *Given* `snap := &CompletionSummarySnapshot{SessionUUID: "xyz-999",
    SessionTitle: "quick-test"}` (all counts zero, `ChangedFiles: nil`),
    *When* `GenerateCompletionSummaryMarkdown(snap, "")` is called, *Then*
    the output contains `"No files changed."` under `## Files Changed` and
    `"No approval decisions were required."` under `## Approvals`, and the
    function returns a non-empty string (never `""`, never an error).
- Diff content is never inlined — only the stat + a reference.
  - *Given* `snap.ChangedFiles` has 40 entries and `snap.DiffTruncated =
    true`, *When* the markdown is generated, *Then* the `## Files Changed`
    section lists the per-file stat table and ends with a line referencing
    "view the full diff in the session's VCS tab" — no raw diff hunk text
    appears anywhere in the output.
**Files**: `session/completion_summary.go`

##### Task 2.1.2a: Status/narrative/approval sections (~5 min)
- Add `writeCompletionSummaryHeader`, `writeNarrativeSection`,
  `writeApprovalSection` helper funcs (each takes `*strings.Builder` +
  relevant snapshot fields, following `backlog_review.go`'s
  `writeItemContextSection` shape: guard on empty input, still emit an
  explicit "none" line rather than skipping the heading).
- `writeApprovalSection` renders all 5 `ApprovalDecisionBreakdown` fields as
  labeled lines (never color-only, per `research/ux.md` §3's WCAG note —
  N/A for markdown text itself, but keeps parity with the frontend rendering
  of the same data) plus the `ApprovalRuleCoverage` "N rules fired, M
  requests unmatched" line.
- Files: `session/completion_summary.go`

##### Task 2.1.2b: Files/timeline/token sections + top-level func (~5 min)
- Add `writeFilesChangedSection` (per-file `git.FileStat` table via GFM
  pipe-table syntax, stat line `+Added/-Removed`, no diff content, ends with
  the "view full diff" reference line), `writeTimelineSection` (formats
  `StartedAt`, `StoppedAt`, `Duration` — label the duration line "Duration
  (wall-clock)" per Open Question 3's resolution), `writeTokenSection`
  (formats `TokenSummary`, appending a caveat line when `UnpricedModels` is
  non-empty or `PricingStale` is true, e.g. `"$12.40 (2 turns using an
  unpriced model, cost may be understated)"` per `research/pitfalls.md` §6).
- Add `func GenerateCompletionSummaryMarkdown(snap *CompletionSummarySnapshot,
  narrative string) string` that composes all sections via one
  `strings.Builder` in the order specified in the story's AC.
- Files: `session/completion_summary.go`

##### Task 2.1.2c: Unit tests for the markdown builder (~5 min)
- Create `session/completion_summary_test.go`, table-driven, covering both
  Given-When-Then examples above (full snapshot, empty snapshot) plus: a
  snapshot with `Approvals.StillOpen > 0` renders a distinct "still open"
  line (not folded into approved/denied, per `research/pitfalls.md` §7).
- Run `go test ./session/... -run TestGenerateCompletionSummaryMarkdown`.
- Files: `session/completion_summary_test.go` (new)

### Epic 2.2: Approval-decision breakdown and rule coverage
**Goal**: Turn already-persisted `AnalyticsStore` events into the counts and
coverage AC-2 asks for — no new event-logging code.

#### Story 2.2.1: `AnalyticsStore.LoadForSession`
**As a** backend developer, **I want** to query `ClassificationAnalytics` by
session, **so that** the snapshot builder doesn't need a new tracking table.
**Acceptance Criteria**:
- Returns only entries for the requested session, in chronological order.
  - *Given* the `classification_analytics` table has 3 rows with
    `session_id="abc-123"` and 2 rows with `session_id="other-session"`,
    *When* `analyticsStore.LoadForSession(ctx, "abc-123")` is called, *Then*
    it returns exactly 3 `AnalyticsEntry` values, all with `SessionID ==
    "abc-123"`, ordered by `Timestamp` ascending.
**Files**: `server/services/analytics_store.go`

##### Task 2.2.1a: Implement `LoadForSession` (~4 min)
- Add `func (s *AnalyticsStore) LoadForSession(ctx context.Context,
  sessionID string) ([]AnalyticsEntry, error)`, querying via
  `s.storage`'s underlying ent client filtered on
  `classificationanalytics.SessionID(sessionID)`, ordered by
  `classificationanalytics.FieldCreatedAt` — follow the existing
  `LoadProgramWindow` (`analytics_store.go:258`) method's shape for how it
  reaches the ent client through `s.storage`.
- Files: `server/services/analytics_store.go`

#### Story 2.2.2: Aggregate into `ApprovalDecisionBreakdown` + `ApprovalRuleCoverage`
**As a** backend developer, **I want** a pure aggregation function over
`[]AnalyticsEntry` + the configured rule set, **so that** counts and
"unmatched" coverage are computed once, correctly.
**Acceptance Criteria**:
- Each `AnalyticsEntry.Decision` value maps to exactly one breakdown bucket;
  unresolved review-queue items are counted separately as `StillOpen`.
  - *Given* entries with `Decision` values
    `["auto_allow","auto_allow","manual_allow","manual_deny","escalate"]`
    (the last one still open per `server/services/approval_store.go`'s
    pending-request state), *When* the aggregation runs, *Then*
    `ApprovalDecisionBreakdown{AutoApproved: 2, ManualApproved: 1, Denied: 1,
    StillOpen: 1}` (interpreting a still-pending `"escalate"` — i.e. one with
    no terminal resolution recorded in `ApprovalStore` — as `StillOpen`, not
    `ReviewQueueResolved`).
- Rule coverage counts rules that fired and flags unmatched requests.
  - *Given* 5 entries where `RuleID` is `"rule-1"` for 3 of them, `""` for 2
    (i.e. unmatched/escalated with no rule), and `rulesStore.All()` returns
    `[{ID:"rule-1",Name:"auto-allow-git-status"},{ID:"rule-2",Name:"deny-rm-rf"}]`,
    *When* the aggregation runs, *Then* `ApprovalRuleCoverage.RulesFired ==
    [{RuleID:"rule-1", RuleName:"auto-allow-git-status", Count:3}]` and
    `UnmatchedCount == 2` (rule-2 is configured but never fired — reported
    separately, not counted as "unmatched requests"; only the 2 no-rule
    entries count toward `UnmatchedCount`).
**Files**: `server/services/completion_summary_service.go` (new)

##### Task 2.2.2a: `aggregateApprovalBreakdown` (~5 min)
- In new file `server/services/completion_summary_service.go`, add `func
  aggregateApprovalBreakdown(entries []AnalyticsEntry, pendingSessionIDs
  map[string]bool) session.ApprovalDecisionBreakdown` — cross-reference
  `entries` against `s.approvalStore`'s still-pending requests (via
  `ApprovalStore`'s existing pending-lookup, e.g. filtering
  `ApprovalStore.Pending()`-style state by session) to distinguish
  `StillOpen` from `ReviewQueueResolved`; map `"auto_allow"→AutoApproved`,
  `"auto_deny"→Denied`, `"manual_allow"→ManualApproved`,
  `"manual_deny"→Denied`, `"escalate"`-with-later-resolution
  `→ReviewQueueResolved`, `"escalate"`-still-pending `→StillOpen`.
- Files: `server/services/completion_summary_service.go`

##### Task 2.2.2b: `aggregateRuleCoverage` (~4 min)
- Add `func aggregateRuleCoverage(entries []AnalyticsEntry, allRules
  []RuleSpec) session.ApprovalRuleCoverage` — group entries by non-empty
  `RuleID` into `RuleCoverageEntry` counts; `UnmatchedCount` = count of
  entries with empty `RuleID`.
- Files: `server/services/completion_summary_service.go`

##### Task 2.2.2c: Unit tests (~5 min)
- Create `server/services/completion_summary_service_test.go` covering both
  Given-When-Then examples from Story 2.2.2.
- Run `go test ./server/services/... -run TestAggregate`.
- Files: `server/services/completion_summary_service_test.go` (new)

### Epic 2.3: Wire diff + tokens + approvals into one snapshot
**Goal**: `BuildCompletionSummarySnapshot` is the single place that reaches
into `Instance`, `TokenStore`, `AnalyticsStore`, and `RulesStore` — everything
downstream only sees the resulting struct.

#### Story 2.3.1: `BuildCompletionSummarySnapshot`
**As a** backend developer, **I want** one function that assembles a full
snapshot from a live `*session.Instance`, **so that** the listener (Phase 3)
has a single, simple call to make.
**Acceptance Criteria**:
- Reads diff stats from the already-cached value, never re-shells `git diff`.
  - *Given* `inst.GetDiffStats()` (`session/instance_worktree.go:284`)
    returns `&git.DiffStats{Added: 42, Removed: 8}` from its in-memory cache,
    *When* `BuildCompletionSummarySnapshot(inst)` is called, *Then* the
    resulting snapshot's `DiffAdded == 42` and `DiffRemoved == 8` without any
    new `git diff` subprocess being spawned (verified by the test asserting
    `git.FileStatsBetween` is the only git-invoking call made, and only when
    the worktree path is non-empty).
- Reads token totals via `TokenStore.GetByUUID`, cost via `PricingTable`.
  - *Given* `tokenStore.GetByUUID("abc-123")` returns a `*tokens.ParseResult`
    with `TotalInput: 12000, TotalOutput: 3400`, *When*
    `BuildCompletionSummarySnapshot` runs, *Then* `snapshot.Tokens.TotalInput
    == 12000` and `snapshot.Tokens.EstimatedCostUSD` equals
    `pricingTable.EstimateCost(result)`'s returned cost exactly (not
    independently recomputed).
**Files**: `server/services/completion_summary_service.go`

##### Task 2.3.1a: Implement the snapshot builder (~5 min)
- Add `func (s *SessionService) BuildCompletionSummarySnapshot(inst
  *session.Instance) (*session.CompletionSummarySnapshot, error)`:
  `SessionUUID/SessionTitle` from `inst`; `StartedAt` from `inst.CreatedAt`
  (`session/instance.go:125`); `StoppedAt = time.Now()`; `Duration =
  StoppedAt.Sub(StartedAt)`; diff fields from `inst.GetDiffStats()`;
  `ChangedFiles` from `git.FileStatsBetween(inst.Path, inst.BaseCommitSHA,
  "HEAD")` guarded by `if inst.GetGitWorktree() != nil` (best-effort — log
  and continue with `nil` on error, never fail the whole snapshot per AC-6);
  `Tokens` from `s.tokenStore.GetByUUID(inst.UUID)` +
  `s.pricingTable.EstimateCost(...)`; `Approvals`/`RuleCoverage` from
  `s.analyticsStore.LoadForSession(ctx, inst.UUID)` (Story 2.2.1) fed through
  `aggregateApprovalBreakdown`/`aggregateRuleCoverage` (Story 2.2.2).
- Files: `server/services/completion_summary_service.go`

##### Task 2.3.1b: Unit test with fakes (~5 min)
- Add a test to `server/services/completion_summary_service_test.go` using a
  fake/in-memory `TokenStore`/`AnalyticsStore` (or the real ones backed by a
  temp SQLite file, matching existing test setup in
  `session/ent_repository_test.go`) to cover both Given-When-Then examples.
- Files: `server/services/completion_summary_service_test.go`

---

## Phase 3: Narrative, Lifecycle Wiring, and RPCs

### Epic 3.1: LLM narrative generation
**Goal**: The one non-deterministic section, isolated and fail-safe.

#### Story 3.1.1: `GenerateCompletionSummaryNarrative`
**As a** backend developer, **I want** a bounded, isolated `claude -p` call
for the "what was done" prose, **so that** narrative failures never take down
the rest of the document (AC-5).
**Acceptance Criteria**:
- Times out at 30s and returns a typed error distinguishable from other
  failures.
  - *Given* the `claude` CLI hangs indefinitely, *When*
    `GenerateCompletionSummaryNarrative(ctx, snap)` is called, *Then* it
    returns `("", err)` where `errors.Is(err, context.DeadlineExceeded)` is
    true, within 30s ± normal scheduling slack — never blocking longer.
- Feeds only structured facts (diff stat + file list + approval counts), not
  raw scrollback, into the prompt.
  - *Given* `snap.ChangedFiles` and `snap.Approvals` are populated, *When*
    the function builds its prompt, *Then* the prompt string contains the
    file list and approval counts but does not contain any raw JSONL
    transcript content — reducing hallucination/prompt-injection surface per
    `research/pitfalls.md` §5.
**Files**: `server/services/completion_summary_service.go`

##### Task 3.1.1a: Implement the shellout (~5 min)
- Add `func (s *SessionService) GenerateCompletionSummaryNarrative(ctx
  context.Context, snap *session.CompletionSummarySnapshot) (string, error)`,
  adapted directly from `GetWorktreeAISummary`
  (`server/services/unfinished_work_service.go:288-369`): `exec.LookPath("claude")`,
  30s `context.WithTimeout`, `aiSemaphore`-bounded (reuse the existing
  package-level `aiSemaphore` var, `unfinished_work_service.go:27`), prompt =
  "Summarize what this agent session did in 2-4 sentences for someone
  reviewing the change, given: <file list>, <approval counts>." piped via
  `claudeCmd.Stdin`.
- No caching (unlike `GetWorktreeAISummary` — a completion summary is
  generated once per terminal transition, not repeatedly for a live diff).
- Files: `server/services/completion_summary_service.go`

##### Task 3.1.1b: Unit test with a fake `claude` binary (~4 min)
- Add a test that stubs `PATH` to point at a fake `claude` script (matching
  any existing pattern for testing `GetWorktreeAISummary`, or a small
  temp-script fixture) verifying both the timeout and prompt-content
  Given-When-Then examples.
- Files: `server/services/completion_summary_service_test.go`

### Epic 3.2: Lifecycle listener for the natural-exit path
**Goal**: Wire generation into the existing `EventExited` extension point,
per ADR-001 — asynchronously, satisfying AC-1 and AC-5 together.

#### Story 3.2.1: `CompletionSummaryListener` + async pipeline
**As a** backend developer, **I want** a `LifecycleListener` that kicks off
generation without blocking, **so that** AC-1 (automatic) and AC-5
(non-blocking) both hold.
**Acceptance Criteria**:
- Ignores every event except `EventExited` with `Status == Stopped`.
  - *Given* a `CompletionSummaryListener` registered on `inst`, *When*
    `inst.FireLifecycleEventForTest(session.EventStarted, "")` is called,
    *Then* no goroutine is spawned and no `CompletionSummary` row is written
    (verified via a spy `SessionService`/mock `Storage`).
  - *Given* the same listener, *When*
    `inst.FireLifecycleEventForTest(session.EventExited, "")` is called with
    `inst.Snapshot().Status == session.Stopped`, *Then*
    `generateAndPersistCompletionSummary` is invoked exactly once, in a
    goroutine (the test's `OnLifecycleEvent` call itself returns in
    microseconds, well before the fake narrative call would complete).
**Files**: `server/services/completion_summary_service.go`

##### Task 3.2.1a: `CompletionSummaryListener` type (~4 min)
- Add `type CompletionSummaryListener struct { svc *SessionService; inst
  *session.Instance }` and `func (l *CompletionSummaryListener)
  OnLifecycleEvent(event session.LifecycleEvent, reason string)`: `if event
  != session.EventExited || l.inst.Snapshot().Status != session.Stopped {
  return }`; then `go l.svc.generateAndPersistCompletionSummary(l.inst,
  reason)` — mirrors `autoArchiveListener.OnLifecycleEvent`
  (`session_service.go:3808-3812`) exactly.
- Files: `server/services/completion_summary_service.go`

##### Task 3.2.1b: `generateAndPersistCompletionSummary` orchestration (~5 min)
- Add `func (s *SessionService) generateAndPersistCompletionSummary(inst
  *session.Instance, reason string)`: build snapshot (Story 2.3.1); persist
  `status=generating` immediately (so a concurrent `GetSessionCompletionSummary`
  read sees "in progress" rather than stale `not_generated`); call
  `GenerateCompletionSummaryNarrative` (Story 3.1.1) with its own bounded
  context; build markdown via `GenerateCompletionSummaryMarkdown` — pass the
  narrative string on success, `""` on any narrative error/timeout (never
  block the whole document on it); persist final `status=ready` (with
  `narrative_generated` reflecting whether the LLM call succeeded) — only use
  `status=failed` if the *deterministic* snapshot-build or the final
  persist-write itself errors, matching AC-5's "generation must not block or
  delay session teardown ... failure is visible rather than silent" (the
  narrative failing alone must never produce `status=failed`, only
  `narrative_generated=false`).
- Files: `server/services/completion_summary_service.go`

##### Task 3.2.1c: Unit tests for both AC examples (~5 min)
- Add tests to `server/services/completion_summary_service_test.go` covering
  the "ignores EventStarted" and "spawns goroutine on qualifying EventExited"
  examples using `FireLifecycleEventForTest`.
- Files: `server/services/completion_summary_service_test.go`

#### Story 3.2.2: Wire the listener into existing registration points
**As a** backend developer, **I want** every instance (newly created and
restored-on-restart) to have the listener registered, **so that** no session
silently skips generation.
**Acceptance Criteria**:
- Every code path that already calls `wireCallbacks`/`WireToInstance` for
  `BacklogLifecycleListener` also registers `CompletionSummaryListener`.
  - *Given* a session created via `CreateDirectorySession`, *When*
    `s.wireCallbacks(instance)` runs, *Then*
    `instance.lifecycleListeners` (verified via a package-internal test
    helper or observed side effect) includes one `*CompletionSummaryListener`
    for that instance.
**Files**: `server/services/session_service.go`, `server/dependencies.go`

##### Task 3.2.2a: Register inside `wireCallbacks` (~3 min)
- In `wireCallbacks` (`session_service.go:956`), add
  `inst.RegisterLifecycleListener(&CompletionSummaryListener{svc: s, inst: inst})`
  alongside the existing `autoArchiveListener`/`sessionExitedPublisher`
  registrations.
- Files: `server/services/session_service.go`

##### Task 3.2.2b: Verify startup-restore path covers it (~3 min)
- Confirm `wireCallbacks` is called for every instance in the
  `loadInstancesWithWiring` restore loop (`server/dependencies.go`, the loop
  around lines 556-566 that calls `backlogLifecycleListener.WireToInstance(inst)`);
  since `wireCallbacks` is a `SessionService` method already called during
  normal instance creation/loading, confirm (via a quick grep + one
  integration test) that server-restart-restored instances also get
  `wireCallbacks` invoked — if not, add the equivalent
  `s.wireCallbacks(inst)` call to that loop.
- Files: `server/dependencies.go`

### Epic 3.3: Best-effort snapshot for the explicit stop/delete path
**Goal**: Per ADR-001, give `DeleteSession`/`stop_session` a synchronous,
deterministic-only snapshot before the row/worktree disappear.

#### Story 3.3.1: `snapshotCompletionSummaryBeforeTeardown`
**As a** backend developer, **I want** a cheap, synchronous pre-teardown
snapshot for explicit session termination, **so that** a document exists at
the moment of deletion even though it won't be retrievable afterward.
**Acceptance Criteria**:
- Called before the worktree is torn down, contains no LLM call.
  - *Given* a live `*session.Instance` about to be destroyed via
    `DeleteSession`, *When*
    `s.snapshotCompletionSummaryBeforeTeardown(inst)` runs, *Then* it
    completes in under 200ms in a test with a small diff (no `claude -p`
    subprocess is spawned — verified by asserting `exec.LookPath`/`claude`
    is never invoked in this code path), and a `CompletionSummary` row with
    `session_uuid == inst.UUID` and `narrative_generated == false` exists
    immediately after the call returns.
**Files**: `server/services/completion_summary_service.go`, `server/services/session_service.go`, `server/mcp/tools_lifecycle.go`

##### Task 3.3.1a: Implement the synchronous helper (~4 min)
- Add `func (s *SessionService) snapshotCompletionSummaryBeforeTeardown(inst
  *session.Instance)`: build snapshot (Story 2.3.1, reusing the same
  builder), render markdown with `narrative=""` (Story 2.1.2), persist
  `status=ready, narrative_generated=false` — all synchronous, no goroutine,
  no LLM call. Log-and-continue on any internal error (never propagate — this
  must not block the caller's teardown).
- Files: `server/services/completion_summary_service.go`

##### Task 3.3.1b: Call it from `DeleteSession` before teardown (~3 min)
- In `DeleteSession` (`session_service.go`, immediately after
  `s.removeFromAllPollers(sessionTitle)` and before the `go func(){
  inst.Destroy() }()` line), add: `if inst := s.FindLiveInstance(sessionTitle);
  inst != nil { s.snapshotCompletionSummaryBeforeTeardown(inst) }` — placed
  here specifically so the worktree still exists (the async `Destroy()`
  goroutine that runs `CleanupWorktree()` hasn't been spawned yet).
- Files: `server/services/session_service.go`

##### Task 3.3.1c: Call it from `stop_session` MCP tool before `Destroy()` (~3 min)
- In `stopSession` (`server/mcp/tools_lifecycle.go`), immediately before the
  `if err := inst.Destroy(); err != nil { ... }` line (~line 351), add a call
  to the same helper (exposed via the `SessionService` the MCP handler
  already holds, `lh.svc`).
- Files: `server/mcp/tools_lifecycle.go`

##### Task 3.3.1d: Unit test (~4 min)
- Add a test asserting the "no LLM call, completes fast, row exists" AC
  example, plus a regression test proving the row survives a subsequent
  `storage.DeleteInstance(title)` call (this doubles as an early check for
  Story 5.1.3's fuller ADR-001 proof test).
- Files: `server/services/completion_summary_service_test.go`

### Epic 3.4: Proto and RPC surface
**Goal**: Expose read + regenerate to the frontend, matching `GetSessionDiff`'s
established shape.

#### Story 3.4.1: Proto additions
**As a** backend developer, **I want** typed proto messages for the summary
and its status, **so that** frontend and backend share one contract.
**Acceptance Criteria**:
- `make proto-gen` regenerates both Go and TS bindings without errors.
  - *Given* the proto edits below, *When* `make proto-gen` runs, *Then*
    `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`
    both contain `CompletionSummary`, `CompletionSummaryStatus`,
    `GetSessionCompletionSummaryRequest/Response`, and
    `RegenerateSessionCompletionSummaryRequest/Response` with no compile
    errors on either side (`go build ./...` and `cd web-app && npx tsc --noEmit`).
**Files**: `proto/session/v1/types.proto`, `proto/session/v1/session.proto`

##### Task 3.4.1a: `CompletionSummaryStatus` enum + `CompletionSummary` message (~4 min)
- In `types.proto`, after the `SessionStatus` enum block (~line 347), add:
  ```protobuf
  enum CompletionSummaryStatus {
    COMPLETION_SUMMARY_STATUS_UNSPECIFIED = 0;
    COMPLETION_SUMMARY_STATUS_NOT_GENERATED = 1;
    COMPLETION_SUMMARY_STATUS_GENERATING = 2;
    COMPLETION_SUMMARY_STATUS_READY = 3;
    COMPLETION_SUMMARY_STATUS_FAILED = 4;
  }

  message CompletionSummary {
    string session_uuid = 1;
    CompletionSummaryStatus status = 2;
    string markdown = 3;
    google.protobuf.Timestamp generated_at = 4;
    string error_message = 5;
    bool narrative_generated = 6;
  }
  ```
- Files: `proto/session/v1/types.proto`

##### Task 3.4.1b: RPCs + request/response messages (~4 min)
- In `session.proto`, add after `rpc GetSessionDiff(...)` (~line 36):
  `rpc GetSessionCompletionSummary(GetSessionCompletionSummaryRequest) returns
  (GetSessionCompletionSummaryResponse) {}` and `rpc
  RegenerateSessionCompletionSummary(RegenerateSessionCompletionSummaryRequest)
  returns (RegenerateSessionCompletionSummaryResponse) {}`. Add messages near
  `GetSessionDiffRequest`/`Response` (~line 652):
  `GetSessionCompletionSummaryRequest{string id = 1;}`,
  `GetSessionCompletionSummaryResponse{CompletionSummary summary = 1;}`,
  `RegenerateSessionCompletionSummaryRequest{string id = 1;}`,
  `RegenerateSessionCompletionSummaryResponse{CompletionSummary summary = 1;}`.
- Files: `proto/session/v1/session.proto`

##### Task 3.4.1c: Run `make proto-gen` (~2 min)
- Run `make proto-gen`; commit all regenerated files in
  `session/gen/session/v1/` and `web-app/src/gen/session/v1/`.
- Files: `session/gen/session/v1/*` (generated), `web-app/src/gen/session/v1/*` (generated)

#### Story 3.4.2: RPC handlers
**As a** frontend developer, **I want** `GetSessionCompletionSummary` and
`RegenerateSessionCompletionSummary` handlers, **so that** `SummaryTab` has
something to call.
**Acceptance Criteria**:
- `GetSessionCompletionSummary` returns `NOT_GENERATED` (not an error) when
  no row exists yet.
  - *Given* no `CompletionSummary` row exists for session `"xyz-999"`, *When*
    `GetSessionCompletionSummary({id: "xyz-999"})` is called, *Then* it
    returns `200`/success with
    `summary.status == COMPLETION_SUMMARY_STATUS_NOT_GENERATED` and an empty
    `markdown`, never a `connect.CodeNotFound` error.
- `RegenerateSessionCompletionSummary` rejects regeneration for a session
  that isn't `Stopped`.
  - *Given* a live session `"abc-123"` with `Status == Active`, *When*
    `RegenerateSessionCompletionSummary({id: "abc-123"})` is called, *Then*
    it returns `connect.CodeFailedPrecondition` with a message stating the
    session must be stopped first, and no `CompletionSummary` row is
    written/modified.
**Files**: `server/services/session_service.go`

##### Task 3.4.2a: `GetSessionCompletionSummary` handler (~4 min)
- Add near `GetSessionDiff` (~line 2585): look up `storage.GetCompletionSummary`
  by resolved session UUID (resolve `req.Msg.Id` to UUID the same way
  `GetSessionDiff` resolves title/UUID, via `ListInstanceData`/`MatchesID`);
  map `CompletionSummaryData` → proto `CompletionSummary`, defaulting
  `status` to `COMPLETION_SUMMARY_STATUS_NOT_GENERATED` when no row exists.
- Files: `server/services/session_service.go`

##### Task 3.4.2b: `RegenerateSessionCompletionSummary` handler (~4 min)
- Add: resolve the live instance via `s.findInstance(req.Msg.Id)`; if `nil`
  or `Snapshot().Status != session.Stopped`, return
  `connect.CodeFailedPrecondition`; otherwise call `go
  s.generateAndPersistCompletionSummary(inst, "manual-regenerate")` and
  return immediately with `status=GENERATING` (optimistic — the frontend
  hook polls for the final result, Task 4.1.1a).
- Files: `server/services/session_service.go`

##### Task 3.4.2c: Handler unit tests (~5 min)
- Add tests to `server/services/session_service_test.go` (or a new
  `completion_summary_rpc_test.go` alongside it) covering both Given-When-Then
  examples above.
- Files: `server/services/session_service_test.go` or new `server/services/completion_summary_rpc_test.go`

---

## Phase 4: Frontend

### Epic 4.1: Data hook

#### Story 4.1.1: `useCompletionSummary`
**As a** frontend developer, **I want** a hook that fetches and optionally
polls the completion summary, **so that** `SummaryTab` stays a thin rendering
component.
**Acceptance Criteria**:
- Polls while status is `GENERATING`, stops once `READY`/`FAILED`.
  - *Given* `GetSessionCompletionSummary` initially returns
    `status: COMPLETION_SUMMARY_STATUS_GENERATING`, *When*
    `useCompletionSummary("abc-123")` is mounted, *Then* it re-calls
    `GetSessionCompletionSummary` every 3s until a response with
    `status !== GENERATING` arrives, then stops polling (verified via fake
    timers in a jest test).
**Files**: `web-app/src/lib/hooks/useCompletionSummary.ts` (new)

##### Task 4.1.1a: Implement the hook (~5 min)
- Create `web-app/src/lib/hooks/useCompletionSummary.ts` mirroring
  `useExportRules.ts`'s `createClient`/`getConnectTransport` setup pattern:
  exposes `{ summary, loading, error, regenerate }`; on mount and whenever
  `sessionId` changes, calls `GetSessionCompletionSummary`; if the returned
  `status === GENERATING`, sets a 3s `setInterval` to re-fetch, clearing it
  on unmount or once status leaves `GENERATING`; `regenerate()` calls
  `RegenerateSessionCompletionSummary` then restarts polling.
- Files: `web-app/src/lib/hooks/useCompletionSummary.ts`

##### Task 4.1.1b: Hook unit test (~5 min)
- Create `web-app/src/lib/hooks/useCompletionSummary.test.ts` using jest fake
  timers to verify the polling-stops-on-terminal-status AC example.
- Files: `web-app/src/lib/hooks/useCompletionSummary.test.ts` (new)

### Epic 4.2: `SummaryTab` component and wiring

#### Story 4.2.1: `SummaryTab.tsx`
**As a** user, **I want** a `Summary` tab on a stopped session showing the
rendered markdown with a copy action, **so that** I can read and reuse the
completion summary (AC-3, AC-4).
**Acceptance Criteria**:
- Renders markdown via `react-markdown` + `remark-gfm`, no `rehype-raw`.
  - *Given* `useCompletionSummary` returns
    `{summary: {status: READY, markdown: "## What Was Done\n..."}}`, *When*
    `SummaryTab` renders, *Then* the markdown is displayed via
    `<ReactMarkdown remarkPlugins={[remarkGfm]}>` inside a
    `markdownStyles.markdownBody`-styled container (matching
    `DescriptionSection.tsx`'s exact pattern) with no `dangerouslySetInnerHTML`
    anywhere in the component.
- Copy button reuses `copyToClipboard`, shows a 2s "Copied" state.
  - *Given* the rendered markdown is `"## What Was Done\n..."`, *When* the
    user clicks the button with `aria-label="Copy completion summary as
    markdown"`, *Then* `copyToClipboard(markdown)` is called with that exact
    string and the button's visible text changes from `"Copy"` to `"Copied"`
    for 2000ms before reverting.
- Three explicit states beyond "ready": loading skeleton, error+retry,
  not-yet-generated empty state.
  - *Given* `useCompletionSummary` returns `{error: new Error("failed to
    generate: claude CLI not found")}`, *When* `SummaryTab` renders, *Then*
    it shows the error message text plus a `<button>` labeled `"Regenerate"`
    that calls `regenerate()` on click — mirroring `VcsPanel.tsx`'s
    `error`/`handleRetry` block exactly.
**Files**: `web-app/src/components/sessions/SummaryTab.tsx` (new), `web-app/src/components/sessions/SummaryTab.css.ts` (new)

##### Task 4.2.1a: Component skeleton + loading/error/empty states (~5 min)
- Create `SummaryTab.tsx` with `// +feature: session-completion-summary`
  marker, `interface SummaryTabProps { session: Session }`, calling
  `useCompletionSummary(session.id)`; implement loading skeleton (mirroring
  `VcsPanel.tsx`'s `role="status" aria-label="Loading..."` bars), error state
  with `Regenerate` button, and not-yet-generated empty state
  (`"No completion summary yet — one will be generated automatically when
  this session finishes."`).
- Files: `web-app/src/components/sessions/SummaryTab.tsx`

##### Task 4.2.1b: Ready state — markdown render + copy button (~5 min)
- Add the `status === READY` branch: render markdown via `ReactMarkdown` +
  `remarkGfm` inside `markdownStyles.markdownBody` (import from
  `../backlog/markdownBody.css` per `DescriptionSection.tsx`'s pattern, or a
  local equivalent if cross-directory import is undesirable — prefer reusing
  the existing one for visual consistency); add the copy button using
  `copyToClipboard` from `@/lib/clipboard`, with local `copied` state
  (`useState` + `setTimeout(2000)`) driving the label text and an
  `aria-live="polite"` status span.
- Files: `web-app/src/components/sessions/SummaryTab.tsx`

##### Task 4.2.1c: `SummaryTab.css.ts` (~4 min)
- Create `SummaryTab.css.ts` following `ArtifactsTab.css.ts`'s import
  convention (`../../styles/theme-contract.css`); container style sets both
  `height: "100%"` and `overflowY: "auto"` per
  `.claude/rules/css-architecture.md`'s page-scroll convention; action-row
  style for the copy button above the markdown body.
- Files: `web-app/src/components/sessions/SummaryTab.css.ts`

##### Task 4.2.1d: Component tests (~5 min)
- Create `SummaryTab.test.tsx` (RTL) covering the markdown-render and
  copy-button Given-When-Then examples, mocking `useCompletionSummary`.
- Files: `web-app/src/components/sessions/SummaryTab.test.tsx` (new)

#### Story 4.2.2: Wire the tab into `SessionDetail`/`SessionDetailView`
**As a** user, **I want** the `Summary` tab visible in the existing tab strip,
**so that** I don't need a new navigation surface.
**Acceptance Criteria**:
- Tab is disabled until the session is `Stopped`.
  - *Given* a session with `status === SessionStatus.ACTIVE`, *When*
    `SessionDetailView` renders its tab strip, *Then* the `Summary` tab entry
    has `disabled: true` (same mechanism as the existing `browser` tab's
    `disabled: !isBrowserAvailable`).
**Files**: `web-app/src/components/sessions/SessionDetail.tsx`, `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 4.2.2a: Extend `SessionDetailTab` union (~2 min)
- In `SessionDetail.tsx:28`, add `"summary"` to the `SessionDetailTab` union
  type: `"terminal" | "diff" | "vcs" | "logs" | "info" | "files" | "browser" |
  "artifacts" | "summary"`.
- Files: `web-app/src/components/sessions/SessionDetail.tsx`

##### Task 4.2.2b: Add tab entry + render block (~4 min)
- In `SessionDetailView.tsx`: import `FileText` from `lucide-react` and
  `SummaryTab` from `./SummaryTab`; add `{ id: "summary", label: "Summary",
  icon: FileText, disabled: session.status !== SessionStatus.STOPPED }` to
  the `tabs` array (~line 283, after `"artifacts"`); add a render block
  `{activeTab === "summary" && (<div className={styles.tabContent}
  role="tabpanel" aria-labelledby="tab-summary"><SummaryTab session={session}
  /></div>)}` following the `"artifacts"` block's exact shape (~line 1239).
- Files: `web-app/src/components/sessions/SessionDetailView.tsx`

### Epic 4.3: Feature registry and e2e test

#### Story 4.3.1: Registry entries
**As a** maintainer, **I want** the new RPCs and UI feature registered, **so
that** `make registry-generate` doesn't flag a coverage gap.
**Acceptance Criteria**:
- `make registry-diff` shows the new entries with no unexpected deletions.
  - *Given* the new registry files below, *When* `make registry-generate`
    runs, *Then* `docs/registry/backend-features.json` and
    `docs/registry/frontend-features.json` include
    `session:get-completion-summary`, `session:regenerate-completion-summary`,
    and `session-completion-summary` with no other entries removed.
**Files**: `docs/registry/features/backend/session/get-completion-summary.json` (new), `docs/registry/features/backend/session/regenerate-completion-summary.json` (new), `docs/registry/features/frontend/session-completion-summary.json` (new)

##### Task 4.3.1a: Backend registry files (~3 min)
- Create `docs/registry/features/backend/session/get-completion-summary.json`
  and `.../regenerate-completion-summary.json`, following
  `docs/registry/features/backend/session/get-diff.json`'s exact shape
  (`id`, `type: "backend"`, `service: "SessionService"`, `method`,
  `protoFile: "proto/session/v1/session.proto"`, `markerFound: true` once the
  `// +api:` marker from Task 4.3.1c is added, `tested: true`, `testIds`
  populated from Task 3.4.2c's test names).
- Files: `docs/registry/features/backend/session/get-completion-summary.json`, `docs/registry/features/backend/session/regenerate-completion-summary.json`

##### Task 4.3.1b: Frontend registry file (~2 min)
- Create `docs/registry/features/frontend/session-completion-summary.json`
  with `id: "session-completion-summary"`, `filePath:
  "web-app/src/components/sessions/SummaryTab.tsx"`, `tested: true`,
  `testIds` from Task 4.2.1d and the e2e test (Story 4.3.2).
- Files: `docs/registry/features/frontend/session-completion-summary.json`

##### Task 4.3.1c: Add `// +api:` markers and regenerate (~3 min)
- Add `// +api: session:get-completion-summary` and `// +api:
  session:regenerate-completion-summary` doc-comments above the two handlers
  from Story 3.4.2 (matching `DeleteSession`'s `// +api: session:delete`
  comment style at `session_service.go:1953`); run `make registry-generate`
  and commit the changed per-feature files plus the regenerated aggregate
  JSON.
- Files: `server/services/session_service.go`, `docs/registry/*-features.json` (regenerated)

#### Story 4.3.2: e2e test
**As a** QA-minded developer, **I want** an e2e test covering the
deterministic (explicit-stop) path, **so that** the tab's rendering and copy
action are verified end-to-end.
**Acceptance Criteria**:
- A session stopped via the UI's delete action shows a `Ready`-status
  `Summary` tab with a deterministic (no-narrative) document.
  - *Given* a directory-mode session is created and then stopped via the
    existing delete UI flow (which — per ADR-001 — triggers the synchronous,
    deterministic-only `snapshotCompletionSummaryBeforeTeardown` path, the
    only path an e2e test can deterministically trigger without waiting on a
    real Claude process to exit on its own), *When* the test navigates to
    that session's `Summary` tab, *Then* `expect(page.getByTestId(
    "completion-summary-markdown")).toBeVisible()` passes and the rendered
    text includes `"No approval decisions were required."` for a session
    with no tool activity.
**Files**: `tests/e2e/session-completion-summary.spec.ts` (new)

##### Task 4.3.2a: Write the spec (~5 min)
- Create `tests/e2e/session-completion-summary.spec.ts` starting with `//
  @feature session:get-completion-summary, session-completion-summary` per
  `.claude/rules/e2e-test-conventions.md`; use `getByTestId`/`getByRole`
  locators only, no `waitForTimeout` (use
  `expect(...).toHaveText(...)`/`waitForSelector` on the tab content); add a
  `data-testid="completion-summary-markdown"` to the rendered markdown
  container in `SummaryTab.tsx` (small addition to Task 4.2.1b).
- Files: `tests/e2e/session-completion-summary.spec.ts`, `web-app/src/components/sessions/SummaryTab.tsx`

##### Task 4.3.2b: Run and verify (~3 min)
- Run `cd tests/e2e && npx playwright test session-completion-summary.spec.ts`
  against the auto-managed isolated test server; confirm it passes.
- Files: none (verification only)

---

## Phase 5: Edge-Case Hardening

### Epic 5.1: Prove the three highest-risk behaviors with tests
**Goal**: The three claims this plan makes that are easiest to silently
regress — AC-6 (minimal session), AC-5 (narrative failure degrades
gracefully), and ADR-001 (storage independence) — each get one purpose-built
test.

#### Story 5.1.1: Minimal-session end-to-end test
**Acceptance Criteria**:
- A session cancelled seconds after creation still produces a valid,
  non-error document via the full pipeline (not just the pure markdown
  function in isolation, per Task 2.1.2c).
  - *Given* a session created and immediately stopped (no file changes, no
    approvals, no token usage recorded), *When* `EventExited` fires, *Then*
    the resulting `CompletionSummary` row has `status="ready"` (never
    `"failed"`) and its `markdown` contains the AC-6 empty-state lines.
**Files**: `server/services/completion_summary_service_test.go`

##### Task 5.1.1a: Integration test (~5 min)
- Add `TestGenerateAndPersistCompletionSummary_MinimalSession` exercising
  `generateAndPersistCompletionSummary` end-to-end against a real (temp
  SQLite-backed) `Storage`, asserting the Given-When-Then above.
- Files: `server/services/completion_summary_service_test.go`

#### Story 5.1.2: Narrative failure never fails the whole document
**Acceptance Criteria**:
- Already covered at the unit level by Task 3.1.1b/3.2.1c's fallback
  assertion; this task adds the full-pipeline version.
  - *Given* `claude` is deliberately missing from `PATH` for this test,
    *When* `generateAndPersistCompletionSummary` runs for a session with real
    diff/approval data, *Then* the persisted row has `status="ready"`,
    `narrative_generated=false`, and the markdown's narrative section reads
    a deterministic fallback line (e.g. `"N files changed (+X/-Y)."`) instead
    of an LLM sentence, per `research/build-vs-buy.md` §5's recommended
    fallback.
**Files**: `server/services/completion_summary_service_test.go`, `session/completion_summary.go`

##### Task 5.1.2a: Deterministic narrative fallback line (~4 min)
- In `GenerateCompletionSummaryMarkdown` (Task 2.1.2a/b), when `narrative ==
  ""`, render a one-line deterministic fallback under `## What Was Done`
  derived purely from `snap.DiffAdded`/`DiffRemoved`/`ChangedFiles` counts
  (e.g. `fmt.Sprintf("%d files changed (+%d/-%d). No AI-generated narrative
  available.", len(snap.ChangedFiles), snap.DiffAdded, snap.DiffRemoved)`).
- Files: `session/completion_summary.go`

##### Task 5.1.2b: Full-pipeline test (~4 min)
- Add `TestGenerateAndPersistCompletionSummary_NarrativeFailureFallback`
  asserting the Given-When-Then above.
- Files: `server/services/completion_summary_service_test.go`

#### Story 5.1.3: `CompletionSummary` survives `Session` row deletion (ADR-001 proof)
**Acceptance Criteria**:
- The core claim of ADR-001, proven directly at the repository layer.
  - *Given* a `CompletionSummary` row exists with `session_uuid="abc-123"`
    and a `Session` row also exists with `title="abc-123-title"`/matching
    UUID, *When* `storage.DeleteInstance("abc-123-title")` is called (the
    same call `DeleteSession`'s RPC handler makes), *Then*
    `storage.GetCompletionSummary("abc-123")` still returns the original row
    unchanged (not a zero-value `not_generated` result) — proving the entity
    has no cascading FK to `Session`.
**Files**: `session/ent_repository_test.go` or `server/services/completion_summary_service_test.go`

##### Task 5.1.3a: Write the survival test (~4 min)
- Add `TestCompletionSummary_SurvivesSessionDeletion` (in
  `session/ent_repository_test.go`, alongside existing `DiffStats`-adjacent
  tests, since it's a repository-layer contract) asserting the
  Given-When-Then above using the real `EntRepository`/`Storage` against a
  temp SQLite DB.
- Files: `session/ent_repository_test.go`

##### Task 5.1.3b: Run the full suite (~3 min)
- Run `make build && make test` (Go) and `cd web-app && npx jest --no-coverage`
  (frontend); run `make lint`; fix any failures surfaced before considering
  the feature complete.
- Files: none (verification only)
