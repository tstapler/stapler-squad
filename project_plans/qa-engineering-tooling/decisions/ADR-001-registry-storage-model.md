# ADR-001: Feature Registry Storage Model

Status: Accepted
Date: 2026-04-17
Deciders: Solo developer (owner)

---

## Context

The QA tooling system requires a living inventory of both backend API features (ConnectRPC methods) and frontend UI features (React components). This inventory — the "feature registry" — must be readable by E2E tests (for feature ID decoration), CI validation jobs, UX analysis scripts, and coverage reporting tools.

The registry must:
- Remain accurate as the codebase changes
- Be auditable (who changed what, when)
- Work offline and without a running server
- Not become a maintenance burden

Three storage models were evaluated.

---

## Options Considered

### Option A: Static JSON Files Committed to Repo (Chosen)

Registry lives at `docs/registry/backend-features.json` and `docs/registry/frontend-features.json`. Scanners generate these files. The committed files are the source of truth. CI validates that the committed files match the current scanner output; errors if divergence exceeds 2%.

Pros:
- Git history preserved; registry changes appear in PR diffs
- Works offline; no runtime dependency
- Human-readable; developer can audit the registry directly
- Compatible with all downstream consumers (tests, scripts, CI) without an API
- Rollback is `git revert`
- Follows the pattern used by mature internal tooling at GitHub, Netflix, and others for API catalogs

Cons:
- Registry can drift if CI scanner fails or is skipped
- Manual edits to "fix" scanner bugs will be overwritten on next scan
- Must run `make registry-generate` locally to see registry updates before push

### Option B: Live HTTP Service / API Endpoint

Registry exposed via the stapler-squad server itself at `/api/v1/features`.

Pros:
- Always current (regenerated at startup)
- Single source of truth

Cons:
- E2E tests and CI scripts depend on a running server; circular dependency
- Adds versioning, backward compatibility, and operational concerns to the registry
- Server restart required to see updates
- Over-engineered for a solo developer context
- Rejected by architecture research as adding zero benefit over static JSON at this scale

### Option C: Generated On-Demand at Test Time

Scanners run as test fixtures; registry generated fresh per test run.

Pros:
- Always current for tests that run it

Cons:
- Scanner runs on every test execution — adds 30-60 seconds to every run
- Scanner failures cause test failures (hides the real test failure)
- No historical registry; can't diff what changed between runs
- Rejected unanimously in research

---

## Decision

**Static JSON files committed to repo with CI validation.**

Registry files at `docs/registry/backend-features.json` and `docs/registry/frontend-features.json` are committed artifacts. Scanners are the write path. CI validates the committed registry against a fresh scan on every PR touching source files. Divergence threshold: 2% of total entries.

---

## Schema Versioning

Registry files include a top-level `"version": "1"` field. Breaking schema changes (removing or renaming fields) require:
1. Increment the version field
2. Update all consumers in the same PR
3. Document the migration in this ADR as an amendment

No auto-migration tooling. The registry is small enough that manual migration is acceptable at MVP scale.

---

## Consequences

- Developers must run `make registry-generate` after adding a new API handler or feature component with a marker
- CI will catch any developer who forgets; the validation step posts a PR comment with the diff
- Registry drift is bounded by CI: no PR can merge with >2% divergence
- The registry is a committed artifact and appears in PR diffs, providing an automatic changelog of feature additions

---

## References

- `project_plans/qa-engineering-tooling/research/findings-architecture.md` — Options 1A through 1D
- `project_plans/qa-engineering-tooling/research/synthesis.md` — Dominant trade-off analysis
