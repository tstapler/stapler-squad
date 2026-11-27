# ssq CLI: requirements and plan

## Requirements
- `ssq` calls the stapler-squad ConnectRPC API (default `http://localhost:8543`, `--url` / `STAPLER_SQUAD_URL`).
- Commands are derived from the proto service descriptors, so a new RPC or request field needs no CLI change.
- Unary RPCs only (Connect JSON over `POST /api/<service>/<rpc>`); streaming RPCs are skipped.
- Output is JSON by default. `--json` / `--json @file` / `--json -` supplies a raw request body; flags override its fields.
- Auth: Bearer token from `--token`, `STAPLER_SQUAD_TOKEN`, or the local token file in the config dir (honors `STAPLER_SQUAD_INSTANCE`), as for ssq-hooks (`pkg/localtoken`).
- A hung RPC fails after `--timeout` (default 15s) with a message naming the URL and RPC; the same budget is sent as `Connect-Timeout-Ms`.
- Friendly aliases for the common session RPCs: `ssq sessions list|get|restart|delete|ack|diff`.

## Design decision: runtime descriptors, not emitted Go
The generated `gen/proto/go` package (git-ignored, produced by `make proto-gen` / Bazel proto rules) registers every service
descriptor in `protoregistry.GlobalFiles`. `internal/ssqcli` walks that registry at startup and builds the cobra tree with
`dynamicpb` + `protojson`. Nothing new is generated or committed, there is no second generation step to wire into Make or
Bazel, and the CLI cannot drift from the protos.

## Layout
- `internal/ssqcli/spec.go`: descriptor walk to `[]Method` (service group, kebab name, input descriptor).
- `internal/ssqcli/client.go`: Connect unary client (timeout, auth, error decoding).
- `internal/ssqcli/command.go`: cobra tree, flag-to-message binding, aliases.
- `cmd/ssq/main.go`: entry point.

## Verification
httptest fake server unit tests (parallel); manual run against an isolated instance on 62871.
