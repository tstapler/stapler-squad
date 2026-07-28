# Requirements: install-service.sh launchd/systemd shell sourcing

**Complexity: 1** (quick task) — the code fix already shipped in commit
`baca1c7c` and is verified present on this branch (see "Verification" below).
The only remaining work is: (a) durably documenting one already-known,
already-scoped deferred gap, and (b) two backlog-administration criteria
blocked by a missing MCP tool capability (see "What is actually new work").
There is no new design surface, no library/stack decision, and no
user-facing surface to research.

Source: backlog item `10128af0-e1eb-47bc-9016-3af8fde83b4d`, duplicate of canonical
item `1dc7ff10-326c-4276-a70f-eb8869713593`. Derived directly from the item
description and acceptance criteria — no interactive ideation interview (none
possible in this pipeline).

## Background

`scripts/install-service.sh` used to source the user's `~/.zshrc` inside the
generated launchd plist / systemd unit. `.zshrc` is written for interactive
shells and can hang, prompt, or emit `stty` errors when run headless by
launchd/systemd, causing non-deterministic service startup.

**This was already fixed on `main` in commit `baca1c7c` ("fix: address
Copilot review feedback"), before this backlog item's work session started.**
That commit replaced the shell-wrapper invocation with a direct
`Program`/`ProgramArguments` (macOS) / `ExecStart` (Linux) invocation of the
binary, with `PATH` supplied explicitly via `EnvironmentVariables` /
`Environment=`. This worktree's branch is at the same commit as `main`
(`4eca0ed34`), so the fix is already present here — confirmed by direct
inspection (see Verification below).

## Acceptance criteria (from the backlog item, indices as reported by
`get_backlog_item`)

0. `scripts/install-service.sh` contains no reference to `.zshrc`,
   `.zprofile`, or `/bin/zsh -c` in `install_macos()` or `install_linux()`.
1. Generated macOS LaunchAgent plist's `ProgramArguments` invokes the binary
   directly (no shell wrapper), `PATH` via `EnvironmentVariables`.
2. Generated Linux systemd user unit's `ExecStart` invokes the binary
   directly, `Environment=PATH=...` set, no shell rc file sourced.
3. This item (`10128af0`) is archived via an `ArchiveBacklogItem` RPC, with
   notes recording it as a duplicate of canonical item `1dc7ff10`, pointing to
   landing commit `baca1c7c` and the verification date/command.
4. Canonical item `1dc7ff10`'s notes are cross-linked (appended) to reference
   this duplicate's archival.
5. The residual `ANTHROPIC_API_KEY`/`GITHUB_TOKEN` env-inheritance gap
   (introduced incidentally by `baca1c7c` removing `.zshrc` sourcing — that
   sourcing was also the only path through which shell-rc-exported secrets
   reached the service process) is explicitly documented as
   deferred-pending-maintainer-decision, not silently declared resolved.

## Verification of criteria 0–2 in this worktree

```
$ grep -n "zshrc\|zprofile\|/bin/zsh -c" scripts/install-service.sh
(no matches)
$ grep -n "ProgramArguments\|EnvironmentVariables\|ExecStart\|Environment=" scripts/install-service.sh
102:ExecStart=$bin_path --remote-access$extra_flags
109:Environment=HOME=$HOME
110:Environment=PATH=$PATH
179:    <key>ProgramArguments</key>
197:    <key>EnvironmentVariables</key>
```

`install_macos()` (lines 143–266) builds `Program`/`ProgramArguments` as
`$bin_path` directly, no `/bin/sh -c` or `/bin/zsh -c` wrapper.
`install_linux()` (lines 75–140) builds `ExecStart=$bin_path ...` directly.
Both satisfied — **no code change required for criteria 0–2.**

## What is actually new work in this session

- **Criterion 5**: the deferred-gap documentation that prior sessions wrote
  lives only in *untracked, uncommitted* `project_plans/` scratch directories
  in a sibling worktree checkout
  (`launchd-shell-sourcing/`, `launchd-env-file/`, `install-service-launchd-env/`
  under `/home/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad`)
  — none of that will ever ship in a PR. This session adds a durable,
  committed record: a new `BUG-006` entry in
  `docs/tasks/completed/system-service-autostart.md` (the existing "Known
  Issues" log for this script, alongside `BUG-001`..`BUG-005`), explicitly
  marked deferred/pending-maintainer-decision.

- **Criteria 3 & 4** require an `ArchiveBacklogItem` RPC and a notes-append
  capability. Neither is exposed by any MCP tool available to this session
  (checked via `ToolSearch` against the full `mcp__stapler-squad__*`
  surface — only `report_progress`, `request_review`, `report_pr_created`
  are available to the `work` role). A prior `review`-role session hit the
  identical gap and flagged it as `UNVERIFIABLE`/blocked. This is a tooling
  capability gap outside this session's control, not a design or code
  question — no amount of research/planning resolves it. These two criteria
  will be reported via `/backlog/fail-N` with this explanation, per the
  pipeline's explicit allowance to fail a criterion that "cannot be met as
  written."

## Non-goals

- Re-deriving the `.zshrc` removal fix — already shipped and verified.
- Implementing the `~/.stapler-squad/env` file or any other fix for the
  `ANTHROPIC_API_KEY` gap — criterion 5 only requires *documenting* the gap
  as deferred, not resolving it. Building a fix here would be scope creep the
  item's own acceptance criteria explicitly warn against ("rather than
  silently declared resolved" — the criterion is about honesty, not
  implementation).
