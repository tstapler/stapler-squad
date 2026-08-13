# Research: Stack / Tooling (Agent 1)

## ent version and codegen command

- `go.mod`: `entgo.io/ent v0.14.5`
- `session/ent/generate.go` (verbatim, this is the only generator directive in the module):
  ```go
  package ent

  //go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
  ```
- Exact regeneration command to run after editing `session/ent/schema/backlog_item.go`:
  ```bash
  go generate ./session/ent
  ```
  This resolves to `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema` executed with cwd `session/ent`, i.e. it reads `session/ent/schema/*.go` and regenerates the sibling generated files (`session/ent/backlogitem.go`, `backlogitem_create.go`, `backlogitem_update.go`, `backlogitem_query.go`, `backlogitem_delete.go`, `migrate/schema.go`, etc.). `--feature sql/upsert` is required — it's what enables `SaveX`/upsert helpers already relied on elsewhere; omitting it would be a build-breaking regression (AC9 explicitly checks for this).
- No `ent generate --feature` flags beyond `sql/upsert` are configured (no `intercept`, `schema/snapshot`, `namedges`, etc.) — plan should not introduce them.

## Optional string field NULL/"" round-trip behavior (confirmed empirically, this ent version + sqlite3 driver)

- Driver: `github.com/mattn/go-sqlite3 v1.14.40` (CGO sqlite3 driver), opened via `sql.Open("sqlite3", dbPath)` then wrapped `entsql.OpenDB(dialect.SQLite, db)` in `session/ent_repository.go`.
- Migration mechanism: **auto-migration only**, no versioned SQL files. `session/ent_repository.go:86` calls `client.Schema.Create(context.Background())` on every startup, which diffs the ent schema against the live DB and issues `ALTER TABLE ... ADD COLUMN` for new fields. This is what will add `external_url` to pre-existing DBs.
- For an existing `Optional()` (non-`.Nillable()`) string field, confirmed by direct inspection of generated code for the sibling field `notes` (identical shape to what `external_url` will be):
  - `session/ent/migrate/schema.go:16`: `{Name: "notes", Type: field.TypeString, Nullable: true}` — the DB column is nullable.
  - `session/ent/backlogitem.go:46-48`: generated struct field is a plain `string` (NOT `*string`): `Notes string \`json:"notes,omitempty"\``.
  - `session/ent/backlogitem.go:233`: row-scan assignment is `_m.Notes = value.String` where `value` is a `sql.NullString` — i.e. ent scans into `sql.NullString` internally and then unconditionally reads `.String`, which is `""` when `.Valid == false` (NULL).
  - **Conclusion**: pre-existing rows with a NULL `external_url` column (added via auto-migration, no backfill) will read back as `ExternalURL == ""` with no panic — exactly what AC2 requires. This is existing, proven behavior in the same codebase for `notes`/`external_id`, not something novel to validate.

## Existing pattern to mirror: `notes` (and `external_id`) field addition

`notes` is the closest existing precedent for a plain optional string field threaded end-to-end, and `external_id` is the closest precedent for a GitHub-plugin-populated field. Mirror both:

1. **Schema** (`session/ent/schema/backlog_item.go`): add adjacent to `external_id`:
   ```go
   field.String("external_url").
       Optional(),
   ```
   No `.MaxLen()` / DB constraint — matches the design question's resolution ("500-char cap enforced in Go before Save, not via a DB constraint"); no existing field in this schema uses ent's length validators, so a Go-level cap in the plugin/mapper code (not a schema annotation) is the idiomatic-for-this-codebase choice. No new index needed — `external_id` has one (`index.Fields("external_id")`, used by `GetBacklogItemByExternalID` lookups); nothing analogous does lookups by URL, so no index for `external_url`.

2. **`BacklogItemData`** (`session/repository.go:257-282`): add `ExternalURL string` next to `ExternalID string` (line ~270 area).

3. **`backlogItemToData`** (`session/ent_repository_backlog.go:21-36` area): add `ExternalURL: item.ExternalURL,` next to `ExternalID: item.ExternalID,` (line 36).

4. **`CreateBacklogItem`** (`session/ent_repository_backlog.go`, ~line 94): add `.SetNillableExternalURL(&data.ExternalURL)` next to `.SetNillableNotes(&data.Notes)` / `.SetNillableExternalID(&data.ExternalID)` (line 93-94) — ent generates a `SetNillableExternalURL` setter automatically for any `Optional()` string field, no extra schema annotation needed for that.

5. **`BacklogItemUpdate`** (`session/repository.go:301-313` and the apply-loop in `session/ent_repository_backlog.go` ~line 220-245): only needed if the plan resolves the "reuse `UpdateBacklogItem`" design question affirmatively for the AC6 backfill path — mirror the `update.Notes != nil { u.SetNotes(*update.Notes) }` block (lines 232-233) as `update.ExternalURL != nil { u.SetExternalURL(*update.ExternalURL) }`. This bypasses `UserModifiedFields` exactly like `Notes`/other fields already do (none of the existing `BacklogItemUpdate` fields are gated by `UserModifiedFields` in this apply loop — that gate lives elsewhere, in `SyncOne`'s local-wins logic per the requirements doc, not in `UpdateBacklogItem` itself). This is a plan-phase decision, not confirmed further here.

6. **GitHub plugins** (`session/backlog_plugin_github.go:156-176`, `session/backlog_plugin_github_prs.go:194-212`): both `MapToBacklogItem` functions already set `ExternalID: item.ExternalID` from the already-populated `ExternalItem`; add `ExternalURL: item.URL` (truncated/capped to 500 chars in Go) alongside it, mirroring the existing `ExternalID` line exactly.

## Summary of exact commands for the plan phase

- Edit schema: `session/ent/schema/backlog_item.go`.
- Regenerate: `go generate ./session/ent` (invokes `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema`).
- Verify: `go build ./session/... ` then `make build && make test` per AC9 (auto-migration runs at repository-open time via `client.Schema.Create`, so no separate migration-apply step exists or is needed — this is not Rails/Prisma-style versioned migrations).
