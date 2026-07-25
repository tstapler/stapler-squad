# Stack Research: Proto Codegen, Exhaustive Switches, and Wire Format

## 1. Proto Codegen Pipeline

### How `make generate-proto` works

Proto files live in `proto/session/v1/`. Generation is driven by `buf` with the config at `buf.gen.yaml` (repo root):

```yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen/proto/go
    opt: [paths=source_relative]
  - remote: buf.build/connectrpc/go
    out: gen/proto/go
    opt: [paths=source_relative]
  - local: web-app/node_modules/.bin/protoc-gen-es
    out: web-app/src/gen
    opt: [target=ts, ts_nocheck=false, keep_empty_files=true]
```

The Makefile target `proto-gen` runs `buf generate proto` when any `*.proto` file is newer than `.proto-gen.stamp`. Two artefact directories are produced:
- `gen/proto/go/session/v1/` — Go bindings (used as `sessionv1` import alias)
- `web-app/src/gen/session/v1/` — TypeScript bindings (imported as `@/gen/session/v1/types_pb`)

### What a proto enum looks like after codegen

**Go side** (example: `SubStatus`): The generator produces a Go type alias to `int32` and named constants with the full-prefix name:

```go
// In gen/proto/go/session/v1/types.pb.go (inferred from usage)
sessionv1.SubStatus_SUB_STATUS_UNSPECIFIED  // = 0
sessionv1.SubStatus_SUB_STATUS_IDLE         // = 1
sessionv1.SubStatus_SUB_STATUS_PROCESSING   // = 2
```

Callers in `server/adapters/instance_adapter.go` use them verbatim:
```go
return sessionv1.SubStatus_SUB_STATUS_PROCESSING
```

**TypeScript side** (from `web-app/src/gen/session/v1/types_pb.ts`): `protoc-gen-es` v2 emits a TypeScript `enum` with the prefix stripped to just the member name:

```typescript
export enum SubStatus {
  UNSPECIFIED = 0,
  IDLE = 1,
  PROCESSING = 2,
  NEEDS_APPROVAL = 3,
  ERROR = 4,
  TESTS_FAILING = 5,
  RATE_LIMITED = 6,
  INPUT_REQUIRED = 7,
  READY = 8,
  SUCCESS = 9,
}
```

Each enum also gets a `Schema` descriptor constant (e.g. `SubStatusSchema`) for runtime reflection. Consumers import the enum directly:
```typescript
import { SubStatus } from "@/gen/session/v1/types_pb";
```

Adding `DetectedStatus` to `types.proto` and running `make generate-proto` will automatically produce both the Go constants and the TypeScript enum. No additional plugin configuration is needed.

---

## 2. Go Exhaustive Switch: `exhaustive` Linter

### Current state

The `exhaustive` linter is **not** in the current `.golangci.yml` enabled list. The enabled linters are:

```yaml
linters:
  enable:
    - depguard
    - forbidigo
    - gochecknoglobals
    - ineffassign
    - nilnil
    - prealloc
    - staticcheck
    - govet
    - unused
```

No `exhaustive` entry exists anywhere in `.golangci.yml`.

### What `exhaustive` enforces

The `exhaustive` linter (github.com/nishanths/exhaustive) reports Go `switch` statements that do not cover all values of a Go `const`-typed enum. It also checks map literal initializations for exhaustive key coverage. It integrates cleanly with golangci-lint v2.

### Minimal config to add it

Add one line to the `enable` block in `.golangci.yml`:

```yaml
linters:
  enable:
    # ... existing entries ...
    - exhaustive
  settings:
    exhaustive:
      # Treat switch statements with a default clause as exhaustive
      # (allows a safety-net default while still catching truly missing cases
      # when default is absent).
      default-signifies-exhaustive: false
      # Only require exhaustiveness for types in packages we own
      # (avoids false positives on proto-generated int32 aliases from buf).
      package-scope-only: false
```

**Important nuance**: The `exhaustive` linter checks switches on Go `const`-typed enums declared in Go packages, not proto-generated types. Because the Go proto bindings generate `SubStatus` as an `int32` type alias (not a true `iota` enum), the linter may not catch switches on proto types without `//nolint` guidance. The most robust enforcement for proto-generated switches is the `default:` + `panic("unhandled")` pattern or a compile-time array trick.

### Alternative: compile-time array guard (no linter needed)

```go
// Compile-time assertion: this array must have one entry per DetectedStatus constant.
// Adding a new constant without extending this array will fail to compile.
var _ = [10]struct{}{} // one per iota value in detection.DetectedStatus
```

This is simpler and tooling-independent but more fragile (manual count). The `exhaustive` linter is preferable for the internal Go `DetectedStatus` enum (defined with `iota`), but the proto-generated type requires the `default: panic` pattern.

---

## 3. TypeScript Exhaustive Switch: The `never` Trick

### Reference implementation: `SubStatusChip.tsx`

`web-app/src/components/sessions/SubStatusChip.tsx` demonstrates the correct pattern for switching on a proto-generated TypeScript enum. It uses `SubStatus` imported from the generated bindings and switches exhaustively with a `default` fallthrough to `null`:

```typescript
import { SubStatus } from "@/gen/session/v1/types_pb";

export function SubStatusChip({ subStatus }: SubStatusChipProps) {
  switch (subStatus) {
    case SubStatus.PROCESSING:   return <span>Thinking…</span>;
    case SubStatus.NEEDS_APPROVAL: return <span>Approve</span>;
    // ... all other cases ...
    case SubStatus.UNSPECIFIED:
    default:
      return null;
  }
}
```

**This does not enforce exhaustiveness at compile time.** Adding `SubStatus.RATE_LIMITED` to the proto and regenerating would silently fall through to `default`.

### Adding `never` exhaustiveness checking

To get compile-time enforcement when adding a new enum value, replace the `default` branch:

```typescript
function assertNever(x: never): never {
  throw new Error(`Unhandled DetectedStatus: ${x}`);
}

switch (detectedStatus) {
  case DetectedStatus.EXECUTING:   return ...;
  case DetectedStatus.PROCESSING:  return ...;
  // all values listed
  case DetectedStatus.UNSPECIFIED: return null;
  default:
    return assertNever(detectedStatus); // TypeScript error if any case missing
}
```

TypeScript narrows `detectedStatus` to `never` after all enum members are covered. Passing a `never` to `assertNever` compiles only when all paths are handled — adding a new proto enum value without a matching case produces a type error.

**Key constraint**: This works only for TypeScript numeric enums (which is what `protoc-gen-es` v2 generates — a standard TypeScript `enum`). It would not work for string unions.

**No ESLint plugin is required**: The `never` trick is pure TypeScript and works with the project's existing `tsc` setup.

---

## 4. Proto Enum Forward-Compatibility: Unknown Values

### The wire format

Connect-RPC uses proto3 binary encoding on the wire. In proto3, enum fields are encoded as `int32` varint. When a client receives a value it doesn't recognize (a new enum member added after the client was compiled), proto3 behaviour is:
- **Go**: The field is stored as the raw `int32` value; no error is thrown.
- **TypeScript (`protoc-gen-es` v2)**: The generated code uses numeric enums. An unrecognized integer value is returned as-is — TypeScript's `switch` will fall through to `default` without a compile error.

### Impact of adding `DETECTED_STATUS_UNKNOWN`

If the server sends `DETECTED_STATUS_UNKNOWN = 7` and an old client (compiled before this enum value existed) receives it, the TypeScript code will receive the integer `7` in the enum field. Since old clients have no `case 7:` branch, the `default:` branch fires (or `assertNever` throws at runtime).

**Mitigation**: Ensure every switch over `DetectedStatus` has a safe `default:` or `assertNever` path. The `DETECTED_STATUS_UNSPECIFIED = 0` value already provides a zero-value safe default. The critical path in `SubStatusChip.tsx` already returns `null` for `UNSPECIFIED`, so adding an unknown-value case there is a matter of ensuring the `default:` returns `null` or a generic chip — not a crash.

### `DETECTED_STATUS_UNKNOWN` as the catch-all

Per R1.1, `DETECTED_STATUS_UNKNOWN` is the replacement for what `StatusReady` (the `.*` catch-all pattern) currently produces. Assigning it a non-zero integer (e.g. `8`) means old clients that only know values 0–7 will treat it as `default`. Since the old default in `SubStatusChip` and `StatusBadge` is either `null` or a generic label, this is safe — old clients just show no chip for the new "unknown" state, which is the correct behaviour.

---

## 5. `SessionStatusChangedEvent` and the WatchSessions Stream

### Current event structure

`proto/session/v1/events.proto` defines a `SessionEvent` oneOf with these variants:

```protobuf
oneof event {
  SessionCreatedEvent  session_created = 2;
  SessionUpdatedEvent  session_updated = 3;
  SessionDeletedEvent  session_deleted = 4;
  SessionStatusChangedEvent status_changed = 5;
  UserInteractionEvent user_interaction = 6;
  SessionAcknowledgedEvent session_acknowledged = 7;
  ApprovalResponseEvent approval_response = 8;
  NotificationEvent notification = 9;
}
```

`SessionStatusChangedEvent` currently carries:
```protobuf
message SessionStatusChangedEvent {
  string session_id = 1;
  SessionStatus old_status = 2;
  SessionStatus new_status = 3;
  optional string detected_status = 4;   // Raw string — the problem
  optional string detected_context = 5;
  WorkingState working_state = 6;
}
```

The `detected_status` field (field 4) is a **raw string**, not a typed enum. This is the root of the type-safety problem: the frontend's `StatusBadge.tsx` switches on this string, and `getDetectedStatusInfo()` maps strings like `"Ready"`, `"Processing"`, `"Active"` to display info.

### `SessionUpdatedEvent` — the target

`SessionUpdatedEvent` currently carries just the full `Session` proto and a list of updated field names:
```protobuf
message SessionUpdatedEvent {
  Session session = 1;
  repeated string updated_fields = 2;
}
```

The `Session` proto already has a `sub_status` field (field 54) but does **not** yet have a `DetectedStatus` field — that is what R1 adds to `types.proto` and R3 plumbs into `SessionUpdatedEvent` (either directly or via the `Session` message's new `detected_status` field).

### Redux handling in the frontend

The stream event handler dispatches either `upsertSession` (for `session_updated`, `session_created`) or `updateSessionStatus` (for `status_changed`). They write to two separate state stores:
- `sessionsAdapter` entity store (keyed by session ID) — updated by `upsertSession`
- `detectedStatusMap` (a plain Record) — updated by `updateSessionStatus`

This is the dual-track split described in the requirements. The `updateSessionStatus` reducer correctly clears `detectedStatusMap` when `newStatus !== ACTIVE`, but `upsertSession` does **not** touch `detectedStatusMap` — so a `session_updated` event that transitions a session from ACTIVE to STOPPED leaves `detectedStatusMap` stale.

---

## Summary of Key Findings

1. **Codegen is straightforward**: Adding `DetectedStatus` to `proto/session/v1/types.proto` and running `make generate-proto` produces both a Go `int32` const block (with `sessionv1.DetectedStatus_DETECTED_STATUS_*` names) and a TypeScript numeric enum (`DetectedStatus.EXECUTING`, etc.) with no additional configuration. The `protoc-gen-es` plugin strips the repeated prefix, matching the existing `SubStatus` pattern exactly.

2. **`exhaustive` linter is not enabled**: `.golangci.yml` has no `exhaustive` entry. For the internal Go `DetectedStatus` iota enum, adding `- exhaustive` to the `enable` list (with `default-signifies-exhaustive: false`) provides compile-time switch coverage. For proto-generated types, use `default: panic("unhandled DetectedStatus: " + s.String())` instead, since the linter may not recognize proto-generated `int32` aliases as exhaustible.

3. **`StatusBadge` and `StatusChangedEvent` use raw strings today**: `SessionStatusChangedEvent.detected_status` is `optional string` on the wire, and `StatusBadge.tsx` switches on raw string literals like `"Ready"`, `"Processing"`, `"Active"`. This is the specific fragility R6 targets — renaming `StatusActive` to `StatusExecuting` on the Go side leaves the frontend `"Active"` string falling through to the `default` branch silently. The fix is to replace `detected_status: string` in `SessionStatusChangedEvent` with the new `DetectedStatus` proto enum and use the `never` trick in the frontend switch.
