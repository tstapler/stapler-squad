# Research: Feature Landscape — pr-fix-steering

## 1. The three sibling "active session blocks a respawn" call sites

All three call sites in `server/services/backlog_service_triage.go` share an identical
three-line shape:

```go
s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)
if active := findActiveWorkSession(sessions); active != nil {
    s.notifyRespawnBlockedByActiveSession(ctx, "<Caller>", itemID, item.Title, session.BacklogStatus(item.Status), active.SessionUUID)
    return nil
}
s.resolveRespawnBlockedActiveLogged(ctx, "<Caller>", itemID)
```

- **`AutoReopenForPRFix`** (`server/services/backlog_service_triage.go:2018`, guard at
  `:2048`) — the target of this project.
- **`AutoRespawnAutonomousWork`** (`:1860`, guard at `:1885`) — identical shape, one
  `findActiveWorkSession` check.
- **`AutoRespawnReview`** (`:2159`, guard at `:2185-2188`) — same shape but checks
  `findActiveWorkSession` OR `findActiveReviewSession` before treating the item as blocked.

All three pass through the same `notifyRespawnBlockedByActiveSession` /
`resolveRespawnBlockedActiveLogged` helper pair (`:1363`, `:1401`), which is itself
explicit in its doc comment that it exists because "these three call sites previously had
zero signal even for a healthy, still-progressing block."

**Design implication (do not implement, per Out of Scope):** because all three sites are
textually identical modulo the caller-name string and which `findActive*Session` call
feeds `active`, the natural extension point is a shared private helper — e.g.
`steerOrNotifyBlockedActiveSession(ctx, caller, itemID, item, active, fixContext)` — that
`AutoReopenForPRFix` calls where it currently calls
`notifyRespawnBlockedByActiveSession` directly. If this project instead special-cases
steering logic inline inside `AutoReopenForPRFix` only, a future extension to the other two
will have to either duplicate the steer-and-cooldown logic a second/third time or do an
invasive refactor. The one real blocker to sharing today: `AutoRespawnReview` and
`AutoRespawnAutonomousWork` have no `fixContext` string to steer with (no `PRStatus` in
scope) — so the shared helper's content-source parameter should be designed as "caller
supplies the steer text or a content-builder callback," not hardwired to `fixContext`,
even though only `AutoReopenForPRFix` populates it in this project.

## 2. `notifyRespawnBlockedByActiveSession` audit-trail precedent

`backlog_service_triage.go:1363`. Key semantics to extend, not duplicate:

- Uses `domain.StuckReasonRespawnBlockedActive` via `storage.MarkStuck(...)` +
  `storage.MarkStuckNotified(...)`. `MarkStuck` returns `applied bool` — true only the
  *first* time a reason is opened for an item (a no-op refresh on subsequent calls with an
  already-open row of the same reason) — this is the "mark once" half of the audit trail.
  `MarkStuckNotified` is only called when `applied` is true, so the notification itself
  fires once per open window, not every tick.
- **However**, the function *unconditionally* publishes an `eventBus` notification every
  call, regardless of `applied` — the doc comment explicitly flags this as "a known,
  pre-existing inconsistency this helper inherits from notifyReworkCapHit/
  notifySpawnAndRollbackFailed... not something to fix here in isolation." So "mark once"
  applies to the durable `BacklogStuckState` row + its `NotifiedAt`, not to the transient
  eventBus notification stream.
- `resolveRespawnBlockedActiveLogged` (`:1401`) clears the open row once the guard's
  condition clears (a session-blocked-respawn window ends) — called at the top of each of
  the three functions once `findActiveWorkSession` returns nil.

**Implication for the new steer path:** the new "record a steer attempt" comment/
notification described in the requirements should reuse this same
MarkStuck/MarkStuckNotified-once-per-open-window shape, but keyed on a **new** stuck
reason or reused `StuckReasonRespawnBlockedActive` extended with a distinguishing context
string (the reason-signature) — not a boolean flag — since the requirement is "re-steer
on reason change, not on every tick," which is exactly the same "mark once per distinct
condition, not per tick" pattern this helper already implements for the *notify* side.
The steer call itself needs its own separate dedup gate (the reason-signature comparison),
since `MarkStuck`'s own idempotency is keyed on `(itemID, reason)`, not on message content.

## 3. `fixContext` — content source and format

Built in `session/backlog_lifecycle_pr.go`, two call sites:

- Closed-without-merge case (`:1395`): plain one-line string, no markdown.
- CI/review/conflict case (`:1516`): `fmt.Sprintf("PR #%d (%s) needs fixes:\n\n%s", item.PrNumber, item.PrURL, prStatus.FeedbackText)`.

`prStatus.FeedbackText` is rendered by `PRStatus.render()` in
`session/git/worktree_git.go:562-618`. Confirmed structure and content:

- **Markdown headers** (`## Merge conflict`, `## Failing CI checks`, `## Review: changes
  requested by @author`, `## Reviewer comments`, `## PR comments`) and markdown lists
  (`- checkname`) — this is *not* plain prose; it's markdown intended for a chat-style
  agent context window, not a terminal prompt.
- The conflict section alone is a large fixed block of prose (rebase instructions,
  `--force-with-lease` warning, `.gitignore` truncation warning, `git diff --stat`
  instruction) — several hundred bytes even with zero dynamic content.
- Failing-CI section is one bullet per failed check name — unbounded by check count.
- Review/comment sections **include reviewer body text verbatim**, unbounded — a long
  human or bot review comment (or several) is included as-is, no truncation.
- Sections concatenate with blank-line (`\n\n`) separators throughout — multi-paragraph,
  multi-blank-line text.

`AutoReopenForPRFix` (`backlog_service_triage.go:2100`) further wraps this into
`prFixNote := fmt.Sprintf("[PR Fix context - PR #%d (%s)]\n%s", ...)` for the *notes*
field used to seed a **new** spawned session's prompt — verbatim, no reformatting, because
that's a written prompt file/note, not a live PTY.

**For steering an *already-running* session, this exact string is not obviously
reusable verbatim as-is**: it's designed to be read as a written brief (markdown, long
prose, unbounded reviewer text), not typed as an interactive nudge. See §5 below for the
PTY-injection mechanics that make this a real design constraint, not just a style nit.

## 4. `session.MaxSteerMessageLength` — the length constraint is real

`session/instance.go:139`: `const MaxSteerMessageLength = 10000` (bytes). Enforced in
`UpdateSession` at `server/services/session_service.go:2930-2933` — an over-length
`steer_message` is rejected outright with `CodeInvalidArgument`, not truncated.

Given §3's findings, `fixContext`/`FeedbackText` **can realistically exceed 10000 bytes**:
the conflict-instructions block alone is several hundred bytes fixed overhead, a failing
CI list is one line per check (dozens of checks in a large matrix build is plausible), and
reviewer/comment bodies are included verbatim and unbounded (a single verbose review body
or several stacked comments can easily run to multiple KB). This is a genuine edge case,
not a hypothetical: the new steer path must define a truncation/summarization strategy
(e.g. a condensed steer message referencing the fuller `fixContext` already parked in the
item's notes/stuck-state context, rather than re-sending the *entire* rendered
`FeedbackText`) rather than assume `fixContext` always fits.

## 5. Program-awareness: `instance.Program` and PTY-injection mechanics

`Instance.Program` (`session/instance.go:161,689`) is a plain `string` — e.g. `"claude"`
or `"aider --model ollama_chat/gemma3:1b"` — no enum type exists. There is **no existing
helper** in the codebase (`grep` for `.Program ==`, `HasPrefix(...Program`, `IsClaudeCode`
found nothing outside test files) that classifies a session as "Claude Code" vs. other
agents — this project will need to introduce the first such check, presumably a
prefix/equality match against `"claude"` (mirroring how `Program` is set at session-creation
time elsewhere), guarded carefully since `Program` can carry trailing flags (the Aider
example above).

Separately — and this bears directly on whether `fixContext` can be reused verbatim per
§3 — the actual steering mechanics differ sharply by session type
(`session_service.go:2929-2972`):

- **Autonomous sessions**: `instance.GetController().SendCommandImmediate(msg + "\r")` —
  a structured command-queue call (ADR-001), not raw keystroke typing. Multi-line/markdown
  content is far less risky here since it isn't interpreted by a live terminal line
  discipline.
- **Non-autonomous (interactive) sessions**: `session.BuildSubmittableInput(msg, true)`
  (just appends an Enter sequence) sent via `instance.SendKeys(text)`, which resolves to
  `TmuxSession.SendKeys` (`session/tmux/tmux.go:1816-1822`) — **this writes the raw bytes
  of `text` directly to the PTY master file descriptor** (`file.Write([]byte(keys))`), not
  through `tmux send-keys` or any bracketed-paste wrapper. Any embedded `\n` characters in
  a multi-paragraph `fixContext` string are written as literal bytes to the pty; whether
  that reads as "newline within one buffered input" or "premature submit" depends on the
  receiving CLI's raw-mode line handling and is not something this codebase controls or
  has tested for multi-paragraph markdown content — SendKeys today is only ever called
  with short, single-line human-typed or MCP `steer_session` messages, not multi-KB
  markdown blocks. This is exactly the risk the requirements' "PTY injection line length,
  no markdown" concern points at, and it's a materially different risk than for the
  autonomous/command-queue path — confirming the plan should treat "condense fixContext
  into a short steer message" as a real design requirement, not just a nice-to-have.

Confirms the requirements' explicit constraint: `instance.Program` must be checked before
including a `/github:pr-ship` instruction, and this should probably be combined with the
truncation/condensation need from §4 into one "build a short, plain-English (+ optional
slash command) steer message" function, rather than passing `fixContext` through as-is.

## 6. `SendKeys`/`SendCommandImmediate` reliability against a busy session

No dedicated mid-turn-interruption test exists (`TestUpdateSession_SteerMessage_*` in
`server/services/session_service_test.go` was not read line-by-line here, but the
production code path itself gives no special handling for "session is mid-generation" —
it fires the write unconditionally). The requirements' Rabbit Holes section already flags
this as "likely already a solved problem" for human/browser-originated steering; this
research did not find evidence of any additional safety net (queueing, busy-detection) —
the steer is a best-effort, fire-and-forget-into-the-pty operation for interactive
sessions, and a queued command for autonomous ones. Treat "steer lands mid-tool-call" as
an accepted, pre-existing risk this project inherits rather than something newly
introduced.

## 7. Test conventions for the new branch

`server/services/backlog_service_triage_test.go:1045-1250` region establishes the pattern
new tests should follow:

- `TestAutoReopenForPRFix_ActiveWorkSession_SkipsWithoutStatusChurn` — asserts no status
  transition, no new session spawned (`creator.calls` empty) when the guard fires.
- `TestAutoReopenForPRFix_ActiveWorkSession_RecordsRespawnBlockedActive` — asserts a
  `StuckReasonRespawnBlockedActive` row via `storage.FindOpenStuckStates` and an
  `events.EventNotification` off an `events.NewEventBus` subscription.
- `TestAutoReopenForPRFix_NoActiveSession_ResolvesAnyOpenRespawnBlockedActiveRow` — asserts
  the resolve-on-guard-pass half.

Mocking pattern: `mockSessionStopper` (`server/services/backlog_service_test.go:173`,
implementing the `SessionStopper` interface at `backlog_service.go:47`) is constructed with
`liveUUIDs map[string]bool` and injected via `svc.SetSessionStopper(...)`. `mockSessionCreator`
is the spy for spawn calls (`creator.calls`). Note: `fakePRFixSpawner` (found in
`session/backlog_lifecycle_pr_trigger_test.go` and siblings) is a **different, package-local
fake** used one layer up, where `session.BacklogLifecycleListener` calls the
`PRFixSpawner` interface (implemented by `*BacklogService`) — it is not used by
`backlog_service_triage_test.go`, which tests `BacklogService` methods directly. New tests
for the steer branch belong in `backlog_service_triage_test.go` and should follow the
`mockSessionStopper` shape: a new `mockSessionSteerer` (or similarly named) fake
implementing the new `SessionSteerer` interface, constructed with an injectable
success/failure/recorded-message shape, wired via a new `svc.SetSessionSteerer(...)` — the
same DI shape `SessionStopper`/`SetSessionStopper` already establishes and that the
requirements explicitly call out as the precedent to follow
(`server/dependencies.go:1191` wires `backlogSvc.SetSessionStopper(sessionService)`; the
analogous `SetSessionSteerer(sessionService)` call is the wiring point for this project).

For the pure dedup/change-detection logic, `session/nudge_dedup.go`'s
`isDuplicateNudge`/`nextLastNudge` (lines 43-70ish, cooldown constant `nudgeCooldown = 3 *
time.Minute` at line 13) is the cited precedent pattern: small, pure functions taking
`(candidate, prevState, now, ...)` and returning a bool/next-state, unit-tested in
isolation without any service/storage wiring. The new reason-signature dedup should follow
this same shape (a pure function over a signature struct + timestamp) rather than being
embedded inline in `AutoReopenForPRFix`.

## Summary of unstated needs / edge cases surfaced

1. **Content must be condensed, not passed through raw.** `fixContext`/`FeedbackText` is
   markdown, unbounded-length, and designed for a written prompt — not a short interactive
   nudge. A truncation/summarization step is required, independent of and in addition to
   the `MaxSteerMessageLength` hard cap (§3, §4).
2. **`MaxSteerMessageLength` (10000 bytes) is a real, hittable ceiling**, not a
   theoretical one — verbose CI matrices or long reviewer comments make this plausible in
   production. The design must define what happens on overflow (truncate with a pointer
   back to the full context, or send only bullet-level facts) rather than letting the
   `UpdateSession` call reject silently.
3. **PTY-injection risk for interactive sessions is real, not cosmetic** — raw bytes hit
   the PTY master directly (no bracketed-paste, no `tmux send-keys -l`), so multi-line
   markdown content carries a genuine risk of being split into multiple submitted lines by
   the receiving CLI's line discipline. This differs from the autonomous path's
   command-queue mechanism, and the design should account for that asymmetry rather than
   treating both branches as equally safe for long content.
4. **No existing `instance.Program` classifier exists** — this project introduces the
   first "is this Claude Code" check; there's no enum, just a raw command string that can
   carry flags (e.g. Aider's `--model ...`), so the match needs to be a careful
   prefix/tokenized check, not bare equality.
5. **Sharing the guard logic across the three sibling call sites is feasible but not
   automatic** — `AutoRespawnReview`/`AutoRespawnAutonomousWork` have no `fixContext`
   equivalent in scope today, so a shared helper (if the plan chooses to introduce one for
   future-proofing, per Out of Scope's explicit non-goal of *implementing* the extension)
   needs a content-source abstraction, not a hardcoded string parameter.
6. **The "mark once, re-mark on change" pattern already has a proven shape** in
   `notifyRespawnBlockedByActiveSession`'s `MarkStuck`/`applied`-gated `MarkStuckNotified`
   — but that pattern is keyed on `(itemID, reason)`, not content, so the new
   reason-signature cooldown needs its own dedicated dedup key/state, most naturally as a
   pure function paralleling `nudge_dedup.go`, not a reuse of `MarkStuck`'s own idempotency.
