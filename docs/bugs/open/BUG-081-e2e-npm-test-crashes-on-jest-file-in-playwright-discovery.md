# BUG-081: `cd tests/e2e && npm test` crashes immediately — Playwright picks up a colocated Jest test file

## Summary

The literal command this repo's own `CLAUDE.md` documents for running the e2e suite
(`cd tests/e2e && npm test`, and the equivalent raw `npx playwright test`) crashes
before running any real spec:

```
ReferenceError: jest is not defined
    at tests/e2e/global-setup.dev-mode.test.ts:5
```

`tests/e2e/global-setup.dev-mode.test.ts` is a **Jest** unit test (uses `jest.fn()`/
`jest.mock()`), but Playwright's default `testDir: './'` / test-file discovery in
`playwright.config.ts` picks it up as one of its own spec files, since it lives directly
under `tests/e2e/` alongside the real Playwright specs. Playwright's runner has no
`jest` global, so the file throws immediately on import.

## Reproduction

```bash
cd tests/e2e && npm test
# or
cd tests/e2e && npx playwright test
```

Both exit 1 immediately after the isolated test server finishes starting, before any
spec's `test()` blocks run.

## Why this hasn't been caught by CI

No CI job actually runs this literal unscoped command and checks its exit code:
- `.github/workflows/demo-publish.yml` runs `npx playwright test --reporter=list || true`
  — the `|| true` swallows the failure.
- `.github/workflows/e2e-video.yml` / `ux-analysis.yml` scope their invocations to
  specific spec files (e.g. `npx playwright test remote-workspaces.spec.ts`), which
  never triggers the broad discovery that picks up the Jest file.

So every CI path that exercises e2e tests happens to sidestep this, while the
documented "just run the suite" command in `CLAUDE.md` is broken.

## Fix approach

Exclude `*.test.ts` from Playwright's spec discovery in `playwright.config.ts` (e.g.
via `testMatch: /.*\.spec\.ts/` or an explicit `testIgnore` for `*.test.ts`), or move
`global-setup.dev-mode.test.ts` out of `tests/e2e/` into a location Playwright's
`testDir` doesn't scan (it presumably needs to stay reachable by whatever Jest config
runs it — check `tests/e2e/package.json`'s test scripts / any Jest config file first).

## Why not fixed here

Discovered while verifying `tests/e2e/remote-workspaces.spec.ts`
(ssh-remote-workspaces Phase 6 Epic 6.3, backlog item `3461c8dd-a3b5-4543-8055-204c183ae396`)
during final review — confirmed pre-existing (the colliding file was last touched
2026-07-09, well before this backlog item started) and unrelated to that change, which
only added a new, separate spec file. The actual scenario under review
(`npx playwright test remote-workspaces.spec.ts`, scoped to one file — the same
pattern the working CI jobs already use) runs and passes cleanly; this bug is about the
unscoped/documented invocation, which is a pre-existing repo-tooling issue rather than
something this backlog item's changes caused or are positioned to fix without expanding
scope into `playwright.config.ts`/Jest wiring unrelated to SSH remote workspaces.
