# ADR-003: Two-Phase Omnibar Model (Discovery + Creation)

Status: Accepted
Date: 2026-04-14
Deciders: Tyler Stapler

## Context

The Omnibar currently operates in a single mode: session creation. Adding session navigation (jump to existing session) and recent-repo quick-pick requires deciding how to blend two fundamentally different intent models in one input:

- **Navigate intent**: user wants to jump to an existing session
- **Create intent**: user wants to start a new session (existing behavior)

Three interaction models were considered:
1. Sigil-based mode switching (e.g., `>` for navigation, no prefix for creation) — VS Code pattern
2. Two-phase sequential model (discovery list → creation form)
3. Single merged result list with action labels

## Decision

Implement a two-phase Omnibar model:
- **Phase 1 (Discovery)**: Input drives a fuzzy result list (sessions + recent repos). No form visible.
- **Phase 2 (Creation)**: The existing session creation form. Entered when a repo result is selected or when input is detected as a local path or GitHub URL.

## Rationale

1. **Sigil-based switching rejected.** VS Code uses sigils because it has 8+ intent modes. Two modes do not justify learned syntax. Visual grouping achieves the same disambiguation with no cognitive overhead.
2. **Single merged list rejected.** Session results (navigate → close omnibar) and repo results (fill form → stay in omnibar) have different primary actions. Interleaving them creates ambiguity for keyboard users. Research evidence: Linear, Raycast both use sections with headers for distinct action types.
3. **Two-phase sequential is the Linear model.** Empty state shows recent sessions + recent repos. Selecting a session closes the omnibar. Selecting a repo transitions to Phase 2 with the path pre-filled.
4. **Phase 2 is unchanged.** The existing creation form requires zero modification. The boundary between Phase 1 and Phase 2 is a mode state variable in `Omnibar.tsx`. This minimizes regression risk.
5. **Path and GitHub URL inputs bypass Phase 1.** When the detector identifies a local path or GitHub URL, the omnibar jumps directly to Phase 2. This preserves all existing creation flows.

## Consequences

- `Omnibar.tsx` gains `omnibarMode: 'discovery' | 'creation'` state variable.
- New components: `OmnibarResultList`, `OmnibarSessionResult`, `OmnibarRepoResult`.
- Phase 2 (the existing form body and submit logic) is untouched.
- Enter key behavior splits: Enter on session result → navigate; Enter on repo result → fill path + transition to creation; Cmd+Enter → submit form (only valid in Phase 2).
- The `canSubmit` gate must exclude `InputType.SessionSearch` mode (no form to submit).
- Escape from Phase 1 result list → dismiss list (first press); Escape again → close omnibar (second press). Same contract as path completion dropdown.
