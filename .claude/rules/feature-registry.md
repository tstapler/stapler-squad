# Feature Registry Rule

When adding or modifying any feature — backend RPC, frontend UI, or both — you MUST update the feature registry before the PR is considered complete.

## What is the registry

`docs/registry/` contains three JSON files that track every feature and its test coverage:

| File | Tracks |
|---|---|
| `backend-features.json` | One entry per RPC method in `proto/session/v1/session.proto` |
| `frontend-features.json` | One entry per significant UI feature (page or creation mode) |
| `coverage-gaps.json` | Auto-derived: features where `tested: false` or `testIds` is empty |

Schema is defined in `docs/registry/schema.json`. See `docs/registry/README.md` for the full interaction guide.

## Required steps for every feature PR

### 1. Update backend-features.json

If you added or changed an RPC method:
- Find the matching entry by `id` (`scope:action` format, e.g. `session:create`)
- Set `"tested": true` once a Go test or e2e test covers the new behaviour
- Add the test function names to `"testIds"` (e.g. `"TestCreateSessionOneOff"`)
- Update `"lastModified"` to the current ISO 8601 timestamp

If you added a new RPC method:
- Add a new entry following the schema; `markerFound` should be `true` if you added a `// +api:` marker in the handler

### 2. Update frontend-features.json

If you added a new UI feature (new page, new creation mode, new major component):
- Add an entry with a kebab-case `id`, `type: "frontend"`, the component name, and the file path
- Set `tested: true` and populate `testIds` once a Jest or Playwright test covers it

If you modified an existing feature's component path or test coverage, update the matching entry.

### 3. Write an e2e test

Every new user-facing feature must have at least one Playwright e2e test in `tests/e2e/`.

- File name: `tests/e2e/<feature-name>.spec.ts`
- Use `test.describe('<feature-name>', ...)` so the test IDs are stable
- The `id` values in `testIds` must match `describe > test` names exactly
- Tests run against `http://localhost:8544` (the test server port)

### 4. Verify no coverage gaps

After updating the registry, confirm `coverage-gaps.json` does not grow. That file lists features with `tested: false` — any net increase needs justification in the PR description.

## Quick reference

```
New RPC method       → add entry to backend-features.json
New UI feature       → add entry to frontend-features.json
New test covering X  → set tested:true, add testId to X's entry
Modified RPC/UI      → update lastModified on the entry
```
