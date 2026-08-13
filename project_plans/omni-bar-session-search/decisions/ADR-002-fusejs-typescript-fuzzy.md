# ADR-002: Fuse.js for TypeScript Client-Side Fuzzy Search

Status: Accepted
Date: 2026-04-14
Deciders: Tyler Stapler

## Context

Session search in the Omnibar must match short, imprecise queries (1-4 characters) against multiple fields: session title, branch name, working directory path, and tags. The fields have different relevance weights (title matters most, tags least). The requirements specify that "myfeat" should match "my-feature-branch" and "squad" should match "stapler-squad".

Three TypeScript fuzzy libraries were evaluated:

**fuse.js** — Bitap (Shift-or) approximate string matching algorithm. Multi-field weighted `keys` config is first-class. Returns scores 0.0-1.0 with optional match position highlights. ~8kB gzip. Zero dependencies. Full TypeScript types. No word-boundary or consecutive-character bonus.

**fzf-for-js** (`fzf` on npm) — Port of the fzf CLI's Smith-Waterman-like algorithm. Best single-field fuzzy quality (consecutive-char and word-boundary bonuses). Multi-field search is not first-class: requires building a composite string or running multiple searches and merging. No built-in per-field weighting.

**match-sorter** — Seven-tier ranking (exact → starts-with → word-starts-with → contains → acronym → fuzzy). Multi-field support via `keys` array. Not true fuzzy: "myfeat" will not match "my-feature-branch" because none of the tiers handle non-contiguous character patterns. Fails the key requirement.

## Decision

Use **fuse.js** for session search in the Omnibar.

Configuration:
```typescript
{
  keys: [
    { name: "title",  weight: 0.5 },
    { name: "branch", weight: 0.3 },
    { name: "path",   weight: 0.15 },
    { name: "tags",   weight: 0.05 },
  ],
  includeScore: true,
  includeMatches: true,
  threshold: 0.4,
  minMatchCharLength: 1,
  ignoreLocation: true,
}
```

## Rationale

1. **Multi-field weighted keys is the core requirement.** The session search problem is inherently multi-field: a match on `title` is more valuable than a match on `path`. Fuse.js handles this natively through its `keys` weight configuration. fzf-for-js requires a composite string workaround that loses per-field weight control.

2. **Bitap quality is sufficient.** While fzf-for-js would produce marginally better ranking for single-field queries (consecutive-char bonuses), the difference is imperceptible for a session list of ≤100 items where all above-threshold results appear in the 8-item list.

3. **`ignoreLocation: true` fixes the path-match penalty.** Without this, Fuse.js penalizes matches that appear far from position 0 in the string. "squad" at position 8 in "stapler-squad" would score worse than "squad" at position 0. `ignoreLocation` removes this bias, treating all positions equally — correct for path and branch names.

4. **match-sorter is disqualified.** It cannot match non-contiguous character patterns. "myfeat" not matching "my-feature-branch" is a hard failure against the requirements.

5. **Bundle size is acceptable.** ~8kB gzip is trivial against the existing ~5MB application bundle.

## Consequences

- `fuse.js` is added to `web-app/package.json` dependencies.
- `web-app/src/lib/hooks/useSessionSearch.ts` is the only consumer.
- The Fuse index is rebuilt when the active session list changes (memoized). The `.search()` call runs synchronously on every keypress — the expected hot path.
- If the threshold proves too permissive (too many low-relevance matches), tighten to `0.3`. If too strict (real matches missed), loosen to `0.5`. The threshold is the primary tuning knob.
