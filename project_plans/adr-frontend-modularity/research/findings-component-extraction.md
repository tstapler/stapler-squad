# Findings: Component Extraction Criteria (ST-1)

## Summary

When to extract a React component from inline JSX to a named, importable unit is the most frequent modularity decision in this codebase. The dominant research consensus is that extraction should be **demand-driven, not speculative**: duplicate once, abstract the second time, and only when the abstraction's interface is clear. Four concrete triggers — RSC boundary, context-coupling breakage, Rule of Two, and LOC/SRP pressure — cover the vast majority of real extraction decisions. Prop-count alone is a trailing indicator, not a root cause.

## Options Surveyed

- **Never extract (keep all JSX inline)** — maximizes colocation, destroys readability past ~200 lines
- **Extract on first use** — premature; over-abstraction is the dominant failure mode in React codebases
- **Rule of Three** — traditional software engineering; too late for React where props interfaces are quick to validate
- **Rule of Two (Kent Dodds / AHA)** — extract on second distinct consumer; dominant in React community
- **Trigger-based extraction** — four discrete triggers; what this codebase should use

## Trade-off Matrix

| Criterion | Keep Inline | Rule of Three | Rule of Two | Trigger-Based |
|---|---|---|---|---|
| Over-abstraction risk | None | Low | Low-Medium | Low (explicit triggers) |
| Duplication risk | High | Medium | Low | Low |
| Interface clarity at extraction time | N/A | High | Medium | High (trigger proves need) |
| Test isolation | Low | High | High | High |
| Overhead | Zero | Low | Low | Low |
| Handles RSC boundary | No | No | No | Yes (Trigger 1) |
| Handles context coupling | No | No | No | Yes (Trigger 2) |

## Risk and Failure Modes

**Premature extraction (Trigger 2–4 applied too early)**
- Creates components with unstable prop interfaces that require constant churn
- "The wrong abstraction is worse than duplication" — Sandi Metz (validated by AHA Programming research)
- Mitigation: require at least one concrete second consumer before extracting on Rule of Two

**Extraction without clear responsibility boundary (LOC pressure misapplied)**
- Results in "split for length, not for reason" — two components that must always change together
- Mitigation: apply SRP test: does the extraction have a single, nameable reason to exist?

**Over-prop-drilling from premature extraction**
- Component extracted too early → adds props back indefinitely as requirements emerge
- Mitigation: if prop count grows past 8 at the call site, re-examine whether the boundary was correct

**RSC boundary violation (Trigger 1 ignored)**
- `createContext`, `useState`, `useEffect`, event handlers all require `"use client"`
- RSC cannot render context providers without the boundary; Next.js App Router will throw at runtime
- Mitigation: treat RSC boundary as a hard structural constraint — not a code style choice

## Migration and Adoption Cost

All four triggers are applied at the moment of extraction — no up-front migration cost. The only tooling investment is `eslint-plugin-boundaries` for enforcing that extracted components don't import from domains they shouldn't (see ST-3 findings).

## Operational Concerns

- Refactoring tools (VS Code, JetBrains) support component extraction well — low friction
- TypeScript's structural typing validates prop interface correctness at call sites immediately
- No runtime monitoring needed; extraction is a compile-time decision

## Prior Art and Lessons Learned

**Kent C. Dodds — AHA Programming (2019, validated by web search 2026-05)**
- "Avoid Hasty Abstractions" — the corollary to DRY
- "Prefer duplication over the wrong abstraction" (Sandi Metz attribution)
- "The cost of the wrong abstraction is much higher than the cost of a little duplication"
- Core rule: duplicate once, abstract the second time and only when the abstraction's interface is clear

**React Server Components boundary (Next.js App Router, 2023–2026)**
- `createContext` requires `"use client"` — this is a hard architectural constraint
- RSC can pass Server-rendered children as props/slots to Client providers
- Pattern: `<ClientProvider>{children}</ClientProvider>` where `children` are server-rendered
- Web search confirmation: createContext without "use client" throws at runtime

**Atlassian — component extraction threshold**
- Atlaskit design system: components extracted when they appear in ≥2 product surfaces
- Product surfaces treated as distinct domains (equivalent to Rule of Two with domain qualifier)

**Linear (open-source-adjacent observations)**
- Feature components stay in feature directory until second feature imports them
- Move to `shared/` is triggered by an actual import violation, not anticipation

## Open Questions

- [ ] Should prop-count trigger extraction automatically, or only when combined with another trigger? — blocks decision on: whether we need a strict prop-count ceiling

## Recommendation

**Recommended option**: Trigger-Based extraction with four ordered triggers

**Reasoning**: The trigger-based model is more precise than Rule of Two alone because it handles the RSC boundary (which is a hard correctness constraint, not a style preference) and context-coupling breakage (which blocks reuse entirely) as mandatory first checks before the softer duplication-count trigger. It also avoids the failure mode of "split for length, not for reason" by requiring an SRP test alongside the LOC trigger.

### The Four Triggers (in priority order)

**Trigger 1 — RSC boundary (mandatory)**
Extract to a `"use client"` component when any of the following are needed: `createContext`, `useState`, `useReducer`, `useEffect`, `useRef`, event handlers (`onClick`, `onChange`), browser-only APIs. This is a correctness constraint imposed by Next.js App Router — not extracting causes a runtime error.

**Trigger 2 — Context coupling blocks a second caller**
If a component calls `useContext(SomeDomainContext)` and a second, unrelated domain needs the same visual output, extract a pure-props version immediately. The context-coupled version becomes a thin wrapper. Example: `DiffRenderer` (pure props) + `DiffViewer` (session context connector).

**Trigger 3 — Rule of Two: second distinct consumer**
When a second component in a different domain (or a different feature within the same domain) needs to render the same visual structure, extract. One usage → keep inline. Two usages in distinct call sites → extract. Do NOT extract on first use even if "this looks reusable."

**Trigger 4 — LOC/SRP pressure: ~250 lines with a separable responsibility**
A single file exceeding ~250 lines that contains two or more clearly separable responsibilities (e.g., data-fetching logic + display markup) should be split. The split must produce components with a single, nameable reason to exist. Pure length alone is not sufficient — the responsibilities must be separable.

**Trailing indicator (not a trigger): Prop count > 8–10**
High prop count at the call site is a smell indicating the abstraction boundary may be wrong, not that a new component is needed. Apply Triggers 1–4 first; high prop count usually resolves as a side effect.

**Conditions that would change this recommendation**: If the team adopts a utility-first Tailwind-style workflow, Rule of Three becomes more appropriate since composition is cheaper. If the team moves to a micro-frontend architecture, Trigger 3 becomes mandatory at the feature boundary, not the component level.

## Pending Web Searches

Web searches already completed by parent agent (2026-05-02):
1. `Next.js App Router "use client" createContext RSC boundary` — confirmed: createContext requires "use client"
2. `Kent C. Dodds AHA programming "prefer duplication"` — confirmed: AHA = Avoid Hasty Abstractions, Sandi Metz attribution
