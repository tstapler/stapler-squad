# Requirements: Linear and JIRA Issue Integration

Source: backlog item `f8572715-8a80-490e-995b-d492b96e18fd`, migrated from
[TylerStaplerAtFanatics/stapler-squad#51](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/51).
Non-interactive requirements pass (no user present to interview) — derived
from the item description plus the existing GitHub Issues integration this
extends.

## Problem

stapler-squad backlog items can be sourced from GitHub Issues today
(`session/backlog_plugin_github.go`'s `GitHubIssuesPlugin`, registered via
`session.NewDefaultRegistry()`), including forward-sync back to GitHub
(`CloseIssue`, `PostIssueComment`). Teams that use Linear or JIRA instead of
GitHub Issues have no equivalent path — they can't pull tickets in as backlog
items, and there's no write-back when a session completes.

## Correction to the source item

The original issue's proposed UX (`ssq issue <url>` CLI subcommand) does not
match this codebase. There is no `ssq issue` command anywhere in `cmd/`.
Issue import happens today via the `import_github_issue` MCP tool
(`server/mcp/tools_backlog.go`) and via the `ItemSourcePlugin` polling/sync
loop (`session/backlog_sync.go`), not a CLI verb. This plan follows the
existing plugin architecture instead of inventing a CLI surface — see
`research/existing-patterns.md` for the full comparison once written.

## Target user and evidence

The originating request (GitHub issue #51, migrated 2026-04-09, labeled `p1`)
is itself the only concrete evidence of demand available in this repo — there
is no ticket count, quoted user, or usage log showing anyone has hit this gap
in practice. Named target user: **a stapler-squad operator whose team's
tracker of record is Linear or JIRA rather than GitHub Issues** — the same
persona the existing `GitHubIssuesPlugin` already serves, just on a different
tracker. This is a real but unvalidated assumption; see Risky Assumption
below.

**Risky assumption**: that a stapler-squad operator whose team is on
Linear/JIRA will actually configure an `ItemSource` for it once built, rather
than continuing to paste ticket context into a session prompt by hand (the
zero-integration-cost status quo). If nobody configures a Linear/JIRA source
post-ship, the two new plugins are dead code with a maintenance cost (API
surface drift, credential-handling security surface) and no offsetting value.

**Success metrics** (falsifiable, checkable without new instrumentation):
- At least one `ItemSource` with `plugin_id` `linear` or `jira` exists in a
  real (non-test) `~/.stapler-squad/` config within 60 days of shipping —
  checkable via `config.json`/backlog storage inspection, no new telemetry
  required.
- Zero regressions in the existing GitHub forward-sync path post-ship
  (pre-mortem P1 #2's regression gate is the direct check for this).
- If neither Linear nor JIRA gains a configured `ItemSource` within 60 days,
  treat this as evidence the risky assumption above was wrong and do not
  invest further (e.g. Asana/Trello) without new demand evidence.

## Goals

1. Linear and JIRA are sourceable as backlog items through the same
   `ItemSourcePlugin` mechanism GitHub Issues uses today — polling sync loop,
   not a new CLI command.
2. A single external issue (Linear or JIRA) can be imported one-off via an
   MCP tool, mirroring `import_github_issue`.
3. On session completion, the originating Linear issue's state or JIRA
   issue's status can optionally be updated (mirrors `CloseIssue` +
   `PostIssueComment` forward-sync for GitHub).
4. Credentials follow the existing per-host keychain pattern
   (`github.GetKeychainTokenForHost`) rather than requiring users to hand-edit
   JSON config with raw secrets, and rather than introducing a differently
   shaped auth story from GitHub.
5. Dashboard: backlog items sourced from Linear/JIRA show a source badge and
   are filterable by source, and the external issue URL is stored as item
   metadata and clickable — matching what GitHub-sourced items already get
   via `ExternalURL`.

## Non-goals

- Two-way *field* sync (title/description edits flowing bidirectionally) —
  out of scope, matches GitHub's own scope (GitHub is fetch + one-way
  status/comment forward-sync only, see `backlog_github_forward_sync.go`).
- Building a generic "pluggable issue tracker" abstraction beyond what
  `ItemSourcePlugin` already provides — the interface already generalizes
  across trackers; this is "implement two more plugins," not a new
  abstraction layer.
- Any tracker beyond Linear and JIRA (e.g. Asana, Trello) — not requested.
- Real-time webhooks — the existing sync loop is poll-based; matching GitHub's
  model (poll interval, cursor-based incremental fetch) is sufficient unless
  research turns up a hard blocker.

## Acceptance Criteria (initial — refined further in plan.md)

1. A `LinearPlugin` implementing `session.ItemSourcePlugin` fetches Linear
   issues via the Linear GraphQL API, maps them to `BacklogItemData`, and is
   registered in `session.NewDefaultRegistry()`.
2. A `JiraPlugin` implementing `session.ItemSourcePlugin` fetches JIRA issues
   via the JIRA REST API, maps them to `BacklogItemData`, and is registered
   in `session.NewDefaultRegistry()`.
3. Both plugins support incremental fetch via a cursor (matching the
   `updated_at`-cursor pattern `convertGithubIssues` uses) so the sync loop
   doesn't refetch the whole tracker every tick.
4. An MCP tool (or tools) analogous to `import_github_issue` exists for
   one-off Linear/JIRA issue import into the backlog.
5. On session completion, an optional forward-sync action updates the source
   Linear issue's state or JIRA issue's status, gated by the same
   "no-silent-automated-action" convention GitHub's forward-sync follows
   (visible comment posted before/with the state change).
6. Credentials (`LINEAR_API_KEY`; `JIRA_BASE_URL` / `JIRA_EMAIL` /
   `JIRA_API_TOKEN`) are read from the existing keychain/secrets mechanism,
   not committed to `PluginConfig.Raw` JSON in plaintext.
7. Backlog items sourced from Linear or JIRA display a source badge in the
   web UI and are filterable by source; the external issue URL is stored and
   clickable, consistent with GitHub-sourced items.
8. New plugins have unit test coverage matching
   `backlog_plugin_github_test.go`'s pattern (empty-token no-op, missing
   required config errors, priority/label mapping, cursor advancement).
9. `docs/registry/features/backend/*.json` gains entries for any new RPC/MCP
   surface per `.claude/rules/feature-registry.md`.

## Constraints / conventions this must follow

- `.claude/rules/interface-pollution-checklist.md` — no speculative
  `IssueTrackerPlugin` mega-interface; `ItemSourcePlugin` already is the
  right-sized abstraction, reuse it as-is.
- `.claude/rules/prefer-go-git-over-subshells.md` — n/a (no git operations
  here), noted only because it's a standing repo convention for new Go code.
- `.claude/rules/feature-registry.md` — any new RPC/MCP tool needs a
  per-feature registry file + e2e test.
- ADR-003's loop-prevention watermark pattern (referenced in
  `backlog_plugin_github.go`'s `CloseIssue` doc comment) must be replicated:
  use the tracker's own post-write timestamp for the watermark, not local
  wall-clock time, to avoid re-importing an item the forward-sync itself just
  changed.
