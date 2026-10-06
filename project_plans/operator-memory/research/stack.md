# Stack Research: Persistent Operator Memory

## File I/O — stdlib only, no new dependency

Confirmed: `os`/`io`/`filepath` from stdlib are sufficient for this feature. Nothing
in this repo uses a third-party file-watch/config library for simple markdown
read/write (`fsnotify` is already a dep but is used for live config reload
elsewhere, not needed here since memory is loaded once per call as a frozen
snapshot per the requirements).

- `os.ReadFile` / `os.WriteFile` for OPERATOR.md / REPO.md.
- `os.MkdirAll(dir, 0755)` to ensure the `memory/` subdirectory exists before
  first write (same pattern `config.GetConfigDirForDir` itself uses at
  `config/config.go:128` for the test-dir override).
- Empty-file handling: `os.ReadFile` on a missing file returns `os.ErrNotExist`
  — treat "file missing" and "file exists but empty/whitespace-only" the same
  way (produce no prompt block), per requirement 5.
- No atomic-write library is needed for this scope (single small file, no
  concurrent-writer requirement yet — the writer is a companion item). If a
  future writer needs atomicity, note `server/services/hook_injector_test.go`'s
  `TestWriteSettingsAtomic_...` pattern (temp file + rename) already exists
  in this codebase as prior art — no need to introduce a new lib.

## Prompt-injection scanning — reuse existing helpers, don't add a library

Found exactly one existing sanitization helper, in `session/backlog_review.go:607-616`:

```go
// sanitizeDiff neutralises triple-backtick sequences in a diff to prevent
// prompt injection: a ``` inside the diff block would close the code fence and
// allow the model to interpret subsequent diff content as instructions.
func sanitizeDiff(diff string) string { return SanitizeDiff(diff) }

// SanitizeDiff neutralizes triple-backtick sequences in a diff so they cannot
// close a markdown code fence when the diff is interpolated into an LLM prompt.
func SanitizeDiff(diff string) string {
	return strings.ReplaceAll(diff, "```", "` `` ")
}
```

This is exported (`SanitizeDiff`) and importable from `session` package. It only
guards against code-fence breakout, not general prompt-injection phrase
detection (e.g. "ignore previous instructions"). The requirements doc's
"content scanned for prompt injection before any write" (req 6) implies
something closer to the Hermes `memory_tool.py` reference design — a phrase/
pattern scan run at *write* time (before persisting operator-authored memory
content), not just fence-neutralization at *read/interpolation* time.

**Grep results**: no existing "injection scanner" beyond `SanitizeDiff` exists
anywhere in the codebase (`grep -rniE "injection|sanitiz"` across all non-test
`.go` files returned only the `SanitizeDiff`-related hits and unrelated noise
like "hook-injection time" comments in `server/services/hook_injector.go`).

**Recommendation**: reuse `session.SanitizeDiff` for fence-neutralization when
interpolating memory content into the prompt tail (same risk as diffs — a
```` ``` ```` in OPERATOR.md/REPO.md could break the surrounding fence). For the
write-time injection *scan* required by req 6, there is no existing helper to
reuse — plan phase should scope a small new function (e.g.
`session.ScanForPromptInjection(content string) error`, colocated next to
`SanitizeDiff` in `backlog_review.go` or a new `session/memory.go`), not a new
external library. A simple denylist/pattern check (stdlib `regexp` or
`strings.Contains` over a small phrase list) is consistent with the project's
existing zero-dependency approach to this problem — no NLP/classifier library
is justified for this scope.

## Cobra command pattern for `stapler-squad memory show`

Two existing patterns in the repo:

1. **Inline in `main.go`** (dominant pattern) — most commands (`resetCmd`,
   `debugCmd`, `versionCmd`, `testPtyCmd`, `listSessionsCmd`,
   `printQRCodesCmd`) are package-level `*cobra.Command` vars declared in the
   `main.go` `var (...)` block (`main.go:363-454+`) and wired up via
   `rootCmd.AddCommand(...)` calls near the bottom of `main.go` (lines
   711-717). Example (`debugCmd`, `main.go:409-429`):

   ```go
   debugCmd = &cobra.Command{
       Use:   "debug",
       Short: "Print debug information like config paths",
       RunE: func(cmd *cobra.Command, args []string) error {
           cfg := config.LoadConfig()
           log.InitializeWithConfig(false, buildLogConfig(false, cfg, false))
           defer func() { log.LogSessionPathsToStderr(); log.Close() }()
           configDir, err := config.GetConfigDir()
           ...
       },
   }
   ```

2. **Separate package `cmd/commands/`** — one existing file,
   `cmd/commands/get_session.go`, defines `GetSessionCmd` as an exported
   `*cobra.Command` in package `commands`, imported into `main.go` as
   `cmdbridge "github.com/tstapler/stapler-squad/cmd"` /
   `"github.com/tstapler/stapler-squad/cmd/commands"` and added via
   `rootCmd.AddCommand(commands.GetSessionCmd)` (`main.go:717`). This one is a
   thin RPC client hitting a hardcoded `localhost:8080` — likely an early/
   unused prototype, not representative of the direct-storage-access pattern
   this feature needs.

**Recommendation**: add a new file `cmd/commands/memory.go` (package
`commands`, following get_session.go's file-per-command convention) defining
`MemoryCmd` as a parent command with a `show` subcommand added via
`MemoryCmd.AddCommand(memoryShowCmd)`, mirroring how `resetCmd`/`debugCmd`
call `config.LoadConfig()` + `config.GetConfigDir()`/`GetConfigDirForDir()`
directly (no RPC round-trip needed — `memory show` is a local file read, not
a server call). Wire into `main.go` via
`rootCmd.AddCommand(commands.MemoryCmd)` alongside the other `AddCommand`
calls at `main.go:711-717`.

## Config-dir resolution for OPERATOR.md / REPO.md placement

`config/config.go`:

- `GetConfigDir() (string, error)` → `GetConfigDirForDir("")` (config/config.go:117-119).
- `GetConfigDirForDir(dir string) (string, error)` (config/config.go:123-165) —
  priority order:
  1. `STAPLER_SQUAD_TEST_DIR` env override (test isolation — see below)
  2. `STAPLER_SQUAD_INSTANCE` env (named instance → `~/.stapler-squad/instances/<id>/`)
  3. Test-mode auto-detection (`IsTestMode()`) → `~/.stapler-squad/test/test-<pid>/`
  4. Preferred workspace file / opt-in per-directory workspace isolation /
     shared `~/.stapler-squad` (via `resolveDefaultConfigDir`, config/config.go:167+)

Per requirements:
- `~/.stapler-squad/memory/OPERATOR.md` (fleet-level) — this is **not**
  workspace-scoped, so it should NOT go through `GetConfigDirForDir(dir)`'s
  per-workspace branch; it belongs directly under the base `~/.stapler-squad`
  dir (i.e. `GetConfigDir()`'s resolved root, since `GetConfigDir()` calls
  `GetConfigDirForDir("")` which for the default/shared case still resolves
  to `~/.stapler-squad` unless a named instance or workspace override is
  active — plan phase should confirm whether OPERATOR.md should sit outside
  even instance/workspace isolation, i.e. truly `~/.stapler-squad/memory/`
  literally, or under the resolved config dir so it isolates per test/named
  instance like everything else. Given the requirement's literal path
  `~/.stapler-squad/memory/OPERATOR.md`, and that test isolation already
  redirects the *whole* `~/.stapler-squad` root via `STAPLER_SQUAD_TEST_DIR`
  (see below), using `config.GetConfigDir()` (not a hardcoded
  `os.UserHomeDir()` join) is correct and keeps tests isolated automatically).
- `<workspace-config-dir>/memory/REPO.md` (per-workspace) — use
  `config.GetConfigDirForDir(workspaceDir)` directly, consistent with how
  other per-workspace state is resolved elsewhere in the codebase.

**Test isolation**: `STAPLER_SQUAD_TEST_DIR` (checked first, `config/config.go:125-131`)
redirects the *entire* config root to a temp dir and auto-creates it
(`os.MkdirAll(testDir, 0755)`). `config/config_test.go:877-888` shows the
standard test pattern:

```go
original := os.Getenv("STAPLER_SQUAD_TEST_DIR")
os.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
defer os.Setenv("STAPLER_SQUAD_TEST_DIR", original) // or Unsetenv if empty
```

As long as the memory store's path resolution goes through
`config.GetConfigDir()` / `config.GetConfigDirForDir()` (not a hand-rolled
`os.UserHomeDir()` + `.stapler-squad` join), tests get automatic isolation for
free with zero new test-only plumbing — this is the important design
constraint to carry into the plan phase.

## go.mod — no new dependency needed

Reviewed `go.mod` in full. Everything required is already available:
`os`/`io`/`filepath`/`strings`/`regexp` (stdlib), `github.com/spf13/cobra`
v1.10.1 (CLI), `github.com/stretchr/testify` v1.11.1 (tests already used
throughout). No markdown-parsing, file-locking, or NLP/pattern-matching
library is justified for this scope — the "injection scan" is expected to be
a small phrase/pattern denylist (same spirit as `SanitizeDiff`'s
fence-neutralization), not a classifier. **No new dependency should be added
in the plan phase**; if a reviewer proposes one (e.g. a markdown AST parser),
push back — the feature only needs to read/write/concatenate whole files as
plain text.

## Key files for the plan phase

- `session/backlog_review.go:607-616` — `SanitizeDiff` (reusable fence-neutralization)
- `session/backlog_review.go:222` — `BuildReviewPrompt` (prompt assembly to extend with the memory tail)
- `session/backlog_triage.go:55` — `BuildHeadlessTriagePrompt` (same)
- `session/headless/features.go:13-226` — stable system-prompt constants (unchanged per req 3; memory is a new suffix, not edited here)
- `session/headless/pool.go` — `CallBlocking` / `Pool` (headless call entry point, where the frozen-snapshot load should happen once per call)
- `config/config.go:117-165` — `GetConfigDir` / `GetConfigDirForDir` (path resolution + test isolation)
- `cmd/commands/get_session.go` — file-per-command Cobra pattern to follow for `cmd/commands/memory.go`
- `main.go:363-454` (pattern), `main.go:711-717` (`rootCmd.AddCommand` wiring)
