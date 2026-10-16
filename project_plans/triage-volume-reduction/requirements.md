# Requirements: reduce triage volume

Source: backlog item b25276ee (2026-10-04 quota audit). Follow-up to the triage off-switch
(quota-aware-backlog-gating) and triage-cost-ceiling.

## Problem
Every created backlog item (RPC create, GitHub import, MCP create) fires one full agentic
`claude -p` triage via `MaybeTriggerTriage`; orphan auto-retry (`AutoRespawnTriage`) repeats it.
Concurrency is a hard-coded 8 (`triageSem`), and the model is whatever the pipeline mode
configures — empty (= account default) for the default pipeline.

## Requirements
- R1 (AC1) Measure triage calls/tokens/cost over the last 7 days from the local DB; record in the PR description.
- R2 (AC2) Automatic triage (`MaybeTriggerTriage`, `AutoRespawnTriage`) is skipped, with a visible
  activity note on the item, when the item already has acceptance criteria or an approved plan.
  Automatic re-triage is skipped when title+description+criteria hash equals the last completed
  triage's input hash. Manual `TriggerTriage` RPC (incl. feedback refine) is never skipped.
- R3 (AC3) Max concurrent triage runs is configurable (`max_concurrent_triage`, default 8 = today's
  behavior); excess runs queue on the semaphore.
- R4 (AC4) Triage uses a configurable model (`headless_triage_model`, default a cheaper tier) when the
  pipeline mode does not pin one; never the account default. Pipeline-mode executor model still wins.
- R5 `make lint`, `go test ./server/services ./session` pass.

## Out of scope
Batching multiple items into one call; dedupe by external_url (GitHub import already dedupes by
issue); both noted as follow-ups.
