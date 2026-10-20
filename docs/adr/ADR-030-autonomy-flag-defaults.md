# ADR-030: Autonomy flags are ON live and OFF in code; code defaults stay off

Status: Accepted (current state recorded; flipping a code default is a separate owner decision) — 2026-10-20
Project: `backlog-make-it-true`, workstream D (promoted from the project's ADR-005).

## Context

Three flags (`backlog:sdd-default-pipeline`, `diagnose_nudge_enabled`, `autonomous_stall_nudge_enabled`) gate
autonomous writes into LLM panes and long-lived headless pipeline load. The code ships all three off; the owner's live
instance has all three on. A fourth, `triage_guidance_halt`, is recorded as an additional row. Until now that split was unrecorded, so a fresh install and the
live instance behaved differently with no document explaining why. Related analysis of the stall paths:
[autonomous-stall-causes.md](../explanation/autonomous-stall-causes.md) (see "Known gaps").

## State

| Flag | Code default | Live state (read 2026-10-20) | Enablement evidence | Rollback |
|---|---|---|---|---|
| `backlog:sdd-default-pipeline` | `false` (registry row `server/services/feature_flag_service.go:244-247` sets no `defaultValue`, so the zero value applies; read through `featureFlagDefault`, `feature_flag_service.go:468-475`, by `sddDefaultPipelineEnabled()`, `server/services/backlog_service_lifecycle.go:158-161`) | `enabled: true` | Date not recorded. | Turn off on the Features page, or `UpdateFeatureFlag` with flag `backlog:sdd-default-pipeline` and `enabled=false`. Effect: new items with no explicit `pipeline_mode` get the flat default pipeline instead of `sdd`. **Not retroactive**: the flag is read only at item creation (`defaultPipelineModeForNewItem`, `backlog_service_lifecycle.go:146-153`, comment at `:144-145`, says it "can never retroactively change an already-created item's stored pipeline_mode"), so items already created as `sdd` stay `sdd`. |
| `diagnose_nudge_enabled` | `false` (`config.DiagnoseNudgeDefault`, `config/config.go:565`) | `enabled: true` | Date not recorded. | Same toggle, flag `diagnose_nudge_enabled`. Effect: `diagnose_nudge_session` writes are blocked at the write call site, even for a diagnostic session that already decided to nudge (`config/config.go:553-575`). It is the kill switch for every autonomous pane write, including the stall nudge below. |
| `autonomous_stall_nudge_enabled` | `false` (`config.AutonomousStallNudgeDefault`, `config/config.go:584`) | `enabled: true` | Date not recorded. | Same toggle, flag `autonomous_stall_nudge_enabled`. Effect: the autonomous stall path stops typing a nudge into an idle work session and falls back to respawning it. It is AND-ed with `diagnose_nudge_enabled`: with diagnose off, this flag reads on but does nothing (`config/config.go:577-589`; "inactive: requires diagnose_nudge_enabled" at `server/services/feature_flag_service.go:584-585`). |

Additional row (not one of the three; single-sourced by Story 4.1.3 and recorded for completeness):

| Flag | Code default | Live state (read 2026-10-20) | Enablement evidence | Rollback |
|---|---|---|---|---|
| `triage_guidance_halt` | `false` (`config.TriageGuidanceHaltDefault`, `config/config.go:542`) | `enabled: true` | Date not recorded. | Turn off on the Features page, or `UpdateFeatureFlag` with flag `triage_guidance_halt` and `enabled=false`. Effect: automated triage stops halting with a `GuidanceRequest` on ambiguity and guesses instead (`config/config.go:536-551`). Read fresh at the halt decision, so it takes effect for in-flight triage. |

`diagnose_nudge_enabled`, `autonomous_stall_nudge_enabled` and `triage_guidance_halt` are declared once in `config` and
read by the registry (`server/services/feature_flag_service.go:317-328`). `backlog:sdd-default-pipeline` has no `config`
constant; its single declaration is the registry row, which its reader consults. VERIFIED by `grep -n` and `sed`.

### Evidence and commands

Live read-back (VERIFIED, read-only, 2026-10-20T10:18Z; one call, output reduced to the four flags):

```
curl -s -m 10 -X POST http://localhost:8543/api/session.v1.SessionService/GetFeatureFlags \
  -H 'Content-Type: application/json' -H 'Connect-Protocol-Version: 1' -d '{}'
-> HTTP 200; flags: backlog:sdd-default-pipeline enabled=true, diagnose_nudge_enabled enabled=true,
   autonomous_stall_nudge_enabled enabled=true, triage_guidance_halt enabled=true
```

Code history of the defaults (VERIFIED, `git log --format='%h %ad %s' --date=short -S<symbol> -- config/config.go`):

- `backlog:sdd-default-pipeline` key first appears in `c2adbc7a6` (2026-07-24, "seed an sdd PipelineMode and an opt-in default-selection flag"); the same search also lists later touches `a9b19d863` (2026-10-10) and `fa2d0f3a1` (2026-10-18).
- `triage_guidance_halt` flag key added in `19d61b8ab` (2026-09-14, #809).
- `diagnose_nudge_enabled` flag key added in `b1df034f2` (2026-09-29, #894).
- `autonomous_stall_nudge_enabled` flag key and `AutonomousStallNudgeDefault` added in `a895185ca` (2026-10-13, #1020).
- `TriageGuidanceHaltDefault` and `DiagnoseNudgeDefault` constants introduced in `c223a4fe5` (2026-10-20), a refactor that moved the existing `false` into one declaration; no default value changed.

These dates show when each default was introduced, not when the live instance turned the flag on.

### Enablement date: not recorded

No enablement date or actor is recorded for any of the three flags in any source this record was allowed to read.
The flag service writes audit lines on each change (`server/services/feature_flag_mutation.go`, `flagAudit`), but those
live in the application log, which was out of scope for this record (it may contain secrets). A prior read of the
live workspace config on 2026-10-17 (cited in the project's `research/build-vs-buy.md`, not re-run here) found all three
true with no date. The soak start date is an open owner item; if the owner supplies it, or a log search for the audit
lines finds it, add it to the table above rather than inferring one.

## Decision

1. Keep the code defaults off. A fresh install must not auto-write into LLM panes. Known gaps that default-on would widen
   are listed in [autonomous-stall-causes.md](../explanation/autonomous-stall-causes.md) ("Known gaps"): two nudgers per
   session, a lifetime nudge cap that is never reset, and restarts from the stale-work and health checkers that still
   kill and respawn.
2. The live ON state is a recorded operator opt-in, not an inherited default. Rollback for each flag is a live flip with no
   deploy.
3. Each default has one declaration: in `config` for the diagnose, stall-nudge and triage flags, in the registry row for
   `backlog:sdd-default-pipeline`.
4. Flipping a code default on needs measurable exit criteria. Outcomes are now countable through the nudge-outcome ledger
   (`/actuator/nudge-outcomes`, `session/nudge_outcomes.go`); the thresholds are an open owner decision and are not set here.

## Alternatives rejected

- Flip all three defaults now: no soak evidence is queryable, and the live instance is already on, so a flip would only
  change fresh installs.
- Leave the state undocumented: that is the situation this record fixes.

## Consequences

- A fresh install and the live instance differ by design; this table is the place to check which flags differ.
- When the owner supplies an enablement date, update the "Enablement evidence" cells.
- Turning `backlog:sdd-default-pipeline` off does not convert existing `sdd` items back to the flat pipeline.
