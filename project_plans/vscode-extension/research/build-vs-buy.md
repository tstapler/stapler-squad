# Build vs. Buy/Source — VS Code Extension for Session Status & Navigation

Scope per `requirements.md`: status bar item, sidebar Tree View, inline approve/deny,
3 commands, 4 settings. Pure ConnectRPC consumer of the existing local server at
`localhost:8543`. No backend changes.

## 1. Existing OSS library/framework (scaffold or shape-reference)

**Scaffolding tools:**
- `generator-code` (npm) / `yo code` — Microsoft's official Yeoman generator for VS Code
  extensions. Scaffolds `package.json` contribution points, `extension.ts` activation
  entry point, TS config, and a webpack/esbuild bundling choice. [npm: generator-code](https://www.npmjs.com/package/generator-code), [microsoft/vscode-generator-code](https://github.com/microsoft/vscode-generator-code)
  - **Pros:** Official, zero-risk starting skeleton, saves ~30 min of boilerplate
    (package.json contribution schema, .vscodeignore, launch.json for F5 debugging).
  - **Cons:** One-time scaffold, not a dependency — after running it there's nothing
    left to "adopt" or keep in sync; it doesn't know about ConnectRPC, TreeDataProvider
    patterns, or this repo's Jest conventions.
  - **Verdict:** Fine to run once as a starting skeleton generator, not a build-vs-buy
    decision in the traditional sense (no ongoing dependency). Optional — the skeleton
    is small enough (`package.json`, `extension.ts`, `tsconfig.json`) to hand-write
    directly in the plan and keep consistent with this repo's existing TS conventions
    (matching `web-app/` patterns) rather than importing generator defaults that would
    then need to be reconciled with this repo's lint/build setup.

- `vscode-extension-tester` (Red Hat) — Selenium-WebDriver-based **e2e UI testing**
  framework for VS Code extensions (drives the real Electron UI: opens tree views,
  clicks items, reads rendered content) — not a scaffold, not applicable to
  implementation. [Red Hat Developer blog](https://developers.redhat.com/blog/2019/11/18/new-tools-for-automating-end-to-end-tests-for-vs-code-extensions), [microsoft/vscode-discussions #1156](https://github.com/microsoft/vscode-discussions/discussions/1156)
  - **Pros:** Would give true UI-level e2e coverage (tree rendering, click-to-approve)
    beyond what VS Code's built-in `@vscode/test-electron` integration harness can verify.
  - **Cons:** Heavy (spins a real Electron + WebDriver session), not this repo's existing
    test stack (Jest, per AC-10 and `tests/e2e/` which is Playwright against the *web*
    app, not VS Code). Requirements only ask for unit-test coverage of data-fetching/
    status-formatting logic (AC-10) — no UI e2e requirement.
  - **Verdict:** Out of scope for v1. Note as a possible fast-follow if the extension
    grows complex enough to need UI-level regression coverage; not needed to hit AC-10.

**Reference-only prior art** (proprietary/MS-owned extensions, OSS-licensed, worth
skimming `extension.ts` / tree-provider structure for conventions, not for code reuse):
- **GitLens** (eamodio/vscode-gitlens) — sidebar tree views with inline actions,
  status bar item pattern, closest UX analog to "session list + inline action" here.
- **GitHub Pull Requests and Issues** (microsoft/vscode-pull-request-github) — polls
  a remote API, renders a tree of PRs with inline approve/comment actions — structurally
  the closest match to "poll ConnectRPC session list, render tree, approve/deny inline."
- **Docker** (microsoft/vscode-docker) — status bar + tree view over a local daemon
  (not a remote SaaS), same "local dev tool front-end" shape as this extension.
- These are useful only as **read-for-convention** references during `sdd:3-plan` (e.g.
  how they structure `TreeDataProvider.onDidChangeTreeData`, poll intervals, and command
  registration) — none exposes a reusable library; their code is extension-specific and
  tightly coupled to GitHub/Docker/Git APIs.

**No existing generic "local dev tool session/task list" VS Code framework exists.**
Searched for a reusable abstraction over `TreeDataProvider` + `StatusBarItem` for this
exact shape (dev-tool session monitor); none found — every example above hand-rolls its
own tree provider against VS Code's native API, which is itself lightweight enough that
no wrapper library has emerged to abstract it.

## 2. SaaS/managed API

**N/A.** stapler-squad is a self-hosted Go server the user runs locally
(`localhost:8543` by default per `staplerSquad.serverUrl`, confirmed in
`project_plans/vscode-extension/requirements.md`); there is no third-party SaaS
equivalent to buy in place of talking to the user's own already-running server. The
"vendor" here is the same repo's own backend — this axis of the build-vs-buy decision
doesn't apply.

## 3. LLM-generated implementation vs. battle-tested library

**ConnectRPC client — use the existing generated bindings, confirmed already a
dependency:**
```
$ grep -i "connectrpc" web-app/package.json
    "@connectrpc/connect": "^2.1.1",
    "@connectrpc/connect-web": "^2.1.1",
```
Verified (`web-app/package.json`). The web-app already uses this exact pattern —
`createClient(SessionService, createConnectTransport({ baseUrl }))` — in multiple
places, e.g. `web-app/src/app/history/page.tsx:68-69` and
`web-app/src/components/sessions/WorkspaceSwitchModal.tsx:93-95`. The generated
protobuf/ConnectRPC TS bindings (`session/gen/session/v1/*.go` server-side,
`web-app/src/gen/session/v1/*_pb.ts` client-side per `make proto-gen`) are shared,
already-generated, already-tested-in-production code — the extension should import
`@connectrpc/connect` + `@connectrpc/connect-web` and reuse (or vendor a copy of) the
generated `*_pb.ts` types rather than hand-rolling raw HTTP/JSON calls against the RPC
endpoints. This is a clear "use the battle-tested library" call, not an LLM-generated
implementation.

**Auth:** confirmed in `server/server.go` — "local HTTP = no auth; remote HTTPS =
WebAuthn required" (comment at line 665, `SetupAuth`/`StartRemote` at lines 741-1033).
Since `staplerSquad.serverUrl` defaults to `http://localhost:8543`, v1 needs no
credential handling, consistent with the existing unauthenticated local web-app dev
flow. If a user points the extension at a remote HTTPS instance, WebAuthn would apply —
out of scope for v1 per requirements' non-goals; flag in the plan as a known gap if
`serverUrl` is ever pointed off-loopback.

**TreeDataProvider / StatusBarItem — use VS Code's native extension API directly:**
No wrapping library needed or found. `vscode.TreeDataProvider`, `vscode.StatusBarItem`,
`vscode.window.createTreeView`, and `vscode.commands.registerCommand` are the stable,
first-party, battle-tested APIs every reference extension above (GitLens, GitHub PRs,
Docker) builds directly on — there is no ecosystem convention of wrapping these in a
third-party abstraction, because the native APIs are already minimal and stable across
VS Code versions. Hand-writing a thin `SessionTreeProvider implements TreeDataProvider`
class against this API is the correct "battle-tested library" choice, not a case where
an LLM would be reinventing something a library already solves.

## 4. Fork or adapt

- **`.vscode/` at stapler-squad repo root:** does not exist (`ls` confirmed no such
  path) — only per-user/IDE settings would live here if present; nothing to adapt.
- **`stapler-scripts/`** (per top-level `~/CLAUDE.md` repo map — this directory lives
  in the **dotfiles** repo, not `stapler-squad`, and was checked there):
  `grep -ril vscode ~/dotfiles/stapler-scripts` returned no matches. No existing
  VS Code extension, script, or tooling to fork.
- **Within `stapler-squad` itself:** no `extensions/` directory or prior VS Code
  extension scaffold exists yet (`project_plans/vscode-extension/` is the only hit for
  "vscode-extension" repo-wide, i.e. this planning doc itself).
- **Verdict:** Nothing to fork or adapt, in this repo or the adjacent dotfiles repo.
  Build fresh under `extensions/vscode/` (per requirements AC-1's suggested path).

## Summary verdict

Build directly on VS Code's native extension API (`TreeDataProvider`, `StatusBarItem`,
`commands.registerCommand`) plus this repo's already-present, already-tested
`@connectrpc/connect` + `@connectrpc/connect-web` generated bindings. No existing
scaffold, framework, SaaS substitute, or internal prior-art extension changes that
conclusion — `yo code` is a reasonable one-time skeleton generator but not a dependency
worth adopting given how small the boilerplate is; `vscode-extension-tester` is a real
but out-of-scope option for future UI-level e2e coverage beyond AC-10's unit-test bar.
