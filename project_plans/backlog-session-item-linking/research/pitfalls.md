# Pitfalls: exposing AttachSessionToItem as an MCP tool + slash-command regeneration + PERMISSION_DENIED hints

Commit researched at: `687391ad4` (HEAD of this worktree).

## 1. `AttachSessionToItem` has no ownership/exclusivity check today — this is the core risk

`server/services/backlog_service_sync.go:29-131` (`AttachSessionToItem`):

- Validates `item_id`/`session_uuid` are non-empty (lines 38-43).
- Validates `item.Status` is `idea`, `ready`, or `in_progress` (lines 54-60) — but **not** whether
  the item already has a *different, still-active* session attached.
- Loads `attachPriorSessions` (line 68) only to seed the context file — it is never consulted to
  block or warn about an existing live owner.
- Unconditionally calls `CreateItemSession` (lines 75-83), which always inserts a **new**
  `ItemSession` row (`session/storage_backlog.go:77`, `session/ent/schema/item_session.go`). There
  is no unique index on `(item_id, session_uuid)` or on `item_id` alone
  (`session/ent/schema/item_session.go:79-87` — the only indexes are `session_uuid` alone and
  `created_at`-by-edge). Nothing marks a prior `ItemSession` as ended (`ended_at` is never set here).
- The existing "is this session allowed to act on this item" check used by `report_progress`,
  `request_review`, `submit_review_verdict`, `report_pr_created`, and `submit_triage_result`
  (`server/mcp/tools_backlog.go:302-306`, `376`, `505`, `665`, `758`) is
  `GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)` — it only asks "does *a* row exist
  linking this session to this item," never "is this the *current/exclusive* owner." Because old
  `ItemSession` rows are never closed out on attach, **both the old and the new session stay
  "linked"** and can both call `report_progress`/`request_review` concurrently after a relink.

**Consequence if exposed as-is:** any live session, given only an `item_id` (which is not secret —
it's visible in `get_backlog_item` results, in other sessions' generated slash commands, in the UI),
could call the new tool to attach itself to an item **currently owned and being worked by another
session**, without the original session's knowledge or consent. The item is not "taken away" from
the first session (no state prevents the first session from continuing to call `report_progress`),
so the practical failure mode is silent duplicate/conflicting work and two sessions racing to
write acceptance-criteria status (`UpdateAcCriterionStatus`, not session-scoped) and post competing
review requests on the same item — a state-machine violation the requirements explicitly call out
as a risk to design against.

## 2. No transactional guard around attach — write races on SQLite

- `CreateItemSession` (line 75) and the subsequent `TransitionBacklogItemStatus` call (line 121)
  are two separate ent calls, not wrapped in a single `ent.Tx`. The existing code already
  acknowledges the transition can fail independently of the attach (`notifyTransitionFailed`,
  line 124) — "a session was attached to the item but its transition to in_progress failed" — a
  known partial-failure shape today, before this feature ships.
- `item.Status` is read once at the top (`GetBacklogItem`, line 46) with no optimistic-concurrency
  field on `BacklogItem` (no version/etag). Two concurrent `AttachSessionToItem` calls for the same
  item both pass the same status check, then both successfully `CreateItemSession`, then both race
  on `TransitionBacklogItemStatus` — whichever loses just logs a warning, it does not roll back the
  other's `ItemSession` row. SQLite's single-writer semantics serialize the raw writes but do not
  make this sequence atomic or first-writer-wins; it's simply last-write-wins with no rejection of
  the second caller.
- `s.worktreeMu` (line 93) only guards the slash-command *file* writes for a single instance path,
  not the DB writes — it cannot prevent two different callers from interleaving `CreateItemSession`
  calls for the same item.

## 3. Slash-command regeneration is not atomic as a set — confirms the crash-mid-regeneration pitfall named in requirements

`session/backlog_commands.go`:

- `WriteBacklogContextFile` (lines ~195-204) **is** crash-safe: it writes to `destPath + ".tmp"`
  then `os.Rename`s over the destination — an atomic swap.
- `WriteSlashCommands` (lines 31-68) is **not** atomic as a set. It builds a `map[string]string` of
  ~2N+4 files (`status.md`, `done-N.md`/`fail-N.md` per acceptance criterion, `review.md`, `ship.md`,
  `help.md`) and writes each one individually via `writeFile` (line 62 calling the helper at
  line 231-234), which does a **direct `os.WriteFile`** with no temp-file/rename step. If the
  process is killed mid-loop (e.g. `make install-service`/systemd restart per
  `.claude/rules/tmux-keep-server-on-restart.md`, or any crash), some command files will reference
  the new item and others will still reference the old item — exactly the "stale slash-command files
  surviving a crash mid-regeneration" scenario the requirements ask to be designed against, and it
  is a **pre-existing** gap, not something this feature introduces — but adding a new,
  agent-triggerable relink path multiplies how often this write happens and therefore how often the
  crash window is hit.
- `selfHealWorktreeScaffolding` (line 35) runs before every write, which is good self-healing for
  git pollution, but does nothing for a torn *set* of already-written files.

## 4. Idempotency

Calling the prospective tool twice for the same `(session_uuid, item_id)` pair is not a no-op:
each call inserts a fresh `ItemSession` row (no upsert — see `--feature sql/upsert` note in
`.claude/rules/ent-schema-generation.md`, which is about *generation*, not about this method using
upsert at all), re-derives `acSnapshot` from current item state (so repeated calls silently produce
different snapshots as the item's AC changes underneath), and re-attempts the
`in_progress` transition (harmless if already `in_progress`, but each failed attempt fires
`notifyTransitionFailed` again — a retry-storm/notification-spam risk if an agent loops the tool
call under confusion).

## 5. No role/state scoping on the caller

- `session.SessionRoleWork` is hardcoded in the request built by `AttachSessionToItem` (line 78)
  regardless of what kind of session is calling — a triage-only or review-only session could call
  the new tool and get itself registered with `session_role = "work"` on an item, which is a
  category confusion the internal-only caller (`SpawnSessionFromItem`) never had to guard against
  because it always calls with intent already known.
- Nothing checks that the *calling* session doesn't already own a different in-progress item
  before letting it attach to a second one — an agent could accumulate links to multiple items
  simultaneously with no cap, which the UI-driven spawn flow doesn't allow today (one item per
  spawned session by construction).
- `item.Status == idea` is accepted (line 54-56), i.e. items that haven't been triaged/marked
  `ready` are attachable. The MCP-exposed tool would let an agent self-serve an untriaged item,
  bypassing whatever gate exists between `idea` and `ready` in the normal UI/triage flow.

## 6. Backward compatibility with already-on-disk command files

Per the requirements' reproduced bug, sessions already running **today** have `.claude/commands/backlog/*.md`
baked with a stale/nonexistent `item.ID` (`session/backlog_commands.go:79-136` bakes `itemID` into
every file's text at generation time). The new tool's design must account for:
- A session calling the new relink tool for the first time mid-task, with an existing (possibly
  half-written, per §3) command set already on disk — regeneration must fully replace, not merge,
  the file set (e.g. if the new item has fewer acceptance criteria than the old one, stale
  `done-N.md`/`fail-N.md` for indices beyond the new count must be removed, not just overwritten —
  `WriteSlashCommands` only ever writes files present in the new map; it never deletes files absent
  from it).
- Sessions that never call the new tool remain stuck with stale commands — the requirements
  explicitly scope this as "no automatic/implicit relinking," so this is an accepted gap, but worth
  stating plainly in the plan rather than implying the new tool fixes every live session
  automatically.

## 7. General pitfalls of exposing an internal admin/service op to an LLM agent caller

These are the categories this specific case instantiates, for reference when writing acceptance
criteria:

- **Authorization scope creep**: an operation written for a single trusted internal caller
  (`SpawnSessionFromItem`) assumes its caller already made the authorization decision (this item
  is unclaimed, this session should own it). Exposing the same method as a tool moves the trust
  boundary to "any agent that can reach the MCP server," so every assumption the internal caller
  satisfied implicitly must become an explicit check.
- **Confused deputy**: the tool executes with the server's full backend privileges on behalf of
  whatever the agent asks, using only a self-reported `item_id` string and the context-injected
  `STAPLER_SESSION_UUID` as identity — there's no separate authorization check that the *item* is
  one this session is entitled to touch (contrast with `report_progress` etc., which at least check
  "is there a link," even if that check is non-exclusive per §1).
- **Idempotency/retries**: LLM agents retry tool calls liberally on ambiguous errors or timeouts;
  any tool without idempotent semantics (§4) will multiply side effects under normal agent retry
  behavior, not just adversarial use.
- **Race conditions under concurrent agents**: this codebase already runs many sessions concurrently
  by design (that's the product) — any new tool must assume it will be called concurrently for the
  same resource, not defensively coded as if calls are serialized.
- **Prompt-injected or hallucinated arguments**: because `item_id` is a plain string argument the
  agent supplies, a prompt-injection payload encountered mid-task (e.g. in a file the agent reads)
  could induce a call with an attacker-chosen `item_id`, hijacking the session's linkage. Validating
  UUID format (already done, `validateUUID`, `tools_backlog.go:47-52`) does not prevent this class.

## Recommendations to carry into the plan

1. Add an explicit "already owned by a different live session" check before creating a new
   `ItemSession` row — decide the policy (reject with an actionable error vs. explicit
   force-relink flag vs. auto-close the prior `ItemSession` by setting `ended_at`) rather than
   leaving both rows "linked" simultaneously.
2. Wrap attach's DB writes (`CreateItemSession` + status transition) in a single `ent.Tx` if ent's
   client supports it here, or otherwise make the sequence explicitly compensating on partial
   failure, rather than only logging.
3. Make `WriteSlashCommands`' file set write atomic-as-a-set: write to a temp subdirectory and
   rename the directory, or write each file via temp+rename (matching `WriteBacklogContextFile`'s
   existing pattern) and delete files for AC indices no longer present in the new item.
4. Make the new MCP tool idempotent for repeat calls with the same `(session_uuid, item_id)` —
   no-op or refresh-only if already linked and no ownership conflict exists.
5. Decide and enforce a caller-role policy (should a triage/review session be able to call this at
   all? should `session_role` be inferred rather than hardcoded to `work`?).
6. Decide whether `idea`-status items should be attachable via the agent-facing tool, or whether
   that should require `ready` (tightening vs. the current internal-only behavior).
7. State explicitly in the plan that sessions which never call the new tool are not retroactively
   fixed — this is a known accepted gap per the requirements' non-goals, not an oversight.
