# Requirements: Omnibar Session Name UX

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-05-05

## Problem Statement

When users open the omnibar, type a name or prompt, and select a session type
(especially one-shot), they are forced to re-type the session name in a separate
field. There is no way to inject a first prompt when creating a session. There
is also no keyboard shortcut or inline syntax to switch session types without
reaching for the mouse.

**Who has this problem:** All users of the Stapler Squad omnibar who create
sessions, particularly one-off/one-shot sessions.

## Success Criteria

1. User types text in the omnibar → session name is auto-populated as a
   smart kebab-case slug — no re-typing required in any session creation mode
2. User can optionally provide a first prompt via an expandable section below
   the session name, or inline using a separator shorthand (Todoist-style `>`)
3. User can cycle through session types with a keyboard shortcut (Tab/Ctrl+Tab)
   while focused in the omnibar input
4. User can switch session type inline via slash-command prefix
   (e.g. `/oneoff`, `/worktree`, `/dir`)

## Scope

### Must Have (MoSCoW)
- Auto-populate session name field from omnibar typed input as a smart
  kebab-case slug in all session creation modes
- Expandable "First prompt (optional)" section below session name
- Inline separator shorthand to split session name from first prompt
  (e.g. `my session > do something important`)
- Keyboard shortcut to cycle through session types (Tab / Ctrl+Tab while in
  the omnibar input)
- Slash-command prefix to switch session type inline
  (`/oneoff`, `/worktree`, `/existing`, `/dir`)
- ADR documenting the Todoist-style inline shorthand syntax pattern and the
  principle that the omnibar supports rich inline commands (to guide future
  feature work)

### Out of Scope
- Adding new session creation types
- Persistent/reusable prompt templates
- Adding new omnibar detection patterns (DetectorRegistry entries)
- Any backend-only changes unrelated to supporting the first prompt

## Constraints

Tech stack: React + TypeScript (vanilla-extract CSS), ConnectRPC/protobuf for
backend, Go server
Timeline: Not set
Dependencies:
- `proto/session/v1/session.proto` — may need a new `first_prompt` field on
  `CreateSessionRequest` to support passing the first Claude prompt through
- Session creation registry (7 touchpoints) — any new fields threaded to the
  RPC require all 7 touchpoints to be updated
- `web-app/src/lib/omnibar/actions/` — OmnibarAction union + dispatch must be
  updated if behavior changes

## Context

### Existing Work
- One-off session feature (2026-04-24) is the canonical example of the
  flag-on-create_session pattern; first-prompt should follow the same pattern
- The omnibar already uses a DetectorRegistry for input pattern detection
- Session types: directory, new_worktree, existing_worktree, one_off
- The inline slash-command concept does not exist yet; this is net-new

### Stakeholders
- Tyler Stapler (sole practitioner / end user)

## Research Dimensions Needed

[ ] Stack - how the slug generation should work (library vs. custom), how
    first_prompt would be threaded from UI → RPC → session startup
[ ] Features - survey comparable tools (Todoist, Linear, Raycast) for
    inline shorthand UX patterns; session-type cycling in launcher UIs
[ ] Architecture - where to parse the inline separator, state shape for
    first-prompt in OmnibarFormState, ADR structure
[ ] Pitfalls - edge cases in slug generation (emoji, special chars, long
    input), shorthand parsing ambiguities, keyboard shortcut conflicts
