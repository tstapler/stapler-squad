# Debugging logs: where they live, how to filter, how to group

## Where logs live

`~/.stapler-squad/logs/`:

| File | What it is |
|---|---|
| `staplersquad.log` | Current live log, JSON-lines (one `slog` record per line: `time`, `level`, `msg`, plus arbitrary key/value fields). |
| `staplersquad-<timestamp>.log.gz` | Rotated log segments (lumberjack). Same JSON-lines format, gzipped. |
| `service.log` / `service.log.old` | Raw stdout/stderr of the systemd/launchd-managed service — startup banners, panics that happen before logging is initialized, and anything written outside the `log` package. Check here first for crash-on-boot issues the structured log never saw. |
| `debug-snapshot-*.json` | One-off diagnostic dumps (goroutine/heap snapshots), not part of the regular log stream — see `docs/how-to/profile-lockups.md`. |

Every line is a flat JSON object — pull specific fields with `jq` rather than `grep`ping text:

```bash
jq -r 'select(.level=="ERROR")' ~/.stapler-squad/logs/staplersquad.log
jq -r 'select(.session=="my-session-title")' ~/.stapler-squad/logs/staplersquad.log
```

## Log levels: global and per-package

Logging goes through the `log/` package (`slog`-based). There are two independent controls:

### Global runtime level

- Default: `INFO` (set in `log/log.go`'s `init()`), for both console and file.
- Change at runtime with no restart via the debug API:
  ```bash
  curl -X POST localhost:8543/api/debug/log-level -d '{"level":"DEBUG"}'
  curl localhost:8543/api/debug/log-level   # read current level
  ```
- In code: `log.SetRuntimeLevel(log.DEBUG)` / `log.GetRuntimeLevel()`.

### Per-package level overrides (log4j/logback-style hierarchy)

Flipping the global level to `DEBUG` floods the log with every package's debug
output — usually you only want it for the one package you're chasing a bug
in. `log/package_level.go` implements the same hierarchical-override model
Java logging frameworks use: set a level on a package path, and it applies to
that package and everything under it, unless a more specific sub-path has its
own override.

**At startup**, via the `STAPLER_SQUAD_LOG_LEVELS` env var — a comma-separated
list of `package=level` pairs, paths relative to the module root:

```bash
STAPLER_SQUAD_LOG_LEVELS="session/tmux=debug,server/services=warn" ./stapler-squad
```

**At runtime**, via the debug API:

```bash
# Turn on DEBUG for one package without touching the global level
curl -X POST localhost:8543/api/debug/log-level/packages \
  -d '{"package":"session/tmux","level":"DEBUG"}'

# See what's currently overridden
curl localhost:8543/api/debug/log-level/packages

# Remove an override (falls back to the global level, or a less-specific
# ancestor override — e.g. clearing "session/tmux" while "session" has its
# own override falls back to that)
curl -X DELETE localhost:8543/api/debug/log-level/packages -d '{"package":"session/tmux"}'
```

In code: `log.SetPackageLevel("session/tmux", log.DEBUG)`, `log.ClearPackageLevel(...)`, `log.GetPackageLevels()`.

**How resolution works:** every `log.Info/Warn/Error/Debug` call site resolves
its own package from the call site's program counter (not the message text),
so no call-site changes are needed to benefit from this — existing code
"just works" once an override is set for its package. See
`log.PackageForPC(pc)` if you want to check how a given call site classifies.

**Why this exists:** before this, the only lever was one global level — there
was no way to see `session/tmux`'s debug output without also getting
`server/services`' (or worse, every websocket handler's) at the same time.

## Reducing log volume: what to check before adding a new log line

A 2026-08-25 audit of `~/.stapler-squad/logs/staplersquad.log` found 6
messages accounted for ~84% of all 35,865 lines in the file. The pattern in
every case was the same: a message that's genuinely useful **once** (state
transition, backpressure warning) logged at Info/Warn **on every occurrence**
of an underlying poll/retry loop, forever. Before adding a log line inside
anything that runs on a ticker, in a retry loop, or per-item in a bulk
operation, ask:

1. **Does this repeat for state that hasn't changed?** `session/health.go`'s
   `SessionHealthChecker` reloads every instance from storage every ~15s;
   `session/instance_serialization.go`'s worktree-missing detection used to
   re-fire its Warn on every single reload for the same already-paused
   session, forever, because the correction was never persisted back to
   storage. Fixed by de-duplicating per session title
   (`loggedMissingWorktree` in that file) — but the actual root cause (state
   drift between the DB and the in-memory correction never being persisted)
   is still open; the dedup only stops the log spam.
2. **Is this the expected/success path, not an error?** `session/tmux/tmux.go`'s
   `"tmux session doesn't exist, no need to kill"` fired 8,000+ times at
   `Info` for what is a completely benign, expected outcome — downgraded to
   `Debug`.
3. **Is this per-item inside a bulk save/load, meant as a debug trace?**
   `session/storage.go`'s `"SaveInstances: converting instance"` logged at
   `Info` for every single instance on every save — downgraded to `Debug`.
4. **Is this backpressure that fires per-dropped-item under sustained load?**
   `session/tokens/store.go` and `session/artifacts/store.go` logged a `Warn`
   for every single dropped item when their parse queues were full — under
   sustained backpressure this is thousands of nearly-identical lines that
   don't add information past the first one. Rate-limited to log every 100th
   drop with a running total (`dropLogInterval`) instead.
5. **Does this fire on every coalesced batch/frame during normal streaming?**
   `session/streamhub/hub.go`'s `onBatchFlush` logged at `Info` on every
   flush — up to ~50×/sec per actively-streaming session. Downgraded to
   `Debug`; the per-flush metric (`recordBatchFlushFramesCoalesced`) already
   captures this for OpenTelemetry/Datadog without a log line per flush.

If you're not sure whether a call site you're about to add will be hot, grep
for its enclosing function's callers and check whether any of them are a
`time.NewTicker`/`for { select {...} }` loop — `tools/lint/hotpolllog` already
catches one specific shape of this (a legacy-logger `.Printf` call directly
inside a `select` case inside a `for` loop) but doesn't catch `slog`-style
`log.Info(...)` calls or loops that call out to a helper containing the log
call, so it isn't a substitute for actually checking.

## Grouping/deduplicating logs (Datadog-style pattern clustering)

For ad hoc investigation, `scripts/log-group.sh` clusters log lines into
patterns the way Datadog's Log Patterns view does: group by
`(level, message)`, with dynamic substrings (UUIDs, filesystem paths,
embedded JSON blobs, bare numbers) normalized out, so structurally identical
messages collapse into one row even on the rare call site that interpolates
a value directly into the message text instead of passing it as a separate
field.

```bash
scripts/log-group.sh                              # top 30 patterns, live log
scripts/log-group.sh -n 50                         # top 50
scripts/log-group.sh -l warn                       # only WARN-level
scripts/log-group.sh ~/.stapler-squad/logs/*.log.gz  # across rotated segments
journalctl --user -u stapler-squad -o cat | scripts/log-group.sh -
```

Most call sites in this codebase already log a fixed message string with
values in separate JSON fields (the `slog` convention: `log.Info("msg",
"key", val)`), which is why a simple `jq -r '.msg' | sort | uniq -c | sort
-rn` gets you 90% of the way there for a quick check — reach for
`log-group.sh` when you need the normalization (a message with an embedded
JSON blob or path baked into the text) or level filtering.

## Metrics as an alternative to log volume

Before adding a log line to track something that happens frequently (a
counter, a rate, a distribution), check whether it belongs as an
OpenTelemetry metric instead — see `docs/how-to/enable-opentelemetry.md`. The
`onBatchFlush` case above is the template: `recordBatchFlushFramesCoalesced`
already exports this as a metric, so the log line was pure duplication once
downgraded off the default-visible path.
