# Architecture Research: Completion Reviewer

Agent 3 (Architecture), SDD Phase 2. Scope: where the `done`-transition hook plugs in,
whether a tool-restricted session can be built today, the `MemoryStore` interface seam,
an Event-Command-Policy table, and consistency requirements.

## 1. Where the `done` transition actually lives

`session/backlog_lifecycle.go` (4955 lines) is not itself the state machine — it's a
`BacklogLifecycleListener` that reacts to session lifecycle events (`onSessionExited`,
`handleReviewSessionExited`) and runs periodic reconciliation sweeps
(`ReconcileStuck`, `BackfillStuckStates`). The actual status mutation + CAS precondition
check lives in `session/storage.go:736` (`Storage.TransitionBacklogItemStatus`), which
forwards to `session/ent_repository_backlog.go:918`
(`EntRepository.TransitionBacklogItemStatus`).

**`BacklogStatusDone` is reached from 8 independent call sites**, not one:

- `session/backlog_lifecycle.go:983` (via `onSessionExited`, when `item.SkipReviewGate`)
- `session/backlog_lifecycle.go:3256`, `:3280` (`reconcileBouncingItems`)
- `session/backlog_lifecycle.go:3649` (`pushAndCreatePR` fallback)
- `session/backlog_lifecycle.go:4462`, `:4907` (self-heal / backfill paths)
- `server/services/backlog_service_lifecycle.go:1073` (manual user action)
- `server/services/backlog_service_triage.go:2825`

Hooking each call site individually (the naive reading of the requirements doc's "wire
into `session/backlog_lifecycle.go` at the `done` transition") would require touching
two packages at 8 sites and would silently miss the next one someone adds.

### The real single choke point: `ItemChangePublisher`

`EntRepository.TransitionBacklogItemStatus` (and every other backlog mutation method)
calls `r.publishItemChanged(item, change)` **exactly once, after every successful DB
write, regardless of caller** (`session/ent_repository_backlog.go:1119`). This already
fires for every one of the 8 call sites above with `BacklogItemChange{Kind:
ChangeStatusTransition, OldStatus, NewStatus: "done"}`.

Key properties, already documented in the codebase and directly relevant to this
item's ACs:

- **Best-effort, non-blocking by contract**: the doc comment on `ItemChangePublisher`
  (`session/backlog_item_change.go:69`) states "Publish is always best-effort:
  implementations must never block or panic."
- **Panic-safe today**: `publishItemChanged` recovers from a panicking publisher
  (`ent_repository_backlog.go:1123`), and the production adapter
  (`server/services/backlog_item_event_publisher.go`) recovers again inside its own
  body — belt and suspenders, per the file's own comment referencing Task 1.3.2b.
- **Cross-package by design**: `session` cannot import `pkg/events` (would cycle), so
  `ItemChangePublisher` is implemented outside the package
  (`server/services.BacklogItemEventPublisher`) and wired in via
  `EntRepository.SetItemChangePublisher` / `Storage.SetItemChangePublisher`, called
  from `server/dependencies.go`. This is the established adapter pattern in this repo
  (mirrors `Notifier`).
- The production adapter forwards to `pkg/events.EventBus`, an **unreliable**,
  ring-buffered pub/sub built for SSE delivery to the frontend — it explicitly drops
  events for a slow subscriber rather than block (`pkg/events/bus.go:90`, comment:
  "Subscriber is slow; drop to prevent blocking others").

**Recommendation**: do not hook the 8 call sites. Instead, plug the completion reviewer
in as a *second* consumer of `BacklogItemChange` events, filtered to
`Kind == ChangeStatusTransition && NewStatus == string(BacklogStatusDone)`. Two viable
wiring points, in order of preference:

1. **New method on `ItemChangePublisher`'s existing chain**: wrap/decorate
   `BacklogItemEventPublisher` (or add a second `ItemChangePublisher` set via a new
   `SetSecondaryItemChangePublisher`, or simplest — make `PublishItemChanged` fan out to
   a slice of publishers) so the completion reviewer's trigger doesn't depend on the
   frontend event bus's drop-on-slow-subscriber behavior. This preserves "at-most-once
   is fine, but don't couple correctness to an unrelated system's backpressure policy."
2. **Subscribe to `pkg/events.EventBus` directly** (server/services layer, since that's
   where the bus lives) and filter for `NewBacklogItemChangedEvent` payloads with
   `Kind: ChangeStatusTransition, NewStatus: "done"`. Simpler (no new interface), but
   ties the reviewer's fire rate to the same buffer the UI's SSE stream depends on —
   acceptable given the ACs already say failures are logged, not surfaced, but worth
   flagging as a real (if rare) miss path if the buffer prunes the event before the
   reviewer's subscriber ever wakes to read it.

Either way, the reviewer itself must **not** run inline in the publish call — it must
`go` off a bounded worker (mirroring the existing `l.reviewSem <-
struct{}{}`/`defer func(){ <-l.reviewSem }()` pattern at
`backlog_lifecycle.go:1011-1021` for `spawnReviewGate`) so a slow or hung reviewer call
can never block the publisher, the DB transaction that just committed, or any other
subscriber.

## 2. Can a session be constructed with a restricted tool set today?

**Two session-spawning mechanisms exist in this codebase, with very different
enforcement guarantees:**

### 2a. `session.Instance` / `InstanceOptions` (full tmux-backed session)

`InstanceOptions` (`session/instance.go:452`) has `AllowedTools string` and
`PermissionMode string`, rendered into the Claude Code CLI invocation as
`--allowedTools` / `--permission-mode` (`session/instance_tmux.go:148-155`). This is
the mechanism the requirements doc's Hermes-derived proposal implicitly assumes.

**This mechanism does NOT provide real technical enforcement**, and this is already
proven and documented in this exact codebase — not a hypothesis:

> `session/headless/features.go:176-188` (`CodebaseReadAllowedTools` doc comment):
> "A scoped Bash allowlist ... was added here ... then reverted — see ADR-001's
> 2026-07-15 addendum. The empirical integration test
> `TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed` ... proved that under
> `--permission-mode bypassPermissions`, `--allowedTools`/`--disallowedTools` do NOT
> provide real technical enforcement for Bash: an explicitly unlisted command executed
> freely and wrote a real file to disk, and command-chaining after an allowed prefix
> also succeeded in full."

`session/backlog_review.go:397-425` (`BuildReviewCallOptions`) carries the same
finding forward as a live warning against re-introducing a Bash grant. **This directly
falsifies the requirements doc's ACs as literally written** ("enforced at the
session-builder level (allowed-tool list / capability gate in code)") if "allowed-tool
list" means `--allowedTools`/`--disallowedTools` under `bypassPermissions` — that
specific mechanism is empirically not a security boundary in this codebase, for Bash at
least. It has not been proven safe or unsafe for MCP tool calls specifically (the ADR's
test targeted Bash), so it should not be trusted for "no delegation, no approval
requests" either without an equivalent integration test.

**The one proven-safe grant** is `Read,Grep,Glob` only, with no Bash and no MCP
wiring at all (`CodebaseReadAllowedTools = "Read,Grep,Glob"`, verified via
`TestPool_RealClaude_WorkDirOnly_GrantsReadAccess` and
`TestPool_RealClaude_WorkDirWithToolFlags_GrantsReadAccess`).

### 2b. `session/headless.Pool` (one-shot, non-interactive `claude -p` call)

This is the mechanism actually used for the review gate today
(`BuildReviewCallOptions` → `headless.Pool.CallBlocking`/`CallWithOptions`), and it is
structurally different from `session.Instance` in exactly the way this item needs:

- It is a bare `claude -p` subprocess (`session/headless/runner.go`'s `ProcessRunner`),
  not a tmux pane, not a full `Instance`.
- Critically, **it never passes `--mcp-config`** — there's no `MCPServerURL` plumbing
  anywhere in `session/headless/`. The stapler-squad MCP server (which exposes
  `create_session`, `run_command`, `request_review`, etc. — see `server/mcp/tools_*.go`)
  is simply never wired into these calls.

**This is the actual enforcement mechanism this item needs, and it already exists as a
precedent**: the way to guarantee "no terminal, no delegation, no approval requests" in
code is not to compute an allowlist and hope Claude Code respects it — it's to **never
give the spawned process an MCP connection or a Bash grant in the first place**. A
process with no MCP server URL cannot call `create_session` no matter what the model
tries, because the tool doesn't exist in that process's tool universe. That's an
enforcement boundary in the harness's process-construction code, exactly matching the
AC's own language ("enforced at the session-builder level ... in code").

### Recommended shape for the completion reviewer's session

Given 2a's proven non-enforcement and 2b's proven precedent, the completion reviewer
should **not** be built as a new `session.Instance`/tmux session at all (resolving the
requirements doc's own "Open Question" about tmux vs. lighter-weight call). It should
be a `headless.Pool` one-shot call, mirroring `BuildReviewCallOptions`:

- `CallOptions{WorkDir: "" (no filesystem grant needed), AllowedTools: "" (no tools at
  all — see below), PermissionMode: "" or default}` — no Bash, no MCP.
- The model is instructed (system prompt, same pattern as
  `headlessReviewSystemPrompt`) to **output structured JSON** describing the learning(s)
  to record — not to call a tool at all. The Go caller parses that JSON and is the
  *only* code path that actually calls `MemoryStore.Append(...)`.
- This sidesteps the entire "is `--allowedTools` a real boundary" question for the
  write path: the model never has write capability, structurally. It only ever
  produces text; the reviewer's Go code decides whether/what to persist, which also
  directly satisfies the AC "never deletes an existing entry" — there is no delete
  tool exposed to delete with, and the Go-side `Append` call is the only write this
  code path is capable of issuing.
- If a future need arises for the model to read prior memory entries before writing
  (e.g. to avoid duplicate learnings), grant `Read`-equivalent access the same way
  `CodebaseReadAllowedTools` does — by injecting the relevant memory content into the
  prompt as text, not by exposing a live query tool. This keeps the enforcement
  boundary at "no MCP config, no Bash" rather than relying on an allowlist string.

This also directly satisfies the AC "reviewer never blocks the main workflow": a
`headless.Pool` call is a bounded subprocess invocation with its own timeout
(`CallOptions` supports a timeout, e.g. `CodebaseReadCallTimeout`), launched from a `go`
goroutine off the event hook (see §1), not a long-lived tmux session competing for the
WIP/session-count budgets the rest of the backlog pipeline manages.

## 3. The `MemoryStore` interface seam

Per `.claude/rules/interface-pollution-checklist.md`, the interface must be defined in
the **consumer** package — the completion reviewer's own package — scoped to exactly
the methods it needs, not shaped around whatever #116 eventually builds.

Given this item's own ACs (append/update only, never delete; tag with source item ID),
the consumer-side interface is narrow:

```go
// session/completion_reviewer.go (or a new session/completionreview subpackage if the
// file otherwise gets too large — session/backlog_lifecycle.go's 4955 lines is a
// cautionary example of what NOT to let this grow into)

// MemoryEntry is the shape this package needs to hand to the operator memory store.
// Deliberately minimal — only the fields this reviewer produces. #116's real store may
// have many more fields (pinning, staleness, dedup keys); this type only names what
// THIS caller populates.
type MemoryEntry struct {
	// SourceItemID ties this entry back to the backlog item that generated it —
	// required by this item's traceability AC.
	SourceItemID string
	// Content is the learning text itself, as produced by the restricted review call.
	Content string
	// Tags is an optional set of free-form labels (e.g. category, component) the
	// review call may emit to aid future retrieval.
	Tags []string
}

// MemoryStore is the write-only seam this package needs from the (not yet built)
// operator memory store (#116). Scoped to exactly one method — append-or-update,
// never delete — because that's this reviewer's entire write surface per its ACs.
// Defined here (the consumer), not in #116's package, per
// .claude/rules/interface-pollution-checklist.md: #116's real implementation just
// happens to satisfy this interface, with no "implements" declaration needed.
type MemoryStore interface {
	// Append records entry, tagged with SourceItemID. Implementations MUST NOT
	// delete or overwrite unrelated entries; whether a pinned entry with the same
	// SourceItemID is updated in place vs. appended as a new entry is entirely
	// #116's own policy — this reviewer only ever calls Append and has no way to
	// request or perform a delete.
	Append(ctx context.Context, entry MemoryEntry) error
}
```

Wiring pattern: mirror `SetSessionArchiver`/`SetReviewRespawner` — a package-level
setter (`SetMemoryStore(m MemoryStore)`) on whatever struct owns the reviewer (likely a
new `CompletionReviewer` struct, not bolted onto the already-4955-line
`BacklogLifecycleListener`), called from `server/dependencies.go` once #116's concrete
store exists. Until then, the reviewer can be fully implemented and unit-tested against
a fake `MemoryStore` — this is the point of defining the seam now.

**Nil-store behavior**: until #116 lands and `SetMemoryStore` is called,
`MemoryStore` will be nil. The reviewer must no-op (log + return) if `l.memoryStore ==
nil`, exactly like `l.getSessionCreator() != nil` gates `spawnReviewGate` today
(`backlog_lifecycle.go:1010`) — this lets the completion-reviewer code ship and be
tested well before #116 merges, satisfying the "clean interface seam ... don't have to
be co-developed" requirement.

## 4. Event-Command-Policy table (EventStorming grammar)

| # | Actor / System | Command | Event | Policy (reaction) |
|---|---|---|---|---|
| 1 | Any of the 8 call sites (session pkg or server/services) | `TransitionBacklogItemStatus(id, Done, ...)` | **`ItemStatusTransitionedToDone`** (`BacklogItemChange{Kind: ChangeStatusTransition, NewStatus: "done"}`, published once per successful CAS write, from `EntRepository.publishItemChanged`) | **P1 — Gate check**: if item has no `Description` OR zero `AcceptanceCriteria` entries, no-op (AC: "only fires when the item has non-trivial content"). Otherwise → Command 2. |
| 2 | CompletionReviewer (new subscriber on the publish chain / event bus) | `SpawnRestrictedReviewCall(itemID)` (fired via `go` from the event handler, bounded by a semaphore mirroring `l.reviewSem`) | **`RestrictedReviewSessionStarted`** | **P2 — Assemble context**: gather item title/description/AC snapshot with final statuses, triage notes, final diff summary (if a work session ran), review verdict + review-session notes (`ItemSessionSummary`, `ProgressNoteData`, `applyVerdictsToACs`'s already-merged AC data). Feed into the `headless.Pool` call's user prompt. |
| 3 | `headless.Pool` (one-shot `claude -p`, no MCP config, no Bash — see §2b) | `RunHeadlessReviewCall(context)` | **`LearningsProduced`** (structured JSON text output — not a tool call) or **`ReviewCallFailed`** (timeout, non-zero exit, malformed JSON) | **P3a (success)**: parse JSON → build `MemoryEntry{SourceItemID: itemID, Content, Tags}` → call `MemoryStore.Append`. **P3b (failure)**: log at error level; do not retry, do not surface to the backlog item or user (AC: "failures are logged, never surfaced"). |
| 4 | CompletionReviewer's own Go code (never the model) | `MemoryStore.Append(entry)` | **`MemoryEntryAppended`** or **`MemoryAppendFailed`** | **P4**: on success, nothing further (terminal). On failure, log and stop — same best-effort/no-retry policy as P3b. The reviewer never deletes; if `MemoryStore`'s real implementation supports pinning, pin-bypass-of-auto-transition is entirely internal to #116's store and invisible to this reviewer (it only calls `Append`). |

Actors/systems named above: **Backlog lifecycle state machine** (`session.Storage` /
`EntRepository`, publisher of the triggering event), **CompletionReviewer** (new,
subscribes to the event, owns the semaphore-gated goroutine and context assembly),
**Restricted session** (`headless.Pool` one-shot call, no MCP/Bash), **Memory store**
(`#116`, consumed only through the `MemoryStore` seam defined in §3).

## 5. Data flow / consistency requirements

**At-most-once (best-effort) firing is correct here, not a compromise** — the
requirements doc's own "failures are logged, not surfaced" is explicit evidence the
product intent is best-effort, and the existing `ItemChangePublisher` contract this
item should plug into is *already* documented as best-effort
(`session/backlog_item_change.go:69`: "must never block or panic"). Two failure modes
to distinguish, since they call for different (non-)handling:

1. **The trigger itself is missed** (event bus drops it under load, or the process
   crashes between the `Done` transition committing and the reviewer's goroutine
   running). **Acceptable per the ACs.** No durable outbox/queue is warranted — this
   would be over-engineering relative to "institutional memory that helps most of the
   time," and every other consumer of this same publish path (the SSE event bus itself)
   already accepts this risk for UI updates. If a stronger guarantee is wanted later,
   the fix is a small reconciliation sweep (mirroring `BackfillStuckStates`/
   `ReconcileStuck`'s existing pattern: periodically scan for `Done` items with no
   matching memory entry) — but that's explicitly listed out of scope for this item
   (no periodic curator pass) and should not be added speculatively.
2. **The review call itself fails** (timeout, malformed JSON, `MemoryStore.Append`
   error). **Also explicitly acceptable** per the AC: log, don't retry, don't surface.
   No compensating transaction is needed because nothing else in the system depends on
   this write succeeding — it's additive, informational, and consumed (by future triage
   sessions) as best-effort context, not a correctness-critical value.

**At-least-once would be actively wrong here**: because the reviewer's only write is
`Append` (never idempotent-by-construction — two runs for the same item would produce
two entries), a retry-until-success policy would risk duplicate/near-duplicate learning
entries for the same item without a dedup key, which is worse for the memory store's
signal-to-noise than an occasional missed entry. **Recommendation: confirm the
challenge is unnecessary — at-most-once is the right choice, not merely a tolerated
one, given `Append`'s non-idempotency and the informational (non-critical) nature of
the data.** If #116's real `MemoryStore.Append` wants at-least-once safety later, that
belongs inside its own implementation (e.g. a content hash or `(SourceItemID, content
hash)` uniqueness constraint) — not something this item's caller should compensate for
by adding retry logic on top of a fire-and-forget trigger.

**Consistency with the source item**: `SourceItemID` tagging (AC requirement) should
be captured from the same `BacklogItemData` snapshot the triggering event carried, not
re-fetched later — the item could theoretically be archived or further mutated between
the event firing and the reviewer's goroutine running; using the snapshot avoids a
TOCTOU read of a status that may have already moved on, and matches how `onSessionExited`
already threads `item` through as a value rather than re-querying at each step.

## Summary of key findings for the plan phase

- Hook point: **`ItemChangePublisher`'s existing best-effort fan-out**
  (`session/ent_repository_backlog.go:1119`), not the 8 individual `Done`-transition
  call sites — filtered to `Kind == ChangeStatusTransition && NewStatus == "done"`.
- Tool restriction: **`--allowedTools`/`--disallowedTools` under `bypassPermissions` is
  proven NOT to be a real enforcement boundary in this codebase** (ADR-001 2026-07-15
  addendum, `TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed`). Real
  enforcement = **no MCP config, no Bash grant, model outputs JSON text, Go code does
  the actual write** — mirroring `session/headless.Pool`'s existing review-gate
  pattern, not `session.Instance`'s tmux/CLI-flag pattern.
- Session shape: **`headless.Pool` one-shot call**, not a tmux `session.Instance` —
  resolves the requirements doc's open question directly, with precedent already in
  production (`BuildReviewCallOptions`).
- `MemoryStore` interface: single-method, consumer-defined, `Append`-only, nil-safe
  until #116 lands (see §3 code).
- Consistency: at-most-once is correct, not just tolerated — `Append`'s
  non-idempotency makes at-least-once actively harmful (duplicate entries) without a
  dedup key #116 hasn't been designed yet.
