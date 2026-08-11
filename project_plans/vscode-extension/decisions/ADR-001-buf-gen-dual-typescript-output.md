# ADR-001: Dual TypeScript Output from buf.gen.yaml for the VS Code Extension

**Status**: Accepted
**Date**: 2026-08-06
**Context**: project_plans/vscode-extension

## Context

The VS Code extension (`extensions/vscode/`) needs the same generated ConnectRPC
TypeScript bindings (`SessionService` client stub + message types) that
`web-app/` already consumes from `web-app/src/gen/`. `buf.gen.yaml` currently
has exactly one TypeScript output block:

```yaml
  # TypeScript protobuf message types generation (for web-app)
  - local: web-app/node_modules/.bin/protoc-gen-es
    out: web-app/src/gen
    opt:
      - target=ts
      - ts_nocheck=false
      - keep_empty_files=true
```

`web-app/src/gen/` is `.gitignore`d (`.gitignore:21`) but force-committed to
the repo anyway (confirmed via `git ls-files web-app/src/gen` returning
tracked files) — generated code is checked in so a fresh clone/CI run has
working TypeScript without requiring `buf` + `protoc-gen-es` to run first in
every context that touches `web-app/`. Requirements AC-9 states the extension
must be a pure API consumer with zero backend/proto *schema* changes; a build
tooling change to `buf.gen.yaml` (an additional output target, no `.proto`
edits) does not violate that constraint but is still an infrastructure
decision worth recording since it sets a precedent for how a second TS
consumer package attaches to codegen.

## Decision

Add a second `local` TypeScript output block to `buf.gen.yaml`, targeting
`extensions/vscode/src/gen`, using `extensions/vscode/node_modules/.bin/protoc-gen-es`
(the extension gets its own `@bufbuild/protoc-gen-es` devDependency, pinned to
the same version as `web-app/package.json`'s):

```yaml
  # TypeScript protobuf message types generation (for extensions/vscode)
  - local: extensions/vscode/node_modules/.bin/protoc-gen-es
    out: extensions/vscode/src/gen
    opt:
      - target=ts
      - ts_nocheck=false
      - keep_empty_files=true
```

`extensions/vscode/src/gen/` follows the same convention as `web-app/src/gen/`:
added to root `.gitignore`, then force-committed (`git add -f`) so the
extension builds from a fresh clone without requiring a `buf generate` step.
`make proto-gen`'s existing staleness check (`Makefile:398-413`) is extended
with one more `[ ! -f ... ]` clause for
`extensions/vscode/src/gen/session/v1/session_pb.ts`, so enabling this output
on an already-stamped tree doesn't silently skip generation for the new
consumer.

## Alternatives Considered

1. **pnpm workspace + shared `@stapler-squad/proto` package** — a single
   generated-code package consumed by both `web-app` and
   `extensions/vscode` via workspace `link:`. Rejected for v1: no
   `pnpm-workspace.yaml` exists anywhere in this repo today (confirmed —
   `research/stack.md`); introducing one is a structural change to the whole
   repo's JS tooling that this feature does not need to make. Revisit only if
   a third TS consumer of the proto bindings appears.
2. **Vendor/copy `web-app/src/gen` into the extension at build time** (a
   prebuild script that copies files across) — rejected: creates a second,
   driftable copy of generated code with no single source of truth, and
   couples the extension's build to `web-app/`'s build ordering for no
   benefit over generating directly from `.proto`.
3. **Hand-write a minimal TS client** (skip codegen, write fetch/JSON calls
   by hand against the RPC method names) — rejected per `research/build-vs-buy.md`:
   throws away compile-time type safety on every field name for a small
   codegen-config cost.

## Consequences

- `buf generate proto` (invoked by `make proto-gen`) now writes to two
  locations; both must be reviewed in future proto-touching PRs.
- `extensions/vscode/package.json` needs `@bufbuild/protoc-gen-es` and
  `@bufbuild/protobuf` as devDependencies/dependencies, matching
  `web-app/package.json`'s pinned versions.
- Two TypeScript copies of the same generated types exist in the repo
  (`web-app/src/gen/` and `extensions/vscode/src/gen/`) — this is the accepted
  tradeoff of Alternative 1's rejection; if drift or duplication cost becomes
  painful, migrate to a pnpm workspace at that point.
