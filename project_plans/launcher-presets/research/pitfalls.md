# Research: Known Pitfalls and Risks — Launcher Presets

## 1. Security

**Threat model is "trusted local config," same as `AvailablePrograms` and the existing
`profile`/alias config — not attacker-controlled in any current deployment mode.**
`GetConfigDir()` (`config/config.go:117-160`) resolves to `~/.stapler-squad/` (or
`STAPLER_SQUAD_TEST_DIR` in tests) on the same machine the server runs on; there is no
multi-tenant or remote-write path into this file today. The launcher-presets file
(`~/.stapler-squad/launcher-presets.json`) sits in the identical trust boundary. A
"shared machine" or "synced dotfiles" scenario is a real way this file *could* end up
attacker-influenced (e.g. a dotfiles repo with a compromised collaborator, or a
multi-user box where another local user can write to your home dir) — but that threat
already exists for `config.json` itself (session defaults, directory rules, aliases all
already let a local writer point sessions at arbitrary `program`/`path` values). Nothing
about presets specifically increases this exposure; requirements.md's "Out of Scope:
preset-level secrets/credential injection" is the right boundary, but there is currently
**no stated validation or sandboxing** for preset `argv[0]` or working directory beyond
non-empty-`argv`/unique-`id` (see requirements.md Must-Have). A preset of
`["rm", "-rf", "$HOME"]` would execute exactly like a hand-typed `program` string does
today — this should be called out explicitly in the plan as accepted risk (matching the
existing `AvailablePrograms`/alias trust model), not silently assumed away.

**argv-array launch avoids shell-string injection ONLY if the whole pipeline is changed to
match — it does not today.** This is the single most important and concrete finding:
`session/tmux/tmux.go:1001` builds `programWithHistory := fmt.Sprintf("env HISTFILE=%s %s",
historyPath, t.program)` and `session/tmux/tmux.go:1009` appends `programWithHistory` as a
**single bare string** to the `tmux new-session` argv (no `--` separator before it). tmux's
default behavior for a single trailing string argument is to hand it to the user's `$SHELL
-c` for interpretation — i.e. **the current `program` field is already a shell string**,
not an argv array, all the way down. `t.program string` is the type at every call site
(`NewTmuxSession(name, program string)`, `session/tmux/tmux.go:681` and siblings).

Contrast with `session/tmux/shell_handle.go:86-89`, which *does* do this correctly for the
"shell" side-panel feature: `tmux new-session -d -s {name} -c {workDir} -- /bin/sh -c
{command}` — the `--` plus explicit `/bin/sh -c` makes the shell-interpretation point
explicit and intentional, with a comment citing "SEC-4: no shell interpolation of the
command string in the tmux argument" as the reason.

**Implication for Phase 3 (Plan):** if a preset's `argv []string` (e.g.
`["codex", "--model", "gpt-5"]`) is naively `strings.Join(argv, " ")`'d into the existing
`program string` field, any argv element containing a space, quote, `$()`, backtick, or
`;` — exactly the paths-with-spaces and nested-quoting cases requirements.md calls out for
`ssh -t host '...'` — will be **re-interpreted by the shell**, silently corrupting the
launch or creating an actual injection vector if any preset argv element is ever derived
from anything less trusted than "hand-authored JSON" (e.g. a future feature that
templates in a variable). Success Criterion 3 ("launch correctly with no shell-quoting
corruption") cannot be met by joining into the existing string field — it requires either:
(a) extending `TmuxSession`'s new-session invocation to accept a real `[]string` argv and
pass it after `--` (mirroring `shell_handle.go`'s pattern), or (b) using `shlex`-style
quoting when joining (fragile, still a de-facto shell-string translation layer, and easy
to get wrong for the nested-quote `ssh -t host '...'` case specifically named in the
requirements). Option (a) is the only one that actually satisfies "no shell interpolation
of preset-supplied strings at any point in the pipeline" (requirements.md, Must Have).
This is a backend/infra change beyond just adding a proto field — flag it explicitly in
the Phase 3 plan rather than discovering it mid-implementation.

## 2. Config-Loading Pitfalls

**Reuse `LoadConfigFromPath`'s structure, but note it does NOT reject-whole-file-on-error
in the way requirements.md wants for presets.** `config/config.go:847-856`
(`LoadConfigFromPath`) does `os.ReadFile` → `json.Unmarshal` → return error on failure,
which is the right shape (whole-file parse, single error path) — reuse this pattern
directly for presets. However, `LoadConfig()` (`config/config.go:782-804`), the caller
most similar to what a presets loader would need, has a fallback: on any error other than
"file does not exist," it logs a warning and returns `DefaultConfig()` — i.e. **it
degrades to an empty/default config rather than failing loudly.** Requirements.md's
Success Criterion 4 explicitly wants the opposite for presets ("fails loudly... rather
than silently dropping presets or crashing the server"). Do not copy `LoadConfig()`'s
degrade-to-default fallback for presets; the loader needs to surface the parse/validation
error to startup logs (and per the requirements, presumably prevent partial application —
i.e. either all presets from the file load, or the file is treated as entirely invalid).

**Duplicate-ID and required-field validation has no existing precedent to copy in this
codebase.** A search of `config/*.go` found no existing "duplicate key" or "already
exists" validation helper for list-of-named-entries config (aliases, directory rules,
profiles are all keyed differently or use maps that make duplicates structurally
impossible). This will need to be written fresh: a straightforward `map[string]bool` scan
over `presets[].id` during validation, erroring on first collision with both IDs/indices
named in the error (so a user editing hand-written JSON can find the fix quickly).

**Encoding/whitespace:** no BOM-handling or encoding-detection exists elsewhere in
`config/` — `os.ReadFile` + `json.Unmarshal` already tolerates UTF-8 with no BOM
correctly and errors clearly on invalid UTF-8; no extra work needed here, just don't
add unnecessary encoding-sniffing complexity.

**Concurrent read while the file is being edited — the existing atomic-write pattern in
this codebase has a known, previously-fixed hazard that a copy-paste from `saveConfig`
would reintroduce.** `config/config.go:828-836` (`saveConfig`) writes to a **fixed**
`tmpPath := configPath + ".tmp"` and renames. This is *not* a race for presets specifically
(presets are read-only from the server's perspective — hand-edited only, per
requirements.md's Out of Scope — so there's no concurrent-writer-from-the-app scenario the
way there is for `config.json`). But if a future editor's own atomic-save also happens to
use a fixed `.tmp` suffix in the same directory as another concurrent writer, the
documented fix already exists in this repo:
`server/services/hook_injector_test.go:414-452`
(`TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON`) and
`internal/claudehooks/claudehooks.go`'s `mutate()` both call out this exact
fixed-tmp-filename hazard and fix it via a unique `os.CreateTemp` name. Not directly
applicable to presets' read path, but worth noting if the presets loader ever gains a
write-back capability (e.g. a future "save this session's launch as a preset" convenience
feature) — use `os.CreateTemp`, not `configPath + ".tmp"`, if that day comes.

The read-side race that *does* matter for presets: reading a file mid-write by an external
editor. Editors that write-in-place (not atomic rename) can be caught by `os.ReadFile`
mid-write, producing a truncated/invalid JSON snapshot. Given Success Criterion 4 ("fails
loudly"), this is actually the *safe* failure mode — a mid-write read fails JSON parsing
and surfaces as an error, rather than silently applying a half-written preset — but the
error message should make clear "this could be a transient mid-save read" if hot-reload
is implemented (see below), to avoid alarming users over an editor's normal save sequence.

## 3. Hot-Reload Pitfalls (fsnotify, Nice to Have)

This codebase has three fsnotify usages worth modeling from, and each teaches a different
lesson relevant to watching a *single config file* (a case none of the three actually
handle today — they all watch directories):

- **`session/history_watcher.go:106`** — the closest precedent for "don't miss an
  atomic-save rename": its event filter is
  `if event.Op&(fsnotify.Create|fsnotify.Rename|fsnotify.Write) == 0 { continue }` —
  i.e. it explicitly handles `Create` and `Rename`, not just `Write`. This matters because
  editors like vim/emacs with `backupcopy=no` (the common default) save by writing a new
  temp file and **renaming it over the original** — a naive `fsnotify.Write`-only watch on
  the file's inode will silently stop firing after the first external rename-based save,
  because the watch is on the *original inode*, which the rename orphans. **A presets
  watcher must either (a) watch the containing directory and filter events by filename
  (matching `history_watcher.go`'s directory-watch approach), or (b) re-`Add()` the watch
  after every `Rename`/`Remove` event to reattach to the new inode.** Watching the file
  path directly with only `Write` (as opposed to the directory) is the single most likely
  hot-reload bug: it will work in a naive manual test (`echo >> file`) and then silently
  stop working the first time the user's real editor does a normal save.

- **`session/mux/autodiscover.go:105-113`** — explicit separate `case` arms for `Create`,
  `Remove`, `Write` on each event, rather than one combined check — a cleaner pattern to
  follow than history_watcher's single combined `if`, if the presets watcher needs to log
  differently per event type (e.g. "presets file removed" vs. "presets file changed").

- **`session/unfinished/gogitstore/mmapwatch.go:26-30, 65-106`** — the **debounce**
  pattern: a single logical file-save can fire multiple raw fsnotify events in quick
  succession (temp file create, write, rename, and on some editors a subsequent
  permissions/chmod touch). `mmapwatch.go` coalesces bursts into one `refreshIndexes()`
  call via a 200ms reset-on-each-event timer (`packWatchDebounce = 200 * time.Millisecond`).
  Reloading presets on every raw event without debouncing would parse+validate the file
  multiple times per save and could log spurious duplicate "reloaded N presets" or
  transient-error messages if a burst event lands on a not-yet-fully-written temp file.
  Reuse this exact 200ms debounce shape for the presets watcher.

- **Watching a file that doesn't exist yet** — `history_watcher.go:38-44` checks
  `os.Stat(watchDir)` before creating the watcher and degrades gracefully (logs a warning,
  closes `stopped`, returns nil — no hard failure) if the directory is absent.
  `mmapwatch.go:55-59` does the same for a pack dir that "may not exist yet." The presets
  file is optional (requirements.md doesn't require it to exist — no presets is a valid
  state), so the watcher must not error/crash at startup if
  `~/.stapler-squad/launcher-presets.json` doesn't exist; it should watch the *directory*
  (which does exist, since it holds `config.json`) and fire on a later `Create` event
  when the user first adds the file — this also naturally solves "watching a file that
  doesn't exist yet" and the rename-breaks-the-watch problem in the same design (watch the
  directory, filter by filename, always).

- **Graceful degradation when fsnotify itself is unavailable**: both
  `session/unfinished/watcher.go:30-34` and `mmapwatch.go:50-54` treat
  `fsnotify.NewWatcher()` failure as non-fatal (log + fall back, don't error the caller).
  Follow the same shape for presets — hot-reload is a Nice to Have per requirements.md, so
  its unavailability on an exotic platform should never block startup or the Must-Have
  startup-load path.

## 4. Frontend Pitfalls

**Stale preset list is a real, currently-unhandled gap — not hypothetical.**
`web-app/src/lib/hooks/useAliases.ts:22-72` (the closest existing analog: a config-file-
backed list surfaced to the Omnibar) fetches **once on mount** with a `fetchTick` dependency
and exposes a manual `refetch()` — but nothing in `OmnibarContext.tsx` ever calls
`refetch()` automatically; the alias list only updates when the component holding the hook
remounts. If a `useLauncherPresets()` hook is built the same way (the natural thing to do,
for consistency), and backend hot-reload (Section 3) is implemented, the *frontend* will
still show the stale list until the Omnibar component remounts (e.g. full page reload) —
directly undermining Success Criterion 1's "see it appear... without restarting the server."
The plan should decide explicitly: either (a) poll on an interval, (b) refetch on Omnibar
open (`isOpen` transition to `true` is already tracked in `OmnibarContext.tsx`, so wiring
`refetch()` there is cheap), or (c) accept "stale until refresh" as v1 UX and say so.
Option (b) is the cheapest fix and matches user expectation ("I edited the file, then
opened the omnibar, and it's there").

**Preset selection overwriting user-typed fields is a real UX ambiguity with no existing
resolved precedent to copy verbatim.** The closest analog, `AliasPalette`'s `onSelect`
(`web-app/src/components/sessions/Omnibar.tsx:1313-1316`), operates on the raw *text input*
(`setInput(completeAlias(alias))`) rather than on structured form fields — i.e. today's
alias-selection flow rewrites what's typed in the omnibar text box, not a multi-field form
state object. Presets, per requirements.md Success Criterion 2, need to "pre-fill the
session-creation form (program, argv/flags, working directory)" — a materially different
UX shape (multiple discrete form fields, not one text string) with no existing
`onSelect`-into-`formState` precedent to lift. Concretely unresolved and worth flagging in
Phase 3: if a user has already typed a custom working directory and then picks a preset
that has its own `default_path`, does the preset silently clobber the user's typed value,
merge (preset fills only empty fields), or prompt? Recommend the plan pick "preset
overwrites all its own mapped fields unconditionally, selecting a preset is a deliberate
reset action" (simplest, matches "one-click shortcut" framing in the problem statement) and
say so explicitly, rather than leaving it to be improvised during implementation.

## 5. Flaky-Test / Config-Race Precedent in This Repo

No currently-flaky test was found under `config/*_test.go` or in the config-loading path
itself (`rg "flaky|race" config/*_test.go` returned nothing). The relevant precedent is
the closed-out bug class described in this repo's own rule
(`.claude/rules/fix-flaky-tests-dont-defer.md`) and worked example
(`server/services/hook_injector_test.go:414-452`,
`TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON`): the
**fixed-temp-filename-under-concurrent-writers** class of bug. It doesn't currently apply
to presets (read-only, no concurrent app-side writers), but if implementation adds *any*
write path back to `launcher-presets.json` (e.g. a "save current session as preset"
convenience action, explicitly out of scope per requirements.md today but a plausible
future ask), the test for that write path should be modeled directly on
`TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON` (N
concurrent goroutines writing, then assert the final file parses and contains expected
keys) rather than a single-writer happy-path test, precisely because a single-writer test
would pass while still carrying the fixed-tmp-name hazard `saveConfig`
(`config/config.go:829`) itself still has today (this hazard is pre-existing in
`saveConfig`, unrelated to this feature — noted here per
`.claude/rules/fix-flaky-tests-dont-defer.md`'s spirit of naming known debt rather than
letting it go unremarked, but fixing it is out of scope for launcher-presets since presets
don't currently write through that path).
