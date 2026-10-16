# Architecture Research: backlog-deep-linking

Agent 3 (Architecture), SDD Phase 2 research. Companion to `requirements.md`.

## 1. ID generation: where the change belongs

**Current shape** (`session/ent/schema/backlog_item.go:22-23`):
```go
field.UUID("id", uuid.UUID{}).Default(uuid.New)
```
This is a hand-written schema file (tracked in git). Everything downstream of it —
`session/ent/backlogitem.go`, `backlogitem_create.go`, `backlogitem_query.go`,
`backlogitem/where.go`, `mutation.go`, `client.go` — is **generated** by
`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema`
(`session/ent/generate.go:3`) and gitignored per repo convention (`session/ent/*.go` and
`session/ent/*/`, excluding `schema/` and `generate.go`). `make ent-gen` is a dependency of
`build`/`test`/`lint`, regenerated from a stamp file.

**Every current consumer already treats the ID as an opaque string at the API boundary**,
which is the key finding that shapes the recommendation:

- `gen/proto/go/session/v1/backlog.pb.go` — `ItemId string` (confirmed: proto fields for
  backlog item IDs are `string`, e.g. `ExistingItemId string` at
  `gen/proto/go/session/v1/backlog.pb.go:2090`). No proto change needed for ID format.
- `server/services/backlog_service_triage.go:613,2501,2978` — receives `req.Msg.ItemId`
  as a plain string, only wraps it in an error message on not-found. No `uuid.UUID` typed
  parameter here.
- `session/storage_backlog.go` and `session/ent_repository_backlog.go` are the **only**
  two files that call `uuid.Parse(id)` on a backlog item ID — 20+ call sites combined
  (`storage_backlog.go:83,125,145,218,...`; `ent_repository_backlog.go:341,357,397,...`).
  This is the actual seam: every one of these does `parsedID, err := uuid.Parse(id)` then
  passes `parsedID` into an ent `.Where(backlogitem.ID(parsedID))`/`.SetID(parsedID)` call.
  This pattern is what would need to branch on ID shape (UUID vs ULID) or move to a
  string-keyed ent field.
- `server/mcp/tools_goal.go`, `server/services/feature_flag_service.go`,
  `session/workspace_peers.go`, `session/chain_firer.go`,
  `server/services/trigger_rate_limiter.go`, `server/services/webhook_trigger_common.go`
  use `uuid.UUID` for **other** entities (session UUIDs, trigger fire events, session
  goals) — unrelated to backlog item IDs and out of scope for this change.

**Recommendation — string-typed ent field + a validating newtype at the boundary, not a
wholesale ent type change:**

1. Change the ent field to `field.String("id")` (no `.Default(...)` — ID is set explicitly
   at creation time by application code, not ent). This is the schema-layer edit the
   requirements' Rabbit Holes section already anticipates ("store as string, validate
   format at the boundary"). It sidesteps ent's `field.UUID` type requiring a
   `uuid.UUID`-shaped Go value — ent's `field.String` primary key support is standard and
   well-trodden (used elsewhere in this schema, e.g. `status`, `category`).
2. Per `.claude/rules/primitive-obsession-checklist.md`, do **not** let the ID become a
   bare `string` at every call site — that reopens exactly the smell the checklist exists
   to catch (a `string` for backlog-item-ID is trivially confusable with a `string` for
   session-ID, workspace-key, branch name, etc., several of which already flow through
   the same functions in `session/storage_backlog.go`). Introduce a small newtype, e.g.
   `session.BacklogItemID` (unexported string field, smart constructor `NewBacklogItemID`
   that accepts either a legacy UUIDv4 string or a `bl_<ULID>` string and validates the
   shape), following the `RepoRef`/`AccountRef` precedent in `github/repo_ref.go` and
   `github/keychain.go`. `String()` returns the raw value for embedding in ent queries and
   proto messages (which stay plain `string` — proto/wire format is not the layer that
   needs the newtype).
3. ID generation becomes a two-way branch at the single construction point (wherever
   `uuid.New()` is currently called for new backlog items — search
   `session/storage_backlog.go`/`ent_repository_backlog.go` for the create path): new items
   call `NewULIDBacklogItemID()` → `"bl_" + ulid.Make().String()`; existing stored rows
   parse back into the newtype without re-validating shape beyond "non-empty, matches one
   of the two known formats." No ent-level `Default()` fires ULID generation implicitly —
   generation is explicit application logic, consistent with wanting a `bl_` prefix ent
   itself has no concept of.
4. Every `uuid.Parse(id)` call site in `storage_backlog.go`/`ent_repository_backlog.go`
   becomes either deleted (no more UUID round-trip needed once the ent field is a plain
   string) or replaced with `session.NewBacklogItemID(id)`'s validation — this is the bulk
   of the mechanical migration work, confined to exactly two files.

**Why not keep `field.UUID` and just prefix-encode**: a ULID is not a valid UUID bit
layout close enough to reuse `uuid.UUID`'s 16-byte representation without lossy
reinterpretation, and the `bl_` prefix is not valid UUID syntax at all — `uuid.Parse would
reject every new-format ID outright. Keeping the ent field as `uuid.UUID` and merely
generating ULID-shaped *strings* into it is not possible without dropping the type back to
string somewhere, so there is no version of this that avoids the schema-field-type change.

## 2. Workspace-peers architecture

Implementation: `session/workspace_peers.go` (confirmed via grep — the only three files
touching `WorkspacePeer`/`workspace_peers`/`ListWorkspacePeers` are
`server/mcp/tools_goal.go`, `server/services/session_service.go`, and this file, which is
the actual logic).

**What a peer record contains today** (`WorkspacePeer` struct, `session/workspace_peers.go:25-40`):
```go
type WorkspacePeer struct {
    SessionUUID string
    Title       string
    Branch      string
    Path        string
    Status      Status
    Goal        *SessionGoalData
    InstanceLive bool   // derived: Status != Stopped, later overridden by tmux liveness
    StaleGoal    bool   // derived: goal not updated within 30 min
}
```

Critically: **there is no hostname, no URL, no port, and no auth token anywhere in this
struct or its construction path.** `ListWorkspacePeers` (`workspace_peers.go:58`) is scoped
to sessions sharing a `workspaceKey` (derived from GitHub owner/repo + main-repo path,
`WorkspaceKey(...)`) **within a single Storage instance** — it queries `s.ListInstanceData()`
and `s.sessionGoalsByUUIDs(...)`, both local to the calling process's own state. There is no
cross-instance registry query here at all; "peers" are other *sessions* (tmux
sessions/worktrees) on the **same** running stapler-squad instance, not other **hosts**
running separate stapler-squad instances. `LiveTmuxSessionUUIDs` (`workspace_peers.go:162`)
confirms this further — it shells out to the local `tmux` binary directly.

**Conclusion for the deep-linking design: cross-host handoff via "the existing
workspace-peers mechanism" as described in the requirements does not exist today.** This
directly confirms and sharpens the Feasibility Risk already flagged in `requirements.md`
("Workspace-peers may not currently expose enough information ... may need its own small
enhancement"). Concretely, what's missing:
- No concept of a remote **host** at all (only same-process session/worktree peers).
- No liveness/reachability signal for another *machine* (only local tmux process
  presence).
- No URL/port a browser could navigate to for a different host.
- No auth — moot today since there's no cross-host communication surface.

This means the requirement "attempt a handoff via the existing workspace-peers mechanism"
cannot ship as scoped without new work: at minimum, a small **host registry** (e.g. a
`WorkspaceHost{Hostname, BaseURL, LastSeenAt}` table/config, populated by instances
announcing themselves — could be as simple as a shared file/DB row per known dev machine,
or genuinely new inter-instance heartbeat traffic) is a prerequisite, not a reuse of
`ListWorkspacePeers`. Given the Appetite (3–6 weeks) and the requirement's own accepted
fallback ("if the target peer is not currently registered/reachable ... show a clear
message"), the pragmatic v1 is: a **static/manually-configured host list** (hostname →
base URL, e.g. in `config.json`) with no liveness probing beyond "was this hostname ever
seen," rather than building a live peer-discovery protocol. That satisfies the fallback
branch cheaply and defers "automatic redirect to a genuinely live peer" to a follow-up,
consistent with the Rabbit Holes note about scoping this tightly in Phase 3.

## 3. `ssq://` URL resolution flow

**Routing should live in the Go server, not purely client-side**, for one non-negotiable
reason: the OS scheme handler invokes a *process*, and the only process that can decide
"is the target item on this host" is the one holding `session/ent` state — Next.js is
served out of that same Go binary's `net/http` mux (`server/server.go`, `srv.mux.Handle(...)`
patterns throughout — routes like `/api/files/raw`, `/mcp`, WebSocket upgrades are all
registered directly on `s.mux`), so a new route (e.g. `srv.mux.HandleFunc("GET
/api/deep-link/resolve", ...)`) is the natural landing spot for "given a parsed
`ssq://host/type/version/id`, resolve host-match and item existence, then redirect the
browser to `/backlog?item=<id>` (in-app client-side route, existing baseline behavior) or
render the cross-host-unreachable message." This keeps parsing/validation of the URL
format (type-prefix, version segment, ID-shape) server-side where it can be unit-tested
against the same `session.NewBacklogItemID` newtype from Section 1, rather than
duplicating that validation logic in TypeScript.

**Two distinct entry paths, both converging on the same server-side resolver:**

1. **In-app paste-into-browser path** (already partially working per requirements: opening
   `/backlog?item=<uuid>` works today). Extending this to also accept a pasted
   `ssq://...` string (e.g. a text field, or recognizing it if pasted into the
   location bar as `https://<host>:8543/deep-link?url=ssq://...`) is pure web-app routing
   — no OS integration needed. This is the low-risk slice the Risk Control section wants
   shipped first.

2. **OS-registered scheme path** (macOS/Linux invoke a binary with the URL as `argv[1]`
   when a user clicks an `ssq://` link anywhere outside the browser). This is the harder
   path and needs a concrete answer to "how does invoking the packaged app forward the URL
   to the *already-running* server on `:8543`":
   - The deployed instance is a long-running systemd/launchd service (`make
     install-service`, per root `CLAUDE.md`), not a per-click-launched process. Cobra's
     `main.go` (`rootCmd`, `resetCmd`, `debugCmd`, etc.) is the CLI entry point but there is
     **no existing single-instance detection, lock file, or PID-based "forward to running
     instance" mechanism** in this codebase (grep for `SingleInstance`, `lock file`,
     `pidfile`/`PIDFile` returned only unrelated matches like `config/singleton.go` [a
     config singleton, not a process singleton] and native-process/VNC display allocation
     code — none of it is a cross-invocation IPC mechanism).
   - The **only** existing IPC-like primitive that fits this shape is the `FocusWindow`
     RPC (`server/services/utility_service.go:106`, exposed over ConnectRPC at
     `/session.v1.SessionService/FocusWindow`, already localhost-origin-gated at
     `utility_service.go:264`). It activates a native app window by bundle ID/PID — a
     precedent for "one process asks the already-running server to do something UI-ish,"
     but it does not currently know how to open a browser tab at a URL.
   - Recommended shape: a new `stapler-squad --open-url ssq://...` cobra subcommand
     (mirroring existing subcommands like `testPtyCmd`, `listSessionsCmd`) that does **not**
     spawn a second server. Instead it does the minimal thing: parse the `ssq://` URL,
     compute the equivalent `http://localhost:8543/...` (or the peer's URL if cross-host,
     per Section 2's static host list), and shell out to the OS "open a URL in the default
     browser" call (`open <url>` on macOS, `xdg-open <url>` on Linux) — no need to reach
     into the running server process's state at all, since resolution (Section 3, item 1's
     server route) happens once the browser actually requests that URL. This avoids needing
     any new inter-process signaling entirely; the CLI subcommand's only job is "turn a
     custom-scheme argv into a clickable http(s) URL and hand it to the OS's browser
     launcher."
   - **Packaging risk carries forward from Requirements' Rabbit Holes, confirmed by
     reading `.claude/docs/codesigning.md`**: the current signed binary is deliberately
     **not** a `.app` bundle — `Info.plist` is embedded into the binary's own
     `__TEXT/__info_plist` Mach-O section (not a sibling `Contents/Info.plist`) specifically
     *because* `codesign` treats a same-directory `Info.plist` as bundle-layout evidence and
     seals the entire (80k+-file) directory as bundle resources, breaking `codesign --verify`
     on any repo change (documented rationale, `.claude/docs/codesigning.md`, "Why
     `Info.plist` lives in `macos/` and not the repo root"). `CFBundleURLTypes` support in
     `Info.plist` is normally resolved through Launch Services scanning `.app` bundles, but
     a bare signed Mach-O with an embedded plist **can** still be registered via
     `lsregister -f <path>` (a documented, if less common, technique) — this needs a Phase 2
     spike to confirm it actually works with this repo's non-bundle build, rather than
     assuming a full bundle rewrite is required. Either way, this is genuinely separable
     from the ID-format and in-app-resolution work and should ship as a distinct, later
     slice (matches the Risk Control section's sequencing).

## 4. Data flow / consistency during the UUID→ULID transition

**No collision risk, and no race, under the recommended design (Section 1).** Reasoning:

- ULIDs (`bl_<26-char Crockford-base32>`) and UUIDv4 strings (`8-4-4-4-12` hex with
  dashes) are lexically disjoint formats — a `bl_`-prefixed string can never collide with
  or be misparsed as a canonical UUIDv4 string, and vice versa. Since the ent primary-key
  field becomes a plain `field.String("id")` with **no shared numeric/byte keyspace**
  (unlike, say, sequential integer IDs where a UUID-to-int migration could genuinely
  collide), there is no scenario where a new ULID-ID item's row could overwrite or be
  confused with an old UUID-ID item's row at the storage layer.
- The two ID generators (`uuid.New()` for old code paths, if any remain, vs. a new
  `ulid.Make()` call for new items) never run concurrently against the *same* row — ID
  generation happens once, synchronously, at item-creation time, before any row exists.
  There's no read-modify-write window where a stale UUID default could race a new ULID
  default, because `Default(uuid.New)` is being **removed** from the ent schema entirely in
  this design (Section 1, point 1) — generation moves to explicit application code with a
  single call site, eliminating the two-generator-races-on-one-field shape altogether.
- The only real transition-period risk is **application-code paths that assume ID shape**
  (e.g. code that does `uuid.MustParse(id)` defensively, or code that infers "is this a
  backlog item" from successfully parsing the string as a UUID) — this is exactly why
  Section 1 introduces `NewBacklogItemID` as the single validation chokepoint: every
  consumer should go through it rather than re-implementing shape detection ad hoc,
  otherwise a future find of "this helper still does `uuid.Parse` on a backlog item ID and
  silently errors on `bl_...` items" is the realistic failure mode, not a runtime
  collision.
- One thing to verify explicitly in Phase 3/implementation: the `sql/upsert` ent feature
  flag (`session/ent/generate.go:3`) generates `ON CONFLICT` upsert code keyed on the
  primary key column — confirm the generated upsert path doesn't assume a UUID column type
  (postgres/sqlite `uuid` column type vs `text`) at the SQL-dialect level; this is a
  migration-file concern (existing rows keep their UUID *values* stored in what becomes a
  `text`/`varchar` column, which is a safe, non-lossy column-type widening) rather than a
  data-race concern.

## 5. EventStorming: Event–Command–Policy table

Actors: **User** (clicks/pastes a link), **Web UI** (Next.js client), **Origin Host**
(the stapler-squad instance the link points at), **Target Host** (a *different* instance,
reached only if cross-host), **OS Scheme Handler** (macOS/Linux invoking the CLI on click),
**Workspace Host Registry** (the new static/config-based list from Section 2 — not the
existing session-scoped `WorkspacePeer`).

| Command | Actor | Event | Policy (triggers next command) |
|---|---|---|---|
| `GenerateDeepLink` | User clicks "Copy link" in web UI | `DeepLinkGenerated` (hostname, type, version, item ID) | Web UI copies `ssq://<hostname>/backlog/v1/<id>` to clipboard. No cross-host check needed at generation time — the link always encodes the *generating* host. |
| `OpenDeepLinkURL` | User clicks link in Slack/terminal/Notes | `SchemeURLInvoked` (raw `ssq://` string, argv) | **Policy**: OS Scheme Handler invokes `stapler-squad --open-url <url>` → triggers `TranslateSchemeURLToHTTP`. |
| `TranslateSchemeURLToHTTP` | CLI subcommand | `HTTPURLResolved` (or `MalformedSchemeURLRejected`) | **Policy**: on success, triggers `LaunchBrowserAtURL` (shell `open`/`xdg-open`); on malformed URL, log at `~/.stapler-squad/logs/stapler-squad.log` per Observability Requirements and abort — no browser launch. |
| `PasteDeepLinkIntoWebApp` | User pastes `ssq://...` directly into a web-app input while already running the app | `DeepLinkPasted` | **Policy**: Web UI forwards to the same server-side resolver route as the OS path (Section 3) rather than re-implementing parsing client-side. |
| `ResolveDeepLink` | Web UI (browser navigation) hits server route with parsed `{hostname, type, version, id}` | `HostMatchEvaluated` — either `LocalHostMatched` or `RemoteHostMismatchDetected` | **Policy A** (local match): triggers `LookupBacklogItem`. **Policy B** (mismatch): triggers `LookupWorkspaceHost` against the Workspace Host Registry. |
| `LookupBacklogItem` | Origin Host server | `BacklogItemFound` or `BacklogItemNotFound` | **Policy**: found → triggers `NavigateToItem` (redirect to `/backlog?item=<id>`, the existing baseline route); not found → triggers `ShowResolutionError` ("unknown item ID", logged per Observability Requirements). |
| `LookupWorkspaceHost` | Origin Host server | `TargetHostKnownAndReachable` or `TargetHostUnknownOrUnreachable` | **Policy A**: known+reachable → triggers `RedirectToTargetHost` (HTTP redirect to Target Host's resolver URL for the same item/type/version). **Policy B**: unknown/unreachable → triggers `ShowResolutionError` ("this item lives on host X, which isn't reachable right now" — exact wording from requirements' Success Metrics). |
| `RedirectToTargetHost` | Origin Host server | `HandoffAttempted` | **Policy**: Target Host receives the redirected request and independently runs `ResolveDeepLink` → `LookupBacklogItem` against its *own* state — Origin Host does not proxy the item data itself, only redirects, since it has no visibility into Target Host's DB. |
| `NavigateToItem` | Web UI | `ItemDisplayed` | Terminal success state — matches Success Metrics' "opening that link ... navigates directly to that item." |
| `ShowResolutionError` | Web UI | `ErrorShown` | Terminal failure state — matches Success Metrics' "never a silent 404 or wrong item." Both `BacklogItemNotFound` and `TargetHostUnknownOrUnreachable` converge here with different messages. |

**Bounded-context boundary this surfaces**: `LookupWorkspaceHost` is a genuinely new
bounded context (Workspace Host Registry) distinct from the existing session-scoped
`WorkspacePeer` context in `session/workspace_peers.go` — the EventStorming table makes
explicit that "peer" (another session/worktree on *this* host) and "host" (another
*machine* running its own stapler-squad instance) are different concepts that happen to
share a name in the requirements doc's prose ("workspace peers mechanism"), which is worth
flagging in Phase 3 planning so the plan doesn't silently conflate them.

## 6. Failure mode: stale gitignored `session/ent/*.go` vs. changed ID field type

**Root cause of the specific risk asked about**: `session/ent/*.go` is gitignored and
regenerated via a **stamp file** dependency in the Makefile (per root `CLAUDE.md`: "Every
Make target that needs it (`build`, `test`, `lint`) already depends on `ent-gen`, which
regenerates from a stamp file"). A stamp-file-based regeneration trigger typically keys off
mtime or a content hash of the `schema/` directory — if that staleness check is
mtime-based and something touches `session/ent/schema/backlog_item.go`'s mtime without
changing content (e.g. a `git stash`/branch-switch that doesn't actually alter the file, or
a checkout that preserves mtimes), or if the check is coarser than per-file, a developer
could end up with:
- **Compile-time safety net (most likely outcome, not a silent flake)**: if the schema
  field type actually changed (`field.UUID` → `field.String`) but stale generated code
  still declares `ID uuid.UUID` on the `BacklogItem` struct, hand-written call sites
  updated to pass a `string`/`BacklogItemID` into what the stale generated code expects as
  `uuid.UUID` **will fail to compile** — Go's static typing turns this into a build error,
  not a runtime flake. This is the good case and the most probable one.
- **Silent-success risk (the actually dangerous case)**: if the stamp-file check is skipped
  entirely (e.g. `go test ./session/...` run directly without going through `make build`
  first, bypassing the `ent-gen` Make dependency — this repo's own `CLAUDE.md` documents
  `go test ./server/services` as needing `make build` first for exactly this reason) and
  the stale generated code still compiles against the *old* schema shape because no other
  file was touched, tests could pass against **old-format (UUID-only) behavior** while the
  developer believes they're testing the new ULID path — a false-green result. This mirrors
  this repo's own documented ent risk in `requirements.md`'s Feasibility Risks
  ("`session/ent/generate.go` — must use `--feature sql/upsert`, generated output is
  gitignored and regenerated via `make ent-gen`") and the root `CLAUDE.md`'s explicit
  warning against force-adding generated ent code, which previously caused "missing/
  incomplete package left main broken until someone ran `make ent-gen` and noticed."
- **Mitigation for this project specifically**: after landing the schema field-type change,
  explicitly run `rm -rf session/ent/*.go session/ent/*/ ` (excluding `schema/` and
  `generate.go`) followed by a clean `make ent-gen && go build ./...` at least once in CI
  and locally before trusting any `go test` result — don't rely on the stamp file's
  staleness detection alone for a **field-type** change (as opposed to an additive field),
  since a type change is exactly the kind of edit most likely to leave behind
  stale-but-still-compiling generated code if the stamp check is coarse-grained. This is a
  process note for Phase 5 implementation, not an architecture change.

## Summary of Recommendations

1. **ID type**: `field.String("id")` on the ent schema (no `Default`), generation moved to
   explicit application code producing either legacy UUIDv4 or `bl_<ULID>`, wrapped in a
   new `session.BacklogItemID` newtype (smart constructor validating either shape) per the
   primitive-obsession rule. Migration is confined to `session/storage_backlog.go` and
   `session/ent_repository_backlog.go`'s ~40 `uuid.Parse`/`uuid.UUID` call sites; proto and
   service-layer code already treats IDs as opaque strings and needs no change.
2. **Workspace-peers reuse is not viable as scoped** — `session/workspace_peers.go`'s
   `WorkspacePeer` is same-instance session/worktree peering with no hostname/URL/liveness
   for other *machines*. Cross-host handoff needs a new, deliberately minimal Workspace
   Host Registry (static config, no live probing) rather than extending `ListWorkspacePeers`.
3. **URL resolution belongs server-side** (a new Go HTTP route on the existing `srv.mux`),
   reusing the same `BacklogItemID` validation as Section 1; the OS-scheme path is a thin
   CLI subcommand (`--open-url`) that only translates `ssq://` → `http://localhost:8543/...`
   and shells out to `open`/`xdg-open` — no new inter-process signaling to the running
   server is needed. `.app`-bundle-vs-bare-binary `CFBundleURLTypes` registration is the
   one open packaging risk and should stay a separately-scoped, later slice per the
   requirements' own Risk Control sequencing.
4. **No ID-collision/race risk** given the lexically-disjoint-format, single-generation-
   point design; the real transition risk is ad hoc shape-sniffing code bypassing the
   `BacklogItemID` chokepoint, not a storage-layer race.
5. **EventStorming surfaces one real bounded-context split** worth naming explicitly in the
   Phase 3 plan: "peer" (session-scoped, existing) vs. "host" (machine-scoped, new).
6. **ent codegen staleness** is usually caught by the Go compiler for a field-type change,
   but a bypassed `make ent-gen` (e.g. running `go test` directly) can produce false-green
   results against stale generated code — mitigate with an explicit clean-regenerate step
   in Phase 5, not by trusting the stamp file for this particular kind of change.
