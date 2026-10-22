# Requirements: Backlog Process Output Formatting

**Source backlog item:** b293f6f1-088f-4ece-99df-ab61bab17e2f  
**Project name:** backlog-process-output-formatting  
**Date:** 2026-10-21

---

## Problem Statement

Stapler Squad's backlog pipeline runs three categories of background/agent processes:

1. **Triage agents** — LLM sessions that analyze a backlog item and output a `HeadlessTriageResult` JSON blob (priority, tasks, acceptance criteria, suggestions) as their final terminal output.
2. **Review agents** — LLM sessions that verify acceptance criteria and output a `headlessVerdictJSON` blob (overall verdict, per-criterion verdicts, summary).
3. **Diagnose agents** — Sessions that call `submit_diagnosis_result`, storing `"Diagnosis result: <OUTCOME>\n\n<summary>"` in the activity note stream.

In all three cases the user currently encounters raw JSON when they inspect the session output or check agent activity. A compact `{"priority":2,"tasks":[{"text":"...","estimate":"2h",...}]}` blob requires mental parsing; verdict details like `{"verdicts":[{"index":0,"status":"pass","evidence":"..."},...]}` are even harder to scan at a glance.

The target audience is the operator (the human watching the Stapler Squad web UI) who wants to understand what a triage or review pass concluded **without** reading raw JSON.

---

## Goals

1. Give operators a formatted, human-readable view of triage, review, and diagnose process results.
2. Preserve the raw JSON / terminal scrollback for users who need exact output or want to debug the agent.
3. Minimise new API surface — reuse data already persisted in the DB (`item_session.triage_result`, `review_verdict`, `activity_note`).
4. Maintain consistency with the existing `SessionSummaryPanel` pattern (ReactMarkdown + structured card components, feature-flagged rollout).

---

## Out of Scope

- Pretty-printing JSON inside the live xterm.js terminal stream (complex, fragile).
- CLI flag support for formatted output (low operator value vs. UI).
- Formatting of MCP tool responses returned to the calling agent (those are consumed by LLMs, not humans).
- Changing the format of headless agent prompts or their emitted JSON.

---

## Acceptance Criteria

| # | Criterion | Status |
|---|-----------|--------|
| AC-1 | A formatted "Results" section appears on the Backlog Item detail view showing the latest triage result (priority label, task list with estimates, suggestions list). It is visible whenever `item_session.triage_result` is non-empty, regardless of whether the triage session is still running. | pending |
| AC-2 | The same Backlog Item detail view shows the latest review verdict in a structured layout (overall outcome badge, per-criterion status rows with evidence snippets). It is visible whenever a `review_verdict` row exists for the item. | pending |
| AC-3 | Diagnose results (outcome + summary text) are rendered as a formatted card in the item's Activity section, replacing the raw `"Diagnosis result: BUG_FILED\n\n..."` string display. | pending |
| AC-4 | The formatted sections degrade gracefully: if data is not yet available they show a loading skeleton; if data is absent they are hidden (not an error state). | pending |
| AC-5 | A "View raw JSON" disclosure/toggle is available on each formatted result panel so operators can inspect the exact payload when needed. | pending |
| AC-6 | The triage Results panel is feature-flagged (`backlog-triage-results-panel`) and defaults to off; it can be enabled via the existing feature-flag RPC/panel without a service restart. | pending |
| AC-7 | The review Results panel is feature-flagged (`backlog-review-results-panel`) and defaults to off. | pending |
| AC-8 | All new React components are covered by Jest unit tests (render, loading, error, raw-toggle states). | pending |
| AC-9 | No existing Go unit or integration tests are broken by the changes. The new Go RPC handlers (if any) have corresponding test coverage. | pending |
| AC-10 | The formatted panels are usable on mobile viewport (≥ 320 px width) — touch-friendly disclosure, readable text, no horizontal overflow. | pending |

---

## Open Questions (not blocking planning)

- Should the triage results also appear as a dedicated tab in the Session detail view (alongside terminal/diff/summary tabs), or only on the Backlog Item detail view? The backlog item view is the primary navigation target for operators reviewing completed pipeline results.
- Should the review Results panel also surface the diff hash that was reviewed, so operators can confirm the review corresponds to the current commit?
- Activity notes (diagnose results) are text blobs; should we add a `type` field to the `activity_note` table to reliably identify diagnose entries, or parse the `"Diagnosis result:"` prefix heuristically?
- `item_session.triage_result` stores the raw JSON string; should a server-side RPC expose a typed `TriageResultProto` message, or is returning the raw JSON string via the existing `GetBacklogItem` RPC sufficient?
