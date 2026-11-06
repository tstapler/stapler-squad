# Verify notes (environment issues and how each was resolved)

## E1 — Playwright chromium headless shell 1194 would not install (RESOLVED, workaround)
- `timeout 120 npx playwright install chromium-headless-shell` -> exit 124 three times; `chrome-linux/` held only `v8_context_snapshot.bin`.
- Workaround (local only, not committed): `~/.cache/ms-playwright/chromium_headless_shell-1194/chrome-linux` symlinked to the already-installed `chromium_headless_shell-1208/chrome-headless-shell-linux64` (Chrome for Testing 145). All e2e results below ran on that browser, not the pinned 1194 build.

## E2 — tmux submodule (RESOLVED, no bypass)
- `make ci` initially failed: `fatal: Unable to find current revision in submodule path 'third_party/tmux'`.
- Fix: `git -C third_party/tmux fetch origin 7f2a35ad...` then `git -C third_party/tmux checkout --detach 7f2a35ad...`. An interim attempt that touched `.tmux-build-3.8.stamp` was discarded (its result is not counted); the stamp was removed and `make ci` rerun.

## E3 — `bun run lint:duplicates` (STILL BROKEN locally)
- `cd web-app && bun run lint:duplicates` -> `Error: Cannot find module '@jscpd/finder'` (`bun install --force`, and `bun pm cache rm` + `bun install --force`, did not help; `make ci` does not run this gate, so its green result says nothing about it). It has NOT run green here.
- Substitute evidence only: `jscpd@4.0.9` installed cleanly with npm outside the repo, run as `node .../jscpd/bin/jscpd src` in `web-app/` with its `.jscpd.json` (threshold 0.14): 7 clones, 0.05% lines / 0.06% tokens, exit 0.

## Results (all from actual runs)
- `make ci` (no bypass, run in background to completion): `exit=0`, 14731 unit tests passed in the first full pass, final lines `PASS: ready / quick-check / pre-commit / install-service`; log lines 13176 `exit=0`.
- Playwright, each project run separately (running both in one invocation shares server state and makes alias specs collide):
  - `jules-dispatch + alias-settings + route-focus-and-announce`, `--project=chromium`: 23 passed. Same for `--project=chromium-dom`: 23 passed.
  - `local-file-browser`, chromium: 5 passed; chromium-dom: 5 passed.
- Jest: `bunx jest --testPathPatterns=LocalFileBrowser` -> 9 passed.

## What the runs found (beyond the planned locator fixes)
- alias-settings AC-02/04/05 also had strict-mode violations: `getByRole("button",{name:"Edit"})` substring-matched "Delete alias e2e-edit" (fixed with `exact: true` in `pages/SettingsPage.ts`); "Add variable" and "Add a tag..." matched the Global Defaults section too (scoped to the alias form); the tag chip assertion now targets its `Remove tag e2e-tag` button.
- local-file-browser symlink: the server deliberately returns the symlink path (`server/services/local_file_service.go`, `Path: filepath.Join(dir, e.Name())`; the existing jest test also expects the link path). Assertion updated to the link path.
- local-file-browser open-terminal: real product bug, `[invalid_argument] title is required` (server/services/session_service_create.go:607). `LocalFileBrowser.tsx` now passes a title derived from the directory name; jest test added.

## Baseline
- 20 rows `fail`->`pass` (jules-dispatch 10, alias-settings 6, local-file-browser 4; counted before/after with python) and 2 route-focus skip-link rows removed (2693 -> 2691). visual-regression untouched.
