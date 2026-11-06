# Stack Research: lower-rework-turn-caps

## Proto / buf toolchain

- **buf CLI version**: `v2` (both `buf.yaml` and `buf.gen.yaml` use `version: v2`)
- **Proto generation command**: `make proto-gen` — runs `buf generate` using `buf.gen.yaml`
- **Go plugin**: remote `buf.build/protocolbuffers/go` + `buf.build/connectrpc/go`, output to `gen/proto/go/`
- **TypeScript plugin**: local `web-app/node_modules/.bin/protoc-gen-es` (protobuf-es), output to `web-app/src/gen/`
- Generated output is gitignored; always regenerate via `make build` (which depends on `proto-gen`)
- The `--feature sql/upsert` ent-gen note does not apply here (no schema changes)

## ConnectRPC versions

| Side | Library | Version |
|---|---|---|
| Go | `connectrpc.com/connect` | v1.20.0 (go.mod) |
| TypeScript | `@connectrpc/connect` | ^2.1.1 (package.json) |
| TypeScript | `@connectrpc/connect-web` | ^2.1.1 (package.json) |

TypeScript client is created via `createClient` + `createConnectTransport` from `@connectrpc/connect-web`
(see `GlobalDefaultsForm.tsx` line 5–6).

## React state management pattern (GlobalDefaultsForm.tsx)

- `useState` for every form field; no Redux/Zustand/Context
- Each field gets its own state pair, e.g.:
  ```ts
  const [maxAutoReworkIterations, setMaxAutoReworkIterations] = useState(0);
  ```
- Server values are loaded in a `useEffect` via the ConnectRPC client's `GetSessionDefaults` call,
  then set directly via the state setters (lines 62+)
- On submit, all state values are collected into the `UpdateGlobalDefaults` request body (lines 100+)
- Pattern to follow for `autonomous_max_turns`:
  1. Add `const [autonomousMaxTurns, setAutonomousMaxTurns] = useState(0);`
  2. Populate from `defaults.autonomousMaxTurns` in the `useEffect`
  3. Include in the submit payload
  4. Add a numeric `<input>` in the JSX matching the existing `maxAutoReworkIterations` input

## Go test framework (defaults_service_test.go)

- `github.com/stretchr/testify/assert` and `github.com/stretchr/testify/require`
- Tests use `require.NoError`, `require.NotNil`, `assert.Equal` style
- Each test creates an isolated service via `newIsolatedDefaultsService(t *testing.T)` helper
- Pattern to add a new test: call `newIsolatedDefaultsService`, issue an `UpdateGlobalDefaults`
  request with the new field set, then assert on the returned `SessionDefaultsConfig`

## Stale comment fix

Proto line 2282 reads `// 0 = use the server default (3).` — the default was lowered to 5
in PR #928 (`5b3ae539c`); update the comment to `(5)` as part of this change.

## Field number to use

Existing `UpdateGlobalDefaultsRequest` fields use numbers 1–8 (plus gaps at 3, 4, 5).
`SessionDefaultsConfig` has fields up to 14. Use the next available field number in each
message; check the proto file directly before assigning to avoid collisions.
