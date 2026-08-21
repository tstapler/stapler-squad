# ADR-002: Fuse.js for TypeScript Fuzzy Session Search

Status: Accepted
Date: 2026-04-14
Deciders: Tyler Stapler

## Context

Session search requires multi-field fuzzy matching across `title`, `branch`, `path`, and `tags` with field-specific weights (title matches matter more than path matches). Three TypeScript candidates were evaluated:

| Library | Algorithm | Multi-field | Bundle |
|---|---|---|---|
| `fuse.js` | Bitap | First-class, weighted | ~8 kB gzip |
| `match-sorter` | Tiered ranking | Yes | ~3-4 kB gzip |
| `fzf` (fzf-for-js) | Smith-Waterman | Single-field only | ~10 kB est. |

## Decision

Use `fuse.js` for client-side session fuzzy search.

## Rationale

1. **Multi-field weighted keys are first-class.** Fuse.js accepts a `keys` array with per-field `weight` values. This is the primary requirement — title match (weight 0.5) must rank above path match (weight 0.1).
2. **TypeScript types included.** No `@types/` package needed.
3. **`includeScore` and `includeMatches`.** Score is returned per result for post-ranking with status boost and recency decay. Match indices are available for future highlight rendering.
4. **Threshold tuning.** The `threshold` parameter controls sensitivity. Starting at 0.4 covers the "myfeat" → "my-feature-branch" case.
5. **`match-sorter` rejected** because its tier-based ranking does not handle non-contiguous character patterns ("myfeat" never matches "my-feature-branch" — fails the ≤3 keystrokes requirement).
6. **`fzf-for-js` rejected** because it lacks first-class multi-field support; composite-string workaround loses per-field weight control.

## Consequences

- Add `fuse.js` to `web-app/package.json` dependencies (not already present).
- Create `web-app/src/lib/fuzzy.ts` as the Fuse.js integration layer with session-specific field configuration.
- Fuse.js instance is created once per query batch (or memoized) — not on every keystroke render.
- Threshold and field weights are tunable constants in `fuzzy.ts`, not scattered across call sites.
- If scoring quality is insufficient after implementation, `fzf-for-js` can be swapped into the same `fuzzy.ts` interface without changing callers.
