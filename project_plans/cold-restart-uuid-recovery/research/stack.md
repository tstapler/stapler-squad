# Stack Research: Cold-Restart UUID Recovery

Scope: `session/instance.go`, `session/instance_tmux.go`, `session/history_detector.go`,
`session/instance_claude.go`. Confirms this is a pure Go-stdlib, no-new-dependency,
surgical reordering fix.

## 1. Relevant Go stdlib / existing-in-repo primitives

`HistoryFileDetector.DetectByPath` (`session/history_detector.go:137-183`) already does
everything the fix needs and requires no live process:

- `os.ReadDir` — lists `~/.claude/projects/<encoded-path>/` (line 146)
- `path/filepath.Join` — builds the encoded-project-dir path (line 144)
- `strings.HasSuffix` / `strings.TrimSuffix` / `strings.HasPrefix` — filters `*.jsonl`,
  strips extension, excludes `agent-*` sidecar files (lines 164-169)
- `isValidUUID` (`regexp`-backed, declared elsewhere in the same package) — validates the
  basename is a real conversation UUID before treating it as a candidate (line 171)
- `sort` — orders candidates by mtime to pick the newest (imported at top of file)
- `entry.Info()` — per-`os.DirEntry` stat for mtime comparison (line 174)

No new stdlib package is needed beyond what `history_detector.go` already imports
(`os`, `path/filepath`, `regexp`, `sort`, `strings`).

## 2. Third-party dependencies — none new required

Confirmed via `go.mod` (module `go 1.26.3`):

- `github.com/go-git/go-git/v5` and its transitive `go-git/gcfg`, `go-git/go-billy` —
  used for git worktree operations elsewhere in `session/git/`. **Not touched by this
  fix** — no git operations are involved in UUID recovery.
- `github.com/puzpuzpuz/xsync/v4` — concurrent map type used elsewhere for
  high-contention shared state. Not needed here; the existing `claudeSessionMu`
  (a plain `sync.RWMutex`, see `HasClaudeSession`/`SetClaudeSession` in
  `session/instance_claude.go:260-273`) already guards `i.claudeSession`, and the fix
  operates entirely under `startLocked`'s existing single-threaded actor context.
- tmux integration is via the in-repo `session/tmux` package (`tmux.NewTmuxSession`,
  `tmux.TmuxSession`) plus process inspection via the in-repo `session/procinfo` package
  (`OpenFiles`, `IsAlive` — the `ProcessFileInspector` interface `HistoryFileDetector`
  depends on, `session/history_detector.go:22-25`). Both are existing first-party
  packages; `DetectByPath` (the function this fix needs to call earlier) doesn't even
  use `procinfo` — it's the **path-based**, no-live-process branch, as opposed to
  `Detect(pid)` which does use `procinfo.OpenFiles`.

**Conclusion: zero new entries in `go.mod` are expected for this fix.**

## 3. Version/tooling constraints

- **Go version**: `go.mod` declares `go 1.26.3`. No version-gated stdlib features are
  needed (everything used is pre-1.20 stable: `os.ReadDir`, `filepath`, `regexp`, `sort`,
  `strings`).
- **`make nil-safety`** (NilAway + `go vet -nilness`, per root `Makefile:672-689`) is
  relevant: `DetectByPath` already returns `nil, nil` with an explicit `//nolint:nilnil`
  annotation (`session/history_detector.go:149`, `:110`) for the "not found" case. Any
  new call site added in `startLocked` that consumes this return must handle the
  `(nil, nil)` case explicitly (i.e., "no history file found" is not an error) or NilAway
  will likely flag it. Reuse the same nil-check idiom already used by
  `tryExtractConversationUUID` (`session/instance_claude.go:326`: `info, err =
  detector.Detect(pid)` followed by an `if info == nil` fallback check at line 336) rather
  than inventing a new pattern.
- **Rules that do NOT apply to this fix** — call this out explicitly so planning doesn't
  waste effort there:
  - `.claude/rules/ent-schema-generation.md` (ent ORM `--feature sql/upsert`) —
    irrelevant; this fix touches no `session/ent/` schema or generated code.
  - `.claude/rules/prefer-go-git-over-subshells.md` (go-git vs `safeexec` CLI shellouts)
    — irrelevant; this fix performs no git operations at all. The UUID recovery path
    reads `~/.claude/projects/*.jsonl` files directly via `os.ReadDir`, not via git.
  - No proto changes, so `make proto-gen` is not needed.
  - No new session-creation mode, so `.claude/rules/session-creation-registry.md`'s
    7-touchpoint checklist does not apply.

## 4. Existing test helpers/patterns to reuse (not reinvent)

- **`session/instance_cold_restore_test.go`** — already has the exact scaffolding for
  simulating "tmux dead + Start(firstTimeSetup=false)":
  - `checkTmuxAvailable(t)` (line 17) — skips if `tmux` binary isn't on `PATH`.
  - `coldRestoreSocket(t)` (line 28) — allocates an isolated tmux server socket per test
    with PID-embedded name + `t.Cleanup` teardown (avoids colliding with the real/dev
    tmux server or other parallel tests).
  - `TestColdRestore_WithUUID` (line 44) is the closest existing analog to the AC #4
    regression test: it constructs an `Instance` via `NewInstanceWithCleanup`, sets
    `inst.SetClaudeSession(&ClaudeSessionData{ConversationUUID: ...})` while tmux is
    dead, and asserts the cold-restore path behaves correctly. The new regression test
    (AC #4: dead tmux + **empty** UUID + JSONL present on disk → revives with `--resume`)
    should follow this same structure but *invert* the setup — leave
    `ConversationUUID` empty and instead pre-populate a fake
    `~/.claude/projects/<encoded-path>/<uuid>.jsonl` file (via a test-injected `homeDir`,
    see next bullet) so `DetectByPath` has something to find.
  - Note: `--resume` flag injection itself is unit-tested separately in
    `claude_command_builder_test.go` per the comment at cold_restore_test.go:71-72 — the
    new regression test only needs to verify the *lifecycle* (UUID gets populated before
    `initTmuxSession`/`buildLaunchCommand` runs), not re-test flag construction.
- **`session/history_detector_test.go`** — exercises `DetectByPath` directly; check its
  existing use of `NewHistoryFileDetectorWithHomeDir` (line 40 in history_detector.go) to
  redirect the `~/.claude/projects/` scan to a temp dir. Reuse this same
  `homeDir`-override constructor for any new test that needs `DetectByPath` to find a
  file without touching the real home directory, rather than adding a second
  test-injection mechanism.
- **`session/instance_workspace_test.go`** — has the canonical test for
  `tryExtractConversationUUID`'s "don't overwrite an existing UUID" guard
  (`TestTryExtractConversationUUID_SkipsWhenAlreadyHasID`, lines 23-40): constructs a bare
  `&Instance{claudeSession: &ClaudeSessionData{ConversationUUID: existingID}}` (no tmux,
  no live process) and asserts the guard at the top of `tryExtractConversationUUID`
  (`session/instance_claude.go:310`: `if i.claudeSession != nil && i.claudeSession.ConversationUUID != "" { return }`)
  fires before any tmux/detector interaction. **This is the exact mechanism AC #5 depends
  on** — the fix must call the same guarded function (or an equivalently-guarded new one)
  so a paused session's already-correct UUID is never overwritten by a newer JSONL
  belonging to a different session. Whatever new pre-`initTmuxSession` recovery call the
  fix adds should either directly reuse `tryExtractConversationUUID`'s existing early
  return, or replicate it precisely — do not write a second, less careful guard.

## Bug location recap (for planning, not re-derived here)

- `startLocked` (`session/instance.go:834`) calls `i.initTmuxSession()` at line 847 —
  before the `!firstTimeSetup` / `!i.pm().IsAlive()` branch (lines 867-910) that currently
  runs `i.tryExtractConversationUUID()` only at line 910, **after**
  `i.pm().Start(startPath)` (line 891) has already launched the process with whatever
  command `initTmuxSession` → `buildLaunchCommand` baked in at line 847.
- `initTmuxSession` (`session/instance_tmux.go:249`) reads `i.claudeSession.ConversationUUID`
  directly (lines 254-258) and passes it into `buildLaunchCommand`, which is what decides
  whether `--resume <uuid>` is embedded in the `claude` launch command.
- The second mirrored occurrence of this same ordering is around `session/instance.go:1061-1116`
  (per requirements.md's "mirrored at ~1061" / "mirrored at :1116" notes) — both call
  sites need the same reordering fix, not just one.
