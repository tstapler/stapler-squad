# Pre-Mortem: lower-rework-turn-caps

**Date**: 2026-10-07  
**Feature**: Expose `AutonomousMaxTurns` in Settings → Global Defaults form  
**Premise**: Imagine this project shipped and failed. What most likely went wrong?

---

## Failure Modes

### FM-1 — Proto field number collision corrupts wire format (P1)

**Failure**: A concurrent PR merges to main after this PR branches but before it merges, claiming `SessionDefaultsConfig` field 15 or `UpdateGlobalDefaultsRequest` field 13 for a different field. The binary protobuf encoding is silent — the server receives `autonomous_max_turns: 50` and interprets it as whatever field the concurrent PR claimed, setting the wrong config knob without error.

**First symptom**: After deploy, changing the "Max Autonomous Session Turns" input and saving has no visible effect on turn caps; inspecting `config.json` shows either no change or an unexpected change to a different field. No error surfaces client- or server-side because protobuf's field-number encoding is type-agnostic for `int32`.

**Prevention**: Before Phase 1, run `git fetch origin && git log origin/main -- proto/session/v1/session.proto` to confirm no concurrent proto changes have landed. Before opening the PR, re-verify field 15 and field 13 are still free in the upstream `main` (not just the branch). If another PR is in flight that touches the proto, coordinate explicitly — serial merge order, not concurrent field reservation.

---

### FM-2 — Submit wiring omitted: field renders but never persists (P2)

**Failure**: Task 3.1.1c (adding `autonomousMaxTurns` to the `updateGlobalDefaults` call in `handleSave`) is accidentally skipped or misspelled. The `autonomousMaxTurns` state variable exists, the input renders with the server-provided value, and save appears to succeed (the RPC returns 200), but the field is silently absent from the request body.

**First symptom**: A user changes the turn cap, saves, reloads the Settings page, and sees the previous value. Checking the network tab shows the `updateGlobalDefaults` request does not include `autonomousMaxTurns`. The server-side log line `updated global session defaults` fires (other fields changed), masking the omission.

**Prevention**: Add the submit-path test identified in adversarial review CONCERN 4 before marking Phase 3 done: simulate a user changing the value and submitting, then assert the mocked `updateGlobalDefaults` was called with `autonomousMaxTurns` equal to the entered value. This test cannot pass if 3.1.1c is missing.

---

### FM-3 — Phase 4 uses `make test` instead of `make ci`, PR fails CI on first push (P2)

**Failure**: Task 4.1.1a runs `make build && make test` as written in the plan, not `make ci`. The PR is pushed; CI's `fmt-check` step fails because the proto edit or Go file touch introduced a `gofmt` deviation. Alternatively, `make registry-generate` produces a diff in `docs/registry/features/` that the CI check catches. The PR is blocked at CI and requires a fix commit, burning reviewer time.

**First symptom**: The first CI run on the PR shows a red `fmt-check` or `registry-diff` job within minutes of push, despite all local tests passing.

**Prevention**: Replace `make build && make test` in Task 4.1.1a with `make ci` per adversarial review CONCERN 2. If `make ci` is too slow for iteration, at minimum add `make fmt-check registry-generate` after `make test` and confirm both exit 0 before pushing. Run `gofmt -w .` on every Go file touched as a baseline habit.

---

### FM-4 — Generated files accidentally committed, causing stale-gen CI failures on the next proto PR (P2)

**Failure**: `gen/session/v1/session.pb.go` and/or `web-app/src/gen/` are staged and committed, either via `git add -A` or a force-add (`git add -f gen/`). The committed generated files become stale the next time any proto changes, because `make proto-gen` regenerates them from source and `git diff --exit-code gen/` detects the divergence. The next unrelated proto PR is blocked by this project's stale gen files.

**First symptom**: A PR that modifies a different proto message fails CI's `proto-gen && git diff --exit-code gen/` check with a diff that points to `AutonomousMaxTurns`-related lines — content owned by this project, not by the PR that surfaced it.

**Prevention**: After `make proto-gen` (Task 1.1.2a), run `git status` and confirm `gen/` and `web-app/src/gen/` are listed as untracked or modified but not staged. When staging for commit, add only named files: `git add proto/session/v1/session.proto server/services/defaults_service.go server/services/defaults_service_test.go web-app/src/components/settings/GlobalDefaultsForm.tsx web-app/src/components/settings/GlobalDefaultsForm.test.tsx server/services/backlog_service.go`. Never use `git add -A` or `git add .` in this repo.

---

### FM-5 — Ceiling-clamping contract untested; a future change silently allows out-of-range values (P3)

**Failure**: The plan's two sub-tests (`ZeroResetsToDefault` and `ExplicitValueRoundTrips`) are implemented but the `AboveCeilingClampedAt200` test from adversarial review CONCERN 1 is not added. A later PR moves ceiling enforcement from `AutonomousMaxTurnsOrDefault()` to a validation layer, or the ceiling constant is changed, or the config read path is refactored. No test catches the breakage; `autonomous_max_turns: 999` is now stored and returned as-is, allowing runaway sessions.

**First symptom**: A backlog session with a misconfigured turn cap runs well past 200 turns, accumulating cost and holding a worktree lock until manually killed. `config.json` shows `autonomous_max_turns: 999`; `AutonomousMaxTurnsOrDefault()` returns `999` rather than `200`.

**Prevention**: Add the `AboveCeilingClampedAt200` sub-test as part of Task 2.1.3b: send `AutonomousMaxTurns: 201`, assert the response returns `int32(200)`. Also export `autonomousMaxTurnsDefault` as `config.AutonomousMaxTurnsDefault` (adversarial MINOR 1) so both the default and ceiling are referenced by compile-time constants rather than magic literals in tests.

---

## Summary

| # | Failure Mode | Severity |
|---|---|---|
| FM-1 | Proto field number collision corrupts wire format | P1 |
| FM-2 | Submit wiring omitted: field renders but never persists | P2 |
| FM-3 | Phase 4 uses `make test` not `make ci`, PR fails CI on push | P2 |
| FM-4 | Generated files accidentally committed | P2 |
| FM-5 | Ceiling-clamping contract untested | P3 |

**P1**: 1 · **P2**: 3 · **P3**: 1

**Top failure mode**: A concurrent proto PR claiming field 15 or 13 causes silent wire-format corruption — the turn cap form saves without error but changes the wrong config knob.
