# Pitfalls Research: backlog-deep-linking

Findings for Phase 2 research, covering common failure modes for custom URL schemes,
ent ID-type changes, cross-host handoff, and ULID generation — scoped to what this
project's requirements (`project_plans/backlog-deep-linking/requirements.md`) actually
propose.

## 1. Custom URL scheme registration (`ssq://`)

### Scheme hijacking (security)
Any application on the machine can register itself as the handler for `ssq://` — there is
no OS-level namespace reservation for custom (non-`https`) schemes on macOS or Linux. A
second app (malicious or just a naming collision — `ssq` is short and guessable) can claim
the scheme and start receiving `ssq://` URLs the user pastes/clicks, including the
backlog-item ID and hostname embedded in the link. Requirements already classify link
contents as "internal" and say "no tokens/secrets in the URL" (`requirements.md:36`) —
that decision is the right mitigation and should stay a hard constraint: a hijacked handler
receiving a link can only see a hostname + type-prefixed ID, not a credential.

- **macOS**: last-registered-wins is *not* guaranteed; Launch Services resolution when
  multiple apps claim the same `CFBundleURLTypes` scheme is undefined/version-dependent
  (historically "last one Launch Services indexes," not deterministic). If the user ever
  installs another dev tool that also picks a `ssq://`-shaped scheme, resolution silently
  breaks with no user-visible error. Consider a more specific scheme (e.g.
  `x-stapler-squad://` or a UUID-namespaced scheme) to reduce collision odds, or at minimum
  document that a collision is a known, unresolvable-by-us failure mode.
- **Linux**: `xdg-mime default <desktop-file> x-scheme-handler/ssq` sets the association in
  `~/.config/mimeapps.list`, and whichever `.desktop` file is registered there wins
  deterministically (no ambiguity like macOS) — but only after `update-desktop-database` and
  `xdg-mime`'s cache have actually picked up the new `.desktop` file. See below.

### macOS Gatekeeper / notarization / self-signing interaction
Cross-referenced `.claude/docs/codesigning.md`: the self-signed `StaplerSquadDev` cert
exists purely to keep a stable **Designated Requirement** so TCC (Full Disk Access, Apple
Events) grants survive rebuilds — it says nothing about Gatekeeper or notarization.
`CFBundleURLTypes` scheme registration is a **Launch Services** concern, not a
Gatekeeper/TCC one: LaunchServices reads `Info.plist` from an app bundle at registration
time and does not itself require notarization or a trusted-CA signature to register a
scheme handler — self-signed and even ad-hoc-signed apps can register custom URL schemes
on a Gatekeeper-permissive local dev machine. **The two subsystems are unrelated failure
surfaces**, so getting TCC persistence working (existing `make setup-codesign` flow) does
**not** de-risk scheme registration; they need independent verification. Where the two
*do* interact: launching the handler from a "click" (Finder wrote a `com.apple.quarantine`
xattr because the app arrived via a browser download or similar) can still trigger a
Gatekeeper prompt on first execution — not applicable here since the binary is built
locally, but worth naming as the actual mechanism so it isn't confused with the TCC issue.

**The real blocking issue is the one requirements already flagged in Rabbit Holes
(`requirements.md:57`) and confirmed here**: `CFBundleURLTypes` registration requires a
real `.app` bundle (`Info.plist` at `Contents/Info.plist`, executable at
`Contents/MacOS/<name>`) that Launch Services can discover and index. Today's
`macos/Info.plist` (630 bytes, `CFBundleIdentifier=com.stapler-squad`) is embedded into the
**bare Go binary's** `__TEXT/__info_plist` Mach-O section via linker flags — deliberately
*not* a bundle layout, per the codesigning doc's explanation of why `Info.plist` must not
sit next to the binary (`codesign` would seal the whole directory as bundle resources
otherwise). That trick is exactly what makes `CFBundleURLTypes` a much bigger lift than it
looks: a linker-embedded plist gives TCC an identity to track, but Launch Services'
`lsregister` scheme-to-app mapping is driven by `.app` bundle discovery, not embedded
Mach-O plist sections. Registering `ssq://` on macOS as currently packaged likely needs a
thin `.app` wrapper (e.g. `StaplerSquadHandler.app` containing a small stub executable that
re-execs/forwards to the systemd/launchd-managed binary) — its own packaging subproject,
matching the requirements doc's own flag to scope tightly in Phase 3 or punt to a follow-up.

### Linux `.desktop`/`xdg-mime` gotchas
- `update-desktop-database ~/.local/share/applications` must be run after dropping a new
  `.desktop` file, or GNOME/KDE's cached MIME association won't see it — `xdg-mime default`
  alone updates `mimeapps.list` but some desktop environments cache resolution and need a
  logout/re-login or an explicit `update-desktop-database` to pick up a brand-new
  `x-scheme-handler/ssq` association.
- **Multiple competing handlers**: unlike macOS, Linux desktops keep an explicit
  single-default in `mimeapps.list`, but if the user (or a previous stapler-squad version,
  e.g. a stale binary path after a rebuild) registered a `.desktop` file that no longer
  exists, `xdg-open ssq://...` fails silently or falls back to a browser trying to load it
  as an `https://ssq` domain — a bad failure mode. Registration should be idempotent
  (overwrite, not append) and re-run on every `make install-service`, the same pattern
  already used for the macOS cert (`setup-codesign.sh` is idempotent).
- `Exec=` line in the `.desktop` file must pass the URL as `%u` and the launched binary must
  parse `ssq://...` from `os.Args[1]`, not stdin — a common first-try bug.

### Repo precedent: don't shell out when Go stdlib/wrapper suffices
Following `.claude/rules/prefer-go-git-over-subshells.md`'s "prefer native integration over
subshell" principle: `xdg-mime`/`update-desktop-database`/`macOS lsregister` have no direct
Go stdlib equivalent (these are genuinely OS integration points, not something a Go library
wraps), so shelling out via the existing `executor/safeexec` wrapper (already used
throughout `session/` — e.g. `session/workspace_peers.go`'s `LiveTmuxSessionUUIDs`) is the
right call here, *not* a violation of that rule. The rule's spirit still applies to the
opening-a-link half of this feature: when the web app or handler needs to open a browser
tab pointing at a resolved `https://` URL, prefer Go's `os/exec` wrapped in `safeexec`
calling the platform opener (`open` on macOS, `xdg-open` on Linux) rather than reinventing
browser discovery — there's no stdlib "open URL in default browser," so a safeexec-wrapped
platform command is the idiomatic Go answer, consistent with existing tmux-shelling
patterns in this codebase.

## 2. ent schema ID-type migration

Checked `session/ent/schema/backlog_item.go:22-23`:
```go
field.UUID("id", uuid.UUID{}).
    Default(uuid.New),
```
and the DB driver (`go.mod:60` — `modernc.org/sqlite`, confirming SQLite, not Postgres).

**Key finding: no ALTER is required, and none should be attempted.** ent's `field.UUID`
enforces a single Go type (`uuid.UUID`) for every row in the column regardless of how the
value was generated — the field type is a *compile-time* Go type constraint on the ent
schema, not a per-row runtime tag. A `uuid.UUID` value and a "ULID" are not interchangeable
at the type level: a real ULID (Crockford-base32, 26 chars, ends up as a 128-bit value with
a different bit layout than RFC 4122 UUID) **cannot** be stored in a `field.UUID` column by
constructing a `uuid.UUID` from raw ULID bytes and calling it a day, because the two formats
encode timestamp/randomness in different byte positions — a round-trip through `uuid.UUID`
would corrupt the sortable-prefix property that's the entire point of using a ULID.

This confirms the requirements doc's own Rabbit Hole note (`requirements.md:59`): **the
correct approach is a compat shim, not a type change on the `id` field.** Concretely:
- Keep `id` as `field.UUID(...)` for backward compatibility with existing rows and every
  consumer that already parses/compares/routes on `uuid.UUID` (`item.ID` is used as
  `uuid.UUID` at `server/services/backlog_service.go:655,724` and almost certainly
  elsewhere in the service layer's public API).
- Introduce the type-prefixed ULID as a **separate, additive** field (e.g.
  `field.String("public_id").Optional()`, storing `"bl_01J..."` as a plain string) that is
  populated only on creation for new items and left empty for pre-existing rows. This is
  additive-only (`ALTER TABLE ... ADD COLUMN`, safe on SQLite with a default), matches "old
  items keep their current UUIDv4 IDs permanently" (`requirements.md:51`), and avoids ever
  needing ent to understand two ID *shapes* in one field.
- Deep-link resolution (`ssq://.../<id>`) then needs to accept **either** shape at the
  routing boundary: try parsing as `bl_<ulid>` first (type-prefix present), fall back to
  bare UUID for old links — the "validate format at the boundary" approach the requirements
  doc already anticipates.
- **ULID library**: no ULID or KSUID dependency exists yet in `go.mod`/`go.sum` (checked —
  none found). Whichever library Phase 3 picks (`oklog/ulid` is the common choice) needs an
  explicit new dependency addition, not an assumption that one is already vendored.
- Standard ent-codegen risk applies as already flagged in Feasibility Risks
  (`requirements.md:69`): must regenerate with
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  per `session/ent/generate.go`, generated output stays gitignored, only the
  `session/ent/schema/backlog_item.go` diff gets committed.

## 3. Cross-host / workspace-peer handoff

Read `session/workspace_peers.go` in full (`WorkspacePeer` struct, `ListWorkspacePeers`,
`ApplyTmuxLiveness`) to check what the peer registry actually exposes today, since this was
an explicit Open Question (`requirements.md:78`).

**Confirmed: the peer registry has no host/URL/reachability information suitable for a
redirect today.** `WorkspacePeer` (`session/workspace_peers.go:25-40`) carries
`SessionUUID`, `Title`, `Branch`, `Path`, `Status`, `Goal`, and two liveness-derived
booleans (`InstanceLive`, `StaleGoal`) — **no hostname, no port, no base URL, no auth
token**. `InstanceLive` is computed either from a DB `Status` field or (more
authoritatively) by shelling out to `tmux list-sessions`/`show-environment` on the
**local** tmux server only (`LiveTmuxSessionUUIDs`, lines 162-190) — this only proves a
tmux session is alive on the *current* machine, so it cannot even answer "is the peer host
up" for a genuinely different host, only "is this workspace's session still running here."
This validates the requirements doc's own hedge (`requirements.md:58`): "if peers are only
known by name with no live reachability signal, 'handoff' may reduce to 'open this URL:
<peer-url>' rather than an automatic redirect" — that is exactly the current state, and
Phase 3 planning should budget for **adding** a resolvable base-URL + liveness field to the
peer registry (or a new, separate registry) as its own prerequisite slice, not assume
`ListWorkspacePeers` already has what's needed.

Additional risks once such a registry exists:
- **Staleness**: any peer registry keyed by hostname/port is a cache that goes stale the
  moment a peer's port changes (dev machine restart, port collision fallback) or the
  process dies without updating its own liveness row — exactly the "gone but Status says
  Active" case `ApplyTmuxLiveness`'s doc comment already exists to patch over for local
  peers; a remote-host equivalent (heartbeat, not just last-known-URL) will be needed for
  genuine cross-host UDT resolvability.
- **Spoofing/trust**: a link claims `ssq://otherhost/...` — if resolution auto-navigates to
  whatever URL is registered for `otherhost` without verifying that the *current* machine's
  peer registry entry for `otherhost` was itself sourced from a mechanism the user trusts
  (not just a value another process wrote to shared local state), a malicious or buggy peer
  registration could redirect a legitimate-looking `ssq://` link to an attacker-controlled
  URL. Given the "internal, single-user/small-team, local/dev-machine" security
  classification (`requirements.md:36-37`) this is a low-severity risk in practice, but it's
  worth an explicit design note: never treat the hostname segment of an incoming link as a
  redirect target without cross-checking it against the local peer registry the *current*
  host already trusts — never construct the redirect URL directly from attacker-controlled
  input.
- **Partial-failure UX**: requirements already specify the right behavior — "clear message
  naming host X, never a silent 404 or... spinner forever" (`requirements.md:20`). The
  concrete pitfall to design against is an unbounded wait on an HTTP call to a peer that's
  half-up (process alive, port bound, but hung) — needs an explicit timeout (a few seconds)
  distinct from "peer not in registry at all," since those two failure modes should probably
  produce different messages ("not reachable" vs. "not registered").

## 4. ULID-specific pitfalls

(General knowledge — no ULID generation exists yet in this repo to inspect, confirmed by
the `go.mod`/`go.sum` grep above.)

- **Clock skew across hosts**: ULID's sortability comes from a 48-bit millisecond timestamp
  prefix taken from each generating host's local clock. Two hosts with clocks skewed by
  more than a few seconds (plausible on dev laptops that sleep/wake without NTP resync) will
  produce ULIDs whose lexicographic order doesn't match true creation order across hosts —
  fine for the stated purpose here (self-describing, roughly sortable within a single
  instance's own creation stream) but should not be treated as a cross-host total order
  guarantee. Given backlog items are typically created and viewed on one instance, this is
  a low-impact pitfall to note, not a blocker.
- **Randomness source**: standard ULID libraries (`oklog/ulid`) require the caller to supply
  an `io.Reader` for the 80-bit random component and a monotonic-entropy wrapper
  (`ulid.Monotonic`) to guarantee strictly increasing IDs within the same millisecond on one
  host — using `math/rand` without a crypto or properly-seeded source, or skipping the
  monotonic wrapper, reintroduces collision risk (low but non-zero, and defeats the
  same-millisecond ordering guarantee) that a naive `ulid.New(ulid.Now(), rand.Reader)` call
  without monotonic entropy would have. Phase 3 should specify a monotonic-source ULID
  generator, not a bare one-shot `ulid.New` per call.
- **Encoding confusion**: ULID uses Crockford base32 (case-insensitive, excludes
  `I`/`L`/`O`/`U` to avoid visual ambiguity with `1`/`0`), not base64 or standard base32 —
  a common bug is round-tripping through a base64 encoder/decoder somewhere in the URL or
  storage path (e.g. an over-eager "let's base64 this ID for the URL" shortcut) and getting
  a differently-shaped, non-Crockford string that no longer sorts the same way or breaks
  the fixed 26-character length assumption other code might rely on.
- **Type-prefix delimiter collision**: the chosen prefix format `bl_<ulid>` needs the
  delimiter (`_`) to never appear inside the ULID's own alphabet (Crockford base32 doesn't
  use `_`, so this is safe) — worth stating explicitly as a validated assumption rather than
  an accident, since a different encoding choice later could reintroduce ambiguity in
  splitting `type` from `id` at the URL-parsing boundary.

## Summary of design-against items for Phase 3

1. Keep the `ssq://` payload free of secrets (already a stated constraint) — hijacking the
   scheme name is possible on both OSes and can't be fully prevented, only made low-value.
2. Treat macOS `.app`-bundle packaging for `CFBundleURLTypes` as its own scoped
   sub-deliverable (or explicit punt) — it is unrelated to the existing TCC/codesign
   mechanism and needs a wrapper bundle, not a plist-embedding trick.
3. Make Linux desktop-file registration idempotent (rewrite, not append) and always call
   `update-desktop-database` after writing it, mirroring the existing idempotent
   `setup-codesign.sh` pattern.
4. Do not attempt to change the `id` field's ent type — add ULID as a new additive
   `public_id`-style column and branch resolution logic on ID shape at the routing
   boundary.
5. Treat cross-host handoff as blocked on a **new** capability (resolvable URL +
   liveness/heartbeat per peer) that `ListWorkspacePeers` does not provide today — budget
   for that as prerequisite work, not a rewire of existing code.
6. Never build a cross-host redirect URL from the incoming link's hostname segment directly
   — resolve it only through the local peer registry the current host already trusts.
7. Use a monotonic-entropy ULID generator (e.g. `oklog/ulid` + `ulid.Monotonic`), not a
   bare random one-shot, and keep Crockford base32 all the way through — no base64 detour.
8. Give cross-host handoff calls an explicit timeout distinct from "peer not registered,"
   so the UX can distinguish "not reachable" from "not known" per the no-silent-failure
   requirement.
