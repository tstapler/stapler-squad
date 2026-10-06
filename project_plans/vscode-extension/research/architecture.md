# Research: Architecture Approach — VS Code Extension

Scope: architecture-only research for `project_plans/vscode-extension/requirements.md`. No
source code changes. Findings below are grounded in file/line references captured 2026-08-06.

## 1. Where should the extension physically live?

**No `extensions/` or `packages/` directory exists yet** (`find . -maxdepth 1 -iname
extensions -o -iname packages` returned nothing; root dir listing via `ls -d */` confirmed
the full top-level set: `assets, bin, build, cmd, config, daemon, docs, executor, Formula,
gen, github, internal, log, logos, logs, macos, node_modules, pkg, profiling, project_plans,
proto, scripts, server, session, telemetry, tests, testutil, third_party, tmp_test_db, tools,
tuitest, uploads, web-app, worktrees`). This is greenfield placement.

**No JS/TS monorepo tooling exists.** `web-app/package.json` has no `workspaces` field, and
there is no `pnpm-workspace.yaml` anywhere in the repo (`find . -maxdepth 2 -iname
"pnpm-workspace.yaml"` — empty). `web-app` uses `pnpm@10.27.0` as a standalone package
(`"packageManager": "pnpm@10.27.0"` in `web-app/package.json:5`), not a workspace member. The
root-level `WORKSPACE` file is a **Bazel** workspace file (`workspace(name =
"stapler_squad")`, loads `rules_foreign_cc` for wrapping C builds) — unrelated to JS
workspaces, and not otherwise wired into `make build`/`make test`; do not treat it as
precedent for pnpm workspaces.

**Recommendation:** create `extensions/vscode/` (matches the requirements doc's own stated
default, AC-1: `extensions/vscode/` or similar) as a **fully standalone npm/pnpm package**
with its own `package.json`, `tsconfig.json`, and `node_modules` — same pattern as `web-app/`
being self-contained rather than a workspace member. Do not introduce a pnpm workspace just
for this feature; that would be a repo-wide tooling change out of scope for a "pure API
consumer" extension. If a second frontend-adjacent package appears later, workspace
consolidation can be revisited then — don't build it speculatively now.

## 2. Integration points — proto/RPC client

**`make proto-gen` output** (`Makefile:398-413`): runs `buf generate proto` and produces:
- Go: `gen/proto/go/`
- TypeScript: `web-app/src/gen/` (confirmed dirs: `web-app/src/gen/session/v1/`)

There is a single `buf.gen.yaml` at repo root driving both outputs — no per-package proto
codegen exists today; `web-app` is the only TS consumer.

**Two viable dependency-boundary options, evaluated:**

- **(A) Cross-package import of `web-app/src/gen/session/v1/*_pb.ts` from
  `extensions/vscode/`.** Simplest short-term, but couples the extension's build to
  `web-app`'s internal path layout and its `tsconfig`/module resolution; a VS Code extension
  bundles with esbuild/webpack targeting Node's CommonJS-friendly output, which is a
  different build target than Next.js's. Reaching outside a package's own `src/` for
  generated code it doesn't own is also the kind of implicit coupling the `web-app` codebase
  doesn't do internally today (each generated file is imported via `@/gen/...` path alias
  scoped to `web-app` itself).
- **(B) Point `buf.gen.yaml` at a second TS output target for
  `extensions/vscode/src/gen/`** (or a shared `gen/typescript/` consumed by both), wiring a
  new `make proto-gen` output the same way `gen/proto/go/` and `web-app/src/gen/` already
  exist side by side. This keeps the extension's build self-contained (no reach-across into
  `web-app/`) and mirrors the existing pattern of "one buf config, multiple generated
  outputs by language/consumer."

**Recommendation: (B).** It is a **one-line addition to `buf.gen.yaml`** (add a second
`protoc-gen-es`/`connect-es` output block pointed at
`extensions/vscode/src/gen`), not a proto or backend change, so it stays inside the
"pure API consumer, no backend changes" non-goal boundary. This should be scoped and
confirmed in the planning phase — it's a build-config change, not a `proto/*.proto` schema
change, so it doesn't violate AC-9. Flag this explicitly in the plan phase since it's the one
piece of "infrastructure" the extension needs beyond writing its own code.

## 3. Data flow — status bar / tree refresh

The web dashboard **already has a push mechanism**, not just polling — reuse it rather than
inventing a new one:

- `web-app/src/lib/transport/watch-ws-transport.ts` — `createWatchTransport()` wraps
  `createConnectTransport` (`@connectrpc/connect-web`) and layers a WebSocket-backed
  streaming transport (`fromWebSocket` async generator) specifically for ConnectRPC
  **server-streaming** calls, since browser `fetch()` can't do bidirectional/long-lived
  streaming cleanly.
- `web-app/src/lib/transport/websocket-transport.ts` — a fuller custom Transport
  implementation using `it-ws/client`, handling Connect's envelope framing over a raw
  WebSocket (used for terminal streaming).
- The **review queue and session-list hooks already use this**:
  `web-app/src/lib/hooks/useReviewQueue.ts:5-7` imports `createWatchTransport` and calls
  `WatchReviewQueueRequest`/`GetReviewQueueRequest`/`AcknowledgeSessionRequest` against
  `SessionService` (proto: `session.proto:52-54`, `rpc WatchReviewQueue(...) returns
  (stream ReviewQueueEvent)`). `useReviewQueue.ts` also exposes a
  `fallbackPollInterval` option (default present in the interface, `useWebSocketPush` flag)
  — i.e. the existing pattern is **push-first with poll fallback**, not poll-only.
  `useSessionService.ts:142,201-206` likewise builds its client via `createWatchTransport`
  with `autoWatch` support.

**Recommendation:** the extension's status bar and tree view should call
`GetReviewQueue`/`ListSessions`(or equivalent) once on activation for the initial paint, then
subscribe to `WatchReviewQueue` (and the session-list equivalent stream, if one exists —
confirm exact RPC name in planning) over a Node WebSocket client (`ws` npm package, since VS
Code extension host is Node, not browser — `it-ws/client`'s browser-oriented assumptions
would need checking, or a from-scratch minimal WS Transport can be written using the `ws`
package against the same envelope-framing logic already documented in
`watch-ws-transport.ts`). This avoids inventing a second live-update mechanism and matches
the "push/poll mechanism against the existing ConnectRPC session service" language already
in AC-2. A fixed poll interval as fallback (mirroring `fallbackPollInterval`) is reasonable
degradation if the WS connection drops.

## 4. Status/type enum consistency

Two related-but-distinct proto enums, and their Go source of truth:

- **`SessionStatus`** (`proto/session/v1/types.proto:325-350`) — comment at line 322: `//
  Maps to session.Status enum in Go.` Values: `SESSION_STATUS_UNSPECIFIED=0`,
  `SESSION_STATUS_ACTIVE=1` (also legacy aliases `RUNNING=1`, `READY=2` deprecated),
  `SESSION_STATUS_PAUSED=4`, `SESSION_STATUS_NEEDS_APPROVAL=5` (deprecated),
  `SESSION_STATUS_CREATING=6`, `SESSION_STATUS_STOPPED=7`, `SESSION_STATUS_HIBERNATED=8`,
  `SESSION_STATUS_RESTORING=9`.
  - Go source of truth: `session/instance.go:24-47`, `type Status int` with `Creating=0,
    Active=1, Paused=2, Stopped=3, Hibernated=4, Restoring=5` (plus deprecated aliases
    `Running/Ready → Active`, `Loading → Creating`). **Note the Go int values do not match
    the proto int values** (e.g. proto `PAUSED=4` vs Go `Paused=2`) — the mapping is by name
    via the service layer, not by raw int, so the extension must consume the **proto enum
    only** (`SessionStatus` from `@bufbuild` generated TS), never assume Go int parity.
- **`SessionType`** (`proto/session/v1/types.proto:354-366`) — `SESSION_TYPE_UNSPECIFIED=0,
  DIRECTORY=1, NEW_WORKTREE=2, EXISTING_WORKTREE=3, NEW_PROJECT=4, ONE_OFF=5`. This governs
  the "New Session in Current Folder" command (AC-6) — that command should almost certainly
  map to `SESSION_TYPE_DIRECTORY` (existing folder), matching the `.claude/rules/
  session-creation-registry.md` precedent for how `directory` mode is used elsewhere in the
  codebase.

**Recommendation:** the tree view's status badge logic must switch on the generated TS
`SessionStatus` enum values (from whichever `types_pb.ts` the extension consumes per §2), not
re-derive its own string badges — mirror how `web-app` components already do this (they
import `SessionStatus` from `@/gen/session/v1/types_pb`, e.g. `useReviewQueue.ts:14`).

## 5. Review queue vs. approve/deny — two distinct RPC families

The requirements' open question ("does review queue map 1:1 to
`list_approval_rules`/`submit_review_verdict`?") is answered: **no, those are backlog-item
policy config, a different concept.** The actual mechanism is two separate but related RPC
families, both in `proto/session/v1/session.proto`:

- **Review Queue** (what to *list* in the sidebar): `GetReviewQueue` (line 43),
  `WatchReviewQueue` → `stream ReviewQueueEvent` (line 54). `ReviewItem`
  (`types.proto:552-600`) carries `session_id`, `session_name`, `reason`
  (`AttentionReason`), `priority`, `status` (`SessionStatus`), `branch`, `path`,
  `working_dir`, `diff_stats`, etc. — everything the tree view needs to render a session row
  plus why it needs attention. It has **no approve/deny action of its own**.
- **Pending Approvals** (what to *act on* for Approve/Deny): `ListPendingApprovals` (line
  126) returns `PendingApprovalProto` (`session.proto:1273-1275`), and `ResolveApproval`
  (line 122) takes `ResolveApprovalRequest` (`session.proto:1245-1258`) — `approval_id`,
  `decision` (**plain string** `"allow"` or `"deny"`, not an enum — line 1250), optional
  `message` shown to the agent, and `override_ci_block` for re-submitting a CI-blocked
  approval.
- Existing web-app precedent for wiring these together:
  `web-app/src/lib/contexts/ApprovalsContext.tsx`, `web-app/src/lib/api/approvalsApi.ts`,
  `web-app/src/components/ui/NotificationPanel.tsx`, and
  `web-app/src/app/notifications/NotificationsPage.tsx` all reference
  `ResolveApproval`/`ListPendingApprovals` — these are the files to read during planning for
  the exact request-shape/approval_id sourcing pattern (notification metadata, per
  `ResolveApprovalRequest`'s own comment on line 1246: "from notification metadata
  .approval_id").

**Recommendation for planning:** the sidebar tree needs to correlate `ReviewItem.session_id`
with any `PendingApprovalProto` for that session to decide whether to render inline
Approve/Deny buttons — confirm the exact correlation key during planning by reading
`ApprovalsContext.tsx`. This is the one area where "read session-list and approve/deny
endpoints are sufficient" (AC-9) needs a concrete confirmation pass, since it's two RPC
families, not one.

## 6. Auth

`server/server.go:664-665` documents the policy directly: **"local HTTP = no auth; remote
HTTPS = WebAuthn required."** `SetupAuth`/`StartRemote` (lines 741-762, 1027-1036) show auth
middleware is only wired for the remote/TLS listener; the local loopback listener
(`server/server.go:762-771`, binds `127.0.0.1`/`::1` explicitly for `localhost`) has
`s.authMiddleware` unset. Since `staplerSquad.serverUrl` defaults to
`http://localhost:8543` (an HTTP loopback URL, matching the existing `web-app` dev-flow
assumption), **no credential handling is needed for the default configuration.** If a user
points `serverUrl` at a remote HTTPS instance, WebAuthn would apply — but that's out of scope
for v1 per the non-goals (extension is a companion to the *local* dashboard); flag as a v1
limitation rather than building auth handling speculatively.

## 7. Testing convention

`web-app/package.json:20` (`"test": "jest"`) and `web-app/jest.config.js` use `ts-jest`
presets. AC-10 asks for Jest-based unit tests on data-fetching/status-formatting logic — the
extension package should adopt the same `jest` + `ts-jest` combination for consistency, run
via its own `package.json` scripts (not wired into `web-app`'s jest config, since it's a
separate package per §1).

## 8. Repo conventions to carry over (non-Go, but still applicable)

`.claude/rules/interface-pollution-checklist.md` is Go-specific in its examples but the
underlying principle generalizes directly to this TypeScript extension:

- **Avoid speculative interfaces/abstraction layers.** A `SessionClient`
  wrapper/`Service`/`Manager` class that just forwards to the generated ConnectRPC client
  with no added behavior is a forwarding-only wrapper (smell #4) — call the generated client
  directly from the tree data provider / status bar controller instead, and only introduce a
  thin wrapping layer once there's a second real reason for it (e.g. caching, retry/backoff
  for the WS reconnect logic in §3 — that's a legitimate reason per the checklist's own
  `cachingUserStore` example).
- **No-op getters/setters** (smell #3) — VS Code extension APIs (TreeDataProvider,
  StatusBarItem) already push toward direct property assignment; don't add wrapper
  accessors around extension state that don't validate or compute anything.
- **Minimal deps** — the repo's existing TS stack already solved the WS-transport-over-
  ConnectRPC problem once (`watch-ws-transport.ts`); reuse/adapt that logic rather than
  pulling in an unrelated state-management or RPC library for a small, single-purpose
  extension.
- **CLAUDE.md conventions** (Conventional Commits, `make proto-gen` after any proto change,
  `make registry-generate` for new RPC/frontend markers) all still apply; the feature
  registry (`.claude/rules/feature-registry.md`) is currently scoped to `docs/registry/
  features/{backend,frontend}` and the existing `web-app`/`server` marker conventions — no
  existing marker convention covers a VS Code extension specifically. Planning should decide
  whether extension features get `frontend`-type registry entries (reusing the existing
  schema) or are exempted; nothing in the current registry tooling special-cases "extension"
  as a distinct type.

## 9. Prior architecture-review/hotspot-analysis output for this area

**None found.** Checked:
- `Glob project_plans/*/research/architecture.md` → only this newly-written file exists;
  no prior project's `research/architecture.md` mentions VS Code or extensions (spot-checked
  neighboring project dirs listed in `git status`: `dynamic-rule-reload`,
  `review-queue-severity`, `session-pr-creation`, `slack-review-notifications`,
  `stale-session-detection` — none relate to this feature).
- `docs/registry/` — no `vscode`/`extension`-named entries in `docs/registry/features/`
  (backend or frontend); grep for "vscode"/"extension" in `docs/registry/` returned nothing.
- No prior `code-hotspot-analysis` or `quality:architecture-review` output specific to this
  area exists — this is a net-new feature area with no prior review to build on.

## Summary of Open Questions — Resolved

| Requirements doc open question | Answer |
|---|---|
| Auth needed for localhost:8543? | No — local HTTP has no auth middleware (`server/server.go:664-665, 762-771`). Remote HTTPS would need WebAuthn but is out of v1 scope. |
| Does "review queue" map to `list_approval_rules`/`submit_review_verdict`? | No. Review queue = `GetReviewQueue`/`WatchReviewQueue` (`session.proto:43,54`) returning `ReviewItem`. Approve/deny = separate `ListPendingApprovals`/`ResolveApproval` (`session.proto:122,126`) family. The two must be correlated by `session_id` for the sidebar UI — confirm exact join logic against `ApprovalsContext.tsx` in planning. |
| Packaging/distribution | Not resolved by this research pass — issue doesn't specify Marketplace vs. CI-built `.vsix`; still an open decision for planning. |
