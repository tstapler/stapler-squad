# ADR-001: Static Discriminated Union for Omnibar Action Registry

**Date**: 2026-04-21
**Status**: Accepted

## Context

The omnibar needs a typed action registration mechanism so that new features automatically expose actions in the command palette. Three patterns were evaluated:

- **Option A**: Static barrel file + TypeScript discriminated union
- **Option B**: Dynamic `registry.register()` at module level
- **Option C**: React Context-based registration on component mount

## Decision

**Option A (Static Discriminated Union).**

```typescript
// lib/omnibar/actions/types.ts
export type OmnibarAction =
  | { type: "navigate_session"; sessionId: string; label: string }
  | { type: "create_session"; path: string; sessionType: SessionType }
  | { type: "clone_session"; sourceSessionId: string; label: string }
  | { type: "pause_session"; sessionId: string; label: string }
  | { type: "resume_session"; sessionId: string; label: string }
  | { type: "delete_session"; sessionId: string; label: string };
```

An exhaustive `switch` on `action.type` in the dispatcher causes a **compile error if a new action type is added without a handler** — this is the architectural guard requirement from the requirements doc.

## Rationale

1. Codebase already uses discriminated unions for `sessionType`, `InputType`, etc. — consistent pattern.
2. The existing detector registry is also statically registered (`createDefaultRegistry()`). Parallel architecture.
3. < 20 actions expected for MVP; dynamic plugin architecture adds complexity without payoff.
4. No React hooks in action handlers — actions are pure data + function, not components. Functions can call `useSessionService()` methods injected at creation time.
5. TypeScript compile error = architectural guard. Runtime assertion = optional second line of defense.

## Consequences

- Adding a new `OmnibarAction` variant requires editing `lib/omnibar/actions/types.ts` AND the dispatch switch. This is intentional friction that prevents silent omission.
- Actions cannot use React hooks directly. Dependencies (session service, router) are injected as parameters at registration time.
- Bundle size: all actions included even if not triggered. Acceptable for < 20 actions.
