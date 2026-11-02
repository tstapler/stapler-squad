# Validation Plan: bazel-conversion

**Date**: 2026-11-01

This is a build-system migration, so the "tests" are verification gates: spike pass/fail rules (S-gomod..S12), parity gate P1-P10, freshness/static checks, sandbox test-set diffs, secret scans, owner device checks. Type vocabulary: **Spike** (falsifiable `PASS iff` rule, Epic 0), **Static** (script/grep/CI check, no build), **Build** (a `bazel build/test` gate), **Parity** (Bazel vs make equivalence), **Perf**, **Security**, **Migration** (reversibility), **Owner** (human/device step). Plan task IDs are in brackets. "Plan" column cites where plan.md already defines the check; "GAP" rows are verifications this document adds because plan.md has no coverage.

## Happy Path Scenario
Given a fresh clone (no `gen/`, `web-app/src/gen/`, `session/ent/*`, empty `third_party/tmux`, no `make proto-gen ent-gen`), when an agent runs `bazel build //...` and `bazel test //...` with a shared disk cache and then `bazel run //:sync-gen`, then the Bazel-built `//:stapler-squad` binary (embedding Bazel-built web dist, tmux and tymuxd) passes the same Playwright e2e pass count as the make-built binary and `bash tools/bzl/parity/run-all.sh` prints `PARITY: PASS`.

## Requirement Index (40 requirements)

| ID | Requirement (source: requirements.md section) |
|---|---|
| SM1 | CI speed: affected-target selection replaces the three selection mechanisms; time-to-green and cache hit rate measured vs baseline |
| SM2 | Hermeticity: one pinned toolchain; clean checkout == worktree == CI; `bazel build //...` on fresh clone with no `make proto-gen ent-gen` |
| SM3 | Worktree cost: cold-build time and disk per new worktree measured before/after with shared disk (and remote) cache |
| SM4 | Baselines measured in Phase 2 (targets set in Phase 3) |
| SM5 | End state: Bazel is build of record; `bazel build //...`/`test //...` cover Go, proto, ent, tmux, web-app; Makefile thin wrappers; CI runs Bazel |
| SM6 | Package scoping (owner request 2026-11-02, ADR-010): behaviour-preserving refactors make the largest packages smaller and more scoped; payoff measured by T9 and checked against the harness prediction |
| C1 | Bzlmod only; no WORKSPACE |
| C2 | Rule-repo forks allowed, pinned via `git_override`/`archive_override`, upstream preferred |
| C3 | Gazelle wherever it applies; hand-write only what it cannot generate |
| C4 | Free-tier runners, no RBE; self-hosted bazel-remote on onyx + shared local disk/repository cache; CI reachability + graceful fallback |
| C5 | Linux (primary) and macOS arm64/amd64 signed builds keep working |
| C6 | Windows: cross-compile only (windows_amd64), no Windows tests |
| C7 | Repo rules: generated code gitignored/uncommitted; Bun not npm/pnpm; Conventional Commits; live `:8543` service undisturbed |
| C8 | Never force push |
| N1 | Warm no-op `bazel build //:stapler-squad` in seconds; cold CI target from measured baseline |
| N2 | Many concurrent worktrees share one cache without corruption |
| N3 | Remote-cache credentials via 1Password/GitHub secrets, never committed |
| N4 | (Data residency: none) - folded into N3, no verification needed |
| I1 | MODULE.bazel, .bazelversion, .bazelrc (+per-OS/CI configs), Gazelle setup, go.mod replace handling |
| I2 | Go: all packages/tests, tags (embed_tmux, embed_tymux, sinkguard, gitoracle, integration, race), embeds, version stamping |
| I3 | sqlite/cgo decision; darwin cgo files; `godebug netdns=go` |
| I4 | Codegen as Bazel actions: proto (Go+Connect+TS), ent with `sql/upsert`, feature-registry/other generated inputs if applicable |
| I5 | tmux from submodule via bzlmod-compatible rule; tymuxd fetch/embed |
| I6 | web-app via Bun-compatible build producing `server/web/dist`; jest tests |
| I7 | Separate modules (`tuitest`, `tools/scanner`, `tools/lint`), custom analyzers, lint/ready gates mapped or wrapped |
| I8 | Makefile wrappers, `install-service.sh`, GHA workflows, release-please/release artifacts, Formula/ |
| I9 | Playwright e2e global-setup uses built binary; e2e participates in the Bazel graph so a PR re-runs only the specs whose measured dependencies changed (ADR-009) |
| I10 | Docs (how-to/reference/explanation), ADR superseding ci-speed ADR-001, CLAUDE.md update |
| O1 | Out of scope: no remote execution |
| O2 | Out of scope: no application rewrites beyond hermeticity needs |
| O3 | Out of scope: no frontend framework / Next-to-Bun change |
| O4 | Out of scope: no committed generated code |
| RC1 | Strangler: Bazel alongside make/CI non-blocking until parity (binary behaviour, test pass set), per-target cutover |
| RC2 | Rollback: `BUILD_SYSTEM=make` for one release cycle; revert is a single PR |
| RC3 | Live service stays on current build until Bazel binary passes e2e + manual isolated-instance check |
| OB1 | Observability: BEP/profile upload, cache hit rate, time-to-green, per-worktree cold time and disk, trend script |
| OB2 | Gazelle-freshness check |
| OB3 | Existing service metrics/dashboards unaffected |
| OQ1 | Open question: which lint gates become Bazel targets vs wrapped |
| OQ2 | Open question: can aspect_rules_js consume `bun.lock` |

(N4 has no testable behaviour; it is counted as covered-by-vacuity and excluded from the "N/N" ratio below, leaving 39 testable requirements plus N4.)

## Requirement -> Verification Mapping

| Req | Plan task | Verification name | Type | Command | Expected | Failure signal |
|---|---|---|---|---|---|---|
| SM1 | 7.3a/b | affected_targets_should_select_rdeps_and_GlobalInputs_force_all | Static | `bash tools/ci/affected-targets_test.sh` | prints `PASS` x5 (docs-only, go-leaf, proto, MODULE.bazel->`//...`, web) | any FAIL line / non-zero exit |
| SM1 | 7.3c | affected_selection_should_miss_nothing_the_legacy_mechanisms_caught | Parity | `grep -cE '^misses_(regex\|script\|paths): 0$' project_plans/bazel-conversion/implementation/affected-soak.md` | `3` | any non-zero miss count (false negative) |
| SM1 | 10.2c | legacy_selection_should_be_removed_after_retention | Static | `grep -rn 'detect-feature-changes' .github tools Makefile` | no output | any hit |
| SM1 | 7.5 P10 | ci_time_to_green_should_not_regress | Perf | `bash tools/bzl/parity/perf.sh` (p50 over >=20 Bazel PR runs) | p50 <= 1.10x `baselines.md` p50; each required workflow p50 <= 1.10x | ratio > 1.10 |
| SM1 | 7.1a/4c | cache_hit_ratio_should_be_reported_from_BEP | Static | `bash tools/ci/bazel-trend_test.sh` | recorded BEP fixture yields `bazel_remote_cache_hit_ratio` | missing/NaN metric |
| SM1 | 0.12b | remote_cache_should_hit_on_second_runner_build | Spike | S10 PASS rule via dispatched `bazel-cache-probe.yml` | second build remote hits > 0 | hits == 0 -> verdict Fail/disk-only |
| SM1 | 7.4c, 7.5 P10 (G7 closed) | cache_hit_ratio_should_meet_threshold_on_main_pushes | Perf | `bash tools/bzl/parity/perf.sh \| jq -e '.cache_hit_ratio_median_last20_main>=0.60'` (owner may waive in `owner-attestations.yaml`) | `true` | `false`; below the floor also raises a workflow annotation |
| SM2 | 7.5a P3 | fresh_clone_should_build_all_with_no_preconditions | Build | `bash tools/bzl/parity/fresh-clone.sh` (empty temp clone, `bazel build //...`) | exit 0, no `make proto-gen ent-gen` | exit != 0 or any "no such package gen/..." |
| SM2 | 0.1c | hermeticity_gap_should_be_quantified_before | Static | `grep -c 'gen/proto/go' implementation/baselines.md` | >= 1 | 0 |
| SM2 | 7.7a | two_clean_builds_should_be_bit_identical | Build | `bash tools/bzl/determinism.sh //tools/lint/...` (+ main targets) | exit 0 | hash mismatch |
| SM2 | 0.2a/0.2d | toolchain_should_be_pinned | Build | `cat .bazelversion`; `bazel run @rules_go//go -- version`; `bazel mod tidy && git diff --exit-code MODULE.bazel MODULE.bazel.lock` (2nd run) | `9.3.1`; `go1.26.6`; clean diff | version drift / lockfile diff |
| SM2 | 11.8a | upgrade_canary_should_flag_newer_versions | Static | `actionlint .github/workflows/bazel-upgrade-canary.yml`; dispatch run | green; reports drift | actionlint error |
| SM2 | 0.6a/3.1a/3.3a | no_host_tool_should_be_used_by_codegen_web_tmux_actions | Build | `bazel build //... --sandbox_default_allow_network=false` with `PATH=/usr/bin:/bin` (no buf/ent/bun/tmux installed) | exit 0 | "command not found" or network error. Only pre-registered D-HERMETIC downgrades (host cc tmux, tagged `no-remote-cache`) may appear, and must be listed |
| SM3 | 0.1b | baseline_worktree_cost_should_be_recorded | Static | `bash tools/bzl/baseline/worktree-cost.sh --runs 5 \| jq -e '(.runs\|length==5) and (.runs\|all(.loadavg1<=2))'` | `true`; no leftover worktree | `false`; `git worktree list` shows temp tree |
| SM3 | 0.10a / P10 | second_worktree_cold_build_should_be_at_most_30pct_of_make | Perf | `bash tools/bzl/baseline/bazel-worktree-cost.sh \| jq -e '.peak_rss_gb<=12 and .cold_ratio_median<=0.30'` | `true` | `false` -> FallbackWorkaround, recorded |
| SM3 | 0.10a, 7.4c, 7.5 P10 (G8 closed) | second_worktree_disk_should_be_within_bound_of_baseline | Perf | `bash tools/bzl/baseline/bazel-worktree-cost.sh \| jq -e '.output_base_disk_kb<=(1.25*.baseline_disk_kb_worktree)'` (owner may waive) | `true` | `false` |
| SM3 | 11.4a | orphan_output_bases_should_be_listed | Static | `bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh --dry-run` | exit 0, lists no live workspace | live workspace listed |
| SM4 | 0.1a-e | baselines_should_exist_for_ci_worktree_scale_testset | Static | `bash tools/bzl/baseline/ci-time-to-green.sh \| jq -e '.n==30 and (.p50_s\|type=="number") and (.per_workflow\|length>=3)'`; `jq -e '.passed\|length>1000' baseline-test-set.json`; `grep -E 'tracked_go_files\|test_packages\|go_mod_files' baselines.md` | `true`; `true`; 3 numeric lines | `false`/missing |
| SM5 | 7.5 | parity_gate_should_pass_all_ten_rows | Parity | `bash tools/bzl/parity/run-all.sh` | `PARITY: PASS`, exit 0 | `PARITY: FAIL P<n>`, exit 1 |
| SM5 | 2.6a/4.7b | whole_graph_should_build_and_test | Build | `bazel build //... && bazel test //... --test_tag_filters=-integration,-manual` | exit 0 (only `ExclusionsFile` entries skipped) | non-zero |
| SM5 | 4.9a | exclusions_ratchet_should_only_shrink | Static | `python3 -m unittest tools/bzl/check-exclusions_test.py`; `python3 tools/bzl/check-exclusions.py` | exit 0; <=10 packages, each with owner+issue | exit 1 naming offender |
| SM5 | 8.2a/b/c | make_wrappers_should_delegate_to_bazel | Build | `BUILD_SYSTEM=bazel make build && ./stapler-squad version`; `BUILD_SYSTEM=bazel make test PKG=./config`; `bash tools/bzl/check-wrappers.sh` | version line; exit 0; exit 0 | missing target / non-zero |
| SM5 | 8.6a | default_build_system_should_be_bazel_after_flip | Static | `make -n build \| grep -c 'bazel build'` | >= 1 | 0 |
| SM5 | 7.6a-c | ci_jobs_should_run_bazel_without_continue_on_error | Static | `actionlint`; `yq '.jobs[]\|select(.name\|test("bazel"))\|."continue-on-error"' .github/workflows/*.yml` | no `true` after flip; PR run green | `true` remains / red |
| SM5 | 1.1a | BUILD_files_should_cover_every_go_package | Static | `git ls-files '*BUILD.bazel' \| wc -l` vs `go list ./... \| wc -l` | equal within documented excluded packages | gap > excluded list |
| SM5 | 3.5a | aux_binaries_should_build | Build | `bazel build //cmd/...` | exit 0 | non-zero |
| C1 | 0.2g (G5 closed) | WORKSPACE_should_not_exist | Static | `bash tools/bzl/check-bazelrc_test.sh`; `test ! -e WORKSPACE && test ! -e WORKSPACE.bazel && ! grep -rn 'enable_workspace\|--enable_bzlmod=false' .bazelrc` and `bazel mod graph >/dev/null` | exit 0 | any WORKSPACE file or workspace flag |
| C1 | 0.2a | module_graph_should_be_stable_with_pinned_BCR_versions | Build | `bazel mod tidy` x2 then `git diff --exit-code MODULE.bazel MODULE.bazel.lock`; CI `--lockfile_mode=error` | clean | diff / lockfile error |
| C2 | 0.14b | fork_register_should_have_upstream_link_and_removal_condition | Static | awk check in Task 0.14b over ADR-008 fork register | every row non-empty | blank cell |
| C2 | 8-ADR | forks_should_be_pinned_not_floating | Static (GAP-minor) | `grep -nE 'git_override\|archive_override' MODULE.bazel` each has `commit =`/`sha256` / `integrity`; no `branch =` | all pinned | `branch =` present |
| C3 | 0.11a-c | gazelle_should_be_idempotent_and_resolve_count_bounded | Spike | S11 PASS rule: `bazel run //:gazelle -- -mode=diff; echo $?` -> `0` twice, with/without synced GenSurface; `grep -c 'gazelle:resolve' BUILD.bazel` <= 8 | all true | non-zero diff, >8 resolve lines -> extension path |
| C3 | 1.1a, 7.2a | GazelleFreshness_gate_should_block_stale_BUILD_files | Static | seeded-failure branch (edit a `.go` import, no BUILD update) run of `bazel.yml` | gate fails with message | gate passes |
| C3 | 0.2e | HandBuildFiles_should_be_tracked_not_ignored_not_in_gitlink | Static | `bash tools/bzl/check-build-paths.sh; echo $?`; `git check-ignore $(cat tools/bzl/hand-build-files.txt)` | `0`; no output | exit 1 naming path |
| C3 | 11.7a | starlark_surface_should_stay_bounded | Static | `bash tools/bzl/check-starlark-size.sh`; `buildifier -lint=warn -r . \| wc -l` | exit 0; `0` | macro >~30 lines or .bzl >500 |
| C4 | 0.12 / S10 | pr_cache_should_be_server_enforced_read_only | Security | S10 PASS rule: PUT via `:9092` -> 4xx; unauth PUT `:9093` -> 401; main credential PUT -> 2xx | as stated | PR credential can write |
| C4 | 7.4b | write_job_should_run_only_on_main_push | Static | `bash tools/bzl/check-workflows.sh`; PR-branch run of write job | exit 0; rejected by `bazel-cache-write` environment | job runs on PR |
| C4 | 0.12c, 7.4b | unreachable_cache_should_not_fail_or_slow_build | Build | `bazel build //tools/lint/... --remote_timeout=3s --remote_retries=1 --experimental_circuit_breaker_strategy=failure` (no `--remote_local_fallback`: a no-op for `--remote_cache` without the incompatible flag) against (a) DROP-ed `192.0.2.1` (and `iptables -j DROP` where root), (b) `tc netem delay 2s loss 20%`, (c) accept-then-silent listener; `echo $?` each; CI pre-flight fallback to `actions/cache` | `0` each, delta vs no-cache <= 10 s each | non-zero or >10 s (the former `127.0.0.1:1` test was an instant refusal and is not the criterion) |
| C4 | 7.4a | cache_flags_should_use_HTTP_protocol_only_and_no_inline_headers | Static | `grep -c 'remote_header' .bazelrc`; `grep -c 'grpcs\?://' .bazelrc` | `0`; `0` | >0 |
| C4 | 7.4b | fork_prs_without_secrets_should_still_build | Build | `workflow_dispatch` with empty secrets | green via disk-only fallback | red |
| C4 | 0.2b | shared_disk_and_repository_cache_should_be_configured | Static | `bazel info --config=agent workspace`; `grep -E 'disk_cache\|repository_cache' .bazelrc` | repo path, no flag errors; both flags present | flag error |
| C4 | 7.4c | disk_cache_gc_flags_should_exist_in_bazel_9_3_1 | Static | `bazel help build --long \| grep -c experimental_disk_cache_gc` | >= 3 | < 3 |
| C4 | 0.12e | D-PROTECT-WRITE_environment_should_gate_write_credential | Owner | `gh api repos/tstapler/stapler-squad/environments/bazel-cache-write --jq '[.protection_rules[].type]\|sort\|join(",")'` | contains `branch_policy` and `required_reviewers` | missing either |
| C5 | 5.1a/b | darwin_arm64_should_build_with_Info_plist | Build | macOS CI job; `otool -s __TEXT __info_plist bazel-bin/.../stapler-squad \| grep -c .` | >= 1 | 0 / build fail |
| C5 | 5.1c | infoplist_test_should_pass_on_macos_and_skip_on_linux | Build | `bazel test //macos:infoplist_test` | PASSED on macOS; incompatible/skipped on Linux | FAILED |
| C5 | 5.1d | darwin_amd64_should_cross_build | Build | `file bazel-bin/.../stapler-squad` | contains `x86_64` | other arch |
| C5 | 5.2a | codesign_target_should_accept_explicit_binary_path | Static | `make -n _codesign-binary BIN=/tmp/x \| grep -c /tmp/x` | >= 1 | 0 |
| C5 | 5.2b / P8 | tcc_grants_should_persist_for_bazel_built_signed_binary | Owner | `scripts/verify-bazel-codesign.sh` on arm64 + amd64 device; `shellcheck scripts/verify-bazel-codesign.sh` | grants persist; result recorded in PR | grant re-prompted |
| C5 | 0.2c, 3.4 | linux_amd64_should_be_primary_pure_go | Build | `bazel build --config=linux //tools/lint/...` and `bazel aquery --config=linux 'mnemonic("CppCompile", //tools/lint/...)'` | exit 0; no actions | any CppCompile |
| C6 | 6.1a | windows_amd64_should_cross_compile | Build | `bazel build //tools/bzl/windows:stapler-squad_windows_amd64 && file bazel-bin/tools/bzl/windows/stapler-squad_windows_amd64.exe` | contains `PE32+` | other format |
| C6 | 6.1b | windows_only_tests_should_be_incompatible_not_failed | Build | `bazel build --keep_going //... 2>&1 \| grep -c '^ERROR'` and `bazel cquery ... IncompatiblePlatformProvider` count | `0`; >= 1 | ERROR > 0 |
| C6 | 6.1c | goreleaser_and_CI_should_align_on_windows | Static | `goreleaser check`; `actionlint` | exit 0 | non-zero |
| C7 | 2.4, GenFreshness | generated_trees_should_stay_ignored | Static | `git ls-files gen web-app/src/gen 'session/ent/*.go' \| grep -v 'session/ent/schema\|session/ent/generate.go' \| wc -l`; `git ls-files --others --ignored --exclude-standard -- '*BUILD' '*BUILD.bazel' \| wc -l` | `0`; `0` | any tracked generated file or BUILD under ignored tree |
| C7 | 2.3c, 0.4a | ent_flags_should_include_sql_upsert | Static | `grep -c 'sql/upsert' tools/bzl/ent/ent_flags.txt`; `grep -n 'ent_flags.txt' Makefile` and ent macro both read it; `grep -c UpsertRule bazel-bin/session/ent/**/*.go` | >=1; both consumers; >=1 | flag missing / no `UpsertRule` |
| C7 | 3.1a | web_build_should_use_bun_only | Static (GAP-minor) | `! grep -rnE '\b(npm\|pnpm\|yarn)\b (install\|ci\|run)' tools/bazel tools/bzl web-app/BUILD.bazel .github/workflows/bazel*.yml Makefile make/` | no hits | any hit |
| C7 | 0.2h (G4 closed) | commits_should_stay_conventional | Static | `bash tools/bzl/check-pr-title.sh 'feat(bazel): x'` exits 0 and `'Update stuff'` exits 1; runs on `pull_request` in `bazel.yml` | match | no match |
| C7 | 7.5e / RC3 | manual_instance_should_not_touch_8543 | Static | `bash scripts/bazel-manual-instance.sh --dry-run`; `shellcheck scripts/bazel-manual-instance.sh`; during gate: `ss -ltnp \| grep -c ':8543'` unchanged and live PID unchanged | exit 0; ports 62871/62872 only | change to :8543 listener PID |
| C8 | 0.2h (G4 closed) | no_force_push_in_workflows_scripts_or_local_pushes | Static | `bash tools/bzl/check-no-force-push.sh`; `bash scripts/hooks/pre-push-no-force_test.sh` (non-fast-forward rejected) | exit 0 both | any hit; rewritten history accepted |
| N1 | 7.5d P10 | warm_noop_build_should_be_at_most_10s | Perf | `bash tools/bzl/parity/perf.sh \| jq -e '.warm_noop_s<=10'` | `true` | `false` |
| N1 | 0.1a, 7.5 P10 | cold_ci_target_should_derive_from_baseline | Perf | `perf.sh` cold CI time vs `baselines.md` p50 x1.10 | within | exceeds |
| N2 | 0.10a | concurrent_builds_should_stay_within_memory_cap | Perf | S6: 3 concurrent builds peak RSS <= 12 GB (`peak_rss_gb<=12`) | `true` | `false` |
| N2 | 0.10c (G6 closed) | concurrent_worktree_builds_should_not_corrupt_shared_disk_cache | Integration | `bash tools/bzl/baseline/disk-cache-concurrency.sh \| jq -e '.hash_lists_equal and (.corruption_lines==0)'` (3 worktrees concurrently, twice, then a 4th vs a no-cache build) | `true` | `false` |
| N2 | 0.10b, 11.2a | MODULE_lock_conflicts_should_resolve_via_merge_driver | Static | two-branch simulation: `bazel mod deps --lockfile_mode=error` after merge | exit 0 | exit != 0 |
| N3 | 0.12d S10-redaction | credentials_should_not_appear_in_any_artifact | Security | `grep -c -e CANARY_SECRET_VALUE -e "$CANARY_B64" bep.json profile.txt execlog.txt command.log stderr.txt stderr-wrongcred.txt \| awk -F: '{s+=$NF} END{print s}'` (success and wrong-credential runs, HTTP verbose on) | `0` | >0 |
| N3 | 7.4 | CI_secret_scan_step_should_find_nothing | Security | workflow "secret scan" step greps `bep.json`, gunzipped profile, exec log, trend files for credential, base64, `Authorization` | step green | step red; artifact not uploaded |
| N3 | 7.4a | credential_helper_should_read_env_not_file | Static | `bash tools/bzl/credential-helper.sh <<< '{"uri":"https://onyx"}' \| jq -e '.headers.Authorization'` with env set; `git grep -nE 'Authorization: (Basic\|Bearer) [A-Za-z0-9]{8,}' -- . ':!docs' \| wc -l` | header present; `0` | secret literal in repo |
| N3 | 7.4 | main_job_should_upload_only_scrubbed_trend_json | Static | `bash tools/bzl/check-workflows.sh` | exit 0 | raw BEP uploaded from write job |
| I1 | 0.2a-e | foundation_files_should_exist_and_resolve | Build | `bazel version` -> `Build label: 9.3.1`; `bazel query //...` has no `//.claude/worktrees` label; `bash tools/bzl/check-bazelignore.sh` | pass | stray label / node_modules unlisted |
| I1 | 0.2c | small_modules_should_build_and_test | Build | `bazel build //tools/lint/... //tools/scanner/... //tuitest/...`; `bazel test //tools/lint/...` | exit 0 (<= listed exclusions) | non-zero |
| I1 | 0.1/0.2 spike S-gomod | go_git_replace_should_resolve_through_go_deps | Spike | S-gomod: `bazel mod tidy` no diff + `bazel build //...` using `tstapler/go-git` | green | resolves upstream go-git |
| I1 | 1.4a/b | TagConfig_names_should_be_sealed | Static | `bash tools/bzl/check-configs.sh; echo $?` | `0` | unknown config |
| I1 | 1.5a, 8.1a | BUILD_SYSTEM_should_reject_unknown_values | Static | `BUILD_SYSTEM=nix make -n help; echo $?` | message + exit `2` | exit 0 |
| I2 | 0.8a/b, 4.8b | all_tag_configs_should_build_and_match_test_set | Parity | `for c in default race integration sinkguard gitoracle; do bash tools/bzl/parity/test-set-diff.sh $c \|\| echo FAIL $c; done`; `bazel aquery --config=race 'mnemonic("GoCompilePkg", //tools/lint/...)' \| grep -c -- '-race'` | no FAIL; >= 1 | `FAIL <cfg>`; 0 `-race` |
| I2 | 0.8b | embed_tymux_should_imply_embed_tmux | Build | `bazel build --//tools/bzl/flags:embed_tmux=true //session/tmux` and `=false` | both exit 0 | non-zero |
| I2 | 3.3c | embed_configs_should_build_with_and_without | Build | `bazel build --config=embed_tymux //session/tmux //session/tymux`; same without config | exit 0 both | non-zero |
| I2 | 3.4a-c / P5 | binary_should_match_make_binary_behaviour | Parity | `bash tools/bzl/workspace_status.sh \| grep -c '^STABLE_'` -> `4`; `bazel build --config=release //:stapler-squad`; `bash tools/bzl/parity/compare-binary.sh` | `4`; 0; 0 (version fields equal, embed sha256 == pins) | field/hash mismatch |
| I2 | 0.5a, 3.2a | go_embed_of_dist_should_work_without_Go_source_change | Build | `bazel test //server/web:all`; `git diff --stat origin/main -- server/web/embed.go` | exit 0; empty | embed.go changed |
| I2 | 1.1c / changelog | CHANGELOG_embed_should_compile | Build | `bazel build //:stapler-squad` (root `embedsrcs = ["CHANGELOG.md"]`) and `./bazel-bin/.../stapler-squad version` | exit 0 | embed error |
| I2 | 4.8a, P1 | go_test_pass_set_should_equal_bazel_pass_set | Parity | `bash tools/bzl/parity/test-set-diff.sh default` | `missing_in_bazel: 0` after `ExclusionsFile` | non-zero missing |
| I2 | 4.2a/b | go_test_defaults_and_sharding_should_not_drop_tests | Build | `bazel test //tools/bzl:go_test_bzl_test`; `diff <(sort sharded.names) <(sort unsharded.names)` | pass; empty | diff non-empty |
| I2 | 4.9b | flake_free_under_repetition | Build | `bazel test //... --runs_per_test=3 --test_tag_filters=-integration,-manual; echo $?` | `0` | flaky target |
| I2 | 4.10a-e, 4.1a | sandbox_failures_should_be_classified_and_fixed_at_source | Build | `python3 tools/bzl/spike/census-check.py --class cwd,runtime-caller` (and `home`, `tmpdir-socket,exec-host-binary`, `network`) | `0` remaining beyond ExclusionsFile | >0 |
| I2 | 4.1a | reporoot_and_testenv_should_stay_stdlib_leaves | Static | `go test ./internal/reporoot ./internal/testenv -count=1`; `go list -deps ./internal/reporoot ./internal/testenv \| grep stapler-squad \| { ! grep -qv -e internal/reporoot -e internal/testenv; }` | `ok`; exit 0 | import cycle risk |
| I2 | 0.9a/b, 4.5b | unix_socket_path_should_fit_limit | Spike/Build | `bazel test --test_env=TMPDIR=/tmp //tools/bzl/spike:sock_test`; `bazel test //session/tmux:socket_path_len_test` | pass | `bind: invalid argument`/path >108 |
| I3 | 1.1b | mattn_sqlite_should_be_removed | Static | `go test ./session -run 'Backlog\|WorkflowEnabled' -count=1`; `grep -c mattn go.mod` | `ok`; `0` | >0 |
| I3 | 1.1c | linux_build_should_have_no_cgo_actions | Build | see C5 pure-Go check | no CppCompile | any |
| I3 | 5.1a/b | darwin_cgo_files_should_build_with_host_cc | Build | macOS job `bazel build --config=macos //:stapler-squad` | exit 0 | link error |
| I3 | 3.4d (G2 closed) | netdns_godebug_should_be_preserved_in_bazel_binary | Parity | `go version -m bazel-bin/.../stapler-squad \| grep -i 'DefaultGODEBUG'` equals same on `go build` binary; contains `netdns=go` (check lives in `compare-binary.sh`) | identical | missing/different |
| I4 | 0.3, 2.1a-c, P4 | proto_go_and_connect_output_should_equal_buf_output | Parity | `bash tools/bzl/parity/compare-proto.sh; echo $?` (gofmt-normalised `diff -r`, import path `.../gen/proto/go/session/v1[/sessionv1connect]`, connect-go v1.20.0) | `0`, empty diff | non-empty diff -> FallbackWorkaround |
| I4 | 0.3e | proto_actions_should_run_with_no_network | Build | `make proto-gen && buf lint` with network disabled; `bazel build //proto/... --sandbox_default_allow_network=false` | exit 0; `git diff --stat -- gen web-app/src/gen` empty | BSR fetch attempt |
| I4 | 0.6e, 2.2a | proto_ts_output_should_equal_make_output | Parity | `diff -r bazel-bin/proto/session/v1/ts web-app/src/gen` | exit 0 | diff |
| I4 | 0.4a-c, 2.3a-c | ent_generation_should_produce_all_packages_deterministically | Spike/Parity | `bazel query 'kind(go_library, //tools/bzl/ent:all)' \| wc -l` == `go list ./session/ent/... \| grep -vc /schema`; `bazel build //session/... //server/...`; `bash tools/bzl/parity/compare-ent.sh`; two-build hash diff empty | equal counts; exit 0; empty | count mismatch / importer unresolved |
| I4 | 2.3a | ent_packages_list_should_be_fresh | Build | `bazel test //tools/bzl:ent_bzl_test //tools/bzl/ent:packages_fresh_test` | pass | stale `packages.txt` |
| I4 | 2.4a/b | sync_gen_should_populate_GenSurface_and_detect_staleness | Build | `bazel run //:sync-gen && go build ./... ; echo $?`; corrupt a synced file then `make gen-freshness` | `0`; freshness fails then recovers after sync | silent pass on corrupt |
| I4 | 2.5a | buf_lint_should_pass_as_bazel_test | Build | `bazel test //proto/session/v1:buf_lint_test` | exit 0 | non-zero |
| I4 | 2.4c (G3 closed) | feature_registry_and_other_generated_inputs_should_be_decided | Static | `grep -c 'registry-generate' decisions/ADR-002*.md decisions/ADR-005*.md`; `make registry-generate && git diff --exit-code docs/registry` | decision recorded (out of Bazel, CI diff check); exit 0 | no decision; registry drift |
| I5 | 0.7a-c, 3.3a / S5 | tmux_should_build_hermetically_from_tarball | Spike | S5 PASS rule: `bazel build //tools/bzl/tmux:tmux_bin` with empty `third_party/tmux`; `bazel-bin/tools/bzl/tmux/tmux -V` -> `tmux 3.8`; `git ls-files third_party/tmux` only gitlink; `git check-ignore tools/bzl/tmux/BUILD.bazel` empty; action sandboxed | all true | non-3.8, host cc used without D-HERMETIC |
| I5 | 3.3b | tymuxd_should_be_fetched_by_pinned_http_file | Build | `bazel build @tymuxd_linux_amd64//file` | exit 0 | sha256 mismatch |
| I5 | 3.3d | tymux_pin_check_should_cover_MODULE_bazel | Static | `bash tools/ci/check-tymux-pin_test.sh` (includes mutated-pin case that must fail) | exit 0 | mutated pin passes |
| I5 | 0.7c | pin_table_has_tag_sha256_commit | Static | `awk` check on `PIN` fields | non-empty | blank |
| I6 | 0.6a-d / S4, 3.1a-d | web_dist_should_equal_make_web_build | Parity | `bash tools/bzl/parity/compare-web.sh; echo $?` (file list + sha256, deltas only from `allowed-deltas.txt`) | `0` | un-allowed delta |
| I6 | 0.6b | web_action_should_not_escape_sandbox | Spike | `bash tools/bzl/spike/count-escapes.sh` | `0 0` | nonzero reads/writes |
| I6 | 0.6c, 3.1c / P6 | jest_should_run_as_bazel_test | Build | `bazel test //web-app:jest_subset` (20 tests), then `bazel test //web-app:jest` | exit 0; Jest pass-set diff empty | missing tests |
| I6 | 3.1a | node_modules_repo_rule_should_cache | Build | `bazel build @web_node_modules//:all` twice | 2nd: 0 processes executed | re-install |
| I7 | 0.2c, 0.13a | tools_lint_isolated_xtools_version | Build | `bazel mod graph \| grep -c 'golang.org/x/tools'` shows two resolutions | >= 2 distinct | unified version |
| I7 | 0.13a / S12, 8.4a/b | custom_analyzers_should_run_as_nogo_or_stay_wrapped | Spike | S12: `bazel build //tools/bzl/spike/nogo_seed:all 2>&1 \| grep -c norawgitcli`; if Pass, `bash tools/bzl/nogo/check-parity.sh` | >= 1; exit 0 | seeded violation unflagged -> wrapped, 8.4 skipped |
| I7 | 8.3a | ready_gates_should_include_new_bazel_checks | Static | `make -n ready \| grep -cE 'gazelle\|buildifier\|gen-freshness'` and golangci-lint/dupl/jscpd still present | >= 3; present | missing gate |
| I7 | 7.6c | lint_jobs_should_run_on_synced_tree | Build | PR run of flipped lint job green; `make ready` under `BUILD_SYSTEM=bazel` | green | red |
| I8 | 8.1b | make_help_should_list_every_target | Static | `bash tools/bzl/check-make-help.sh; echo $?` | `0` | undocumented target |
| I8 | 9.1a/b | install_service_should_assert_bazel_binary_before_swap | Static | `make -n install-service` shows the three required lines; `bash scripts/install-service-preflight.test.sh`; `shellcheck scripts/install-service.sh` | `0` | missing assertion |
| I8 | 9.1d | live_install_should_be_owner_run_after_P9 | Owner | owner runs `make install-service`; `systemctl --user status stapler-squad` + `curl -s localhost:8543/...` health | healthy; tmux sessions survive | service down |
| I8 | 3.7a/b | goreleaser_should_consume_bazel_or_record_exception | Static | `goreleaser check`; `grep -n goreleaser decisions/ADR-005-permanent-exceptions-outside-bazel.md` | exit 0; decided line | "TBD" |
| I8 | 3.7b | release_please_should_be_unaffected | Static (GAP-minor) | after flip, next Release PR opens and `release-please` config diff vs main empty: `git diff --stat origin/main -- release-please-config.json .release-please-manifest.json` | empty | changed |
| I8 | 3.7c (G1 closed) | homebrew_formula_should_still_resolve_release_artifacts | Static | `ruby -c Formula/stapler-squad.rb`; `goreleaser release --snapshot --skip=publish --clean` then `bash tools/bzl/check-formula.sh dist/` | syntax ok; names and sha256 match | mismatch |
| I8 | 1.3a, 7.2a | actionlint_should_pass_on_all_new_workflows | Static | `actionlint .github/workflows/*.yml` | exit 0 | lint error |
| I9 | 7.5b / P7 | e2e_pass_count_should_equal_between_binaries | Parity | `bash tools/bzl/parity/e2e.sh` (uses `TEST_SERVER_BINARY`) | prints `e2e_pass_bazel == e2e_pass_make` | counts differ |
| I9 | 7.6b | e2e_shards_should_run_on_bazel_binary_in_CI | Build | PR run of flipped e2e jobs; `grep -n TEST_SERVER_BINARY tests/e2e/global-setup.ts tests/e2e/helpers/*.ts` | green; present | red / not wired |
| I10 | 0.14c | ci_speed_ADR_001_should_be_marked_superseded | Static | `grep -c 'Superseded by' project_plans/ci-speed/decisions/ADR-001-reject-bazel-bzlmod-adoption.md` | `1` | 0 |
| I10 | 0.14a | ADRs_should_be_accepted_and_spike_verdicts_valid | Static | `python3 tools/bzl/check-spike-results.py project_plans/bazel-conversion/implementation/spike-results.yaml; echo $?`; `python3 -m unittest tools/bzl/check-spike-results_test.py`; `grep -hE '^\*\*Status\*\*: Accepted' project_plans/bazel-conversion/decisions/ADR-*.md \| wc -l` | `0`; ok; `8` | invalid verdict / <8 |
| I10 | 11.1a-c | agent_skill_rule_claude_md_should_exist | Static | `grep -c '^description:' .claude/skills/bazel-stapler-squad/SKILL.md`; `head -5 .claude/rules/bazel-build-files.md \| grep -c globs`; CLAUDE.md Bazel section within limit | `1`; `1`; within | 0 |
| I10 | 11.3a-c | docs_should_exist_and_commands_be_run | Static | `grep -c '^### ' docs/how-to/troubleshoot-bazel.md` (>=11); `grep -c 'ADR-001' docs/explanation/why-bazel.md` (>=2); `grep -c 'make sync' docs/how-to/build-with-bazel.md` (>=1) | thresholds met | below |
| I10 | 11.6a | CODEOWNERS_should_parse | Static | `gh api repos/tstapler/stapler-squad/codeowners/errors --jq '.errors\|length'` | `0` | >0 |
| I10 | 11.2b | agent_config_output_should_be_terse | Static | `bazel test --config=agent //config/... 2>&1 \| wc -l` | <= 40 | > 40 |
| O1 | n/a | no_remote_execution_should_be_configured | Static | `! grep -nE 'remote_executor\|--remote_exec' .bazelrc .github/workflows/*.yml` | no hits | any hit |
| O2 | 4.1-4.10 | non_test_go_source_changes_should_be_hermeticity_only | Static | `git diff --stat origin/main -- '*.go' ':!*_test.go' ':!internal/reporoot' ':!internal/testenv'` reviewed in each epic PR; `git diff origin/main -- server/web/embed.go session/tmux/embed*.go` empty (ADR-007) | only hermeticity edits | behaviour changes |
| O3 | n/a | web_framework_should_be_unchanged | Static | `git diff origin/main -- web-app/package.json \| grep -E '^[+-].*"(next\|react\|bun)"'` empty beyond lock-related | empty | framework dep change |
| O4 | 2.4 | generated_code_should_never_be_committed | Static | same check as C7 generated_trees_should_stay_ignored | `0` | tracked generated file |
| RC1 | 1.3a, 7.6 | bazel_job_should_be_non_blocking_until_soak_rule_met | Static | `yq '.jobs[].continue-on-error' .github/workflows/bazel.yml` pre-flip = `true`; soak ledger rule: last N runs zero unattributed failures, zero `bazel-real` in last K | `true` pre-flip; ledger satisfies N=30, K=10 (owner may change) | flip before ledger satisfied |
| RC1 | 7.5c | owner_attestations_should_only_change_by_environment_approved_owner | Security | seeded attestation edit by non-owner makes attest step exit 1 | exit 1 | step passes |
| RC1 | 7.6a-c | each_job_flip_should_keep_legacy_job_running | Static | PR run shows both `bazel-*` and `legacy-*` jobs green | both green | legacy job removed early |
| RC2 | 1.5a | make_path_should_not_depend_on_bazel | Static | `PATH=$(dirname $(command -v make)):/usr/bin make -n ready \| grep -c bazel` with `BUILD_SYSTEM=make` | `0` | >0 |
| RC2 | 8.5a, 10.1a | FallbackCanary_should_pass_make_ready_build_and_e2e_shard_without_bazel | Migration | dispatch `fallback-canary.yml` on a runner with no Bazel; `grep -c 'continue-on-error' .github/workflows/fallback-canary.yml` | green; `0` | red or `continue-on-error` present |
| RC2 | 8.6a, 10.1a | build_system_flip_should_be_single_PR_reversible | Migration | `git revert --no-edit <flip PR sha>` on scratch branch, then `make -n build \| grep -c 'go build'` and `BUILD_SYSTEM=make make build` | revert is conflict-free; `go build` path restored; build exits 0 | conflict or broken make path. Name: `migration_should_be_reversible` |
| RC2 | 10.1a | bazel_only_source_changes_are_banned_during_retention | Static | seeded Go file compiling only with Bazel-style layout fails the `legacy-*` job in draft PR | `legacy-*` red | passes |
| RC2 | 10.2a/b | removal_only_after_retention | Static | `make -n build test ready \| grep -c 'go build\|go test'` -> `0` post-removal; `grep -rn 'BUILD_SYSTEM=make' CLAUDE.md docs .claude \| wc -l` -> `0` | `0`; `0` | residual references |
| RC3 | 7.5e P9, 9.1 | isolated_instance_from_bazel_binary_should_pass_smoke_before_live_install | Owner/Parity | `bash scripts/bazel-manual-instance.sh` (ports 62871/62872, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`); startup log contains `Using private tmux socket` | smoke list passes | any failure blocks Story 9.1d |
| OB1 | 1.3b | BEP_and_profile_should_upload_from_read_only_jobs | Static | `gh run view <id> --json artifacts --jq '.artifacts[].name'` | lists both | missing |
| OB1 | 7.1a | trend_script_should_emit_all_metrics | Static | `bash tools/ci/bazel-trend_test.sh` | emits `ci_time_to_green_seconds`, cache hit ratio, cold build seconds, disk kb, `gazelle_freshness_failures_total`, `affected_target_count`, `affected_selection_false_negatives_total` | missing metric |
| OB2 | 7.2a/b | gazelle_freshness_should_run_in_CI_and_precommit | Static | `make gazelle`; `bash scripts/hooks/pre-commit-gazelle.sh` on clean tree; seeded stale BUILD fails CI step | exit 0; exit 0; fails | stale passes |
| OB3 | 7.5b (G9 closed) | service_metrics_should_be_unchanged | Parity | on the P9 isolated instance: `curl -s localhost:62871/metrics \| grep -oE '^[a-z_]+' \| sort -u` vs same from make-built binary (step in `parity/e2e.sh`) | identical metric-name set | missing/extra names |
| OQ1 | 8.3, 8.4 | lint_gate_mapping_should_be_recorded | Static | table in `docs/reference` or ADR-005 lists each gate (golangci-lint, dupl, jscpd, custom analyzers, buf) as Bazel/nogo/wrapped | every gate has an entry | unlisted gate |
| OQ2 | 0.6d, ADR-003 | bun_lock_consumption_should_have_costed_decision | Spike | S4 table in `spike-results.md` has Workaround and Fork rows with class and chosen path; ADR-003 status Accepted | rows present | missing |

## Spike Verdict Gates (Epic 0)

| Spike | Gate name | Type | PASS rule (verbatim reference) | Fail consequence |
|---|---|---|---|---|
| S-gomod | spike_gomod_should_pass | Spike | plan.md Fallback matrix row 0.2 | merge tools/lint into main go_deps / nearest SDK + go.mod toolchain (owner approval) |
| S1 | spike_proto_go_should_pass | Spike | row 0.3 | `buf generate` run_binary |
| S1-ts | spike_proto_ts_should_pass | Spike | `diff -r bazel-bin/web-app/gen web-app/src/gen` exit 0 | pinned-tarball `protoc-gen-es` (D-HERMETIC) |
| S2 | spike_ent_should_pass | Spike | row 0.4 (53 packages, UpsertRule, hash-equal, importers resolve) | custom rule / entc no-remote-cache |
| S3 | spike_embed_should_pass | Spike | row 0.5 | tree-expansion rule |
| S4 | spike_web_should_pass | Spike | row 0.6 (sha256 equal, Jest 20 equal, 0 escapes; >1.5x time stays `Pass` with `repo-rule+no-sandbox`) | `rules_bun` (Task 0.6f, `FallbackWorkaround`), then aspect_rules_js fork (`FallbackFork`), then `Fail` + D-HERMETIC |
| S5 | spike_tmux_should_pass | Spike | row 0.7 | genrule host cc (D-HERMETIC) |
| S6 | spike_worktree_should_pass | Spike | row 0.10 (incl. wrapper live-latency check, Task 0.10c integrity and the cross-worktree sharing checks E1-E6 of Tasks 0.10e-f; Tasks 0.10g-h are non-gating inputs) | per-worktree output base |
| S13 | spike_lockfile_should_pass | Spike | row 0.10d (20-PR replay) | do not commit the lock (owner approval) |
| S7 | spike_tags_should_pass | Spike | row 0.8 | transition macro |
| S8/S9 | spike_sandbox_census_should_pass | Spike | row 0.9 (`census-check.py` exit 0) | n/a |
| S10 / S10-redaction | spike_cache_should_pass | Spike | row 0.12 | disk-cache-only (owner decides) |
| S11 | spike_gazelle_should_pass | Spike | row 0.11 | custom Gazelle extension |
| S12 | spike_nogo_should_pass | Spike | row 0.13 | keep `make lint-custom` wrapped |
| S14 | spike_selective_e2e_should_pass | Spike | row 0.15 (193/193 manifest entries, replay median share <= 0.50, p90 <= 0.90, `__ALL__` fallback <= 15%, 0 mutation misses, S-a to S-e) | diff-line coverage selection with an escape budget, or e2e stays a full run (owner decides) |
| Gate | spike_results_should_validate | Static | `python3 tools/bzl/check-spike-results.py ...spike-results.yaml` exit 0, all 17 ids present, a `Fail` blocks dependent epics | unknown verdict / missing id |

## Requirement Coverage Review

Every one of the 40 requirements has at least one verification above (39 testable plus N4, which has no testable behaviour). After Phase 4 all nine gaps are closed by plan tasks; the verification rows above now cite those tasks.

| Gap | Requirement | Closed at |
|---|---|---|
| G1 | I8 Formula/ | Task 3.7c (snapshot dry run plus `check-formula.sh`; ADR-005 row) |
| G2 | I3 `godebug netdns=go` | Task 3.4d (`DefaultGODEBUG` equality in `compare-binary.sh`) |
| G3 | I4 feature-registry / other generated inputs | Task 2.4c (ADR-002 item 6, ADR-005 row; CI `make registry-generate` diff) |
| G4 | C7/C8 Conventional Commits, no force-push | Task 0.2h (`check-no-force-push.sh`, `pre-push` hook, `check-pr-title.sh`) |
| G5 | C1 no WORKSPACE | Task 0.2g (`check-bazelrc.sh`) |
| G6 | N2 cache corruption under concurrency | Task 0.10c (`disk-cache-concurrency.sh`) |
| G7 | SM1 cache hit rate threshold | Task 7.4c and P10 row (median at least 0.60 over the last 20 warm main push runs; owner may waive) |
| G8 | SM3 disk per worktree | Task 0.10a, Task 7.4c and P10 row (at most 1.25 x baseline `disk_kb_worktree`; owner may waive) |
| G9 | OB3 service metrics | Task 7.5b (metric-name-set diff in `parity/e2e.sh`) |

## Phase 4 additions (verifications for pre-mortem controls)

| Control | Plan task | Verification name | Type | Command | Expected |
|---|---|---|---|---|---|
| Host containment | 0.2f, 0.10 | bazel_wrapper_should_cap_concurrency_and_memory | Static/Perf | `bash tools/bzl/wrapper_test.sh`; S6 live-latency run | at most 2 concurrent, `MemoryMax` set; RSS <= 12 GB, stream p99 <= 1.2x idle |
| Live unit OOM priority (early and final) | 0.2i, 9.1b | live_unit_should_set_OOMScoreAdjust | Static | `bash scripts/install-service-preflight.test.sh` | asserts `OOMScoreAdjust=-900` |
| Generated-file repair | 7.8a-c, 0.10d | main_and_PR_drift_should_be_auto_repaired | Build | seeded drift run of `bazel-main-repair.yml` and `bazel-autofix.yml`; S13 replay | repair PR within 10 min; autofix push is non-forced; zero lock conflicts |
| Test-cache false-green control | 4.2a, 4.9c, 8.5b, 0.2g | env_coupled_tests_should_never_be_cached | Build | `bazel test //tools/bzl:go_test_bzl_test` (tag policy); nightly `--nocache_test_results --runs_per_test=3`; canary pass-set equality | `external` tag on the four classes; no `flaky = True`; ledger rows appended |
| Cold-floor and pre-flight | 7.4b, 0.3f, P10 | cold_ci_should_be_within_1_5x_and_cache_outage_should_fall_back | Perf | `perf.sh` cold row; unreachable-cache run of `bazel.yml` | cold <= 1.5 x p50; job continues on `actions/cache` |
| Flag names verified | 0.2b, 0.14 | flag_names_should_be_spike_verified | Static | `grep -c 'spike-verified' spike-results.md` | at least 24 (20 flags plus 4 recorded-absent) |
| Retention evidence | 10.1, 10.2a | retention_should_require_release_canary_and_owner_installs | Owner | attestation file check by the environment-approved job | Release PR, 2 canary runs, Linux and macOS installs attested |
| S4 fallback reachable | 0.6f | S4_failure_should_reach_rules_bun_then_fork | Spike | `python3 tools/bzl/check-spike-results.py` on a seeded `FallbackWorkaround` S4 row | exit 0 and `path` names `rules_bun` |

## Triad additions (iteration 1)

| Control | Plan task | Verification name | Type | Command | Expected |
|---|---|---|---|---|---|
| Pain numbers and targets recorded | 0.1f, 0.1g | pain_and_targets_should_be_recorded_and_owner_confirmed | Static | `bash tools/bzl/baseline/pain-numbers.sh \| jq -e '.regen_steps_per_session_median\|type=="number"'`; `yq -e '.targets_confirmed' owner-attestations.yaml` | `true`; a date |
| Improvement targets enforced | 7.5g, P10 | improvement_targets_T1_to_T8_should_hold | Perf | `bash tools/bzl/parity/improvement.sh \| grep -cE '^T[1-8]: PASS$'` | `8` (T6 may be UNMEASURED until the flip) |
| Waivers visible and bounded | 7.5c, 7.5f | waivers_should_be_reasoned_expiring_and_reported | Static | `python3 -m unittest tools/bzl/parity/check-waivers_test.py`; `run-all.sh --dry-run --waivers <ok>` | exit 0; `PARITY: PASS WITH WAIVERS` |
| Economic gate | 0.14e | economic_gate_should_trip_on_each_criterion | Static | `python3 -m unittest tools/bzl/economic-gate_test.py` | one trip fixture per EG1-EG5 exits 3; all-clear exits 0 |
| Error translation contract | 1.6a, 1.6d | every_known_failure_should_print_a_remedy | Static | `bash tools/bzl/translate-errors_test.sh`; `bash tools/bzl/check-error-legibility.sh` | `PASS` x11 plus negative fixtures; `LEGIBLE: 9/9` |
| Raw bazel quiet and capped | 1.6b | wrapper_should_apply_agent_config_cap_output_and_announce_queue | Static | `bash tools/bzl/wrapper_test.sh` | success output <= 40 lines, failure <= 60 lines ending `full log:`, E7 message within 2 s, timeout exit 75 |
| Doctor and precondition banner | 1.6c | doctor_should_flag_each_seeded_fault | Static | `bash tools/bzl/doctor_test.sh`; `make help \| head -3 \| grep -c 'make sync'` | all seeded faults non-zero with hint; `1` |
| Troubleshooting stub | 1.6e | stub_should_cover_states_1_2_3_4_10 | Static | `grep -E '^### State (1\|2\|3\|4\|10):' docs/how-to/troubleshoot-bazel.md \| wc -l` | `5` |
| Cheap gen freshness | 2.4d, 8.2d | stale_gen_should_be_detected_without_bazel | Static | `bash tools/bzl/check-gen-fresh-cheap_test.sh` (PATH without bazel) | `gen: fresh` / `gen: STALE` with exit codes; under 0.5 s |
| Tag-reason guard | 4.2c | untagged_reason_should_fail_ci | Static | `bash tools/bzl/check-test-tags_test.sh` | missing comment or issue exits 1 |
| Batch size cap | 4.10f | no_sandbox_batch_should_exceed_five_files | Static | `jq -e '[.batches[]\|(.files\|length)]\|max<=5' tools/bzl/spike/batches.json` | `true` |
| Sweep completion (absolute) | 4.7a, 4.7b | residual_sweeps_should_end_with_empty_parity_diff | Parity | `bazel test //... --test_tag_filters=-integration,-manual`; `test-set-diff.sh default` | exit 0; `missing_in_bazel: 0`; exclusions <= 10 |
| PKG/TAGS mapping | 8.2e | pkg_and_tags_should_map_to_expected_bazel_commands | Static | `bash tools/bzl/check-pkg-mapping.sh` | exit 0 over 15 rows |
| Skill proposal guard | 11.1d | unverified_commands_should_be_marked_proposal | Static | `bash tools/bzl/check-skill-proposals_test.sh` | marked passes, unmarked fails |
| Fresh-worktree journey | 11.3d | journey_doc_should_quote_recorded_numbers | Static | `grep -cE 'baselines.md\|bazel-worktree-cost' docs/how-to/first-build-in-a-fresh-worktree.md` | at least `1` |
| Root BUILD never collapsed | 11.6a, 11.6b | gazelle_directive_change_should_be_visible | Static | `git check-attr linguist-generated BUILD.bazel`; `bash tools/bzl/check-gazelle-directives_test.sh` | `unset`; directive change exits 1 |
| Safe removal | 10.2a | removal_should_delete_exactly_the_expected_recipes | Static | `python3 tools/bzl/check-removal.py --before "$RUNNER_TEMP/before.targets"` | exit 0; seeded cases exit 1 |

## Triad additions (iteration 2)

| Control | Plan task | Verification name | Type | Command | Expected |
|---|---|---|---|---|---|
| Wrapper file vs helper directory | all | wrapper_path_and_helper_dir_should_not_collide | Static | `! grep -rnE 'tools/baz[e]l/' project_plans/bazel-conversion .claude docs Makefile make; test -z "$(git check-ignore tools/bzl/tmux/BUILD.bazel)"; test ! -d tools/bazel` | no dir-form reference; `check-ignore` prints nothing; `tools/bazel` is never a directory (it is the wrapper file once Task 0.2f lands) |
| Wrapper portability | 0.2f, 5.1a | wrapper_should_work_without_flock_ionice_systemd | Static | `BAZEL_WRAPPER_NO_FLOCK=1 bash tools/bzl/wrapper_test.sh`; `CI=true bash tools/bzl/wrapper_test.sh` | exit 0 on Linux and macOS runner |
| Early live-unit protection | 0.2i | live_service_should_have_oom_priority_before_bazel_load | Owner | `bash scripts/check-live-oom-protect.sh` | exit 0 (3 until the owner acts) |
| Fixtures before use | 0.1h, 1.6d | fixtures_should_exist_before_replay | Static | `bash tools/bzl/baseline/fixtures/run-make-path.sh \| jq -e '.fixtures\|length==9'` | `true` |
| Sample representativeness | 0.1i | partial_change_sample_should_be_representative | Static | `bash tools/bzl/baseline/pr-sample.sh \| jq -e '(.partial\|length==30) and (.repr.ok or .restratified)'` | `true` |
| Hermeticity before and after | 0.1j, 7.5g, 7.7a | hermeticity_should_improve_and_downgrades_be_counted | Perf | `bash tools/bzl/baseline/hermeticity.sh --system make`; `improvement.sh` T7 line | numbers present; T7 PASS only if H4 equals owner-accepted downgrades |
| Risky assumptions | 0.1f, 0.14e | ra1_ra2_should_be_premeasured | Static | `jq -e '(.ra1_median_fanout\|type=="number") and (.ra2_non_go_share\|type=="number")' tools/bzl/baseline/measured.json` | `true` |
| Mid-point gate | 2.7a | midpoint_gate_should_trip_on_each_target | Static | `python3 -m unittest tools/bzl/economic-gate_test.py` | one trip fixture per T1, T3, T4; clear exits 0 |
| Incremental path guard | 0.2e, 1.1a | build_path_check_should_not_need_later_tasks | Static | `bash tools/bzl/check-build-paths.sh` (Epic 0); `--full` after Task 1.1a | exit 0 in both |
| Make targets exist before hints | 1.5a, 0.2h | hint_targets_should_exist_when_hints_ship | Static | `make -n gazelle sync setup-git` | exit 0 |
| Translator precedence and negatives | 1.6a | translator_should_not_hint_on_benign_text | Static | `bash tools/bzl/translate-errors_test.sh --negative` | `PASS` for every negative fixture and the E1/E8 pair |
| Cache auth failure is loud and counted | 1.6a, 7.1a, 7.4b | cache_auth_failure_should_be_loud_and_counted | Static | `bash tools/ci/bazel-trend_test.sh` (403 fixture) | `bazel_cache_auth_failures_total 1` |
| Heartbeat and background run | 1.6b | long_run_should_heartbeat_and_background_mode_should_detach | Static | `bash tools/bzl/wrapper_test.sh` | at most 10 heartbeats; `BAZEL_BG=1` returns in 2 s |
| BUILD staleness warning | 1.6f, 8.2d | stale_BUILD_should_warn_before_build | Static | `bash tools/bzl/check-build-fresh-cheap_test.sh` | warns on new file/import; silent when BUILD also changed |
| Plain-go shims | 2.4e | plain_go_failure_should_name_make_sync | Static | `printf 'cannot find package "x/gen/proto/go/session/v1"' \| bash tools/bzl/translate-errors.sh --source go` | contains `Remedy: run 'make sync'` |
| Batch merged tied to census | 4.10f | merged_flag_should_follow_census | Static | `python3 -m unittest tools/bzl/spike/census-batches_test.py` | hand-set `merged` reverted when census slice is non-zero |
| Tooling before use | 4.8a, 4.9a, 4.10f | tooling_should_precede_its_callers | Static | task order in plan.md | 4.8a and 4.9a listed before 4.10 execution |
| Canary reconciles Bazel absence | 8.5a, 8.5b | canary_make_job_should_be_bazel_free | Static | `yq` checks in Task 8.5b | `true` / `true` |
| Removal PR can be green | 10.2d, 10.2c, 10.2a | legacy_jobs_should_be_deleted_before_recipes | Static | `test ! -e .github/workflows/fallback-canary.yml; grep -c 'legacy-' .github/workflows/build.yml` | exit 0; `0` before Task 10.2a starts |
| Repair push starts CI | 7.8a, 7.8b | repair_push_should_trigger_workflows | Build | `gh run list --workflow=bazel.yml --branch <repair-branch> --json event` | at least one run |
| Orphan cleanup protects owner files | 11.4a | owner_uncommitted_files_should_be_untouched | Static | `sha256sum -c "$TMPDIR/owner-files.sha"` | exit 0 |
| Dependency change annotation | 11.6c | dep_changes_should_be_summarised_on_PRs | Static | `bash tools/bzl/annotate-deps_test.sh` | exit 0 |
| Doc commands verified | 11.1d, 11.3a, 11.3d | doc_commands_should_be_listed_or_marked | Static | `bash tools/bzl/check-skill-proposals.sh --docs <files>` | exit 0 |

## Docs-derived additions (from `research/bazel-docs-best-practices.md`, 2026-11-01)

| Control | Plan task | Verification name | Type | Command | Expected |
|---|---|---|---|---|---|
| Startup flags reach the server (D1) | 0.2b, 0.2g | startup_flags_should_reach_server_and_startup_config_lines_rejected | Static | `bash tools/bzl/check-bazelrc_test.sh` (E9); `tr '\0' '\n' < /proc/$(bazel info server_pid)/cmdline \| grep -c max_idle_secs=300` | fixture `startup:agent` exits 1, `startup:linux` exits 0; count >= 1 |
| Known-absent flags rejected (D20, X1) | 0.2g, 0.9b | absent_flags_should_never_be_committed | Static | `bash tools/bzl/check-bazelrc_test.sh`; `bazel help build --long \| grep -c incompatible_sandbox_hermetic_tmp` | exit 0; `0` |
| Cross-path cache hit ratio (E1, E2) | 0.10e | second_worktree_at_other_path_should_hit_95pct | Perf | `bash tools/bzl/baseline/cross-worktree-cache.sh \| jq -e '.e1_hit_ratio>=0.95 and .e2_nonallow_spawns==0'` | `true` |
| Go compile path independence and digest determinism (E3, E4) | 0.10e | gocompile_and_outputs_should_be_path_independent | Parity | same script, `.e3_gocompile_identical and .e4_digest_lists_equal and .e4_control_flagged` | `true` |
| GC runs under idle timeout (E5) | 0.10f, 11.4c | disk_cache_gc_should_run_with_short_idle | Perf | `bash tools/bzl/baseline/cache-layout-checks.sh \| jq -e '.e5_gc_trimmed_with_idle_delay'` | `true` |
| Repo contents cache and reflink (E6, E10) | 0.10f, 0.10a | extracted_repos_should_be_shared_and_hits_reflinked | Perf | same script, `.e6_offline_build_ok`, `.e10_shared_extent` boolean recorded; `bazel-worktree-cost.sh \| jq -e '.cache_fs == .output_base_fs'` | `true`; filesystem equal |
| Containment alternatives and queue value (E7, E11) | 0.10g | serialised_builds_should_cost_at_most_60pct_cpu | Perf | `bash tools/bzl/baseline/containment-checks.sh \| jq -e '.e11_serial_over_concurrent_cpu<=0.60'` | `true` (recorded input to S6) |
| Disk + remote layering (E8) | 0.12f | disk_and_remote_cache_should_compose | Perf | `bash tools/bzl/baseline/cache-layering.sh \| jq -e '.b_remote_hits>0 and .c_delta_s<=10'` | `true` |
| Multiplex-worker inventory (user-requested; non-gating) | 0.10h | multiplex_workers_should_be_inventoried | Spike | `bash tools/bzl/baseline/worker-inventory.sh \| jq -e '(.outcome=="none-found" or (.rss_reduction_pct\|type=="number")) and (.verdict=="Pass" or .verdict=="NoAction")'` | `true`; "none found" is Pass-with-no-action, never skipped; within-one-build memory only, not cross-worktree |
| Orphan base with live server (D16, D17) | 11.4a | orphan_server_should_be_killed_via_pid_file | Static | `bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases_test.sh` | exit 0 |
| Merge driver text from docs (D19) | 0.10b | merge_driver_should_match_documented_snippet | Static | `grep -c 'bazel.build/external/lockfile' spike-results.md` | >= 1 |

## Selective e2e additions (ADR-009, 2026-11-02)

| Control | Plan task | Verification name | Type | Command | Expected |
|---|---|---|---|---|---|
| Registry coverage measured, gap list committed | 0.15b | registry_coverage_should_be_measured_and_gaps_listed | Static | `python3 tools/bzl/e2e/registry-coverage.py --json \| jq -e '.specs==193'`; `python3 tools/bzl/e2e/check-completeness.py` | `true`; exit 0 (gap list may only shrink) |
| Observed manifest covers every spec | 0.15a, 12.1c | manifest_should_cover_every_spec | Static | `python3 tools/bzl/e2e/check-manifest.py` | exit 0, nothing `unmanaged` |
| Selection is narrow | 0.15c, 12.4a | replay_should_select_at_most_half_of_specs | Perf | `bash tools/bzl/e2e/replay-selection.sh --json \| jq -e '.share_median<=0.50 and .share_p90<=0.90 and .all_fallback_rate<=0.15'` | `true` |
| No missed failures | 0.15d, 12.5c | mutation_backtest_should_have_zero_misses | Parity | `bash tools/bzl/e2e/mutation-backtest.sh --summary \| grep -c '^MISS '` | `0` |
| Bazel semantics hold | 0.15d | slice_cache_semantics_should_hold | Spike | `bash tools/bzl/e2e/semantic-checks.sh \| grep -cE '^S-[a-e]: PASS$'` | `5` |
| Binary not in the e2e graph | 12.3b | e2e_targets_should_not_depend_on_binary | Static | `bash tools/bzl/e2e/check-no-binary-dep.sh` | exit 0; exit 1 on the seeded dependency |
| Stale binary refused | 12.3a | wrapper_should_refuse_binary_from_other_revision | Static | `bash tools/bzl/e2e/wrapper_e2e_test.sh` | exit 0 |
| Verdict equals today's comparator | 12.2b | e2e_target_verdict_should_match_baseline_comparator | Static | `bash tools/bzl/e2e/run_playwright_test.sh` | exit 0 |
| Escapes detected and filed | 12.5b | nightly_failure_not_selected_on_pr_should_count_as_escape | Static | `python3 -m unittest tools/ci/e2e_escape_test.py` | exit 0 |
| Quarantine bounded, no cache write from PRs | 12.5d | quarantine_should_expire_and_prs_should_not_write_cache | Static | `python3 tools/bzl/e2e/check-quarantine.py`; `bash tools/bzl/check-workflow-cache-writes.sh` | exit 0 both |
| e2e work avoided (T8) | 12.4d, 7.5g | e2e_minutes_per_partial_pr_should_halve_with_zero_escapes | Perf | `bash tools/bzl/parity/e2e-avoided.sh \| grep -c '^T8: PASS$'` | `1` |
| Other slow suites classified | 12.7a | slow_suites_should_be_classified | Static | `bash tools/bzl/e2e/check-slow-suites.sh` | exit 0 |
| Shard planner balanced | 12.4b | shard_plan_should_be_within_125pct_of_mean | Static | `python3 -m unittest tools/ci/plan_e2e_shards_test.py` | exit 0 |

## Package scoping additions (ADR-010, 2026-11-02)

| Control | Plan task | Verification name | Type | Command | Expected |
|---|---|---|---|---|---|
| SM6: the model reproduces the real graph | 13.1a | rebuilt_package_graph_should_equal_go_list_graph | Static | `python3 tools/bzl/scoping/graph.py --identity-check \| jq -e '.diffs==0'` | exit 0 |
| SM6: the model reproduces RA1 | 13.1b | replay_should_reproduce_ra1_merged_median | Perf | `python3 tools/bzl/scoping/replay.py --model merged --pr-sample tools/bzl/baseline/pr-sample.json --json \| jq -e '.go_touching_median>0.25 and .go_touching_median<0.28'` | exit 0 |
| SM6: a test-only import does not enlarge dependents (target model) | 13.1a | test_edge_should_not_propagate_in_target_model | Static | `python3 -m unittest tools/bzl/scoping/graph_test.py` | exit 0 |
| SM6: planner rejects cycles and over-budget relocations | 13.1c | planner_should_reject_cycle_and_over_budget_relocation | Static | `python3 -m unittest tools/bzl/scoping/plan_split_test.py` | exit 0 |
| SM6: codemod is behaviour-preserving on a fixture | 13.1d | move_codemod_should_build_vet_and_test_after_move | Static | `bash tools/bzl/scoping/move_test.sh` | exit 0 |
| SM6: conflicts with open work are found | 13.1e | conflict_check_should_report_open_branch_touching_cluster | Static | `bash tools/bzl/scoping/conflicts_test.sh` | exit 0 |
| SM6: T9 collector | 13.1f, 13.7a | t9_should_hold_after_epic_13 | Perf | `bash tools/bzl/parity/scoping.sh \| grep -cE '^T9[a-e]: PASS$'` | `5` |
| SM6: no test-only layering inversion | 13.2a-c | lower_layer_tests_should_not_import_higher_layers | Static | `bash tools/bzl/scoping/check-test-inversions.sh` | exit 0; exit 1 on the seeded fixture |
| SM6: no test lost by the moves | 13.2a-b | test_function_count_should_be_unchanged | Static | `go test -list '.*' ./session/... ./server/... ./session/git/... \| grep -c '^Test'` (before and after) | equal |
| SM6: Tier 0 payoff | 13.2b | test_loc_share_should_drop_after_test_moves | Perf | `python3 tools/bzl/scoping/replay.py --commits 500 --json \| jq -e '.t9a_test_loc_share_500<=0.46'` | exit 0 |
| SM6: Tier A payoff | 13.3a-c | tier_a_should_shrink_the_two_largest_packages | Perf | `python3 tools/bzl/scoping/replay.py --commits 500 --json \| jq -e '.t9d_services_loc<=89000 and .t9d_session_loc<=91500 and .t9c_changes_with_lower_lib_loc>=0.11'` | exit 0 |
| SM6: Tier B payoff | 13.4b-c, 13.5c-e | tier_b_should_hit_predicted_sizes | Perf | `python3 tools/bzl/scoping/replay.py --commits 500 --json \| jq -e '.t9d_services_loc<=73000 and .t9d_session_loc<=72000 and .t9c_mean_lib_loc_share<=0.395'` | exit 0 |
| SM6: T1 stays narrow on the target model | every Story 13.x | target_model_share_should_stay_below_bound | Perf | `python3 tools/bzl/scoping/replay.py --commits 500 --json \| jq -e '.t9e_target_model_share<=0.30'` | exit 0 |
| SM6: relocated declarations introduce no new abstraction | 13.4a, 13.5a | relocation_should_not_add_interfaces | Static | `python3 tools/bzl/scoping/plan-split.py --pkg session --map tools/bzl/scoping/clusters/session-backlog.json --json \| jq -e '.new_interfaces==0'` | exit 0 |
| SM6: moved code respects Instance snapshot reads | 13.5c-d | moved_files_should_not_read_raw_instance_fields | Static | `go build ./session/... && make actor-field-guard` | exit 0 |
| SM6: Gazelle clean after a split | 13.3, 13.4, 13.5 | gazelle_should_have_no_diff_after_split | Static | `bazel run //:gazelle && git diff --quiet -- '*.bazel'` (once Story 1.1 exists) | exit 0 |
| SM6: packages do not regrow | 13.6a | package_budget_should_hold_and_fail_on_growth | Static | `bash tools/bzl/scoping/check_budget_test.sh` | exit 0 |
| SM6: new packages are boundary-guarded | 13.6b | depguard_should_reject_seeded_forbidden_import | Static | `bash tools/bzl/scoping/depguard_seed_test.sh` | exit 0 |

## Test Stack
- **Unit**: Go `testing` for helpers (`internal/reporoot`, `internal/testenv`, `tools/bzl/testsummary`), Python `unittest` for `check-spike-results.py` / `check-exclusions.py`, shell fixture tests (`affected-targets_test.sh`, `bazel-trend_test.sh`, `check-tymux-pin_test.sh`), Starlark `analysistest` (`ent_bzl_test`, `go_test_bzl_test`).
- **Integration**: `bazel build/test` on real graph, parity scripts under `tools/bzl/parity/`, `actionlint`, `shellcheck`, `goreleaser check`.
- **E2E / UX**: Playwright suite via `tests/e2e/` against the Bazel binary (P7); manual isolated instance (P9); owner device TCC check (P8). No user-facing UI changes, so no UX acceptance tests.

## UX Acceptance Tests
N/A. The only user-facing surfaces are the Makefile wrappers and agent ergonomics, covered by `check-make-help.sh`, `check-wrappers.sh`, and the `--config=agent` output bound (`wc -l <= 40`).

## Migration Test
Yes: `migration_should_be_reversible` (RC2 rows): flip PR revert restores `go build` path and `BUILD_SYSTEM=make make build` exits 0; FallbackCanary proves the make path weekly on a runner with no Bazel. Rollback of generated-code handling is trivial because `GenSurface` is never committed.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go (helpers added by this project) | `go test ./internal/reporoot ./internal/testenv ./tools/bzl/testsummary -coverprofile=c.out && go tool cover -func=c.out` | >= 80% line |
| Bazel test-set parity | `bash tools/bzl/parity/test-set-diff.sh <config>` | `missing_in_bazel: 0`, <= 10 excluded packages |
| Python checkers | `python3 -m unittest discover tools/bzl` | all pass, includes rejected-input cases |
| Requirements | this table | 40/40 mapped; gaps G1-G9 closed at the tasks listed above (verified at the Epic 0 gate, Task 0.14) |
