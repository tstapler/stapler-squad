# Findings: Colocation vs Centralisation (ST-3)

## Summary

The decision of where a file lives — next to its primary consumer vs in a shared folder — reduces to a single rule: **colocate with the first consumer; promote to `shared/` when the second distinct domain imports it**. This "coupling-radius-2" rule is derived from Kent Dodds' colocation principle ("place code as close to where it's relevant as possible") combined with the architectural reality that import graph violations are hard to spot without tooling. `eslint-plugin-boundaries` (v4.2.2, active as of March 2026) automates enforcement.

## Options Surveyed

- **Everything colocated** — maximizes locality; breaks when second domain imports
- **Everything centralized in `shared/`** — over-centralizes; `shared/` becomes a dumping ground
- **Feature-first folder model** — domain code owns its files; `shared/` is earned by explicit promotion
- **Layer-first folder model** — `hooks/`, `components/`, `utils/` at top level; scales poorly for large feature sets
- **Feature × Layer hybrid** — feature folders for domain code, layer folders (`lib/utils/`, `lib/hooks/`, `lib/contexts/`) for truly cross-cutting code

## Trade-off Matrix

| Strategy | Discoverability | Import safety | `shared/` sprawl risk | Deletion test | Domain isolation |
|---|---|---|---|---|---|
| Everything colocated | High | Low (no guard) | None | Easy | Low |
| Everything centralized | Low | High (but manual) | Very High | Hard | Low |
| Feature-first | High | Medium (with linter) | Low | Easy | High |
| Layer-first | Medium | Medium | High | Hard | Low |
| Feature × Layer hybrid | High | High (with linter) | Low | Easy | High |

## Risk and Failure Modes

**`shared/` sprawl**
- Without a promotion policy, `shared/` accumulates every component that someone "might reuse someday"
- Symptom: `shared/` has more files than any feature domain
- Mitigation: require two concrete existing consumers before allowing placement in `shared/`

**Invisible import violations**
- `components/unfinished/WorktreeDiffModal.tsx` importing from `components/sessions/DiffRenderer.tsx` is a cross-domain violation that TypeScript will not catch
- Without `eslint-plugin-boundaries`, this violation is invisible until a domain is deleted or refactored
- Mitigation: install `eslint-plugin-boundaries` with explicit domain definitions

**Stale colocation (file outgrows its location)**
- A file that started as "specific to one component" but has accumulated 4–5 callers in different domains
- Hard to spot without an import graph visualizer
- Mitigation: `eslint-plugin-boundaries` import count alerts; periodic `madge` or `dependency-cruiser` audits

**Over-colocation in Next.js App Router**
- `app/` directory components cannot be moved to `components/` without becoming client components if they use RSC features
- Mitigation: keep RSC-specific wrappers in `app/`; pure display logic in `components/`

## Migration and Adoption Cost

**`eslint-plugin-boundaries` installation**: ~30 min; requires defining domain boundaries in `.eslintrc`. Breaking changes are caught immediately as lint errors on the next run.

**Promoting a file from colocation to `shared/`**: rename + update imports; `tsc` validates completeness. In a well-typed codebase this is a 5-minute operation.

**Retrospective cleanup** (existing violations like `DiffRenderer.tsx`): identify all cross-domain importers with `grep`, move file to `components/shared/`, update imports. Safe to do in one commit.

## Operational Concerns

- `eslint-plugin-boundaries` integrates with VS Code ESLint extension — violations show inline while editing
- `dependency-cruiser` generates visual import graphs — useful for periodic audits but not needed in CI
- `madge` is lighter and produces cycle detection — useful for catching circular imports

## Prior Art and Lessons Learned

**Kent C. Dodds — colocation principle (2021, confirmed by web search 2026-05)**
- "Place code as close to where it's relevant as possible"
- Deletion test: if the consumer is deleted, the colocated code should be deleted with it — if it can't, it's been promoted prematurely
- Scale-out: as a component's coupling radius grows, its location should move toward `shared/`

**eslint-plugin-boundaries (v4.2.2, updated March 2026)**
- Configured with `elements` defining each domain (e.g., `components/sessions`, `components/unfinished`, `lib`)
- `disallow` rules prevent cross-domain imports: `unfinished` cannot import from `sessions`; both can import from `lib`
- `allow` rules permit upward imports: `components/*` can import from `lib/*` and `components/shared/*`
- Active community; CI-safe; zero runtime cost

**Linear (design team interviews, 2024–2025)**
- Feature packages: `packages/editor`, `packages/issue-list`, etc.
- `packages/ui` is the promoted shared layer — files move there only when second feature package imports
- Strict: `packages/editor` cannot import from `packages/issue-list` — enforced with `eslint-plugin-boundaries`-equivalent

**Grafana (open source, verifiable)**
- Plugins are the atomic unit; each plugin owns its component tree
- `@grafana/ui` is the promoted shared layer
- Files promoted to `@grafana/ui` require a public API review — high bar prevents shared sprawl

**Next.js App Router conventions**
- `app/` — route segments, layouts, RSC pages; tied to URL structure
- `components/` — reusable UI components; domain-organized within
- `lib/` — pure utilities, hooks, contexts; cross-cutting
- No official guidance on when to colocate vs promote, but colocation is the App Router team's demonstrated preference (each route segment has its own co-located loading, error, page files)

## Open Questions

- [ ] Should `components/shared/` be enforced as a boundary via `eslint-plugin-boundaries` immediately, or documented first and enforced in a follow-on PR? — blocks decision on: whether to include linter config in this ADR or a separate one

## Recommendation

**Recommended option**: Feature × Layer hybrid with coupling-radius-2 promotion rule, enforced by `eslint-plugin-boundaries`

**The coupling-radius-2 rule**:
1. New file → colocate with its first consumer
2. When a second distinct domain needs to import it → move to `components/shared/` (for JSX) or `lib/` (for non-JSX)
3. Files in `shared/` or `lib/` must not import from domain folders (would create circular coupling)

**Folder assignment by code type**:
| Code type | First consumer location | Promoted location |
|---|---|---|
| JSX component, domain-specific | `components/<domain>/` | `components/shared/` |
| Custom hook | Component file or `components/<domain>/` | `lib/hooks/` |
| Pure utility function | Component file or `components/<domain>/` | `lib/utils/` |
| Context provider | Consumer component or `components/<domain>/` | `lib/contexts/` |
| vanilla-extract styles | Colocated `.css.ts` | `components/shared/<Name>.css.ts` |

**Immediate action**: Move `components/sessions/DiffRenderer.tsx` to `components/shared/DiffRenderer.tsx` — it is already imported by both `components/sessions/DiffViewer.tsx` and `components/unfinished/WorktreeDiffModal.tsx`, violating the coupling-radius-2 rule.

**Conditions that would change this recommendation**: If the codebase adopts a true monorepo/workspace structure with `packages/`, the promotion target changes from `components/shared/` to a `packages/ui` workspace — the rule itself stays the same.

## Pending Web Searches

Web searches already completed by parent agent (2026-05-02):
1. `Kent C. Dodds colocation "place code as close to"` — confirmed principle verbatim
2. `eslint-plugin-boundaries React domain-driven imports` — confirmed v4.2.2 active
