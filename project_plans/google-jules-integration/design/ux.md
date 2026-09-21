# UX Design: google-jules-integration

SDD Phase 3 (design), following `implementation/plan.md` (Ready for
implementation). This document designs the surfaces plan.md actually
committed to — **not** `research/ux.md`'s original recommendation. The one
material deviation, recorded in plan.md ("Deviation from `research/ux.md`
§2"): a Jules-backed unit of work is an `ItemSession` only, with no `Session`
ent row and no tmux pane, so it never reaches `SessionDetailView.tsx` and
gets no "Activity" tab there. Its status/activity surface lives entirely in
`BacklogItemDetail`'s `SessionsSection`, `ActionsSection`, and the existing
`ProgressHistorySection` timeline. What research/ux.md got right and this
doc keeps: icon+label+color (never color-alone), the polite/assertive
live-region split, the `Phase`-shaped stale-vs-failed distinction, and
`GitHubBadge` reused unchanged for the PR.

## 1. Surface inventory

| # | Surface | Component (plan.md) | Treatment |
|---|---|---|---|
| A | Settings: API key, enable toggle, egress-repo list, usage counters | `JulesSettings.tsx` | Full |
| B | Dispatch action + first-use egress confirmation dialog | `ActionsSection` gated button + `JulesDispatchDialog.tsx` | Full |
| C | Session status chip in the item's session list | `JulesStatusBadge.tsx` in `SessionsSection.tsx` | Full |
| D | PR provenance marker | `PullRequestSection.tsx` (+`GitHubBadge`, unchanged) | Condensed |
| E | Activity/status history | Reused `ProgressHistorySection.tsx` (append-only notes from `applyJulesState`) | Condensed |
| F | Structured logs | `jules` slog lines (Observability Plan) | Condensed |

Error states are not a separate surface — each one is pinned to the surface
where the user would encounter it (§6 has the cross-cutting table).

---

## 2. Surface A — Jules Settings panel

**Route**: `/settings/jules` (`web-app/src/app/settings/jules/page.tsx`).
Reached from the settings nav, same list as `SlackNotificationSettings`.

### 2.1 Wireframe

```
┌─ Settings ▸ Jules ──────────────────────────────────────────────┐
│                                                                   │
│  Google Jules                                                    │
│  Dispatch backlog items to Jules' cloud coding agent. Code for   │
│  a dispatched item is sent to Google's infrastructure — see      │
│  "Cloud egress" below before enabling.                           │
│                                                                   │
│  ┌ Enable Jules integration ───────────────────────  [ ○──● ]┐   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                   │
│  API key                                                         │
│  ┌───────────────────────────────────────────┐  [ Save key ]    │
│  │ Key stored — enter a new key to replace it │  (type=password)│
│  └───────────────────────────────────────────┘                  │
│  Get a key at jules.google.com/settings ↗                        │
│                                                                   │
│  Test connection                                                  │
│  Repo: [ tstapler/stapler-squad          ▾ ]  [ Test connection ]│
│  ┌ role="status" ───────────────────────────────────────────┐   │
│  │ ✓ Connected — tstapler/stapler-squad is reachable.        │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                   │
│  Cloud egress — repos you've allowed                              │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ tstapler/stapler-squad                        [ Revoke ]   │  │
│  │ tstapler/dotfiles                              [ Revoke ]   │  │
│  └───────────────────────────────────────────────────────────┘  │
│  (No repos yet — the confirmation appears the first time you     │
│   dispatch an item from a new repo.)                             │
│                                                                   │
│  Limits                                                           │
│  Max concurrent Jules sessions   [ 2  ]  (default 2, max 10)     │
│  Max Jules sessions per day      [ 15 ]  (default 15, max 300)   │
│                                                                   │
│  Usage                                                            │
│  7 dispatched · 5 completed · 2 failed                           │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

### 2.2 Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Opens Settings → Jules | Panel loads `GetJulesConfig`; key field renders **empty** with placeholder text (never the real key — write-only field), toggle/caps/usage populate. |
| 2 | Pastes a key, clicks **Save key** | `UpdateJulesConfig` call; on success, field clears back to the "stored" placeholder and a transient `role="status"` confirmation ("Key saved.") appears; on failure, an inline error under the field, key input retains focus, field is **not** cleared (user shouldn't have to retype). |
| 3 | Toggles **Enable Jules integration** on | If no key is stored yet, the toggle still flips (persisted) but a warning line appears under it: "Add an API key below to activate this." No dispatch surface elsewhere lights up until a key exists (see Surface B gating). |
| 4 | Picks a repo, clicks **Test connection** | Button shows a busy state ("Testing…"), then the `role="status"` region below updates to either a success line or the actionable "not connected" message (§6). |
| 5 | Clicks **Revoke** on an acknowledged repo | Confirm inline (no separate modal — this is a low-consequence, reversible toggle: re-dispatching to that repo just re-prompts the egress confirmation). `UpdateJulesConfig` removes the repo; row disappears with a brief "Removed" status line. |
| 6 | Edits a limit field, tabs out | Value is clamped client-side to the documented range (2–10, 15–300) before save, matching the server's clamp so the user never submits a value the server silently rewrites without telling them. |

### 2.3 Error / edge-case handling

| Trigger | What the user sees | Exit path |
|---|---|---|
| Key rejected on save (malformed/empty after trim) | Inline error under the field: "Enter a key from jules.google.com/settings." | Retype and re-submit; nothing else on the page is blocked. |
| `Test connection` — repo not connected at jules.google.com | `role="status"`: "tstapler/stapler-squad is not connected to Jules. Connect it at jules.google.com, then test again." with an outbound link. | Link to jules.google.com opens in a new tab; user can retest without leaving the page. |
| `Test connection` — API unreachable/rate-limited | `role="status"`: "Couldn't reach Jules right now. Try again in a moment." (never a raw HTTP code as the primary message). | Retest button stays enabled/available immediately — this is not a lockout. |
| Limits set above the hard ceiling | Field shows an inline note ("Capped at 10") and snaps to the ceiling on blur, not on every keystroke. | User sees the value they'll actually get before saving. |

---

## 3. Surface B — Dispatch action and egress-confirmation dialog

**Entry point**: a `Dispatch to Jules` button in `BacklogItemDetail`'s
`ActionsSection`, gated per plan.md Story 3.2.2. Opens `JulesDispatchDialog`.

### 3.1 Wireframe — gated button states

```
Feature off (enabled:false):           [ nothing rendered — no dead button ]

Enabled, no key:
  [ Dispatch to Jules ]  (disabled, greyed)
   ⓘ Add a Jules API key in Settings to enable cloud sessions.

Jules session already open for this item:
  [ Dispatch to Jules ]  (disabled, greyed)
   ⓘ A Jules session is already running for this item.

No branch known for this item yet:
  [ Dispatch to Jules ]  (disabled, greyed)
   ⓘ This item has no branch yet — spawn a local session
     (or push a branch) before dispatching to Jules.

Ready to dispatch:
  [ Dispatch to Jules ]  (enabled, primary-adjacent styling —
                           same tier as "Spawn session", not the
                           item's primary CTA)
```

Gating precedence, top to bottom (only the first matching state applies —
never two reasons shown at once): feature off (button not rendered) → no key
→ Jules session already open → no branch known → enabled. "No branch known"
means zero of the item's `ItemSession` rows carry a non-empty
`worktree_branch` — the same field `SessionsSection`'s per-row branch badge
already reads (§4.1), sourced from `GitWorktreeData.BranchName` via
`GetWorktreeDataBySessionUUID`. A brand-new item that has never had a local
session falls into this state by construction. Deliberately **not** a
free-text-only affordance: the repo has no signal for "was this specific
branch actually pushed" (only local-ref presence is tracked), so rather than
open the dialog on a guess, dispatch is blocked until the item has *some*
locally-known branch to prefill and confirm.

### 3.2 Wireframe — dialog, first dispatch to a new repo

```
┌─ Dispatch to Jules ───────────────────────────────────── [ ✕ ] ─┐
│                                                                   │
│  ⚠ The contents of tstapler/stapler-squad will be sent to        │
│    Google's cloud VM to run this session.                        │
│    ☐ I understand and want to continue                           │
│                                                                   │
│  Branch                                                           │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ backlog/fix-flaky-poller-test                               │  │
│  └───────────────────────────────────────────────────────────┘  │
│  (pre-filled from the item's own most recent local branch —      │
│   editable; see "Branch prefill" below)                          │
│  Jules starts from a branch already pushed to GitHub —            │
│  local-only branches won't work.                                 │
│                                                                   │
│  Prompt                                                           │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ Fix the flaky poller test in session/                       │  │
│  │ worktree_pr_poller_test.go — see acceptance criteria below. │  │
│  │                                                               │  │
│  └───────────────────────────────────────────────────────────┘  │
│  (prefilled from the item's title + acceptance criteria)         │
│                                                                   │
│                                    [ Cancel ]   [ Dispatch ]      │
│                                                  (disabled until  │
│                                                   box checked +   │
│                                                   branch present) │
└───────────────────────────────────────────────────────────────────┘
```

For a repo already in `EgressAcknowledgedRepos`, the entire `⚠ …` block is
absent — the dialog opens directly to Branch/Prompt, and `Dispatch` enables
as soon as both fields are non-empty.

**Branch prefill**: the dialog never opens with a genuinely blank Branch
field — §3.1's gating already keeps the dialog unreachable for an item with
no known branch. When it is reachable, the initial value is the item's most
recently created `ItemSession`'s `worktree_branch` — the same value already
shown per-row by `SessionsSection`'s branch badge (§4.1) — and the field
stays editable so the user can dispatch a different already-pushed branch if
they want. Because there is no signal in this repo for "was this branch
actually pushed" (only local-ref presence is tracked, per the gating note
above), the prefill is a best-effort default, not a guarantee — the static
helper text under the field stays as the standing warning, and a rejection
from Jules because the branch isn't found is handled like any other
server-side dispatch failure (§3.4).

### 3.3 Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Clicks **Dispatch to Jules** | Dialog opens; focus moves to the first interactive control (checkbox if shown, else Branch field). Background page is inert (`aria-hidden`/focus-trapped). |
| 2 | (First use of this repo) Reads the egress line, checks the box | `Dispatch` becomes enabled once branch + prompt are also filled — the checkbox alone doesn't unlock it. |
| 3 | Adjusts the prefilled branch/prompt if needed | Live client-side validation only (non-empty); no server round-trip until submit. |
| 4 | Clicks **Dispatch** | Button enters a busy state ("Dispatching…", disabled to prevent double-submit); on success the dialog closes, focus returns to the button that opened it, and the new session row appears in `SessionsSection` with `JulesStatusBadge` phase `queued`. |
| 5 | Presses `Esc` or clicks **Cancel** at any point | Dialog closes with no side effect; focus returns to the opening button. Nothing was dispatched, nothing was persisted (the egress checkbox state is not saved unless Dispatch actually succeeds). |

### 3.4 Error / edge-case handling

| Trigger | What the user sees | Exit path |
|---|---|---|
| Empty branch on submit attempt | `Dispatch` stays disabled; helper text under the field is the standing guidance, not a delayed error toast. | Fill the field — no dead end, nothing was submitted. |
| Server rejects: prefilled/typed branch was never pushed (or doesn't exist on GitHub) | Inline banner naming the branch: "Jules couldn't find `<branch>` on GitHub. Push it, then try again." Dialog stays open, input intact. | Push the branch elsewhere, then retry in place — same recoverable pattern as the other server-rejection rows below. |
| Server rejects: concurrency cap reached | Inline banner inside the dialog: "2 Jules sessions are already running (limit 2). Wait for one to finish or raise the limit in Settings." with a link to `/settings/jules`. Dialog stays open with the user's input intact. | Wait, or follow the settings link — prompt/branch aren't lost either way. |
| Server rejects: daily cap reached | Same banner pattern: "15 Jules sessions were started in the last 24 hours (daily limit 15)." | Same — link to Settings, input preserved. |
| Server rejects: repo not registered with Jules | Inline banner: "tstapler/stapler-squad is not connected to Jules. Connect it at jules.google.com, then try again." with outbound link. | Link opens jules.google.com in a new tab; dialog stays open so the user can retry after connecting. |
| Server rejects: Jules disabled/key missing (race — toggled off in another tab) | Banner: "Jules is not configured. Add an API key in Settings." + Settings link. | Dialog stays open; no silent failure. |
| Network/transient failure | Banner: "Couldn't reach Jules. Try again." `Dispatch` re-enables immediately (not locked out). | Retry in place. |
| Double-click / rapid double-submit | Second click is a no-op while the first is in flight (button disabled); server-side reservation guard is the real backstop (plan.md Story 2.2.1), so even a race lands on exactly one session. | N/A — invisible to the user by design. |

---

## 4. Surface C — `JulesStatusBadge` in `SessionsSection`

### 4.1 Wireframe — row states

```
Queued:
  ☁ Jules: Queued           ⋯ View this session on jules.google.com

Running:
  ☁ Jules: Running          ⋯ View this session on jules.google.com

Running, poll stale (poller hiccup, not a task failure):
  ☁ Jules: Running          Last updated 8m ago, retrying…
                             ⋯ View this session on jules.google.com

Needs review (PR opened, still shown briefly before the row ends):
  ☁ Jules: Needs Review     ⋯ View this session on jules.google.com

Done:
  ☁ Jules: Done             ⋯ View this session on jules.google.com

Failed:
  ☁ Jules: Failed           ⋯ View this session on jules.google.com
  (red variant; row does NOT get the generic "ended"/orphan
   treatment used for leaked local sessions)

Reconnect required (key revoked/expired mid-session):
  ☁ Jules: Reconnect required   [ Update key ]
  (distinct copy from "Failed" — the fix is re-entering a key,
   not investigating a task failure. Account-wide, not
   session-specific: a 401/403 during any poll tick sets this
   for every open Jules session at once, and it clears itself,
   for all of them, the next time any poll succeeds — see below.)
```

Row layout replaces the branch-badge + `SessionMonitor` a local session
row shows — there is no PTY, so nothing tries to imply one:

```
┌ Linked sessions ─────────────────────────────────────────────────┐
│ role="list"                                                       │
│  ┌ role="listitem" ───────────────────────────────────────────┐  │
│  │ ☁ Jules: Running   Last updated 8m ago, retrying…            │  │
│  │ View this session on jules.google.com ↗                      │  │
│  └───────────────────────────────────────────────────────────┘  │
│  ┌ role="listitem" ───────────────────────────────────────────┐  │
│  │ 🖥 work   backlog/other-item   2h ago         [Steer ▾]      │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Opens the item detail page while a Jules session is open | `SessionsSection` renders the row with whatever phase is already known from the loaded item sessions; if no state is known yet, the badge renders nothing (never a placeholder "Queued" flash) until the first real value arrives. |
| 2 | Leaves the tab open | Badge updates via the existing ConnectRPC query-invalidation-on-refresh pattern (no component-local polling loop). A `Queued → Running` or `Running → Needs Review` transition is announced through a shared `aria-live="polite"` region without requiring focus on the row. |
| 3 | Clicks **View this session on jules.google.com** | Opens Jules' own session page in a new tab — the escape hatch to deeper diagnostics stapler-squad doesn't try to replicate. |
| 4 | Session reaches `Failed` | Badge flips to the red `Failed` variant; the transition fires through a `role="alert"` region (assertive) — the one case that should interrupt, unlike routine progress ticks. A progress note with Jules' own failure text appears in the timeline (Surface E) same tick. |
| 5 | Poll hiccups (API unreachable/rate-limited) | Badge keeps the **last known** phase and adds "Last updated Nm ago, retrying…" — it must never flip to `Failed` for a poller problem; that would misinform the user into investigating a task failure that didn't happen. |
| 6 | Jules rejects a poll with 401/403 (key revoked/expired at Google's end, mid-session) | Every open Jules session's badge — across every item, not just this one — flips to `☁ Jules: Reconnect required` with the `Update key` link; the underlying session is **not** ended and **not** touched as a task failure, since the credential, not the work, is the problem. |
| 7 | User updates the key in Settings | No separate "Retry" click is needed. The very next poll tick that succeeds (for any open session) clears the account-wide condition automatically; every affected badge reverts to its own normally-computed phase (`Running`/`Queued`/etc.) on that same refresh. |

### 4.3 Error / edge-case handling

| Trigger | What the user sees | Exit path |
|---|---|---|
| Poll stale (>1 missed tick) | Secondary text "Last updated Nm ago, retrying…" next to the unchanged phase label. | Self-resolves once polling recovers; no action required, nothing to click. |
| Jules session failed | Red `Jules: Failed` badge + timeline note with Jules' verbatim message + link to jules.google.com. | `View this session on jules.google.com` for deeper diagnostics; item itself returns to `ready` so the user can re-dispatch or hand it to a local agent instead. |
| Key revoked/expired mid-session | `Jules: Reconnect required` badge (amber, distinct from red `Failed`) + inline `Update key` link straight to `/settings/jules`, on every open Jules session at once (account-wide condition, plan.md Story 2.3.4). | One click to the fix (Settings → paste new key → Save). No separate "retry" action after that: the badge(s) clear themselves automatically on the next successful poll tick — this is explicitly not folded into generic "Failed" per research/ux.md §4.1, both because the remedy differs and because it self-resolves without a dead-end retry click. |
| Session vanished from Jules' side (404) | Row ends; badge shows `Jules: Failed` with note text "This session is no longer visible in Jules." rather than hanging in `Running` forever. | Item returns to `ready`; re-dispatch is available again. |
| Session exceeded max age (24h, still open) | Same failed-with-note treatment, note text explains the timeout explicitly rather than looking identical to a Jules-side failure. | Same — item returns to `ready`. |
| PR opened but stapler-squad couldn't attach it | Badge stays `Needs Review`/`Done` as appropriate; a **separate** secondary notice (not a full failure) appears near the PR section: "Jules opened a PR but stapler-squad couldn't link it automatically. Check the session on jules.google.com for the PR link." | Manual link to jules.google.com; underlying work is not lost or hidden. |

---

## 5. Condensed surfaces

### 5.1 Surface D — PR provenance marker

Small `Jules` text/icon marker rendered beside the existing `GitHubBadge` in
`PullRequestSection.tsx` when the item's most recent session role is
`jules_work`. No new PR component — `GitHubBadge` itself is unmodified.

```
┌ Pull Request ──────────────────────────────────┐
│  [PR #700 · Ready]  ☁ via Jules                 │
└──────────────────────────────────────────────────┘
```

**Acceptance criteria**:
- Marker appears only when the PR-producing session's role is `jules_work`; absent for every other agent.
- Marker never replaces or restyles `GitHubBadge` itself — same badge a local-agent PR gets.
- Marker text/icon pair meets the same never-color-alone rule as every other badge in this doc.
- Marker has an `aria-label` (e.g. "Opened by Jules") so it's announced, not just visually implied by a cloud icon.

### 5.2 Surface E — Activity/status history

Per plan.md's deviation record, Jules state transitions are written as
ordinary entries to the existing append-only progress-note history
(`Storage.AppendProgressNote`), which `ProgressHistorySection.tsx` already
renders as a timeline — no new frontend component.

```
┌ Progress ──────────────────────────────────────┐
│ 10:02  Jules session is now planning.            │
│ 10:04  Jules session is now in progress.          │
│ 10:41  Jules opened pull request #700.            │
└──────────────────────────────────────────────────┘
```

**Acceptance criteria**:
- Every state *change* (not every poll tick) produces exactly one note — no duplicate-note spam from repeated identical polls.
- Failure notes carry Jules' own message text verbatim, attributed as Jules' text, not stapler-squad's.
- The timeline entry order matches the actual state transition order even if two ticks land close together (poller processes and appends sequentially per session).
- No new component, styling, or interaction pattern — this is a straight reuse; a reviewer should be able to confirm `ProgressHistorySection.tsx` has zero Jules-specific branches.

### 5.3 Surface F — Structured logs

```json
{"time":"2026-09-01T10:04:12Z","level":"INFO","logger":"jules","msg":"jules session state changed","jules_session":"sessions/xyz","from":"QUEUED","to":"PLANNING"}
{"time":"2026-09-01T10:41:03Z","level":"WARN","logger":"jules","msg":"jules poll failed","jules_session":"sessions/xyz","status_code":503,"error":"transient upstream error"}
```

**Acceptance criteria**:
- Every log line named in plan.md's Observability Plan is emitted at the documented level, under the `jules` logger.
- No line — at any level — ever contains the API key, the `x-goog-api-key` header value, or a raw request body.
- `jules poll failed` and `jules unknown session state` are greppable in isolation from routine `Info` traffic (distinct `msg` values, per `docs/how-to/debug-with-logs.md` conventions) — an operator debugging Jules doesn't have to read unrelated session logs.
- A human operator, given only these log lines (no source access), can tell dispatch-rejected-by-guard apart from an actual API failure from the `msg` field alone.

---

## 6. Cross-cutting error-state table

Every failure mode below is one item was already worked into a surface's
own table above; this view exists to check completeness — one row per
distinct failure, with its home surface and its exit path, confirming no
error is an unrecoverable dead end.

| Failure | Home surface | User-visible signal | Exit path |
|---|---|---|---|
| No API key configured | Settings (empty state) + gated `Dispatch to Jules` button (disabled+reason) | "Add a Jules API key in Settings…" | Settings link from the disabled button's description |
| Key invalid/rejected | Settings (save-time inline error) | "Enter a key from jules.google.com/settings." | Retype, re-save |
| Key revoked mid-session | Session row badge (every open session, account-wide) | `Jules: Reconnect required` + `Update key` link | One click to Settings; badge(s) clear automatically on the next successful poll — no separate retry |
| No branch known for the item | Gated button (disabled+reason) | "This item has no branch yet — spawn a local session (or push a branch)…" | Spawn a local session or push a branch, then the button enables itself |
| Repo not connected at jules.google.com | Settings test-connection + dispatch dialog rejection | Names the exact `owner/repo`, links to jules.google.com | Connect there, retest/retry in place |
| Branch not pushed / not found on GitHub | Dispatch dialog rejection | Names the branch, asks the user to push it | Push the branch, retry in place — input preserved |
| Egress not acknowledged | Dispatch dialog | Named-repo confirmation checkbox | Check the box (one-time per repo) |
| Concurrency cap reached | Dispatch dialog rejection | Names the limit and current count | Wait, or raise the cap in Settings |
| Daily cap reached | Dispatch dialog rejection | Names the limit | Wait until the window rolls, or raise the cap |
| Duplicate dispatch (already in flight) | Gated button (disabled+reason) | "A Jules session is already running for this item." | Wait for it to finish, visible via the badge |
| Jules API unreachable/rate-limited (poll) | Session row badge | Stale phase preserved + "Last updated Nm ago, retrying…" | Self-resolves; no click needed |
| Jules session failed | Session row badge + timeline | Red `Failed` + Jules' own message + jules.google.com link | Item returns to `ready`; re-dispatch or hand to a local agent |
| Session vanished (404) | Session row badge + timeline | `Failed` + explanatory note | Item returns to `ready` |
| Session exceeded max age | Session row badge + timeline | `Failed` + timeout-specific note | Item returns to `ready` |
| PR opened but import failed | Secondary notice near PR section | Explains the mismatch, links to jules.google.com | Manual link; underlying PR isn't lost |
| Abandoned reservation (create call never confirmed) | Session row ends silently in backend; surfaced as `Failed` + "check jules.google.com in case a session was created" | Same badge pattern as other failures | User checks jules.google.com directly |

**No row above has an empty Exit path cell** — this is the human check for
requirements.md's Risk Control section and the "no dead ends" acceptance
criterion below.

---

## 7. UX acceptance criteria

Numbered, each testable by a human clicking through the running app (not by
reading source).

1. **Settings round trip**: a user can add a Jules API key, enable the
   feature, and confirm the "Test connection" success message in **≤ 4
   actions** (open Settings → Jules, paste key + Save, toggle Enable, Test
   connection).
2. **Dispatch happy path**: from an item's detail page, a user with Jules
   already configured and the repo already acknowledged can dispatch to
   Jules in **≤ 3 clicks** (Dispatch to Jules → fill/confirm prefilled
   branch+prompt → Dispatch).
3. **First-use egress confirmation cannot be skipped**: for a repo not yet
   in the acknowledged list, the `Dispatch` button in the dialog is
   observably disabled until the named-repo checkbox is checked — verified
   by attempting to submit with the box unchecked and confirming nothing is
   sent.
4. **No color-only signal**: every Jules status badge state (Queued,
   Running, Needs Review, Done, Failed, Reconnect required) is
   distinguishable with color turned off (verify via a grayscale filter or
   a colorblind simulator) — icon shape and text label alone convey state.
5. **Staleness never reads as failure**: with the Jules API artificially
   blocked (e.g. network throttled in devtools), a running session's badge
   keeps its `Running` label and adds the "retrying…" secondary text — it
   must **not** visually match the red `Failed` state at any point during
   the outage.
6. **Failure is announced, not just displayed**: with a screen reader
   running (VoiceOver/NVDA), a session transitioning to `Failed` produces
   an audible interruption (assertive announcement); a routine
   `Queued → Running` transition does not interrupt whatever the screen
   reader was doing (polite only).
7. **Every error state has a visible exit path**: for each row in §6's
   table, a human can locate a clickable/actionable next step within the
   same view — no error state where the only recovery is reloading the page
   or reading source code.
8. **Focus discipline in the dispatch dialog**: opening the dialog moves
   focus into it; `Tab` from the last control wraps to the first; `Esc` or
   Cancel returns focus exactly to the `Dispatch to Jules` button that
   opened it (matches the existing modal convention verified elsewhere in
   this repo, e.g. `BacklogItemDetail.focusReturn.test.tsx`).
9. **Keyboard-only completion**: the entire dispatch flow (open dialog →
   check confirmation → fill branch/prompt → submit) is completable with
   keyboard alone, no mouse.
10. **Screen-reader labels present**: every icon-bearing element in this
    doc (badge, escape-hatch link, revoke button, PR provenance marker) has
    a non-empty accessible name distinct from its visual-only `title`
    attribute — verified via the browser's accessibility tree inspector,
    not `title` alone.
11. **Color contrast ≥ 4.5:1**: every new badge/text-on-background pairing
    introduced by this feature (all `JulesStatusBadge` phase variants, in
    both light and dark themes) passes a contrast checker at the
    token-driven colors actually shipped — verified against the rendered
    page, not the design mockup.
12. **No optimistic flash**: on first page load before any Jules state is
    known, no badge — neutral or otherwise — renders; the row is either
    absent or shows nothing until a real phase value arrives (verified by
    throttling the initial data fetch and observing no placeholder chip).
13. **Settings changes are reflected live**: revoking a repo's egress
    acknowledgment in Settings, then immediately opening the dispatch
    dialog for an item in that repo, re-shows the confirmation checkbox
    (no stale client-side cache of "already acknowledged").
14. **API key never appears in the DOM or network response bodies visible
    to the client**: after saving a key, inspecting the settings page's
    rendered DOM and the `GetJulesConfig` response in devtools' network tab
    shows no substring of the key — only `has_api_key: true`.
15. **Branch prefill and no-branch gating**: for an item with a prior local
    session, opening the dispatch dialog shows the Branch field already
    filled with that session's branch (never blank); for an item with no
    prior local session at all, `Dispatch to Jules` is observably disabled
    with the "no branch yet" reason and the dialog cannot be opened.
16. **Reconnect-required clears itself**: with a Jules session left open and
    the configured key artificially invalidated, the badge shows
    `Jules: Reconnect required`; after entering a working key in Settings
    and waiting for one poll interval (no button click beyond saving the
    key), the badge returns to its normal phase without any "Retry" action
    having been clicked.

---

## 8. Accessibility checklist (summary)

- Icon + text label + color for every status indicator — never color alone (Criterion 4).
- `role="status"`/`aria-live="polite"` for routine transitions; `role="alert"` only for failure (Criteria 5, 6).
- Semantic list structure (`role="list"`/`role="listitem"`) for the session row list, matching the existing `SessionsSection` pattern — not visual indentation alone.
- Focus trap + focus return for the dispatch dialog (Criterion 8).
- Full keyboard operability, no mouse-only affordances (Criterion 9).
- `aria-label` distinct from `title` on every icon-bearing element (Criterion 10).
- WCAG AA contrast (4.5:1) on all new token-driven color pairings, checked in both themes (Criterion 11).
- No misleading default/placeholder state before real data arrives (Criterion 12).
