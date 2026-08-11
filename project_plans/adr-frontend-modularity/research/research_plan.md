# Research Plan: Frontend Reuse & Modularity Decision Criteria

## Decision Being Informed
What rules govern when a frontend piece of code should be extracted as a shared/reusable
component, hook, utility, or context — vs left inline — in a data-dense, real-time
React/Next.js developer tool (stapler-squad).

## App Context
- Next.js App Router, ~135 TSX/TS source files
- vanilla-extract (build-time CSS), Redux Toolkit (state), ConnectRPC (streaming RPCs)
- Domains: sessions, unfinished worktrees, review queue, terminal streaming, omnibar
- Similar apps: Vercel dashboard, Linear, GitHub PR views, Grafana, VS Code web

## Subtopics

### ST-1: Component extraction criteria (when to extract a component)
**Question**: What decision criteria distinguish "extract to shared component" from "keep inline"?
**Axes**: Rule of Three, prop-count thresholds, domain leakage, render complexity
**Search cap**: 4 queries
**Output**: `findings-component-extraction.md`

### ST-2: Hook / utility layer separation (lib/ architecture)
**Question**: When does logic belong in a custom hook vs a context vs a pure utility function?
**Axes**: Side-effect ownership, React lifecycle coupling, testability, data ownership
**Search cap**: 4 queries
**Output**: `findings-hook-utility-layer.md`

### ST-3: Colocation vs centralisation (file/folder layout)
**Question**: When should files live next to their primary consumer vs in a shared folder?
**Axes**: Coupling radius, feature-vs-layer folder models, domain ownership
**Search cap**: 3 queries
**Output**: `findings-colocation.md`

### ST-4: State of the art in similar production apps
**Question**: What patterns do high-quality data-dense developer tools (Linear, Vercel, Grafana, VS Code web) use for modularity?
**Axes**: Public design systems, open-source examples, conference talks
**Search cap**: 4 queries
**Output**: `findings-production-patterns.md`

## Scope Limit
Total searches: ≤ 15. Each subagent uses training knowledge + pending-search list; parent runs searches.

## Synthesis Output
`research/synthesis.md` → feeds `/plan:adr`
