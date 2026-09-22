# Research: Features, Prior Art, Edge Cases (cli-flag-discovery)

Labels: VERIFIED = read in repo this session; PRIOR-ART = from general knowledge of the named tools (not re-fetched, treat as UNVERIFIED detail).

## 1. How programs are defined in the repo (VERIFIED)

- Built-ins: `BuiltInPrograms()` in `server/services/defaults_service.go:627-637`: claude, pi, aider, opencode, gemini, agy, bash. Each is `{ID, Label, Command, Description}`; `Command` is a bare binary name resolved via PATH.
- User programs: `ProgramConfigProto` (`proto/session/v1/session.proto:2526`): `command`, `cli_flags` (separate free-text field), `env` map, `is_builtin`. Note `env` is a structured map, so `FOO=1 claude` in `command` is not the intended shape, but users may still type it.
- Command strings are a single whitespace-split string, not argv. Existing code treats them with `strings.Fields` and basename matching: `isClaude`/`isPi`/`yoloFlagFor` (`session/instance_tmux.go:145-195`), `claudeBinaryPathFromProgram` (`session/claude_version_check.go:133`). Comment at `instance_tmux.go:145` explicitly anticipates env wrappers like `env -u VAR claude`. No shlex/quote-aware parser exists in `config/`, `session/`, `server/` (grep for shlex/shellquote found none). So quoting is unhandled today; probe must pick a tokenizer policy.
- `session/instance_tmux.go:177`: aider's `--yes-always` was verified against real `aider --help` (v0.78.0), evidence that real help output is the ground truth and flags drift between versions, which is the motivation for discovery.
- MCP tools hardcode `Enum("claude","aider")` (`server/mcp/tools_lifecycle.go:59`), unrelated to probing.

## 2. Prior art

| Tool | What it does | Lesson for us |
|---|---|---|
| cobra `__complete` / `completion` (PRIOR-ART) | Tool itself emits completions with descriptions; hidden `__complete` subcommand returns `flag\tdesc` lines | Authoritative when available. Could be an optional first-choice source for cobra binaries (`gh`, `kubectl`), but needs per-tool detection; out of scope, note as future tier. |
| clap_complete (PRIOR-ART) | Generates completion scripts at build time from the arg model, not by parsing help | Same: build-time knowledge, not runtime scraping. We have only text. |
| fish `complete --command X --wraps` / `fish_update_completions` (PRIOR-ART) | Parses man pages into completions; also heuristically parses `--help` | Closest analog: regex heuristics over help text, accepted to be lossy. Validates "best-effort, empty on failure". |
| zsh `_gnu_generic` / bash-completion `_longopt` (PRIOR-ART) | Runs `cmd --help`, greps `--[a-z-]+` tokens | Minimal viable parser is "find `--word` at line start"; descriptions are a bonus. Confirms the approach is standard. |
| help2man (PRIOR-ART) | Runs `--help`/`--version`, regex-transforms into man page | Same heuristics; documents that it needs the tool to follow GNU conventions. |
| VS Code / IDE shell integration, Warp, Fig/Amazon Q autocomplete (PRIOR-ART) | Curated spec files per tool (Fig specs) with runtime fallback | Curated overrides beat scraping for the handful of programs we care about (claude, aider). A checked-in per-program override/seed is a possible enhancement, not MVP. |
| In this repo | Nothing similar. Closest: the aider `--help` verification comment and `ProgramsManager` free text. | Greenfield; reuse `config/executor.go` `CommandExecutor` for testability. |

## 3. Edge cases and failure modes the design must handle

Command parsing (resolve "what do we exec"):
1. `FOO=1 claude --x`: leading `NAME=value` tokens are env, not the binary. Skip them for LookPath; do NOT pass them to the probe (sanitized env). Report `found` for `claude`.
2. Wrappers `env ... claude`, `npx foo`, `uv run tool`, `bunx`, `pipx run`, `sudo`, `nice`, `time`: first token is the wrapper, so `--help` on it describes the wrapper, not the target. Policy options: (a) probe the exact command string plus `--help` (wrapper passes it through: `npx foo --help`, `uv run tool --help` work; `env claude --help` also works), (b) only probe the first token. Recommend (a): append `--help` to the full tokenized command, and LookPath only the first non-env-assignment token. Caveat: `npx` may download packages (network, slow, exceeds 3s timeout); surface as "timed out / not probed" rather than "not found". Consider an allowlist-free but explicit "probe skipped for network-fetching runners" (npx, bunx, uvx, pipx run) or a longer opt-in.
3. `~` and `~/bin/tool`: `exec.LookPath` does not expand `~`; expand manually (`~` and `~/` only; `~user` unsupported). Relative paths (`./tool`) resolve against server cwd, not the user's project: flag as ambiguous or resolve against nothing.
4. Quotes: `claude --append-system-prompt "a b"`. `strings.Fields` breaks these. For probing, only the binary (and wrapper args) matter; use a small quote-aware tokenizer (or fail closed: unbalanced quote returns `found=false` with a reason, not an RPC error). Never route through a shell.
5. Empty/whitespace command, command that is only env assignments, absolute path that exists but is not executable or is a directory: all `found=false` with a reason string.
6. Shell metacharacters (`;`, `&&`, `|`, `$(...)`, backticks): with no shell they are literal argv, but `foo; rm -rf ~` would be looked up as binary `foo;`. Treat as not found; never pass through `sh -c`. Security: probe only the form being edited (already in requirements #8).
7. `cli_flags` field vs `command` field both contain flags: validation should union tokens from `command` (after the binary) and `cli_flags`.

Execution:
8. Tools that ignore `--help` and start a TUI/REPL/server (`bash` is a built-in: `bash --help` is fine, but `pi`, `opencode`, `claude` in old versions, `top`, `vim`): closed stdin (or `/dev/null`), no TTY (no pty), hard 3s timeout, kill the whole process group (`Setpgid` + `kill(-pgid)`) so children die; also set `TERM=dumb`, `NO_COLOR=1`, `CI=1`. Mark result `probe_status=timeout` distinct from "no flags".
9. `--help` on stderr (python argparse older versions/some Go tools): capture combined stdout+stderr; prefer stdout if non-empty else stderr, or concatenate.
10. Non-zero exit on `--help` (Go `flag` package exits 2; many tools exit 1): do not treat exit code as failure. Only treat as failure if start fails (exec error) or output is empty. Treat killed-by-timeout separately.
11. Tools that need `-h` instead of `--help` (Go `flag` accepts both; `ssh` prints usage on unknown flag; `java -help`): try `--help` only per requirements; note fallback `-h` as a possible cheap second attempt if first yields zero flags. Do not exceed one extra exec.
12. Side effects: some tools treat unknown flags as positional and act. Only pass exactly `--help`. Run with cwd set to a temp/empty dir (not the repo/worktree) and a sanitized env (`PATH`, `HOME`, `LANG`; drop tokens/API keys). Consider `HOME` handling: tools like claude read `~/.claude` config on startup even for `--help`; dropping HOME could change output or make the tool create files. Recommend keeping real HOME but stripping secret-looking vars (`*_TOKEN`, `*_KEY`, `ANTHROPIC_*`, `GH_*`).
13. Huge output: cap 256KB via limited reader; stop reading and kill process once cap hit (do not `cmd.Output()` unbounded); mark `truncated=true`. Still parse the truncated prefix.
14. Pager: some tools pipe help through `less` when stdout is a TTY. We use pipes so no pager, but set `PAGER=cat`, `GIT_PAGER=cat`, `MANPAGER=cat` defensively. Git: `git --help` execs `man`, needs `git -h` or `git help -a`; `git --help` with no TTY may try to launch `man` and fail or hang. Requirements list "git-style help" fixtures; probing plain `git --help` is likely to spawn man. Recommend `GIT_...` or accept `git -h` fallback. This is a concrete gotcha (PRIOR-ART knowledge, not run here; verify by running `git --help` under a pipe before implementing).
15. Concurrency and DoS: on-blur triggers for every keystroke-blur; dedupe in-flight probes per resolved path (singleflight), cap concurrency (repo has an OOM history from over-spawning, see memory index), rate limit by the client debounce.
16. Cache: key on resolved path + mtime + size + the exact args probed (wrapper form differs from bare form). `sync.Map` never evicts; bound it (LRU or max entries) and cache negative results (timeout) with short TTL so a slow tool is not re-run on each blur. Version upgrades that keep mtime are rare; fine.
17. Symlinked/shim binaries (asdf, nvm, pyenv shims, Homebrew): mtime of the shim does not change when the underlying version changes. Resolve symlinks (`EvalSymlinks`) for the cache key, but asdf/pyenv shims are shell scripts that stay identical across versions, so stale cache is possible. Acceptable; provide a "re-probe" affordance or short TTL (e.g. 10 min) in addition to mtime.
18. Service PATH differs from the user's shell PATH: the systemd unit / launchd job has a minimal PATH, so `LookPath("claude")` may say not found although it works in the terminal (tmux session inherits which env? check how sessions are launched). High-impact false negative. Design should report the PATH used or reuse the same PATH resolution the session launcher uses. `config/config.go:1267,1286` already does candidate lookup via `executor.LookPath`; check whether it augments PATH.

Parsing:
19. ANSI color codes (`\x1b[1m--flag\x1b[0m`): strip CSI/OSC sequences before parsing; also `\r` and backspace-overstrike (man-style `_\b`).
20. Formats: `-x, --long <val>  desc`; `--long=VAL`; `--long VAL`; `--long[=VAL]` (optional value); `-x VAL`; `--[no-]long` and `--no-long` (negatable: emit both `--long` and `--no-long`); `--long, -x`; comma-joined aliases `--foo, --bar`; clap `--long <VAL>` and `[possible values: a, b]`; cobra `-x, --long string   desc (default "x")`; argparse `--long LONG, -x LONG` and `{a,b,c}` choices; Go `flag` `-long value` with single dash long flags (`-verbose`); aider `--flag VALUE` plus `[$AIDER_ENV]` suffix and `--flag, --no-flag` via BooleanOptionalAction (aider prints `--dark-mode` and `--no-dark-mode` variants separately or as `--[no-]`... UNVERIFIED, capture a real `aider --help` fixture).
21. `takes_value` heuristics: value placeholder in `<...>`, ALLCAPS token, `=`, or cobra type word (`string`, `int`, `strings`, `duration`). Unknown => default false, but an unknown `takes_value` must not cause a false "unknown flag" warning for the following token.
22. Wrapped/multiline descriptions: continuation lines are indented deeper than the flag column and have no leading `-`; join into the previous flag's description. A description line that legitimately starts with `-` (e.g. "-1 means unlimited") is misparsed as a flag; require flag lines to match `^\s{0,8}-{1,2}[A-Za-z]` followed by end or 2+ spaces/`,`/`=`/space+placeholder.
23. Section headers (`Options:`, `Flags:`, `Global Flags:`, `OPTIONS`): use only to set context, do not require. Subcommand-only help (`git`, `docker`, `kubectl` top-level list commands, few global flags): return only the global flags found and set `flags_partial=true`/empty; empty is not an error (requirement 4).
24. Usage line `Usage: tool [OPTIONS] <FILE>` contains `--` tokens sometimes (e.g. `[--flag]`); dedupe by flag name and prefer the entry with a description.
25. Deprecated/hidden flags: help omits hidden flags, so "unknown flag" warnings will false-positive on valid hidden/undocumented flags (claude has many). Warnings must be soft and worded "not found in --help", never "invalid". Also flags passed to a wrapped tool after `--`.
26. Non-English/locale output: set `LC_ALL=C`/`LANG=C.UTF-8` for deterministic output.
27. Version drift: cache is per binary; record the `--version`? Not needed, but showing "flags from <path> (probed <time>)" helps trust.

Validation (UI):
28. Unknown-flag check must handle `--flag=value`, `--flag value`, `-abc` (bundled short flags), `--` terminator, positional values, quoted values with spaces, `--no-x` when only `--x` parsed. Prefer false negatives over false positives.
29. Autocomplete insertion should add a trailing space or `=` based on `takes_value`.
30. Mobile: tap-to-show tooltip; long descriptions truncated; do not rely on hover (req 9).

## 4. Unstated user needs

- Refresh/re-probe control and visible state: "probing...", "timed out", "not probed (wrapper)". A silent empty flag list is indistinguishable from "tool has no flags".
- Show the resolved path (and maybe version) so the user can spot the wrong binary on PATH.
- Distinguish "not on server PATH" from "not installed" (finding 18); many users run the service under systemd/launchd.
- Session creation: warn before spawn, but never block (users may have flags valid for newer versions or hidden flags).
- Remote/other-host sessions: is the binary looked up on the server host only? Sessions for remote paths, if any, may run elsewhere; probe answers only for the server host. Note as limitation.
- Env-defined programs: `ProgramConfigProto.env` should be honored for PATH-affecting overrides? Probably not; but do not leak those secrets into the probe env unless required.
- Built-ins (`bash`, `agy`, `pi`) should be probed as well and can be prewarmed lazily, but not on server start (avoid spawning 7 processes at boot).
- Accessibility: found/not-found indicator not by color alone (aria-live status text).
- i18n/no-op: RPC must be cheap when nothing changed; debounce client-side and skip probing unchanged commands.

## 5. Recommendations feeding the plan

- Tokenizer: skip leading `NAME=value` tokens; handle simple quotes; `~` expansion; return typed result `{binary, args, reason}`.
- Result shape: add `probe_status` enum (OK, NOT_FOUND, TIMEOUT, NO_FLAGS_PARSED, OUTPUT_TRUNCATED, SKIPPED) and `truncated`, `source_stream` fields beyond the requirement's minimum; keeps the UI honest.
- Combined-output capture, exit code ignored, process-group kill, `LC_ALL=C`, `NO_COLOR=1`, `TERM=dumb`, secret-stripped env, cwd=temp dir.
- Fixtures: capture real `--help` from claude, aider, gh (cobra), a clap tool (`rg`/`fd`), argparse (`python3 -m ...`), Go `flag`, and `git -h`, with ANSI variants.
- Verify before coding: `git --help` under a pipe, `claude --help` with stripped HOME, service PATH resolution.
