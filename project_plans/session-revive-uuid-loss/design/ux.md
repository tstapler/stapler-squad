# UX Design: session-revive-uuid-loss (Phase 3)

Source: `requirements.md` AC3, `research/ux.md`, `implementation/plan.md` Epic 2.1–2.3
(backend signal) and Epic 3.1 (frontend surface). This doc designs the
user-facing surfaces for the `ReviveOutcome` signal — specifically the
`FRESH_LOST_HISTORY` case: a cold restore found no in-memory or recoverable
on-disk conversation UUID despite the session having captured one before.

## Scope check

Per plan.md's Pattern Decisions table ("Frontend surface" row), Phase 3 as
scoped builds **two** surfaces: the existing toast pipeline (zero new
frontend code, verification only — Task 3.1.2a) and a new durable
`RevivedContextBadge` on `SessionCard`/`SessionRow` (Task 3.1.1a-d). The
in-session detail-panel banner (ux.md research layer 3, `MemoryPressureCallout`-style)
is explicitly listed in plan.md's Unresolved Questions as deferred pending a
product call — not blocking this plan, not built by any Phase 3 task. This
doc designs it anyway (Surface 4 below) so the decision-maker has a concrete
artifact to approve or reject, but it is marked **out of current scope** and
must not be treated as a Phase 3 acceptance requirement.

## Surface inventory

| # | Surface | Component | Status | Durability |
|---|---------|-----------|--------|------------|
| 1 | Toast | `web-app/src/components/ui/NotificationToast.tsx` (existing, unmodified) | In scope (verification only) | Transient — auto-closes/minimizes per `lib/notification-policy.ts` |
| 2 | Session card badge | `web-app/src/components/sessions/RevivedContextBadge.tsx` in `SessionCard.tsx` | In scope (new) | Durable — reflects `session.reviveOutcome` on every render |
| 3 | Session row badge | Same component in `SessionRow.tsx`, plus extended row `aria-label` | In scope (new) | Durable |
| 4 | In-session detail banner | Not yet named — hypothetical `RevivedContextBanner.tsx` | **Deferred** — designed for reference only | Durable, dismissible |

3 surfaces are designed in full for Phase 3 implementation; a 4th is
designed as a ready-to-approve reference for the deferred fast-follow.

---

## Surface 1: Toast (moment-of-restart signal)

No new component. `onColdRestoreLostHistory` (plan.md Task 2.3.1b) publishes
`events.NewNotificationEvent(..., type=WARNING, priority=MEDIUM, ...)`, which
`NotificationToast.tsx` already renders generically — verified precedent:
`onRateLimitRecovery` uses the identical WARNING/MEDIUM shape today with no
per-type component code (plan.md Task 3.1.2a).

### Wireframe

```
┌──────────────────────────────────────────────────────┐
│ ⚠  Warning                                        [×] │
│                                                        │
│  Session "my-worktree-session" started fresh —        │
│  previous conversation could not be resumed           │
│                                                        │
│  The session's tmux pane restarted and the previous   │
│  conversation history could not be found on disk.     │
│  Earlier context is not available.                    │
│                                                        │
│  my-worktree-session          · 12s ago               │
└──────────────────────────────────────────────────────┘
        (bottom-right corner, stacks above other toasts)
```

### Interaction flow

1. **Trigger**: `startLocked`/`start` completes a cold restore with
   `outcome.Outcome == ReviveOutcomeFreshLostHistory` →
   `EventStarted` fires with `ReasonColdRestoreLostHistory` →
   `coldRestoreOutcomeListener` calls `onColdRestoreLostHistory` →
   `events.NewNotificationEvent(...)` is published on the event bus.
2. **System response**: the notification reaches the frontend over the
   existing notification stream; `NotificationToast` mounts it like any
   other WARNING/MEDIUM notification — no user action required to see it if
   they are already looking at the app.
3. **User does nothing** (no action button is offered — this is informational,
   not an approval gate, matching `notifyTransitionFailed`'s WARNING-tier
   shape, not the ERROR-tier `approval_needed` shape that gets action buttons).
4. **Auto-dismiss**: per `lib/notification-policy.ts`'s existing WARNING/MEDIUM
   timing, the toast auto-minimizes then auto-closes. This is the exact gap
   Surface 2/3 exist to cover — ux.md's finding that restarts typically
   happen while the user is away, so the toast alone is not sufficient.
5. **Manual dismiss**: clicking `[×]` closes it immediately (existing
   `closeButton` behavior, unmodified).

### Edge cases

| Case | Behavior |
|---|---|
| User is not looking at the browser when the toast fires | Toast auto-closes unseen. This is expected and acceptable *because* Surface 2/3 (durable badge) persists independently — this is why plan.md requires both, not either. |
| Session is `Hidden` (headless backlog session) | No toast fires at all — `onColdRestoreLostHistory` is gated on `!inst.Hidden` (plan.md AC, Story 2.3.1). Correct: a user isn't watching a hidden session's toast stream, so firing one would be noise with no audience. |
| Two restarts lose history in quick succession (watchdog churn) | Each fires its own notification with `notifID = "cold-restore-lost-history-<uuid>"` — not deduplicated by design (matches `onRateLimitRecovery`'s per-event shape). Acceptable: this is an existing, accepted notification-volume tradeoff, not something this feature needs to newly solve (requirements.md Non-goals explicitly excludes "fixing the upstream restart churn itself"). |
| Multiple toasts stack | Existing stacking/minimize behavior in `NotificationToast.tsx`, unmodified — out of scope here. |

---

## Surface 2: RevivedContextBadge on `SessionCard` (grid view)

New component `web-app/src/components/sessions/RevivedContextBadge.tsx`,
mounted in `SessionCard.tsx` next to the existing `<StatusBadge .../>` call
(`web-app/src/components/sessions/SessionCard.tsx:538`).

### Wireframe

```
┌───────────────────────────────────────────┐
│ my-worktree-session                    ⋮  │
│ /home/user/.stapler-squad/worktrees/proj-42│
│                                             │
│ [● Active]  [⚠ Context lost]               │  ← StatusBadge, then RevivedContextBadge
│                                             │
│  ...terminal preview / task list...        │
└───────────────────────────────────────────┘
```

Badge close-up (matches `ConnectionIndicator.tsx`'s visual family — small
pill, warning-tone background, icon + short label):

```
┌───────────────────┐
│ ⚠  Context lost    │   role="status"
└───────────────────┘   aria-label="This session lost its previous
                          conversation and started fresh"
                          icon is aria-hidden="true"
```

### Interaction flow

1. **Render-time only** — no user action triggers this; it is a pure
   function of `session.reviveOutcome` on every render of the card
   (`RevivedContextBadge.tsx` returns `null` unless
   `reviveOutcome === ReviveOutcome.FRESH_LOST_HISTORY`).
2. **Discovery**: user scans the session grid (dashboard/list view) and sees
   the badge on any card whose last restart lost context — this is the "know
   before you open it" surface from ux.md's job-to-be-done.
3. **Hover** (optional, not required by any AC): if a `Tooltip` wrapper is
   added later (matching the existing `Tooltip` usage on `SubStatusChip`/
   status spans in `SessionCard.tsx`), it would repeat the same full-sentence
   text already in the `aria-label`. Not required for Phase 3 — the always-
   visible label text ("Context lost") plus `aria-label` on hover-less devices
   already satisfies discoverability; a tooltip is a nice-to-have, not a gap.
4. **Click-through**: clicking the badge itself does nothing special — the
   card's own click handler (opens the session) still fires, since the badge
   is an inline `<span>`, not a separate interactive element. Opening the
   session is the only "recovery action" available (there is nothing to
   retry — the context is gone), so no dedicated action button is needed on
   the badge, unlike `StuckItemsSection`'s "always offer a recovery action"
   convention (that pattern applies to *actionable* degraded states; this one
   has no action beyond "be aware").
5. **Badge disappears** the next time this session restarts with any
   `reviveOutcome` other than `FRESH_LOST_HISTORY` (e.g. a subsequent normal
   restart with `RESUME_LIVE`) — it is not a one-way "flag forever," it
   reflects the *most recent* restart's outcome only (`LastReviveOutcome`,
   per the Domain Glossary — "Last", not "Ever").

### Edge cases

| Case | Behavior |
|---|---|
| `reviveOutcome` is `UNSPECIFIED`, `RESUME_LIVE`, or `RESUME_RECOVERED` | Badge renders `null` — no visual change at all, not even an empty slot (verified by Task 3.1.1b's explicit early return). |
| Session has never done a cold restore (freshly created) | `reviveOutcome` defaults to `REVIVE_OUTCOME_UNSPECIFIED` (proto3 zero value) → badge does not render. Matches Migration Plan's stated zero-value-default behavior for pre-existing sessions. |
| Card is very narrow (mobile/responsive grid) | Badge is an `inline-flex` pill sized like the existing `StatusBadge`/`SubStatusChip` siblings it sits next to — wraps to the next line under the same flex-wrap container those badges already use; no separate mobile treatment needed since it reuses the existing badge-row layout. |
| Both `StatusBadge` and `RevivedContextBadge` would render simultaneously | Both show — they are not mutually exclusive (one describes current detected status, the other describes what happened on last restart). No suppression logic needed, unlike `StatusBadge`'s own suppression-vs-`SubStatusChip` rule (which is about avoiding literal duplication of the *same* status information, not different information). |

---

## Surface 3: RevivedContextBadge on `SessionRow` (list/table view)

Same component, mounted in `SessionRow.tsx` next to its existing
status-badge rendering, per Task 3.1.1d.

### Wireframe

```
┌──┬───┬────────────────────────────┬──────────────────┬─────────┐
│☐ │ ● │ my-worktree-session         │ /home/.../proj-42 │ Claude  │
│  │   │ [⚠ Context lost]            │                    │         │
└──┴───┴────────────────────────────┴──────────────────┴─────────┘
  checkbox  status  name + badge row      path              program
             dot
```

The row's own `aria-label` (currently, `SessionRow.tsx:210`:
`` `Session ${title}, status: ${...}, program: ${...}${path...}` ``) is
extended, not duplicated:

```
Before: "Session my-worktree-session, status: active, program: claude, path: ~/proj-42"
After:  "Session my-worktree-session, status: active, program: claude, path: ~/proj-42, context: lost"
```

### Interaction flow

1. Identical render-only behavior to Surface 2 — pure function of
   `session.reviveOutcome`.
2. **Screen-reader flow**: because the row is a single `tabIndex={0}` focusable
   element with one combined `aria-label` (not a nested list of separately
   announced landmarks), a screen-reader user tabbing to the row hears the
   full sentence in one announcement, ending with "...context: lost" —
   consistent with ux.md's explicit accessibility recommendation ("extend
   the existing card-level aria-label rather than adding a second,
   separately-announced landmark").
3. Sighted users see the same visual pill as Surface 2, positioned in the
   name/path stack alongside `SubStatusChip`/`GitHubBadge`.
4. Keyboard flow: row is already keyboard-focusable and activatable via
   Enter/Space (`handleKeyDown`, `SessionRow.tsx`) — the badge does not add
   or remove any keyboard stop; it is `aria-hidden`-free but not itself
   `tabIndex`-focusable (it carries information, not an independent action).

### Edge cases

| Case | Behavior |
|---|---|
| Table view has narrow columns (name cell truncated) | Badge sits below the name in the existing `pathLineStyle` flex row, which already wraps `SubStatusChip`/`GitHubBadge`/path text — same container, same overflow handling, no new truncation logic needed. |
| Row's `aria-label` already lists several sub-statuses | The "context: lost" suffix is appended unconditionally at the end when the condition is true — order is stable and doesn't reflow around other conditional segments (`status`, `program`, `path` are always present in that fixed order already; this fix only appends one more fixed segment). |
| Select-mode active (checkboxes visible) | No interaction — badge is purely informational and doesn't participate in selection; unaffected by `selectMode`. |

---

## Surface 4 (deferred — not in Phase 3 scope): In-session detail banner

Designed for reference per plan.md's Unresolved Question ("Should the
ux.md layer-3 in-session banner be built now or deferred as a fast-follow?
... owner: requirements owner / product call during implementation
review"). **Do not build this as part of Phase 3** — include only if a
reviewer explicitly asks for it during implementation review, per the plan's
own scoping decision.

### Wireframe

```
┌─────────────────────────────────────────────────────────────────┐
│ ⚠  This session lost its previous conversation and started      │
│    fresh. Earlier context is not available.               [Dismiss] │
└─────────────────────────────────────────────────────────────────┘
  ...session terminal / task panel below...
```

Styled like `MemoryPressureCallout.tsx`: `role="alert"`, `aria-live="polite"`,
a dismiss button, `sessionStorage`-scoped dismissal keyed by session ID (so
dismissing it in one browser tab doesn't need to persist across devices —
matches `MemoryPressureCallout`'s own `sessionStorage` key pattern, e.g.
`"revived-context-dismissed"` storing a `Set<sessionId>`).

### Interaction flow (if built)

1. Renders at the top of the session detail/terminal panel when
   `session.reviveOutcome === ReviveOutcome.FRESH_LOST_HISTORY` and the
   session ID is not already in the `sessionStorage` dismissed set.
2. User clicks **Dismiss** → banner disappears, ID added to the dismissed
   set, banner does not reappear for this session in this browser tab's
   session storage scope even on next visit (until storage is cleared or a
   *new* restart produces a fresh `FRESH_LOST_HISTORY` outcome, which would
   need to re-key dismissal by restart timestamp, not just session ID, to
   avoid suppressing a second, later loss — noted as an open design question
   if this surface is ever built, not resolved here).
3. No dismiss = banner persists across re-renders/navigation within the
   session detail view — this is "the surface most likely to actually be
   read" per ux.md, since it sits exactly where the user is about to type.

### Edge cases (if built)

| Case | Behavior |
|---|---|
| User dismisses, then session restarts again and loses context a second time | Per the open question above, naive `sessionId`-only dismissal would incorrectly suppress the second banner. Any implementation of this surface must re-key on `(sessionId, restart-timestamp)` or clear the dismissal on a new `EventStarted` — flagged so it isn't silently under-scoped if picked up later. |
| Banner and toast both fire for the same event | Both are additive, not exclusive (ux.md's explicit "layered, in order of durability... they're not alternatives" instruction) — expected, not a bug. |

---

## UX acceptance criteria

All criteria are for the **in-scope** surfaces (1–3) unless marked
"(deferred)".

### Functional

1. A user can determine, without opening any session, whether its last
   restart lost conversation context — by scanning the session grid or list
   for the "Context lost" badge — in **0 additional clicks** (badge is
   always visible on the card/row, no expand/hover required to see it exists).
2. A user who is actively viewing the app when a context-losing restart
   happens sees the toast within the existing notification pipeline's normal
   delivery latency (no new latency introduced — Task 3.1.2a is verification
   only, no pipeline change).
3. A user who was away when the restart happened and returns later still
   sees the signal (badge), in **0 additional clicks**, satisfying AC3's
   "durable" requirement — this is the test that most directly proves the
   toast-alone approach (rejected in ux.md) would have failed.
4. The badge and toast never appear when `reviveOutcome` is anything other
   than `FRESH_LOST_HISTORY` — verified by Task 3.1.1b/4.2.1a's explicit
   null-render test for `UNSPECIFIED`, `RESUME_LIVE`, `RESUME_RECOVERED`.
5. No dead end: the only "error state" here is informational (context is
   gone, nothing to retry) — the exit path is simply continuing to use the
   session normally, which is already possible with zero extra clicks (the
   badge/banner never blocks or intercepts any existing action). There is no
   modal, no blocking dialog, no action the user is forced to take.

### Accessibility

6. Badge is keyboard-navigable *by inheritance*: it adds no new focus stop,
   and the row/card it lives in remains reachable and activatable via
   existing keyboard flows (Tab, Enter/Space) — verified by confirming
   `RevivedContextBadge`'s root element has no `tabIndex` and no `onClick` of
   its own.
7. Screen-reader users get the full meaning through one of two paths: (a) on
   `SessionCard`, the badge's own `role="status"` + `aria-label="This session
   lost its previous conversation and started fresh"` is announced when
   focus/live-region updates reach it; (b) on `SessionRow`, the meaning is
   folded into the row's single combined `aria-label` so one Tab-stop
   announcement covers it — no screen-reader user has to discover a second,
   separately-announced element to get the information (ux.md's explicit
   accessibility rule).
8. The badge's icon (`⚠` or equivalent) is `aria-hidden="true"` in all
   cases — verified by Task 4.2.1a's test asserting the icon span carries
   `aria-hidden`. The icon is decoration only; removing it must not remove
   any information (the text label "Context lost" and the `aria-label`
   carry the meaning independently).
9. `aria-live="polite"` (not `"assertive"`) on the badge — matches
   `ConnectionIndicator.tsx`'s precedent that this class of signal is not
   time-critical/blocking (only `approval_needed`-class notifications get
   `assertive` per `NotificationToast.tsx`'s existing rule).
10. Color contrast: the badge's warning-tone background/text combination
    must meet **≥ 4.5:1** contrast ratio (WCAG AA, normal text) — reuse
    whichever existing `--warning*`/`vars.color.statusWarning`-equivalent
    token from `web-app/src/app/globals.css` or the vanilla-extract theme
    contract already passes this bar elsewhere in the app (`.claude/rules/css-architecture.md`
    forbids new hardcoded hex values), so no new contrast validation is
    needed beyond confirming the *existing* token is reused, not a new color
    introduced.
11. The badge text itself ("Context lost") is present as real DOM text, not
    conveyed by icon/color alone — satisfies WCAG 1.4.1 (Use of Color) by
    construction, matching `ConnectionIndicator.tsx`'s `label` + `dots` split.

### Deferred surface (Surface 4) — criteria to apply only if built

12. (deferred) Banner offers a visible **Dismiss** control — no dead end,
    matching `MemoryPressureCallout.tsx`'s existing dismiss affordance.
13. (deferred) Dismissing the banner must not permanently suppress a *future,
    later* context-loss event for the same session — see the re-keying edge
    case above; this is a blocking correctness requirement if Surface 4 is
    ever implemented, not optional polish.

---

## Traceability

| Requirement | Surface(s) | UX AC(s) |
|---|---|---|
| AC3 "durable, user-visible signal ... session event/status field the frontend can surface" | 1 (toast, in-the-moment) + 2/3 (badge, durable) | 1, 2, 3, 5 |
| Goal 3 "not just a backend log line" | 1, 2, 3 | 2, 3 |
| Goal 4 "do not regress legitimate fresh-start cases" | 2, 3 (null-render for non-`FRESH_LOST_HISTORY`) | 4 |
| ux.md accessibility reference (`ConnectionIndicator.tsx`) | 2, 3 | 6–11 |
| ux.md "toast alone is insufficient" finding | 2, 3 (durable badge as the fix) | 3 |
