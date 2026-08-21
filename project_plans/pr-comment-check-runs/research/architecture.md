# Architecture Research: Comment Noise → Check Runs

Agent 3 (Architecture), SDD research phase. Scope: where a shared "post status"
primitive should live, how it relates to existing check-run *reading*, SHA/commit
consistency, and whether centralizing this in Go is even feasible given automation
runs as independent Claude Code sessions.

All line references are VERIFIED against this worktree at commit `86700b173df48bf57ed87c3b259abb36e13ea050`.

## 1. Where the primitives live today

Two independent GitHub-comment code paths exist, both string-in/error-out, neither aware of the other:

- **Go backend, `gh`-CLI-backed:** `github.PostPRComment(owner, repo string, prNumber int, body string) error`
  ([github/client.go:624](https://github.com/tstapler/stapler-squad/blob/86700b173df48bf57ed87c3b259abb36e13ea050/github/client.go#L624)) shells out to `gh pr comment`. Wrapped by
  `session.Instance.PostComment(body string) error` ([session/pr_tracking.go:71](https://github.com/tstapler/stapler-squad/blob/86700b173df48bf57ed87c3b259abb36e13ea050/session/pr_tracking.go#L71)), exposed over
  ConnectRPC as `GitHubService.PostPRComment` ([server/services/github_service.go:137](https://github.com/tstapler/stapler-squad/blob/86700b173df48bf57ed87c3b259abb36e13ea050/server/services/github_service.go#L137)).
  `GitHubService` itself is explicitly a thin RPC-to-`gh`-shim: its doc comment says it has
  "no dependency on review queue, terminal streaming, or search" and "only need[s] to look
  up a session by ID and call PR operations on it" ([github_service.go:14-18](https://github.com/tstapler/stapler-squad/blob/86700b173df48bf57ed87c3b259abb36e13ea050/server/services/github_service.go#L14-L18)). It has zero
  policy about *when* to comment — callers decide.
- **Go backend, raw-HTTP-backed, keychain-token-authed:** `session.GitHubIssuesPlugin.PostIssueComment(ctx, config, externalID, body string) error`
  (`session/backlog_plugin_github.go:378`), used only by `forwardSyncCloseComment`
  ([server/services/backlog_github_forward_sync.go:32,152](https://github.com/tstapler/stapler-squad/blob/86700b173df48bf57ed87c3b259abb36e13ea050/server/services/backlog_github_forward_sync.go#L32)) — the one deliberate, backend-owned,
  "no silent automated action"-gated comment. Out of scope per requirements; noted only
  because it shows the codebase already has *two* independent comment-posting stacks
  (`gh` CLI vs raw HTTP), a precedent that matters for §2/§4 below.
- **Claude Code skills, `gh`-CLI-only, no Go backend involved:** `github-pr` SKILL.md
  (`~/.claude/skills/github-pr/SKILL.md`, sourced from `~/dotfiles/.claude/skills/github-pr/SKILL.md`
  — this worktree has no local `.claude/skills/github-pr/`, confirmed via `find`) documents
  `gh pr comment`, `gh pr review --approve/--request-changes`, `gh api .../comments` as the
  *only* commenting mechanisms, all direct `gh` CLI invocations with no Go/RPC/MCP layer in
  between. Grepping `pr-ship` (`~/dotfiles/.claude/skills/github/skills/pr-ship/SKILL.md`),
  `pr-refine`, and `code:review`'s SKILL.md for `gh pr comment`/`gh issue comment`/`PostPRComment`
  returned **zero matches** — these orchestration skills don't themselves post top-level
  status comments in their documented instructions; per the requirements doc's own framing
  ("driven ad hoc by whichever session/skill is shepherding a PR"), the actual noisy
  comments are typed inline by the agent at runtime, not codified anywhere greppable. That
  absence *is* the finding: there is no fixed catalog to extend, only prose to add.

No code path anywhere in the repo calls the GitHub Check Runs *write* API
(`Checks.Create`/`POST .../check-runs`) — confirmed via `grep -rn "Checks.Create\|check_runs.*POST"`
across `github/`, `server/services/`, `session/`: zero hits. Check-run creation would be
wholly new capability, not a gap-fill in an existing pattern.

## 2. Read vs. write integration point

The only existing check-run code is read-only, and it's architecturally distant from the
comment-posting primitives above — different auth mechanism, different transport, different
package layer:

- `session.GitHubPRsPlugin.fetchCILabel(ctx, cfg, sha string) string`
  ([session/backlog_plugin_github_prs.go:168-200](https://github.com/tstapler/stapler-squad/blob/86700b173df48bf57ed87c3b259abb36e13ea050/session/backlog_plugin_github_prs.go#L168-L200)) does a raw
  `net/http` GET to `repos/{owner}/{repo}/commits/{sha}/check-runs`, authenticated with a
  keychain-managed token (`github.GetKeychainTokenForHost`), decoded into the minimal
  `githubCheckRun{ Conclusion string }` struct ([backlog_plugin_github_prs.go:47-50](https://github.com/tstapler/stapler-squad/blob/86700b173df48bf57ed87c3b259abb36e13ea050/session/backlog_plugin_github_prs.go#L47-L50)).
  It's called from `computeLabels` during backlog PR polling, purely to derive a
  `pr:ci-failing` label — best-effort, errors swallowed silently (`session/backlog_plugin_github_prs.go:179-183`).
- `PostPRComment` et al. in `github/client.go` shell out to `gh` CLI with `gh auth token`-derived
  credentials (`CheckGHAuth()`), a completely separate auth path from the plugin's keychain HTTP client.

**Is there a natural place to add `CreateCheckRun`/`UpdateCheckRun` next to `PostPRComment`
in `github_service.go`?** Structurally yes for the *write*, but the *read* precedent argues
for putting the low-level Checks API client function in `github/client.go` (peer to
`PostPRComment`, using `gh api repos/{owner}/{repo}/check-runs -X POST -F ...` or
`gh api .../check-runs/{id} -X PATCH -F ...`, staying consistent with the `gh`-CLI-backed
family rather than introducing a third HTTP-token-based path) — then exposing it through
`GitHubService` the same shape as `PostPRComment` (`GitHubService.CreateCheckRun`/`UpdateCheckRunStatus`,
delegating to a new `Instance` method in `session/pr_tracking.go`, e.g.
`Instance.SetCheckRun(name, status, conclusion, summary string) error`, mirroring
`Instance.PostComment`). This keeps `github_service.go`'s existing "thin RPC-to-primitives shim"
character intact and gives the read path (`fetchCILabel`) a *sibling* write path to converge
on later if the two auth mechanisms are ever unified — but unifying them is not required to
ship this feature, since they're only related by "both hit `.../check-runs`," not by shared code.

## 3. SHA-scoping and lifecycle: comment vs. check run are structurally different objects

A PR comment is an append-only, fire-and-forget artifact keyed to the **PR number** — it
survives every force-push/rebase untouched, and posting one is a single atomic call
(`gh pr comment` → done). A check run is keyed to a **commit SHA**
(`repos/{owner}/{repo}/commits/{sha}/check-runs`, confirmed at
[backlog_plugin_github_prs.go:169](https://github.com/tstapler/stapler-squad/blob/86700b173df48bf57ed87c3b259abb36e13ea050/session/backlog_plugin_github_prs.go#L169)) and has a
`pending → in_progress → completed` state machine (with a `conclusion` set only at
`completed`) that a single caller normally owns start-to-finish via `POST` (create) then
`PATCH` (update the same `id`).

This has two direct consequences for the design:

- **Force-push/rebase staleness.** A check run created against SHA `abc123` does not migrate
  to SHA `def456` after a rebase — GitHub's UI will show the old run against a commit that no
  longer exists in the PR's current history, or (depending on GitHub's own de-dup heuristics
  for same-named runs) simply show no check for the new head until one is created against it.
  Whatever creates a check run must re-create (not just update) it against the new head SHA
  after every force-push. This is a natural fit for something that already re-reads
  `pr.Head.SHA` on every poll — i.e., `GitHubPRsPlugin.Fetch`'s existing per-refresh cycle
  (`session/backlog_plugin_github_prs.go:114` `prs []githubPR`, each with `.Head.SHA`) — but a
  poor fit for a one-shot skill invocation that captures the SHA once and has no polling loop
  to notice it went stale.
- **Progress semantics need a session that's still alive to call `PATCH`.** A comment is
  fire-and-forget: post once, never touch again. A check run's value proposition (`pending →
  in_progress → completed`) requires the *same logical unit of work* to call create-then-later-update
  against the same `check_run.id` (or `external_id`/`head_sha`+`name` as a lookup key if the
  id wasn't persisted). A Claude Code session that runs `gh api .../check-runs -X POST` inline
  has nowhere durable to stash the returned `id` for a later `PATCH` unless it writes it to a
  file in the worktree or to backlog item state — there is no existing mechanism for a skill
  to persist a small piece of cross-invocation state tied to "this PR's current check run."
  `session.Instance` (long-lived, one per session, already tracks `GitHubOwner`/`GitHubRepo`/`GitHubPRNumber`)
  is the natural place to hold that id if the Go backend owns the check run; a bare skill
  session has no equivalent unless it re-derives the id by listing check runs for the SHA and
  matching on `name` (doable via `gh api repos/.../commits/{sha}/check-runs --jq '.check_runs[] | select(.name=="...")'`,
  the same read shape `fetchCILabel` already uses).

## 4. Centralize in Go, or document as skill-prompt convention — or both?

Automation today is **not** one fixed backend service — it's N independent Claude Code
sessions, each running a skill (`pr-ship`, `pr-refine`, `code:review`, backlog-driven
autonomous shepherding) that talks to GitHub **directly via `gh` CLI**, confirmed by
`github-pr`'s SKILL.md documenting only `gh` invocations with zero Go/RPC/MCP calls in its
patterns, and by the fact that `PostPRComment`'s ConnectRPC path (`server/services/github_service.go:137`)
requires a `session.Instance` keyed by a stapler-squad session ID — something a bare `gh`-CLI-driven
skill session run outside stapler-squad's own session-tracking has no reason to have or use.
This means:

- **Centralizing the *decision* (comment vs. check) behind a Go RPC is not feasible as the
  primary enforcement mechanism** — skills don't route their GitHub calls through the Go
  backend today, and requiring every skill invocation to first resolve a stapler-squad
  session ID just to post a status would be a bigger, out-of-scope architecture change
  (requirements explicitly rule out "redesigning backlog automation end-to-end"). The
  decision logic has to live primarily as **documented convention in skill prompts** —
  i.e., a written rule in `github-pr`'s SKILL.md (or a new shared reference doc it links,
  parallel to how it already links out to `github-actions-debugging` and
  `github-address-pr-comments`) stating the comment-vs-check-run criterion, plus updates to
  `pr-ship`/`pr-refine`/`code:review`'s own SKILL.md files at the specific points where they
  currently post status (Gate reporting in `pr-ship`, verdict reporting in `code:review`) to
  say "set a check run" instead of "comment" for pass/fail-shaped state.
- **A Go helper is still worth building, but as an *available tool*, not a *mandatory
  gate*.** A `gh api repos/.../check-runs` one-liner is easy enough for a skill to run
  directly without any new Go code — the actual blocker to a good check-run UX is knowing
  the check-run `id` for later `PATCH`, which is a state-management problem, not an
  API-access problem. So the highest-leverage Go addition is probably a small wrapper script
  (shipped alongside the skill, à la `github-address-pr-comments`' `pr-threads.py` pattern of
  a versioned script embedded in the SKILL.md) that does create-or-update-by-name against a
  SHA in one call, rather than a full ConnectRPC surface — that keeps the primitive usable by
  both a skill's direct `gh`/script invocation *and*, if wanted later, a Go caller
  (`GitHubService`/`Instance`) without forcing skills through RPC. This mirrors the existing
  split in §1: `gh`-CLI-backed primitives in `github/client.go` for the Go backend's own use
  (e.g. `forwardSyncCloseComment`), and a standalone script for skill-invoked, non-session-scoped use.
- **Where a Go RPC *would* pull its weight:** the one flow that already runs through Go
  session state end-to-end is backlog-driven autonomous shepherding, since those sessions are
  `session.Instance`s with `GitHubOwner`/`GitHubRepo`/`GitHubPRNumber` already populated and a
  natural place (`Instance`) to stash a check-run id across the session's lifetime. Adding
  `Instance.SetCheckRun`/`GitHubService.CreateCheckRun` (per §2) is worth doing specifically
  to give *that* flow a typed, tested path — while accepting that `pr-ship`/`pr-refine`/`code:review`
  running as ad hoc Claude Code sessions will call the underlying `gh api` primitive directly
  per the documented convention, not through the RPC.

## Summary of the shape of the fix

1. Comment-vs-check-run decision logic: **documented convention in skill prompts**
   (`github-pr` SKILL.md + point updates in `pr-ship`/`pr-refine`/`code:review`), because
   skills don't route GitHub calls through the Go backend today and requiring that would be
   out of scope.
2. Check-run *writing* capability: new, sits next to the existing check-run *reading*
   (`fetchCILabel`) only by API surface (`.../check-runs`), not by shared code — build it as
   (a) a `gh api`-based primitive in `github/client.go` peer to `PostPRComment`, exposed via
   `Instance`/`GitHubService` for the one flow that's already Go-session-scoped (backlog
   autonomous shepherding), and (b) a standalone script skills can call directly, mirroring
   `github-address-pr-comments`' `pr-threads.py` pattern, for the ad hoc `gh`-CLI-driven skills.
3. SHA-scoping means whatever creates a check run must be prepared to re-create (not just
   patch) it after every force-push, and the `pending→in_progress→completed` update requires
   persisting the check-run id (or re-deriving it by SHA+name) across the life of the unit of
   work — `session.Instance` has a natural slot for this; a bare skill invocation does not and
   would need to re-derive via a read call each time.
