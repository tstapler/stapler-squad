# Stack Research: remote-detection-manifests

Agent 1 (Stack). Scope: Go HTTP client patterns, version-compare + atomic local caching,
fsnotify reuse, and existing "fetch at startup, fall back to bundled" precedent.

## 0. Critical prerequisite finding — the premise is currently false on `main`

The task brief says to layer this on top of the "ALREADY-SHIPPED `detector-plugins` TOML
loader (files: `session/detection/registry.go`, a plugin loader file,
`session/detection/pattern_set.go`)". That is **not accurate as of `main` HEAD
(`8cbddebab`)**:

- `session/detection/registry.go` (14 lines) only registers the five hardcoded built-in
  binary detectors (`binaries.NewClaudeDetector()` etc.) — no TOML/plugin code path exists
  in it.
- `session/detection/pattern_set.go` (146 lines) is an unrelated, older type (`PatternSet`,
  regex-matching primitive for status detection) — verified via
  `git log --oneline --all -- session/detection/pattern_set.go`, its history goes back to
  commit `52c7390cf` ("perf(detection): remove PatternSet mutex") and earlier, predating the
  detector-plugins work entirely.
- The actual plugin-loader files named in the detector-plugins commit
  (`session/detection/plugins.go`, `session/detection/detector_snapshot.go`,
  `session/detection/plugin_watcher.go`) **do not exist anywhere in the working tree**
  (`ls` confirms `No such file or directory` for all three).
- `git merge-base --is-ancestor 3c25e94f9 HEAD` → **NO**. Commits `3c25e94f9` /
  `005e75827` (the ones requirements.md cites as "shipped ... merged 2026-08-02") exist only
  on branch `backlog/stapler-squad-detector-plugins-recovery2`, which is **not merged into
  `main`**.
- `gh pr view 307 --repo tstapler/stapler-squad` confirms: PR #307
  ("feat(detection): user-extensible TOML agent-detector plugins") is **`state: CLOSED`,
  `mergedAt: null`** — it was closed, not merged.

**Implication for this project**: `project_plans/detector-plugins/requirements.md`'s
"already shipped" framing and its 90-day checkpoint (both cited in this project's own
requirements.md) rest on a PR that was never merged. There is currently no local TOML
plugin schema/loader on `main` to layer a remote-fetch feature on top of. This is a
plan-blocking discrepancy — worth flagging to the requirements/plan phases before
architecture work proceeds, since "reuse the existing detector-plugins TOML schema" has
no implementation to reuse yet. (The design intent/schema likely still exists in
`project_plans/detector-plugins/` planning docs and can be resurrected from
`backlog/stapler-squad-detector-plugins-recovery2`, but that's a decision for planning,
not something this research phase should silently paper over.)

The remainder of this doc answers the stack questions as asked, independent of that gap —
they apply whether the local TOML loader is resurrected first or built alongside the
remote-fetch layer.

## 1. Existing Go HTTP client patterns in this repo

No dedicated HTTP client library is used anywhere — every network caller uses stdlib
`net/http` directly. Representative examples (all outside `vendor/`, non-test):

| File | Pattern |
|---|---|
| `github/http_client.go` | Package-level shared `*http.Client` with fixed `Timeout` (`ghHTTPClient = &http.Client{Timeout: 30 * time.Second}`), `http.NewRequestWithContext`, token cached in `sync/atomic.Value` + `atomic.Int64` timestamp with a 1-minute TTL |
| `server/services/anthropic_limits_client.go` | Per-struct `*http.Client{Timeout: 10 * time.Second}`, `sync.Mutex`-guarded `cached` field of the result type, returns the cached value with an updated `Available:false` / `LastErrorCode` on any request or decode failure — i.e. **fetch-with-fallback-to-last-good-value**, not fetch-or-error |
| `server/services/gemini_limits_client.go`, `server/services/anthropic_client.go` | Same shape as above, one client per provider |
| `server/services/domain_checker.go` | Another direct `net/http` caller |
| `session/backlog_plugin_github_prs.go`, `session/backlog_plugin_github.go` | GitHub REST calls via the shared `github` package client above |
| `session/cdp/manager.go` | CDP/Chrome DevTools Protocol over HTTP |

**Takeaway**: the idiomatic pattern here is a package/struct-scoped `*http.Client` with an
explicit `Timeout` (never the zero-value default client), `http.NewRequestWithContext` so
callers can bound/cancel, and — critically for this feature — the
`AnthropicLimitsClient`/`GeminiLimitsClient` fetch-with-cached-fallback shape is the closest
existing analog to "fetch remote manifest, fall back to last-known-good on any failure."
Reuse that shape (mutex-guarded cached struct, update only on success, always return
*something* usable) rather than inventing a new resilience pattern. No retry/backoff
library (no `github.com/cenkalti/backoff`, no `hashicorp/go-retryablehttp`) is present in
`go.mod` — none of the existing callers retry, they just fail fast and fall back to cache.

No `otelhttp`-wrapped client was found in these examples despite
`go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` being a dependency — that's
used for the *inbound* server, not outbound calls like this one; not required to match.

## 2. Version comparison + local caching + atomic write

**No third-party file-caching or atomic-write library exists in `go.mod`** (no
`google/renameio`, no `natefinch/atomic`, etc.) — every atomic-write site in the codebase
(20 files, e.g. `config/state.go`, `config/config.go`, `session/tokens/store.go` via its
underlying stores, `server/notifications/store.go`, `server/services/rules_store.go`) uses
the same hand-rolled stdlib idiom. Canonical example, `config/state.go:293-304`
(`saveToDiskWithoutLocking`):

```go
tmpPath := statePath + ".tmp"
if err := os.WriteFile(tmpPath, data, 0644); err != nil {
    return fmt.Errorf("failed to write temporary state file: %w", err)
}
if err := os.Rename(tmpPath, statePath); err != nil {
    os.Remove(tmpPath) // best-effort cleanup
    return fmt.Errorf("failed to atomically update state file: %w", err)
}
```

This is the pattern to reuse for the remote-manifest cache file(s): write to `<name>.tmp`
(or a `os.CreateTemp` in the same directory — same-filesystem is required for `os.Rename`
to be atomic), then `os.Rename` over the real path, with best-effort `os.Remove` of the tmp
file on failure. No new dependency needed.

For **version comparison**, no existing semver library is in `go.mod` (no
`Masterminds/semver`, no `hashicorp/go-version`). Given the detector-plugins TOML schema
(per requirements.md) already defines a `version` field per manifest, a simple
string/integer monotonic comparison (or embedding a `fetched_at`/ETag-style caching header)
is more consistent with existing repo conventions than adding a semver dependency — e.g.
mirror how `server/services/anthropic_limits_client.go` tracks `FetchedAt time.Time` on its
cached struct, or use HTTP `ETag`/`If-None-Match` conditional GETs (stdlib `net/http`
supports this natively — no library needed) to avoid re-downloading/re-validating unchanged
manifests.

## 3. fsnotify — already a real dependency, reusable

`github.com/fsnotify/fsnotify v1.9.0` **is** in `go.mod` (confirmed independent of the
detector-plugins PR status above) and is actively used on `main` in 12+ non-test files,
including:

- `session/tokens/store.go` (`session/tokens/doc.go` docstring: "A background walker
  pre-parses all JSONL files on startup; fsnotify callbacks keep the cache fresh for active
  sessions" — this is close in shape to what a manifest cache directory watcher needs)
- `session/history_watcher.go`, `session/history_linker.go`
- `session/unfinished/watcher.go`, `session/unfinished/gogitstore/{store,registry,mmapwatch}.go`
- `session/mux/autodiscover.go`
- `daemon/daemon.go` (`watcher, err := fsnotify.NewWatcher()` at line ~146, inside a `go
  func()` background goroutine launched from daemon startup — a good template for a
  non-blocking startup watcher)
- `server/auth/setup.go`

**Can the remote-fetch cache reuse the same watched directory as detector-plugins'
`~/.stapler-squad/detectors/*.toml`?** Architecturally yes, if/when that directory exists
(see §0 — it doesn't yet on `main`). The requirements' own precedence rule — "built-in →
local user `.toml` → remote-fetched (local always wins)" — argues for a *separate*
subdirectory (e.g. `~/.stapler-squad/detectors/remote/` or a top-level
`~/.stapler-squad/detector-manifests-cache/`) rather than writing fetched files directly
into the user-authored `detectors/` directory: mixing fetched and hand-authored files in
one fsnotify-watched directory makes it harder to (a) tell which files are safe to
overwrite on next fetch vs. must never be touched by automation, and (b) apply "local always
wins" without parsing filename/origin metadata. A second watcher instance (or a second
watched path added to the existing `fsnotify.Watcher`) is cheap — `fsnotify.NewWatcher()` is
already called per-subsystem elsewhere (daemon, tokens store, unfinished-work watcher) rather
than shared as one global watcher, so following that convention (a dedicated watcher for the
remote-cache directory) is consistent with existing style, not a new pattern.

## 4. Existing "fetch at startup, fall back to bundled" precedent to reuse

No exact match exists (no update-checker, no remote-config-with-embedded-fallback pattern
today). The closest structural precedents, in order of relevance:

1. **`AnthropicLimitsClient`/`GeminiLimitsClient` fetch-with-cached-fallback** (§1) — the
   resilience shape (mutex-guarded last-good value, degrade on failure, never block the
   caller on a failed fetch) is the right model for "fall back to cached/bundled on
   failure."
2. **`daemon/daemon.go`'s background-goroutine + fsnotify-watcher startup pattern** (`go
   func() { ... fsnotify.NewWatcher() ... }()`) — the right model for "must not block
   startup/session-hot-path latency; async background refresh preferred." Launch the fetch
   in a goroutine from `main.go`/`daemon.go` init, the same place `InitPlugins` would have
   been wired in per the (unmerged) detector-plugins commit message ("`InitPlugins` wired
   into `main.go` (idempotent via `sync.Once`)") — that `sync.Once`-guarded init-from-main
   convention is worth carrying over even though the plugin loader itself isn't merged yet.
3. **`session/tokens` package** — background walker pre-populates a cache on startup, then
   fsnotify keeps it warm; same shape needed here (seed from bundled/cached manifests
   synchronously and fast, then let the network fetch update the cache asynchronously
   in the background without blocking any detector resolution in the hot path).

No embedded-bundled-fallback (`go:embed`) precedent was found for *this* feature, but
`go:embed` is stdlib and trivial to add for "ship the last-known-good manifest set inside
the binary" if the design calls for a true zero-network fallback beyond the on-disk cache.

## Summary of dependencies needed vs. already available

| Need | Already in `go.mod`? | Action |
|---|---|---|
| HTTP client w/ timeout | Yes (`net/http` stdlib, used everywhere) | Reuse pattern from `github/http_client.go` / `anthropic_limits_client.go` |
| Retry/backoff | No | Not used elsewhere either — match existing "fail fast, fall back to cache" convention, skip adding a retry lib |
| Atomic file write | No (stdlib only) | Reuse `os.WriteFile(tmp)` + `os.Rename` idiom from `config/state.go` |
| Version/semver compare | No | Prefer simple field comparison / HTTP ETag over adding a semver dep, consistent with repo's zero-extra-dep convention here |
| fsnotify | **Yes**, `v1.9.0`, heavily used | Reuse directly; give the remote cache its own watched subdirectory, not the same one user `.toml` files live in |
| TOML parsing | **No** — not on `main` (see §0) | Blocked on resolving the detector-plugins merge status first |
