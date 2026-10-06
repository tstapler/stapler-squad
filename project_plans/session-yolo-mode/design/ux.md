# UX Design: `auto_approve` (Yolo Mode) Toggle

**Phase**: 3 — Design | **Feature**: `project_plans/session-yolo-mode/`
**Inputs**: `requirements.md`, `research/ux.md`, `implementation/plan.md` (Epics 4-6)

This document is the wireframe/flow/acceptance-criteria companion to `research/ux.md`. It does
not re-derive design decisions — every control, copy string, `data-testid`, and CSS token below
is taken verbatim from `implementation/plan.md`'s locked tasks (cited inline as `plan.md:<line>`)
so this artifact stays consistent with what Phase 3 already committed to build.

## Surfaces covered

| # | Surface | Plan reference |
|---|---|---|
| a | Omnibar creation-panel checkbox (+ disabled/unsupported-agent state) | Epic 4 (`plan.md:387-436`) |
| b | Session-card steady-state badge | Epic 5 (`plan.md:441-525`) |
| c | Session-card pending-restart badge variant | Epic 5 (`plan.md:526-544`) |
| d | Overflow-menu toggle action | Epic 6 (`plan.md:615-638`) |
| e | Confirm-enable dialog | Epic 6 (`plan.md:583-614`) |
| f | Direct disable (no dialog) | Epic 6 (`plan.md:626-632`) |

---

## (a) Omnibar creation-panel checkbox

### Wireframe — supported agent (`program: "claude"`)

```
┌─ Omnibar / Create Session ──────────────────────────────────────┐
│  Program:  [ claude                                    ▾ ]      │
│  ...                                                             │
│  ☐ 🤖 Autonomous mode (Beta)                                    │
│    Runs unattended for up to N turns...                         │
│                                                                   │
│  ☐ ⚡ Auto-approve (skip permission prompts)                    │
│    Skips ALL permission/approval prompts for this agent. Risk   │
│    of unintended file changes — use only in disposable/          │
│    sandboxed workspaces.                                         │
│                                                                   │
│  [ Create Session ]                                              │
└───────────────────────────────────────────────────────────────┘
```

### Wireframe — unsupported agent (`program: "codex"`), checkbox disabled

```
┌─ Omnibar / Create Session ──────────────────────────────────────┐
│  Program:  [ codex                                     ▾ ]      │
│  ...                                                             │
│  ☒ (dimmed) ⚡ Auto-approve (skip permission prompts)           │
│    Not supported for "codex" yet.                                │
│                                                                   │
│  [ Create Session ]                                              │
└───────────────────────────────────────────────────────────────┘
```
(Checkbox renders dimmed/`disabled`; text is grey, not struck through — it is unavailable, not
an error.)

### Interaction flow

1. User selects/types a `program` value in the Omnibar creation panel.
2. On every `program` change, `isAutoApproveSupported(program)` (plan.md:421-425) re-evaluates.
3. If supported: checkbox is interactive; hint reads the risk sentence (plan.md:415).
4. If unsupported: checkbox gets `disabled`; hint switches to `Not supported for "<program>" yet.` (plan.md:416); if the checkbox was previously checked and the user then changes `program` to an unsupported one, the checked state is **not** silently cleared by this surface — `OmnibarFormState.autoApprove` still holds `true`, but the disabled control can't be unchecked by the user and the create-time payload still reflects whatever was last set. *(Flag for implementation: confirm `canSubmit`/submit-payload logic in `Omnibar.tsx` does not silently submit `autoApprove: true` for an unsupported program if the user checked it before switching agents — recommend forcing `autoApprove` back to `false` in form state when `program` transitions to unsupported, so the checkbox's disabled+unchecked visual state and the submitted payload never disagree. This is not explicitly specified in Task 4.1.1/4.1.2 and should be closed before Phase 5 implementation.)*
5. User checks the box (supported case) → `setFormField("autoApprove", true)` (plan.md:409).
6. User submits → `createSession` called with `autoApprove: true` in the payload (plan.md:434).

### Error / edge cases

| Case | Handling |
|---|---|
| Unsupported agent | Checkbox `disabled` (not hidden), hint explains why (plan.md:94-98, 416) |
| Program field empty/not yet chosen | `isAutoApproveSupported("")` → base `""` not in `["claude","aider"]` → disabled, hint reads `Not supported for "this agent" yet.` (fallback string, plan.md:416) |
| Program changed after checking, to unsupported | See flow step 4 — flagged as an open gap for implementation to close (force-uncheck on transition) |
| Screen reader on disabled checkbox | Native `disabled` attribute — announced as dimmed/unavailable; hint text remains in the accessibility tree immediately below (research/ux.md:94-98) |

---

## (b) Session-card steady-state badge

### Wireframe — Active session, badge clickable (toggle handler present)

```
┌─ Session Card ───────────────────────────────────────────┐
│  ● my-feature-branch                    [claude] [⚡ Auto]│
│  Active · 00:14:32                              ▲          │
│  git worktree: feature/foo                      │          │
│                                       clickable, disables   │
│                                       on click (no confirm) │
└─────────────────────────────────────────────────────────┘
```

### Wireframe — read-only context (no toggle handler, e.g. embedded pane header)

```
[claude]  [⚡ Auto]      ← <span role="img">, not clickable
```

### Interaction flow

1. `session.autoApprove === true` and `!pendingAutoApproveChange` (plan.md:502) → badge renders.
2. If `onToggleAutoApprove` is provided (session-card context): renders as `<button>`
   (plan.md:504-513); `data-testid="badge-auto-approve"`, visible text `⚡ Auto`,
   `aria-label="Auto-approve enabled — this session skips permission prompts; click to disable"`.
3. Click → `onToggleAutoApprove(session.id, false)` fires directly, **no confirm dialog**
   (plan.md:509) — this is surface (f), reachable both from this badge and from the overflow
   menu's "Disable auto-approve" item.
4. If no handler (read-only context): renders `<span role="img">` (plan.md:514-522), same
   `data-testid`, non-clickable `aria-label="Auto-approve enabled: this session skips permission
   prompts"`.

### Error / edge cases

| Case | Handling |
|---|---|
| Badge click fires while a drag/other card interaction is in flight | `e.stopPropagation()` on the button's `onClick` (plan.md:509) prevents the click from also triggering the parent card's row-click/navigation handler |
| `updateSession` fails after disabling via badge click | Not explicitly specified in plan.md's Task 6.1.1 handler (plan.md:558-565) beyond `console.error` — **UX gap**: badge optimistically shows disabled state or the toggle silently fails with no visible feedback. Recommend a toast/inline error on `updateSession` rejection so the user isn't left believing auto-approve is off when the RPC actually failed; this is a "no dead ends" requirement (see acceptance criteria below) and should be confirmed during implementation review. |

---

## (c) Session-card pending-restart badge variant

### Wireframe

```
┌─ Session Card ───────────────────────────────────────────┐
│  ○ my-feature-branch                [claude] [⏳ Auto-approve pending] │
│  Paused                                                    │
└─────────────────────────────────────────────────────────┘
```
(Muted/outlined style — `autoApprovePendingBadge` uses `vars.color.accentBg` /
`vars.color.textSecondary`, i.e. the *neutral* palette, not the amber `warning*` triad used by
the steady-state badge — plan.md:462-473. This is a deliberate visual demotion: pending state is
informational, not an active-risk signal yet.)

### Interaction flow

1. `hasPendingAutoApproveChange(session)` (plan.md:492-497): true when session is
   `PAUSED`/`STOPPED` **and** `session.autoApprove` disagrees with whether a known yolo flag
   literal (`--dangerously-skip-permissions` / `--yes-always`) is present in
   `session.launchCommand`.
2. While true, the steady-state "⚡ Auto" badge (b) is suppressed (`!pendingAutoApproveChange`
   guard, plan.md:502) and this badge renders instead: `data-testid="badge-pending-auto-approve"`,
   text `⏳ Auto-approve pending`, `aria-label="Auto-approve change pending: takes effect on
   resume or restart"` (plan.md:528-538).
3. Badge is **not** clickable/actionable — it is read-only status (no `onClick` in plan.md's
   spec); the user resumes/restarts the session through existing session controls, at which
   point the next launch bakes in (or removes) the flag and the badge flips to (b) or disappears.

### Error / edge cases

| Case | Handling |
|---|---|
| Session is `ACTIVE` with a pending toggle | Cannot happen by construction — `SetAutoApprove` restarts immediately if `Status == Active` (plan.md:350-352), so an Active session's `launchCommand` and `autoApprove` are never observed out of sync from the UI's perspective |
| `launchCommand` empty/not yet set (brand-new session, never launched) | `hasPendingAutoApproveChange` guards `!session.launchCommand` → returns `false` (plan.md:494) — no pending badge shown before first launch, avoiding a false-positive "pending" flash on session creation |
| User never resumes the paused session | Badge persists indefinitely — acceptable; it accurately reflects "not yet applied," and there is no time-based escalation specified (out of scope) |

---

## (d) Overflow-menu toggle action

### Wireframe — session currently `autoApprove: false`

```
┌─ ⋮ overflow menu ─────────────┐
│  Rename                        │
│  Duplicate                     │
│  ⚡ Enable auto-approve   ☐    │   ← role="menuitemcheckbox", aria-checked="false"
│  🤖 Enable autonomous mode     │
│  Pause                         │
│  ...                           │
└────────────────────────────────┘
```

### Wireframe — session currently `autoApprove: true`

```
┌─ ⋮ overflow menu ─────────────┐
│  Rename                        │
│  Duplicate                     │
│  ⏹ Disable auto-approve  ☒    │   ← aria-checked="true"
│  ...                           │
└────────────────────────────────┘
```

### Interaction flow

1. User opens the session's overflow menu, sees "Enable auto-approve" / "Disable auto-approve"
   depending on current state (plan.md:627-636), `role="menuitemcheckbox"`, `aria-checked`
   bound to `session.autoApprove`.
2. Click on **Enable**: menu closes (`close()`), `setIsAutoApproveConfirmOpen(true)` (plan.md:628)
   → routes to surface (e), the confirm dialog. `updateSession` is **not** called yet.
3. Click on **Disable**: menu closes, `onToggleAutoApprove(session.id, false)` fires immediately
   (plan.md:630) → routes to surface (f), no dialog.

### Error / edge cases

| Case | Handling |
|---|---|
| Overflow menu opened for a session whose agent is no longer supported (edge case per research §4: agent changed post-creation) | `research/ux.md:126-129` specifies the toggle action itself should be disabled with a hint, mirroring the Omnibar's disabled+hinted pattern — **not explicitly re-stated in plan.md's Task 6.2.1** (the menu item as written has no disabled/support-check branch). This is a gap between research and plan; recommend closing before/during implementation by gating the menu item the same way the Omnibar checkbox is gated (`AutoApproveSupported`-equivalent check on the frontend, or surfacing the backend's `AutoApproveSupported(program)` via the session read model). |

---

## (e) Confirm-enable dialog

### Wireframe

```
┌──────────────────────────────────────────────────────────┐
│  Enable Auto-Approve                                  [×] │
│                                                             │
│  "my-feature-branch" will skip ALL permission/approval    │
│  prompts for its agent (e.g. file edits, shell commands). │
│                                                             │
│  ⚠ This is genuinely unsafe outside a disposable/          │
│  sandboxed workspace — unintended file modifications or   │
│  data loss are possible. You can disable it at any time   │
│  from this menu.                                            │
│                                                             │
│           [ Enable Auto-Approve ]     [ Cancel ]           │
└──────────────────────────────────────────────────────────┘
```
(Rendered via `createPortal(..., document.body)` — per `.claude/rules/css-architecture.md`'s
"no `position:fixed`/`absolute` overlay without `createPortal`" rule; plan.md:585 already
follows this.)

### Interaction flow

1. Triggered only from surface (d)'s "Enable auto-approve" menu item — never auto-opens.
2. `role="dialog"`, `aria-modal="true"`, `aria-labelledby="autoApproveDialogTitle"`
   (plan.md:589-591) — matches the existing autonomous-mode confirm dialog's accessibility
   shape verbatim, plus `useFocusTrap` (plan.md:582) so focus can't leave the dialog while open.
3. `Escape` key or clicking the backdrop closes without enabling (plan.md:586, 594).
4. "Enable Auto-Approve" button → `onToggleAutoApprove(session.id, true)` fires, dialog closes
   (plan.md:601).
5. "Cancel" button → dialog closes, no RPC call (plan.md:606).

### Error / edge cases

| Case | Handling |
|---|---|
| `updateSession` fails after clicking "Enable Auto-Approve" | Same gap noted under surface (b) — no explicit error-toast/rollback specified in plan.md's Task 6.1.1 handler; dialog has already closed optimistically by the time the RPC would reject. Recommend surfacing the error (toast or inline banner) rather than a silent console.error, so the "no dead ends" acceptance criterion below is actually met. |
| User clicks "Enable" twice quickly (double-submit) | Not explicitly guarded in plan.md; recommend disabling the button after first click (standard double-submit guard) — small implementation detail, not a design gap severe enough to block Phase 3 plan but worth a note for the implementing agent. |
| Keyboard-only user | Focus trap keeps Tab cycling inside dialog; Escape closes; both buttons reachable via Tab/Shift+Tab and activatable via Enter/Space (native `<button>`) |

---

## (f) Direct disable (no dialog)

### Wireframe

Reached from two entry points, both producing the identical result — no additional wireframe
beyond what's already shown at their origin (surfaces b and d):

```
Entry 1: click "⚡ Auto" badge on session card (surface b)
Entry 2: click "Disable auto-approve" in overflow menu (surface d)
                            │
                            ▼
              onToggleAutoApprove(session.id, false)
                            │
                            ▼
              updateSession(sessionId, { autoApprove: false })
                            │
              ┌─────────────┴─────────────┐
              ▼                           ▼
   Session Active: SetAutoApprove    Session Paused: flag flips,
   restarts (plan.md:350-352) →      no restart (plan.md:359) →
   badge updates to "no badge"       badge shows pending-variant (c)
   once restart completes            until next resume/restart
```

### Interaction flow

1. No confirmation step — by design, matches every other risk-toggle precedent in the codebase
   (autonomous mode) and the external pattern survey in `research/ux.md` §1 ("confirmation
   gated on the transition into the risky state, not on staying in it").
2. Backend `SetAutoApprove(false, persist)` (plan.md:338-354): persists first, then restarts
   only if the session is currently `Active`.

### Error / edge cases

| Case | Handling |
|---|---|
| Session is mid-restart when disable is clicked again | Out of scope for this design pass — concurrency handling belongs to the actor-model `sendSyncErr` mechanism already in place for other setters (plan.md:339-344), not a new UX concern |
| `updateSession` fails | Same gap flagged under (b)/(e) — no visible error feedback specified; recommend closing during implementation |

---

## UX Acceptance Criteria

Each is independently testable by a human clicking through the running app (or, where noted,
by an automated e2e test per `tests/e2e/session-auto-approve.spec.ts`, plan.md:687-699).

### Task completion

1. **Enable at creation**: user can enable auto-approve for a new session in **1 click**
   (check the box) plus the existing session-creation flow — no extra steps beyond checking
   one checkbox before submitting.
2. **Discover unsupported state**: user can determine whether auto-approve is available for
   their chosen agent in **0 additional clicks** — the disabled state and hint text are visible
   the moment the program field resolves to an unsupported agent, without opening any menu.
3. **Enable post-creation**: user can enable auto-approve on an existing session in **3 clicks**
   (open overflow menu → click "Enable auto-approve" → click "Enable Auto-Approve" in the
   confirm dialog).
4. **Disable post-creation**: user can disable auto-approve on an existing session in **1 click**
   from the session-card badge, or **2 clicks** via the overflow menu (open menu → click
   "Disable auto-approve") — no dialog in either path.
5. **Identify at a glance**: a user scanning the collapsed session list can identify every
   unguarded session without opening or hovering any of them — the steady-state badge
   (`⚡ Auto`) is visible in the default card layout, matching the "social job" requirement in
   `research/ux.md` §5.

### Error states

6. **Unsupported agent, creation time**: shows the message `Not supported for "<program>" yet.`
   and the checkbox is disabled but visible (not hidden) — the exit path is simply choosing a
   different `program` value, which re-enables the checkbox.
7. **Unsupported agent, post-creation** *(pending the gap noted under surface (d) being
   closed)*: the overflow menu's toggle action should be disabled with an equivalent hint
   (tooltip or inline text) rather than allowing a click that silently no-ops — this criterion
   is **not yet satisfiable as specified in plan.md** and should be resolved before this is
   marked done.
8. **RPC failure on toggle** *(pending the gaps noted under surfaces (b)/(e)/(f))*: toggling
   auto-approve on or off, when `updateSession` rejects, shows a visible error (toast or inline
   message) and leaves the badge/checkbox state reflecting the *last confirmed* server state —
   not the optimistic click target. This criterion is **not yet satisfiable as specified in
   plan.md's Task 6.1.1** and should be resolved before this is marked done.

### No dead ends

9. Every error/disabled state above has a stated exit path: unsupported-agent states exit by
   changing the `program` selection; the confirm dialog exits via Cancel or Escape; a failed
   RPC (once criterion 8 is implemented) exits by re-attempting the toggle or dismissing the
   error message — no state in this design traps the user without a next action.

### Accessibility

10. **Keyboard navigation**: every interactive element (Omnibar checkbox, session-card badge
    button, overflow-menu item, confirm-dialog buttons) is reachable via Tab and activatable via
    Enter/Space, using native `<input type="checkbox">` and `<button>` elements throughout — no
    custom `role="switch"` or non-native control requiring manual key handling (research/ux.md
    §3, plan.md's implementation uses native elements exclusively).
11. **Screen-reader labels** — exact strings, cited from plan.md:
    - Omnibar checkbox: visible label text `⚡ Auto-approve (skip permission prompts)` serves as
      the accessible name via the wrapping `<label>` (no separate `aria-label` needed,
      plan.md:404-412).
    - Steady-state badge (clickable): `aria-label="Auto-approve enabled — this session skips
      permission prompts; click to disable"` (plan.md:507).
    - Steady-state badge (read-only): `aria-label="Auto-approve enabled: this session skips
      permission prompts"` (plan.md:518).
    - Pending-restart badge: `aria-label="Auto-approve change pending: takes effect on resume
      or restart"` (plan.md:534).
    - Overflow-menu item: `aria-label={session.autoApprove ? "Disable auto-approve for
      ${session.title}" : "Enable auto-approve for ${session.title}"}` (plan.md:623), plus
      `role="menuitemcheckbox"` / `aria-checked={session.autoApprove}` (plan.md:619-620).
    - Confirm dialog: `role="dialog"`, `aria-modal="true"`,
      `aria-labelledby="autoApproveDialogTitle"` pointing at the `<h3 id="autoApproveDialogTitle">
      Enable Auto-Approve</h3>` heading (plan.md:589-591, 596).
12. **Color contrast ≥ 4.5:1** — VERIFIED for text-on-background pairs. Computed WCAG contrast
    ratios for `vars.color.warningText` on `vars.color.warningBg` across all 6 theme variants
    defined in `web-app/src/styles/theme.css.ts`:

    | Theme (line) | `warningBg` | `warningText` | Contrast ratio |
    |---|---|---|---|
    | Light (~100-102) | `#fef3c7` | `#92400e` | 6.37:1 |
    | Dark (~204-206) | `#78350f` | `#fbbf24` | 5.43:1 |
    | Theme 3 (~309-311) | `#2a1a00` | `#ffcc44` | 11.21:1 |
    | Theme 4 (~421-423) | `#1a1600` | `#fcee09` | 14.99:1 |
    | Theme 5 (~533-535) | `#1a1400` | `#e4c840` | 11.04:1 |
    | Theme 6 (~646-648) | `#78350f` | `#fbbf24` | 5.43:1 |

    All 6 pass 4.5:1 (WCAG AA normal text) with margin. **Flag**: the badge's 1px border
    (`vars.color.warning` on `vars.color.warningBg`, plan.md:459) is a separate,
    **non-text** contrast concern (WCAG 1.4.11, 3:1 minimum for UI component boundaries) —
    spot-checked for light theme only: `#f59e0b` on `#fef3c7` = **1.93:1, below the 3:1
    non-text threshold**. This was not verified in Phase 2 research (research/ux.md §6 only
    asserts the tokens "exist," not that the border pairing meets contrast). Since the border
    is decorative/redundant with the badge's own fill-color distinction from the card
    background (not the sole means of conveying badge boundary), this is a should-fix, not a
    blocker — recommend spot-checking the remaining 5 themes' border/bg pairs during
    implementation review and, if consistently low, either dropping the border or switching it
    to a token with better separation from `warningBg`.
