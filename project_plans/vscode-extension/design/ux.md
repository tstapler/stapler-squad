# UX Design: VS Code Extension for Session Status & Workspace Navigation

Source: `project_plans/vscode-extension/requirements.md`, `project_plans/vscode-extension/research/ux.md`,
`project_plans/vscode-extension/implementation/plan.md`.

This doc specifies interaction design for all native VS Code surfaces the extension renders.
No webview — every surface is a built-in control (`StatusBarItem`, `TreeView`, `QuickPick`,
`window.show*Message`, `contributes.configuration`), so accessibility, theming, and keyboard
navigation are inherited from VS Code itself rather than implemented here. Component names
below match the implementation plan (`StatusBarController`, `SessionTreeDataProvider`,
`SessionTreeItem`, `ApprovalTreeItem`, `PollScheduler`, `ConnectionState`, `NotifiedApprovalIds`,
`vscodeAdapter`) so this doc and the code stay traceable to each other.

---

## Surface 1 — Status Bar Item (`StatusBarController`)

### States

```
CONNECTED (normal)
┌──────────────────────────────────┐
│ ⚡ 3 sessions  🔔 2 need review   │  ← StatusBarItem.text
└──────────────────────────────────┘
   hover → tooltip (Markdown):
   ┌─────────────────────────────────────────┐
   │ **Stapler Squad** — localhost:8543       │
   │ 3 active sessions                        │
   │ 2 pending review (1 approval, 1 urgent)  │
   │ Last updated: 12:03:41 PM                │
   │ Click to open dashboard                  │
   └─────────────────────────────────────────┘

CONNECTED, zero activity (legitimate quiet state)
┌──────────────────────────────────┐
│ ⚡ 0 sessions                     │  ← no "need review" segment when depth is 0
└──────────────────────────────────┘

DISCONNECTED (server unreachable)
┌──────────────────────────────────────┐
│ ⚠️ Stapler Squad unreachable          │  ← statusBarItem.warningBackground
└──────────────────────────────────────┘
   hover → tooltip:
   ┌─────────────────────────────────────────┐
   │ **Stapler Squad** — cannot reach         │
   │ http://localhost:8543                    │
   │ Last successful poll: 12:01:12 PM        │
   │ (2 min 29 sec ago)                       │
   │ Click to open dashboard anyway           │
   └─────────────────────────────────────────┘

HIDDEN (staplerSquad.showStatusBar = false)
(nothing rendered — StatusBarController.hide() called synchronously
 from the config-change listener, no reload)
```

### Interaction flow

1. `PollScheduler` ticks every 8s (immediate first tick on activation) → calls `ListSessions` +
   `GetReviewQueue` → `computeStatusCounts()` → `StatusBarController.refresh(counts, "connected")`.
2. User hovers → tooltip shows the per-reason breakdown and last-updated timestamp (GitLens
   pattern from `research/ux.md` §1 — real estate is too tight for the breakdown in `.text`).
3. User clicks → `openDashboardCommand(config.serverUrl)` → `vscodeAdapter.openExternal(...)` →
   default browser opens `staplerSquad.serverUrl`. Same command as Story 5.1.1, so clicking the
   status bar and running "Stapler Squad: Open Dashboard" are identical, not two divergent paths.
4. User flips `staplerSquad.showStatusBar` in Settings → `onDidChangeConfiguration` fires →
   `StatusBarController.show()`/`.hide()` called immediately, no "reload window" prompt (AC-7).

### Error / edge cases

- **Poll tick throws (server down, wrong port, network blip).** `ConnectionState` flips to
  `"disconnected"` on the *first* failed tick — no debounce/grace period, because a stale
  "3 sessions" reading during a real outage is worse than a slightly twitchy warning glyph
  (`research/ux.md` §4). The warning glyph and background color replace the counts entirely;
  the last-known-good numbers are never shown next to a warning icon.
- **Recovery.** First successful tick after ≥1 failure flips back to `"connected"` and renders
  fresh counts immediately — no "still recovering" intermediate state, since a fresh successful
  RPC response is by definition current.
- **`serverUrl` is unreachable AND malformed (bad URL string).** `vscode.Uri.parse` on an invalid
  string throws synchronously on click; the click handler wraps this in the same
  `vscodeAdapter.showErrorMessage` pattern as Surface 5 below rather than throwing an unhandled
  extension-host error — "Invalid Stapler Squad server URL: `<value>`. Check `staplerSquad.serverUrl`
  in Settings" with an "Open Settings" action.

---

## Surface 2 — Sidebar Tree View (`SessionTreeDataProvider`)

### Populated state (approvals promoted above sessions)

```
STAPLER SQUAD (activity bar icon)
└── SESSIONS (view id: staplerSquadSessions)
    │
    │  ── pending approvals first (ApprovalTreeItem) ──
    ├── 🔒 fix-login-bug          Bash          [✓] [✕]   ← inline Approve/Deny icons
    ├── 🔒 refactor-api-client    Write         [✓] [✕]
    │
    │  ── then sessions (SessionTreeItem), current workspace marked ──
    ├── ● fix-login-bug           🔒 Needs Approval
    ├── ● refactor-api-client     ⚙️ Processing
    ├── ● add-dark-mode (current) ✅ Ready            ← isCurrentWorkspace styling
    └── ● cleanup-deps            ⌛ Stale
```

### Empty state (no sessions, no approvals)

```
STAPLER SQUAD
└── SESSIONS
    └── No active sessions — create one from the command palette
        (single explanatory tree item, `command`-bound to
         staplerSquad.newSessionInCurrentFolder — clicking it
         runs the command directly, not just descriptive text)
```

### Interaction flow

1. **Row content.** `SessionTreeItem.label = session title`, `.description = "${icon} ${label}"`
   sourced verbatim from `getDetectedStatusInfo`/`getAttentionReasonInfo` (ported from
   `StatusBadge.tsx` per the Pattern Decisions table) — a user who already knows the web
   dashboard's vocabulary (🔒 Needs Approval, ⚠️ Error, ⌛ Stale, etc.) reads the tree with zero
   relearning.
2. **Click a session row** → bound `command: staplerSquad.openSessionWorktree` fires with
   `resolveWorktreeUri(session)` → `vscodeAdapter.openFolder(uri, {forceNewWindow: false})` →
   worktree opens as the workspace root in the current window (AC-4). Opening in a *new* window
   is available via the "Open Session Worktree" QuickPick + VS Code's own folder-open modifier
   conventions, not a second click target on the row itself — keeps the row a single, predictable
   action (Krug: don't make the user think about which click does what).
3. **Approve/Deny (`ApprovalTreeItem`)** — two inline codicon buttons (`$(check)` / `$(close)`)
   via `view/item/context` with `group: "inline"`, scoped to `contextValue == "pendingApproval"`.
   Clicking Approve: `resolveApproval({approvalId, decision: "allow"})` fires, the row is removed
   from the tree **immediately** (optimistic — `SessionTreeDataProvider.optimisticallyRemoveApproval`),
   and the next poll tick's `ListPendingApprovals` reconciles state (item stays gone if the RPC
   truly succeeded). No separate confirmation dialog for Approve/Deny — GitHub PR extension
   precedent (`research/ux.md` §1) treats this as a low-cost, reversible-by-retry action, and
   requiring a modal per click would defeat the "approve without leaving the editor" goal.
4. **Refresh** is driven entirely by the shared `PollScheduler` tick (same interval as the status
   bar, per Task 3.3.1d) — there is no manual "refresh" button in v1; a stale-for-up-to-8-seconds
   tree is an accepted tradeoff of the polling architecture (Step 0.5 in the plan), not a UX gap
   to patch over with a spinner.

### Error / edge cases

- **Empty tree (zero sessions, zero approvals).** Never a blank panel — `getChildren()` returns
  the single explanatory, command-bound node (`research/ux.md` §4). This is the only tree state
  that doubles as an action shortcut rather than a status readout.
- **Approve/Deny RPC fails** (network drop, session already resolved server-side, stale approval
  ID). The optimistically-removed row is restored (`restoreApproval`) and
  `vscodeAdapter.showErrorMessage("Failed to approve appr-9: <error>", "Retry")` fires — see
  Surface 5 for the full notification contract. The tree never silently diverges from server
  truth; a developer who clicked Approve either sees it gone for good, or sees it come back with
  an explicit reason and a retry path.
- **`ListSessions`/`ListPendingApprovals` poll fails while the tree already has content.** The
  tree keeps showing its last-known-good state rather than clearing to empty — clearing a
  populated tree because of a transient poll failure would be indistinguishable from "all your
  sessions vanished," which is a worse failure mode than a few stale rows during a blip. (The
  status bar's stricter "never show stale counts" rule does NOT apply here — the status bar's
  problem is a bare number with no context that can be misread as current; a tree row with
  ⚠️/⌛ status icons and (implicitly, via the status bar going into its warning state
  simultaneously) an already-visible disconnected signal elsewhere in the UI is not the same
  ambiguity.) The `PollScheduler`'s failure is still logged to the OutputChannel per the
  Observability Plan regardless of which UI surface reacts to it.
- **`resolveWorktreeUri` fallback** (directory-type session, no `gitWorktree`). Same click
  behavior, just a different resolved path (`session.path`) — invisible to the user; there is no
  separate UI state for "this session has no worktree," since `session.path` is always a valid
  folder to open.

---

## Surface 3 — Command Palette (3 commands)

```
Ctrl+Shift+P → "stapler"
┌────────────────────────────────────────────────┐
│ Stapler Squad: Open Dashboard                    │
│ Stapler Squad: New Session in Current Folder     │
│ Stapler Squad: Open Session Worktree             │
└────────────────────────────────────────────────┘
```

### Interaction flow

| Command | Trigger → Result |
|---|---|
| `Stapler Squad: Open Dashboard` | Same as clicking the status bar item — opens `serverUrl` in the default browser. No dialog, no confirmation; instantaneous. |
| `Stapler Squad: New Session in Current Folder` | Zero workspace folders open → error message (see below). One folder → session created immediately with `title = folder basename`, `path = fsPath`, no prompt. 2+ folders → QuickPick of folder names/paths first (Surface 4), then create. |
| `Stapler Squad: Open Session Worktree` | Opens the QuickPick described in Surface 4 immediately (no intermediate step), regardless of how many sessions exist. |

Command titles are copied verbatim from the requirements doc (`Stapler Squad: Open Dashboard`,
etc.) — the `title` field doubles as the palette's accessible name, so consistent phrasing matters
for screen-reader users scanning a palette shared with dozens of other extensions' commands
(`research/ux.md` §3).

### Error / edge cases

- **"New Session in Current Folder" with zero workspace folders open.** `resolveTargetFolder`
  returns `undefined`; the command handler calls
  `vscodeAdapter.showErrorMessage("No folder is open. Open a folder or workspace first.")` — no
  QuickPick shown for an empty list, and no silent no-op. This is a dead end only in the sense
  that VS Code itself requires a folder to be open first; the message tells the user exactly what
  precondition is missing rather than doing nothing visible.
- **`createSession` RPC fails** (server down, invalid path, duplicate title). Same
  `showErrorMessage(..., "Retry")` contract as Surface 5 — re-running "New Session in Current
  Folder" is itself the retry action, so "Retry" re-invokes the same command with the same
  already-resolved folder (no need to re-prompt the QuickPick a second time for a 2+-folder
  workspace).
- **"Open Session Worktree" with zero sessions.** The QuickPick opens with VS Code's own built-in
  empty-list affordance ("No results found") — acceptable here (unlike the tree view's empty
  state) because the command was an explicit user action expecting a list, not a passively
  observed panel; VS Code's native empty-QuickPick treatment is a sufficient, already-familiar
  signal in this context.

---

## Surface 4 — QuickPick Flows

### 4a. "Open Session Worktree" (`openSessionWorktreeQuickPickCommand`)

```
Open Session Worktree
┌──────────────────────────────────────────────┐
│ Select a session to open its worktree          │
├──────────────────────────────────────────────┤
│ fix-login-bug            🔒 Needs Approval      │  ← label / description
│ refactor-api-client      ⚙️ Processing          │
│ add-dark-mode            ✅ Ready               │
│ cleanup-deps             ⌛ Stale               │
└──────────────────────────────────────────────┘
```

- `label = session title` (announced first by screen readers), `description = "${statusIcon}
  ${statusLabel}"` (announced next) — status intentionally goes in `description`, not `detail`,
  because `detail` isn't reliably read by all screen readers depending on VoiceOver/NVDA settings
  (`research/ux.md` §3). This is the one place in the whole extension where that distinction is
  load-bearing.
- Selecting an item invokes the exact same `openSessionWorktreeCommand(worktreeUri)` function the
  tree row's click uses (Task 3.3.2a) — one code path, two entry points, so behavior (including
  any future `autoOpenWorktree` fast-follow behavior) never drifts between the two triggers.
- Cancel (`Esc`) is a true no-op — no partial state change, nothing to revert.

### 4b. "New Session in Current Folder" — multi-folder resolution

```
(only shown when 2+ workspace folders are open)
Select a folder for the new session
┌──────────────────────────────────────────────┐
│ backend                  /home/dev/svc/backend │
│ frontend                 /home/dev/svc/frontend│
│ shared-libs               /home/dev/svc/shared │
└──────────────────────────────────────────────┘
```

- `label = folder name`, `description = fsPath` — reversed emphasis from 4a deliberately: here
  the *path* is the disambiguating detail (folder names can collide across a multi-root
  workspace with similarly-named checkouts elsewhere on disk), not a status badge.
- Cancel (`Esc`) aborts session creation entirely — no default/first-folder fallback, since
  silently picking a folder the user didn't select would create a session in the wrong directory
  with no visible error (a correctness issue, same class as Surface 5's approve/deny concern).

---

## Surface 5 — Notifications

### 5a. `notifyOnQueueItem` (new pending-approval toast)

```
┌──────────────────────────────────────────────────────┐
│ ℹ️  Stapler Squad: "fix-login-bug" needs your approval  │
│    for Bash                                             │
│                                          [Open]  [Dismiss]│
└──────────────────────────────────────────────────────┘
```

- Fired once per approval ID via `NotifiedApprovalIds.getNewIds()` — an item still pending on
  poll tick #2 does **not** re-notify; an item that resolves and later reappears (new approval,
  same session, different ID reuse edge case) is treated as new again since `reconcile()` clears
  resolved IDs from the tracking set (Task 4.3.1a/c).
- "Open" action jumps to the tree view (`vscode.commands.executeCommand("workbench.view.extension.staplerSquad")`)
  so the user can act on it inline rather than the notification itself trying to embed
  Approve/Deny buttons — VS Code notification action buttons are plain-text commands, not a good
  fit for a decision that should be visually correlated with the tool-input detail already shown
  in the tree row's tooltip.
- Respects `staplerSquad.notifyOnQueueItem = false` — no toast at all when disabled; the tree
  view and status bar still reflect the item, so disabling notifications never hides the
  underlying information, only the interruption.

### 5b. Approve/Deny failure

```
┌──────────────────────────────────────────────────────┐
│ ⚠️  Failed to approve appr-9: connect: ECONNREFUSED     │
│                                           [Retry]  [Dismiss]│
└──────────────────────────────────────────────────────┘
```

- Always paired with the tree-row revert described in Surface 2 — the notification and the
  visible tree state agree at every moment; there's no path where the toast says "failed" but
  the row is still gone (or vice versa).
- "Retry" re-invokes the exact same `approveItemCommand`/`denyItemCommand(approvalId, ...)` call
  — no re-navigation required, one click to retry.
- Also logged to the "Stapler Squad" OutputChannel (Observability Plan) so a dismissed toast
  doesn't destroy the only record of the failure — a user who dismisses without reading can still
  find it later via "Stapler Squad" in the Output panel dropdown.

### 5c. Non-localhost `serverUrl` warning (Story 5.2.2)

```
┌──────────────────────────────────────────────────────┐
│ ℹ️  Stapler Squad is configured to connect to a         │
│    non-local server (onyx.staplerhome.internal:8444).  │
│    This connection is unauthenticated.                  │
│                                        [Open Settings]  │
└──────────────────────────────────────────────────────┘
```

- Fires once per session (not per poll tick) the first time a non-localhost `serverUrl` is
  observed — informational, not blocking; the extension still connects. This is a safety nudge,
  not an error, since a legitimate non-local deployment is possible.

---

## Surface 6 — Settings (`contributes.configuration`)

```
Settings UI (Ctrl+,) → search "stapler squad"

Stapler Squad
  Server Url                                    [http://localhost:8543        ]
  ℹ  The base URL of your stapler-squad server.

  ☑ Show Status Bar
  ℹ  Show session count and review-queue depth in the status bar.

  ☐ Auto Open Worktree
  ℹ  When opening a session's worktree, automatically open its changed files.

  ☑ Notify On Queue Item
  ℹ  Show a notification when a new item needs your approval.
```

### Interaction flow

- All four settings live-apply with no reload prompt (AC-7): `onDidChangeConfiguration` is
  filtered to `affectsConfiguration("staplerSquad")` and updates behavior immediately —
  `showStatusBar` toggles visibility instantly (Surface 1), `serverUrl` changes are picked up on
  the *next* poll tick (not mid-flight — an in-flight RPC against the old URL is allowed to
  finish/fail on its own terms rather than being torn down mid-request), `notifyOnQueueItem` and
  `autoOpenWorktree` are read fresh by `getConfig()` on every poll tick / open action respectively
  (no caching per `research/pitfalls.md`).
- Descriptions in `package.json`'s `contributes.configuration` double as the Settings UI's `ℹ`
  hover/inline text — each description above states both what the setting does and, where
  relevant (Auto Open Worktree), what triggers it, so a user configuring this from Settings UI
  alone (never having read the requirements doc) can predict the behavior.

### Error / edge cases

- **Malformed `serverUrl` string** (e.g. missing scheme, typo). No inline JSON-schema validation
  beyond `type: "string"` — VS Code's settings UI doesn't support URL-format validation out of the
  box for a plain string setting. The failure surfaces reactively: the next poll tick fails,
  `ConnectionState` flips to `"disconnected"` (Surface 1), and the status bar tooltip shows the
  literal (malformed) configured value, letting the user spot the typo themselves. This is an
  accepted v1 gap, not silently unhandled — the disconnected state already exists as the single
  place this class of misconfiguration surfaces.

---

## UX Acceptance Criteria

Each is independently testable by a human exercising the running extension in the Extension
Development Host, without reading source.

### Task efficiency

1. **Open dashboard**: user can open the web dashboard in ≤ 1 click (status bar) or ≤ 2 keystrokes-to-selection (command palette: `Ctrl+Shift+P` → type → Enter).
2. **Approve/deny a pending item**: user can approve or deny a review-queue item in exactly 1 click from the sidebar, with no intermediate confirmation dialog, and no need to open the browser dashboard at all.
3. **Open a session's worktree**: user can open any active session's worktree folder in ≤ 1 click from the tree, or ≤ 3 steps via command palette + QuickPick (open palette → run command → select session).
4. **Create a new session from the current folder**: ≤ 1 step when a single folder is open (command runs immediately, no prompts); ≤ 2 steps (command + QuickPick selection) when multiple folders are open.

### State visibility (Nielsen: visibility of system status)

5. The status bar always shows one of exactly three states — live counts, a distinct disconnected warning, or hidden (per config) — and never shows numeric counts while `ConnectionState === "disconnected"`.
6. The sidebar tree never renders as a blank panel: it shows session rows, approval rows, or the single explanatory "No active sessions" node — one of the three, always.
7. Pending-approval rows are always visually and positionally distinguished from session rows (rendered first, distinct icon/contextValue) — a user scanning the tree top-to-bottom sees "what needs me" before "what's just running."
8. A user hovering the status bar item or a tree row can find, without clicking, enough detail (tooltip) to know *why* an item is in its current state — no status is presented as a bare label with no further explanation available.

### Error handling (Nielsen: help users recognize, diagnose, and recover from errors)

9. **Server unreachable**: status bar shows `⚠️ Stapler Squad unreachable` with `statusBarItem.warningBackground` styling; tooltip states the configured `serverUrl` and the last successful poll timestamp — this exact information, not a generic "error" string.
10. **Approve/Deny failure**: user sees `vscode.window.showErrorMessage` containing the approval ID and the underlying RPC error text, with an explicit **Retry** action button; the tree row that was optimistically removed is restored to its pre-click state within the same failure handler (no manual refresh needed to see the revert).
11. **New Session with no folder open**: user sees an explicit error message naming the missing precondition ("No folder is open...") — never a silent no-op when the command is invoked with nothing to act on.
12. **No dead ends**: every error state in this document (disconnected status bar, approve/deny failure toast, no-folder-open error, malformed serverUrl) has a stated exit path — a Retry action, an Open Settings action, a self-evident fix (open a folder), or (status bar) the passive fix of the server coming back up and the next poll tick auto-recovering. No error state requires the user to guess what to do next or restart the extension/window.

### Consistency (Nielsen: match with the real world / consistency and standards)

13. Every status icon+label pair rendered in the tree view (`SessionTreeItem.description`) is byte-identical to the corresponding pair in the web dashboard's `StatusBadge.tsx` for the same `DetectedStatus`/`AttentionReason` value — verified by the `statusInfo.test.ts` spot-checks in the plan (e.g. `NEEDS_APPROVAL` → `"🔒 Needs Approval"` in both places).
14. Command titles in the palette match the requirements doc's exact strings (`Stapler Squad: Open Dashboard`, `Stapler Squad: New Session in Current Folder`, `Stapler Squad: Open Session Worktree`) with no abbreviation or rewording.

### Accessibility

15. **Keyboard navigation**: every interactive surface (status bar item, tree rows, inline Approve/Deny icons, QuickPick items, command palette entries, Settings UI fields) is reachable and operable via keyboard alone, using only VS Code's built-in focus/arrow-key/Enter conventions — no custom key handling is introduced, so this is inherited for free from native controls, not something this extension can regress without deliberately overriding default behavior.
16. **Screen-reader labels present**: `SessionTreeItem.accessibilityInformation.label` states the full sentence a sighted user assembles visually (e.g. `"fix-login-bug, Needs Approval"`); `ApprovalTreeItem.accessibilityInformation.label` states `"Approval needed for Bash in fix-login-bug"` (tool + session, not just "approval needed"); `StatusBarItem.accessibilityInformation.label` states `"4 active sessions, 2 need review"` in place of the emoji-laden `.text`. None of these three are omitted for any row/state, including the disconnected status bar state and the empty-tree state.
17. **QuickPick screen-reader order**: in "Open Session Worktree," status information is in `description` (announced second, reliably read) not `detail` (unreliable across screen readers) — verified by inspecting `toQuickPickItems()`'s output shape, not just visually.
18. **Color is never the only signal**: the disconnected status bar state is distinguished by both icon (⚠️ vs ⚡) and text ("unreachable" vs a count) in addition to `statusBarItem.warningBackground` — a user with color-vision deficiency or a screen reader gets the same information as a sighted user relying on the background color alone.
19. **Contrast and theming**: because every surface is a native VS Code control (no custom CSS, no webview), color contrast automatically tracks the user's active VS Code theme (including high-contrast themes) with zero extension-specific styling code to audit — this is a structural property of the "no webview" architecture decision, not a claim requiring per-theme manual verification.

### Non-goals explicitly out of scope for this design (confirmed against requirements.md)

- No webview-hosted rich approval UI (tool-input diff view, syntax highlighting) — Approve/Deny tooltips show raw `toolInput` key/values only, matching Task 4.2.1a.
- No drag-and-drop reordering, no multi-select bulk-approve — one row, one action, one click, per requirement AC-5's scope.
- `autoOpenWorktree` (AC-8) is a fast-follow (Phase 8 in the plan); its interaction model (cap at `MAX_AUTO_OPEN_FILES = 10`, explicit-trigger-only, never from a poll tick) is specified in the plan and not re-litigated here since it doesn't ship in v1.
