# Stack
Go only; no new dependencies. Touch points (verified by reading):
- `server/services/backlog_service_triage.go:2711` `MaybeTriggerTriage` — gate for all 4 create paths.
- `.../backlog_service_triage.go:2496` `AutoRespawnTriage` — orphan retry path.
- `.../backlog_service_trigger_triage.go:161` `TriggerTriage` — manual + auto; semaphore at :376-395; model resolved at :329-339 from `pipelineEngine.ExecutorFor` (empty for default pipeline / unpinned stage = account default).
- `server/services/backlog_service.go:567` `triageSem: make(chan struct{}, 8)` hard-coded.
- `config/config.go` `HeadlessTriage*OrDefault` accessor pattern (0 = default).
- `session/backlog_triage.go:25` `HeadlessTriageResult` (persisted JSON per triage ItemSession) — carries new `InputHash`.
- `Storage.AppendActivityNote` (session/storage.go:1538) — visible per-item note, shown in UI/`get_backlog_item`.
