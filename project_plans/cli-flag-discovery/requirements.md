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

## Interpretation notes (added in SDD phase 4; the AC text above is unchanged and mirrors the backlog item)

These record how the plan (`implementation/plan.md`) reads the ACs where the literal text and the design differ. They are readings, not approvals: ADR-001 owner sign-off is **PENDING human review**.

- **AC8 ("only the command being configured")**: read as one command per request, taken from the form being edited or selected (Program Config field, or the session-creation picker's selected program), first token only, `--help` only, hardened env and cwd, and a request guard limiting callers to the local UI. The server does not keep a saved-program allow-list, so it will probe any single command the UI sends (ADR-001, Flagged Choice 1). Scripts (shebang or unrecognized file type) are not executed until the user explicitly clicks Check, and the session-creation picker never executes a program on selection (ADR-001, "Confirm before executing scripts").
- **AC2 (`exec.LookPath`)**: the plan uses a custom `lookInDirs` over the login-shell PATH plus the server PATH, and rejects relative and world-writable targets. This is intentional hardening and PATH-fidelity work on top of `LookPath` semantics, not a deviation from "does the binary exist".
- **AC3 ("no shell")**: applies to the `--help` execution. A constant script (no user text) derives the login-shell PATH so lookup matches how tmux sessions find programs. The 3s timeout stays the default unless the cold-start measurement in plan Task 1.1.3a contradicts it (then the plan makes the limit configurable).
- **AC6 ("tooltip")**: delivered as a tap-to-reveal inline description plus a disclosure button, with no hover dependency (also satisfies AC9). No Radix tooltip is used.
- **AC7 (session creation)**: scoped to the selected program's saved `cli_flags`. Alias `extraFlags` are not validated (they never reach the creation panel). This scope-down needs reviewer acceptance.
- **AC10 (fixtures)**: `git` is a negative fixture (no options table); `tmux` is a second negative; `gh`, `rg`, `uv`, `aider`, `claude`, `gemini` and `agy` are positive fixtures.
