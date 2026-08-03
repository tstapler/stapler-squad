# ADR-004: Plugin Trust Boundary — RE2 Only, Resource Caps, No Sandbox

## Status
Accepted — 2026-08-01

## Context

`requirements.md` §Non-Functional asks for two things that need an explicit,
written answer so a future reviewer does not either under- or over-build:

1. "Malformed or malicious plugin content has bounded blast radius: arbitrary
   regexes only (no code execution, no shell-out, no file/network access from
   plugin content)."
2. "Regex compilation must not be able to hang the process (catastrophic
   backtracking) in a way that blocks startup indefinitely — document what
   protection Go's `regexp` package already provides (RE2, linear-time) vs.
   what if anything needs to be added."

The temptation, seeing "untrusted user regex," is to add a sandbox, a timeout
goroutine around every `regexp.Compile`, or a regex linter. Most of that would
be defending against a threat that Go's regexp engine structurally does not
have.

## Decision

### 1. Threat model: resource consumption only, never privilege escalation

Go's `regexp` package compiles to **RE2**, which guarantees linear-time
matching in the length of the input. RE2 deliberately **does not implement
backreferences, lookahead/lookbehind, or recursive constructs** — the
constructs that make catastrophic backtracking (ReDoS) possible in PCRE-style
engines. There is no known input that makes a Go `regexp` match take
exponential time.

Combined with the plugin content being *only* regex strings and identifiers
(no expression language, no scripting runtime, no shell-out, no file or
network primitives — see `requirements.md` §Out of Scope, which rules out
Lua/maki-style scripting explicitly), the realistic worst case a hostile
`.toml` file can achieve is:

- a large compiled program (memory), and
- slower matching (CPU, still linear).

Not code execution. Not file access. Not privilege escalation. **No sandbox,
no process isolation, no timeout-around-compile is warranted, and adding one
would be defending a vector that does not exist.**

This ADR exists primarily so that conclusion is written down. A future
reviewer proposing a plugin sandbox should be pointed here first.

### 2. Resource caps (hygiene, not security)

Because compile cost and per-match cost still scale with pattern count and
pattern size — and hot-reload re-parses the directory on every settled
filesystem event — the loader enforces cheap, documented bounds:

| Cap | Value | Rejection message references |
|---|---|---|
| `maxPatternsPerPlugin` | 50 patterns per file | file path, count, cap |
| `maxRegexLength` | 4096 bytes per `regex` value | file path, `patterns[i].name`, length, cap |
| `maxPluginFileSize` | 256 KiB read limit per file | file path, size, cap |

These are constants in `session/detection/plugins.go`, not config keys — they
exist to make "someone pasted a generated 10k-pattern file" a clean rejection
rather than a mystery slowdown, and no user should need to tune them. Raising
one later is a one-line change.

`maxPluginFileSize` is enforced by stat-then-read so a pathological file is
rejected before it is read into memory.

### 3. Path handling

- Only files matching `*.toml` directly inside the plugin directory are
  loaded. **No recursive descent** into subdirectories.
- Symlinked entries are **skipped**, with a `log.Warn` naming the file. This
  is an explicit choice rather than an accident: following symlinks out of the
  plugin directory turns "drop a file in a directory" into an arbitrary-file-
  read primitive on any path the daemon user can reach, and the value of
  supporting symlinked plugin files is near zero. Detected via
  `os.Lstat` + `Mode()&os.ModeSymlink != 0`.
- The directory path itself is derived from `config.GetConfigDir()`
  (`config/config.go:117-119`) joined with `"detectors"`, never from raw
  `os.UserHomeDir()` — this keeps the feature correctly isolated under
  `STAPLER_SQUAD_TEST_DIR` and `STAPLER_SQUAD_INSTANCE` like every other piece
  of application state.

### 4. Kill switch

`STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS=1` makes `InitPlugins` return
immediately without creating the directory, scanning, or starting a watcher.
The built-ins-only snapshot installed by package `init()` remains active, so
the process behaves exactly as it does today.

Note this is an **opt-out env var, not a `config.GetFeatureFlag` entry**:
`GetFeatureFlag` returns `false` for any unset key (`config/config.go:999-1004`),
so gating on it would leave the feature off by default and violate
acceptance criteria #1 and #5, which describe it working with no configuration
beyond dropping a file.

## Consequences

- No sandboxing code, no `context.WithTimeout` around `regexp.Compile`, no
  regex complexity linter is written. If a future contributor proposes one,
  the burden is on them to name a concrete RE2 vector.
- The three caps need three test cases each proving the rejection message
  names the file and the offending field.
- Skipping symlinks means a user who symlinks a plugin from a dotfiles repo
  gets a warning and no detector. That warning message should say *why* ("
  symlinked plugin files are not followed") so the workaround (copy, don't
  link) is obvious.
