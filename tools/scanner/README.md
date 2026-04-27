# Feature Registry Scanner

Generates and validates the per-feature registry under `docs/registry/features/`.

## Architecture

Each RPC and UI component gets **one JSON file**. Files are committed as the source of truth.
The monolithic aggregate files (`backend-features.json` etc.) are gitignored generated artifacts.

```
docs/registry/features/
  backend/<domain>/<action>.json   ← one per RPC  (committed)
  frontend/<type>/<id>.json        ← one per component (committed)
```

## Tools

| Tool | Language | Purpose |
|---|---|---|
| `backend/cmd/` | Go | Scans proto + `// +api:` markers → writes per-feature files |
| `frontend/src/` | TypeScript | Scans React files for `// +feature:` markers → writes per-feature files |
| `aggregate.py` | Python 3 | Assembles per-feature files into monolithic JSON (local/CI use) |
| `validate-registry.sh` | Bash | Compares committed per-feature files vs scanner output (CI) |

## Workflow

### Add a new RPC
1. Add the RPC to `proto/session/v1/session.proto`
2. Add the method → ID mapping to `backend/proto_scanner.go` (`methodToID` map)
3. Run `make registry-generate` — creates `docs/registry/features/backend/<domain>/<action>.json`
4. Add `// +api: <id>` marker to the handler in `server/services/`
5. Commit the new per-feature file

`TestScanProto_NoUnmappedMethods` fails CI if step 2 is skipped.

### Add test coverage to a feature
Edit the feature file directly — the scanner preserves `testIds` on regeneration:
```json
{
  "id": "session:create",
  "tested": true,
  "testIds": ["TestCreateSession_Success", "TestCreateSession_InvalidInput"]
}
```
`make test-ux-polish` picks them up automatically from the registry.

### Validate locally
```bash
make registry-diff                       # dry run: show divergence vs proto
./tools/scanner/validate-registry.sh    # full output with exit codes
```

## Backend scanner

```bash
# From repo root (the scanner is its own Go module under tools/scanner/)
cd tools/scanner && go build -o backend/cmd/scanner ./backend/cmd/
./tools/scanner/backend/cmd/scanner [protoFile] [servicesDir] [outputDir]

# Defaults:
#   protoFile   = proto/session/v1/session.proto
#   servicesDir = server/services/
#   outputDir   = docs/registry/features/backend
```

The scanner reads existing per-feature files before writing, so `testIds` and `tested` added
manually are never overwritten.

## Aggregate script

Rebuilds the gitignored monolithic files when needed by tooling:

```bash
python3 tools/scanner/aggregate.py docs/registry/features/backend docs/registry/backend-features.json
python3 tools/scanner/aggregate.py docs/registry/features/frontend docs/registry/frontend-features.json
# Or:
make registry-aggregate
```

## Running tests

```bash
cd tools/scanner
go test ./backend/...
```

`TestScanProto_NoUnmappedMethods` finds the real proto via `runtime.Caller` and fails if any
RPC method lacks a `methodToID` entry. It skips gracefully when run outside the repo.
