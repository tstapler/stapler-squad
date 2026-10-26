# ADR-031: Large launch prompts are delivered by a typed `PromptDelivery`, not by size checks

Status: Accepted — 2026-10-25

## Context

`Instance.Prompt` is handed to the agent as an argument at process start. Today only the claude launch builder
consumes it (`appendClaudeOutputAndPromptFlags`, `session/instance_tmux.go`); pi, opencode, agy, gemini and
unrecognised programs ignore `Prompt` at launch. `promptArg()` picks between two forms with one ad-hoc size check:

- under `maxInlinePromptBytes` (4 KiB): `launchcmd.Literal`;
- otherwise `launchcmd.FileContents(path)`, rendered as `"$(cat 'path')"` for a shell and read by
  `ExecCommand.Args` for a direct exec.

Both forms end as one argv entry. Linux caps a single entry at `MAX_ARG_STRLEN` (128 KiB, `launchcmd.MaxArgBytes`):

- tymux and native: `ExecCommand.Args` returns `ErrArgTooLarge`, surfaced by `session/tymux/launch.go` as the Start error
  `tymux: resolve launch arguments`.
- tmux: the shell's `exec claude "$(cat file)"` fails with `E2BIG`.

A prompt of 128 KiB or more therefore cannot start a session. Interactive `claude` has no stdin-prompt mode (stdin
only feeds `-p`).

## Options

**a. Launch with no prompt, then paste it after startup.** Wait for `detection.StatusIdle` and write the prompt as a
chunked bracketed paste. Rejected for now: the repo has no bracketed-paste writer on any backend (`grep -rn BracketedPaste`
finds nothing; `SendPromptWithEnter` is `SendKeys` plus Enter). It would need a new write primitive on the tmux, native
and tymux backends, an idle-detection race on every launch, and re-delivery logic on restart/relaunch, which the
argv forms get for free because `currentLaunchCommand` rebuilds them. The existing post-launch path
(`InitialPrompt` via the session driver) also collapses newlines and truncates at 4096 bytes
(`sanitizeInitialPromptForTmux`), so it cannot be reused for this.

**b. Write the prompt to a file and pass a short instruction argument.** Works on every backend because the argv stays
small, and survives relaunch. Cost: the agent must read the file with its own tools, which is outside the worktree, so
claude needs `--add-dir <prompt-cache dir>` to read it without a permission prompt (flag present in `claude --help`).
Chosen for interactive claude.

**c. `-p` with stdin.** `claude -p` reads its prompt from stdin when no positional prompt is given.
`ExecCommand.StdinFile` renders as `< file` for tmux/native and as `stdin_path` for tymuxd when the
`create_session.stdin_file` capability is advertised (`session/tymux/launch.go`). Chosen for one-shot (`-p`) claude.
It is not available interactively.

## Decision

Add `PromptDelivery`, a closed enum, and one pure chooser, `choosePromptDelivery(channel, mode, size)`, in
`session/prompt_delivery.go`. Callers switch on the result; no other code compares prompt size.

| Channel (agent kind) | Mode | Size | Delivery |
|---|---|---|---|
| any | any | 0 | `PromptNone` |
| non-claude kinds (pi, opencode, agy, gemini, unrecognised) | any | any | `PromptNone` (unchanged: these builders never took a launch prompt) |
| claude | any | < 4 KiB | `PromptInline` (`Literal`, unchanged) |
| claude | any | 4 KiB .. < 128 KiB | `PromptFileBacked` (`FileContents`, unchanged) |
| claude | interactive | >= 128 KiB | `PromptReferenceFile` (instruction argv + `--add-dir`) |
| claude | one-shot (`-p`) | >= 128 KiB | `PromptStdin` (`StdinFile`) |

Backend is not an input: all three backends consume the same `ExecCommand`, and the choice differs by mode only.

Large-prompt files are named from a hash of the session id and prompt, so repeated `launchCommand` calls (tmux session
construction, `currentLaunchCommand` on restore) reuse one file rather than writing a new one each time. They are
removed by `Instance.cleanupPromptFile` at Destroy and, unlike `FileContents` files, are not on the 30 s timer, since a
relaunch needs them again.

## Consequences

- Nothing is truncated. If the large-prompt file cannot be written, the launch falls back to `Literal` and fails loudly
  (`ErrArgTooLarge` / tmux "command too long") instead of dropping or cutting the prompt.
- Reference-file delivery depends on the agent reading a file; a very large prompt is read in several `Read` calls.
  That is weaker than a paste for prompts that exceed the agent's context anyway.
- `--add-dir` exposes the whole prompt-cache directory (other sessions' prompt files) to that session. Same user and
  host; not isolated further.
- Deliberately left: other agent kinds still receive no launch prompt.

### Addendum: the live `InitialPrompt` driver path

`sanitizeInitialPromptForTmux` used to cut the prompt typed into a running pane at 4096 bytes (an arbitrary constant,
not a tmux or PTY limit: the write goes to the PTY master, which has no argv limit) and collapsed newlines to spaces,
both silently. It is replaced by `ParseDriverPrompt` (`session/driver_prompt.go`), which has no size cap:

- Text with a control byte (newline, tab, ESC) is wrapped in a bracketed paste so it arrives as text, not keystrokes.
  Plain printable text is typed as before.
- `SubmitDriverContent` writes in `maxInlinePromptBytes` chunks (this ADR's inline class), reporting bytes written on a
  failure. There is no second threshold set, and no size at which a live prompt is refused.
- Text containing the paste terminator `ESC[201~` is refused with `ErrPromptUndeliverable`; the driver logs an error,
  adds the session to the review queue and stops retrying.
