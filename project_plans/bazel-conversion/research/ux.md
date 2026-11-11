# Research: Developer / Agent Experience (UX) — bazel-conversion

**Date**: 2026-11-01. **Scope**: the "users" are the owner, Claude Code agent sessions in many parallel worktrees, and CI.

**Evidence labels**: VERIFIED = opened or ran it in this session. INFERRED = reasoned from verified facts. FROM-MEMORY = background knowledge of external projects/tools, not re-fetched this session (no web research was run); confirm before it goes into an ADR.

## 0. Verified local facts

- `bazel` and `bazelisk` are installed at `/home/linuxbrew/.linuxbrew/bin` (VERIFIED). No `.bazelrc` or BUILD files exist yet (VERIFIED, `ls`).
- Dev host: 24 cores, 61 GB RAM, 623 GB free on `/home` (VERIFIED, `nproc`/`free`/`df`). Many concurrent agents on one box is the real load profile. Bazel server memory times N worktrees is the OOM risk (see section 4).
- `Makefile` has a self-documenting `help` target and about 140 `##`-annotated targets (VERIFIED, grep). Agents already discover commands via `make help`. The wrapper layer should keep that surface rather than replace it.
- `docs/how-to/` has 37 pages. Docs placement rules from `CLAUDE.md`: Diataxis under `docs/{how-to,reference,explanation}`, never `.claude/docs/`. Broad AI checklists become project skills at `.claude/skills/<slug>/SKILL.md`, because skill bodies load only when invoked. Path-scoped guardrails use `.claude/rules/*.md` with `globs:` frontmatter. (VERIFIED, `CLAUDE.md`)
- Existing Bazel skills in `~/.claude/skills` (VERIFIED, read headers and section lists). None is a stapler-squad project skill; all are global.

| Skill | Lines | Covers | Reusable here |
|---|---|---|---|
| `bazel-version-upgrades` | 46 | Bazel 9: core rules no longer built in (need `load` from `rules_*`), `MODULE.bazel` mandatory, repo mapping, "name not defined" troubleshooting | Direct. Cite it in the error table for "name 'sh_binary' is not defined". |
| `bazel-github-actions-ci` | 390 | `bazel-contrib/setup-bazel` (disk-cache per job, `repository-cache` and `external-cache` shared, `cache-save` only on push), hardening, draft-PR skip, `test_suite` vs `alias`, "sandbox `user.home` writes fail", `.bazelrc.user.example` for local disk cache, anti-patterns | High. The CI cache and sandbox-HOME sections apply directly. Compose/X11/Kotlin/Android parts do not. It is a SteleKit retrospective, not generic. |
| `bazel-perf` | 145 | Profiling: `--profile` + Perfetto, jq top-N, elapsed vs critical path, strategy tally, persistent-worker memory math, "do not add `--jobs` blindly" | High for the Phase-2 baseline method and the OOM lessons. Contains SteleKit paths (`//kmp:...`) that must be generalised. |
| `bazel-kotlin` | 258 | Kotlin/JVM `MODULE.bazel`, Gazelle JVM, Maven lockfile workflow, `.bazelversion`/`.bazelrc` | Pattern only (lockfile-refresh workflow maps to `go.mod` + `go_deps`). Not Go-specific. |
| `bazel-angular` | 162 | `aspect_rules_js` + Bazel 9 in the sandbox, `ng build` sandbox trap, bundle embedded into a Rust binary, "verify an edit changes the binary" | High for web-app to `go:embed` coupling. It uses an npm lockfile, so `bun.lock` support is still an open question. |

**Gap**: no skill covers rules_go, Gazelle for Go, `go_deps`/bzlmod, proto/buf codegen, cgo, or worktree workflow. The project skill (section 7) fills that gap and should link to the five above rather than copy them.

## 1. Comparable DX patterns in Go+web Bazel repos (all FROM-MEMORY)

| Pattern | What large repos do | Fit for this repo |
|---|---|---|
| Keeping `go build`/gopls working | Repos with rules_go commonly keep `go.mod` authoritative and let `go_deps` read it, so plain `go build` and gopls still work for hand-written code. Where code is generated, they either commit it or provide a sync target. rules_go also ships a `gopackagesdriver` so gopls can read Bazel's view (`GOPACKAGESDRIVER`). | Generated code (`gen/`, `web-app/src/gen/`, `session/ent/*`) is gitignored by rule, so plain `go build`/gopls breaks in a fresh worktree unless a sync step materialises it. Plan for `bazel run //:sync` (below) and document the gopackagesdriver as an option. |
| `bazel run //:gazelle` | Standard: a root `gazelle(name="gazelle")` target. `# gazelle:prefix`, `# gazelle:exclude`, `# gazelle:resolve` directives live in the root BUILD file. CI runs `bazel run //:gazelle` then `git diff --exit-code`. | Matches the stated "Gazelle freshness gate" in requirements. |
| `bazel run //:sync` for generated code | Pattern: a single target that copies Bazel-built generated outputs back into the source tree for editors, using `write_source_files` from `aspect_bazel_lib`, which also adds a `diff_test` for committed outputs. | Adapt: here the outputs are not committed. The target would write them to the gitignored paths only. A staleness `diff_test` is not needed. Do not commit, per repo rules. |
| Watch loops | `ibazel build/test //pkg:target` re-runs on file change. Used for web/TS inner loops. | Web inner loop already has Bun dev server on port 62875. Do not route it through Bazel. Offer `ibazel` only for Go test loops, optional. |
| `.bazelrc` layering | Committed `.bazelrc` with `common`/`build`/`test` defaults, named configs (`--config=ci`, `--config=race`, `--config=linux`/`macos` via auto `--enable_platform_specific_config`), and `try-import %workspace%/user.bazelrc` for personal overrides (gitignored). `.bazelrc.user.example` documents it (the `bazel-github-actions-ci` skill already recommends this). | Adopt as-is. |
| Wrapper script | Many repos ship `./tools/bazel` or `./bazel` wrapper (bazelisk-aware) to inject flags or to handle first-run. `.bazelversion` pins the version for Bazelisk. | `.bazelversion` yes. A wrapper is only worth it if we need output-base policy (section 3); otherwise `make` wrappers suffice. |
| Test tags | `size`, `tags = ["manual","requires-network","no-sandbox","exclusive"]` plus `--test_tag_filters` configs for slow/integration lanes. | Maps the existing `make test` (skips slow) vs `test-integration` split, and `gotags` (`integration`, `sinkguard`, `gitoracle`, `embed_*`) to configs. |

## 2. Mental models and command discoverability

An agent landing in a fresh worktree knows the current answer from `CLAUDE.md` and `make help`. After cutover the same questions need one obvious answer each. Proposed canonical command table (this is the content for the CLAUDE.md section; it is a proposal, none of it exists yet):

| Agent intent | Today | Post-cutover command (proposal) | Expected failure shape |
|---|---|---|---|
| Build the binary | `make build` | `make build` (wrapper) = `bazel build //:stapler-squad` | Compile error with label and file:line |
| Test one package | `go test ./session -run TestFoo` | `bazel test //session:session_test --test_filter=TestFoo --test_output=errors` | Test log path printed; with `--test_output=errors` failing output inline |
| Test everything affected | `make test-affected` | `make test-affected` wrapping `bazel test` with a target list from the change detector (Phase 3 decision: bazel-diff or `rdeps` query) | n/a |
| Regenerate BUILD files | n/a | `make gazelle` = `bazel run //:gazelle` | Diff in working tree |
| Add a Go dependency | `go get` | `go get x && make gazelle && bazel mod tidy` (exact steps depend on rules_go `go_deps` plus `use_repo`; VERIFY in Phase 3) | "no such package @@gazelle++go_deps+..." means `use_repo` not updated |
| Add or edit proto / ent schema | `make proto-gen` / `make ent-gen` | Edit, then `bazel build //...`; to refresh editor view run `make sync` | Codegen action failure shows the tool's own stderr |
| Run the live-adjacent manual instance | CLAUDE.md recipe | Same recipe, with `bazel build` output copied to `~/.stapler-squad/manual-builds/manual-N/` (must not overwrite `./stapler-squad`) | n/a |
| "Why is this slow?" | `go test -v` | `bazel-perf` skill: `--profile`, elapsed vs critical path | n/a |

Principles (INFERRED from the repo's current conventions and the "agent-authored repo" risk in requirements):

1. **One discovery surface**: keep `make help` as the index and have every Bazel-backed target show up there with the underlying `bazel` command in its `##` text. Agents then copy the real command and learn it.
2. **Targets are named after Go packages**: Gazelle default naming (`//session:session`, `//session:session_test`) gives agents a predictable mapping from `go test ./session` to a label. Do not override naming without a strong reason.
3. **Never ask an agent to hand-edit BUILD files for Go imports.** Rule: after changing imports, run `make gazelle`. Hand-edited bits (genrules for ent/proto/tmux/web) get a "managed by hand" header comment, and Gazelle directives like `# gazelle:ignore` mark them.
4. **Error and remediation text should live in one place** (reference page), linked from the skill (section 4).

## 3. Worktrees: the core multi-agent DX problem

Requirement rabbit hole: Bazel output bases are keyed on workspace path, so N worktrees means N output bases plus N Bazel server JVMs unless shared caches are configured. Relevant facts and options:

- A shared `--disk_cache` and `--repository_cache` (and `--distdir`) across worktrees removes repeated downloads and action re-execution. Disk cache has no built-in eviction in older Bazels; `--experimental_disk_cache_gc_max_size`/`--experimental_disk_cache_gc_max_age` exist in recent versions (FROM-MEMORY, verify flag names against `bazel help build` on 9.3.1 in Phase 3). Concurrent writers to a disk cache are designed to be safe (content-addressed with atomic rename; FROM-MEMORY).
- The `bazel-github-actions-ci` skill already contains the `.bazelrc.user.example` convention: `build --disk_cache=~/.cache/bazel-disk-cache`. Reuse it. A shared path in the committed `.bazelrc` (e.g. `common --disk_cache=~/.cache/stapler-squad-bazel/disk`) gives agents the benefit with zero setup, which is the better default for agent sessions; CI overrides with `--config=ci`.
- Per-worktree Bazel server memory: each worktree's server idles with a JVM. With 24 sessions possible this is the OOM risk. Mitigations (INFERRED; measure in Phase 2): `startup --max_idle_secs=300` so idle servers exit, a cap on `--local_resources`/`--jobs`, and the existing "backlog WIP limit of 2" memory (2026-07-12 OOM incident) as an upper bound on concurrent heavy builds. A single shared output base across worktrees is not safe: Bazel serialises commands per output base, so agents would block each other. Treat that as rejected unless Phase 2 shows otherwise.
- The `worktree-disk-cleanup` project skill (modified, uncommitted at session start) must learn about `bazel-*` symlinks and output bases (`bazel info output_base`, `bazel clean --expunge`) so cleaning a worktree also reclaims its output base. Add as a Phase 5 task.
- Worktree creation cost today is the `bun install` plus `proto-gen`/`ent-gen` preconditions. The success metric "`bazel build //...` works on a fresh clone" removes that precondition; the `sync` target is only for editors/gopls.

## 4. Output readability and context cost for agents

Agents pay for every output token, so log verbosity is a DX cost. Proposed flags, in a `--config=agent` (auto-applied via `user.bazelrc`-style opt-in or by the make wrappers; FROM-MEMORY on flag semantics, verify names on 9.3.1):

| Flag | Purpose |
|---|---|
| `--color=no --curses=no` | No ANSI or cursor redraw in captured logs |
| `--noshow_progress` (or `--show_progress_rate_limit`) | Drop the progress spinner lines |
| `--noshow_loading_progress` | Drop "Loading: N packages" lines |
| `--test_output=errors` | Print test log only on failure |
| `--test_summary=terse` | Short per-target result list |
| `--verbose_failures` | Full failing command line on action errors (needed to reproduce) |
| `--build_event_json_file=<path>` | BEP written to a file, not stdout; analyse with `jq` instead of reading it. In CI, upload it (requirements Observability section). |
| `--profile=<path>` | Trace for the `bazel-perf` workflow |

Notes:
- Keep human output intact for the owner (`--config=dev` default can stay interactive); the wrappers in `make` choose agent-friendly flags when stdout is not a TTY (INFERRED, simple `[ -t 1 ]` check).
- Large Go test output is the main token sink. `--test_output=errors` plus `--test_summary=terse` bounds it. Point agents at `bazel-testlogs/<pkg>/<target>/test.log` for the full log rather than streaming it.
- The existing project guidance "gotestsum for readable pass/fail-per-test" (CLAUDE.md) has a Bazel analogue: Bazel emits JUnit-style `test.xml` per target; a thin summariser can render it. Decide in Phase 3 whether to keep gotestsum around Bazel or use `test.xml`.

## 5. Error states and the remediation each should produce

Each needs a one-screen entry in `docs/reference/bazel-troubleshooting.md` (proposed) and a one-line pointer in the skill. Messages below are what the doc should say; the "symptom" strings are FROM-MEMORY and must be reproduced during implementation before being quoted verbatim.

| # | Error state | Symptom | Remediation to print / document |
|---|---|---|---|
| 1 | Stale BUILD file (new import not in `deps`, or deleted file still in `srcs`) | `undefined: foo` / `no such file` / `missing strict dependencies` | "Run `make gazelle` and commit the BUILD diff." CI gate: `bazel run //:gazelle && git diff --exit-code` failing with the same sentence. |
| 2 | Missing external dep | `no such package '@@gazelle++go_deps+com_github_x'` or `repository ... not found` | "Run `go get <mod>`, `go mod tidy`, `make gazelle`, then add the repo to `use_repo(go_deps, ...)` in `MODULE.bazel` (or run the tidy command that does so)." Link `go.mod` replace note for `tstapler/go-git`. |
| 3 | Generated code expected but absent (editor/gopls, plain `go build`) | `package .../gen/... not found` | "Plain `go` needs generated code. Run `make sync`. `bazel build` does not need it." This inverts today's `make proto-gen ent-gen` precondition and should be spelled out. |
| 4 | Sandbox failure (test reads `$HOME`, repo-root cwd, network, real tmux/git, writes outside TEST_TMPDIR) | `permission denied`, `no such file or directory` for a path outside the runfiles, `user.home` write failure (documented in `bazel-github-actions-ci`) | Decision tree: (a) fix the test to use `TEST_TMPDIR`/runfiles per the repo policy `fix-flaky-tests-dont-defer`; (b) only if it truly needs host resources, tag `no-sandbox`/`requires-network`/`local` with a comment and a tracking issue. Never silently tag. Reproduce with `bazel test --sandbox_debug --test_output=streamed //x`. |
| 5 | Cache corruption / poisoned local cache | Checksum mismatch, odd stale outputs, or "worked on CI, fails locally" | `bazel clean` first; if persists, remove the disk cache dir (`rm -rf ~/.cache/stapler-squad-bazel/disk`, safe, content-addressed); last resort `bazel clean --expunge`. Do not delete the repository cache casually (re-download). |
| 6 | Bazel server OOM / killed | `Server terminated abruptly (error code: 14)` or "Bazel server crashed" / OS OOM kill; another agent session dies too | "Check `uptime`/`free -g`; reduce `--jobs`/`--local_resources`; run `bazel shutdown` in idle worktrees; cap concurrent builds (backlog WIP limit is 2)." Reference `bazel-perf` OOM lessons (run 33803270917). |
| 7 | Out of disk | `No space left on device` in output base, disk cache, or `/tmp` | Order of reclaim: `bazel clean` in finished worktrees (the `worktree-disk-cleanup` skill), disk-cache GC, then `bazel clean --expunge`. Also memory `instinct_full_disk_tmp_gotmpdir_socket_path.md`: long temp paths break unix-socket tests. |
| 8 | Bazel version drift (agent has a different `bazel`) | Rule APIs missing, "name 'x' is not defined" | `.bazelversion` + Bazelisk; point to `bazel-version-upgrades`. |
| 9 | Concurrent command lock | `Another command (pid=N) is running. Waiting for it to complete on the server` | Expected per output base; not an error. Document: do not kill it, wait, or use a separate worktree. Note the cost: two agents in one worktree serialise. |
| 10 | Remote cache unreachable or creds missing | Warnings about remote cache; slower build | Must degrade to local build (timeout + retries + circuit breaker; `--remote_local_fallback` is a no-op for `--remote_cache` without `--incompatible_remote_local_fallback_for_remote_cache`, VERIFIED by run 2026-11-01). Credentials only via 1Password/GitHub secrets per requirements. |
| 11 | macOS binary unsigned after build | TCC prompts reappear | "Bazel outputs are not codesigned; run `make _codesign-binary` / install path signs." This is a device-verified item per requirements. |

## 6. Jobs-to-be-done

| Job | Functional | Emotional | Social |
|---|---|---|---|
| Owner | One build, one cache, CI that only runs affected work | "Green means shippable": the same Bazel target graph runs locally and in CI, with a parity soak before cutover (requirements Risk Control) | Review agent PRs that touch BUILD files in seconds |
| Agent | Predictable commands; fast cold start in a new worktree; short failure logs | Confidence that a local `bazel test //pkg` result equals CI's | Diff of BUILD changes is mechanical and explained by the skill |
| CI | Hermetic, cacheable, deterministic | n/a | n/a |

**Making BUILD-file PRs reviewable (social job)** — proposals (INFERRED):
- BUILD files for Go are Gazelle-generated, so reviewers skim, not audit. Hand-written Bazel (`MODULE.bazel`, `.bazelrc`, codegen genrules, `.bzl`) gets a `CODEOWNERS` entry or a PR-template checklist so it is reviewed carefully, while `**/BUILD.bazel` generated churn is collapsed with `.gitattributes` `linguist-generated` where Gazelle-managed (marker comment convention is project-defined).
- PR template line: "BUILD changes: generated by `make gazelle` (yes/no); hand-written Bazel changed (list)".
- CI freshness gate (Gazelle) posts one-sentence remediation (error 1).
- Trust: parity checks (identical binary behaviour, same test pass set) reported as a comment during the strangler soak.

## 7. Teaching agents: where each piece goes

Following `CLAUDE.md` Documentation Placement (VERIFIED, read above):

1. **Project skill** `.claude/skills/bazel-stapler-squad/SKILL.md` (always-loaded cost is only the one-line description). Body: command table (section 2), Gazelle rule, worktree rules, agent flags (section 4), pointer table to error states (section 5), and links to the global skills `bazel-version-upgrades`, `bazel-perf`, `bazel-github-actions-ci`, `bazel-angular`. Create with `stapler-squad-add-skill` conventions. Description should trigger on "bazel", "BUILD.bazel", "gazelle", "MODULE.bazel", "go_deps".
2. **CLAUDE.md section** (short, under "Development Commands"): replace the `go build .` / `make build` first block with: "Build of record is Bazel; use `make` wrappers; see the `bazel-stapler-squad` skill; after changing Go imports run `make gazelle`". Add rows to the Reference Documents Index. Keep to about 15 lines; detail goes in the skill and docs (context-cost rule).
3. **Path-scoped rule** `.claude/rules/bazel-build-files.md` with `globs: ["**/BUILD.bazel","**/*.bzl","MODULE.bazel",".bazelrc"]`: do not hand-edit Gazelle-managed rules; header-comment convention; no `.bazelrc` flag changes without reading `bazel-perf` memory math. This is the intended use of `.claude/rules/` (VERIFIED, `CLAUDE.md`). Pairs with a lint/hook.
4. **Diataxis docs**:
   - `docs/how-to/build-with-bazel.md` (build, test one package, add dep, regenerate, run manual instance)
   - `docs/how-to/set-up-bazel-worktree-caches.md` (shared disk/repository cache, `user.bazelrc`, idle-shutdown)
   - `docs/how-to/troubleshoot-bazel.md` (the section 5 table)
   - `docs/reference/bazel-targets-and-configs.md` (`.bazelrc` configs, `gotags` mapping, make-to-bazel wrapper map, label conventions)
   - `docs/explanation/why-bazel.md` (supersession of ci-speed ADR-001, three drivers) plus an ADR in `docs/adr/`.
   Add rows in the `CLAUDE.md` index for each.
5. **Enforcement beats prose** (consistent with the repo's `quality:reflect-and-fix` ladder): Gazelle freshness in `make ready` and CI; a pre-commit hook for BUILD freshness when `*.go` imports change; a test that `make help` lists every Bazel wrapper.

## 8. Open questions for Phase 3 (not decided here)

- Does the committed `.bazelrc` point at a shared home-directory disk cache by default (best for agents) or only via `user.bazelrc` (safer, explicit)?
- Keep gotestsum in the loop or use Bazel `test.xml`?
- Is `GOPACKAGESDRIVER` worth supporting, or is `make sync` plus plain gopls enough?
- Affected-target selection tool (bazel-diff vs `bazel query rdeps` on changed files) — affects the `make test-affected` wrapper.
- Exact `go_deps` + `use_repo` tidy workflow under rules_go for Bazel 9.3.1 and the `tstapler/go-git` replace — verify empirically before writing it into docs.
- Whether pure-Go sqlite (modernc, already in `go.mod`) removes most cgo/toolchain error states from section 5.
