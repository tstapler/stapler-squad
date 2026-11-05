# Verification report: ghostty-terminal-migration (sdd:6-verify)

Run 2026-10-30 on `backlog/stapler-squad-ghostty-terminal-migration`. Layer 1 and 2 ran as three read-only review agents (React/TS idioms, architecture, refactor candidates). The Go idiom layer was skipped: the diff contains no non-test Go change (`feature_flag_service_test.go` and two other small files only).

## Layer 1 — TypeScript / React idioms

| # | Finding | Severity | Action |
|---|---|---|---|
| 1 | Rejected `import("ghostty-web")` cached forever; no catch at call site | MUST FIX | Fixed: cache cleared on failure, caller logs and drops pending writes (`GhosttyTerminal.tsx`) |
| 2 | `prop in overrides` matched `Object.prototype` members | MUST FIX | Fixed: `Object.hasOwn` |
| 7 | `handle.write`/`writeln` bypassed the query responder | SUGGEST | Fixed: all writes go through one `compatTerminal()` |
| 4 | FitAddon not disposed | SUGGEST | Fixed: `fit.dispose?.()` before `term.dispose()` |
| 3, 5, 6, 8, 9 | Disposed-facade guard, duplicate resize, proxy memoization, RIS/DECSTR reset of sync state, search scroll convention | SUGGEST | Follow-up |

Browser verification found two defects the static review missed, both fixed with tests:
- `ghosttyQueryResponder` used a global regex that overflowed the stack on multi-MB scrollback replays; replaced with a bounded `indexOf` scanner (tests with a 200k-line chunk).
- `TerminalOutput`'s write instrumentation assigned over `terminal.write` on the Proxy, which landed on the raw terminal and made the facade call itself forever (`Maximum call stack size exceeded` on every session load). Fixed by capturing the raw `write` at facade creation and adding a `set` trap; regression test in `ghosttyAdapter.test.ts`.

## Layer 2 — Architecture and refactoring

No BLOCKER. The plan has no Tech Debt Disposition table, so no disposition rows to check.

| Finding | Severity | Action |
|---|---|---|
| Plan tasks 3.1 (GhosttyTerminal unit tests), 3.2 (pool tests on both renderers), 3.4 (e2e conformance spec), 2.6 (OSC 8 link menu) not delivered | CONCERN | Deferred; flag stays off by default so xterm behaviour is unchanged. Not done in this change |
| Proxy-of-Proxy facade overlaps `toITerminal` | CONCERN | Follow-up: collapse into one narrow interface |
| Renderer flag special-cased in three places in `flagStore`/`TerminalPool` | CONCERN | Follow-up: table-driven persisted flags |
| Plan 2.7 routes in `TerminalOutput`; code routes in `TerminalPool` | CONCERN | Intentional (pool is the sole mount point) |
| 12 refactor candidates (duplicated headless bootstrap, cell-to-text, imperative-handle size, naming) | S/XS | Follow-up; none affects correctness |

## Layer 3 — Correctness

Run after the fixes above:
- `bunx tsc --noEmit`: clean. `bunx jest` on `src/lib/terminal` and `src/components/sessions`: 221 suites, 2318 tests pass. `make ready-vt-conformance-gate`: passes with `baseline.json` ghostty `knownFailing` empty.
- Go: `go vet ./...` exit 0. `go test ./...`: 111 packages ok; `server/services` reported FAIL only from the TestMain goroutine-leak check after all its tests passed (see the item summary for the isolated re-run). `make ci` itself stops at `.tmux-build-3.8.stamp` because the `third_party/tmux` submodule cannot initialise in this worktree (`fatal: Unable to find current revision in submodule path`), before reaching tests.
- Security: no auth, network or shell surface in the diff.

## Layer 4 — Browser

Run against a manual instance with the flag on: ghostty canvas renders, input echoes, pane switch keeps the same canvas with no WASM re-fetch and no console errors. Numbers are in `docs/how-to/vt-conformance-runbook.md`. vim, htop and Claude Code TUI were not run.

## Verdict

PASS with recorded follow-ups, subject to the `server/services` re-run noted in the item summary.
