# Research: Technology Stack — Launcher Presets

Phase 2 research dimension: stack/dependencies. See `../requirements.md` for full context.

## Summary

No new Go dependencies are needed. Everything Launcher Presets requires — JSON parsing,
schema/shape validation, file-watching, and safe multi-argument process launch — is already a
dependency of this repo, and each has an established in-repo usage pattern to copy. The one
real architectural question isn't "which library" but "how do the existing single-shell-string
launch paths (`session/instance_tmux.go`) need to change to carry an `argv` array without
re-flattening it through a shell string" — see the "argv vs shell-string" section below, which
is the crux of Success Criterion 3.

## 1. JSON config loading + validation

**Use `encoding/json` (stdlib) + hand-written Go struct validation. Do not add a schema
validation library for this.**

- The repo already has two live patterns for "load a JSON config file at startup, fail loudly
  on bad input":
  - `config/config.go:847` `LoadConfigFromPath(path string) (*Config, error)` — `os.ReadFile`
    → `json.Unmarshal` → returns `(nil, err)` on failure, straightforward and exactly the shape
    needed for `~/.stapler-squad/launcher-presets.json`.
  - `config/claude.go:236` `ValidateJSON` uses `github.com/xeipuuv/gojsonschema` (already a
    go.mod dependency, v1.2.0) to validate `settings.json` against a **JSON Schema string**
    embedded in Go source (`settingsJSONSchema`).
- **Recommendation: skip gojsonschema for this feature.** It exists in the repo for one
  specific case (validating arbitrary user-edited Claude `settings.json` against an external,
  complex schema with many optional/nested fields). Launcher presets have a small, fixed shape
  (`version`, `presets[].{id,label,argv,program?,default_path?}`) — hand-written Go validation
  after `json.Unmarshal` (check `len(argv) > 0`, check `id` non-empty and unique via a
  `map[string]bool` seen-set, check `version == 1`) is fewer lines, has better error messages
  (can name the exact preset index/id that's wrong), and avoids maintaining a second
  schema-string artifact in sync with the Go struct. Reserve gojsonschema for cases needing
  actual JSON Schema semantics (`oneOf`, `$ref`, external schema files) — this isn't one.
- Requirement "reject the whole file with a clear error on schema/JSON errors (do not
  partially apply)" maps directly to `LoadConfigFromPath`'s pattern: parse into a temporary
  struct, run all validations, only assign to the live config on full success.
- Version field (`"version": 1`) gives forward-compat room; validate `version == 1` explicitly
  now and fail with a clear "unsupported version" error for anything else, rather than
  silently ignoring unknown fields (Go's `json.Unmarshal` already ignores unknown fields by
  default, which is fine for additive changes but should not extend to the version number
  itself).

## 2. File-watching for hot reload

**Use `github.com/fsnotify/fsnotify` (already a dependency, v1.9.0) — copy the pattern in
`session/unfinished/watcher.go`, not a new library.**

- `session/unfinished/watcher.go` (`WatchDirWatcher`) is the canonical in-repo fsnotify
  pattern:
  - `NewWatchDirWatcher` calls `fsnotify.NewWatcher()` and **degrades gracefully to nil** if
    unavailable (`w.watcher = nil`, logs a warning) rather than failing hard — this repo's
    convention for fsnotify is "best effort, never load-bearing for correctness."
  - The event loop (`fsnotifyLoop`) is a `select` over `ctx.Done()`, `watcher.Events`, and
    `watcher.Errors`, run in its own goroutine (`go w.fsnotifyLoop(ctx)`).
  - A `periodicReWalk` ticker (60s) runs **in addition to** fsnotify as a belt-and-suspenders
    fallback — worth considering for presets too (e.g. re-stat the file every N seconds) since
    fsnotify on some filesystems/editors (atomic rename-on-save, editors that write to a temp
    file then `rename()` over the original) can miss events if the watch is on the file itself
    rather than its parent directory.
  - **Watch the parent directory, not the file directly** — this is implied by the watcher.go
    pattern (it watches `.git/` dirs, and directory-level fsnotify events survive
    delete+recreate, which file-level watches do not on Linux: an inotify watch on a specific
    inode is invalidated when that inode is unlinked, e.g. by `mv` for atomic saves). For
    `launcher-presets.json`, watch `~/.stapler-squad/` (or the specific parent dir) and filter
    events by `filepath.Base(event.Name) == "launcher-presets.json"`.
  - Debounce: `fsnotify` commonly fires multiple events for a single logical save (e.g.
    WRITE + CHMOD, or multiple WRITEs from an editor's buffered writer). `watcher.go` doesn't
    debounce because repo-scans are naturally idempotent; a presets reloader should debounce
    with a short timer (e.g. 200-500ms) coalescing bursts before re-reading the file, to avoid
    reading a half-written file mid-save. Use a single `time.Timer` reset on each event
    (standard fsnotify debounce idiom), not a ticker.
- No version bump or new dependency needed; fsnotify v1.9.0 (current go.mod pin) is a recent
  stable release and there's no reason to bump it for this feature.

## 3. Safe argv-array process launching (vs. shell-string)

This is the part worth the most attention, because **the existing launch pipeline is
shell-string-based, not argv-based**, and the requirement explicitly wants to avoid
shell-quoting corruption.

### Current architecture (as of this research)

- `session/instance_tmux.go` builds a **single shell command string** for every session,
  regardless of program:
  - `buildLaunchCommand` (`session/instance_tmux.go:105`) starts from `i.Program` (a plain
    string) and appends `strings.Fields(i.CLIFlags)`, each individually run through
    `shellQuote` (line 116).
  - `shellQuote` (`session/instance_tmux.go:126`) POSIX-single-quotes each token: `"'" +
    strings.ReplaceAll(s, "'", `'\''`) + "'"`. This is a correct, standard shell-quoting
    primitive (handles embedded single quotes via the classic `'\''` close-escape-reopen
    trick) but it is fundamentally **string-based quoting for a string-based transport**: the
    assembled command is handed to `tmux new-session` as one string, which tmux hands to a
    shell to interpret. Every token has to survive two rounds of textual reconstruction
    (Go slice → shell-quoted string → shell re-parses it into argv) to arrive back at the
    argv the caller originally had.
  - `buildClaudeCommand` (line 133) does the same per-flag shell-quoting for claude-specific
    args (`--resume`, `--append-system-prompt`, etc.).
  - Large payloads (prompts ≥ 4KB, see `maxInlinePromptBytes`) are routed around tmux's ~16KB
    command-string limit via a temp file + `$(cat ...)` shell substitution
    (`promptArg`, line 198) — an additional shell-string-specific workaround that wouldn't be
    needed if the transport were argv-native.
- **Executing the assembled tmux command itself is already correct/safe**: everywhere in the
  codebase that shells out (`executor/safeexec`, used pervasively per
  `.claude/rules/prefer-go-git-over-subshells.md`'s sibling convention) uses
  `exec.CommandContext(ctx, name, arg...)`-style **variadic argv**, not a shell string handed
  to `sh -c`. The shell-string step is specific to the *tmux new-session command payload*
  (because tmux's own new-session syntax takes "the command to run in the new pane" as shell
  syntax, by tmux's own design), not to how this Go process invokes `tmux` itself
  (`safeexec.CommandContext(ctx, tmux.Binary(), args...)` in `session/pty_discovery.go` is
  already argv-safe).

### Implication for Launcher Presets

- A preset's `argv []string` (e.g. `["ssh", "-t", "host", "cd ~/repo && exec claude"]`) can be
  carried as a real Go `[]string` all the way from the JSON config file through the RPC
  (`repeated string argv` per the open proto question in requirements.md) up to the point
  where it's handed to `buildLaunchCommand`'s tmux-string-assembly step — **that boundary is
  unavoidable given tmux's new-session API**, but it should be the *only* place a shell-string
  reconstruction happens, and it should reuse the existing `shellQuote` helper per-argv-element
  (i.e. `strings.Join(mapShellQuote(argv), " ")`) rather than introducing a second, differently
  -behaved quoting routine. This keeps "argv-based, no ad-hoc shell interpolation" true for
  every layer *except* the final tmux hand-off, which is a pre-existing, well-tested,
  correctly-escaped shell-quoting boundary already exercised by every other session in this
  system today (not something Launcher Presets needs to solve fresh).
  - Concretely: today `plainProgram{cmd: program}` in `classifyProgram` returns a single
    un-quoted string typed directly by the user via `program` (trusted as one flag, not
    reconstructed). An `argv`-based preset should instead build its command via
    `shellQuote` per-element the same way `i.CLIFlags` fields already are (line 116) — so a
    preset element containing `cd ~/repo && exec claude` (an intentional shell fragment inside
    an `ssh -t host '...'` remote-exec array element) is preserved byte-for-byte as one shell
    token, while a preset element containing e.g. a path with spaces or an embedded `$(...)` is
    **not** given a chance to be interpreted, because the whole element is inside single quotes
    when the assembled string reaches the shell tmux spawns.
- **No new library needed for this.** `shellQuote` (`session/instance_tmux.go:126`) already
  does exactly the per-token POSIX single-quoting a `[]string` → shell-command-string
  conversion needs; there is no reason to reach for `mvdan.cc/sh/v3` (already a dependency, but
  used elsewhere — `pkg/classifier/command_parser.go` — for *parsing* shell syntax, not
  *generating* it) or a third-party shlex/shellwords library. Introducing a second quoting
  implementation for the same shell-string boundary is the kind of dependency Launcher Presets
  should avoid; converge on the existing `shellQuote`.
- Anywhere in the pipeline that does NOT need to go through a shell (e.g. if a future
  implementation phase decides to spawn the argv directly via `safeexec.CommandContext` instead
  of through tmux's shell-string new-session syntax — out of scope per this feature's
  Constraints, which keep the existing tmux pane launch path) should use Go's native
  `exec.Command(argv[0], argv[1:]...)`-style variadic call, per
  `.claude/rules/prefer-go-git-over-subshells.md`'s sibling convention and the `safeexec`
  wrapper's existing contract (`executor/safeexec/safeexec.go`) — never build a shell string
  for that path in the first place. This is the standard "argv-array avoids shell injection"
  guidance (no shell metacharacter interpretation occurs when a program is invoked directly
  with an argv slice rather than via `/bin/sh -c "<string>"`), and it's already how every
  `safeexec.CommandContext` call site in this repo (git, gh, tmux, ps, xdotool — see
  `session/git/worktree_git.go`, `session/pty_discovery.go`, `session/vnc/window_tracker.go`)
  works.

## 4. ConnectRPC / Protobuf — no new tooling

- `GetLauncherPresets` RPC and any new `repeated string argv` field on `CreateSessionRequest`
  use the existing proto toolchain already in go.mod: `connectrpc.com/connect v1.19.0`,
  `google.golang.org/protobuf v1.36.11`, generated via the repo's existing `make proto-gen`
  (buf-based, `buf.build/go/...` deps already present). No stack changes here — this is pure
  "add fields/RPC, regenerate" work per `CLAUDE.md`'s "New API Endpoints" section.

## 5. Frontend — no new libraries

- Omnibar UI (Presets section) is plain React/TypeScript + vanilla-extract
  (`.claude/rules/css-architecture.md`) inside the existing `web-app/src/components/sessions/`
  and `web-app/src/lib/omnibar/` structure. No new frontend dependency is implicated by this
  feature — it's additive UI on an existing component tree, consuming a new generated
  ConnectRPC client method the same way existing RPCs are consumed
  (`web-app/src/lib/hooks/useSessionService.ts` pattern).

## Dependency Change Summary

| Need | Library | Status |
|---|---|---|
| JSON parsing | `encoding/json` (stdlib) | Already used everywhere |
| Config shape validation | Hand-written Go validation (no schema lib) | New code, no new dependency |
| File-watching / hot reload | `github.com/fsnotify/fsnotify` v1.9.0 | Already a dependency (`session/unfinished/watcher.go`) |
| Argv → shell-command-string (tmux hand-off only) | `shellQuote` in `session/instance_tmux.go` (reuse) | Already exists, no new dependency |
| Direct argv process exec (if ever needed outside tmux) | `executor/safeexec` (wraps `os/exec` with `WaitDelay`) | Already a dependency/convention |
| RPC/proto | `connectrpc.com/connect`, `google.golang.org/protobuf`, buf toolchain | Already present, `make proto-gen` |
| Frontend UI | React/TypeScript + vanilla-extract | Already present, no new package |

**Net new go.mod / package.json entries required: none.**

## Open Items Carried Forward to Phase 3 (Plan)

- Confirm whether `argv[0]` should always be validated against `AvailablePrograms`
  (`config/config.go:736`) or accepted as-is (presets are user-authored and may reference
  programs not auto-detected, e.g. a full path or a remote-exec wrapper like `ssh`) — this is a
  validation-strictness decision, not a stack decision, but affects where in the Go validation
  code (`argv` load-time checks vs. RPC-time) it belongs.
- Whether hot-reload (fsnotify) debouncing should share a small helper with
  `session/unfinished/watcher.go`'s pattern or stay preset-local — a minor internal-API
  decision for Phase 3, not a new dependency either way.
