# Architecture Research: Google Jules Integration

Agent 3 (Architecture), SDD research phase for `google-jules-integration`. Builds
on `project_plans/linear-jira-integration/research/architecture.md` (cited by
section below as "linear-jira/architecture.md §N") for the parts that
generalize — the `ItemSourcePlugin` interface shape, credential-storage
precedent, and the `PluginConfig.Raw` pattern. This doc covers only what that
one doesn't: Jules is a **push/dispatch** integration (stapler-squad creates
work), not a **pull/import** one (an external tracker's items flow in), and
Jules has no local process at all — no PTY, no worktree the way Claude
Code/Aider sessions have one.

## 0. Resolving the "Source model mismatch" rabbit hole (Jules API, confirmed)

Requirements' Rabbit Holes section flagged this as the swing question between
(b) and (c). Fetched `developers.google.com/jules/api` and the Sessions REST
reference directly:

- **Sources must be pre-registered through the Jules *web app*** (installing
  Jules' GitHub App and picking repos) — this cannot be done via the REST API
  at all. A user must visit jules.google.com at least once, per repo, before
  any API integration (thin or full) can dispatch work against it. This is a
  real, unavoidable manual setup step — worth surfacing in the plan as an
  onboarding prerequisite, not something an adapter can automate away.
- **`Sessions.create` requires `sourceContext.source` (a pre-registered
  `sources/{id}`) + `sourceContext.githubRepoContext.startingBranch`** — an
  *existing, already-pushed* branch name in that GitHub repo. **Jules cannot
  target an arbitrary local git worktree** — confirming the rabbit hole's
  concern. This resolves the requirements' Open Question directly: option
  (c) never gives Jules a local worktree; it gives Jules a repo + branch
  name that must already exist on GitHub. If stapler-squad wants Jules to
  start from a fresh branch (rather than an existing shared one like `main`),
  it must push an empty branch first — a `git push` of a ref, not a worktree
  checkout.
- **`state` progresses through `QUEUED → PLANNING → AWAITING_PLAN_APPROVAL →
  IN_PROGRESS → COMPLETED/FAILED`**, with `requirePlanApproval` (default
  `false`, i.e. auto-approved) controlling whether `AWAITING_PLAN_APPROVAL` is
  a real stopping point. For fire-and-forget MVP (requirements' explicit
  non-goal: no mid-task steering), leave `requirePlanApproval=false` and
  `automationMode=AUTO_CREATE_PR` — Jules opens the PR itself once done, no
  polling of `Activities` needed for MVP.
- **The resulting PR surfaces on the Session resource itself**:
  `outputs[].pullRequest.{url,title,description}` once `state=COMPLETED`. No
  separate call needed to discover the PR — one `GetSession` poll loop
  answers both "is it done" and "what's the PR."
- **Auth**: a single `X-Goog-Api-Key` header, max 3 keys per user. No
  per-host/per-username concept (matches Linear's shape per
  linear-jira/architecture.md §5, not GitHub's `AccountRef{Username,Host}`
  shape). **No documented rate limits** — poll conservatively (mirror
  `WorktreePRPollerConfig`'s 60s interval / 5min no-change backoff defaults,
  `session/worktree_pr_poller.go:39-55`) and treat this as an explicit
  unknown to revisit once real usage data exists.

## 1. Push vs. pull mismatch — the right seam is a new interface, not `ItemSourcePlugin`

`ItemSourcePlugin.Fetch` (linear-jira/architecture.md §1) answers "what
external items exist that I don't have yet" — a pull model that creates *new*
`BacklogItem`s. Jules' create direction is the opposite: an *existing*
`BacklogItem` the user is already tracking should cause Jules to start work.
Forcing this through `Fetch`/`MapToBacklogItem` would be backwards — there's
no new external item to map, and no cursor to advance.

The **existing session-creation seam is `SessionCreator`**
(`server/services/backlog_service.go:30-36`):

```go
type SessionCreator interface {
    CreateDirectorySession(ctx, title, path, prompt string, tags []string, oneShot, hidden bool) (*session.Instance, error)
    CreateWorktreeSession(ctx, title, repoPath, worktreePath, prompt string, tags []string, oneShot, hidden bool) (*session.Instance, error)
}
```

Both methods return `*session.Instance` — a live, tmux-backed, worktree-owning
object (`session.NewInstance`, `session/instance.go:864` unconditionally
constructs a `ProcessManager` at line ~950: `NewProcessManager(ctx,
BackendTmux, ...)`). There is **no natural way to satisfy this interface for
Jules** without faking a tmux pane that doesn't exist. Widening
`SessionCreator` with a `CreateJulesSession` method would leak a
Jules-specific concept into an interface that's otherwise entirely
"start a local process" — the same interface-pollution smell
`interface-pollution-checklist` flags, and the one linear-jira/architecture.md
§4 deliberately avoided by defining forward-sync capability interfaces at
the *consumer* site instead.

**Recommendation**: a new, small, consumer-defined interface — e.g.

```go
// JulesDispatcher creates a Jules-backed session of work for a backlog item
// already targeting a GitHub-hosted repo/branch. Unlike SessionCreator, this
// never touches tmux, ProcessManager, or a local worktree.
type JulesDispatcher interface {
    DispatchToJules(ctx context.Context, item *session.BacklogItemData, prompt string) (julesSessionName string, err error)
}
```

owned by a new, small `jules` package (adapter) and consumed by a new
`BacklogService` method (or a sibling service — see §4) that plays the same
*role* `SpawnSessionFromItem` plays for local agents but does not call
`spawnSessionAfterGates`'s worktree/tmux machinery at all (§2, §3).

## 2. Session model fit — `session/ent/schema` assumes tmux, but `ItemSession` doesn't require an `Instance` at all

`session/ent/schema/session.go` (the `Session` entity backing `Instance`) is
built around a live local process: `path`, `branch`, `height`/`width` (PTY
dimensions), `existing_worktree`, `backend` (ProcessManager backend pin),
`worktree` edge (one-to-one to a `Worktree` entity). Every field assumes
"there is a tmux pane with a filesystem worktree behind it." A Jules session
has none of that — no PTY, and per §0, stapler-squad doesn't even own the
checkout Jules works in.

**But `ItemSession` (the join entity that actually drives the backlog
pipeline) does not require a `Session`/`Instance` row to exist at all** —
this is the load-bearing finding for this project. `ItemSession.SessionUUID`
is documented as a "*loose FK to Session; not an ent edge*"
(`session/ent/schema/item_session.go:23-24`), and the codebase already has a
**live, production example of an `ItemSession` with no backing tmux
`Instance`**: headless triage calls.

`BacklogService.TriggerTriage` (`server/services/backlog_service_triage.go:2714`)
creates an `ItemSession` with a synthetic UUID —
`triageSessionUUID := headlessTriageUUIDPrefix + uuid.New().String()`
(`headlessTriageUUIDPrefix = "headless-triage-"`,
`server/services/backlog_service_triage.go:423`) — and role
`session.SessionRoleTriage`, then drives the actual work in a goroutine that
calls a `claude -p` subprocess directly, with **no `session.NewInstance`,
no tmux, no `Session` ent row ever created**. `IsTmuxBackedSessionRole`
(`session/backlog.go:73-75`) exists precisely to let cleanup sweeps
(`reconcileTerminalItemSessions`, `archiveItemWorkSessions`) distinguish
"this role has a live tmux pane to kill" (`work`, `review`) from "this role
never had one" (`triage`) — its own doc comment says triage sessions "were
never tracked as a live Instance in the first place and have nothing to
kill."

**This is the exact template for a Jules-backed session**: a fourth
`SessionRole` (e.g. `SessionRoleJulesWork = "jules_work"`), excluded from
`IsTmuxBackedSessionRole` (so archive/cleanup sweeps correctly skip trying to
kill a nonexistent tmux pane), with `ItemSession.SessionUUID` holding the
Jules session's own resource name (`sessions/{id}`) instead of a
stapler-squad-generated UUID — giving the poller (§3) a natural key to
re-look-up the Jules session by, the same way `session_uuid`'s index
(`session/ent/schema/item_session.go:107`) is already used for O(1) lookup
on `EventExited`-style hooks.

**What a Jules-backed session needs that `ItemSession` already has**:
`started_at`, `last_progress_at` (updatable from each poll tick),
`estimated_cost_usd` (if Jules exposes cost — TBD, not in the fields fetched
in §0), `ac_snapshot`/`pipeline_mode_snapshot` (unchanged — the *prompt*
Jules receives is still built the same way other backlog work sessions'
prompts are). **What it does NOT need**: `base_commit_sha`/`last_commit_sha`/
`commit_count_since_spawn` (these are populated by
`refreshWorkSessionGitActivity` reading a *local* worktree's HEAD — Jules
work happens on Google's infrastructure, so these fields stay at their zero
value for Jules-backed rows, mirroring how `ExternalItem.State` stays `""`
for two-way-sync-incapable plugins per linear-jira/architecture.md §3).

**No ent schema migration is required** — `ItemSession`'s fields are already
generic enough (nothing here needs a new column), only a new `SessionRole`
constant and a specific value written to the existing `session_uuid` string
field.

## 3. Data flow for option (c): create → poll → PR-resolved

Two more existing primitives complete the picture, both already decoupled
from `Instance`/tmux:

- **`storage.SetBacklogItemPRAndTransition`** — the same primitive
  `report_pr_created` (an MCP tool, callable only from *inside* a live
  agent's own session — impossible for Jules, which has no stapler-squad MCP
  access) and the **system-side manual-override path**
  (`server/services/backlog_service_lifecycle.go:398-425`,
  `UpdateBacklogItem`'s `pr_url`/`pr_number` presence-gated escape hatch) both
  call to atomically record a PR and transition `review → pr_pending` in one
  write. This is exactly what a Jules poller should call once
  `GetSession` returns `state=COMPLETED` with a non-empty
  `outputs[].pullRequest.url` — **no MCP tool, no live agent, no tmux
  involved**, just a server-side goroutine calling a storage method directly.
- **`reconcilePRPendingItem`/`ReconcilePRPending`**
  (`session/backlog_lifecycle_pr.go:1287`, `:1592`) — the merge-detection
  sweep that walks `pr_pending` items to `done`. It reads `item.PrNumber` off
  the **`BacklogItem` ent row directly** and calls a `prPendingChecker`
  (`IsPRMerged(prNumber int) (bool, error)`,
  `session/backlog_lifecycle_pr.go:98`) built per-repo-path
  (`defaultPRPendingCheckerFactory(repoPath string)`,
  `:125`) — **it has no dependency on `Instance` or a live session at all**.
  Once a Jules-produced PR is recorded via the primitive above, this sweep
  picks it up automatically, with zero new code. This directly satisfies the
  requirements' success metric ("pollable via the existing PR review path,
  not a separate surface") — and clarifies that the literal mechanism is
  `ReconcilePRPending`, not `WorktreePRPoller`: `WorktreePRPoller`
  specifically scans **local worktrees with no active session**
  (`session/worktree_pr_poller.go:14-33`, built on
  `unfinished.Scanner`-discovered `WorktreeScanItem`s) — a Jules PR has no
  local worktree at all (§0), so `WorktreePRPoller` would never see it.
  `PRStatusPoller` is similarly out — it operates on `[]*Instance`
  (`session/pr_status_poller.go:52-60`, `fetchAndUpdatePRStatus(inst
  *Instance)`). Both existing pollers are the *local-agent* half of PR
  tracking; `ReconcilePRPending` is the *item-level* half both halves
  ultimately feed into, and it's the one a Jules integration actually needs.

**Full flow**:

```
BacklogItem (ready, repo already registered as a Jules Source)
  → user clicks "Dispatch to Jules" (new UI action)
  → new JulesDispatcher.DispatchToJules:
      - resolve sources/{id} for item.RepoPath's owner/repo (cache; see §4)
      - push a fresh branch if item has no existing pushed branch (§0)
      - POST /v1alpha/sessions {prompt, sourceContext, automationMode: AUTO_CREATE_PR}
      - storage.CreateItemSession(role: jules_work, session_uuid: "sessions/{id}")
      - transitionWithGuard(item, ready → in_progress, TriggeredByUser)
  → new JulesSessionPoller (mirrors WorktreePRPollerConfig's shape: interval,
    call timeout, backoff — session/worktree_pr_poller.go:39-55) ticks:
      - GetSession(sessions/{id}) for every open jules_work ItemSession
      - on state ∈ {QUEUED, PLANNING, IN_PROGRESS}: update last_progress_at,
        emit BacklogStatusEvent-equivalent note (no status transition)
      - on state == COMPLETED with outputs[].pullRequest.url:
          storage.SetBacklogItemPRAndTransition(item, url, number, note)
          → item now pr_pending, ItemSession ended_at set
      - on state == FAILED: transition item back to a fixable state
        (mirrors AutoReopenAfterFailedReview's role, new code) with the
        FAILED reason as a note
  → ReconcilePRPending (existing, unmodified) takes over: polls GitHub
    merge status → done, same as every other agent's PR.
```

**What's new**: the dispatcher, the poller, one `SessionRole`, one
`JulesDispatcher`-shaped MCP/UI entry point, a source-ID cache. **What's
reused unmodified**: `ItemSession`, `BacklogStatusEvent`,
`SetBacklogItemPRAndTransition`, `transitionWithGuard`, `ReconcilePRPending`,
the planning-gate/WIP-cap concepts (though see §6 on whether Jules should be
subject to the same WIP cap — it consumes no local resources, so probably
not, or a separate Jules-specific cap).

## 4. Integration points — where the code lives

- **`jules/` package (new, sibling to `github/`)** — the `JulesClient`
  wrapping the REST API (`ListSources`, `CreateSession`, `GetSession`), and
  `jules/keychain.go` for the API key. For the keychain shape, **prefer
  `session/sshremote/keystore.go` as the template over `github/keychain.go`**:
  Jules' single global API key (no per-host/per-username axis, confirmed §0)
  matches `sshremote`'s "one identity, no host dimension" shape more closely
  than GitHub's `AccountRef{Username,Host}` multi-account model — including
  its 5s D-Bus-hang timeout guard (`sshremote/keystore.go:29-33`) and
  test-seam pattern, which `github/keychain.go` also has but `sshremote` is
  the more recent, more directly analogous precedent. Service namespace:
  a new `"stapler-squad-jules"` (mirroring `sshremote`'s
  `"stapler-squad-ssh"`, deliberately distinct per that file's own comment
  on why domains shouldn't share a keychain service).
- **`server/services/jules_dispatch_service.go` (new)** — owns
  `JulesDispatcher`, called from a new RPC (or reuses
  `SpawnSessionFromItem`'s shape as `DispatchJulesSessionFromItem`) and from
  a new MCP tool if item creation should also be triggerable headlessly.
  Registered/wired in `server/dependencies.go` alongside where
  `syncRegistry`/`backlogCtrl` are built (linear-jira/architecture.md §2's
  wiring pattern), and in `server/server.go` alongside where
  `WorktreePRPoller`/`PRStatusPoller` are started, for the new
  `JulesSessionPoller`.
- **Source-ID cache**: `ListSources` must be resolved once per repo (owner/repo
  → `sources/{id}`) since Jules' own web app is the only place to register a
  source (§0). This is small, in-memory-with-refresh state, analogous to
  `PRStatusPoller`'s `etagCache` — does not need its own ent table for MVP;
  a `sync.Map` inside the `jules` package keyed by `owner/repo`, refreshed on
  cache miss, is sufficient at this scale.
- **Frontend**: a "Dispatch to Jules" action on the backlog item detail view
  (only shown when a Jules API key is configured — the existing
  "opt-in by construction" pattern requirements already call for), and a
  provenance indicator on the item mirroring §7 of linear-jira/architecture.md
  (BacklogItemCard.tsx / SourceSection.tsx's hardcoded-GitHub badge — a
  Jules-dispatched item is a *pre-existing* stapler-squad item gaining a
  Jules-run session, not an imported item, so this is a session-role badge
  ("Jules is working on this"), not the source-provenance badge those
  components already render — a related but distinct UI surface).

## 5. EventStorming — option (c) flow

Multiple real actors (user, stapler-squad server, Jules' cloud service,
GitHub) and at least one genuinely new policy (mapping Jules `state` onto
backlog status), so unlike linear-jira/architecture.md §9 (which skipped this
as "just two more plugins"), this integration earns the table.

| Actor | Command | Event | Policy | Resulting Command/Event |
|---|---|---|---|---|
| User | Click "Dispatch to Jules" | — | Item must be `ready`/`in_progress` (reopen), repo has a registered Jules Source, API key configured | `DispatchToJules(item)` |
| stapler-squad | `DispatchToJules` | — | Item has a pushed branch, or push a fresh one first | `POST /sessions` (Jules API) |
| Jules | (accepts) | `JulesSessionCreated` | — | `ItemSession{role:jules_work}` created; item → `in_progress` |
| Jules (async, off-system) | agent runs | `JulesSessionStateChanged(PLANNING\|IN_PROGRESS)` | Poller detects on next tick (no push/webhook — REST poll only, confirmed §0 has no webhook resource) | Update `last_progress_at`; no status transition |
| Jules (async) | opens PR | `JulesSessionCompleted{pullRequest}` | Poller detects `state=COMPLETED` | `SetBacklogItemPRAndTransition` → item → `pr_pending` |
| Jules (async, failure path) | — | `JulesSessionFailed` | Poller detects `state=FAILED` | Transition item back to a fixable/reopenable state + note (new policy, mirrors `AutoReopenAfterFailedReview`'s role but for an external failure, not a local review verdict) |
| GitHub | PR merged | `PullRequestMerged` | `ReconcilePRPending` (existing, unmodified) | Item → `done` |
| stapler-squad | (background) | — | No webhook from Jules — poll-only; rate-limit-unknown (§0) means backoff must be conservative and adjustable | `JulesSessionPoller` tick |

## 6. Recommendation

**Phased (b)-then-(c)**, but with (b) scoped much thinner than the
requirements doc's own description of it, because Jules' actual API shape
makes (c)'s hardest-sounding piece — "make Jules-backed sessions fit the
session model" — cheap (§2: reuses the headless-triage pattern verbatim,
zero schema migration), while the *dispatch* half is genuinely new work no
matter which option is chosen:

- **Option (b) as literally described in requirements** ("Jules' own GitHub
  App creates PRs exactly as today; stapler-squad only imports the resulting
  PR") **does not meet the stated success metric** — "create a Jules-backed
  unit of work from stapler-squad's backlog UI without visiting
  jules.google.com" requires *some* API call to `Sessions.create`, which is
  push/dispatch, not pull/import. Pure (b) is really alternative (a)
  (deep-link only) with an import step bolted on, and requirements already
  rejected (a).
- **What phasing usefully means here**: ship the dispatch call
  (`Sessions.create`) and the completion poll
  (`GetSession` → `SetBacklogItemPRAndTransition`) together as a first slice
  — this is the minimum that satisfies both success metrics and is not
  meaningfully smaller than "full" once §2/§3 show the session-model and
  PR-flow integration cost is near zero. **Defer only**: the `Activities` API
  (mid-session steering — already an explicit non-goal), `FAILED`-state
  auto-recovery policy (start with "surface as stuck, let the user decide,"
  same as any other stuck item, rather than building
  `AutoReopenAfterFailedReview`-equivalent logic for Jules on day one), and
  the WIP-cap question (§3) — start with Jules dispatches uncapped or capped
  separately, tune once real usage exists.
- **Reasoning**: the requirements' own rabbit hole ("adding a non-tmux-backed
  session kind may require more refactoring than it looks like") does not
  hold up under inspection — `ItemSession` already supports non-tmux rows in
  production (triage), and the PR-resolution path
  (`SetBacklogItemPRAndTransition` → `ReconcilePRPending`) is already fully
  decoupled from `Instance`. The actual new surface area is narrow and
  isolated (§4: one new package, one new service, one new poller, one new
  role constant) — consistent with the constraint that alpha-API churn must
  not ripple into `session/` core, since none of `session/`'s core types
  need to change.

## 7. Summary of concrete integration points for plan.md

| # | File | Change |
|---|---|---|
| 1 | `jules/client.go` (new) | `JulesClient`: `ListSources`, `CreateSession`, `GetSession` |
| 2 | `jules/keychain.go` (new) | Single-global-key credential storage, mirroring `session/sshremote/keystore.go`'s shape (service `"stapler-squad-jules"`) |
| 3 | `session/backlog.go` | Add `SessionRoleJulesWork = "jules_work"`; confirm it's excluded from `IsTmuxBackedSessionRole` |
| 4 | `server/services/jules_dispatch_service.go` (new) | `JulesDispatcher` interface + implementation: push-branch-if-needed, resolve source ID, `CreateSession`, `storage.CreateItemSession`, `transitionWithGuard` |
| 5 | `session/jules_session_poller.go` (new) | Poll loop mirroring `WorktreePRPollerConfig`'s shape; on `COMPLETED` calls `storage.SetBacklogItemPRAndTransition`; on `FAILED` transitions to a fixable state |
| 6 | `server/dependencies.go`, `server/server.go` | Wire `JulesClient`, `jules_dispatch_service`, start `JulesSessionPoller` alongside `WorktreePRPoller`/`PRStatusPoller` |
| 7 | `server/mcp/tools_backlog.go` or a new RPC | User/agent-triggerable "dispatch to Jules" entry point |
| 8 | `web-app/src/components/backlog/...` | "Dispatch to Jules" action (gated on API key configured) + a Jules-session-role indicator (distinct from the source-provenance badge in linear-jira/architecture.md §7) |
| 9 | `docs/registry/features/backend/*.json` | New entries per `.claude/rules/feature-registry.md` |
