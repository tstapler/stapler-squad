# Test I/O and Storage Isolation Strategy

This repo's tests have accreted several independent, ad hoc fixes for the same underlying
problem: a test that touches disk or a config-resolved directory can silently escape isolation
and hit real machine/user state instead of a fresh per-test fixture. This doc names the pattern
so new tests (and new state-directory helpers) follow it by default instead of rediscovering it.

## The two isolation layers that already exist

**Database**: `session.NewTestEntRepository(t)` (`session/testing.go`) backs storage with a named,
shared-cache **in-memory** SQLite database instead of a `t.TempDir()`-backed file — no disk I/O, no
WAL fsync overhead, and each call gets a uniquely-named DB so parallel tests never see each other's
data. This is the model to copy for any new storage layer: prefer in-memory over temp-file-backed
when the backing engine supports it.

**Config-resolved state directories**: `config.GetConfigDir()`/`GetConfigDirForDir()` already
implements a priority ladder — explicit `STAPLER_SQUAD_TEST_DIR` override, explicit
`STAPLER_SQUAD_INSTANCE`, automatic per-PID isolation under `go test` (`IsTestMode()`), then
workspace-mode/shared-state fallback. `config.json` and `sessions.json` always went through this.

## The bug this doc is a response to

Seven `*DirOrDefault()` methods on `Config` resolve a real filesystem directory a production
feature writes to (checkpoints, triage artifacts, headless-failure captures, backlog attachments,
prompt cache, one-off session dirs, new-project base dir). Five of them
(`HibernationCheckpointDirOrDefault`, `TriageArtifactDirOrDefault`,
`HeadlessFailureCaptureDirOrDefault`, `BacklogAttachmentDirOrDefault`, `PromptCacheDirOrDefault`)
were implemented as a **hardcoded `os.UserHomeDir()` + literal `.stapler-squad/<name>` join**,
completely bypassing `GetConfigDir()`'s isolation ladder. Every test that exercised one of these
paths — not just in `server/services`, in *any* package — wrote real files into the developer's
actual `~/.stapler-squad/<name>`, indistinguishable from files the live production service itself
writes there. Confirmed on this maintainer's machine: **661 files** in `headless-failures/`,
**27,578 files** in `triage-artifacts/`, accumulated silently over the life of the repo.

This was also the root cause of BUG-103 item 2 (`docs/bugs/fixed/`): a `require.Eventually` wait
on a file write to that shared, ever-growing, concurrently-written directory looked like a
scheduler-contention flake, but was really I/O contention on one real shared resource.

**Fixed**: those five helpers now resolve as `filepath.Join(GetConfigDir(), "<name>")`, inheriting
the exact same isolation as `config.json`/`sessions.json`. Production behavior is unchanged in the
default case — `GetConfigDir()` resolves to exactly `~/.stapler-squad` there.

`OneOffBaseDirOrDefault` and `NewProjectBaseDirOrDefault` deliberately were **not** changed the same
way — they resolve to `~/oneoff` and `~/Projects` by design (real, user-visible project
directories a human is meant to find on disk, not app state under `.stapler-squad`), so routing
them through `GetConfigDir()` would be a behavior change, not a bug fix. Any test that creates a
real one-off/new-project directory should set `cfg.OneOffBaseDir`/`cfg.NewProjectBaseDir` to a
`t.TempDir()` explicitly rather than relying on the default.

## The env-var boilerplate problem

Before a test can rely on `GetConfigDir()`'s isolation, it typically needs to set
`STAPLER_SQUAD_TEST_DIR` to its own temp directory — `IsTestMode()`'s automatic per-PID isolation
is not always enough on its own, because an *ambient* `STAPLER_SQUAD_TEST_DIR`/`STAPLER_SQUAD_INSTANCE`
already set in the invoking shell (e.g. left over from an earlier e2e run in the same terminal)
outranks it in `GetConfigDirForDir`'s priority order. This exact hazard was independently
rediscovered and hand-patched at 130+ call sites across this repo (`grep -rn
't.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())'`), plus ~15 more hand-rolled
`os.Getenv`/`os.Setenv`/`defer`-restore blocks in `config_test.go` alone, predating that package's
adoption of `t.Setenv`.

**Use `envtest.NewIsolatedStateDir(t)`** (`envtest/envtest.go`) for new tests instead of writing
the `t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())` one-liner again — it's the same one line,
just named and in one place. It has the same constraint as `t.Setenv` itself: call it before
`t.Parallel()` on the same `t`, or not combined with `t.Parallel()` at all.

**Use `envtest.ClearAmbientStaplerSquadStateEnv()`** from a package's `TestMain` if the package has
tests that call `config.LoadConfig()`/`SaveConfig()`/any `*DirOrDefault()` and don't all
individually isolate — this is the belt to `NewIsolatedStateDir`'s suspenders, closing the gap for
every test in the package rather than relying on each one remembering to opt in. Already wired
into `config`, `session`, and `server/services`'s `TestMain`s.

Neither is a required migration for the 130+ existing call sites — they work today. This is the
pattern to reach for in new tests, and a low-risk improvement when already touching a nearby one.

## Checklist for a new config-resolved state directory

Before adding an eighth `*DirOrDefault()` method (or any new production code path that writes to a
directory derived from `Config`):

1. Does this directory hold app state (session data, caches, diagnostics) or user-visible project
   content? App state → resolve via `GetConfigDir()`, matching the five fixed helpers. User-visible
   content (a project directory a human will `cd` into) → the `OneOffBaseDir`/`NewProjectBaseDir`
   pattern (hardcoded real-home default, but with an explicit override field a test can set) is
   correct as-is.
2. Add a test asserting the default resolves under `STAPLER_SQUAD_TEST_DIR` when set — see
   `TestStateSubdirsOrDefault_should_RouteThroughGetConfigDir_When_NoOverride` in `config_test.go`
   for the pattern. A helper with zero test coverage is exactly how the five-helper bug above went
   unnoticed for this long.

## Known remaining gap

The pre-existing pollution in `~/.stapler-squad/headless-failures` and `~/.stapler-squad/triage-artifacts`
(661 and 27,578 files respectively, on this maintainer's machine) predates the fix and was not
cleaned up automatically — tests should never delete real user directories, and a one-time cleanup
is a manual, explicit action for whoever owns that machine's `~/.stapler-squad`, not something to
script into the fix itself.
