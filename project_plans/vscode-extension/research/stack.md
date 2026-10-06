# Research: Stack for the VS Code Extension

Research only — no source code was written for this doc.

## 1. Scaffolding

- Standard generator is still `yo code` (`npx --package yo --package generator-code -- yo code`
  → "New Extension (TypeScript)"), maintained at
  [microsoft/vscode-generator-code](https://github.com/microsoft/vscode-generator-code). It now
  asks which bundler to use as part of the prompt flow.
- **Bundler: esbuild**, not webpack. VS Code's own
  [bundling guide](https://code.visualstudio.com/api/working-with-extensions/bundling-extension)
  presents esbuild as the default recommendation — minimal config, small bundles, fast startup.
  Caveat: esbuild only strips types, it does not type-check, so `tsc --noEmit` must run as a
  separate `pretest`/`compile` step (matches this repo's existing pattern of `next lint` +
  `tsc` being separate from the `swc`-based Next.js build).
- **TypeScript version**: stick with the version already pinned in `web-app/package.json`
  (`^5.9.3`) for consistency across the repo rather than jumping to TypeScript 7 (the
  Go-ported native compiler that went stable mid-2026) — TS7 is a build-perf upgrade, not
  something this extension's scope needs, and mixing major TS versions across `web-app/` and
  a new `extensions/vscode/` package adds needless drift.
- No root `pnpm-workspace.yaml` or root `package.json` exists in this repo
  ([confirmed: neither file is present](/home/tstapler/Programming/stapler-squad)) — `web-app/`
  is a standalone pnpm project, not part of a monorepo workspace. A new `extensions/vscode/`
  package would need its own `package.json`/`pnpm-lock.yaml` (matching the repo's existing
  `packageManager: pnpm@10.27.0` pin) rather than assuming workspace-relative imports work.

## 2. VS Code Extension API surfaces needed

All map directly to the 4 goals in the requirements doc:

| Requirement | API |
|---|---|
| Status bar item (count + queue depth, click→browser) | `vscode.window.createStatusBarItem`, `.command` property, `vscode.env.openExternal(vscode.Uri.parse(...))` |
| Sidebar Tree View | `vscode.window.registerTreeDataProvider` + a class implementing `vscode.TreeDataProvider<T>` (`getChildren`, `getTreeItem`, `onDidChangeTreeData` event emitter to push updates without a manual refresh — needed for AC-5) |
| Click session → open worktree | `vscode.commands.executeCommand('vscode.openFolder', uri, { forceNewWindow: boolean })` — the boolean controls same-window vs new-window, matching AC-4's "same window or new window, user-configurable" |
| Approve/Deny inline actions | `TreeItem.contextValue` + `package.json` `view/item/context` menu contributions wired to two commands, each calling `ResolveApproval` (see §3) |
| Command palette | 3 commands declared in `package.json` `contributes.commands` + `contributes.menus.commandPalette`, registered via `vscode.commands.registerCommand` |
| Config (`staplerSquad.*`) | `contributes.configuration` in `package.json`; read via `vscode.workspace.getConfiguration('staplerSquad')`; listen for live changes via `vscode.workspace.onDidChangeConfiguration` (needed for AC-7's "take effect without reload" requirement — `showStatusBar`/`serverUrl`/`notifyOnQueueItem` can all be live-reloaded this way; only a change that requires re-establishing a long-lived poll/watch connection needs a documented reload) |
| `notifyOnQueueItem` | `vscode.window.showInformationMessage` (or `showWarningMessage`) triggered from the poll/watch loop when new queue items appear |

## 3. Backend integration — reuse existing ConnectRPC, but swap transport package

Confirmed via `server/server.go`:
- **No auth in the default local flow.** Comment at `server/server.go:664-665`: *"Auth is
  provided by the existing middleware chain: local HTTP = no auth; remote HTTPS = WebAuthn
  required."* `SetupAuth`/`StartRemote` only wire in `authMiddleware` for the remote/TLS path
  (`server/server.go:741-743`, `:1027-1036`). This directly answers the requirements doc's open
  question: for the default `http://localhost:8543` target, the extension needs **no
  credentials**, same as the web-app's local dev flow. If a user points `staplerSquad.serverUrl`
  at a remote HTTPS instance, WebAuthn-gated auth would apply and is out of scope for v1 (flag
  as a known limitation, not silently ignored).
- All RPCs are ConnectRPC, mounted under `/api/` (`web-app/src/lib/config.ts:12`,
  `getApiBaseUrl()` returns `<origin>/api`). Relevant existing RPCs (all defined in
  `proto/session/v1/session.proto`, generated into `web-app/src/gen/session/v1/`) —
  **no backend/proto changes needed**, confirming AC-9:
  - `ListSessions` / `GetSession` — session list + status for the tree view and status bar count
  - `GetReviewQueue` (`session.proto:43`) / `WatchReviewQueue` (`:54`) — review queue depth + items
  - `ResolveApproval` (`:122`) — the Approve/Deny action (`ResolveApprovalRequest` takes a
    session id + `"allow"`/`"deny"` decision string, `session.proto:1245-1252`) — this is the
    RPC AC-5 needs, not a separate "review queue" concept; it directly answers the requirements
    doc's third open question.
  - `ListPendingApprovals` (`:126`) — alternative/complementary read for pending approval state
  - `CreateSession` — for "New Session in Current Folder"

- **Generated TS bindings already exist** (`web-app/src/gen/session/v1/session_pb.ts`,
  `session_connect.ts`) but are generated *into* `web-app/src/gen` specifically —
  `buf.gen.yaml`'s TS plugin (`protoc-gen-es`) has a single hardcoded `out: web-app/src/gen`
  target, and there is no monorepo/workspace wiring (see §1) that would let
  `extensions/vscode/` import across package boundaries by path. Two real options, to resolve
  during planning:
  1. **Extend `buf.gen.yaml`** with a second `protoc-gen-es` output block targeting
     `extensions/vscode/src/gen` (mirrors the existing web-app block, uses the extension's own
     `node_modules/.bin/protoc-gen-es`) — regenerated by the same `make proto-gen` target used
     today. Codegen is duplicated on disk but zero new tooling/workspace concepts.
  2. Introduce a `pnpm-workspace.yaml` at the repo root covering both `web-app/` and
     `extensions/vscode/`, and factor `web-app/src/gen` out into a shared local package. Cleaner
     long-term but a bigger structural change than this feature's non-goals ("no backend/proto
     changes") suggest is in scope — recommend deferring unless a third TS consumer appears.
  - Either way, the message/service *definitions* (`session_pb.ts`, `session_connect.ts`) are
    transport-agnostic — only the `Transport` construction differs (see next point), so the
    generated code itself needs no forking.

- **Transport package: `@connectrpc/connect-node`, not `@connectrpc/connect-web`.**
  `web-app`'s transport (`web-app/src/lib/api/transport.ts:1,20`) uses
  `createConnectTransport` from `@connectrpc/connect-web`, which is fetch-API-based and targets
  browser/any-fetch-capable runtime. The VS Code extension host runs as a Node.js process
  (Electron's Node integration), and `@connectrpc/connect-node` is the intended package for a
  vanilla Node runtime — it uses Node's `http`/`https` modules instead of depending on `fetch`.
  Pin to the same major version already used in `web-app` (`@connectrpc/connect` /
  `@connectrpc/connect-web` are both `^2.1.1`) — use `@connectrpc/connect-node@^2.x` to match.
  (Node 22+, which VS Code's bundled Electron/Node runtime satisfies well past, does have a
  global `fetch`/`WebSocket` now, so `connect-web`'s fetch-based transport would likely also
  work — but `connect-node` is the documented/supported combination for this runtime and avoids
  relying on undocumented fetch behavior in the extension host.)

- **Streaming (`WatchSessions`, `WatchReviewQueue`) uses a custom WebSocket envelope, not
  vanilla Connect streaming.** `web-app/src/lib/transport/watch-ws-transport.ts` hand-rolls a
  `fromWebSocket()` async generator over the browser `WebSocket` API specifically for the watch
  RPCs — it's not just `createConnectTransport` from `connect-web`. Porting this to the
  extension would require either Node's native `WebSocket` global (stable in current LTS Node)
  or the `ws` package, plus re-implementing the envelope decode logic. **Recommendation for v1:
  use interval polling** (`ListSessions` + `GetReviewQueue` every 5-10s via `setInterval`,
  matching AC-2's "refreshing on an interval **or** via a push/poll mechanism" wording, which
  explicitly allows polling) rather than porting the WS watch transport — much smaller surface
  area, and matches the "companion surface, not a full reimplementation" non-goal. Streaming can
  be a documented fast-follow if 5-10s latency proves too slow in practice.

## 4. Testing stack

Two distinct layers, matching AC-10's "matching how other TS code in this repo is tested (Jest)"
instruction:

- **Pure logic (data-fetching/formatting, RPC response → status bar string, tree item labels,
  etc.)**: plain **Jest**, same as `web-app` (`web-app/jest.config.js`,
  `@types/jest ^30.0.0`, `jest ^30.2.0`, `ts-jest ^29.4.11`). This code should be structured so
  it has **no dependency on the `vscode` module** — pass in plain data, return plain
  view-model objects — so it's testable without any VS Code runtime at all. This is the
  majority of what AC-10 asks for.
- **VS Code API integration** (things that actually call `vscode.window.createStatusBarItem`,
  `registerTreeDataProvider`, command registration, etc.): the current standard is
  **`@vscode/test-cli`** (wraps **`@vscode/test-electron`**) — `@vscode/test-cli` is now the
  recommended entry point for new extensions; it launches a real (or downloaded) VS Code
  instance and runs Mocha-based tests with full `vscode` API access, superseding hand-rolled
  `@vscode/test-electron` harness setup. This is a **different test runner from Jest**
  (Mocha, not Jest) and a heavier, slower path — reserve it for the handful of tests that
  genuinely need a live `vscode` instance (tree view registration, command execution,
  status-bar rendering), not for logic that can be unit-tested in plain Jest.
- Mocking the `vscode` module inside Jest (rather than using `@vscode/test-cli`) is possible
  via a manual mock module but is generally discouraged by VS Code's own docs in favor of
  either (a) keeping `vscode`-dependent code thin enough to not need testing, or (b) using the
  real API via `@vscode/test-cli`. Recommend the "thin adapter" approach: isolate all `vscode.*`
  calls behind small wrapper functions/interfaces, unit-test everything behind them with Jest,
  and leave the thin `vscode`-calling layer either untested or covered by a small number of
  `@vscode/test-cli` smoke tests.

## 5. Packaging

- **`@vscode/vsce`** (the modern scoped package; the old unscoped `vsce` is deprecated) is the
  standard tool to build a `.vsix` (`vsce package`) and publish (`vsce publish`).
- Current major-version guidance: **Node.js 22.x+** is required by current `vsce` releases;
  `vsce >= 2.26.1` is the documented minimum for publishing. Use the latest `@vscode/vsce`
  major at implementation time rather than pinning to a specific patch now.
- Marketplace auth: global Azure DevOps PATs are being retired (Dec 1 2026) in favor of
  Microsoft Entra ID-based publishing — relevant only if/when this ships to the public
  Marketplace; irrelevant for local `.vsix` builds via CI, which is one of the two
  distribution options the requirements doc's open question leaves unresolved (Marketplace/Open
  VSX publish vs. CI-built `.vsix` installed manually). Not a blocker for research; flag for a
  decision in the plan phase.

## 6. Consistency with existing repo tooling

- `web-app/package.json`: `packageManager: pnpm@10.27.0`, TypeScript `^5.9.3`, Jest `^30.2.0`,
  `@types/jest ^30.0.0`, `ts-jest ^29.4.11`, `@connectrpc/connect ^2.1.1` /
  `@connectrpc/connect-web ^2.1.1`, `@bufbuild/protobuf ^2.11.0`. No root-level Node version
  pin exists in the repo (no `.nvmrc`/`.node-version` found at repo root or in `web-app/`) —
  the extension package should declare its own `engines.node` (informed by `@vscode/vsce`'s
  Node 22+ requirement) rather than assuming an inherited pin.
- `web-app/tsconfig.json`: `target: ES2020`, `module: esnext` — the extension's `tsconfig.json`
  will differ regardless (extensions typically target `commonjs`/`Node16`-style module
  resolution for the extension host, per VS Code's own generator defaults), so exact tsconfig
  parity isn't the goal — TypeScript *version* parity is.
- Status values: the tree view's status badges (AC-3, "matches the status values already used
  by the web dashboard") should mirror `SessionStatus` from `proto/session/v1/types.proto:325`
  (`SESSION_STATUS_ACTIVE`, `_PAUSED`, `_CREATING`, `_STOPPED`, `_HIBERNATED`, `_RESTORING`, plus
  deprecated `_RUNNING`/`_READY`/`_LOADING`/`_NEEDS_APPROVAL` aliases still on the wire) and the
  existing badge component `web-app/src/components/sessions/StatusBadge.tsx` /
  `ReviewQueueBadge.tsx` for the label/color mapping to copy rather than reinvent.

## Open items for the plan phase (not resolved by this research)

1. Which of the two codegen-sharing options in §3 (duplicate `buf.gen.yaml` output vs.
   introduce a pnpm workspace) to commit to.
2. Marketplace/Open VSX publish vs. CI-built `.vsix` only (requirements doc's second open
   question — this research didn't find anything in-repo that presupposes an answer).
3. Whether `autoOpenWorktree` (AC-8, flagged as nice-to-have) ships in v1 or as a fast-follow.
