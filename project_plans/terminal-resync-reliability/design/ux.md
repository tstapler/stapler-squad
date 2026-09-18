# UX Design: terminal-resync-reliability

**Phase 3.5 — UX design.** Source: `requirements.md`, `research/ux.md`, `implementation/plan.md`.

## 0. Bottom line

Per `research/ux.md` §0 and §4, this project's UX-correct direction is **"the existing
reconnect banner fires less often, not a new banner/indicator."** This design doc holds that
line: it does not invent a "resyncing…" spinner, a per-terminal status badge, or a queued-state
affordance. Three surfaces get real design treatment because they are genuinely interactive or
their *timing/behavior* changes in a user-perceptible way; everything else is config/log output
and gets a condensed entry.

## 1. Surface inventory

| # | Surface | Type | Why it's in scope |
|---|---|---|---|
| 1 | Terminal reconnect/hard-fail banner (`TerminalOutput.tsx:1719-1728`) | Interactive (Retry button on hard-fail variant) | All 5 fixes change *when* this fires; Epic 8.3 changes its accessibility markup. Full treatment. |
| 2 | Multi-terminal visibility-scoped + staggered resync (no new widget) | Behavioral flow, user-perceptible timing | AC1/AC5 — the core "does switching tabs feel instant" experience the whole project targets. Full treatment even though there's no new chrome. |
| 3 | Feature Flags admin page, 7 new toggles (`web-app/src/app/settings/features/page.tsx`) | Interactive (operator-facing) | AC7 — 7 new entries appended to `knownFeatureFlags`, rendered on an existing admin page. Full treatment. |
| 4 | Observability log/metric output (console debug lines, structured server logs) | Non-interactive | Condensed entry. |
| 5 | `GetFeatureFlags` RPC JSON payload (backs surface 3, also consumed by non-UI callers) | Non-interactive | Condensed entry. |

Explicitly **not** designed (per research §0/§4 and requirements' "no invented UI" framing):
a per-terminal "resyncing" indicator, a "queued" spinner for staggered terminals, or any new
visible state for correlation-ID matching (item 2) or server capacity fixes (item 3) — these are
purely internal reliability wins with no distinct visible state of their own.

---

## 2. Surface 1 — Terminal reconnect/hard-fail banner

### Wireframe

```
┌─ Terminal pane (xterm.js) ──────────────────────────────────┐
│                                                                │
│                 ╭─────────────────────────╮                   │  <- state B: soft banner
│                 │   Reconnecting terminal…  │                  │     (pill, semi-transparent
│                 ╰─────────────────────────╯                   │      black bg, muted text,
│                                                                │      pointer-events:none)
│  $ npm run dev                                                │
│  > Compiling...                                               │
│  ...                                                          │
└────────────────────────────────────────────────────────────┘

┌─ Terminal pane (xterm.js) ──────────────────────────────────┐
│                                                                │
│      ╭───────────────────────────────────────────╮            │  <- state C: hard-fail banner
│      │  Connection lost — [ Retry ]               │            │     (solid error-color pill,
│      ╰───────────────────────────────────────────╯            │      inverse text, real button)
│                                                                │
│  $ npm run dev                                                │
└────────────────────────────────────────────────────────────┘
```

State A (not pictured) is simply the terminal with no overlay — the target state for every
scoped-out / successfully-correlated / fast-lane resync this project ships.

### State diagram (per-resync-attempt lifecycle)

```
                    resync requested
                          │
                          ▼
                 ┌──────────────────┐
         ┌───────│  A: no banner    │  <-- stays here if resync completes
         │        └──────────────────┘      before RESYNC_BANNER_DELAY_MS (2000ms)
         │  resolves <2s                    (today's happy path; also the *new* happy
         │  (unchanged)                      path for every terminal Fix 1 scopes out
         ▼                                   entirely — it never even reaches "requested")
   ┌───────────┐
   │ done, A   │
   └───────────┘

                 still pending at 2000ms
                          │
                          ▼
                 ┌──────────────────┐
                 │  B: soft banner   │──── resolves before 4000ms ───► done, back to A
                 │ "Reconnecting     │      (banner clears, no scrollback marker —
                 │  terminal…"       │       this is the fast-lane/skip-slow-path win:
                 └──────────────────┘       AC3/AC4 pull more attempts out of this
                          │                  zone entirely)
                 still pending at 4000ms
                 (RESYNC_STALL_TIMEOUT_MS)
                          │
                          ▼
                 ┌──────────────────┐
                 │ forced disconnect │
                 │ + reconnect       │
                 └──────────────────┘
                          │
                          ▼
              scrollback: "--- reconnected ---"
                          │
                 reconnect succeeds?  ──No──►  ┌──────────────────────┐
                          │                    │ C: hard-fail banner   │
                         Yes                   │ "Connection lost —    │
                          │                    │  Retry"               │
                          ▼                    └──────────────────────┘
                     done, A                              │
                                                  user clicks Retry
                                                           │
                                                           ▼
                                                  back to top (resync requested)
```

**What this project changes about the diagram, not the diagram's shape:** Fixes 1/4 (visibility
scoping + stagger) reduce how often a resync attempt is even created for a background terminal.
Fixes 2/3 (correlation ID + skip-slow-path/fast-lane) reduce how often an attempt that *is*
created takes long enough to cross the 2000ms/4000ms thresholds. No new box, arrow, or banner
copy is added to this diagram — every fix pushes traffic toward "done, A" faster, or removes the
attempt before it starts.

### Interaction flow

1. User backgrounds the browser tab (switches to another app/tab) with N sessions' terminals
   mounted underneath.
2. User returns focus to the tab (or to a specific session's terminal tab within it).
3. **Today:** every mounted `TerminalOutput` fires a resync burst. **After this project (flag
   on):** only the terminal(s) whose `isVisible` prop is currently true fire a resync; the rest
   do nothing (state A, forever, for this event — no request is ever made, so no banner path is
   possible).
4. If the visible terminal's resync resolves in <2s (the common case, and more common than today
   once Fixes 2/3/4 land): no visible change at all. This is the target experience — "switching
   tabs among several live sessions is indistinguishable from a single-session product" (research
   §2, "emotional read of the fix succeeding").
5. If it's still pending at 2s: the existing "Reconnecting terminal…" pill appears
   (`aria-live="polite"`, `role="status"` — new in this project, see Epic 8.3). No user action
   required or possible; `pointer-events: none` means the terminal underneath stays interactive.
6. If it clears before 4s: pill disappears silently, back to state A. No scrollback marker (that
   only appears after an *actual* forced disconnect+reconnect, not a slow-but-successful resync).
7. If it's still pending at 4s: the stall watchdog fires — same forced disconnect+reconnect as
   today, `--- reconnected ---` written to scrollback once the reconnect completes.
8. If the reconnect itself fails: hard-fail banner (**decision: `role="alert"`**, resolved — see
   Accessibility below), `Connection lost — Retry`. User clicks
   **Retry** → `handleHookReconnect` re-runs the connect flow → success returns to state A,
   failure re-shows the same hard-fail banner (no dead end — the exit path is the same button,
   available indefinitely).

### Error / edge cases

| Case | What the user sees | Exit path |
|---|---|---|
| Background terminal genuinely stalls despite all 5 fixes (e.g. real network issue, not a resync-burst artifact) | Same as today: soft banner at 2s → forced reconnect at 4s → `--- reconnected ---` marker | Automatic; no user action needed unless reconnect itself fails |
| Exec-gate fast lane exhausted (4 slots, Fix 3b) under an unusually large burst | Resync queues for a fast-lane slot instead of the default 8-slot pool; if it still can't get one before 4s, falls through to the same soft→hard-fail path as any other stall | Retry button (hard-fail state) |
| Dimension-mismatch skip heuristic wrong (a terminal was genuinely resized while backgrounded, not just stale) | `stale_dimensions=true` incorrectly skips the slow path → server captures pane at wrong dimensions → next real resize/fit event (one of the 3 protected triggers, unmodified by this project) corrects it on next interaction | Self-heals via existing resize-triggered resync; worst case is a single frame of stale-looking content, not a stuck state |
| Correlation ID never arrives (client not yet upgraded mid-rollout, or dropped frame) | Falls back to today's "any output clears pending" heuristic (Risk Control in plan.md) — behaves exactly like pre-project code, including its existing false-positive-completion behavior | No new exit path needed — this is the existing behavior, unchanged |
| Feature flags all off (default / rollback) | Byte-for-byte identical to today's behavior across every row above | N/A — this *is* the rollback path |
| User clicks Retry repeatedly while genuinely offline | Hard-fail banner persists, Retry remains clickable every time | Retry button never disables/hides itself — no dead end |

### Accessibility

- **New in this project (Epic 8.3, drive-by fix flagged in `research/ux.md` §3/§6):** both
  `reconnectingBanner` and `hardFailedBanner` divs currently have neither `role` nor `aria-live`
  (verified by reading `TerminalOutput.tsx:1719-1728`).
  - **`reconnectingBanner`** (soft, recoverable, self-clearing state): `role="status"
    aria-live="polite"`, mirroring the existing `resizingOverlay` pattern (`role="status"
    aria-label="Terminal resizing"`, `TerminalOutput.tsx:1747-1755`) and the
    `pausedOverlay`/crashed-overlay pattern (`role="status" aria-live="polite"`,
    `SessionDetailView.tsx`).
  - **`hardFailedBanner`** (**decision, resolving the open question this doc previously left
    unpicked): `role="alert"`**, not `role="status"`. Rationale: this state is an unprompted
    interruption — the terminal is disconnected and stays that way until the user acts — so it
    needs an assertive announcement the instant it appears, which is exactly what `role="alert"`
    provides (implicit `aria-live="assertive"`, no separate `aria-live` attribute needed).
    `role="status"`'s polite queuing is for transient/recoverable states like `reconnectingBanner`,
    where interrupting the user's current screen-reader output isn't warranted since the state
    usually self-clears in under a couple seconds.
  - Both satisfy WCAG 4.1.3 (Status Messages, AA) without moving focus — `role="alert"` announces
    but does not steal focus, so the Focus bullet below still holds for both banners.
- **Keyboard**: the hard-fail banner's `Retry` button must be reachable via normal tab order and
  activatable with Enter/Space — it's a plain `<button>` today, so this is expected to already
  hold, but that's an assumption, not a verified fact. **Action**: added as Task 8.3.1.3 in
  `implementation/plan.md` — an owned, scheduled check (not left implicit) that tabs through the
  terminal container with `hardFailedBanner` rendered and asserts focus reaches `Retry` via normal
  tab order with no intervening `tabIndex={-1}` skip, activatable with Enter/Space; covered by an
  explicit assertion in `TerminalOutput.test.tsx` rather than relying on manual spot-check.
- **Focus**: neither banner may steal focus when it appears — the user may be actively typing in a
  different, currently-focused terminal while a background one's banner shows (this can only
  happen pre-Fix-1/flag-off, or for a genuinely-visible terminal that's stalling; post-Fix-1 a
  background terminal never gets far enough to show a banner at all).
- **Color contrast**: resolved against the actual token values in
  `web-app/src/styles/theme.css.ts` — `textMuted` `#6b6b6b` (light, `:68`) / `#8a8a8a` (dark,
  `:175`); `textInverse` `#ffffff` (light, `:71`) / `#0a0a0a` (dark, `:178`); `error` `#ef4444` in
  both themes (`:103`, `:210`). Computed ratios: **`hardFailedBanner` (`textInverse` on `error`)
  is ~3.76:1 in light theme — fails WCAG AA's 4.5:1 normal-text threshold** (it clears only the
  3:1 large-text/UI-component threshold); dark theme is ~5.26:1 (passes).
  `reconnectingBanner` (`textMuted` on `rgba(0,0,0,0.7)`) is harder to pin down without a live
  render — depending on what's actually composited underneath, it estimates to roughly 1.6-3.9:1
  in light theme (**likely fails**) and ~6:1 in dark theme (passes). **This doc's earlier assertion
  that both banners meet ≥4.5:1 was unverified and is now known to be wrong for light theme** —
  don't treat 4.5:1 as satisfied by construction.

  **Decision (resolving the open either/or)**: neither banner's text qualifies for the WCAG 1.4.3
  large-text exception (18pt/24px regular or 14pt/18.66px bold) — both render at `0.8125rem`
  (13px) regular weight (`TerminalOutput.css.ts:499,519`), well under that threshold — so a
  large-text justification isn't available here and isn't being claimed. The fix is a token swap
  in both cases, following patterns already in use elsewhere in the codebase:
  - **`hardFailedBanner`**: swap `background: vars.color.error` → `vars.color.errorDark`
    (`theme.css.ts:106`, `#b91c1c` in light theme vs. `error`'s `#ef4444`), keeping
    `color: vars.color.textInverse` unchanged. `errorDark` + `textInverse` is already an
    established pairing in this codebase (e.g. `AliasesManager.css.ts`'s `confirmDeleteBtn`).
    Computed: white (`#ffffff`) on `#b91c1c` is **~6.47:1 in light theme** (passes AA normal-text
    4.5:1 with margin); dark theme's `errorDark` already equals `error` (`#ef4444`), so its
    existing ~5.26:1 is unchanged.
  - **`reconnectingBanner`**: swap `background: "rgba(0, 0, 0, 0.7)"` → `vars.color.modalBackground`
    and `color: vars.color.textMuted` → `vars.color.textPrimary` — the same surface/text pairing
    `Modal.css.ts` already uses everywhere else in the app, so it inherits that pairing's
    already-verified per-theme contrast instead of relying on a hardcoded, theme-blind rgba
    backdrop (`textInverse` was considered and rejected here: it inverts *with* the theme, e.g.
    `#0a0a0a` in dark theme, which would fail against a fixed dark backdrop in dark theme even
    though it fixes light theme — `modalBackground`/`textPrimary` is the pairing that holds in
    both themes, at the cost of losing the previous translucency). **Action**: implemented as Task
    8.3.1.2 in `implementation/plan.md`, with the exact token swaps above pinned in the task so
    the implementer isn't re-deciding this; still requires a browser contrast-checker
    spot-check against the real rendered pill before ship (analytical computation isn't a
    substitute for verifying the actual composited pixels).
  - **Automated backstop**: this is a `web-app/src/` change, so it runs through this repo's
    existing PR-triggered Axe-Core CI gate (`CLAUDE.md`'s "UX analysis CI" section) automatically —
    Axe Core blocks the PR on WCAG AA violations, so a regression in either banner's contrast (or
    the same mistake recurring in a different banner) is caught even if the manual spot-check is
    skipped or mis-measured. The manual check above is a pre-merge sanity check; Axe-Core is the
    enforced gate.

---

## 3. Surface 2 — Multi-terminal visibility-scoped + staggered resync (behavioral, no new widget)

This surface has no new component. What changes is *when and how many* resync requests fire
relative to what the user is looking at, and that timing is itself the UX artifact this project
is accountable for — research §2 calls the stagger-vs-preemption behavior a genuine risk worth
designing deliberately, not leaving to implementation default.

### Before/after timeline — N terminals, one tab-focus event

```
BEFORE (today — the bug):
  t=0ms   tab regains focus (document.visibilitychange)
          │
          ├─► Terminal A (visible)     ─┐
          ├─► Terminal B (background)  ─┼─ all fire requestFullResync() on the SAME tick
          ├─► Terminal C (background)  ─┘
          │
  all 3 contend for the same 8-slot "default" exec-gate + the ~450ms slow path
  → B or C (NOT the one the user is looking at) is the one most likely to sit
    queued past 4000ms and get force-disconnected. This is the bug's exact
    signature per requirements.md's Baseline: "it's usually not the one I'm
    typing in that runs into trouble."

AFTER (Fix 1 + Fix 4, flags on):
  t=0ms   tab regains focus
          │
          ├─► Terminal A (isVisible=true)   ─── resync fires immediately
          ├─  Terminal B (isVisible=false)  ─── NO resync fires — scoped out entirely
          └─  Terminal C (isVisible=false)  ─── NO resync fires — scoped out entirely

  Only 1 request on the wire instead of 3. B and C simply show correct content
  whenever the user actually switches to them (their own switch-to event sets
  isVisible=true and fires their own resync then — same as A did just now).
```

### Timeline — several terminals genuinely visible together in a short window

("Several visible in the same burst" per plan.md Epic 6.1 means: multiple session/shell tabs
flip `isVisible=true` in quick succession — e.g. the user alt-tabs through several browser tabs
each showing a different stapler-squad session within the debounce window — not a simultaneous
split-screen of multiple panes in one view, since `SessionDetailView.tsx` shows exactly one
`isVisible=true` pane per tab-group at a time (`visibility: hidden` on the rest,
`TerminalOutput.tsx` pool pattern).)

```
t=0ms     Terminal A becomes visible  ──► stagger queue: [A]           ──► A fires at t=0ms (front of queue)
t=40ms    Terminal B becomes visible  ──► stagger queue: [B]           ──► B fires at t=~100ms (jittered slot)
t=90ms    Terminal C becomes visible  ──► stagger queue: [C]           ──► C fires at t=~250ms (jittered slot)

Preemption case — user immediately switches away from the queue order:

t=0ms     Terminal A becomes visible  ──► queue: [A]                  ──► A fires t=0ms
t=40ms    Terminal B becomes visible  ──► queue: [A done, B]          ──► B queued for t=~150ms
t=60ms    user clicks Terminal D's tab (D becomes visible NOW)
                                       ──► D PREEMPTS: moves to front of queue,
                                           fires immediately (t=60ms) — does NOT
                                           wait behind B's still-queued slot
t=150ms                                   B fires on its original schedule
```

**Why preemption matters (research §2):** if D — the terminal the user is *actively looking at
right now* — were left waiting behind B's stagger slot just because B claimed a slot first, the
user would see stale content in D for that gap and read it as broken, which is exactly the
corruption perception the original visibility-resync feature (PR #184) was built to eliminate.
"Newly-visible always preempts merely-queued-while-still-backgrounded" is therefore a **hard
requirement of this surface's design**, not an implementation nicety — it's already reflected in
plan.md Task 6.1.1.3, and this doc confirms it as the UX-load-bearing behavior.

### Interaction flow

1. User has 3+ sessions open (each keeps its terminals mounted for keep-alive).
2. User switches between session tabs / browser tabs in quick succession (a realistic pattern —
   "checking in" on several running agents).
3. Each switch sets that terminal's `isVisible=true` and fires (or queues) its resync; the
   previously-visible terminal's `isVisible` flips to `false` and it does **not** re-fire on its
   own account (only a genuine focus/visibility event triggers a resync, not an `isVisible`
   transition to `false`).
4. If the user lands on a terminal and stays there for >2s while its resync is still resolving,
   they see Surface 1's soft banner — same rules as any single-terminal resync.
5. If the user is fast enough that they land on terminal D before B's stagger slot fires, D's
   resync preempts and fires immediately (see diagram above) — the user never perceives a delay
   tied to "how many other terminals I've recently glanced at."

### Error / edge cases

| Case | Behavior | Rationale |
|---|---|---|
| Stagger delay pushes even the *currently-visible* terminal past the 2s banner threshold | Falls into Surface 1's existing soft-banner path — **not** a new "queued" indicator (research §4: "if in practice staggering makes hitting 2s common... that's a signal the stagger delay is too aggressive relative to `RESYNC_BANNER_DELAY_MS`, and the two should be tuned together") | Keeps the visible-state model at exactly 3 states (none/soft/hard-fail); a 4th "queued" state would add accessibility + E2E test-locator surface (`.claude/rules/e2e-test-conventions.md`) for a case the existing model already covers once timings are tuned |
| User rapidly cycles through more terminals than the stagger queue can drain before the next switch | Each new visible terminal keeps preempting; older queued (never-fired) entries are simply superseded/dropped — a terminal only needs a fresh resync once it's actually looked at, so a stale queued entry for a terminal the user glanced at and left is not worth firing | Avoids a resync storm on rapid tab-cycling, which is the same failure mode this whole project fixes, just client-side-triggered instead of visibility-event-triggered |
| Feature flag `terminal:resync-stagger` off | No queue at all — every newly-visible terminal fires immediately (same as the "AFTER (Fix 1 + Fix 4)" diagram's single-terminal case, just without the jitter) | This is the intermediate rollout state between Fix 1 alone and Fix 1+4 together |

### Manual QA checklist — numeric thresholds (pre-ship, human judgment call)

`RESYNC_BANNER_DELAY_MS` (2000ms), `RESYNC_STALL_TIMEOUT_MS` (4000ms), and the stagger jitter
window (0-300ms) were all chosen by design reasoning in this doc and `research/ux.md`, not
validated with real users. Before `terminal:resync-stagger` ships default-on, whoever implements
Epic 6.1 should manually walk through these on a real machine (not just assert the numeric
constants in unit tests):
- Does the 2s-pending soft banner feel "fast enough to not worry" when triggered by the stagger
  queue rather than a genuinely slow resync?
- Is the 0-300ms jitter window perceptible as a delay when switching between 2-4 terminals in
  quick succession, versus today's un-staggered immediate-fire behavior?
- With staggering on, does a burst of 4+ simultaneously-visible terminals now commonly push the
  *currently-focused* terminal's own resync past the 2s banner threshold (the first row of the
  table above) — if so, `RESYNC_BANNER_DELAY_MS` and the jitter window need to be tuned together,
  not shipped as independently-chosen constants.

**Decision rule if any answer is "no" / the check surfaces a problem**: do not ship
`terminal:resync-stagger` default-on. Concretely:
- If the 2s banner feels premature/anxiety-inducing under stagger, or the jitter window is
  perceptible as lag, or a 4+ burst commonly pushes the focused terminal past 2s: tune
  `RESYNC_BANNER_DELAY_MS` and/or the jitter window together (per the third bullet's own
  rationale) and re-run this checklist before re-attempting default-on — this is a config/constant
  change, not a design change, so it doesn't require a new design doc revision.
- If re-tuning doesn't resolve it (the stagger approach itself feels wrong, not just its
  constants): block the PR/rollout for `terminal:resync-stagger` defaulting on, keep it
  off-by-default and file a follow-up task to revisit Epic 6.1's design, and ship the rest of this
  project's flags independently (they're per-fix flags per Risk Control, so this doesn't block
  Fixes 1/2/3/5).
- Either way, the checklist runner records which bullet failed and the resulting action (retune
  vs. block) in the Task 6.1.1.7 PR/commit — "checked, passed" or "checked, retuned to X" or
  "checked, blocked — see follow-up #Y," not a silent pass/fail with no trail.

This is a pre-ship checklist item, not a new epic or a formal usability study.

---

## 4. Surface 3 — Feature Flags admin page, 7 new resync toggles

### Wireframe

```
┌─ /settings/features ─────────────────────────────────────────────┐
│  Feature Flags                                                     │
│  Toggle experimental or optional features. Changes take effect     │
│  immediately — no restart needed.                                  │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ Backlog                                          [ On  ●–] │   │
│  └────────────────────────────────────────────────────────────┘   │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ Terminal resync: scope to visible terminal        [Off –●] │   │
│  │ Only resync the terminal you're actively looking at        │   │
│  │ instead of every mounted one. Applies to newly-focused      │   │
│  │ terminals only — already-open tabs need a reload.           │   │
│  └────────────────────────────────────────────────────────────┘   │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ Terminal resync: correlation IDs                   [Off –●] │   │
│  │ ...                                                          │   │
│  └────────────────────────────────────────────────────────────┘   │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ Terminal resync: skip slow path for backgrounded    [Off –●] │  │
│  │ terminals                                                    │   │
│  └────────────────────────────────────────────────────────────┘   │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ Terminal resync: exec-gate fast lane               [Off –●] │   │
│  └────────────────────────────────────────────────────────────┘   │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ Terminal resync: stagger bursts                    [Off –●] │   │
│  └────────────────────────────────────────────────────────────┘   │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ Terminal resync: wire compression                   [Off –●] │  │
│  └────────────────────────────────────────────────────────────┘   │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ Terminal resync: batch requests                    [Off –●] │   │
│  └────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────┘
```

### `FEATURE_META` labels (plan.md Task 1.2.1.5)

The page renders a human label via `FEATURE_META[name].label` (`page.tsx:24-28`); if a flag has
no `FEATURE_META` entry, `label` falls back to the **raw flag name** (`meta?.label ?? name`).
Plan.md's Task 1.2.1.5 adds `FEATURE_META` label entries for all 7 new `terminal:resync-*` flags
introduced by Tasks 1.2.1.1–1.2.1.3, mirroring existing entries' shape (e.g.
`"terminal:resync-visibility-scope": { label: "Terminal: visibility-scoped resync" }`), so this
gap is closed — the wireframe above reflects the labeled end state, not a pending fix.

### Interaction flow

1. Operator (not an end user in the typical sense — this page is reached via app settings, not
   surfaced to the general session-management UI) navigates to Settings → Feature Flags.
2. Page loads flag list via `useFeatureFlags()`; loading state shows "Loading…" text
   (`flagDescription` style, existing).
3. Operator locates one of the 7 new `terminal:resync-*` rows (readable label per the gap above),
   reads its description (states the "not live-updated on already-open tabs" caveat per
   plan.md's Migration Plan where relevant).
4. Operator clicks the toggle (`role`-less `<button aria-pressed={enabled}>` today — already
   keyboard-accessible via native button semantics and `aria-pressed`).
5. `setFlag(name, !enabled)` fires; on success the badge flips `Off`→`On` and the thumb slides;
   on failure the existing `errorMessage` (`role="alert"`) appears above the list.
6. Operator is reminded (via the persistent description text, not a toast) that already-open
   browser tabs won't see the new value until reloaded — this is a one-time-fetch limitation
   (`FeatureFlagsContext.tsx`), documented as an accepted trade-off in plan.md's Migration Plan.

### Error / edge cases

| Case | What the operator sees | Exit path |
|---|---|---|
| `setFlag` RPC fails (network/server error) | `errorMessage` region (`role="alert"`) shows `{error}. Please refresh.` above the flag list; the toggle itself does not silently flip | Refresh the page (explicit instruction in the error text) |
| Flag list fails to load at all | Same `errorMessage` region; list area shows nothing below it | Refresh the page |
| No flags configured (empty registry) | `emptyMessage` text: "No feature flags configured." | N/A — not reachable once Task 1.2.1.1-3 land (registry will have 10 entries) |
| Operator toggles a flag, then immediately reloads their *own* terminal tab in another window | New behavior takes effect on that reload (flags are fetched fresh on mount) — consistent, no stale-state surprise for a tab that reloads | N/A |

---

## 5. Condensed entries (non-interactive surfaces)

### 5.1 Observability log/metric output

Representative sample (console debug line, client-side, per Observability Plan #5):

```
[resync] session=sess-2 resync_id=8f2a... completed in 340ms (visible=true, staged=false)
```

Representative sample (server structured log, per Observability Plan #2/#3):

```json
{"level":"debug","msg":"resync: skipped stale-dimension slow path","session_id":"sess-2","resync_id":"8f2a...","skipped_slow_path":true,"elapsed_ms":4}
{"level":"debug","msg":"resync exec-gate slot acquired","gate_key":"socket-42#resync","wait_ms":2,"pool":"fast-lane"}
```

Acceptance criteria:

- A resync's success path emits exactly one duration log line per attempt, at debug level, never
  at a level that surfaces to end users (console noise budget: this is a developer/operator
  diagnostic, not user-facing UI).
- The stall-watchdog's existing warning-level console line is unchanged in format (only the *rate*
  at which it fires should drop) — no regression test should need to change its assertion on the
  warning's text.
- Every one of the 5 Observability Requirements bullets in `requirements.md` has at least one
  emission point exercised by an automated test (per plan.md Epic 7.1's own AC8 GWT) — this is a
  test-coverage criterion, not a UI one, but it's what makes the "near-zero stall fires" success
  metric independently verifiable after ship rather than trusted on faith.
- Correlation-ID mismatches log at debug (not silently dropped) so a rollout regression is
  diagnosable from logs alone, without needing to reproduce interactively.

### 5.2 `GetFeatureFlags` RPC JSON payload

Representative sample:

```json
{
  "flags": [
    {
      "name": "terminal:resync-visibility-scope",
      "enabled": false,
      "description": "Only resync the terminal you're actively looking at. Existing open tabs need a reload to pick up a change.",
      "statusDetail": ""
    }
  ]
}
```

Acceptance criteria:

- All 7 new flags default `enabled: false` (per plan.md AC7 GWT) — verified by an integration
  test, not just documentation.
- Every flag's `description` states the "not live-updated on already-open tabs" caveat where
  applicable (per plan.md's Migration Plan), since this payload is the sole source of the caveat
  text surfaced anywhere (there's no separate tooltip or help doc).
- Flag `name` values exactly match the `terminal:resync-*` strings used in code-path gating
  (`terminal:resync-visibility-scope` through `terminal:resync-batching`) — a typo mismatch here
  would silently make a toggle a no-op, which is untestable from the UI alone and must be covered
  by the flag-wiring integration tests in plan.md Epic 8.1.

---

## 6. UX acceptance criteria (consolidated, human-testable)

**Surface 1 — banner:**

1. A human tester backgrounding and re-focusing a browser tab with 3+ mounted terminals sees **no
   banner at all** on the terminal(s) that were not the one they switched to, in ≤ 1 tab-switch
   action (this is the primary success signal — 0 clicks/steps beyond the switch itself).
2. When the soft banner does appear (a genuinely slow resync), it reads "Reconnecting terminal…"
   and disappears on its own with no user action, or escalates to the hard-fail banner — there is
   no state where the soft banner persists indefinitely with no exit.
3. The hard-fail banner always shows "Connection lost — Retry" and the Retry button is always
   clickable — no dead end. Clicking Retry attempts a reconnect immediately (no additional
   confirmation step; 1 click).
4. Both banners are announced by a screen reader without moving focus — `reconnectingBanner` via
   `role="status" aria-live="polite"`, `hardFailedBanner` via `role="alert"` (see Accessibility
   decision in §2) — testable with VoiceOver/NVDA: focus stays in the terminal's input area while
   the banner text is announced.
5. Text contrast for both banner variants is ≥ 4.5:1 against their background in both light and
   dark theme — testable with a contrast-checker against the resolved token values at runtime.
   **Resolved via token swap** (see §2 Accessibility decision): `hardFailedBanner` moves to
   `errorDark`/`textInverse` (~6.47:1 light, ~5.26:1 dark), `reconnectingBanner` moves to
   `modalBackground`/`textPrimary` (inherits `Modal.css.ts`'s already-passing per-theme pairing) —
   implemented in `implementation/plan.md` Task 8.3.1.2, with Axe-Core CI (`CLAUDE.md`'s "UX
   analysis CI" section) as the automated regression backstop for this AC going forward.

**Surface 2 — visibility scoping / stagger:**

6. A human tester rapidly switching between 4+ session tabs perceives **no added latency** on the
   terminal they land on and stay on for >1s, compared to switching among an equal number of
   single-terminal sessions (the "indistinguishable from single-session" bar from research §2).
7. A terminal the tester switches to is never left showing stale content because it was "still
   queued" behind another terminal's stagger slot — testable by instrumented timing: the newly-
   visible terminal's resync request timestamp must be ≤ the timestamp of any resync it preempted.
8. With `terminal:resync-stagger` off, behavior is identical to pre-project (immediate resync per
   newly-visible terminal, no jitter) — a regression test, not just a doc claim.
9. The 2s/4s banner thresholds and the 0-300ms stagger jitter window have been manually
   sanity-checked by a human on real hardware per the §3 "Manual QA checklist" before
   `terminal:resync-stagger` ships default-on — not just asserted as passing in unit tests of the
   numeric constants.

**Surface 3 — feature flags admin page:**

10. An operator can find and toggle any one of the 7 new resync flags in ≤ 2 steps (navigate to
    Settings → Feature Flags, click the toggle) with a human-readable label, not a raw flag key
    (see §4 — satisfied by plan.md Task 1.2.1.5's `FEATURE_META` entries).
11. A failed toggle attempt shows a specific, actionable message (`{error}. Please refresh.`) via
    `role="alert"`, and refreshing is a real, working exit path (not a dead end).
12. Toggling a flag never silently no-ops — either the badge visibly flips within one round trip,
    or an error is shown; there is no third, ambiguous outcome.

**Cross-cutting:**

13. **No dead ends**: every error state identified in §2/§3/§4's tables has a named, working exit
    path (Retry button, page refresh, or automatic self-resolution) — verified row-by-row against
    those tables during test-plan authoring in Phase 4 (validate).
14. **Flag-off parity**: with all 7 flags off, a human tester cannot distinguish this project's
    code from pre-project code in any of the 3 surfaces above — this is the rollback safety bar
    and should be the first manual test run before any flag is flipped on in production.
15. **No new visible states were introduced** beyond what's cataloged in §2's state diagram (A/B/C)
    — a reviewer checklist item for design review: if an implementation PR adds a 4th visible
    state (e.g. a "queued" indicator), it should be flagged as scope creep against this design doc
    unless accompanied by a documented change to this file.

---

## Summary

- **3 interactive surfaces designed in full** (banner state machine + interaction flow + error
  table + accessibility; multi-terminal visibility/stagger timeline + preemption flow + error
  table; feature-flags admin page wireframe + interaction flow + error table, with one gap
  flagged for plan.md: missing `FEATURE_META` labels for the 7 new flags).
- **2 condensed non-interactive entries** (observability log/metric output, `GetFeatureFlags` JSON
  payload), each with a representative sample and acceptance criteria.
- **15 UX acceptance criteria** written across the three interactive surfaces plus 3 cross-cutting
  criteria (no dead ends, flag-off parity, no new visible states beyond this doc's catalog).
- **Zero new UI components proposed** — consistent with `research/ux.md`'s finding that this is a
  reliability fix whose entire correct UX outcome is "the existing banner fires less often."
