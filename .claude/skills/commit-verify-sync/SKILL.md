---
name: commit-verify-sync
description: Use when asked to "push what we have," "commit and make sure CI passes," or "sync with main" — commits local changes as logical Conventional Commits, pushes to origin/main, watches the resulting CI run, and root-causes and fixes any failure (including flaky tests) before considering the work done. Not for feature branches/PRs — this repo pushes directly to main.
---

# Commit → Verify CI → Sync With Main

Four-step loop for landing local work on `main` with actually-green CI, not just a
green push. This repo (`tstapler/stapler-squad`) commits straight to `main` — no PR
branch, no review gate — so CI on `main` is the only safety net. Treat a red run on
`main` as urgent, not as something to note and move past.

## 1. Stage deliberately, not broadly

`git status` first. Stage only the files that belong to the logical change you're
landing — **never `git add -A`/`git add .`** in this repo; other in-flight work
(uncommitted web-app CSS, half-finished experiments) routinely sits in the working
tree alongside what you're actually landing. Split unrelated changes into separate
commits rather than bundling them.

Use [Conventional Commits](https://www.conventionalcommits.org/) prefixes per
`CLAUDE.md`'s release-please table — `fix:`/`feat:`/`feat!:` drive version bumps,
`chore:`/`docs:`/`refactor:`/`test:` don't. Pick the prefix that matches what
actually changed, not what's easiest to type.

## 2. Push to main

```bash
git push origin main
```

No `--force`. If the push is rejected (someone else landed first), `git pull
--rebase origin main` and resolve, don't force past it.

## 3. Watch the CI run to completion — don't fire-and-forget

Find the run the push triggered and block on it:

```bash
gh run list --repo tstapler/stapler-squad --branch main --limit 5
gh run watch <run-id> --repo tstapler/stapler-squad --exit-status
```

`gh run watch` is long-running (minutes) — launch it as a background command so you
can keep working, and treat its completion notification as the trigger to check the
result. Don't poll it in a sleep loop.

A push typically fans out into several workflows (Build, Lint, Benchmarks,
Release Please, etc.) — watch the one(s) actually gating correctness (Build/Lint at
minimum); Release Please and Benchmarks aren't blocking in the same sense.

## 4. On red: root-cause and fix, don't re-excuse

If CI fails, pull the failing job's log (`gh run view <run-id> --log-failed
--repo tstapler/stapler-squad`) and fix the actual cause before pushing again.

**Flaky tests are the trap here.** Per
`.claude/rules/fix-flaky-tests-dont-defer.md`, "known pre-existing flake, unrelated,
re-ran and it passed" is not an acceptable resolution — it's exactly the pattern
that let `TestRemoveHooksConfig_...` and BUG-074's three tmux-restart tests get
re-excused across multiple PRs before finally being root-caused. If a failure looks
flaky:

1. Reproduce locally with a repeat count before trusting a single failing run:
   `go test -run '<TestName>' -count=8 ./path/...`
2. Root-cause it for real — a missing synchronization point, shared mutable state
   across parallel tests, a fixed/non-unique temp resource name, or (as in BUG-074) a
   dependency-injection bypass where one code path constructs its own real
   dependency instead of using an injected/mock one. Don't stop at "it's timing
   related" — name the actual race.
3. Fix it in the same session if it's in scope; if it's genuinely out of scope for
   the current change (e.g. it'd require touching shared test infra other in-flight
   work also depends on), file it as a tracked bug in `docs/bugs/open/` immediately
   — don't let "later" mean "never."
4. Re-verify with the same repeat-count command before pushing the fix, and again
   confirm the real CI run goes green (not just the local repro) — a fix that only
   works locally, or a bug doc that documents a theory you never confirmed against
   the actual failing code path, doesn't count as done. BUG-074's own history is the
   cautionary example: its first write-up (an async registry-race theory) was
   plausible-sounding but wrong, and wasn't caught until someone actually traced
   which executor the failing tests' mocked dependency injection was using.

Only commit the fix (staged narrowly, per step 1) once local repeats and the CI
run both confirm green — then loop back to step 2.

## Local build/test/lint commands referenced above

```bash
make build && make test     # generates protos, then tests
make lint                   # required — make build fails if this fails
make quick-check             # build + test + lint together
go test ./session/...        # targeted package run
```
