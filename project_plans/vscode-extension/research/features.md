# Research: Feature Landscape for VS Code Extension

Scope note: this is a P3 feature per the requirements doc — kept research proportional, not exhaustive.

## 1. Status badges — what to mirror

`web-app/src/components/sessions/StatusBadge.tsx` is the canonical status→label/icon mapping and is
the thing the extension's tree-view badges must stay consistent with (AC-3: "matches the status
values already used by the web dashboard"). It's driven by two proto enums, not one:

- `AttentionReason` (why a session needs attention — `APPROVAL_PENDING`, `INPUT_REQUIRED`,
  `ERROR_STATE`, `IDLE_TIMEOUT`/`IDLE`, `TASK_COMPLETE`, `UNCOMMITTED_CHANGES`, `STALE`,
  `WAITING_FOR_USER`) via `getAttentionReasonInfo()`.
- `DetectedStatus` (what the agent is currently doing — `READY`, `PROCESSING`, `NEEDS_APPROVAL`,
  `INPUT_REQUIRED`, `ERROR`, `TESTS_FAILING`, `IDLE`, `EXECUTING`, `SUCCESS`,
  `WAITING_FOR_AGENT`) via `getDetectedStatusInfo()`.

Both are `@bufbuild/protobuf`-generated TS enums (`@/gen/session/v1/types_pb`), so the extension gets
them for free from the same generated bindings if it imports the shared proto package — it should
**not** hand-roll a parallel status→icon map. `getDetectedStatusInfo` ends its switch with
`assertNever(status)`, an exhaustiveness guard worth replicating (a plain TS `switch` on the enum with
a `default: assertNever(x)` — or the VS Code-side equivalent) so a future proto enum addition breaks
the extension's build instead of silently rendering nothing.

Session worktree path is `SessionInfo.worktree_path` (proto/session/v1/types.proto:1346, "Absolute
path to the worktree directory") — this is the field AC-4 ("opens that session's worktree path as a
VS Code workspace folder") should read, not `path` (multiple unrelated messages reuse that field name
at different offsets — `worktree_path` is the specific, unambiguous one for session worktrees).

## 2. Review/approval queue — two distinct concepts, requirements doc conflates them

The Open Questions section asks whether "review queue" maps 1:1 to
`list_approval_rules`/`submit_review_verdict`. It does not — there are **two separate systems** in
this codebase both loosely called "review":

**A. Tool-use approval queue (matches the issue's "Approve/Deny" language exactly)**
- RPCs: `ResolveApproval` (proto/session/v1/session.proto:122), `ListPendingApprovals` (:126).
- Frontend: `web-app/src/lib/hooks/useApprovals.ts` → `ApprovalsContext` → `approve(approvalId)` /
  `deny(approvalId, message?)`. Rendered by `ApprovalPanel.tsx` / `ApprovalCard.tsx`
  (`onApprove`/`onDeny` props, web-app/src/components/sessions/ApprovalCard.tsx:31-32).
- This is a live queue of **pending tool-permission prompts** ("Claude wants to run `rm -rf`, allow?")
  — binary approve/deny is the entire action surface, which matches AC-5's "Approve/Deny actions"
  wording precisely.
- Also has a real-time push path: `GetReviewQueue`/`WatchReviewQueue` RPCs
  (proto/session/v1/session.proto:52-54) surface a broader "review queue" of sessions needing any
  kind of attention (not just approval), consumed by `useReviewQueue.ts` — this is the "review-queue
  depth" number the status bar (AC-2) most likely wants, since it's a queue-depth/count concept, not
  a list of individual approve/deny actions.

**B. Backlog review verdict (PASS/FAIL/NEEDS_REWORK, not approve/deny)**
- MCP tool `submit_review_verdict`, backend `server/mcp/tools_backlog.go`,
  `server/services/backlog_service_triage.go`. Also `ApprovePlan`/`OverrideVerdict` RPCs
  (proto/session/v1/backlog.proto:807,813) for a different flow (planning-artifact approval, verdict
  override) in the autonomous backlog pipeline.
- `list_approval_rules`/`upsert_approval_rule`/`delete_approval_rule` (session.proto:145-152) are a
  third, unrelated concept: **auto-approval rule configuration** (rules that make tool-use approvals
  happen automatically without a human), not a queue of items to review.

**Recommendation for the plan phase:** AC-5 ("Approve/Deny actions that call the existing
approve/deny RPCs") should target `ResolveApproval`/`ListPendingApprovals` (system A), and the status
bar's "review queue depth" (AC-2) should read from `GetReviewQueue`'s `totalItems` — not the backlog
verdict system (B), which is a different domain (session PASS/FAIL grading) with no natural
"Approve/Deny" affordance. This should be called out explicitly as a plan-phase decision since the
requirements doc's Open Questions left it unresolved.

## 3. "New Session in Current Folder" → CreateSession mapping

Per `.claude/rules/session-creation-registry.md`, this does **not** need a new `SessionType` or new
touchpoints — it's a straightforward call into the existing flow:

- Proto: `SessionType_SESSION_TYPE_DIRECTORY` (already exists).
- Go: `server/services/session_service.go:1654-1655` maps it straight to
  `session.SessionTypeDirectory` — no new case needed.
- The extension supplies `path` = the currently-open VS Code workspace folder
  (`vscode.workspace.workspaceFolders[0].uri.fsPath` or a quick-pick if multiple folders are open),
  and `sessionType = SESSION_TYPE_DIRECTORY` in the `CreateSessionRequest`. This is exactly the
  "directory" mode the web UI already exposes ("Existing folder" in `OmnibarCreationPanel.tsx`'s
  `SESSION_TYPES`), just triggered from the editor's own cwd instead of a typed path.
- No proto change, no `make proto-gen`, no `session_service.go` switch-case edit — confirms
  AC-9 ("no impact on `server/services/`, `session/`, or `proto/`") holds for this specific command.

## 4. Competitive landscape (VS Code extensions for AI-agent orchestration)

Kept brief per the P3 scope note.

- **cmux** (native macOS terminal for parallel agent sessions) ships a companion VS Code extension
  that handles: sidebar status, lifecycle notifications, session naming, tool-activity display, and a
  mark-read/unread attention cycle. That "mark read/unread" affordance is a UX pattern worth
  borrowing for `notifyOnQueueItem` (see §6) — it avoids re-notifying on every poll for an item the
  user has already seen but not acted on.
- **Crystal → Nimbalyst** (Stravu) is a full desktop app (not primarily a VS Code extension) for
  running parallel Claude Code sessions in git worktrees, with diff visualization and uncommitted-
  change detection — this is the tool the requirements doc's non-goals section explicitly excludes
  ("streaming AI-agent edits directly into open editor buffers... that's the Nimbalyst/crystal model,
  not requested here").
- A third-party extension, **"Worktree Sessions for Claude Code"** (VS Code Marketplace,
  `vana123.vswt`), independently validates the exact shape of this feature: sidebar tree of git
  worktrees + Claude Code sessions, shows which are running, resume/create/diff/pull/push/PR/finish
  actions per worktree. Useful as a UX reference for the sidebar tree view's action set, though it's
  not integrated with stapler-squad's own session/worktree model.

No public source was reviewed in depth (all via WebSearch summaries) — if the plan phase wants exact
UX details (icon choices, tree-item context menus), a follow-up look at the `vana123.vswt` extension's
published source/screenshots would be worth the extra time; skipped here to stay proportional to a
P3 item.

## 5. Edge cases

**Multiple VS Code windows across different worktrees simultaneously.** The extension has no way to
know which worktree/session an open VS Code window "belongs to" except by comparing the workspace
folder's path against each session's `worktree_path` (see §1). If a workspace folder's path doesn't
match any session's `worktree_path`, the extension should not assume "no sessions" — it should still
show the *global* active-session count/review-queue depth in the status bar (that's what AC-2 asks
for), but the sidebar tree view's session list is inherently a **global list of all active sessions
across all worktrees**, not scoped to the current window. This matters because two VS Code windows
open on two different worktrees will each independently poll/stream the same server and show the same
global session list — which is correct (matches the web dashboard's list-all model) but worth stating
explicitly in the plan so it isn't mistaken for a bug. `list_workspace_peers` (see the mcp tool list)
is one existing signal for cross-worktree awareness the extension could optionally use to highlight
"this workspace" in the list, but nothing in the current RPC surface computes "which session is this
workspace folder" server-side — the extension has to do the path match client-side.

**Server not running (`localhost:8543` unreachable).** No existing web-app pattern to mirror 1:1
since the web app doesn't run detached from its own server. The extension should degrade the status
bar to something like `⚠ Stapler Squad unavailable` (VS Code convention:
`$(warning)`/`$(debug-disconnect)` codicon + `StatusBarItem.backgroundColor` set to
`new ThemeColor('statusBarItem.warningBackground')`) rather than showing a stale/zero count, and the
sidebar tree view should show a single explanatory node (not an empty list, which reads as "zero
sessions" rather than "can't reach the server"). `useWatchBacklogItems.ts`'s `connectionState`
state machine (`connecting`/`live`/`reconnecting`/`polling`/`stale`) is a good model to port
conceptually — the extension doesn't need the full gap-detection/backstop machinery (it's not
consuming a `WatchXxx` stream by default — see below) but should have at least a
connected/disconnected boolean driving that status bar affordance.

**Rapidly changing session count — polling vs. push.** The backend already exposes streaming RPCs:
`WatchSessions` (session.proto:28), `WatchReviewQueue` (session.proto:54), and `WatchBacklogItems`
(backlog.proto:924) — all server-streaming, and `useReviewQueue.ts`/`useWatchBacklogItems.ts` both
default to push-with-polling-fallback (WebSocket bridge for `WatchReviewQueue`, plain HTTP
server-streaming for `WatchBacklogItems`; both fall back to a 30s poll after 5 failed reconnects with
exponential backoff capped at 30s). VS Code extensions can consume server-streaming HTTP directly
(no browser-specific WebSocket-bridge requirement — `useWatchBacklogItems.ts`'s comment at
web-app/src/lib/hooks/useWatchBacklogItems.ts:175-180 notes it deliberately avoids the WS bridge
because plain Connect server-streaming over HTTP already works). For a P3, low-frequency-update
surface (a status bar count + occasional tree refresh), **plain polling on a longer interval
(e.g. 15-30s) is likely sufficient and much simpler to implement/test than porting the full
reconnect/backstop state machine** — the plan phase should treat streaming as a stretch goal, not
day-one scope, and flag that explicitly (mirroring how AC-8 flags `autoOpenWorktree` as a
nice-to-have).

**Auth.** Confirmed via `server/server.go:664-665`: *"Auth is provided by the existing middleware
chain: local HTTP = no auth; remote HTTPS = WebAuthn required."* `SetupAuth()` installs an
`authMiddleware` that is `nil` (disabled) for the local HTTP listener by default. The web-app's
`createAuthInterceptor()` (web-app/src/lib/config.ts:37-52) doesn't attach any credential — it's a
passive interceptor that redirects to `/login` only if the server itself returns
`Code.Unauthenticated`. **Conclusion: the extension talking to `http://localhost:8543` (the
`serverUrl` default) needs no credentials at all**, matching the web-app's own dev-flow assumption.
If a user points `serverUrl` at a remote HTTPS deployment (WebAuthn-protected), that's out of scope
for v1 per the same reasoning the requirements doc uses for other exclusions — worth a one-line
callout in the plan rather than solving WebAuthn-from-a-VS-Code-extension now.

## 6. Unstated user needs

- **Workspace-folder-to-session correlation** (expanded from §5): the issue as written implies the
  sidebar just lists "all active sessions," but a developer with one worktree open in the current VS
  Code window almost certainly wants *that* session visually distinguished (e.g. bolded / pinned to
  top / "Current Workspace" section) from the rest of the global list — otherwise the tree view is no
  more useful than the web dashboard tab they're trying to avoid switching to. This is implied by
  Goal 2 ("navigate directly from a session to its worktree folder") but not stated as its own
  acceptance criterion — worth surfacing as a plan-phase addition, not scope creep, since it's the
  difference between "a list" and "a workspace-aware list."
- **`notifyOnQueueItem` semantics.** The setting is declared (default `true`) but the issue doesn't
  say what "notify" means concretely. VS Code's native notification API is
  `vscode.window.showInformationMessage` (or `showWarningMessage` for errors/failures) — these are
  modal-lite, dismissible, and support action buttons (e.g. an inline "Approve"/"Deny" button pair
  directly on the notification, which would be a nice unstated-but-implied win matching AC-5's inline
  actions). The unstated part: **notification de-duplication**. Without a "seen" cursor (item ID +
  timestamp, or the cmux-style read/unread cycle noted in §4), a naive polling implementation will
  re-fire a notification for the same still-pending queue item on every poll tick — this needs an
  explicit dedup mechanism (track already-notified approval/queue-item IDs in extension global state,
  clear on resolve) called out in the plan, since it's the kind of gap that only surfaces once a
  reviewer/tester actually runs the extension against a queue item sitting unresolved for a few
  minutes.
