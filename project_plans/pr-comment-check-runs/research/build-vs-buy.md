# Build vs. Buy — GitHub Check Runs for PR/Issue Comment Noise Reduction

Agent 6 research for `project_plans/pr-comment-check-runs/`. Scope per requirements: add a
"create/update check run" capability so status-with-clean-pass/fail automation can stop
posting comments. Out of scope: check-run taxonomy, `forwardSyncCloseComment`, backlog
automation redesign.

## 1. Existing OSS library vs. `gh` CLI

**VERIFIED (code read):** `go.mod` has no GitHub API client library at all — no
`google/go-github`, no `shurcooL/githubv4`. Grep of `go.mod` for `go-github`/`githubv4`
returns nothing.

The codebase currently talks to GitHub two ways, both already in `github/client.go` and
`session/backlog_plugin_github_prs.go`:

- **`gh` CLI via `safeexec.CommandContext`** — used for everything that mutates state today:
  `PostPRComment` ([github/client.go:624](github/client.go#L624), `gh pr comment ... --body`),
  `MergePR`, `ClosePR`, plus reads like `GetPRDiff`, `GetPRComments`. Auth comes from
  `gh auth token --hostname <host>` ([github/cli_import.go:71](github/cli_import.go#L71)) —
  the user's own `gh` OAuth session, not a separate token store.
- **Raw `net/http` + a stored PAT** — used only in the read-only backlog plugin
  (`session/backlog_plugin_github_prs.go`), which already calls the Checks *read* endpoint:
  `GET repos/{owner}/{repo}/commits/{sha}/check-runs`
  ([session/backlog_plugin_github_prs.go:169](session/backlog_plugin_github_prs.go#L169)),
  decoded into the `githubCheckRun{Conclusion string}` struct
  ([session/backlog_plugin_github_prs.go:47-50](session/backlog_plugin_github_prs.go#L47-L50)).
  Token comes from `github.GetKeychainTokenForHost` (Settings-managed, host-keyed).

Neither path creates or updates a check run today — only reads.

**`gh` CLI check-run support (VERIFIED via WebSearch):** `gh` has no dedicated
`gh check-run create` subcommand. The realistic path is `gh api -X POST
repos/{owner}/{repo}/check-runs -f name=... -f head_sha=... -f status=... -F
output[title]=... --input -` (or a JSON body via `--input -`/`-F` for nested `output`
fields) — the exact same `gh api ... -f/--jq` pattern this repo already uses for
`IsForkRepo` ([github/client.go:540](github/client.go#L540), `gh api repos/%s/%s --jq
.fork`). Confirmed against GitHub's REST docs and `gh api` field-flag semantics.

**`google/go-github`'s Checks service (VERIFIED via WebSearch):** the library does have
`ChecksService.CreateCheckRun` / `UpdateCheckRun` (`github/checks.go` upstream), but pulling
it in would be a **new dependency** this repo has deliberately avoided so far — every other
GitHub interaction goes through `gh` CLI shellouts or hand-rolled `net/http` structs scoped
to exactly the fields needed (`githubPR`, `githubCheckRun`). Adding a ~40-file client library
for one write endpoint cuts against that established pattern.

**Critical constraint (VERIFIED via WebSearch, not previously known to the codebase):** the
Checks API **does not support fine-grained PATs** — only classic PATs and GitHub App
installation tokens can create/update check runs (GitHub confirmed this was removed for
fine-grained tokens after early support caused edge-case issues; see Sources). This matters
because the requirements doc's two posting primitives use two different credential sources:
`gh auth token` (github/client.go, likely a classic OAuth-scoped token — `gh auth login`
grants classic-style scopes) is probably fine, but the Settings-managed keychain PAT used by
`session/backlog_plugin_github_prs.go` needs to be verified as classic, not fine-grained,
before check-run writes are wired to it. This is a go/no-go gate for implementation, not
just a nice-to-know.

**Verdict: Recommended — build via `gh api -X POST check-runs`, following the exact
`IsForkRepo`-style `gh api` pattern already in `github/client.go`.** No new dependency;
matches the existing "shell out to `gh`, mutate via CLI" convention for every other
write path. Do **not** add `google/go-github` for this alone — it's a viable but
unnecessary alternative (Viable, not Recommended) given the repo's established
zero-SDK-dependency pattern for GitHub writes.

## 2. SaaS / managed alternative

Candidates considered: GitHub's own "Required workflows"/branch protection status checks
(not applicable — those consume checks, they don't help an app *produce* fewer comments),
Danger/Danger-JS-as-a-bot, a Probot-based hosted check-run app, Renovate-style hosted bot.

- **Danger (self-hosted or Danger JS in CI)** — solves "structured PR feedback" but is a
  *comment*-posting tool by default (its own dashboard-per-PR-comment model), not a
  check-run-first one; using it would mean adding a Ruby/Node toolchain to a Go monorepo
  for a capability `gh api` already covers in ~1 subprocess call.
- **Hosted GitHub App (Probot-based, e.g. a generic "status bot")** — would require
  installing a third-party app with `checks:write` (and typically broader) repo permissions.
  The requirements context is explicit that stapler-squad is "a personal/small-team tool,"
  and per this repo's own credential model (`gh auth token`, Settings-managed keychain PAT —
  no GitHub App installation flow exists anywhere in `github/` or `session/`), introducing
  a GitHub App is a new trust boundary and a new auth mechanism, not a drop-in.
- **GitHub Actions-native check runs** (i.e., let CI workflows report their own check runs,
  which GitHub does automatically for every workflow job) — already happens for free for
  anything running in Actions. It doesn't help here because the requirement is about
  *stapler-squad's own automation* (skills like `github:pr-ship`, backlog shepherding)
  reporting status from outside a workflow run, which Actions' automatic check reporting
  doesn't cover.

**Verdict: Not recommended.** Every SaaS/hosted option either solves a different problem
(comment formatting, not comment-vs-check routing) or requires a new trust boundary
(GitHub App installation) that contradicts this repo's existing single-PAT/`gh`-CLI
credential model for a small personal/team tool. The capability needed is a thin,
well-documented REST call — not a service worth outsourcing.

## 3. LLM-generated implementation vs. tested library — is there a real algorithm here?

**No.** Reviewed the two call sites the requirements doc names:

- `PostPRComment` (`server/services/github_service.go:137`, `github/client.go:624`) and
  `Instance.PostComment` (`session/pr_tracking.go:71`) are both pure passthroughs — take a
  string body, shell out. There's no decision logic inside them today; callers (skills)
  decide *whether* to call them.
- The "decide comment-vs-check from content" question the research prompt raises isn't
  actually a new algorithm to build: it's a **calling-convention change** — skills that
  currently always call `PostComment`/`PostPRComment` need a second primitive
  (`CreateOrUpdateCheckRun`) to call instead when the payload is a clean pass/fail/state
  (per requirements: "Status information with a clean pass/fail or state shape"). That
  classification already exists implicitly in each skill's own logic (a skill author/prompt
  already knows whether it's reporting "CI passed" vs. "here's a substantive review
  finding needing human reaction") — it's a routing decision made by the caller, not a
  data structure or algorithm the new code must invent.
- The check-run payload itself (`name`, `head_sha`, `status`, `conclusion`, `output.title`,
  `output.summary`) is a fixed, GitHub-documented shape — no parsing/inference required.

**Verdict: Axis not relevant to this ticket.** This is API plumbing (one more `gh api`
wrapper function alongside `PostPRComment`, `MergePR`, `ClosePR` in `github/client.go`)
plus a documented calling convention for skill authors, not a "build vs. buy a tested
data-structure/algorithm" decision. Skip further analysis on this axis.

## 4. Fork or adapt an existing internal pattern

Two existing patterns are directly reusable as templates, per the research prompt's own
lead:

- **`githubCheckRun`-reading code** (`session/backlog_plugin_github_prs.go:47-200`) is the
  read-side of exactly the API this ticket needs the write-side of. It already knows the
  URL shape (`repos/%s/%s/commits/%s/check-runs`), auth pattern (keychain token via
  `GetKeychainTokenForHost`), and JSON envelope (`{"check_runs": [...]}`). A new
  `CreateCheckRun`/`UpdateCheckRun` function is the natural sibling — same package,
  same auth helper, POST instead of GET. This is a genuine "extend, don't invent" case.
- **`forwardSyncCloseComment`'s "no silent automated action" gating pattern**
  (`server/services/backlog_github_forward_sync.go:18-32`) is a *convention* (always leave
  a visible trace of automated action; see the `externalIssueCloser` interface pairing
  `CloseIssue` with `PostIssueComment`), not a code template for check-run creation itself.
  It's directly relevant to the *broader* ticket goal (reduce noise) as a precedent to keep
  in mind — e.g. even a check-run-only status update should probably still be discoverable
  without digging through Checks UI — but it doesn't provide API-call scaffolding the way
  the `githubCheckRun` read code does. Worth citing in the plan phase as the "make automated
  actions visible" precedent; not something to literally extend/fork here.

**Verdict: Recommended** — add the write-side function next to the existing
`githubCheckRun` read struct/logic in `session/backlog_plugin_github_prs.go` (or promote
both to `github/client.go` alongside `PostPRComment`/`MergePR`/`ClosePR` if a Go-backend
call path is needed, following the `gh api -X POST` pattern from `IsForkRepo`), rather than
building a new abstraction. Do not attempt to generalize `forwardSyncCloseComment`'s gating
pattern into this feature — it's a related precedent, not a reusable code template.

## Summary Table

| Option | Verdict |
|---|---|
| `gh api -X POST check-runs` (extend `github/client.go`'s existing `gh api` pattern) | **Recommended** |
| Add `google/go-github` dependency for `ChecksService` | Viable, not recommended |
| Hosted GitHub App / Danger-style SaaS bot | Not recommended |
| LLM-generated "comment vs. check" decision algorithm | Axis not applicable — no real algorithm exists |
| Extend `githubCheckRun` read code with a write sibling | **Recommended** |
| Fork/generalize `forwardSyncCloseComment`'s gating pattern | Not recommended as a code template (useful only as a design precedent) |

## Open question to carry into planning

Confirm whether the Settings-managed keychain PAT (`GetKeychainTokenForHost`, used by
`session/backlog_plugin_github_prs.go`) is a classic PAT or fine-grained PAT — the Checks
API rejects fine-grained tokens (WebSearch-verified, Sources below). If skills end up
calling through the Go backend using that token rather than `gh auth token`, this is a
blocking prerequisite, not a nice-to-know.

## Sources

- [REST API endpoints for check runs — GitHub Docs](https://docs.github.com/en/enterprise-cloud@latest/rest/checks/runs)
- [Creating GitHub Checks (and Understanding the Checks API) — Ken Muse](https://www.kenmuse.com/blog/creating-github-checks/)
- [go-github/github/checks.go — google/go-github](https://github.com/google/go-github/blob/master/github/checks.go)
- [Missing `Checks` permission in personal access token — GitHub community discussion #129512](https://github.com/orgs/community/discussions/129512)
- [Building CI checks with a GitHub App — GitHub Docs](https://docs.github.com/en/apps/creating-github-apps/writing-code-for-a-github-app/building-ci-checks-with-a-github-app)
