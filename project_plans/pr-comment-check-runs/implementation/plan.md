# Implementation Plan: pr-comment-check-runs

**Feature**: Add a PAT-compatible commit-status write capability (Statuses API, not the GitHub-App-only Checks API) plus a documented comment-vs-status convention, so automation reports pass/fail state without posting a PR comment.
**Date**: 2026-08-12
**Status**: Ready for implementation
**ADRs**: ADR-001 (Commit Status API instead of Check Runs API), ADR-002 (Hybrid enforcement: doc convention + session-scoped Go primitive + standalone script, no mandatory RPC gate)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `CommitStatusState` | A Go named string type (`type CommitStatusState string`) with four constants (`CommitStatusStatePending/Success/Error/Failure`) mirroring the four literal values GitHub's Statuses API accepts for `state`. | Defined in `github/commit_status.go`. Chosen over a raw `string` per `type-driven-design` — makes illegal states unrepresentable. |
| `CommitStatusRequest` | A value object bundling `SHA`, `State`, `StatusContext`, `Description`, `TargetURL` for one status write; constructed only via `NewCommitStatusRequest`, which validates non-empty `SHA`/`StatusContext`/`Description`. | Prevents the 5-same-typed-string-parameter pile that `primitive-obsession-checklist.md` flags. |
| `StatusContext` | The GitHub Statuses API `context` field — the string identifying *which* check a status belongs to (GitHub groups/dedupes statuses by this string per SHA). This project's naming convention: `stapler-squad/<check-name>`, e.g. `stapler-squad/backlog-review`. | Field is named `StatusContext`, not `Context`, in Go structs to avoid colliding with `context.Context` in the same signatures. |
| `github.SetCommitStatus` | The low-level primitive (`github/commit_status.go`) that shells out to `gh api -X POST repos/{owner}/{repo}/statuses/{sha}` with the fields from a `CommitStatusRequest`. Returns only an error — no logging, no session awareness. | Peer to `github.PostPRComment`/`github.MergePR`/`github.IsForkRepo` in the same package; same `gh`-CLI-backed, `CheckGHAuth()`-gated shape. |
| `Instance.SetCommitStatus` | The session-scoped wrapper (`session/pr_tracking.go`) that re-fetches the PR's *current* head SHA via `RefreshPRInfo()` before writing, so a status write can never target a commit that a force-push/rebase has already superseded. | Mirrors `Instance.PostComment`'s `IsPRSession()` guard + logging shape. |
| `Instance.SetCommitStatusOrFallback` | Wraps `Instance.SetCommitStatus`; on a write failure, posts exactly one deduplicated PR comment (marked with `statusWriteFailureMarker`) reporting the failure, so a broken status-write path is never silently indistinguishable from "nothing to report." | The one place this feature still posts a comment on a failure path — by design, not an oversight. |
| `GitHubService.SetCommitStatus` | The ConnectRPC handler (`server/services/github_service.go`) exposing `Instance.SetCommitStatus` to any Go-session-scoped caller (today: available for backlog-shepherding code to call; not yet wired into shepherding logic itself — see Risk Control). | Thin RPC-to-primitive shim, same shape as the existing `PostPRComment` RPC. |
| `pr-status.py` | A standalone script (`~/.claude/skills/github-pr/scripts/pr-status.py`) giving ad hoc `gh`-CLI-driven Claude Code skill sessions (no stapler-squad session ID to resolve) the same "resolve head SHA, then set a status" capability, without going through the Go backend. | Mirrors `github-address-pr-comments`'s `pr-threads.py` in spirit; simpler in practice since it ships as a real file in `scripts/`, not an embedded-then-bootstrapped code block. |
| Comment-vs-Status Convention | The documented policy (`~/.claude/skills/github-pr/references/comment-vs-status-convention.md`, linked from `github-pr/SKILL.md`) stating when automation should call `PostComment`/`pr pr comment` vs. `SetCommitStatus`/`pr-status.py`. | Enforcement is advisory (a doc a fresh agent reads), not compiler-checked — see ADR-002 and `pitfalls.md`'s note that no code-level guard is possible here. |
| `statusWriteFailureMarker` | The HTML-comment marker string (`<!-- stapler-squad-status-write-failed -->`) prepended to any fallback comment, used to detect "have I already reported this failure" before posting a second one. | Same shape as `forwardSyncCloseComment`'s fixed-body dedup, but a marker substring rather than an exact-match body. |
| Comments-per-shepherded-PR metric | The AC2 measurement definition: count of automation-originated comments on PRs shepherded by backlog automation or `pr-ship`, excluding `forwardSyncCloseComment`'s fixed body and any human/bot (e.g. Copilot) comments. | Defined in the convention doc, not built as dashboard infra — out of scope per requirements. |
| `PRInfo.HeadSHA` | New field on the existing `github.PRInfo` struct, populated from `gh pr view --json headRefOid`. | Required because no existing code path exposes the PR's current head *commit SHA* (only `HeadRef`, the branch name) — Statuses are SHA-scoped, not branch-scoped. |

---

## Pattern Decisions

### Creative pass: alternatives considered for "where does the comment-vs-status decision get enforced"

1. **Mandatory Go RPC gate** — every GitHub write (comment or status) routes through a single ConnectRPC method that itself decides comment-vs-status. *Strength*: one call site, fully testable, no drift possible. *Weakness*: skills talk to GitHub via `gh` CLI directly today with no stapler-squad session ID to resolve (`github-pr`'s SKILL.md documents zero Go/RPC calls); forcing every ad hoc skill invocation through a resolved session ID is a bigger architecture change than this ticket's scope, and requirements explicitly rule out "redesigning backlog automation end-to-end."
2. **Doc-only convention, no new Go capability** — write the policy in `github-pr/SKILL.md` and stop; every caller (Go or skill) reads and follows it. *Strength*: zero new code, ships in an afternoon, matches the reality that most noise sources today aren't even in Go. *Weakness*: leaves backlog shepherding — the one flow that already runs as a long-lived `session.Instance` with `GitHubOwner`/`GitHubRepo`/`GitHubPRNumber` populated — with no typed, tested primitive to call; it would have to hand-roll `gh api` inline with zero test coverage, and the SHA-staleness pitfall (pitfalls.md §1d) has no natural place to be handled correctly.
3. **Hybrid — doc convention (policy) + session-scoped Go primitive (`Instance.SetCommitStatus`) + standalone script (`pr-status.py`) for ad hoc callers** — each caller type gets a primitive shaped for how it actually talks to GitHub today. *Strength*: matches the codebase's existing bifurcation exactly (Go-session-scoped code already exists for `PostComment`; ad hoc skills already have their own script pattern via `pr-threads.py`); the SHA-staleness handling lives once, correctly, in `Instance.SetCommitStatus`, and `pr-status.py` re-derives it independently for ad hoc callers. *Weakness*: two implementations of "set a commit status" (Go function, Python script) that must stay behaviorally consistent — accepted because both are thin wrappers around the same fixed GitHub REST contract with no business logic to diverge on (build-vs-buy.md §3: "no real algorithm here").

**Chosen: Option 3 (Hybrid).** See ADR-002.

### Pattern table

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Comment-vs-status enforcement (cross Go + skills) | Hybrid: doc convention + session-scoped Go primitive + standalone script | `architecture.md` §4; Creative pass Option 3 | Mandatory Go RPC gate for every caller (Creative pass Option 1) | Ad hoc `gh`-CLI-driven skill sessions have no stapler-squad session ID to resolve; requiring one is an out-of-scope architecture change |
| | | | Doc-only convention, no new Go capability (Creative pass Option 2) | Leaves backlog shepherding — the one flow already Go-session-scoped — without a typed, tested primitive, and no natural home for the SHA-staleness fix |
| `CommitStatusRequest` shape | Value object with a validating smart constructor (`NewCommitStatusRequest`) | `type-driven-design`; `primitive-obsession-checklist.md` | `SetCommitStatus(ctx, owner, repo, sha, state, context, description, targetURL string)` — 7 raw params, 6 of them `string` | Six same-typed string params representing distinct concepts is exactly the smell the checklist names; a call-site argument swap (e.g. `description` and `context` transposed) would compile silently |
| `CommitStatusState` | Named string type + 4 constants | `golang-naming`; `type-driven-design` | Raw `string` for `state` | GitHub's API accepts exactly 4 literal values; a typed enum rejects a 5th at construction time instead of failing at the GitHub API call |
| `github.SetCommitStatus` transport | `gh api -X POST` shellout, new file `github/commit_status.go` | ADR-020 (direct-HTTP-for-GraphQL doesn't apply to a plain REST POST); `build-vs-buy.md` §1; existing `IsForkRepo` `gh api` pattern | Raw `net/http` POST authenticated with the keychain-managed PAT (mirroring `backlog_plugin_github_prs.go`'s *read* path) | Every existing GitHub *write* in this repo (`PostPRComment`, `MergePR`, `ClosePR`) goes through `gh` CLI; a second write-auth path (keychain PAT) for one new function fragments the auth model with no offsetting benefit |
| | | | Add `google/go-github` for its typed `ChecksService`/commit-status support | Repo has zero GitHub SDK dependency today, by deliberate, repeatedly-verified convention (`stack.md` §1, `build-vs-buy.md` §1); one write endpoint doesn't justify a new ~40-file dependency |
| Status-write failure handling | Plain function (`SetCommitStatusOrFallback`) + a marker-string dedup guard reusing the existing `GetPRComments`/`PostComment` primitives | `ux.md` §4; `forwardSyncCloseComment`'s "no silent automated action" precedent | Silent drop on write failure | A silently-dropped status is indistinguishable from "genuinely nothing to report" — defeats this feature's entire trust goal (ux.md's "emotional job" analysis) |
| | | | Invent a third notification channel (e.g. a dedicated failure-log surface) | Reuses the one channel ("comment = you need to look at this") the whole feature is designed around, instead of adding a new one users have to learn |
| Ad hoc (non-session) skill writes | Standalone script `pr-status.py`, shipped as a real file in `github-pr/scripts/` | `architecture.md` §4; existing `github-pr/scripts/*.sh` precedent | Extend `pr-threads.py`'s embedded-in-markdown + bootstrap-copy pattern | `pr-threads.py`'s embed-then-copy pattern exists for a different skill's own reasons; `github-pr` already ships real script files directly in `scripts/` (`pr-ci-failures.sh` etc.) — no bootstrap step needed for a script living in the same skill directory |
| Checks API vs Statuses API | Statuses API (`POST /repos/{o}/{r}/statuses/{sha}`) | `pitfalls.md` §1a, VERIFIED against GitHub REST docs | Real Check Runs API (`POST .../check-runs`) as literally named in requirements | GitHub restricts Checks API *writes* to GitHub Apps; this repo's entire auth stack is PAT/OAuth-token (`gh auth token`, keychain PAT) — see ADR-001 |
| | | | Stand up a new GitHub App scoped to `checks:write` | Real, bounded new infra/trust boundary (app registration, installation, private-key storage) the requirements don't ask for; listed as a deferred option in `pitfalls.md`, not adopted here |

---

## Migration Plan

Omitted — no schema or data changes. `make proto-gen` regenerates Go/TS bindings from a proto addition (Task 2.3.1a/b), which is a code-generation step, not a data migration; no `session/ent/schema` changes are involved.

## Observability Plan

- **Logs**: `Instance.SetCommitStatus` and `Instance.SetCommitStatusOrFallback` log via the existing `log.Info`/`log.Error` calls at the same level of detail as `Instance.PostComment` (session title, PR number, status context, outcome). The low-level `github.SetCommitStatus` function stays log-free, matching the existing split where low-level `github/*.go` functions return only errors and the `session` layer owns logging.
- **Metrics**: the "comments-per-shepherded-PR" metric (Domain Glossary) is a documented, scriptable `gh` query in the convention doc — not new dashboard/telemetry infrastructure. No new metrics are emitted by the Go binary for this feature; it's additive API-writing capability, not a hot path worth instrumenting on its own.
- **Alerts**: none. This is a personal/small-team tool (`build-vs-buy.md` §2); a failed status write already has its own visible signal — the one-shot fallback comment (Epic 2.4) — so no separate paging/alerting surface is warranted.

## Risk Control

- **Feature flag**: none. Every task in this plan is additive: a new file (`github/commit_status.go`), new methods on existing types, a new RPC, and new doc/script files. Nothing in this plan modifies or calls into any *existing* comment-posting call site (`PostPRComment`, `Instance.PostComment`, `forwardSyncCloseComment`) — those are all confirmed unchanged by every task above. **Important scope note for AC2's falsifiability**: `features.md`'s audit found no fixed backend call site currently posting the kind of "CI retriggered and green"-style status comment described in the requirements' Problem Statement — that behavior, if it exists at all, is ad hoc inline `gh pr comment` typed by an agent session at runtime, not a code path this plan can migrate. This plan therefore *builds the capability and the convention*; the actual measured comment-volume drop (AC2) only materializes once a future backlog-shepherding change starts calling `GitHubService.SetCommitStatus` in place of an ad hoc comment. Wiring that call is out of scope here (out of scope per requirements: "redesigning backlog automation end-to-end") — treat it as this plan's natural follow-up, not a gap in this plan.
- **Rollback procedure**: plain `git revert` of this feature's commits. No other code path depends on `github.SetCommitStatus`/`Instance.SetCommitStatus`/`GitHubService.SetCommitStatus` yet (see feature flag note above), so rollback has zero blast radius on any shipping behavior.
- **Staged rollout**: N/A — no traffic-facing rollout. Task 4.2.1a is a one-time manual verification against a single real scratch PR before the feature is considered done; there is no phased percentage rollout to design since nothing auto-invokes the new capability yet.

## Unresolved Questions

- [ ] Does the `gh auth token` session actually used by this app's `gh api` calls carry the Statuses API's required scope (`repo:status` on a classic PAT, or the fine-grained PAT "Commit statuses: write" permission)? `stack.md`/`build-vs-buy.md` confirm the *Checks* API rejects fine-grained PATs outright, but neither research doc explicitly re-verifies the *Statuses* API's fine-grained-PAT compatibility — it's inferred, not VERIFIED. — blocks Task 4.2.1a (first real `SetCommitStatus` call against a live PR) — owner: whoever runs Task 4.2.1a; run `gh api -X POST repos/<scratch-repo>/statuses/<scratch-sha> -f state=success -f context=stapler-squad/test -f description=test` once against a disposable test PR before treating Phase 2 as done, and record the result (success or the exact 403/422 body) in that task's notes.
- [ ] Does the Statuses API behave identically against any GitHub Enterprise Server host this repo's `github/keychain.go` per-host token model implies is in scope, or is v1 github.com-only? — blocks any GHE-hosted PR relying on this feature — owner: whoever first uses this feature against a GHE remote; until then, Task 1.1.1a's convention doc should state the scope limit explicitly (github.com-verified only) rather than silently assume parity.

## Dependency Visualization

```
Phase 1: Convention doc (defines "stapler-squad/<name>" naming + the rule itself)
  Epic 1.1 (Story 1.1.1, 1.1.2)
        │
        │  (naming convention + rule text feeds both Go code and script/doc)
        ├────────────────────────────┬───────────────────────────┐
        ▼                            ▼                           │
Phase 2: Go backend capability   Phase 3: pr-status.py script     │
  Epic 2.1 SetCommitStatus         Epic 3.1                       │
  primitive (github package)         │                            │
        │                            │                            │
        ▼                            │                            │
  Epic 2.2 HeadSHA +                 │                            │
  Instance.SetCommitStatus           │                            │
        │                            │                            │
        ├──────────────┐             │                            │
        ▼              ▼             │                            │
  Epic 2.3 RPC    Epic 2.4 Failure   │                            │
  exposure        fallback comment   │                            │
        │              │             │                            │
        └──────┬───────┴─────────────┴────────────────────────────┘
               ▼
Phase 4: Measurement definition (AC2) + AC4 verification
  Epic 4.1 (needs convention doc's terms)   Epic 4.2 (needs Phase 2 shipped, for the manual check)
```

Phase 2 and Phase 3 can be implemented in parallel by different subagents once Phase 1's Story 1.1.1 (which fixes the `stapler-squad/<name>` context-naming convention both will reference) is done. Epic 2.3 and Epic 2.4 both depend on Epic 2.2 (`Instance.SetCommitStatus`) but not on each other, and can also run in parallel.

---

## Phase 1: Comment-vs-Status Convention (Documentation/Policy)

### Epic 1.1: Written convention accessible to future skill authors
**Goal**: A single authoritative, discoverable document exists stating when automation should post a comment vs. set a commit status — satisfying AC1 — and the two existing skills most likely to grow new status-reporting behavior point at it.

#### Story 1.1.1: Author the comment-vs-status convention doc
**As a** future skill author (human or agent), **I want** one authoritative doc defining the comment-vs-status rule and the status-context naming convention, **so that** I don't have to re-derive the policy from scratch or guess a `context` string that collides with another check.

**Acceptance Criteria** (maps to requirements AC1):
- A clear, written convention exists, discoverable by future skill work.
  - *Given* the file `~/.claude/skills/github-pr/references/comment-vs-status-convention.md` does not exist yet, *When* Story 1.1.1's tasks complete, *Then* the file exists and states: (a) post a comment only for something requiring a human decision/reaction (a blocker, a question, a conflict); (b) call `SetCommitStatus`/`pr-status.py` for anything with a clean pass/fail/state shape; (c) GitHub Issues have no status/check channel at all — comments remain the only option there (per `pitfalls.md` §1a's "Issues, not PRs" note); (d) on a status-write failure, post exactly one deduplicated fallback comment rather than silently dropping the signal; and (e) it is linked from `github-pr/SKILL.md`'s "Progressive Context" section, the same mechanism already used for `references/api-patterns.md`.

**Files**: `~/dotfiles/.claude/skills/github-pr/SKILL.md`, `~/dotfiles/.claude/skills/github-pr/references/comment-vs-status-convention.md`

##### Task 1.1.1a: Write the convention reference doc (~4 min)
- Create `~/.claude/skills/github-pr/references/comment-vs-status-convention.md` (repo path: `~/dotfiles/.claude/skills/github-pr/references/comment-vs-status-convention.md`) covering: the rule itself with a short decision table (comment vs. status, each row an example — "CI retriggered and green" → status; "found a merge conflict, need your call" → comment); the `stapler-squad/<check-name>` context-naming convention with two concrete example strings (`stapler-squad/backlog-review`, `stapler-squad/code-review`); the GitHub-Apps-only Checks API constraint and why this project targets the Statuses API instead (one paragraph, cites `pitfalls.md`'s finding in spirit — no need to re-cite the research doc itself since it won't ship with the skill); the issues-have-no-status-channel caveat; the fallback-comment-on-failure behavior and its dedup marker; and a "how to call this" pointer table (`Instance.SetCommitStatus` for Go-session-scoped callers, `pr-status.py` for ad hoc skill sessions).
- Files: `~/dotfiles/.claude/skills/github-pr/references/comment-vs-status-convention.md`

##### Task 1.1.1b: Add a summary section + link in github-pr/SKILL.md (~3 min)
- Add a new `## Status Reporting: Comment vs. Commit Status` section to `~/.claude/skills/github-pr/SKILL.md`, inserted after `## gh api for Advanced Queries` and before `## Token Optimization Patterns` — a 5-line summary of the rule (post a comment only for something needing human reaction; everything else is a commit status) plus a link to `references/comment-vs-status-convention.md`.
- Files: `~/dotfiles/.claude/skills/github-pr/SKILL.md`

##### Task 1.1.1c: Wire the new doc into Progressive Context (~2 min)
- Add `references/comment-vs-status-convention.md` as a new bullet under the existing `## Progressive Context` section (alongside the existing `references/api-patterns.md` pointer) and add a `pr-status.py` row to the existing `## Quick Reference` table (`| Set a commit status | \`python3 ~/.claude/skills/github-pr/scripts/pr-status.py set --owner O --repo R --pr N --context C --state S --description D\` |`).
- Files: `~/dotfiles/.claude/skills/github-pr/SKILL.md`

#### Story 1.1.2: Point the highest-risk drift spots at the convention
**As a** maintainer editing `pr-ship` or `code:review` in the future, **I want** a one-line pointer exactly where a new "post a status comment" step would naturally get added, **so that** the convention gets consulted before old comment-heavy defaults creep back in (`pitfalls.md`'s "silent reversion risk" §2).

**Acceptance Criteria**:
- The convention is followed by the relevant skills, and stays followed as they change.
  - *Given* `~/.claude/skills/github/skills/pr-ship/SKILL.md`'s `## Exit Condition` section (which today reports success only in-session, per `features.md`'s audit — no GitHub write), *When* Story 1.1.2's task completes, *Then* a one-line blockquote appears directly above `## Exit Condition` reading `> Before adding any new PR status reporting here, check the comment-vs-status convention in `~/.claude/skills/github-pr/references/comment-vs-status-convention.md` — pass/fail-shaped state belongs in a commit status, not a comment.` — so a future edit that adds a completion comment is redirected to the convention first.

**Files**: `~/dotfiles/.claude/skills/github/skills/pr-ship/SKILL.md`, `~/dotfiles/.claude/skills/code/skills/review/SKILL.md`

##### Task 1.1.2a: Add the pointer to pr-ship (~2 min)
- Insert the one-line blockquote above `## Exit Condition` in `~/.claude/skills/github/skills/pr-ship/SKILL.md`.
- Files: `~/dotfiles/.claude/skills/github/skills/pr-ship/SKILL.md`

##### Task 1.1.2b: Add the pointer to code:review (~2 min)
- Insert the same one-line blockquote in `~/.claude/skills/code/skills/review/SKILL.md`, placed near wherever it documents presenting/reporting review findings (this skill currently presents findings in-session only, per `features.md`'s audit — no GitHub write to redirect, same rationale as pr-ship).
- Files: `~/dotfiles/.claude/skills/code/skills/review/SKILL.md`

---

## Phase 2: Go Backend Commit-Status Write Capability

### Epic 2.1: `CommitStatusRequest` type + `github.SetCommitStatus` primitive
**Goal**: A typed, tested, `gh`-CLI-backed function exists that can write a single commit status, following the exact conventions of `github.PostPRComment`/`github.IsForkRepo`.

#### Story 2.1.1: Add the low-level commit-status writer
**As** the Go backend, **I want** a typed `github.SetCommitStatus` function, **so that** any Go-session-scoped caller can report pass/fail state without hand-rolling a `gh api` call.

**Acceptance Criteria** (supports requirements AC3):
- Routine pass/fail state is settable via a status-check write, not a comment.
  - *Given* a valid `CommitStatusRequest{SHA: "abc123def4567890abc123def4567890abc123d", State: github.CommitStatusStateSuccess, StatusContext: "stapler-squad/backlog-review", Description: "Backlog auto-review passed"}` and `repo, _ := github.NewRepoRef("tstapler", "stapler-squad")`, *When* `github.SetCommitStatus(ctx, repo, req)` is called, *Then* it runs `gh api -X POST repos/tstapler/stapler-squad/statuses/abc123def4567890abc123def4567890abc123d -f state=success -f context=stapler-squad/backlog-review -f description="Backlog auto-review passed"` and returns `nil` on a successful (2xx) response, or a wrapped error including the `gh` stderr on failure.

**Files**: `github/commit_status.go`, `github/commit_status_test.go`

##### Task 2.1.1a: Define `CommitStatusState` and `CommitStatusRequest` (~4 min)
- Create `github/commit_status.go`. Define `type CommitStatusState string` with constants `CommitStatusStatePending = "pending"`, `CommitStatusStateSuccess = "success"`, `CommitStatusStateError = "error"`, `CommitStatusStateFailure = "failure"`. Define `type CommitStatusRequest struct { SHA string; State CommitStatusState; StatusContext string; Description string; TargetURL string }` (unexported validation only happens in the constructor; fields stay exported since this is a plain value object, not an invariant-guarding type like `RepoRef`). Add `func NewCommitStatusRequest(sha string, state CommitStatusState, statusContext, description string) (CommitStatusRequest, error)` returning an error if `sha`, `statusContext`, or `description` is empty, or if `state` isn't one of the 4 constants.
- Files: `github/commit_status.go`

##### Task 2.1.1b: Implement `SetCommitStatus` (~4 min)
- In `github/commit_status.go`, add `func SetCommitStatus(ctx context.Context, repo RepoRef, req CommitStatusRequest) error`. Follow `IsForkRepo`'s shape: call `CheckGHAuth()` first; build `gh api -X POST repos/{repo.Owner()}/{repo.Repo()}/statuses/{req.SHA} -f state={req.State} -f context={req.StatusContext} -f description={req.Description}`, adding `-f target_url={req.TargetURL}` only when `req.TargetURL != ""`; use `safeexec.CommandContext` with a 30s timeout (same as `PostPRComment`); on `*exec.ExitError`, wrap with the captured stderr, same pattern as `IsForkRepo`/`PostPRComment`.
- Files: `github/commit_status.go`

##### Task 2.1.1c: Unit tests for the constructor and state values (~5 min)
- Create `github/commit_status_test.go`. Table-test `NewCommitStatusRequest`: empty SHA → error; empty StatusContext → error; empty Description → error; an invalid `CommitStatusState("bogus")` → error; all 4 valid states with non-empty other fields → no error and fields round-trip correctly. (No test invokes the real `gh` subprocess — that's covered by Task 4.2.1a's manual verification, consistent with how `IsForkRepo`/`PostPRComment` have no subprocess-level unit test today either.)
- Files: `github/commit_status_test.go`

### Epic 2.2: `HeadSHA` + session-scoped wrapper
**Goal**: `Instance.SetCommitStatus` always targets the PR's *current* head SHA, so a write never lands on a commit a force-push/rebase already superseded (`pitfalls.md` §1d).

#### Story 2.2.1: Add `PRInfo.HeadSHA` and `Instance.SetCommitStatus`
**As a** session-scoped caller (backlog shepherding, once wired in a follow-up), **I want** `Instance.SetCommitStatus` to re-resolve the head SHA on every call, **so that** a status write against a stale SHA doesn't silently become a no-op nobody sees.

**Acceptance Criteria** (supports requirements AC3):
- *Given* an `Instance` with `GitHubOwner="tstapler"`, `GitHubRepo="stapler-squad"`, `GitHubPRNumber=123`, whose PR's current head SHA on GitHub is `def4567890abc123def4567890abc123def4567` (different from any SHA the session may have observed earlier, e.g. after an intervening rebase), *When* `instance.SetCommitStatus("stapler-squad/backlog-review", github.CommitStatusStateSuccess, "Backlog auto-review passed")` is called, *Then* it first calls `RefreshPRInfo()`, reads `def4567890abc123def4567890abc123def4567` from the response's `HeadSHA` field, and writes the status against that SHA — never a cached or caller-supplied one.

**Files**: `github/client.go`, `session/pr_tracking.go`, `session/pr_tracking_test.go`

##### Task 2.2.1a: Add `HeadSHA` to `PRInfo` (~3 min)
- In `github/client.go`: add `HeadSHA string \`json:"headRefOid"\`` to `PRInfo` (near `HeadRef`); add `HeadRefOid string \`json:"headRefOid"\`` to the internal `ghPRResponse` decode struct; add `headRefOid` to the `fields` string passed to `gh pr view --json` in `GetPRInfoCtx`; map `resp.HeadRefOid` → the returned `PRInfo.HeadSHA`.
- Files: `github/client.go`

##### Task 2.2.1b: Add `Instance.SetCommitStatus` (~4 min)
- In `session/pr_tracking.go`, add:
  ```go
  func (i *Instance) SetCommitStatus(statusContext string, state github.CommitStatusState, description string) error
  ```
  Guard with `IsPRSession()` (same error message shape as `PostComment`). Call `i.RefreshPRInfo()` to get the current `HeadSHA`. Build `repo, err := github.NewRepoRef(i.GitHubOwner, i.GitHubRepo)`. Build `req, err := github.NewCommitStatusRequest(prInfo.HeadSHA, state, statusContext, description)`. Call `github.SetCommitStatus(context.Background(), repo, req)`. Log via `log.Info`/`log.Error` at the same call sites and detail level as `PostComment` (session title, PR number, status context, outcome).
- Files: `session/pr_tracking.go`

##### Task 2.2.1c: Test `Instance.SetCommitStatus` (~4 min)
- Add `TestInstance_SetCommitStatus` to `session/pr_tracking_test.go` (check whether this file exists first — if not, create it following the naming/structure of the nearest existing `Instance` method test file). Cover: non-PR session → error, `RefreshPRInfo`/`SetCommitStatus` not called; PR session with `RefreshPRInfo` returning `HeadSHA: "abc..."` → the underlying `github.SetCommitStatus` call (via whatever seam `PostComment`'s existing tests use to avoid a real subprocess — mirror that exact pattern) receives that SHA.
- Files: `session/pr_tracking_test.go`

### Epic 2.3: RPC exposure for Go-session-scoped callers
**Goal**: `GitHubService.SetCommitStatus` exists as a typed, tested ConnectRPC method, giving future Go-session-scoped callers (e.g. a follow-up wiring backlog shepherding) a capability to call instead of shelling `gh` themselves.

#### Story 2.3.1: Add the `SetCommitStatus` RPC
**As** a future Go-session-scoped caller, **I want** `GitHubService.SetCommitStatus`, **so that** I can report status through the same RPC layer `PostPRComment` already uses.

**Acceptance Criteria**:
- *Given* `SetCommitStatusRequest{Id: "<valid-pr-session-uuid>", StatusContext: "stapler-squad/backlog-review", State: COMMIT_STATUS_STATE_SUCCESS, Description: "Backlog auto-review passed"}`, *When* `GitHubService.SetCommitStatus` is called, *Then* it returns `SetCommitStatusResponse{Success: true}`.
  - *Given* `Id` refers to a session where `IsPRSession()` is `false`, *When* called, *Then* it returns a `connect.CodeFailedPrecondition` error, mirroring `PostPRComment`'s existing non-PR-session handling.

**Files**: `proto/session/v1/session.proto`, `server/services/github_service.go`, `server/services/github_service_test.go`, `docs/registry/features/backend/pr/set-commit-status.json`

##### Task 2.3.1a: Add proto messages + RPC (~4 min)
- In `proto/session/v1/session.proto`, near `PostPRCommentRequest`/`PostPRCommentResponse`: add `enum CommitStatusState { COMMIT_STATUS_STATE_UNSPECIFIED = 0; COMMIT_STATUS_STATE_PENDING = 1; COMMIT_STATUS_STATE_SUCCESS = 2; COMMIT_STATUS_STATE_ERROR = 3; COMMIT_STATUS_STATE_FAILURE = 4; }`; add `message SetCommitStatusRequest { string id = 1; string status_context = 2; CommitStatusState state = 3; string description = 4; string target_url = 5; }`; add `message SetCommitStatusResponse { bool success = 1; string message = 2; }`. Add `rpc SetCommitStatus(SetCommitStatusRequest) returns (SetCommitStatusResponse) {}` to the service block directly below the existing `rpc PostPRComment(...)` line.
- Files: `proto/session/v1/session.proto`

##### Task 2.3.1b: Regenerate proto bindings (~2 min)
- Run `make proto-gen`. Verify `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` picked up the new messages/enum/RPC. Commit the generated files alongside the proto source in the same change, per this repo's proto workflow.
- Files: `session/gen/session/v1/*.go` (generated), `web-app/src/gen/session/v1/*_pb.ts` (generated)

##### Task 2.3.1c: Implement the handler (~4 min)
- In `server/services/github_service.go`, add `func (gs *GitHubService) SetCommitStatus(ctx context.Context, req *connect.Request[sessionv1.SetCommitStatusRequest]) (*connect.Response[sessionv1.SetCommitStatusResponse], error)`, mirroring `PostPRComment`'s exact shape: validate `req.Msg.Id`/`StatusContext`/`Description` non-empty → `findInstance` → `IsPRSession()` guard (`CodeFailedPrecondition`) → map the proto `CommitStatusState` enum to `github.CommitStatusState` (a small local `switch`) → call `instance.SetCommitStatus(...)` → wrap errors as `CodeInternal` → return `SetCommitStatusResponse{Success: true, Message: ...}`.
- Files: `server/services/github_service.go`

##### Task 2.3.1d: Test the handler (~4 min)
- Add `TestGitHubService_SetCommitStatus` to `server/services/github_service_test.go`, covering the same case shapes as that file's existing `PostPRComment` tests: unknown `Id` → not-found error; non-PR session → `CodeFailedPrecondition`; valid PR session → success response, underlying `Instance.SetCommitStatus` called with the expected arguments (mirror whatever test-double seam the existing `PostPRComment` tests use).
- Files: `server/services/github_service_test.go`

##### Task 2.3.1e: Feature registry entry (~2 min)
- Create `docs/registry/features/backend/pr/set-commit-status.json` mirroring `docs/registry/features/backend/pr/post-comment.json`'s shape: `{"id": "pr:set-commit-status", "type": "backend", "service": "SessionService", "method": "SetCommitStatus", "protoFile": "proto/session/v1/session.proto", "markerFound": false, "tested": true, "testIds": ["TestGitHubService_SetCommitStatus"], "lastModified": "<current ISO 8601 timestamp>"}`. Run `make registry-generate` and commit the regenerated aggregate `docs/registry/backend-features.json`.
- Files: `docs/registry/features/backend/pr/set-commit-status.json`, `docs/registry/backend-features.json` (generated)

### Epic 2.4: Failure fallback (status write fails → one deduplicated comment)
**Goal**: A status-write failure is always visible to the user, never silently indistinguishable from "all clear" (`ux.md` §4's "emotional job" analysis).

#### Story 2.4.1: Add `Instance.SetCommitStatusOrFallback`
**As the** user, **I want** a status-write failure to surface as a comment, exactly once, **so that** a broken write path (e.g. an auth scope gap) never masquerades as "nothing to report."

**Acceptance Criteria**:
- *Given* `Instance.SetCommitStatus` returns an error (e.g. wrapping a `gh api` 403 for a missing `repo:status` scope) and the PR currently has no comment containing `<!-- stapler-squad-status-write-failed -->`, *When* `instance.SetCommitStatusOrFallback("stapler-squad/backlog-review", github.CommitStatusStateSuccess, "Backlog auto-review passed")` is called, *Then* exactly one PR comment is posted whose body starts with `<!-- stapler-squad-status-write-failed -->` and includes the underlying error text, and the method still returns the original `SetCommitStatus` error to its caller.
- *Given* a PR comment already contains that marker, *When* `SetCommitStatusOrFallback` is called again and fails again, *Then* zero additional comments are posted.

**Files**: `session/pr_tracking.go`, `session/pr_tracking_test.go`

##### Task 2.4.1a: Implement `SetCommitStatusOrFallback` (~4 min)
- In `session/pr_tracking.go`, add `const statusWriteFailureMarker = "<!-- stapler-squad-status-write-failed -->"` and:
  ```go
  func (i *Instance) SetCommitStatusOrFallback(statusContext string, state github.CommitStatusState, description string) error
  ```
  Call `i.SetCommitStatus(statusContext, state, description)`. On success, return `nil`. On error: call `i.GetPRComments()`; if any existing comment's `Body` contains `statusWriteFailureMarker`, skip posting and return the original error; otherwise call `i.PostComment(fmt.Sprintf("%s\n\nAutomated status write to `%s` failed: %v", statusWriteFailureMarker, statusContext, err))` (log if the fallback comment itself fails, but still return the *original* `SetCommitStatus` error either way, so the caller's error handling doesn't change based on whether the fallback comment succeeded).
- Files: `session/pr_tracking.go`

##### Task 2.4.1b: Test the fallback + dedup (~4 min)
- Add `TestInstance_SetCommitStatusOrFallback` to `session/pr_tracking_test.go`. Three cases: (1) `SetCommitStatus` succeeds → `PostComment` never called. (2) `SetCommitStatus` fails, `GetPRComments` returns no marker comment → `PostComment` called exactly once with a body containing `statusWriteFailureMarker`. (3) `SetCommitStatus` fails, `GetPRComments` returns an existing comment containing the marker → `PostComment` never called; the original error is still returned in both (2) and (3).
- Files: `session/pr_tracking_test.go`

---

## Phase 3: Standalone Script for Ad Hoc Skill-Driven Writes

### Epic 3.1: `pr-status.py`
**Goal**: Any `gh`-CLI-driven skill session (no stapler-squad session ID to resolve) can set a commit status with the same SHA-freshness guarantee `Instance.SetCommitStatus` provides.

#### Story 3.1.1: Write and document `pr-status.py`
**As an** ad hoc skill session (`pr-ship`, `pr-refine`, `code:review`, or any future skill), **I want** a script that resolves the PR's current head SHA and sets a status, **so that** I have the same capability as a Go-session-scoped caller without going through the Go backend (`architecture.md` §4's finding that skills don't route GitHub calls through Go today).

**Acceptance Criteria** (supports requirements AC1, AC3):
- *Given* PR #123 in `tstapler/stapler-squad` whose current head SHA is `def4567890abc123def4567890abc123def4567`, *When* a skill runs `python3 ~/.claude/skills/github-pr/scripts/pr-status.py set --owner tstapler --repo stapler-squad --pr 123 --context stapler-squad/code-review --state success --description "No blocking findings"`, *Then* the script first runs `gh pr view 123 --repo tstapler/stapler-squad --json headRefOid --jq .headRefOid` to resolve `def4567890abc123def4567890abc123def4567`, then runs `gh api -X POST repos/tstapler/stapler-squad/statuses/def4567890abc123def4567890abc123def4567 -f state=success -f context=stapler-squad/code-review -f description="No blocking findings"`, and prints the resulting status's `html_url` (or the target-repo status page URL) on success, non-zero exit on failure.

**Files**: `~/dotfiles/.claude/skills/github-pr/SKILL.md`, `~/dotfiles/.claude/skills/github-pr/scripts/pr-status.py`

##### Task 3.1.1a: Write `pr-status.py` (~5 min)
- Create `~/.claude/skills/github-pr/scripts/pr-status.py` (repo path: `~/dotfiles/.claude/skills/github-pr/scripts/pr-status.py`), executable, with argparse subcommand `set` taking `--owner --repo --pr --context --state {pending,success,error,failure} --description [--target-url] [--hostname]`. Logic: resolve head SHA via `gh pr view <pr> --repo <owner>/<repo> [--hostname H] --json headRefOid --jq .headRefOid`; call `gh api -X POST repos/<owner>/<repo>/statuses/<sha> -f state=<state> -f context=<context> -f description=<description> [-f target_url=<target-url>]`; print the constructed status page URL and exit 0 on success, print `gh`'s stderr and exit 1 on failure. Follow `~/.claude/scripts/pr-threads.py`'s argparse/subcommand structure for consistency (single-file script, no external deps beyond `gh` on `PATH`).
- Files: `~/dotfiles/.claude/skills/github-pr/scripts/pr-status.py`

##### Task 3.1.1b: Document `pr-status.py` in github-pr/SKILL.md (~3 min)
- Add a `## Setting a Commit Status (pr-status.py)` section to `~/.claude/skills/github-pr/SKILL.md` (near the new `## Status Reporting: Comment vs. Commit Status` section from Task 1.1.1b) with the concrete example command from this story's AC, and add `scripts/pr-status.py` to the existing `## Progressive Context` "For PR analysis scripts: see `scripts/` directory" pointer (no change needed there beyond ensuring the new script is discoverable alongside `pr-ci-failures.sh` etc.).
- Files: `~/dotfiles/.claude/skills/github-pr/SKILL.md`

##### Task 3.1.1c: Note the no-bootstrap-needed distinction (~3 min)
- Add one sentence to the new `## Setting a Commit Status` section clarifying that, unlike `pr-threads.py` (which is embedded in `github-address-pr-comments/SKILL.md` and copied to `~/.claude/scripts/` on first use), `pr-status.py` ships as a real file directly in `github-pr/scripts/` — already on disk via the dotfiles symlink, callable at the fixed path with no bootstrap step, consistent with the skill's other `pr-*.sh` scripts.
- Files: `~/dotfiles/.claude/skills/github-pr/SKILL.md`

---

## Phase 4: Measurement Definition (AC2) and AC4 Verification

### Epic 4.1: Comments-per-shepherded-PR metric
**Goal**: AC2 ("comment volume drops measurably") is falsifiable — a precise metric definition exists before anyone claims the drop happened.

#### Story 4.1.1: Define the metric
**As** whoever judges this feature's success, **I want** a precise, documented metric for "automation comments per shepherded PR," **so that** AC2 can be checked against real numbers, not a vibe (`pitfalls.md` §4).

**Acceptance Criteria** (requirements AC2):
- *Given* a set of PRs merged in a chosen date range that were shepherded by backlog automation or `pr-ship` (excluding `forwardSyncCloseComment`'s fixed-body comments and any human/Copilot comments), *When* the documented procedure is run — `gh pr list --state merged --search "<date-range>" --json number,comments` fanned out per PR, each PR's comment list filtered to exclude the `forwardSyncCloseComment` fixed string (`"Closed automatically — the linked backlog item was marked done in stapler-squad."`) and any comment whose author isn't this app's automation identity — *Then* it produces a single "automation comments per shepherded PR" number for that range, directly comparable to the same procedure run again after future backlog-shepherding work adopts `SetCommitStatus`.

**Files**: `~/dotfiles/.claude/skills/github-pr/references/comment-vs-status-convention.md`

##### Task 4.1.1a: Document the metric procedure (~4 min)
- Add a `## Measuring Comment Volume (AC2)` section to `comment-vs-status-convention.md` defining: the population (backlog-shepherded + `pr-ship`-shepherded merged PRs only — explicitly excluding purely human-authored PRs, per `pitfalls.md`'s "Attribution" pitfall); the exclusions (the `forwardSyncCloseComment` fixed string, Copilot review comments, any human-authored comment); the exact `gh pr list`/`gh pr view` command sequence to compute comments-per-PR; and a restatement of AC2's second, non-numeric clause ("would I have needed to read this to know the PR was fine?") as a manual/LLM-graded sampling step for whatever comments do remain post-migration, since that half of AC2 isn't derivable from a count alone.
- Files: `~/dotfiles/.claude/skills/github-pr/references/comment-vs-status-convention.md`

### Epic 4.2: AC4 verification (no new code required)
**Goal**: Confirm AC4 is genuinely satisfied by GitHub's native UI once Phase 2 ships, with zero new stapler-squad UI work — per `ux.md` §3's finding that `checkConclusion`/`CIStatusBadge` already exist and comment count is natively rendered by github.com itself.

#### Story 4.2.1: Verify glanceability on a real PR
**As a** user triaging many concurrent PRs, **I want** to glance at a PR's checks and comment count on github.com and know whether it needs anything from me, **so that** I don't have to open every comment.

**Acceptance Criteria** (requirements AC4):
- *Given* PR #123 has exactly one commit status (`stapler-squad/backlog-review: success`, written via Task 2.3.1c/Epic 2.1's capability) and zero automation-originated comments, *When* the user opens `https://github.com/tstapler/stapler-squad/pull/123` in a browser, *Then* the merge-box shows a green check next to `stapler-squad/backlog-review` and the PR header's comment-count badge reflects only human-authored comments — both already rendered by GitHub's native PR page with no stapler-squad UI changes required.

**Files**: none — verification only, GitHub's native UI

##### Task 4.2.1a: Manual verification against a scratch PR (~3 min)
- After Phase 2 (Epics 2.1–2.3) ships, run `github.SetCommitStatus` once against a disposable scratch PR (e.g. via a throwaway `go run` snippet or a temporary test invoking `Instance.SetCommitStatus`), then confirm via `gh pr view <N> --json statusCheckRollup,comments --jq '{checks: .statusCheckRollup, commentCount: (.comments | length)}'` that the new status appears in `statusCheckRollup` and the comment count is unaffected. This is also the check for this plan's first Unresolved Question (Statuses API scope on the live `gh auth token`) — record the actual result (success, or the exact `gh api` error) in this feature's shipping PR description.
- Files: none (manual verification step, no file changes; results recorded in the PR description, not a repo file)
