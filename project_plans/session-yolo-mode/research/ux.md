# UX Research: `auto_approve` (Yolo Mode) Toggle

**Phase**: 2 — Research | **Feature**: `project_plans/session-yolo-mode/requirements.md`

## 0. Existing in-repo precedent (read first — this is the template, not a hypothetical)

Autonomous mode (`session.autonomousMode`) is functionally the closest analog already shipped
in this codebase: an opt-in flag that changes how unattended the agent process is, surfaced as
an Omnibar checkbox + a session-card badge + a post-creation toggle action. It should be the
starting point for `auto_approve`'s UX, not a from-scratch design.

- **Omnibar checkbox** — `web-app/src/components/sessions/OmnibarCreationPanel.tsx:466-475`:
  a `<label className={checkboxClass}><input type="checkbox" .../> 🤖 Autonomous mode (Beta)</label>`
  with a `<span className={hint}>` underneath explaining behavior. The label wraps the input,
  so the visible text *is* the accessible name for free — no separate `aria-label` needed.
- **Session-card badge** — `web-app/src/components/sessions/SessionCard.tsx:570-591`: renders
  either a `<button>` (clickable, when `onToggleAutonomousMode` is provided — click disables it
  inline) or a `<span role="img">` (read-only fallback), both with `data-testid="badge-autonomous"`
  and a descriptive `aria-label` (e.g. `"Auto-pilot active (turn 3/10) — click to disable"`).
  Two related states — `autonomousOutcome === "done"` / `"stuck"` — reuse the same badge class
  with inline `style={{ background: "var(--success-bg)", ... }}` / `var(--warning-bg)` overrides
  rather than new CSS classes.
- **Styling** — `web-app/src/components/sessions/SessionCard.css.ts:804-815`: `autonomousBadge`
  is a pill (`borderRadius: vars.radii.full`, `fontSize: vars.fontSize.xs`, `fontWeight: 600`)
  using **neutral** tokens (`vars.color.accentBg` / `vars.color.textSecondary` / `vars.color.borderColor`)
  — appropriate for autonomous mode, which isn't inherently risky. `auto_approve` is inherently
  risky (it removes a safety gate), so it should NOT reuse the neutral `accentBg` — see §6.
- **Post-creation toggle** — `web-app/src/components/sessions/SessionActionsOverflow.tsx`: an
  overflow-menu action (`onToggleAutonomousMode`) gated behind a confirm-on-enable step
  (`isAutonomousConfirmOpen`, line ~418) when turning it **on**, but a direct one-click action
  (no confirm) when turning it **off**. This asymmetry — confirm to arm, no confirm to disarm —
  is exactly the pattern risk-toggle UX literature recommends (§1) and should be copied for
  `auto_approve`.

## 1. Comparable UX patterns (external)

| Tool | Pattern | Takeaway for `auto_approve` |
|---|---|---|
| GitHub Actions "Workflow permissions" (repo settings → Actions) | Radio choice between "Read repository contents" (default) vs "Read and write permissions", with a **static warning callout** ("Uncheck this box... only if you understand the risk") beside the write option, no per-run toggle | Persistent, textual risk framing next to the control beats a bare label; the risky option is never pre-selected |
| GitHub Actions `pull_request_target` / fork PR "Approve and run" | Requires an explicit maintainer click before an untrusted workflow runs with secrets — an interrupt, not a passive toggle | Confirms "friction on enable, not on every use" is the right shape: gate the *decision* once, not each subsequent action |
| Browser "not secure" / mixed-content padlock | Persistent chrome-level badge (icon + color), always visible regardless of which tab/page is focused, click reveals detail | The badge must be visible in the default (collapsed/list) view, not hidden behind a click or hover — matches the "social job" requirement (§5) |
| `sudo` / macOS "administrator privileges" prompt | One-time interrupt at the moment privilege is invoked, distinct icon (shield/lock), never silently granted | Reinforces confirm-on-enable; also — sudo re-prompts per elevated action, which `auto_approve` deliberately does NOT do (its entire point is to skip prompts), so the *badge* must carry the persistent-risk signal that sudo's interrupt normally carries |
| VS Code "Restricted Mode" / "Do you trust the authors of the files in this folder?" | Modal on first open of untrusted workspace; a persistent status-bar item ("Restricted Mode") remains visible for the life of the window while restricted | Directly analogous to "opt-in once, stay visible for the session's life" — the status-bar item is the closest external precedent to the session-card badge |
| 1Password / SSH agent "unlocked" indicator | Small persistent icon change (locked vs unlocked padlock) in the menu bar | Icon-state-change-only (no confirm dialog) for *already-decided* risk is acceptable once the initial grant happened elsewhere — supports "badge shows state, doesn't re-litigate the decision" |

Common thread across all of these: **color (amber/red) + icon + persistent visibility +
confirmation gated on the transition into the risky state, not on staying in it.** None of them
make the risky state silent or put the confirmation behind a second click to *discover* the
option exists.

## 2. User mental model

**What "on" promises**: the agent will not stop and wait for the user to approve each tool
call/file write/shell command — it runs to completion (or failure) unattended. The user's
implicit contract is *speed and non-interruption*, so anything that reintroduces a pause after
enabling it reads as a broken promise, not a safety feature.

**What would violate that expectation / feel surprising**:
- **Silent no-op for an unsupported agent** — this is the single most dangerous UX failure mode
  named in the requirements and confirmed by scope: only Claude Code and Aider have a flag
  mapping today (`requirements.md` "Out of Scope"). If a user enables the toggle for a session
  whose detected agent/program has no mapping, and the backend just... doesn't append a flag,
  the user believes they're running unguarded and are not — they may leave the session
  unattended expecting no prompts, then get blocked on a prompt they never see because they've
  walked away. This is worse than the feature not existing at all. **The toggle must be
  disabled (not hidden) with an inline hint** ("Not supported for &lt;agent&gt;") when the
  detected agent has no mapping — same pattern as `sessionType !== "one_off"` conditionally
  hiding the autonomous-mode checkbox in `OmnibarCreationPanel.tsx:464`, but disabled+labeled
  rather than removed, since removal would itself look like a silent capability gap.
- **Badge present but stale** — if `auto_approve` is toggled post-creation (the "should have"
  surface) but doesn't take effect until the agent process restarts, showing the "⚡ Auto" badge
  immediately (before restart) would claim a guarantee that isn't true yet. See §4 for the
  pending-state treatment.
- **Toggle surviving a session clone/restart in an unexpected direction** — not in scope to
  design fully here, but worth flagging: because this is a *persisted* per-session field (not
  ephemeral UI state), a user who toggles it on, closes the tab, and returns later needs the
  badge to still accurately reflect it — the "reload persistence" success criterion already
  covers this at the data layer; UX just needs to trust and render that state, not re-derive it.

## 3. Accessibility

**Omnibar checkbox** — follow the existing `autonomousMode` checkbox exactly
(`OmnibarCreationPanel.tsx:466-475`): a native `<input type="checkbox">` wrapped in a `<label>`
with visible text (e.g. `⚡ Auto-approve (skip permission prompts)`), plus a `<span className={hint}>`
below it explaining the risk in one sentence. Native checkbox + wrapping label needs no extra
ARIA — the label text is already the accessible name, and native `<input type="checkbox">`
gets correct keyboard behavior (Space toggles, Tab focuses) for free, unlike a custom
`role="switch"` div which would need `aria-checked`, `tabIndex=0`, and manual Space/Enter
handling. Do not introduce a custom switch component for this — no other risk toggle in the
codebase uses one, and the native checkbox already satisfies the e2e convention
(`.claude/rules/e2e-test-conventions.md`) via `getByRole('checkbox', { name: /auto-approve/i })`
without needing a `data-testid`.

When disabled for an unsupported agent, add `disabled` to the `<input>` (removes it from the
tab order's *interactive* set but keeps it announced as "dimmed, unavailable" by screen
readers) and keep the hint text visible explaining why, rather than `aria-disabled` alone —
`disabled` is preferable here because there is no case where a screen-reader user needs to
inspect a disabled-but-still-focusable control's state beyond the hint text already present.

**Session-card badge** — follow `autonomousBadge`'s exact pattern
(`SessionCard.tsx:570-591`): render as a `<button>` when a toggle handler is available
(click disables it, mirroring `onToggleAutonomousMode`) or `<span role="img">` when read-only,
**always** with an explicit `aria-label` — never bare emoji/icon content. Concretely:

```tsx
aria-label={`Auto-approve enabled — this session skips permission prompts${onToggleAutoApprove ? "; click to disable" : ""}`}
data-testid="badge-auto-approve"
```

`role="img"` on the read-only `<span>` is required (matches the existing `memoryBadge` and
`autonomousBadge` spans) so screen readers treat the emoji+text as a single announced unit
instead of reading "lightning bolt emoji" then the text separately. The visible text should
not be the emoji alone ("⚡") — pair it with a short label ("⚡ Auto") so sighted users scanning
quickly and screen-reader users both get the same signal, consistent with the requirement's own
example badge text.

## 4. Error/edge-case UX

**Unsupported agent, toggle attempted**:
- *At creation time*: disable the Omnibar checkbox once an agent/program is selected that has
  no flag mapping, with hint text `"Not supported for <agent name> — flag mapping only exists
  for Claude Code and Aider."` This requires knowing the agent selection before the checkbox
  renders, which the Omnibar panel already does for other conditional fields (e.g. `sessionType`
  gating at `OmnibarCreationPanel.tsx:464`) — same mechanism, keyed on detected agent instead of
  session type.
- *Post-creation*: if the agent is changed/detected differently after the fact (edge case, but
  the "should have" toggle acts on an existing session), the toggle action itself should be
  disabled in the overflow menu with the same hint on hover/tooltip, not silently no-op when
  clicked.

**Pending-restart state** (post-creation toggle flipped, not yet applied): the requirements
explicitly scope this as "takes effect on next launch/restart... not a live in-place flag
swap." The UI must not show the steady-state "⚡ Auto" badge the instant the toggle flips,
because the running process still has the old flag. Two precedents already in this codebase for
"changed but not yet live" state:
- `pendingProgramChange` badge (`SessionCard.tsx:627`, styled with `workflowBadge`) — an
  existing pattern for exactly this shape of problem (a setting changed but not yet reflected
  in the live process). Reuse this pattern/style rather than inventing a new "pending" visual
  language: something like a dashed-border or muted variant of the auto-approve badge reading
  `"⚡ Auto (pending restart)"` with `aria-label="Auto-approve will take effect after this
  session's next restart"`.
- The autonomous-mode confirm-dialog pattern in `SessionActionsOverflow.tsx:418` also shows the
  codebase already treats "this changes agent behavior" actions as needing explicit
  arm-confirmation — the post-creation `auto_approve` toggle-on action should get the same
  confirm-before-enable step, direct no-confirm disable, matching §0's asymmetric-friction note.

## 5. Jobs-to-be-done

- **Functional**: skip interruption prompts for trusted, unattended automated work — the user
  wants to kick off a session and walk away without babysitting approval dialogs.
- **Emotional**: confidence that this is opt-in, visible, and reversible — not a setting that
  crept on via a config default or got silently inherited from a cloned session. The
  confirm-on-enable step (§0, §4) and the always-visible badge both serve this: the user should
  never have to *wonder* whether a given session is unguarded.
- **Social**: a teammate scanning the session list in a shared environment needs to identify
  unguarded sessions **at a glance, without opening each one** — this is the primary reason the
  badge must be persistently visible in the collapsed session-card view (not hidden behind a
  hover/expand), matching VS Code's persistent "Restricted Mode" status-bar item and the browser
  "not secure" padlock (§1) rather than 1Password's menu-bar-only indicator (which assumes a
  single-user context this shared session list doesn't have).

## 6. CSS/token conventions

Per `.claude/rules/css-architecture.md`, any new badge styling must live in a `.css.ts` file
using `vanilla-extract` + `vars.*` tokens — never hardcoded hex or bare `var(--x)` strings in
new `.css.ts` files.

**Confirmed existing tokens usable for this badge** (`web-app/src/styles/theme.css.ts`, defined
across all theme variants — light/dark/custom themes at lines ~100-106, 204-210, 309-315,
421-427, 533-539, 646-652):
```
vars.color.warning / warningBg / warningText
vars.color.error   / errorBg   / errorText / errorDark
```
CSS custom-property equivalents also already exist in `globals.css` (`--warning`, `--warning-bg`,
`--warning-text`, `--error`, `--error-bg`, `--error-text`) and are already used for inline style
overrides on the autonomous badge's "stuck" state (`SessionCard.tsx:608`: `var(--warning-bg)`).

**Recommendation**: unlike `autonomousBadge` (neutral `accentBg`/`textSecondary`, appropriate
for a non-risky flag), the `auto_approve` badge should use `vars.color.warning` /
`vars.color.warningBg` / `vars.color.warningText` as its base palette (amber, not red — this is
a deliberate, opt-in, reversible power-user setting, not an error state, so `error`/red should
be reserved for something actually broken, e.g. a flag-mapping failure). This also gives a
natural, already-tokenized way to distinguish the pending-restart variant (§4) — e.g. a muted or
outlined treatment of the same warning tokens — without adding new palette entries. No new
tokens need to be added to `theme.css.ts`; the existing `warning*` triad covers both the
steady-state and pending-restart badge needs.

## Summary of concrete UX requirements for Phase 3 (plan)

1. Omnibar: native checkbox + wrapping `<label>`, hint text below, disabled+hinted (not hidden)
   when the detected agent has no flag mapping — mirror `OmnibarCreationPanel.tsx:466-475`.
2. Session card: badge as `<button>` (toggle available) or `<span role="img">` (read-only),
   always with explicit non-emoji-only `aria-label`, `data-testid="badge-auto-approve"` —
   mirror `SessionCard.tsx:570-591`, styled with `vars.color.warning*` tokens (not the neutral
   tokens `autonomousBadge` uses).
3. Post-creation toggle: confirm-before-enable, no-confirm-to-disable (asymmetric friction),
   pending-restart visual variant of the same badge until the process actually relaunches —
   mirror `SessionActionsOverflow.tsx`'s autonomous-mode confirm dialog and
   `SessionCard.tsx:627`'s `pendingProgramChange` pattern.
4. No new vanilla-extract tokens required — reuse `vars.color.warning` / `warningBg` /
   `warningText` already defined in `web-app/src/styles/theme.css.ts`.
