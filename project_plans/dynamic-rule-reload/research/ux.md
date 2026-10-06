# UX Research: dynamic-rule-reload

## 1. Current state of ApprovalRulesPanel — source visibility already built

`web-app/src/components/sessions/ApprovalRulesPanel.tsx` and
`web-app/src/lib/hooks/useApprovalRules.ts` already do most of requirement #3
("some way to see which rules come from claude-settings vs user vs seed").

- **Source filter tabs** (`ApprovalRulesPanel.tsx:434-451`): `all | user | config | seed |
  claude-settings`, each with a live count, e.g. `Claude Settings (3)`.
- **Source column + badge** in the rules table (`:550-565`): every row shows a
  `sourceBadge`/`configFileBadge` pill via `sourceLabel()` (`:61-69`) — `"claude-settings"` →
  `"Claude Settings"` — with a `title` tooltip: *"These rules come from your
  ~/.claude/settings.json file"*.
- **Config-file hint banner** (`:452-457`): when the `config` tab is active, shows the file
  path (`~/.config/stapler-squad/shared_rules.yaml`). No equivalent hint exists yet for the
  `claude-settings` tab, but the empty-state copy for that tab already says *"No rules from
  your ~/.claude/settings.json file were found."* (`:497-499`).
- **Read-only enforcement**: rows are only editable/deletable when `rule.source === "user"`
  (`:576, :591`) — claude-settings rows show `"Always on"` and no edit/delete controls, which
  already communicates read-only-ness visually.

**Gap**: there is a generic `refresh` button (`↻`/`⟳`, `:349-357`) that re-fetches
`listApprovalRules` from the server, but it does **not** trigger a re-parse of
`~/.claude/settings.json` — see backend finding below. So today, clicking refresh after
hand-editing `settings.json` silently does nothing for that source (it just re-fetches the
same stale server-side list).

**Backend confirms the requirements.md claim**: `grep -rn "LoadClaudeSettingsRules"` across
`server/` and `session/` finds exactly one hit — the function's own definition in
`server/services/claude_settings_parser.go:59`. It is never called from `rules_service.go`,
`session_service.go`, or anywhere else. `RulesService.allRuleSpecs()` (backing
`ListApprovalRules`) has no code path that invokes it. This is dead code, confirming the
regression: claude-settings rules are not loaded into the live classifier at all today, and
there is also no file-watcher (`grep -rn "watch\|fsnotify\|Watcher"` over the relevant files
returns nothing) — so "auto-reload" doesn't exist in any form yet, manual or automatic.

## 2. Toast system — reuse `showActionToast`, don't build a new one

The app has one custom toast system, no third-party lib (`sonner`/`react-hot-toast`/Radix
Toast are absent from `package.json`). It's `NotificationContext.tsx` +
`NotificationToast.tsx`, rendered as a fixed-position stack (`zIndex.toast`, bottom-right).

The exact primitive for this feature is already established and used ~20 times in
`BacklogItemDetail.tsx` and `board/page.tsx` for "routine action" toasts:

```ts
const { showActionToast } = useNotifications();
showActionToast(message, "success" | "error", key);
```

- **`key` dedupes**: a second call with the same key replaces the toast instead of stacking
  (`NotificationContext.tsx:264-288`). For this feature, a stable key like
  `"claude-settings-reload"` prevents duplicate toasts if the file-watcher fires twice or the
  user double-clicks "Reload rules".
- **Auto-dismiss**: success = 5000ms, error = 10000ms — already differentiated, matching
  requirement #5's ask for an error variant.
- **Bypasses the history panel/audit log** by design (per the docstring at
  `NotificationContext.tsx:71-76`) — appropriate here since this is a routine, non-actionable
  event, not something that needs a persistent record.

Do not build a new toast mechanism or reach for a library — every other "did an action
succeed/fail" flow in the codebase goes through `showActionToast`, and this feature is
exactly that shape.

## 3. Accessibility — already handled by the existing toast

`NotificationToast.tsx:166` sets `aria-live={type === "approval_needed" ? "assertive" :
"polite"}` and `role="alert"` on every toast. `showActionToast`-created notifications have
`notificationType: "task_complete"` (success) or `"error"` — neither is `"approval_needed"`,
so they get `aria-live="polite"`, the correct choice for a non-interruptive, non-actionable
event. No new accessibility work is needed; just call the existing hook.

## 4. Comparable UX patterns — silent-vs-interruptive tradeoff

Reference points and their default posture:

| Tool | Pattern | Interruptive? |
|---|---|---|
| VS Code "File changed on disk" | Modal-adjacent banner, blocks the editor tab until user picks Reload/Compare/Ignore | Yes — because silently reloading could discard unsaved edits |
| webpack-dev-server / Vite HMR | Silent auto-apply, tiny overlay only on **error** | No — because HMR failures are usually cosmetic/recoverable |
| Docker Compose watch | Terminal log line + optional restart, no UI prompt | No — CLI tool, no UI to interrupt |
| GitHub Desktop "repo changed externally" | Non-blocking banner with a Refresh action, no auto-apply of destructive ops | No, but visible and actionable |

This feature's rules are **security-relevant auto-approval behavior** — closer to VS Code's
case than webpack's, because a bad/malicious edit to `settings.json` could silently grant
auto-approval for a dangerous tool call. But unlike VS Code, there's no local unsaved-edit
conflict to protect against (the UI never edits claude-settings rules), so a **blocking modal
is overkill**. The right middle ground, and what requirements.md's scope already implies:

- **Toast, not modal** — non-interruptive, matches `showActionToast`'s existing posture.
- **But the toast must let the user *see* what changed**, not just say "rules reloaded." A
  bare ambient toast is insufficient for a security-relevant change — recommend the toast
  message include a delta summary if cheaply available (e.g. `"Claude settings reloaded — 2
  rules added, 1 removed"`), and/or make the toast clickable to jump to/filter the
  `claude-settings` tab (which already exists — see §1) so the user can visually diff against
  what was there before. If a delta count isn't cheaply computable, at minimum the toast
  should say what to check: `"Claude settings rules reloaded — review Claude Settings tab."`

## 5. Error UX (malformed JSON on reload)

Should be an **error-variant toast** (`showActionToast(msg, "error", key)`), for two reasons:
it matches the existing success/error pairing pattern in `notificationType` (`task_complete`
vs `error`, different icon, different color per `notificationMapping.ts`), and error toasts
already auto-dismiss slower (10s vs 5s) which fits a message the user needs a moment longer to
read.

- **Surface the previous-state guarantee, not the raw parse error.** The primary message
  should be: `"Failed to reload Claude settings rules — previous rules still active."` This is
  the security-relevant fact (nothing silently disappeared/changed).
  - Recommend NOT trying to cram a raw JSON parse error (line/column, jq-style trace) into a
    5-10s auto-dismissing toast — it's not actionable at a glance and toasts aren't the place
    for a stack trace. If the raw error is worth showing at all, put it in a
    `title`/`aria-label` tooltip on the toast or log it to console (there's already a
    `console.error` convention throughout the codebase for this — see
    `useApprovalRules.ts:72`), not inline in the toast body.
  - This mirrors the existing `error`/`exportError` banner pattern already in
    `ApprovalRulesPanel.tsx:390-394` (`Export failed: {exportError.message}`) — that one *does*
    inline `.message`, so if this feature wants exact consistency with sibling error banners
    rather than the toast pattern, precedent exists either way. Given this error is triggered
    by a file edited **outside** the app (the user may not have the tab open), the toast is the
    more important surface of the two; a redundant inline banner in the panel is optional
    polish, not required.

## 6. Job-to-be-done: the manual button matters more than the auto-toast

The persona editing `~/.claude/settings.json` by hand is tuning **global** Claude Code
permissions — this is very likely not stapler-squad-specific intent. Most users editing that
file are thinking about Claude Code CLI/IDE behavior, not this app's classifier. Two
consequences:

1. **They probably don't expect stapler-squad to notice at all**, let alone in real time. An
   auto-reload toast is a bonus, not something they're anticipating — so it must be low-key
   (toast, not modal) and must not claim credit for something the user didn't ask for in a way
   that reads as alarming.
2. **They likely don't have the stapler-squad tab open when they edit the file** — so the
   auto-toast will frequently fire into an empty room and be missed entirely (it's not
   persisted anywhere by design, per §2 — `showActionToast` bypasses history). This makes the
   **manual "Reload rules" button the load-bearing affordance**, not the toast: it's what a
   user reaches for the next time they *do* open stapler-squad and want to confirm their edit
   took effect, especially since (per §1) the existing generic refresh button silently does
   NOT pick up settings.json changes today — a user who already tried the existing refresh and
   saw no change has a reasonable, currently-unaddressed expectation that "reload" should
   include claude-settings.

**Recommendation**: place "Reload rules" specifically in/near the `claude-settings` tab
context (not as a generic top-of-panel action) — e.g. an inline action in the empty-state /
hint area for that tab, similar to the existing config-file hint banner at
`ApprovalRulesPanel.tsx:452-457`. That keeps it discoverable exactly when a user is looking at
that source and wondering why their hand-edit isn't reflected, rather than another button in
the already-crowded header button row (`generateButtonRow`, six buttons already:
Generate/Cancel/Export/Import/Refresh).

## Bottom line

This feature's user-facing surface is genuinely small, and requirements.md's own framing
undersells how much is already done:

- Requirement #3 (source visibility) is **already fully implemented** — filter tabs, badges,
  tooltips, read-only treatment. No new UI needed there.
- Requirements #1 and #2 (manual reload button + toast) reduce to: one button (best placed
  inside the `claude-settings` tab context, not the header) that calls a new backend reload
  RPC, wired to the **existing** `showActionToast(message, "success"|"error", "claude-settings-reload")`
  call — no new toast component, no new accessibility work (the existing `role="alert"
  aria-live="polite"` already covers it).
- The one piece of real UX judgment needed: the success toast should reference the
  `claude-settings` tab (ideally with a delta count) rather than being a bare "Reloaded!", and
  the error toast should promise "previous rules still active" rather than surfacing a raw
  parse error inline.
- The larger gap is backend, not frontend: `LoadClaudeSettingsRules` is dead code and no file
  watcher exists at all, so the reload RPC itself and the watch mechanism are net-new backend
  work — the frontend piece really is "one small button and one toast call," as flagged as a
  possible outcome in the task brief.
