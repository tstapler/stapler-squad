---
name: observability-grafana-dashboards
description: Use when a new OTel metric (or pprof profile type) is added to stapler-squad and needs a Grafana dashboard, when auditing whether existing metrics have dashboard coverage, or when a dashboard's panels feel wrong/noisy. Covers the RED/USE framework for deciding what a dashboard should answer, panel-type selection by metric shape, the per-project Grafana folder/provisioning convention, and the verification steps that catch silently-broken panels (wrong metric name, wrong profileTypeId) before calling a dashboard done.
---

# Building and Maintaining stapler-squad's Grafana Dashboards

Dashboard source lives in `~/dotfiles/stapler-scripts/observability/grafana/dashboards/stapler-squad/`
(machine-shared docker-compose stack, not in this repo — see the `observability-victoriametrics-tempo`
skill for the full architecture). This skill is about *authoring* dashboards; use that skill for
*querying* the live backends first to find out what data actually exists.

## Step 1: Find every un-dashboarded metric before designing anything

Don't design a panel from a guess at what's instrumented. Enumerate real instruments first:

1. Grep the Go source for meter registration: `Int64Counter`, `Int64ObservableGauge`,
   `Float64Histogram`, `Int64ObservableCounter`, etc. (see `telemetry/telemetry.go` and any
   `*_metrics.go` file for the pattern).
2. Cross-reference against the existing dashboard JSON's `targets[].expr` fields — anything
   instrumented but never queried in a dashboard is a gap.
3. For each gap, confirm the *live* metric name via VictoriaMetrics rather than guessing the
   OTel→Prometheus mangling yourself (dots→underscores, unit suffixes, `_total` on counters) — see
   the metric-name-mangling gotcha in `observability-victoriametrics-tempo`. A dashboard panel with
   a wrong metric name doesn't error, it just renders "No data" forever, which is worse than an
   error because nothing calls it out.

## Step 2: Decide what question the dashboard answers (RED vs. USE)

Per Grafana's own dashboard guidance and the Google SRE golden-signals framework: **one dashboard,
one question.** Don't build a single sprawling "everything" dashboard — a reviewer or a future
incident responder needs to know which 4-6 panels matter, and that's impossible past ~15 panels on
one screen. Group by the question, not by "things emitted from the same Go file."

- **RED** (Rate, Errors, Duration) — for anything request/command-shaped: tmux control-mode
  commands, HTTP/RPC calls. Ask: is this operation healthy right now, and for which
  command/method/endpoint?
- **USE** (Utilization, Saturation, Errors) — for anything resource-shaped: cgroup memory, HTTP
  connection pool, open stream count, FIFO queue depth. Ask: is this resource close to its ceiling?
- **Continuous profiling (flamegraphs)** — a different question entirely ("where is the CPU/memory
  actually going"), always its own dashboard, never mixed into a RED/USE one.

Layout convention used in this repo's dashboards (`tmux-control-mode.json`,
`http-rpc-connections.json` are the reference examples):
1. Row 1 ("Golden signals"): the 3-4 panels that answer the dashboard's one question at a glance,
   above the fold.
2. Row 2 ("Breakdown"): the same signals sliced by a label (`command`, `method`) for drill-down.
3. Row 3 ("Context", optional): supporting counters that explain an anomaly in rows 1-2 but aren't
   the headline metric (e.g. `safeexec.sigkill_escalations` next to connection saturation, since a
   process being SIGKILLed can look like a connection drop).

## Step 3: Pick the panel type by metric shape, not habit

| Metric shape | Panel type | Notes |
|---|---|---|
| Counter, rate over time, multiple label values | `timeseries` | `sum(rate(...[5m])) by (label)` — never graph a raw counter, always `rate()`/`increase()` |
| Counter, "how many in the last incident window" | `barchart` or `stat` | `increase(...[30m])`, small absolute numbers are more readable as a bar/stat than a rate |
| Gauge, single current value, has a known danger threshold | `stat` | Set `thresholds` (green/orange/red) — e.g. `http.server.connections_open` at 5/6 (the browser's per-origin cap) |
| Gauge, trend over time matters as much as current value | `timeseries` | Same thresholds as the `stat` twin, so a viewer sees both "now" and "how did we get here" |
| Histogram | `timeseries` of `histogram_quantile(0.5\|0.99\|0.999, sum(rate(..._bucket[5m])) by (le, <label>))` | Always show at least p50 and p99 together — p99 alone hides whether the whole distribution shifted or just the tail |
| pprof profile (CPU/heap/goroutine) | `flamegraph` (Pyroscope datasource, `queryType: "profile"`) + a `timeseries` twin (`queryType: "metrics"`, same `profileTypeId`) above it | The timeseries lets you spot *when* to drag-select a range; the flamegraph alone has no time navigation of its own |

Percent/ratio panels (e.g. timeout rate = timeouts / attempts): unit `percentunit` with `max: 1`,
never compute the raw percentage yourself and set unit `percent` — let Grafana's unit formatter do
it, and never report a raw error *count* without its corresponding attempt-rate panel right next to
it (a bare count is meaningless without the denominator — this project has been burned by exactly
that, see `observability-victoriametrics-tempo`'s "Timeout RATE" note).

**Consistency across panels in the same dashboard**: same color per series across panels (Grafana
does this automatically per legend label if the label is spelled identically — keep `legendFormat`
strings consistent, e.g. always `{{command}}`, never `{{command}}: latency` on one panel and
`cmd={{command}}` on another). Normalize units so magnitudes are comparable (percent not raw count
for anything resource-utilization-shaped).

## Step 4: Folder and provisioning convention (read before adding a new provider block)

Dashboards are file-provisioned per project folder — `grafana/dashboards/<project>/*.json` — each
with its **own explicit provider entry** in `grafana/provisioning/dashboards/dashboards.yml`
(`folder: '<project>'`, `options.path: /var/lib/grafana/dashboards/<project>`).

**Do not use a single provider with `foldersFromFilesStructure: true`** to infer folders from
subdirectories — it silently fails against Grafana's `nestedFolders` feature toggle (on by default
since 11.x, confirmed against `grafana:11.5.2` on 2026-09-09): dashboards land in "General" with no
provisioning error logged, and the only way to notice is checking
`GET /api/dashboards/uid/<uid>`'s `meta.folderTitle` by hand. See
[grafana/grafana#73271](https://github.com/grafana/grafana/issues/73271) and the comment block at
the top of `dashboards.yml` for the full story.

To add dashboards for a project that doesn't have a folder yet: create
`grafana/dashboards/<project>/`, add the JSON there, then add a provider block to `dashboards.yml`
pointed at that exact path — don't let an existing provider's path overlap with the new directory
(overlapping paths double-provision the same dashboard under two providers and error).

## Step 5: Verify before calling it done — a "No data" panel doesn't error

A dashboard JSON with a wrong `expr`, wrong `profileTypeId`, or wrong datasource `uid` loads fine
and shows "No data" — indistinguishable at a glance from "correct query, just no traffic yet." Do
not skip verification:

```bash
# 1. Bring the stack up / restart Grafana to pick up new provisioning files
cd ~/dotfiles/stapler-scripts/observability && docker compose up -d

# 2. Confirm the dashboard landed in the right folder (not "General")
curl -s -u admin:admin "http://localhost:48300/api/dashboards/uid/<uid>" | \
  python3 -c "import json,sys; d=json.load(sys.stdin); print(d['meta']['folderTitle'])"

# 3. Directly query each panel's exact target through Grafana's own query API — this exercises
#    the real datasource proxy + query, not just "did the JSON parse"
curl -s -u admin:admin -X POST "http://localhost:48300/api/ds/query" -H "Content-Type: application/json" \
  -d '{"queries":[{"refId":"A","datasource":{"type":"prometheus","uid":"victoriametrics"},
       "expr":"<paste the panel'"'"'s exact expr>"}],"from":"now-15m","to":"now"}' | \
  python3 -c "import json,sys; d=json.load(sys.stdin); print(d['results']['A'].get('error'), len(d['results']['A'].get('frames',[])))"
```

For a Pyroscope/flamegraph panel, the query body needs `queryType`/`profileTypeId`/`labelSelector`
instead of `expr` — see this skill's own dashboards for the exact shape, or the git history of
`pprof-profiles.json`'s authoring session for a worked verification example (generated real traffic
with a throwaway `--profile` build, confirmed `last_scrape` health via Alloy's debug API at
`localhost:12345/api/v0/web/components/pyroscope.scrape.<name>`, then confirmed real flamegraph data
came back with `preferredVisualisationType: "flamegraph"` in the frame).

**Flamegraph panels render a distinct "broken" state on an empty window** — not "No data" like every
other panel type, but "Data is missing fields: label, level, value, self" (confirmed against
`grafana:11.5.2` on 2026-09-09). This fires whenever the CPU profile scrape window genuinely
captured zero samples (e.g. the process was idle, or — as happened during this dashboard's own
authoring session — the service was mid-restart/rollback and the pprof endpoint had no live
process behind it for a few scrape cycles), not just when the query is wrong. Don't assume this
message means a broken `profileTypeId`/`labelSelector` — re-check with the verification query in
Step 5 first; if that returns real frames for the same window, the panel will self-heal on the next
`refresh` tick once real samples exist again.

If metrics never fire in practice because the live service doesn't have `OTEL_ENABLED=true` (or
`--profile` for pprof), that's expected — VictoriaMetrics/Pyroscope will be empty and panels will
show "No data" until the flag is on. That's a different, expected "no data" state from a broken
query; don't confuse the two. Generate a real signal (a throwaway `--profile`/`OTEL_ENABLED=true`
build under a distinct `STAPLER_SQUAD_INSTANCE`, per `docs/how-to/manage-systemd-service.md`'s
"manual testing" pattern — never against the live deployed instance) to tell them apart.

## Current dashboard inventory

| Dashboard (uid) | Question it answers | Method |
|---|---|---|
| `ssq-tmux-control-mode` | Is tmux control mode healthy, and for which command? | RED |
| `ssq-http-rpc-connections` | Is the HTTP/1.1 connection pool close to the browser's 6-per-origin cap? | USE |
| `ssq-cgroup-memory` | Is the process close to its MemoryHigh/MemoryMax cgroup ceiling? | USE |
| `ssq-pprof-profiles` | Where is CPU/heap/goroutines actually going right now? | Continuous profiling |
