# Architecture Research: `PipelineEngine` Seam

Research Agent 3 (Architecture) — SDD Phase 2, `backlog-configurable-pipeline`.

## 1. Rabbit Hole #1 — `PipelineEngine` vs `WorkflowEngine`

### What `WorkflowEngine` actually governs

`session/workflow_engine.go` (65 lines, read in full):

```go
type WorkflowEngine interface {
    CanTransition(from, to BacklogStatus) bool
    ValidateGates(item BacklogItemTransitionInput, to BacklogStatus) error
    AllowedTransitions(from BacklogStatus) []BacklogStatus
}
```

`DefaultWorkflowEngine` wraps the existing `validTransitions` map and `TransitionGuard` free
function, deep-copying the map at construction (`NewDefaultWorkflowEngine`) exactly the
deep-copy-on-construct pattern the requirements doc calls out to reuse.

ADR-013 (read in full) frames `WorkflowEngine` as answering one question only: **"is this
status edge legal, and do its gates pass?"** It is a graph/guard object over `BacklogStatus`
values. Its planned Phase-2 sibling, `ConfiguredWorkflowEngine`, exists to let *states themselves*
be user-defined (DB-backed), not to change what happens *inside* a state.

`PipelineEngine` answers a completely different question: **"given this item is about to do
work (write slash commands / build a triage prompt / build a review prompt), which
skills/commands/prompt content should be used?"** It operates *within* a status, not *between*
statuses. It never decides whether `in_progress → review` is legal — `WorkflowEngine` still owns
that exclusively.

### Recommendation: separate interface, composed by the caller (not extended)

Do **not** extend `WorkflowEngine` with pipeline methods, and do **not** have `PipelineEngine`
call into `WorkflowEngine` or vice versa. They are siblings consulted independently by the same
caller (`BacklogService`), exactly the way `BacklogService` already holds
`engine session.WorkflowEngine` as one field — add `pipelineEngine session.PipelineEngine` as a
second, unrelated field. Justification:

- **Interface segregation** — `WorkflowEngine`'s three methods are consumed by
  `TransitionBacklogItemStatus` and `BacklogLifecycleListener`. `PipelineEngine`'s methods would be
  consumed by `WriteSlashCommands`, `TriggerTriage`, and `ReviewGateRunner.Run` — a disjoint call-site
  set. Merging them into one interface would force every consumer to depend on methods it doesn't use
  (violates the "narrow, consumer-defined" rule in `interface-pollution-checklist.md`).
- **Independent evolution** — ADR-013's Phase 2 (`ConfiguredWorkflowEngine`, custom states) is
  explicitly out of scope here. If `PipelineEngine` methods lived on the same interface,
  `ConfiguredWorkflowEngine` would be forced to also implement pipeline concerns it has no
  reason to know about.
- **No dependency between them at runtime** — a pipeline mode does not need to add or skip
  *statuses* (confirmed by requirements' scope-out: "custom **states** — separate initiative").
  It only changes what happens while the item sits in `in_progress` (which slash commands get
  written) or transiently during `TriggerTriage`/review (which prompt text is built). Since no
  pipeline mode needs to alter the state graph, there is no call from `PipelineEngine` into
  `WorkflowEngine`'s `AllowedTransitions`/`CanTransition`, and no need for the reverse either.

### Proposed method signature sketch

Mirror `WorkflowEngine`'s shape (narrow, deep-copy-on-construct, `Default*` implementation first):

```go
// session/pipeline_engine.go

// PipelineMode identifies a configured pipeline/skill-set for a backlog item.
// Stored as a plain string on BacklogItemData (see §4) — PipelineEngine translates
// the string into behavior; it is not a Go enum so new modes don't require a proto/DB migration.
type PipelineMode string

const (
    PipelineModeDefault PipelineMode = "" // empty string = today's fixed hardcoded pipeline
    PipelineModeSDDFull  PipelineMode = "sdd_full"
)

// PipelineEngine is the runtime policy for which skills/commands/prompts a backlog
// item's pipeline stage uses. Narrow — only the operations needed by WriteSlashCommands,
// TriggerTriage's prompt builder, and ReviewGateRunner.Run.
type PipelineEngine interface {
    // SlashCommandSet returns the set of slash-command files WriteSlashCommands should
    // write for this item's pipeline mode (name -> markdown body), replacing (or extending)
    // the hardcoded status/done-N/fail-N/review/ship set.
    SlashCommandSet(item *BacklogItemData) map[string]string

    // TriagePromptFor returns the triage prompt to use instead of BuildHeadlessTriagePrompt's
    // fixed template, given the item is in the given mode. Falls back internally to the
    // default builder when mode is PipelineModeDefault.
    TriagePromptFor(item *BacklogItemData, artifactAbsPath string) string

    // ReviewPromptFor is the review-gate equivalent, replacing BuildHeadlessReviewPrompt.
    ReviewPromptFor(item *BacklogItemData, acSnapshot []AcCriterion, diff string, diffTruncated bool, verificationNotes string) string
}

// DefaultPipelineEngine reproduces today's hardcoded behavior verbatim (zero regression):
// SlashCommandSet returns exactly what WriteSlashCommands writes today, and the prompt
// methods delegate straight to BuildHeadlessTriagePrompt / BuildHeadlessReviewPrompt.
type DefaultPipelineEngine struct{}

func NewDefaultPipelineEngine() *DefaultPipelineEngine { return &DefaultPipelineEngine{} }
```

Wiring mirrors `WorkflowEngine` exactly: `BacklogService` gains a `pipelineEngine
session.PipelineEngine` field, `NewBacklogService(..., engine session.WorkflowEngine, pipelineEngine
session.PipelineEngine)` nil-defaults to `session.NewDefaultPipelineEngine()` (see
`server/services/backlog_service.go:134-136` for the exact pattern to copy), and
`server/dependencies.go:457-459` gets a second `pipelineEngine :=
session.NewDefaultPipelineEngine()` constructed once at startup alongside `workflowEngine`.

## 2. Rabbit Hole #2 — Is the seam cosmetic without touching `autonomous_driver.go`?

**Definitive answer: not cosmetic, and `autonomous_driver.go` does not need to change.**
Evidence, tracing the actual call chain from backlog item to running orchestrator:

1. `server/services/backlog_service_triage.go:255` (`SpawnSessionFromItem`, step 8):
   `prompt := session.BuildTokenBudgetedPrompt(item, priorSessions)`.
2. `session/backlog_context.go:132-134` (`BuildTokenBudgetedPrompt`): `output :=
   BuildSessionInitialPrompt(item, priorSessions)` (falls back to trimmed variants only if over
   the 4000-token budget).
3. Back in `SpawnSessionFromItem` (`backlog_service_triage.go:281-286`), that `prompt` string is
   passed straight into `s.sessionCreator.CreateWorktreeSession(ctx, title, item.RepoPath,
   worktreePath, prompt, spawnTags, false, false)` — this becomes `inst.Prompt`.
4. If `req.Msg.Autonomous` (`backlog_service_triage.go:314-316`):
   `s.autonomousStarter.StartAutonomousDriverForInstance(inst)`.
5. `server/services/autonomous_orchestration_service.go:165`:
   `driver := session.NewAutonomousDriver(inst, a.pool, inst.Prompt, 0)` — `inst.Prompt` (the
   exact string built in step 2) is passed as the driver's `goal` parameter, verbatim.
6. Inside `session/autonomous_driver.go`, `goal` is treated as an **opaque string** — it is
   never parsed, switched on, or matched against known values. It flows straight into
   `buildOrchestrationPrompt(goal, tail, turnCount, maxTurns)` (`autonomous_driver.go:347-355`),
   which wraps it in `<goal>...</goal>` XML tags for the orchestrator LLM call. The orchestrator
   system prompt (`autonomous_driver.go:336-341`, `autonomousSystemPrompt`) is generic — "direct a
   session toward a goal," reply `NEXT_MESSAGE` or `DONE` — with zero backlog- or
   pipeline-specific logic.

**Conclusion**: `autonomous_driver.go` has no awareness of backlog pipelines, slash commands, or
SDD phases at all — it is a pure goal-string-to-orchestration-turn loop. The entire lever for
changing runtime behavior is the *content* of the prompt built in step 2 (`BuildSessionInitialPrompt`)
and the slash commands written by `WriteSlashCommands` (which the interactive/headless session
reads independently of the driver, via `.claude/commands/backlog/*.md` and the on-disk
`.backlog-context.md` fallback — see `session/backlog_commands.go:114-120`,
`WriteBacklogContextFile`). A `PipelineEngine` that:
- swaps which files `WriteSlashCommands` writes (e.g. write `/sdd:full` pointer files instead of
  the fixed `status/done-N/fail-N/review/ship` set), and/or
- changes what `BuildSessionInitialPrompt`/`BuildTokenBudgetedPrompt` puts in the prompt (e.g.
  "Use `/sdd:full` to work this item" instead of the default backlog instructions)

...genuinely changes what the interactive Claude Code session does *and* what the autonomous
orchestrator LLM sees as its goal — without a single line of `autonomous_driver.go` changing.
This is the concrete, non-cosmetic proof rabbit hole #2 asked for. No scope pull-in of
`autonomous_driver.go` is needed; the deferral in the requirements' Scope (Out) section is safe
as written.

One caveat worth flagging in the ADR: `autonomous_driver.go`'s `waitForRateLimitClear` and
`fireCompletion`/turn-count logic are agnostic of pipeline mode today, and will remain so — if a
future pipeline mode needs a different `maxTurns` or timeout, `NewAutonomousDriver(inst, pool,
goal, maxTurns, opts...)` already accepts `maxTurns` as a parameter (currently hardcoded to `0` at
both call sites), so `PipelineEngine` could expose that too without an interface change — flagged
as a possible follow-on, not required for this project's scope.

## 3. Integration Points — exact call sites

| Site | File:Line | Current signature | What a `PipelineEngine`-aware version needs |
|---|---|---|---|
| Slash command generation | `session/backlog_commands.go:20` `WriteSlashCommands(item *BacklogItemData, worktreePath string) error` | Hardcodes `status.md`, `done-N.md`/`fail-N.md` per AC criterion, `review.md`, `ship.md`, `help.md` | Consult `pipelineEngine.SlashCommandSet(item)` for the file set instead of (or in addition to) the hardcoded block; `DefaultPipelineEngine.SlashCommandSet` returns exactly today's fixed set so `WriteSlashCommands`'s file-writing loop becomes mode-driven with zero behavior change for `PipelineModeDefault`. Called from `server/services/backlog_service_triage.go:436` (`writeSessionFiles`) and `server/services/backlog_service_sync.go:93` — both already have `item *BacklogItemData` in scope, so no extra plumbing needed to reach the engine (assuming `BacklogService` holds the engine as a field, per §1). |
| Triage prompt building | `server/services/backlog_service_triage.go:718` `triagePrompt = session.BuildHeadlessTriagePrompt(item, artifactAbsPath)` (fresh) / `:716` `BuildHeadlessRetriagePrompt(...)` (feedback) inside `TriggerTriage` (func starts `:626`) | `session.BuildHeadlessTriagePrompt(item *BacklogItemData, artifactAbsPath string) string` | Replace the direct call with `s.pipelineEngine.TriagePromptFor(item, artifactAbsPath)`; `DefaultPipelineEngine` delegates straight to `BuildHeadlessTriagePrompt`. The retriage/feedback path (`BuildHeadlessRetriagePrompt`) is a separate function not in this project's explicit method sketch — worth a plan-phase decision on whether it needs its own `PipelineEngine` method or stays fixed (retriage is inherently a "refine the existing plan" operation, arguably mode-independent). |
| Review gate | `session/review_gate.go:54` `func (r *ReviewGateRunner) Run(ctx context.Context, item *BacklogItemData, is ItemSessionSummary, onPass func(...)) ` — prompt built at `:251` `headlessPrompt := BuildHeadlessReviewPrompt(item, acSnapshot, diff, truncated, is.VerificationNotes)` (also called directly at `server/services/backlog_service_triage.go:985` for `TriggerReReview`) | `session.BuildHeadlessReviewPrompt(item *BacklogItemData, acSnapshot []AcCriterion, diff string, diffTruncated bool, verificationNotes string) string` | `ReviewGateRunner` needs a `pipelineEngine PipelineEngine` field (added via `NewReviewGateRunner`, mirroring its existing getter-func injection style) so `Run` can call `r.pipelineEngine.ReviewPromptFor(...)` instead of the free function. `TriggerReReview`'s direct call at `backlog_service_triage.go:985` needs the same swap via `s.pipelineEngine`. Note `Run` short-circuits entirely on `item.SkipReviewGate` (`review_gate.go:60-62`) — a pipeline mode that wants a *different* review flow (not just a different prompt) would need this short-circuit to also consult the engine, but that's out of scope per requirements (mode changes skills/prompts, not the review-gate on/off decision, which stays owned by `SkipReviewGate`). |

`WriteSlashCommands` is filesystem-templating into a git worktree (rabbit hole #3, requirements
§3) — the `SlashCommandSet(item) map[string]string` signature above keeps `PipelineEngine` itself
pure/in-memory (it returns content, not file writes); `WriteSlashCommands` remains the only
place that touches `os.MkdirAll`/`writeFile`. This satisfies the requirement that the engine
itself stay a narrow, testable Go interface while the I/O stays where it already lives.

## 4. Data Flow — `BacklogItemData` from ent to integration points

Traced via `session/repository.go:330-358` (`BacklogItemData` struct) and its callers:

- **Every one of the three integration points above already receives a fully-loaded
  `*BacklogItemData`** before it would need to consult `PipelineEngine`:
  - `WriteSlashCommands` — called from `writeSessionFiles` (`backlog_service_triage.go:433`),
    which is called from `SpawnSessionFromItem` at line 269, using the `item` loaded at line 166
    (`item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)`).
  - `TriggerTriage` — loads `item` once at line 166 of the same file (function starts `:626`,
    load at `:635`) and reuses it through the whole handler, including the prompt-building step
    at `:718`.
  - `ReviewGateRunner.Run` — receives `item *BacklogItemData` as a parameter; its two callers in
    `session/backlog_lifecycle.go` (`:383`, `:459`) each call `l.storage.GetBacklogItem(ctx,
    is.BacklogItemID)` immediately beforehand.
- **A new `PipelineMode string` field added to `BacklogItemData` requires zero new DB round
  trips.** `GetBacklogItem` already does a single ent read that would include the new column;
  every call site above is already inside that item's lifetime. This directly satisfies the "no
  new synchronous DB round-trip" NFR — the field just needs to be added to the existing ent →
  domain-DTO mapping (wherever `BacklogItemData` is constructed from `*ent.BacklogItem`, alongside
  `SkipReviewGate`/`SkipPlanning`/`AutoSpawnSession`), not fetched separately.
- **No denormalization/caching is needed.** The field is read at most once per triage trigger,
  once per session spawn, and once per review — all already-paid-for reads on the hot path
  identified in the requirements doc (`SpawnSessionFromItem` complexity 41, `TriggerTriage`
  complexity 35). Adding one more struct field read has negligible marginal cost compared to the
  existing complexity in those functions.
- **Update path** follows the existing optional-field pattern exactly
  (`session/repository.go:407-422`, `BacklogItemUpdate`): add `PipelineMode *string` (or
  `*PipelineMode` if the plan phase decides to keep the Go-level `PipelineMode` type) to
  `BacklogItemUpdate`, matching `SkipReviewGate *bool` / `SkipPlanning *bool` /
  `AutoSpawnSession *bool` already there at lines 413-415. This is a `*string`, not a bare `bool`,
  so the proto3-zero-value-clobber bug flagged in the requirements' Constraints section (bucket-[2]
  of the prior audit) is naturally avoided as long as the proto field is wrapped the same way
  `auto_spawn_session` was (check whether that field uses a proto3 `optional bool` or a wrapper —
  worth confirming in the planning phase against `proto/session/v1/backlog.proto:117` before
  copying the pattern verbatim, since a plain `string` field is empty-string-is-unset by proto3
  convention which is actually *safe* here, unlike a plain `bool` where false-is-unset is
  ambiguous with a real "false" value).
- **ent schema**: `session/ent/schema/backlog_item.go:39` (`skip_review_gate`) and `:43`
  (`auto_spawn_session`) are the two fields to pattern-match for adding `pipeline_mode` as a new
  `field.String("pipeline_mode").Optional()` (or `.Default("")`) field. Regeneration must use
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` per
  `.claude/rules/ent-schema-generation.md`.

## 5. Event-Command-Policy Table

This reduces mostly to CRUD (set a field, read a field at 3 call sites) rather than a multi-step
saga, so a full EventStorming table is not warranted for the bulk of the feature. There is exactly
one place where a policy-style "whenever X, then Y" rule is worth surfacing explicitly, because it
crosses a state boundary the plan phase needs to design deliberately:

| Trigger (Event) | Policy (Whenever…Then) | Command | Resulting Event |
|---|---|---|---|
| User sets `pipeline_mode` on a `BacklogItemData` via the UI selector (`BacklogItemForm.tsx`) | Whenever an item's pipeline mode is set/changed, no immediate side effect fires — the mode is inert data until the item next enters triage or work | `UpdateBacklogItem(item_id, pipeline_mode)` | `BacklogItemUpdated{pipeline_mode}` |
| `TriggerTriage` invoked for an item with `pipeline_mode != default` | Whenever triage is triggered, then build the triage prompt via `PipelineEngine.TriagePromptFor` instead of the fixed template | `BuildHeadlessTriagePrompt` → `PipelineEngine.TriagePromptFor` | `TriageSessionSpawned{prompt reflects mode}` |
| `SpawnSessionFromItem` invoked for an item with `pipeline_mode != default` | Whenever a work session is spawned, then write the mode's slash-command set instead of the fixed one, and build `inst.Prompt` reflecting the mode | `WriteSlashCommands` → `PipelineEngine.SlashCommandSet`; `BuildSessionInitialPrompt` (needs mode-awareness — see open question below) | `WorkSessionSpawned{commands + prompt reflect mode}` |
| `ReviewGateRunner.Run` invoked for an item with `pipeline_mode != default` | Whenever the review gate runs, then build the review prompt via `PipelineEngine.ReviewPromptFor` instead of the fixed template | `BuildHeadlessReviewPrompt` → `PipelineEngine.ReviewPromptFor` | `ReviewCompleted{prompt reflects mode}` |

**Open question for the plan phase**: `BuildSessionInitialPrompt` (`session/backlog_context.go:71`,
called from `BuildTokenBudgetedPrompt`) is the function that ultimately produces `inst.Prompt` —
the string that becomes the autonomous driver's `goal` (§2). It is not in the three integration
points explicitly named in the requirements (`WriteSlashCommands`,
`TriggerTriage`/`BuildHeadlessTriagePrompt`, `review_gate.go`/`BuildHeadlessReviewPrompt`), but per
§2's trace it is arguably the *most* load-bearing lever for making autonomous-mode sessions
mode-aware. The plan phase should decide: either (a) add a fourth `PipelineEngine` method,
`InitialPromptFor(item, priorSessions) string`, so autonomous-mode sessions genuinely change
behavior too, or (b) explicitly document that `PipelineMode` only affects
interactive-session slash commands and headless triage/review prompts, and autonomous-mode
sessions are out of scope for v1 (in which case rabbit hole #2's "not cosmetic" proof would rest
solely on the slash-command-set lever, which is still sufficient but weaker). Given `AutoSpawnSession`
and `Autonomous` are clearly live, actively-used flags in this same code path, recommend (a).
