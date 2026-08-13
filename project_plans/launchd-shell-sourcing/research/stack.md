# Stack Research: launchd/systemd shell sourcing (docs-only task)

## Scope note

This item's only remaining code-adjacent work is a **documentation entry**
(criterion 5) in `docs/tasks/completed/system-service-autostart.md`. There is
no code change to make. This research exists to (a) confirm the doc entry's
technical claims are accurate and (b) confirm no better-practice fix is being
silently skipped, so the "deferred" framing is honest.

## 1. `scripts/install-service.sh` conventions

- `#!/bin/sh` (POSIX sh, not bash/zsh) — `set -e` on line 20, no `set -u`/`set -o pipefail` (not POSIX).
- Color-coded logging helpers used throughout: `log_info`, `log_success`,
  `log_error` (defined ~line 29-32), each a one-line `printf` wrapper. Any
  new user-facing messaging in this script should reuse these, not raw `echo`.
- Structure: `install_linux()` (~line 75-140) and `install_macos()` (~line
  143-266) are the two platform branches; a `--uninstall` and flag-parsing
  path exists above them.
- Env vars already documented in the script's own header comment (lines
  15-18): `STAPLER_SQUAD_BIN` (binary override), `PROFILE_PORT`. No existing
  `STAPLER_SQUAD_ENV_FILE`-style var.

## 2. Env var mechanism per platform (confirms fix is correct/complete)

Both generated units set exactly two vars today — `HOME` and `PATH` — via the
standard, correct native mechanism for each platform:

- **systemd user unit** (`install_linux`, ~line 102-110):
  `ExecStart=$bin_path --remote-access$extra_flags`, then
  `Environment=HOME=$HOME` / `Environment=PATH=$PATH`. `Environment=` is the
  documented systemd.service directive for injecting env vars into the unit;
  this is correct usage, no wrapper needed.
- **macOS LaunchAgent plist** (`install_macos`, ~line 179-200):
  `<key>ProgramArguments</key>` invokes `$bin_path` directly (no `Program`
  shell string), and `<key>EnvironmentVariables</key>` is a `<dict>` with
  `HOME` and `PATH` entries. This is Apple's documented plist key for
  per-job env injection.

**Alternative mechanisms considered and correctly NOT used here:**
- `launchctl setenv` — sets vars in the **launchd global namespace** for
  *all* jobs bootstrapped by that launchd instance; it's session-wide state
  set imperatively (usually from a LoginHook or another launchd job), not a
  per-plist declarative mechanism. Wrong tool for "this one service needs
  these vars."
- systemd `environment.d` (`~/.config/environment.d/*.conf`) — only consumed
  by `systemd --user` **PAM-launched session managers**, not read by
  individual unit files. Also wrong layer for a single service's vars.
- `EnvironmentFile=` (systemd) — **this is the closest match to a real fix**
  for the deferred gap, not something already handled. It lets a unit source
  a `KEY=VALUE` file at start time without shell semantics (no `stty`, no
  prompts, no plugin hangs) — i.e., exactly the safe replacement for what
  `.zshrc` sourcing used to (accidentally) provide. macOS has no direct
  equivalent; `EnvironmentVariables` in the plist must be inlined at
  install-time, which would mean either (a) the install script reading a
  file and inlining its contents into the plist dict, or (b) a wrapper
  script the plist invokes that sources the file with `. "$file"` under
  `/bin/sh` (safe, since POSIX `.` has none of zsh's interactive-shell
  behavior) before exec-ing the binary.

This confirms the `EnvironmentFile=` / `~/.stapler-squad/env` direction
already sketched in `.backlog-context.md` (see below) is the right shape for
a *future* fix — but implementing it is explicitly a non-goal for this
session (see requirements.md "Non-goals").

## 3. Existing `~/.stapler-squad/env` convention — none shipped yet

No code or docs in this worktree implement a `~/.stapler-squad/env` file.
The **only** reference in the whole tree is a suggestion, not an
implementation, inside the ephemeral pipeline scratch file
`.backlog-context.md` (embedded backlog item body, "Fix Options" list):

> "Or support an optional `~/.stapler-squad/env` file that users can
> populate with just the env vars needed by the service."

`.backlog-context.md` is pipeline-generated/untracked scratch (rewritten on
every spawn), not a durable design doc — this is exactly why requirements.md
calls for committing a durable record instead. `~/.stapler-squad/` itself is
a real, established convention for this app's runtime state (see project
CLAUDE.md: `config.json`, `sessions.json`, `logs/`, `worktrees/` all live
there already), so `~/.stapler-squad/env` would be consistent with existing
placement conventions if/when someone implements it — but that's future
work, not something to build now.

## 4. Existing docs on this service's env var handling

`docs/tasks/completed/system-service-autostart.md` is the authoritative doc
for this script, with a "Known Issues — Proactive Bug Identification"
section (line 566) containing `BUG-001` through `BUG-005`, each following a
consistent format: `### BUG-NNN: <title> [SEVERITY: <level>]` followed by
**Description**, **Mitigation**, **Files Affected**, **Prevention**
subsections, separated by `---`.

Directly relevant precedent: **BUG-001** ("PATH in Service Environment
Missing User Tooling", line 568) already documents that the service's env is
deliberately minimal (`HOME`/`PATH` only) and that this can cause missing
tools like `tmux`. The new `BUG-006` entry for criterion 5 is a direct sequel
to BUG-001's theme — same root cause category (minimal service env vs. user
shell env), different symptom (missing secrets like `ANTHROPIC_API_KEY` /
`GITHUB_TOKEN` instead of missing PATH tooling). No ADR specifically covers
env var handling (searched `docs/adr/` for ADR-002 / autostart-related ADRs
referenced in BUG-001's mitigation text — no matching file found in this
worktree; the ADR reference in BUG-001 may be stale or the ADR may live only
in a different worktree/branch).

## Conclusions for the writer (Agent 3 / doc author)

- Follow the exact `### BUG-NNN: ... [SEVERITY: ...]` → Description /
  Mitigation / Files Affected / Prevention / `---` format used by
  BUG-001..005.
- Number it `BUG-006`.
- Content should state: (a) what regressed (secrets previously reached the
  service incidentally via `.zshrc` sourcing, now don't), (b) that this is
  intentional-tradeoff-not-yet-resolved, (c) the credible fix shape for a
  future implementer — `EnvironmentFile=` (Linux) + install-time inlining or
  a minimal `. env-file` wrapper (macOS) reading from a
  `~/.stapler-squad/env`-style file — without actually building it here.
  Mirror BUG-001's "Mitigation" tone: describe the fix path, don't take it.
- No stack/tooling decision is needed beyond what's already used in this
  script (POSIX `sh`, existing log helpers) — this is a pure Markdown edit,
  same file, same section, same format as its five neighbors.
