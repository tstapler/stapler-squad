# BUG-047: `WriteToSession` (Web UI Chat + `write_to_session`/`run_command` MCP Tools) Appends `\n` Instead of `\r`, So Messages Sent to a Live Session Are Written But Never Submitted [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-24)
**Discovered**: 2026-07-24, investigating why backlog item `12981e9d` ("Unfinished page needs CSS work for sizing", PR #210) had sat at `status=review` for 2.5+ hours despite BUG-039/041/043/044/045/046 all being deployed and confirmed working.
**Fixed**: 2026-07-24 — `session/instance_tmux.go`, `server/services/terminal_service.go`, `server/mcp/tools_terminal.go`
**Impact**: Any text sent to a live Claude Code session via the web UI's session chat box (`WriteToSession` RPC) or the `write_to_session`/`run_command` MCP tools was written into the pane's PTY but **never actually submitted** to the underlying program — it lands as unsubmitted (or silently-inserted-as-multiline) text sitting in the input box forever, with **no error surfaced to the sender**. The sender sees their message appear to "type" into the pane and has no way to know it was never processed. This affects every session, not just backlog items — any human or agent using the web UI's chat box to talk to a running session, or any MCP client calling `write_to_session`/`run_command`, was silently affected whenever the target was a raw-mode TUI (the Claude Code CLI itself) rather than a plain cooked-mode shell prompt.

## Live Evidence

Item `12981e9d`'s work session (tmux `staplersquad_stapler-squad-fix-unfinished-page-mobile-sizing-r4`, still alive) had already done everything right: completed all acceptance criteria, gotten a review PASS, run `/github:pr-ship 210`, and reached a fully green, `mergeStateStatus: CLEAN`, `mergeable: MERGEABLE` PR with the ship gate printing:

```
Merge with: gh pr merge 210 --squash --delete-branch --repo tstapler/stapler-squad
```

The pane's scrollback then showed a final line with **no agent response following it** — unlike every other turn in the same transcript, which all show an `●` reply:

```
❯ merge it
```

Process inspection confirmed the session was genuinely idle, not crashed or hung on a lock: `ps -o pid,ppid,stat,etime,pcpu` showed the `claude` process (PID 2232079) had been running 2h33m with only **1.9% total CPU** — essentially zero activity since printing the ship-gate summary. `tmux capture-pane -S -2000` (full scrollback) confirmed `merge it` was the very last thing rendered, sitting in the input box, never processed.

## Root Cause (confirmed)

Two different "press Enter" conventions coexist in this codebase, and three of six call sites picked the wrong one:

**Confirmed working** (`\r`, carriage return, `0x0D`):
- `session/tmux/tmux.go`'s `TapEnter()` — writes the raw byte `0x0D` directly to the PTY.
- `session/session_driver.go:327` — the initial task-prompt send: `inst.SendKeys(initialPrompt + "\r")`.
- `session/session_driver.go:489` — the backlog nudge send: `inst.SendKeys(nudge + "\r")`.
- `session/autonomous_driver.go:249-257` — the autonomous turn-injection submit keystroke, with an existing comment (pre-dating this fix, referencing BUG-031) that already documented the exact failure mode: *"Use SendKeys (raw PTY write) instead of SendCommandImmediate so that only `"\r"` is sent. SendCommandImmediate goes through the command executor which appends `"\n"`, producing `"\r\n"`. In Claude Code's TUI input, `"\r\n"` inserts text into the multiline buffer without submitting."*

**Broken** (`\n`, line feed, `0x0A`) — before this fix:
- `server/services/terminal_service.go`'s `WriteToSession` (the ConnectRPC handler backing the web UI's session chat box): `if req.Msg.PressEnter { text += "\n" }`.
- `server/mcp/tools_terminal.go`'s `writeToSession` (the `write_to_session` MCP tool): `if pressEnter { text += "\n" }`.
- `server/mcp/tools_terminal.go`'s `runCommand` (the `run_command` MCP tool): `inst.SendKeys(command + "\n")`.

In a raw-mode PTY (which interactive TUIs like the Claude Code CLI's Ink-based interface use — no kernel-level `ICANON`/`ICRNL` line processing), a physical Enter key sends `\r`, and the application's own input parser recognizes `\r` as submit. A bare `\n` is not the terminal's actual Enter byte in raw mode — depending on the receiving app's multiline handling, it's either ignored or (as the `autonomous_driver.go` comment already documented for the `\r\n` case) inserted as a literal line into the multi-line input buffer without submitting. A plain cooked-mode shell prompt masks this (canonical-mode line discipline treats `\n` as the buffer terminator regardless of source), which is likely why `run_command`'s bug went unnoticed for shell-target use — but any raw-mode TUI target, most importantly the Claude Code CLI itself, silently swallowed the Enter.

Three of six call sites had independently hand-rolled their own "append Enter" logic, with no shared, tested definition of what "Enter" means for a SendKeys call — exactly the drift this class of bug thrives on.

## Fix Applied

Added a single, documented, exported source of truth in `session/instance_tmux.go`, next to `SendKeys`:

```go
// EnterKeySequence is the byte sequence that submits a line of input to an
// interactive terminal session, matching what a real terminal sends for a
// physical Enter keypress in raw/cbreak mode (TapEnter, elsewhere in this
// package, writes the same 0x0D byte directly to the PTY). ...
const EnterKeySequence = "\r"

// BuildSubmittableInput appends EnterKeySequence to input when pressEnter is
// true, producing the exact string that must be handed to SendKeys ...
func BuildSubmittableInput(input string, pressEnter bool) string {
	if pressEnter {
		return input + EnterKeySequence
	}
	return input
}
```

Updated all three broken call sites to use it:
- `server/services/terminal_service.go`: `text := session.BuildSubmittableInput(req.Msg.Input, req.Msg.PressEnter)`
- `server/mcp/tools_terminal.go` (`writeToSession`): `text := session.BuildSubmittableInput(input, pressEnter)`
- `server/mcp/tools_terminal.go` (`runCommand`): `inst.SendKeys(session.BuildSubmittableInput(command, true))`

Also switched the three already-correct call sites (`session/session_driver.go` ×2, `session/autonomous_driver.go`) from their own literal `"\r"` to the shared `EnterKeySequence` constant, so there is exactly one definition of "Enter" in the codebase going forward instead of six independent ones.

## Files Affected

- `session/instance_tmux.go` — new `EnterKeySequence` constant + `BuildSubmittableInput` helper
- `session/instance_tmux_test.go` — new regression test
- `session/session_driver.go` — two call sites switched to the shared constant
- `session/autonomous_driver.go` — one call site switched to the shared constant
- `server/services/terminal_service.go` — `WriteToSession` fixed to use `BuildSubmittableInput`
- `server/mcp/tools_terminal.go` — `writeToSession` and `runCommand` fixed to use `BuildSubmittableInput`

## Verification

- `TestBuildSubmittableInput_UsesCarriageReturnNotNewline` (`session/instance_tmux_test.go`) — asserts `BuildSubmittableInput("merge it", true) == "merge it\r"`, that `pressEnter=false` leaves input untouched, and that `EnterKeySequence == "\r"`.
- **Verified to fail against pre-fix behavior**: reproduced the old `input + "\n"` logic standalone — `BuildSubmittableInputOld("merge it", true)` produces `"merge it\n"`, which fails the test's `"merge it\r"` assertion (`match=false`), confirming the test would have caught this bug before the fix.
- `go build ./session/... ./server/services/... ./server/mcp/...` — clean.
- `go test ./session ./server/services/... ./server/mcp/...` — full suites green, no regressions.
- `golangci-lint run ./session/... ./server/services/... ./server/mcp/...` — 0 issues.

## Immediate Unblock for Item `12981e9d` / PR #210

This fix does not retroactively submit the already-typed, unsubmitted `merge it` sitting in the live pane — that text is still there. The pane is alive and idle (not crashed), so sending one more properly-terminated message (or a bare `TapEnter()`/Ctrl sequence) to that specific session will complete the already-approved merge. Left for the coordinator/human to action directly rather than done as part of this investigation, per the investigation's instruction not to force live state changes.

## Reflection

**Classification**: A "silent no-op with no error surfaced" bug — the RPC/tool call succeeded (`SendKeys` returned no error; the bytes were genuinely written to the PTY), so there was nothing for the caller to catch or retry on. The failure is entirely in the *semantic* gap between "bytes reached the PTY" and "the receiving program treated it as a submitted line" — a distinction only visible by knowing the receiving program's raw-mode input handling, which the calling code has no way to introspect.

**Earliest achievable enforcement**: The regression test added here (`TestBuildSubmittableInput_UsesCarriageReturnNotNewline`) is a pure-function unit test with no PTY/mocking required — the earliest practical level, now that the "append Enter" logic is centralized into one testable function instead of six independent call sites. Before this fix, no test could have caught this without a real (or carefully faked) raw-mode PTY target, since a plain shell-backed fake would have masked the bug exactly as cooked-mode shells do in production.

**Recurring shape**: A close sibling of BUG-031 (referenced directly in the pre-existing `autonomous_driver.go` comment this fix's centralization now also protects) — both are instances of "this codebase's terminal-submission convention is non-obvious and not enforced by the type system, so each new call site has a real chance of re-deriving it wrong." Centralizing into `EnterKeySequence`/`BuildSubmittableInput` is the structural fix for the class, not just this instance — any future SendKeys-with-enter call site now has an obvious, tested helper to reach for instead of hand-rolling a terminator.

## Related

- `session/autonomous_driver.go`'s pre-existing comment already names this exact failure mode from an earlier incident (BUG-031), but that fix apparently addressed only the autonomous-driver call site, not the general pattern — this bug is the same defect surfacing again at three different, independently-written call sites.
- Discovered while investigating why backlog item `12981e9d` / PR #210 was not converging; see the investigation's companion finding, BUG-048 (open), for a second, independent contributing factor found during the same investigation.
