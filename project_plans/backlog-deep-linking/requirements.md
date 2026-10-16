# Requirements: backlog-deep-linking

**Date**: 2026-08-19
**Type**: feature addition (cross-cutting: ID format, storage, web routing, OS integration)
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement
stapler-squad backlog items currently have no shareable, addressable link. The only way to point someone at a specific item is `/backlog?item=<uuid>` on whichever host happens to be running that instance's web UI — it only works if the recipient already has that exact host/port open, carries no information about what kind of entity the ID refers to, and breaks silently if the item lives on a different instance (stapler-squad manages sessions/backlog across multiple machines — see `state-isolation.md` and workspace peers). Users (Tyler, and any collaborators sharing a workspace) have no way to paste a link into Slack, a commit message, or another tool and have it resolve to the right item on the right host.

## Baseline
Today: copy the query-string URL from the address bar (`web-app/src/app/backlog/page.tsx:230`, `/backlog?item=<uuid>`) and hope the recipient is looking at the same running instance. IDs are opaque UUIDv4 strings (`session/ent/schema/backlog_item.go:22-23`, `field.UUID("id", uuid.UUID{}).Default(uuid.New)`) with no type or instance information encoded. Links to items on a different host/instance have no resolution path at all today.

## Users / Consumers
- Tyler and any collaborators using the same stapler-squad workspace across multiple machines/instances (workspace peers).
- External tools/surfaces a link might be pasted into: Slack, terminal, commit messages, other docs.
- Internal consumers: the Next.js web app (`web-app/src/app/backlog/page.tsx`), the backlog ent schema/service layer, and (new) an OS-level URL scheme handler.

## Success Metrics
- A user can generate a copyable link for any backlog item from the web UI, and opening that link on the originating host navigates directly to that item (replaces manually copying `?item=<uuid>` from the address bar).
- A link pointing at an item on a *different* host, when opened, either navigates there automatically via workspace peers (preferred) or — if that peer isn't currently reachable/registered — shows a clear "this item lives on host X" message naming the host, never a silent 404 or wrong item.
- A pasted `ssq://` link is clickable from outside the browser (Slack, terminal, Notes) on Linux dev machines and opens/focuses the web app to the right item. macOS OS-level scheme registration is explicitly deferred past v1 (see ADR-003) — on macOS, a pasted `ssq://` link is not yet clickable from outside the browser; the web app's in-app resolution and `https://`-equivalent route still work there.
- New backlog items get an ID that is self-describing at a glance (type-prefixed, sortable) without requiring a lookup to know "is this a backlog item or a session."

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints
- No hard external deadline.
- Must not require sudo/root or interactive credential prompts to set up scheme registration locally (consistent with existing `make setup-codesign` / TCC patterns for macOS).
- Existing backlog item IDs (UUIDv4, ent-managed primary key) must keep working indefinitely — no forced migration.

## Non-functional Requirements
- **Performance SLO**: not specified — link generation/resolution is not a hot path.
- **Scalability**: not applicable — ID generation volume is bounded by backlog item creation rate.
- **Security classification**: internal. Links may traverse Slack/other tools outside the local machine, so hostnames embedded in links should not leak anything more sensitive than what's already visible in the running UI (no tokens/secrets in the URL).
- **Data residency**: not applicable — single-user/small-team local/dev-machine deployments only.

## Scope

### In Scope
- New ID scheme for **newly created** backlog items: type-prefixed ULID (e.g. `bl_01J...`) — sortable/monotonic, encodes creation time, prefix identifies entity type. Apply the same scheme to session IDs if it can be done without disrupting existing session ID consumers (stretch within this project; backlog items are the priority and must ship regardless).
- Deep link URL format: `ssq://<hostname>/<type>/<version>/<id>` (e.g. `ssq://myhost/backlog/v1/bl_01J...`). `version` is a separate segment for future format changes, not baked into the ID.
- "Copy link" affordance in the web UI for a backlog item that generates this URL.
- In-app resolution: opening/pasting an `ssq://...` URL (or its `https://`-equivalent web route) while the web app is running navigates to the correct backlog item if it's on the current host.
- Cross-host resolution: if the link's hostname doesn't match the current instance, attempt a handoff via the existing workspace-peers mechanism (`mcp__stapler-squad__list_workspace_peers` and whatever peer-registry backs it) to redirect the user to the correct instance. If the target peer is not currently registered/reachable, fall back to a clear "this item lives on host X, which isn't reachable right now" message — never fail silently or resolve to the wrong item.
- OS-level `ssq://` scheme registration on Linux (`.desktop` file with a `MimeType=x-scheme-handler/ssq;` entry) so links are clickable from outside the browser. macOS registration (via the existing app bundle path — see `.claude/docs/codesigning.md`) is deferred past v1 — see Out of Scope and ADR-003.
- Backward compatibility: existing UUIDv4 IDs and the existing `/backlog?item=<uuid>` query-param route both continue to work unchanged.

### Out of Scope
- Migrating/regenerating IDs for existing backlog items — old items keep their current UUIDv4 IDs permanently (explicit decision; simpler, zero migration risk, ID formats will be inconsistent across old/new items going forward — acceptable).
- Mobile support (iOS/Android URL scheme registration).
- Any web-app change beyond backlog items (e.g. deep-linking sessions or other entity types) — covered only if the ID-format stretch goal above is picked up; not a hard requirement.
- Guaranteed cross-host handoff in every topology (e.g. peer behind NAT with no tunnel/relay configured) — the fallback error message is an acceptable v1 terminus for unreachable peers.
- macOS OS-level `ssq://` scheme registration — deferred past v1 per ADR-003 (packaging/bundle constraints; confirmed with the user 2026-08-19). In-app resolution and the `https://`-equivalent route still work on macOS; only the "click a link outside the browser" path is unavailable there until a follow-up.

## Rabbit Holes
- **OS-level scheme registration**: macOS custom URL scheme handlers (`CFBundleURLTypes` in `Info.plist`) typically require the app to run from a proper `.app` bundle, not a bare Go binary launched via systemd/launchd — needs to be reconciled with how `make install-service` currently packages/runs the binary (see `.claude/docs/codesigning.md`, `.claude/docs/bundling-tmux.md`). This could turn into its own packaging project if the current deployment model doesn't support bundle-based launch. Flag for Phase 3 planning to scope tightly or explicitly punt registration to a follow-up if packaging work balloons.
- **Cross-host handoff via workspace peers**: depends on what `list_workspace_peers` / the underlying peer registry actually tracks today (liveness? reachable URL? auth?) — needs research in Phase 2 before assuming a redirect is straightforward. If peers are only known by name with no live reachability signal, "handoff" may reduce to "open this URL: <peer-url>" rather than an automatic redirect.
- **ULID generation for sortability**: ent's `field.UUID(...).Default(uuid.New)` pattern assumes a `uuid.UUID` Go type; swapping to a ULID-as-string (or a custom type) touches the ent schema, generated code, and every place that parses/compares/routes on backlog item ID as a `uuid.UUID`. Needs a compat shim (e.g. store as string, validate format at the boundary) rather than a wholesale type change.
- **Version segment semantics**: what actually changes between `v1` and a hypothetical `v2` needs a concrete definition (schema of the resolved payload? URL structure itself?) or it's a placeholder that never gets used — Phase 3 should pin this down concretely, not leave it vague.

## Alternatives Considered
- Keep opaque UUIDs and only add URL/host context in the link (no ID format change) — rejected per user preference for a self-describing ID; also considered and explicitly rejected in favor of type-prefixed ULIDs during requirements clarification.
- Full backfill-migration of all existing IDs to the new format — rejected as unnecessary risk to existing stored references (sessions.json/config.json, any external links) for no user-visible benefit beyond cosmetic consistency.

## Feasibility Risks
- Packaging/bundling constraints may block or significantly shrink OS-level scheme registration scope (see Rabbit Holes).
- Workspace-peers may not currently expose enough information (live reachability, resolvable URL) to support an automatic handoff redirect — may need its own small enhancement before deep-link handoff can work as envisioned.
- ent schema changes to ID generation carry standard ent-codegen risk (`session/ent/generate.go` — must use `--feature sql/upsert`, generated output is gitignored and regenerated via `make ent-gen`).

## Observability Requirements
Log link generation and resolution failures (e.g. "peer unreachable," "malformed ssq:// URL," "unknown item ID") at a level visible in `~/.stapler-squad/logs/stapler-squad.log`, so failed deep-link opens are debuggable without needing a live repro. No new metrics/alerts — this is a low-volume, non-critical-path feature.

## Risk Control
Ship the ID-format change and in-app link generation/resolution first (self-contained, no OS integration), verify manually, then layer in OS-level scheme registration and cross-host handoff behind that. No feature flag needed — new ID format only affects newly created items, and `ssq://` link resolution is purely additive (old query-param links keep working). Rollback is git revert; no data migration to unwind since old items are untouched.

## Open Questions
- Exact reachability/URL info available from workspace-peers today — resolve in Phase 2 research before Phase 3 commits to an automatic-handoff design.
- Whether `make install-service`'s current systemd/launchd-based launch is compatible with macOS's `.app`-bundle requirement for `CFBundleURLTypes`, or whether scheme registration needs a thin wrapper bundle — resolve in Phase 2 research.
- Concrete definition of what the `version` URL segment actually versions (resolve in Phase 3 plan).
- Whether the session-ID stretch goal (apply the same type-prefixed-ULID scheme to sessions) is worth doing in this pass or should be split into its own follow-up — decide in Phase 3 based on how much the backlog-item work already touches shared ID-handling code.
