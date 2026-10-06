# Architecture Review: session-yolo-mode
**Date**: 2026-08-06
**Verdict**: BLOCKED

## Constitution Violations
N/A — no constitution file in this repo (checked `docs/adr/ADR-000-architecture-constitution.md`, does not exist).

## Blockers

- [ ] **Task 2.2.1 (`buildLaunchCommand`, `session/instance_tmux.go`)** — The auto-approve flag is injected in the wrong place for Claude and will be silently swallowed as inert prompt text for the most common creation path, not activated as a real CLI flag — remediate by moving the Claude-side injection inside `buildClaudeCommand`, before the `Prompt`/`--` block.

  `buildClaudeCommand` (lines 133–166, already read in full) appends `-- <prompt>` as its **last** element whenever `i.Prompt != "" && (claudeSessionID == "" || i.OneShot)` — i.e. on essentially every newly-created session that has an initial prompt (`claudeSessionID == ""`) and every one-shot/headless run. The codebase's own doc comment on that line explains why: `"--"` tells Claude's CLI parser to stop treating **anything after it** as a flag, specifically so a prompt that itself begins with `--` isn't misparsed. That's the standard, universal semantics of `--` in getopt/yargs/commander-style parsers — it doesn't just protect the immediately-following token, it demotes every subsequent argv token to positional.

  Task 2.2.1 appends the yolo flag in `buildLaunchCommand`, **after** `buildClaudeCommand` has already returned the full string including the trailing `-- <prompt>`:
  ```go
  switch p := classifyProgram(i.Program).(type) {
  case claudeProgram:
      cmd = i.buildClaudeCommand(p.base, claudeSessionID)   // already ends in `-- <prompt>`
  ...
  }
  if i.AutoApprove {
      if flag := yoloFlagFor(i.Program); flag != "" {
          cmd = cmd + " " + flag   // appended AFTER the `--` separator
      }
  }
  ```
  For a new Claude session created with an initial prompt, the resulting command is effectively `claude ... -- '<prompt>' --dangerously-skip-permissions`. Claude's parser has already seen `--` and will treat `--dangerously-skip-permissions` as a second positional argument, not as the bypass flag — so the flag never takes effect and Claude will still show permission prompts. Meanwhile the session-card badge (Task 5.2.1) reads the raw `session.autoApprove` boolean for `Active` sessions (the `hasPendingAutoApproveChange` literal-presence check is scoped to `Paused`/`Stopped` only) and will unconditionally show "⚡ Auto — Skipping ALL permission prompts", so the UI actively lies about the session's real behavior for exactly the case this feature exists to serve.

  This does **not** affect Aider: `plainProgram` returns `i.Program` unmodified with no `--`/positional-prompt handling in this function at all, so appending the flag afterward is safe there. It is a Claude-only, prompt-only defect — but that is the dominant creation path (Omnibar sessions almost always carry an initial prompt).

  **Compounding gap**: Task 2.3.1's own acceptance criteria ("the resulting command contains `--dangerously-skip-permissions` exactly once") is a substring check with no `i.Prompt` set in the scenario, so it would pass even with this bug present — the planned test suite would ship this broken and green.

  **Remediation**: give Claude its own injection point, inside `buildClaudeCommand`, positioned with the other flags and before the `Prompt`/`--` append (e.g. immediately after the existing `AutoYes` block at instance_tmux.go:154-156):
  ```go
  if i.AutoYes {
      parts = append(parts, "--permission-mode", PermissionModeBypassPermissions)
  }
  if i.AutoApprove {
      if flag := yoloFlagFor(base); flag != "" {
          parts = append(parts, flag)
      }
  }
  if i.OneShot { ... }
  if i.Prompt != "" && ... { parts = append(parts, "--", i.promptArg()) }
  ```
  Keep the generic post-switch append in `buildLaunchCommand` only for the `plainProgram` (Aider, etc.) branch, where no positional-prompt separator exists to fight with. Also strengthen Task 2.3.1's acceptance criteria to assert flag-before-`--`-separator ordering (or run the test with `i.Prompt` set), not just string containment, so this class of bug is actually caught next time.

## Concerns

- [ ] **Task 3.2.1 (`SetAutoApprove`, `session/instance_actor_setters.go`)** — Restart-on-change is not serialized against concurrent restarts the way its own stated model (`SwitchProgram`) is, reopening the exact race `SwitchProgram`'s `programSwitchMu` was built to close.

  The Pattern Decisions table explicitly says `SetAutoApprove` "follows `SwitchProgram`'s restart-on-Active-change pattern," and `SwitchProgram`'s doc comment (`session/instance_program.go:32-37`) is explicit about *why* it holds `i.programSwitchMu` for the whole set→persist→restart sequence: "so a manual program-switch request and an automatic capacity-monitor fallback firing near-simultaneously serialize instead of double-restarting." `Instance.Restart` (`session/instance.go:1509`) itself has no internal lock — it directly calls `i.StopController()` / `i.KillSession()` / recreates the tmux session, and relies entirely on its callers to serialize concurrent invocations.

  The planned `SetAutoApprove` reads `i.Status` and calls `i.Restart(true)` **outside** any dedicated mutex — only the field write goes through `sendSyncErr`. A `SwitchProgram` call and a `SetAutoApprove` call (or two concurrent `UpdateSession` calls touching different fields) racing on the same instance can both observe `Status == Active` and both call `Restart`, colliding on `KillSession`/tmux-session-recreate with no coordination between the two mutex domains (`programSwitchMu` vs. none). Low likelihood in a single-user tool, but it is a real regression relative to the very pattern the plan cites as its model, and a bad tmux session gets corrupted/duplicated quietly rather than loudly.

  **Remediation**: either reuse (and rename, since it now covers more than program switches) `programSwitchMu` around `SetAutoApprove`'s persist+restart sequence, or move the mutex into `Restart` itself so every restart-triggering setter is serialized by construction instead of by convention at each call site.

- [ ] **Story 3.1/3.3 (`CreateSession`/`UpdateSession`, `server/services/session_service.go`)** — `auto_approve=true` combined with an unsupported agent is a representable-but-invalid state at the RPC boundary; only the Omnibar UI enforces the invariant client-side.

  `AutoApproveSupported(program)` exists specifically so the UI can disable the checkbox for e.g. `codex` (Task 4.1.2), but Story 3.1's create-path wiring threads `req.Msg.AutoApprove` straight into `InstanceOptions` with no corresponding server-side check, and Story 3.3's update-path wiring has the same gap. Any RPC caller that isn't the Omnibar form — a future MCP `create_session` schema change (flagged as an open question in the plan itself), a script, `curl` against the ConnectRPC endpoint — can set `auto_approve=true` on an unsupported program. `yoloFlagFor` silently no-ops (by design, per Task 2.1.1), so the flag is durably persisted and the badge renders "⚡ Auto" for a session that was never actually made unguarded. This is the parse-at-boundary gap Lens 2.7 asks about: the invariant "`auto_approve` implies a supported agent" is real domain logic, enforced today only as client-side UX, not as a server-side guarantee.

  **Remediation**: in `CreateSession`/`UpdateSession`, when `AutoApprove` is being set true, check `session.AutoApproveSupported(resolvedProgram)` and either reject with `connect.CodeInvalidArgument` (consistent with this repo's existing validation-guard style, e.g. the path-required guard in `.claude/rules/session-creation-registry.md`) or clear the flag with a logged warning — pick one and document it in the plan rather than leaving it purely client-enforced.

## Nitpicks
- `AUTO_APPROVE_SUPPORTED_AGENTS` (frontend, `OmnibarCreationPanel.tsx`) duplicates `yoloFlagByAgent`'s key set (backend, `instance_tmux.go`) as a second, manually-synced list. The plan already acknowledges this ("keep in sync manually — small enough surface that a shared RPC isn't warranted"), which is a reasonable proportionality call for two agents — but there's no test tying the two lists together, so a future third agent added to one and not the other fails silently (UI enables a checkbox for an agent the backend can't actually inject a flag for, or vice versa). A one-line Go test asserting `len(yoloFlagByAgent) == 2` with named keys would at least make a drift show up as a diff-review prompt rather than a silent runtime mismatch.
- Task 5.1.1 pre-emptively notes the `vars.color.warningBg`/`warningText`/`warning` tokens need verifying before use — confirmed they already exist in `web-app/src/styles/theme.css.ts` (all five theme variants define them), so this is a non-issue; downgraded from a flagged risk to informational only.
