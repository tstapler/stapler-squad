# UX Research: VS Code Extension for Session Status & Workspace Navigation

Source requirements: `project_plans/vscode-extension/requirements.md`

## 1. Comparable UX Patterns in Similar VS Code Extensions

### GitLens — status bar as a low-friction, non-blocking info surface
GitLens's git-blame status bar item shows commit info for the current line without
opening any panel — a "quietly displays... without cluttering the code view" pattern
([12 GitLens Features](https://techcommunity.microsoft.com/blog/educatordeveloperblog/12-gitlens-features-that-revolutionized-my-coding-workflow-in-vs-code/4421891)).
Two takeaways for this extension's status bar item:
- **Click-to-drill-down, not click-to-act.** GitLens's status bar item is informational;
  deeper interaction happens via hover tooltip or a follow-up click. The proposed
  `⚡ 4 sessions  🔔 2 need review` item matches this: a single click opens the
  dashboard rather than trying to cram approve/deny into the status bar itself — keeps
  the status bar item a summary, not a control surface.
- **Tooltip customization matters more than the visible text.** GitLens exposes a
  `tooltipFormat` setting because the status bar's real estate is tiny; VS Code's
  `StatusBarItem.tooltip` (Markdown-capable) is the natural place for the per-reason
  breakdown (e.g. "2 approval-pending, 1 error, 1 stale") that won't fit in the label.

### GitHub Pull Requests extension — inline actions + query-driven tree grouping
The extension's tree view organizes PRs into labeled query buckets — the default
queries are "Waiting For My Review", "Assigned To Me", "Created By Me"
([microsoft/vscode-pull-request-github](https://github.com/microsoft/vscode-pull-request-github)).
It supports both a lightweight "virtual" review mode (comment without checking out
the branch) and a full checkout mode
([Working with GitHub in VS Code](https://code.visualstudio.com/docs/sourcecontrol/github)).
Takeaways:
- **Group tree items by what needs the developer's action first**, not just
  alphabetically/chronologically. For this extension, review-queue items (need
  Approve/Deny) should render as a distinct top section or be visually promoted above
  idle/running sessions — mirroring "Waiting For My Review" as the first, not last,
  query bucket.
- **Inline action buttons on tree items** (VS Code's `TreeItem.contextValue` +
  `view/item/context` menu contributions, rendered as inline icons via `"group":
  "inline"` in `package.json`) is exactly the mechanism needed for Approve/Deny
  buttons next to a queue item — no separate modal or webview required.
- **Optimistic-but-verified state transitions**: the PR extension updates the tree
  immediately after a review action but reflects failures by reverting/flagging the
  item rather than pretending success. This maps directly to AC-5 ("update the tree
  without a manual refresh") — the tree should optimistically remove/gray out an
  approved item, then reconcile against the next poll rather than silently trusting
  the local click.

### Docker / Remote-Containers extensions — status bar as connection/environment state
These extensions use the status bar to answer "what environment am I in / is the
daemon reachable" at a glance (container name, remote host, or a warning icon when
Docker isn't running) rather than data counts. The applicable pattern here is the
**distinct error/offline glyph** — Docker's status bar swaps to a warning icon rather
than silently showing an empty state when the daemon is unreachable. This directly
informs the "server unreachable" requirement below (section 4): the extension's
status bar must have a visually distinct disconnected state, not just `0 sessions`.

## 2. User Mental Model — Consistency with the Web Dashboard

Read `web-app/src/components/sessions/StatusBadge.tsx` and `ReviewQueueBadge.tsx` to
extract the vocabulary already trained into users of the web dashboard. The extension's
tree badges should reuse this vocabulary verbatim rather than inventing new labels/icons,
so a developer doesn't have to learn a second status taxonomy.

**`AttentionReason` labels** (`StatusBadge.tsx:15-37`): Approval Pending 🔒, Input
Required ✏️, Error ⚠️, Idle ⏰, Complete ✅, Uncommitted Changes 📝, Stale ⌛, Your Input
Needed ✏️.

**`DetectedStatus` labels** (`StatusBadge.tsx:39-68`): Ready ✅, Processing ⚙️, Needs
Approval 🔒, Input Required ✏️, Error ⚠️, Tests Failing ❌, Idle ⏰, Executing ⚡, Success
✅, Waiting for Agent ⏳.

**`Priority` labels** (`ReviewQueueBadge.tsx:23-81`): Urgent 🔴, High 🟡, Medium 🔵, Low
⚪, color-coded via CSS class per level, not just emoji.

Recommendation: the VS Code `TreeItem.label`/`description` for each session should
reuse the same label text and emoji as `getAttentionReasonInfo()` /
`getDetectedStatusInfo()` — e.g. a session with `NEEDS_APPROVAL` renders as
`🔒 Needs Approval` in the tree exactly as it would in the web card. Review-queue tree
items should reuse `getPriorityEmoji`/`getPriorityText` for the same reason. This is a
config/data mapping concern more than a design one — the RPC response already carries
the same enum values (`AttentionReason`, `DetectedStatus`, `Priority` from
`session/v1/types_pb`), so the extension's TypeScript client can port
`getAttentionReasonInfo`/`getDetectedStatusInfo`/priority-label logic nearly verbatim
instead of re-deriving it from scratch — reducing both dev effort and cross-product
drift risk.

## 3. Accessibility — Specific VS Code API Properties

VS Code's built-in controls (`TreeView`, `StatusBarItem`, `QuickPick`) get baseline
screen-reader support for free, but only when specific fields are populated — omitting
them leaves the control silent to assistive tech even though it renders correctly
visually.

- **`vscode.TreeItem.tooltip`** — Markdown or plain string shown on hover; also read by
  some screen readers as supplementary text. Set this per session (e.g. full status +
  last-activity timestamp) since the visible label/description is necessarily
  truncated in a narrow sidebar.
- **`vscode.TreeItem.accessibilityInformation`** — `{ label: string; role?: string }`,
  the primary API for screen-reader label + ARIA role on a tree node. This is
  independent of `label`/`description` (which are what's rendered) — VS Code
  recommends leaving `role` unset for normal tree items and only setting it when the
  item's rendering deviates from a standard tree row (e.g. a queue-item row with
  embedded Approve/Deny buttons, which is closer to a toolbar row than a plain list
  item) ([Tree View API docs](https://code.visualstudio.com/api/extension-guides/tree-view)).
  For review-queue items, set `accessibilityInformation.label` to the full sentence
  a sighted user assembles visually — e.g. `"Urgent priority, approval pending,
  session feature-x"` — since the visual row splits that across badge + label + inline
  buttons.
- **`vscode.StatusBarItem.tooltip`** — same Markdown-capable tooltip mechanism as
  `TreeItem`; use it for the per-reason breakdown (see section 1) rather than trying to
  fit everything in `text`.
- **`vscode.StatusBarItem.accessibilityInformation`** — same shape as `TreeItem`'s;
  needed because `StatusBarItem.text` may contain emoji/codicons that don't read
  cleanly ("lightning bolt 4 sessions bell 2") — set an explicit `label` such as
  `"4 active sessions, 2 need review"`.
- **`vscode.QuickPickItem.label`/`.description`/`.detail`** — for the "Open Session
  Worktree" quick-pick (AC-6), `label` is what's announced by default; put the
  session's status into `description` (announced next) rather than only `detail`
  (may not be read by all screen readers depending on VoiceOver/NVDA settings) —
  concretely: `label: sessionName`, `description: "🔒 Needs Approval"`.
- **Command palette entries** (`package.json` `contributes.commands`) — the `title`
  field doubles as the accessible name; keep the three command titles exactly as
  specified in the requirements (`Stapler Squad: Open Dashboard`, etc.) so they're
  unambiguous when read by a screen reader against other extensions' commands in the
  same palette list.

## 4. Error States

- **Server unreachable.** Per the Docker-extension pattern (section 1), the status bar
  must NOT collapse to the zero-state text (`⚡ 0 sessions`) when a poll fails — that
  is indistinguishable from "genuinely no active work" and would train the user to
  ignore the status bar during an actual outage. Use a distinct glyph/text, e.g.
  `⚠️ Stapler Squad unreachable` with `StatusBarItem.backgroundColor` set to
  `new vscode.ThemeColor('statusBarItem.warningBackground')`, and keep the last-known
  good counts out of the label entirely (don't show stale numbers next to a warning
  icon — that invites misreading a stale count as current). Tooltip should state the
  configured `serverUrl` and the last successful poll time so the user can self-diagnose
  (wrong port, service down, etc.) without leaving the editor.
- **No sessions active (legitimate empty state).** The tree view's `TreeDataProvider`
  should return a single explanatory node rather than an empty list — VS Code renders
  a fully empty tree as a blank panel with no explanation, which reads as broken, not
  quiet. Suggested copy: `"No active sessions — create one from the command palette"`
  with the `Stapler Squad: New Session in Current Folder` command wired as the tree
  item's `command` so it's a one-click affordance, not just informational text.
- **Approve/Deny action fails.** Per this repo's convention of never acting silently
  on AI/automation edge cases (`.claude/rules/` — "document AI decisions in edge
  cases" instinct extends to any automated state-changing action failing quietly),
  a failed approve/deny RPC must not just log to the Output channel and move on.
  Minimum bar: `vscode.window.showErrorMessage()` with the underlying RPC error message
  and a "Retry" action button, AND revert the optimistic tree update (per section 1's
  GitHub-PR-extension pattern) so the item's visible state matches server truth. Silent
  failure here is worse than in a "reference-only" feature because Approve/Deny mutate
  real session state — a developer who believes they denied an item that's still
  sitting in the queue is a correctness bug, not just a UX rough edge.

## 5. Jobs-to-be-Done (brief)

- **Functional**: "Let me see if anything needs me without alt-tabbing to the browser
  dashboard, and let me jump straight to the right worktree." Directly served by the
  status bar count + tree view + click-to-open-worktree (AC-2–4).
- **Emotional**: "Reduce background anxiety about whether something is waiting on me
  while I'm heads-down in an unrelated file." This is why the status bar's *offline*
  state (section 4) matters as much as its *populated* state — an extension that can
  silently go quiet during an outage actively undermines the anxiety-reduction goal
  it exists for.
- **Social**: N/A — single-user tool, no team/sharing dimension in scope.
