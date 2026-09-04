# Research: Similar Features, Edge Cases, Unstated Needs

## Closest existing precedent: SessionGoal (1:1 freeform data per session)

`SessionGoal` (`session/ent/schema/session_goal.go`) is architecturally the
closest match to "one field attached 1:1 to a session, persisted in ent/SQLite":

```go
// session/ent/schema/session_goal.go
field.String("session_uuid").NotEmpty(),
field.String("goal").MaxLen(2000).NotEmpty(),
...
index.Fields("session_uuid").Unique(), // 1:1 per session
```

Key structural fact: **`Session` itself is not an ent entity** — core session
state lives in `sessions.json` (per repo `CLAUDE.md`: "State and logs live in
`~/.stapler-squad/`... `sessions.json`"). `SessionGoal` and `SessionSummary`
are separate ent/SQLite tables keyed by `session_uuid`/`session_id`, merged
into the in-memory `Session`/instance data at load time
(`session/storage.go:322`, `client.SessionGoal.Query()...` inside whatever
loads instance data). **This means session notes should follow the same
shape: a new `SessionNote` ent entity table with a unique `session_uuid`
index, not a new field bolted onto a JSON-backed `Session` struct.**
`session/storage.go:1191-1330` (`SetSessionGoal`, `GetSessionGoal`, and the
tx-based upsert around line 1293-1321) is the direct template for
`SetSessionNote`/`GetSessionNote` storage methods — including the "load
existing row → mutate/create inside a tx" upsert pattern.

`SessionSummary` (`session/ent/schema/session_summary.go`) is the second
precedent, notable for a **deliberate design choice already documented in
this repo**: it has no `Edges()` back to `Session` specifically so the
summary row survives session deletion (see its doc comment, referencing
ADR-001/ADR-002). This is directly relevant to one of the requirement's open
edge cases (see "Note on archived/deleted session" below) — the repo already
has both patterns available (survive-deletion vs. not) and has picked
deliberately per-entity before.

`BacklogItem.notes` (`session/ent/schema/backlog_item.go:72`) is explicitly
called out in the requirements doc as *not* the right precedent — it's a
plain `Optional()` string field on an already-ent-backed entity, which
doesn't transfer directly since `Session` isn't ent-backed.
`BacklogProgressNote` (`session/ent/schema/backlogprogressnote.go`) is an
append-only *history* log — explicitly out of scope per the requirements'
non-goals (single field, last-write-wins, no revisioning).

## RPC/service wiring precedent

`SessionGoal` is **not** exposed through a `SessionService` ConnectRPC method
— it's set exclusively via an MCP tool (`server/mcp/tools_goal.go`,
`set_session_goal`) and rendered read-only in the frontend
(`web-app/src/components/sessions/GoalPanel.tsx`, sourced from
`SessionGoalSummary` embedded as field 59 on the `Session` proto message).
That's the wrong RPC precedent for notes, since notes need to be
**user-editable from the UI**, not agent-set.

The right RPC precedent is `UpdateSessionRequest`
(`proto/session/v1/session.proto:579-609`), which already carries a set of
independently-optional per-field updates (`title`, `category`, `program`,
`working_dir`, `tags`, `rate_limit_enabled`, `pause_reason`). Adding
`optional string note = <next field number>` here — mirroring `title`/
`category` — lets the existing single `UpdateSession` RPC carry note edits
without a new RPC. (A dedicated `SetSessionNote` RPC, mirroring
`RenameSessionRequest`, is the alternative if the team prefers giving notes
their own endpoint the way rename got one — either is consistent with
existing conventions.)

On the frontend, `SessionDetailView.tsx` has a full working example of the
inline click-to-edit pattern notes need: `isEditingCategory` /
`isEditingProgram` / `isEditingWorkingDir` / `isEditingTags` state
(`SessionDetailView.tsx:202-226`), all built on a shared factory,
`makeStringFieldEditor` (`SessionDetailView.tsx:363-380`):

```ts
function makeStringFieldEditor(editValue, originalValue, setValue, setEditing, updateFn) {
  return {
    handleSave: async () => { if (editValue !== originalValue) await updateFn(editValue); setEditing(false); },
    handleCancel: () => { setValue(originalValue); setEditing(false); },
  };
}
```

paired with `useSessionService.ts`'s `updateSession(id, updates: Partial<UpdateSessionRequest>)`
(`useSessionService.ts:93,301-331`). Session notes should reuse this exact
factory rather than inventing new edit-state machinery — the only real
addition is toggling between a `ReactMarkdown` read view and a `<textarea>`
edit view (existing fields are plain text inputs, not markdown).

## Markdown rendering precedent (already used twice, same libraries)

`react-markdown` + `remark-gfm` are already used for exactly this
"freeform markdown blob, rendered read-only" shape in two places:

- `web-app/src/components/backlog/detail/DescriptionSection.tsx:3-27` — backlog item description
- `web-app/src/components/sessions/SessionSummaryPanel.tsx:5-6,460` — session summary narrative

Both follow the identical pattern: `<ReactMarkdown remarkPlugins={[remarkGfm]}>{text}</ReactMarkdown>`
wrapped in a shared `markdownBody.css` class
(`web-app/src/components/backlog/markdownBody.css`, reused by
`SessionSummaryPanel.css` import). **Security note (verified):** neither
usage passes `rehypeRaw` or any HTML-passthrough plugin, and
`rehype-sanitize` is not even a dependency (checked `web-app/package.json`)
— it isn't needed because `react-markdown` does not render raw HTML by
default; markdown-embedded `<script>`/`<img onerror>` etc. are rendered as
literal text, not DOM. As long as the notes renderer copies this pattern
exactly (no `rehypeRaw`), there is no XSS exposure to add sanitization for.
Only becomes a risk if someone later adds `rehype-raw` to any of these three
consumers — worth a one-line comment warning future editors, not a runtime
guard.

## SessionCard indicator precedent

`SessionCard.tsx:765-773` already renders a goal-derived badge inline in the
card body (not in the `badges` row) when `session.goal?.goalText` is
truthy — `truncateGoal(session.goal.goalText, 61)` plus a task-fraction
suffix. This is a reasonable template for a notes indicator, though the
requirement asks for a "lightweight visual indicator" (implying icon/dot,
not truncated text) — the `badges` div (`SessionCard.tsx:458`) alongside
`ReviewQueueBadge`/`GitHubBadge`/`StatusBadge` is the more literal home for
an icon-only badge, with a `title="Has a note"` tooltip and a
`data-testid="badge-has-note"` (matching existing `data-testid="badge-*"`
conventions used by the autonomous/workflow badges at lines 576, 587, 598,
609, 622, 631).

## Multi-tab / concurrent-edit behavior (already solved generically)

`UpdateSession` publishes `events.NewSessionUpdatedEvent(instance, []string{...})`
to the event bus (e.g. `server/services/session_service.go:2940` for the
title-rename case), which every open tab receives via the `WatchSessions`
streaming RPC. Because the requirements doc explicitly rules out
optimistic-concurrency revisioning ("last-write-wins is acceptable"), no new
conflict-handling code is needed — a second tab's `UpdateSession` call will
simply publish another event and the first tab's UI will pick up the
overwritten value on the next stream event, consistent with how `title`/
`category`/`tags` already behave across tabs today. Worth confirming in
implementation that the note's `updated_at` (or equivalent) round-trips
through this event so a tab mid-edit doesn't get its in-progress textarea
silently clobbered by an incoming stream update (existing fields don't fully
solve this either — `isEditingCategory` etc. simply doesn't re-sync from
props while `isEditing*` is true, per the `useEffect` guard pattern visible
at `SessionDetailView.tsx:206-207`; notes should copy that guard so a
stream-driven note update doesn't stomp an open edit).

## Edge cases and failure modes to design for

1. **Very long notes.** `SessionGoal.goal` caps at `MaxLen(2000)`;
   `BacklogItem.notes` and `SessionSummary.markdown`/`narrative` are
   unbounded `Optional()`/`Text()` fields with no app-level cap (the
   `BacklogProgressNote.note` doc comment explicitly notes "rendered call
   sites are responsible for truncation... stored unbounded here").
   Recommendation: pick a length cap up front (the requirements doc doesn't
   specify one) and enforce it in the ent schema (`MaxLen`) plus a matching
   frontend char-count/limit on the textarea, rather than leaving it
   unbounded like `BacklogItem.notes` — an unbounded free-text field on a
   frequently-fetched entity (sessions load on every list/poll) risks
   bloating `ListSessions` response payloads more than an occasionally-read
   backlog item description does.
2. **Concurrent edits from multiple browser tabs.** Covered above — last-
   write-wins via existing event-bus propagation is acceptable per
   non-goals, but guard in-progress edits from being clobbered by an
   incoming stream update (see previous section).
3. **Markdown rendering safety.** Not actually an XSS risk given the
   existing `react-markdown`-without-`rehype-raw` pattern (verified above) —
   but very large or deeply nested markdown (e.g. deeply nested lists, huge
   tables via `remark-gfm`) could still cause a rendering perf hit; the
   length cap in (1) doubles as mitigation here.
4. **Note on a session that gets archived.** `ArchiveSession`
   (`server/services/session_service.go:4283-4306`) only sets
   `archived_at` on the session record — it doesn't touch any
   `SessionGoal`/`SessionSummary`-style side tables, so an existing note
   would naturally survive archival with no extra code, exactly like the
   goal does today. No special-casing needed as long as `GetSession`/
   `ListSessions` continues to hydrate the note for archived sessions the
   same way it does for active ones.
5. **Note on a session that gets deleted.** This is a real, currently
   **unhandled** gap in the closest precedent: `DeleteSession`
   (`server/services/session_service.go:2015-2107`) deletes the JSON-backed
   instance record (`s.storage.DeleteInstance(sessionTitle)`) but — per a
   grep of the handler — never deletes the corresponding `SessionGoal` row
   by `session_uuid`. That means goal rows currently leak as orphans on
   session delete, and a naively-implemented `SessionNote` would inherit the
   same leak. Two real options, not a default: (a) explicitly delete the
   note row in `DeleteSession` (tightest, avoids orphan accumulation,
   diverges from `SessionGoal`'s current behavior), or (b) explicitly
   decide to leave it orphaned like `SessionGoal` does today, documented as
   an intentional non-goal-fix. Either is defensible, but the plan should
   pick one rather than silently inheriting whichever `SessionGoal`
   happens to do by omission.
6. **Empty vs. whitespace-only notes.** The "non-empty" indicator badge on
   `SessionCard` (acceptance criterion 3) needs a clear definition: a note
   of `"   \n"` should almost certainly not trigger the badge or count as
   "has a note." `SessionGoal.goal` uses `.NotEmpty()` at the ent-schema
   level for its required field, but that only rejects a literal empty
   string, not whitespace — trimming should happen either at the API
   boundary (reject/normalize whitespace-only submissions to a true empty
   note, deleting the row rather than storing `"   "`) or at minimum in the
   badge-visibility check on the frontend (`session.note?.trim()`).

## Unstated needs beyond the explicit requirements

- **Quick-add from `SessionCard` without opening detail view.** The
  requirements only ask for editing "from the session detail view" and a
  *read* indicator on the card, but every other quick-editable field in this
  codebase (tags, category via right-click context menus, or the omnibar)
  has some faster path than "open full detail view." Given the actual
  BacklogItem `notes` field precedent is edited from a form and this note
  is meant for micro-context ("left this waiting on X"), a quick-add
  affordance directly on the card indicator (e.g. click badge → inline
  popover) would likely get used more than round-tripping through the full
  detail view each time — worth flagging as a fast-follow even if out of
  MVP scope, since the requirements doc doesn't rule it out explicitly the
  way it does titled/multi notes or revisioning.
- **Search/filter sessions by note content.** `SessionSearchDetector`
  (omnibar priority 200, per `.claude/rules/feature-testing-registry.md`)
  is the existing "everything else" search fallback for sessions — it's
  unclear from the requirements whether note text should be indexed there.
  Not in the stated acceptance criteria; flag as an open question for the
  plan phase rather than assuming in scope.
- **Note timestamps.** `SessionGoal.updated_at` (`UpdateDefault(time.Now)`)
  and `BacklogProgressNote.created_at` both surface "when was this last
  touched" in their respective UIs. The requirements doc doesn't ask for a
  visible timestamp on the note, but the ent schema should still capture
  `updated_at` (essentially free, and precedented by every sibling entity)
  even if the UI doesn't surface it in v1 — cheap insurance against a later
  "when did I write this note" ask.
- **Mobile touch targets for the edit toggle.** Per repo-wide instinct
  (`feedback_mobile_desktop_ux.md`), the existing inline-edit affordances in
  `SessionDetailView.tsx` (small `✓`/pencil-style buttons, e.g.
  `styles.saveButton` at line 901) should be checked against touch-target
  size when reused for notes — the requirements doc already flags this
  generally but it's worth calling out that the *specific* precedent
  component being reused (the inline `isEditing*` toggle buttons) is a
  known instance to check, not a hypothetical new pattern.

## Files worth reading in full before/during planning

- `session/ent/schema/session_goal.go` — schema template
- `session/storage.go:1191-1330` — Set/Get storage-method template (upsert-in-tx pattern)
- `proto/session/v1/session.proto:579-620` — `UpdateSessionRequest`, where a `note` field would land
- `server/services/session_service.go:2015-2107` — `DeleteSession`, where the orphan-row decision (edge case 5) needs to be made
- `web-app/src/components/sessions/SessionDetailView.tsx:200-230,363-420` — inline-edit state + `makeStringFieldEditor` factory to reuse
- `web-app/src/components/backlog/detail/DescriptionSection.tsx` — minimal `ReactMarkdown` + `markdownBody.css` read-mode template
- `web-app/src/lib/hooks/useSessionService.ts:93,301-382` — `updateSession` RPC call shape
- `web-app/src/components/sessions/SessionCard.tsx:458,765-773` — badge row vs. inline-goal-text precedent for the card indicator
