# Feature Registry Rule

When adding or modifying any feature — backend RPC, frontend UI, or both — you MUST update the feature registry before the PR is considered complete.

## What is the registry

`docs/registry/` uses **per-feature JSON files** — one file per feature, not monolithic files:

```
docs/registry/
  features/
    backend/   ← one .json per RPC method / backend feature
    frontend/  ← one .json per UI feature
  schema.json  ← entry schema
  README.md    ← full interaction guide
```

The monolithic `backend-features.json`, `frontend-features.json`, and `coverage-gaps.json` are **generated artifacts** produced by `make registry-generate`. Never edit them directly — edit the per-feature source files instead.

Schema is defined in `docs/registry/schema.json`.

## Required steps for every feature PR

### 1. Add or update a per-feature file in `docs/registry/features/`

**New RPC method** → create `docs/registry/features/backend/<feature>.json`:
```json
{
  "id": "scope:action",
  "type": "backend",
  "name": "Human readable name",
  "markerFound": true,
  "tested": false,
  "testIds": []
}
```
Set `markerFound: true` if you added a `// +api: scope:action` marker in the handler.

**Existing RPC method** → find and edit the matching file under `docs/registry/features/backend/`:
- Set `"tested": true` once a Go test or e2e test covers the new behaviour
- Add test function names to `"testIds"` (e.g. `"TestCreateSessionOneOff"`)
- Update `"lastModified"` to the current ISO 8601 timestamp

**New UI feature** → create `docs/registry/features/frontend/<feature>.json`:
```json
{
  "id": "kebab-case-id",
  "type": "frontend",
  "name": "Component name",
  "filePath": "web-app/src/...",
  "tested": false,
  "testIds": []
}
```

### 2. Write an e2e test

Every new user-facing feature must have at least one Playwright e2e test in `tests/e2e/`.

- File name: `tests/e2e/<feature-name>.spec.ts`
- Use `test.describe('<feature-name>', ...)` so the test IDs are stable
- The `id` values in `testIds` must match `describe > test` names exactly
- Tests run against `http://localhost:8544` (the test server port)

### 3. Regenerate and verify no new coverage gaps

```bash
make registry-generate
```

This regenerates the aggregated JSON files. Check that `docs/registry/coverage-gaps.json` count does not grow — any net increase in untested features needs justification in the PR description.

## Quick reference

```
New RPC method       → create docs/registry/features/backend/<feature>.json
New UI feature       → create docs/registry/features/frontend/<feature>.json
New test covering X  → set tested:true, add testId to X's entry file
Modified RPC/UI      → update lastModified on the entry file
Always after changes → make registry-generate
```
