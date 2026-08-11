# UX Design: dynamic-rule-reload (frontend surface)

Status: ready for implementation review. Consistent with the design already
locked in `implementation/plan.md` Phase 6 (Story 6.1.1 / Tasks 6.1.1a-d) and
`research/ux.md`'s findings. Two genuine gaps found during this pass are
called out explicitly in §7 rather than silently folded into "the plan is
fine" — everything else here is wireframing/specifying what the plan already
decided.

## 0. Scope confirmation

`research/ux.md` is confirmed still accurate: this is one button in
`ApprovalRulesPanel.tsx`'s `claude-settings` filter tab, one new RPC
(`reloadClaudeSettingsRules`), and reuse of the existing
`showActionToast` / `NotificationToast.tsx` system (`role="alert"
aria-live="polite"`, 5000ms success / 10000ms error auto-dismiss). No new
toast component, no new accessibility primitive. The only additions this
design makes on top of the plan are: (a) an explicit in-flight/disabled
button state and (b) explicit handling for a *thrown* RPC error (network/
timeout), neither of which Task 6.1.1a-c's code sketch currently shows -
see §7.

## 1. Surfaces designed

| # | Surface | New / existing |
|---|---|---|
| S1 | Idle state - hint + "Reload rules" button in `claude-settings` tab | New (button); hint text existing pattern |
| S2 | In-flight state - button while RPC is pending | New |
| S3 | Success toast | Reuses existing toast primitive |
| S4 | Error toast - RPC responds `success: false` (e.g. malformed settings.json) | Reuses existing toast primitive |
| S5 | Error toast - RPC call itself throws (network/timeout) | New handling (gap - see §7.1) |
| S6 | Double-click / rapid re-click guard | New (gap - see §7.2) |
| S7 | Empty state for `claude-settings` tab (pre-existing) | Confirmed unchanged, still correct |

7 surfaces designed, 9 UX acceptance criteria in §8.

---

## 2. S1 - Idle state: hint + button

Location: `ApprovalRulesPanel.tsx`, directly below the source filter tabs,
in the `sourceFilter === "claude-settings"` conditional block - same slot
pattern as the existing `configFileHint` block for the `config` tab
(`ApprovalRulesPanel.tsx:452-457`).

```
┌─ Source filter tabs ──────────────────────────────────────────────┐
│  [ All (12) ] [ User (3) ] [ Config (2) ] [ Seed (4) ] [*Claude Settings (3)*] │
└─────────────────────────────────────────────────────────────────────┘
┌─ claude-settings hint row ─────────────────────────────────────────┐
│  Loaded from ~/.claude/settings.json          [ ↻ Reload rules ]  │
└─────────────────────────────────────────────────────────────────────┘
┌─ Rules table (filtered to claude-settings source) ─────────────────┐
│  Name          Source            Decision   ...                   │
│  allow-git-*   Claude Settings   Allow      ...  (Always on)       │
│  ...                                                                │
└─────────────────────────────────────────────────────────────────────┘
```

**Interaction flow:**
1. User has stapler-squad open, clicks the `Claude Settings` filter tab (or
   is already on it).
2. User sees the hint row with the source file path and the button, sitting
   directly above the rows they're trying to verify - no scrolling to a
   separate header area required.
3. User hand-edits `~/.claude/settings.json` in another window/terminal,
   returns to the tab, clicks **Reload rules**.

**Why here and not the header button row:** per `research/ux.md` §6, this
is the load-bearing affordance (more likely to be used than the auto-toast,
since users editing `settings.json` usually aren't thinking about
stapler-squad and won't have the tab open when fsnotify fires). Placing it
in the already-crowded 6-button header (`Generate/Cancel/Export/Import/
Refresh`) would bury it exactly where the user isn't looking when they need
it - confirmed correct, no change from the plan.

---

## 3. S2 - In-flight state

The plan's Task 6.1.1c sketch (`handleReloadClaudeSettings`) does not show a
loading flag. This design adds one, following the exact convention already
used by the sibling Export button two rows up in the same header
(`{exporting ? "Exporting…" : "Export YAML"}`, `ApprovalRulesPanel.tsx:339`
and `disabled={exporting}` at `:336`):

```
┌─ claude-settings hint row (in-flight) ─────────────────────────────┐
│  Loaded from ~/.claude/settings.json     [ ⟳ Reloading… ] (disabled)│
└─────────────────────────────────────────────────────────────────────┘
```

**Interaction flow:**
1. User clicks **Reload rules**.
2. Button immediately becomes disabled and its label changes to
   `Reloading…` (same text-swap convention as `Exporting…`/`Export YAML`).
   This is the mechanism that also satisfies S6 (double-click guard) -
   a disabled button cannot be clicked again.
3. On promise settle (success, `{success:false}`, or thrown error), the
   button re-enables and its label reverts to `Reload rules`, in a `finally`
   block so it recovers even on a thrown error (see §7.1).

Implementation note (not a new pattern, just applying the existing one):
add a local `const [reloading, setReloading] = useState(false)` in
`ApprovalRulesPanel.tsx` alongside the existing `exporting` state, following
the same shape.

---

## 4. S3 - Success toast

```
┌──────────────────────────────────────────────┐
│  ✓  Reloaded 4 claude-settings rule(s).       │   ← bottom-right stack,
└──────────────────────────────────────────────┘     auto-dismiss 5000ms
```

**Interaction flow:**
1. RPC resolves `{success: true, ruleCount: 4, message: "Reloaded 4
   claude-settings rule(s)."}`.
2. `showActionToast(message, "success", "claude-settings-reload")` fires.
   The dedupe key means a second reload (auto fsnotify firing right after a
   manual click, or a fast second manual click after re-enable) replaces
   this toast rather than stacking a second one.
3. Table below has already re-rendered (via `refresh()` inside the
   `reloadClaudeSettingsRules` callback) by the time the toast appears, so
   the user can see the new/changed rows directly under the toast without
   an extra action.

This matches `research/ux.md` §4's recommendation exactly (reference the
tab context, not a bare "Reloaded!") - no change.

---

## 5. S4 - Error toast (RPC returns `success: false`)

```
┌──────────────────────────────────────────────────────────────┐
│  ✕  Failed to reload Claude settings rules — previous rules   │
│     still active (1 path failed to parse).                    │
└──────────────────────────────────────────────────────────────┘   auto-dismiss 10000ms
```

**Interaction flow:**
1. User hand-edits `settings.json` and introduces a JSON syntax error (e.g.
   trailing comma), then clicks **Reload rules**.
2. RPC resolves `{success: false, message: "Failed to reload Claude
   settings rules — previous rules still active (1 path failed to
   parse)."}`.
3. `showActionToast(message, "error", "claude-settings-reload")` fires -
   10s dismiss, matches the "needs a moment longer to read" rationale in
   `research/ux.md` §5.
4. Table is unchanged (previous valid rule set stays active - backend
   requirement #7). No visual diff in the table itself; the toast is the
   only signal, which is why the message states the safety guarantee
   explicitly rather than a bare "Reload failed."
5. Raw JSON parse detail (line/column) is **not** in the toast body - only
   `console.error` per existing `useApprovalRules.ts:72` convention. This
   keeps the message stable and short regardless of how bad the malformed
   file is.

**Exit path:** the button returns to idle (S1) immediately since it never
left the tab. The user's next action is either to fix the file and retry,
or leave it - no modal, no blocking state, matches "toast not modal" from
`research/ux.md` §4.

---

## 6. S5 - Error toast (RPC call throws - network/timeout)

Not explicitly covered by the plan's code sketch; designed here because the
task brief calls it out directly ("RPC timeout/network error"). Distinct
from S4: the promise **rejects** rather than resolving with
`success: false` - e.g. the server process is unreachable, ConnectRPC
times out, or an unexpected 5xx comes back.

```
┌──────────────────────────────────────────────────────────────┐
│  ✕  Could not reach the server to reload rules. Try again.    │
└──────────────────────────────────────────────────────────────┘   auto-dismiss 10000ms
```

**Interaction flow:**
1. User clicks **Reload rules** while the server is unreachable (network
   blip, service restart mid-request - plausible in this repo given
   `make install-service` restarts).
2. `clientRef.current.reloadClaudeSettingsRules(req)` throws.
3. Handler catches it, logs the raw error via `console.error` (matching the
   existing convention), and shows a generic error toast with the same
   `"claude-settings-reload"` key and error variant/10s dismiss as S4, but
   with network-specific copy since "previous rules still active" would be
   misleading phrasing for a request that never reached the server at all.
4. Button re-enables via the `finally` block from S2 regardless of which
   branch (success / `success:false` / thrown) was taken.

**Exit path:** same as S4 - button returns to idle, user can retry
immediately (button is not left permanently disabled).

---

## 7. Gaps found (not in the current plan/task sketch)

### 7.1 Thrown RPC errors have no handler in Task 6.1.1a/c's code sketch

`Task 6.1.1a`'s callback (`useApprovalRules.ts`) is:
```ts
const resp = await clientRef.current.reloadClaudeSettingsRules(req);
await refresh();
return resp;
```
and `Task 6.1.1c`'s handler reads `resp.message`/`resp.success` with no
`try/catch`. If the RPC promise rejects (network error, timeout, 5xx -
exactly the case the task brief asks this design to cover), there is no
catch block anywhere in the sketch: the promise rejection propagates
unhandled from an `onClick` handler, producing **no toast, no re-enabled
button, and only a console error** - a silent dead end for the "no dead
ends" AC below. This is the same currently-latent gap as `deleteRule`'s
call site (`ApprovalRulesPanel.tsx:602`, `onClick={() => deleteRule(rule.id)}`,
also uncaught) - not a regression this feature introduces, but this
feature's own task description explicitly asks for network/timeout
handling, so it should not ship with the same latent hole.

**Recommendation:** wrap the RPC call in `useApprovalRules.ts`'s
`reloadClaudeSettingsRules` (or in the component's `handleReloadClaudeSettings`)
in `try/catch/finally`: `catch` produces a synthesized
`{success: false, ruleCount: 0, message: "Could not reach the server to
reload rules. Try again."}`-shaped result (or the component catches
directly and calls `showActionToast` itself), `finally` clears the
`reloading` state from §3. This keeps the "one button, one toast" scope
intact - no new component, just a catch block reusing S5's copy.

### 7.2 No double-click guard specified

Task 6.1.1b/c's sketch wires `onClick={handleReloadClaudeSettings}`
directly with no disabled/loading gate, so a user double-clicking (or
clicking again while a slow request is in flight) fires a second concurrent
RPC. Two concurrent reloads racing is exactly the class of bug
`requirements.md` open question #2 flags at the backend level (`RulesService`
mutex serializing reload paths) - the frontend should not make that race
easier to trigger than necessary.

**Recommendation:** the `reloading` state and `disabled={reloading}` from
§3 (S2) close this gap on the frontend side for free - once added, a second
click while `reloading === true` is a no-op because the button is disabled,
not just visually "pending." This does not replace the backend mutex
(server must still be correct against concurrent triggers from fsnotify +
manual click racing), but it removes the easiest way a human would
accidentally trigger the race.

Both gaps are additive to the existing plan (no scope change to backend,
no new UI element beyond what Task 6.1.1b already adds) - just closing
holes in the two states the task brief explicitly asked this design to
cover.

### 7.3 Minor: `retryButton` CSS class carries error semantics

Task 6.1.1b reuses the `retryButton` class (`ApprovalRulesPanel.css.ts:147-155`:
`background: vars.color.errorBg`, `color: vars.color.error`) for the Reload
button. That class is otherwise used exactly once today, for the error
banner's "Retry" action (`ApprovalRulesPanel.tsx:463`). Reusing it for a
routine, non-error "Reload rules" action means the button will render in
error-toned (red-tinted) styling by default, even when nothing is wrong -
sending an "alarm" signal for a routine action, which cuts against
`research/ux.md`'s own §6 guidance that the auto/manual reload should be
"low-key" and not read as alarming.

**Recommendation:** reuse `refreshButton` (`ApprovalRulesPanel.css.ts:41-53`)
instead - it is already neutral-toned (`hoverBackground`/`borderColor`/
`textPrimary`, the same styling as the header's existing `↻` refresh icon
button) and already exists, so this is a zero-new-CSS swap, consistent with
`.claude/rules/css-architecture.md`'s "existing tokens only" rule for small
additions - just pick the neutral existing class instead of the error-toned
existing class.

---

## 8. Empty state confirmation (pre-existing, S7)

```
┌─ claude-settings tab, zero rules ──────────────────────────────────┐
│  Approval rules let you automatically allow or deny tool calls     │
│  from Claude without manual review.                                │
│                                                                      │
│  No rules from your ~/.claude/settings.json file were found.       │
│                                                                      │
│  Loaded from ~/.claude/settings.json          [ ↻ Reload rules ]  │
└──────────────────────────────────────────────────────────────────────┘
```

`ApprovalRulesPanel.tsx:497-499`'s existing empty-state copy ("No rules
from your ~/.claude/settings.json file were found.") remains correct once
the reload button ships, and in fact becomes *more* useful: previously a
user seeing this message had no in-app way to confirm whether it meant
"you have no allow rules in that file" vs. "the app hasn't picked up your
file yet" (since, per `research/ux.md` §1, the generic refresh button
silently didn't re-parse `settings.json` at all). With Task 6.1.1's button
present in the same hint-row slot regardless of whether the table is empty
or populated (the `sourceFilter === "claude-settings"` conditional in S1 is
independent of the `visibleRules.length === 0` conditional that renders the
empty state), a user seeing this empty state now has an immediate, correct
next action: click Reload rules to confirm rather than wonder. No copy
change needed - confirmed still correct, not a gap.

---

## 9. UX acceptance criteria

1. **Task completion**: from the `claude-settings` tab already open, a user
   can trigger a reload and see the result in **1 click** (click "Reload
   rules"; toast + table update both happen without further input).
2. **Discoverability**: the button is visible without scrolling once the
   `claude-settings` tab is selected, on both desktop (≥1024px) and mobile
   viewport widths (per `.claude/CLAUDE.md`'s "mobile + desktop UX
   requirement" - the hint row must not be hidden behind
   `headerButtonsHiddenOnMobile` the way the header's Export/Import buttons
   are).
3. **In-flight feedback**: within 100ms of the click, the button visibly
   changes to a disabled `Reloading…` state (S2) - the user gets immediate
   confirmation the click registered, no perceived-dead-click gap.
4. **Success state**: on a successful reload, a toast reading `"Reloaded N
   claude-settings rule(s)."` appears and the table below already reflects
   the new rule set - no manual refresh needed after the toast.
5. **Error state (malformed file)**: on `{success:false}`, the error toast
   reads `"Failed to reload Claude settings rules — previous rules still
   active..."` (never a raw JSON parse trace) and offers the same "click
   Reload rules again" action as idle state - the button re-enables
   immediately, giving an explicit retry path.
6. **Error state (network/timeout)**: on a thrown RPC error, an error toast
   still appears (never a silent no-op) reading `"Could not reach the
   server to reload rules. Try again."`, and the button re-enables. This
   closes gap §7.1 - it is a required criterion, not optional polish, per
   the task brief's explicit callout of "RPC timeout/network error."
7. **No dead ends**: every one of S1-S6's terminal states (success,
   RPC-reported failure, thrown-error failure) leaves the button clickable
   again with no page reload or navigation required. Verified by: clicking
   Reload rules 3 times in a row (success, then simulate a `success:false`
   response, then simulate a thrown error) never leaves the button
   permanently disabled or the tab in a stuck state.
8. **Double-click safety**: rapidly double-clicking "Reload rules" fires
   at most one RPC call (button disables on the first click before the
   second can register) - closes gap §7.2.
9. **Accessibility**: button is reachable via Tab key and activates via
   Enter/Space (native `<button>`, no custom keyboard handling needed);
   has an accessible name matching its visible text ("Reload rules", plus
   "Reloading…" while disabled - `aria-disabled` implicit via the native
   `disabled` attribute, which also removes it from the tab order while
   in-flight, consistent with the existing Export/Import button pattern);
   toast `role="alert" aria-live="polite"` is inherited for free from
   `NotificationToast.tsx:166` (confirmed in `research/ux.md` §3, no new
   work). Color contrast ≥4.5:1 for the button and toast text is enforced
   automatically by the existing Axe Core CI gate on PRs touching
   `web-app/src/` (per this repo's `CLAUDE.md` "UX analysis CI" section) -
   no manual contrast audit needed as a precondition for this design, but
   CI must pass before merge, including on the `refreshButton`-restyled
   button recommended in §7.3.
