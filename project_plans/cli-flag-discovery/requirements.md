# CLI Flag Discovery & Binary Verification

## Problem
Program Config (`web-app/src/components/settings/ProgramsManager.tsx`) and session creation accept a free-text command and CLI flags with no validation. Typos in the binary or flags only surface when the tmux session fails.

## Goals
- Verify a program's binary exists (`exec.LookPath`) and report resolved path.
- Parse `<binary> --help` into a flag list; use it for autocomplete, unknown-flag warnings, and tooltips.

## Acceptance criteria
1. New RPC `ProbeProgram(command)` returns `found`, `resolved_path`, and parsed flags (name, short, takes_value, description).
2. Binary check uses `exec.LookPath` (first token of command, `~`-expanded); missing binary returns `found=false`, not an RPC error.
3. `--help` runs with a hard timeout (3s), stdin closed, output size cap (256KB), no shell; results cached per resolved path + mtime.
4. Parser handles GNU/argparse/cobra/clap style (`-x, --long <val>  desc`); unparseable output yields empty flags, never an error.
5. Program Config form shows "binary found / not found" indicator on command blur.
6. CLI flags input offers autocomplete from discovered flags and warns (non-blocking) on unknown flags; each flag shows a description tooltip.
7. Session creation UI shows the same missing-binary warning and flag validation for the selected program.
8. Security: probing only allowed for the command the user is configuring; executed with sanitized env, only `--help`; unit tests cover timeout and oversized output.
9. Works on mobile (tap-to-show tooltip, no hover dependency).
10. Go unit tests (parser fixtures for claude/aider/git-style help), jest tests for the component.

## Out of scope
Subcommand flag discovery, man page parsing, flag value completion.
