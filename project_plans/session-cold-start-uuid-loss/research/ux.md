# UX Research: Surfacing Cold-Start Context Loss (AC4)

## 1. Comparable UX patterns

Dev tools that face "we couldn't restore exactly what you had" converge on the same shape:
low-key, informational, dismissible/persistent-but-quiet, never a modal or red error.

- **tmux/screen**: no equivalent UX at all — a dead session that gets recreated is silent
  (this is literally the bug: stapler-squad currently matches tmux's own silence, which is
  the wrong bar to clear for an agent tool where the "memory" *is* the product).
- **Browser tab crash recovery** (Chrome/Firefox "Restore pages?"): appears once, at the
  point of the action, as a passive one-line infobar, not a dialog demanding a decision —
  and it disappears on its own once acknowledged. Tone: neutral fact ("Chrome didn't shut
  down correctly"), not an apology or alarm.
- **VS Code** "Failed to restore terminal session" / workspace-trust-lost banners: a
  `Notification` toast (info/warning severity, not error) with an optional action link
  ("Restart Terminal"), auto-dismissing but also retrievable from the Notifications bell
  afterward — critically, it does **not** vanish into the void if the user wasn't looking
  at the moment it fired.
- **Slack/Linear "reconnecting..." banners**: transient top-of-view strip, resolves itself,
  never blocks input.

Common thread relevant here: the triggering event (inactivity-timeout restart) can happen
while the user is *not looking at the UI* — same failure mode Chrome/VS Code solve by
making the signal **persistent-until-seen**, not just a toast that fires-and-forgets. A
transient toast alone (e.g. this repo's `NotificationEvent`, event-bus retained only 1
hour, see `proto/session/v1/events.proto:9-34`) is the wrong primary channel for this
specific case — it would satisfy "surfaced in real time" but fail "the user finds out
eventually," which is the actual job here (see §6).

## 2. User mental model

The stapler-squad user's working mental model of a session is closer to **a persistent
remote terminal (tmux/screen) than a stateless CLI invocation** — that's the entire premise
of the product (git worktrees + long-lived tmux sessions +
`--tmux-keep-server`/hibernation machinery, see `.claude/rules/tmux-keep-server-on-restart.md`).
Users already know tmux panes can die; what they trust the *agent layer* to do differently
is remember the conversation regardless. The requirements doc's own framing confirms this
is a trust break, not a cosmetic gap: "Users may not notice until the agent demonstrably
'forgot' everything, at which point recovery requires manually reconstructing state from
git diffs/files" (`requirements.md:36-39`).

What would surprise the user: discovering — via the agent visibly not knowing something it
was told minutes ago — that this happened, with no prior indication. The surprise is not
"my session restarted" (expected, tmux can die) but "the restart silently erased the
conversation and nothing told me."

Minimum information to recalibrate trust, in priority order:
1. **This is a fresh conversation, not the resumed one** — the single fact that changes
   what the user should do next.
2. **When** it happened (so they can correlate with "did I lose the last 10 minutes or the
   last 2 hours").
3. Optionally, **why** ("tmux session was restarted and no resumable transcript was found")
   — useful for the power user who's seen this bug report, unnecessary for everyone else.

They do *not* need the internal mechanism (UUID, JSONL path) — that's implementation detail
for the log, not the badge.

## 3. Existing patterns in this codebase to follow

Three existing mechanisms are directly on-point, in order of closest fit:

### 3a. `creationProgress` — the strongest precedent (persisted "why" string surviving past its triggering phase)

`session.creation_progress` (`proto/session/v1/types.proto:172-174`) is a
human-readable string set by the backend actor (`session/instance_actor_setters.go:67-68`,
`SetCreationProgress`), normally cleared on successful start
(`session/health.go:193,240` — `SetCreationProgress("")`) but **deliberately left set**
when something noteworthy happens instead, so it survives into a later render. The
frontend already documents and reuses this exact "repurpose a completion-phase field as a
durable diagnostic note" pattern — see the comment at
`web-app/src/components/sessions/SessionCard.tsx:501-505`:

```tsx
) : session.status === SessionStatus.STOPPED && session.creationProgress ? (
  // ponytail: reuses creationProgress — the field is only cleared on a
  // successful start, so a startup/reconnect failure written here (see
  // instance.SetCreationProgress in health.go / connectrpc_websocket.go)
  // survives past the Creating phase and doubles as a "why stopped" reason.
```

It's rendered two ways worth copying:
- `SessionCard.tsx:506-514` — a `Tooltip` wrapping the existing status pill, with
  `aria-label` carrying the full text (`Session status: ${text} — ${creationProgress}`) so
  screen readers get it without hover.
- `SessionDetailView.tsx:717-721` — a plain `<p>` under a bolder title in a full-screen
  overlay state.

This is the template to extend for AC4: add a durable string (or a small enum + string,
see §5) set on the Instance actor at the exact point `instance.go` currently only logs
`log.Warn("cold start: tmux dead, no conversation UUID, starting fresh", ...)`
(`session/instance.go` — both call sites named in requirements.md), cleared the next time a
resume succeeds, surfaced via the same Tooltip+aria-label pattern. Because the session
*stays Active* in this scenario (unlike the Stopped case `creationProgress` currently
covers), the badge should attach to the existing status pill/SubStatusChip area rather than
only the Stopped-state branch — see §5 for exact placement.

### 3b. `pause_reason` — sibling precedent for a persisted, user-facing reason string

`session.pause_reason` (`proto/session/v1/types.proto:191-193`) is the same shape one field
over: a plain string, persisted, with fixed known values (`"manual"`,
`"auto:inactivity"`, ...), formatted for display via a dedicated helper
`formatPauseReason` (`web-app/src/lib/sessions/formatPauseReason.ts`, used at
`SessionCard.tsx:492` and `SessionDetailView.tsx:719`). If the fix lands on a fixed set of
recovery outcomes (recovered-from-disk / fresh-start / partial), model it the same way:
raw enum-like string in the proto/DB, human copy generated by a small formatter function,
not templated inline in JSX.

### 3c. `SubStatus` — explicitly wrong fit, note why

`SubStatus` (`proto/session/v1/types.proto:410-434`, rendered by
`web-app/src/components/sessions/SubStatusChip.tsx`) looks tempting (it already has a chip
component with `role="status"` + `aria-label` + tooltip-via-`title`, see
`SubStatusChip.tsx:29-42`) but its doc comment rules it out: "**Derived at read time from
the detection layer; never stored in the database**" (`types.proto:411`). Cold-start
context loss is a one-time historical fact about *this specific revive*, not a live,
re-derivable terminal state — it must be persisted, so it needs a real field, not a
`SubStatus` value.

### 3d. `NotificationEvent` / event bus — supplementary, not primary

`NotificationEvent` (`proto/session/v1/events.proto:388-405`, consumed via
`web-app/src/lib/hooks/useSessionNotifications.ts`) is the right channel for *if the user
is currently watching this session*, but the event bus retains events for only one hour
(`events.proto:29-33`) and a toast is easy to miss mid-restart-churn. Fine as a bonus
real-time nudge; must not be the only signal (see §1, §6).

No existing "session event timeline" / audit-log component was found (`session/history.go`
is unrelated — it wraps Claude's own `~/.claude/history.jsonl` conversation index, not a
stapler-squad session event log). `SessionLogsTab.tsx` renders raw process logs via the
shared `LogViewer`, not a structured system-event stream, so it's not a good primary
surface either (a WARN-level log line already exists there today and is exactly what
AC4 says is insufficient).

## 4. Accessibility requirements

Per `.claude/rules/css-architecture.md` and the Axe Core CI gate (root `CLAUDE.md:211`,
blocks on WCAG AA violations touching `web-app/src/`):

- **Role**: use `role="status"` (polite, non-interrupting) for the badge itself, matching
  `SubStatusChip.tsx`'s existing convention — this is an informational state change, not an
  urgent error (`role="alert"` is reserved in this codebase for things needing immediate
  attention, e.g. `TriageErrorBanner.tsx:19`, `MemoryPressureCallout.tsx:66`). Cold-start
  fallback is regrettable but not actionable-right-now, so `alert`'s interrupt semantics are
  disproportionate — reserve `alert` for the case where recovery couldn't even determine
  whether the found transcript is right (§5's "partial" case may warrant `alert` if it
  requires a decision).
- **aria-label must carry the full sentence**, not rely on hover/tooltip text alone —
  follow `SessionCard.tsx:510`'s pattern (`aria-label={... — ${creationProgress}}`) so
  screen-reader users get the explanation without a mouse.
- **No color-only signal**: don't rely solely on an amber/red pill color; pair with an icon
  + text label (existing chips like `SubStatusChip.tsx` already do glyph + text, e.g. "⏳
  Waiting for Agents" at line 40).
- **Contrast**: any new color must come from `theme.css.ts` tokens (`vars.color.*`), not a
  hardcoded hex — per `.claude/rules/css-architecture.md`'s "Never Do" list. If a new status
  color is needed (distinct from existing `statusDanger`), add it to the theme contract
  first, don't inline a hex.
- **Dismissal state, if any, must not remove the info entirely** — `MemoryPressureCallout`'s
  per-session `sessionStorage` dismiss pattern (`MemoryPressureCallout.tsx:29-47`) is fine
  for a repeatable recommendation, but here the underlying fact ("this conversation started
  fresh") is a one-time historical event; dismissing the banner should not make it
  impossible to later find on the session detail view (i.e., persist it as a field the
  detail view can still show even after the card-level badge is dismissed/tooltip closed).
- Keep the string short enough not to trigger reflow/truncation issues already guarded
  against elsewhere in `SessionCard.css.ts`; this is a review-checklist item CI won't catch
  automatically, verify visually.

## 5. Partial vs. total failure UX

Per requirements AC1/AC3, there are three outcomes to distinguish, and the UX should not
flatten them into one message:

| Outcome | What happened | UI treatment |
|---|---|---|
| **Recovered** (AC1) | In-memory UUID empty, but a transcript was found on disk (or a durably-persisted UUID from AC2) and resume succeeded | **No banner at all.** This is the success path the whole fix exists to make more common — showing a badge here would be UX noise for something that worked exactly as the user expects. Optionally a one-line non-alarming note in `SessionDetail`/logs ("resumed from recovered transcript") for the curious power-user, but not a persistent pill. |
| **Genuine fresh start** (AC3, no transcript ever existed — true first-ever start) | No JSONL, no persisted UUID, nothing to recover | **No banner.** This is normal/expected (first-time-setup already has its own "Creating" flow); a "context lost" badge here would be a false alarm and erode trust in the badge itself the next time it's genuinely warranted. Must be distinguishable in code from case 3 below by checking "was there ever a `ConversationUUID`/transcript for this session" vs. "search found nothing" — don't conflate "nothing to find" with "something to find but recovery failed." |
| **Cold start without resume — true context loss** (this ticket's core case: recovery was attempted per AC1/AC2 but still came up empty despite the session having run before) | Session previously had a conversation, tmux died, in-memory UUID was empty, and disk recovery also found nothing usable (transcript missing/corrupt) | **Visible badge/note** per AC4, using the `creationProgress`-style durable field pattern from §3a: short label on the status pill (e.g. "Started fresh — previous context not found"), full sentence in `aria-label` and tooltip, persisted so it's visible whenever the user next looks, not just at the moment it happened. |

A fourth case worth naming even though it's not explicitly separated in the acceptance
criteria: **ambiguous recovery** (a transcript exists but there's low confidence it's the
*right* one, e.g. multiple candidate JSONLs under the encoded-path directory with no
timestamp/UUID tiebreaker). If the implementation can end up here, it should be its own
distinguishable message ("found a possible previous conversation but couldn't confirm it —
resumed with the most recent one" / "started fresh to avoid resuming the wrong
conversation") rather than silently collapsing into either the clean-recovery or
total-failure buckets — the user's next action differs (spot-check whether the agent's
claimed memory matches reality) from either extreme.

## 6. Job-to-be-done

**Functional job**: let the user correctly decide their next action — re-explain context,
or go check git/files for where they actually left off — without first having to notice the
agent "forgot" something through trial and error, and without having to read server logs
they likely don't know exist (`~/.stapler-squad/logs/stapler-squad.log`, per root
`CLAUDE.md`'s Application Data section).

**Emotional job**: preserve trust in the tool's reliability claim. This product's core pitch
is long-running, resumable agent sessions across restarts/hibernation; a silent context-loss
event that surfaces only as the agent visibly "not knowing" something is the single most
damaging failure mode for that trust, because it's discovered adversarially (the user
catches the tool being wrong) rather than disclosed proactively (the tool tells the user it
might be wrong). A calm, factual, low-alarm badge — modeled on `creationProgress`/
`pause_reason`, not `TriageErrorBanner`'s `role="alert"` — converts an eroding "wait, does
this thing actually remember anything?" moment into a manageable "OK, this one restarted
without your notes, got it" moment.
