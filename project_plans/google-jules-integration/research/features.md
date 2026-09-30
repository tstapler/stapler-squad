# Feature Research: Google Jules Integration

Agent 2 (Features), SDD Phase 2 research for `google-jules-integration`.

## 1. Industry patterns for cloud/async coding agents

Google Jules' own REST API (alpha, `jules.googleapis.com/v1alpha`) is
resource-shaped as `Sources` / `Sessions` / `Activities`, auth via
`X-Goog-Api-Key` (max 3 active keys/user, auto-disabled if leaked) — [API
Reference](https://developers.google.com/jules/api),
[Sessions](https://jules.google/docs/api/reference/sessions/),
[Sources](https://jules.google/docs/api/reference/sources/). Critically,
**`sessions.create` requires `sourceContext.githubRepoContext.startingBranch`**
— Jules only ever operates against a GitHub-hosted branch on a connected
`Source` (a GitHub repo), never an arbitrary local path. This directly
answers requirements.md's top Open Question: **Jules cannot target a
stapler-squad-managed local worktree.** A branch must already be pushed to
GitHub before a Jules session can start on it. `requirePlanApproval` exists
as a session-creation flag (mirrors the plan-gate stapler-squad already
ships for local agents, per `docs/jules-feature-adoption.md`).

Competing async coding agents converge on the same two-phase pattern —
**push-trigger, pull-status**:

- **OpenAI Codex cloud** (Slack/Linear integrations) — `@Codex` mention or
  "Delegate" on a Linear issue creates a cloud task against a
  GitHub-backed repo + configured cloud environment; Codex replies on the
  originating issue/thread with progress and a link to the completed task.
  ([Codex Slack/Linear](https://codex.danielvaughan.com/2026/03/27/codex-slack-linear-cloud-tasks/))
- **Devin** — a session can bind to a Slack thread bidirectionally (webapp
  messages appear in-thread, thread replies reach Devin); Slack's "code
  channel" shows a status chip (working/blocked/done), a link back to the
  session, and chips/tabs for opened PRs — i.e. a session-centric UX
  surfaced *inside* the collaboration tool rather than requiring a visit to
  the agent's own UI. ([Devin/Slack docs](https://docs.devin.ai/integrations/slack))

Common shape across all three: **creation is push (host tool → agent API),
status/result is pull-back-into-host** (webhook where available, otherwise
polling), and the *artifact* that lands in the host tool is either a PR link
or a PR that flows through the host's own normal review path — none of them
replace the host's PR review UI with the agent's own. This validates
requirements.md's framing: regardless of (b) vs (c), the *destination* for a
Jules-produced PR should be stapler-squad's existing `WorktreePRPoller`
review path, not a new Jules-specific review surface.

No public evidence of a *webhook* push from Jules back to third-party tools
was found in this pass (Jules' own examples show Slack/Linear/GitHub App
integrations, but the public REST API docs describe pull-only `Sessions`/
`Activities` reads) — polling is the safe assumption for MVP, matching this
codebase's existing `WorktreePRPoller`/`PRStatusPoller` cadence-based design
and requirements.md's own NFR ("polling cadence only").

## 2. Fit against this codebase's existing extension points

### `ItemSourcePlugin` (option b — thin PR-import) is the right existing seam, with one caveat

`session/backlog_plugin.go`'s `ItemSourcePlugin` interface
(`PluginID()`, `Fetch(ctx, config, cursor) ([]ExternalItem, newCursor, err)`,
`MapToBacklogItem(item, sourceID) BacklogItemData`) is exactly the
abstraction `project_plans/linear-jira-integration/research/architecture.md`
independently recommends reusing for Linear/JIRA, and it fits Jules'
"import the PR Jules already opened" direction just as cleanly:

- **`GitHubPRsPlugin`** (`session/backlog_plugin_github_prs.go`) is the
  concrete template, and it is the *better* template than
  `WorktreePRPoller` for this use case specifically because
  `GitHubPRsPlugin.Fetch` queries GitHub's Pulls API directly by
  `owner`/`repo` — it has **no dependency on a local worktree existing**.
  This directly resolves requirements.md's Rabbit Hole "Jules opens a PR
  against a branch stapler-squad doesn't know about": a
  `JulesPRsPlugin` (or reusing `GitHubPRsPlugin` itself, since a
  Jules-opened PR is a completely ordinary GitHub PR authored via Jules'
  own GitHub App) polling by owner/repo would surface it regardless of
  whether any local worktree/branch is known.
- By contrast, `WorktreePRPoller` (`session/worktree_pr_poller.go`) is keyed
  off `WorktreeSource.GetWorktrees()` — i.e. it only polls PRs for branches
  stapler-squad already has a local worktree for. **A Jules-opened PR
  against a branch with no local worktree is invisible to
  `WorktreePRPoller` by construction.** This is a real architectural
  mismatch for option (c) if Jules sessions aren't given a corresponding
  local worktree — see §2's option (c) discussion below.
- Net effect: **option (b) may not even need a new plugin file.** If Jules'
  GitHub App pushes PRs to the same repo already registered as a
  `github_prs` `ItemSource`, the *existing* `GitHubPRsPlugin` picks them up
  today with zero new code — the "integration" work in that case is almost
  entirely UI (surfacing "opened by Jules" provenance) rather than backend.
  Confirm in Phase 3 whether visually distinguishing a Jules-authored PR
  from a human/other-agent PR matters for the MVP (likely yes, per §4).

### Registry wiring (unchanged either way)

`session.NewDefaultRegistry()` (`session/backlog_plugin.go`) is the single
registration point; `server/dependencies.go` wires one shared
`*PluginRegistry` into both the periodic `SyncLoop` and the manual-trigger
`BacklogService`. A `JulesPRsPlugin` (if a distinct one is warranted, e.g.
to also enrich items with Jules session status via the REST API — see
below) adds one `r.Register(...)` line and needs no other call-site change,
same as Linear/JIRA.

### What's missing from `ItemSourcePlugin` for option (c) — it's a pull interface, Jules-creation is a push

`ItemSourcePlugin.Fetch` is explicitly a **pull** shape: "give me what's new
since this cursor." There is no method on the interface, nor an analogous
one elsewhere in `session/`, for **"create a unit of remote work"** — the
closest existing precedent is `server/mcp/tools_backlog.go`'s
`importGitHubIssue` (a *one-off pull* — fetch one existing external item by
URL), not a push/create. Concretely, option (c) needs a new capability this
plugin ecosystem doesn't have any shape for yet:

```go
// Not in ItemSourcePlugin today — would need to be new, e.g. a
// consumer-defined interface at the session-spawn call site, per this
// repo's interface-pollution-checklist convention (define narrow,
// where consumed — not on the base ItemSourcePlugin).
type RemoteSessionCreator interface {
    CreateSession(ctx context.Context, config PluginConfig, branch, prompt string) (remoteSessionID string, err error)
    GetSessionStatus(ctx context.Context, config PluginConfig, remoteSessionID string) (status ExternalSessionStatus, err error)
}
```

This is a **materially different integration point** than the
sync-loop/forward-sync machinery `architecture.md` describes for
Linear/JIRA — those are about *closing/transitioning* an item that
originated externally; Jules-as-session-backend is about *spawning* work
that originates *in stapler-squad*. The natural analogy in this codebase
isn't the plugin registry at all — it's the **session-spawn path**
(`BacklogController.SpawnSessionFromItem`-equivalent, wherever local
tmux/worktree session creation is invoked today) needing a second, non-tmux
branch. See §2's data-model note below.

### `ItemSession`/session-role model assumes tmux-backed by default — confirmed gap

`session/backlog.go`'s `IsTmuxBackedSessionRole` gates purely on
`session_role` (`work`/`triage`/`review`), returning true for `work` and
`review` — there is **no existing "backend" or "kind" field** anywhere in
`session/ent/schema/item_session.go` distinguishing a local-tmux session
from a remote one. Every `ItemSession` field (`session_uuid`,
`last_commit_sha`/`base_commit_sha` re-read from **worktree HEAD** on every
reconciliation tick, `claimant_host_id` tied to *this* physical
process/host) assumes a live local worktree with a live tmux pane. This
confirms requirements.md's Rabbit Hole: **a Jules-backed "session" cannot be
shoehorned into `ItemSession` as-is** — `refreshWorkSessionGitActivity`-style
logic that re-reads worktree HEAD would need either a no-op branch for
remote sessions (poll via the Jules API instead) or a parallel, thinner data
shape. This is squarely a Phase 3 (architecture) decision, but the
feature-level implication is: **option (c)'s MVP-safest shape is "Jules
session with no local worktree at all, tracked as a lightweight record that
converges into a normal PR-import once Jules pushes a branch"** — i.e. (c)
degrades to (b) at the point a PR exists, and the only genuinely new surface
is the pre-PR "Jules is working" status view (via `Activities`), which per
requirements.md's own Rabbit Holes/Out-of-Scope is explicitly **not** MVP
(no mid-session steering). This makes a strong case for recommending **(b)
now, (c)'s create-only half as a fast-follow**, matching requirements.md's
own suggested phased framing.

## 3. Edge cases and failure modes

| Case | Existing precedent to mirror | Gap to close |
|---|---|---|
| **API key expired/invalid** | `github/client.go`'s `isGHRateLimited`/auth-vs-rate-limit distinction (403 with no rate-limit signal = unauthenticated, treated as "source disabled," not a hard error — see `decodeGithubIssuesFetchConfig`'s doc comment pattern in `architecture.md` §1) | Jules adapter needs the equivalent: a 401/403 from `jules.googleapis.com` on `Fetch`/`CreateSession` should degrade to "feature inactive, log once" not a repeated hard error every poll cycle. Since this is opt-in-by-key (per requirements.md's Risk Control), an invalid key is the *expected* steady state for most installs — must not spam logs. |
| **Jules session fails/times out** | No local precedent — local agent "failure" is a crashed/hung tmux process, detected via `EventExited`/orphan-recovery sweeps (`reconcileOrphanedTriageItems`) | A Jules session has no local process to crash-detect; failure is only observable via polling `Sessions`/`Activities` status. Needs a bounded max-poll-duration + terminal-failure state so a stuck/abandoned Jules session doesn't leave a backlog item in permanent "in progress" limbo the way a local session's tmux death would at least be detectable. |
| **PR opened against a branch stapler-squad doesn't know about** | `GitHubPRsPlugin` (queries by owner/repo, not by known worktree) vs. `WorktreePRPoller` (queries by known worktree only) — see §2 | Confirmed real: this is exactly why `GitHubPRsPlugin`, not `WorktreePRPoller`, is the correct import mechanism for Jules PRs. |
| **Duplicate task creation** (user creates the same task twice, once via stapler-squad's UI and once directly on jules.google.com, or double-clicks "send to Jules") | Polling-sync dedup is by `ExternalID` diff in `SyncLoop.SyncOne` (`architecture.md` §3) — but that's a *pull*-side dedup, only catches duplicates once both PRs exist and get imported. Agent-side `report_duplicate` MCP tool is for a *local agent* self-reporting mid-task, not applicable to Jules (no local agent process to call it). | A push-side (option c) create call needs its own guard — e.g. refuse to create a second Jules session for a `BacklogItem` that already has an active/unresolved Jules session recorded, checked before the API call, not after. Option (b)-only (no create path) sidesteps this entirely — another point favoring (b) first. |
| **Rate limiting** | `github/rate_limit.go`'s `DefaultRateLimiter` (`IsLimited()`/`Update()` fed by every response's headers, checked pre-emptively before firing another guaranteed-to-fail request) | Jules' alpha API rate limits are undocumented publicly (open question, correctly flagged in requirements.md). Mirror the same defensive shape — track 429/Retry-After from responses, back off, and expose `IsLimited()`-equivalent so the sync loop skips a poll cycle instead of guaranteeing another failure. |
| **Data residency / source leaves the machine** | No existing precedent — local agents never send code off-box | Requirements.md already flags this correctly as "opt-in, surfaced, not silent" (NFR). Feature-level implication: the UI action that creates a Jules session needs an explicit, undismissable-by-default confirmation (not just a settings toggle buried once) the first time, similar in spirit to how the plan-gate UI surfaces an explicit approval step rather than defaulting through. |

## 4. Unstated user needs

- **Provenance/attribution in the UI, not just data-model plumbing.**
  `architecture.md` §7 documents that `BacklogItemCard.tsx` and
  `SourceSection.tsx` hardcode a GitHub-issue-shaped badge (`CircleDot`
  icon, "Imported from GitHub issue #N"). Whatever path Jules PRs take (b or
  c), a user will want to visually tell "this PR came from Jules" apart from
  "this PR came from my local Claude Code session" or "this PR came from a
  human" at a glance in the session/backlog list — not just by clicking in.
  This generalization work is already flagged as needed for Linear/JIRA;
  Jules should piggyback on the same generalized badge-config-by-plugin-ID
  mechanism rather than adding a third hardcoded special case.
- **Cost/quality comparison against local agents is a real latent want, not
  explicit in requirements.md.** The requirements doc's "Appetite" section
  cuts mid-session steering and doesn't mention analytics, but a user
  choosing *when* to route a task to Jules vs. a local Claude Code/Aider
  session will naturally want to know "was this worth it" after the fact —
  at minimum, whether Jules produced a mergeable PR without human
  rework, cross-referenced against `estimated_cost_usd`
  (`ItemSession.estimated_cost_usd`, currently populated "for headless
  sessions from claude -p output" per its schema comment) for local
  sessions. Not MVP, but worth a forward-looking data-model note: if a
  Jules `ItemSession`-equivalent doesn't at least record *something*
  analogous to cost/duration/outcome, that comparison becomes unanswerable
  later without a backfill. Flag as a non-blocking Phase 3 consideration,
  not a requirement to add now.
- **Fallback when Jules is down/rate-limited/erroring.** Requirements.md's
  Risk Control section treats "stop creating Jules sessions" as sufficient
  rollback, which is right for a global kill switch, but doesn't address the
  *per-task* moment: if a user asks to route one specific backlog item to
  Jules and creation fails (key invalid, rate limited, alpha API 500), the
  expected fallback is almost certainly "fall through to a normal local
  agent session for this item," mirroring how this repo already treats a
  missing/invalid credential as "source disabled" rather than a hard block
  elsewhere (§3 table, row 1). Whether that fallback is automatic or
  requires the user to explicitly retry as a local session is a UX call for
  Phase 3, but the failure path should not be a dead end.
- **Visibility that a session is remote, not local, once it's running.**
  Given `ItemSession`'s local-worktree assumptions (§2), if a lightweight
  Jules-backed record does get added, users will expect the same signals
  local sessions already give (last-activity timestamp, running/stuck
  detection) even though the underlying mechanism (poll vs. tmux-attach) is
  completely different — the existing UI shouldn't silently show a Jules
  session as if it were an idle/stuck local session using the same status
  vocabulary, since the recovery action ("check tmux pane") wouldn't apply.

## 5. Summary recommendation for Phase 3

- **(b) thin PR-import is the low-risk, high-confidence MVP slice** — it
  reuses `ItemSourcePlugin`/`GitHubPRsPlugin` almost as-is (possibly
  zero new backend code if Jules PRs land in an already-registered repo;
  otherwise a thin `JulesPRsPlugin` variant), sidesteps the `ItemSession`
  local-worktree-assumption problem entirely (§2), and directly satisfies
  requirements.md's stated success metric ("PR pollable via the existing PR
  review path"). Main net-new work is frontend provenance generalization
  (§4), which is needed for Linear/JIRA anyway and should be built once,
  shared.
- **(c) full API-driven creation is real, separable follow-on work**, not
  a natural extension of (b) — it needs a new push-shaped capability
  interface (§2) and, per the confirmed `sourceContext.githubRepoContext`
  requirement, a **pre-push-a-branch** step that inverts this codebase's
  local-worktree-first flow order. Recommend scoping it as: create session
  fire-and-forget (no Activities-based mid-session UI, per requirements.md's
  own cut), then let the resulting PR converge through the same (b)
  mechanism once Jules opens it — i.e. (c) is genuinely "(b) plus a thin
  create button," not a separate parallel pipeline.
