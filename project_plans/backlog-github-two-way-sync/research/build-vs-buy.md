# Research: Build vs. Buy — Bidirectional Backlog ↔ GitHub Issue Sync

Agent 6 (Build vs. Buy). Scope: `project_plans/backlog-github-two-way-sync/requirements.md`'s four
new mechanisms — closed-issue detection in `GitHubIssuesPlugin.Fetch`
(`session/backlog_plugin_github.go:90`), forward sync (backlog→GitHub), backward sync
(GitHub→backlog) in `SyncLoop.SyncOne` (`session/backlog_sync.go:195-303`), and loop prevention
between the two.

## Codebase facts that bound every option below

- `go.mod` has **no GitHub API client library** (`google/go-github`, `shurcooL/githubv4`, or
  anything else) — confirmed by grep across all 191 lines. `GitHubIssuesPlugin` and
  `GitHubPRsPlugin` (`session/backlog_plugin_github.go`, `session/backlog_plugin_github_prs.go`)
  are hand-rolled `net/http` + `encoding/json` against the raw REST API, using only stdlib
  imports. This is a deliberate, established convention (also independently confirmed by the
  sibling `backlog-github-issue-link/research/build-vs-buy.md`), not an oversight.
- Every existing GitHub API call in the repo is a **read** (`GET`). Grepping for
  `http.MethodPatch`/`http.MethodPost`/`CreateComment`/`/comments` across `session/` and
  `server/` turns up nothing outbound to GitHub's REST API — forward sync (closing/labeling an
  issue) would be the first *write* path this codebase has ever made to GitHub.
- The repo already has an **origin-tagging pattern for status transitions**: `TriggeredByUser` /
  `TriggeredBySystem` constants (`session/backlog.go:92-93`), threaded through
  `TransitionBacklogItemStatus` and recorded per-transition in `BacklogStatusEvent`
  (`session/repository.go:316`, `session/storage_backlog.go:639`). This is the same shape as the
  "origin tag" loop-prevention pattern discussed in §3 — it isn't new to this feature, it's an
  existing convention to extend.
- `session` already computes **state-derived labels** for GitHub PRs
  (`GitHubPRsPlugin.computeLabels`, `session/backlog_plugin_github_prs.go:141-...`), i.e. deriving
  local labels from remote state is a pattern the codebase already has one direction of (GitHub →
  local labels); this project adds the reverse (local status → GitHub labels/close) and needs both
  directions to coexist without thrashing.
- The sibling `project_plans/backlog-github-issue-link/` project (fully planned, unimplemented) already
  designed the `ExternalURL` field addition to `BacklogItemData`/`BacklogItemUpdate`/the ent schema
  via a plain-string, no-newtype, `SyncOne`-caller-side-gating approach (ADR-001, "Pattern
  Decisions" table in `implementation/plan.md`). `Labels` needs the identical treatment.

---

## 1. Existing OSS library for bidirectional GitHub issue sync

**Searched**: standalone Go libraries, and prior art among GitHub Actions/webhook-based two-way
sync tools (even if not directly reusable as a Go dependency) — `espressif/sync-jira-actions`,
`canonical/sync-issues-github-jira`, GitHub Marketplace's "Bidirectional GitHub and Jira
Integration", Atlassian's own "GitHub + Jira Two-Way Sync".

**Findings**:
- No standalone Go library or service implements "sync an arbitrary local datastore's status/labels
  bidirectionally with GitHub issues" — this is inherently applicaton-specific (what "status" means
  locally is never generic), so nothing packages the *mapping* logic, only the *transport*
  (`go-github` for API calls, which this repo already avoids — see above).
- Every bidirectional-sync tool found in this category (canonical's own Action, Espressif's Action)
  is explicitly **one-way** (GitHub → Jira) despite living under a "sync" name — the search itself
  surfaced that "most GitHub Actions focused on GitHub↔Jira integration are one-way; true
  bidirectional sync typically requires third-party iPaaS solutions," which is directly relevant:
  even in the much larger and older GitHub↔Jira ecosystem, no OSS Action-level library solved true
  two-way sync — that problem always escalated to a hosted platform (Unito, Exalate). That's a
  signal this is a genuinely hard general problem, not that this project is reinventing something
  already solved for free.
- **Prior-art convention worth borrowing** (not reusable as code, but as a pattern): every
  bidirectional tool surveyed (Unito, Exalate) implements loop-prevention via a "last sync
  direction" / "who changed it last" marker per synced field, not by diffing full state — this
  independently corroborates §3's recommendation below.

**Pros of adopting a library**: none identified — no candidate exists for the actual problem
(bidirectional status/label mapping against a private local schema).

**Cons**: N/A (nothing to adopt).

**Verdict: Not recommended (no viable candidate) — must be built.** This isn't a "build vs. buy"
choice at the library layer; it's confirmed there is no library layer to buy into. Continue the
existing `net/http`-only convention: forward sync is a `PATCH /repos/{owner}/{repo}/issues/{number}`
call and label sync a `POST .../labels` call, both trivially hand-rolled to match the existing
`githubIssue`/`githubAPIURL` shape in `backlog_plugin_github.go`.

---

## 2. SaaS / managed (GitHub Apps marketplace, Zapier, Unito, etc.)

**Findings**:
- **Unito** ([unito.io/integrations/github-jira](https://unito.io/integrations/github-jira/),
  [GitHub Marketplace: 2-Way Sync by Unito](https://github.com/marketplace/2-way-sync-by-unito))
  and **Exalate** ([exalate.com](https://exalate.com/blog/jira-github-issues-integration/)) are the
  two real hosted two-way sync platforms found, both built for GitHub↔(Jira/Asana/ClickUp/etc.),
  not GitHub↔"a private local SQLite-backed backlog app with no public API." Pricing starts ~$10–20/mo
  scaling with seats/connections (Unito) — a nonzero recurring cost for a feature whose entire
  counterparty (the local `BacklogItem` ent schema, `session/ent/schema/backlog_item.go`) has no
  webhook endpoint, no public schema, and no stable API contract for a third party to sync against.
- These platforms sync between two systems **each with their own API**. This project's local side
  is not an addressable system from the outside — there is no ConnectRPC/REST endpoint a SaaS tool
  could push webhook events to (the whole sync model here is poll-based, per the requirements'
  explicit non-goal: "Real-time push (webhooks) — sync remains on the existing poll-based
  `SyncLoop` cadence"). Adopting Unito/Exalate would mean either (a) building and exposing a new
  authenticated public API surface just to be the "other side" of their sync (far more surface
  area than the feature itself), or (b) running a GitHub webhook receiver against a locally-run,
  often not publicly reachable, personal-tool instance — architecturally mismatched with "run on
  localhost:8543, single-user, git-worktree-based dev tool" (per this repo's own CLAUDE.md).
- GitHub Apps Marketplace itself has no generic "sync my private DB to my issues" app — every
  listing found is Jira/Asana/Linear/Trello-specific, i.e. syncs against another *named* SaaS
  product's schema, not an arbitrary local store.

**Pros**: zero code to write for the sync engine itself; handles rate-limiting, retries, and some
loop-prevention out of the box for the *systems it supports*.

**Cons**: no product in this space targets "arbitrary local app ↔ GitHub issues"; would require
building and hosting a public API/webhook surface that doesn't otherwise need to exist; recurring
per-seat cost for a personal/internal tool; loses control over exactly which local fields
(`Status`, `Labels`, `UserModifiedFields` local-wins semantics) map to which GitHub state — the
local-wins guard (AC4/AC7) is itself a business rule no generic SaaS sync tool exposes as a
first-class primitive.

**Verdict: Not recommended.** The fundamental mismatch is that this project's local backlog store
is not, and per the non-goals should not become, an independently addressable system with its own
public API — which is the one thing every SaaS two-way sync tool requires of both sides.

---

## 3. LLM-generated loop-prevention logic vs. established pattern

**The problem** (requirements AC7): a backlog item closed by *our own* forward sync must not be
immediately reprocessed by backward sync as if it were an external change, or if it is reprocessed,
the reprocessing must be idempotent and not thrash status.

**Established patterns surveyed**:
- **Origin tagging / "who changed it last"** — the pattern independently used by every
  bidirectional sync tool found in §1 (Unito, Exalate) and already present in *this exact codebase*
  as `TriggeredByUser`/`TriggeredBySystem` (`session/backlog.go:92-93`), recorded per-transition on
  `BacklogStatusEvent`. The established technique: before writing to the remote side, record that
  *this specific write* originated locally (a marker/timestamp/hash); when the next poll observes
  that same remote state, check the marker first and skip re-applying it locally if it matches a
  write this system itself just made.
- **Last-write-wins with a version/timestamp check** (CRDT-adjacent, but full CRDT machinery is not
  warranted here — see below) — compare the remote `updated_at` (already fetched and used as the
  sync cursor, `newCursor` in `Fetch`, `session/backlog_plugin_github.go:130,157-158`) against the
  timestamp of the last forward-sync write; if the remote timestamp is not newer than what our own
  write produced, treat it as "our own echo," not an external change.
- **Event sourcing** (full event log as source of truth, state derived by replay) — evaluated and
  rejected as overkill: `BacklogStatusEvent` already *is* an append-only event log for status
  transitions specifically (`session/repository.go:316`-adjacent, `storage_backlog.go`), but full
  event-sourcing (deriving current state purely by replaying all events, no mutable snapshot) would
  be a much larger architectural change than this feature warrants — the existing schema already
  keeps a mutable `BacklogItem.Status` snapshot plus an event log for audit, which is sufficient.

**Why not "ad hoc" LLM-improvised logic** (e.g. a bespoke `lastSyncedAt` field compared loosely
against `time.Now()`, or a boolean `syncedByUs` flag that's never cleared): the established
pattern's key property — recording the *specific write's* fingerprint (timestamp, or the exact
value written) rather than a coarse "did we touch this recently" flag — is what prevents two known
failure modes an ad hoc flag misses: (a) a user makes a genuine external GitHub change within the
same poll window as our own forward-sync write (a coarse timer-based debounce would wrongly
swallow it), and (b) a flag that's set but never precisely compared against the *value* written
can't tell "GitHub now shows exactly what we wrote" from "GitHub changed again, coincidentally to
a similar state" — silently absorbing a second, unrelated external change.

**Recommended pattern (concrete)**: extend the *existing* `TriggeredBy` origin-tag idiom rather than
inventing a parallel mechanism:
1. When forward sync closes/labels an issue, record the exact remote `updated_at` (or the GitHub
   API response's own timestamp) the write produced, alongside the existing
   `TriggeredBySystem`-tagged `BacklogStatusEvent` this project's backward-sync logic will also
   need to write for GitHub-driven transitions.
2. In backward sync (`SyncLoop.SyncOne`'s existing-item branch, `session/backlog_sync.go:259-291`),
   before applying a remote-observed close/label change, compare the fetched issue's `updated_at`
   against the last-known-forward-sync-write timestamp for that item; if they match (our own echo),
   skip applying it — this mirrors the already-proven `UserModifiedFields` local-wins gate
   structurally (same file, same function, same "check a marker before writing" shape) rather than
   introducing a second, differently-shaped guard.

**Verdict: Recommended — follow the established origin-tag + remote-timestamp-comparison pattern,
implemented as a natural extension of the existing `TriggeredByUser`/`TriggeredBySystem` +
`BacklogStatusEvent` mechanism already in `session/backlog.go` and `session/storage_backlog.go`.**
Do not invent a new ad hoc dedup mechanism; do not reach for full event-sourcing/CRDT machinery —
both are either already partially present (origin tags) or disproportionate (event sourcing) for a
single-writer-per-side, poll-interval-bounded sync loop.

---

## 4. Fork/adapt: sibling `github_prs` plugin and `backlog-github-issue-link`

**`project_plans/backlog-github-issue-link/`** (fully planned, unimplemented — requirements,
research, `implementation/plan.md`, and `decisions/ADR-001-external-url-backfill-and-prompt-boundary.md`
all exist, verified read in full):

- Designs exactly the `ExternalURL *string`-on-`BacklogItemUpdate` / plain-`string`-on-
  `BacklogItemData` pattern this project's requirements doc (line 33-41) says to reuse. Read in
  full; the two decisions worth carrying forward verbatim:
  - **ADR-001 Decision 1**: any unconditional backfill/sync field (this project's `Labels`, and the
    GitHub-driven `Status` backward-sync) must set its own `anyField = true` *structurally separate*
    from the three existing `UserModifiedFields`-gated blocks in `SyncOne`
    (`session/backlog_sync.go:265-276`) — never folded into the same gated-block idiom, or a future
    refactor can silently make the whole update a no-op for user-locked items. This directly
    generalizes to backward-sync's status/label writes, which per this project's AC4 must *also*
    respect `UserModifiedFields` local-wins (the opposite backfill's unconditional behavior needs to
    be told apart from status/label sync's conditional behavior in the same function).
  - **ADR-001 Decision 3** (the corrected-during-validation one): don't assume an external API's
    documented-looking behavior works as expected — it verified GitHub's actual closing-keyword
    syntax against GitHub's own docs rather than trusting a plausible-looking string. The
    equivalent risk in this project: verify GitHub's actual `PATCH .../issues/{number}` behavior
    for `state`/`labels` fields (partial vs. full replace semantics, particularly for labels — GitHub's
    labels endpoint fully replaces the label set by default) directly against GitHub's REST API docs
    before implementation, not by assumption.
- The rejected alternatives in ADR-001 (a bespoke `BackfillExternalURL` method; a direct ent-client
  bypass of `BacklogItemUpdate`) apply identically to a hypothetical bespoke `SyncGitHubLabels`
  method for this project — same rejection reasoning (duplicates `UpdateOneID`/`Save` boilerplate,
  breaks the `SyncOne`-only-talks-to-`Storage` architectural seam).

**`GitHubPRsPlugin`** (`session/backlog_plugin_github_prs.go`, sibling to `GitHubIssuesPlugin` in
the same package):

- Already computes `Labels` as a return value of `MapToBacklogItem` today (`computeLabels`, line
  141 onward) — but derives them **one-way**, from remote PR state into local display labels; it
  has no closing/backward-status-sync logic to fork, since PRs are explicitly out of scope for this
  project (requirements' non-goals list). Still directly useful as **the existing convention for
  how `Labels []string` should be computed and shaped** on `ExternalItem`/`BacklogItemData` — this
  project's `GitHubIssuesPlugin.MapToBacklogItem` (`session/backlog_plugin_github.go:166-185`)
  should carry `item.Labels` (already populated in `Fetch`, line 151, but dropped in
  `MapToBacklogItem`) using the identical field shape `GitHubPRsPlugin` already established, not a
  parallel or differently-typed labels representation.

**Verdict: Recommended — extend, don't reinvent.** Both the `ExternalURL` struct/gating pattern
from `backlog-github-issue-link`'s ADR-001 and `GitHubPRsPlugin`'s existing `Labels []string`
shape are the two concrete precedents to extend for this project's `Labels` field and its
backward-sync writes. The planning phase for this project should cite ADR-001 directly (as the
requirements doc already instructs) rather than re-deriving the same struct-based
`BacklogItemData`/`BacklogItemUpdate`/ent-schema-field approach from scratch.

---

## Summary table

| Option | Verdict |
|---|---|
| 1. OSS library for bidirectional GitHub issue sync (Go lib or reusable service) | **Not recommended** — none exists for this problem shape; even the broader GitHub↔Jira ecosystem has no OSS two-way library, only hosted platforms |
| 2. SaaS/managed (Unito, Exalate, Zapier, GitHub Apps marketplace) | **Not recommended** — requires making the local backlog store an addressable system with a public API/webhook surface, which contradicts the project's poll-based, single-user, non-goal-excludes-webhooks design |
| 3. Loop-prevention: ad hoc vs. established pattern | **Recommended: established pattern** — origin-tag (`TriggeredByUser`/`TriggeredBySystem`, already in `session/backlog.go:92-93`) + remote-timestamp comparison, extended rather than a new ad hoc dedup flag; full event-sourcing/CRDT rejected as disproportionate |
| 4. Fork/adapt sibling `backlog-github-issue-link` (ADR-001) + `GitHubPRsPlugin` `Labels` shape | **Recommended: extend, don't reinvent** — reuse `BacklogItemData`/`BacklogItemUpdate`/ent-schema-field pattern and gating rules from ADR-001, and `GitHubPRsPlugin`'s existing `Labels []string` shape |

**Overall**: this feature is a build, using stdlib `net/http` (matching the codebase's existing,
deliberate no-GitHub-SDK convention) for the one new outbound write path (issue close/label PATCH),
extending two already-established local patterns (ADR-001's struct/gating design,
`TriggeredBy`-style origin tagging) rather than introducing new abstractions or an external
dependency.
