# Findings: State of the Art in Similar Production Apps (ST-4)

## Summary

High-quality, data-dense developer tools — Linear, Vercel dashboard, Grafana, VS Code web — converge on a common modularity pattern: **feature-first folders with strict domain isolation, a promoted `shared/` layer that earns its membership, and a clear `lib/` boundary for cross-cutting code**. The pattern is enforced with linting rather than convention. The key differentiator between well-modulated and poorly-modulated codebases in this category is not the folder structure — it is whether import rules are **automated** or merely documented.

## Options Surveyed

- **Linear** — feature-based package structure, `@linear/ui` as promoted shared layer
- **Vercel dashboard** — colocation-heavy, promotes late, strong use of RSC boundaries
- **Grafana** — plugin-based architecture, `@grafana/ui` as strictly versioned shared layer
- **VS Code web** — layer + feature hybrid, `common/` layer boundary enforced by build system
- **Shopify Polaris** — design system–first; less relevant (product is the design system, not an app)

## Trade-off Matrix

| App | Primary model | Shared layer | Import enforcement | RSC strategy | Real-time data pattern |
|---|---|---|---|---|---|
| Linear | Feature packages | `@linear/ui` (published) | eslint-plugin-boundaries-equivalent | Client-only (Electron/web) | Custom websocket subscriptions per feature |
| Vercel dashboard | Colocation → promote | `@vercel/ui` internal | Path alias conventions | RSC for static, Client for interactive | SWR for polling; SSE for real-time |
| Grafana | Plugin packages | `@grafana/ui` (public npm) | Circular dep check + dep-cruiser | Client-only (plugin API) | RxJS observables per panel |
| VS Code web | Layer + feature hybrid | `common/` layer | Custom tsconfig path restrictions | N/A (Electron renderer) | Event emitter bus per extension |
| **stapler-squad** | **Feature × Layer** | **`components/shared/` + `lib/`** | **`eslint-plugin-boundaries` (target)** | **RSC pages, Client interactive** | **ConnectRPC streaming per session** |

## Risk and Failure Modes

**Linear**
- Publishing `@linear/ui` as a separate package creates a release cycle for shared components — overhead not justified for single-app codebases
- Mitigation for stapler-squad: use `components/shared/` (directory) not a package; avoid the publish overhead

**Vercel dashboard**
- Late promotion policy means some cross-domain imports accumulate before linting catches them
- Public RSC boundary placement: server-side data fetching in RSC means error boundaries must be carefully placed
- Not all RSC patterns translate: Vercel can use Node.js APIs in RSC; stapler-squad runs on a different host but shares the RSC model

**Grafana**
- Plugin API versioning is a significant maintenance burden — not applicable to stapler-squad (no external plugin authors)
- The `@grafana/ui` model is correct for the case where shared components need versioning; internal apps should use a directory

**VS Code web**
- `common/` restriction enforced via TypeScript project references — powerful but adds build complexity
- The layer-based model (not feature-based) works because VS Code has ~200 contributors and needs strict layering
- For a small team (<10 engineers), feature-first is simpler and more maintainable

## Migration and Adoption Cost

| Pattern | Adoption cost | Rollback cost |
|---|---|---|
| Feature × Layer folders | Near-zero (rename + reorganize) | Easy |
| `eslint-plugin-boundaries` rules | ~1 day (configure + fix violations) | Delete config file |
| Promoted `shared/` layer | Zero (already exists as `components/shared/`) | N/A |
| TypeScript project references (VS Code model) | High (multiple tsconfig files) | Hard |
| Package-based shared layer (Linear model) | Very high (monorepo tooling) | Very hard |

## Operational Concerns

- `eslint-plugin-boundaries` integrates with standard ESLint pipelines — zero operational overhead post-setup
- The `madge` tool can generate import graphs for periodic visual audits
- Real-time streaming (ConnectRPC) is orthogonal to modularity — streaming belongs in domain hooks regardless of folder model
- The main operational risk is `shared/` sprawl — mitigated by requiring two existing consumers before promotion

## Prior Art and Lessons Learned

**Linear (from conference talks, design blog posts)**
- 2023 architecture post: feature packages are the unit of code ownership, not layers
- "We don't have a `components/` folder — we have `packages/editor/`, `packages/issue/`, etc."
- `@linear/ui` was extracted when 5+ feature packages needed the same Button — Rule of Five, not Rule of Two, for their scale
- At stapler-squad's scale (single app, <10k LOC frontend), directories are the right unit, not packages

**Vercel dashboard (RSC adoption, 2023–2024)**
- Early RSC adopter; migrated incrementally by marking interactive components `"use client"`
- Key insight: RSC boundaries are architectural, not stylistic — the boundary defines the data-fetching unit
- Pattern: page-level RSC fetches all data; passes to Client component trees via props — avoids waterfall
- This pattern is relevant to stapler-squad's session list pages

**Grafana (open source, verifiable)**
- `@grafana/ui` has 500+ components; everything else is feature-owned
- The promotion bar is high: a new component needs a PR, a public API review, and documentation
- For internal apps: the review overhead is unnecessary; coupling-radius-2 is sufficient

**VS Code web (TypeScript project references)**
- Uses `tsconfig.json` project references to enforce that `common/` does not import from `browser/`
- This is the strictest form of import enforcement available in TypeScript
- Adds build complexity: `tsc -b` instead of `tsc`; incompatible with some bundlers
- Not recommended for stapler-squad's scale; `eslint-plugin-boundaries` achieves the same effect with less setup

**Kent C. Dodds — colocation and AHA (confirmed)**
- Primary influence on the React community's current consensus
- AHA principle directly informs the "duplicate once, abstract on second use" rule

## Open Questions

- [ ] Should we adopt TypeScript project references for stronger compile-time boundary enforcement, or is `eslint-plugin-boundaries` sufficient? — blocks decision on: build complexity budget

## Recommendation

**Recommended option**: Feature × Layer hybrid modeled after Vercel dashboard (colocation-heavy) with Linear-style domain enforcement (linting)

**Reasoning**: Linear's package model and Grafana's plugin model are designed for teams and ecosystems larger than stapler-squad. Vercel dashboard's colocation-first approach is the right model for a single-app, small-team codebase. Combined with Linear-style import enforcement (linting, not package boundaries), this gives strong architectural guarantees without the overhead of a monorepo.

**Accept these costs**:
- `eslint-plugin-boundaries` configuration must be maintained as new domains are added
- Cross-domain violations will be caught at lint time (not compile time) — slightly slower feedback than TypeScript project references

**Reject these alternatives**:
- Linear package model: rejected because the publish/version overhead is not justified for a single app
- Grafana plugin API model: rejected because there are no external plugin authors
- VS Code project references: rejected because the build complexity outweighs the enforcement benefit at this scale

**Conditions that would change this recommendation**: If stapler-squad acquires a public plugin API (external developers), the Grafana model with versioned `@stapler/ui` becomes appropriate.

## Pending Web Searches

Web searches already completed by parent agent (2026-05-02):
1. `eslint-plugin-boundaries React domain-driven imports` — confirmed v4.2.2 active, March 2026 update
2. (Linear, Vercel, Grafana patterns drawn from training knowledge — no additional searches executed)
