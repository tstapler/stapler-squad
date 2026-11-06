# Verify blockers (environment)

## B1 — Playwright chromium headless shell will not install
- Command: `cd tests/e2e && timeout 120 npx playwright install chromium-headless-shell` -> exit 124 (timeout).
- Output: download reaches `100% of 104.3 MiB`, then extraction stalls; `~/.cache/ms-playwright/chromium_headless_shell-1194/chrome-linux/` holds only `v8_context_snapshot.bin`.
- Effect: every browser spec fails with `browserType.launch: Executable doesn't exist at .../chromium_headless_shell-1194/chrome-linux/headless_shell` (20/20 targeted specs, including ones that passed before).
- Retried 3 times; stopped per instruction. No e2e row is claimed to pass.

## B2 — `make quick-check` cannot run here
- `make[1]: *** [Makefile:356: build-tmux] Error 128` / `fatal: Unable to find current revision in submodule path 'third_party/tmux'`.
- Web build and `go build` succeed; `go vet .` run separately.

## Consequences for acceptance criteria
- AC1, AC3, AC4 (pass runs) and AC9's 20 `fail`->`pass` flips: NOT done; baseline rows left as `fail` because flipping without a run would be a guess. Code for them is in place (`keyring.MockInit()` in main.go; 4 alias locators fixed) but unverified in a browser. AC3's symlink/open-terminal cause is still uninvestigated.
- AC2: `go build` passes; the "no D-Bus timeout" half is unverified.
- AC5: done and verified by grep (no `skip link` in the spec) — 2 baseline entries removed (98-line diff).
- AC6: `testIgnore` includes `DEV_SERVER_SPECS = '**/dev-mode-hmr.spec.ts'` for both projects (playwright.config.ts:208, :254).
- AC7: visual-regression rows untouched.
- AC8: not verified (B2).

## B3 — `bun run lint:duplicates` crashes before scanning
- `cd web-app && bun run lint:duplicates` -> `Error: Cannot find module '@jscpd/finder'` (from `@jscpd/html-reporter`), exit 1. No `web-app/` file is changed by this branch.

## sdd:6-verify summary (inline; diff is 3 Go lines + 4 locator strings + 2 baseline rows)
- Idioms/architecture: `gofmt -l main.go` clean; `go vet .` clean; `go-keyring v0.2.8` already a direct dependency in go.mod:51. The plan's optional `jules.InitTestMode()` facade was not added (concern, not blocker).
- Baseline: 2693 -> 2691 specs; 20 target rows (jules 10, alias 6, local-file-browser 4) still `fail` pending a browser run.
- Verdict: NOT PASS-ready for AC1-4, AC8, AC9; blocked by B1-B3, not by a found defect.
