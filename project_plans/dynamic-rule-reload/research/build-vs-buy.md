# Build vs Buy: settings.json file-watch → classifier reload

Scope per requirements.md: watch `~/.claude/settings.json` and
`<project>/.claude/settings.json` for hand-edits, and hot-reload just that
rule source into the existing `RuleBasedClassifier.ReplaceRules()`
atomic-swap. This doc covers only the new engineering surface — the file
watch + debounce + reload trigger.

## 1. Existing OSS library

`github.com/fsnotify/fsnotify` is **already a direct dependency**:

- `go.mod:16`: `github.com/fsnotify/fsnotify v1.9.0`
- `go.sum:128-129`: same version, both hashes present.
- Already imported in 5+ packages: `session/unfinished/watcher.go`,
  `session/unfinished/gogitstore/mmapwatch.go`, `session/history_watcher.go`,
  `session/history_linker.go`, `session/mux/autodiscover.go`, `daemon/daemon.go`,
  `server/auth/setup.go`, `main.go`.

This settles the question: no new dependency is needed. fsnotify is the
industry-standard OS-level watcher (inotify/kqueue/ReadDirectoryChangesW
wrapper) used by Kubernetes, Prometheus, Docker, Consul, and Viper itself —
exactly the kind of low-level, platform-specific code that should never be
hand-rolled, and it's already proven-in-production inside this repo.

**Recommendation: use `fsnotify` directly, at the currently vendored v1.9.0.
No version bump, no new dependency.**

### Alternative considered: `spf13/viper`'s `WatchConfig()`

Not a dependency — confirmed via `grep -rn "spf13" go.mod`: only
`github.com/spf13/cobra` is present, no `spf13/viper`. Viper's `WatchConfig()`
is itself just a thin wrapper around fsnotify (same debounce-on-rename
problem, same event loop) plus a large amount of unrelated machinery this
repo doesn't use — remote config providers, env-var binding, multi-format
decoding (YAML/TOML/HCL/env/flags), a global singleton config object.

Adopting viper here would mean:
1. Pulling in a large new dependency graph for a single `WatchConfig()` call.
2. Migrating `server/services/claude_settings_parser.go`'s existing
   hand-rolled JSON parsing (`ParseClaudeSettings`, `LoadClaudeSettingsRules`)
   onto viper's config model to get any benefit from the wrapper, which is
   out of scope per requirements.md ("no rebuilding file-watching for
   DB-backed rules," narrow scope).
3. Getting no capability fsnotify doesn't already give directly — viper's
   `WatchConfig` doesn't solve debounce for you either (see below); it still
   requires the caller to reason about the same atomic-rename semantics.

**Verdict: adopting viper would add a dependency layer with no net new value
for this narrow use case. Reject.**

## 2. SaaS / managed service

Not applicable — this is a local disk file-watch inside a single process.
No network service is involved.

## 3. LLM-generated implementation vs battle-tested library

- **The watcher itself (inotify/kqueue/ReadDirectoryChangesW):** must be
  fsnotify, never hand-rolled. This is exactly the class of code the
  `interface-pollution-checklist`/engineering-discipline conventions in this
  repo warn against reinventing — subtle, platform-specific, easy to get
  wrong (partial reads, watch-descriptor exhaustion, rename-vs-write
  semantics differ per OS).

- **Debounce/coalescing on top of fsnotify events:** normal and expected to
  be small bespoke code, confirmed by looking at how this repo's own live
  (non-retired) fsnotify consumer does it — `session/unfinished/gogitstore/mmapwatch.go`:

  ```go
  // packWatchDebounce coalesces bursts of pack-dir events — a single repack
  // ...
  const packWatchDebounce = 200 * time.Millisecond
  ```

  with a `timer := time.NewTimer(...)` / `timer.Reset(...)` select-loop in
  `packWatchLoop` (`mmapwatch.go:65-92`). That's ~30 lines total, mirroring
  `session/unfinished/scanner.go`'s `fsnotifyLoop` goroutine-exit pattern.
  External research corroborates this is the standard shape of the problem,
  not something fsnotify itself solves: editors save via a temp-file +
  atomic rename, which fans out into multiple CREATE/RENAME/WRITE events per
  logical save, and fsnotify's own maintainers note the library reports raw
  filesystem operations, not save intent — coalescing is left to the
  consumer by design (see fsnotify GitHub issues #372, #553, #255; no
  off-the-shelf debounce helper ships in the fsnotify module itself).

  **Confirms:** write a small (~20-40 line) debounce timer around the
  `fsnotify.Watcher.Events` channel — this is normal, expected, and matches
  existing repo precedent, not something to source externally.

## 4. Fork or adapt existing code

Two candidate templates exist in this repo, both for the *retired*
DB-rules watcher and the *current* pack-dir watcher:

### a) Retired: `server/services/rules_store.go` `WatchAndReload` (now a no-op)

Full original implementation recovered from git history, commit
`1c31f024d0b87a406f2959916e85933a9ee5820a`
(`server/services/rules_store.go:134-183` at that revision, before it was
turned into a no-op comment when DB-backed rules replaced file-backed
storage):

```go
func (s *RulesStore) WatchAndReload(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.WarningLog.Printf("[RulesStore] Failed to create watcher: %v", err)
		return
	}

	// Watch parent directory so we catch atomic renames.
	dir := filepath.Dir(s.filePath)
	if err := watcher.Add(dir); err != nil {
		log.WarningLog.Printf("[RulesStore] Failed to watch %s: %v", dir, err)
		watcher.Close()
		return
	}

	go func() {
		defer watcher.Close()
		debounce := time.NewTimer(0)
		if !debounce.Stop() {
			<-debounce.C
		}
		pending := false

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Name == s.filePath {
					if !pending {
						debounce.Reset(100 * time.Millisecond)
						pending = true
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.WarningLog.Printf("[RulesStore] Watcher error: %v", err)
			case <-debounce.C:
				pending = false
				before := len(s.All())
				if err := s.reload(); err != nil {
					log.WarningLog.Printf("[RulesStore] Reload failed, keeping previous rules: %v", err)
				} else {
					after := len(s.All())
					log.InfoLog.Printf("[RulesStore] Reloaded rules from %s (%d → %d rules)", s.filePath, before, after)
				}
			}
		}
	}()
}
```

This is directly relevant: it already solves "watch parent dir for atomic
renames, filter by exact filename, debounce, ctx-cancelable goroutine" for a
single-file JSON-rules reload — nearly the exact shape needed here (two
files instead of one; filter needs to match either path, or run two watches
/ watch two parent dirs).

### b) Current, in-production: `session/unfinished/gogitstore/mmapwatch.go` `packWatchLoop`

More directly reusable as a *style* template because it's live code that
passes CI today (not retired), uses the repo's current structured logging
(`log.Warn`/`log.Info` instead of the older `log.WarningLog.Printf`), and
documents its goroutine-exit convention explicitly as mirroring
`session/unfinished/scanner.go`'s `fsnotifyLoop`.

**Recommendation: adapt, don't write fresh.** Use (a)'s retired
`WatchAndReload` as the structural template (directory-watch-for-atomic-rename
+ debounce-timer + ctx-cancel select loop, extended to 1-2 paths instead of
1), but follow (b)'s current logging/lifecycle conventions
(`log.Warn`/`log.Info`, named debounce constant, comment cross-referencing
the sibling implementation) since it's the actively-maintained precedent.
Do not copy the retired code verbatim — the `log.WarningLog`/`log.InfoLog`
global-logger style it uses is the previous generation of this repo's
logging convention, superseded by the `log.Warn(msg, "key", val)` structured
style already used in the newer watcher.

## Final recommendation

- **Import: none beyond what's already in `go.mod`.** Use
  `github.com/fsnotify/fsnotify v1.9.0`, already vendored and used in 7+
  other files in this repo.
- **Reject viper's `WatchConfig()` wrapper** — not a current dependency,
  adds a large unrelated dependency graph, and provides no capability
  fsnotify doesn't already give directly for this narrow scope.
- **New dependency footprint: zero.**
- **New code:** one small file (~60-90 lines) implementing a
  directory-watch + debounce + `ReplaceRules()` callback, structurally
  adapted from the retired `RulesStore.WatchAndReload` (git history,
  `server/services/rules_store.go` at commit `1c31f024d0b`) but written in
  the current structured-logging style used by
  `session/unfinished/gogitstore/mmapwatch.go`'s `packWatchLoop`. The
  debounce timer (~100-200ms, matching both precedents) is expected,
  normal, hand-written glue — not something to source externally.

## Sources

- [fsnotify/fsnotify issue #372 — "Robustly watching a single file is HIGHLY nontrivial"](https://github.com/fsnotify/fsnotify/issues/372)
- [fsnotify/fsnotify issue #553 — "correct way to determine when file writes are finished"](https://github.com/fsnotify/fsnotify/issues/553)
- [fsnotify/fsnotify issue #255 — "Edit file but it trigger Rename Event"](https://github.com/fsnotify/fsnotify/issues/255)
- [pkg.go.dev/github.com/fsnotify/fsnotify](https://pkg.go.dev/github.com/fsnotify/fsnotify)
