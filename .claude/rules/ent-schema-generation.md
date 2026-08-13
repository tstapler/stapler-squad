# ent ORM Schema Generation — Always Pass `--feature sql/upsert`

Always use the command from `session/ent/generate.go` when regenerating the ent schema. The `--feature sql/upsert` flag is required.

**Wrong:**
```bash
go run entgo.io/ent/cmd/ent generate ./session/ent/schema
```

**Right:**
```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

## Why

Omitting `--feature sql/upsert` silently breaks `UpsertRule` and similar upsert methods — the generated code compiles but the upsert operations don't exist. The `-mod=mod` flag is also required to allow module graph updates. The correct command is in `session/ent/generate.go` as the `//go:generate` directive — always check there first or run `go generate ./session/ent/`.
