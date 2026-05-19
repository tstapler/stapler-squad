# Requirements: AI-Assisted Rule Generation for ssq-hooks

## Problem Statement

The ssq-hooks classifier requires users to manually craft regex patterns and fill out rule forms to reduce escalations. Three distinct pain points drive demand for AI assistance:

1. **Too many manual approvals** — Safe commands that recur frequently hit the escalate queue because no matching rule exists. Users must repeatedly approve them.
2. **Pattern authoring difficulty** — Writing correct `toolPattern`/`commandPattern` regexes is error-prone. Users often create overly broad or broken patterns.
3. **Rule coverage gaps** — Unmatched command volume is visible in analytics but converting that data into rules requires manual effort.

## Goals

- Enable an AI agent to propose rule suggestions in all three surfaces (Rules UI, Review Queue, Analytics gaps panel).
- Agent suggestions require explicit human approval before saving — no auto-save.
- Agent uses analytics history, existing rules, seed rule examples, and optionally a raw command sample as context.

## User Stories

### US-1: Bulk Suggestion from Analytics
As a user on the `/rules` page, I can click "Generate Suggestions" to trigger an AI agent that analyzes the last N days of escalated/denied commands, cross-references existing rules, and proposes a list of new rules with pre-filled name, toolName/toolPattern, commandPattern, decision, reason, alternative, and priority. I review each proposal and accept or discard it individually.

### US-2: Rule from Review Queue Item
As a user in the review queue, when I manually approve or deny a request I can click "Create Rule from This" to open a rule form pre-filled by an AI agent that has analyzed the command, inferred the safest pattern scope, and suggested a reason/alternative. I can edit any field before saving.

### US-3: Rule from Analytics Coverage Gap
As a user viewing the analytics panel, when I click on a coverage-gap item the system triggers an AI agent that generates a rule suggestion for that specific tool/command pattern. I review and accept or discard the suggestion.

### US-4: Rule from Pasted Command
As a user creating a rule manually, I can paste a raw command string into an input and an AI agent proposes the most specific safe pattern (commandPattern regex, appropriate toolName, risk-calibrated decision, reason). I review the proposal before saving.

## Functional Requirements

### FR-1: GenerateSuggestedRule RPC
A new ConnectRPC endpoint `GenerateSuggestedRule` accepts:
- `source`: enum (analytics_gaps | review_queue_item | command_sample)
- `window_days`: int32 (for analytics_gaps source)
- `command_sample`: string (for command_sample source)
- `analytics_item_id`: string (for review_queue_item source)

Returns a `SuggestedRuleProto`:
- All fields of `ApprovalRuleProto` pre-filled
- `confidence`: float (0–1) for the agent's confidence in the pattern
- `explanation`: string (why the agent chose these fields)
- `source_commands`: repeated string (the sample commands that informed the pattern)

### FR-2: Suggestion Review UI
A `SuggestedRuleCard` component displays the full proposed rule with all fields editable inline. An "Accept & Save" button upserts the rule via the existing `UpsertApprovalRule` RPC. A "Discard" button dismisses the card.

### FR-3: Rules Page — Generate Suggestions Panel
A "Generate Suggestions" button on `/rules` triggers `GenerateSuggestedRule` with `source: analytics_gaps`. The response renders a list of `SuggestedRuleCard` components. The panel shows a loading state while the agent runs.

### FR-4: Review Queue — Create Rule From This
A "Create Rule from This" action on any review-queue item calls `GenerateSuggestedRule` with `source: review_queue_item`. The resulting `SuggestedRuleCard` is shown in a modal/drawer.

### FR-5: Analytics Gap Item — Suggest Rule
In `ApprovalAnalyticsPanel`, each coverage-gap item gains a "Suggest Rule" icon button that calls `GenerateSuggestedRule` with `source: analytics_gaps` scoped to that tool/command category. Shows a `SuggestedRuleCard` inline or in a popover.

### FR-6: Command Sample Input
In the manual rule creation form, an optional "Generate from command" text input calls `GenerateSuggestedRule` with `source: command_sample` and pre-fills the form fields.

### FR-7: Agent Context Assembly
The backend agent handler assembles context before calling the AI:
- All existing rules (via `AllRules`)
- Seed rule examples (from embedded classifier seed rules)
- Analytics data for the requested window (via `GetAnalyticsSummary`)
- The command sample or analytics item as the focal point

### FR-8: No Auto-Save
The agent never writes rules to the database directly. All persistence goes through the existing `UpsertApprovalRule` RPC only after explicit user confirmation.

## Non-Functional Requirements

- **Latency**: Agent calls may take 5–30 seconds; UI must show a meaningful loading state and allow cancellation.
- **Cost control**: Agent calls are on-demand (user-initiated) only, never background/scheduled.
- **Pattern safety**: Generated `commandPattern` regexes must be validated (compilable) server-side before returning to the client. Invalid patterns return an error.
- **Privacy**: Command samples and analytics data stay within the local instance; no telemetry to external services beyond the configured AI provider.

## Out of Scope

- Auto-saving rules without user confirmation.
- Scheduled/background rule auditing.
- Multi-step agent conversations or iterative refinement (V1: single-shot suggestion only).
- Integration with external rule repositories or shared rule packs.

## Success Metrics

- Escalation rate drops ≥ 20% within 7 days of a user running "Generate Suggestions" once.
- User can create a well-formed rule from a pasted command in < 60 seconds.
- Zero auto-saved rules (human-in-the-loop maintained throughout).
