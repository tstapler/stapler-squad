# Findings: Hook / Utility Layer Separation (ST-2)

## Summary

The lib/ layer in a React/Next.js codebase has three distinct sub-layers with clear ownership rules: **pure utilities** (`lib/utils/`) have zero React coupling and are tested like any function; **custom hooks** (`lib/hooks/`) own side effects and React lifecycle; **contexts** (`lib/contexts/`) own cross-component shared state. The dominant mistake is conflating these: hooks that are pure functions, or contexts used for data that a hook could own. The decision rule is: use the layer with the fewest dependencies that still meets the ownership requirement.

## Options Surveyed

- **Everything in hooks** — common in React codebases; blurs pure-function testability
- **Context for all shared state** — over-provisions; causes unnecessary re-renders and provider nesting
- **Pure utils + hooks + contexts (three-tier)** — matches React's own documentation; what this codebase should use
- **Zustand / Jotai atoms as fourth tier** — viable for this codebase if Redux becomes insufficient; not needed yet

## Trade-off Matrix

| Layer | Side Effects | React Coupling | Test Complexity | Scope | Re-render Risk |
|---|---|---|---|---|---|
| `lib/utils/` pure fn | None | None | Low — plain Jest | Call site only | None |
| `lib/hooks/` custom hook | Yes (fetch, subscriptions) | useEffect, useState | Medium — renderHook | Component tree subtree | Low (local state) |
| `lib/contexts/` context | Yes (via hooks internally) | Provider + Consumer | High — full tree render | Entire subtree under provider | High (all consumers re-render on value change) |
| Redux slice (existing) | Via RTK middleware | useSelector / useDispatch | Medium — Redux test utils | Global | RTK's `shallowEqual` mitigates |

## Risk and Failure Modes

**Context overuse (most common)**
- Every context value change re-renders all consumers — no partial subscription
- Mitigation: split contexts by change frequency; high-frequency values (terminal output) belong in local hook state, not context
- Pattern: `SessionVcsContext` (low-frequency: VCS state) is appropriate; streaming terminal bytes are NOT

**Pure function in a hook**
- Side-effect-free logic wrapped in `useCallback` / `useMemo` unnecessarily
- Costs: memo overhead, dependency array management, harder to test
- Mitigation: if a function takes only plain arguments and returns a plain value, it belongs in `lib/utils/`

**Hook that should be a context**
- Data fetched per-component that is actually shared across the tree, causing duplicate fetches
- Symptom: two sibling components that both call `useSomething()` and receive different instances of the same data
- Mitigation: if two components in the same tree need the same data, hoist to context

**Context that should be a hook**
- Single-consumer context (only one component ever calls `useContext(X)`)
- Over-provision: provider nesting adds complexity with no benefit
- Mitigation: colocate the fetch/state logic in a hook inside the single consumer

## Migration and Adoption Cost

Low — this is a classification policy, not a library change. The codebase already follows a three-tier pattern (`lib/utils/parseDiff.ts`, `lib/hooks/`, `lib/contexts/`). The decision rule formalizes what is already emerging.

## Operational Concerns

- Custom hooks are testable with `@testing-library/react`'s `renderHook` — no full component tree needed
- Pure utils are testable with plain Jest — fastest feedback loop
- Context tests require wrapping with `<SomeContext.Provider>` — heavier but standard practice
- React DevTools shows context value changes — useful for debugging re-render frequency

## Prior Art and Lessons Learned

**React documentation — custom hooks**
- Official guidance: extract to a hook when multiple components share stateful logic
- Custom hooks encapsulate the subscription, not the state itself (unless `useRef` is used for mutable state)

**Kent C. Dodds — when to use React context**
- "Context is designed for data that can be considered 'global' for a tree of React components"
- Over-use pattern: context used for data that is only consumed in one place
- Under-use pattern: multiple sibling hooks fetching the same data independently

**Vercel, Linear, Grafana patterns (training knowledge)**
- All three separate pure utilities from hooks
- Grafana's plugin system is the extreme: pure compute functions are isolated in separate packages from React hooks
- Linear uses contexts for user session / workspace state; feature data is hook-owned with SWR/TanStack Query

**ConnectRPC streaming in this codebase**
- Terminal streaming is a side-effect-heavy operation → belongs in a custom hook (`useTerminalStream`)
- Session VCS data (diff, branch) changes infrequently → appropriate for context (`SessionVcsContext`)
- Auth/config (static across lifetime) → appropriate for context or module-level singleton

## Open Questions

- [ ] At what point does Redux (RTK) become the better home for shared state that is currently in context? — blocks decision on: when to add slices vs contexts

## Recommendation

**Recommended option**: Three-tier `lib/` classification with explicit ownership rules

**The decision tree**:

1. **Does the code use any React import (hooks, JSX)?**
   - No → `lib/utils/` (pure function or pure class)
   - Yes → continue to 2

2. **Does the code need to be shared across the component tree (multiple consumers in different subtrees)?**
   - No → keep the hook in the component file or colocate in the same folder
   - Yes → continue to 3

3. **Is the data high-frequency (changes on every render or with user input) or low-frequency (changes on navigation or server events)?**
   - High-frequency OR consumed by exactly one component → `lib/hooks/` custom hook
   - Low-frequency AND consumed by ≥2 components in different subtrees → `lib/contexts/`

4. **Is the data truly global (user auth, app config, feature flags)?**
   - Yes → consider Redux slice in existing RTK store

**Specific examples for this codebase**:
- `parseDiff(content)` → `lib/utils/parseDiff.ts` (pure function, no React)
- `useSessionVcsContext()` is already correct — VCS data is low-frequency, shared across session view
- Terminal streaming bytes → custom hook colocated with terminal component (high-frequency, single consumer)
- Auth interceptor → module-level singleton in `lib/config.ts` (static lifetime)

**Conditions that would change this recommendation**: If the codebase adopts TanStack Query or SWR, the hook/context boundary shifts — query hooks eliminate most "context for shared fetch data" patterns.
