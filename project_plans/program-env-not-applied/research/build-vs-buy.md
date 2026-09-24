# Build vs. Buy

No build-vs-buy evaluation applies to this item. It's a wiring/root-cause bugfix
in existing first-party code — no new library, SaaS dependency, or generated data
structure is in scope.

## What was checked

- **Env-var construction** (`session/instance_tmux.go:579-614`,
  `session/tmux/tmux_session_start.go:210-219,613-623`): `buildExtraEnv()` builds
  plain `fmt.Sprintf("%s=%s", k, v)` strings, and the tmux layer passes each as its
  own `"-e", kv` argv pair to `new-session` — never concatenated into a shell
  string, so there's no escaping/quoting problem a library would solve. This is
  the correct shape for `exec.Command`-style argv (each element already isolated),
  not a case needing an env-list library (e.g. `envconfig`/`godotenv`-style
  parsing is for reading `.env` files, not for building `-e` argv pairs).
- **`go.mod`**: only found `github.com/anmitsu/go-shlex` (indirect, transitive —
  not imported anywhere under `session/` per `grep -rn "go-shlex\|shlex\."`
  returning no direct call sites) and no other env/exec-var library. Nothing
  applicable is already imported and unused.
- **Snapshot pattern**: `atomic.Pointer[InstanceSnapshot]`
  (`session/instance_snapshot.go`) is stdlib `sync/atomic`, not a third-party
  library — it's a hand-rolled copy-on-write publish pattern documented in
  `.claude/rules/instance-lock-free-reads.md`, already correctly used by
  `resolveExtraEnvVars()`/`buildExtraEnv()` via `i.Snapshot()`. No package
  (e.g. `xsync`) backs it; nothing to swap in.
- **Existing internal env-building helpers elsewhere in the repo**
  (`config/clihelp/runner.go`, `executor/managed_process.go`,
  `executor/shortlived.go`, `github/client.go`, `server/services/exec_unix.go`,
  `session/repo_path.go`, `session/tymux/supervise.go`, `session/vnc/window_tracker.go`,
  `session/tmux/tmux.go`): these all build `os.Environ()`-plus-overrides slices for
  direct `exec.Cmd.Env`, a different mechanism from tmux's `-e` flag injection.
  None is a generic "build env var list" helper the fix should reuse — the
  existing `resolveExtraEnvVars()` → `buildExtraEnv()` pair is already the
  single, already-shared point (also feeding `claudeSettingsEnvOverrideArgs()`)
  that the fix should extend, not bypass or duplicate.

## Bottom line

The root cause is a wiring gap (an env value computed correctly but not
reaching the tmux `-e` argv, or a resolution-order bug between
`resolveExtraEnvVars()`'s custom-program and instance-level maps) inside code
that already has the right shape and the right library choices. Fix it in
place; no dependency addition is warranted.
