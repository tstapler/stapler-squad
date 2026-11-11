# Wave 0 baseline (Story 0.1.4, tasks 0.1.4a and 0.1.4b)

Recorded 2026-11-08 in worktree `~/.cache/ssq-wt-mobile-input`, branch `feat/mobile-input-layer`, before any feature code. Scratch and `GOTMPDIR` lived under `~/.cache/tmp`, never `/tmp`.

## Versions

bun 1.4.4, node v26.8.1, go1.26.6 linux/amd64 (Manjaro, 61 GB RAM).

## Commands, results, wall time

| Command | Result | Wall time |
|---|---|---|
| `cd web-app && unset npm_config_user_agent && bun install` | 1406 packages, exit 0 (store shared via `~/.stapler-squad/shared-node-modules`) | 0.4 s |
| `make build` (repo root) | exit 0, protos + ent + web UI + Go binary | 44.5 s |
| `cd web-app && bunx jest --no-coverage --testPathPatterns="lib/terminal"` | 40 suites passed / 40, 418 tests passed / 418 | 8.3 s |
| `go test ./server/services -run TestNoSuchTest -count=1` | ok (compiles, no tests ran) | 25.3 s |
| `make ready` | FAILED at the `test` step, see below. Stopped the chained rerun (`make -k ready`) after about 10 min while it was in `test-race`; per the brief, fell back to `make lint` plus web lint | 5 m 52 s (first run) |
| `make lint` | exit 0: golangci-lint 0 issues, custom analyzers ok, shellcheck ok (69 scripts) | 3 m 15 s |
| `cd web-app && bunx eslint src --max-warnings=0` | 0 errors, 1248 warnings (pre-existing; exit 1 only because of `--max-warnings=0`) | 16 s |
| `cd web-app && bun run lint:css && bun run lint:css-vars` | ok | about 5 s |
| `cd web-app && bun run lint:duplicates` | exit 0, 9 clones, **295 duplicated lines (0.07%)**, 3227 tokens (0.08%), threshold 0.14% | 28 s |

`make ready` first run: `DONE 14953 tests, 39 skipped, 9 failures in 180s`. The second run (`make -k ready`) reproduced the same 9 failures at the same step.

## Pre-existing failures (not fixed here)

1. `session/vc` (9 test failures: `TestDetectVCS`, `TestNewProvider`, `TestIsJujutsuRepo`, `TestNewJujutsuProvider` and subtests). `DetectVCS()` returns Jujutsu for empty and git directories. Cause (INFERRED from `session/vc/detector.go:42`, `isJujutsuRepo` walks parents looking for `.jj`; `/home/tstapler/.jj/` exists, and `TMPDIR` is under `$HOME`): temp dirs under `$HOME` resolve as Jujutsu repos. A standalone `go test ./session/vc -count=1` passed on re-run, so the result depends on the temp dir location; not confirmed in isolation. CI has no `~/.jj`, so this is local-environment only.
2. `eslint .` in `web-app/` reports 336 errors, all from the untracked build output `web-app/.next/` (created by `make build`); `eslint src` has 0 errors. The 1248 `src` warnings exist at baseline, so a zero-warning bar cannot be assumed locally; compare against this count (or use `lint:affected`) when judging feature-code lint.
3. `bun run lint:duplicates` fails with `Cannot find module '@jscpd/finder'` under the shared bun store (undeclared dependency of `@jscpd/html-reporter`). Workaround that produced the numbers above: `NODE_PATH=$PWD/web-app/node_modules/.bun/node_modules`.

Not run to completion (killed after 10 min budget): `test-race`, `test-integration`, `ready-complexity-gate`, `ready-duplication-gate-web` via make, `ready-vt-conformance-gate`, `lint-workflows`. Re-run `make ready` once at the end of the critical path.

## Task 0.1.4a: jscpd fixture exclusion

`web-app/.jscpd.json` `ignore` now also has `**/terminal/mobileInput/__fixtures__/**` (the replay-fixture directory is `web-app/src/lib/terminal/mobileInput/__fixtures__/`, per plan.md Story 1.1.3). `format` is not set, so every format is scanned and JSON counts.

Proof with two synthetic 80-event JSON files in that directory:

| Config | Duplicated lines | Clones |
|---|---|---|
| With exclusion | 295 (0.07%), same as baseline | 9 |
| Without exclusion | 778 (0.19%), over the 0.14% threshold | 10 |

The synthetic files were deleted afterwards.

## Task 0.1.4b: parallel-worker rules

- Each parallel worker uses its OWN git worktree created from the feature branch `feat/mobile-input-layer`; no two workers share a working tree.
- Scratch stays on disk below 1 GB; check `free -g` first. Never use `/tmp` (RAM tmpfs). Keep `TMPDIR`/`GOTMPDIR` short and outside any `.git` or `.jj` ancestor (see failure 1).
- Before any Go or jest run: `unset npm_config_user_agent`.
- No unscoped `pkill`; kill by explicit PID.
- gocognit/funlen/dupl gates apply to modified existing functions. For the `connectrpc_websocket.go` change, put the logic in a new file and thread only a returned `bool` through the `:1653`/`:2489` closures.
- New hooks must pass `react-hooks/exhaustive-deps`, `no-restricted-imports` on `next/*`, and the web lint (note the 1248-warning baseline above).
- Registry check, `config_surface_proof_test.go` and BUG-087 (no "gate" in Go test filenames) run in the same task as the flag (1.3.1a).
