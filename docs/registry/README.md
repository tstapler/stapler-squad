# Feature Registry

The registry tracks every backend RPC method and frontend feature in stapler-squad, mapping each to its test coverage. It is the single source of truth for "what exists and is it tested."

## Files

| File | Purpose |
|---|---|
| `schema.json` | JSON Schema definition for registry entries |
| `backend-features.json` | One entry per RPC method in `proto/session/v1/session.proto` |
| `frontend-features.json` | One entry per significant UI feature |
| `coverage-gaps.json` | Derived view: features where `tested: false` |

## Entry format

### Backend entry

```json
{
  "id": "session:create",
  "type": "backend",
  "backend": {
    "service": "SessionService",
    "method": "CreateSession",
    "protoFile": "proto/session/v1/session.proto",
    "markerFound": true,
    "handlerFile": "server/services/session_service.go"
  },
  "tested": true,
  "testIds": ["TestCreateSessionOneOff", "one-off-session-creation > creates session with one_off flag"],
  "lastModified": "2026-04-24T00:00:00.000Z"
}
```

- **id**: `scope:action` format. Scope = resource (`session`, `review-queue`, `approval`), action = verb (`create`, `list`, `get`, `delete`, `watch`).
- **markerFound**: `true` if the handler file contains a `// +api:` comment on or near the handler function. Add one if missing.
- **testIds**: Go test function names (`TestFoo`) or Playwright `describe > test` strings.

### Frontend entry

```json
{
  "id": "session-create-one-off",
  "type": "frontend",
  "frontend": {
    "component": "OmnibarCreationPanel",
    "path": "web-app/src/components/sessions/OmnibarCreationPanel.tsx",
    "markerLine": 21
  },
  "tested": true,
  "testIds": ["one-off-session-creation > shows one-off option in creation panel"],
  "lastModified": "2026-04-24T00:00:00.000Z"
}
```

- **id**: kebab-case, descriptive.
- **markerLine**: line number of the `// +feature:` comment in the component file (add one if missing).
- **testIds**: Playwright `describe > test` strings or Jest `describe > it` strings.

## How to add a marker comment

In Go handler files, add directly above the handler function:
```go
// +api: session:create
func (s *SessionService) CreateSession(...) {
```

In TypeScript component files, add near the top of the exported component:
```tsx
// +feature: session-create-one-off
export function OmnibarCreationPanel(...) {
```

These markers let tooling regenerate the registry automatically.

## How to update the registry manually

### New RPC method
1. Add a proto method to `session.proto`
2. Implement the handler in `session_service.go` with a `// +api:` marker
3. Add an entry to `backend-features.json` with `tested: false, testIds: []`
4. Write a test, then set `tested: true` and add the test name to `testIds`

### New frontend feature
1. Add a `// +feature:` comment to the component
2. Add an entry to `frontend-features.json` with `tested: false, testIds: []`
3. Write a Playwright test in `tests/e2e/<feature>.spec.ts`
4. Set `tested: true` and add the Playwright test IDs to `testIds`

### Existing feature now has tests
Find the entry by `id`, set `"tested": true`, append the test name(s) to `"testIds"`, update `"lastModified"`.

### Feature removed
Delete the entry from the registry file. Remove its test file too.

## coverage-gaps.json

This file lists feature IDs where `tested: false`. It is checked in CI — a PR that increases the gap count must include a justification in the PR description (e.g. "stubbed RPC not yet implemented"). Do not mark `tested: true` without a real test to back it up.

## Regenerating the registry

A generator script is planned (`scripts/gen-registry.go`). Until it exists, update the JSON files by hand following the rules above. The schema validates all entries — run `npx ajv validate -s docs/registry/schema.json -d docs/registry/backend-features.json` to check.
