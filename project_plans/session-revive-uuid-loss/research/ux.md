# UX Research: session-revive-uuid-loss (Phase 2)

## Scope check

AC3 explicitly requires "a durable, user-visible signal ... a session event/status
field the frontend can surface — distinguishing 'resumed' from 'lost & restarted
fresh.'" This is confirmed **not** purely backend infra: there is a real UI surface
to design. This doc covers that surface only (goals 1/2/4/5 are backend logic,
covered by other research docs).

## 1. Existing comparable patterns in this codebase

Three precedents exist, at three different "durability" levels. The new signal
should be built from the same primitives, not a new one.

### a. Operator notification pipeline (`Notifier.Notify`) — the closest match

`session/backlog_lifecycle.go:731` (`BacklogLifecycleListener.notify`) is the
exact pattern named in the memory rule ("self-heal/auto-close actions should post
a visible comment + notify(), not act silently"). It's a thin wrapper:

```go
// notify publishes a best-effort operator notification. No-op if no notifier is wired.
func (l *BacklogLifecycleListener) notify(itemID, title, message string, notificationType, priority int32) {
	if n := l.getNotifier(); n != nil {
		n.Notify(itemID, title, message, notificationType, priority)
	}
}
```

`notifyTransitionFailed` (`session/backlog_lifecycle.go:743`) is the closest
sibling case — "an automated action left state that doesn't match what the user
would expect, tell them" — and uses:
- `notificationType = 7` (`NOTIFICATION_TYPE_ERROR`)
- `priority = 3` (`NOTIFICATION_PRIORITY_HIGH`)

For lost-context-fresh-restart, `NOTIFICATION_TYPE_WARNING` (8) /
`NOTIFICATION_PRIORITY_MEDIUM` (2) fits better than ERROR/HIGH — the session
still works, it just lost memory; nothing is broken or blocking. Full enum:
`web-app/src/gen/session/v1/types_pb.ts:3994-4149` (`NotificationType`,
`NotificationPriority`).

This pipeline renders as a **toast** (`web-app/src/components/ui/NotificationToast.tsx`)
— transient, auto-closes/auto-minimizes per `lib/notification-policy.ts`. Good
for "tell the user right now" but **not sufficient alone**: revive/restart
typically happens while the user is away from the browser (inactivity watchdog,
crash recovery), so a toast that already auto-closed before they return is a
silent failure by another name.

### b. Durable per-item status surface — "Stuck Backlog Items"

`web-app/src/components/backlog-stuck/StuckItemsSection.tsx` is the pattern for
"an automated system detected a degraded state that isn't resolved by a single
toast." It's a `StuckReason` enum (`PR_READY_UNMERGED`, `BOUNCING`,
`REWORK_CAP`, etc.) rendered as a **persistent, filterable, grouped list**
mounted on `/unfinished` — not a toast, not a log line. Each reason has:
- a human label (`getStuckReasonLabel`)
- a fixed, deliberate display order that is explicitly "NOT a severity ranking"
  (see comment at `StuckItemsSection.tsx:14-16`)
- a recovery action always offered (test:
  `StuckItemsSection_should_alwaysOfferRecoveryAction_When_InAnyDegradedState`)

This is the right shape for "lost & restarted fresh" if it should be
discoverable *after the fact*, independent of whether the user was watching —
i.e. a status that lives on the session/backlog item itself, not just an
event stream. A minimal version: a boolean/enum field on `Session` (comparable
to how `historyFilePath` / `claudeConversationUuid` are already exposed at
`web-app/src/gen/session/v1/types_pb.ts:328,336`) that the session detail view
and session list badge can both read.

### c. Persistent inline status badge — `SessionCard` / `SessionRow`

`SessionCard.tsx:180-521` and `SessionRow.tsx:72-260` already render a
`SessionStatus`-driven badge/dot with:
- a color (`getStatusColor`)
- a text label (`getStatusText`)
- `aria-label="Session status: {text}"` on the dot itself

`SessionStatus` (`web-app/src/gen/session/v1/types_pb.ts:3414-3489`) has 9
values including `HIBERNATED` and `RESTORING` — states the UI already
distinguishes visually. "Resumed vs. lost-and-restarted-fresh" is a natural
sibling of `RESTORING`: not a new top-level status (the session is still
`ACTIVE` once it starts), but a **sub-signal** surfaced alongside status,
same as memory-pressure badges are layered onto `ACTIVE` rather than replacing
it (`MemoryPressureCallout.tsx`).

### Recommended composite pattern

Combine (a)+(b)+(c) rather than picking one:
1. Fire the existing `notify()` pipeline once, at WARNING/MEDIUM, so an
   in-the-moment user sees a toast ("Started a new session — the previous
   conversation could not be resumed").
2. Persist a durable field on the session (e.g. `lastReviveLostContext: bool`
   or richer, timestamped) so it survives the toast auto-closing and is
   visible on return — rendered as a badge/icon on `SessionCard`/`SessionRow`
   (pattern c) and/or a banner in the session detail panel.
3. If the session is backlog-driven, also post a backlog item comment —
   mirrors the memory rule's stated pattern exactly and reuses the
   already-durable backlog comment/timeline surface instead of inventing a
   new persistence path (out of scope per requirements.md Non-goals, but the
   comment-post *call*, not a new schema, is cheap and in-scope for goal 3).

## 2. User mental model

A user returning to a session distinguishes two very different situations, and
currently the UI cannot tell them apart:

- **"My session is still here and remembers what we were doing"** — the
  expected/default case after any restart, hibernation, or revive.
- **"My session looks the same but doesn't remember anything"** — the
  dangerous case: nothing in the UI currently signals this, so the user's
  default assumption (memory persisted) is silently wrong. They will resume
  the conversation assuming context ("continue where we left off," "you said
  X earlier") and get answers from an agent that has no idea what X was —
  which reads as the agent being confused or wrong, not as an infra failure.

This matches the general "silent degrade is worse than a visible failure"
principle already codified in this repo for connection state
(`ConnectionIndicator.tsx` comment: "the whole point of the pre-mortem fix was
making a self-healing-but-currently-not-updating state visible to the user,
not hiding it behind the same label/animation as an actively-retrying
stream"). The same logic applies here: a context-losing restart must not look
identical to a context-preserving one.

Industry comparables (no code changes needed to cite, background only):
- **Browser tab/session restore** ("Restore pages?") always states whether
  restore succeeded, partially succeeded, or failed — never silently opens a
  blank tab where a filled form used to be.
- **IDE crash recovery** (VS Code "We noticed you have an unsaved backup...",
  JetBrains "unsaved changes recovered") explicitly names what was and wasn't
  recovered rather than just reopening a clean buffer.
- **Editor "recovered unsaved changes" banners** are inline and persistent
  (not just a toast) precisely because the user may not be looking at the
  screen the moment recovery happens — same timing problem as this bug
  (restarts triggered by an inactivity watchdog while the user is away).

## 3. Accessibility

`ConnectionIndicator.tsx` (`web-app/src/components/backlog/ConnectionIndicator.tsx:44-52`)
is the accessibility reference implementation already in this codebase:

```tsx
<div
  className={wrapper}
  role="status"
  aria-live="polite"
  aria-atomic="true"
  aria-label={STATE_ARIA_LABEL[connectionState]}
  data-testid="connection-indicator"
>
  <span className={dots[connectionState]} aria-hidden="true" />
```

Key rules to carry over to whatever badge/banner is chosen for
resumed-vs-lost-fresh:
- The visual dot/icon is `aria-hidden="true"` — it is decoration, never the
  sole information carrier.
- A separate `aria-label` (not just visible text near the icon) states the
  meaning in full sentences, distinct per state — not a generic "warning."
- `role="status"` + `aria-live="polite"` (not `"assertive"` — this is not
  time-critical or blocking, matching `NotificationToast.tsx`'s existing rule
  that only `approval_needed` gets `aria-live="assertive"`).
- `SessionCard.tsx`/`SessionRow.tsx` already put the status text into the
  card's own `aria-label` (`SessionRow.tsx:210`) rather than requiring a
  separate landmark — the new signal should extend that existing aria-label
  string ("...status: active, context: resumed" / "...context: lost, started
  fresh") rather than adding a second unannounced icon-only badge.

## 4. What the session should look like immediately after a lost-context fresh restart

Layered, in order of durability (all should fire, they're not alternatives):

1. **Toast** (existing `Notifier`/`NotificationToast` pipeline) — fires once,
   at the moment of restart, type WARNING/priority MEDIUM. Matches
   `notifyTransitionFailed`'s shape but a new title/message:
   "Started a new session — the previous conversation could not be resumed."
2. **Durable status signal on the session** — surfaced as an aria-labeled
   badge on `SessionCard`/`SessionRow` (pattern c above) so it's visible on
   any later visit, not just the moment it happened. This is the part that
   satisfies AC3's "durable" requirement — a toast alone does not, since it
   auto-closes.
3. **In-session banner in the detail/terminal panel** — a persistent,
   dismissible banner (same visual family as `MemoryPressureCallout.tsx`:
   `role="alert" aria-live="polite"`, dismiss button, `sessionStorage`-scoped
   dismissal) at the top of the session detail view: "This session lost its
   previous conversation and started fresh. Earlier context is not
   available." This is the surface most likely to actually be read, since
   it's exactly where the user goes to interact with the agent.
4. **System message injected into the terminal itself is not recommended** as
   the primary channel — it's easy to scroll past, isn't reliably announced
   to assistive tech, and pollutes the transcript the agent itself sees on
   next resume. Prefer banner (3) over an in-terminal synthetic message; if a
   terminal marker is wanted for continuity/searchability, it should
   supplement, not replace, the banner.
5. **Backlog item comment**, if the session is backlog-driven — reuses the
   existing backlog comment/timeline surface (already durable, already
   visible on the backlog item detail page) and is the most direct
   application of the memory rule's literal wording ("post a visible comment
   + notify()").

## 5. Job-to-be-done

The job is: **"Tell me my mental model of this session is wrong, before I act
on it."** Concretely it prevents two costs:
- **Wasted re-explanation time** — the user re-typing context the agent no
  longer has, discovered only after the agent's response reveals it never
  had it.
- **Misplaced trust in a wrong-seeming answer** — worse than the first cost,
  because a plausible-but-uninformed answer from an agent that silently lost
  context can look like a real (wrong) answer rather than an obvious "I don't
  know," and the user may not immediately recognize it needs re-verification.

A signal that only exists in a backend log line does zero work against either
cost, because the user performing the job (deciding whether to trust the
session) never sees a log line. The signal must live where the user makes
that trust decision: the session card/list (before they even open it) and the
session detail panel (right where they're about to type).

## Recommendation summary for Phase 3 (plan)

- Reuse `Notifier.Notify` (WARNING/MEDIUM) for the in-the-moment toast — no
  new notification plumbing needed.
- Add a durable field to the session (exact shape — bool flag vs. richer
  struct with timestamp/reason — is a Phase 3 architecture decision, not a UX
  one) exposed over the existing proto (`Session` message already exposes
  `historyFilePath`/`claudeConversationUuid` at the same tier).
- Render it as: (1) an aria-labeled badge/dot addition on `SessionCard`/
  `SessionRow`, extending the existing status `aria-label` string, and (2) a
  dismissible banner in the session detail panel styled like
  `MemoryPressureCallout.tsx` (`role="alert"`, `aria-live="polite"`,
  `sessionStorage` dismissal keyed by session ID).
- If backlog-driven, also post a backlog item comment via the existing
  comment/timeline mechanism.
- Do not use an in-terminal injected system message as the primary channel.
