# Adoption Recommendation: go-auto-instrumentation (`otelc`)

**Date**: 2026-08-22
**Purpose**: requirements.md's fourth Success Metric — "a written recommendation on whether/how to move this from opt-in to the default build path, including what would have to be true first." This document synthesizes `spike-verdicts.md` (all four Phase 1 spikes plus Spike E and its addendum), `overhead-report.md` (Phase 3), and `parity-report.md` (Phase 2's Epic 2.3) into a single verdict.

---

## Verdict

**Adopt as default once the named conditions below hold.** Do not adopt as default now.

Why not "adopt now": Spike B — the project's single go/no-go gate — passed outright (`spike-verdicts.md`, "Spike B — CGO and SQLite weaving", Verdict: PASS), the opt-in build path is fully working and structurally isolated from the default build (`Task 2.1.3c`'s Isolation Guard proof, `spike-verdicts.md` lines 264–305), and the Subprocess Hook that was flagged as first-to-cut in `requirements.md`'s Appetite section was in fact built and verified end-to-end (`spike-verdicts.md`, "Spike E — Addendum", Story 5.1.3's captured span). That is a genuinely strong result. But requirements.md's own first Success Metric — "functionally identical … passes the existing test/e2e suite" — is only partially substantiated: `parity-report.md`'s Story 2.3.1 records the Go unit-test suite's behavior under weaving as **NOT SAFELY DETERMINABLE** (two reproduced extreme-load incidents, not retried a third time), and Story 2.3.3's summary table records the Playwright e2e suite as **not run** against the woven binary. A default-build flip changes what every contributor's `go build`/`make ci` produces; making that change before the suite that would catch a regression has ever been run against the new default is the exact failure mode `pre-mortem.md` and `ADR-002` were written to avoid extending past Phase 1.

Why not "do not adopt at all": nothing found in four spikes, an overhead report, and a parity report falsifies the tool. No hot path showed a measurable regression (`overhead-report.md` Story 3.1.3), the flagship target (ent's `database/sql` driver) produces real spans (`spike-verdicts.md` Spike B, Story 1.3.2), and coexistence with the existing manual `otelhttp`/`otelconnect` instrumentation is clean (`spike-verdicts.md` Spike C, Story 1.4.1: exactly one root span per driven request, no Duplicate Span Hierarchy). A "do not adopt" verdict would be stronger evidence than what was actually found — the gaps below are about **unverified**, not **disproven**.

---

## Named preconditions

Each precondition is independently checkable, cites its source artifact, and states what "checked" looks like.

### P1 — `go test` weaving verified safe in a resource-isolated environment
**Not yet true.** Two independent `go test -c` attempts under Toolexec Injection each triggered a reproducible, near-instant extreme system-load spike (load average 14648 and 1878 respectively, on a 24-core machine) — `parity-report.md`, Story 2.3.1, table and command log. A third, general failure mode was found separately while building the Subprocess Hook: `otelc` v1.0.1 produces duplicate global symbols when multiple import-related packages are given to `otelc setup`/`go test` together (`spike-verdicts.md`, "Spike E — Addendum", defect 3) — a distinct, reproducible link-time failure, not the load-spike one.
**Check**: run `go test ./...` under Toolexec Injection inside a container or VM with a hard cgroup CPU/process-count limit (per-package if the duplicate-symbol bug in P8 below is still unresolved), and confirm (a) no load anomaly and (b) pass/fail parity with the unwoven baseline for every package. Never attempt this on a shared interactive workstation — `parity-report.md` Story 2.3.1 states this explicitly.

### P2 — Playwright e2e suite run against the woven binary
**Not yet true.** `parity-report.md` Story 2.3.2/2.3.3: the `TEST_SERVER_BINARY` override was implemented in `tests/e2e/helpers/test-server.ts` and verified as a no-op-when-unset by inspection, but the full suite run (`TEST_SERVER_BINARY=$(pwd)/../../stapler-squad-otel npm test`) was not executed, for machine-safety reasons unrelated to any defect in the binary.
**Check**: `cd tests/e2e && TEST_SERVER_BINARY=$(pwd)/../../stapler-squad-otel npm test` completes with the same pass rate as a normal run, while watching for `ensureBinary()`'s "Building Go binary…" log line — its presence means the harness silently rebuilt an unwoven binary instead of using the woven one, invalidating the run.

### P3 — macOS build survival validated
**Not yet true — Linux-only verdict.** Every spike in `spike-verdicts.md` ran on Linux. `validation.md`'s coverage table (Gap #1) and `plan.md`'s Unresolved Question 6 both name the macOS `CGO_LDFLAGS`/Info.plist embedding path (`Makefile:148-153`, `Makefile:278-283`) as untested against a woven build.
**Check**: run `make build-otel-auto` on a macOS host and confirm the Info.plist blob still embeds and verifies correctly (the same `otool -s __TEXT __info_plist` check the existing macOS build targets already run) after Toolexec Injection.

### P4 — Build-time cost is explicitly accepted, or reduced
**Not yet true — cost is real and currently unaddressed.** `overhead-report.md` Story 3.1.4: with a warm build cache (the realistic day-to-day case), `make build-otel-auto` costs **1.79x** the wall-clock time (60.11s vs 33.627s) and **~4.81x** the CPU-seconds (312.89 vs 65.06) of `make build`, because `otelc setup`'s per-invocation module-resolution/rule-pinning tax does not benefit from Go's build cache the way a plain `go build` does. Flipping this onto every contributor's default inner loop multiplies that tax across every `make build`/`make ci` invocation repo-wide.
**Check**: either (a) a documented decision that this cost is accepted as the price of default coverage, made explicitly at flip time — not silently absorbed — or (b) a re-run of Story 3.1.4's exact methodology against a newer `otelc` release showing the warm-cache ratio has materially closed.

### P5 — Slow-shutdown / orphaned-tmux-session risk fixed or mitigated
**Not yet true.** `overhead-report.md` Story 3.1.2's secondary finding: the woven+disabled instance took 10+ seconds to respond to SIGTERM (vs. 1–3s for the baseline and the woven+enabled instance) and had to be escalated to SIGKILL, which left 5 orphaned tmux sessions behind that graceful shutdown never reached (`overhead-report.md`, Cleanup section).
**Check**: repeat the same kill test (`kill <pid>` on a woven+disabled instance under load) and confirm exit within the baseline's ~1–3s window with zero orphaned tmux sessions afterward (`tmux ls` clean).

### P6 — Persistent background export-retry cost when disabled, addressed
**Not yet true.** `overhead-report.md` Story 3.1.2's secondary finding: even with `OTEL_ENABLED`/`DD_TRACE_ENABLED` unset, the woven binary's Injected Bootstrap still attempts an "auto-export" and logs a repeating `traces export: … connect: connection refused` roughly once per second for as long as no collector is reachable — a background goroutine and log-volume cost a true no-op would not have. This is not a data leak (Spike D independently confirmed zero spans reach a collector when disabled — `spike-verdicts.md`, Spike D, Story 1.5.1), just resource/noise cost.
**Check**: confirm no repeating export-retry log line appears in a woven+disabled process's output/log over an extended run, or document the cost as accepted if `otelc` has no suppression knob for it.

### P7 — The two pre-existing, unrelated bugs discovered during this project are filed (and, ideally, resolved) independently
**Not yet true — filed nowhere yet; not fixed, correctly out of scope for this project.**
- (a) `telemetry/telemetry.go`'s `resource.Merge` Schema URL conflict (semconv v1.24.0 pinned at `telemetry/telemetry.go:20` vs. the SDK's resolved v1.41.0 default) makes `telemetry.Initialize` error out and silently prevents this repo's own manual `TracerProvider` from ever installing — on `main`, today, independent of otelc or weaving (`spike-verdicts.md`, Spike C, "Blocking discovery"; also documented in `.claude/docs/opentelemetry-auto-instrumentation.md`'s "Known limitations"). Because otelc's Injected Bootstrap installs its provider first (at `init()` time), this bug is currently what makes Spike C's coexistence result look clean — there is really only one active provider today, not two coordinating ones. **If this bug is fixed independently, Spike C's coexistence finding must be re-verified**, since two independently-configured providers would then race to call `otel.SetTracerProvider`.
- (b) The `log` package's `GetConfigDir()` (`log/log.go:312-320`) is hardcoded to `~/.stapler-squad` and does not consult `STAPLER_SQUAD_INSTANCE` (unlike `config.GetConfigDir()`, which does), so every instance's file-logged lines share one log file (`spike-verdicts.md`, Spike C, "Logging note").
**Check**: both filed as their own tracked bugs (GitHub issue or backlog item), independent of this project, per `requirements.md`'s Out of Scope ("Building or contributing upstream fixes … bugs found get filed") and this repo's `.claude/rules/fix-flaky-tests-dont-defer.md` posture of flagging rather than silently dropping discovered defects. (a) is a blocking precondition for *trusting* the coexistence finding at default-flip time; (b) is not blocking but should not be lost.

### P8 — otelc's multi-target `go test` duplicate-symbol bug (v1.0.1) resolved or generally worked around
**Not yet true.** `spike-verdicts.md`, "Spike E — Addendum", defect 3: `otelc setup`/`go test` run against multiple import-related target packages together (e.g. `./executor/...` alongside `./session/git/...`, where one imports the other) fails to link with a duplicated-symbol error in otelc's own bundled `go.opentelemetry.io/otel` instrumentation runtime — reproducible with zero custom rules, so this is a general `otelc` v1.0.1 limitation, not a defect in the Subprocess Hook. The only workaround exercised was running each target package as its own separate `otelc-auto-build.sh go test <single-package>` invocation (`spike-verdicts.md` addendum, "Story 5.1.2 verification"). See "Scoped out" below — no general fix was attempted.
**Check**: either a fixed `otelc` release (check `otelc version` and the project's release notes/issue tracker for this symbol-collision class) or a general per-repo fix (e.g. `scripts/otel-auto-build.sh` auto-splitting a multi-target `go test` invocation into per-package calls) exists before relying on a single, repo-wide `go test ./...` invocation under weaving.

### P9 — `otelc` version re-confirmed at flip time
**Not yet true (not yet re-checked since project start).** `otelc` is an explicitly external binary, not pinned in `go.mod`/`go.sum` (`.claude/docs/opentelemetry-auto-instrumentation.md`'s opening line; ADR-004's "Context"). Every finding in this document was produced against `v1.0.1` (`spike-verdicts.md`, "Tool acquisition — otelc"), installed on Go 1.26.3/1.26.4 against otelc's stated Go 1.25+ floor (no stated ceiling).
**Check**: run `otelc version` immediately before any default-flip decision and confirm it still matches `v1.0.1`, or — if it has moved — re-run at minimum Spike B (the go/no-go gate) and the Suppression Smoke Test (`scripts/otel-auto-smoke.sh --suppression`) against the new version before trusting this document's findings for it.

---

## Functional-parity evidence, stated at the strength it's actually backed by

Per requirements.md's first Success Metric ("functionally identical … passes the existing test/e2e suite"), `parity-report.md`'s Story 2.3.3 summary table is the authoritative source. Restated here without inflation:

| Suite | Woven? | Result |
|---|---|---|
| Go unit suite (`go test ./...`) | Attempted, not completed | **Unverified — coverage gap** (P1 above) |
| CLI surface (`--help`, `version`) | Yes | **PASS** — byte-identical apart from a disclosed 5-line Injected Bootstrap log preamble printed on every invocation |
| e2e startup contract (manual HTTP/ConnectRPC drive) | Not run this session as a dedicated check | **Substituted by prior evidence** — Spike B/C already drove a woven binary through HTTP + ConnectRPC requests successfully with a clean SIGTERM exit (`spike-verdicts.md` Spike B, Story 1.3.1) |
| Playwright e2e suite | Override implemented, run not attempted | **Not run — coverage gap** (P2 above) |

Only the CLI-surface leg is a genuine PASS on the literal suite named in the Success Metric. The other three legs are either a coverage gap (P1, P2) or substituted with adjacent-but-not-identical evidence (the e2e startup contract). This recommendation does not claim "passes the existing test/e2e suite" as settled — it names P1 and P2 as the specific, checkable work that would settle it.

---

## Scoped out

These were investigated or discovered but deliberately not carried further in this project, with the reason and what it would take to pick each one up:

- **A general fix for otelc's multi-target `go test` duplicate-symbol bug (P8).** Root-caused down to `generateRuntimePerPackage` writing an independent copy of otelc's own global runtime stubs into each target package's generated file (`spike-verdicts.md` addendum, defect 3) — but no general fix was attempted. Reason: this surfaced while implementing the Subprocess Hook (Story 5.1.2), itself already the first-to-cut item in the Large appetite (`requirements.md`, Appetite); an upstream-facing fix or a general per-repo splitting mechanism is further out of scope than the hook story that found it. To pick it up: either file the bug against `open-telemetry/opentelemetry-go-compile-instrumentation` with the exact reproduction (`spike-verdicts.md` addendum has the command and error), or extend `scripts/otel-auto-build.sh` to auto-split a multi-package `go test` invocation into per-package calls the way Story 5.1.2's own verification did by hand.
- **A third attempt at full-suite `go test` weaving.** Both attempts recorded in `parity-report.md` Story 2.3.1 were killed by their own safety monitoring after reproducing severe load spikes; a third attempt on the same shared, multi-tenant machine was explicitly not made. Reason: `parity-report.md` states this outcome would need a resource-isolated environment to attempt safely, which this session's machine was not. To pick it up: see P1's check above.
- **The full e2e startup contract and Playwright suite runs (P2).** Not run this session for machine-safety reasons following the same load-spike incident, not because of any finding against the binary. To pick it up: see P2's check above.
- **Fixing the two pre-existing, unrelated bugs (P7).** Correctly out of scope per `requirements.md`'s Out of Scope section (fixes to code outside this project's own build-path/instrumentation work) — filed as follow-up items instead of fixed here.
- **The pre-existing flaky tests surfaced during Phase 3's baseline benchmark capture** (`TestHandleActuatorHealth_ReturnsOK_InNormalConditions`, `TestServer_Shutdown_JoinsBackgroundTickers`, both in package `server`, plus `TestManagedProcess_Wait_nonZeroExit` in package `executor`) — encountered while running `go test -bench … ./...` with `-count=8` for `overhead-report.md` Story 3.1.1, unrelated to weaving and pre-dating this project. Not fixed here (out of this story's scope per `overhead-report.md` Story 3.1.1's own note); per `.claude/rules/fix-flaky-tests-dont-defer.md` these should be root-caused or filed as tracked bugs by a follow-up rather than re-excused again the next time they're seen. **Recorded here as the explicit flag that rule requires**, not silently dropped.

**Not scoped out** (for the avoidance of doubt, since the Large appetite named it first-to-cut): the Subprocess Hook (Phase 5) **was built and verified**, not cut — `spike-verdicts.md`'s Spike E Addendum records a real captured `git` subprocess span from `instrumentation/otelc/safeexec`'s hook, and `scripts/otel-auto-smoke.sh --with-subprocess` makes that a repeatable assertion.

---

## Rollback (for a hypothetical future default flip)

Two independent layers, both already structurally in place today per ADR-003 and `research/features.md` §3 ("A rollback-safe opt-in surface"):

1. **Revert the build target change.** Because containment is structural — a dedicated `build-otel-auto` target producing a distinct `stapler-squad-otel` binary, never referenced by `build`, `ci`, `ready`, `quick-check`, `pre-commit`, or `install-service` (ADR-003) — a default-flip would itself be a small, revertible Makefile/CI change (making `build`/`ci` invoke the weaving path). Reverting that one change fully restores the pre-flip state; nothing about the live systemd-managed instance or its deploy path is otherwise touched.
2. **Ship it built, but leave the runtime toggles unset.** `OTEL_ENABLED` and `DD_TRACE_ENABLED` independently gate telemetry emission at runtime and are read by the running binary, not the build (ADR-004's "Context" section is explicit that build-time and runtime configuration are two different processes). Even if the default *build* were flipped to weave, leaving both toggles unset at the operator/deploy layer means the woven binary behaves as documented in Spike D (`spike-verdicts.md`, Spike D, Story 1.5.1: zero spans reach a collector when both toggles are unset) — a second, independent kill switch that doesn't require a code revert at all.

---

## Staying current

The Isolation Guard that protects the default build (`Task 2.1.3c`, proven to correctly fail on `ci`/`ready` reachability and pass when reverted) also guarantees, by design, that CI never runs `make build-otel-auto` — so nothing else in this repo's automation will ever notice the woven build silently breaking (`pre-mortem.md` #2, P1). This document names a concrete, dated revisit trigger rather than leaving this as a point-in-time verdict:

- **Revisit by 2027-02-22** (approximately 6 months from this document's date): re-run `./scripts/otel-auto-smoke.sh` (and `--with-subprocess`) against the current `main`, and re-check `otelc version` against the `v1.0.1` recorded in `spike-verdicts.md`'s "Tool acquisition" entry. If either the smoke test fails or the version has moved, treat this as a signal to re-run this document's preconditions rather than assume they still hold.
- This should be filed as a dated backlog item (or an equivalent periodic, non-CI-wired check — e.g. a calendar reminder tied to the same cadence this repo already uses for other non-CI-gated maintenance) at the time this document is accepted, since — per the same Isolation Guard property — no automated system will raise this on its own.

---

## Self-review

Re-read once, adversarially, against "no percentage/conclusion without a command/file-path/table backing it":
- The verdict's "Spike B passed outright" and "Subprocess Hook was built, not cut" claims both cite the exact `spike-verdicts.md` sections and story IDs backing them, not just spike-verdicts.md as a whole.
- Every precondition (P1–P9) names the source artifact and section, and states a concrete, re-runnable check — none rests on "this should probably be fine."
- The build-time ratios (1.79x, 4.81x) are copied verbatim from `overhead-report.md`'s Story 3.1.4 ratio table, not recomputed or rounded differently here.
- The functional-parity table is a direct restatement of `parity-report.md`'s own Story 2.3.3 table, not a paraphrase that could drift from it — checked side by side against the source table above.
- The flaky-test items are named with their exact test names and packages, matching `overhead-report.md` Story 3.1.1 and this repo's `.claude/rules/fix-flaky-tests-dont-defer.md`, and are flagged rather than silently dropped, per that rule's explicit requirement.
- No claim in this document states or implies that `go test` weaving "doesn't work" or "fails" — the sourced language throughout is "not safely determinable" / "unverified," matching `parity-report.md`'s own careful phrasing, since overclaiming a failure would be exactly as dishonest as overclaiming a pass.
