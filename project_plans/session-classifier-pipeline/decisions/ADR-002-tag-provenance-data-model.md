# ADR-002: Minimal tag provenance via two side-maps, not a per-tag metadata struct

**Status**: Accepted
**Date**: 2026-09-11
**Context**: session-classifier-pipeline, Phase 3 planning

## Context

`research/features.md` flags that `Instance.Tags []string`
(`session/instance.go:254`) carries no provenance today — `AddTag`/
`RemoveTag`/`SetTags` (`session/instance_tags.go`) treat every tag as a bare
string with no origin. It identifies two edge cases that need *some* form of
provenance to satisfy this project's own constraints:

1. **Retraction** (requirements.md: sync rules re-run "on every mutation" to
   a fixpoint) — if branch `bugfix/foo` matched a rule that added tag
   `Bugfix`, and the session is later renamed off that branch, the rule's
   condition no longer holds. Without knowing *which* tags a rule added, a
   naive "remove tags whose rule no longer matches" pass risks removing an
   identical user-added `Bugfix` tag that happens to share the same string.
2. **User-removal should stick** (ux.md's "may reappear" problem) — if a
   user manually removes a rule-derived tag, the very next mutation's
   eager fixpoint re-run would silently re-add it with no explanation
   (research/ux.md calls this the single biggest trust risk, citing Gmail
   filters as the cautionary example).

features.md explicitly flags that a full `{source, pinned}` struct per tag
"is a LARGER SURFACE CHANGE than 'add a rule engine alone'" and asks this
plan to scope the *minimal* viable version.

## Decision

Do **not** change `Instance.Tags []string`'s shape or wire format. Instead,
add two new small persisted side-maps to `Instance`, alongside `Tags`:

```go
// RuleTagProvenance maps a tag value to the rule ID that most recently
// applied it via the sync fixpoint or the LLM poller. Absence from this map
// means the tag is user-owned (manually added, or its provenance was lost/
// never tracked) and must never be auto-retracted.
RuleTagProvenance map[string]string // tag -> ruleID (sentinel "llm" for LLM-derived)

// SuppressedRuleTags is the set of tags a user explicitly removed from a
// session that had rule provenance at removal time. The sync fixpoint and
// LLM poller must skip re-adding any tag present here, until the user
// removes the suppression (e.g. by re-adding the tag manually, which clears
// its suppression entry).
SuppressedRuleTags map[string]bool
```

Both are plain fields guarded by the same `i.mu` as `Tags` (see
`instance-lock-free-reads.md`), included in `InstanceSnapshot`, and
persisted the same way `Tags` is (JSON column in the existing session
storage row — not a new ent entity).

This is the **minimal** viable extension: two `map[string]{string,bool}`
fields, not a new `[]TagInfo{Value, Source, Pinned}` structure that would
require rewriting every `AddTag`/`RemoveTag`/`GetTags` call site and the
wire-format the frontend already consumes (`SessionCard.tsx`'s tag pills,
`TagEditor.tsx`'s CRUD). Reading/writing `Tags []string` is unchanged for
every existing caller; only the new rule-engine and poller code paths read
or write the two new maps.

## Rejected Alternatives

- **Bare `[]string`, no provenance at all** (do nothing): rejected — fails
  the retraction and no-silent-reappear requirements outright; this is the
  literal Gmail-filters failure mode research/ux.md warns against.
- **Full per-tag `{Value, Source, Pinned}` struct replacing `[]string`**:
  rejected as over-scoped for this project's Success Metrics — no
  requirement asks for arbitrary per-tag metadata (color, description,
  multiple simultaneous sources), and it would force a `Tags`
  wire-format/serialization change touching every existing tag call site
  and the frontend's tag-pill rendering, for no capability this project's
  acceptance criteria need.
- **Separate ent-backed "tag provenance" table**: rejected — provenance is
  small, per-session, and already lives alongside `Tags` in the same
  session storage row; a second table/join is unwarranted relational
  overhead for two maps that are always read/written together with the
  session they belong to.

## Consequences

- `session/instance_snapshot.go`'s `InstanceSnapshot` gains two more
  defensively-copied map fields (same convention as `Tags` at
  `instance_snapshot.go:106,184`).
- The sync fixpoint, when it adds a tag, records `RuleTagProvenance[tag] =
  rule.ID`; when a rule's condition stops matching on a later pass, it only
  removes `tag` if `RuleTagProvenance[tag] == rule.ID` (never a tag it
  doesn't own).
- `RemoveTag` (session/instance_tags.go) gains a check: if the removed tag
  has an entry in `RuleTagProvenance`, move it into `SuppressedRuleTags`
  before deleting the provenance entry, so the fixpoint/poller skip it next
  time. Re-adding the same tag value via `AddTag` clears its suppression.
- `SetTags` gets the identical check, applied via a tag-set diff (old vs.
  new) rather than a single tag value — this is the path the shipped Tag
  Editor Modal's save button actually calls (`TagEditor.tsx`'s `handleSave` →
  `UpdateSession` RPC → `instance.SetTags(tags)`), never `RemoveTag`/`AddTag`
  directly. Without this, the suppression guarantee above holds in unit
  tests but never fires from real user interaction — the two rule/tag
  mutation call sites (`RemoveTag`/`AddTag` vs. `SetTags`) share one small
  unlocked helper pair (suppress-if-provenanced / clear-suppression) so they
  can't drift apart.
- `Unclassified` (the LLM-failure sentinel) is excluded from
  `SuppressedRuleTags` by all three mutation paths, server-side — not only
  hidden from the remove control in the UI. Any caller (MCP tool, a
  different frontend surface, direct API call) that removes `Unclassified`
  simply clears its provenance entry rather than suppressing it, since a
  suppressed `Unclassified` would compound with the LLM poller's own
  suppression handling in a confusing way with no user-facing benefit.
- The LLM poller uses the `"llm"` sentinel value in `RuleTagProvenance` for
  its own writes, so the same retraction/suppression logic covers
  LLM-derived tags without a third parallel mechanism. The poller's apply
  path (`ApplyLLMTagResult`) filters candidate tags through the same
  `filterSuppressedTags` helper the sync fixpoint uses, rather than a
  separately-maintained check, since the poller re-runs on content-hash
  changes unrelated to suppression state and is otherwise the path most
  likely to silently resurrect a tag the user just removed.
