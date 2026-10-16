# Implementation Plan: backlog-configurable-pipeline

**Feature**: Per-backlog-item, runtime-definable pipeline modes (which slash-commands/prompts drive triage, work, and review) via a new DB-persisted `PipelineMode` table, a `PipelineEngine` seam consulted at the three (four) hot-path call sites, and UI to select/manage/inspect modes.
**Date**: 2026-07-15
**Status**: Phases 1-3 shipped (`8affe06cc`, `6c77f3a27`, `7e542c27b`); bucket-3 gap confirmed substantially resolved as of 2026-08-21 audit — see `docs/tasks/backlog-feature-improvement.md`.
**ADRs**: `project_plans/backlog-configurable-pipeline/decisions/ADR-001-pipeline-mode-db-persisted.md`

---

## Step 0.5 — Alternatives Explored

1. **Closed Go-code registry** (`map[string]PipelineEngine`, mirroring `session/backlog_plugin.go`'s `PluginRegistry`). *Strength*: zero persistence/caching design needed — free, in-memory, safest against malformed content. *Weakness*: every new mode requires a PR + `make install-service` deploy, which fails the requirement's decisive success metric ("no engineering involvement" to add a mode). **Rejected** — see ADR-001.
2. **DB-persisted modes, queried per-call** (mirror `WorkflowRepository` exactly, no cache). *Strength*: least new code — copy an existing, working pattern verbatim. *Weakness*: adds a synchronous DB read to `TriggerTriage`/`WriteSlashCommands`/`ReviewGateRunner.Run`, directly violating the NFR and repeating ADR-013's already-rejected "Alt B". **Rejected**.
3. **DB-persisted modes with an explicit copy-on-write in-process cache**, empty-string mode short-circuiting the cache/DB entirely. *Strength*: satisfies both the runtime-configurability requirement and the no-uncached-DB-read NFR; the default (99% common) path touches neither cache nor DB. *Weakness*: genuinely new design work — no existing precedent for the caching layer specifically (only for schema/CRUD shape). **Selected** — the weakness is accepted and budgeted for explicitly in Phase 1, Epic 1.3.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `PipelineMode` (ent entity) | A DB row: a named, slug-addressed, user-creatable definition of which slash-commands and prompt content a backlog item's pipeline uses. | New. `session/ent/schema/pipeline_mode.go`. Table `pipeline_modes`. Mirrors `session.Workflow`'s slug/name/description/enabled shape — not the same type. |
| `PipelineModeSlug` | The string identifier of a `PipelineMode` (e.g. `"quick"`, `"full"`). Stored on `BacklogItemData.PipelineMode` as a plain `string`; `""` means "no mode chosen — use built-in default." | Not a wrapped Go type — plain `string`, following the same convention as `BacklogStatus`'s sibling fields being plain strings validated at the Go layer, not `field.Enum`. |
| `PipelineModeDefault` | The Go constant `PipelineMode = ""` (empty string). Resolving this value never touches the cache or the DB — it dispatches straight to the pre-existing hardcoded functions. | `session/pipeline_engine.go`. This is the concrete mechanism that keeps the NFR ("no uncached DB read on the hot path for the common case") true by construction. |
| `PipelineEngine` | The narrow interface (5 methods) that `WriteSlashCommands`, triage-prompt building, review-prompt building, initial-prompt building, and mode-content-hash lookup (`ContentHashFor`, used only by the Epic 1.6 snapshot call sites) consult instead of calling the hardcoded functions directly. | `session/pipeline_engine.go`. Sibling of `WorkflowEngine`, not a wrapper around it — see Pattern Decisions. |
| `CachingPipelineEngine` | The single concrete implementation of `PipelineEngine`. Resolves `PipelineModeDefault` for free (no lookup); resolves any other slug via `pipelineModeCache`; falls back to default behavior + a Warn log on any unresolvable/malformed slug. | `session/pipeline_engine.go`. Constructed once via `NewPipelineEngine(repo PipelineModeRepository) *CachingPipelineEngine`, shared by `BacklogService` and `BacklogLifecycleListener`. |
| `pipelineModeCache` | An in-process, copy-on-write cache (`atomic.Pointer[map[string]resolvedPipelineMode]`) populated from `PipelineModeRepository.ListEnabled` at construction, replaced wholesale on `Invalidate`. | `session/pipeline_engine.go`. Read path (`Get`) is a single atomic load + map lookup — no locks, no per-call DB I/O. The write path (`Load`/`Invalidate`) serializes its DB-read + `Store` sequence behind an internal `sync.Mutex` so two concurrent `Invalidate` calls can't lost-update each other (see Story 1.3.2) — this mutex is never held by `Get`. |
| `resolvedPipelineMode` | An unexported, immutable snapshot struct (slug, name, the 9 rendered-template-source fields, and `ContentHash`) held inside the cache. Deep-copied from `*ent.PipelineMode` on load so concurrent readers never see a partially-updated ent object during a cache swap. `ContentHash` is computed once at load time (SHA-256 hex, truncated to 16 chars, over the 9 raw content-template field values in fixed order) — see `ItemSessionSummary.PipelineModeSnapshotHash` below. | `session/pipeline_engine.go`. Mirrors `NewDefaultWorkflowEngine`'s deep-copy-on-construct discipline. |
| `PipelineModeRepository` | The persistence interface for `PipelineMode` CRUD: `Create/Update/Delete/GetByID/GetBySlug/ListAll/ListEnabled`. | `session/pipeline_mode_repository.go`. Interface shape mirrors `WorkflowRepository` exactly (same method names/shape) per the Constraints section of requirements.md. |
| `EntPipelineModeRepository` | The ent-backed implementation of `PipelineModeRepository`. | `session/ent_pipeline_mode_repository.go`. Mirrors `EntWorkflowRepository`. |
| `PipelineModeCreateInput` / `PipelineModeUpdateInput` | Plain structs for repository Create/Update calls; `Update` uses `*T` pointer fields for partial-update semantics (only non-nil fields are applied). | Mirrors `WorkflowCreateInput`/`WorkflowUpdateInput`. |
| content-template field | One of 9 typed `string` columns on `PipelineMode` (`StatusCommandTemplate`, `DoneCommandTemplate`, `FailCommandTemplate`, `ReviewCommandTemplate`, `ShipCommandTemplate`, `HelpCommandTemplate`, `TriagePromptTemplate`, `ReviewPromptTemplate`, `InitialPromptTemplate`). Each supports a small fixed set of `{{placeholder}}` substitutions, never a general templating DSL. | This is the concrete answer to the NFR's "structured, not a single free-text blob" requirement. See Migration Plan. |
| `renderTemplate` | The unexported function that performs the fixed-placeholder substitution (`strings.NewReplacer`, not `text/template`) on a content-template field. Deliberately not Turing-complete — no conditionals/loops — to resist the "templating engine" rabbit hole flagged in requirements.md. Phase 1 (Task 1.3.3d) leaves unrecognized `{{...}}` tokens un-substituted as a temporary, Phase-1-only passthrough; Phase 2 (Story 2.3.1) supersedes this with write-time allow-list rejection, so by the time Phase 2 ships, every persisted template field is guaranteed to contain only recognized placeholder names. | `session/pipeline_engine.go`. Recognized allow-list (shared by `renderTemplate` and `ValidatePipelineModeContent`): `item_id`, `item_title`, `item_description`, `criteria_index`, `criteria_count`, `criteria_text`, `repo_path`. |
| `SlashCommandSet` | `PipelineEngine` method: `SlashCommandSet(item *BacklogItemData) (map[string]string, error)` — returns the filename→rendered-content map that `WriteSlashCommands` writes to `.claude/commands/backlog/`. | Replaces the hardcoded block in `session/backlog_commands.go`. |
| `TriagePromptFor` | `PipelineEngine` method: builds the headless-triage prompt for a given mode. Delegates to `BuildHeadlessTriagePrompt` for the default mode. | `session/pipeline_engine.go`. |
| `ReviewPromptFor` | `PipelineEngine` method: builds the headless-review prompt for a given mode. Delegates to `BuildHeadlessReviewPrompt` for the default mode. | `session/pipeline_engine.go`. |
| `InitialPromptFor` | `PipelineEngine` method: builds the interactive/autonomous session's initial prompt (`inst.Prompt`). Delegates to `session.BuildTokenBudgetedPrompt`/`BuildSessionInitialPrompt` for the default mode. **4th of 5 methods** (see `ContentHashFor` below for the 5th) — see Pattern Decisions for why this exceeds the "1-3 methods" guidance. | `session/pipeline_engine.go`. This is the method that makes the seam behavior-changing for `AutonomousDriver`-backed sessions without touching `autonomous_driver.go` (per `research/architecture.md` §2). |
| `ContentHashFor` | `PipelineEngine` method: **5th method**, returns the content hash of a resolved mode's 9 raw content-template fields (or `("", false)` for the default mode or an unresolved slug). Used only by Epic 1.6's snapshot-write call sites. | `session/pipeline_engine.go`. Added to close architecture-review.md's Blocker 1 — see `ItemSessionSummary.PipelineModeSnapshotHash` above. |
| `BacklogItemData.PipelineMode` | New `string` field (default `""`) on the existing domain struct, holding the resolved item's chosen mode slug. | `session/repository.go:341` region. Plain `string` (not pointer) — mirrors `RepoPath`/`Status`, since a resident domain value always has a concrete (possibly empty) value. |
| `BacklogItemUpdate.PipelineMode` | New `*string` field on the existing partial-update struct — `nil` means "field omitted, don't touch"; non-nil `""` means "explicit reset to default." | `session/repository.go:417` region. Mirrors `SkipReviewGate *bool` etc., but as `*string` (proto `optional string`, not `bool`) specifically to close the proto3-bool-clobbering bug class at its source — see `research/pitfalls.md` §2. |
| `ItemSessionSummary.PipelineModeSnapshot` / `.PipelineModeSnapshotHash` | Two fields, both frozen at the moment a triage/work session first starts for an item. `PipelineModeSnapshot` is the resolved mode *slug* (protects against the item's own `pipeline_mode` field being reassigned later — case (a)). `PipelineModeSnapshotHash` is a SHA-256 (hex, truncated to 16 chars) digest of that mode's 9 raw content-template field values, concatenated in fixed field order, computed once at cache-load time and captured via `PipelineEngine.ContentHashFor` when the session starts (protects against the *referenced mode's own template content* being edited later — case (b), the harder DB-mutability-specific risk). Empty for `PipelineModeDefault` (code-backed content can't drift without a redeploy) and for an already-unresolved slug at session-start (nothing to hash). | `session/repository.go:283` region. Mirrors the existing `AcSnapshot` field's snapshotting discipline (`research/pitfalls.md` §4), but as two fields, not one — a slug-only snapshot protects against (a) only; the hash closes (b). See Migration Plan and Story 3.4.1 for the "what ran" UI's use of the hash. |
| "what ran" surface | The read-only UI panel in `BacklogItemDetail.tsx` showing which `PipelineModeSnapshot` value drove a given `ItemSession`, annotated `"(content since changed)"` when the live mode's current `content_hash` no longer matches the session's frozen `PipelineModeSnapshotHash`. | Reads `ItemSession.pipelineModeSnapshot`/`.pipelineModeSnapshotHash` (exposed via proto by Task 1.6.1c) and each `PipelineMode`'s live `content_hash` (exposed by Story 2.1.1) — never the item's live (possibly since-changed) `pipeline_mode` field. See Story 3.4.1. |
| `RadioGroup` (shared component) | A generalized, parameterized version of `OmnibarCreationPanel.tsx`'s hand-rolled `SessionTypeRadioGroup` (options array → ARIA radiogroup), extracted so the pipeline-mode selector doesn't duplicate the implementation. | `web-app/src/components/ui/RadioGroup.tsx` (new). Two a11y bugs present in the original are fixed during extraction — see Epic 3.1. |
| `PipelineModeOverridesSection` | The visually-grouped sub-section in `BacklogItemForm.tsx` containing the existing 3 checkboxes (`skipPlanning`, `skipReviewGate`, `autoSpawnSession`), relabeled as "Overrides" independent of the mode choice, per `research/ux.md` §2's compose-not-subsume UX recommendation. | `web-app/src/components/backlog/BacklogItemForm.tsx`. No new state — visual grouping only. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `PipelineEngine` persistence model | DB-persisted table + explicit in-process cache (copy-on-write) | `session.Workflow`/`WorkflowRepository` shape + new caching layer | Closed Go-code registry (`PluginRegistry` style) | Fails the "no engineering involvement to add a mode" success metric. See ADR-001. |
| `PipelineEngine` interface location | Defined in `session` package (shared domain layer), consumed by both `server/services.BacklogService` and `session.BacklogLifecycleListener`/`ReviewGateRunner` | `PipelineEngine`'s own two real cross-package consumers (see Reason) | Define in `server/services` (strict "consumer package" reading of the interface-pollution checklist) | `PipelineEngine` has two genuine, independently-verified consumers on opposite sides of the package boundary: `server/services.BacklogService` (Story 1.5.1b) and `session.ReviewGateRunner` (Story 1.5.1c). Defining it in `server/services` would force `session.ReviewGateRunner` — which lives in `session` itself — to import a "consumer" package for a seam it also consumes directly; defining it in `session` lets both consumers depend on the shared domain layer instead of on each other's package. (Note: `WorkflowEngine` is NOT a valid precedent for this reasoning — direct inspection confirms it has exactly one consumer in the entire codebase, `server/services/backlog_service.go`, and zero test doubles; it is itself an uncorrected instance of interface-pollution-checklist smells #1 and #2, not a pattern to cite. See architecture-review.md's Blocker 2 for the verification.) |
| `PipelineEngine` concurrency-safe caching | `atomic.Pointer[map[string]resolvedPipelineMode]` for reads, copy-on-write, swapped wholesale on invalidate; a `sync.Mutex` guards ONLY the write-side DB-read + `Store` sequence inside `Load`/`Invalidate` | go-concurrency idiom: atomic.Pointer copy-on-write for read-heavy/write-rare shared state, refined with a writer-side mutex to serialize the read-then-store sequence | `sync.RWMutex` guarding a live map | Reads (every triage/spawn/review call) vastly outnumber writes (operator edits a mode definition rarely); atomic.Pointer gives lock-free reads with no risk of readers blocking behind a writer, and a wholesale swap is simpler to reason about than mutating a shared map under a lock. The writer-side mutex is a necessary refinement, not a contradiction of "lock-free reads": without it, two concurrent `Invalidate` calls (e.g. a double-submitted edit from two browser tabs) can interleave their DB-read + `Store` steps such that the slower/older read's `Store` lands last, silently reverting the cache to stale data (a lost-update, not a torn read — `atomic.Pointer` alone does not prevent this). The mutex only ever blocks writer against writer; `Get` never touches it. |
| `PipelineMode` cache invalidation | Explicit `Invalidate(ctx)` call after every Create/Update/Delete/Enable/Disable RPC handler | `DetectorRegistry`'s `Register`/`Lookup` shape for the in-memory half | Short-TTL background refresh | Single-operator tool: writes are rare and always operator-initiated through the same process that also serves reads — invalidate-on-write gives immediate consistency with less code than a poll loop, and there's no multi-instance/multi-writer scenario where TTL staleness would matter. |
| `PipelineMode` content shape | 9 separate typed `string` columns (one per slash-command file + one per prompt), each with fixed-placeholder substitution only | requirements.md NFR ("structured... not a single unstructured free-text blob") + rabbit-hole warning against a templating DSL | Single JSON blob column (`map[string]string`) | A JSON blob is not meaningfully more "structured" than free text from the DB's point of view (no column-level typing/constraints) and would need its own ad hoc validation layer; separate columns get per-field `NOT NULL`/length constraints from ent directly and are trivially diffable in a migration review. |
| `PipelineEngine` method count | 5 methods (`SlashCommandSet`, `TriagePromptFor`, `ReviewPromptFor`, `InitialPromptFor`, `ContentHashFor`) | `research/architecture.md` §2's recommendation (a): add `InitialPromptFor` rather than leave autonomous-mode prompt content mode-unaware; `ContentHashFor` added to close architecture-review.md's Blocker 1 (content-drift protection for `ItemSessionSummary.PipelineModeSnapshotHash`) | 3 methods, defer `InitialPromptFor` to a follow-up; or a second, narrower interface just for `ContentHashFor` | requirements.md's Scope section suggests "1-3 methods," but `AutoSpawnSession`/autonomous mode are live, actively-used flags on the exact same code path this feature targets; shipping without `InitialPromptFor` would make the seam cosmetic for every autonomous-mode session (see `research/architecture.md` §2's "risk of ending up cosmetic" analysis). `ContentHashFor` is needed by the same two consumers (`BacklogService.SpawnSessionFromItem`/`TriggerTriage`, `ReviewGateRunner` indirectly via snapshot-on-first-session) that already hold `PipelineEngine` as an interface value — introducing a second, single-method interface just to keep a round method count would be exactly the kind of speculative interface-pollution `.claude/rules/interface-pollution-checklist.md` warns against, not less of it. Each of the 5 methods is one more get/render/hash call sharing the same resolve-and-cache mechanism (see architecture-review.md's Lens 1.1 Concern) — interface segregation is about avoiding unrelated responsibilities, not a strict method-count ceiling. Explicitly overriding the "1-3" guidance here, as instructed by the research file's own open question. |
| `PipelineEngine` ↔ `WorkflowEngine` relationship | Fully separate, sibling interfaces, both held as independent fields on `BacklogService`/`BacklogLifecycleListener`; no calls between them | `research/architecture.md` §1's "separate interface, composed by the caller" recommendation | Extend `WorkflowEngine` with pipeline methods, or have `PipelineEngine` call into `WorkflowEngine` | Disjoint call-site sets (state-transition legality vs. within-status content selection) and disjoint reasons to change; coupling them would pull ADR-013 Phase 2 (custom states) back into scope, which is explicitly out of scope here. |
| Mode vs. `Skip*`/`AutoSpawnSession` booleans | **Compose** — mode selection and the 3 existing booleans are independent; mode never auto-sets or overrides a checkbox | `research/features.md` §3's recommendation (option 2) | **Subsume** — mode fully replaces the 3 booleans | Composing is the lower-risk, ship-`DefaultPipelineEngine`-first-with-zero-regression path the Risk Control section requires; subsuming would force every existing item's implicit boolean state to be reinterpreted as a mode choice on migration day, which is unnecessary scope for this phase. |
| Mode mutability | Immutable-after-first-triage-session, via a resolved-mode snapshot on `ItemSessionSummary` | `research/pitfalls.md` §4, extending the existing `AcSnapshot` precedent | Always resolve live from the item's current `pipeline_mode` field | Since modes are now DB-mutable, resolving live would let an in-flight item's triage and review stages silently run under two different mode definitions (or definitions-as-they-existed-at-different-times) with no record of which was used — `AcSnapshot` already solves exactly this class of problem on the same struct. |
| Item-level `pipeline_mode` proto field | `optional string pipeline_mode = N;` (synthesized oneof, real wire presence), handler gated on presence not truthiness | `research/pitfalls.md` §2, closing the proto3-bool-clobbering bug class at its origin | Plain `string` (no `optional`), or a proto `enum PipelineMode` | Plain `string` repeats the exact bug class the `SkipReviewGate`/`SkipPlanning`/`AutoSpawnSession` incident already taught this codebase (omitted vs. explicit-empty are indistinguishable). `enum` requires a fixed compile-time value set, which directly contradicts runtime-definable modes (ADR-001). |
| `PipelineModeRepository` return type | Return `*ent.PipelineMode` directly from repository methods, no separate domain DTO | Mirrors `WorkflowRepository`'s existing (ent-coupled) return-type convention | Introduce a `PipelineModeData` domain struct decoupled from ent, mirroring `BacklogItemData` | No current consumer needs decoupling from ent for this type (only `pipelineModeCache`'s internal `resolvedPipelineMode` needs a stable, deep-copied snapshot, which is a separate internal type anyway); adding a parallel DTO here would be a translation layer with no behavior, which `.claude/rules/interface-pollution-checklist.md` explicitly flags as a smell (forwarding-only wrapper). |
| Pipeline-mode selector UI | Extract `SessionTypeRadioGroup` into a shared, parameterized `RadioGroup` component; reuse for pipeline-mode selection | `research/ux.md` §1's recommendation | Build a second, near-identical hand-rolled ARIA radiogroup | Zero session-specific logic is baked into `SessionTypeRadioGroup`'s rendering today; duplicating it would also duplicate its 2 known a11y gaps (missing `aria-labelledby`/`aria-describedby`) instead of fixing them once. |
| Management CRUD surface | Dedicated settings page (`/settings/pipeline-modes`), mirroring `/settings/backlog-sources` | requirements.md Scope + `web-app/src/app/settings/backlog-sources/page.tsx` precedent | Inline modal/drawer off the backlog board | A modal would need to duplicate `/settings`'s existing layout/nav chrome (`web-app/src/app/settings/layout.tsx`) for no benefit; a dedicated settings page is the established location for this class of "operator-only configuration, not a per-item action" feature (`/settings/backlog-sources` is the direct sibling). |

---

## Out of Scope (plan-level, in addition to requirements.md's Out of Scope section)
- `research/features.md` §4's "prospective preview" (a "what will run" static preview shown before triggering triage, distinct from Epic 3.4's retrospective "what ran" surface) was considered and explicitly deferred, not overlooked — it's a reasonable low-incremental-cost follow-up once Epic 3.4's retrospective surface and its underlying data (mode content, hash) exist, but is not part of this plan's Phase 3 scope.
- `design/ux.md`'s recommended Epic 3.5 (a compact pipeline-mode badge on `BacklogBoard.tsx`'s list-view cards) is deliberately deferred, NOT added as a new epic in this plan, contingent on both the Phase-0 adoption spike (Risk Control section) and Phase 4's real-usage proof (Epic 4.1/the ongoing adoption-signal check-in) succeeding — do not build it speculatively. Adding more UI scope before adoption is proven would compound the exact P1 adoption risk the Phase-0 gate exists to de-risk, not reduce it. If Phase 0 and Phase 4 both validate real usage, Epic 3.5 remains the cheap, low-risk follow-up `design/ux.md` describes and can be picked up then.

## Migration Plan

- **Migration file**: generated by `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` after adding `session/ent/schema/pipeline_mode.go` and the `pipeline_mode` field on `session/ent/schema/backlog_item.go` and the `pipeline_mode_snapshot` (slug) + `pipeline_mode_snapshot_hash` (content hash) fields on `session/ent/schema/item_session.go`. Ent auto-migrates on startup for this codebase (no separate `.sql` migration file checked in — confirmed by the absence of a `migrations/` directory alongside `session/ent/schema/`; `ent.Client.Schema.Create(ctx)` runs at boot). New table: `pipeline_modes` (columns: `id` uuid PK, `slug` string unique, `name` string, `description` string optional, `enabled` bool default true, `status_command_template` string, `done_command_template` string, `fail_command_template` string, `review_command_template` string, `ship_command_template` string, `help_command_template` string, `triage_prompt_template` string, `review_prompt_template` string, `initial_prompt_template` string, `created_at`/`updated_at` time). New columns on `backlog_items`: `pipeline_mode` string default `""`. New columns on `item_sessions`: `pipeline_mode_snapshot` string default `""` (resolved mode slug at session-start) and `pipeline_mode_snapshot_hash` string default `""` (SHA-256, hex, truncated to 16 chars, over that mode's 9 raw content-template field values at session-start — see Epic 1.6 and the Domain Glossary; there is no `pipeline_mode_snapshot_version` field — that name was a planning-stage placeholder that never made it into an actual epic and is corrected here).
- **Reversibility**: ent's auto-migrate is additive-only (new table, new nullable/defaulted columns) — no destructive change to existing tables/columns, so no down-migration is required for rollback; reverting the code (not the schema) is sufficient, and the new columns/table are simply unused by older code.
- **Zero-downtime strategy**: single-operator/single-instance deployment (`make install-service` restarts one systemd user service) — no rolling-deploy coordination needed. New columns default to `""`/`true` so pre-existing rows never contain `NULL` in a `NOT NULL`-equivalent field.
- **Rollback procedure**: `git revert` the schema-adding commit(s) and re-run `make install-service`; the `pipeline_modes` table and new columns are simply left orphaned in SQLite/the configured DB (no data loss, no destructive down-migration needed given the single-operator/no-multi-tenant context).

## Observability Plan
- **Logs**: `[PipelineEngine]`-prefixed (new prefix, following the `[BacklogLifecycle]`/`[TriggerTriage]` convention) — Info on every resolved mode at triage-start and review-start (`item=<id> mode=<slug-or-"default">`); Debug on cache load/invalidate/miss; Warn (never silent) whenever a stored `pipeline_mode` slug fails to resolve, including the item ID and the unresolved slug, on every one of the 4 call sites independently (not just once).
- **Metrics**: none required — single-operator tool, no oncall rotation, per requirements.md.
- **Alerts**: none required — same rationale.

## Risk Control
- **Feature flag**: none required for `PipelineEngine` itself — the `PipelineModeDefault` (`""`) short-circuit IS the flag: every existing item is unaffected until a mode is explicitly chosen, and Phase 1 ships with zero UI to choose one yet (the selector lands in Phase 3). This is the concrete "reviewable in isolation, no-op by construction" milestone requirements.md's Risk Control section requires.
- **Rollback procedure**: Phase 1 is a single reviewable commit set (Epic 1.1–1.7) landed before any second mode exists or any CRUD UI is exposed; `git revert` of that commit range fully removes the seam with no data-loss risk (see Migration Plan reversibility above).
- **Staged rollout**: Phase 0 (adoption spike — go/no-go gate on whether Phase 1 starts at all, see below) → Phase 1 (seam + zero-regression characterization tests) → Phase 2 (CRUD RPCs, still no UI to reach them) → Phase 3 (frontend selector + management UI + "what ran" surface) → Phase 4 (a real second mode defined through the now-live UI, proving the seam is not cosmetic). Each phase is independently shippable and reviewable; do not collapse phases into one PR.
- **`NewPipelineEngine` startup-failure behavior**: unlike `NewDefaultWorkflowEngine()` (zero-arg, infallible, pure in-memory), `NewPipelineEngine(repo)` does a real `cache.Load` → `repo.ListEnabled` DB call at construction time and can fail (DB unavailable, migration race, transient connection error). If `cache.Load` fails during `NewPipelineEngine`, the constructor does NOT return an error that aborts server startup — it logs a Warn-level `[PipelineEngine] cache.Load failed at startup, continuing with an empty cache: <err>` and returns a usable `*CachingPipelineEngine` backed by an empty cache. The server continues booting normally; every non-default `pipeline_mode` resolution will hit the existing fail-closed-to-default path (Story 1.3.3) with its own Warn log until the next successful `Invalidate` or retry. This is required because the feature is purely additive/opt-in (per the `PipelineModeDefault` short-circuit above) — a transient DB hiccup at boot must never crash the whole server for a feature most items don't use yet. See Story 1.3.3a for the constructor change and its test.
- **Phase 0 spike — validate the core premise BEFORE any Phase 1 code exists (promoted from a Phase-3-only gate to a pre-Phase-1 gate; this is the fix for the Product Triad Review's P1 finding that the original "adoption litmus test" only covered Phase 3's settings UI while Phases 1-2 — the ent schema, this codebase's first hot-path DB cache, the 5-method `PipelineEngine` seam, and the CRUD RPCs — described by ADR-001 and the pre-mortem itself as the majority of this plan's Large-appetite engineering cost, shipped unconditionally with no adoption check at all):**
  Before any `session/ent/schema/pipeline_mode.go` code, any cache design, or any `PipelineEngine` code is written — i.e. before Epic 1.1 starts — run a cheap, throwaway spike that tests the actual premise (that DB-persisted, UI-editable pipeline modes are lower-friction than the status quo for this specific single operator) without building any of the DB/cache/CRUD infrastructure the premise is used to justify.
  **Concrete spike design**: hand-author a second mode's worth of content as plain files — e.g. manually write an alternate `.claude/commands/backlog-quick/*.md` set, or a standalone prompt text file — and manually wire ONE real backlog item to use it via the cheapest possible mechanism: a temporary env var, a hardcoded `if item.ID == "..."` branch, or literally swapping the files `WriteSlashCommands` writes for one test run. No schema, no RPC, no UI. Time this end-to-end (author the content → get the item running under it) against the alternative path: editing `WriteSlashCommands`'s Go source directly for the same content and running `make install-service`.
  **Binary gate**: if the hand-authored-files approach is NOT meaningfully lower-friction than the Go-edit-and-redeploy path, the entire DB-persisted `PipelineMode` investment (Phases 1-4 as currently scoped) is not justified — STOP, do not proceed to Phase 1, and instead consider whether a much smaller version of this feature — literally the 2-3 more `Skip*`-style booleans that requirements.md's own "Alternatives Considered" section explicitly rejected as not scaling, but which may be the right call if the DB-persisted CRUD investment doesn't pay for itself — would serve the actual need better. If the spike DOES show meaningful friction reduction, proceed to Phase 1 as planned. **Record the spike's outcome and timing in this paragraph before Phase 1 begins** (owner: the repo operator, since implementer and target user are the same single person on this project).
  **Ongoing adoption signal, independent of the one-time spike result**: re-check the `pipeline_modes` row count and each mode's last-used timestamp manually at the end of Phase 4 and again ~1 month after Phase 4 ships — owner: the repo operator (single-operator tool, no team to delegate this check to). If usage never exceeds the Epic 4.1 proof mode (i.e. no operator-defined mode beyond the one created solely to prove the seam works ever gets used on a real item), treat this as a signal to stop investing further in this feature — e.g. do not build the deferred Epic 3.5 badge or any other follow-up — rather than silently assuming success.
- **Appetite / scope-cut check-in (Phase 3 — a SECOND, later decision point, distinct from the Phase 0 gate above)**: this plan ships the full CRUD management UI in Phase 3 (Epic 3.3: create/edit/enable/disable/delete via `/settings/pipeline-modes`), not a minimal/CLI-first cut, even though requirements.md's Appetite section pre-authorizes descoping it to "a minimal/admin-only form (or even a one-time seed script plus direct-DB editing)" if the Large (3–6 week) appetite is tight. This is a deliberate choice, recorded here rather than left implicit: Phase 1 and Phase 2 are independently shippable and already deliver the zero-regression seam + the full API surface a minimal form would also need, so Phase 3's UI is the only remaining appetite risk — if the budget is tight when Phase 3 starts, it can be descoped to a minimal form (or the pre-authorized seed-script/direct-DB-editing fallback) at that point without touching any Phase 1/2 work, using the same staged-shipping strategy above. This gate only matters if the Phase 0 spike above passed and Phases 1-2 actually shipped; it is about the polish level of the UI, not about whether to build Phases 1-2 at all (that question is Phase 0's, above).

## Unresolved Questions
- [x] ~~Should the 9 content-template placeholder names (`{{item_id}}`, `{{criteria_index}}`, etc.) be validated against a fixed allow-list at write time (reject unknown `{{...}}` tokens) or only at render time (silently leave unknown tokens un-substituted)?~~ **RESOLVED — write-time allow-list rejection, implemented in Story 2.3.1.** The recognized allow-list is `item_id`, `item_title`, `item_description`, `criteria_index`, `criteria_count`, `criteria_text`, `repo_path` (shared by `renderTemplate` and `ValidatePipelineModeContent`, see Domain Glossary). Phase 1's Task 1.3.3d passthrough behavior (unrecognized tokens left un-substituted) is explicitly a temporary, Phase-1-only state — no CRUD path exists yet to write a template containing an unrecognized token in Phase 1, so the gap is real but unreachable until Phase 2, at which point Story 2.3.1's validation closes it at the write boundary.
- [ ] Exact wording/threshold for when the management UI's mode list should collapse into "More" progressive disclosure (`research/ux.md` §1 flags this for "once the list is long" but doesn't set a number) — blocks Story 3.1.2 — owner: implementer, default to reusing `OmnibarCreationPanel.tsx`'s existing split point (2 primary + rest behind "More") until real usage data suggests otherwise.
- [ ] Whether `PipelineMode.Delete` should hard-block deletion when any `BacklogItemData.PipelineMode` still references the slug, or allow it and rely on the fail-closed-to-default resolution path — blocks Story 2.2.3 — owner: implementer, default to **allow deletion, rely on fail-closed resolution** (simpler, and the fail-closed path is being built regardless as the answer to the "mode deleted after items reference it" edge case in `research/features.md` §3).

## Dependency Visualization
```
Phase 0: Adoption spike (GO/NO-GO gate — see Risk Control; must pass before Epic 1.1 starts)
  Hand-authored files + manual one-item wiring, timed against a Go-edit-and-redeploy
        |
        v  (only if spike shows meaningful friction reduction)
Phase 1: Foundation (seam wraps existing hardcoded behavior — zero regression)
  Epic 1.1 (ent schema: PipelineMode)
        |
        v
  Epic 1.2 (PipelineModeRepository) ----+
        |                               |
        v                               v
  Epic 1.3 (PipelineEngine + cache) <---+
        |
        +--------------------------------------+
        v                                       v
  Epic 1.4 (item-level pipeline_mode field)   Epic 1.6 (ItemSession snapshot field)
        |                                       |
        v                                       |
  Epic 1.5 (wire PipelineEngine into 4 call sites) <--+
        |
        v
  Epic 1.7 (characterization tests + observability + isolated commit gate)  <== Phase 1 ships here
        |
        v
Phase 2: CRUD RPCs
  Epic 2.1 (proto: PipelineMode message + RPCs) --> Epic 2.2 (Go handlers + cache invalidation) --> Epic 2.3 (structural validation)
        |
        v
Phase 3: Frontend
  Epic 3.1 (shared RadioGroup) --> Epic 3.2 (item selector) --> Epic 3.3 (management CRUD page)
        |                                                              |
        v                                                              v
  Epic 3.4 ("what ran" surface + content-drift detection,          <-+
            depends on Epic 1.6's snapshot slug+hash fields
            AND Epic 2.1's PipelineMode.content_hash field)
        |
        v
Phase 4: Proof of seam + registry + e2e
  Epic 4.1 (define real "quick" mode via live UI) --> Epic 4.2 (observability polish) --> Epic 4.3 (feature registry + e2e tests)
```

---

**A note on task-sizing vs. the Large (3-6 week) appetite**: the 2-5-minute granularity on every leaf-level Task below follows this project's own SDD planning convention — fine-grained, atomic, independently-reviewable-in-isolation units — NOT traditional 1-4 hour AIC-style estimation units. Summing raw task-minutes will always undercount real elapsed time by roughly an order of magnitude once test-writing, ent/proto regeneration cycles, code review, and debugging are folded in (this plan has 96 leaf-level tasks; 96 × ~4 min ≈ 7 hours is not the real estimate). The 3-6 week Large appetite from requirements.md is a Story/Epic-level roll-up estimate, not a literal sum of the leaf-level task times — read it that way, not as a contradiction of the appetite.

## Phase 1: Foundation — `PipelineMode` Data Model & `PipelineEngine` Seam

**Phase 0 precondition**: do not start Epic 1.1 until the Phase 0 adoption spike (see Risk Control section) has run and passed its binary gate — see the Dependency Visualization diagram above.

### Epic 1.1: `PipelineMode` ent schema
**Goal**: A new ent-backed table exists, matching the shape recorded in the Migration Plan, ready for a repository layer to sit on top of it.

#### Story 1.1.1: Create the `PipelineMode` ent schema file
**As a** backend developer, **I want** a `PipelineMode` ent entity, **so that** pipeline-mode definitions can be persisted and queried like `session.Workflow` is today.
**Acceptance Criteria**:
- The file `session/ent/schema/pipeline_mode.go` defines an ent `PipelineMode` schema with fields `id (uuid)`, `slug (string, unique)`, `name (string)`, `description (string, optional)`, `enabled (bool, default true)`, the 9 content-template `string` fields listed in the Migration Plan, and `created_at`/`updated_at` (time, matching `session/ent/schema/workflow.go`'s timestamp convention).
  - *Given* the file `session/ent/schema/pipeline_mode.go` does not yet exist, *When* it is created with `field.String("slug").Unique()` and `field.Bool("enabled").Default(true)`, *Then* `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (per `.claude/rules/ent-schema-generation.md`) succeeds and generates `session/ent/pipelinemode/`, `session/ent/pipelinemode_create.go`, etc.
- **Files**: `session/ent/schema/pipeline_mode.go`

##### Task 1.1.1a: Write the `PipelineMode` ent schema struct + `Fields()` (~5 min)
- Create `session/ent/schema/pipeline_mode.go` following the exact structure of `session/ent/schema/workflow.go` (same package, same `ent.Schema` embedding pattern, same `Fields() []ent.Field` shape).
- Add `id`, `slug` (`.Unique()`), `name`, `description` (`.Optional()`), `enabled` (`.Default(true)`), the 9 content-template string fields (each `.Comment(...)`-documented per the codebase's `Comment()` convention seen on `session/ent/schema/backlog_item.go:39-43`), `created_at`, `updated_at`.
- Files: `session/ent/schema/pipeline_mode.go`

##### Task 1.1.1b: Regenerate ent bindings (~2 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (the exact command from `session/ent/generate.go` — never bare `ent generate`).
- Run `go build ./...` to confirm the generated code compiles.
- Files: `session/ent/pipelinemode/*.go` (generated), `session/ent/pipelinemode_*.go` (generated), `session/ent/client.go` (generated), `session/ent/runtime/runtime.go` (generated)

---

### Epic 1.2: `PipelineModeRepository`
**Goal**: A CRUD persistence interface + ent implementation exists, mirroring `WorkflowRepository`/`EntWorkflowRepository` exactly in shape.

#### Story 1.2.1: Define `PipelineModeRepository` interface and input structs
**As a** backend developer, **I want** a narrow repository interface for `PipelineMode` CRUD, **so that** `CachingPipelineEngine` and the future RPC handlers depend on an interface, not a concrete ent client.
**Acceptance Criteria**:
- `session/pipeline_mode_repository.go` defines `PipelineModeRepository` with methods `Create(ctx, PipelineModeCreateInput) (*ent.PipelineMode, error)`, `Update(ctx, uuid.UUID, PipelineModeUpdateInput) (*ent.PipelineMode, error)`, `Delete(ctx, uuid.UUID) error`, `GetByID(ctx, uuid.UUID) (*ent.PipelineMode, error)`, `GetBySlug(ctx, string) (*ent.PipelineMode, error)`, `ListAll(ctx) ([]*ent.PipelineMode, error)`, `ListEnabled(ctx) ([]*ent.PipelineMode, error)` — the exact method set of `WorkflowRepository` (`session/workflow_repository.go:12-19`), substituting `PipelineMode` for `Workflow`.
  - *Given* `session/workflow_repository.go`'s `WorkflowRepository` interface as the shape template, *When* `PipelineModeRepository` is defined with the same 7 methods, *Then* `session/pipeline_mode_repository.go` compiles and a mock/fake implementing it satisfies the interface with no extra methods required.
- **Files**: `session/pipeline_mode_repository.go`

##### Task 1.2.1a: Write `PipelineModeRepository` interface + `PipelineModeCreateInput` (~4 min)
- Create `session/pipeline_mode_repository.go`, copying `session/workflow_repository.go:1-36`'s structure: package `session`, the 7-method interface, and `PipelineModeCreateInput` struct with fields `Slug, Name, Description string`, `Enabled bool`, and the 9 content-template `string` fields.
- Files: `session/pipeline_mode_repository.go`

##### Task 1.2.1b: Write `PipelineModeUpdateInput` (~3 min)
- Add `PipelineModeUpdateInput` to the same file with all fields as `*T` pointers (partial-update semantics), mirroring `WorkflowUpdateInput` (`session/workflow_repository.go:40-53`).
- Files: `session/pipeline_mode_repository.go`

#### Story 1.2.2: Implement `EntPipelineModeRepository`
**As a** backend developer, **I want** an ent-backed implementation of `PipelineModeRepository`, **so that** pipeline modes are actually persisted.
**Acceptance Criteria**:
- `session/ent_pipeline_mode_repository.go` implements all 7 `PipelineModeRepository` methods using the `*ent.Client`, following `session/ent_workflow_repository.go`'s conditional-setter pattern (only call `.SetX(...)` when the input field is non-zero/non-nil).
  - *Given* a `PipelineModeCreateInput{Slug: "quick", Name: "Quick Fix", Enabled: true, TriagePromptTemplate: "Fix {{item_id}} quickly."}`, *When* `EntPipelineModeRepository.Create(ctx, input)` is called against a fresh test DB, *Then* it returns a `*ent.PipelineMode` with `Slug == "quick"` and a subsequent `GetBySlug(ctx, "quick")` returns the same row.
  - *Given* an existing `PipelineMode` with slug `"quick"`, *When* `Create` is called again with `Slug: "quick"`, *Then* it returns an `ent.ConstraintError` (unique constraint on `slug`), matching `EntWorkflowRepository.Create`'s documented duplicate-slug behavior.
- **Files**: `session/ent_pipeline_mode_repository.go`

##### Task 1.2.2a: Implement `NewEntPipelineModeRepository` + `Create` (~5 min)
- Create `session/ent_pipeline_mode_repository.go`, copying `session/ent_workflow_repository.go:1-60`'s structure for the constructor and `Create` method.
- Files: `session/ent_pipeline_mode_repository.go`

##### Task 1.2.2b: Implement `Update`, `Delete`, `GetByID`, `GetBySlug` (~5 min)
- Add the remaining 4 methods, mirroring `EntWorkflowRepository`'s equivalents (find them via `grep -n "func (r \*EntWorkflowRepository)" session/ent_workflow_repository.go`).
- Files: `session/ent_pipeline_mode_repository.go`

##### Task 1.2.2c: Implement `ListAll`, `ListEnabled` (~4 min)
- Add the 2 list methods, mirroring `EntWorkflowRepository.ListAll`/`ListEnabled` (the `enabled=true` predicate equivalent to `cron_enabled=true` in the workflow version).
- Files: `session/ent_pipeline_mode_repository.go`

##### Task 1.2.2d: Unit tests for `EntPipelineModeRepository` (~5 min)
- Add `session/ent_pipeline_mode_repository_test.go` covering: Create + GetBySlug round-trip, duplicate-slug ConstraintError, Update partial-field semantics (only supplied fields change), Delete removes the row, ListEnabled excludes disabled rows.
- Files: `session/ent_pipeline_mode_repository_test.go`

---

### Epic 1.3: `PipelineEngine` interface, cache, and default-mode wrapping
**Goal**: The `PipelineEngine` seam exists and, for `PipelineModeDefault`, produces byte-identical output to today's hardcoded functions — with zero new DB/cache dependency on that path.

#### Story 1.3.1: Define `PipelineEngine` interface and `PipelineModeDefault`
**As a** backend developer, **I want** a narrow `PipelineEngine` interface with 5 methods, **so that** `WriteSlashCommands`/triage/review/initial-prompt/snapshot-hash call sites depend on an interface instead of hardcoded free functions.
**Acceptance Criteria**:
- `session/pipeline_engine.go` defines `type PipelineMode string`, `const PipelineModeDefault PipelineMode = ""`, and `type PipelineEngine interface` with `SlashCommandSet(item *BacklogItemData) (map[string]string, error)`, `TriagePromptFor(item *BacklogItemData, artifactAbsPath string) string`, `ReviewPromptFor(item *BacklogItemData, acSnapshot []AcCriterion, diff string, diffTruncated bool, verificationNotes string) string`, `InitialPromptFor(item *BacklogItemData, priorSessions []ItemSessionSummary) string`, `ContentHashFor(mode PipelineMode) (hash string, ok bool)` (added in Story 1.3.3 for the Epic 1.6 snapshot-hash call sites — see Pattern Decisions' "method count" row).
  - *Given* the file does not yet exist, *When* `session/pipeline_engine.go` is created with this interface, *Then* `go build ./session/...` succeeds with no implementations yet required (interface-only compiles).
- **Files**: `session/pipeline_engine.go`

##### Task 1.3.1a: Write the `PipelineEngine` interface + `PipelineMode`/`PipelineModeDefault` types (~4 min)
- Create `session/pipeline_engine.go` with the package doc comment explaining the sibling (not extension) relationship to `WorkflowEngine` (cite `research/architecture.md` §1's reasoning inline).
- Files: `session/pipeline_engine.go`

#### Story 1.3.2: Implement `pipelineModeCache` (copy-on-write, writer-serialized)
**As a** backend developer, **I want** a lock-free-for-readers, writer-serialized, copy-on-write in-process cache of enabled pipeline modes, **so that** resolving a non-default mode never blocks on a DB round trip on the hot path, and concurrent operator edits can never lost-update each other.
**Acceptance Criteria**:
- `pipelineModeCache` wraps `atomic.Pointer[map[string]resolvedPipelineMode]` plus an unexported `sync.Mutex` (`writeMu`) that guards ONLY the write path. `Load(ctx, repo)` acquires `writeMu`, calls `repo.ListEnabled`, deep-copies each `*ent.PipelineMode` into a `resolvedPipelineMode` value (computing `ContentHash` — see Task 1.3.2a), builds the new map, atomically `Store`s the pointer, then releases `writeMu`. `Get(slug)` does a single atomic `Load()` + map lookup with no locking whatsoever — it never touches `writeMu`. `Invalidate(ctx, repo)` has an identical body to `Load` (acquire `writeMu` → re-fetch → `Store` → release), so two concurrent `Invalidate` calls (or an `Invalidate` racing the initial `Load`) are fully serialized on the DB-read-then-store sequence — the slower call's `Store` cannot land after a faster, later-started call's `Store`, because the mutex forces one call's entire read-then-store to complete before the other's DB read even begins.
  - *Given* a `pipelineModeCache` populated with one enabled mode `{Slug: "quick", ...}`, *When* `Get("quick")` is called from 50 concurrent goroutines while `Invalidate` runs concurrently on another goroutine, *Then* every `Get` call returns either the pre- or post-invalidate snapshot in full (never a torn/partial read) — verified by a `-race`-clean test with `go test -race`.
  - *Given* a slug `"missing"` not present in the cache, *When* `Get("missing")` is called, *Then* it returns `(resolvedPipelineMode{}, false)` — the caller (Epic 1.3.3) is responsible for the Warn-log-and-fallback behavior, not the cache itself.
  - *Given* N concurrent `Invalidate(ctx, repo)` calls against a fake `PipelineModeRepository` whose `ListEnabled` returns different data on each call (e.g. tagged with a monotonically increasing version field) and whose response latency is deliberately varied (a fast call started after a slow call, via a per-call artificial delay keyed to call order), *When* all N calls complete, *Then* the final cache state matches the data returned by whichever call's DB read *began last* — never an earlier call's data lingering because its `Store` happened to land after a later call's `Store`. This is the concurrent-`Invalidate`-vs-`Invalidate` lost-update case that plain `atomic.Pointer` (with no writer mutex) does NOT protect against, distinct from the `Get`-vs-`Invalidate` torn-read case above.
- **Files**: `session/pipeline_engine.go`

##### Task 1.3.2a: Define `resolvedPipelineMode` struct + `pipelineModeCache.Load`/`Get` (~5 min)
- Add `resolvedPipelineMode` (unexported: `Slug, Name string`, the 9 rendered-template-source fields, and `ContentHash string`) and `pipelineModeCache` (`ptr atomic.Pointer[map[string]resolvedPipelineMode]`, `writeMu sync.Mutex`) with `Load(ctx, PipelineModeRepository) error` and `Get(slug string) (resolvedPipelineMode, bool)`. `Load` computes each mode's `ContentHash` via `computeContentHash(m.StatusCommandTemplate, ..., m.InitialPromptTemplate)` (SHA-256, hex, truncated to 16 chars, fields concatenated in fixed declaration order) while building the `resolvedPipelineMode`.
- Files: `session/pipeline_engine.go`

##### Task 1.3.2b: Implement `pipelineModeCache.Invalidate` with writer-side mutex serialization (~4 min)
- Add `Invalidate(ctx, PipelineModeRepository) error`. Body: `c.writeMu.Lock(); defer c.writeMu.Unlock()`, then the same re-fetch + build + atomic `Store` sequence as `Load` — exposed as a separate method name for call-site clarity at RPC write handlers (Epic 2.2), not because the implementation differs from `Load` beyond both now sharing the mutex-guarded sequence. Update `Load` itself to acquire the same `writeMu` (it is, after all, just the first write).
- Files: `session/pipeline_engine.go`

##### Task 1.3.2c: Race-safe concurrency test for `pipelineModeCache` — torn-read case (~5 min)
- Add `session/pipeline_engine_test.go` with a `-race` test: concurrent `Get` + `Invalidate` calls against a fake `PipelineModeRepository`, asserting no torn reads and no data race. Additionally: hold a `Get()` result across a simulated concurrent `Invalidate()` call and assert the held value's fields are unchanged afterward (the actual invariant this design protects — an in-flight LLM call reading `resolvedPipelineMode` fields hundreds of ms after `Get` returned must see a stable, immutable snapshot even if 3 invalidations happen concurrently), per architecture-review.md's Lens 1.4 concern.
- Files: `session/pipeline_engine_test.go`

##### Task 1.3.2d: Lost-update concurrency test for concurrent `Invalidate` calls (~5 min)
- Add a second `-race` test to `session/pipeline_engine_test.go`: N goroutines (e.g. N=10) each call `Invalidate` against a fake `PipelineModeRepository` whose `ListEnabled` returns a distinct, version-tagged result per call and whose latency is inversely correlated with call-start order (so a call that starts later would finish first if unserialized), asserting the final cache state deterministically reflects the last call to *begin* its DB read, not the last to be scheduled — the concrete regression test for adversarial-review.md's Blocker 1 (lost-update, not torn-read).
- Files: `session/pipeline_engine_test.go`

#### Story 1.3.3: Implement `CachingPipelineEngine` with fail-closed resolution
**As a** backend developer, **I want** the single `PipelineEngine` implementation to resolve `PipelineModeDefault` for free and any other slug via the cache, falling back to default behavior with a Warn log on any miss, **so that** an unresolvable/deleted mode never silently no-ops or crashes a live item.
**Acceptance Criteria**:
- `CachingPipelineEngine.SlashCommandSet(item)`: if `item.PipelineMode == PipelineModeDefault`, calls `buildDefaultSlashCommandSet(item)` (Story 1.3.4) directly — no cache/DB touch. Otherwise, calls `cache.Get(item.PipelineMode)`; on hit, renders that mode's 6 command templates via `renderTemplate`; on miss, logs `log.WarningLog.Printf("[PipelineEngine] unresolved pipeline_mode=%q item=%s — falling back to default", item.PipelineMode, item.ID)` and calls `buildDefaultSlashCommandSet(item)`.
  - *Given* a `BacklogItemData{ID: "abc-123", PipelineMode: ""}`, *When* `CachingPipelineEngine.SlashCommandSet(item)` is called, *Then* the returned map is byte-identical to what today's hardcoded `WriteSlashCommands` body produces for the same item, and no call is made to `pipelineModeCache.Get` (verified via a test double that fails the test if `Get` is invoked).
  - *Given* a `BacklogItemData{ID: "abc-123", PipelineMode: "deleted-mode"}` where `"deleted-mode"` is not present in the cache, *When* `CachingPipelineEngine.SlashCommandSet(item)` is called, *Then* it returns the same default command set as the empty-mode case AND a single `[PipelineEngine]`-prefixed Warn log line is emitted containing `item=abc-123` and `deleted-mode`.
- The same default-short-circuit + cache-hit + Warn-fallback pattern is implemented for `TriagePromptFor`, `ReviewPromptFor`, and `InitialPromptFor`.
  - *Given* a `BacklogItemData{PipelineMode: ""}` and `artifactAbsPath: "/tmp/plan.md"`, *When* `TriagePromptFor(item, "/tmp/plan.md")` is called, *Then* it returns exactly `session.BuildHeadlessTriagePrompt(item, "/tmp/plan.md")`'s output.
- `ContentHashFor(mode PipelineMode) (hash string, ok bool)`: for `PipelineModeDefault`, returns `("", false)` (no DB-backed content to hash — default is code, not DB-mutable). For any other slug, returns `(cache.Get(slug).ContentHash, true)` on a cache hit, or `("", false)` on a miss (unresolved slug — no Warn log here, since this method is only ever called from the Epic 1.6 snapshot-write path immediately after a successful resolution already logged its own outcome; a caller invoking `ContentHashFor` for an already-unresolved slug is a pre-existing bug elsewhere, not a new failure mode to log again).
- **`NewPipelineEngine` startup-failure behavior (graceful degradation)**: `NewPipelineEngine(repo)` calls `cache.Load(ctx, repo)` once at construction. If `cache.Load` returns an error (DB unavailable, migration race, transient connection error), `NewPipelineEngine` does NOT return that error to its caller — it logs `log.WarningLog.Printf("[PipelineEngine] cache.Load failed at startup, continuing with an empty cache: %v", err)` and returns a valid `*CachingPipelineEngine` backed by an empty (zero-mode) cache. `NewPipelineEngine`'s signature stays `(repo PipelineModeRepository) (*CachingPipelineEngine, error)` for future-proofing (e.g. a future validation error truly worth failing construction on), but the Phase 1 implementation never actually returns a non-nil error for a `cache.Load` failure specifically. This is required because `PipelineEngine` is purely additive/opt-in — a transient DB hiccup at boot must never crash the whole server for a feature most items don't use yet (see Risk Control section).
  - *Given* a `PipelineModeRepository` test double whose `ListEnabled` always returns an error, *When* `NewPipelineEngine(repo)` is called, *Then* it returns `(engine, nil)` where `engine` is non-nil and usable — `engine.SlashCommandSet(defaultItem)` succeeds via the default short-circuit, and `engine.SlashCommandSet(itemWithNonDefaultMode)` falls back to default output with a Warn log (the normal unresolved-slug path, since the cache is empty), and exactly one additional Warn log line was emitted at construction time naming the `cache.Load` error.
- **Files**: `session/pipeline_engine.go`

##### Task 1.3.3a: Implement `NewPipelineEngine` constructor (graceful degradation) + `SlashCommandSet` (~6 min)
- Add `CachingPipelineEngine` struct (`repo PipelineModeRepository`, `cache *pipelineModeCache`) and `NewPipelineEngine(repo PipelineModeRepository) (*CachingPipelineEngine, error)` which calls `cache.Load` once at construction. On a `cache.Load` error, log a Warn (per the acceptance criteria above) and proceed with the empty-cache engine rather than returning the error — do NOT mirror `NewDefaultWorkflowEngine`'s infallibility claim literally, since unlike that zero-arg constructor, this one does a real DB call and must define its own failure behavior explicitly.
- Implement `SlashCommandSet` per the acceptance criteria above.
- Files: `session/pipeline_engine.go`

##### Task 1.3.3b: Implement `TriagePromptFor` and `ReviewPromptFor` (~5 min)
- Implement both methods with the default-short-circuit + cache-hit + Warn-fallback pattern.
- Files: `session/pipeline_engine.go`

##### Task 1.3.3c: Implement `InitialPromptFor` and `ContentHashFor` (~5 min)
- Implement `InitialPromptFor` with the same pattern, delegating to `session.BuildTokenBudgetedPrompt(item, priorSessions)` for the default case (the exact call currently inline at `server/services/backlog_service_triage.go:260`). Implement `ContentHashFor` per the acceptance criteria above.
- Files: `session/pipeline_engine.go`

##### Task 1.3.3f: Test `NewPipelineEngine`'s graceful-degradation-on-`cache.Load`-failure behavior (~4 min)
- Add a test to `session/pipeline_engine_test.go`: construct `NewPipelineEngine` with a `PipelineModeRepository` test double whose `ListEnabled` returns an error; assert construction returns `(non-nil engine, nil error)`, the engine is fully usable for the default mode, falls back correctly for a non-default mode, and a Warn log was emitted — the concrete regression test for adversarial-review.md's Blocker 4.
- Files: `session/pipeline_engine_test.go`
- Files: `session/pipeline_engine.go`

##### Task 1.3.3d: Implement `renderTemplate` fixed-placeholder substitution (~4 min)
- Add `renderTemplate(tmpl string, placeholders map[string]string) string` using `strings.NewReplacer`, with an allow-list of recognized placeholder names: `item_id`, `item_title`, `item_description`, `criteria_index`, `criteria_count`, `criteria_text`, `repo_path` (same allow-list `ValidatePipelineModeContent` uses in Story 2.3.1 — see Domain Glossary's `renderTemplate` entry). Unrecognized `{{...}}` tokens are left as-is at render time — this is a TEMPORARY, PHASE-1-ONLY passthrough (no CRUD write path exists yet in Phase 1 for an operator to actually persist a template containing an unrecognized token), explicitly superseded once Story 2.3.1 ships in Phase 2: from that point on, every persisted content-template field is guaranteed (by write-time rejection) to contain only recognized placeholder names, so this passthrough branch becomes unreachable in practice rather than being the permanent behavior.
- Files: `session/pipeline_engine.go`

##### Task 1.3.3e: Unit tests for `CachingPipelineEngine` fail-closed behavior (~5 min)
- Add tests to `session/pipeline_engine_test.go`: default-mode short-circuits the cache (no `Get` call), unresolved-slug falls back to default output + emits the Warn log (capture via a log-writer test double), resolved-slug renders its own templates (not the default ones), and a resolved-slug template containing an unrecognized `{{unknown_token}}` placeholder renders with the token left un-substituted (exercising the Phase-1-only passthrough branch from Task 1.3.3d, previously untested per adversarial-review.md's Blocker 2).
- Files: `session/pipeline_engine_test.go`

#### Story 1.3.4: Extract today's hardcoded slash-command body into `buildDefaultSlashCommandSet`
**As a** backend developer, **I want** `WriteSlashCommands`'s current hardcoded file-content generation extracted into a pure function returning `map[string]string`, **so that** `CachingPipelineEngine`'s default path and the on-disk writer share one source of truth with zero behavior drift.
**Acceptance Criteria**:
- `session/backlog_commands.go` gains `buildDefaultSlashCommandSet(item *BacklogItemData) (map[string]string, error)`, containing exactly the content-generation logic currently inline in `WriteSlashCommands` (lines ~35-70+ per the current file read at `session/backlog_commands.go`, covering `status.md`, per-criterion `done-N.md`/`fail-N.md`, `review.md`, `ship.md`, `help.md`). `WriteSlashCommands` itself is refactored to call `buildDefaultSlashCommandSet` and write the returned map's entries to disk (retaining its existing `MkdirAll` retry-3-times logic), rather than building content inline.
  - *Given* a `BacklogItemData` with 2 AC criteria, *When* `buildDefaultSlashCommandSet(item)` is called, *Then* the returned map has exactly the keys `status.md, done-0.md, fail-0.md, done-1.md, fail-1.md, review.md, ship.md, help.md` with content identical to what the pre-refactor `WriteSlashCommands` wrote to disk for the same item (verified by a golden-file characterization test — see Story 1.7.1).
- **Files**: `session/backlog_commands.go`

##### Task 1.3.4a: Extract `buildDefaultSlashCommandSet` from `WriteSlashCommands` (~5 min)
- Move the content-building logic (the `fmt.Sprintf`/`writeFile`-content-construction parts, not the `os.MkdirAll`/disk-write parts) into the new function; `WriteSlashCommands` now: builds the dir, calls `buildDefaultSlashCommandSet` (or, after Story 1.3.3, the injected `PipelineEngine.SlashCommandSet`), then loops over the returned map calling the existing `writeFile` helper.
- Files: `session/backlog_commands.go`

##### Task 1.3.4b: Update `WriteSlashCommands` signature to accept a `PipelineEngine` (~4 min)
- Change `WriteSlashCommands(item *BacklogItemData, worktreePath string) error` to `WriteSlashCommands(engine PipelineEngine, item *BacklogItemData, worktreePath string) error` (engine-first, matching Go convention for a "policy object" parameter, and consistent with how `TransitionBacklogItemStatus` already takes an engine-shaped param in nearby code). Update both call sites: `server/services/backlog_service_triage.go:436` and `server/services/backlog_service_sync.go:93` (deferred to Epic 1.5 — this task only changes the signature and leaves call sites broken/to-be-fixed by Epic 1.5's tasks, OR do both in the same task if under the 3-5 file cap; given exactly 3 files are touched, do both here).
- Files: `session/backlog_commands.go`, `server/services/backlog_service_triage.go`, `server/services/backlog_service_sync.go`

---

### Epic 1.4: Item-level `pipeline_mode` field
**Goal**: `BacklogItemData`/`BacklogItemUpdate` carry the chosen mode slug end-to-end (ent → domain → proto → RPC handlers), using the presence-gated `optional string` pattern that avoids the proto3-bool-clobbering bug class.

#### Story 1.4.1: Add `pipeline_mode` ent field to `BacklogItem`
**As a** backend developer, **I want** a `pipeline_mode` column on `backlog_items`, **so that** an item's chosen mode slug is durable.
**Acceptance Criteria**:
- `session/ent/schema/backlog_item.go` gains `field.String("pipeline_mode").Default("").Comment("Slug of the PipelineMode this item uses to drive triage/work/review content. Empty string means the built-in default (today's fixed hardcoded pipeline).")`, placed after the existing `auto_spawn_session` field (line 43 region) for locality with its sibling per-item configuration flags.
  - *Given* the current schema has no `pipeline_mode` field, *When* it is added with `.Default("")`, *Then* `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` regenerates successfully and every existing `BacklogItem` row (pre-migration) reads back `PipelineMode == ""` after the server restarts (ent auto-migrate backfills the default for existing rows).
- **Files**: `session/ent/schema/backlog_item.go`

##### Task 1.4.1a: Add the ent field + regenerate (~4 min)
- Add the field per the acceptance criterion; run the regeneration command; run `go build ./...`.
- Files: `session/ent/schema/backlog_item.go`, `session/ent/*` (generated)

#### Story 1.4.2: Add `pipeline_mode` proto field (3 messages, `optional string`)
**As a** backend/frontend developer, **I want** `pipeline_mode` exposed on the wire with real presence semantics, **so that** "omitted" and "explicit reset to default" are distinguishable — closing the proto3-bool-clobbering bug class at its source.
**Acceptance Criteria**:
- `proto/session/v1/backlog.proto` gains `optional string pipeline_mode = 25;` on `BacklogItem` (next available field number after `auto_spawn_session = 24`, confirmed at `proto/session/v1/backlog.proto:117`), `optional string pipeline_mode = 11;` on `CreateBacklogItemRequest` (next after `auto_spawn_session = 10` at line 157), and `optional string pipeline_mode = 13;` on `UpdateBacklogItemRequest` (next after `auto_spawn_session = 12` at line 196).
  - *Given* `UpdateBacklogItemRequest` with `pipeline_mode` unset (field omitted entirely), *When* the message is serialized and deserialized, *Then* the generated Go/TS accessor reports "not present" (`req.Msg.PipelineMode == nil` in Go), distinct from an explicit `pipeline_mode: ""` which reports `req.Msg.PipelineMode != nil && *req.Msg.PipelineMode == ""`.
- `make proto-gen` regenerates `session/gen/proto/go/session/v1/backlog.pb.go` and `web-app/src/gen/session/v1/backlog_pb.ts` with the new `optional` field producing a nilable Go pointer / TS `string | undefined`.
- **Files**: `proto/session/v1/backlog.proto`

##### Task 1.4.2a: Add the 3 `optional string pipeline_mode` fields (~3 min)
- Edit `proto/session/v1/backlog.proto` at the 3 line locations above.
- Files: `proto/session/v1/backlog.proto`

##### Task 1.4.2b: Run `make proto-gen` and verify generated bindings (~3 min)
- Run `make proto-gen`; confirm `session/gen/proto/go/session/v1/backlog.pb.go` gains a `PipelineMode *string` field on the 3 generated Go structs and `web-app/src/gen/session/v1/backlog_pb.ts` gains an optional `pipelineMode?: string` field.
- Files: `session/gen/proto/go/session/v1/backlog.pb.go` (generated), `web-app/src/gen/session/v1/backlog_pb.ts` (generated)

#### Story 1.4.3: Add `PipelineMode` to `BacklogItemData`/`BacklogItemUpdate` + ent repository mapping
**As a** backend developer, **I want** the domain struct and ent repository layer to carry `PipelineMode`, **so that** storage reads/writes round-trip the field.
**Acceptance Criteria**:
- `session/repository.go`'s `BacklogItemData` (line ~341 region) gains `PipelineMode string` (plain, not pointer — mirrors `RepoPath`). `BacklogItemUpdate` (line ~417 region) gains `PipelineMode *string` (pointer, for partial-update presence).
  - *Given* a `BacklogItemUpdate{PipelineMode: nil}` (field omitted), *When* `EntRepository.UpdateBacklogItem` is called, *Then* the stored `pipeline_mode` column is untouched. *Given* `BacklogItemUpdate{PipelineMode: ptr("")}` (explicit reset), *When* the same method is called, *Then* the stored column becomes `""`.
- `session/ent_repository_backlog.go` gains `SetPipelineMode(data.PipelineMode)` in the create path (mirroring `SetAutoSpawnSession(data.AutoSpawnSession)` at line 207) and an `if update.PipelineMode != nil { u.SetPipelineMode(*update.PipelineMode) }` block in the update path (mirroring lines 439-440), plus the field added to whatever `fromEnt`-style mapping function reads `item.AutoSpawnSession` at line 143 into `BacklogItemData`.
- **Files**: `session/repository.go`, `session/ent_repository_backlog.go`

##### Task 1.4.3a: Add `PipelineMode` to `BacklogItemData` and `BacklogItemUpdate` (~3 min)
- Files: `session/repository.go`

##### Task 1.4.3b: Wire `PipelineMode` through ent create/update/read mapping (~5 min)
- Add the 3 mapping points in `session/ent_repository_backlog.go` per the acceptance criterion.
- Files: `session/ent_repository_backlog.go`

#### Story 1.4.4: Wire `pipeline_mode` through Create/Update RPC handlers (presence-gated)
**As a** backend developer, **I want** the `CreateBacklogItem`/`UpdateBacklogItem` handlers to gate on proto field presence, **so that** an omitted `pipeline_mode` never silently clobbers an item's existing mode.
**Acceptance Criteria**:
- `server/services/backlog_service_lifecycle.go`'s `CreateBacklogItem` (line ~125) sets `PipelineMode: req.Msg.GetPipelineMode()` (proto-generated getter returns `""` for `nil`, which is the correct default for a new item) in the domain struct construction, mirroring line 160's `AutoSpawnSession: req.Msg.AutoSpawnSession`.
- `UpdateBacklogItem` (line ~195) gains, alongside the existing block at lines 231-236, an explicit presence check: `if req.Msg.PipelineMode != nil { update.PipelineMode = req.Msg.PipelineMode }` — NOT an unconditional wrap like the pre-existing `SkipReviewGate`/`SkipPlanning`/`AutoSpawnSession` lines (231-236), which is precisely the bug class being avoided here.
  - *Given* an existing item with `PipelineMode: "quick"`, *When* `UpdateBacklogItem` is called with a request that sets `title` but leaves `pipeline_mode` unset (`req.Msg.PipelineMode == nil`), *Then* the item's stored `pipeline_mode` remains `"quick"` after the update (not clobbered to `""`).
  - *Given* the same item, *When* `UpdateBacklogItem` is called with `pipeline_mode` explicitly set to `""`, *Then* the item's stored `pipeline_mode` becomes `""` (explicit reset honored).
- **Files**: `server/services/backlog_service_lifecycle.go`

##### Task 1.4.4a: Wire `CreateBacklogItem` (~3 min)
- Files: `server/services/backlog_service_lifecycle.go`

##### Task 1.4.4b: Wire `UpdateBacklogItem` with presence gating (~4 min)
- Files: `server/services/backlog_service_lifecycle.go`

##### Task 1.4.4c: Regression test proving omitted `pipeline_mode` doesn't clobber (~5 min)
- Add a test to `server/services/backlog_service_lifecycle_test.go` (or the file containing existing `UpdateBacklogItem` tests) implementing directly the two Given-When-Then scenarios already specified in Story 1.4.4's acceptance criteria above: (1) create/seed an item with `PipelineMode: "quick"`, call `UpdateBacklogItem` with `title` set and `pipeline_mode` omitted (`req.Msg.PipelineMode == nil`), assert the stored `pipeline_mode` is still `"quick"`; (2) same item, call `UpdateBacklogItem` with `pipeline_mode` explicitly set to `""`, assert the stored `pipeline_mode` becomes `""`. There is no existing precedent test to mirror for this — `SkipReviewGate`/`AutoSpawnSession` predate the `optional string` presence-gating pattern and use plain, non-optional proto `bool` fields, so they have no comparable clobbering-regression test to copy (verified: no match for `clobber`/`currentFlags` anywhere in `server/services/*_test.go` or `web-app/src/**/*.test.ts`); write these two cases directly from the acceptance criteria instead of searching for one.
- Files: `server/services/backlog_service_lifecycle_test.go`

#### Story 1.4.5: Map `PipelineMode` in `backlogItemToProto`
**As a** frontend developer, **I want** `pipeline_mode` returned on every `BacklogItem` proto response, **so that** the UI can read an item's current mode.
**Acceptance Criteria**:
- `server/services/backlog_service.go`'s proto-mapping function (line ~471 region, alongside `AutoSpawnSession: item.AutoSpawnSession`) sets `PipelineMode: &item.PipelineMode` (or the ptr-helper this codebase uses for `optional string` proto fields — check existing usage via `grep -n "proto.String\|&item\." server/services/backlog_service.go` for the established idiom before writing this).
  - *Given* a `BacklogItemData{PipelineMode: "quick"}`, *When* `backlogItemToProto(item)` is called, *Then* the resulting `*sessionv1.BacklogItem.PipelineMode` dereferences to `"quick"`.
- **Files**: `server/services/backlog_service.go`

##### Task 1.4.5a: Add the mapping line (~2 min)
- Files: `server/services/backlog_service.go`

---

### Epic 1.5: Wire `PipelineEngine` into the 4 call sites
**Goal**: `WriteSlashCommands`, headless triage, review-gate, and initial-prompt construction all consult the shared `PipelineEngine` instance instead of calling the hardcoded functions directly — and both `WriteSlashCommands` callers (sync path and triage path) are updated together so neither is left on the old path.

#### Story 1.5.1: Construct and share one `PipelineEngine` instance
**As a** backend developer, **I want** exactly one `CachingPipelineEngine` instance shared by `BacklogService` and `BacklogLifecycleListener`, **so that** the two never observe divergent cache state after a write.
**Acceptance Criteria**:
- **Bootstrap-ordering is verified, not assumed**: constructor injection of `pipelineEngine` into both `NewBacklogService` and `NewBacklogLifecycleListenerWithPool` is confirmed safe by direct inspection of `server/dependencies.go`, not asserted by analogy to `WorkflowEngine` (whose zero-arg, dependency-free constructor sidesteps this class of problem entirely and is therefore not evidence either way). Specifically: `storage.GetEntClient()` — the only prerequisite for constructing `pipelineModeRepo`/`pipelineEngine` — is already called successfully as early as `BuildCoreDepsWithOptions` (`server/dependencies.go:296`, for `NewErrorRegistry`), well before `BuildRuntimeDeps` even starts. Within `BuildRuntimeDeps` itself, `storage` is aliased at line 444 (function entry), while `NewBacklogLifecycleListenerWithPool` isn't called until line 483 and `services.NewBacklogService` not until line 871 — both far downstream of where `entClient`/`pipelineModeRepo`/`pipelineEngine` can be constructed. This differs from `WorkflowRepository`'s existing deferred `Set*`-style injection (lines 989-1006, documented pattern also referenced by `server/services/session_service.go:189-218`'s "Dependency-injection audit" comment): that deferral exists because `workflowScheduler`/`workflowSvc` need `sessionService` already fully constructed first (a real downstream ordering constraint specific to those types) — `pipelineEngine` has no equivalent downstream dependency on `sessionService` or on anything else built between lines 444 and 483. Conclusion: constructor injection (not a deferred `SetPipelineEngine`-style setter) is the correct, verified choice for this story — do not switch to deferred injection.
- `server/dependencies.go` constructs `entClient := storage.GetEntClient()` and `pipelineModeRepo := session.NewEntPipelineModeRepository(entClient)` early in `BuildRuntimeDeps` (near line 444, immediately after `storage` is aliased — NOT reusing the later `entClient` local at line 992, which is scoped to the `workflowRepo` block and constructed too late for this purpose), then `pipelineEngine, err := session.NewPipelineEngine(pipelineModeRepo)` near the existing `workflowEngine := session.NewDefaultWorkflowEngine()` (line ~459) — both before line 483's `NewBacklogLifecycleListenerWithPool` call. The same `pipelineEngine` value is passed into `services.NewBacklogService(storage, sessionService, cfg, workflowEngine, pipelineEngine)` (extending the call at line 871) AND into `NewBacklogLifecycleListenerWithPool`/`NewReviewGateRunner` (`session/backlog_lifecycle.go:296`).
  - *Given* `server/dependencies.go`'s `BuildRuntimeDeps`, *When* it runs, *Then* exactly one `*session.CachingPipelineEngine` value exists in the dependency graph and both `BacklogService.pipelineEngine` and `BacklogLifecycleListener.runner.pipelineEngine` point at the same instance (verified by a pointer-equality assertion in an integration test — see Task 1.5.1e).
- **Re-verify at implementation time**: the construction-order claim above (and its cited `server/dependencies.go` line numbers) was verified once already during the architecture-review repair round. `server/dependencies.go` is touched by other concurrent work in this repo, so line numbers and even the relative ordering of `storage`/`NewBacklogLifecycleListenerWithPool`/`services.NewBacklogService` may have shifted by the time this story is actually implemented — re-confirm the ordering still holds against the file's then-current state before wiring the constructor injection (a cheap one-line sanity check via `grep -n` before writing code, not a new design decision).
- **Files**: `server/dependencies.go`

##### Task 1.5.1a: Construct `pipelineModeRepo` and `pipelineEngine` early in `BuildRuntimeDeps` (~4 min)
- Construct immediately after `storage := svc.Storage` (line 444 region), before `NewBacklogLifecycleListenerWithPool` (line 483) — per the bootstrap-ordering evidence in the acceptance criteria above, not near the late `workflowRepo` block at line ~991.
- Files: `server/dependencies.go`

##### Task 1.5.1b: Add `pipelineEngine session.PipelineEngine` field + constructor param to `BacklogService` (~4 min)
- `server/services/backlog_service.go`: add field (alongside the existing `engine session.WorkflowEngine` field) and thread it through `NewBacklogService`'s signature and the nil-guard fallback (mirroring the existing `engine` nil-guard pattern in the same constructor).
- Files: `server/services/backlog_service.go`

##### Task 1.5.1c: Add `pipelineEngine PipelineEngine` field + constructor param to `ReviewGateRunner`/`BacklogLifecycleListener` (~5 min)
- `session/review_gate.go`: add field to `ReviewGateRunner` struct + `NewReviewGateRunner` param. `session/backlog_lifecycle.go`: thread the same value through to line 296's `NewReviewGateRunner(...)` call, adding a `pipelineEngine` param/field to `BacklogLifecycleListener` itself.
- Files: `session/review_gate.go`, `session/backlog_lifecycle.go`

##### Task 1.5.1d: Update all `NewReviewGateRunner`/`NewBacklogService` call sites (tests) (~5 min)
- Update the ~20 test call sites found via `grep -rn "NewReviewGateRunner(" --include="*.go" .` (`session/review_gate_test.go`, `session/review_gate_integration_test.go`) to pass a test-double `PipelineEngine` (a simple struct returning today's default behavior, or nil if the runner is made nil-tolerant like other optional deps — follow the codebase's existing nil-guard convention for optional constructor params).
- Files: `session/review_gate_test.go`, `session/review_gate_integration_test.go`

##### Task 1.5.1e: Pointer-equality integration test for the single shared `PipelineEngine` instance (~3 min)
- Add an integration test (e.g. in a `server/dependencies_test.go`-equivalent, or wherever `BuildRuntimeDeps` is already exercised in tests) that runs `BuildRuntimeDeps` and asserts, via `==` on the `*session.CachingPipelineEngine` pointer, that `BacklogService`'s and `BacklogLifecycleListener`'s (and, transitively, `ReviewGateRunner`'s) injected `PipelineEngine` values are the same instance — the test Story 1.5.1's own acceptance criteria already promises but that no prior task actually wrote (adversarial-review.md's Concern 3).
- Files: `server/dependencies_test.go` (or existing equivalent)

#### Story 1.5.2: `WriteSlashCommands` call sites consult `PipelineEngine`
**As a** backend developer, **I want** both callers of `WriteSlashCommands` to pass the shared engine, **so that** the sync-time path and the triage-time path never diverge in which command set they write.
**Acceptance Criteria**:
- `server/services/backlog_service_triage.go:436` calls `session.WriteSlashCommands(s.pipelineEngine, item, worktreePath)`.
- `server/services/backlog_service_sync.go:93` calls `session.WriteSlashCommands(s.pipelineEngine, item, worktreePath)`.
  - *Given* a `BacklogItemData{PipelineMode: "quick"}` synced in via `AttachSessionToItem` (the `backlog_service_sync.go:93` call site), *When* the sync path runs, *Then* the written `.claude/commands/backlog/*.md` files reflect the `"quick"` mode's templates, not the default set — proving the two callers can no longer drift (the exact regression `research/pitfalls.md` §5 point 1 warns about).
- **Files**: `server/services/backlog_service_triage.go`, `server/services/backlog_service_sync.go`

##### Task 1.5.2a: Update the 2 call sites (~3 min)
- Files: `server/services/backlog_service_triage.go`, `server/services/backlog_service_sync.go`

##### Task 1.5.2b: Regression test proving both callers use the same engine output (~5 min)
- Add a test asserting `SpawnSessionFromItem`'s and `AttachSessionToItem`'s written command files are identical for the same item+mode — a direct test for the "2 independent callers must not drift" blast-radius risk.
- Files: `server/services/backlog_service_triage_test.go`

#### Story 1.5.3: Triage prompt building consults `PipelineEngine.TriagePromptFor`
**As a** backend developer, **I want** `TriggerTriage`'s prompt construction to go through the engine, **so that** a non-default mode changes what the triage LLM call actually sees.
**Acceptance Criteria**:
- `server/services/backlog_service_triage.go:718` (`triagePrompt = session.BuildHeadlessTriagePrompt(item, artifactAbsPath)`) becomes `triagePrompt = s.pipelineEngine.TriagePromptFor(item, artifactAbsPath)`. The retriage branch at line 716 (`BuildHeadlessRetriagePrompt`) is left unchanged and NOT routed through `PipelineEngine` — explicitly documented as mode-independent per `research/architecture.md` §3's recommendation ("refine the existing plan" is inherently mode-independent).
  - *Given* `item.PipelineMode == "quick"` with a custom `TriagePromptTemplate`, *When* `TriggerTriage` runs its first-triage (non-retriage) branch, *Then* the LLM call receives the `"quick"` mode's rendered triage prompt, not `BuildHeadlessTriagePrompt`'s default text.
- **Files**: `server/services/backlog_service_triage.go`

##### Task 1.5.3a: Update the call site + add a comment documenting the retriage exclusion (~3 min)
- Files: `server/services/backlog_service_triage.go`

#### Story 1.5.4: Review-gate prompt building consults `PipelineEngine.ReviewPromptFor`
**As a** backend developer, **I want** both `ReviewGateRunner.Run` and `TriggerReReview` to build their review prompt via the engine, **so that** review behavior varies by mode too.
**Acceptance Criteria**:
- Before writing this task's code, re-read the CURRENT state of `session/review_gate.go` (around the `BuildHeadlessReviewPrompt` call, previously at line ~251 per stale research but confirmed changed by PR #155 — locate via `grep -n "BuildHeadlessReviewPrompt" session/review_gate.go`) and `server/services/backlog_service_triage.go`'s `TriggerReReview` (confirmed at line 887 onward in this plan's own verification pass) — do not trust old line numbers.
- `ReviewGateRunner.Run`'s call to `BuildHeadlessReviewPrompt(...)` becomes `r.pipelineEngine.ReviewPromptFor(...)` with identical arguments. `TriggerReReview`'s direct call to the same function makes the identical substitution using `s.pipelineEngine`.
- `Run`'s existing `if item.SkipReviewGate { return }` short-circuit (confirmed at `session/review_gate.go:121`) is untouched — mode selection does not gate whether review runs at all, only its prompt content, per the compose-not-subsume Pattern Decision.
  - *Given* `item.PipelineMode == "quick"` and `item.SkipReviewGate == false`, *When* `ReviewGateRunner.Run` is invoked, *Then* the review LLM call receives the `"quick"` mode's rendered review prompt AND the review gate still runs (is not skipped).
- **Files**: `session/review_gate.go`, `server/services/backlog_service_triage.go`

##### Task 1.5.4a: Re-verify current `BuildHeadlessReviewPrompt` call site in `review_gate.go` and update it (~4 min)
- Files: `session/review_gate.go`

##### Task 1.5.4b: Update `TriggerReReview`'s call site (~4 min)
- Files: `server/services/backlog_service_triage.go`

#### Story 1.5.5: `SpawnSessionFromItem`'s initial prompt consults `PipelineEngine.InitialPromptFor`
**As a** backend developer, **I want** the prompt handed to `inst.Prompt` (and therefore to `AutonomousDriver`'s `goal`) to go through the engine, **so that** autonomous-mode sessions genuinely change behavior under a non-default mode, not just interactive slash-command sets.
**Acceptance Criteria**:
- `server/services/backlog_service_triage.go:260`'s `prompt := session.BuildTokenBudgetedPrompt(item, priorSessions)` becomes `prompt := s.pipelineEngine.InitialPromptFor(item, priorSessions)`.
  - *Given* `item.PipelineMode == "quick"` and `item.AutoSpawnSession == true`, *When* `SpawnSessionFromItem` runs and the item is autonomous, *Then* `NewAutonomousDriver`'s `goal` parameter (passed `inst.Prompt` verbatim, per `research/architecture.md` §2's traced call chain) contains the `"quick"` mode's rendered initial-prompt content, not the default `BuildTokenBudgetedPrompt` output.
- **Files**: `server/services/backlog_service_triage.go`

##### Task 1.5.5a: Update the call site (~3 min)
- Files: `server/services/backlog_service_triage.go`

---

### Epic 1.6: Snapshot resolved mode onto `ItemSession`
**Goal**: An item's mode choice is immutable-after-first-triage-session in effect, by recording what was actually resolved at session-start time — mirroring the existing `AcSnapshot` field.

#### Story 1.6.1: Add `pipeline_mode_snapshot` and `pipeline_mode_snapshot_hash` fields to `ItemSession`
**As a** backend developer, **I want** an `ItemSession`-level snapshot of both the resolved mode slug AND a content hash of that mode's definition, **so that** a later mode edit/deletion doesn't retroactively change what a historical session is shown to have run — closing both the "item's mode reassigned" case (slug) and the "mode's own content edited" case (hash), per architecture-review.md's Blocker 1.
**Acceptance Criteria**:
- `session/ent/schema/item_session.go` gains `field.String("pipeline_mode_snapshot").Default("").Comment("The PipelineMode slug resolved and in effect when this session first started — snapshotted so later edits to the item's live pipeline_mode don't retroactively change what this session is shown to have run. Mirrors ac_snapshot's discipline.")` AND `field.String("pipeline_mode_snapshot_hash").Default("").Comment("SHA-256 (hex, truncated to 16 chars) of the resolved mode's 9 raw content-template field values, concatenated in fixed order, computed at the moment this session started. Empty for the default mode (code-backed, can't drift) or an already-unresolved slug. Compared against the live mode's current hash by the \"what ran\" UI (Story 3.4.1) to detect the referenced mode's content having been edited since — the slug alone cannot detect this.")`.
- `session/repository.go`'s `ItemSessionSummary` (line ~283 region) gains `PipelineModeSnapshot string` AND `PipelineModeSnapshotHash string`, both placed near `AcSnapshot` (line 288).
  - *Given* an `ItemSessionSummary` created before these fields existed, *When* it is read back, *Then* both fields are `""` (safe zero-value default, distinguishable from "was `default` mode" only in that both render identically — acceptable since `""` already means "default" everywhere else in this design).
- **Files**: `session/ent/schema/item_session.go`, `session/repository.go`

##### Task 1.6.1a: Add both ent fields + regenerate (~4 min)
- Files: `session/ent/schema/item_session.go`, `session/ent/*` (generated)

##### Task 1.6.1b: Add `PipelineModeSnapshot`/`PipelineModeSnapshotHash` to `ItemSessionSummary` + ent mapping (~4 min)
- Files: `session/repository.go`, `session/ent_repository_backlog.go` (wherever `ItemSessionSummary` is populated from ent — locate via `grep -n "AcSnapshot:" session/ent_repository_backlog.go`)

##### Task 1.6.1c: Expose both snapshot fields on the `ItemSession` proto message (~4 min)
- `proto/session/v1/backlog.proto`'s `message ItemSession` (line 65) gains `string pipeline_mode_snapshot = 16;` and `string pipeline_mode_snapshot_hash = 17;` (next available field numbers after `worktree_path = 15`). Run `make proto-gen`; wire both fields into the `ItemSession`-to-proto mapping function alongside the other fields mapped there. Required so Story 3.4.1's "what ran" UI (a frontend consumer) can actually read these values — neither field is otherwise reachable from the frontend.
- Files: `proto/session/v1/backlog.proto`, `session/gen/proto/go/session/v1/backlog.pb.go` (generated), `web-app/src/gen/session/v1/backlog_pb.ts` (generated), wherever `ItemSession`→proto mapping lives (likely `server/services/backlog_service.go`)

#### Story 1.6.2: Populate both snapshot fields at session-start call sites
**As a** backend developer, **I want** the slug and content-hash snapshots written exactly once, when a session/triage first starts, **so that** they reflect what was actually resolved at that moment.
**Acceptance Criteria**:
- `SpawnSessionFromItem` (`server/services/backlog_service_triage.go:157`) sets `PipelineModeSnapshot: item.PipelineMode` AND `PipelineModeSnapshotHash: hash` (where `hash, _ = s.pipelineEngine.ContentHashFor(item.PipelineMode)`, ignoring the `ok` bool — an unresolved slug or default mode correctly yields `hash == ""`) when creating the new `ItemSession` row (locate the exact `CreateItemSession`-equivalent call within this function).
- `TriggerTriage`'s session-creation path does the same for headless-triage-spawned sessions, using the same `ContentHashFor` call.
  - *Given* `item.PipelineMode == "quick"` at the moment `SpawnSessionFromItem` is called, *When* the resulting `ItemSession` is later read back (even after the item's `pipeline_mode` field is subsequently changed to `"full"`, or after `"quick"`'s own `triage_prompt_template` is edited), *Then* `ItemSessionSummary.PipelineModeSnapshot == "quick"` and `PipelineModeSnapshotHash` equals `"quick"`'s content hash AT THE TIME OF SPAWN (frozen, does not track subsequent edits to `"quick"`'s content), while `BacklogItemData.PipelineMode` and the live `"quick"` mode's current content hash may have since diverged.
- **Files**: `server/services/backlog_service_triage.go`

##### Task 1.6.2a: Wire both snapshot fields into `SpawnSessionFromItem`'s session-creation call (~4 min)
- Files: `server/services/backlog_service_triage.go`

##### Task 1.6.2b: Wire both snapshot fields into `TriggerTriage`'s session-creation call (~4 min)
- Files: `server/services/backlog_service_triage.go`

---

### Epic 1.7: Characterization tests, observability, and the isolated zero-regression commit gate
**Goal**: Prove Phase 1's default-mode path is byte-identical to pre-change behavior, and add the Info/Debug/Warn logging the Observability Requirements mandate — then land Phase 1 as its own reviewable commit before Phase 2 begins.

#### Story 1.7.1: Golden-file characterization tests for the default mode
**As a** backend developer, **I want** a snapshot test comparing pre- and post-`PipelineEngine` output for `WriteSlashCommands`, triage prompt, review prompt, and initial prompt, **so that** a silent behavior drift in the default path is caught by CI, not by a live item misbehaving.
**Acceptance Criteria**:
- A new test file captures the exact output of `buildDefaultSlashCommandSet`, `BuildHeadlessTriagePrompt`, `BuildHeadlessReviewPrompt`, and `BuildTokenBudgetedPrompt` for 2-3 representative `BacklogItemData` fixtures (varying AC-criteria count, at least one with 0 criteria) BEFORE any `PipelineEngine` wiring, stored as golden fixture files; a second test asserts `CachingPipelineEngine`'s equivalent methods (`SlashCommandSet`, `TriagePromptFor`, `ReviewPromptFor`, `InitialPromptFor`) produce byte-identical output against the same fixtures.
  - *Given* the golden fixture for a 2-criteria item captured pre-refactor, *When* `CachingPipelineEngine{}.SlashCommandSet(sameItem)` is called post-refactor, *Then* `reflect.DeepEqual` (or a byte-for-byte string comparison per map key) reports no difference.
- **Files**: `session/pipeline_engine_characterization_test.go`, `session/testdata/pipeline_engine/*.golden` (new fixture directory)

##### Task 1.7.1a: Capture golden fixtures for 3 representative items (~5 min)
- Files: `session/testdata/pipeline_engine/*.golden`

##### Task 1.7.1b: Write the characterization test comparing engine output to fixtures (~5 min)
- Files: `session/pipeline_engine_characterization_test.go`

#### Story 1.7.2: Observability logging at the 4 call sites
**As an** operator, **I want** Info-level logs of which mode was resolved at triage-start and review-start, Debug-level cache activity, and Warn-level unresolved-mode fallbacks, **so that** "why did this item run the wrong skill set" is debuggable without new tooling.
**Acceptance Criteria**:
- `TriggerTriage` logs `log.InfoLog.Printf("[PipelineEngine] item=%s stage=triage mode=%q", item.ID, resolvedModeLabel(item.PipelineMode))` once per triage start (`resolvedModeLabel` renders `""` as `"default"` for log readability).
- `ReviewGateRunner.Run` logs the same shape with `stage=review`.
- `pipelineModeCache.Load`/`Invalidate` log at Debug: `log.DebugLog.Printf("[PipelineEngine] cache refreshed: %d enabled modes", len(modes))`.
- Every fallback path from Story 1.3.3 logs at Warn (already specified there — this story verifies all 4 content-generating `PipelineEngine` methods — `SlashCommandSet`, `TriagePromptFor`, `ReviewPromptFor`, `InitialPromptFor` — do it consistently, not just `SlashCommandSet`. `ContentHashFor`, the 5th method, is intentionally excluded from this check — it does not fallback-Warn by design, per Story 1.3.3's acceptance criteria).
  - *Given* `TriggerTriage` runs for an item with `PipelineMode == "quick"`, *When* the triage LLM call is dispatched, *Then* the server log contains exactly one line matching `[PipelineEngine] item=<id> stage=triage mode="quick"`.
- **Files**: `server/services/backlog_service_triage.go`, `session/review_gate.go`, `session/pipeline_engine.go`

##### Task 1.7.2a: Add Info logs at triage-start and review-start (~4 min)
- Files: `server/services/backlog_service_triage.go`, `session/review_gate.go`

##### Task 1.7.2b: Add Debug logs to cache Load/Invalidate; verify Warn logs are consistent across all 4 engine methods (~4 min)
- Files: `session/pipeline_engine.go`

#### Story 1.7.3: Land Phase 1 as an isolated, reviewable commit
**As a** backend developer, **I want** Phase 1 (Epics 1.1-1.7) committed as its own PR before Phase 2 begins, **so that** a regression in the seam itself is caught in isolation from any second-mode or CRUD-UI change, per the Risk Control section.
**Acceptance Criteria**:
- `make build && make test` and `make lint` (per `make quick-check`) pass with only Phase 1's files changed; no `PipelineMode` CRUD RPCs, no frontend selector, and no second mode exist yet in this commit range — `item.PipelineMode` is reachable only via direct DB write (e.g. a manual test fixture), not via any shipped UI or RPC.
  - *Given* a fresh clone at the tip of the Phase 1 commit range, *When* `make ci` is run, *Then* it passes, and manually setting an item's `pipeline_mode` column to a nonexistent slug via direct SQL and triggering triage produces the Warn-log-and-default-fallback behavior (Story 1.3.3), not a crash or silent no-op.
- **Files**: N/A (process gate, not a file change)

##### Task 1.7.3a: Run `make ci` and confirm zero-regression on existing backlog test suite (~5 min)
- Files: N/A

---

## Phase 2: CRUD RPCs for `PipelineMode`

### Epic 2.1: Proto `PipelineMode` message + CRUD RPCs
**Goal**: The wire contract for creating/editing/enabling/disabling/listing pipeline modes exists, mirroring `ItemSource`'s CRUD RPC shape (the closest existing precedent within `backlog.proto`) and `WorkflowRepository`'s method surface.

#### Story 2.1.1: Define the `PipelineMode` proto message
**As a** frontend developer, **I want** a `PipelineMode` message on the wire, **so that** the UI can list/create/edit mode definitions.
**Acceptance Criteria**:
- `proto/session/v1/backlog.proto` gains, near the existing `ItemSource` message (line ~122), a new `message PipelineMode { string id = 1; string slug = 2; string name = 3; string description = 4; bool enabled = 5; string status_command_template = 6; string done_command_template = 7; string fail_command_template = 8; string review_command_template = 9; string ship_command_template = 10; string help_command_template = 11; string triage_prompt_template = 12; string review_prompt_template = 13; string initial_prompt_template = 14; google.protobuf.Timestamp created_at = 15; google.protobuf.Timestamp updated_at = 16; string content_hash = 17; }`. `content_hash` is the mode's CURRENT live content hash (same SHA-256-hex-16 computation as `resolvedPipelineMode.ContentHash`/`ItemSessionSummary.PipelineModeSnapshotHash`, computed on read by the RPC handler from the row's current 9 template fields — not stored as its own DB column, since it's fully derived) — this is what lets the frontend's "what ran" surface (Story 3.4.1) detect drift by comparing a session's frozen `pipelineModeSnapshotHash` against the matching mode's current `content_hash`.
  - *Given* the message definition above, *When* `make proto-gen` runs, *Then* `session/gen/proto/go/session/v1/backlog.pb.go` gains a `PipelineMode` Go struct with all 15 fields and `web-app/src/gen/session/v1/backlog_pb.ts` gains the matching TS type.
- **Files**: `proto/session/v1/backlog.proto`

##### Task 2.1.1a: Write the `PipelineMode` message, including the derived `content_hash` field (~5 min)
- Files: `proto/session/v1/backlog.proto`
- Note for Epic 2.2's handler implementation: every response path that returns a `PipelineMode` (Create/Update/Get/List) must compute `content_hash` via the same `computeContentHash` helper Task 1.3.2a defines, applied to the just-persisted/just-read row's 9 template fields — not left as `""`.

#### Story 2.1.2: Define CRUD request/response messages + RPCs on `BacklogService`
**As a** frontend developer, **I want** `CreatePipelineMode`/`UpdatePipelineMode`/`DeletePipelineMode`/`GetPipelineMode`/`ListPipelineModes` RPCs, **so that** the management UI (Epic 3.3) has an API to call.
**Acceptance Criteria**:
- `proto/session/v1/backlog.proto`'s `service BacklogService` (line 354) gains 5 new `rpc` declarations, request/response messages following the exact shape of `CreateItemSourceRequest`/`Response` etc. (lines 404-413): `CreatePipelineModeRequest{slug, name, description, enabled, ...9 template fields}` → `CreatePipelineModeResponse{PipelineMode item}`; `UpdatePipelineModeRequest{id, ...optional variants of the same fields}` → `UpdatePipelineModeResponse{PipelineMode item}`; `DeletePipelineModeRequest{id}` → `DeletePipelineModeResponse{}`; `GetPipelineModeRequest{slug}` → `GetPipelineModeResponse{PipelineMode item}`; `ListPipelineModesRequest{}` → `ListPipelineModesResponse{repeated PipelineMode items}`.
  - *Given* the RPC definitions, *When* `make proto-gen` runs, *Then* `session/gen/proto/go/session/v1/backlog.connect.go` (or equivalent generated ConnectRPC file) gains 5 new handler method stubs on the `BacklogServiceHandler` interface.
- **Files**: `proto/session/v1/backlog.proto`

##### Task 2.1.2a: Write `CreatePipelineMode`/`UpdatePipelineMode` request/response messages + RPCs (~5 min)
- Files: `proto/session/v1/backlog.proto`

##### Task 2.1.2b: Write `DeletePipelineMode`/`GetPipelineMode`/`ListPipelineModes` request/response messages + RPCs (~5 min)
- Files: `proto/session/v1/backlog.proto`

##### Task 2.1.2c: Run `make proto-gen` and verify generated handler stubs (~3 min)
- Files: `session/gen/proto/go/session/v1/*.go` (generated), `web-app/src/gen/session/v1/*.ts` (generated)

---

### Epic 2.2: Go service handlers + cache invalidation
**Goal**: The 5 RPCs are implemented on `BacklogService`, and every write path calls `pipelineEngine.cache.Invalidate` so reads never see stale data after an operator edit.

#### Story 2.2.1: Implement `CreatePipelineMode`/`UpdatePipelineMode` handlers
**As a** backend developer, **I want** the create/update RPC handlers implemented, **so that** the management UI can persist mode definitions.
**Acceptance Criteria**:
- A new file `server/services/backlog_service_pipeline_mode.go` implements `CreatePipelineMode` (calls `s.pipelineModeRepo.Create(...)`, then `s.pipelineEngine.InvalidateCache(ctx)` — new exported method on `CachingPipelineEngine` wrapping the internal cache's `Invalidate`) and `UpdatePipelineMode` (partial-update via `PipelineModeUpdateInput`, same invalidation call).
  - *Given* an empty `pipeline_modes` table, *When* `CreatePipelineMode` is called with `{slug: "quick", name: "Quick Fix", enabled: true, triage_prompt_template: "Fix {{item_id}} fast."}`, *Then* the response's `item.slug == "quick"` AND a subsequent `SlashCommandSet`/`TriagePromptFor` call for an item with `PipelineMode: "quick"` immediately reflects the new mode (no stale-cache window) because `InvalidateCache` was called synchronously before the handler returned.
- **Cache-invalidation failure after a successful DB write does not fail the RPC**: if `s.pipelineModeRepo.Create`/`Update` succeeds but the subsequent `s.pipelineEngine.InvalidateCache(ctx)` call itself fails (e.g. a transient DB hiccup on the `ListEnabled` re-fetch inside `Invalidate`), the handler still returns success to the caller — the row IS correctly persisted, and failing the RPC over a read-side cache-refresh hiccup would be a confusing, inaccurate error (the write did not fail). The handler logs `log.WarningLog.Printf("[PipelineEngine] cache invalidation failed after successful write id=%s: %v — cache may be stale until next successful invalidation", id, err)` at Warn level and returns the success response as normal. This is distinct from the NFR's "fail closed and loud" language, which covers mode-*resolution* failures (unresolvable slug), not this *invalidation* failure mode — per adversarial-review.md's Concern 2.
  - *Given* a `PipelineModeRepository` test double whose `Update` succeeds but whose engine's `InvalidateCache` is forced to fail (e.g. via a fake repository's `ListEnabled` returning an error only on the re-fetch triggered by `Invalidate`), *When* `UpdatePipelineMode` is called, *Then* the RPC still returns a success response with the updated row's data, AND a `[PipelineEngine]` Warn log line naming the invalidation failure is emitted.
- **Files**: `server/services/backlog_service_pipeline_mode.go`

##### Task 2.2.1a: Implement `CreatePipelineMode` (~5 min)
- Files: `server/services/backlog_service_pipeline_mode.go`

##### Task 2.2.1b: Implement `UpdatePipelineMode` (~5 min)
- Files: `server/services/backlog_service_pipeline_mode.go`

##### Task 2.2.1c: Add `InvalidateCache(ctx)` method to `CachingPipelineEngine` (~3 min)
- Files: `session/pipeline_engine.go`

##### Task 2.2.1d: Test cache-invalidation failure after a successful write does not fail the RPC (~4 min)
- Add a test proving the acceptance-criteria scenario above: a successful `Create`/`Update` followed by a forced `InvalidateCache` failure still returns RPC success with a Warn log, not an error response — per adversarial-review.md's Concern 2.
- Files: `server/services/backlog_service_pipeline_mode_test.go`

#### Story 2.2.2: Implement `DeletePipelineMode`/`GetPipelineMode`/`ListPipelineModes` handlers
**As a** backend developer, **I want** the remaining 3 RPCs implemented, **so that** the management UI can list and remove mode definitions.
**Acceptance Criteria**:
- `DeletePipelineMode` calls `s.pipelineModeRepo.Delete(...)` then `s.pipelineEngine.InvalidateCache(ctx)` — per the Unresolved Questions default, does NOT block on existing item references (relies on fail-closed resolution, Story 1.3.3).
- `GetPipelineMode`/`ListPipelineModes` are read-only, calling `GetBySlug`/`ListAll` respectively (not `ListEnabled` — the management UI must see disabled modes too, to allow re-enabling them).
  - *Given* a mode `"quick"` referenced by an existing `BacklogItemData.PipelineMode`, *When* `DeletePipelineMode` is called for `"quick"`'s ID, *Then* the delete succeeds (no referential-integrity error) and a subsequent triage for that item falls back to default mode with a Warn log (Story 1.3.3's behavior, now exercised via a real delete instead of a test fixture).
- **Files**: `server/services/backlog_service_pipeline_mode.go`

##### Task 2.2.2a: Implement `DeletePipelineMode` (~4 min)
- Files: `server/services/backlog_service_pipeline_mode.go`

##### Task 2.2.2b: Implement `GetPipelineMode`/`ListPipelineModes` (~4 min)
- Files: `server/services/backlog_service_pipeline_mode.go`

##### Task 2.2.2c: Wire `pipelineModeRepo` into `BacklogService` construction (~3 min)
- `server/services/backlog_service.go`: add `pipelineModeRepo session.PipelineModeRepository` field + constructor param; `server/dependencies.go`: pass the same `pipelineModeRepo` constructed in Story 1.5.1a.
- Files: `server/services/backlog_service.go`, `server/dependencies.go`

#### Story 2.2.3: Register the 5 handlers + integration tests
**As a** backend developer, **I want** the handlers registered on the ConnectRPC mux and covered by integration tests, **so that** the RPCs are actually reachable and correct end-to-end.
**Acceptance Criteria**:
- `server/server.go` (or wherever `BacklogService`'s handler is mounted) requires no change if `BacklogService` already implements the full `BacklogServiceHandler` interface (ConnectRPC generates a single interface per service) — confirm via `go build ./...` that `BacklogService` still satisfies `sessionv1connect.BacklogServiceHandler` after the 5 new methods are added.
- Integration tests in `server/services/backlog_service_pipeline_mode_test.go` cover: full Create→Get→Update→Delete round trip; cache invalidation is observable (a `SlashCommandSet` call before and after an `UpdatePipelineMode` reflects the change without a process restart); `ListPipelineModes` includes disabled modes, `ListEnabled`-backed cache does not.
  - *Given* a `PipelineMode` created via `CreatePipelineMode` then immediately updated via `UpdatePipelineMode` to change its `triage_prompt_template`, *When* `TriagePromptFor` is called for an item referencing that mode within the same test (no restart), *Then* it reflects the updated template, not the original one.
- **Files**: `server/services/backlog_service_pipeline_mode_test.go`

##### Task 2.2.3a: Verify interface satisfaction + write the CRUD round-trip test (~5 min)
- Files: `server/services/backlog_service_pipeline_mode_test.go`

##### Task 2.2.3b: Write the cache-invalidation-observability test (~4 min)
- Files: `server/services/backlog_service_pipeline_mode_test.go`

---

### Epic 2.3: Structural validation of content-template fields
**Goal**: A malformed mode definition fails predictably (validation error) at write time, per the NFR's "structural integrity, not access control" requirement.

#### Story 2.3.1: Validate content-template fields on Create/Update
**As an** operator, **I want** a malformed mode definition rejected with a clear error, **so that** a typo doesn't silently produce a broken prompt or invalid slash-command file days later.
**Acceptance Criteria**:
- `CreatePipelineMode`/`UpdatePipelineMode` handlers call a new `session.ValidatePipelineModeContent(fields)` function before persisting: rejects an empty `slug` or a `slug` containing characters outside `[a-z0-9-]` (mirrors whatever slug validation `WorkflowRepository`'s create path already uses — locate via `grep -n "slug" session/ent_workflow_repository.go`), rejects any content-template field containing raw shell metacharacters intended to prevent accidental future misuse if a template is ever read into a shell context (defense in depth — the design itself never does this per the Constraints/NFR, but validation makes the invariant enforced, not just documented), AND — implementing the Unresolved Questions section's now-resolved default — scans every one of the 9 content-template fields for `{{...}}` tokens and rejects the request if any token's name is not in the recognized allow-list (`item_id`, `item_title`, `item_description`, `criteria_index`, `criteria_count`, `criteria_text`, `repo_path` — the same list `renderTemplate` in Task 1.3.3d recognizes). This closes the gap flagged in adversarial-review.md's Blocker 2: Phase 1's `renderTemplate` passthrough silently left unknown tokens un-substituted with no write-time enforcement anywhere; from Phase 2 onward, an unrecognized placeholder is rejected at the RPC boundary before it can ever be persisted, matching the NFR's "fail predictably (validation error) rather than producing a broken prompt" requirement.
  - *Given* `CreatePipelineModeRequest{slug: "Quick Fix!", ...}` (invalid slug — uppercase + space + punctuation), *When* `CreatePipelineMode` is called, *Then* it returns `connect.CodeInvalidArgument` with a message naming the invalid field, and no row is written.
  - *Given* `CreatePipelineModeRequest{slug: "quick", triage_prompt_template: "Fix {{item_id}} using {{made_up_placeholder}}.", ...}` (unrecognized placeholder), *When* `CreatePipelineMode` is called, *Then* it returns `connect.CodeInvalidArgument` naming `triage_prompt_template` and the unrecognized token `made_up_placeholder`, and no row is written.
  - *Given* `CreatePipelineModeRequest{slug: "quick", triage_prompt_template: "Fix {{item_id}}: {{item_title}}.", ...}` (all recognized placeholders), *When* `CreatePipelineMode` is called, *Then* it succeeds.
- **Files**: `session/pipeline_engine.go` (or a new `session/pipeline_mode_validation.go`), `server/services/backlog_service_pipeline_mode.go`

##### Task 2.3.1a: Implement `ValidatePipelineModeContent` including placeholder allow-list scanning (~6 min)
- Extract `{{...}}` tokens from each of the 9 content-template fields (same tokenizer shape `renderTemplate` already uses for substitution, reused here for scanning-only rather than substitution) and reject any token not in the 7-name allow-list, naming both the offending field and the unrecognized token in the returned error.
- Files: `session/pipeline_mode_validation.go`

##### Task 2.3.1b: Call validation from both Create and Update handlers (~3 min)
- Files: `server/services/backlog_service_pipeline_mode.go`

##### Task 2.3.1c: Validation unit tests, including the placeholder allow-list cases (~5 min)
- Cover: invalid slug rejected, valid slug accepted, shell-metacharacter content rejected, a template with an unrecognized placeholder rejected (naming the field + token), a template using only recognized placeholders accepted.
- Files: `session/pipeline_mode_validation_test.go`

---

## Phase 3: Frontend

**Scope note**: this phase ships the full CRUD management UI (Epic 3.3: create/edit/enable/disable/delete via `/settings/pipeline-modes`), not a minimal/CLI-first cut, as a deliberate choice — see the Risk Control section's "Appetite / scope-cut check-in" entry for the reasoning and the fallback if the appetite is tight by the time this phase starts.

### Epic 3.1: Shared `RadioGroup` component (extracted from `SessionTypeRadioGroup`)
**Goal**: A reusable, parameterized ARIA radiogroup component exists, with the 2 known a11y gaps fixed, ready to back both the existing session-type selector and the new pipeline-mode selector.

#### Story 3.1.1: Extract `RadioGroup` from `OmnibarCreationPanel.tsx`
**As a** frontend developer, **I want** `SessionTypeRadioGroup`'s rendering logic generalized into a standalone component, **so that** the pipeline-mode selector doesn't duplicate ~130 lines of ARIA radiogroup implementation.
**Acceptance Criteria**:
- `web-app/src/components/ui/RadioGroup.tsx` exports `RadioGroup<T extends string>({ options, value, onChange, groupLabel, hintForValue }: RadioGroupProps<T>)`, where `options: {value: T; label: string; description?: string}[]`, implementing: `role="radiogroup"` + `role="radio"` + `aria-checked` per button (not `aria-selected`), roving tabindex + arrow-key cycling (arrow keys move AND select, no Space/Enter requirement) — logic ported verbatim from `OmnibarCreationPanel.tsx`'s current `SessionTypeRadioGroup` implementation (lines ~105-150).
  - *Given* `<RadioGroup options={[{value:"a",label:"A"},{value:"b",label:"B"}]} value="a" onChange={fn} groupLabel="Test" />`, *When* the user presses ArrowRight while option "a" is focused, *Then* `fn` is called with `"b"` and the second button's `aria-checked` becomes `"true"`.
- `OmnibarCreationPanel.tsx` is refactored to use the new `RadioGroup` for its `SESSION_TYPES` selector, with no behavior change (verified by existing Omnibar tests continuing to pass unmodified).
- **Files**: `web-app/src/components/ui/RadioGroup.tsx`, `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 3.1.1a: Create `RadioGroup.tsx` with the ported rendering logic (~5 min)
- Files: `web-app/src/components/ui/RadioGroup.tsx`

##### Task 3.1.1b: Refactor `OmnibarCreationPanel.tsx` to use `RadioGroup` (~5 min)
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 3.1.1c: Run existing Omnibar tests to confirm no behavior change (~3 min)
- `cd web-app && npx jest --no-coverage --testPathPatterns="OmnibarCreationPanel"`
- Files: N/A

#### Story 3.1.2: Fix the 2 known a11y gaps during extraction
**As a** screen-reader user, **I want** the radiogroup's visible label and hint text programmatically linked, **so that** I hear both when tabbing to the group — not just when the label is visually adjacent.
**Acceptance Criteria**:
- `RadioGroup` accepts a `groupLabelId` (or generates one via `useId()`) and sets `aria-labelledby={groupLabelId}` on the `role="radiogroup"` div, replacing the original's `aria-label` string duplicate.
- `RadioGroup` accepts an optional hint span, gives it a stable `id`, and sets `aria-describedby` on the radiogroup div pointing at it.
  - *Given* a `RadioGroup` with `groupLabel="Pipeline mode"` and a hint span "Choose which skills drive this item", *When* a screen reader (or `@testing-library`'s accessible-name query) inspects the radiogroup, *Then* its accessible name is "Pipeline mode" (via `aria-labelledby`) and its accessible description includes the hint text (via `aria-describedby`) — both verified via `getByRole("radiogroup", { name: "Pipeline mode", description: /Choose which skills/ })` in a test.
- **Files**: `web-app/src/components/ui/RadioGroup.tsx`

##### Task 3.1.2a: Implement `aria-labelledby`/`aria-describedby` wiring (~4 min)
- Files: `web-app/src/components/ui/RadioGroup.tsx`

##### Task 3.1.2b: Add `RadioGroup.test.tsx` covering the a11y wiring (~5 min)
- Files: `web-app/src/components/ui/RadioGroup.test.tsx`

---

### Epic 3.2: Pipeline-mode selector in `BacklogItemForm.tsx`
**Goal**: A user can choose an existing pipeline mode on a backlog item, with the 3 existing checkboxes visually regrouped as independent "Overrides" per the compose-not-subsume UX decision.

#### Story 3.2.1: Fetch available modes and add `pipelineMode` form state
**As a** backlog UI operator, **I want** the form to load the list of enabled pipeline modes, **so that** I can select one when creating/editing an item.
**Acceptance Criteria**:
- `web-app/src/lib/hooks/useBacklogService.ts` gains a `listPipelineModes()` function calling the new `ListPipelineModes` RPC (mirroring the existing pattern for `autoSpawnSession`'s data flow at lines 87/128/270/430/460 — locate the equivalent `list*`-style hook for a comparable precedent, e.g. `useWorkflows` if one exists, else the `ItemSource` list hook).
- `BacklogItemForm.tsx` gains `const [pipelineMode, setPipelineMode] = useState(initialValues?.pipelineMode ?? "")` (line ~41 region, alongside the 3 existing `useState` calls) and includes `pipelineMode` in the `onSubmit` payload (line ~80) and the `useCallback` dependency array (line ~88).
  - *Given* a form with no `initialValues` (new item), *When* the form mounts, *Then* `pipelineMode` state is `""` and the rendered `RadioGroup` shows "Default" selected.
- **G-4 (loading state, ux.md G-table)**: while `listPipelineModes()` is pending, the `RadioGroup` renders with only the hardcoded `"Default"` option enabled/selectable and any other options disabled or skeleton-rendered — form submission is never blocked while the fetch is in flight.
  - *Given* `listPipelineModes()` has not yet resolved, *When* the form renders, *Then* only the "Default" radio option is interactive and the Save button is not disabled on account of the pending fetch.
- **G-3 (mode-list fetch failure, ux.md G-table)**: if `listPipelineModes()` rejects, the `RadioGroup` still renders with only `"Default"` present and selectable, and a `role="status"` inline notice reads *"Couldn't load pipeline modes — you can still save with Default."* — the form remains fully submittable.
  - *Given* `listPipelineModes()` rejects, *When* the form renders, *Then* the `role="status"` notice with that exact copy is present, "Default" is selectable, and submitting the form with no other changes succeeds.
- **G-2 (item references an unresolvable mode, ux.md G-table)**: when editing an item whose `initialValues.pipelineMode` does not match any slug in the fetched (enabled-only) mode list, render an extra synthetic disabled radio option at the position it would occupy — label `"Unknown mode ('<slug>')"`, `aria-checked="true"`, `aria-disabled="true"` — plus a hint: *"This item references a pipeline mode that no longer exists or is disabled. Choosing a different mode below will replace it when you save."* Never silently fall back to showing "Default" as selected.
  - *Given* `initialValues.pipelineMode === "legacy-fast"` and `"legacy-fast"` is absent from the fetched enabled-modes list, *When* the edit form renders, *Then* a disabled radio option labeled `"Unknown mode ('legacy-fast')"` is shown as checked, and no other option is pre-selected in its place.
- **Files**: `web-app/src/lib/hooks/useBacklogService.ts`, `web-app/src/components/backlog/BacklogItemForm.tsx`

##### Task 3.2.1a: Add `listPipelineModes` to `useBacklogService.ts` (~4 min)
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 3.2.1b: Add `pipelineMode` state + submit wiring to `BacklogItemForm.tsx` (~4 min)
- Files: `web-app/src/components/backlog/BacklogItemForm.tsx`

#### Story 3.2.2: Render the `RadioGroup` selector + regroup the 3 checkboxes as "Overrides"
**As a** backlog UI operator, **I want** the mode selector shown as the primary control with the 3 checkboxes visually subordinate and labeled "Overrides," **so that** I understand mode selection and the checkboxes are independent, per `research/ux.md` §2.
**Acceptance Criteria**:
- `BacklogItemForm.tsx` renders `<RadioGroup options={pipelineModes} value={pipelineMode} onChange={setPipelineMode} groupLabel="Pipeline mode" />` above the existing checkbox block (currently lines 214-268 region), and wraps the 3 existing checkboxes (`skipPlanning`/`skipReviewGate`/`autoSpawnSession`, lines 221/239/257) in a `<fieldset><legend>Overrides (independent of pipeline mode)</legend>...</fieldset>` — no change to the checkboxes' own state/logic, purely visual regrouping.
- Each `RadioGroup` option button has `data-testid={`backlog-pipeline-mode-${mode.slug || "default"}`}` per `.claude/rules/e2e-test-conventions.md`.
  - *Given* the form is rendered, *When* a user inspects the DOM, *Then* the mode selector appears before the "Overrides" fieldset in document order, and the fieldset's `<legend>` text is "Overrides (independent of pipeline mode)".
- **G-1 (unmet mode prerequisite, ux.md G-table)**: if the selected mode's content-template fields reference `{{repo_path}}` and `item.repoPath` is empty, render a non-blocking inline warning below the "Overrides" fieldset — *"⚠ <Mode name> mode requires a repository path — add one above."* — using the same `role="alert"` treatment as the existing `errors.repoPath` field error. This warning never disables the mode selection or blocks submission (warn, don't disable — mirrors G-1's non-blocking contract).
  - *Given* a mode whose content references `{{repo_path}}` is selected and `item.repoPath === ""`, *When* the form renders, *Then* a `role="alert"` warning naming the selected mode is visible, and the Save button remains enabled.
- **Files**: `web-app/src/components/backlog/BacklogItemForm.tsx`

##### Task 3.2.2a: Render `RadioGroup` + add `data-testid`s (~4 min)
- Files: `web-app/src/components/backlog/BacklogItemForm.tsx`

##### Task 3.2.2b: Wrap the 3 checkboxes in the "Overrides" fieldset (~3 min)
- Files: `web-app/src/components/backlog/BacklogItemForm.tsx`

##### Task 3.2.2c: Update `BacklogItemForm.test.tsx` for the new selector + fieldset (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemForm.test.tsx`

#### Story 3.2.3: Feature registry entry for the selector
**As a** maintainer, **I want** the new selector registered per `.claude/rules/feature-registry.md`, **so that** coverage tooling tracks it.
**Acceptance Criteria**:
- `docs/registry/features/frontend/backlog-pipeline-mode-selector.json` is created with `id: "backlog-pipeline-mode-selector"`, `filePath: "web-app/src/components/backlog/BacklogItemForm.tsx"`, `tested: false` initially (flips to `true` once Epic 4.3's e2e test lands).
  - *Given* the new file, *When* `make registry-generate` runs, *Then* `docs/registry/frontend-features.json` (generated aggregate) includes the new entry with no errors.
- **Files**: `docs/registry/features/frontend/backlog-pipeline-mode-selector.json`

##### Task 3.2.3a: Create the registry entry + run `make registry-generate` (~3 min)
- Files: `docs/registry/features/frontend/backlog-pipeline-mode-selector.json`

---

### Epic 3.3: Management CRUD page (`/settings/pipeline-modes`)
**Goal**: An operator can create/edit/enable/disable pipeline-mode definitions through a dedicated settings page, mirroring `/settings/backlog-sources`.

#### Story 3.3.1: Scaffold the settings page + list view
**As an** operator, **I want** a page listing all pipeline modes (enabled and disabled), **so that** I can see what exists before creating a new one.
**Acceptance Criteria**:
- `web-app/src/app/settings/pipeline-modes/page.tsx` is created, following `web-app/src/app/settings/backlog-sources/page.tsx`'s structure (client component, fetches via `useBacklogService`'s `listPipelineModes` from Story 3.2.1, renders a table/list of `{slug, name, enabled}`).
- `web-app/src/app/settings/page.tsx` (the settings index) gains a nav link/card to `/settings/pipeline-modes`, mirroring the existing link to `/settings/backlog-sources`.
  - *Given* 2 pipeline modes exist (`"quick"` enabled, `"legacy"` disabled), *When* an operator navigates to `/settings/pipeline-modes`, *Then* both rows render, with `"legacy"`'s row visually indicating disabled state (e.g. dimmed/badge), matching `/settings/backlog-sources`'s existing enabled/disabled treatment.
- **Files**: `web-app/src/app/settings/pipeline-modes/page.tsx`, `web-app/src/app/settings/page.tsx`

##### Task 3.3.1a: Scaffold `page.tsx` with the list view (~5 min)
- Files: `web-app/src/app/settings/pipeline-modes/page.tsx`

##### Task 3.3.1b: Add the nav link on the settings index (~2 min)
- Files: `web-app/src/app/settings/page.tsx`

#### Story 3.3.2: Create/edit form + enable/disable/delete actions
**As an** operator, **I want** to create a new mode, edit an existing one's content-template fields, and enable/disable/delete it, **so that** I can iterate on pipeline modes without an engineer.
**Acceptance Criteria**:
- The page includes a form (new component `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`) with fields for `slug` (disabled/read-only on edit — slugs are immutable after creation, matching `Workflow`'s convention if confirmed via a quick check of `web-app/src/app/settings` workflow-editing UI, else document this as a new, explicitly-stated invariant), `name`, `description`, `enabled` toggle, and 9 `<textarea>` inputs for the content-template fields, each labeled with its target file/prompt name (e.g. "Triage prompt", "review.md content").
- Submitting calls `CreatePipelineMode`/`UpdatePipelineMode`; a "Delete" button (with a confirm dialog, matching existing delete-confirmation UX elsewhere in `/settings`) calls `DeletePipelineMode`.
  - *Given* the create form filled with `slug: "quick", name: "Quick Fix", triage_prompt_template: "Fix {{item_id}} fast."` and all other content-template fields left blank, *When* the operator submits, *Then* `CreatePipelineMode` is called with those values and, on success, the new mode appears in the list view from Story 3.3.1 without a page reload.
  - *Given* an invalid slug (`"Quick Fix!"`), *When* the operator submits, *Then* the form displays the `CodeInvalidArgument` error message returned by the backend validation (Story 2.3.1) inline, and no navigation/list-refresh occurs.
- **Files**: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`, `web-app/src/app/settings/pipeline-modes/page.tsx`

##### Task 3.3.2a: Build `PipelineModeForm.tsx` with the 9 content-template textareas (~5 min)
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`

##### Task 3.3.2b: Wire Create/Update submit handlers + inline error display (~5 min)
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`

##### Task 3.3.2c: Wire enable/disable toggle + delete-with-confirm (~5 min)
- Files: `web-app/src/app/settings/pipeline-modes/page.tsx`

##### Task 3.3.2d: `PipelineModeForm.test.tsx` covering create, validation error, and delete-confirm (~5 min)
- Files: `web-app/src/app/settings/pipeline-modes/PipelineModeForm.test.tsx`

#### Story 3.3.3: Feature registry entry for the management page
**Acceptance Criteria**:
- `docs/registry/features/frontend/backlog-pipeline-mode-management.json` created per `.claude/rules/feature-registry.md`'s template.
  - *Given* the new file, *When* `make registry-generate` runs, *Then* it appears in the aggregated `frontend-features.json` with `tested: false` until Epic 4.3.
- **Files**: `docs/registry/features/frontend/backlog-pipeline-mode-management.json`

##### Task 3.3.3a: Create the registry entry + run `make registry-generate` (~3 min)
- Files: `docs/registry/features/frontend/backlog-pipeline-mode-management.json`

---

### Epic 3.4: "What ran" read-only surface in `BacklogItemDetail.tsx`
**Goal**: For any item, an operator can see which mode actually drove each session — reading the frozen `ItemSession.pipelineModeSnapshot`, never the item's live (possibly since-changed) field.

#### Story 3.4.1: Render the snapshot per `ItemSession` in `BacklogItemDetail.tsx`, with content-drift detection
**As a** backlog UI operator, **I want** to see which pipeline mode drove each session on an item, and be warned if that mode's own content has since changed, **so that** I can verify a risky change didn't silently ride through a reduced-scrutiny mode — or through a mode whose behavior has since drifted — per the Trust job-to-be-done in `research/ux.md` §5 and architecture-review.md's Blocker 1 remediation (a).
**Acceptance Criteria**:
- `BacklogItemDetail.tsx` (near the existing `<GateVerdictBox>` render at line ~702) adds a `role="group"` labeled "Pipeline" section per `ItemSession`, showing `session.pipelineModeSnapshot || "default"`. If the snapshot slug is not found in the currently-fetched mode list (deleted/renamed), it degrades to `"custom (unrecognized mode: '<slug>')"` rather than a blank — per `research/ux.md` §4's explicit fallback requirement.
- **Content-drift detection**: if the snapshot slug IS found in the currently-fetched mode list (i.e. the mode still exists), compare `session.pipelineModeSnapshotHash` (added to the `ItemSession` proto message by Task 1.6.1c) against the matching mode's current `content_hash` (added to the `PipelineMode` proto message by Story 2.1.1). If they differ (and `session.pipelineModeSnapshotHash` is non-empty — an empty snapshot hash means "default mode" or "pre-this-feature session," neither of which has a meaningful comparison), append `" (content since changed)"` to the displayed mode name.
  - *Given* an `ItemSession` with `pipelineModeSnapshot: "quick"` and `"quick"` still exists in the current mode list with an unchanged `content_hash` matching the session's `pipelineModeSnapshotHash`, *When* `BacklogItemDetail` renders that session, *Then* the "Pipeline" group shows "Quick Fix" (the mode's `name`, resolved by matching `slug`), with no drift annotation.
  - *Given* the same session, but an operator has since edited `"quick"`'s `triage_prompt_template` (so the live mode's `content_hash` no longer matches the session's stored `pipelineModeSnapshotHash`), *When* `BacklogItemDetail` renders that session, *Then* the "Pipeline" group shows `"Quick Fix (content since changed)"` — this is the concrete fix for architecture-review.md's Blocker 1: an operator editing a mode's content no longer silently rewrites what every historical session using that mode appears to have run.
  - *Given* an `ItemSession` with `pipelineModeSnapshot: "legacy-fast"` where `"legacy-fast"` no longer exists in the current mode list, *When* `BacklogItemDetail` renders that session, *Then* the "Pipeline" group shows `"custom (unrecognized mode: 'legacy-fast')"`, not a blank or `undefined` (this case takes priority over drift detection — there's no live mode to compare against).
- **Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 3.4.1a: Render the per-session "Pipeline" group with the found/unrecognized fallback AND the content-drift comparison/annotation (~6 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 3.4.1b: Update `BacklogItemDetail.test.tsx` covering the found (unchanged), found-but-drifted, and unrecognized-mode cases (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

---

## Phase 4: Proof of Seam + Registry + E2E

### Epic 4.1: Define a real "quick" pipeline mode via the live UI
**Goal**: Prove the seam is not cosmetic — a mode created entirely through the UI (no code change, no redeploy) measurably changes what `WriteSlashCommands` writes and what the triage/review LLM calls receive, end to end.

#### Story 4.1.1: Create and use the "quick" mode on a real item
**As an** operator, **I want** to define a "quick" mode through `/settings/pipeline-modes` and select it on a backlog item, **so that** the success metric ("adding a new mode requires no engineering involvement... immediately use it on a backlog item") is demonstrably true.
**Acceptance Criteria**:
- Using the UI built in Phase 3, an operator creates a `PipelineMode{slug: "quick", name: "Quick Fix", triage_prompt_template: "This is a quick-fix item — skip deep architecture analysis, focus on the smallest correct change for: {{item_id}}.", ...}` with distinct content in at least 2 of the 9 template fields (to prove more than one call site varies).
- The operator selects `"quick"` on an existing (or new) backlog item via `BacklogItemForm.tsx`'s selector (Epic 3.2) and triggers triage.
  - *Given* the `"quick"` mode created above and selected on item `X`, *When* triage is triggered for item `X`, *Then* the `[PipelineEngine] item=X stage=triage mode="quick"` log line appears (Story 1.7.2) AND the LLM call's prompt contains the custom `triage_prompt_template` text, verified by inspecting the stored `TriageResult`/prompt-capture mechanism already used for triage debugging.
- **Files**: N/A (manual/scripted verification against the running system, not a code change)

##### Task 4.1.1a: Create the "quick" mode via the UI and verify persistence (~3 min)
- Files: N/A

##### Task 4.1.1b: Select "quick" on a test item, trigger triage, verify the log line and prompt content (~5 min)
- Files: N/A

##### Task 4.1.1c: Verify `WriteSlashCommands` output differs from default for the same item (~4 min)
- Compare the written `.claude/commands/backlog/*.md` files for the test item against a default-mode item's files — confirm at least one file's content differs, proving `SlashCommandSet` genuinely varies, not just logs a mode name.
- Files: N/A

---

### Epic 4.2: Observability polish
**Goal**: Confirm the full Observability Requirements are met end-to-end, not just per-unit-test.

#### Story 4.2.1: End-to-end log verification across all 4 call sites + cache events
**Acceptance Criteria**:
- Running the "quick" mode item from Epic 4.1 through triage, a work session spawn, and a review produces, in order: an Info log at triage-start, an Info log at review-start, at least one Debug cache-load log at process startup, and (by deleting the mode mid-flow on a second test item) a Warn fallback log — all captured in one manual verification pass against `~/.stapler-squad/logs/stapler-squad.log`.
  - *Given* the log file after the full flow above, *When* grepped for `[PipelineEngine]`, *Then* it contains at least one Info line per stage, at least one Debug cache line, and (from the deletion sub-case) exactly one Warn line naming the deleted slug.
- **Files**: N/A

##### Task 4.2.1a: Run the full flow and grep the log file for the expected `[PipelineEngine]` lines (~5 min)
- Files: N/A

---

### Epic 4.3: Feature registry + e2e tests
**Goal**: Close out the feature-registry and e2e-coverage obligations from `.claude/rules/feature-registry.md` and `.claude/rules/feature-testing-registry.md`.

#### Story 4.3.1: Backend feature registry entries for the 5 new RPCs
**Acceptance Criteria**:
- `docs/registry/features/backend/` gains one `.json` file per new RPC (`create-pipeline-mode.json`, `update-pipeline-mode.json`, `delete-pipeline-mode.json`, `get-pipeline-mode.json`, `list-pipeline-modes.json`), each with `markerFound: true` once a `// +api: pipeline-mode:create` (etc.) marker is added to the corresponding handler in `server/services/backlog_service_pipeline_mode.go`.
  - *Given* the 5 new files and markers, *When* `make registry-generate` runs, *Then* `docs/registry/backend-features.json` includes all 5 with no `coverage-gaps.json` net increase once `testIds` are populated by Story 4.3.2.
- **Files**: `docs/registry/features/backend/create-pipeline-mode.json`, `docs/registry/features/backend/update-pipeline-mode.json`, `docs/registry/features/backend/delete-pipeline-mode.json`, `docs/registry/features/backend/get-pipeline-mode.json`, `docs/registry/features/backend/list-pipeline-modes.json`, `server/services/backlog_service_pipeline_mode.go`

##### Task 4.3.1a: Add `// +api:` markers to the 5 handlers (~3 min)
- Files: `server/services/backlog_service_pipeline_mode.go`

##### Task 4.3.1b: Create the 5 registry JSON files + run `make registry-generate` (~5 min)
- Files: `docs/registry/features/backend/*.json`

#### Story 4.3.2: E2E test for mode selection + management
**As a** QA reviewer, **I want** an e2e test exercising the mode selector and management page, **so that** this feature has the required Playwright coverage per `.claude/rules/e2e-test-conventions.md`.
**Acceptance Criteria**:
- `tests/e2e/backlog-pipeline-mode.spec.ts` starts with `// @feature backlog:pipeline-mode-select, backlog:pipeline-mode-manage` and covers: (1) creating a mode via `/settings/pipeline-modes`, (2) selecting it on a backlog item via `getByTestId("backlog-pipeline-mode-quick")`, (3) verifying the "what ran" surface shows the mode name after a triage run, using only `data-testid`/ARIA-role locators (no CSS class selectors) and no `waitForTimeout` (uses `expect(locator).toHaveValue(...)`/`waitForSelector` per the convention).
  - *Given* a running test server (`STAPLER_SQUAD_INSTANCE=e2e-local`), *When* `npx playwright test backlog-pipeline-mode.spec.ts` runs, *Then* all 3 scenarios pass.
- **Files**: `tests/e2e/backlog-pipeline-mode.spec.ts`

##### Task 4.3.2a: Write the mode-creation + selection e2e scenario (~5 min)
- Files: `tests/e2e/backlog-pipeline-mode.spec.ts`

##### Task 4.3.2b: Write the "what ran" verification e2e scenario (~5 min)
- Files: `tests/e2e/backlog-pipeline-mode.spec.ts`

##### Task 4.3.2c: Run the new e2e spec, then flip `tested: true` + populate `testIds` on all 4.3.1/3.2.3/3.3.3 registry entries, run `make registry-generate` (~5 min)
- Files: `docs/registry/features/backend/*.json`, `docs/registry/features/frontend/backlog-pipeline-mode-selector.json`, `docs/registry/features/frontend/backlog-pipeline-mode-management.json`
