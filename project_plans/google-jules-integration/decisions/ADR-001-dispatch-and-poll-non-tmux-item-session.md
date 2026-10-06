# ADR-001: Model a Jules unit of work as a non-tmux `ItemSession`, dispatched and polled

**Status**: Accepted
**Date**: 2026-09-01
**Project**: google-jules-integration
**Supersedes**: n/a

## Context

Google Jules runs coding sessions on Google-managed cloud VMs. Verified from the
API docs during Phase 2 research (`research/stack.md` §Sources,
`research/architecture.md` §0):

- `Sessions.create` requires `sourceContext.source` (a `sources/{id}` registered
  through Jules' **web app** — there is no create endpoint for sources) plus
  `sourceContext.githubRepoContext.startingBranch`, an **already-pushed** branch
  on that GitHub repo. Jules can never target a local git worktree.
- Status is pull-only. No webhook, streaming, or SSE endpoint is documented.
- `Session.outputs[].pullRequest.url` carries the PR once `state == COMPLETED`.

stapler-squad's session model, by contrast, assumes a local process:
`SessionCreator` (`server/services/backlog_service.go:30-36`) returns
`*session.Instance`, and `session.NewInstance` unconditionally constructs a tmux
`ProcessManager`. `session/ent/schema/session.go` has PTY dimensions, a worktree
edge, and a backend pin.

Three shapes were on the table (requirements.md's own (a)/(b)/(c), refined by
research into the four approaches in `implementation/plan.md`).

## Decision

Model a Jules unit of work as an **`ItemSession` row with a new
`SessionRoleJulesWork = "jules_work"` role and no backing `Session`/`Instance`**,
created by a new push-shaped `JulesDispatcher` and advanced by a dedicated
`JulesSessionPoller`.

Concretely:

- `ItemSession.session_uuid` holds `"jules-" + sessions/{id}` — the loose-FK
  string column is already documented as "not an ent edge"
  (`session/ent/schema/item_session.go:23-24`).
- `SessionRoleJulesWork` is **excluded** from `IsTmuxBackedSessionRole`
  (`session/backlog.go:73-75`), so terminal-item cleanup sweeps skip it.
- On `state == COMPLETED` with a PR, the poller calls the existing
  `Storage.SetBacklogItemPRAndTransition` (`session/storage.go:898`), after which
  the existing `ReconcilePRPending`
  (`session/backlog_lifecycle_pr.go:1592`) walks the item to `done` with **zero**
  new code — satisfying the requirement that the PR flow through the existing
  review path rather than a new surface.

## Alternatives Considered

### Widen `SessionCreator` with `CreateJulesSession` — Rejected

Both existing methods return `*session.Instance`. Satisfying that contract for a
cloud session means fabricating a tmux `ProcessManager` for a process that does
not exist, or refactoring `session/`'s core so `Instance` becomes optional —
precisely the ripple requirements.md's isolation constraint forbids. It is also
textbook interface pollution: an otherwise coherent "start a local process"
interface would gain one method that starts nothing locally.

### Pull-only PR import via `ItemSourcePlugin` (requirements' option (b)) — Rejected

`ItemSourcePlugin.Fetch` answers "what external items are new since this cursor".
There is no create direction anywhere in the plugin ecosystem — the nearest
precedent, `importGitHubIssue`, is a one-off *pull*. Without a
`Sessions.create` call the user must still start the work at jules.google.com,
which fails the success metric verbatim ("create a Jules-backed unit of work from
stapler-squad's backlog UI without visiting jules.google.com"). It is
alternative (a) — deep-link only — with an import step attached, and (a) was
already rejected.

### Poll on the existing backlog reconciliation tick — Rejected

Avoids a new goroutine, but couples an alpha API's latency into the sweep that
also drives local sessions. One hung call would stall local-session
reconciliation, contradicting `research/pitfalls.md` §1's "fail soft — an adapter
error must not crash the poller loop that also serves local-agent sessions".

### A new ent entity for remote sessions — Rejected

Would need a schema migration and duplicate most of `ItemSession`. Unnecessary:
`ItemSession` already carries non-tmux rows in production (headless triage,
`headlessTriageUUIDPrefix`, `server/services/backlog_service_triage.go:423`), and
every field a Jules row needs (`started_at`, `last_progress_at`, `ended_at`,
`end_reason`, `ac_snapshot`) already exists. The fields it does not need
(`base_commit_sha`, `last_commit_sha`, `commit_count_since_spawn`) simply stay at
their zero values, exactly as they do for triage rows.

## Consequences

**Positive**

- No ent schema migration, no data backfill, no change to `session/` core types.
- PR resolution and merge detection are entirely free — existing, unmodified code.
- Alpha-API churn is confined to the `jules/` package plus one poller file.

**Negative**

- A second background poller goroutine exists whose lifecycle and backoff shape
  partly duplicate `WorktreePRPoller`. Accepted deliberately: the duplication is
  structural similarity, not copied logic, and the isolation is the point.
- Jules sessions are invisible to every UI keyed on `*session.Instance` —
  `SessionDetailView`, the session list, `SessionMonitor`. They surface only on
  the backlog item detail page. This is why `plan.md` deviates from
  `research/ux.md` §2's "Activity tab in `SessionDetailView`" recommendation;
  that view is unreachable for a session with no `Session` row.
- Staleness needs its own model. `session/backlog_lifecycle_stale.go` is tuned
  to local tmux processes going quiet and would misfire on a healthy
  long-running cloud task, so the poller enforces its own `MaxSessionAge`.

## Addendum (Phase 4 plan repair, pre-mortem P1 #2)

requirements.md's success metric originally read "a Jules session appears in
`list_sessions`/backlog UI" — a literal contradiction of this ADR's own
Consequences section above, which already states Jules sessions are invisible
to every `*session.Instance`-keyed surface. requirements.md has been narrowed
to name the backlog item detail page explicitly and record `list_sessions`/MCP
visibility as an accepted, architecture-driven tradeoff rather than a gap;
this ADR's Decision and Consequences are unchanged — the addendum only
resolves the wording mismatch, it does not revisit the design.

## References

- `research/architecture.md` §0-§3, §7
- `research/features.md` §2
- `session/backlog.go:48-75`, `session/ent/schema/item_session.go:23-24`
- `server/services/backlog_service.go:30-36`
- `server/services/backlog_service_triage.go:418-423`
- `session/storage.go:898`, `session/backlog_lifecycle_pr.go:1592`
