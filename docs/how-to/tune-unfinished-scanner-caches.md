# Tune, observe and roll back the unfinished-work scanner caches

The scanner's shared caches sit behind four rollback-unit flags. A unit's default flips to `true` in
`config.DefaultUnfinishedCaches` (`config/config.go`) after its own manual A/B passes (`baseline.md`,
"S2 A/B"); today all four are on (`scan_skip` last, after the round-2 A/B showed engagement). The config key is the off-switch: an absent
key keeps the default, an explicit `false` disables the unit (`session/unfinished/flags.go:24`). Flags are read once at startup, so every
change below needs a restart (`SetFlags`, `session/unfinished/flags.go:72`, called from `server/dependencies.go`).

## Read the counters (no OpenTelemetry needed)

```bash
curl -s localhost:8543/actuator/unfinished-scanner | jq .
```

The handler is `handleActuatorUnfinishedScanner` (`server/actuator.go`); it renders
`unfinished.BuildReport()` (`session/unfinished/report.go`), built from atomics with no lock. Keys:

| Key | Meaning |
|---|---|
| `flags` | normalized effective flags, including the dependent `scan_skip_prelude`, `scan_skip_dirty_gen` |
| `shadow_verify_every` | sampler rate (1/N scans); 0 means off; the rate also reads 0 when every flag is off |
| `scan_cycles_total`, `scan_cycle_seconds_sum` | completed scan tasks and their wall time |
| `caches.{graph,reach,aheadbehind,behind,messages,diffmemo,index,untracked}` | `hits`, `misses`, `evictions`, `coalesced`, `bytes`, `entries`; `index` and `untracked` report only hits/misses (their skip counters) |
| `repostates`, `repostate_bytes` | live shared `RepoState` count and summed bytes |
| `decoded_commits_total` | commit decodes by the stock parents reader (`commit_parents_stock.go`) |
| `parity_mismatch_total`, `parity_transient_total` | per field (`ahead`, `behind`, `messages`, `dirty`, `shortstat`) |
| `shadow_verify` | `sampled`, `skipped_pressure`, `oracle_errors` |
| `dirty_full_walk_total` | forced full walks by reason |
| `scan_skip`, `signature_skip` | index-cache and untracked-directory engagement; signature `checks`, `hits`, `misses` |
| `stat_calls_total`, `stat_calls_per_scan` | file stats issued by the scan loop |
| `reach_builds`, `cas_retries_total`, `graph_full_fallbacks_total`, `graph_full_rebuilds_total`, `graph_extension` | build kinds, arena/CLOCK contention, plain-walk fallbacks, full arenas replaced (at most once per `GraphRebuildInterval`, default 5 min, per repo), walks that extended the graph (`over_100ms` slow ones) |
| `mergebase_redundancy_unproven_total{bound,pruned_miss}` (series `unfinished.repostate.mergebase_redundancy_unproven_total{outcome}`) | merge-base redundancy checks (`isAncestorOfOther` plain walk and `reaches` id walk, summed) that answered "not redundant" without finding the candidate: `bound` = the visit bound ran out, `pruned_miss` = the search ended after skipping commits older than the candidate. Both are where a skewed committer clock can leave a redundant (still valid, not best) base; a sustained rate points at clock skew or very deep criss-cross history |

The same series are exported as `unfinished.repostate.*` OTel instruments
(`session/unfinished/metrics_repostate.go`, catalog `repostateCatalog`) when telemetry is on. They are
per cache, never per repo. A cache whose flag is off reports 0.

Startup logs one INFO, `unfinished scanner configured`, with the parents reader and the flag values
(`LogStartup`, `report.go`). Per-scan detail is debug level only (`scan cycle` line, `scanstate.go`).

## Run the shadow verify

With any rollback unit on, the scanner re-runs 1 scan in 200 on the legacy path and compares
(`session/unfinished/shadow_verify.go`; skipped under severe memory pressure). It needs no setup.
The legacy replay is expensive (about 18 to 45 MB per sampled scan), so 1 in 50 would add a visible
share of scanner allocations in steady state; for a parity soak, raise the rate with an environment
variable read once at startup:

```sh
STAPLER_SQUAD_SHADOW_VERIFY_EVERY=3 ./stapler-squad   # 1 in 3 scans; 0 turns it off; unset keeps 1 in 200
```

An invalid or negative value is ignored with a warning. The effective rate is `shadow_verify_every` in the
actuator. Confirm `shadow_verify.sampled` grows and `parity_mismatch_total` stays all zero. A non-zero mismatch
is a `HOLD` for flipping any default; the debug log carries both values.

## Roll back one unit

Edit the instance's `config.json` (`~/.stapler-squad/config.json`, or
`instances/<name>/config.json`) and restart:

```json
"unfinished_caches": {
  "shared_commit_state": false,
  "diff_counter": false,
  "scan_skip": false,
  "watch_worktree_gitdirs": false
}
```

| Flag | Disables |
|---|---|
| `shared_commit_state` | shared graph, reach bitsets, ahead/behind/message memos (also turns off `scan_skip`'s prelude features, with a startup WARN) |
| `diff_counter` | Myers counter and per-file diff memo |
| `scan_skip` | ref snapshot, signature skip, index cache, untracked-directory cache, shared stat pass |
| `watch_worktree_gitdirs` | linked-worktree gitdir watching (also turns off `scan_skip`'s DirtyGen features) |

`make install-service` restarts the live service and kills its tmux server unless the unit passes
`--tmux-keep-server`; restart by another route, or get the owner's go-ahead first
(`docs/explanation/tmux-keep-server-on-restart.md`).

## Roll back the go-git fork

A fork rollback is a module change, not a flag, and needs network: follow
`docs/how-to/roll-back-go-git-fork.md` (`go get`, `go mod tidy`, `go mod verify`).
