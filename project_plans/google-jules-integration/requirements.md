# Requirements: google-jules-integration

**Date**: 2026-08-31
**Type**: feature addition (new external integration)
**Complexity**: 3 — system design

## Problem Statement
stapler-squad manages AI coding agent sessions (Claude Code, Aider) as local
tmux-backed processes in git worktrees. Google Jules is a cloud-hosted coding
agent — its own per-task VM, reachable via a public (alpha) REST API
(`Sources`/`Sessions`/`Activities`, see
[developers.google.com/jules/api](https://developers.google.com/jules/api)) —
that Google is positioning for third-party integrations (Slack, Linear,
GitHub). Today there is no way to create, track, or review Jules work from
inside stapler-squad's backlog/session pipeline; a user who wants
cloud-offloaded execution alongside local agents has to leave the tool
entirely.

`docs/jules-feature-adoption.md` (PR #174) already analyzed Jules but only to
borrow its *UX ideas* (plan-gate, PR-summary card, activity feed) for local
agents — it explicitly did not integrate with the real Jules product. This is
a different, new piece of scope.

## Baseline
A user who wants Jules to work a task today must go to jules.google.com or
Jules' GitHub App, create the task by hand, and track/review the resulting PR
outside stapler-squad — no unified session list, no concurrency throttling,
none of the backlog pipeline (plan gate, status events, PR review flow) that
local-agent sessions already get.

## Users / Consumers
Existing stapler-squad operators (solo dev / small team) already using the
backlog + session pipeline for Claude Code/Aider. Any PR Jules opens should
flow through the same GitHub review path (`WorktreePRPoller`) other agents'
PRs already use.

## Success Metrics
- A user can create a Jules-backed unit of work from stapler-squad's backlog
  UI without visiting jules.google.com.
- That work's status and resulting PR converge into the backlog UI's item
  detail page (`SessionsSection`, `PullRequestSection`) as they do for
  local-agent sessions, and the PR is pollable via the existing PR review
  path, not a separate surface — measured by: a Jules session's status is
  visible on the backlog item detail page, and its PR flows through
  `ReconcilePRPending` unmodified.
  - **Narrowed during Phase 4 plan repair** (pre-mortem P1 #2) from the
    original wording "appears in `list_sessions`/backlog UI": `SessionService.ListSessions`
    (`server/services/session_service.go:1697-1710`) is built entirely from
    `session.Instance` objects, and ADR-001 deliberately never creates one for
    a Jules session (no tmux, no PTY to attach `list_sessions`/MCP semantics
    to — see ADR-001's Consequences). A Jules session therefore cannot appear
    in `list_sessions` or the `mcp__stapler-squad__list_sessions` tool under
    this architecture. Accepted as a tradeoff rather than a gap to close in
    MVP: the backlog item detail page is the surface headless (triage)
    sessions already use for the same reason, and `list_sessions`/MCP
    visibility can be added later (e.g. a synthetic, non-`Instance`-backed
    entry) without another architectural change if it turns out to matter in
    practice.

## Appetite
Medium (1–2 weeks) for an MVP slice. Full bidirectional session control
(steering mid-task via the Activities API) is Large and is explicitly cut
from MVP scope below — see Rabbit Holes.

## Constraints
- Jules REST API is **alpha** — Google states specs/keys/definitions may
  change. Any adapter must be isolated (single package) so churn doesn't
  ripple into `session/` core.
- Requires a per-user Jules API key (from jules.google.com/settings) —
  repo's existing secret-storage pattern for such a key needs confirming in
  research (is there a per-user secret store today, or only global config?).

## Non-functional Requirements
- **Performance SLO**: not applicable — polling cadence only, no latency-sensitive path.
- **Scalability**: not applicable at current single-user/small-team scale.
- **Security classification**: confidential — a Jules API key is a credential; must not be logged, must follow whatever pattern GitHub App/OAuth credentials already use in this repo.
- **Data residency**: unlike local agents, Jules executes on Google's cloud VM — the user's source code leaves the local machine. This is a material behavior difference from Claude Code/Aider and must be surfaced to the user (opt-in, not silent), not a hard compliance requirement.

## Scope
### In Scope
- Research phase determines the integration shape (see Open Questions) — this
  requirements doc intentionally leaves that decision to Phase 2/3, per the
  user's explicit direction to "look into what would work best given what's
  already implemented."
- At minimum: an adapter to the Jules REST API, a way for a backlog item to
  result in Jules doing the work, status of that work visible in the
  existing backlog/session UI, and the resulting PR flowing through the
  existing PR review path.

### Out of Scope
- Replicating Jules' own chat/UI.
- Real-time interactive steering of a running Jules session (send-message
  loop) — MVP is fire-and-forget create + poll.
- Building a competing local cloud-VM sandbox for Claude Code/Aider —
  already explicitly rejected in `docs/jules-feature-adoption.md`.
- Multi-tenant / team Jules account management.

## Rabbit Holes
- **Alpha API instability** — Google may change the REST surface under us.
- **Source model mismatch** — Jules' `Sources` resource is described as a
  GitHub repo; unclear whether it can target an arbitrary local git worktree
  the way other stapler-squad agents do, or only a pushed branch. If only
  pushed branches, Jules sessions follow a fundamentally different flow than
  local-worktree-first agents.
- **Session abstraction assumption** — `session/ent/schema` and
  `IsTmuxBackedSessionRole`-style logic may assume every session is a local
  tmux pane. Adding a non-tmux-backed session kind may require more
  refactoring than it looks like at first glance.
- **Credential storage** — where a per-user Jules API key lives needs to fit
  an existing pattern, not invent a new secrets mechanism.

## Alternatives Considered
- **(a) No code integration** — just deep-link to jules.google.com. Rejected: doesn't unify the backlog, doesn't meet the success metric.
- **(b) Thin: PR-import only** — let Jules' own GitHub App create PRs exactly as it does today; stapler-squad only imports the resulting PR into the backlog/review path, reusing the existing GitHub-issue-import pattern. No new session-creation flow, no REST API adapter needed for creation, only for optional status enrichment.
- **(c) Full: API-driven session backend** — stapler-squad creates/polls Jules sessions directly via the REST API, tracked as a first-class session type alongside Claude Code/Aider.
- Research/plan phases must pick between (b) and (c), or recommend (b) as a phased first step toward (c).

## Feasibility Risks
- Alpha API instability and undocumented rate limits/pricing.
- Unclear whether Jules can operate against a stapler-squad-managed local
  worktree at all (see Rabbit Holes) — if not, the "session" it creates is
  conceptually closer to a remote PR-producing job than a local agent
  session, which changes the right data model.

## Observability Requirements
Log Jules API create/poll calls and errors distinctly, following the
existing logging conventions (`docs/how-to/debug-with-logs.md`). No new
oncall alert — this is a best-effort, opt-in feature at this scale.

## Risk Control
Opt-in by construction: the feature only activates once a user configures a
Jules API key, so there's no flag needed beyond "key configured or not."
Rollback is trivial — stop creating Jules sessions; existing local-agent
sessions are unaffected since this is additive.

## Open Questions

Resolved by Phase 2 research (see `research/*.md`):
- **Worktree targeting** — Jules only operates on a pre-registered GitHub `Source` + already-pushed branch, never an arbitrary local worktree (`research/stack.md`, `research/architecture.md`). This rules out literal option (b) as a dispatch mechanism (it can only import PRs Jules already opened elsewhere) and settles the real MVP shape: dispatch a session to a pushed branch, then let the resulting PR converge through the existing review path.
- **Credential storage** — repo already has a fitting pattern: the `CredentialChain`/keychain approach used for GitHub tokens (`github/keychain.go`), not the weaker AES-in-config path used for Slack webhooks (`research/pitfalls.md`, `research/build-vs-buy.md`).
- **Data-model fit** — no new entity or schema migration needed. `ItemSession` already supports non-tmux-backed rows (headless triage); a Jules-backed session just needs a new `SessionRole` excluded from `IsTmuxBackedSessionRole` (`research/architecture.md`).
- **Activities API / steering** — polling-only, no steering endpoint documented; fire-and-forget create+poll is sufficient for MVP as originally scoped (`research/stack.md`).

*(unresolved after Phase 2 research)* Exact Jules API rate-limit and pricing figures were not pinned down from public docs — `research/build-vs-buy.md` and `research/pitfalls.md` confirm there's no existing proactive spend-cap primitive in this repo (closest is `MaxConcurrentBacklogWorkItems`), which the plan should extend defensively rather than block on discovering Google's exact quota numbers.
