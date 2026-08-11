# Research Synthesis: Frontend Reuse & Modularity Decision Criteria

## Decision Required

Define the rules that govern when a frontend piece of code in stapler-squad should be extracted as a shared/reusable component, hook, utility, or context — versus left inline or kept colocated with its primary consumer.

## Context

stapler-squad is a data-dense, real-time React/Next.js developer tool (~135 TSX/TS source files, Next.js 15 App Router, ConnectRPC streaming, vanilla-extract CSS, Redux Toolkit). The codebase is growing and inconsistency is emerging: some components mix data-fetching with display; some utilities live inside component files; a cross-domain import violation already exists (`components/sessions/DiffRenderer.tsx` imported by `components/unfinished/WorktreeDiffModal.tsx`). The team needs a documented decision framework to prevent this class of structural drift from accumulating.

The decision is motivated by a concrete extraction event: during implementation of `WorktreeDiffModal`, the original `DiffViewer` called `useSessionVcsContext()` directly, blocking reuse in the modal. The fix required extracting `DiffRenderer` (pure props) + `parseDiff` (pure utility). This pattern needs to be codified.

## Options Considered

| Option | Summary | Key Trade-off |
|---|---|---|
| Keep everything inline / no rules | No friction; no coordination cost | Structural drift accumulates silently; cross-domain violations invisible |
| Rule of Three (traditional) | Extract on third consumer | Too late for React; wrong abstraction cost is high |
| Rule of Two (AHA) | Extract on second consumer | Good default; doesn't handle RSC boundary or context-coupling constraints |
| Trigger-based extraction + coupling-radius-2 placement | Four explicit triggers; promotion-by-import-radius | Requires discipline; automate with eslint-plugin-boundaries |
| Linear-style packages | Feature packages with versioned shared layer | Over-engineered for single-app, <10k LOC frontend |

## Dominant Trade-off

The fundamental tension is **premature abstraction vs structural drift**. Extracting too early produces unstable prop interfaces that require constant churn ("the wrong abstraction is worse than duplication" — Sandi Metz). Extracting too late produces invisible coupling that blocks reuse and resists refactoring.

The recommended option lands on **demand-driven extraction**: duplicate once, extract on the second concrete consumer, and let import-graph violations (detected by linting) force promotion decisions rather than anticipating them speculatively.

## Recommendation

**Choose**: Trigger-based extraction with coupling-radius-2 placement, enforced by `eslint-plugin-boundaries`

**Because**: The four triggers (RSC boundary, context-coupling, Rule of Two, LOC/SRP) handle all real extraction cases while the AHA principle prevents over-extraction. The coupling-radius-2 placement rule mechanically determines file location without committee decisions. The linter enforcement prevents invisible cross-domain violations — the class of bug that already exists (`DiffRenderer` cross-domain import).

**Accept these costs**:
- `eslint-plugin-boundaries` configuration must be set up and maintained
- Teams must deliberately check "is this a second consumer?" rather than extracting proactively
- Linting (not compile-time) is the enforcement mechanism — violations caught at CI, not editor

**Reject these alternatives**:
- Rule of Three: rejected because at the scale of a React codebase, the third consumer is too late; the second consumer's import already proves the abstraction boundary is real
- Linear package model: rejected because package publish/versioning overhead is not justified for a single-app codebase at this scale
- No rules (status quo): rejected because the cross-domain violation already present (`DiffRenderer`) proves the current informal approach produces structural drift

## Open Questions Before Committing

- [ ] `eslint-plugin-boundaries` domain configuration: which domains should be defined? `sessions`, `unfinished`, `review`, `shared`, `lib`? — doesn't block the ADR, but required before enforcement is active
- [ ] Should `DiffRenderer.tsx` move to `components/shared/` immediately as part of this ADR's implementation, or as a follow-on? — recommend: include in the same PR as the ADR's tooling setup

## Sources

- [`findings-component-extraction.md`](./findings-component-extraction.md) — extraction triggers, AHA principle, RSC boundary
- [`findings-hook-utility-layer.md`](./findings-hook-utility-layer.md) — lib/ three-tier model, hook vs context ownership
- [`findings-colocation.md`](./findings-colocation.md) — coupling-radius-2 rule, eslint-plugin-boundaries, folder assignment
- [`findings-production-patterns.md`](./findings-production-patterns.md) — Linear, Vercel, Grafana, VS Code web patterns
