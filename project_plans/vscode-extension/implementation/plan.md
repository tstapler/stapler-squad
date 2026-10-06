# Implementation Plan: vscode-extension

**Feature**: VS Code extension surfacing session status, review-queue depth, and worktree navigation for stapler-squad, as a pure ConnectRPC API consumer with zero backend changes.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: ADR-001 (buf.gen.yaml dual TypeScript output) — `project_plans/vscode-extension/decisions/ADR-001-buf-gen-dual-typescript-output.md`

---

## Step 0.5 — Creative Pass: Architecture Alternatives

1. **Thin-adapter + interval polling** (chosen). Strength: reuses VS Code's native `TreeDataProvider`/`StatusBarItem` APIs directly, needs no new transport code beyond `@connectrpc/connect-node`, and AC-2's wording ("refreshing on an interval or via a push/poll mechanism") explicitly licenses it. Weakness: up to ~10s of staleness between a server-side change and the extension noticing it, and N open VS Code windows each poll independently (no shared cache).
2. **Full streaming-transport port** (port `web-app/src/lib/transport/watch-ws-transport.ts`'s hand-rolled WebSocket envelope to run inside the Node extension host). Strength: near-real-time updates, matches web-app's actual production data flow exactly. Weakness: that transport is hand-rolled specifically for a browser `WebSocket`/CORS environment; porting it to Node's `ws` package is a second bespoke transport implementation to build and maintain for a P3 backlog item — disproportionate lift documented in `research/stack.md`.
3. **Webview-based UI** (single HTML/JS panel rendered via `vscode.window.createWebviewPanel`, reusing web-app React components). Strength: could reuse `StatusBadge.tsx`/`ApprovalCard.tsx` components as-is instead of porting their logic. Weakness: throws away native `TreeView` accessibility, keyboard navigation, and theming for free; needs a CSP + postMessage bridge for zero functional gain over `TreeDataProvider`, which is explicitly ruled out in `research/pitfalls.md`.

**Chosen**: Alternative 1 (thin-adapter + interval polling). Alternatives 2 and 3 are recorded as rejected in the Pattern Decisions table below, and Alternative 2 is preserved as a named, out-of-scope fast-follow (Phase 8, Epic 8.2).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `SessionViewModel` | Plain object derived from a generated `Session` proto message, adding `statusLabel`/`statusIcon` and `isCurrentWorkspace`; the shape consumed by the tree view and status bar. | Built by `toSessionViewModel()`, a mapper function — not a class. |
| `ActiveSessionCount` | Count of sessions in the latest `ListSessions` poll with `status === SessionStatus.ACTIVE`. | Feeds the `⚡ N sessions` status bar segment. |
| `ReviewQueueDepth` | `ReviewQueue.totalItems` from the latest `GetReviewQueue` poll. | Feeds the `🔔 N need review` status bar segment. Distinct RPC family from `PendingApproval`. |
| `ConnectionState` | `"connected" \| "connecting" \| "disconnected"` — tracks the `PollScheduler`'s last RPC outcome. | Drives the status bar's muted/warning glyph; never show stale counts while `"disconnected"`. |
| `WorktreeUri` | A `vscode.Uri` resolved via `resolveWorktreeUri(session)` = `session.gitWorktree?.worktreePath ?? session.path`. | See Pattern Decisions — corrects an inaccurate research claim that `Session.worktree_path` exists directly; it does not (see `proto/session/v1/types.proto:71` and `:498`). |
| `PendingApprovalViewModel` | Plain object merging a `PendingApprovalProto` with its correlated `ReviewItem` (joined by `session_id`), used to render inline Approve/Deny rows. | Built by `toApprovalViewModel()` / `correlateApprovals()`. |
| `ApprovalDecision` | `"allow" \| "deny"` string literal type matching `ResolveApprovalRequest.decision`. | No enum exists on the wire for this — it's a raw string field. |
| `SessionTreeItem` | `vscode.TreeItem` subclass representing one session row. | `contextValue = "session"`. |
| `ApprovalTreeItem` | `vscode.TreeItem` subclass representing one `PendingApprovalViewModel` row. | `contextValue = "pendingApproval"` — drives the inline Approve/Deny menu group. |
| `SessionTreeDataProvider` | Implements `vscode.TreeDataProvider<SessionTreeItem \| ApprovalTreeItem>`; single source for the sidebar view, promotes pending-approval rows above idle/running sessions. | Required by VS Code's own Tree View API — not a discretionary pattern choice. |
| `PollScheduler` | Thin wrapper around `setInterval` that owns start/stop/`dispose()`, catches and logs every tick's errors to the `OutputChannel`, and is pushed into `context.subscriptions`. | The single disposal-leak guard point per `research/pitfalls.md`. |
| `VsCodeAdapter` | Module isolating every direct `vscode.window`/`vscode.workspace`/`vscode.commands`/`vscode.env` call so business logic (view-model mapping, correlation, formatting) has zero import-time dependency on the `vscode` module and can run under plain Jest. | Testability seam, not a GoF pattern — see Pattern Decisions. |
| `StaplerSquadConfig` | Plain object `{serverUrl, showStatusBar, autoOpenWorktree, notifyOnQueueItem}` read from `vscode.workspace.getConfiguration("staplerSquad")`, refreshed on every `onDidChangeConfiguration` event. | Never hardcode `localhost:8543` outside this module's default. |
| `NotifiedApprovalIds` | In-memory `Set<string>` of already-notified pending-approval IDs; an ID is added on first sight and removed once it's no longer present in a poll response. | De-dup mechanism for `notifyOnQueueItem` — prevents re-notifying every poll tick for the same still-pending item. |
| `CurrentWorkspaceMatch` | Boolean on `SessionViewModel` — true when `resolveWorktreeUri(session).fsPath` equals any open `vscode.workspace.workspaceFolders[].uri.fsPath`. | Visually distinguishes the session backing the currently-open editor window (unstated-but-real need flagged in `research/features.md`). |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall architecture | Thin-adapter + interval polling | Step 0.5 creative pass | Full streaming-transport port of `watch-ws-transport.ts` | Bespoke Node-side WS transport is a disproportionate build for a P3 feature when AC-2's wording already permits polling; deferred to Phase 8 as a named fast-follow. |
| Overall architecture | (same) | Step 0.5 creative pass | Webview-based UI | Loses native `TreeView` accessibility/theming/keyboard-nav for zero functional gain; `research/pitfalls.md` rules a webview out explicitly since no requirement needs one. |
| `vscode.*` isolation | Thin adapter / testability seam | `research/build-vs-buy.md`, `research/stack.md` | Direct `vscode.*` calls scattered through mapper/correlation logic | Would make AC-10's unit tests require `@vscode/test-electron` for every test instead of plain Jest, contradicting the "majority via plain Jest" testing split. |
| Proto DTO → view-model | Parse-at-boundary mapper function (`toSessionViewModel`), not a class | `type-driven-design` skill | A `SessionViewModel` class with getters wrapping the proto `Message` | No-op getter/setter smell (`.claude/rules/interface-pollution-checklist.md` item 3) — a plain function returning a plain object is simpler and needs no `vscode`/proto runtime to unit test. |
| Tree rendering | `vscode.TreeDataProvider<T>` (API-mandated) | VS Code Extension API | Custom webview list | Not optional — this is the API's own required interface for a native sidebar tree, and it's the only view surface this feature needs. |
| RPC client construction | Direct `createClient(SessionService, transport)` at call sites, no wrapper class | `.claude/rules/interface-pollution-checklist.md` | A `SessionApiClient` Manager/Service class forwarding every method 1:1 to the generated client | Forwarding-only wrapper smell (item 4) — the generated client is already the right shape; only wrap where real behavior (dedup, backoff) is added, which `PollScheduler` already owns separately. |
| Poll loop lifecycle | `PollScheduler` with explicit `dispose()`, pushed to `context.subscriptions` | `research/pitfalls.md` | Bare `setInterval` with no disposal path | Undisposed intervals are the most common VS Code extension leak per `research/pitfalls.md`; they keep firing (and polling a possibly-torn-down server) after `deactivate()`. |
| Approve/Deny mutation | Optimistic tree update + reconciliation on next poll tick, revert on RPC failure | `research/ux.md` | Blind-trust local mutation with no server re-verification | `research/ux.md`: AC-5 is a correctness-affecting mutation and must never silently diverge from server truth on failure. |
| Config live-apply | `workspace.onDidChangeConfiguration` listener updates in-memory `StaplerSquadConfig` immediately | `research/pitfalls.md` | Read config once at activation; require "reload window" | `research/pitfalls.md` confirms all four settings can live-apply with no VS Code API limitation forcing a reload — simplifies AC-7. |
| Status/label vocabulary | Port `getAttentionReasonInfo`/`getDetectedStatusInfo` verbatim into `src/viewModels/statusInfo.ts` | `research/ux.md`, `web-app/src/components/sessions/StatusBadge.tsx` | Re-derive/re-invent a parallel status→label map from scratch | Two independently-maintained status vocabularies (web-app's and the extension's) would drift the first time either is edited; porting the exact `switch` + `assertNever` guard keeps them mechanically identical. |
| `WorktreeUri` resolution | `session.gitWorktree?.worktreePath ?? session.path` mapper | `proto/session/v1/types.proto:71` (`Session.git_worktree`), `:498` (`GitWorktree.worktree_path`) — read directly during planning | Assume a top-level `Session.worktree_path` field, as `research/features.md` originally claimed (citing `types.proto:1346`) | **Correction found during planning**: `types.proto:1346` is `UnfinishedWorktree.worktree_path` — an unrelated message. `Session` itself has no top-level `worktree_path` field; the real path is nested under the optional `GitWorktree git_worktree = 20` field (present for `new_worktree`/`existing_worktree` sessions), with `Session.path` as the correct fallback for `directory`-type sessions where `path` already *is* the working directory. |

---

## Migration Plan

N/A — no schema/data changes. This feature is a pure ConnectRPC client running inside the VS Code extension host; it adds no database migrations, no proto *message/RPC schema* changes (only a build-output target, see ADR-001), and no changes to `session/`, `server/services/`, or `config/`. There is nothing to backfill and nothing that changes shape for existing sessions/backlog items/config files.

## Observability Plan

The extension's only observability surface is a single `vscode.OutputChannel` named **"Stapler Squad"** (`src/outputChannel.ts`), created once at activation and pushed into `context.subscriptions`. No telemetry/analytics pipeline is introduced. Concretely logged, one line per event:

- **Poll failures** — every `PollScheduler` tick that throws: RPC name (`ListSessions`/`GetReviewQueue`/`ListPendingApprovals`), error message, and the running consecutive-failure count (drives the `ConnectionState` transition to `"disconnected"`).
- **Reconnect recovery** — the first successful tick after ≥1 failed tick: logs total downtime duration since the last success.
- **Approve/Deny RPC errors** — logged in addition to the user-facing `showErrorMessage`, so a failure that the user dismisses is still recoverable from the channel.
- **Config changes applied** — which `staplerSquad.*` key changed and its new value on every `onDidChangeConfiguration` firing that this extension acts on (serverUrl is logged as-is since it's already visible in `settings.json`, not a secret).
- **Extension lifecycle** — one line each on `activate()` entry/exit and `deactivate()`.

## Risk Control

No feature flag is needed or appropriate: the extension is a separate, independently installable VS Code extension package (`extensions/vscode/`) that ships or does not ship independently of the main `stapler-squad` binary and web app. It has zero coupling to any running server flag or config gate (confirmed by AC-9 — it only ever calls existing, unmodified RPCs). Rollback is simply uninstalling the extension (or not installing/upgrading the `.vsix`) — there is no server-side state to roll back, and per the Migration Plan above there is nothing to migrate forward or backward.

## Unresolved Questions

1. **Packaging/distribution mechanism** (BLOCKING for Phase 7, Epic 7.2). `research/pitfalls.md` recommends a CI job building a `.vsix` on tag, attached to a GitHub Release, over publishing to the VS Code Marketplace / Open VSX (avoids publisher-account overhead for an internal tool). This plan defaults to that recommendation in Story 7.2.1. Owner: confirm before Phase 7 starts — if Marketplace/Open VSX publishing is actually wanted, Story 7.2.1's workflow needs a `vsce publish` step and a publisher account, which is a materially different setup (secrets, `publisher` field validation, versioning cadence) than a Release-attached artifact.
2. **`engines.node` / minimum VS Code version pin**. `web-app/package.json` has no `engines` field to mirror; `research/stack.md` recommends Node 22.x+ for `@vscode/vsce`. This plan defaults `extensions/vscode/package.json`'s `engines.vscode` to `^1.85.0` (a reasonably recent floor enabling `onStartupFinished` and current `TreeItem` APIs) — not verified against any stated org policy on minimum VS Code version. Confirm or adjust in Task 1.1.1a.

## Dependency Visualization

```
1.1 scaffold (package.json / tsconfig / esbuild)
        │
1.2 codegen wiring (buf.gen.yaml + ADR-001, generate once)
        │
1.3 RPC client + VsCodeAdapter + PollScheduler + config + OutputChannel
        │
        ├────────────────────┬─────────────────────────┐
        ▼                    ▼                          │
2.x status bar        3.x tree view: sessions            │ (parallel — both
   (AC-2)              (AC-3, AC-4)                       │  depend only on 1.3)
        │                    │                            │
        │              4.x review queue + approve/deny    │
        │              (AC-5, needs 3.x's TreeDataProvider)
        │                    │
        └─────────┬──────────┘
                   ▼
        5.x commands (AC-6) + configuration (AC-7)
        (needs status bar + tree view + RPC client wired)
                   │
                   ▼
        6.x tests (AC-10)
        (unit tests can start as soon as each unit in 1.x-5.x lands;
         smoke tests need full activation wiring from 5.x)
                   │
                   ▼
        7.x packaging (vsce + CI .vsix)
                   │
                   ▼  (stretch — does not block v1 completion)
        8.x autoOpenWorktree (AC-8) fast-follow
        8.x streaming fast-follow (documented only, no tasks)
```

---

## Phase 1: Foundation

### Epic 1.1: Package Scaffold & Build Tooling
**Goal**: A buildable, loadable extension skeleton exists before any feature code is written.

#### Story 1.1.1: Extension package builds and loads via Extension Development Host
**As a** stapler-squad developer, **I want** a working extension skeleton, **so that** every subsequent feature has a place to land and can be verified with F5.
**Acceptance Criteria**:
- AC-1: A VS Code extension package exists, builds, and loads via Extension Development Host without errors.
  - *Given* a fresh clone of the repo with `extensions/vscode/package.json`, `tsconfig.json`, and `src/extension.ts` scaffolded, *When* a developer runs `cd extensions/vscode && npm install && npm run compile` (esbuild bundle to `dist/extension.js`) and then presses F5 in VS Code with `extensions/vscode` open as the workspace root, *Then* a new Extension Development Host window launches with no error notifications, and the "Stapler Squad" OutputChannel shows an "extension activated" line.
**Files**: `extensions/vscode/package.json`, `extensions/vscode/tsconfig.json`, `extensions/vscode/esbuild.js`, `extensions/vscode/.vscodeignore`, `extensions/vscode/src/extension.ts`

##### Task 1.1.1a: Create package.json manifest (~5 min)
- Hand-write `extensions/vscode/package.json` (not `yo code`-generated): `name: "stapler-squad-vscode"`, `displayName: "Stapler Squad"`, `publisher: "tstapler"` (placeholder — confirm before Phase 7 per Unresolved Question 2), `engines: {"vscode": "^1.85.0"}`, `activationEvents: ["onStartupFinished"]`, `main: "./dist/extension.js"`, `packageManager: "pnpm@10.27.0"` (matches `web-app/package.json`), empty `contributes` object (filled in by later epics), scripts stub (`compile`, `watch`, `test`, `package` — bodies added in later tasks).
- Files: `extensions/vscode/package.json`

##### Task 1.1.1b: Create tsconfig.json (~3 min)
- Mirror `web-app/tsconfig.json`'s `strict: true`, `target: ES2020`, `moduleResolution: bundler`, add `"types": ["node", "vscode"]`, `"module": "commonjs"` (VS Code extension host requirement, unlike web-app's `esnext`), `outDir` unused (esbuild handles bundling; `tsc --noEmit` is the separate type-check step per `research/stack.md`).
- Files: `extensions/vscode/tsconfig.json`

##### Task 1.1.1c: Create esbuild bundler script + npm scripts (~5 min)
- `extensions/vscode/esbuild.js`: bundles `src/extension.ts` → `dist/extension.js`, `bundle: true`, `platform: "node"`, `format: "cjs"`, `external: ["vscode"]`, `minify` only in a `--production` flag branch.
- `package.json` scripts: `"compile": "node esbuild.js"`, `"watch": "node esbuild.js --watch"`, `"typecheck": "tsc --noEmit"`, `"pretest": "npm run compile && npm run typecheck"`.
- Files: `extensions/vscode/esbuild.js`, `extensions/vscode/package.json`

##### Task 1.1.1d: Create .vscodeignore and extend .gitignore (~2 min)
- `extensions/vscode/.vscodeignore`: excludes `src/**`, `node_modules/**` (except bundled deps esbuild already inlined), `**/*.map`, `.vscode-test/**` from the packaged `.vsix`.
- Add `extensions/vscode/dist/`, `extensions/vscode/node_modules/`, `extensions/vscode/*.vsix` to root `.gitignore` (generated code exception for `src/gen/` handled separately in Task 1.2.1b).
- Files: `extensions/vscode/.vscodeignore`, `.gitignore`

##### Task 1.1.1e: Create extension.ts activation stub (~3 min)
- `activate(context: vscode.ExtensionContext)` logs one line to the (not-yet-created, stubbed inline for now) output and does nothing else; `deactivate()` is a no-op. This is the seam later epics attach controllers/providers/commands to via `context.subscriptions.push(...)`.
- Files: `extensions/vscode/src/extension.ts`

---

### Epic 1.2: Proto Codegen Wiring (ADR-001)
**Goal**: The extension has typed ConnectRPC bindings for `SessionService`, generated the same way `web-app/` gets its own, with zero proto *schema* changes (AC-9).

#### Story 1.2.1: buf.gen.yaml emits TypeScript bindings into extensions/vscode/src/gen
**Acceptance Criteria**:
- AC-9: Extension has no impact on `server/services/`, `session/`, or proto message/RPC schemas — only a build-output target is added.
  - *Given* the implementation is complete, *When* `git diff --stat -- server/ session/ proto/session/v1/session.proto proto/session/v1/types.proto` is run, *Then* the diff is empty — the only proto-adjacent change in the whole feature is the new output block added to `buf.gen.yaml` in this story.
**Files**: `buf.gen.yaml`, `.gitignore`, `extensions/vscode/package.json`, `Makefile`

##### Task 1.2.1a: Add TypeScript output block to buf.gen.yaml (~3 min)
- Add a second `local` plugin block after the existing `web-app` one, per ADR-001: `local: extensions/vscode/node_modules/.bin/protoc-gen-es`, `out: extensions/vscode/src/gen`, same `opt` list as the web-app block.
- Files: `buf.gen.yaml`

##### Task 1.2.1b: Add extensions/vscode/src/gen to .gitignore (~2 min)
- Add `extensions/vscode/src/gen/` next to the existing `web-app/src/gen/` line (root `.gitignore:21`), matching the existing gitignored-but-force-tracked convention.
- Files: `.gitignore`

##### Task 1.2.1c: Add protoc-gen-es and @bufbuild/protobuf devDependencies (~2 min)
- Add `@bufbuild/protoc-gen-es` and `@bufbuild/protobuf` to `extensions/vscode/package.json`, pinned to the same versions as `web-app/package.json` (`@bufbuild/protobuf: ^2.11.0`).
- Files: `extensions/vscode/package.json`

##### Task 1.2.1d: Run make proto-gen and force-add generated output (~4 min)
- Run `pnpm install` in `extensions/vscode/` (so `node_modules/.bin/protoc-gen-es` exists), then `make proto-gen` from repo root; verify `extensions/vscode/src/gen/session/v1/{session_pb.ts,session_connect.ts,types_pb.ts,...}` are produced; `git add -f extensions/vscode/src/gen` to check the generated code in (mirrors `web-app/src/gen`'s tracked-despite-gitignored convention).
- Files: `extensions/vscode/src/gen/**` (generated, force-added)

##### Task 1.2.1e: Extend Makefile proto-gen staleness check (~3 min)
- Add one more `[ ! -f extensions/vscode/src/gen/session/v1/session_pb.ts ]` clause to the `if` condition in `proto-gen`'s target body (`Makefile:400-404`) so enabling this new output on an already-`.proto-gen.stamp`-touched tree doesn't silently skip regeneration for the new consumer.
- Files: `Makefile`

---

### Epic 1.3: RPC Client & Core Infrastructure
**Goal**: A shared, testable substrate — transport, config, output channel, adapter, poll loop — that every feature epic builds on.

#### Story 1.3.1: connect-node transport and SessionService client singleton
**Files**: `extensions/vscode/src/rpc/transport.ts`, `extensions/vscode/src/rpc/client.ts`, `extensions/vscode/package.json`

##### Task 1.3.1a: Add @connectrpc/connect and @connectrpc/connect-node dependencies (~2 min)
- Add `@connectrpc/connect: ^2.1.1` and `@connectrpc/connect-node: ^2.x` to `extensions/vscode/package.json` dependencies, matching web-app's pinned `@connectrpc/connect` major version (per `research/stack.md` — connect-node, not connect-web, since the extension host is a Node process).
- Files: `extensions/vscode/package.json`

##### Task 1.3.1b: Create transport.ts (~4 min)
- `getConnectTransport(baseUrl: string): Transport` using `createConnectTransport` from `@connectrpc/connect-node`, no interceptors initially (auth interceptor not needed — local loopback has no auth middleware, confirmed at `server/server.go` around the local-file-browser comment "local HTTP = no auth"). Not memoized as a module-level singleton keyed only by import (unlike web-app's `getConnectTransport()`) because `baseUrl` can change at runtime via config — memoize keyed by `baseUrl` instead.
- Files: `extensions/vscode/src/rpc/transport.ts`

##### Task 1.3.1c: Create client.ts (~3 min)
- `getSessionServiceClient(baseUrl: string)`: `createClient(SessionService, getConnectTransport(baseUrl))`, imported from generated `extensions/vscode/src/gen/session/v1/session_pb.ts`. Direct call, no wrapper class (Pattern Decisions row).
- Files: `extensions/vscode/src/rpc/client.ts`

#### Story 1.3.2: Config module and OutputChannel
**Files**: `extensions/vscode/src/config.ts`, `extensions/vscode/src/outputChannel.ts`

##### Task 1.3.2a: Create config.ts (~5 min)
- `getConfig(): StaplerSquadConfig` reads `vscode.workspace.getConfiguration("staplerSquad")` fresh on every call (never cached across the module — read on every tick per `research/pitfalls.md`'s "never hardcode, read from config every time" guard); `onConfigChanged(listener)` wraps `vscode.workspace.onDidChangeConfiguration`, filtering to `e.affectsConfiguration("staplerSquad")`.
- Files: `extensions/vscode/src/config.ts`

##### Task 1.3.2b: Create outputChannel.ts (~2 min)
- `getOutputChannel(): vscode.OutputChannel` — lazily creates and memoizes a single `vscode.window.createOutputChannel("Stapler Squad")`; `activate()` (Task 1.1.1e follow-up) pushes it into `context.subscriptions`.
- Files: `extensions/vscode/src/outputChannel.ts`

#### Story 1.3.3: Thin VsCodeAdapter (testability seam)
**Files**: `extensions/vscode/src/adapters/vscodeAdapter.ts`

##### Task 1.3.3a: Create vscodeAdapter.ts (~5 min)
- Exports narrow functions wrapping the direct `vscode.*` calls the rest of the extension needs: `openExternal(uri)`, `openFolder(uri, opts)`, `showErrorMessage(msg, ...actions)`, `showInformationMessage(msg)`, `getWorkspaceFolders()`. Each is a one-line pass-through — this module is the *only* place `import * as vscode` appears outside `extension.ts` and the `TreeDataProvider`/`StatusBarItem` classes VS Code's API itself requires to touch `vscode` directly.
- Files: `extensions/vscode/src/adapters/vscodeAdapter.ts`

#### Story 1.3.4: PollScheduler with disposal and error containment
**Files**: `extensions/vscode/src/poll/pollScheduler.ts`, `extensions/vscode/src/__tests__/pollScheduler.test.ts`

##### Task 1.3.4a: Create pollScheduler.ts (~5 min)
- `class PollScheduler` (concrete type, not an interface — single implementation): constructor takes `(tickFn: () => Promise<void>, intervalMs: number, log: (msg: string) => void)`; `start()` calls `tickFn` immediately then on `setInterval`; every tick wrapped in `try/catch` logging failures via `log(...)` (never an uncaught rejection escaping the interval callback, per `research/pitfalls.md`); `dispose()` clears the interval and implements `vscode.Disposable`.
- Files: `extensions/vscode/src/poll/pollScheduler.ts`

##### Task 1.3.4b: Unit test pollScheduler (~5 min)
- Jest `fake timers`: assert `tickFn` runs immediately and again after `intervalMs`; assert a rejected `tickFn` promise is caught and passed to `log`, and the interval keeps running afterward (not torn down by one failure); assert `dispose()` stops further ticks.
- Files: `extensions/vscode/src/__tests__/pollScheduler.test.ts`

---

## Phase 2: Status Bar (AC-2)

### Epic 2.1: Session/Review-Queue Polling → Status Bar
**Goal**: The status bar shows live counts, a distinct disconnected state, and opens the dashboard on click.

#### Story 2.1.1: ActiveSessionCount + ReviewQueueDepth computed each poll tick
**Files**: `extensions/vscode/src/viewModels/statusCounts.ts`, `extensions/vscode/src/__tests__/statusCounts.test.ts`

##### Task 2.1.1a: Create statusCounts.ts (~4 min)
- `computeStatusCounts(sessions: Session[], reviewQueue: ReviewQueue): {activeSessionCount: number; reviewQueueDepth: number}` — pure function, no `vscode` import, no RPC calls; `activeSessionCount` filters `sessions` by `status === SessionStatus.ACTIVE`, `reviewQueueDepth` is `reviewQueue.totalItems` directly.
- Files: `extensions/vscode/src/viewModels/statusCounts.ts`

##### Task 2.1.1b: Unit test statusCounts (~3 min)
- Given a mix of `SESSION_STATUS_ACTIVE`, `SESSION_STATUS_PAUSED`, `SESSION_STATUS_HIBERNATED` sessions and a `ReviewQueue{totalItems: 2}`, assert `activeSessionCount` counts only the active ones and `reviewQueueDepth` is `2`.
- Files: `extensions/vscode/src/__tests__/statusCounts.test.ts`

#### Story 2.1.2: StatusBarItem controller — live counts, disconnected state, click-to-dashboard
**Acceptance Criteria**:
- AC-2: Status bar shows live active-session count and review-queue depth, refreshing on an interval, and opens `staplerSquad.serverUrl` in the default browser on click.
  - *Given* a running stapler-squad server at `http://localhost:8543` with 4 sessions (3 `SessionStatus.ACTIVE`) and a `ReviewQueue.totalItems` of 2, *When* the `PollScheduler`'s 8-second tick calls `ListSessions` and `GetReviewQueue` and `computeStatusCounts` returns `{activeSessionCount: 3, reviewQueueDepth: 2}`, *Then* `StatusBarItem.text` reads `"⚡ 3 sessions  🔔 2 need review"`, and *When* the user clicks the status bar item, *Then* `vscodeAdapter.openExternal(vscode.Uri.parse("http://localhost:8543"))` is invoked.
  - *Given* the poll tick's `ListSessions` call throws a connection-refused error, *When* the tick's `catch` block sets `ConnectionState = "disconnected"`, *Then* `StatusBarItem.text` shows a distinct glyph (`"⚠️ Stapler Squad unreachable"`), `StatusBarItem.backgroundColor = new vscode.ThemeColor("statusBarItem.warningBackground")`, and the tooltip shows the configured `serverUrl` plus the last successful poll timestamp — never the stale `3 sessions` / `2 need review` text.
**Files**: `extensions/vscode/src/statusBar/statusBarController.ts`, `extensions/vscode/src/extension.ts`, `extensions/vscode/src/viewModels/statusBarFormat.ts`, `extensions/vscode/src/__tests__/statusBarFormat.test.ts`

##### Task 2.1.2a: Create statusBarController.ts — happy path (~5 min)
- `class StatusBarController`: owns a `vscode.StatusBarItem` (`vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100)`), a `refresh(counts, connectionState)` method setting `.text`/`.tooltip`/`.backgroundColor`, and a `command` bound to an internal `staplerSquad.openDashboardFromStatusBar` command id that calls `vscodeAdapter.openExternal`. `dispose()` implements `vscode.Disposable`.
- Files: `extensions/vscode/src/statusBar/statusBarController.ts`

##### Task 2.1.2b: Implement disconnected-state rendering (~4 min)
- Extend `refresh()` to branch on `ConnectionState`: `"connected"` renders the counts text; `"disconnected"` renders the warning glyph/background/tooltip described in the AC above and explicitly skips rendering any count.
- Files: `extensions/vscode/src/statusBar/statusBarController.ts`

##### Task 2.1.2d: Extract formatStatusBarState as a pure function (~4 min)
- `extensions/vscode/src/viewModels/statusBarFormat.ts` exports `formatStatusBarState(counts: {activeSessionCount: number; reviewQueueDepth: number}, connectionState: ConnectionState): {text: string; tooltip: string; backgroundColor?: "warning"}` — pure, no `vscode` import; returns the semantic string `"warning"` rather than a `vscode.ThemeColor` instance, letting `StatusBarController.refresh()` map it to the real `ThemeColor` at the call site. `refresh()` (Task 2.1.2b) is refactored to call this function instead of branching on `ConnectionState` inline, moving the disconnected-state logic out of a `vscode`-importing class so it's Jest-testable per AC-10.
- Files: `extensions/vscode/src/viewModels/statusBarFormat.ts`, `extensions/vscode/src/statusBar/statusBarController.ts`

##### Task 2.1.2e: Unit test formatStatusBarState (~3 min)
- Given `connectionState: "connected"` and `{activeSessionCount: 3, reviewQueueDepth: 2}`, assert `text === "⚡ 3 sessions  🔔 2 need review"` and no `backgroundColor`. Given `connectionState: "disconnected"`, assert `text === "⚠️ Stapler Squad unreachable"`, `backgroundColor === "warning"`, and the tooltip never includes the stale counts.
- Files: `extensions/vscode/src/__tests__/statusBarFormat.test.ts`

##### Task 2.1.2c: Wire PollScheduler + StatusBarController into extension.ts (~5 min)
- In `activate()`: construct `getSessionServiceClient(config.serverUrl)`, a `PollScheduler` whose `tickFn` calls `ListSessions`/`GetReviewQueue`, computes counts via `computeStatusCounts`, and calls `statusBarController.refresh(...)`; on failure calls `refresh(..., "disconnected")` and logs via `outputChannel`. Push both `PollScheduler` and `StatusBarController` into `context.subscriptions`.
- Files: `extensions/vscode/src/extension.ts`

#### Story 2.1.3: showStatusBar config toggle live-applies
**Files**: `extensions/vscode/src/extension.ts`

##### Task 2.1.3a: Wire config change listener to show/hide (~3 min)
- Register `onConfigChanged` (Task 1.3.2a) in `activate()`: when `showStatusBar` flips, call `statusBarController.show()`/`.hide()` immediately — no reload prompt.
- Files: `extensions/vscode/src/extension.ts`

---

## Phase 3: Sidebar Tree View — Sessions (AC-3, AC-4)

### Epic 3.1: Status Vocabulary Port
**Goal**: The tree view's status badges are byte-for-byte consistent with the web dashboard's, per the Pattern Decisions row banning a re-derived vocabulary.

#### Story 3.1.1: Port getAttentionReasonInfo / getDetectedStatusInfo
**Files**: `extensions/vscode/src/viewModels/statusInfo.ts`, `extensions/vscode/src/utils/assertNever.ts`, `extensions/vscode/src/__tests__/statusInfo.test.ts`

##### Task 3.1.1a: Create assertNever.ts (~1 min)
- Copy verbatim from `web-app/src/lib/utils/assertNever.ts` (3-line exhaustiveness-guard function — no vscode/proto dependency).
- Files: `extensions/vscode/src/utils/assertNever.ts`

##### Task 3.1.1b: Port statusInfo.ts (~5 min)
- Port `getAttentionReasonInfo(reason: AttentionReason)` and `getDetectedStatusInfo(status: DetectedStatus)` from `web-app/src/components/sessions/StatusBadge.tsx` lines 15-68, using this extension's own generated `AttentionReason`/`DetectedStatus` enums (`extensions/vscode/src/gen/session/v1/types_pb.ts`) instead of web-app's. Drop the `variant` field (no CSS classes in a tree view) — keep `label` and `icon` only, since `TreeItem.description` is plain text.
- Files: `extensions/vscode/src/viewModels/statusInfo.ts`

##### Task 3.1.1c: Unit test statusInfo against known enum values (~4 min)
- Assert `getDetectedStatusInfo(DetectedStatus.NEEDS_APPROVAL)` returns `{label: "Needs Approval", icon: "🔒"}` and `getAttentionReasonInfo(AttentionReason.UNCOMMITTED_CHANGES)` returns `{label: "Uncommitted Changes", icon: "📝"}` — spot-checking parity with `StatusBadge.tsx`'s cases per the ux.md vocabulary table.
- Files: `extensions/vscode/src/__tests__/statusInfo.test.ts`

### Epic 3.2: SessionViewModel + WorktreeUri Mapping

#### Story 3.2.1: toSessionViewModel mapper including CurrentWorkspaceMatch
**Files**: `extensions/vscode/src/viewModels/sessionViewModel.ts`, `extensions/vscode/src/adapters/worktreeUri.ts`, `extensions/vscode/src/__tests__/sessionViewModel.test.ts`, `extensions/vscode/src/__tests__/worktreeUri.test.ts`

##### Task 3.2.1a: Create worktreeUri.ts (~4 min)
- `resolveWorktreeUri(session: Session): string` — returns `session.gitWorktree?.worktreePath || session.path` (pure string function; the `vscode.Uri.file(...)` wrapping happens at the call site in the tree/command layer, not here, keeping this function `vscode`-free and Jest-testable).
- Files: `extensions/vscode/src/adapters/worktreeUri.ts`

##### Task 3.2.1b: Unit test worktreeUri (~3 min)
- Given a `Session` with `gitWorktree: {worktreePath: "/home/dev/.stapler-squad/worktrees/fix-login-bug"}`, assert that path wins over `session.path`; given a `Session` with `gitWorktree: undefined` and `path: "/home/dev/projects/myrepo"`, assert the fallback is used.
- Files: `extensions/vscode/src/__tests__/worktreeUri.test.ts`

##### Task 3.2.1c: Create sessionViewModel.ts (~5 min)
- `toSessionViewModel(session: Session, openWorkspaceFolders: string[]): SessionViewModel` — combines `getDetectedStatusInfo`/`getAttentionReasonInfo` (falling back sensibly when `detectedStatus` is `UNSPECIFIED`/`UNKNOWN`, matching `StatusBadge.tsx`'s own reason-then-detectedStatus precedence) with `resolveWorktreeUri` and `isCurrentWorkspace = openWorkspaceFolders.includes(resolveWorktreeUri(session))`.
- Files: `extensions/vscode/src/viewModels/sessionViewModel.ts`

##### Task 3.2.1d: Unit test sessionViewModel (~4 min)
- Given a plain `Session`-shaped object with `detectedStatus: DetectedStatus.ERROR` (AC-10's own example case) and `openWorkspaceFolders: ["/home/dev/projects/myrepo"]` matching `session.path`, assert `statusLabel === "Error"`, `statusIcon === "⚠️"`, and `isCurrentWorkspace === true`.
- Files: `extensions/vscode/src/__tests__/sessionViewModel.test.ts`

### Epic 3.3: TreeDataProvider + Click-to-Open

#### Story 3.3.1: SessionTreeDataProvider lists sessions with badges and an empty-state node
**Acceptance Criteria**:
- AC-3: Sidebar Tree View lists active sessions with a status badge per session, matching the web dashboard's status values.
  - *Given* `ListSessions` returns a `Session` with `id: "sess-42"`, `title: "fix-login-bug"`, `status: SessionStatus.ACTIVE`, `detectedStatus: DetectedStatus.NEEDS_APPROVAL`, *When* `SessionTreeDataProvider.getChildren()` builds its `SessionViewModel` via `toSessionViewModel`, *Then* the resulting `SessionTreeItem.label` is `"fix-login-bug"` and `.description` is `"🔒 Needs Approval"` — the exact icon+label pair `StatusBadge.tsx`'s `DetectedStatus.NEEDS_APPROVAL` case renders on the web dashboard.
  - *Given* `ListSessions` returns zero sessions, *When* `getChildren()` is called, *Then* a single explanatory `SessionTreeItem` reading `"No active sessions — create one from the command palette"` is returned, `command`-bound to `staplerSquad.newSessionInCurrentFolder` — never a blank panel.
**Files**: `extensions/vscode/src/tree/sessionTreeItem.ts`, `extensions/vscode/src/tree/sessionTreeDataProvider.ts`, `extensions/vscode/package.json`, `extensions/vscode/src/extension.ts`

##### Task 3.3.1a: Create sessionTreeItem.ts (~4 min)
- `class SessionTreeItem extends vscode.TreeItem`: constructor takes a `SessionViewModel`, sets `label`, `description` (`"${icon} ${label}"`), `tooltip` (Markdown-capable per `research/ux.md`, includes `context`/`detectedContext` when present), `contextValue = "session"`, and `accessibilityInformation = {label: "${title}, ${statusLabel}", role: "treeitem"}`.
- Files: `extensions/vscode/src/tree/sessionTreeItem.ts`

##### Task 3.3.1b: Create sessionTreeDataProvider.ts — sessions + empty state (~5 min)
- `class SessionTreeDataProvider implements vscode.TreeDataProvider<SessionTreeItem>`: `onDidChangeTreeData` backed by a private `vscode.EventEmitter`; `getChildren()` returns `sessions.map(toSessionViewModel).map(vm => new SessionTreeItem(vm))`, or the single empty-state `SessionTreeItem` per the AC above when the list is empty; `getTreeItem(el) { return el; }`; `refresh(sessions)` updates internal state and fires the emitter. (Approval-row promotion is added in Epic 4.2 — this task only handles the sessions list.) Disposal of this `EventEmitter` is wired in Task 3.3.1e, not here.
- Files: `extensions/vscode/src/tree/sessionTreeDataProvider.ts`

##### Task 3.3.1c: Register the view in package.json (~3 min)
- Add `contributes.viewsContainers.activitybar` (id `staplerSquad`, icon TBD-placeholder codicon) and `contributes.views.staplerSquad` (id `staplerSquadSessions`, name "Sessions").
- Files: `extensions/vscode/package.json`

##### Task 3.3.1d: Wire provider refresh to PollScheduler tick (~3 min)
- In `activate()`, call `sessionTreeDataProvider.refresh(sessions)` from the same `tickFn` that already updates the status bar (Task 2.1.2c), keeping both views on one shared poll rather than two independent intervals (mitigates the "thundering herd" risk `research/pitfalls.md` flags across multiple windows, at least within a single window).
- Files: `extensions/vscode/src/extension.ts`

##### Task 3.3.1e: Register the tree view in extension.ts and wire disposal (~4 min)
- `SessionTreeDataProvider` implements `vscode.Disposable`: add a `dispose()` method that disposes its internal `EventEmitter` (Task 3.3.1b). In `activate()`: `const treeView = vscode.window.createTreeView("staplerSquadSessions", { treeDataProvider: sessionTreeDataProvider })` (preferred over bare `registerTreeDataProvider` since it returns a disposable `TreeView` handle); push both `treeView` and `sessionTreeDataProvider` into `context.subscriptions`. Without this task, `contributes.views`/`contributes.viewsContainers` (Task 3.3.1c) declares the view but no provider is ever attached to it, so AC-3/AC-4 render nothing, and the `EventEmitter` backing `onDidChangeTreeData` leaks on deactivate.
- Files: `extensions/vscode/src/tree/sessionTreeDataProvider.ts`, `extensions/vscode/src/extension.ts`

#### Story 3.3.2: Click opens the session's worktree as a workspace folder
**Acceptance Criteria**:
- AC-4: Clicking a session opens its worktree path as a VS Code workspace folder.
  - *Given* a `SessionTreeItem` for a session whose `resolveWorktreeUri(session)` is `"/home/dev/.stapler-squad/worktrees/fix-login-bug"`, *When* the user clicks the tree row (triggering the bound command `staplerSquad.openSessionWorktree` with that path), *Then* `vscodeAdapter.openFolder(vscode.Uri.file("/home/dev/.stapler-squad/worktrees/fix-login-bug"), {forceNewWindow: false})` is called.
  - *Given* a `directory`-type session with no `gitWorktree` and `path: "/home/dev/projects/myrepo"`, *When* the same command runs, *Then* the fallback path `"/home/dev/projects/myrepo"` is used (per `resolveWorktreeUri`'s fallback, Task 3.2.1a).
**Files**: `extensions/vscode/src/commands/openSessionWorktree.ts`, `extensions/vscode/src/tree/sessionTreeItem.ts`, `extensions/vscode/package.json`, `extensions/vscode/src/__tests__/openSessionWorktree.test.ts`

##### Task 3.3.2a: Create openSessionWorktree.ts command handler (~4 min)
- `openSessionWorktreeCommand(worktreePath: string, forceNewWindow = false)`: calls `vscodeAdapter.openFolder(vscode.Uri.file(worktreePath), {forceNewWindow})`. Kept as a plain function taking a string (not a `SessionTreeItem`) so it's independently callable from the command-palette quick-pick flow in Epic 5.1 too.
- Files: `extensions/vscode/src/commands/openSessionWorktree.ts`

##### Task 3.3.2b: Bind SessionTreeItem.command and register in package.json (~3 min)
- Set `SessionTreeItem.command = {command: "staplerSquad.openSessionWorktree", title: "Open Worktree", arguments: [resolveWorktreeUri(session)]}` in the constructor (Task 3.3.1a follow-up edit); register `staplerSquad.openSessionWorktree` in `contributes.commands` and bind the handler in `extension.ts` via `vscode.commands.registerCommand`.
- Files: `extensions/vscode/src/tree/sessionTreeItem.ts`, `extensions/vscode/package.json`, `extensions/vscode/src/extension.ts`

##### Task 3.3.2c: Unit test the folder-open argument construction (~3 min)
- Mock `vscodeAdapter.openFolder`; assert `openSessionWorktreeCommand("/some/path")` calls it with `vscode.Uri.file("/some/path")` and `{forceNewWindow: false}`.
- Files: `extensions/vscode/src/__tests__/openSessionWorktree.test.ts`

---

## Phase 4: Review Queue & Approvals (AC-5)

### Epic 4.1: Approval Correlation
**Goal**: Join the two distinct RPC families — `ReviewItem` (from `GetReviewQueue`) and `PendingApprovalProto` (from `ListPendingApprovals`) — by `session_id`, per `research/architecture.md`'s explicit warning that they are different concepts.

#### Story 4.1.1: correlateApprovals joins PendingApprovalProto with ReviewItem
**Files**: `extensions/vscode/src/viewModels/approvalViewModel.ts`, `extensions/vscode/src/__tests__/approvalViewModel.test.ts`

##### Task 4.1.1a: Create approvalViewModel.ts (~5 min)
- `toApprovalViewModel(approval: PendingApprovalProto, reviewItem: ReviewItem | undefined): PendingApprovalViewModel` merges `approval.id`/`sessionId`/`toolName`/`toolInput`/`secondsRemaining` with `reviewItem?.sessionName` (falls back to `sessionId` when no `ReviewItem` correlates — e.g. `session_id: "unknown"` per the proto comment on `PendingApprovalProto.session_id`). `correlateApprovals(approvals: PendingApprovalProto[], reviewItems: ReviewItem[]): PendingApprovalViewModel[]` builds a `Map<string, ReviewItem>` keyed by `session_id` once, then maps each approval through it (avoids an O(n·m) nested loop).
- Files: `extensions/vscode/src/viewModels/approvalViewModel.ts`

##### Task 4.1.1b: Unit test correlation including the "unknown" session_id edge case (~4 min)
- Given one `PendingApprovalProto{sessionId: "sess-42"}` and a matching `ReviewItem{sessionId: "sess-42", sessionName: "fix-login-bug"}`, assert the merged view model's `sessionName === "fix-login-bug"`. Given a second approval with `sessionId: "unknown"` and no matching `ReviewItem`, assert the fallback path (`sessionName` falls back to `sessionId`) doesn't throw.
- Files: `extensions/vscode/src/__tests__/approvalViewModel.test.ts`

### Epic 4.2: Inline Approve/Deny with Optimistic Update

#### Story 4.2.1: ApprovalTreeItem rendered above sessions with inline Approve/Deny icons
**Files**: `extensions/vscode/src/tree/approvalTreeItem.ts`, `extensions/vscode/src/tree/sessionTreeDataProvider.ts`, `extensions/vscode/package.json`, `extensions/vscode/src/viewModels/buildTreeRows.ts`, `extensions/vscode/src/__tests__/buildTreeRows.test.ts`

##### Task 4.2.1a: Create approvalTreeItem.ts (~4 min)
- `class ApprovalTreeItem extends vscode.TreeItem`: constructor takes a `PendingApprovalViewModel`, `label = sessionName`, `description = "🔒 " + toolName` (e.g. `"🔒 Bash"`), `contextValue = "pendingApproval"`, `tooltip` including `toolInput` key/values, `accessibilityInformation = {label: "Approval needed for ${toolName} in ${sessionName}", role: "treeitem"}`.
- Files: `extensions/vscode/src/tree/approvalTreeItem.ts`

##### Task 4.2.1b: Promote approval rows above sessions in the tree ordering (~4 min)
- Extend `SessionTreeDataProvider.getChildren()`: prepend `correlateApprovals(...).map(vm => new ApprovalTreeItem(vm))` before the session rows (mirrors the GitHub PR extension's "Waiting For My Review" bucket-above-the-rest pattern per `research/ux.md`) — not alphabetical/chronological.
- Files: `extensions/vscode/src/tree/sessionTreeDataProvider.ts`

##### Task 4.2.1d: Extract buildTreeRows as a pure function (~4 min)
- `extensions/vscode/src/viewModels/buildTreeRows.ts` exports `buildTreeRows(sessions: SessionViewModel[], approvals: PendingApprovalViewModel[]): Array<SessionViewModel | PendingApprovalViewModel>` — pure ordering-only logic: approvals first, then sessions, regardless of input order. `SessionTreeDataProvider.getChildren()` (Task 4.2.1b) is refactored to call this function instead of prepending the mapped `ApprovalTreeItem`s inline, moving the ordering logic out of a `vscode`-importing class so it's Jest-testable per AC-10.
- Files: `extensions/vscode/src/viewModels/buildTreeRows.ts`, `extensions/vscode/src/tree/sessionTreeDataProvider.ts`

##### Task 4.2.1e: Unit test buildTreeRows (~3 min)
- Given a mix of `SessionViewModel[]` and `PendingApprovalViewModel[]` passed in arbitrary order, assert every approval row precedes every session row in the returned array, regardless of input ordering.
- Files: `extensions/vscode/src/__tests__/buildTreeRows.test.ts`

##### Task 4.2.1c: Add inline Approve/Deny menu contribution (~3 min)
- `contributes.menus["view/item/context"]`: two entries scoped to `viewItem == pendingApproval`, `group: "inline"`, commands `staplerSquad.approveItem` / `staplerSquad.denyItem`, each with a codicon (`$(check)` / `$(close)`).
- Files: `extensions/vscode/package.json`

#### Story 4.2.2: Approve/Deny commands — optimistic update, revert on failure
**Acceptance Criteria**:
- AC-5: Review-queue items render inline with Approve/Deny actions that call the existing RPCs and update the tree without a manual refresh.
  - *Given* a `PendingApprovalViewModel` with `id: "appr-9"`, `sessionId: "sess-42"`, `toolName: "Bash"`, correlated to a `ReviewItem` with `reason: AttentionReason.APPROVAL_PENDING` for the same `session_id`, *When* the user clicks the inline Approve icon on that `ApprovalTreeItem`, *Then* `resolveApproval({approvalId: "appr-9", decision: "allow"})` is called, the tree immediately removes that `ApprovalTreeItem` (optimistic), and the following poll tick's `ListPendingApprovals` confirms `"appr-9"` is gone (reconciliation, no manual refresh needed).
  - *Given* the same click but `resolveApproval` rejects with a network error, *When* the `catch` handler runs, *Then* the `ApprovalTreeItem` is restored to the tree and `vscodeAdapter.showErrorMessage("Failed to approve appr-9: <error>", "Retry")` is shown — never fails silently.
**Files**: `extensions/vscode/src/commands/approveDeny.ts`, `extensions/vscode/src/tree/sessionTreeDataProvider.ts`, `extensions/vscode/src/__tests__/approveDeny.test.ts`, `extensions/vscode/package.json`

##### Task 4.2.2a: Create approveDeny.ts command handlers (~5 min)
- `approveItemCommand(approvalId: string)` / `denyItemCommand(approvalId: string, message?: string)`: call `resolveApproval({approvalId, decision: "allow" | "deny", message})` via the shared `SessionService` client; each returns a `Promise<{success: boolean}>` for the caller (the tree provider) to react to.
- Files: `extensions/vscode/src/commands/approveDeny.ts`

##### Task 4.2.2b: Wire optimistic removal + reconciliation into sessionTreeDataProvider (~5 min)
- `SessionTreeDataProvider.optimisticallyRemoveApproval(approvalId)` removes the item from in-memory state and fires the change emitter immediately, before the RPC resolves; the next `refresh(sessions, approvals)` call (from the regular poll tick) is the reconciliation step — it always replaces state wholesale from the latest server response, so a since-reverted optimistic removal self-heals on the very next tick.
- Files: `extensions/vscode/src/tree/sessionTreeDataProvider.ts`

##### Task 4.2.2c: Wire failure handling — revert + showErrorMessage with Retry (~4 min)
- In `extension.ts`'s command registration for `staplerSquad.approveItem`/`denyItem`: on `approveItemCommand`/`denyItemCommand` rejection, call `sessionTreeDataProvider.restoreApproval(approvalViewModel)` (re-adds the item, fires the emitter) and `vscodeAdapter.showErrorMessage(...)` with a `"Retry"` action that re-invokes the same command.
- Files: `extensions/vscode/src/extension.ts`

##### Task 4.2.2d: Unit test approveDeny success/failure paths (~4 min)
- Mock the `SessionService` client's `resolveApproval`: assert `approveItemCommand("appr-9")` calls it with `{approvalId: "appr-9", decision: "allow"}`; assert a rejected mock propagates the rejection for the caller (`extension.ts`'s wiring) to catch — this test verifies the command function's contract, not the VS Code-side revert (covered by a `@vscode/test-cli` smoke test in Phase 6 if time allows).
- Files: `extensions/vscode/src/__tests__/approveDeny.test.ts`

### Epic 4.3: notifyOnQueueItem De-dup

#### Story 4.3.1: NotifiedApprovalIds prevents re-notifying the same still-pending item every poll tick
**Files**: `extensions/vscode/src/notify/notifiedApprovalIds.ts`, `extensions/vscode/src/extension.ts`, `extensions/vscode/src/__tests__/notifiedApprovalIds.test.ts`

##### Task 4.3.1a: Create notifiedApprovalIds.ts (~4 min)
- `class NotifiedApprovalIds`: wraps an in-memory `Set<string>`; `getNewIds(currentApprovalIds: string[]): string[]` returns IDs present in `currentApprovalIds` but not yet in the set, and adds them to the set as a side effect; `reconcile(currentApprovalIds: string[])` removes any tracked ID no longer present in `currentApprovalIds` (i.e. it was resolved), so a same-ID approval re-created later would re-notify correctly.
- Files: `extensions/vscode/src/notify/notifiedApprovalIds.ts`

##### Task 4.3.1b: Wire into the poll tick (~3 min)
- In `extension.ts`'s `tickFn`, after fetching `ListPendingApprovals`, call `notifiedApprovalIds.getNewIds(...)` and `vscodeAdapter.showInformationMessage(...)` once per new ID (only when `config.notifyOnQueueItem` is true), then `notifiedApprovalIds.reconcile(...)`.
- Files: `extensions/vscode/src/extension.ts`

##### Task 4.3.1c: Unit test de-dup logic (~4 min)
- Given `getNewIds(["a", "b"])` then `getNewIds(["a", "b"])` again, assert the second call returns `[]` (no re-notify). Given `reconcile(["a"])` after `b` was tracked, then `getNewIds(["a", "b"])`, assert `"b"` is returned again (it was cleared on resolve, so it's treated as new if it reappears).
- Files: `extensions/vscode/src/__tests__/notifiedApprovalIds.test.ts`

---

## Phase 5: Commands & Configuration (AC-6, AC-7)

### Epic 5.1: Command Palette
**Goal**: All three required commands exist, function per their descriptions, and are discoverable via Ctrl+Shift+P.

#### Story 5.1.1: Open Dashboard command
**Acceptance Criteria**:
- AC-6 (Open Dashboard): *Given* `staplerSquad.serverUrl` is `"http://localhost:8543"`, *When* the user runs "Stapler Squad: Open Dashboard" from the command palette, *Then* `vscodeAdapter.openExternal(vscode.Uri.parse("http://localhost:8543"))` is called — the same effect as clicking the status bar item (Story 2.1.2).
**Files**: `extensions/vscode/src/commands/openDashboard.ts`, `extensions/vscode/package.json`

##### Task 5.1.1a: Create openDashboard.ts (~2 min)
- `openDashboardCommand(serverUrl: string)`: calls `vscodeAdapter.openExternal(vscode.Uri.parse(serverUrl))`. Reused by both the status bar click handler (Task 2.1.2a, refactored to call this) and the new command.
- Files: `extensions/vscode/src/commands/openDashboard.ts`

##### Task 5.1.1b: Register command in package.json + commandPalette menu (~2 min)
- `contributes.commands`: `{command: "staplerSquad.openDashboard", title: "Stapler Squad: Open Dashboard"}`; no explicit `contributes.menus.commandPalette` entry needed (VS Code ≥1.74 auto-infers palette visibility from `contributes.commands`, per `research/pitfalls.md`).
- Files: `extensions/vscode/package.json`

#### Story 5.1.2: New Session in Current Folder command
**Acceptance Criteria**:
- AC-6 (New Session): *Given* the command palette is open in a window with `vscode.workspace.workspaceFolders[0].uri.fsPath === "/home/dev/projects/myrepo"`, *When* the user selects "Stapler Squad: New Session in Current Folder", *Then* `createSession({title: "myrepo", path: "/home/dev/projects/myrepo", sessionType: SessionType.SESSION_TYPE_DIRECTORY})` is called via the RPC client (per `server/services/session_service.go`'s existing handling of `SESSION_TYPE_DIRECTORY` — no new backend touchpoint needed per `.claude/rules/session-creation-registry.md`, confirmed in `research/features.md`).
  - *Given* multiple workspace folders are open, *When* the same command runs, *Then* a `QuickPick` of folder paths is shown first, and the selected folder's basename becomes `title` and fsPath becomes `path`.
**Files**: `extensions/vscode/src/commands/newSessionInCurrentFolder.ts`, `extensions/vscode/package.json`, `extensions/vscode/src/__tests__/newSessionInCurrentFolder.test.ts`

##### Task 5.1.2a: Create newSessionInCurrentFolder.ts — folder resolution (~5 min)
- `resolveTargetFolder(workspaceFolders: {name: string; uri: {fsPath: string}}[]): {name: string; fsPath: string} | undefined` — pure function: returns `undefined` for zero folders (caller shows an error), returns the single folder directly for one, and (for 2+) is called by the command handler which shows a `QuickPickItem[]` (label = folder name, description = fsPath) and resolves to the user's pick.
- Files: `extensions/vscode/src/commands/newSessionInCurrentFolder.ts`

##### Task 5.1.2b: Wire createSession RPC call (~4 min)
- `newSessionInCurrentFolderCommand()`: gets `vscodeAdapter.getWorkspaceFolders()`, resolves the target via Task 5.1.2a (prompting via QuickPick when needed), calls `createSession({title: basename(fsPath), path: fsPath, sessionType: SessionType.SESSION_TYPE_DIRECTORY})`, shows a confirmation `showInformationMessage` on success.
- Files: `extensions/vscode/src/commands/newSessionInCurrentFolder.ts`, `extensions/vscode/package.json` (register command)

##### Task 5.1.2c: Unit test folder-resolution logic (~3 min)
- Given zero folders, assert `undefined`. Given one folder, assert it's returned directly with no prompt needed. Given two folders, assert the function signature supports the caller doing its own QuickPick (this test only covers the pure resolver, not the QuickPick UI itself).
- Files: `extensions/vscode/src/__tests__/newSessionInCurrentFolder.test.ts`

#### Story 5.1.3: Open Session Worktree quick-pick command
**Acceptance Criteria**:
- AC-6 (Open Session Worktree): *Given* the current `SessionViewModel[]` includes `{title: "fix-login-bug", statusLabel: "Needs Approval", statusIcon: "🔒", worktreeUri: "/home/dev/.stapler-squad/worktrees/fix-login-bug"}`, *When* the user runs "Stapler Squad: Open Session Worktree" and selects that item from the `QuickPick` (label = `"fix-login-bug"`, description = `"🔒 Needs Approval"` per `research/ux.md`'s guidance to put status in `description` not `detail`, which some screen readers skip), *Then* `openSessionWorktreeCommand("/home/dev/.stapler-squad/worktrees/fix-login-bug")` (Task 3.3.2a) is invoked — reusing the exact same open-folder flow as clicking the tree row.
**Files**: `extensions/vscode/src/commands/openSessionWorktreeQuickPick.ts`, `extensions/vscode/package.json`, `extensions/vscode/src/__tests__/openSessionWorktreeQuickPick.test.ts`

##### Task 5.1.3a: Create openSessionWorktreeQuickPick.ts (~4 min)
- `toQuickPickItems(sessions: SessionViewModel[]): vscode.QuickPickItem[]` — pure mapping: `label = title`, `description = "${statusIcon} ${statusLabel}"`. `openSessionWorktreeQuickPickCommand()` fetches the latest sessions (via the shared client), shows the QuickPick, and on selection calls `openSessionWorktreeCommand(selected.worktreeUri)`.
- Files: `extensions/vscode/src/commands/openSessionWorktreeQuickPick.ts`

##### Task 5.1.3b: Register command in package.json (~2 min)
- `contributes.commands`: `{command: "staplerSquad.openSessionWorktreeQuickPick", title: "Stapler Squad: Open Session Worktree"}`.
- Files: `extensions/vscode/package.json`

##### Task 5.1.3c: Unit test QuickPickItem construction (~3 min)
- Given a `SessionViewModel[]` with one entry as in the AC above, assert `toQuickPickItems` produces `label: "fix-login-bug"`, `description: "🔒 Needs Approval"`.
- Files: `extensions/vscode/src/__tests__/openSessionWorktreeQuickPick.test.ts`

### Epic 5.2: Configuration Contribution

#### Story 5.2.1: package.json contributes.configuration for all four settings, live-apply
**Acceptance Criteria**:
- AC-7: All four `staplerSquad.*` settings are declared with the correct defaults and take effect without a reload.
  - *Given* `package.json`'s `contributes.configuration` declares `staplerSquad.serverUrl` (default `"http://localhost:8543"`), `staplerSquad.showStatusBar` (default `true`), `staplerSquad.autoOpenWorktree` (default `false`), `staplerSquad.notifyOnQueueItem` (default `true`), *When* a developer edits `settings.json` to set `"staplerSquad.showStatusBar": false` and saves, *Then* `onConfigChanged`'s listener fires, `getConfig()` returns the new value, and `statusBarController.hide()` is called immediately — no "reload window" notification appears at any point.
**Files**: `extensions/vscode/package.json`, `extensions/vscode/src/config.ts`

##### Task 5.2.1a: Add contributes.configuration block (~4 min)
- Add the four settings with `type`, `default`, and `description` per the requirements doc's table; `serverUrl: {type: "string", default: "http://localhost:8543"}`, `showStatusBar: {type: "boolean", default: true}`, `autoOpenWorktree: {type: "boolean", default: false}`, `notifyOnQueueItem: {type: "boolean", default: true}`.
- Files: `extensions/vscode/package.json`

##### Task 5.2.1b: Wire autoOpenWorktree + notifyOnQueueItem reads into config.ts (~2 min)
- Confirm `StaplerSquadConfig` (Task 1.3.2a) already reads all four keys — this task is the verification/completion pass ensuring `autoOpenWorktree`/`notifyOnQueueItem` (used by Phase 4's Epic 4.3 and Phase 8's Epic 8.1) are present in the type and read correctly, not just `serverUrl`/`showStatusBar`.
- Files: `extensions/vscode/src/config.ts`

#### Story 5.2.2: Non-localhost serverUrl warning guard
**Files**: `extensions/vscode/src/config.ts`, `extensions/vscode/src/__tests__/config.test.ts`

##### Task 5.2.2a: Add one-time non-blocking warning (~4 min)
- In `config.ts`, on first `getConfig()` call (or first config-change event) where `new URL(serverUrl).hostname` is not `"localhost"`/`"127.0.0.1"`/`"::1"`, call `vscodeAdapter.showInformationMessage(...)` once (not on every poll tick) warning the user their `serverUrl` points at a non-local host — guards against e.g. pasting the WebAuthn-protected mobile endpoint (`https://onyx.staplerhome.internal:8444`, per user memory) into a config field that has no auth handling.
- Files: `extensions/vscode/src/config.ts`

##### Task 5.2.2b: Unit test the guard (~3 min)
- Given `serverUrl: "http://localhost:8543"`, assert no warning fires. Given `serverUrl: "https://onyx.staplerhome.internal:8444"`, assert the warning fires exactly once even across repeated `getConfig()` calls.
- Files: `extensions/vscode/src/__tests__/config.test.ts`

---

## Phase 6: Testing Infrastructure (AC-10)

### Epic 6.1: Jest Unit Test Harness

#### Story 6.1.1: Own jest.config.js, not wired into web-app's
**Acceptance Criteria**:
- AC-10: Basic Jest test coverage exists for the extension's data-fetching/status-formatting logic.
  - *Given* the pure-logic modules built across Phases 1-5 (`statusInfo.ts`, `sessionViewModel.ts`, `worktreeUri.ts`, `approvalViewModel.ts`, `statusCounts.ts`, `statusBarFormat.ts`, `buildTreeRows.ts`, `pollScheduler.ts`, `notifiedApprovalIds.ts`, `config.ts`'s guard, the command-arg-construction functions), *When* `cd extensions/vscode && npx jest --no-coverage` runs, *Then* every one of those modules' test files passes, none of them importing the `vscode` module.
**Files**: `extensions/vscode/jest.config.js`, `extensions/vscode/package.json`

##### Task 6.1.1a: Create jest.config.js (~4 min)
- `preset: "ts-jest"`, `testEnvironment: "node"` (no jsdom needed — this is a Node extension host, not a browser), `roots: ["<rootDir>/src"]`, `testMatch: ["**/__tests__/**/*.test.ts"]`, own `moduleNameMapper` only if needed (no CSS files to mock, unlike web-app's config) — deliberately not extending or importing `web-app/jest.config.js`, per `research/architecture.md`'s "own package.json scripts, not wired into web-app's jest config".
- Files: `extensions/vscode/jest.config.js`

##### Task 6.1.1b: Add test script to package.json (~1 min)
- `"test": "jest --no-coverage"`, `"test:coverage": "jest"`.
- Files: `extensions/vscode/package.json`

##### Task 6.1.1c: Run full unit suite and confirm green (~3 min)
- `cd extensions/vscode && npx jest --no-coverage`; confirm all test files from Phases 1-5 pass with zero `vscode` import errors (a stray `import * as vscode` in a "pure" module would surface here as a `Cannot find module 'vscode'` failure, since Jest runs outside the extension host).
- Files: none (verification task)

### Epic 6.2: @vscode/test-cli Smoke Tests

#### Story 6.2.1: One smoke test verifying activation and command registration
**Files**: `extensions/vscode/package.json`, `extensions/vscode/.vscode-test.mjs`, `extensions/vscode/test/suite/extension.test.ts`

##### Task 6.2.1a: Add @vscode/test-cli and @vscode/test-electron devDependencies (~2 min)
- Add both to `extensions/vscode/package.json` devDependencies (Node.js 22.x+ per `research/stack.md`).
- Files: `extensions/vscode/package.json`

##### Task 6.2.1b: Create .vscode-test.mjs config (~3 min)
- `files: "test/suite/**/*.test.js"` (compiled output — `tsc` compiles `test/suite/*.ts` alongside `src/`, or a separate `tsconfig.test.json`), `mocha: {ui: "tdd", timeout: 20000}`.
- Files: `extensions/vscode/.vscode-test.mjs`

##### Task 6.2.1c: Create extension.test.ts smoke test (~5 min)
- Mocha-based (this is a *different* test runner from Jest, per `research/stack.md` — reserved for a small number of smoke tests only): `suite("Extension Activation")` — `vscode.extensions.getExtension("tstapler.stapler-squad-vscode")` activates, then asserts `vscode.commands.getCommands(true)` includes `"staplerSquad.openDashboard"`, `"staplerSquad.newSessionInCurrentFolder"`, `"staplerSquad.openSessionWorktreeQuickPick"`.
- Files: `extensions/vscode/test/suite/extension.test.ts`

##### Task 6.2.1d: Add test:integration script (~1 min)
- `"test:integration": "vscode-test"`.
- Files: `extensions/vscode/package.json`

---

## Phase 7: Packaging & Distribution

### Epic 7.1: vsce Packaging

#### Story 7.1.1: Local .vsix build
**Files**: `extensions/vscode/package.json`

##### Task 7.1.1a: Add @vscode/vsce devDependency and package script (~2 min)
- Add `@vscode/vsce` (scoped package, not the deprecated unscoped `vsce`, per `research/stack.md`) as a devDependency; `"package": "vsce package"` script.
- Files: `extensions/vscode/package.json`

##### Task 7.1.1b: Add vsce-required manifest fields (~2 min)
- Add `"icon"` (placeholder path, e.g. `resources/icon.png` — a real asset is a separate design task, not blocking this plan), `"repository"` pointing at this repo, `"license"`; confirm `"publisher"` (Task 1.1.1a's placeholder) is finalized per Unresolved Question 1/2 before this task is executed for real.
- Files: `extensions/vscode/package.json`

##### Task 7.1.1c: Verify vsce package produces a working .vsix (~3 min)
- Run `cd extensions/vscode && npx vsce package`; confirm a `.vsix` file is produced with no errors (warnings about a missing `README.md` are acceptable and non-blocking — no README is created per this repo's "don't proactively create docs" convention unless explicitly requested).
- Files: none (verification task)

### Epic 7.2: CI-Built Release
*(Addresses Unresolved Question 1 — flagged BLOCKING; this story implements the research-recommended default. Confirm the packaging decision before running this epic for real.)*

#### Story 7.2.1: GitHub Actions workflow builds and attaches .vsix on tag
**Files**: `.github/workflows/vscode-extension-release.yml`

##### Task 7.2.1a: Create the release workflow (~5 min)
- Trigger: `push: tags: ["vscode-v*"]`. Steps: checkout, setup Node (pinned version per Unresolved Question 2), `pnpm install` in `extensions/vscode/`, `npm run compile && npm run typecheck && npm test`, `npx vsce package`, upload the resulting `.vsix` as a release asset via `gh release upload` (or `softprops/action-gh-release`) to the GitHub Release matching the pushed tag.
- Files: `.github/workflows/vscode-extension-release.yml`

---

## Phase 8: Fast-Follow / Stretch (NOT part of the v1 critical path)

Per the requirements doc's own "may ship as fast-follow" language for AC-8, and `research/stack.md`'s recommendation to defer streaming, neither epic in this phase blocks v1 completion. AC-8's story/tasks are written out in full so the work isn't lost; the streaming fast-follow is documented only, per the Step 7 instruction not to expand it into tasks.

### Epic 8.1: autoOpenWorktree (AC-8)

#### Story 8.1.1: Open changed files (capped) on explicit worktree-open action only
**Acceptance Criteria**:
- AC-8: When `autoOpenWorktree` is enabled, opening a session with a dirty worktree opens its changed files in editor tabs, capped, and only on an explicit open action — never a background poll tick.
  - *Given* `staplerSquad.autoOpenWorktree === true` and a session's worktree has 3 changed files (from `GetVCSStatus`), *When* the user opens that session's worktree via the Story 3.3.2 flow, *Then* all 3 files are opened as editor tabs via `vscode.window.showTextDocument`.
  - *Given* the same config but 30 changed files, *When* the same open action runs, *Then* only the first `MAX_AUTO_OPEN_FILES = 10` are opened (capped, per `research/pitfalls.md`'s flood-of-tabs risk) — never triggered from a `PollScheduler` tick, only from the explicit `openSessionWorktreeCommand` call path.
**Files**: `extensions/vscode/src/autoOpen/autoOpenWorktree.ts`, `extensions/vscode/src/commands/openSessionWorktree.ts`, `extensions/vscode/src/__tests__/autoOpenWorktree.test.ts`

##### Task 8.1.1a: Create autoOpenWorktree.ts (~4 min)
- `capChangedFiles(files: string[], max = 10): string[]` — pure function, `files.slice(0, max)`. `openChangedFilesInEditor(worktreePath: string, changedFiles: string[])` calls `vscode.window.showTextDocument` for each capped file path.
- Files: `extensions/vscode/src/autoOpen/autoOpenWorktree.ts`

##### Task 8.1.1b: Wire into openSessionWorktree.ts, guarded by config, explicit-trigger-only (~4 min)
- In `openSessionWorktreeCommand`, after the `openFolder` call, if `getConfig().autoOpenWorktree`, call `GetVCSStatus({sessionId})`, `capChangedFiles(...)`, `openChangedFilesInEditor(...)` — this call path is only reachable from the explicit command, never from `PollScheduler`'s `tickFn`.
- Files: `extensions/vscode/src/commands/openSessionWorktree.ts`

##### Task 8.1.1c: Unit test the cap and explicit-trigger-only guard (~3 min)
- Assert `capChangedFiles(Array(30).fill("f"), 10)` returns exactly 10 items. Assert (via a source-level check or a documented invariant in the test's comment, since "never called from a poll tick" isn't directly unit-testable) that `pollScheduler.ts`'s `tickFn` never imports `autoOpenWorktree.ts` — a simple import-boundary assertion or lint rule is acceptable here.
- Files: `extensions/vscode/src/__tests__/autoOpenWorktree.test.ts`

### Epic 8.2: Streaming Fast-Follow (documented only — no tasks)

**Story 8.2.1** (stub): Replace interval polling with a Node-side port of `WatchSessions`/`WatchReviewQueue` (either vanilla Connect server-streaming via `@connectrpc/connect-node`, or a Node equivalent of `web-app/src/lib/transport/watch-ws-transport.ts`'s hand-rolled WebSocket envelope) once polling proves too slow or too chatty across many open VS Code windows in practice. Explicitly out of v1 scope per `research/stack.md` and the Step 0.5 creative-pass rejection above (Alternative 2). No tasks are written for this story — it is a placeholder for a future planning pass once real usage data justifies the switch.
