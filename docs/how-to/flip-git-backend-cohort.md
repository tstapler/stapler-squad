# Flip a git backend cohort (shadow soak, read counters, revert)

Each git operation belongs to a cohort (`refs`, `diffstatus`, `localwrite`, `network`, `worktree`).
Every cohort defaults to `cli`. A cohort moves `cli` -> `shadow` -> `gogit`; this page covers the
first step and the evidence that allows the second. Design: ADR-004 and ADR-006 in
`project_plans/go-git-fork-full-git-replacement/decisions/`; plan Story 2.1.3.

In `shadow` the git CLI still answers every call. The in-process backend (go-git) runs the same
read in the same call, the two answers are compared, and the difference is counted. Shadow never
changes the returned value or error, never spawns git, and only runs for read-only operations.

## Start the soak (`refs`)

Set the cohort in `~/.stapler-squad/config.json` (under `instances/<name>/` for an isolated
instance, see `docs/reference/state-isolation.md`), then restart the service:

```json
{ "git_backend_cohorts": { "refs": "shadow" } }
```

Valid values are `cli`, `gogit`, `shadow`. An unknown key or value logs a WARN naming it and
resolves to `cli`. There is no environment variable that turns shadow on, by design. The only
environment override is the rollback switch `STAPLER_SQUAD_GIT_BACKEND=cli` (any non-empty value
forces every cohort to `cli`; other values are treated the same and logged).

Confirm at startup: the log has no `invalid git backend mode` WARN, and
`curl -s 'localhost:8543/actuator/git-backend?format=text'` lists `cohort refs shadow`.

Do not set `gogit` for any cohort until this page's flip criteria hold. Gate G8 must also be
recorded in `project_plans/go-git-fork-full-git-replacement/implementation/gates.md` (outcome B
is recorded there today).

## Read the counters

```bash
make git-soak-status                                 # table + verdict; exit 0 ready, 1 not ready, 2 unreachable
curl -s 'localhost:8543/actuator/git-backend?format=text'
curl -s  localhost:8543/actuator/git-backend         # JSON, same data
```

The counters are kept in memory by the process (OTLP is not required) and also written once a
minute to `<config dir>/git-backend-soak.json`, so a restart does not reset the 7-day window. The
ledger is deleted at startup when no cohort is in shadow, which restarts the clock.

| Metric | Labels | Meaning |
|---|---|---|
| `git_backend_shadow_calls_total` | `operation` | Shadowed calls (the denominator) |
| `git_backend_shadow_mismatch_total` | `operation`, `class` | `racy` (vanished on a pinned re-read), `real`, `false_clean` |
| `git_backend_shadow_duration_ms` | `operation`, `implementation` | Wall time per side; p50 is estimated from the histogram buckets |
| `git_capability_detect_total` | `outcome`, `sub_reason` | Capability preflights; `detect_error` carries a closed `sub_reason` |
| `git_backend_cli_spawn_total` | `operation`, `reason` | Git processes the CLI backend started |
| `git_backend_fallback_total` | `operation`, `cohort`, `reason` | Calls diverted to the CLI instead of the cohort backend |

Real and `false_clean` mismatches also write a redacted repro record (no path or URL; the
repository is a hash of its root) to `<config dir>/shadow-mismatches/` and log
`git backend shadow mismatch` at WARN.

## Day-one capability checklist (first 24 h)

1. Run `make git-soak-status`. In the `capability preflights` line, `detect_error` over the total
   must be below 1%. A zero total is normal when only `refs` is shadowed: read operations with no
   capability dependency never consult the probe.
2. List every nonzero `detect_error sub_reason` line and note it.
3. If the ratio is 1% or more, pause the window: fix the sub-reason in the resolver, restart the
   clock (see revert below), then continue. At 100% every call routes to the CLI and the soak
   measures nothing.
4. Check `fallback` lines: a large `capability_detect_error` or `config` count on a refs
   operation means shadow is not actually running for it.

## Flip criteria (all must hold; the verdict section of the report checks them mechanically)

- The window is at least 7 days (`window since`), and at least 500 shadow calls on every
  operation in the cohort. An operation the app never calls stays at 0 and holds the window open.
- `class="real"` and `class="false_clean"` are 0 for every operation. `racy` is reported and does
  not block. On a real mismatch, fix it using the repro record, then restart the clock.
- go-git p50 is not slower than the CLI p50 for every operation (the G3 rule in `gates.md`).
  The verdict is per cohort: one slow operation (G3 measured `Log` as slower) keeps the whole
  cohort NOT READY. Per-operation overrides are not supported yet, so splitting the cohort is a
  decision for the maintainer.
- Capability `detect_error` ratio below 1%.
- Gate G8 recorded in `gates.md` (manual).

When these hold, set `"refs": "gogit"` and restart. The verdict never changes configuration.

## Revert

Set the cohort back to `cli` (or remove the key) in `config.json` and restart the service. For an
emergency without editing the file, start the service with `STAPLER_SQUAD_GIT_BACKEND=cli`.
Leaving shadow deletes the soak ledger on the next start.

## Limits

- Counters and the ledger are per instance. The ledger is flushed every minute, so a crash loses
  up to a minute of counts.
- `git_backend_cli_spawn_total` counts spawns made through the router. Callers still on their own
  CLI backend (for example the session worktree path) are not counted there.
- The shadow read runs in the caller's goroutine, after the CLI call, so a shadowed refs call
  costs the CLI latency plus the in-process latency (sub-millisecond for `ResolveRef`, per G3).
  Both sides are timed in `git_backend_shadow_duration_ms`.
