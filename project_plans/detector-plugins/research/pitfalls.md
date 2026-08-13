# Research: Known Pitfalls — User-Supplied TOML Regex Plugin System with Hot-Reload

Scope: risks for the detector-plugins feature (requirements.md) — a filesystem-watched,
TOML-defined, regex-based status-detector plugin system merged with the built-in Go
detectors in `session/detection/`.

## 1. Regex-specific risks (ReDoS / catastrophic backtracking)

**Go's `regexp` package (used by `session/detection/pattern_set.go`) is RE2-based and
genuinely eliminates classic catastrophic backtracking.** RE2 compiles patterns to a
finite automaton and evaluates all alternatives simultaneously rather than
depth-first-with-backtracking, which gives an `O(n)` (or `O(n·m)` for pattern size `m`)
worst-case bound regardless of input. There is no pathological input that makes
`regexp.MatchString` hang the way `(a+)+$` hangs PCRE/Python `re`/JS `RegExp`. This is a
real, load-bearing guarantee — not a nice-to-have — and the requirements doc's framing
("document what protection RE2 already provides") is correct to call it out explicitly
in code comments/ADR, since a reviewer unfamiliar with RE2 internals may otherwise
insist on a timeout wrapper that isn't actually needed for backtracking safety.

**What RE2 does *not* eliminate — real footguns that remain:**

- **Compile-time cost scales with pattern complexity and count.** `regexp.Compile` still
  builds an NFA/DFA per pattern; a user file with hundreds of patterns, or patterns using
  large bounded repetition (`x{1,1000}` expands internally), can make `NewPatternSet`
  slow at load time. This repo's pattern set is compiled per-detector at construction
  (`pattern_set.go:compile()`), not per-match, so the risk is a slow *reload*, not a slow
  per-line match — but a plugin directory with many files, or one file with an
  unreasonably large `[[patterns]]` list, could still make hot-reload noticeably laggy.
  **Recommendation:** cap pattern count per plugin file (e.g. ≤50) and total compiled
  pattern length, rejecting with a clear validation error rather than silently accepting
  and eating the compile cost — cheap to enforce, and matches the requirement's own
  suggestion of a "max-pattern-length guard."
- **Matching cost is linear in input length, not constant.** RE2's guarantee is
  `O(n)`, not `O(1)` — a very long single line of PTY output (a user pasting a huge blob,
  or a runaway process dumping megabytes without a newline) multiplied across many
  compiled patterns (10 status categories × N patterns each, called via
  `MatchLines` on every detection tick) is still real CPU work per tick. The existing
  code already has partial mitigation for this shape of problem via
  `hasScreenOverwrite(rawPTY)` and priority-ordered early returns in `MatchLines`, but a
  user plugin with many patterns adds directly to this per-tick cost. **Recommendation:**
  bound the number of patterns actually compiled from plugin files in aggregate (not
  just per-file), and consider truncating the text passed to `MatchString` to a sane max
  line length before matching, independent of plugin count.
- **RE2 syntax is a subset of PCRE.** Backreferences (`\1`) and lookaround
  (`(?=...)`, `(?<=...)`) are unsupported — `regexp.Compile` returns a compile error for
  these, which the validator already needs to surface per-file (requirement #3), but
  it's worth explicitly testing this case since users familiar with PCRE/JS regex will
  reach for lookaround out of habit and need an error message that says *why*, not just
  "invalid regex."

## 2. fsnotify / hot-reload pitfalls

This repo already has two independent fsnotify consumers to learn from —
`session/unfinished/watcher.go` (`WatchDirWatcher`) and
`session/unfinished/gogitstore/mmapwatch.go` (`SharedObjectStore.packWatchLoop`) — both
instructive, including one bug-shaped gap worth not repeating.

- **Editors that save via temp-file-then-rename bypass a `Write`-only watch.** This is
  the single most-reported fsnotify pitfall (fsnotify/fsnotify#214, #255, #282, #372):
  Vim, and many other editors/tools, write a new temp file and `rename(2)` it over the
  original rather than writing in place. On Linux this surfaces as a `Rename` event on
  the *old* inode (which most naive watchers ignore) followed by a `Create` event for
  the new file with the original name — inotify then silently drops the watch on the
  renamed-away inode, so a `Write`-only handler stops seeing that filename's changes
  entirely until the watch is re-added. **This repo's own existing `WatchDirWatcher`
  (`session/unfinished/watcher.go:156`) only checks
  `event.Has(fsnotify.Write) || event.Has(fsnotify.Create)`** — it does not handle
  `Rename`, so it plausibly has this exact gap for git-dir files (lower risk there since
  git writes refs via its own rename-based atomic mechanism triggering `Create` on the
  new name, which the existing code does catch — but it's evidence this pattern gets
  missed even inside the same codebase). For the detector-plugins watcher: watch the
  **directory** (`~/.stapler-squad/detectors/`), not individual files, and react to
  `Write`, `Create`, and `Rename` events on directory entries, then always **re-read the
  full file by name** rather than trying to track it by inode/fd. Watching the directory
  sidesteps the "watch removed out from under a renamed file" failure mode entirely,
  since the directory's watch is never invalidated by children being renamed.
- **Debounce rapid-fire events.** A single logical save can emit 2–3 fsnotify events
  (e.g. `Write` + `Chmod`, or `Rename` + `Create` for atomic saves). The existing
  `packWatchLoop` in `mmapwatch.go` (lines ~65–99) is the repo's canonical debounce
  pattern: coalesce a burst into one `time.Timer` reset per event, act only after a
  quiet period (`packWatchDebounce = 200 * time.Millisecond`). Reuse this exact shape
  for the detector reload loop rather than reloading on every raw event — reloading
  and recompiling all plugin patterns per individual fsnotify event is wasteful and, for
  a rapid sequence of temp-file-rename events, could trigger a burst of reloads that
  race each other.
- **Watching a directory that doesn't yet exist.** `fsnotify.Watcher.Add()` errors if
  the path doesn't exist. Requirement #6 (directory bootstrap on first run) sidesteps
  this if bootstrap always runs before the watcher starts — but the watcher should not
  hard-fail startup if `Add()` still errors for some other reason (permissions, disk
  issue); follow `WatchDirWatcher`'s existing fallback pattern
  (`NewWatchDirWatcher`, `watcher.go:24-36`): log a warning and fall back to no watching
  or periodic polling rather than crashing the process. `WatchDirWatcher` also has a
  `periodicReWalk` safety net (60s ticker, `watcher.go:190-205`) purely to catch anything
  fsnotify missed — worth mirroring as a low-cost belt-and-suspenders re-scan (e.g. every
  60s, diff directory listing against loaded plugin set) given how many fsnotify edge
  cases exist.
- **inotify instance/watch limits (Linux-specific).** Default
  `fs.inotify.max_user_watches` is commonly 8192 per user, and
  `fs.inotify.max_user_instances` (separate limit — max number of *inotify file
  descriptors*, i.e. `fsnotify.NewWatcher()` calls, per user) commonly defaults as low as
  **128**. This feature only needs to watch a single directory
  (`~/.stapler-squad/detectors/`), so the *watch count* is a non-issue — but
  `max_user_instances` matters if stapler-squad (or the user's broader desktop
  environment — IDEs, `watchman`, other file-watching daemons) is already near the
  128-instance ceiling; creating yet another `fsnotify.Watcher()` could fail with
  `too many open files` / `no space left on device` (the actual inotify error for this
  limit; it's misleading). This is another reason to **share one process-wide watcher**
  (or reuse an existing one, if one already exists in `session/unfinished`) rather than
  spinning up a dedicated `fsnotify.NewWatcher()` just for detector plugins, and to fail
  soft (log + fallback) exactly like `NewWatchDirWatcher` already does, not fail startup.
- **Platform differences (macOS FSEvents vs Linux inotify).** fsnotify abstracts both,
  but semantics genuinely differ: FSEvents on macOS coalesces events more aggressively
  and can report events for a directory in batches after a delay, and (per
  fsnotify/fsnotify#54) has documented cases of missed `Write` events for certain save
  patterns on macOS specifically. Since this repo explicitly targets macOS + Linux
  (`bootstrap/playbook.yml`-equivalent cross-platform intent, and this repo's own
  `session/tmux/tmux_unix.go` split), the debounce window should be generous enough
  (200ms, matching the existing precedent) to absorb FSEvents' batching, and the
  periodic-re-walk fallback becomes more important on macOS than Linux, not less.

## 3. Config / plugin-system-specific pitfalls

- **Partial/torn reads mid-write.** A hot-reload watcher can fire on a `Create` event
  for a file the editor/user is still writing to (multi-write saves, or a slow network
  filesystem). Reading and parsing a half-written TOML file produces a parse error that
  the loader must treat as *this file's* error only (already required by requirement #3:
  "log-and-skip, not fail-fast for the whole process") — but a more robust mitigation is
  to debounce the read itself (see above: don't react to the first event, react after a
  quiet period) and/or stat the file for a stable size/mtime across two reads before
  parsing, since debounce alone doesn't guarantee the writer has finished (a `rename()`
  onto the final name, which most editors use for atomic saves, *does* guarantee this —
  another reason favoring the "watch dir, react to Create/Rename, re-read by name"
  approach: the file that lands under its final name via `rename()` is guaranteed
  complete, unlike one still being written to in place).
- **TOML parsing library choice affects strictness.** Go has no TOML support in
  stdlib; this repo's `go.sum` has no existing TOML dependency, so this feature
  introduces one. The two mainstream choices differ meaningfully for a
  user-authored-config use case:
  - `pelletier/go-toml/v2` has a built-in `Decoder.DisallowUnknownFields()` for strict
    decoding — a typo'd key (`binary_name` instead of `binary_names`, `staus` instead of
    `status`) errors immediately instead of silently being ignored.
  - `BurntSushi/toml` supports the same detection via `MetaData.Undecoded()` /
    `IsDefined()`, but it's opt-in code the caller must write, not a single decoder flag.
  Given requirement #3 explicitly calls for rejecting files with clear errors including
  "which field," **prefer whichever library's strict/unknown-field-detection path is
  exercised by default** — go-toml v2's `DisallowUnknownFields()` is the more
  foolproof default since it can't be forgotten. A typo'd field name is exactly the
  class of mistake a non-Go-developer TOML author will make constantly (see fail2ban
  discussion below), and unknown-field detection turns "silently does nothing" into
  "loud, actionable error" — which is precisely what requirement #3 already demands.
- **Schema evolution once users have written v1 files.** The requirements doc already
  anticipates an optional `version` field for the author's own tracking, but it's not
  wired to any compatibility behavior. Once any user has a working plugin file in the
  wild, adding a required field or renaming an existing one is a breaking change with no
  migration path. **Recommendation to carry into the plan phase:** treat the TOML schema
  as a public API the moment this ships — additive-only changes (new optional fields)
  are free; anything else needs either a version-gated parser branch or a deprecation
  window with a loud (not just logged) warning. This is a plan-phase decision, not
  something to solve in this research pass, but it should be an explicit line item so it
  doesn't get silently under-scoped.
- **Silent misconfiguration: typo'd `status` value.** Requirement #3 already requires
  rejecting an unknown `status` string outright (good — this is the "fail loud" choice,
  and it is the correct one). The prior-art evidence for *why* this must be fail-loud,
  not fail-silent, comes directly from fail2ban's failure mode: fail2ban's own docs and
  multiple GitHub issues (fail2ban/fail2ban#1737, #2485, #3305) show a large fraction of
  "my custom filter doesn't work" reports trace back to a filter that parses and loads
  fine but simply never matches anything (or matches the wrong lines) — with no
  validation step to tell the author it's dead on arrival. A typo'd `status` (`"redy"`
  instead of `"ready"`) is the direct analog for this feature and must be a load-time
  validation error, not something that surfaces only as "my agent never shows a status
  badge" days later.
- **Trust boundary / privilege of arbitrary regex content.** The requirement already
  scopes this correctly — "arbitrary regexes only (no code execution, no shell-out,
  no file/network access from plugin content)" — and RE2's design (no backreferences,
  no lookaround, no recursive constructs) means there's no known regex-only sandbox
  escape or code-execution vector via a malicious pattern in Go's `regexp`. The
  remaining bounded risk is pure resource consumption (covered above), not privilege
  escalation — worth stating explicitly in the plan/ADR so a future reviewer doesn't
  over-engineer a sandboxing layer for a threat that doesn't exist at the regex-engine
  level.
- **`id`/`binary_names` collision handling surprising users.** Requirement #4 says a
  user plugin whose `binary_names` matches a *built-in* detector's binary name
  overrides that built-in — reasonable, and should be logged at info/warn level so it's
  discoverable, not silent (a user should be able to tell *why* their plugin took
  effect over the built-in `claude` detector, especially if their pattern behaves worse
  than the built-in one and they need to debug it later). Requirement #3 separately says
  `id`/`binary_names` collisions *between two user plugin files* must be rejected with a
  clear error — this needs a defined tie-break for "reject which one": likely "first
  loaded wins, second is rejected with an error naming both files," since directory
  iteration order (readdir order) is not alphabetical or creation-order on most
  filesystems and picking a winner silently would be non-deterministic across restarts.
  **Recommendation:** sort plugin files by name before loading so collision winners are
  at least deterministic and reproducible, even if the "first wins" choice itself is
  somewhat arbitrary.

## 4. Concurrency pitfalls specific to Go

- **The reload-vs-read race is exactly the shape this repo already has a documented
  precedent for**, per `.claude/rules/go-double-checked-locking.md` and its canonical
  implementation in `session/git/worktree_git.go`'s `IsDirtyWithHint` — but note the
  precedent there is about *not discarding a locally-computed value* when returning from
  a lock, which isn't quite the same hazard as the detector-plugin reload path. The
  hazard here is closer to what `session/git/worktree_git.go` actually models with
  `g.isDirtyCache.Load()` / `.Store()` on an `atomic.Value`: **the whole compiled
  `PatternSet`/registry should be treated as an immutable value, swapped atomically as a
  unit on reload, never mutated in place while a detector goroutine might be mid-match.**
  `PatternSet` is already documented as "Immutable after `NewPatternSet` returns — no
  lock needed" (`pattern_set.go:9`) — this is the right invariant to preserve. The
  concrete failure mode to avoid: building the new `PatternSet` by mutating fields on
  the *existing* live object (e.g. appending to `ps.readyRegexes` in place) while a
  concurrent `MatchLines` call is iterating that same slice — a classic unsynchronized
  read/write race on a slice, invisible until `-race` or production flakiness. The safe
  pattern: build a brand-new `PatternSet`/`DetectorRegistry` value off to the side from
  the reloaded plugin files plus `DefaultRegistry()`'s built-ins, validate it fully, and
  only then swap it into an `atomic.Pointer[DetectorRegistry]` (or `atomic.Value`) in one
  `Store()` call — mirroring `isDirtyCache`'s load/store shape, not a mutex-guarded
  in-place mutation. This also cleanly satisfies requirement #3's "a single invalid file
  must not prevent... the whole process" — the swap simply never happens if the new
  registry fails to build, and the old (still-good) registry stays live via the
  atomic pointer's last-stored value.
- **Avoid the "return the shared slot after unlock" bug this repo has already named as
  an anti-pattern.** If a mutex (rather than atomic swap) is used instead, the same file
  warns: "always return the locally-computed value, not the cache slot" — for this
  feature translated as: after a reload rebuilds the registry, any goroutine currently
  mid-detection holding a reference obtained *before* the swap should keep using that
  reference for the duration of its own call, not re-read the slot partway through and
  get a torn mix of old/new state. Passing the registry (or `PatternSet`) by value/
  pointer into the detection call at the top of each tick, rather than reaching back into
  a shared package-level variable mid-function, avoids this entirely.
- **fsnotify's own event/error channels must be drained on a single goroutine** (both
  existing precedents in this repo — `fsnotifyLoop` in `watcher.go`, `packWatchLoop` in
  `mmapwatch.go` — already do this correctly: one goroutine owns `w.Events`/`w.Errors`,
  debounces, and triggers the reload; nothing else touches the `*fsnotify.Watcher`
  concurrently). Follow the same single-owner-goroutine shape for the detector watcher
  rather than introducing a new concurrency pattern.

## 5. Real-world prior art — what breaks when non-developers author config-as-code

- **fail2ban custom filters** (closest analog: line-oriented regex against log/terminal
  output, authored by end users in a text file, no compile step beyond regex validity):
  the dominant failure mode across the linked issues is filters that are *syntactically
  valid but semantically dead* — greedy `.*` consuming past the intended match,
  unescaped literal brackets/parens in the target text, or a filter that matches in the
  standalone `fail2ban-regex` test tool but not in the live daemon because of a
  parameter-substitution difference (`__prefix_line` behaving differently when set as a
  raw string vs. through the normal config mechanism). fail2ban's ecosystem response was
  a dedicated dry-run test tool (`fail2ban-regex`) that shows exactly which log lines did
  and didn't match, and a `--print-all-missed` flag. **Direct implication for this
  feature:** ship (or at minimum plan for) a CLI/debug path — e.g. a `--test-detector
  <file> <sample-text>` flag or an admin-facing "which pattern matched, if any" trace —
  because "my plugin loaded with no errors but never detects anything" will otherwise be
  an unactionable support burden, and requirement #3's validation only catches
  structural errors (bad regex, unknown status), not "regex compiles fine but never
  matches real output," which is a distinct and probably more common failure class.
- **logstash/grok patterns**: grok's well-known pain point is debugging *why* a pattern
  didn't match at all against a log line, given regexes are often composed from
  named sub-patterns — largely orthogonal here since this feature's patterns are flat,
  standalone regexes with no macro/sub-pattern composition, so this specific pitfall
  doesn't transfer directly, but reinforces the same "match tracing" gap as fail2ban.
- **ESLint/Prettier user config plugins**: the recurring failure class is config
  *shape* drift across major versions (flat config vs. `.eslintrc` breaking migration)
  and silent precedence surprises when multiple config sources apply to the same file —
  directly analogous to this feature's "user plugin vs. built-in, which wins" question
  (already addressed above) and the schema-versioning risk (already addressed above).
- **VS Code extensions**: the closest lesson is the manifest/schema-validation-at-
  install-time pattern — `package.json` contributions are JSON-schema-validated before
  the extension is even allowed to activate, with errors surfaced in the Extensions UI,
  not just a log line the user has to go looking for. **Implication:** requirement #3's
  "clear, actionable error" should be surfaced somewhere the user will actually see it
  outside a log file tail — e.g. a status/health indicator in the stapler-squad UI
  showing "N detector plugins loaded, M failed (click for details)" — since a CLI/daemon
  log is far less discoverable to a non-developer author than an IDE's own UI is for an
  extension author. This is a UX-phase consideration to carry forward, not something to
  solve in this research pass.

## Summary of concrete recommendations to carry into `sdd:3-plan`

1. Cap per-file pattern count (and consider a total-length guard) — RE2 doesn't need it
   for backtracking safety, but compile-time and per-tick match-time cost still scale
   with pattern count.
2. Watch the **directory**, react to `Write`/`Create`/`Rename` on entries (not
   `Write`-only — this repo's own `WatchDirWatcher` only checks `Write`/`Create` and is
   itself a partial example of the gap), and always re-read the full file by name.
3. Debounce with a `time.Timer` reset per event, reusing the exact shape of
   `mmapwatch.go`'s `packWatchLoop` (~200ms), plus a low-frequency periodic re-scan
   fallback mirroring `WatchDirWatcher.periodicReWalk`.
4. Reuse a single process-wide `fsnotify.Watcher` rather than a dedicated one for this
   feature, and fail soft (log + fallback) if `fsnotify.NewWatcher()`/`Add()` errors,
   matching `NewWatchDirWatcher`'s existing fallback pattern.
5. Prefer a TOML library with built-in unknown-field rejection
   (`pelletier/go-toml/v2`'s `DisallowUnknownFields()`) so typo'd field names are a
   loud load error, not silent no-ops.
6. Treat the compiled registry/PatternSet as copy-on-write: build the new value fully
   off to the side, validate it, then swap it into an `atomic.Pointer`/`atomic.Value` in
   one `Store()` — no in-place mutation of live compiled regex slices, and no
   mutex-guarded "read the shared slot after unlock" pattern (the anti-pattern already
   named in `.claude/rules/go-double-checked-locking.md`).
7. Sort plugin files deterministically (by filename) before loading so
   collision-rejection winners are reproducible across restarts.
8. Plan for a "why didn't my plugin match anything" debug/test path (fail2ban's
   `fail2ban-regex --print-all-missed` is the direct prior art) — structural validation
   alone (bad regex, unknown status) won't catch the dominant real-world failure mode of
   a syntactically valid but practically dead-on-arrival pattern.
9. Treat the TOML schema as a public API from day one — additive-only evolution, or an
   explicit versioned-parsing/deprecation plan — since users will have written files
   against whatever ships first.
10. Surface validation errors somewhere more discoverable than a log file (UI health
    indicator), following VS Code's install-time-validation-in-the-UI precedent.

## Sources

- [Suggestion: keep watching file on rename · Issue #214 · fsnotify/fsnotify](https://github.com/fsnotify/fsnotify/issues/214)
- [Edit file but it trigger Rename Event · Issue #255 · fsnotify/fsnotify](https://github.com/fsnotify/fsnotify/issues/255)
- [Modifying a file using vi causes a "Rename" event instead of a "Write" event · Issue #282 · fsnotify/fsnotify](https://github.com/fsnotify/fsnotify/issues/282)
- [Robustly watching a single file is HIGHLY nontrivial · Issue #372 · fsnotify/fsnotify](https://github.com/fsnotify/fsnotify/issues/372)
- [fsnotify.Write is not catching updates on Mac · Issue #54 · fsnotify/fsnotify](https://github.com/fsnotify/fsnotify/issues/54)
- [Miss events when use mv to overwrite the file · Issue #80 · fsnotify/fsnotify](https://github.com/fsnotify/fsnotify/issues/80)
- [GitHub - google/re2](https://github.com/google/re2)
- [ReDoS Vulnerability Explained: Causes, CVEs, Fixes](https://safeguard.sh/resources/blog/regular-expression-denial-of-service-redos-explained)
- [The maximum limit of inotify watches has been reached – Atomicorp](https://support.atomicorp.com/hc/en-us/articles/19645954610971-The-maximum-limit-of-inotify-watches-has-been-reached)
- [Linux inotify limits - Watchexec](https://watchexec.github.io/docs/inotify-limits.html)
- [Inotify limits - Maestral](https://maestral.app/docs/inotify-limits)
- [Fail2ban won't ban a custom filter but it finds it in fail2ban-regex · Issue #1737 · fail2ban/fail2ban](https://github.com/fail2ban/fail2ban/issues/1737)
- [Nextcloud: custom filter/regex don't match · Issue #2485 · fail2ban/fail2ban](https://github.com/fail2ban/fail2ban/issues/2485)
- [Fail2ban: regex used in failregex does not work · Issue #3305 · fail2ban/fail2ban](https://github.com/fail2ban/fail2ban/issues/3305)
- [Developing Filters — Fail2Ban documentation](https://fail2ban.readthedocs.io/en/latest/filters.html)
- [go-toml v2 plan · pelletier/go-toml · Discussion #506](https://github.com/pelletier/go-toml/discussions/506)
- [toml package - github.com/pelletier/go-toml/v2 - Go Packages](https://pkg.go.dev/github.com/pelletier/go-toml/v2)
- [toml package - github.com/BurntSushi/toml - Go Packages](https://pkg.go.dev/github.com/BurntSushi/toml)

## Repo files referenced

- `project_plans/detector-plugins/requirements.md`
- `session/detection/pattern_set.go`
- `session/detection/registry.go`
- `session/detection/binary_detector.go`
- `session/unfinished/watcher.go` (`WatchDirWatcher`, `fsnotifyLoop`, `NewWatchDirWatcher`, `periodicReWalk`)
- `session/unfinished/gogitstore/mmapwatch.go` (`packWatchLoop`, `packWatchDebounce`)
- `session/git/worktree_git.go` (`IsDirtyWithHint`, `isDirtyCache` atomic.Value pattern)
- `.claude/rules/go-double-checked-locking.md`
- `go.mod` (`github.com/fsnotify/fsnotify v1.9.0` already a dependency; no TOML library present yet)
