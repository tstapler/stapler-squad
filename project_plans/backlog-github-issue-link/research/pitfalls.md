# Research: Pitfalls

## 1. ent codegen pitfalls

**What actually changes when you add `field.String("external_url").Optional()` to
`session/ent/schema/backlog_item.go` and run the correct generate command**
(`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`,
per `session/ent/generate.go`):

- `session/ent/migrate/schema.go` — a new `{Name: "external_url", Type:
  field.TypeString, Nullable: true}` column entry is added to
  `BacklogItemsColumns` (mirroring the existing `external_id` entry at line
  123).
- `session/ent/backlogitem.go` — new `ExternalURL string` struct field +
  `scanValues`/`assignValues` wiring.
- `session/ent/backlogitem/backlogitem.go` — new `FieldExternalURL` constant
  and predicate helpers (`backlogitem.ExternalURL(...)`, `.ExternalURLEQ`, etc.).
- `session/ent/mutation.go` — `SetExternalURL`/`ExternalURL`/`ClearExternalURL`
  mutation methods.
- `session/ent/backlogitem_create.go` / `backlogitem_update.go` — new
  `SetExternalURL`/`SetNillableExternalURL` builder methods (the pattern
  `CreateBacklogItem` already uses for `SetNillableExternalID` at
  `session/ent_repository_backlog.go:94` should be mirrored).
- `session/ent/backlogitem_query.go` and predicate/where files — new query
  support.

**Compile error vs. silent runtime bug — which one actually happens:**

Because `ent generate` regenerates all of the above files atomically from one
template pass over `schema/*.go`, the realistic failure modes split into two
very different categories:

- **Forgetting to regenerate at all** after editing `schema/backlog_item.go`:
  any new Go code that references `item.ExternalURL` or calls
  `.SetExternalURL(...)` simply won't compile — the field/method doesn't
  exist in the stale generated files yet. `go build ./session/...` catches
  this immediately. **This is a compile error, not a silent bug** — safe by
  construction, and exactly what AC9 ("no compile errors in `session/ent/*`")
  is guarding.
- **Running `ent generate` without `--feature sql/upsert`**: this *does*
  regenerate `migrate/schema.go` correctly (the new column is present, so
  schema creation/migration and the ExternalURL round-trip all work fine) —
  but it silently drops `UpsertRule`/`OnConflict`-style builder methods
  elsewhere in the generated code. Any pre-existing code path that depends on
  those methods either fails to compile (safe) or, if no such method is
  called anywhere yet, produces no visible symptom until someone later adds
  upsert-based code and it "mysteriously" isn't there. **Grep the repo before
  and after regeneration** for existing `.OnConflict(` / upsert usage (none
  found in `session/backlog_sync.go`'s current create/update path — it uses
  plain `Create()`/`UpdateOneID()`, not upsert) to confirm this flag omission
  wouldn't be caught by this feature's own tests. This is the "silent
  UpsertRule breakage" this repo's CLAUDE.md/`.claude/rules/ent-schema-generation.md`
  warns about — but it does **not** intersect with the ExternalURL column
  path itself; it's an orthogonal risk that happens to be triggered by the
  same wrong command.

**The one real silent-runtime-bug vector**: hand-patching a single generated
file (e.g. adding the `ExternalURL` struct field to `backlogitem.go` by hand
instead of regenerating everything) instead of running full codegen. This is
fragile — `scanValues`/column-ordinal wiring is generated in lockstep across
files — and could produce a build that compiles but where the field is never
persisted or always reads back empty. Nothing in this plan should do this;
flagging it only because "just add the field to the struct" is the wrong
intuition for a codebase this ent-generated.

**Nillable trap**: use `.Optional()` **without** `.Nillable()`. `Optional()`
alone generates a plain `ExternalURL string` (ent maps SQL `NULL` → Go `""`
automatically on scan — no nil-pointer panic, matches AC2's "pre-existing
rows read back `ExternalURL == ""`"). Adding `.Nillable()` on top would
instead generate `ExternalURL *string`, forcing nil-checks into every
consumer (`backlog_context.go`'s new prompt section, both hand-built
`ent.BacklogItem{}` literals, `backlogItemToData`) that this plan doesn't
otherwise need. Match the existing `external_id` field's shape exactly
(`field.String("external_id").Optional()`, no `.Nillable()`) — confirmed at
`session/ent/schema/backlog_item.go`.

## 2. Token-budget truncation pitfall (AC5)

`BuildTokenBudgetedPrompt` (`session/backlog_context.go:133-154`):

```go
// Pass 2: truncate description to 500 chars.
truncatedItem := *item
truncatedItem.Description = sanitizeField(item.Description, 500)
output = BuildSessionInitialPrompt(&truncatedItem, nil)
```

- `truncatedItem := *item` is a full value copy of the `ent.BacklogItem`
  struct. `ExternalURL` will be a plain `string` field (see §1), and Go
  strings are immutable value types (a string header copy, not a pointer to
  shared mutable buffer) — copying the struct and then only reassigning
  `.Description` leaves `truncatedItem.ExternalURL` holding the same string
  value as `item.ExternalURL`, correctly. **No aliasing bug**: there's no
  pointer field involved for `ExternalURL`, so there's nothing to alias.
  (Contrast with `PlanApprovedAt *time.Time` on the same struct, which *would*
  alias on a shallow copy — irrelevant here since the new field isn't a
  pointer.)
- Since the new "Linked GitHub Issue" section lives inside
  `BuildSessionInitialPrompt` itself (called identically by the full render,
  pass 1, and pass 2), it is automatically present in all three outputs with
  no extra plumbing — confirms AC5 ("both truncation passes still include the
  fact line and instruction line unchanged") is satisfied by construction as
  long as the new section doesn't do its *own* independent truncation that
  forgets to run on the `truncatedItem` copy.
- **Budget-shift risk**: the new section (one fact line + one instruction
  line, ~100–150 characters) adds a small **fixed** token cost to every
  render, including pass 1 and pass 2. For a prompt that previously landed at
  exactly the 4000-token line via pass 1, this could push it into needing
  pass 2 where it didn't before; for a prompt already needing pass 2, it adds
  ~25–40 estimated tokens (`len/4`) on top. This is a real, if minor,
  regression in effective budget headroom for every item, not just ones with
  a linked issue.
- **Pass 2 already has no hard ceiling**: note that pass 2 does not re-check
  `estimated <= 4000` before returning — it truncates Description to 500
  chars and returns unconditionally, even if `AcceptanceCriteria` or `Notes`
  (uncapped in pass 2) still push the total over budget. So "could this
  change push a prompt over budget where it wasn't before" is already true
  today independent of this feature; the new section only shifts the margin
  slightly. Recommendation: keep the new section to exactly two short lines
  (URL fact line + instruction line) to minimize this fixed overhead, and
  don't add per-item conditional expansion (e.g. don't also echo the raw
  issue number a second time).

## 3. Backfill limitation pitfall (AC6)

Confirmed in `session/backlog_sync.go:195-299` (`SyncOne`): the loop only
processes `items` returned by `plugin.Fetch(ctx, cfg, cursor)`, and both
GitHub plugins' `Fetch` implementations only query `state=open` (or
equivalent) issues/PRs. Consequently:

- If an issue/PR is closed after the backlog item was first created (before
  `ExternalURL` existed), it will **never** again appear in a subsequent
  `Fetch` result for that source, so the existing-item branch of `SyncOne`
  will never see it to backfill `ExternalURL`.
- If the source itself is disabled/deleted, `SyncOne` never runs for it at
  all.
- In both cases the row's `ExternalURL` stays permanently `""`.

Requirements.md's "Explicitly out of scope" section already states this
outcome as an *intended, documented limitation*, not a bug — AC6 says the
same ("with the documented, tested limitation..."). **This is not a
contradiction to flag or resolve** — it *is* the correct framing, and the
implementation should:
1. Add a code comment at the backfill call site in `SyncOne` stating exactly
   this limitation (why a closed issue can never be backfilled retroactively
   without a broader `state=all` fetch, which is explicitly out of scope).
2. Add a test that pins this as expected behavior (e.g. an existing item with
   empty `ExternalURL` whose `ExternalID` is *not* present in the `Fetch`
   result stays unchanged after `SyncOne` runs) rather than a test that
   merely doesn't cover the case. AC8 already calls for a
   "backfill/known-limitation pin" test — this confirms that test needs to
   assert the *negative* case (no backfill when item isn't in the open-state
   fetch results), not just the positive backfill case.

**A related, easy-to-miss correctness bug this reveals** (not explicitly
asked but discovered while reading `SyncOne`): the existing-item branch only
issues `UpdateBacklogItem` at all if `anyField` is `true`:

```go
if !anyField {
    skipped++
    continue
}
```

`anyField` is currently only set by the three local-wins fields
(title/description/priority), each individually gated by
`UserModifiedFields`. If a user has manually edited all three fields for an
item (all three gated off), `anyField` stays `false` and the loop `continue`s
— **no `UpdateBacklogItem` call happens at all**, even if the plan's
unconditional `ExternalURL` backfill has a real, non-empty value to write.
The implementation must set `anyField = true` whenever
`existing.ExternalURL == "" && data's fetched URL != ""` (independent of
`UserModifiedFields`), or the "unconditional" backfill promised in AC6 will
silently never fire for any item whose other three fields are all
user-locked. Recommend a test with all three fields user-modified + an empty
`ExternalURL` + a plugin URL present, asserting the backfill still happens.

## 4. String-capping pitfall (Title/Description precedent vs. new ExternalURL cap)

`MapToBacklogItem` in both `session/backlog_plugin_github.go:158-166` and
`session/backlog_plugin_github_prs.go:194-202` truncates via raw byte
slicing:

```go
if len(title) > 200 {
    title = title[:200]
}
```

**Clarifying the actual risk in Go** (the task description's "corrupting the
string or panicking" is half right): byte-index slicing a Go string **never
panics** on a length-based cut like this — Go strings are just byte slices,
and `s[:n]` for `n <= len(s)` is always memory-safe. The real risk is
**corruption, not a panic**: cutting mid-rune can produce a string whose
trailing bytes are an invalid/incomplete UTF-8 sequence, which typically
renders as one or more U+FFFD replacement characters, or — worse for a URL
specifically — silently truncates one byte into a multi-byte percent-encoded
or IDN sequence, producing a URL that is neither the original nor a
well-formed truncation.

**Should `ExternalURL` follow the same raw-slice pattern?** Recommendation:
**yes, for consistency, with the risk explicitly accepted rather than
silently inherited**:

- GitHub's `html_url` field (what both plugins already read via
  `issue.HTMLURL` / `pr.HTMLURL`) is a plain ASCII URL by contract — GitHub
  API URLs are `https://github.com/<owner>/<repo>/issues/<n>` or
  `/pull/<n>`, never containing raw multi-byte UTF-8 (any IDN-hostname or
  non-ASCII-query edge case would already be percent-encoded/punycode ASCII
  by the time it's serialized into `html_url`).
- 500 chars is far larger than any realistic GitHub issue/PR URL (~50–90
  chars), so this cap will essentially never trigger in practice — this is a
  defense-in-depth cap, not a functional truncation path that needs to
  preserve URL semantics after cutting (a URL truncated at an arbitrary byte
  offset is already broken/unusable regardless of rune-safety).
- Requirements.md's out-of-scope list does not ask for fixing the
  pre-existing Title/Description raw-slice pattern, and matching that
  existing style keeps the change minimal and consistent (same helper
  pattern, same call sites, same diff shape reviewers already expect from
  this codebase).
- **Minimal safe hardening** (optional, cheap, not required by any AC): if
  wanting to close the theoretical corruption gap without adding a new
  helper, run `strings.ToValidUTF8(truncated, "")` after the byte-slice cut —
  one stdlib call, no new rune-walking logic, strips any invalid trailing
  partial sequence. Not required to satisfy the ACs; call out as a
  nice-to-have only if the implementer has spare scope, not a blocker.

## 5. Concurrency/locking pitfall (`SyncOne` + `lockForSource`)

- `lockForSource(source.ID.String())` (`session/backlog_sync.go:25-29`)
  serializes **only** concurrent `SyncOne` calls for the *same* `ItemSource`
  (periodic tick vs. manual `TriggerSync` RPC racing each other). It does
  **not** lock against other callers of `UpdateBacklogItem` for the same
  backlog item from elsewhere — e.g. `AttachSessionToItem` /
  `SpawnSessionFromItem` in `server/services/backlog_service.go`, or a
  user-initiated PATCH from the web UI.
- This is **not a new race introduced by adding `ExternalURL`**, because
  `EntRepository.UpdateBacklogItem` (`session/ent_repository_backlog.go:186-248`)
  builds a **column-scoped** `UpdateOneID(id)` — it only calls `.SetX(...)`
  for the specific fields present in the `BacklogItemUpdate` struct, then
  issues one `UPDATE ... SET <only-those-columns> WHERE id=?` via `Save()`.
  There is no full-row read-modify-write. So a concurrent `SyncOne` backfill
  writing only `external_url` and a concurrent `AttachSessionToItem` write
  touching only `title`/`notes`/etc. touch **disjoint SQL columns** and
  cannot lose each other's updates — this holds today for the three existing
  local-wins fields and will hold identically for a new `ExternalURL *string`
  field added to `BacklogItemUpdate` following the same pattern.
- The only pre-existing race this doesn't protect against — unrelated to
  this feature — is the `Get` (for precondition check) followed by a
  separate `Save()` in `UpdateBacklogItem`: a precondition based on stale
  `updated_at`/`status` read before another writer's concurrent commit. Since
  the planned `ExternalURL` backfill will call `UpdateBacklogItem` with
  `precondition == nil` (matching how `SyncOne` already calls it for the
  other three fields), this pre-existing gap doesn't apply to the new field
  either.
- **Conclusion**: no new locking/repository method is needed for AC6; adding
  `ExternalURL *string` to `BacklogItemUpdate` and setting it unconditionally
  in `SyncOne`'s existing-item branch (subject to the `anyField` fix in §3)
  is safe under the current concurrency model.

## Summary of concrete recommendations for the planning phase

1. Schema field: `field.String("external_url").Optional()` — no `.Nillable()` —
   matching `external_id` exactly.
2. Regenerate with the exact command in `session/ent/generate.go`
   (`--feature sql/upsert`, `-mod=mod`); verify via `go build ./session/...`
   and grep for any new/changed `session/ent/*` files to confirm the full
   generated set moved together, not just `migrate/schema.go`.
3. `BacklogItemUpdate.ExternalURL *string` bypassing `UserModifiedFields`,
   and fix `SyncOne`'s `anyField` gate so the backfill fires even when
   title/description/priority are all user-modified (currently would skip
   the `UpdateBacklogItem` call entirely in that case — see §3).
4. New prompt section added inside `BuildSessionInitialPrompt` (not
   duplicated in the two truncation passes) — keep it to two short lines to
   minimize the fixed token-budget overhead added to every render (§2).
5. Truncate the new field with the same raw `[:500]` byte-slice pattern used
   for Title/Description, accepting the (practically negligible, given
   GitHub's ASCII `html_url` contract) theoretical UTF-8-corruption risk
   rather than introducing new rune-safety helpers not required by any AC.
6. Explicit test for AC6's negative case (item not in `Fetch` results stays
   unbackfilled) as well as the positive backfill case and the
   all-fields-user-modified-but-URL-still-backfills case (§3).
