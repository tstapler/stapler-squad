# History adapters physical move: plan (HIST-slice step 2, `session/historyadapters`)

Measured 2026-11-14 on `efcbe4db7` (main after #1646), in a scratch copy of the tree. Owner decision: do the physical move (supersedes roadmap Q6 and the "Deferred: session P6-historyadapters" entry in `roadmap.md`). No production code is changed by this document.

## 0. Verdict

**GO, as two small mechanical PRs plus one prep PR; the roadmap's four blockers no longer exist.** Evidence is a compile-and-test probe, not reading: the whole history stack (5 adapters, `history_post_export.go`, `history_adapter.go`, `history_transfer.go`, `inhibition.go` and all their tests) was copied into a scratch package. It compiled with **zero source edits** apart from the package clause, and its 21 tests passed (1 skipped) under a 4-line `TestMain` (section 2.1). The payoff is OCP and a compile-time seam, **not** build speed: `scoping touch` reports a gain of 0.00 pp (section 1.4). Go/no-go per PR is in section 4.

## 1. Current measurements

### 1.1 What moves

| Group | Files (non-test) | LOC |
|---|---|---|
| Adapters | `session/agy_adapter.go` 431, `claude_adapter.go` 319, `gemini_adapter.go` 176, `opencode_adapter.go` 417, `pi_adapter.go` 177 | 1,520 |
| Post-export hook | `session/history_post_export.go` | 94 |
| Registry | `session/history_adapter.go` | 24 |
| Porter | `session/history_transfer.go` | 54 |
| Redaction | `session/inhibition.go` | 114 |
| **Total non-test** | **9 files** | **1,806** (4.1% of session's 44,361 non-test LOC in 176 files) |
| Tests that move | `claude_adapter_test.go` 43, `opencode_adapter_test.go` 112, `pi_adapter_test.go` 120, `history_post_export_test.go` 83, `history_adapter_test.go` 74, `history_transfer_test.go` 831, `inhibition_test.go` 58 | 1,321 |

Counts: `wc -l` over the files; `session` non-test total from `cat` of `session/*.go` excluding `_test.go`. Not in the move: `session/history.go`, `history_*_test.go` other than the above, `history_fork.go`.

### 1.2 Re-measured roadmap blockers

The roadmap's deferred entry (`roadmap.md:327`) listed four prerequisites. Status now:

| Roadmap prerequisite | Status | Evidence |
|---|---|---|
| Lowered canonical/records package | **Not needed, already done.** `CanonicalTurn`, blocks and roles live in `session/instance/canonical.go:102`; `HistoryAdapter`, `HistoryAdapterRegistry`, `ErrNoHistoryAdapter`, `HistoryPorterHook` and `HistoryRegistryHook` live in `session/instance/instance_program.go:109,123,162,171,183`. The roadmap's "records" (P1-records, rejected) is unrelated. | `grep` plus compile probe |
| `HistorySession` port in a lower package | **Not needed; recommend against.** Adapters take `*instance.Instance` and use only exported methods: `GetClaudeConversationUUID` x16, `ActiveDir` x9, `Title` x6 (field read), `WithClaudeSession` x5, `TryExtractConversationUUID` x4, `RecordPortedClaudeSession` x2, `PiSessionID` x2, `WithPiSession` x1 (grep over the 6 files). A port with one real implementation and no fake is the pattern `interface-pollution-checklist` rejects; the hermetic tests already use a real `Instance`. | grep, probe |
| Instance setters via the actor path | **Already satisfied.** `grep -nE '\b(inst\|i)\.[A-Z]\w+ = [^=]'` over the 6 adapter files returns nothing. Writes go through `RecordPortedClaudeSession` / `WithClaudeSession` / `WithPiSession`, added by `a11050427` (`session/instance/instance_claude.go`, `instance_pi_status.go`). | grep |
| Pi event reader relocation | **Already done** by `0eff4ffda`: the Pi event model and reader are in `session/instance/pi_events.go`; `pi_adapter.go` (177 lines) is only the adapter. | `git log`, probe |

What did change since the roadmap was written: `Instance` moved to `session/instance` (W3, #1638), which is why the leaf can import it directly. The cost is that the leaf is not light: `go list -deps` of the probe package lists 122 module packages (session: 131), because it imports `session/instance`. The leaf is a leaf *of `session`*, not a small dependency closure.

### 1.3 Back-edges and importers

`scoping move --from session --to session/historyadapters --dry-run` (the repo's own tool) for the full 16-file set reports every staying reference:

- Staying tests only: `session/instance_fork_test.go:142` uses unexported `rawClaudeTurn`; `session/instance_program_test.go:188-212` uses `DefaultHistoryAdapters()` (16 times), `NewAgyAdapter`, `NewGeminiAdapter`. No staying non-test file uses a moved symbol once the registry and porter move too.
- For the narrower step A (adapters + `history_post_export.go` only): staying `history_adapter.go:9-13` constructs the five adapters, and `history_transfer.go:39` uses `PostExporter`.
- Outside session: one non-test importer, `server/services/import_service.go:144` calling `session.ReadCanonicalTurnsFromFile` (declared at `session/claude_adapter.go:92`). The tool rewrites it to the new import.
- Direct importers of `session` today: 36 non-test packages (`go list -e` over `./...`). None name an adapter type; only `import_service.go` names an adapter-file symbol.
- Hook wiring is by `init()`: `session/history_adapter.go:24` sets `instance.HistoryRegistryHook`; `session/history_transfer.go:54` sets `instance.HistoryPorterHook`. `instance.HistoryRegistry()` panics when unset (`session/instance/instance_program.go:189`).
- Seven non-test packages import `session/instance` without importing `session` (`pkg/events`, `server/deliverygate`, `server/push`, `server/services/devtools`, `session/backlog/stuck`, `session/panereply`, `testutil`); nine packages do so in test code. Whether any of them reaches `HistoryRegistry()` is UNVERIFIED. This is why hook registration stays an `init()` imported by `session` (section 2.3).
- Stale comments naming the old paths (update in the move PRs, not behavior): `session/instance/instance_program.go:166,170,181,186,189`, `instance_claude.go:23,532`, `pi_events.go:157`, `pi_status_source_metrics.go:19,34,38`, `pi_status_source.go:4,62,521`, `scroll_adapter.go:47`, `instance_program_test.go:205`.

### 1.4 Payoff measurements

- `go run ./tools/bzl/scoping touch --files <the 16 files>`: `prod_changes=248 universal_hits=128 touch=0.5161 gain=0 (0.00 pp)`. Moving them does not reduce invalidation of other changes, so the SC6 ledger row stays "near zero", as the roadmap said.
- Churn: 9 of the last 500 commits on `HEAD` touch any of the 9 files; 7 touch the adapters or `history_post_export.go`. Most of those are the in-place refactors that prepared this move (`a11050427`, `0eff4ffda`, `00abc4d27`). Editing an adapter rarely recompiles `session` today, so there is little compile time to save.
- `scoping criteria --lower --relocated 0 --uses 7 --importers session,server/services --natural session,server/services`: SC1 PASS (count 0, max 10), SC3 PASS (0 of 5). `scoping t9` is all PASS today (T9d: services=70090, session=44251, max 72000); the move only lowers session.
- Budget: `tools/bzl/scoping/budget.txt:4` caps `session` at `files=179 loc=45520 imports=85`; a move only lowers it. A new row for the leaf is needed (section 3).

## 2. Dependency cut and lowering order

### 2.1 Target shape

```
session/historyadapters   (new leaf; imports session/instance, session/domain, session/dbpool, internal/sqlitedsn, log)
  agy_adapter.go claude_adapter.go gemini_adapter.go opencode_adapter.go pi_adapter.go
  history_post_export.go history_adapter.go (DefaultHistoryAdapters + init)
  history_transfer.go (PortSessionHistory + init) inhibition.go
session                   (keeps one blank import so every binary linking session still gets the hooks)
```

No new lowering package is required: the cut is already at `session/instance`. Probe imports (`go list`): agy `sqlitedsn, domain, instance`; claude `instance`; gemini `log, domain, instance`; opencode `sqlitedsn, log, dbpool, domain, instance`; pi `log, domain, instance`. None import `session`, so there is no cycle.

Probe setup, so a later agent can reproduce: copy the 16 files into `session/probe_h`, change `package session` to `package probe_h`, change `"detection/testdata/pi/..."` to `"../detection/testdata/pi/..."` in the Pi test, and add

```go
func TestMain(m *testing.M) {
	instance.InstallHermeticHooksForTest()                  // session/instance/instance_hermetic_testing.go
	instance.HistoryRegistryHook = DefaultHistoryAdapters() // the helper above replaced the real registry with a no-op one
	instance.HistoryPorterHook = PortSessionHistory
	os.Exit(m.Run())
}
```

Result: `go vet` clean; `go test -count=1 ./session/probe_h` ok (21 PASS, 1 SKIP). Needs `make sync` first in a fresh worktree (`gen/` and `session/ent` are not committed).

### 2.2 Cut order (what has to be true before each PR)

1. **Prep, in place** (PR-1): make the two staying tests and one declaration independent of what moves, so the later PRs are pure `scoping move` plus rewrite.
   - Relocate `PostExporter` (`session/history_post_export.go:20-22`) into `session/history_transfer.go`, its only consumer (`history_transfer.go:39`). Consumer-defined interface, structural typing: the adapters' `AfterExport` methods satisfy it without importing it. Use `scoping relocate --dir session --to history_transfer.go --decls PostExporter`.
   - `instance_fork_test.go:142`: replace `rawClaudeTurn` with a local `struct{ Message json.RawMessage \`json:"message"\` }` (it only asserts `Message != nil`).
   - Hygiene while here: the six raw `inst.Title` reads (`history_post_export.go:62,78`, `gemini_adapter.go:174`, `opencode_adapter.go:389,413`, `pi_adapter.go:175`) become `inst.Snapshot().Title` (`Title` is published in `session/instance/instance_snapshot.go:46`). This matches `.claude/rules/instance-lock-free-reads.md`; note that rule's globs are `session/instance/**` only, so it does not auto-load for the leaf. INFERRED: whether `Title` is ever written concurrently; the change is behavior-neutral either way.
2. **Move A** (PR-2): adapters + `history_post_export.go` + their 3 + 1 tests move; `history_adapter.go` and `history_transfer.go` stay and import the leaf.
3. **Move B** (PR-3): registry, porter, inhibition and their tests move; `session` gains the blank import; the two remaining staying tests switch to `historyadapters.DefaultHistoryAdapters()`.

### 2.3 Wiring decision: keep `init()` registration, imported by `session`

Registration stays an `init()`. `session` keeps a one-line blank import of the leaf (new file `session/history_wiring.go`), so every binary and test that links `session` still gets both hooks and no `instance.HistoryRegistry()` panic appears. This makes `session -> historyadapters`, not "leaf imported only by the composition root" as `package-layout.md` prefers. That deviation is deliberate and recorded as PR-4 (NO-GO, section 4): moving registration to `server/dependencies.go` would drop the hooks for any test or tool that links `session` but not the composition root, and the 7 `instance`-only importers above are unaudited.

A guard test in `session` closes the hole: `TestHistoryHooks_RegisteredWhenSessionLinked` asserts `instance.HistoryRegistryHook != nil` and `instance.HistoryPorterHook != nil`, so a removed blank import fails a test rather than panicking in production.

### 2.4 Explicit non-goals

- No `HistorySession` port (section 1.2).
- No change to `Instance`, to the hook variables in `session/instance`, or to the `HistoryAdapter` interface.
- No new adapters, no change to adapter ordering (`DefaultHistoryAdapters` order claude, agy, opencode, gemini, pi is exercised by `history_adapter_test.go` (Resolve cases), which moves with it).

## 3. PR sequence

Sized as one agent wave each; PRs are strictly sequential (each rebases on the previous merge). Common setup in every worktree: `make sync` (generates `gen/`, `session/ent`), `export GOCACHE=<private dir>` and `df -h /home` first (the repo has had disk-full incidents from Go cache regrowth). Common closing steps: `make gazelle` (never hand-edit `BUILD.bazel`), `make ready`, then `tools/bazel test //session/... //server/services:all` per `bazel-stapler-squad`. PRs open ready for review per repo rule (not draft).

### PR-1 `refactor(session): detach staying tests and PostExporter from the history adapters` (prep)

- Changes: the three items in 2.2 step 1. No file moves.
- Verify:
  - `go build ./... && go vet ./session/`
  - `go test ./session -run 'Fork|PortSessionHistory|HistoryAdapter|SwitchProgram|PostExport' -count=1`
  - `go run ./tools/bzl/scoping move --from session --to session/historyadapters --files agy_adapter.go,claude_adapter.go,gemini_adapter.go,opencode_adapter.go,pi_adapter.go,history_post_export.go,claude_adapter_test.go,opencode_adapter_test.go,pi_adapter_test.go,history_post_export_test.go --dry-run` prints no WARNING for `history_transfer.go` or `instance_fork_test.go` (the remaining warnings are `history_adapter.go` constructors only, handled in PR-2).
- Guard traps: none new. Pure in-package edit; `make gazelle` should be a no-op.
- Rollback: `git revert` the merge; no data or config effect.

### PR-2 `refactor(session): move the five history adapters into session/historyadapters` (Move A)

- Changes:
  - `go run ./tools/bzl/scoping move --from session --to session/historyadapters --files agy_adapter.go,claude_adapter.go,gemini_adapter.go,opencode_adapter.go,pi_adapter.go,history_post_export.go,claude_adapter_test.go,opencode_adapter_test.go,pi_adapter_test.go,history_post_export_test.go`; the tool rewrites `server/services/import_service.go` and the moved files. Then hand-edit `session/history_adapter.go` to call `historyadapters.NewClaudeAdapter()` etc.
  - Add `session/historyadapters/main_test.go` (`TestMain` as in 2.1, but via `sessiontest.Main`: `os.Exit(sessiontest.Main(m))` after the two hook lines, so `keyring.MockInit` and the loopback-network guard still apply; `session/internal/sessiontest/main.go:19` imports no `session`).
  - Pi test path: `"../detection/testdata/pi/basic_session.jsonl"` (`session/pi_adapter_test.go:22` today).
  - Add budget row `session/historyadapters files=<n> loc=<m> imports=<k>` (measure with `scoping budget --init session/historyadapters`, 3% rounded) in `tools/bzl/scoping/budget.txt`, and `session/historyadapters !server` in `tools/bzl/scoping/boundaries.txt` next to line 34. Lower the `session` row (`budget.txt:4`) only if `ready-scoping-budget` demands it; a cap may only be lowered.
  - Replace stale file-path comments listed in 1.3 that point at moved files.
- Verify:
  - `go build ./... && go vet ./session/... ./server/services/`
  - `go test ./session/historyadapters/ -count=1 -race` (expect the 21 PASS count of the probe for the moved subset; recount, do not copy)
  - `go test ./session -count=1 -run 'HistoryAdapter|history_transfer|PortSessionHistory|SwitchProgram|Fork|Checkpoint'` and `go test ./server/services -run Import -count=1`
  - `go list -deps ./session/historyadapters | grep -c 'stapler-squad/session$'` prints 0; `go list -deps ./session/historyadapters | grep stapler-squad/server$` prints nothing
  - `go run ./tools/bzl/scoping t9` all PASS; `go run ./tools/bzl/scoping boundaries`; `make ready-scoping-budget ready-scoping-boundaries`
  - `GOOS=windows go build ./session/... ./cmd/...` and `GOOS=darwin go build ./session/...` (the adapters carry no build tags today; confirm no file in the new dir gains one)
- Guard traps (all checked against the tree):
  - **Bazel data**: `session/BUILD.bazel` lists the adapters (lines 9, 32, 85, 91-93, 105, 109 for sources; 382, 444, 446, 449, 503, 509 for tests) and regenerates; the new `session/historyadapters/BUILD.bazel` is Gazelle-generated. The Pi test reads a file in another package, so the leaf's `go_test` needs `data = ["//session/detection:testdata"]`. That label is already used by `session/BUILD.bazel:639`; the `data` attribute there is not Gazelle output, so copy the explicit-`data` pattern of `session/instance/BUILD.bazel:232` and run `tools/bazel test //session/historyadapters:all`. Do not add the leaf to `tools/bzl/hand-build-files.txt`.
  - **Path-scanning tests**: ten or more tests under `session/*_test.go` walk or glob source files (`wake_polling_guard_test.go`, `no_reexport_alias_test.go`, `backend_assertion_guard_test.go`, `legacy_pr_readers_test.go`, ...). After the move they no longer see the adapter files. None names an adapter file (grep of `*.go` for `<agent>_adapter.go` / `history_*.go` under tests returned only comments), so no test breaks, but each guard silently covers 1.5k fewer lines. Under Bazel a scanning test that walks into the new directory needs `//session/historyadapters:go_sources` in its `data`; run `tools/bazel test //session:all` to find any.
  - **`sinkguard` tag**: only `server/deliverygate/sink_guard_test.go` and `flag_env_guard_test.go` carry `//go:build sinkguard`. `sink_guard_test.go:290` loads `./session/...`, so the new package is scanned; the adapters call no notification sinks (grep for `Notif` returned nothing). Run `go test -tags sinkguard ./server/deliverygate -count=1` anyway.
  - **delivery-gate `system_ids_test`**: `server/deliverygate/system_ids_test.go:41` scans every `NewNotificationEvent` call under the module root (walk at :46). The leaf has none, so no new ID form; run `go test ./server/deliverygate -run TestProducers -count=1`.
  - **`noarchivedrevival`**: its file set (`.claude/rules/noarchivedrevival.md:5`) names no adapter file, and the adapters call none of `Start(false)`, `RecoverFromStopped`, `restartForRetry` (grep). Nothing to update. Do not add `Start`/revive calls in the leaf later.
  - **`actor-field-guard`** (`Makefile:1301`): scans a fixed list of six non-adapter files, and the adapters have no direct field writes (grep). `pr-field-guard` is a `git ls-files` scan with a `session/instance/` exclusion; the leaf has no `GitHubPR*` writes. Both pass unchanged.
  - **`norawdb`**: `//nolint:norawdb` markers at `agy_adapter.go:376` and `opencode_adapter.go:355` travel with the code; `dbpool.OpenForeignReadOnly` at `opencode_adapter.go:61,142` is the approved path. ADR-036's table names the two files at the old paths (`docs/adr/ADR-036-database-pool-registry.md:28`); update the paths.
  - **goreleaser and make-only builds**: `.goreleaser.yaml` has no `session/` references (grep). The leaf has no build tags, so `go build`, `make build` (otelc-woven) and embedded-tmux builds compile it unchanged; verify with `make build`, not only `go build`, because otelc weaving is make-only.
  - **golangci `--new-from-rev` gates** (`gocyclo`, `gocognit`, `funlen`, `dupl`): git sees renames, but `scoping move` rewrites imports, so run `make ready-complexity-gate` before pushing; fix with a refactor, not a suppression, if a moved function trips it.
  - **`make gazelle-check`, `buildifier-check`** in `make ready` (`Makefile:1157`): run them; remedy for drift is `make gazelle && bazel mod tidy && make sync`.
- Rollback: `git revert` the merge commit. The hook wiring is unchanged (`init()` in `session`), so a revert restores the previous state with no data or flag implications. Budget/boundary rows revert with it.

### PR-3 `refactor(session): move history registry, porter and redaction into session/historyadapters` (Move B)

- Changes:
  - `scoping move` the remaining 6 files (`history_adapter.go`, `history_transfer.go`, `inhibition.go` and their tests `history_adapter_test.go`, `history_transfer_test.go`, `inhibition_test.go`).
  - Add `session/history_wiring.go` containing only a blank import of the leaf and a doc comment (why: hooks register in the leaf's `init()`; removing the import makes `instance.HistoryRegistry()` panic).
  - Add `TestHistoryHooks_RegisteredWhenSessionLinked` in `session` (2.3).
  - `instance_program_test.go:188-212`: call `historyadapters.DefaultHistoryAdapters()` and `NewAgyAdapter()` / `NewGeminiAdapter()` through the import (or move the two tests into the leaf with the registry; prefer moving, since they are adapter-resolution tests).
  - Leaf `TestMain` already re-registers real hooks (2.1).
  - Docs in the same PR: replace the deferred bullet at `roadmap.md:327` with an outcome note, and add `historyadapters` to the registry examples in `docs/explanation/package-layout.md:27,36`; fix the stale comments in 1.3 (`instance_program.go:166-189` currently say the hooks are assigned in `history_transfer.go`/`history_adapter.go` of `session`).
- Verify: the PR-2 list, plus `go test ./session -run TestHistoryHooks -count=1`, `go test ./session/instance -count=1`, `go test ./server/... -run 'SwitchProgram|Checkpoint|Fork' -count=1`, and a manual-instance smoke per `CLAUDE.md` (private port block, `STAPLER_SQUAD_TMUX_SOCKET`, never `:8543`): switch a session from claude to agy and back, confirm the `PortSessionHistory: ported session history` log line.
- Guard traps: all of PR-2's, plus **init order**: the leaf's `init()` must run before any `Instance` calls `HistoryRegistry()`; Go runs imported packages' `init` first, and `session` imports the leaf, so this holds, but the guard test is the proof. **Test isolation**: `history_transfer_test.go` (831 lines) mutates `HOME` and instance state; run it with `-race -count=3` once to confirm no new cross-test interference under the leaf's own `TestMain`.
- Rollback: `git revert`; if only the wiring file is suspect, restoring `session/history_adapter.go` and `history_transfer.go` is covered by the same revert.

## 4. Go / no-go

| Step | Call | Evidence | Honest payoff |
|---|---|---|---|
| PR-1 prep | **GO** | Dry-run shows exactly 3 staying references; XS in-place edit | Removes the only test-side back-edges; makes PR-2 mechanical |
| PR-2 move A | **GO** | Probe compiles with zero edits; 1,614 non-test LOC; one external importer | A new agent adapter no longer touches `session`; adapters cannot reach `session` internals (compile-time); session -3.6% LOC. No build-time gain (0.00 pp) |
| PR-3 move B | **GO** (owner decision; lowest value of the three) | Probe passes all 7 moved test files incl. the 831-line transfer test; 192 extra non-test LOC | `session` stops naming any adapter or the porter; `DefaultHistoryAdapters` is the single registration line for a new agent. Adds a blank-import coupling that PR-4 would have to undo |
| PR-4 composition-root registration (remove `init()` hooks) | **NO-GO** | 7 non-test and 9 test-side packages import `instance` without `session`; whether they reach `HistoryRegistry()` is UNVERIFIED | Would match "leaf imported only by the composition root", but risks a production `panic` for a purely structural gain. Revisit only after auditing those packages or replacing the panic with an explicit constructor argument |
| `HistorySession` port | **NO-GO** | 1 implementation, no fake needed, tests use real `Instance` | None; adds an interface to maintain |
| Records/canonical lowering | **N/A** | Already in `session/instance` | n/a |

Expected payoff, stated plainly: OCP and encapsulation, not speed. Measured: T9b gain 0.00 pp, 9 of 500 recent commits touch the code, and the leaf's dependency closure (122 packages) is nearly as large as `session`'s (131). Do not cite this move in the Bazel-scoping ledger as a gain.

## 5. Cost (price-weighted, per `~/.claude/skills/sdd/skills/ESTIMATION.md`)

All bands INFERRED; no implementer run has been measured. Assumed mix: implementation runs 10-15% output, review runs 5-10%. Story total includes the x1.5-2 verification multiplier.

| PR | Class | Raw tokens (one worker run) | Story CU (incl. verification) | Shape |
|---|---|---|---|---|
| PR-1 | XS | < 30k | ~0.05-0.1M | 3 small edits, 1 wave |
| PR-2 | S-M | 60-120k (mostly reading `make ready` and Bazel output) | ~0.15-0.3M | `scoping move` + BUILD regen + budget rows + comment fixes; 1 wave, 1 CI repair loop expected |
| PR-3 | S-M | 60-120k | ~0.15-0.3M | Move + wiring file + guard test + docs; 1 wave |
| Total | Medium-low | | **~0.35-0.7M CU** | 3 sequential waves; no parallelism between PRs (same files) |

Wall-clock blockers (the only calendar time): CI per PR (cold Bazel queue, `make ready` locally first), owner review of each PR, and merge ordering (PR-2 waits for PR-1, PR-3 for PR-2). Overrun checkpoint: pause and report if spend exceeds 25% over the row above.

## 6. Self-review notes

- Every line citation above was opened or grepped in this worktree at `efcbe4db7`; `scoping`/`go list`/probe numbers were produced by running the commands shown. Not run: kibitzer `split_sim` (the repo's own `scoping move --dry-run` and the compile probe cover the same question with stronger evidence), `make lint-custom`, `make ready`, and any Bazel build (all gated on a real move; they are listed as verify steps).
- UNVERIFIED: whether the `instance`-only packages call `HistoryRegistry()`; Bazel `data` generation for the leaf's Pi test (pattern given, outcome depends on the Gazelle run); whether `make ready-complexity-gate` flags any moved function.
- The probe used hermetic no-op hooks for the process manager; adapter behavior against real tmux sessions is covered only by the manual-instance smoke in PR-3.
