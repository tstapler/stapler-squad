# Feature Landscape Research — Completion Reviewer (Agent 2)

Scope: how a new "fires on backlog `done` transition, spawns a restricted session" feature
fits the existing lifecycle-hook, context-assembly, and tool-restriction patterns in this
codebase. Requirements: `project_plans/completion-reviewer/requirements.md`.

## 1. Existing "fires on backlog state transition" hooks — the established pattern

There are **two** hook layers already in production, and the completion reviewer should use
the outer one, not bolt onto the inner one.

### 1a. Inner layer: `session.ItemChangePublisher` (repository-level, synchronous, in-process)

`session/backlog_item_change.go` defines `BacklogItemChange` (`Kind`, `OldStatus`, `NewStatus`,
`Verdict`, etc.) and the `ItemChangePublisher` interface (`PublishItemChanged(item, change)`).
Every repository mutation method in `session/ent_repository_backlog.go` /
`session/storage_backlog.go` calls `r.publishItemChanged(...)` right after committing the DB
write — this is the single choke point for *all* backlog mutations, not just status
transitions (`ChangeStatusTransition`, `ChangeVerdictRecorded`, `ChangeSessionAttached`,
`ChangeItemUpdated`, `ChangeItemArchived`, `ChangeItemRemoved`, `ChangeTriageProgressUpdated`
— `session/backlog_item_change.go:13-30`). `publishItemChanged` (`session/ent_repository_backlog.go:1119`)
is nil-checked and panic-recovered — a publisher may not be wired (tests), and a panic inside
it must never break the repository call that triggered it.

### 1b. Outer layer: the event bus (`server/events`), fed by `BacklogItemEventPublisher`

`server/services/backlog_item_event_publisher.go`'s `BacklogItemEventPublisher.PublishItemChanged`
is the one production implementation of `ItemChangePublisher` — it's a thin adapter (wired via
`Storage.SetItemChangePublisher`) that converts each `session.BacklogItemChange` into an
`events.BacklogItemEventPayload` and calls `p.Bus.Publish(...)`. Consumers subscribe to the bus
(`server/services/backlog_service_events.go`'s `WatchBacklogItems` is one example subscriber,
used for the RPC stream to the frontend). The **in-flight `webhook-triggers` project**
(`project_plans/webhook-triggers/requirements.md`) is building outbound HTTP callbacks on
lifecycle events via this exact same bus — i.e. "a new side effect triggered by a backlog
transition" is a recognized, actively-being-solved shape in this codebase, and the event bus is
where it's being solved, not inside `backlog_lifecycle.go` itself.

**Why the outer layer, not the inner one, for this feature:** `TransitionBacklogItemStatus(...,
BacklogStatusDone, ...)` is called from **at least 6 separate sites** in
`session/backlog_lifecycle.go` (verified via grep: lines ~3256, 3280, 3649, 4462, 4907, plus the
`onSessionExited`/`SkipReviewGate` fast-path at line 983). Hooking each call site individually
(as the requirements doc's proposed solution literally describes — "wire the hook into
`backlog_lifecycle.go` at the `done` transition") means finding and updating all of them, and
missing one silently breaks AC #1. Subscribing to `ChangeStatusTransition` events where
`NewStatus == string(BacklogStatusDone)` (event-bus layer, or even the `ItemChangePublisher`
layer directly if bus dependency should be avoided) collapses this to **one** subscription
point that structurally cannot miss a call site, because every call site already funnels
through `publishItemChanged`. Recommend: add the completion-reviewer as either (a) a second
`ItemChangePublisher` fan-out (composite publisher) or, more consistent with the webhook-
triggers precedent, (b) an event-bus subscriber filtering on
`Kind == ChangeStatusTransition && NewStatus == "done"`.

### 1c. Non-blocking discipline already established

`BacklogLifecycleListener.OnLifecycleEvent` doc comment (`session/backlog_lifecycle.go:316`):
"non-blocking; all DB work is dispatched to a goroutine." The repo already has a standard shape
for this — bounded goroutine dispatch with a semaphore to prevent unbounded fan-out, e.g.
`l.reviewSem` guarding `spawnReviewGate` (`session/backlog_lifecycle.go:1009-1022`):
```go
go func() {
    select {
    case l.reviewSem <- struct{}{}:
    case <-l.shutdownCtx.Done():
        return
    }
    defer func() { <-l.reviewSem }()
    l.spawnReviewGate(item, is)
}()
```
The completion reviewer's "never blocks the main workflow" AC should copy this shape exactly:
a bounded semaphore (its own, sized independently of `reviewSem`), a `shutdownCtx` bailout, and
the fire being a `go func()` off the event-bus callback (which itself must stay non-blocking per
the bus's own contract).

## 2. What context is already assembled at/near the `done` transition

The context AC #1 asks for (title, description, AC snapshot with final statuses, triage notes,
diff summary, review verdict, review-session notes) is **not** assembled as one bundle anywhere
today, but every piece of it individually already has a plumbed accessor, and the closest
existing analog is `session/backlog_context.go`'s `BuildSessionInitialPrompt` /
`BuildTokenBudgetedPrompt`:

- **Title/Description/Priority/Status**: `session.BacklogItemData` (`session/repository.go:356`)
  — `Title`, `Description string`, `AcceptanceCriteria AcCriteriaJSON`, `Status string`.
- **AC snapshot with final statuses**: `session.AcCriterion` (`session/domain/backlog.go:248`) —
  `{Index, Text, Status AcStatus, Note}`, `Status` one of `pending/in_progress/done/fail`.
  `ParseAcCriteria(item.AcceptanceCriteria)` deserializes it. `BuildSessionInitialPrompt`
  (`session/backlog_context.go:138-141`) already renders this as a checklist via
  `buildAcChecklist(criteria)` — reusable almost verbatim for the reviewer's prompt.
- **Review verdict**: persisted via the `submit_review_verdict` MCP tool
  (`server/mcp/tools_backlog.go`, referenced in `session/backlog_lifecycle.go:1029-1031`) into
  the `review_verdicts` ent table, exposed as `ItemSessionSummary.ReviewVerdict
  *ReviewVerdictSummary` (`session/repository.go:267,318` — `OverallOutcome`, `Summary`,
  `PerCriterion` JSON of `CriterionVerdict`). `ListItemSessions` (unlike
  `GetItemSessionBySessionUUID`) eagerly loads this edge — `handleReviewSessionExited`
  (`session/backlog_lifecycle.go:1049-1058`) is the canonical example of reading it back.
- **Prior-attempt / review-session notes**: `BuildSessionInitialPrompt`'s "Prior Attempts"
  section (`session/backlog_context.go:149-193`) already walks `priorSessions` (ended-only),
  printing role, commit count, last commit message, verdict outcome, reviewer summary, and
  per-criterion failure evidence for the most recent attempts — this is effectively the
  "review-session notes" the AC wants, already token-budget-aware
  (`sanitizeField`/`truncateField` caps).
- **Triage notes**: item's `Notes` field (`session/backlog_context.go:143-147`,
  `sb.WriteString(sanitizeField(item.Notes, 1000))`).
- **Diff size/summary**: **not** part of `BacklogItemData`/`ItemSessionSummary` — it's fetched
  on demand via the `get_session_diff` MCP tool / `GetSessionDiff` (session git ops), keyed off
  a session's worktree. The completion reviewer will need to fetch this itself (via the work
  session's worktree path, if still present) rather than finding it pre-assembled; by the time
  an item is `done` the worktree may already be cleaned up — this is a real gap, not just
  missing plumbing (see Edge Cases below).

**Bottom line**: no single "done-transition context bundle" function exists yet, but
`BuildSessionInitialPrompt`'s structure (title/description/AC-checklist/notes/prior-attempts-
with-verdicts, all through the same `sanitizeField`/`truncateField` token-budget guards) is the
right template to adapt for a `BuildCompletionReviewContext(item, sessions)`-shaped function
rather than inventing a new assembly format — reuse `buildAcChecklist`, `sanitizeField`,
`parsePerCriterionVerdicts` directly.

## 3. Critical prior finding: tool restriction via `AllowedTools`/`PermissionMode` is proven UNRELIABLE for Bash

This directly threatens AC #2 ("enforced at the session-builder level ... not by prompt
instruction alone") and must shape the design.

`session/headless/integration_test.go`'s `TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed`
(lines 138-166) is a **permanent empirical regression test** documenting a real finding
(recorded in `ADR-001`'s 2026-07-15 addendum, referenced from `session/backlog_review.go:405-424`):
under `PermissionMode: "bypassPermissions"`, `AllowedTools`/`DisallowedTools` string
allow/deny-lists provide **no real technical enforcement for the `Bash` tool** — an unlisted
Bash command ran freely, and command-chaining after an allowed prefix also fully succeeded. As a
direct result, production reverted a Bash grant that briefly existed in
`headless.CodebaseReadAllowedTools` (`session/backlog_review.go:406-414`); the current
`BuildReviewCallOptions` codebase-read path only grants `Read,Grep,Glob` — no `Bash` — precisely
because of this finding. `TestPool_RealClaude_WorkDirWithToolFlags_GrantsReadAccess`
(`session/headless/integration_test.go:96-123`) confirms `AllowedTools`/`PermissionMode` **do**
correctly scope non-Bash builtin tools (Read/Grep/Glob) — the failure mode is specific to Bash's
shell-escape surface, not a blanket "the flag does nothing" finding.

**Implication for this design**: the completion reviewer's restricted session must not lean on
`AllowedTools` alone as its enforcement boundary, and especially must never grant Bash even
transitively. There is no existing MCP-scoping mechanism in `session/headless.CallOptions`
today (no `MCPConfig`/allowed-server field exists — grepped, confirmed absent) — genuine
enforcement for a "memory-write only" tool would need either (a) a new, minimal `--mcp-config`
that exposes *only* the memory-write MCP tool and no other server (enforcement via absence of
capability, not an allow-list string), or (b) the same
`TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed`-style empirical test written against
the actual completion-reviewer call shape before trusting it, per this repo's own precedent of
never trusting an unverified CLI security boundary. AC #8 ("tool-restriction enforcement — the
restricted session literally cannot invoke a non-memory tool") should be read as requiring this
kind of empirical proof test, not just a code review of the flag being set.

## 4. Session-spawn pattern: real tmux `Instance` vs headless `Pool` call

Two spawn mechanisms exist and the requirements doc's Open Questions correctly flags the
ambiguity:

- **Real tmux-backed session** (`SessionRoleReview` pattern): `spawnReviewGate`
  (`session/backlog_lifecycle.go:1303`) creates a real hidden `session.Instance` via
  `l.getSessionCreator()`, tracked with `Role: SessionRoleReview`
  (`session/backlog_lifecycle.go:1970`), whose exit is handled by
  `handleReviewSessionExited`. Heavier (tmux pane, worktree, full agent harness,
  `AllowedTools`/`PermissionMode` set via `InstanceOptions`, `session/instance.go:242-249`).
- **Headless one-shot call** (`session/headless.Pool.CallBlocking`): used for the
  empty-diff/codebase-read review path (`BuildReviewCallOptions`,
  `session/backlog_review.go:397-425`) and general "ask the LLM a bounded question, get text
  back" needs. No tmux pane, no persistent session-list entry, `CallOptions{WorkDir,
  AllowedTools, PermissionMode, DisallowedTools}` only.

Given AC #2/#3 (no terminal, no delegation, no approval requests, must never block, must be
short-lived) and that the reviewer's job is "read assembled context, write one memory entry" —
not explore a worktree — the **headless `Pool.CallBlocking` shape is the closer fit**, not a
full `SessionRoleReview`-style tmux instance. It avoids needing a new `SessionRole` constant,
avoids the tmux/worktree lifecycle entirely (nothing to clean up, nothing that can leave an
orphaned pane per `.claude/rules/service-restart-orphan-process.md`), and its `CallOptions`
already has the shape (`AllowedTools`, no default `WorkDir`) this hook needs — modulo the
"needs a memory-only MCP tool exposed" gap in section 3. This is a design recommendation for
Agent 3/Phase 3, not yet a decision.

## 5. Edge cases the design must handle

- **No description / no AC recorded**: AC #5 already requires this as an explicit no-op gate
  ("has a description AND at least one acceptance criterion recorded"). `BacklogItemData.Description
  == ""` or `ParseAcCriteria(item.AcceptanceCriteria)` returning `nil`/empty must short-circuit
  before spawning anything — cheap, no LLM call, no memory write.
- **`done` reached via the `SkipReviewGate` fast path**: `session/backlog_lifecycle.go:981-984`
  — `item.SkipReviewGate` sends `in_progress -> done` directly, bypassing review entirely. The
  context bundle in this path has **no review verdict at all** (`ReviewVerdict == nil`) and no
  `SessionRoleReview` entry to read prior-attempt evidence from. The reviewer must degrade
  gracefully (verdict-less context) rather than assume a verdict always exists — this is a
  *normal*, not exceptional, path for items with `SkipReviewGate: true`.
- **`done` reached via `closeIfSupersededByMain`** (`session/backlog_lifecycle.go:4907` and
  similar self-heal paths): item transitions `pr_pending -> done` via a system decision (already-
  shipped-elsewhere detection), again with no fresh review verdict tied to *this* transition —
  same degrade-gracefully requirement.
- **Rapid re-transition (`done -> reopened -> done` again)**: `autoReopenWithBackoffGate`
  (used throughout `backlog_lifecycle.go` for FAIL/PARTIAL/UNVERIFIABLE verdicts) is the existing
  reopen path; nothing currently prevents an item from cycling through `done` more than once
  (e.g. a stuck-detector false-reopen, or a manual reopen from the UI). Without dedup, each
  `done` re-entry would re-fire the reviewer and could write a near-duplicate memory entry tagged
  with the same item ID. The codebase's existing idempotency primitive for "don't re-act on a
  condition already recorded" is `Storage.RemediationBlocked(ctx, itemID, stuckReason)` (used at
  `session/backlog_lifecycle.go:1099` and elsewhere) — a similar per-item, per-reason gate (or a
  simple "have we already written a completion-review memory entry for this item's *current*
  `done` occurrence" check keyed off item ID + updated_at/occurrence count) should gate re-firing.
- **Memory store unavailable or slow**: hard-blocked dependency (operator memory, #116, not yet
  built) — the interface seam should assume the write call can fail or hang, and per AC #3 must
  time out and log rather than block or retry-forever. Mirrors the `shutdownCtx`/bounded-
  semaphore shape in section 1c.
- **Spawned session itself fails or hangs**: `handleReviewSessionExited`'s handling of a review
  session that "exited without ever calling `submit_review_verdict`" (crashed, killed, ran out of
  turns — `session/backlog_lifecycle.go:1079-1113`) is the direct analog: treat a
  no-memory-write outcome as a no-op failure, log it, notify only if it's a *repeated* failure
  pattern (see `IsRepeatedFailure`/`ReviewVerdictSummary` in `session/stuck_decisions.go:76-110`
  for the existing "distinguish one-off from systemic" heuristic) — never surface it as blocking
  or retry the `done` transition itself.
- **Diff summary unavailable**: by the time an item is `done`, especially well after the fact
  (a re-fired reviewer, or a `done` reached hours after the work session's worktree was cleaned
  up), `GetSessionDiff`-equivalent data may no longer be retrievable. The context assembler must
  treat diff summary as best-effort/optional, not required.

## 6. Unstated needs beyond the explicit ACs

- **Auditability without a UI** (out of scope per requirements, but still needed *somewhere*):
  every other background system action in this codebase that silently mutates state leaves a
  trace via one of two established channels — `l.notify(...)` (creates a `Notification` row,
  seen at every `autoReopenWithBackoffGate`/`closeIfSupersededByMain` call site) or an appended
  `Note` on a `BacklogItemPrecondition` at transition time (e.g.
  `session/backlog_lifecycle.go:4901-4903`'s self-heal note). `.claude/rules/document-ai-decisions-in-edge-cases`
  equivalent memory item (`feedback_document_ai_decisions_in_edge_cases.md`) states this
  explicitly project-wide: "self-heal/auto-close actions should post a visible comment +
  notify(), not act silently." A completion reviewer that writes to opaque memory with *zero*
  visible trace on the item itself (not even a low-priority `notify()` or an item-log line) would
  violate this established norm even though the ACs don't require a UI — recommend at minimum a
  low-priority `notify()` or structured log line per firing, distinct from the memory entry
  itself, so a human debugging "why did the fleet suddenly know X" has a starting point.
- **Idempotency/tagging discipline for future consumers**: AC #4 requires tagging memory entries
  with the source item ID "for traceability" — given section 5's re-transition edge case, the tag
  should probably also carry enough to distinguish *which* `done` occurrence produced it (e.g. a
  timestamp or occurrence counter), not just the bare item ID, so a future dedup/curator pass
  (explicitly out of scope here, but named in the requirements doc as a known future job) isn't
  forced to guess.
- **Respecting pinned memory** (AC #7) needs the interface seam to expose *some* pin-check
  primitive before any write — since the memory store doesn't exist yet, this should be treated
  as a hard interface requirement to hand to whoever builds #116 (a `IsPinned(entry) bool` or
  equivalent), not something the reviewer can special-case internally.

## Key files referenced

- `project_plans/completion-reviewer/requirements.md`
- `session/backlog_lifecycle.go` (transition call sites, `spawnReviewGate`, `handleReviewSessionExited`, `onSessionExited`)
- `session/backlog_item_change.go` (`BacklogItemChange`, `ItemChangePublisher`)
- `session/ent_repository_backlog.go:1109-1133` (`publishItemChanged`)
- `server/services/backlog_item_event_publisher.go` (event-bus adapter)
- `server/services/backlog_service_events.go` (existing bus subscriber pattern)
- `session/backlog_context.go` (`BuildSessionInitialPrompt`, `BuildTokenBudgetedPrompt`, `sanitizeField`, `buildAcChecklist`)
- `session/repository.go:356` (`BacklogItemData`), `session/domain/backlog.go:248` (`AcCriterion`)
- `session/headless/caller.go` (`CallOptions`), `session/headless/integration_test.go` (Bash-enforcement finding), `session/backlog_review.go:397-425` (`BuildReviewCallOptions`, ADR-001 reference)
- `session/stuck_decisions.go` (`IsRepeatedFailure`, `ReviewVerdictSummary`)
- `project_plans/webhook-triggers/requirements.md` (sibling in-flight feature, same event-bus pattern)
