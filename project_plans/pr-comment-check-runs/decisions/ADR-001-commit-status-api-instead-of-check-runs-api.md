# ADR-001: Commit Status API Instead of the Check Runs API

## Status
Accepted

## Context

The requirements doc (`project_plans/pr-comment-check-runs/requirements.md`) asks, literally, for automation to "surface [status] through GitHub's native Check Run API (or PR status checks)" instead of posting a comment. Those are presented as roughly interchangeable — they are not.

GitHub exposes two distinct, non-overlapping mechanisms for attaching pass/fail state to a commit:

1. **Check Runs** (`POST /repos/{owner}/{repo}/check-runs`, `PATCH /repos/{owner}/{repo}/check-runs/{id}`) — the modern API, with structured `output.title`/`output.summary`/`output.text` (Markdown, up to 64K chars), per-line `annotations`, and a `pending → in_progress → completed` state machine.
2. **Commit Statuses** (`POST /repos/{owner}/{repo}/statuses/{sha}`) — the legacy, simpler API: one `state` (`pending`/`success`/`error`/`failure`), one `description` string, one optional `target_url`. No annotations, no rich Markdown detail pane, no create-then-patch lifecycle — each POST is a fresh, complete status.

**The two differ in who is allowed to write them.** VERIFIED against [GitHub REST API docs — Checks: runs](https://docs.github.com/en/rest/checks/runs) (fetched 2026-08-12, quoted in `research/pitfalls.md` §1a):

> "Write permission for the REST API to interact with checks is only available to GitHub Apps. OAuth apps and authenticated users can view check runs and check suites, but they are not able to create them."

This is corroborated by two independent GitHub community threads confirming the "Checks" permission was briefly available to fine-grained PATs and then removed ([discussion #129512](https://github.com/orgs/community/discussions/129512), [discussion #179545](https://github.com/orgs/community/discussions/179545)) — this is an intentional, current platform restriction, not a temporary gap or a documentation lag.

**This repo's entire GitHub auth stack is PAT/OAuth-token based, never a GitHub App:**
- `github/http_client.go` (`getGHToken`) — precedence `GITHUB_TOKEN` env → `GH_TOKEN` env → OS keychain (`GetKeychainToken()`).
- `github/cli_import.go` (`GetCLIToken`) — shells out to `gh auth token --hostname <host>`, i.e. the `gh` CLI's own OAuth user token.
- `github/keychain.go` — a per-host, per-account PAT vault; no App/installation-token concept.
- Grepping the whole repo for `GITHUB_APP|installation.*token|App.*Private.*Key|jwt|x-access-token` returns zero hits (VERIFIED, `pitfalls.md` §1a / `stack.md` §2).

Standing up a GitHub App solely to obtain `checks:write` — registering the app, installing it on the target repo(s), implementing JWT→installation-token exchange, storing and rotating a private key — is real, bounded new infrastructure and a new trust boundary. `build-vs-buy.md` §2 evaluated this explicitly (as one shape of "hosted GitHub App / Probot-based bot") and rejected it: this is a personal/small-team tool with a single-PAT credential model today, and a GitHub App installation is a materially bigger commitment than the capability this ticket asks for.

## Decision

**Target the Commit Statuses API (`POST /repos/{owner}/{repo}/statuses/{sha}`), not the Check Runs API, for this feature.**

Concretely:
- `github.SetCommitStatus` (`github/commit_status.go`) writes via `gh api -X POST repos/{owner}/{repo}/statuses/{sha}`, authenticated the same way every other GitHub *write* in this repo already is (`gh auth token` via `CheckGHAuth()`).
- The `CommitStatusState` type and its four values (`pending`/`success`/`error`/`failure`) map directly onto the Statuses API's `state` field — there is no `neutral`/`action_required`/`skipped`/`cancelled`/`timed_out` to model, because those are Check-Run-only conclusion values the Statuses API doesn't support.
- Rich detail (Markdown `output.text`, per-line `annotations`) is not available in this design. `description` is a single short string (GitHub truncates at 140 characters). Anything that genuinely needs rich prose stays a comment — this is itself part of what the comment-vs-status convention doc (Phase 1) has to state explicitly, not leave implicit.

This is a **deviation from the literal requirements ask**, and is the reason this decision gets its own ADR rather than being folded silently into the plan: the requirements doc names "Check Run API" by name, and what ships is a different, more limited API that happens to render in the same PR-header UI area.

## Consequences

### Positive
- No new dependency, no new trust boundary, no new credential type. `SetCommitStatus` slots into the exact same `gh`-CLI-shellout, `CheckGHAuth()`-gated pattern as `PostPRComment`/`MergePR`/`ClosePR`/`IsForkRepo`.
- Ships with the current single-PAT auth model — no GitHub App registration/installation/private-key-storage project has to happen first.
- Commit Statuses render in the same PR merge-box / `statusCheckRollup` aggregate that `web-app`'s existing `CIStatusBadge`/`deriveMergeabilityState` already consume (`ux.md` §3) — the UI payoff this feature wants requires no new frontend work either way, whether the underlying write is a Check Run or a Status.
- Statuses are idempotent-per-`(sha, context)` at the GitHub UI level (repeated POSTs to the same context just update what's shown) — no create-then-remember-an-id lifecycle to manage, unlike Check Runs' `pending → in_progress → completed` state machine, which would have required persisting a check-run ID somewhere across the life of a unit of work (`architecture.md` §3). This is a secondary win, not the primary reason for the choice, but it does simplify `Instance.SetCommitStatus` to a single stateless call.

### Negative
- No rich Markdown detail pane, no per-line annotations, no `neutral` conclusion for "ran but not pass/fail." Anything needing those stays a comment, and the convention doc must say so explicitly or a future skill author will assume statuses can carry more than they can.
- Commit Statuses are the older, arguably "legacy" API from GitHub's own framing — the same signal a Check Run would carry appears in the UI as "Commit Status" rather than "Check," which is a cosmetic difference `pitfalls.md` §1c notes is unlikely to matter for a single-user glance-and-decide workflow, but is a real, visible difference from what the requirements doc pictured.
- Still SHA-scoped: a force-push/rebase requires re-writing the status against the new head SHA, exactly as a Check Run would have required (`Instance.SetCommitStatus` handles this by always calling `RefreshPRInfo()` first — see `plan.md` Epic 2.2 — but this cost exists regardless of which API was chosen).

### Neutral
- If a future need genuinely requires Check-Run-only capabilities (annotations, rich Markdown, a `neutral` conclusion, or GitHub's own "Checks" tab specifically rather than the merge-box rollup), the path forward is Option 2 from `pitfalls.md` §1a: stand up a minimal GitHub App scoped only to `checks:write`, run it alongside the existing PAT-based auth for everything else. That is a new project, not an extension of this one, and is explicitly deferred rather than ruled out.
