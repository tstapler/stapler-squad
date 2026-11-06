# ADR-001: Unify Next not-found behavior via `app/not-found.tsx`

**Date**: 2026-10-22
**Status**: Proposed — pending owner confirmation that the new behavior is intentional
**Deciders**: Tyler Stapler

---

## Context

PR 7a (#1117) pinned the Next build's behavior for unknown paths as `{ status: 200, finalPath: "/", title: "Stapler Squad Sessions" }` — an unknown path silently redirected to `/` because no `app/not-found.tsx` existed. After PRs 4/5a/6a merged, the observed behavior changed: an unknown path now renders the `NotFound` component at the original URL with title "Page not found - Stapler Squad".

The `NotFound.tsx` component already exists (added for the Bun build in PR 7a). The `not-found.spec.ts` Next baseline test now fails because `next-baseline.json` still records the old redirect behavior.

Two paths forward:

**Option A — Investigate and revert**: Determine which of PRs 4/5a/6a caused the fall-through behavior to change, and revert it so unknown paths redirect to `/` again on Next. Update nothing in `next-baseline.json`.

**Option B — Accept and formalize**: Create `web-app/src/app/not-found.tsx` as a one-line re-export of `NotFound`, making the new behavior intentional and identical across both builds. Update `next-baseline.json` to record the new observed values.

---

## Decision

**Option B** — create `not-found.tsx` and update the fixture.

**Rationale**:

1. A dedicated not-found page is strictly better UX than a silent redirect to `/`: the user sees the URL they tried, a clear "Page not found" message, and a "Go to Sessions" link. Redirecting to `/` gives no signal that the path was invalid.
2. The `NotFound.tsx` component already exists and is already tested by the Bun-only block in `not-found.spec.ts`. The one-line re-export is zero-logic duplication.
3. Unifying the behavior across both builds reduces the test surface: the `not-found (Next baseline)` and `not-found (Bun only)` blocks will assert essentially the same semantics once the fixture is updated.
4. The "investigate what changed" path (Option A) spends effort to recover behavior that is inferior to what we already have.

---

## Consequences

- `web-app/src/app/not-found.tsx` is created (new file, one line).
- `tests/e2e/fixtures/next-baseline.json`'s `notFound` key is updated from `{ status: 200, finalPath: "/", title: "Stapler Squad Sessions" }` to `{ status: 200, finalPath: "/nonexistent", title: "Page not found - Stapler Squad" }`.
- The two `not found (Next baseline)` tests in `not-found.spec.ts` will pass on Next.
- The `not-found (Bun only)` block is unaffected (it already asserts the NotFound view).
- No production routing behavior is changed beyond what already happened implicitly when the App Router began resolving unknown paths to the NotFound component.

---

## Owner Confirmation Required

This ADR must be confirmed by the owner before Phase 3 implementation begins. A comment on the PR or an inline sign-off in the task is sufficient.
