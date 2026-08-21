# Architecture Review: Launcher Presets
**Date**: 2026-08-06
**Verdict**: CONCERNS

No `docs/adr/ADR-000-architecture-constitution.md` exists in this repo (checked
`docs/adr/` — highest-numbered files are `ADR-026-mergeability-state-synthesis.md`
and topic-numbered ADRs like `020-alias-at-trigger-character.md`; no `000` file
present). No constitution-based blockers apply.

## Constitution Violations

None — no constitution document exists to violate.

## Blockers

None found. The core structural decision (ADR-001: separate preset file/RPC/UI,
shared `extra_args` argv carrier reusing the existing `shellQuote` primitive) is
sound and is applied consistently across every task in plan.md — no task quietly
reaches back into `AliasConfig`/`ProfileDefaults` storage or RPCs to implement
presets. The `extra_args` → `buildLaunchCommand` path correctly reuses
`shellQuote` per-element (verified against `session/instance_tmux.go:105-124`)
rather than resurrecting the `strings.Fields` corruption bug it exists to fix.

## Concerns

- [ ] **Task 1.2.1b / Task 5.1.1b — `LauncherPresetProto.program` can disagree with `argv[0]`, and the UI is specced to prefer displaying the disagreeing value.** ADR-001 states "the preset's optional `program` field is presentation-only... this keeps 'what actually launches' unambiguous (always `argv`, never a value that could disagree with it)" — but `validateLauncherPresets` (Task 1.1.1c) never checks `Program == Argv[0]` when both are present, so a hand-edited file with `"program": "aider", "argv": ["claude", "--flag"]` loads successfully. Task 5.1.1b then specs the preset row to render "`argv.join(" ")` (or `program` if present, falling back to `argv[0]`)" — i.e. it *prefers* the potentially-wrong `program` value as the "what will launch" preview. This directly undermines Success Criterion 2's intent ("pre-fills the form... so the user can review before submitting") since the reviewed preview can misstate the actual launched program. **Remediation**: either drop the optional `program` field entirely (the UI can always derive a display string from `argv[0]`/`argv.join(" ")`, so the field adds no information `argv` doesn't already carry) or add a load-time check that rejects the file when `program != "" && program != argv[0]`, consistent with this feature's existing "reject the whole file loudly" model (Story 1.1.1).

- [ ] **Task 1.2.1c — `GetLauncherPresetsResponse{presets, load_error}` allows an illegal state at the type level.** The two fields are documented as mutually exclusive by convention ("presets is empty in that case... Empty load_error + empty presets means 'no file/no presets'") but nothing in the proto schema enforces it — a future handler change could populate both non-empty fields (or leave both ambiguous) and no caller would get a compile-time or parse-time signal. This is exactly the "parse, don't validate" gap the type-driven-design lens flags: a `oneof` (e.g. `oneof result { PresetList success = 1; string load_error = 2; }`) would make "success XOR error" structural rather than a comment-documented handler discipline. **Remediation**: model the response as a `oneof` (or a nested `Result`-shaped message) in Task 1.2.1c before `make proto-gen` is run, since widening a flat two-field response into a `oneof` later is a breaking wire-format change for any external caller, while getting it right now is free.

- [ ] **Epic 2.3 / Story 2.3.1 — `extra_args` composing with profile/alias-resolved `cli_flags` has no test and is documented only as an absence of interaction, not a designed one.** Per `server/services/session_service.go:1373-1436` (verified directly), `instanceCLIFlags` is resolved from the selected profile/alias *unconditionally*, independent of whether `req.Msg.Program` was overridden by a preset. Since preset selection does not clear the `profile`/`alias` fields (ADR-001 Decision 1: presets "compose with `working_dir`... does not touch `branch` or `profile` at all"), a user who has a profile selected and then picks a preset gets a launch command combining the profile's `cli_flags` (from `resolved.CLIFlags`) *and* the preset's `extra_args`, in that order (`buildLaunchCommand`'s `CLIFlags` loop runs before the new `ExtraArgs` loop — Task 2.2.1a). Task 2.3.1a's own note ("`extra_args` has no defaults-resolution concept; it is a direct passthrough") describes this as if the two carriers don't interact, but they visibly compose into a single command string at launch. Story 2.3.1's only AC exercises `ExtraArgs` in isolation (no profile/alias set). **Remediation**: add one integration test asserting the intended composed order (profile/alias `cli_flags` first, preset `extra_args` last) when both a profile and a preset are in play, and promote the current implicit behavior to an explicit, documented design decision (a one-line addition to the Domain Glossary or Pattern Decisions table) rather than an emergent property of two independently-correct code paths.

## Nitpicks

- Story 1.1.1's "missing file" AC text ("returns `(nil, os.ErrNotExist-wrapping-err)`") and Task 1.1.1b's implementation note ("return `(nil, err)` unwrapped... do NOT wrap") read as contradictory guidance for the same case. Both satisfy `os.IsNotExist(err)`/`errors.Is(err, os.ErrNotExist)` in practice, but reconcile the wording so the implementing engineer isn't left guessing which is authoritative.
- Pattern Decisions row 5 cites "GoF Command (a discrete, idempotent action)" for `handlePresetSelect` (Task 4.2.1a), but the implementation is a plain `useCallback` closure calling `setFormField` three times — no `Command` object, invocation queue, or undo/redo semantics is actually introduced. The behavior described (discrete, unconditional overwrite) is correct and well-justified; the GoF citation just overstates what pattern was applied. Trim or reword to avoid implying a `Command` abstraction exists.
- Dependency graph edge `P1_2 --> P2_1` (proto changes gating `Instance`/`InstanceOptions.ExtraArgs`) is unnecessary: `session.Instance`'s new field (Task 2.1.1a/b) is a plain Go struct addition with no dependency on the generated proto types — only Phase 2.3 (`CreateSession` folding `req.Msg.ExtraArgs`) actually needs Phase 1.2 to land first. Doesn't block anything since the suggested order is still safe, just imprecise.
- The flagship remote-exec example (`ssh -t host '...'`) still requires the user to pick a valid local directory under the unchanged `directory`/`new_worktree` session-type path-required guard (`server/services/session_service.go:1268-1269`), since ADR-001 correctly rules out a new `SessionType`. This is already an accepted tradeoff from requirements.md's Open Question 2 ("Assume local tmux pane running the argv as-is... unless research finds otherwise"), so it's not a new architectural gap — just worth one explicit sentence in the plan (e.g., recommending `one_off` as the natural session type for path-irrelevant remote presets) since it's the exact use case named in the Problem Statement.
- The forwarding-only `LauncherPresetsService` → `SessionService.GetLauncherPresets` delegate (Task 1.3.1b) is Interface-Pollution-Checklist Smell #4 in isolation, but it exactly mirrors the existing `defaultsSvc` delegate pattern (`server/services/session_service.go:3363-3425`), which exists because ConnectRPC requires one generated service interface implemented by one struct. Not a new violation — noted only so a reviewer doesn't flag it as novel.
