# ADR-003: ConnectRPC API Design for Session Defaults

Date: 2026-04-13
Status: Accepted

## Context

The frontend needs to: (a) fetch resolved defaults when the create-session dialog opens,
(b) manage global defaults, profiles, and directory rules from the Settings page.

Options considered:
1. One fat RPC (`GetSessionDefaults` returns everything; `UpdateSessionDefaults` accepts
   everything) — simpler proto surface, harder to make atomic per-section saves
2. Granular RPCs per operation — more endpoints, but each maps to one UI action and
   one atomic config write
3. Embed defaults into existing `CreateSession` RPC — no new surface, but couples
   creation with settings management

## Decision

**Granular RPCs** added to the existing `SessionService` (no new service):

```
GetSessionDefaults  → returns full SessionDefaults struct
ResolveDefaults     → returns merged result for a given workingDir + profileName
UpdateGlobalDefaults
UpsertProfile / DeleteProfile
UpsertDirectoryRule / DeleteDirectoryRule
```

`ResolveDefaults` is the key endpoint called on dialog open and on profile/path changes.
It returns not just the merged values but also `used_global`, `used_directory`,
`used_profile`, and `matched_directory` so the React form can show per-field source
badges.

`CreateSessionRequest` gains two optional fields:
- `profile string` — apply profile on server side during creation (belt-and-suspenders)
- `skip_defaults bool` — bypass all defaults (for scripted or explicit-empty sessions)

## Rationale

- Granular RPCs map 1:1 to UI save buttons — each results in one predictable config write
- `ResolveDefaults` is read-only; it can be called freely without side effects
- Extending `SessionService` avoids a new handler registration chain and keeps the
  proto file consistent with the existing single-service pattern
- Returning source metadata from `ResolveDefaults` enables the per-field badge UX
  without a second RPC

## Consequences

- Proto file gains ~8 new RPCs and ~15 new message types; manageable in a single proto
- Settings page saves are not transactional (each section saves independently); users
  won't see partial saves but concurrent edits across sections are not atomic
- `ResolveDefaults` is O(n) over DirectoryRules on every dialog open; negligible for
  typical rule counts (<20)
