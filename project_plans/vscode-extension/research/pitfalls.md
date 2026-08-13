# Research: Known Pitfalls & Risks — VS Code Extension

Companion to `../requirements.md`. Research-only; no source code changes.

## 1. VS Code extension-host pitfalls

**Activation events.** Do not use `"activationEvents": ["*"]` — it forces the extension host
to activate on every VS Code window launch, which is exactly the startup-perf tax the
VS Code team has been warning against since the "Activation Events" guidance in the
extension API docs (`onStartupFinished` deprecated `*` for this reason). Correct pattern
for this extension:
- `onStartupFinished` (fires after VS Code's own startup work is done, not blocking cold
  start) if the status bar item should appear passively without explicit user action, **or**
- `onView:staplerSquadSessions` (lazy — only activates when the sidebar tree view is
  expanded) if status bar can be deferred until the view is opened.
- Commands contributed via `contributes.commands` auto-generate an implicit
  `onCommand:staplerSquad.xxx` activation event in modern `package.json` — no need to list
  these manually in `activationEvents` (VS Code ≥1.74 infers them from `contributes`).

Given AC-2 requires the status bar to show live data on load, `onStartupFinished` is the
right choice over `*`.

**Unhandled promise rejections in polling/streaming loops.** A `setInterval` callback or a
long-lived async stream-reader that throws (network error, JSON parse error, RPC error) and
isn't wrapped in try/catch can crash or degrade the extension host process, taking down
*every other extension* in that window, not just this one. Every poll tick / stream
reconnect loop must catch and swallow (log to an `OutputChannel`, not `console.error`) —
never let a rejection escape an interval or async generator loop uncaught.

**Disposal / memory leaks.** `StatusBarItem`, `TreeView`, `EventEmitter` (used to drive
`TreeDataProvider.onDidChangeTreeData`), and any `setInterval`/reconnecting stream handle
must all be pushed into `context.subscriptions` (or an equivalent `Disposable[]` disposed in
`deactivate()`). Missing this is the single most common VS Code extension leak: on extension
deactivate/reload (frequent during `F5` Extension Development Host iteration, or a
Marketplace auto-update), an un-disposed interval keeps firing against a `serverUrl` that no
longer has a valid extension context, and un-disposed `EventEmitter`s pin memory across
reload cycles.

**Webview: not needed.** Requirements call for a status bar item + native `TreeView`
(`contributes.views`) + command palette entries — none of these require a `Webview`. Native
tree views get built-in accessibility, theming, and keyboard nav for free; a webview would
add a CSP/webview-security surface (message-passing sanitization, `retainContextWhenHidden`
memory cost) for no UX benefit here. Recommend explicitly ruling out webview in the plan
unless a future fast-follow needs custom HTML rendering (e.g. rich diff preview).

## 2. Networking pitfalls

**Server port/host is not a fixed constant.** `CLAUDE.md`'s "Manual/interactive testing"
section documents that a second instance is commonly run with `PORT=8999` and
`STAPLER_SQUAD_INSTANCE=<name>`, and the e2e suite (`.claude/rules/e2e-test-conventions.md`)
runs against `:8544`. The default `staplerSquad.serverUrl` of `http://localhost:8543` is
correct as a *default* but the extension must not hardcode it anywhere else — always read
from config, and expect that a developer running a manual second instance alongside the
live one will need to repoint `serverUrl` per-workspace (VS Code workspace-scoped settings,
not just user/global settings, matter here).

**Prefer the existing streaming RPCs over polling.** `proto/session/v1/session.proto` already
defines `WatchSessions` (server-streaming `SessionEvent`) and `WatchReviewQueue`
(server-streaming `ReviewQueueEvent`), explicitly documented in the proto comments as
"real-time... without polling" / "without requiring polling." A naive `setInterval`-based
poll loop is not just suboptimal — it duplicates functionality the backend already built to
solve this exact problem, and multiple VS Code windows (one per worktree, a common
stapler-squad workflow) each independently polling `ListSessions`/`ListApprovalRules` (or
equivalent) on a timer is a real thundering-herd risk against a single local Go process. Use
`WatchSessions`/`WatchReviewQueue` server-streaming instead, with a bounded-backoff
reconnect loop as the *only* timer-driven code path (reconnect-on-drop, not steady-state
poll). If the RPC surface doesn't yet expose a single call for "status bar summary" (active
count + queue depth) cheaply, note it as an open question for `sdd:3-plan` — worth checking
whether `ListSessions`/`ListApprovalRules` unary calls are cheap enough for a low-frequency
status-bar-only refresh even while `WatchSessions` drives the tree.

**Server down / restarting.** `.claude/rules/tmux-keep-server-on-restart.md` documents that
`make install-service` / `systemctl --user restart` disruptively tears down the live
service (and even kills tmux sessions without `--tmux-keep-server`). The extension must treat
connection failure as an expected, frequent transient state — not an error to surface
loudly. Recommended UX: on stream/poll failure, downgrade the status bar item to a muted
"disconnected" glyph (no popup/toast), retry with exponential backoff, and only log to the
extension's `OutputChannel`. Repeated toast/error notifications on every dropped connection
during a routine service restart would be exactly the kind of spam `notifyOnQueueItem`-style
settings are meant to avoid triggering accidentally.

**CORS likely does not apply, but confirm the transport.** `server/server.go` wires
`CORSWithOrigins(s.origins)` into the middleware chain (`otelhttp -> logging -> CORS -> gzip
-> [auth] -> mux`, `server/server.go:847`), and `server/middleware/cors.go` shows two CORS
handlers exist: `CORS` (echoes back any `Origin` header, used for browser dev flows) and
`CORSWithOrigins` (allowlist-based, used by the wired-in middleware). This matters for a
**webview** (which runs in a Chromium-like renderer context and sends an `Origin` header
subject to CORS) but the plan here uses no webview — the extension's network calls run in
the **extension host**, which is a Node.js/Electron-utility-process context with no
same-origin policy, so CORS headers are irrelevant to it. Only becomes relevant if a future
fast-follow adds a webview panel that talks to the ConnectRPC server directly from
renderer-side JS.

**Auth: local server is unauthenticated by design.** `server/server.go:664-665`: "Auth is
provided by the existing middleware chain: local HTTP = no auth; remote HTTPS = WebAuthn
required." `localhost:8543` accepts unauthenticated requests from any local process — this
is an existing, accepted trust boundary (any local process can already reach it via curl/the
web-app), so the extension talking to it unauthenticated adds no *new* risk when `serverUrl`
correctly points at localhost. The risk is **misconfiguration**: per memory
(`project_mobile_app_server.md`), the mobile app's remote endpoint is
`https://onyx.staplerhome.internal:8444`, which *is* WebAuthn-protected and uses a private CA
cert. If a user copy-pastes that value into `staplerSquad.serverUrl` by mistake, the
extension's plain `fetch`/ConnectRPC client (no WebAuthn ceremony, no custom CA trust)
will simply fail every call — not a data-leak risk, but a silent-failure UX trap. Recommend
the extension validate `serverUrl` at config-read time and surface a one-time warning
(not a hard block — some users may run this over Tailscale/LAN legitimately) when the host
is not `localhost`/`127.0.0.1`, per the requirements doc's own open question.

## 3. `autoOpenWorktree` disruption risk

Requirements AC-8 already flags this as a "nice-to-have... may ship as a fast-follow." Concrete
failure mode to document in the plan: a session with a large dirty worktree (e.g. a
lockfile regen, a generated-code sync, or a broad refactor touching 30+ files) would, if
`autoOpenWorktree` naively opens "the changed files" on every session switch, dump dozens of
editor tabs into the window on a single click — destroying the user's existing tab layout
and making the feature actively hostile rather than convenient. If implemented at all,
should (a) cap the number of auto-opened files (e.g. top N by relevance or just refuse above
a threshold and fall back to opening the worktree root unopened), (b) only trigger on
explicit worktree-open action, never on a background status refresh, and (c) respect
`git diff --stat`-style change size before deciding to auto-open vs. just revealing the
Source Control panel instead of individual tabs.

## 4. Config pitfalls (live-apply vs. reload)

VS Code's `workspace.onDidChangeConfiguration` allows most settings to live-apply without a
window reload — appropriate for all four settings here:
- `serverUrl` — re-point the RPC client and restart the `WatchSessions`/`WatchReviewQueue`
  streams on change; no reload needed.
- `showStatusBar` — call `.show()`/`.hide()` on the existing `StatusBarItem`; no reload.
- `autoOpenWorktree` — pure boolean read at the point of use; no reload.
- `notifyOnQueueItem` — pure boolean read at the point of use; no reload.

None of these require `"scope": "window"`-forced-reload behavior (that's only needed for
settings that affect activation events or contributed UI structure itself, e.g. changing
`contributes.views` dynamically, which doesn't apply here). AC-7 already anticipates this
correctly ("take effect without reloading... where VS Code supports live config") — the
finding is that **all four** settings fall into the live-apply category, so no settings here
should need the reload fallback language at all; flag this to `sdd:3-plan` as a simplification
opportunity (drop the "documented reload requirement" branch of AC-7 as dead code for this
feature's specific settings, unless plan finds a reason otherwise).

## 5. Distribution/packaging pitfalls

Requirements' "Open Questions" section leaves Marketplace vs. Open VSX vs. manual `.vsix`
unresolved. Manual `.vsix` install (`code --install-extension foo.vsix`) is the lowest-friction
path to ship *something* but creates real adoption friction if not automated:
- No auto-update — every server-side proto change or bugfix requires each developer to
  manually re-download and reinstall the `.vsix`.
- No discovery — a new team member has to be told the extension exists and where to get it;
  it won't show up in the Extensions view search.

Since this repo already has CI (release-please conventional-commit versioning per
`CLAUDE.md`), the lowest-friction internal option is a CI job that builds the `.vsix` on tag
and attaches it as a GitHub Release asset (installable via `code --install-extension
<path-or-url>` or drag-and-drop into the Extensions view) — avoids the Marketplace/Open VSX
publisher-account and review-latency overhead while still giving a stable, versioned
download link. Flag as a decision for `sdd:3-plan`/`sdd:1-ideate` follow-up rather than
something this research phase should resolve unilaterally.

## Sources checked

- `project_plans/vscode-extension/requirements.md`
- `server/server.go` (CORS/auth middleware wiring, lines ~53, 664-665, 741-853, 979-1036)
- `server/middleware/cors.go` (`CORS` vs `CORSWithOrigins`)
- `proto/session/v1/session.proto` (`WatchSessions`, `WatchReviewQueue` streaming RPCs, lines 27-54)
- `/home/tstapler/Programming/stapler-squad/CLAUDE.md` — "Manual/interactive testing" section (PORT/STAPLER_SQUAD_INSTANCE override pattern)
- `.claude/rules/tmux-keep-server-on-restart.md` — service restart disruption
- `.claude/rules/e2e-test-conventions.md` — `:8544` test port precedent
- User memory `project_mobile_app_server.md` — `https://onyx.staplerhome.internal:8444`, WebAuthn + private CA
- General VS Code extension API knowledge: `activationEvents`, `Disposable`/`context.subscriptions` pattern, `onDidChangeConfiguration` (2025-2026 best practices, no single canonical doc cited — standard VS Code Extension API guidance)
