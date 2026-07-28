# Implementation Plan: backlog-github-issue-link

**Feature**: Surface the linked GitHub issue/PR URL to imported backlog items' agent prompts and DB rows, so the agent can reference the originating issue in its PR.
**Date**: 2026-07-25
**Status**: Ready for implementation
**ADRs**: ADR-001-external-url-backfill-and-prompt-boundary.md

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name. Exact names here must be used consistently in code, tests, and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `ExternalURL` | The full `html_url` of the originating GitHub issue/PR (e.g. `https://github.com/acme/widget/issues/42`), carried on `BacklogItemData`, `BacklogItemUpdate`, and the `BacklogItem` ent entity/DB column. Capped at 500 chars. | Plain `string`, not a newtype — see Pattern Decisions for why. Empty string means "no linked issue" (manually-created item, or not yet backfilled). |
| `ExternalID` | The bare issue/PR number as a string (e.g. `"42"`), already existing before this change. Unique only within a given `ItemSource` (two repos can each have issue #1). | Pre-existing field; `ExternalURL` is added as its sibling, never a lookup key. |
| `closingKeywordFor` | New Go function in `session/backlog_context.go`: given an `ExternalURL`, deterministically returns the fully-punctuated instruction prefix — `"Fixes "` (space, no colon) for `/issues/` URLs or `"Related: "` (colon+space) for `/pull/`-or-unrecognized URLs — matching AC3's literal wording exactly. Never left to agent-side inference (AC3). | Pure function, `strings.Contains`-based, no `net/url` parsing needed. Returns the punctuation itself so the caller never has to assemble it (see Task 4.1.2b) — this removes the punctuation-assembly responsibility from the caller entirely. |
| `githubShortRefFor` | New Go function in `session/backlog_context.go`: given an `ExternalURL`, extracts the `"owner/repo#N"` reference GitHub's closing-keyword parser actually recognizes (e.g. `"acme/widget#42"`). | Added during sdd:4-validate to fix a confirmed defect: GitHub's documented closing-keyword syntax does NOT accept a bare full URL, only `#N`/`owner/repo#N` — see Story 4.1.1. Used only in the instruction line, never the fact line (which still shows the full URL for human readability). |
| `BacklogItemData` | The domain-model struct (`session/repository.go:257-282`) that all repository methods return/accept; the ent-independent representation of a backlog item. | Gains `ExternalURL string` field. |
| `BacklogItemUpdate` | The mutable-fields struct (`session/repository.go:301-313`) passed to `UpdateBacklogItem`; every field is a pointer, `nil` means "leave unchanged." | Gains `ExternalURL *string`. `UpdateBacklogItem` itself has no gating logic — all local-wins/backfill decisions live in the caller (`SyncOne`). |
| `SyncOne` | `session/backlog_sync.go:195-303` — the per-source sync tick: fetches items via the plugin, creates new ones, and applies local-wins updates to existing ones. | The "ExternalURL backfill" is the unconditional write of `ExternalURL` on the existing-item branch, described below. |
| `anyField` | Existing local variable in `SyncOne` (`session/backlog_sync.go:263`) — tracks whether any field actually needs updating, gating whether `UpdateBacklogItem` is called at all (`if !anyField { skipped++; continue }` at line 280). | This plan's core subtlety (ADR-001, Decision 1): the `ExternalURL` backfill sets `anyField = true` independently of the three `UserModifiedFields`-gated blocks, so a fully-user-locked item still gets backfilled. |
| `ExternalURL` backfill | The behavior added to `SyncOne`'s existing-item branch: `if existing.ExternalURL == "" && data.ExternalURL != "" { update.ExternalURL = &data.ExternalURL; anyField = true }` — bypasses `UserModifiedFields` local-wins entirely, unconditional per AC6. | Known limitation (accepted, not a bug): only fires for items still returned by the plugin's `state=open` Fetch; closed/renamed-then-closed issues never get backfilled. |
| `GetBacklogItemByExternalID` | Existing lookup (`session/ent_repository_backlog.go:466`), scoped by `(sourceID, externalID)`. Unaffected by this change — `ExternalURL` is never a lookup key. | No new index, no new collision risk. |
| Fact line | The new `"Linked GitHub Issue/PR: <url>"` line rendered inside `BuildSessionInitialPrompt`'s inert-data block — treated as *data about the item*, like Title/Priority. | See ADR-001, Decision 2. |
| Instruction line | The new line rendered *outside* the inert-data block (e.g. `` "...include the line `Fixes acme/widget#42` in the PR body..." `` — `closingKeywordFor`'s output concatenated directly with `githubShortRefFor`'s output, no separate colon), alongside the existing plan.md pointer — a genuine first-party instruction, never inside the "treat as inert data" section. | See ADR-001, Decisions 2 and 3. |

---

## Pattern Decisions

**Step 0.5 alternatives considered for the `SyncOne` AC6 backfill mechanism:**
1. **Extend `BacklogItemUpdate` with `ExternalURL *string`, set unconditionally in `SyncOne`'s existing caller loop** (chosen). Strength: zero new abstractions — `UpdateBacklogItem` is already a dumb field-setter with all gating living in the caller, so this is the minimal diff that fits the existing shape exactly. Weakness: requires care that `anyField` is set correctly (see ADR-001) — an easy one-line mistake if not flagged.
2. **Bespoke `BackfillExternalURL(ctx, itemID, url string)` repository method, called separately from the main update.** Strength: keeps "backfill" semantically distinct from "local-wins update" in the type signature. Weakness: duplicates the `UpdateOneID`/`Save` boilerplate for one column, doubles DB round-trips per synced item, and still needs its own guard against clobbering a real value — no net simplification.
3. **Direct ent call bypassing `BacklogItemUpdate` entirely** (`er.client.BacklogItem.UpdateOneID(id).SetExternalURL(url).Save(ctx)` inline in `SyncOne`). Strength: fewest lines touched in the immediate diff. Weakness: breaks the existing architectural seam where `SyncOne` only talks to the domain-level `Storage`/`BacklogItemUpdate` API, never the ent client directly — every other field update in this function goes through that seam, and bypassing it here would be an inconsistent one-off that later readers would have to explain.

Rejected: #2 and #3, in the table below.

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `SyncOne` ExternalURL backfill | Transaction Script (extend existing procedural `BacklogItemUpdate` caller loop) | PoEAA (Fowler) | Bespoke `BackfillExternalURL` repository method | Duplicates `UpdateOneID`/`Save` boilerplate for zero behavioral gain; doubles DB round-trips |
| `SyncOne` ExternalURL backfill | Transaction Script (via `BacklogItemUpdate`) | PoEAA (Fowler) | Direct ent client call bypassing `BacklogItemUpdate` | Breaks the existing architectural seam — every other field update in `SyncOne` goes through the domain-level `Storage` API, not the ent client directly |
| `ExternalURL` field type | Plain `string`, no newtype | type-driven-design (considered and rejected) | `type ExternalURL string` newtype, or a `URL` value object with parse/validate | `ExternalURL` is pure display data — written once (capped at 500 chars, never parsed for owner/repo/number), read once (rendered as-is in the prompt and in `closingKeywordFor`'s substring check). A newtype would add a wrapper/unwrap ritual at every call site (`ExternalID` — its exact sibling field — is also a plain `string` for the same reason) with no compiler-enforced invariant it actually protects; the only real invariant (≤500 chars) is enforced once at the write boundary (the two `MapToBacklogItem` methods), matching the existing `Title`/200-char and `Description`/2000-char pattern. |
| `BacklogItem` (ent schema) | Repository pattern (PoEAA), already established | PoEAA (Fowler) | N/A — pre-existing pattern, not revisited | This change adds one field to an existing entity; does not change the persistence pattern |
| `closingKeywordFor` | Plain function, stdlib `strings.Contains` | Build-vs-buy research (verdict: stdlib only) | `net/url.Parse` + `.Path` inspection; regex; a GitHub API client library (e.g. go-github) | Substring check is fully correct for this narrow, trusted-input (GitHub-generated `html_url`), two-shape problem; `net/url`/regex/SDK all add complexity the problem doesn't need. Consistent with the codebase's existing convention of reserving `net/url` for actual security stakes (CORS, proxy routing), not casual string checks. |
| Fact line vs. instruction line placement in `BuildSessionInitialPrompt` | Two-part rendering split across the existing inert-data boundary (no new GoF pattern) | Existing ADR-012 context-injection mechanism | Single combined "## Linked Issue" data subsection | Would place a first-party instruction inside the block the agent is told to treat as inert data, contradicting the block's own prompt-injection defense — see ADR-001, Decision 2 |

---

## Migration Plan

- **Migration file**: none as a standalone SQL file — this codebase uses ent's auto-migration (`session/ent_repository.go:86`, `client.Schema.Create(ctx)` runs on every process startup and diffs the ent schema against the live DB). The "migration" for this change is the schema field addition in `session/ent/schema/backlog_item.go` (`field.String("external_url").Optional()`) plus regenerating with `go generate ./session/ent` (which invokes `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema` per `session/ent/generate.go`). At next startup, `client.Schema.Create` issues `ALTER TABLE backlog_items ADD COLUMN external_url TEXT NULL` against the live SQLite DB.
- **Reversibility**: irreversible via ent's auto-migration (ent does not generate down-migrations). Not a concern here: the column is nullable with no default and no backfill required at migration time — dropping it later (if ever needed) would be a manual `ALTER TABLE ... DROP COLUMN` outside ent's auto-migration path, which is out of scope for this feature.
- **Zero-downtime strategy**: not applicable — this is a local single-process SQLite application (`~/.stapler-squad/` state directory per-workspace), not a multi-instance production DB. Auto-migration runs at startup before the server accepts traffic; there is no rolling-deploy window to protect.
- **Rollback procedure**: revert the PR. The `external_url` column would remain in the DB (harmless, unused, `NULL`/empty) until a future auto-migration removes it — ent's auto-migration does not drop columns absent from the schema, so no data loss risk from a partial rollback either.

## Observability Plan

- **Logs**: no new structured log lines are required — `SyncOne` already logs a per-tick summary line (`session/backlog_sync.go:300-301`, `"[SyncLoop] source=%s plugin=%s created=%d updated=%d skipped=%d errored=%d"`); the `ExternalURL` backfill rides along inside the existing `updated` counter with no separate instrumentation needed. `UpdateBacklogItem` errors are already logged with item ID at `session/backlog_sync.go:286-288`.
- **Metrics**: no new metrics — this is plumbing for an existing sync/prompt-render path already covered by the existing OTel instrumentation on `SyncOne`/`UpdateBacklogItem` (per CLAUDE.md's existing OpenTelemetry section); no new operation >100ms is introduced.
- **Alerts**: no new alerts required.

## Risk Control

- **Feature flag**: not gated — this is additive, backward-compatible plumbing (empty `ExternalURL` renders identically to today per AC4); no flag needed.
- **Rollback procedure**: standard revert via PR close + revert commit.
- **Staged rollout**: full rollout on merge — no user-facing surface, no migration risk beyond the additive nullable column described in the Migration Plan.

## Unresolved Questions

None. All design choices are resolved by the research and ADR-001; nothing here blocks any story from starting.

## Dependency Visualization

```
Phase 1: Data Model Foundation
  Epic 1.1 (schema + generated ent)
    Story 1.1.1 (schema field + go generate) ──┐
                                                 │
  Epic 1.2 (repository plumbing)                │
    Story 1.2.1 (BacklogItemData/Update fields) ◄┘
    Story 1.2.2 (converter + Create + Update)    ◄── depends on 1.2.1
                          │
        ┌─────────────────┼─────────────────────┬───────────────────────┐
        ▼                 ▼                     ▼                       ▼
Phase 2: Plugins    Phase 3: Sync Backfill  Phase 4: Prompt        Phase 5: Service literals
  Epic 2.1            Epic 3.1                Epic 4.1               Epic 5.1
  (Issues + PRs       (SyncOne anyField       (closingKeywordFor      (SpawnSessionFromItem +
   MapToBacklogItem)   fix + backfill test)    + fact/instr. lines    AttachSessionToItem
                                                + truncation check)    ExternalURL literals)
        │                 │                     │                       │
        └─────────────────┴─────────────────────┴───────────────────────┘
                          │
                          ▼
              Phase 6: Final Verification (AC9)
              make build && make test, fix fallout
```
Phases 2, 3, 4 have no inter-dependency once Phase 1 lands and can be implemented in any order; Phase 5 depends on Phase 1 (needs `ExternalURL` on `BacklogItemData`) and Phase 4 (its integration test asserts the fact/instruction lines Phase 4 adds). Phase 6 depends on all prior phases.

---

## Phase 1: Data Model Foundation

### Epic 1.1: Ent schema field + codegen
**Goal**: Add the `external_url` column to the `BacklogItem` ent entity so generated code exposes `ExternalURL`/`SetExternalURL`/`SetNillableExternalURL`.

#### Story 1.1.1: Add `external_url` field to ent schema and regenerate
**As a** developer, **I want** the ent schema and generated code to carry an `ExternalURL` field, **so that** the repository layer can read/write it.
**Acceptance Criteria**:
- AC2 (part 1): `BacklogItem` ent entity/DB column gains `external_url`.
  - *Given* `session/ent/schema/backlog_item.go` has no `external_url` field, *When* `field.String("external_url").Optional()` is added to `Fields()` and `go generate ./session/ent` is run, *Then* `session/ent/backlogitem.go` contains a plain `ExternalURL string` struct field (not `*string`) and `session/ent/migrate/schema.go` contains a `{Name: "external_url", Type: field.TypeString, Nullable: true}` column entry.
- AC2 (part 2): pre-existing rows read back `ExternalURL == ""`, no NULL panic.
  - *Given* a SQLite DB with a `backlog_items` row created before this migration (so `external_url` is `NULL` after auto-migration adds the column), *When* that row is fetched via `r.client.BacklogItem.Get(ctx, id)`, *Then* `item.ExternalURL == ""` and no panic occurs — because ent's generated scan code reads a `sql.NullString` and only assigns `.String` to the struct field when `value.Valid` is true, otherwise leaving the field at Go's zero value for `string` (`""`) — confirmed identical to the existing `Notes` field's scan behavior at `session/ent/backlogitem.go:229-234`.
**Files**: `session/ent/schema/backlog_item.go`, `session/ent/backlogitem.go` (generated), `session/ent/migrate/schema.go` (generated), `session/ent/backlogitem_create.go` (generated), `session/ent/backlogitem_update.go` (generated), `session/ent/mutation.go` (generated), `session/ent_repository_backlog_test.go` (new file)

##### Task 1.1.1a: Add `external_url` field to schema (~2 min)
- In `session/ent/schema/backlog_item.go`, in `Fields()`, add immediately after the existing `field.String("external_id").Optional(),` block (currently lines 55-56):
  ```go
  field.String("external_url").
      Optional(),
  ```
- Do NOT add `.Nillable()` — must match `external_id`'s exact shape (plain `Optional()`) so NULL reads back as `""`, not a nil-pointer-check burden.
- Do NOT add a new index — `external_url` is never a lookup key (only ever written/read as opaque display data).
- Files: `session/ent/schema/backlog_item.go`

##### Task 1.1.1b: Regenerate ent code and verify compilation (~3 min)
- Run: `go generate ./session/ent` (from repo root; resolves to `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema` per `session/ent/generate.go` — do NOT omit `--feature sql/upsert`, that flag is already baked into the generate directive so a plain `go generate ./session/ent` picks it up automatically).
- Run: `go build ./session/...` and confirm it compiles — it is expected to **pass** at this checkpoint. Adding a purely additive `Optional()` field to the ent schema and regenerating never breaks a build on its own; nothing elsewhere in the codebase is required to reference a newly-added struct field for `go build` to succeed. This step is an early sanity check that codegen produced valid Go (i.e. that `--feature sql/upsert` wasn't dropped and the generated files parse/typecheck), not a red flag if it succeeds. If `go build ./session/ent/...` in particular fails, the codegen step was wrong.
- Files: `session/ent/backlogitem.go`, `session/ent/migrate/schema.go`, `session/ent/backlogitem_create.go`, `session/ent/backlogitem_update.go`, `session/ent/mutation.go` (all regenerated, no hand-edits)

##### Task 1.1.1c: Add the NULL-safety migration test (~4 min) — closes sdd:4-validate Gap 1
- In a new file `session/ent_repository_backlog_test.go` (colocated with `session/ent_repository_backlog.go`, mirroring the `TestEntRepository_*` naming/placement convention used for `session/ent_repository.go` in `session/ent_repository_test.go`), add `TestGetBacklogItem_ExternalURL_ReadsEmptyStringForPreExistingRow`: create a `BacklogItem` directly via `repo.client.BacklogItem.Create()` without setting `external_url` (simulating a row written before this column existed, so it scans from SQL `NULL`), then `repo.client.BacklogItem.Get(ctx, id)` and assert `ExternalURL == ""` with no panic. This is the actual test proving AC2's NULL-safety claim — Task 1.1.1b's build-passing check alone doesn't exercise the runtime scan path.
- Files: `session/ent_repository_backlog_test.go` (new)

---

### Epic 1.2: Repository struct + converter plumbing
**Goal**: Thread `ExternalURL` through the domain-level `BacklogItemData`/`BacklogItemUpdate` structs and their ent converters, so callers above the ent layer never touch `ent.BacklogItem` directly for this field.

#### Story 1.2.1: Add `ExternalURL` to `BacklogItemData` and `BacklogItemUpdate`
**As a** developer, **I want** the domain-model structs to carry `ExternalURL`, **so that** repository callers can read/write it without touching ent directly.
**Acceptance Criteria**:
- AC1 (part 1, struct-level): `BacklogItemData` gains an `ExternalURL` field.
  - *Given* `session/repository.go`'s `BacklogItemData` struct at line 257 has no `ExternalURL` field, *When* `ExternalURL string` is added next to `ExternalID string` (line 271), *Then* any code constructing `BacklogItemData{ExternalURL: "https://github.com/acme/widget/issues/42"}` compiles.
**Files**: `session/repository.go`

##### Task 1.2.1a: Add `ExternalURL` to `BacklogItemData` (~2 min)
- In `session/repository.go`, in the `BacklogItemData` struct (lines 257-282), add `ExternalURL string` immediately after `ExternalID         string` (line 271).
- Files: `session/repository.go`

##### Task 1.2.1b: Add `ExternalURL` to `BacklogItemUpdate` (~2 min)
- In `session/repository.go`, in the `BacklogItemUpdate` struct (lines 301-313), add `ExternalURL *string` (position doesn't matter functionally; add after `Notes *string` at line 309 for readability, grouping it near the sibling `ExternalID`-adjacent fields).
- Files: `session/repository.go`

#### Story 1.2.2: Wire `ExternalURL` through the ent converter and CRUD methods
**As a** developer, **I want** `backlogItemToData`, `CreateBacklogItem`, and `UpdateBacklogItem` to carry `ExternalURL` both directions, **so that** the field round-trips through the repository layer.
**Acceptance Criteria**:
- AC2 (part 3): the field round-trips through `CreateBacklogItem`/`GetBacklogItem`.
  - *Given* `storage.CreateBacklogItem(ctx, BacklogItemData{Title: "t", ExternalURL: "https://github.com/acme/widget/issues/42", ...})`, *When* the created item is refetched via `storage.GetBacklogItem(ctx, created.ID)`, *Then* `refetched.ExternalURL == "https://github.com/acme/widget/issues/42"`.
**Files**: `session/ent_repository_backlog.go`, `session/backlog_lifecycle_test.go`

##### Task 1.2.2a: Add `ExternalURL` to `backlogItemToData` converter (~2 min)
- In `session/ent_repository_backlog.go`, in `backlogItemToData` (lines 21-50), add `ExternalURL: item.ExternalURL,` immediately after `ExternalID:         item.ExternalID,` (line 36).
- Files: `session/ent_repository_backlog.go`

##### Task 1.2.2b: Add `ExternalURL` to `CreateBacklogItem` (~2 min)
- In `session/ent_repository_backlog.go`, in `CreateBacklogItem` (lines 71-110), add `.SetNillableExternalURL(&data.ExternalURL).` immediately after `.SetNillableExternalID(&data.ExternalID).` (line 94), before `.SetNillableArchivedAt(data.ArchivedAt)` (line 95).
- Files: `session/ent_repository_backlog.go`

##### Task 1.2.2c: Add `ExternalURL` apply-block to `UpdateBacklogItem` (~2 min)
- In `session/ent_repository_backlog.go`, in `UpdateBacklogItem` (lines 186-251), add immediately after the existing `if update.Notes != nil { u.SetNotes(*update.Notes) }` block (lines 232-234):
  ```go
  if update.ExternalURL != nil {
      u.SetExternalURL(*update.ExternalURL)
  }
  ```
- Files: `session/ent_repository_backlog.go`

##### Task 1.2.2c-2: Add the Create/Get round-trip test (~3 min) — closes sdd:4-validate Gap 2
- In `session/backlog_lifecycle_test.go`, following that file's existing `storage.CreateBacklogItem(ctx, itemData)` pattern, add `TestCreateBacklogItem_ExternalURL_RoundTripsThroughGetBacklogItem`: create an item with `ExternalURL` set, refetch via `storage.GetBacklogItem`, assert the value round-trips exactly.
- Files: `session/backlog_lifecycle_test.go`

##### Task 1.2.2d: Verify Phase 1 compiles standalone (~2 min)
- Run: `go build ./session/...` — should now compile cleanly (all `ExternalURL` references in `session/` package now resolve; plugin/sync/prompt files from later phases don't yet reference it, so no new compile errors are introduced by this phase).
- Files: none (verification only)

---

## Phase 2: Plugin Mapping (AC1)

### Epic 2.1: GitHub plugins populate `ExternalURL`
**Goal**: Both `GitHubIssuesPlugin.MapToBacklogItem` and `GitHubPRsPlugin.MapToBacklogItem` populate `ExternalURL` from the already-fetched `item.URL`, capped at 500 chars.

#### Story 2.1.1: `GitHubIssuesPlugin.MapToBacklogItem` populates `ExternalURL`
**As a** developer, **I want** imported GitHub issues to carry their `html_url`, **so that** the agent prompt can reference the originating issue.
**Acceptance Criteria**:
- AC1: `GitHubIssuesPlugin.MapToBacklogItem` populates `ExternalURL`, capped at 500 chars.
  - *Given* `ExternalItem{ExternalID: "42", Title: "Bug", URL: "https://github.com/acme/widget/issues/42"}` (already populated from `issue.HTMLURL` at `session/backlog_plugin_github.go:144`), *When* `GitHubIssuesPlugin.MapToBacklogItem(item, sourceID)` is called, *Then* the returned `BacklogItemData.ExternalURL == "https://github.com/acme/widget/issues/42"`.
  - *Given* `ExternalItem{URL: strings.Repeat("x", 600)}` (a 600-char URL, unrealistic but must not panic), *When* `MapToBacklogItem` is called, *Then* `BacklogItemData.ExternalURL` has length exactly 500 (raw `[:500]` slice, same pattern as `Title`/200 and `Description`/2000 at lines 158-166).
**Files**: `session/backlog_plugin_github.go`, `session/backlog_plugin_github_test.go`

##### Task 2.1.1a: Add `ExternalURL` to `GitHubIssuesPlugin.MapToBacklogItem` (~3 min)
- In `session/backlog_plugin_github.go`, in `MapToBacklogItem` (lines 156-176), add a capped `url` variable mirroring the existing `title`/`desc` truncation pattern (lines 158-166), then add `ExternalURL: url,` to the returned `BacklogItemData` literal (line 168-175) next to `ExternalID:  item.ExternalID,`:
  ```go
  url := item.URL
  if len(url) > 500 {
      url = url[:500]
  }
  ```
- Files: `session/backlog_plugin_github.go`

##### Task 2.1.1b: Add URL round-trip + cap tests for `GitHubIssuesPlugin` (~4 min)
- In `session/backlog_plugin_github_test.go`, extend `TestGitHubIssuesPlugin_MapToBacklogItem_TruncatesLongFields` (line 85) or add a new test asserting: (1) a normal-length `URL` round-trips unchanged into `ExternalURL`, (2) a >500-char `URL` is capped at exactly 500 chars in `ExternalURL`.
- Files: `session/backlog_plugin_github_test.go`

#### Story 2.1.2: `GitHubPRsPlugin.MapToBacklogItem` populates `ExternalURL`
**As a** developer, **I want** imported GitHub PRs to carry their `html_url`, **so that** the agent prompt can reference the originating PR.
**Acceptance Criteria**:
- AC1: `GitHubPRsPlugin.MapToBacklogItem` populates `ExternalURL`, capped at 500 chars.
  - *Given* `ExternalItem{ExternalID: "17", Title: "Add feature", URL: "https://github.com/acme/widget/pull/17"}` (already populated from `pr.HTMLURL` at `session/backlog_plugin_github_prs.go:133`), *When* `GitHubPRsPlugin.MapToBacklogItem(item, sourceID)` is called, *Then* the returned `BacklogItemData.ExternalURL == "https://github.com/acme/widget/pull/17"`.
**Files**: `session/backlog_plugin_github_prs.go`, `session/backlog_plugin_github_test.go`

##### Task 2.1.2a: Add `ExternalURL` to `GitHubPRsPlugin.MapToBacklogItem` (~3 min)
- In `session/backlog_plugin_github_prs.go`, in `MapToBacklogItem` (lines 194-212), apply the identical addition as Task 2.1.1a (same truncation pattern, same field name) to the returned `BacklogItemData` literal (lines 204-211) next to `ExternalID:  item.ExternalID,`.
- Files: `session/backlog_plugin_github_prs.go`

##### Task 2.1.2b: Add URL round-trip + cap test for `GitHubPRsPlugin` (~3 min)
- In `session/backlog_plugin_github_test.go`, extend `TestGitHubPRsPlugin_MapToBacklogItem_TruncatesLongFields` (line 216) to also assert `ExternalURL` round-trips and caps at 500 chars, mirroring Task 2.1.1b.
- Files: `session/backlog_plugin_github_test.go`

##### Task 2.1.3: Run package tests for Phase 2 (~2 min)
- Run: `go build ./session/... && go test ./session/... -run 'TestGitHubIssuesPlugin|TestGitHubPRsPlugin'`
- Files: none (verification only)

---

## Phase 3: Sync Backfill (AC6)

### Epic 3.1: `SyncOne` unconditionally backfills `ExternalURL`
**Goal**: Existing rows with an empty `ExternalURL` get backfilled from the plugin's fetch result, bypassing `UserModifiedFields` local-wins, without breaking the existing `anyField`/`skipped` short-circuit for rows that need no backfill. See ADR-001, Decision 1 for the exact reasoning this implements.

#### Story 3.1.1: Add the unconditional `ExternalURL` backfill to `SyncOne`
**As a** developer, **I want** `SyncOne`'s existing-item branch to backfill `ExternalURL` regardless of `UserModifiedFields`, **so that** pre-existing imported items eventually get a linked-issue URL without requiring the user to re-import.
**Acceptance Criteria**:
- AC6 (positive case): backfill fires for an existing item with no `ExternalURL`.
  - *Given* an existing `BacklogItem` with `ExternalID: "1"`, `ExternalURL: ""`, `UserModifiedFields: ""` (no fields locked), and a plugin `Fetch` returning `ExternalItem{ExternalID: "1", URL: "https://github.com/acme/widget/issues/1"}`, *When* `SyncOne` runs, *Then* `existing.ExternalURL` is updated to `"https://github.com/acme/widget/issues/1"` and the sync event reports `ItemsUpdated == 1`.
  - *Given* the same setup but with `UserModifiedFields: '["title","description","priority"]'` (all three other syncable fields locked), *When* `SyncOne` runs, *Then* `ItemsUpdated == 1` (NOT `ItemsSkipped`) and `ExternalURL` is still backfilled — this is the critical regression case from ADR-001: without `anyField = true` set independently of the three gated blocks, this case would incorrectly report `ItemsSkipped == 1` and silently drop the backfill.
- AC6 (negative case, documented known limitation): items not returned by `Fetch` are never backfilled.
  - *Given* an existing `BacklogItem` with `ExternalURL: ""` whose `ExternalID` is NOT present in the plugin's `Fetch` result for this sync tick (simulating a closed issue), *When* `SyncOne` runs, *Then* that item's `ExternalURL` remains `""` — pinned as an accepted limitation, not a bug (requirements.md, "Explicitly out of scope").
**Files**: `session/backlog_sync.go`

##### Task 3.1.1a: Add the `ExternalURL` backfill condition to `SyncOne` (~3 min)
- In `session/backlog_sync.go`, in `SyncOne`'s existing-item branch, add immediately after the three existing `UserModifiedFields`-gated blocks (after line 276, the `priority` block's closing brace) and before the `// Status is always local-wins...` comment (line 277):
  ```go
  // Backfill ExternalURL unconditionally, bypassing local-wins, per the
  // known limitation documented in requirements.md AC6: this only fires
  // for items still returned by the plugin's Fetch (state=open) — see
  // ADR-001 for why anyField must be set here independently of the three
  // UserModifiedFields-gated blocks above.
  if existing.ExternalURL == "" && data.ExternalURL != "" {
      update.ExternalURL = &data.ExternalURL
      anyField = true
  }
  ```
- Do NOT nest this inside any `if !containsField(...)` block — it must be structurally independent so it fires even when all three other fields are user-modified (ADR-001, Decision 1).
- Files: `session/backlog_sync.go`

##### Task 3.1.1b: Extend `fakeSyncPlugin.MapToBacklogItem` to map `URL` → `ExternalURL` (~2 min)
- In `session/backlog_sync_test.go`, in `fakeSyncPlugin.MapToBacklogItem` (lines 38-47), add `ExternalURL: item.URL,` to the returned `BacklogItemData` literal, mirroring the real plugins. This is additive and backward-compatible: existing tests that don't set `ExternalItem.URL` will still get `ExternalURL == ""`, so `TestSyncOne_SkipsWhenAllFieldsAreUserModified` (line 204) continues to pass unchanged (its fixture has no `URL` set, and the created item also has no `ExternalURL`, so the new condition's `data.ExternalURL != ""` check is false and `anyField` is untouched by it).
- Files: `session/backlog_sync_test.go`

#### Story 3.1.2: Regression tests for the backfill (positive, negative, all-fields-locked)
**As a** developer, **I want** tests pinning the backfill's exact behavior, **so that** a future refactor of `anyField`/`SyncOne` can't silently regress AC6.
**Acceptance Criteria**: (covered by Story 3.1.1's GWT examples above — this story is the test-writing task)
**Files**: `session/backlog_sync_test.go`

##### Task 3.1.2a: Add `TestSyncOne_BackfillsExternalURLOnExistingItem` (~4 min)
- In `session/backlog_sync_test.go`, add a new test near `TestSyncOne_LocalWinsSkipsUserModifiedFields` (line 156): create an item via `storage.CreateBacklogItem` with `ExternalID: "1"`, `ExternalURL: ""`, no `UserModifiedFields` set; configure `fakeSyncPlugin` to return `ExternalItem{ExternalID: "1", Title: "T", URL: "https://github.com/acme/widget/issues/1"}`; run `sl.SyncOne`; assert `refetched.ExternalURL == "https://github.com/acme/widget/issues/1"` and `events[0].ItemsUpdated == 1`.
- Files: `session/backlog_sync_test.go`

##### Task 3.1.2b: Add `TestSyncOne_BackfillsExternalURLEvenWhenAllOtherFieldsAreUserModified` (~4 min)
- In `session/backlog_sync_test.go`, add a new test modeled on `TestSyncOne_SkipsWhenAllFieldsAreUserModified` (line 204) but with `ExternalURL: ""` on the created item and the plugin's `ExternalItem` including a `URL`; after `SetUserModifiedFields('["title","description","priority"]')`, run `sl.SyncOne`; assert `events[0].ItemsUpdated == 1` (NOT `ItemsSkipped`) and `refetched.ExternalURL` equals the plugin's URL, while `refetched.Title == "Local"` (still locked). This is the exact regression case ADR-001 calls out.
- Files: `session/backlog_sync_test.go`

##### Task 3.1.2c: Add `TestSyncOne_DoesNotBackfillExternalURLForItemsNotInFetchResult` (~3 min)
- In `session/backlog_sync_test.go`, add a new test: create two existing items (`ExternalID: "1"` and `ExternalID: "2"`), both with `ExternalURL: ""`; configure `fakeSyncPlugin` to return `Fetch` results only for `ExternalID: "1"` (simulating item 2's issue having been closed and dropped from the `state=open` Fetch); run `sl.SyncOne`; assert item 1's `ExternalURL` is backfilled and item 2's `ExternalURL` remains `""`. Add a code comment referencing this as the documented AC6 limitation, not a bug.
- Files: `session/backlog_sync_test.go`

##### Task 3.1.3: Run package tests for Phase 3 (~2 min)
- Run: `go build ./session/... && go test ./session/... -run TestSyncOne`
- Files: none (verification only)

---

## Phase 4: Prompt Rendering (AC3, AC4, AC5)

### Epic 4.1: `closingKeywordFor` + fact/instruction line rendering
**Goal**: `BuildSessionInitialPrompt` renders the linked-issue fact line and a deterministic closing-keyword instruction line whenever `ExternalURL` is non-empty, with the fact line inside and the instruction line outside the existing inert-data boundary (ADR-001, Decision 2). No changes needed to `BuildTokenBudgetedPrompt` or `WriteBacklogContextFile` — both inherit automatically.

#### Story 4.1.1: Add `closingKeywordFor` and `githubShortRefFor` helpers
**As a** developer, **I want** pure functions mapping a linked-issue URL to (a) its fully-punctuated GitHub closing-keyword prefix and (b) the actual `owner/repo#N` reference GitHub's closing-keyword parser recognizes, **so that** the agent is told deterministically (never left to inference) both what keyword to write and what reference will actually trigger GitHub's auto-close/cross-reference behavior.

**IMPORTANT — corrects a confirmed defect found during sdd:4-validate's pre-mortem**: GitHub's documented closing-keyword syntax (https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue, confirmed via direct fetch on 2026-07-25) recognizes only two reference forms: `KEYWORD #N` (same-repo) or `KEYWORD OWNER/REPO#N` (cross-repo) — **a bare full URL like `https://github.com/acme/widget/issues/42` is NOT a documented/recognized closing-keyword reference form**. The original plan (now corrected below) rendered the instruction line as `Fixes <full-url>`, which would silently fail to trigger GitHub's auto-close on merge — passing every test and AC in this plan while not achieving the feature's actual purpose. The fix: derive the `owner/repo#N` short reference from `ExternalURL` and use that (not the raw URL) as what follows the keyword in the instruction line. The fact line (Task 4.1.2a) is unaffected — it's human-readable context, not parsed by GitHub, so it still shows the full URL.

**Acceptance Criteria**:
- AC3 (part 1): `closingKeywordFor` returns the correct, fully-punctuated keyword for each URL shape — the returned string is used directly by the caller with no added punctuation, so it must match AC3's literal wording exactly, trailing space included.
  - *Given* `"https://github.com/acme/widget/issues/42"`, *When* `closingKeywordFor(url)` is called, *Then* it returns `"Fixes "` (trailing space, no colon).
  - *Given* `"https://github.com/acme/widget/pull/17"`, *When* `closingKeywordFor(url)` is called, *Then* it returns `"Related: "` (trailing colon+space).
  - *Given* `""` (empty, though callers already gate on non-empty per AC4), *When* `closingKeywordFor(url)` is called, *Then* it returns `"Related: "` (safe default) without panicking.
  - *Given* an unrecognized shape like `"https://example.com/foo"`, *When* `closingKeywordFor(url)` is called, *Then* it returns `"Related: "` (safe fallback).
- AC3 (part 1b, new): `githubShortRefFor` derives the GitHub-recognized `owner/repo#N` reference from the same URL.
  - *Given* `"https://github.com/acme/widget/issues/42"`, *When* `githubShortRefFor(url)` is called, *Then* it returns `"acme/widget#42"`.
  - *Given* `"https://github.com/acme/widget/pull/17"`, *When* `githubShortRefFor(url)` is called, *Then* it returns `"acme/widget#17"`.
  - *Given* a malformed/non-GitHub URL like `"https://example.com/foo"`, *When* `githubShortRefFor(url)` is called, *Then* it returns the input unchanged (safe fallback — never panics; the rendered instruction line will just be less precise for a URL shape that can't occur from either real plugin today).
**Files**: `session/backlog_context.go`, `session/backlog_context_test.go`

##### Task 4.1.1a: Implement `closingKeywordFor` and `githubShortRefFor` (~4 min)
- In `session/backlog_context.go`, add next to `BuildSessionInitialPrompt` (before line 71):
  ```go
  // closingKeywordFor returns the fully-punctuated GitHub auto-close/reference
  // instruction prefix implied by a linked issue/PR URL's shape — exactly as
  // worded in requirements AC3 ("Fixes " for issues, "Related: " for PRs),
  // trailing space/colon-space included. Deterministic — never left to agent
  // inference, per requirements AC3. GitHub only auto-closes issues (not
  // PRs) via these keywords, so "Related: " is used for /pull/ URLs and as
  // the safe fallback for any unrecognized shape. Returning the punctuation
  // here (rather than a bare keyword) removes the punctuation-assembly
  // responsibility from the caller entirely — the caller concatenates this
  // return value directly with githubShortRefFor's output, no separator added.
  func closingKeywordFor(url string) string {
      switch {
      case strings.Contains(url, "/issues/"):
          return "Fixes "
      case strings.Contains(url, "/pull/"):
          return "Related: "
      default:
          return "Related: "
      }
  }

  // githubShortRefFor extracts the "owner/repo#N" reference GitHub's
  // closing-keyword parser actually recognizes from a GitHub issue/PR HTML
  // URL (https://github.com/{owner}/{repo}/issues|pull/{n}). GitHub's
  // closing keywords (Fixes/Closes/Resolves) only recognize "#N" (same-repo)
  // or "owner/repo#N" (cross-repo) — never a bare full URL — confirmed
  // against GitHub's docs. Falls back to returning url unchanged if it
  // doesn't match the expected shape (never panics).
  func githubShortRefFor(url string) string {
      trimmed := strings.TrimPrefix(strings.TrimPrefix(url, "https://github.com/"), "http://github.com/")
      parts := strings.Split(trimmed, "/")
      if len(parts) >= 4 && (parts[2] == "issues" || parts[2] == "pull") {
          return fmt.Sprintf("%s/%s#%s", parts[0], parts[1], parts[3])
      }
      return url
  }
  ```
- Files: `session/backlog_context.go`

##### Task 4.1.1b: Add `closingKeywordFor` and `githubShortRefFor` table tests (~4 min)
- In `session/backlog_context_test.go`, add table-driven tests covering:
  - `closingKeywordFor`: `/issues/` URL → `"Fixes "` (trailing space, no colon), `/pull/` URL → `"Related: "` (trailing colon+space), empty string → `"Related: "`, unrecognized shape → `"Related: "`. Use exact equality (`assert.Equal`), not a loose substring check.
  - `githubShortRefFor`: `"https://github.com/acme/widget/issues/42"` → `"acme/widget#42"`; `"https://github.com/acme/widget/pull/17"` → `"acme/widget#17"`; a malformed URL → returned unchanged.
- Files: `session/backlog_context_test.go`

#### Story 4.1.2: Render the fact line inside, and the instruction line outside, the inert-data boundary
**As an** agent working an imported backlog item, **I want** the prompt to tell me the linked issue URL and exactly what closing keyword to use, **so that** my PR auto-closes the issue (or cross-references the PR) without me having to guess.
**Acceptance Criteria**:
- AC3 (part 2): both lines render when `ExternalURL` is non-empty.
  - *Given* an `ent.BacklogItem` with `ExternalURL: "https://github.com/acme/widget/issues/42"`, *When* `BuildSessionInitialPrompt(item, nil)` is called, *Then* the output contains `"Linked GitHub Issue/PR: https://github.com/acme/widget/issues/42"` (the full URL, for human-readable context) somewhere between `"## Acceptance Criteria"` and `"--- END BACKLOG ITEM DATA ---"`, AND contains the exact literal substring `"Fixes acme/widget#42"` (the GitHub-recognized `owner/repo#N` short reference, via `githubShortRefFor` — NOT the full URL, per Story 4.1.1's corrected design) somewhere AFTER `"--- END BACKLOG ITEM DATA ---"`.
- AC4: no `ExternalURL` → output unchanged from today.
  - *Given* an `ent.BacklogItem` with `ExternalURL: ""`, *When* `BuildSessionInitialPrompt(item, nil)` is called, *Then* the output contains neither `"Linked GitHub Issue/PR"` nor `"Fixes"`/`"Related:"` closing-keyword text, and is byte-for-byte identical to the pre-change output for the same item (verified by the existing `TestBuildSessionInitialPrompt_ContainsTaskProtocolBlock` and `TestBuildSessionInitialPrompt_WithPriorAttempts_ContainsHandoffSection` tests continuing to pass unmodified).
**Files**: `session/backlog_context.go`, `session/backlog_context_test.go`

##### Task 4.1.2a: Add the fact line inside the inert-data block (~2 min)
- In `session/backlog_context.go`, in `BuildSessionInitialPrompt` (lines 72-129), add immediately after the `"## Acceptance Criteria"` section (after line 89, `sb.WriteString("\n")`) and before the `if item.Notes != ""` block (line 91):
  ```go
  if item.ExternalURL != "" {
      fmt.Fprintf(&sb, "\nLinked GitHub Issue/PR: %s\n", item.ExternalURL)
  }
  ```
- The rendered text must literally contain `"Linked GitHub Issue/PR: "` immediately followed by the URL, matching AC3's exact phrasing — do not wrap it in its own `##` heading, to keep this to exactly one rendered line (plus the guard) and minimize token-budget shift per the pitfalls research.
- Files: `session/backlog_context.go`

##### Task 4.1.2b: Add the instruction line outside the inert-data block (~3 min)
- In `session/backlog_context.go`, in `BuildSessionInitialPrompt`, add immediately after `sb.WriteString("--- END BACKLOG ITEM DATA ---\n\n")` (line 118) and before the `if item.PlanArtifactsPath != ""` block (line 120):
  ```go
  if item.ExternalURL != "" {
      fmt.Fprintf(&sb, "This item is linked to %s. When you open your PR, include the line `%s%s` in the PR body so GitHub cross-references (and, for issues, auto-closes) it.\n\n",
          item.ExternalURL, closingKeywordFor(item.ExternalURL), githubShortRefFor(item.ExternalURL))
  }
  ```
- **Two distinct things follow the keyword here, do not conflate them**: the human-readable sentence still mentions the full URL (`item.ExternalURL`, for the agent's/reviewer's own context), but the actual backtick-quoted line-to-include-in-the-PR-body uses `githubShortRefFor(item.ExternalURL)` (e.g. `acme/widget#42`), NOT the raw URL — because GitHub's closing-keyword parser does not recognize a bare URL (confirmed via GitHub's docs, see Story 4.1.1). Rendered example: `` This item is linked to https://github.com/acme/widget/issues/42. When you open your PR, include the line `Fixes acme/widget#42` in the PR body... ``
- The format verb is `%s%s` (not `%s: %s`) for the keyword+ref concatenation — `closingKeywordFor` returns the fully-punctuated prefix (`"Fixes "` / `"Related: "`) directly, so no separate colon is added.
- Keep this to one instruction sentence to minimize token-budget shift (pitfalls research: new section should stay ~2 short lines total across both fact + instruction additions).
- Files: `session/backlog_context.go`

##### Task 4.1.2c: Add prompt-rendering tests for fact/instruction line present and absent (~4 min)
- In `session/backlog_context_test.go`, add: (1) a test with `ExternalURL: "https://github.com/acme/widget/issues/42"` asserting the fact line (full URL) is present before the boundary and, using the **exact literal substring** (not a loose `Contains(..., "Fixes")`), `assert.Contains(t, prompt, "Fixes acme/widget#42")` appears after `"--- END BACKLOG ITEM DATA ---"`; (2) a test with `ExternalURL: "https://github.com/acme/widget/pull/17"` asserting `assert.Contains(t, prompt, "Related: acme/widget#17")` appears after the boundary; (3) a test with `ExternalURL: ""` asserting neither line appears (AC4). Asserting the exact literal `owner/repo#N` substring (rather than just `Contains(..., "Fixes")` or the raw URL) is required so a future regression that reverts to rendering the un-recognized full URL, or reintroduces a colon after `Fixes`, is caught by this test.
- Files: `session/backlog_context_test.go`

#### Story 4.1.3: Confirm `BuildTokenBudgetedPrompt`'s truncation passes still include both lines (AC5)
**As a** developer, **I want** confidence that token-budget truncation doesn't silently drop the linked-issue content, **so that** AC5 holds even for long items.
**Acceptance Criteria**:
- AC5: both truncation passes still include the fact line and instruction line.
  - *Given* an `ent.BacklogItem` with `ExternalURL: "https://github.com/acme/widget/issues/42"` and a `Description` long enough (and enough prior sessions) to exceed the 4000-estimated-token budget on the first pass, *When* `BuildTokenBudgetedPrompt(item, priorSessions)` is called, *Then* the returned prompt (after either the "drop prior sessions" pass or the "truncate description to 500 chars" pass) still contains `"Linked GitHub Issue/PR: https://github.com/acme/widget/issues/42"` and the `"Fixes acme/widget#42"` instruction line — because both passes call `BuildSessionInitialPrompt` (line 143 with `nil` priorSessions, line 152 with a shallow-copied `truncatedItem`) which unconditionally includes the new sections whenever `ExternalURL != ""`, and `ExternalURL` is a plain string value (no aliasing risk in the `truncatedItem := *item` shallow copy at line 150).
**Files**: `session/backlog_context_test.go`

##### Task 4.1.3a: Add token-budget truncation test covering the new sections (~4 min)
- In `session/backlog_context_test.go`, add a test constructing an `ent.BacklogItem` with `ExternalURL` set and a `Description` long enough to force `BuildTokenBudgetedPrompt` past its first truncation pass (mirror however existing tests in this file construct long descriptions, or use `strings.Repeat` to build a >16000-char description so `len(output)/4 > 4000`); assert the returned prompt still contains the fact line and instruction line text.
- Files: `session/backlog_context_test.go`

##### Task 4.1.4: Run package tests for Phase 4 (~2 min)
- Run: `go build ./session/... && go test ./session/... -run 'TestBuildSessionInitialPrompt|TestBuildTokenBudgetedPrompt|TestClosingKeywordFor|TestRenderBacklogContextFile'`
- Files: none (verification only)

---

## Phase 5: Service Call Sites (AC7)

### Epic 5.1: `server/services/backlog_service.go` literals + integration test
**Goal**: Both hand-built `ent.BacklogItem{}` literals include `ExternalURL`, and an integration test proves the real spawned session's prompt contains the linked-issue section end-to-end.

#### Story 5.1.1: Add `ExternalURL` to both `ent.BacklogItem{}` literals
**As a** developer, **I want** `SpawnSessionFromItem` and `AttachSessionToItem` to pass `ExternalURL` into the ent struct used for prompt-building, **so that** the linked-issue section actually reaches spawned/attached sessions' prompts.
**Acceptance Criteria**:
- AC7 (part 1): both literals include `ExternalURL`.
  - *Given* a `BacklogItemData` with `ExternalURL: "https://github.com/acme/widget/issues/42"` loaded in `SpawnSessionFromItem` (`server/services/backlog_service.go:1076-1097`), *When* the `entItem := &ent.BacklogItem{...}` literal is constructed, *Then* it includes `ExternalURL: item.ExternalURL,` and the subsequent `session.BuildTokenBudgetedPrompt(entItem, priorSessions)` call (line 1098) produces a prompt containing the linked-issue fact and instruction lines.
**Files**: `server/services/backlog_service.go`

##### Task 5.1.1a: Add `ExternalURL` to the `SpawnSessionFromItem` literal (~2 min)
- In `server/services/backlog_service.go`, in the `entItem := &ent.BacklogItem{...}` literal inside `SpawnSessionFromItem` (lines 1086-1097), add `ExternalURL: item.ExternalURL,` (e.g. after `Notes: item.Notes,` at line 1093).
- Files: `server/services/backlog_service.go`

##### Task 5.1.1b: Add `ExternalURL` to the `AttachSessionToItem` literal (~2 min)
- In `server/services/backlog_service.go`, in the `entItem := &ent.BacklogItem{...}` literal inside the attach-flow (lines 1229-1240), add `ExternalURL: item.ExternalURL,` (e.g. after `Notes: item.Notes,` at line 1236).
- Files: `server/services/backlog_service.go`

**Known follow-ups (not addressed in this PR, flagged for awareness, not blockers)**:
- `backlogItemToProto` (`server/services/backlog_service.go:387-405`) is deliberately **not** extended with `ExternalURL` in this PR. This is a conscious scope boundary, not an oversight: no AC requires web UI/API exposure of the linked-issue URL, and the feature is scoped to agent sessions and PRs. A future PR can add `ExternalURL` to the proto/API surface if product wants it rendered in the web UI.
- The two hand-built `ent.BacklogItem{}` literals (`SpawnSessionFromItem` and `AttachSessionToItem`, edited above) remain a manual-lockstep risk: nothing enforces the two stay in sync for future field additions (this is in fact how `ExternalID` was historically missed from both). Not fixed in this PR — disproportionate for a 9-AC plumbing change — but flagged here so the next field addition doesn't repeat the same omission pattern.

#### Story 5.1.2: Integration test proving the spawned session's prompt contains the linked-issue section
**As a** developer, **I want** an end-to-end test through the real `SpawnSessionFromItem` handler, **so that** AC7 is proven at the boundary an agent session actually sees, not just at the ent-struct level.
**Acceptance Criteria**:
- AC7 (part 2): integration-level proof.
  - *Given* a real backlog item created via `storage.CreateBacklogItem(ctx, BacklogItemData{Title: "zzyzx widget", Status: string(session.BacklogStatusReady), SkipPlanning: true, RepoPath: t.TempDir(), ExternalURL: "https://github.com/acme/widget/issues/42"})` (status pre-set to `"ready"` and `SkipPlanning: true` so the planning gate at `server/services/backlog_service.go:1051-1054` is bypassed without needing the full triage/approve flow), and a `mockSessionCreator` wired via `NewBacklogService`, *When* `svc.SpawnSessionFromItem(ctx, &sessionv1.SpawnSessionFromItemRequest{ItemId: itemID})` is called, *Then* `creator.calls[0].prompt` contains `"Linked GitHub Issue/PR: https://github.com/acme/widget/issues/42"` (fact line, full URL) and the exact literal substring `"Fixes acme/widget#42"` (instruction line, GitHub-recognized short reference via `githubShortRefFor` — NOT the raw URL, per Story 4.1.1's corrected design) — not a loose `Contains(..., "Fixes")`, so a regression reintroducing either the raw URL or a colon is caught here too.
**Files**: `server/services/backlog_service_test.go`

##### Task 5.1.2a: Write `TestSpawnSessionFromItem_PromptContainsLinkedIssueSection` (~5 min)
- In `server/services/backlog_service_test.go`, add a new test near `TestBacklogFullLifecycle_TriageApprovalSpawn_CarriesRealPromptContent` (line 447), but simpler: skip the triage/approve flow entirely by creating the item directly via `storage.CreateBacklogItem` with `Status: string(session.BacklogStatusReady)`, `SkipPlanning: true`, `RepoPath: t.TempDir()`, and `ExternalURL: "https://github.com/acme/widget/issues/42"` set. Wire a `mockSessionCreator` (pattern at line 92/142) via `NewBacklogService(storage, creator, nil, nil)`. Call `svc.SpawnSessionFromItem` with `Autonomous: false` (no need to exercise the autonomous-driver path for this test). Assert `require.Len(t, creator.calls, 1)`, then `assert.Contains(t, creator.calls[0].prompt, "Linked GitHub Issue/PR: https://github.com/acme/widget/issues/42")` (fact line, full URL) and `assert.Contains(t, creator.calls[0].prompt, "Fixes acme/widget#42")` (instruction line, short reference — not the raw URL, not a loose `Contains(..., "Fixes")`). Also add a second, PR-URL variant of this test (or a table-driven case) with `ExternalURL: "https://github.com/acme/widget/pull/17"` asserting `assert.Contains(t, creator.calls[0].prompt, "Related: acme/widget#17")`, so the PR-keyword branch is explicitly covered at the integration level too.
- Files: `server/services/backlog_service_test.go`

##### Task 5.1.2b: Add the `AttachSessionToItem` integration test (~4 min) — closes sdd:4-validate Gap 3
- In `server/services/backlog_service_test.go`, add `TestAttachSessionToItem_PromptContainsLinkedIssueSection`, modeled on the existing `TestAttachSessionToItem_WritesContextFileWithPlanArtifactsAndPriorSessions` test (line 545): create an item with `ExternalURL` set, call `svc.AttachSessionToItem`, and assert the written context/prompt content contains the fact and instruction lines. This is required because Task 5.1.1b's literal has no dedicated test otherwise — the plan's own "Known follow-ups" note flags these two hand-built literals as a manual-lockstep risk precisely because an omission like this (as happened historically with `ExternalID`, present in neither literal before this feature) is otherwise silent.
- Files: `server/services/backlog_service_test.go`

##### Task 5.1.3: Run package tests for Phase 5 (~2 min)
- Run: `go build ./server/... && go test ./server/services/... -run 'TestSpawnSessionFromItem|TestAttachSessionToItem|TestBacklogFullLifecycle'`
- Files: none (verification only)

---

## Phase 6: Final Verification (AC8, AC9)

### Epic 6.1: Full build + test pass, fallout fixes
**Goal**: Confirm the entire change set builds and all tests pass, with no compile errors anywhere in `session/ent/*` (confirming `--feature sql/upsert` was not omitted), and all pre-existing tests still pass unmodified.

#### Story 6.1.1: Run `make build && make test` and fix any fallout
**Acceptance Criteria**:
- AC8: all existing tests in `backlog_plugin_github_test.go` and `backlog_context_test.go` continue to pass.
  - *Given* the full test suite as modified by Phases 1-5, *When* `go test ./session/... ` is run, *Then* every pre-existing test function in both files (e.g. `TestGitHubIssuesPlugin_Fetch_ParsesIssuesAndComputesPriority`, `TestBuildSessionInitialPrompt_ContainsTaskProtocolBlock`) passes with no changes to their assertions.
- AC9: `make build && make test` exits 0 for `session` and `server/services`, no compile errors in `session/ent/*`.
  - *Given* the complete implementation, *When* `make build && make test` is run from repo root, *Then* the command exits 0, and `go vet ./session/ent/...` reports no errors (confirming the ent codegen step in Task 1.1.1b used `--feature sql/upsert` correctly).
**Files**: none (verification task — fixes, if any, land in whichever file the failure points to)

##### Task 6.1.1a: Run the full build and test suite (~5 min)
- Run: `make build && make test`
- If any failure surfaces, diagnose root cause (per CLAUDE.md's engineering discipline: state the root-cause hypothesis before fixing) and fix in the specific file the failure implicates — do not guess broadly.
- Files: whichever file(s) a failure implicates (expected: none, if Phases 1-5 were followed exactly)

##### Task 6.1.1b: Confirm no regressions in `session/ent` package specifically (~2 min)
- Run: `go vet ./session/ent/... && go build ./session/ent/...`
- Files: none (verification only)

**Resolved during sdd:4-validate (was a P1 pre-mortem item, no longer an open follow-up)**: the original plan rendered the instruction line as `Fixes <full-url>`, an assumption that GitHub's live closing-keyword parser accepts a bare URL. This was checked directly against GitHub's documentation (https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue, fetched 2026-07-25): **a bare full URL is not a recognized closing-keyword reference form** — only `#N` (same-repo) or `owner/repo#N` (cross-repo) are documented as valid. Story 4.1.1/Task 4.1.1a now derive and render the `owner/repo#N` short reference (`githubShortRefFor`) instead of the raw URL, so the instruction line will actually trigger GitHub's auto-close/cross-reference behavior on merge, matching documented syntax exactly. A remaining, much narrower manual-verification recommendation: after shipping, open one real test PR referencing a real linked issue with the exact rendered text and confirm the auto-close fires — this covers residual risk from any GitHub behavior not captured in public docs (e.g. cross-repo/fork edge cases), not the core syntax question, and is not a merge blocker.
