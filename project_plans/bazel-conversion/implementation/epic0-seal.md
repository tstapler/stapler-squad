# Epic 0 sealing summary (interim) — bazel-conversion

**Branch**: `sdd/bazel-epic0` (tip at write time: merge `44bb98334`; this file is committed on top). **Worktree**: `~/code/github.com/tstapler/stapler-squad.worktrees/bazel-epic0` (moved out of `.claude/worktrees/` with `git worktree move`). Nothing is pushed.

**Status: NOT sealed.** Story 0.14 (the gate for Epics 1-11) cannot pass yet: 3 spikes have no verdict, one verdict is `Fail`, and Task 0.14's checker and economic gate are not built. This document records what is done, what the evidence says, and what each remaining item waits on. Source of truth for verdicts: `spike-results.yaml` (13 rows, parses with `yq`); evidence per spike: `spike-results.md`.

## Verdicts (13 recorded)

| Spike | Verdict | Mechanism | Cost |
|---|---|---|---|
| S-gomod | Pass | `go_deps.from_file`, go-git `replace` resolves | M |
| S1 proto Go | FallbackWorkaround | `buf generate` genrule with pinned local plugins; `go_proto_library` + connect failed the byte-compare | M |
| S1-ts | Pass | `buf generate` + `protoc-gen-es` from the Bun install | S |
| S2 ent | FallbackWorkaround | declared-output `ent_gen` rule + entc program (no fork); importer check later satisfied: `//session/... //server/...` 253/253 built, 5/5 importer tests passed | M |
| S3 embed | Pass | `copy_directory` + `embedsrcs` | S |
| S4 Bun web build | Pass (**contested**) | repo rule + no-sandbox; see D2 | M+M+S+S |
| S5 tmux | Pass | rules_foreign_cc 0.17.0 `configure_make`, tmux 3.8, static link repaired | M |
| S7 | Pass | direct | S |
| S8 sandbox census | Pass | 484 failing cases over 10 packages, each in exactly one class | M |
| S9 short socket path | Pass (caveat) | Bazel 9.3.1 already mounts a private short `/tmp`; the "without" arm is a stand-in bind under the long `TEST_TMPDIR` (169 bytes, fails) because the mechanism cannot be switched off | S |
| S11 Gazelle | Pass | directives + `map_kind`, no extension; 6 resolve lines, idempotent | M |
| S12 nogo | FallbackWorkaround | analyzers need root-x/tools twin libraries (link-time collision with the isolated `tools/lint` module); +415 actions (+31%) on a 130-target stand-in; 14 of 15 analyzer tests still skip under Bazel | M |
| S13 lockfile merges | **Fail** | owner-gated: GitHub-side merge-driver check not run; replay favours commit lock + `bazel-lockfile-merge` driver + auto regen (driver alone left 13/20 failing `--lockfile_mode=error`) | S |

**No `FallbackFork`** — no rules repo needs forking so far (Task 0.14b has nothing to create).

## Not run (no verdict)

| Item | Blocked on |
|---|---|
| S6 worktree cost (Story 0.10) | host load <= 2.0 (it has been 3-32); live-service OOM drop-in (Task 0.2i) is a stated precondition |
| S14 e2e slicing (Story 0.15) | quiet host; its outcome decides ADR-009 and whether EG5 can clear |
| S10 / S10-redaction remote cache (Story 0.12) | owner: onyx storage budget <= 100 GB, listeners :9092/:9093, Tailscale ACL, TLS cert, `bazel-cache-write` env, `main` ruleset decision D-PROTECT-WRITE |
| Baseline timings Tasks 0.1b, 0.1e, 0.1j | quiet host (load guard in `lib.sh` has no override) |
| Task 0.1g targets T1-T9 | owner confirmation (`targets_confirmed` in `owner-attestations.yaml`); `targets.yaml` is a scaffold |
| Task 0.14 (checker `check-spike-results.py`, `economic-gate.py`, ADR finalisation) | the above |

## Measured baselines that matter (from `tools/bzl/baseline/measured.json`, `baselines.md`)

- CI time-to-green: p50 2339 s, p90 4298 s, p95 5001 s (n=30).
- **RA2 = 0.987** — non-Go share of the critical path; e2e is ~1974 of 2601 s. **EG5 trips by construction** until S14 shows e2e can be sliced (ADR-009). This is the biggest threat to the economic case: Bazel alone cannot move time-to-green.
- RA1 median fan-out 0.259 (target-model 0.053): narrow enough to favour selective testing.
- Regen steps median 0 (T6 degenerate). Fresh-clone `go build ./...` fails with 50 missing packages. 3373 tracked `.go` files, 154 test packages. The feature registry covers ~53% of specs, so ADR-009 uses observed-dependency slices.

## Findings that change the plan (apply before Epic 1)

From S8/S9: the 484 failures are `cwd` 211, `shard-timeout` 190 (one hung test, `TestStreamViaHub_should_SendHubStartFailedError_...`, needs `tmux` on PATH), `exec-host-binary` 62, `home` 21.
1. Task 4.1b: drop the global `--test_env=STAPLER_SQUAD_TMUX_SOCKET` (3 `session/mux` + 4 `session/tmux` tests assert their own socket names) and `--test_env=TMPDIR=/tmp` (no-op; `--test_env=TMPDIR` is ignored).
2. Epic 4 needs a test HOME fixture with a gitconfig (or `GIT_AUTHOR_*`), `tmux`/`gh` on the test PATH or tests that skip, and `data` for `testdata/`, `scripts/`, `web-app` fixtures (Gazelle excludes them).
3. The "49 of 154 sandbox-hostile" figure is not re-derived (only 10 packages ran).

From other spikes / worker notes (all unapplied in `plan.md`): `@rules_go` not `@io_bazel_rules_go`; `--config` tag chains are not additive (last `tags=` wins); `--nodisk_cache` -> `--disk_cache=`; `proto_library` loads from `@protobuf//bazel:proto_library.bzl`; `toolchains_protoc` unnecessary; `UpsertRule` symbol does not exist (evidence is `OnConflict*`); `packages.bzl` not `packages.txt`; `bazel build //session/ent/...` does not build the 52 generated libs (they are `//tools/bzl/ent`); `make test PKG=` does not exist; Task 1.1a should fail on Gazelle "matches multiple rules"; `bazel query` rejects `--config=agent`; read-only files in `.bazel-out` need `chmod -R u+w`; `tools/bazel` needs `BAZEL_REAL`; `.bazelignore` needs `web-app/node_modules`, `web-app/out`; Task 0.13a's verify path is `//tools/bzl/spike-nogo:seed`; follow-ups F6-F11 at the end of `plan.md`. 141 generated BUILD files are deliberately not committed (Task 1.1a); the census worktree (`bazel-epic0-s89-census`) holds uncommitted Gazelle edits to 23 already-committed BUILD files that must not be merged as-is.

## Incident (2026-11-01 21:40)

`/tmp` is a 31 GB RAM tmpfs. An S13 worker left a ~16-17 GB scratch there, the kernel OOM-killed a Bazel JVM (`oom_score_adj` 1000 via `~/.config/systemd/user/stapler-squad.service.d/oom.conf`), and `stapler-squad.service` (`OOMPolicy=stop`) stopped 2 s later and restarted (NRestarts=2). The service is active on :8543/:8444. The cause of the 16 GB scratch is unconfirmed; the causal chain is not fully traced. Memory note saved. Worker rules since: scratch on disk, <= ~1 GB, `free -g` check, <= 3 concurrent spikes.

## Owner decisions needed (in gate order)

| # | Decision | Recommendation |
|---|---|---|
| D1 | S13: run the GitHub-side merge-driver experiment (which repo?) and set ADR-001 lockfile policy | commit lock + `bazel-lockfile-merge` driver + auto regen |
| D2 | S4: accept Pass or apply the strict reading. The parity oracle `make web-build` is nondeterministic against the default env (174 of 380 files differ); Pass holds only with pinned Bun 1.4.3 / Node 24.13.1 / `BUN_INSTALL_GLOBAL_STORE=0` | treat as FallbackWorkaround unless you pin those in the `make` path too |
| D3 | Story 8.4: nogo with per-analyzer twin libraries (M + ~S each) or keep only `make lint-custom` | keep `make lint-custom` for now, twins later |
| D4 | Proto plugins: pin remote buf plugins or move proto-gen onto Bazel (Epic 2); fix the 72 `buf lint` findings | — |
| D5 | Epic 1 deletes the abandoned S1 `go_proto_library` attempts | yes |
| D6 | Service OOM: `OOMPolicy=continue`, or isolate Bazel servers in a capped scope; apply Task 0.2i drop-in `zz-oom-protect.conf` (owner-run; the existing drop-in deliberately makes agents sacrificial) | decide before S6 |
| D7 | Confirm ADR-004's changed failure policy (it was Accepted before the change) | — |
| D8 | Confirm targets T1-T9 and EG1-EG5; ADR-010 Tier B/C, freeze windows, T1 definition, T9 values | — |
| D9 | Remote cache setup items on onyx/GitHub (see S10 row); D-PROTECT-WRITE | — |
| D10 | Smaller: soak N=30/K=10; 900 s queue timeout; waiver policy (90-day, 20-char); `go.mod` bump to Bazel-resolved versions and root `go.mod` skew; CODEOWNERS API check after merge; `make setup-git` per clone; file issues for two UNFILED entries in `tools/bzl/test-exclusions.yaml` | — |

## What an agent can do next without you

1. Apply the plan-text corrections above to `plan.md` (docs only).
2. Build Task 0.14a's `check-spike-results.py` (it will report S6, S10, S10-redaction, S14 missing and S13 `Fail`).
3. When load drops to 2.0 or less: S14, S6 (after D6), and the baseline timings; the host has not been quiet since the census started.

## Housekeeping

- Planning branch `sdd/bazel-conversion-plan` (main checkout) holds the plan: 14 epics / 88 stories / 269 tasks, ~38.3M CU after verification, ADRs 001-010 (002/003/007/009/010 Proposed).
- Per-spike branches `sdd/bazel-epic0-*` are all merged into `sdd/bazel-epic0`; their worktrees still sit under the main checkout's `.claude/worktrees/` and can be removed (`git worktree remove`) after you confirm — not done here.
- Pre-existing unrelated dirty files in the main checkout were left untouched.
