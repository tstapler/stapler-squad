# Stack Research: backlog-deep-linking

Agent 1 (Stack) — SDD Phase 2 research. Covers library/framework choices, versions, and
integration patterns for: type-prefixed sortable IDs, ent schema integration, `ssq://` URL
parsing, Next.js deep-link routing, and OS-level scheme registration.

## 1. Go ULID / type-prefixed ID libraries

Requirements (`project_plans/backlog-deep-linking/requirements.md:42`) specify the exact target
shape: `bl_01J...` — a short type prefix + underscore + a Crockford-base32 ULID starting with a
timestamp component (`01J...` is the classic oklog-style ULID encoding of a 2024+ timestamp).

| Library | Format | Sortable | Maintenance | Fit |
|---|---|---|---|---|
| [`oklog/ulid/v2`](https://github.com/oklog/ulid) | 128-bit: 48-bit ms timestamp + 80-bit randomness, Crockford Base32, 26 chars | Yes (lexicographic) | Actively maintained, `v2` is current, has a `sync.Pool`-backed monotonic `ulid.Monotonic` source | **Best match** — this is literally what produces `01J...`-style strings; requirements' own example is oklog's canonical format |
| `segmentio/ksuid` | 160-bit, Base62, 27 chars, second-precision | Yes | Stable but lower activity than oklog/ulid | Larger ID space, but second-precision (not ms) and doesn't match the `01J` example format |
| [`go.jetify.com/typeid` (formerly `jetify-com/typeid-go`)](https://github.com/jetify-com/typeid-go) | `prefix_<base32-lowercase-UUIDv7>`, 26-char suffix | Yes (UUIDv7-based) | Actively maintained (updated 2026), purpose-built for "Stripe-style" type-prefixed IDs, uses Go generics for compile-time type safety between ID kinds | Closest **conceptual** match (type-prefix + sortable) but produces a different string shape (lowercase, UUIDv7-derived) than the `01J...` example in requirements |

**Recommendation**: `github.com/oklog/ulid/v2` + a thin hand-rolled prefix wrapper (`"bl_" + ulid.Make().String()`), not the TypeID library. Reasons:
- Matches the exact string format already specified in requirements (`bl_01J...`).
- No new ID-parsing convention to learn — TypeID's `typeid.New[T]()` generic-per-kind API is nice for compile-time type safety, but it's a heavier adoption (a `Subject`/`TypeID` interface indirection) for a feature whose rabbit-holes section already flags "needs a compat shim, not a wholesale type change" — the plain ULID + prefix approach is the minimal-surface-area option.
- `ulid.Monotonic(rand.Reader, 0)` gives monotonic ordering for IDs generated in the same millisecond (relevant since backlog items can be created in rapid succession, e.g. batch import).
- Zero existing dependency on ULID/KSUID/TypeID in this repo (`go.sum` has no `ulid|typeid|ksuid` entries) — this is a new dependency either way, so pick the one matching the spec'd format.

**API shape** (`oklog/ulid/v2`):
```go
import (
    "crypto/rand"
    "sync"
    "github.com/oklog/ulid/v2"
)

var entropyPool = sync.Pool{New: func() any { return ulid.Monotonic(rand.Reader, 0) }}

func NewBacklogItemID() string {
    entropy := entropyPool.Get().(io.Reader)
    defer entropyPool.Put(entropy)
    return "bl_" + ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
```
Parsing back: strip the `bl_` prefix, `ulid.ParseStrict(rest)` to validate format at the boundary (per the requirements' "validate format at the boundary" compat-shim note).

## 2. Integrating into the ent schema

Current schema (`session/ent/schema/backlog_item.go:22-23`):
```go
field.UUID("id", uuid.UUID{}).Default(uuid.New)
```

**Constraint from requirements**: existing UUIDv4 IDs must keep working indefinitely, no forced
migration (`requirements.md:31,51`) — so this cannot be a wholesale swap of the `id` field's Go/DB
type. ent's primary key is a single typed column; there is no per-row "this row's ID is a UUID,
that row's is a string" support without changing the column type for the whole table, which would
break every already-stored UUID value's on-disk representation.

**ent does support non-UUID string primary keys directly** — confirmed both in [ent's official
docs](https://entgo.io/docs/schema-fields/) and by precedent already in this repo:
`session/ent/schema/analytics_event.go:17` (`field.String("id").Unique().NotEmpty().Immutable()`)
and `session/ent/schema/shell.go:23` (same pattern, app-code-assigned). Those existing schemas
prove the migration mechanics work in this codebase (ent-gen produces a `string`-typed ID in the
generated client, `WithID(...)` builder methods accept a string) — but none of them use
`DefaultFunc` for ID generation; IDs are assigned by application code before `Create()`.

**Recommended path — do NOT change `backlog_item.go`'s `id` field.** Add a second field instead:
```go
field.String("public_id").
    Unique().
    Immutable().
    Optional(). // nil/empty for pre-existing rows; only auto-populated in a schema hook or service-layer Create path
    Comment("Type-prefixed ULID (bl_01J...) for new items; existing items pre-dating this feature have no public_id and are addressed by their UUID id."),
```
- Keeps `id` (the ent PK, `uuid.UUID`) untouched — zero risk to every existing FK/edge/lookup that assumes `uuid.UUID`.
- New items get `public_id` populated at creation (service layer, via `NewBacklogItemID()` above, not `DefaultFunc`, since `DefaultFunc` can't return an error and errors are worth surfacing — see the ent FAQ note on schema hooks — though for a pure ULID generator that never errors, `DefaultFunc(func() string { return NewBacklogItemID() })` is also viable and simpler).
- Old items simply have no `public_id`; the deep-link resolver falls back to matching on `id` (UUID) when a link's ID segment parses as a UUID, and on `public_id` when it parses as `<prefix>_<ulid>`.
- Migration: `make ent-gen` regenerates the client from the schema change; the actual SQL migration is auto-generated by ent's versioned-migration tooling (or its atlas-backed auto-migrate if this project uses that — check `session/ent/generate.go` / whatever calls `.Schema.Create(ctx)` at startup) and only needs to `ALTER TABLE backlog_items ADD COLUMN public_id ...` plus a unique index — additive, no backfill needed since it's nullable/optional for old rows.

This directly matches the requirements' own "Rabbit Holes" resolution guidance
(`requirements.md:59`): "Needs a compat shim (e.g. store as string, validate format at the
boundary) rather than a wholesale type change."

## 3. `ssq://` URL parsing in Go

Target format (`requirements.md:43`): `ssq://<hostname>/<type>/<version>/<id>`.

Go's `net/url` (stdlib, already used throughout the repo — see
`server/services/webhook_ssrf.go`, `server/push/subscriber.go`, `github/commits.go`, etc. via
`grep -l "net/url"`) parses arbitrary schemes without any special-casing needed:

```go
u, err := url.Parse("ssq://myhost/backlog/v1/bl_01J...")
// u.Scheme == "ssq"
// u.Host   == "myhost"        (net/url treats the authority-style host correctly for any scheme
//                               with "//" after the colon, per RFC 3986 — not `http`-specific)
// u.Path   == "/backlog/v1/bl_01J..."
segments := strings.Split(strings.Trim(u.Path, "/"), "/")
// segments == ["backlog", "v1", "bl_01J..."]
```//"and query parsing" via `u.Query()` if the version/format ever needs query params.

No third-party library needed — `net/url.Parse` handles custom schemes identically to `http`/`https`
as long as the URL has the `scheme://host/path` shape (RFC 3986 generic syntax); it does not
require the scheme to be a recognized/registered one. Validate `u.Scheme == "ssq"` explicitly at
the parse boundary, and reject empty `u.Host` (a link with no hostname segment is malformed per the
required format) — this is exactly the kind of malformed-URL case the requirements'
Observability section calls out for logging (`requirements.md:72`, "malformed ssq:// URL").

For the `https://`-equivalent web route (requirements.md:45), the parsing is simpler still: it's
just a normal Next.js dynflow route/query param on the existing web server, no custom scheme
parsing needed at all.

## 4. Next.js / React versions and App Router deep-link pattern

Current versions (`web-app/package.json`):
- `next`: **15.3.2**
- `react` / `react-dom`: **^19.0.0**

Both are current-generation (Next 15 is the latest major as of 2026; React 19 is stable). No
version bump needed for this feature — App Router's `useSearchParams`/`useRouter` (already in use,
`web-app/src/app/backlog/page.tsx:8,227,230`) is suffient for both the existing `?item=<uuid>`
query param and a new resolution path for `ssq://`/`/backlog/v1/<id>`-shaped links.

Two concrete integration points, both low-risk given the existing pattern:
1. **New path-segment route** (`web-app/src/app/backlog/[version]/[id]/page.tsx` or similar) if the
   `https://host/backlog/v1/bl_01J...` shape needs its own dedicated Next.js route distinct from
   `?item=`. App Router supports this natively via dynamic segments (`[version]`, `[id]`) — no new
   dependency.
2. **Reuse the existing `?item=` handling**: simplest path — have the new route be a thin
   server/client redirect that rewrites `/backlog/v1/<id>` → `/backlog?item=<id>` (via
   `redirect()` from `next/navigation` or a `next.config.js` rewrite), so `page.tsx`'s existing
   `searchParams.get("item")` resolution logic (`page.tsx:230`) needs no duplication — it already
   works whether `<id>` is a UUID or a new `bl_`-prefixed ULID string, since it's just a string
   comparison against item IDs. This is the lower-effort option and avoids touching the
   already-complex `page.tsx` component twice.

No new frontend dependency is required for either option — `next/navigation`'s `redirect`,
`useRouter`, `useSearchParams` are already imported and used in this exact file.

## 5. OS-level custom URL scheme registration

### macOS: `CFBundleURLTypes`

The mechanism is well-documented (`CFBundleURLTypes` array in `Info.plist`, each entry with
`CFBundleURLSchemes: ["ssq"]`), but **this repo's current `Info.plist` embedding approach cannot
support it as-is**. Per `.claude/docs/codesigning.md` ("How it works" section) and confirmed by
reading `macos/Info.plist`:
- The binary is a **bare Mach-O executable**, not an `.app` bundle. `Info.plist` is embedded into
  the binary's `__TEXT/__info_plist` Mach-O section via `CGO_LDFLAGS="-sectcreate __TEXT __info_plist macos/Info.plist"` at build time — this is a technique for making `codesign`/TCC treat a
  bare binary as if it had bundle identity (for Full Disk Access / Apple Events grants), **not**
  a real bundle registration with Launch Services.
- **`CFBundleURLTypes` / custom URL scheme registration requires Launch Services to know about a
  real `.app` bundle** (`YourApp.app/Contents/Info.plist`, registered via `lsregister` or by the
  Finder indexing `/Applications`). A `__TEXT/__info_plist`-embedded plist in a bare binary is
  invisible to Launch Services for URL-scheme dispatch purposes — that trick only works for the
  TCC/codesign use case documented in `codesigning.md`, not for `open ssq://...` handler
  resolution. This confirms the risk flagged in requirements' Rabbit Holes
  (`requirements.md:57`) and Open Questions (`requirements.md:79`).
- **Implication**: registering `ssq://` on macOS needs a *real* thin `.app` bundle wrapper (e.g.
  `StaplerSquadLink.app/Contents/MacOS/<tiny launcher binary>` that just forwards the URL to the
  already-running background service via a local HTTP/socket call, plus a real
  `Contents/Info.plist` with `CFBundleURLTypes`), registered once via
  `lsregister -f /Applications/StaplerSquadLink.app` or by placing it under `/Applications` (no
  sudo required for either — `lsregister` and drag-to-Applications are both user-level operations,
  satisfying the requirements' "no sudo" constraint at `requirements.md:30`). This is additive
  packaging work, not a change to how `make install-service` runs the main systemd/launchd-managed
  binary — the launcher app bundle's only job is "receive the `ssq://` URL from the OS, then hand
  it to the already-running service" (e.g. HTTP POST to `localhost:8543/internal/deep-link`).

### Linux: `.desktop` file + `MimeType=x-scheme-handler/ssq;`

No `.desktop` file exists in this repo yet (`find ... -iname "*.desktop"` returned nothing) — this
is new. The standard mechanism (freedesktop.org XDG spec, no sudo needed for a per-user install):
```ini
# ~/.local/share/applications/stapler-squad-linkhandler.desktop
[Desktop Entry]
Type=Application
Name=Stapler Squad Link Handler
Exec=/path/to/stapler-squad-linkhandler %u
MimeType=x-scheme-handler/ssq;
NoDisplay=true
```
Registration:
```bash
update-desktop-database ~/.local/share/applications/   # refresh the MIME database
xdg-mime default stapler-squad-linkhandler.desktop x-scheme-handler/ssq
```
Both commands are user-level (`~/.local/share`), no root required — satisfies the same
no-sudo constraint. `%u` in `Exec=` passes the full clicked URL as an argument to the handler
binary, which (same as macOS) just needs to forward it to the running service — this can be a tiny
subcommand of the existing `stapler-squad` binary itself (e.g. `stapler-squad open-link ssq://...`)
rather than a separate binary, since Linux has no bundle-identity requirement blocking it the way
macOS's Launch Services does. This is the **simpler side of the OS-registration work** — worth
sequencing after the macOS bundle question is resolved, per the Risk Control section's phasing
(`requirements.md:75`).

## Summary of new dependencies

| Dependency | Purpose | Where |
|---|---|---|
| `github.com/oklog/ulid/v2` | Type-prefixed sortable ID generation | Go backend (`session/`) |
| *(none)* | `ssq://` URL parsing — stdlib `net/url` suffices | Go backend |
| *(none)* | Deep-link routing — existing Next.js/React versions suffice | `web-app/` |
| *(none, new file)* | Linux `.desktop` entry for scheme registration | new, e.g. `linux/stapler-squad-linkhandler.desktop` |
| *(new, real `.app` bundle)* | macOS scheme registration — cannot reuse the existing bare-binary `__TEXT/__info_plist` embedding trick | new packaging surface, blocked on Phase 3 scoping decision |
