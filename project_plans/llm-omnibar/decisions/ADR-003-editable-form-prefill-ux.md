# ADR-003: Editable Form Pre-fill UX

**Status**: Proposed
**Date**: 2026-04-18

## Context

When a user types `> fix the login bug in auth service` in the omnibar, three UX patterns are possible:

1. **Immediate creation**: Parse the intent and create the session immediately, with no user review. The session appears in the list as the LLM-generated parameters are applied.
2. **Preview card**: Show a compact card summarizing the parsed parameters; the user confirms or cancels; on confirm, the session is created.
3. **Editable form pre-fill**: Open the existing `NewSessionModal` (or, in the case of the omnibar's creation mode, the inline creation form) with fields pre-filled from the parsed intent; the user can edit any field before submitting.

The `Omnibar` component currently has two modes: "discovery" (search existing sessions) and "creation" (create a new session via an inline form). The creation mode already contains all necessary form fields (`title`, `path`, `branch`, `program`, `sessionType`, `prompt`, `category`). The `onCreateSession` callback accepts an `OmnibarSessionData` object that maps directly to the `SessionIntent` fields.

The features research confirmed this decision: session creation is a multi-field, consequential action. A wrong path or branch produces a confusing failure downstream (git clone fails, worktree creation fails). Immediate creation is unrecoverable without deleting the session. The preview card adds an extra interaction step vs. directly opening the editable form.

The pitfalls research identified a race condition (5.1): if the submit button is enabled while parsing is in progress, users can submit before the LLM result is applied. This must be mitigated by disabling the submit button during parsing.

## Decision

Use the **editable form pre-fill** pattern, implemented entirely within the existing `Omnibar` component (no new modal):

1. When the omnibar input starts with `>` (and has content after the prefix), detect it as "intent mode" and show a spinner in the input field immediately — before the API call returns. The omnibar stays open; the submit button is disabled.

2. Call `POST /api/sessions/intent` (ConnectRPC `ParseIntent`) with the description (stripped of the `>` prefix). The UI disables the submit button for the duration.

3. On parse completion, transition the omnibar to "creation" mode and pre-fill the form fields from the `ParseIntentResponse`:
   - `title` ← `SessionIntent.Title`
   - `path` (the input field) ← `SessionIntent.Path`
   - `branch` ← `SessionIntent.Branch`
   - `program` ← `SessionIntent.Program`
   - `sessionType` ← `SessionIntent.SessionType`
   - `prompt` (initial prompt textarea, if visible) ← `SessionIntent.InitialPrompt`
   - `category` ← first tag (if any)

4. Fields where `Confidence < 0.7` receive a CSS class that applies an amber left-border highlight, implemented using an existing CSS custom property bridge (following the project's vanilla-extract ADR-009).

5. If `SuggestedSessionID` is non-empty, display a dismissible banner above the form: "An existing session may match — [session title]. Use it instead?" with a button that navigates to that session via `onNavigateToSession`.

6. The user may edit any pre-filled field. Submitting the form proceeds through the existing `onCreateSession` callback without change.

7. If parsing fails (timeout, backend error), display the error inline in the omnibar (the existing `error` state), clear the spinner, and re-enable the input so the user can try again or proceed with manual entry.

**Detection logic**: The `>` prefix check is added to the existing `detect()` function in `src/lib/omnibar.ts`, returning a new `InputType.INTENT` value. The `Omnibar` component dispatches to the intent flow on this detection result.

## Consequences

**Positive**:
- No new React component required; the existing omnibar creation form is reused
- Users can inspect and correct all LLM-suggested values before committing
- Amber highlight for low-confidence fields surfaces uncertainty without blocking the workflow
- The `SuggestedSessionID` banner gives users a path to reuse existing sessions without forcing them into it
- Disabling submit during parsing (mitigation for pitfall 5.1) is a natural consequence of this flow

**Negative / accepted costs**:
- The 3–10s parse latency (depending on backend and cold-start conditions) means the omnibar is blocked during parsing; the spinner mitigates perceived unresponsiveness but does not eliminate it
- Field-by-field progressive pre-fill (showing fields appear one-by-one as the JSON streams) is a v1.1 enhancement; v1 waits for the full response before populating the form
- The confidence amber highlight is best-effort — the `Confidence` field from the CLI backend is not calibrated (the model may return 0.9 for a wrong path); it should be treated as a relative signal, not an absolute accuracy measure

**Rejected alternatives**:
- *Immediate creation*: Rejected. A wrong path or branch causes a session-creation failure that the user must debug. No recovery path.
- *Preview card*: Rejected. Adds an extra click before the user can edit. The form itself is the preview — opening it pre-filled serves both purposes.
- *New modal component*: Rejected. The existing omnibar creation mode already has all required fields. A separate modal would duplicate UI and diverge from the existing creation path.
