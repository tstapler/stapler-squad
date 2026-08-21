# ADR-010: Frontend Reuse & Modularity Decision Criteria

## Status
Accepted

## Context

stapler-squad's web UI (`web-app/`) is a data-dense, real-time developer tool built on Next.js 15 App Router, vanilla-extract CSS, Redux Toolkit, and ConnectRPC streaming. As of May 2026, the codebase has ~135 TSX/TS source files across five domains: sessions, unfinished worktrees, review queue, terminal streaming, and the omnibar.

Two classes of structural problem have begun to appear:

**1. Context-coupling blocking reuse.** The original `DiffViewer` component called `useSessionVcsContext()` directly. When `WorktreeDiffModal` (in the `unfinished/` domain) needed to display a diff without a session context in its tree, the component could not be reused. The fix required extracting `DiffRenderer` (pure-props display) + `parseDiff` (pure utility) — a pattern that should have been applied proactively.

**2. Invisible cross-domain import violations.** `components/sessions/DiffRenderer.tsx` was created as a sessions-domain component but is immediately imported by `components/unfinished/WorktreeDiffModal.tsx`. TypeScript does not catch this; the import compiles cleanly. Without a documented policy and linting enforcement, this class of violation accumulates silently.

These two observations motivated a research phase (see `project_plans/adr-frontend-modularity/research/`) covering component extraction criteria, hook/utility layer separation, colocation vs centralisation, and patterns from similar production tools (Linear, Vercel dashboard, Grafana, VS Code web).

### Root Cause

No documented extraction criteria. Developers must decide individually and informally when to extract, what layer to use, and where to place the result. In a small codebase this is fine; as it scales it produces structural drift that is expensive to correct after the fact.

### Requirements

Any decision framework must:

- Distinguish the four extraction targets (component, hook, utility, context) with clear ownership rules
- Handle the RSC/client boundary as a hard architectural constraint, not a style preference
- Prevent cross-domain import violations without requiring manual enforcement
- Be demand-driven (no speculative extraction) to avoid the AHA failure mode (wrong abstraction is worse than duplication)
- Require minimal tooling to enforce — the framework should be automatable

## Decision

Adopt the **trigger-based extraction model** with **coupling-radius-2 placement** as the canonical decision framework for frontend code organisation in `web-app/`.

---

### Part 1: Extraction Decision Tree

**Step 1 — RSC boundary (mandatory correctness check)**

Does the code use any of: `createContext`, `useState`, `useReducer`, `useEffect`, `useRef`, event handlers, browser-only APIs?

→ Yes: it must be in a `"use client"` component or hook. If it currently lives in an RSC file, extract immediately. This is a correctness constraint — Next.js App Router throws at runtime, not build time.

→ No: it is a candidate for a pure utility function (`lib/utils/`). Continue to Step 3.

**Step 2 — Context coupling (reuse blocker)**

Does the code call `useContext(SomeDomainContext)` or any domain-specific hook that wraps a context?

→ Yes, and a second caller from a different domain needs the same output:
Extract a **pure-props version** immediately. The context-coupled version becomes a thin wrapper that passes context values as props to the pure version.

Example: `DiffRenderer` (pure props, importable anywhere) + `DiffViewer` (thin wrapper that reads `SessionVcsContext` and passes props to `DiffRenderer`).

→ No: continue to Step 3.

**Step 3 — Rule of Two (duplication trigger)**

Is this the **first** caller of this piece of code?

→ Yes: keep inline or colocate with the consumer. Do not extract speculatively. "Prefer duplication over the wrong abstraction" (Sandi Metz / AHA Programming).

→ No, a **second distinct domain** needs it: extract. At this point the abstraction boundary is proven by two concrete use cases — the interface is unlikely to be wrong.

**Step 4 — LOC/SRP pressure (size trigger)**

Does the file exceed ~250 lines **and** contain two or more clearly separable responsibilities?

→ Yes: split into components with a single, nameable reason to exist. Pure length alone is not sufficient — the responsibilities must be separable and the resulting components must each have a single clear name.

→ No: keep as-is.

**Trailing indicator (not a trigger): Prop count > 8–10**

High prop count at the call site signals that the abstraction boundary may be in the wrong place — not that a new component is needed. Apply Steps 1–4 first. High prop count usually resolves as a side effect of applying the correct trigger.

---

### Part 2: Layer Assignment

Once extraction is decided, choose the layer:

| Code type | Does it use React? | Is it shared across domains? | Target location |
|---|---|---|---|
| Pure function / pure class | No | N/A | `lib/utils/` |
| Custom hook | Yes | No (single feature) | Colocate in feature folder or component file |
| Custom hook | Yes | Yes (≥2 domains) | `lib/hooks/` |
| Context provider + hook | Yes | Yes (≥2 domains) | `lib/contexts/` |
| JSX component | Yes | No (single domain) | `components/<domain>/` |
| JSX component | Yes | Yes (≥2 domains) | `components/shared/` |
| vanilla-extract styles | No (build-time) | Follows component | Colocate as `.css.ts` |

**The three-tier `lib/` model**:

1. `lib/utils/` — zero React imports, pure functions, tested with plain Jest
2. `lib/hooks/` — React lifecycle (side effects, subscriptions), tested with `renderHook`
3. `lib/contexts/` — cross-tree shared state, low-frequency data, tested with `<Provider>` wrapper

Context overuse warning: context re-renders all consumers on every value change. High-frequency values (terminal bytes, cursor position) belong in local hook state, not context. Low-frequency values (VCS state, session metadata) are appropriate for context.

---

### Part 3: File Placement — Coupling-Radius-2 Rule

1. **New file** → colocate with its first consumer
2. **When a second distinct domain imports it** → move to the promoted location (`components/shared/` for JSX; `lib/` for non-JSX)
3. Files in `components/shared/` and `lib/` **must not import from domain folders** — that would create circular coupling

The deletion test: if the first consumer is deleted, should the colocated code be deleted with it? If yes, it belongs colocated. If no, it has already been promoted prematurely.

---

### Part 4: Enforcement — `eslint-plugin-boundaries`

Install `eslint-plugin-boundaries` (v4.2.2+) to make cross-domain import violations lint errors rather than invisible accidents.

**Domain definitions**:
```
sessions       → components/sessions/**
unfinished     → components/unfinished/**
review         → components/review/**
omnibar        → components/omnibar/** (and lib/omnibar/**)
shared         → components/shared/**
lib            → lib/**
app            → app/**
```

**Import rules**:
- `sessions`, `unfinished`, `review`, `omnibar` may import from `shared` and `lib` — not from each other
- `shared` and `lib` may not import from any domain folder
- `app` may import from any domain (it is the composition root)

This converts the silent violation that already exists (`unfinished` importing from `sessions`) into a lint error on the next CI run.

---

### Part 5: Immediate Implementation Actions

1. **Move `DiffRenderer.tsx`** from `components/sessions/` to `components/shared/` — it already has two domain consumers (`sessions/DiffViewer.tsx` and `unfinished/WorktreeDiffModal.tsx`)
2. **Move `DiffRenderer.css.ts`** (which is `DiffViewer.css.ts` today) to `components/shared/DiffViewer.css.ts` or rename to `DiffRenderer.css.ts`
3. **Install and configure `eslint-plugin-boundaries`** with the domain definitions above
4. **Fix the one existing violation** (after move, `unfinished` importing `shared` is permitted)

---

## Consequences

### Positive

- Extraction decisions have explicit, checkable criteria — no more "should I extract this?" debates
- RSC boundary is treated as a hard constraint, preventing runtime errors from misplaced context
- Cross-domain import violations are lint errors — caught in CI before merge
- `lib/` layer is cleanly separated by React coupling (utils vs hooks vs contexts) — each sub-layer has a clear test strategy
- The AHA principle prevents over-abstraction: code duplicates once before abstracting, ensuring the abstraction interface is proven by two real use cases

### Negative / Constraints

- `eslint-plugin-boundaries` configuration must be updated when new domains are added
- The coupling-radius-2 rule requires developers to check import graphs before placing new files — a lightweight discipline cost
- Rule of Two is enforced by convention, not compilation — a developer can still extract speculatively; linting won't catch it (it only catches import direction, not extraction timing)
- Context-coupling extraction (Step 2) can feel like "extra work" when only one caller exists today; the payoff comes when the second caller arrives

### Scope

This ADR covers the `web-app/` directory only. Backend (Go) and proto-layer modularity are governed by separate conventions.

## References

- Research plan: `project_plans/adr-frontend-modularity/research/research_plan.md`
- Findings: `project_plans/adr-frontend-modularity/research/findings-*.md`
- Synthesis: `project_plans/adr-frontend-modularity/research/synthesis.md`
- Kent C. Dodds — AHA Programming: https://kentcdodds.com/blog/aha-programming
- Kent C. Dodds — Colocation: https://kentcdodds.com/blog/colocation
- Sandi Metz — "The Wrong Abstraction": https://sandimetz.com/blog/2016/1/20/the-wrong-abstraction
- eslint-plugin-boundaries: https://github.com/javierbrea/eslint-plugin-boundaries
- ADR-009 (vanilla-extract): `docs/adr/009-vanilla-extract-type-safe-css.md` — CSS rules this ADR's placement rules apply to
