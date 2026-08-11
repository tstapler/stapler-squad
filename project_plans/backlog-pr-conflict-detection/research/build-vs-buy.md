# Build vs. Buy — Merge-Conflict Detection/Handling

**Research question**: Should merge-conflict detection/handling for `ReconcilePRPending` be built from scratch (extending `GetPRStatus`/gh CLI) or sourced from an existing solution?

## Codebase findings (context for all options below)

- `PRStatus` (`session/git/worktree_git.go:326-334`) and `GetPRStatus` (`worktree_git.go:338-438`) are the sole PR-status data path. The `gh pr view` call at line 345-346 requests only `statusCheckRollup,reviews,comments` — no `mergeable`/`mergeStateStatus`.
- `go.mod` has **no** `go-github`, `githubv4`, or any GitHub GraphQL/REST client dependency. `gh` CLI (invoked via `safeexec.CommandContext`) is the only integration point to GitHub across the whole codebase (also used in `github/client.go`, `server/services/github_service.go`, `session/backlog_commands.go`).
- No webhook receiver exists anywhere in the codebase (`server/` has no `/webhook` or `/hooks` route registered) — confirms requirements.md's framing that adding one would be a new integration class.
- No Mergify/Renovate/Dependabot config files exist in the repo root.
- `ReconcilePRPending` (`session/backlog_lifecycle.go:530-585`) is a simple poll loop: `IsPRMerged` → `GetPRStatus` → gate on `CIFailing`/`HasBlockingReviews` → `AutoReopenForPRFix`. This is the extension point.
- `ConflictedFiles` (`session/vcs/vcs.go:121`, populated via `session/vc/git_provider.go:221`) is **local working-copy** conflict-marker counting (unmerged index entries after a local `git merge`/`rebase`), used by the JJ/Git VCS abstraction for worktree status — a structurally different concern from GitHub's server-side `mergeable`/`mergeStateStatus` PR field. Not directly reusable but same *shape* of problem (interpreting a tri-state-ish signal into "did this go cleanly").
- `session/backlog_commands.go:73-75` references the `/github:pr-ship` skill, which already does interactive merge-conflict resolution as part of driving a PR to green — this is prompt-level guidance for a human-launched Claude session, not reusable Go code, but it's the closest prior art for what to tell a spawned fix-agent about resolving conflicts (see §4).

## 1. Existing OSS library / GitHub App

| Option | Requires GitHub App + webhook? | Maturity | Fit for stapler-squad (local-first, single-user, self-hosted Go, no webhook receiver) |
|---|---|---|---|
| **Mergify** | Yes — GitHub App install + Mergify-hosted rule engine, webhook-driven | Mature, widely used | Poor. Requires installing a third-party GitHub App with repo write access and routing webhooks to Mergify's cloud (or self-hosted Mergify Engine, itself a whole separate service to run). Violates "no new external integrations" constraint outright. |
| **Dependabot auto-rebase** | Built into GitHub, but only auto-rebases Dependabot-authored PRs | Mature (GitHub-native) | N/A — scoped to Dependabot's own dependency-bump PRs, not general backlog-authored PRs. Doesn't solve this problem at all. |
| **Renovate** | Yes — GitHub App (or self-hosted bot needing its own PAT/webhook or scheduled run) | Mature | Same category problem as Mergify: it's a dependency-update bot with its own PR lifecycle, not a generic "detect+fix conflicts on arbitrary PRs" utility. Self-hosting it as a scheduled job to manage *other* PRs it didn't create is not its design center. |
| **GitHub native "Update branch" button / auto-merge + required-branch-up-to-date** | No app/webhook — pure REST/GraphQL API, or `gh pr update-branch` | Mature (GitHub platform feature) | Closest fit *mechanically* — but it's a button/setting, not a library. Automating it means calling `gh pr update-branch <n>` (or `gh api PUT /repos/.../pulls/.../update-branch`) via the *existing* gh-CLI pattern. This isn't really a third-party dependency at all — it's a slightly different verb on the same `gh` binary already in use. Worth calling out as an implementation detail for Phase 3, not a "buy" decision. |

**Verdict**: Not recommended (Mergify, Renovate) — both require GitHub App installation + webhook delivery, which is explicitly out of scope per requirements.md ("No new external integrations... webhook-based conflict detection... rejected"). Dependabot auto-rebase doesn't apply to non-Dependabot PRs. GitHub's native "Update branch" is not an external integration at all (it's another `gh` subcommand) and should be considered purely as an implementation tactic during Phase 3 planning, not evaluated as a "buy" option.

## 2. SaaS / managed API

Searched for a hosted "tell me if this PR is mergeable and rebase it" service distinct from the platform bots above (e.g. generic merge-conflict-as-a-service offerings). No such standalone product exists in mainstream use — conflict detection/rebase automation is exclusively bundled into the GitHub-App-based tools already assessed in §1 (Mergify, Renovate, GitHub's own platform). There is no lighter-weight "just an API call" SaaS tier that avoids the GitHub App/webhook requirement — the App model *is* how these products get PR-write access, so unbundling isn't available even at higher cost.

**Verdict**: Not recommended — no product exists in this shape, and the ones that come close still require the same GitHub App installation model rejected in §1. Not a real option for a local-first, single-user, self-hosted tool with no webhook receiver.

## 3. LLM-generated implementation vs. battle-tested library (the `mergeable`/`mergeStateStatus` interpretation logic)

This is the one piece of genuinely new algorithmic logic: given `mergeable` (`MERGEABLE`/`CONFLICTING`/`UNKNOWN`) and `mergeStateStatus` (`CLEAN`/`DIRTY`/`BLOCKED`/`BEHIND`/`DRAFT`/`HAS_HOOKS`/`UNKNOWN`/`UNSTABLE`) from GitHub, decide: is this PR conflicted right now, or is it a transient `UNKNOWN` mid-computation?

- **Complexity**: This is a small, pure function over two string enums — `func interpretMergeability(mergeable, mergeStateStatus string) conflictSignal`. No I/O, no concurrency, fully unit-testable with table-driven tests over the known enum value combinations (documented in GitHub's public GraphQL schema for `PullRequest.mergeable`/`mergeStateStatus`). This is squarely "simple enough that a small, well-tested Go function is clearly fine" — not reckless custom code. The risk (per requirements.md's Rabbit Holes) is entirely in *behavior* (how to treat transient `UNKNOWN`), not in *implementation difficulty*.
- **Would a library help?** A Go GitHub client (go-github, githubv4) would give typed enum constants for these fields instead of raw strings, which is marginally nicer but:
  - It reads via REST/GraphQL, which means running a *second, parallel* PR-data path alongside `gh` CLI — every other field (`statusCheckRollup`, `reviews`, `comments`, `IsPRMerged`) stays on `gh`. That's a split-brain integration (two ways to auth/fetch PR data, two failure modes, two things to keep in sync) for the sake of two extra JSON fields.
  - `gh pr view --json mergeable,mergeStateStatus` returns exactly these two fields as plain strings — trivially added to the existing `--json` flag list already at `worktree_git.go:346`. Same call, same auth, same error handling, same test harness (`FakeGHRunner`-style patterns already used for `GetPRStatus` tests presumably exist for CI/reviews and should extend directly).
  - requirements.md is explicit: "`gh` CLI is already the exclusive PR-status data source (`GetPRStatus`) and must remain so." Introducing go-github for one field pair is a direct constraint violation, not just scope creep.
- **Scope creep assessment**: Yes, switching client libraries for this one field would be scope creep against a Medium-appetite project. The existing `gh pr view --json` pattern already handles arbitrary additional JSON fields for free — extending the flag list is a ~2-line change, while adopting a new Go GitHub client means new dependency, new auth wiring (gh CLI already handles token/auth transparently; a library needs its own token plumbing), and duplicate PR-fetch code paths for no functional gain.

**Verdict**: Recommended — write the small Go interpretation function directly, extend the existing `gh pr view --json` call with the two new fields, follow the same pattern as the `CIFailing`/`HasBlockingReviews` parsing already in `GetPRStatus`. Do not introduce a GitHub API library for this.

## 4. Fork or adapt existing conflict-resolution logic

- **`ConflictedFiles`** (`session/vcs/vcs.go:121`, `session/vc/git_provider.go:221`): local working-copy unmerged-file counting for the JJ/Git VCS abstraction layer. Different data source (local `git status`/`jj status` vs. GitHub's server-computed `mergeable`), different purpose (worktree health display vs. PR-fix triggering). Not directly reusable code, but confirms the codebase already has *a* precedent for "count/flag conflict state" as a struct field pattern (`PRStatus` should follow the same shape: a bool/enum field alongside `CIFailing`/`HasBlockingReviews`, consistent with existing conventions).
- **`/github:pr-ship` skill** (referenced at `session/backlog_commands.go:73-75`, not found as a local file — appears to be a marketplace/global skill, not repo-tracked): already performs interactive merge-conflict resolution as one step of driving a PR to mergeable state for a human-launched session. This is the closest prior art in the ecosystem for *what to tell an agent* about resolving conflicts, and directly informs the "conflict-specific fixCtx/prompt addition" in scope — reuse its prompt phrasing/approach as a starting point for the new fix-session prompt content, but it's guidance to adapt, not code to fork (it's not Go, not part of the autonomous pipeline, and drives an interactive session rather than `AutoReopenForPRFix`).
- **This session's 3 manual fixes (PRs #147, #148, #150-predecessor)**: no reusable code — these were done by hand via `gh pr view --json mergeable`, manual rebase, and manual `.gitignore`-corruption conflict resolution. No script or function was extracted or committed capturing that logic. Nothing to fork here beyond the general lesson (already captured in requirements.md's Feasibility Risks) that `.gitignore`-style conflicts are a known hard case worth flagging in the fix-session prompt.

**Verdict**: Viable but limited — no Go code to fork; the only adaptable asset is prompt language from `/github:pr-ship`'s conflict-resolution step, useful for authoring the `fixCtx` addition in Phase 3/5.

## Summary Verdict

Build from scratch, extending the existing `gh`-CLI-based `GetPRStatus`/`ReconcilePRPending` pipeline:

1. Add `mergeable`/`mergeStateStatus` to the existing `gh pr view --json` flag list (2-line change to `worktree_git.go:346`).
2. Add a conflict field to `PRStatus` and a small, table-tested pure function to interpret the enum pair (handles the `UNKNOWN` transient-race case called out in requirements.md's Rabbit Holes).
3. Extend `ReconcilePRPending`'s gate (line 568) to also trigger on the new conflict signal, reusing `AutoReopenForPRFix` unchanged per the explicit constraint.
4. Borrow prompt phrasing from the `/github:pr-ship` skill's conflict-resolution guidance for the new `fixCtx` conflict-specific prompt content.

No OSS tool, GitHub App, or SaaS product fits without violating the "no new external integrations / gh CLI remains exclusive / no webhooks" constraints — and the one genuinely new piece of logic (mergeable-state interpretation) is small enough that reaching for a GitHub API library would itself be the over-engineered choice, not the disciplined one.
