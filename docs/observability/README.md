# Grafana Dashboards

`grafana/dashboards/*.json` are this project's Grafana dashboards, migrated
here from `~/dotfiles/stapler-scripts/observability/grafana/dashboards/stapler-squad/`
(2026-09-12) so they version alongside the metrics they visualize instead of
living in an unrelated repo.

| Dashboard | uid | Covers |
|---|---|---|
| `cgroup-memory.json` | `ssq-cgroup-memory` | USE-method cgroup memory (`cgroup_memory_*`, see `docs/how-to/enable-opentelemetry.md`) |
| `http-rpc-connections.json` | `ssq-http-rpc-connections` | RED-method HTTP/RPC connection saturation (`http.server.connections_*`, `rpc.server_streams.open`) |
| `pprof-profiles.json` | `ssq-pprof-profiles` | Pyroscope flamegraphs (CPU/heap/goroutines), scraped from the pprof server |
| `tmux-control-mode.json` | `ssq-tmux-control-mode` | RED-method tmux control-mode command latency/throughput |

None of these cover the tymux backend yet — `session_backend_operation_duration_ms`
(added closing BUG-108) has no panel. Building one is a natural follow-up now
that the metric exists; see that bug's "Out of scope" note.

## Viewing them

These dashboards don't run their own Grafana — they're provisioned into the
single machine-wide observability stack defined in
`~/dotfiles/stapler-scripts/observability/` (Grafana + VictoriaMetrics +
Tempo + Pyroscope + an OTel Collector, shared across every local project).
That stack's `docker-compose.yml` bind-mounts this directory directly
(`docs/observability/grafana/dashboards` → `/var/lib/grafana/dashboards/stapler-squad`
in the `grafana` container), so editing a file here and running
`docker compose restart grafana` in the observability stack picks it up —
there is no separate copy to keep in sync.

```bash
cd ~/dotfiles/stapler-scripts/observability && make up   # if not already running
open http://localhost:48300   # admin / admin
```

`Dashboards → stapler-squad` in the Grafana UI. `allowUiUpdates: true` is set
on this provider (see that repo's `grafana/provisioning/dashboards/dashboards.yml`),
so an edit made in the Grafana UI writes back to the JSON file here directly
— `git diff` after using the UI to see what changed, same "dashboards as
code" workflow as editing the JSON by hand.

Point stapler-squad's own OTel exporter at this stack per
`docs/how-to/enable-opentelemetry.md` — it defaults to `localhost:4317`
(`telemetry.DefaultOTLPEndpoint`), which is exactly where the stack's
collector listens, so there's usually nothing to configure beyond enabling
the app's own OTel flag.
