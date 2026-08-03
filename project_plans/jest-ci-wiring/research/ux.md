# DX Research: Wire Jest (web-app) into CI

## Scope call

This is a pure CI/infra change — a new `npx jest` step in `.github/workflows/lint.yml`.
There is no UI, no end-user-facing surface, no accessibility surface, and no
job-to-be-done for an end customer. **No end-user-facing UX applies.** The only
"users" are developers submitting PRs to this repo, who will see a new required
check in the GitHub Checks UI. This doc does a lightweight developer-experience
(DX) pass instead of a UX review.

## What good failure feedback looks like here

Reviewed `.github/workflows/lint.yml` (all steps run in a single `golangci` job,
`runs-on: ubuntu-latest`) for the existing failure-surfacing patterns a new Jest
step should match:

- **`golangci-lint` step** (`golangci/golangci-lint-action@v7`) — this is the
  gold standard in this repo: the action natively posts **inline PR annotations**
  (file + line + message) in the GitHub Checks/Files-changed UI, on top of the
  plain job-log output. No extra reporter action is added on top of it — the
  action does this itself.
- **ESLint step** (`npx next lint $FILE_FLAGS`) — no annotation action wraps it.
  Output is raw log text only; a developer has to open the Actions log tab to see
  which line failed. This is the weaker precedent, not the target.
- **`gofmt` check step** — plain shell script, `echo`-formatted list of
  unformatted files straight to the log with a suggested fix command
  (`gofmt -w $(git diff ...)`). Also log-only, no annotations, but at least
  prints an actionable remediation command inline.
- **Feature catalog validation step** — `tsc --noEmit` + custom script, log-only.

Pattern in this repo: only `golangci-lint` gets rich inline annotations (because
the upstream action provides it for free); everything else is "readable job-log
output," and that's treated as acceptable — ESLint, gofmt, and the catalog
validator are all shipped that way with no complaints on record.

**Recommendation:** a link to raw log output (i.e., plain `npx jest` output in
the Actions log, matching the ESLint/gofmt/tsc precedent) is acceptable for this
PR. It doesn't need to hit the `golangci-lint` bar of inline file/line
annotations to be consistent with how this repo already treats non-Go checks.
Requirement #2 (fail the job on test failure) is satisfied by Jest's own
non-zero exit code — no extra tooling needed for that.

## Quick win worth flagging (optional, not required)

- **`--ci` flag**: `npx jest --ci` gives cleaner, deterministic CI-mode output
  (disables snapshot auto-write, uses a CI-appropriate reporter). Cheap to add
  to the new step's command and improves log readability at zero cost — worth
  doing as part of "the fix" itself since it's a one-flag addition, not a new
  dependency or workflow.
- **GitHub Actions test-reporter action** (e.g. `dorny/test-reporter` or similar,
  turning Jest JSON/JUnit output into inline PR annotations like
  `golangci-lint` gets): this would close the gap between the Jest step and the
  golangci-lint bar. Explicitly **out of scope** per requirements.md — that
  spec's ceiling is "fail the job on test failure," and reporting/coverage
  integration is called out as out of scope. Flagging it here only as a
  potential fast-follow, not something to build now.

## Bottom line

- N/A — no end-user-facing UX surface; this is CI/DX only.
- Failure surfacing: plain log output (ESLint/gofmt precedent) is acceptable;
  inline annotations (golangci-lint precedent) would be nicer but isn't required
  and isn't blocking.
- One cheap DX improvement worth folding into the implementation itself:
  add `--ci` to the Jest invocation for cleaner CI-mode output. A dedicated
  annotation-reporter action is a legitimate fast-follow but is out of scope
  per requirements.md.
