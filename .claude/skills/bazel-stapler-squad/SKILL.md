---
name: bazel-stapler-squad
description: Use when working with bazel, bazelisk, BUILD.bazel, gazelle, MODULE.bazel, go_deps, .bazelrc or tools/bazel in stapler-squad - building, testing, adding a Go dependency, regenerating BUILD files, or reading a Bazel failure.
---

# Bazel in stapler-squad

Bazel is the build of record (PROVISIONAL until the cutover gate). Gazelle owns Go BUILD files: run it, never hand-edit them.

Every command below is a **proposal** until it is listed in `tools/bzl/verified-commands.txt` with its run log;
`bash tools/bzl/check-skill-proposals.sh` fails CI on an unlisted, unmarked command. Replace proposals with
verified output before the PR merges.

## Commands

| Intent | Command |
|---|---|
| Build the binary | `make build` (wrapper over `bazel build //:stapler-squad`) <!-- proposal --> |
| Test one package | `bazel test //session:session_test --test_output=errors` <!-- proposal --> |
| Test one test by name | `bazel test //session:session_test --test_filter=TestFoo --test_output=errors` <!-- proposal --> |
| Regenerate BUILD files | `make gazelle` <!-- proposal --> |
| Check BUILD files are fresh | `make gazelle-check` <!-- proposal --> |
| Plain go / gopls need generated code | `make sync` <!-- proposal --> |
| Host health, first thing when something looks wrong | `make bazel-doctor` <!-- proposal --> |
| Why is it slow | the `bazel-perf` skill (`--profile`, critical path) |

Targets follow Gazelle naming (`//session:session`, `//session:session_test`); do not override it.
Always go through `tools/bazel` (Bazelisk runs it automatically; direct calls need `BAZEL_REAL`). It queues for one of
N host-wide slots, runs Bazel in a memory-capped scope and, for agents, adds `--config=agent`
(quiet, no ANSI, capped failure view with one remedy line). Do not add `--config=agent` to a Bazel query; it rejects it.

## Gazelle owns BUILD files

- After changing imports or adding/removing a `.go` file: `make gazelle`, then commit the BUILD diff. <!-- proposal -->
- Files listed in `tools/bzl/hand-build-files.txt` are hand-written (generated-code, tmux, windows, web). Their header
  says so and they carry `# gazelle:ignore`; edit those by hand, nothing else.
- Directives (`# gazelle:...`) live in the root `BUILD.bazel`; a change needs owner review.
- Rule: `.claude/rules/bazel-build-files.md` loads when you edit a BUILD or `.bzl` file.

## Add a Go dependency (proposal - not spike-verified)

Steps (verify each before relying on it):

```
go get example.com/mod@vX.Y.Z
go mod tidy
make gazelle
bazel mod tidy
```

Missing `use_repo` shows as `no such package '@@gazelle++go_deps+...'`; the hint is quoted below.

<!-- errors.yaml:E2 -->
bazel-hint: a Go dependency is not wired into MODULE.bazel. Remedy: run 'go get <module> && go mod tidy && make gazelle && bazel mod tidy'.

## Worktrees

- One output base per worktree (hash of the path); caches (disk, repository) are shared. `make bazel-doctor` shows slots and holders. <!-- proposal -->
- Never run the Bazel-built binary against `:8543`, and never use the Bazel run command on the main binary there. Copy the output to
  `~/.stapler-squad/manual-builds/manual-<N>/stapler-squad` and follow the manual-instance recipe in `CLAUDE.md`.
- Removing a worktree: `bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh` (dry run) lists orphan output bases. <!-- proposal -->
- Concurrent cap: at most 2 backlog work sessions; Bazel slots are host-wide (`BAZEL_WRAPPER_SLOTS`).

## Long builds under an agent Bash timeout

Run the build with the harness background facility, or set `BAZEL_BG=1` so the wrapper detaches at once (exit 76).
Poll with `bash tools/bzl/bg-status.sh` (prints `running` or `finished exit=<n>`). Never re-run a build that is still running:
the second call only queues behind the first.

## Reruns and filters

- `FORCE=1` is the old `go test -count=1` (it adds `--nocache_test_results`). <!-- proposal -->
- `RUN=` maps to `--test_filter`, not `go test -run`. Measured semantics (`spike-results.md`, shard-union section): the
  value reaches Go as an unanchored regular expression, so `RUN=TestLiveState` also runs `TestLiveStateError`; anchor with `^...$`. <!-- proposal -->
- Native `go test` and gopls work after `make sync` <!-- proposal -->, but a native result is never authoritative; Bazel is.

## Failure states

Each error has one place: `docs/how-to/troubleshoot-bazel.md` (anchors `#state-1` ... `#state-11`). The wrapper prints the
same remedy from `tools/bzl/errors.yaml`; never retype a hint, quote it through an `<!-- errors.yaml:E<n> -->` marker.

## Related skills

`bazel-version-upgrades` (rules and Bazel bumps), `bazel-perf` (profiling, memory math), `bazel-github-actions-ci` (CI wiring),
`bazel-angular` (frontend rules), `worktree-disk-cleanup` (Bazel output bases).
