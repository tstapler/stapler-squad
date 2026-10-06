# Architecture Research: Remote-Fetched Detector Manifests

## 0. Load-bearing correction: `detector-plugins` is NOT merged to `main`

The brief for this item states detector-plugins "shipped 2026-08-02 as commits
`3c25e94f9`/`005e75827`." **This does not hold up under verification** and materially
changes what "integration point" means for the rest of this document:

- `git merge-base --is-ancestor 3c25e94f9 HEAD` (repo HEAD = `8cbddebab`) → **not an
  ancestor**. Same result for the PR's final commit `32f504c8`.
- `git log --oneline main` has zero commits touching `session/detection/plugins.go`,
  `detector_snapshot.go`, or `plugin_watcher.go`.
- `grep -n "pelletier/go-toml" go.mod` → no match; `grep -rln "InitPlugins\|EnsurePluginDir\|ResolveDetectorForProgram" --include=*.go .` (excluding `.claude/worktrees/`) → **no match anywhere in `main`**.
- The GitHub PR (`gh pr view 307 --repo tstapler/stapler-squad`) is **CLOSED, not
  merged**. The closing comment (2026-08-02) reads: *"Closing as superseded: this
  branch's last known commit (32f504c8...) is already present on main, so this item's
  work has already shipped through another path."* That claim is contradicted by every
  check above — `32f504c8` is not an ancestor of `main`, and none of its symbols exist
  in `main`'s tree.

So: `project_plans/detector-plugins/research/architecture.md` and its four ADRs are a
**real, reviewed, Accepted design** (and a working implementation exists — see §1
below) — but as of this research pass, **that code lives only in a git worktree
(`.claude/worktrees/agent-a81a7aa22827ecb09/session/detection/`) and a closed,
unmerged PR, not in `main`**. This research treats that implementation as the
integration target because it is the only concrete, reviewed design available and
because re-landing it is presumably a prerequisite this project doesn't own — but the
plan phase must flag, as its own risk item, that **the remote-fetch layer cannot be
built until detector-plugins (or something occupying the same file/symbol shape) is
actually on `main`**, and that the PR-closure claim above should be corrected or
re-investigated before anyone assumes otherwise.

All file:line citations below are to the working implementation at
`.claude/worktrees/agent-a81a7aa22827ecb09/session/detection/*.go` (identical in
shape to what `project_plans/detector-plugins/research/architecture.md` predicted,
confirming that research was accurate) — paths given as
`session/detection/<file>.go` for readability, since that's what they'll be once
merged.

## 1. What actually exists (confirmed by reading the implementation, not just the ADRs)

Four files, matching the design in `project_plans/detector-plugins/decisions/ADR-002-registry-level-snapshot-not-statusdetector-yaml-path.md`:

- **`session/detection/registry.go:8-17`** — `DefaultRegistry()`, unchanged built-ins-only constructor.
- **`session/detection/registry.go:32-53`** — `MergedRegistry(builtins *DetectorRegistry, plugins []BinaryDetector) *DetectorRegistry`: seeds a fresh registry from `builtins.Names()` via `Upsert` (`:34-38`), then upserts every plugin over it (`:40-50`), logging when a plugin's `Name()` collides with an already-present entry (`:42-48`). **Key property for this item: `Upsert`-based construction is transitive** — calling `MergedRegistry` twice, feeding the first call's output in as the new `builtins` argument, produces correct 3-layer last-write-wins precedence with zero changes to this function's signature.
- **`session/detection/detector_snapshot.go:34-37`** — the swap primitive:
  ```go
  var (
      activeSnapshot  atomic.Pointer[detectorSnapshot]
      snapshotWriteMu sync.Mutex
  )
  ```
  Exactly the `pipelineModeCache` pattern (`session/pipeline_engine.go:123-140`) that `project_plans/detector-plugins/research/architecture.md:136-165` identified as the precedent to reuse — already built, already in this exact shape.
- **`session/detection/detector_snapshot.go:168-217`** — `rebuildSnapshot(ctx, dir)`: locks `snapshotWriteMu` for the whole build-then-store sequence (`:177-178`), calls `LoadPluginDir(dir)` (`:180`), treats a directory-level scan error as fatal-to-this-reload-only — **logs and returns, leaving the previous `activeSnapshot` untouched** (`:182-190`) — treats every other `PluginLoadError` as a per-file skip (`:192-198`), then builds `builtins := DefaultRegistry()`, a `provenance` map, calls `MergedRegistry(builtins, asBinaryDetectors(detectors))` (`:209`), builds the snapshot, and does the single `activeSnapshot.Store(snap)` (`:211`).
- **`session/detection/plugin_watcher.go`** — `PluginWatcher.watchLoop` (`:76-146`): one goroutine owns the fsnotify `Events`/`Errors` channels, a 200ms debounce timer (`pluginReloadDebounce`, `:17`) reset on every qualifying event (`:124-130`), and a 60s periodic ticker (`pluginRescanInterval`, `:19-23`, `:95-96`) as a fsnotify-unavailable/missed-event safety net. Both the debounce fire and the ticker fire call the same `rebuildSnapshot(ctx, w.dir)` (`:140`, `:143`).
- **`session/detection/plugins.go:370-376`** — `PluginDir()` returns `filepath.Join(cfgDir, "detectors")`, `cfgDir` from `config.GetConfigDir()` — never raw `os.UserHomeDir()`, so it inherits `STAPLER_SQUAD_TEST_DIR`/`STAPLER_SQUAD_INSTANCE` isolation.
- **`session/detection/plugins.go:486-...`** — `InitPlugins(ctx)`, guarded by a package-level `*sync.Once` (`:472`) so a second call is a documented no-op; on the `STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS` kill switch it returns immediately (`:488-491`); otherwise `EnsurePluginDir()` (`:493`) then (truncated in the excerpt read, but per the PR description) an initial `rebuildSnapshot` and `StartPluginWatcher`. Called once from `main.go`, after logging init, before the daemon/web-server split — matching ADR-002 §3's plan exactly.
- **Validation/resource-cap machinery** — `parsePluginFile` (`plugins.go:50`), `validatePluginFile` (`plugins.go:204`), `PluginLoadError` (`plugins.go:176-198`) enforce ADR-004's three caps (`maxPatternsPerPlugin`, `maxRegexLength`, `maxPluginFileSize`) and produce a `[path, field, reason]`-shaped error consumed uniformly by `rebuildSnapshot`.

This is a complete, working answer to "how does a hot-reloadable local registry
work" — the remote-fetch layer's job is to become a **second input** to
`rebuildSnapshot`/`MergedRegistry`, not to reinvent any of the above.

## 2. Concrete integration points

### 2.1 `MergedRegistry` — reuse unchanged, call it twice

`registry.go:32-53`'s `Upsert`-based last-write-wins semantics compose. For
precedence **built-in → remote-fetched → local user `.toml`** (local always wins,
per this item's requirement), the merge in `rebuildSnapshot` becomes two calls
instead of one:

```go
builtins := DefaultRegistry()
withRemote := MergedRegistry(builtins, asBinaryDetectors(remoteDetectors))   // built-in < remote
final := MergedRegistry(withRemote, asBinaryDetectors(localDetectors))       // remote < local
```

No change to `MergedRegistry`'s signature or body. This is the same trick ADR-002
already uses to layer plugins over built-ins — just applied one more time.

### 2.2 `rebuildSnapshot` — generalize from one directory to two

`detector_snapshot.go:168-217` currently takes a single `dir` and calls
`LoadPluginDir(dir)` once. It needs to become (conceptually)
`rebuildSnapshot(ctx, localDir, remoteCacheDir string)`, calling
`LoadPluginDir` **twice** — once per directory — since `LoadPluginDir`
(`plugins.go:517`) is already a pure "scan this directory, return
`[]*PluginDetector` + `[]PluginLoadError`" function with no assumption baked in
about *why* the directory has files in it. The remote cache directory (§2.4) is
scanned with the exact same function, get the exact same per-file validation
(`validatePluginFile`, the three ADR-004 resource caps) and the exact same
failure-isolation behavior (one bad cached file → log and skip, not abort) for
free.

The provenance-building loops (`detector_snapshot.go:200-208`) need a third loop
for remote detectors, so `DetectorProvenance()` (`:142-148`) — already designed as
a "which file won" diagnostic per ADR-002's consequences — can distinguish
`""` (built-in) / a local `.toml` path / a remote cache file path (and, ideally,
the manifest's declared remote source URL, if `PluginDetector`'s provenance is
extended to carry it — a plan-phase field decision, not an architecture one).

### 2.3 `pattern_set.go` — untouched

Confirmed no change needed: `NewPatternSet` (`pattern_set.go:26-32`) and
`MatchLines`'s fixed priority chain (`:69-141`) operate on
`dtypes.StatusPatterns` values regardless of where those values came from.
Remote-fetched patterns go through the identical `toStatusPatterns` →
`NewPatternSet` compile step (`plugins.go:126-176` → `pattern_set.go:26-32`) as
local plugin files. This is the same conclusion `project_plans/detector-plugins/decisions/ADR-002-registry-level-snapshot-not-statusdetector-yaml-path.md:64-71` reached for local plugins, and it holds unchanged for remote ones: **no new detector-matching code path is warranted.**

## 3. Never block startup: goroutine + timeout + fire-and-forget swap

`InitPlugins(ctx)` (`plugins.go:486`) today does `EnsurePluginDir` → an initial
`rebuildSnapshot` (local-disk-only, fast) → `StartPluginWatcher` (spawns its own
goroutine, `plugin_watcher.go:67`, and returns immediately) — the call site in
`main.go` is **not** blocked on the watcher, only on one local directory scan.

The remote layer must preserve that shape but the "fast local scan" step for the
remote side is different: at startup there is no network I/O in the hot path at
all —

1. **Synchronous, at `InitPlugins`-time (or a sibling `InitRemoteManifests(ctx)`
   called right after it):** call the *same* `rebuildSnapshot`-generalized-to-two-dirs
   from §2.2, where `remoteCacheDir` is scanned exactly like `localDir` — this is
   local disk I/O only (whatever was cached from the last successful fetch, or
   nothing on first run), same cost profile as the existing local-plugin scan.
   **No network call happens here.** This satisfies "must not block the
   session-startup hot path" structurally, not by racing a timeout against
   startup.
2. **Asynchronous, a separate goroutine started after step 1 returns:**
   ```go
   go refreshRemoteManifests(ctx, remoteCacheDir)
   ```
   `refreshRemoteManifests` does the actual network fetch under a bounded
   `context.WithTimeout(ctx, remoteFetchTimeout)` (mirroring this repo's existing
   HTTP client convention — `github/http_client.go:17`,
   `server/services/anthropic_client.go:37`,
   `server/services/gemini_limits_client.go:37` all construct a package/struct-level
   `&http.Client{Timeout: N * time.Second}` rather than relying solely on
   context deadlines; the remote-manifest fetcher should do the same, with the
   request-level `ctx` still threaded through for shutdown cancellation).
3. **On successful fetch + validate + version-compare (§4):** write the new
   manifest into `remoteCacheDir` via a temp-file-then-`os.Rename` (atomic on the
   same filesystem) — **not** a direct `os.WriteFile` to the final name, because
   unlike a human editor's local `.toml` save (which the existing debounce timer
   already tolerates being "a little racy"), this write is program-to-program and
   could otherwise race a concurrent `LoadPluginDir` scan mid-write. Then call the
   two-dir `rebuildSnapshot` directly (no fsnotify round-trip needed — the fetcher
   already knows precisely when it changed something).
4. **On any failure** (network error, timeout, non-200, parse/validation
   rejection, or "fetched version is not newer than cached version"): log and
   return. `activeSnapshot` and `remoteCacheDir`'s on-disk contents are both left
   exactly as they were — this is the same "previous snapshot remains
   authoritative" behavior `rebuildSnapshot` already implements for a
   directory-scan failure (`detector_snapshot.go:182-190`), just triggered by a
   network-layer failure instead of a filesystem-layer one.

No channel or callback is needed to report the result back to any caller: nothing
in scope for this item consumes a return value from the background refresh (no
"reload now" RPC is in the requirements) — this is the exact carve-out
`project_plans/detector-plugins/research/architecture.md:172-181` already
documented for `rebuildSnapshot`'s "return the locally-computed value" question
under `.claude/rules/go-double-checked-locking.md`: it only binds a caller that
needs the result back, and there isn't one here.

## 4. Data flow / consistency for the concurrent swap

**No new concurrency primitive is needed.** The remote-fetch goroutine is just
another writer serialized through the *already-existing*
`activeSnapshot atomic.Pointer[detectorSnapshot]` +
`snapshotWriteMu sync.Mutex` pair (`detector_snapshot.go:34-37`), for the exact
reason the doc comment there gives: holding the mutex only around the final
`Store` would let a slower-started-but-slower-finishing writer's stale data
land after a faster writer's fresh data. With three possible writers now — the
initial startup rebuild, the fsnotify-triggered local rebuild, and the
network-triggered remote rebuild — that lost-update risk is exactly as real as
it was with two, and the existing whole-sequence mutex already closes it. No
per-source lock, no separate "remote snapshot" pointer to reconcile against
the local one — one map, one pointer, one mutex, three possible callers of
`rebuildSnapshot`.

Reads are unaffected: `lookupBinaryDetector` (`detector_snapshot.go:102-109`)
and `ResolveDetectorForProgram` (`:128-136`) already re-`Load()` the pointer on
every call — per `project_plans/detector-plugins/research/architecture.md:234-244`'s
finding, no session object or long-lived goroutine caches the whole registry, so
a remote-triggered swap takes effect on the very next `DetectForProgram` call
for every in-flight session simultaneously, with no per-session staleness to
reconcile. This property was established for the local-plugin case and carries
over to the remote case without modification, because the read path has no idea
(and doesn't need to know) which of the three writers produced the snapshot it's
reading.

## 5. Where cache state lives: a separate directory, not `~/.stapler-squad/detectors/`

**Recommendation: `config.GetConfigDir()/detectors-remote-cache/`, a sibling of
`detectors/` (`plugins.go:370-376`'s `PluginDir()`), not a subdirectory of it and
not the same directory.**

Reasons, each tied to something already decided or already built:

1. **Trust boundary must stay structurally visible.** ADR-004's entire threat
   model (§1: "arbitrary regexes only... no file/network access from plugin
   content") is scoped to *locally-authored* files — content the machine's own
   user typed. Remote-fetched content is a strictly different trust tier (this
   item's own premise: "new trust boundary... spoofing/compromise risk"). Mixing
   both kinds of file in one directory means `ls ~/.stapler-squad/detectors/`
   can no longer tell a user (or a future auditor) which files they wrote and
   which arrived over the network — a `find ~/.stapler-squad -maxdepth 1 -type d`
   split keeps that distinction free.
2. **`PluginWatcher` must not fsnotify-react to cache writes.**
   `plugin_watcher.go:60` calls `fw.Add(dir)` on exactly the directory passed in,
   non-recursively (matching ADR-004 §3's "no recursive descent" rule). If the
   remote cache lived inside `detectors/`, every atomic cache write from §3 step
   3 would fire a `fsnotify.Write|Create|Rename` event, debounce, and trigger a
   *second*, redundant `rebuildSnapshot` on top of the one the fetcher already
   triggers directly — harmless but wasteful, and it muddies "why did a reload
   just happen" in the logs (`log.Info("detector plugins loaded", ...)`,
   `detector_snapshot.go:214`, would fire twice per remote update with no way to
   tell which trigger caused which). A separate, unwatched directory means the
   remote fetcher is the *only* writer that ever triggers a rebuild from remote
   content, which it already does directly and synchronously in §3 step 3 — no
   watcher needed on that directory at all.
3. **Precedence collision detection stays simple.** `validatePluginFile`
   presumably enforces "no collision with another *user* file" within one
   directory scan (per `project_plans/detector-plugins/research/architecture.md:270`'s
   EventStorming row). If remote and local files shared a directory, that
   same-directory collision check would need to start distinguishing
   remote-vs-local provenance to know whether a same-`id` collision is a real
   authoring mistake (two local files) or expected precedence override
   (a local file intentionally shadowing a remote one) — an extra branch for no
   benefit, when two directories already partition that for free: collision
   detection stays per-directory (unchanged), and precedence between directories
   is handled purely by merge order (§2.1), never by collision detection.
4. **Derivation matches the existing convention exactly.** Both directories come
   from `config.GetConfigDir()` (`config/config.go:117-119`), so both
   automatically inherit `STAPLER_SQUAD_TEST_DIR`/`STAPLER_SQUAD_INSTANCE`
   isolation — no new environment-variable plumbing required, matching
   `plugins.go:363-369`'s documented rationale for why `PluginDir()` goes through
   `config.GetConfigDir()` instead of `os.UserHomeDir()`.

## 6. Trust boundary: what ADR-004 covers, and the one thing it doesn't

ADR-004 (`project_plans/detector-plugins/decisions/ADR-004-plugin-trust-boundary-and-resource-caps.md`)
already settles the **resource-consumption** threat model for arbitrary regex
content — Go's `regexp` compiles to RE2 (linear-time, no backtracking, no
catastrophic-blowup vector), and the three caps
(`maxPatternsPerPlugin`/`maxRegexLength`/`maxPluginFileSize`) bound compile/memory
cost. **That reasoning is source-independent — a regex fetched over HTTPS
compiles through the identical `regexp.Compile` call as one typed by a local
user, so nothing about ADR-004 §1–2 needs to change for remote content.**

What ADR-004 does *not* cover, because local-file authorship made it moot: a
**semantic-spoofing** threat, which this item's brief names directly — remote
detection patterns are not just "match this regex," they carry a `status` field
(`ready`/`processing`/`needs_approval`/`input_required`/`error`/.../`success`)
that downstream UI treats as an approval gate. A compromised or malicious
remote source doesn't need a regex-engine exploit to cause harm; it only needs
to publish a manifest where a pattern that should be tagged `needs_approval` is
tagged `success` instead (or vice versa, to spam false approval prompts). RE2
safety and the resource caps say nothing about this — it's an integrity/data-
trust problem, not a compute-bound problem, and needs its own answer at the ADR
phase. Concrete inputs for that ADR (not decided here):

- **Transport**: HTTPS-only fetch, no plaintext fallback — table-stakes, cheap.
- **Pinning vs. floating**: fetching a specific content hash / pinned commit SHA
  (e.g. a GitHub raw URL at a tagged ref) vs. a floating "latest" endpoint trades
  reproducibility for freshness; the version-compare mechanism (§7) needs
  *something* stable to compare against regardless of which is chosen.
- **Reuse, don't rebuild, the validation gate**: whatever trust decision is
  made, the fetched bytes must go through the *same* `parsePluginFile` /
  `validatePluginFile` (`plugins.go:50,204`) pipeline as local files — not a
  parallel, remote-specific parser — so the same `DisallowUnknownFields()`
  strictness (ADR-001) and the same ten-`status`-value validation
  (`statusField`, `plugins.go:94`) apply uniformly. This is a reuse point, not a
  new decision.
- Full sandboxing / code-execution defenses remain out of scope for the same
  reason ADR-004 gives them: the plugin format has no expression language, no
  scripting, no shell-out — that property doesn't change based on where the
  bytes came from.

## 7. Version compare — flagged, not decided

No semver library exists in this repo today (`grep -n "semver\|Masterminds\|hashicorp/go-version" go.mod` → no match). ADR-003
(`project_plans/detector-plugins/decisions/ADR-003-plugin-toml-schema-v1.md:82-91`)
already reserves a `version` field on the *schema* (parser-selection semantics:
`"1"` vs. a future `"2"`) — that is a different axis from a
*manifest-content* version used to decide "is the thing I just fetched newer
than what I have cached." Whether the latter needs a full semver dependency or
can be a plain monotonic string/integer compare is a build-vs-buy question for
the next research pass, not resolved here; flagging it so the plan phase
doesn't silently assume a library that isn't in `go.mod`.

## 8. EventStorming table: intentionally omitted

Per the task framing, this is a simple problem (fetch → cache → version-compare
→ fallback), not multi-actor business logic with distinct human/system actors
and branching policies — `project_plans/detector-plugins/research/architecture.md:261-274`'s
EventStorming table earned its place there because the local-plugin flow has
several genuinely distinct failure branches worth a compact command/event view
(per-file invalid vs. directory-level error vs. override collision, each with
different retry/skip semantics). The remote-fetch layer's failure modes are
narrower and already fully described in prose in §3 step 4 and §6 (network
failure, validation failure, stale version — all three collapse to the same
"leave the previous snapshot untouched" action) — a table would repeat that
prose with no new information.

## Summary of concrete anchors for the plan phase

| Concern | File:Line (worktree; not yet on `main` — see §0) |
|---|---|
| `MergedRegistry` — reuse unchanged, call twice for 3-layer precedence | `session/detection/registry.go:32-53` |
| `rebuildSnapshot` — generalize from one dir to two (local + remote cache) | `session/detection/detector_snapshot.go:168-217` |
| Atomic swap primitive — reused as-is, no new lock | `session/detection/detector_snapshot.go:34-37` |
| `LoadPluginDir` — reused unchanged as the remote-cache-directory scanner | `session/detection/plugins.go:517` |
| Parse/validate pipeline — reused unchanged for fetched bytes | `session/detection/plugins.go:50 (parsePluginFile), 204 (validatePluginFile), 176-198 (PluginLoadError)` |
| `PluginDir()` — pattern to mirror for the new remote-cache dir | `session/detection/plugins.go:370-376` |
| `InitPlugins` — where a sibling `InitRemoteManifests(ctx)` call site would hang | `session/detection/plugins.go:486` |
| `PluginWatcher` — fsnotify pattern, explicitly NOT reused for the cache dir (§5.2) | `session/detection/plugin_watcher.go:44-146` |
| ADR-004 resource caps — reused unchanged, source-independent | `project_plans/detector-plugins/decisions/ADR-004-plugin-trust-boundary-and-resource-caps.md:51-66` |
| ADR-003 schema `version` field — different axis from manifest-content version (§7) | `project_plans/detector-plugins/decisions/ADR-003-plugin-toml-schema-v1.md:82-91` |
| Repo's existing bounded-HTTP-client convention to mirror | `github/http_client.go:17`, `server/services/anthropic_client.go:37` |
| `config.GetConfigDir()` — base for both `detectors/` and the new cache dir | `config/config.go:117-119` |
