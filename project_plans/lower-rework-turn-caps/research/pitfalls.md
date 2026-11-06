# Pitfalls & Risks — lower-rework-turn-caps

## 1. Proto field number availability — no collision risk

**`SessionDefaultsConfig`**: fields 1–14 are used; field 14 is `retry_policy` (`session.proto:2213`). Next available: **15**.

**`UpdateGlobalDefaultsRequest`**: fields 1–12 are used; field 12 is `retry_policy` (`session.proto:2292`). Next available: **13**.

No gaps or reserved ranges are present between those bounds in either message, so simply appending `int32 autonomous_max_turns = 15` and `int32 autonomous_max_turns = 13` is safe.

## 2. `make proto-gen` — requires `buf`, but it is installed

The Makefile (`Makefile:547-556`) runs `buf generate proto`. `buf` 1.66.1 is present at `/home/linuxbrew/.linuxbrew/bin/buf`. The generated output lands in two gitignored directories — `gen/` (Go protobuf/connect stubs) and `web-app/src/gen/` (TypeScript connect stubs) — which do NOT exist in a fresh worktree. **`make proto-gen` must be run before `go build` or the TypeScript compiler can find the new field.** Do not `git add` anything under `gen/` or `web-app/src/gen/`; `.gitignore` (`line 33-35`) explicitly excludes them and force-adding generated files has caused past breakage (see CLAUDE.md).

The one fragile edge: `proto-gen` also requires `web-app/node_modules/.modules.yaml` (it's a Make prerequisite at `Makefile:551`). In a fresh worktree without `pnpm install`, the stamp check fires the full generation path unconditionally — this is fine, but it means `make proto-gen` in a bare checkout will also attempt to build the TypeScript protoc plugin. If `pnpm` or node modules are missing, the generation will fail before `buf generate` runs. Run `make ensure-tools` first if in doubt.

## 3. `sharedBacklogCfg` propagation — NOT needed for `AutonomousMaxTurns`

The `sharedBacklogCfg` pattern (`defaults_service.go:44-64`, `186-190`) exists for exactly two fields: `MaxAutoReworkIterations` and `MaxConcurrentBacklogWorkItems`. Those are the only fields `BacklogService` reads from a long-lived `*config.Config` pointer (loaded once at process start via `server/dependencies.go`). Without the propagation, a change would not take effect until the next process restart.

`AutonomousMaxTurns` is different: every caller of `AutonomousMaxTurnsOrDefault()` does a fresh `config.LoadConfig()` at the moment it starts a driver — see `autonomous_orchestration_service.go:217,235` and `session_creation_pipeline.go:346`. There is no long-lived pointer. Saving via `cfg.AutonomousMaxTurns = int(req.Msg.AutonomousMaxTurns)` + `config.SaveConfig(cfg)` is sufficient; the next autonomous session creation will read the new value from disk automatically.

**Risk if the pattern IS incorrectly copied**: adding `d.sharedBacklogCfg.AutonomousMaxTurns = …` to the propagation block would compile and be harmless (BacklogService never reads that field), but it's dead code and should not be added.

## 4. Stale "server default (3)" comments — two proto locations, one Go comment

The comment `DefaultMaxAutoReworkIterations = 5` in `config/config.go:1059` is the authoritative Go constant. Three places currently embed the stale "(3)" value:

| File | Line | Text that needs updating |
|---|---|---|
| `proto/session/v1/session.proto` | 2200–2201 | `"use the server default (3)"` in `SessionDefaultsConfig.max_auto_rework_iterations` comment |
| `proto/session/v1/session.proto` | 2282 | `// 0 = use the server default (3).` in `UpdateGlobalDefaultsRequest.max_auto_rework_iterations` comment |
| `server/services/backlog_service.go` | 950 | `// configurable rework cap (config.MaxAutoReworkIterationsOrDefault, default 3).` |

The `config/config.go:315,320` "(3)" references are for separate fields (`DiagnoseNudgeMaxAttempts` and `NoopDispatchThreshold`) that genuinely default to 3 — those are correct and must not be changed.

The `docs/tasks/backlog-feature-improvement.md:998` and several `project_plans/` `.md` files also say "default 3" but those are historical plan artifacts, not live code paths; they do not affect runtime behavior.

## 5. Ceiling enforcement — clamped at read time, not at save time

`UpdateGlobalDefaults` saves the raw int32 value directly to `cfg.AutonomousMaxTurns` (following the same pattern as every other field, e.g. `MaxAutoReworkIterations` at line 150). There is no clamp in the handler itself. The clamp to `[1, 200]` happens inside `AutonomousMaxTurnsOrDefault()` (`config/config.go:1084-1091`) every time the value is read.

Consequence: a value of 500 passed via the RPC will be written as 500 to `config.json`, but `AutonomousMaxTurnsOrDefault()` returns 200. `sessionDefaultsToProto` already calls `AutonomousMaxTurnsOrDefault()` (see how it calls `MaxAutoReworkIterationsOrDefault()` at line 549 as the pattern), so the response to `UpdateGlobalDefaults` will echo 200, not 500. This is the existing project-wide convention — it is intentional, not a bug to fix. Do not add an explicit clamp or error return in the handler.

## 6. Generated file policy — `gen/` and `web-app/src/gen/` are gitignored; ConnectRPC stubs are too

Both the Go ConnectRPC stubs (`gen/proto/go/session/v1/session.pb.go`, `*_grpc.pb.go`, `*connect.go`) and the TypeScript stubs (`web-app/src/gen/…`) are generated by `buf generate proto` and are covered by the `.gitignore` entries `gen/` and `web-app/src/gen/` (`.gitignore:33-35`). This is the same policy as the ent ORM code. Do not commit them; do not force-add them.

After adding the new proto fields, the workflow is:
1. Edit `proto/session/v1/session.proto` (commit this).
2. Run `make proto-gen` to regenerate stubs (do not commit the output).
3. Wire the new field in `defaults_service.go` and `GlobalDefaultsForm.tsx`, both of which import the generated types at runtime — the new `AutonomousMaxTurns` field will be available in `*sessionv1.SessionDefaultsConfig` and `*sessionv1.UpdateGlobalDefaultsRequest` after step 2.

## 7. `sampleDefaults` fixture in `GlobalDefaultsForm.test.tsx` — must include new field

The Jest test file (`GlobalDefaultsForm.test.tsx:32-43`) defines `sampleDefaults` as a plain object with explicit fields. If `autonomousMaxTurns` is omitted from this fixture, tests that call `expect(mockUpdateGlobalDefaults).toHaveBeenCalledWith(expect.objectContaining({...}))` will still pass (because `objectContaining` is used), but the "renders the loaded value" pattern tests (like line 63-69 for `maxAutoReworkIterations`) will need a new test case for `autonomousMaxTurns` that sets it in the fixture and asserts the field is both displayed and round-tripped through the save payload.
