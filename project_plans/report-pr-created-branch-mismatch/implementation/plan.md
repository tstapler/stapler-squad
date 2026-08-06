# Implementation Plan: report_pr_created cannot link a PR opened on a fallback branch

Project: `report-pr-created-branch-mismatch`. Source requirements:
`project_plans/report-pr-created-branch-mismatch/requirements.md`. Research:
`research/architecture.md`, `research/pitfalls.md`, `research/build-vs-buy.md`,
`research/features.md`, `research/ux.md` (same directory).

## Definition of Done (hard requirement — read before starting implementation)

**(Fourth plan-repair pass — pre-mortem.md P1 #4.)** This plan ships as **one
unit**, not two. Stories 1–4 (the relaxed branch-match check +
`override_reason` fallback) must **not** be merged or shipped without Story 6
(the reconciliation-automation guard, including Task 6.3a) also complete.
Story 3 is what *creates* the exposure Story 6 closes: relaxing
`report_pr_created`'s branch check means a wrongly-attached PR can now reach
`item.PrNumber`, and Story 6 is the only thing standing between that and
three automated GitHub-mutating/completing call sites in
`session/backlog_lifecycle.go` treating it as ground truth
(`closeIfSupersededByMain`'s auto-`ClosePR`, and two `IsPRMerged`-driven
auto-`done` transitions — see
`decisions/ADR-001-override-reason-security-model.md` and this plan's Risk
Control section). Shipping Stories 1–4 alone would silently reopen the exact
Blocker two rounds of adversarial review already found and fixed against
this plan — that is a scope regression, not an acceptable partial delivery,
regardless of how green Stories 1–4's own tests look in isolation.

There is exactly **one** build/test gate for this entire plan: **Task 6.7**,
run only after Stories 1–6 are all complete, with its checklist fully ticked.
Task 4.6 (Story 4's own build+test run) is an **intermediate checkpoint**,
not a shipping signal — passing it means Stories 1–4 compile and pass
together, nothing more. Do not open a PR, request review, or consider this
plan complete on the strength of Task 4.6/Story 1–4 passing in isolation;
proceed straight to Story 5 and Story 6.

Also required before shipping — not exercised by Task 6.7's `go test` run at
all, since it's behind a separate build tag precisely so it isn't run by
every CI invocation — is Task 1.3's live-GitHub verification (see Story 1
and pre-mortem.md P1 #1/#3): run it locally and paste its output into the PR
description or session notes before this plan is considered shippable.

## Step 0.5 — Creative pass: alternatives considered

**Approach A — Ancestry/compare-API fallback** (`architecture.md`'s original
recommendation: exact-branch fast path, then GitHub `compare` endpoint as a
fallback to prove the PR's head descends from the tracked branch).
*Strength:* zero new MCP-visible surface, purely internal to
`VerifyPRMatchesBranch`. *Weakness:* mathematically fails on the bug's actual
scenario — `pitfalls.md` §1 shows a deliberately history-severed clean branch
(the recommended recovery move: cut fresh from `origin/main` specifically to
shed a polluted branch's history) shares no discriminable commit ancestry with
the tracked branch, so `compare` reports `diverged`/no-common-ancestor exactly
as often for a legitimate recovery PR as for a totally unrelated one. It does
not solve the reported bug. **Rejected.**

**Approach B — Separate manual-override MCP tool** (`architecture.md`'s option
b / requirements' AC2 read literally: a new `link_pr_manually`/`relink_backlog_pr`
tool, gated by a stronger "operator" check). *Strength:* keeps
`report_pr_created`'s existing strict contract 100% untouched; a clean
audit-distinct escape hatch. *Weakness:* `pitfalls.md` §2 shows this
codebase has no operator/human auth primitive at all — `session.SessionRole`
is `work`/`triage`/`review`, all three LLM-driven. A new tool gated on
`role=work` is callable by the exact same session that made the original
mistake, so it buys no real additional safety over relaxing the existing
tool, while costing a new tool registration, a `docs/registry/` entry per
`.claude/rules/feature-registry.md`, a new hint-text block to keep in sync
with `report_pr_created`, and a second doc-comment to maintain. **Rejected**
as disproportionate to "a small, contained bug fix" (requirements.md).

**Approach C — Root-cause fix (number-keyed lookup) + same-tool relaxed
check gated by a mandatory `override_reason` argument** (chosen). Fixes the
actual root cause `build-vs-buy.md` identifies (branch-name-keyed GitHub
lookup silently returns "no PR" once the real head branch diverges) by
looking the PR up by number instead, which for free also gives the real head
branch for a fast, unchanged exact-match path. When it doesn't match, the
same tool accepts the PR only if it verifiably exists, belongs to the correct
owner/repo, and is open or merged — a real GitHub-verified check — *plus* a
caller-supplied `override_reason` string, logged loudly, as the procedural
deterrent `pitfalls.md` calls for in place of a technical human-gate that
doesn't exist in this codebase. *Strength:* smallest diff (one seam, no new
tool), actually fixes the reported scenario (no ancestry math required),
and — as a bonus — uses *fewer* GitHub calls per attempt than today's code
(see Observability Plan). *Weakness:* the residual protection against a
determined/hallucinating same-role session is procedural (a logged reason),
not technical — documented explicitly in `decisions/ADR-001-...md` rather
than overstated.

**Chosen: Approach C.**

## Step 1 — System type

This is a validation-logic bug fix inside one existing MCP tool handler
(`reportPRCreated`, Transaction Script) plus one new low-level data-fetch
function (`GetPRByNumber`). No domain model, no new persistence, no new
service layer. Patterns are chosen accordingly in Step 3 — most components
need none.

## Step 2 — Domain Glossary

| Term | Definition |
|---|---|
| `PRVerification` | New struct (`server/mcp/tools_github.go`) — the factual result of checking a self-reported PR against GitHub for `report_pr_created`: does it exist, does its head branch match, what is its real head branch and state. |
| `PRVerification.Exists` | `bool` — whether `prNumber` corresponds to a real pull request in `owner/repo` at all. `false` is always a definitive rejection, override or not. |
| `PRVerification.Matched` | `bool` — whether the PR's real GitHub head branch exactly equals the item's tracked branch (the fast path, unchanged pre-fix behavior). |
| `PRVerification.ActualHeadBranch` | `string` — the PR's real head branch as reported by GitHub; empty when `Exists` is `false`. |
| `PRVerification.State` | `string` — normalized PR state: `"open"`, `"closed"`, or `"merged"`. |
| `PRVerification.Author` | **(third plan-repair pass)** `string` — the PR's real GitHub author login as reported by GitHub; empty when `Exists` is `false`. Sourced from the REST single-PR endpoint's `user.login` field — **not** `author.login`, the `gh pr view --json` shape `ghPRResponse` (`github/client.go`) uses for the subprocess-based path. Used only by `decideOverridePolicy`'s author-match gate (Task 3.1); `VerifyPRMatchesBranch` itself stays a pure fact-reporter and makes no policy decision based on it. |
| `GetPRByNumber` | New function (`github/client.go`) — REST (no `gh` subprocess) lookup of a single PR by number; returns `github.ErrNoPR` (typed) when the number doesn't exist. The root-cause fix. Parses `user.login` into `PRInfo.Author` (Task 1.1, third plan-repair pass). |
| `VerifyPRMatchesBranch` | Existing function (`server/mcp/tools_github.go`), signature unchanged, return type changes from `(bool, error)` to `(PRVerification, error)`. Now calls `GetPRByNumber` instead of the branch-keyed `GetPRForBranch`. |
| `tracked branch` | The backlog item's own git branch (`backlog/<item-slug>`), resolved via `h.sessionBranch` → `GetWorktreeDataBySessionUUID`. |
| `fallback branch` | A differently-named branch (e.g. `feature/<slug>`) a work session opens a PR from when the tracked branch is polluted by another session's commits. |
| `fast path` | `verification.Matched == true` — behaves exactly as before this fix; no `override_reason` needed. |
| `fallback path` / `override path` | `verification.Exists == true && verification.Matched == false` — requires a non-empty `override_reason` argument and `verification.State` in `{open, merged}`. |
| `override_reason` | New optional MCP tool argument on `report_pr_created`. Required only on the fallback path; the caller's one-sentence explanation for why the PR's head branch differs, persisted only in the server log (not on the backlog item) as an audit trail. |
| `backlogHandlers.verifyPR` | Existing seam-wrapper method (`tools_backlog.go`); return type changes to match `VerifyPRMatchesBranch`. |
| `backlogHandlers.verifyPRMatchesBranch` | Existing overridable struct field (test seam); function type changes to return `PRVerification`. |
| `ErrNoPR` | Existing typed sentinel (`github` package), reused by `GetPRByNumber` for its 404 case — the same "definitive, not string-sniffed" contract `GetPRForBranch` already established. |
| `NewPRVerification` | New smart constructor (`server/mcp/tools_github.go`) — the only way `PRVerification` values are built, in both production code and test literals; enforces the `Matched ⇒ Exists` invariant so it can't silently diverge across the ~10 hand-rolled struct literals in tests. **Signature (third plan-repair pass)**: `NewPRVerification(exists, matched bool, actualHeadBranch, state, author string) PRVerification` — gained the `author` parameter to carry the PR's real GitHub author login through to `decideOverridePolicy`'s new author-match gate. `author` is not part of the `Matched ⇒ Exists` invariant this constructor enforces; no new invariant is added for it. |
| `decideOverridePolicy` | New pure function (`server/mcp/tools_backlog.go`) — extracts the existence/match/state/override-reason/**author-match** branching out of `reportPRCreated` into a unit-testable decision function, called once from the handler. **Signature (third plan-repair pass)**: `decideOverridePolicy(v PRVerification, overrideReason, callerLogin string) (accept bool, code connect.Code, msg string)` — gained a `callerLogin` parameter so the override branch can additionally require `v.Author == callerLogin`. Stays a pure function of its three inputs (no `ctx`, no I/O); `reportPRCreated` is still the only place that resolves `callerLogin`, via `resolveCallerGitHubLogin` below, and only when the override branch is actually reached. |
| `resolveCallerGitHubLogin` | **(third plan-repair pass)** New overridable function-field seam on `backlogHandlers` (`server/mcp/tools_backlog.go`), mirroring the existing `verifyPRMatchesBranch`/`resolveSessionBranch` seam pattern exactly: `func(ctx context.Context) (string, error)`. Defaults to `githubpkg.GetCurrentUserLogin` when nil; overridable in tests to avoid a real GitHub API call. `reportPRCreated` calls it (via the `h.callerGitHubLogin` wrapper, Task 2.2) only when `verification.Exists && !verification.Matched && overrideReason != ""` — i.e. only on a path that could actually be accepted by the override gate — never on the fast path or on a call that's already rejected for a missing `override_reason`, to avoid an unneeded GitHub round trip. |
| `github.GetCurrentUserLogin` | **(third plan-repair pass, reused primitive, unchanged by this plan)** Existing function (`github/client.go:196`) — resolves the GitHub login of the single identity the whole `stapler-squad` server process is authenticated as (env `GITHUB_TOKEN`/`GH_TOKEN`, else the first configured OS-keychain account — see `github/http_client.go`'s `getGHToken`). Reused here as "the calling session's own GitHub identity": this codebase has no per-session GitHub identity or token — every session's `gh`/git operations already authenticate as this same single (or first-configured) account — so this existing primitive is sufficient without inventing new session-identity plumbing. See `decisions/ADR-001-override-reason-security-model.md` for what collapsing "session identity" into "server identity" does and does not buy. |
| `github.PRStateOpen` / `PRStateClosed` / `PRStateMerged` | New `const` values (`github` package) — the single source of truth for the three PR-state strings `GetPRByNumber`'s normalization and `decideOverridePolicy`'s state gate both reference, instead of each hardcoding `"open"`/`"merged"`/`"closed"` independently. |
| `verifyPRHeadBranchMatchesTracked` | New guard method (`session/backlog_lifecycle.go`) — re-verifies, via a fresh `github.GetPRByNumber` call, that `item.PrNumber`'s real head branch still equals the item's current tracked branch, immediately before any of the three automated GitHub-mutating/completing call sites (`closeIfSupersededByMain`, `ReconcilePRPending`'s merge-detected done transition, `reconcileBouncingItems`'s `IsPRMerged`-driven done transition) treats `item.PrNumber` as ground truth. Fails closed (treats any resolution/lookup error as "not verified") — see Story 6. |

## Step 3 — Pattern Decisions

No new dependency. No stability/licensing/security concerns — everything
reuses primitives already in the repo (`ghHTTPClient`, `newGHRequest`,
`GhBaseURL` test-override var, the `backlogHandlers` function-field seam
pattern).

| Component | Pattern Chosen | Alternative Rejected | Reason |
|---|---|---|---|
| Overall fix shape | Root-cause number-keyed lookup + same-tool relaxed check with mandatory `override_reason` (Transaction Script — logic lives inline in `reportPRCreated`, no new domain type) | Ancestry/compare-API fallback (`architecture.md`'s recommendation) | `pitfalls.md` §1: mathematically indistinguishable from an unrelated PR once the recovery branch's history is deliberately severed from the tracked branch — doesn't solve the reported bug. |
| Overall fix shape (cont.) | (same as above) | Separate manual-override MCP tool (option b / AC2 read literally) | `pitfalls.md` §2: no operator/human role exists in this codebase (`work`/`triage`/`review` only); a second tool is exactly as callable by the same session as relaxing the existing one, for real added surface (new tool registration + feature-registry entry + hint-text sync). |
| GitHub data source for verification | New `GetPRByNumber` — REST, no subprocess (`github/client.go`), mirrors `GetPRForBranch`'s existing REST style | Reuse `GetPRInfoCtx` (`gh pr view` subprocess) + extend `--json` fields per `build-vs-buy.md`'s suggestion | A nonexistent PR number produces an untyped `gh` CLI stderr string from `GetPRInfoCtx`, forcing string-sniffing — exactly the anti-pattern `VerifyPRMatchesBranch`'s own doc comment (`tools_github.go:259-262`) says `GetPRForBranch` was built to avoid. REST gives a clean `404` → typed `ErrNoPR`. |
| Verification return shape | New `PRVerification` struct (4 fields) | Keep `(bool, error)` | A 2-value contract can't express the 3 distinct outcomes the fallback policy must branch on (matched / real-PR-different-branch / no-PR-at-all) — `pitfalls.md` §3 flags this exact gap ("no slot for 'one signal says no, the other is unknown'"). Struct is a plain data holder, not a speculative interface — no interface introduced, just a value type (type-driven-design, not GoF). |
| Where the override policy lives | Inside `reportPRCreated` (`tools_backlog.go`) — `VerifyPRMatchesBranch` stays a pure fact-reporter | Push the override/state-gate logic into `VerifyPRMatchesBranch` itself | Keeps "what does GitHub say" (general-purpose, unit-testable via `GhBaseURL` httptest override) separate from "should we trust it for this item" (caller-specific policy: item status, `override_reason` arg). Matches the Transaction Script framing from Step 1 — no Strategy/Chain-of-Responsibility needed for two `if` branches. |
| Test/override mechanism | Reuse the existing `backlogHandlers.verifyPRMatchesBranch` function-field seam, only its return type changes | Add a second seam/field for the fallback path | Requirements explicitly forbid a second competing seam mechanism; the existing seam's shape already carries everything the fallback path needs once its return type is a struct instead of a bool. |
| `PRVerification` construction | `NewPRVerification(exists, matched bool, actualHeadBranch, state, author string) PRVerification` smart constructor, called by both the real producer and every test literal (architecture-review.md Concern, Task 2.1; `author` param added, third plan-repair pass) | Keep the struct literal-constructible with no invariant enforcement | `Matched ⇒ Exists` is currently true only by convention across ~10 hand-rolled test literals; a smart constructor makes the illegal combination (`Matched: true, Exists: false`) either unconstructable or a loud assertion failure, per this repo's `type-driven-design` skill, for a few lines of cost. |
| Override-policy testability | Extract `decideOverridePolicy(v PRVerification, overrideReason, callerLogin string) (accept bool, code, msg string)` as a pure function, called once from `reportPRCreated` (architecture-review.md Concern, Task 3.1; `callerLogin` param added, third plan-repair pass) | Leave the branching inline in `reportPRCreated` | Lets Tasks 4.1-4.5 (plus Task 4.4a, added third plan-repair pass) become table-driven unit tests against the pure function instead of full-handler integration tests (mock storage, item/session fixtures) — same Transaction Script shape, no new type, cheaper tests. |
| PR state representation | `github.PRStateOpen`/`PRStateClosed`/`PRStateMerged` string constants, referenced by both `GetPRByNumber`'s normalization and `decideOverridePolicy`'s state gate (architecture-review.md Concern) | Leave `"open"`/`"merged"`/`"closed"` as independently hardcoded literals at each call site | Two call sites (plus a third pre-existing one at `session/backlog_lifecycle.go:2663`) already hardcode these strings with no shared source of truth; a few lines of `const` removes the duplication this fix would otherwise add to. |
| Reconciliation-automation exposure (ADR-001 blocker) | Guard `closeIfSupersededByMain` / the two `IsPRMerged`-driven done transitions with a fresh `github.GetPRByNumber`-based branch-match re-check at the point of GitHub mutation, using the same primitives Story 1/2 already introduce (Story 6) | (a) Persist a `PrLinkedViaOverride` boolean on the backlog item (ent schema field + migration + generated-code regen); (b) embed a distinguishable marker in the existing free-text `notes` field and grep for it | (a) is a full ent schema migration (`session/ent/schema/backlog_item.go` + `go generate ./session/ent/` regenerating everything under `session/ent/`) for a bug-fix-scoped plan — disproportionate per Approach C's own "smallest diff" framing, and the guard-at-consumption-site approach gives the identical safety property (no auto-mutation on a branch-mismatched PR) without it. (b) is fragile (free-text parsing, no compiler/type backing) and provides no more information than a live re-check already gives for free. The guard also only fires once per item, at the moment of an actual GitHub mutation (PR already detected merged, or item's commit already confirmed on main) — not on every reconciliation tick — so it does not add a recurring per-tick GitHub call for the common (non-override) case. **(Second plan-repair pass, Task 6.3a)**: the same guard is additionally re-run at the two `fixCtx`-building sites `closeIfSupersededByMain`'s `false` return falls through to, so the spawned fix-session is told when the PR association is unverified instead of being handed it as fact; unlike the mutation-site guard, this one does run on every tick that reaches a fix-spawn attempt for an affected item (not just once at a would-be mutation) — see the Observability Plan's "second plan-repair pass" bullet for the accepted cost. |
| Override-path acceptance tightening (**third plan-repair pass** — cross-artifact consistency blocker) | Add a PR-author-match requirement to the override path: `decideOverridePolicy` gains a `callerLogin` parameter and rejects when `v.Author != callerLogin`, resolved via the existing `github.GetCurrentUserLogin` (through a new `resolveCallerGitHubLogin` seam) — no per-session identity plumbing invented, since none exists in this codebase (every session already authenticates to GitHub as the same single server-wide identity) | (a) Do the author check as a separate step inside `reportPRCreated`, outside `decideOverridePolicy`; (b) require a distinct "operator" role | (a) would split the override path's five acceptance criteria (exists/matched/reason/author/state) across two places instead of one pure, table-driven-tested function — the same reasoning that motivated Task 3.1's original extraction (architecture-review.md Concern, Lens 1) argues against re-fragmenting it now. (b) restates the already-rejected Approach B / `pitfalls.md` §2 finding above: no operator role exists in this codebase. This row **tightens** the already-chosen Approach C; it is not a fresh alternatives analysis. It narrows "unrelated PR" from "any real PR in the repo" down to "a PR this same GitHub identity authored," closing the gap a cross-artifact consistency check found between requirements.md's "a PR that has no relationship whatsoever to the item's work must still be rejected" constraint and Task 4.4's test, which — before this pass split it into Task 4.4 (closed-state) and Task 4.4a (open/merged, different author) — could only prove the closed-state case. |

## Step 4 — Tasks

Each task lists exact files and is sized 2–5 minutes.

### Story 1 — Root-cause fix: look up the PR by number, not by branch name

**Task 1.1** — Add `PRStateOpen = "open"`, `PRStateClosed = "closed"`,
`PRStateMerged = "merged"` `const` values to `github/client.go` (placed next
to the `PRInfo` type), the single source of truth for the three PR-state
strings this plan introduces two independent consumers of (architecture-review.md
Concern — Lens 2 item 5). Add `GetPRByNumber` to `github/client.go` (new
function, placed after `GetPRForBranch`, ~line 428). REST
`GET repos/{owner}/{repo}/pulls/{number}` via the existing
`newGHRequest`/`ghHTTPClient` pair (no `gh` subprocess). Returns
`github.ErrNoPR` on `404`. Parses `number`, `head.ref`, `base.ref`, `state`,
`merged`, `html_url`, and `base.repo.full_name` from the response; **before**
returning success, compares `base.repo.full_name` against the requested
`owner+"/"+repo` and returns a non-`ErrNoPR` error on mismatch — a one-line
defensive check closing the "does this trust the request URL's owner/repo
blindly" gap (adversarial-review.md Concern). Parses the rest into a
`*PRInfo` (no new `PRInfo` fields needed — `Number`/`HeadRef`/`BaseRef`/`State`/`HTMLURL`/`Author`
already exist); normalizes `State` to `PRStateMerged` when the response's
`merged` boolean is `true`, otherwise passes through GitHub's `PRStateOpen`/`PRStateClosed`.
**(Third plan-repair pass)** Also parses `user.login` into `PRInfo.Author` — the
raw REST single-PR endpoint's author field is `user.login`, **not** `author.login`,
the `gh pr view --json` shape `ghPRResponse` (this same file, ~line 93) already
uses for the subprocess-based `GetPRInfoCtx` path. Copying that struct shape
here would silently leave `PRInfo.Author` empty for every REST-sourced
`PRInfo` — a gap the original version of this task left unaddressed (it
listed `number`/`head.ref`/`base.ref`/`state`/`merged`/`html_url`/`base.repo.full_name`
but not the author field at all, even though `PRInfo.Author` already exists
and `decideOverridePolicy`'s author-match gate, Task 3.1, now depends on it
being populated). Define a distinct local anonymous response struct for this
endpoint (e.g. `User struct{ Login string \`json:"login"\` } \`json:"user"\``)
rather than reusing `ghPRResponse`.
Files: `github/client.go`.

**Task 1.2** — New test file `github/client_pr_by_number_test.go` (package
`github`, no import needed for `GhBaseURL` — same package). Cases, each an
`httptest.Server` responding with a fixed JSON body/status:
- `TestGetPRByNumber_should_ReturnPRInfo_When_PRExists` — 200 with
  `head.ref="feature/ci-status-diff-viewer"`, `base.ref="main"`,
  `state="closed"`, `merged=true`, `user.login="tstapler"`, `base.repo.full_name`
  matching the requested owner/repo → asserts `State == github.PRStateMerged`
  **and `Author == "tstapler"`** (third plan-repair pass — pins that the REST
  `user.login` field, not `author.login`, is what populates `PRInfo.Author`,
  the field-name gotcha Task 1.1 calls out).
- `TestGetPRByNumber_should_ReturnErrNoPR_When_PRDoesNotExist` — 404 →
  `errors.Is(err, github.ErrNoPR)`.
- `TestGetPRByNumber_should_ReturnError_When_Forbidden` — 403 → non-nil,
  non-`ErrNoPR` error (transient-shaped).
- `TestGetPRByNumber_should_ReturnError_When_ServerError` — 500 → non-nil,
  non-`ErrNoPR` error (transient-shaped, mirrors `GetPRForBranch`'s existing
  status-code branches at `github/client.go:394-401`) — pins the shape
  `reportPRCreated` depends on to surface `ErrInternalError`/retryable rather
  than a hard reject (adversarial-review.md Concern).
- `TestGetPRByNumber_should_ReturnError_When_RepoFullNameMismatch` — 200 with
  a well-formed body but `base.repo.full_name` set to a *different*
  `owner/repo` than requested → non-nil error, confirming the new defensive
  check actually rejects rather than silently trusting the body.
Files: `github/client_pr_by_number_test.go`.

**Task 1.3** (**fourth plan-repair pass — resolves pre-mortem.md P1 #1/#3:
no task previously re-verified the fix against the real, confirmed-stuck PR
on live GitHub**) — New test file `github/client_pr_by_number_live_test.go`
(package `github`), gated behind a dedicated `//go:build live_github` tag —
deliberately **not** the existing `integration` tag this repo already uses
(`server/mcp/server_integration_test.go`, `session/mcp_integration_test.go`,
`session/tmux/server_registry_integration_test.go`, run via `make
test-integration`/`make ci`'s `go test -race -tags integration ./...`),
because that tag's own Makefile comment scopes it to "requires real tmux" —
this test's dependency is live network access to `api.github.com`, a
different and unrelated failure mode that would make the tmux-scoped
integration suite flaky on network hiccups for no reason. Checked
`github/*_test.go` for an existing network-gated/env-var-gated pattern first
(`grep -rln "testing.Short\|GITHUB_TOKEN\|t.Skip" github/*_test.go`) — none
exists there, so this introduces a new build tag rather than overloading an
existing one, consistent with how `integration`/`harness` are already scoped
per-concern elsewhere in this repo (`server/services/backlog_triage_harness_test.go`'s
`//go:build harness`).

One test: `TestGetPRByNumber_should_MatchRealGitHubPR_When_LivePR326` — calls
the real, unauthenticated `github.GetPRByNumber(context.Background(),
"tstapler", "stapler-squad", 326)` against real `api.github.com` (PR #326 is
real, already-merged, and publicly visible —
https://github.com/tstapler/stapler-squad/pull/326 — so no `GITHUB_TOKEN` is
required; GitHub's unauthenticated rate limit of 60 req/hour/IP is
comfortably enough for a single manual/CI run). Asserts:
- `err == nil`
- `info.HeadRef == "feature/ci-status-diff-viewer"`
- `info.State == github.PRStateMerged`
- `info.BaseRef` is non-empty (not pinned to an exact literal — not
  independently confirmed at plan-writing time — but its presence proves the
  `base.ref` parse path works against a real response)
- `info.Author` is non-empty, plus `t.Logf("real PR #326 author: %s",
  info.Author)` so whoever runs this can eyeball it against `gh pr view 326
  --json author` and confirm the `user.login`-not-`author.login` parsing fix
  from Task 1.1 actually produced the right value against real data, not just
  a hand-written httptest fixture
This is the one test in the whole plan that exercises the REST response
shape (`base.repo.full_name`, `user.login`, `head.ref`/`base.ref` nesting)
against real GitHub data — every other test (Tasks 1.2, 2.3, 4.0–4.5, 6.6)
uses `httptest.Server`-mocked or function-field-stubbed responses, none of
which can catch a wrong shape assumption; the fix could pass all of them and
still fail on first real use if, say, GitHub's real `base.repo.full_name`
format doesn't match what Task 1.1's defensive check expects. Not run by
default `go test ./...`, `make test`, `make test-race`, or `make
test-integration`/`make ci` — the `live_github` build tag excludes it from
all of them; it is a deliberately manual, out-of-CI check.

**Required before shipping — see the Definition of Done section above.**
Whoever implements this plan must run:
```
go test -tags live_github -run TestGetPRByNumber_should_MatchRealGitHubPR_When_LivePR326 -v ./github/...
```
locally, confirm it passes, and paste its `-v` output (including the logged
`Author` value) into the PR description or session notes before this plan is
considered shippable. This is a read-only GET against a real, already-merged,
publicly-visible PR — it makes no write and does not touch the
confirmed-stuck item `3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d`, so it does not
conflict with requirements.md's "no retroactive fix" scope decision.
Files: `github/client_pr_by_number_live_test.go`.

### Story 2 — `PRVerification` type + rewritten `VerifyPRMatchesBranch`

**Task 2.1** — In `server/mcp/tools_github.go`: add the `PRVerification`
struct (see Domain Glossary) directly above `VerifyPRMatchesBranch`
(~line 249). Add a smart constructor
`func NewPRVerification(exists, matched bool, actualHeadBranch, state, author string) PRVerification`
(**third plan-repair pass**: gained the trailing `author` parameter)
immediately below the struct definition that enforces the implicit
`Matched ⇒ Exists` invariant (`matched && !exists` is a construction error —
e.g. `log.ErrorLog.Printf` and force `matched = false` rather than panic, so
a future violation is loud in logs/tests but never crashes production) —
closes the "illegal state representable" gap architecture-review.md flagged
(Lens 2 item 6). `author` is not part of this invariant; no additional
enforcement is added for it. Rewrite `VerifyPRMatchesBranch`'s body to call
`githubpkg.GetPRByNumber(ctx, owner, repo, prNumber)` instead of
`GetPRForBranch(ctx, owner, repo, expectedBranch)`; on `ErrNoPR` return
`NewPRVerification(false, false, "", "", ""), nil`; on other error return
`PRVerification{}, err`; on success return
`NewPRVerification(true, info.HeadRef == expectedBranch, info.HeadRef, info.State, info.Author), nil`
(`info.Author` now reliably populated by Task 1.1's `user.login` parsing fix).
Rewrite the doc comment: explain the root-cause fix (number-keyed, not
branch-keyed), that `Matched == false` with `Exists == true` is now a
*possible-to-accept* fallback case (policy decided by the caller,
`reportPRCreated`, not by this function), that all `PRVerification`
values — production and test — must go through `NewPRVerification`, never a
bare struct literal, and (**third plan-repair pass**) that `Author` records
the PR's real GitHub author login, consumed only by `reportPRCreated`'s
override-path author-match gate (Task 3.1) — `VerifyPRMatchesBranch` itself
makes no policy decision based on it, consistent with staying a pure
fact-reporter.
Files: `server/mcp/tools_github.go`.

**Task 2.2** — In `server/mcp/tools_backlog.go`: change the
`verifyPRMatchesBranch` field type (struct doc block, ~line 99-102) from
`func(...) (bool, error)` to `func(...) (PRVerification, error)`; update
`h.verifyPR`'s signature and body (~line 605-610) to match, calling the
renamed return type through unchanged. **(Third plan-repair pass)** Also add
a new `resolveCallerGitHubLogin func(ctx context.Context) (string, error)`
field to the `backlogHandlers` struct (same struct block, alongside
`verifyPRMatchesBranch`/`resolveSessionBranch`), doc-commented in the same
shape as `resolveSessionBranch`'s existing comment: defaults to
`githubpkg.GetCurrentUserLogin` when nil; overridable in tests to avoid a
real GitHub API call. Add a thin `h.callerGitHubLogin(ctx context.Context)
(string, error)` wrapper method mirroring `h.verifyPR`'s existing
`if h.X != nil { return h.X(...) }` fallback pattern exactly, so
`reportPRCreated` (Task 3.1) calls one consistent accessor rather than
branching on the field directly.
Files: `server/mcp/tools_backlog.go`.

**Task 2.3** — In `server/mcp/tools_backlog_test.go`: update the 5 existing
`verifyPRMatchesBranch:` field literals to the new signature, preserving each
test's existing intent as a `PRVerification` value built via
`NewPRVerification` (never a bare struct literal, per Task 2.1's invariant).
**(Third plan-repair pass)** every `NewPRVerification` call below gains a
trailing `author` argument (`"tstapler"` as a placeholder value, chosen for
compile-shape/realism only — none of these 5 tests exercise the override
branch, so `author` is never actually compared against a resolved
`callerLogin` in any of them; see Task 4.0/4.1 for the tests that do):
- `TestReportPRCreated_should_TransitionToPRPending_When_ValidPR` (~line 812):
  `func(...) (PRVerification, error) { return NewPRVerification(true, true, "backlog/ship-it", github.PRStateOpen, "tstapler"), nil }`.
- `TestReportPRCreated_should_ReturnError_When_PersistFails` (~line 864): same
  shape as above.
- `TestReportPRCreated_should_NoOp_When_AlreadyPRPendingSamePR` (~line 911):
  same shape (never actually invoked — idempotency short-circuits first — but
  must compile).
- `TestReportPRCreated_should_RejectCall_When_BranchMismatch` (~line 985):
  `NewPRVerification(true, false, "totally-unrelated-branch", github.PRStateOpen, "tstapler"), nil` —
  and since the request in this test does **not** set `override_reason`, it
  rejects on the missing-reason check (Task 3.1 step 3a) before author is
  ever compared, so the assertions (still `ErrInvalidArgument`, item
  untouched) continue to hold unchanged, preserving this test's regression
  intent per requirements.md.
- `TestReportPRCreated_should_ReturnRetryableError_When_GitHubLookupTransientlyFails`
  (~line 1026): `func(...) (PRVerification, error) { return PRVerification{}, fmt.Errorf("GitHub API: rate limited (403)") }`
  (unchanged — doesn't use `NewPRVerification` at all).
Files: `server/mcp/tools_backlog_test.go`.

### Story 3 — Same-tool fallback path (resolves AC1 + AC2)

**Task 3.1** — In `server/mcp/tools_backlog.go`, add a new pure function
`decideOverridePolicy(v PRVerification, overrideReason, callerLogin string) (accept bool, code connect.Code, msg string)`
(architecture-review.md Concern — Lens 1 items 1 & 4: extracts the decision
out of the giant handler so it's table-driven-unit-testable on its own,
without mock storage/item/session fixtures; **`callerLogin` parameter added
in the third plan-repair pass**, closing a cross-artifact consistency
blocker — see below):
1. `if !v.Exists` → `accept=false`, `ErrInvalidArgument`, `"PR #%d does not exist in %s/%s on GitHub — refusing to record it. Double-check the PR number/URL."` (existence can never be overridden; `%d`/`%s` filled by the caller, not this function, which only returns the branch-specific fragment plus enough to format — see implementation note below).
2. `else if v.Matched` → `accept=true`, no error (fast path, unchanged — `callerLogin` is not consulted; the item's own tracked branch is already trusted).
3. `else` (`Exists && !Matched`):
   - `overrideReason == ""` → `accept=false`, `ErrInvalidArgument`, the AC3
     message (see Task 3.1's exact string in the Given-When-Then for AC3
     below).
   - **(third plan-repair pass, new)** `overrideReason != ""` but
     `callerLogin == "" || v.Author == "" || v.Author != callerLogin` →
     `accept=false`, `ErrInvalidArgument`, `"PR #%d was authored by %q, not
     your own GitHub identity (%q) — the override path can only attach PRs
     you authored yourself. Refusing to record it."` (`reportPRCreated` fills
     `%d`/`%q`/`%q` with `prNumber`/`verification.Author`/`callerLogin`; an
     empty `callerLogin` renders as `""`, which is itself informative — it
     means this server's GitHub identity could not be resolved at all).
     Checked **before** the state gate below — see rationale.
   - `overrideReason != ""`, author matches, but
     `v.State != github.PRStateOpen && v.State != github.PRStateMerged`
     → `accept=false`, `ErrInvalidArgument`, `"PR #%d is %s (not open or merged) — refusing to record it even with override_reason."`
   - otherwise → `accept=true`, no error (override path accepted).

   **Why author-match is checked before the state gate**: both are
   independent yes/no gates the override path must pass. When a PR fails
   both (e.g. a stranger's closed PR), the author-mismatch message is the
   more fundamental one to surface — it tells the caller *whose* PR this is,
   rather than inviting a "wait for it to merge and retry" misreading from
   the state message alone. Task 4.0's table pins this ordering explicitly
   with a combined case.
(Implementation note: since the exact message strings need `prNumber`/`owner`/`repo`/
branch names that `decideOverridePolicy` doesn't have, it returns a small
enum/code plus the invariant fields already on `v` and the caller's
`overrideReason`/`callerLogin`, and `reportPRCreated` formats the final
message string — keeps `decideOverridePolicy` a pure function of its inputs,
fully table-driven-testable, while `reportPRCreated` remains the one place
that knows the request's `prNumber`/`owner`/`repo`/tracked-branch context.)
In `reportPRCreated` itself (~line 707-715), replace the single `!matched`
reject with:
1. Parse optional `overrideReason, _ := args["override_reason"].(string)`;
   trim whitespace; if non-empty and `len(overrideReason) > 500`, return
   `ErrInvalidArgument` ("override_reason must be <= 500 characters").
2. Call `verification, verifyErr := h.verifyPR(...)`; unchanged transient-error
   handling (`verifyErr != nil` → `ErrInternalError`, "retry").
3. **(third plan-repair pass, new)** `callerLogin := ""`; if
   `verification.Exists && !verification.Matched && overrideReason != ""`
   (i.e. we're on a path `decideOverridePolicy` could actually accept — never
   on the fast path, the not-exists path, or a call already doomed to reject
   for a missing `override_reason`, so a mistaken retry-without-a-reason
   never pays for an identity lookup it doesn't need): call
   `login, loginErr := h.callerGitHubLogin(ctx)` (Task 2.2's new accessor);
   on `loginErr != nil`, return `ErrInternalError`,
   `"could not resolve your GitHub identity to verify the override — retry: %v"`
   (mirrors the existing transient-error shape used for `h.verifyPR`'s own
   failure); otherwise `callerLogin = login` (an empty string here —
   unauthenticated — is not itself an error; it flows into
   `decideOverridePolicy` and deterministically fails the author-match gate).
4. Call `decideOverridePolicy(verification, overrideReason, callerLogin)`;
   on `accept == false`, format and return the corresponding message per its
   code.
5. On `accept == true` **and** `!verification.Matched` (i.e. the override
   path was actually taken, not the fast path): fall through to the existing
   `SetBacklogItemPRAndTransition` call, then emit the audit log line (see
   Observability Plan): `log.Warn("report_pr_created: recording PR via override (head branch differs from tracked branch)", "session", callerUUID, "item", itemID, "pr_number", prNumber, "actual_head_branch", verification.ActualHeadBranch, "tracked_branch", branch, "pr_author", verification.Author, "override_reason", overrideReason)`
   (**`pr_author` field added, third plan-repair pass** — now that authorship
   is load-bearing in the acceptance decision, the audit trail should record
   it explicitly rather than leave it implicit).
6. On `accept == true` **and** `verification.Matched` → fast path, unchanged
   (no author resolution, no audit log line — only the override path is
   audited).
Files: `server/mcp/tools_backlog.go`.

**Task 3.2** — In `server/mcp/tools_backlog.go`'s tool registration block
(~line 1013-1037): add
`mcpgo.WithString("override_reason", mcpgo.Description("Only required when the PR's actual head branch (per GitHub) differs from this item's tracked branch — e.g. the tracked branch was polluted by another session sharing this worktree, so you opened the PR from a clean branch instead. Explain why in one sentence; it is recorded in the server log as an audit trail. Omit when the PR's head branch matches the tracked branch. The PR must also have been authored by this same GitHub identity — the override path cannot attach a PR someone/something else opened, even with a reason."))`
(**last sentence added, third plan-repair pass** — no `mcpgo.Required()` —
optional). Update the tool's top-level `mcpgo.WithDescription(...)` (~line
1015-1017) to mention the fallback case exists and is gated by
`override_reason` **and self-authorship**. Update the "work" role hint text
(~line 213) to append one sentence: `"If the PR's head branch differs from your tracked branch (e.g. you had to open it from a clean fallback branch), pass override_reason explaining why — do not just retry report_pr_created unchanged. This only works for a PR you opened yourself."`
(**final sentence added, third plan-repair pass**).
Files: `server/mcp/tools_backlog.go`.

### Story 4 — Tests for the new contract

**Task 4.0** (Lens 1 remediation — table-driven unit test for the pure
decision function) — New test `TestDecideOverridePolicy` in
`server/mcp/tools_backlog_test.go`, table-driven directly against
`decideOverridePolicy` (Task 3.1) — no mock storage, no item/session
fixtures. Cases, each built via `NewPRVerification` (Task 2.1) plus a
`callerLogin` argument to `decideOverridePolicy` (**third plan-repair pass**
adds the `callerLogin` dimension and two new rows below):
- not-exists (`NewPRVerification(false, false, "", "", "")`) → reject, "does
  not exist" message code, **regardless of `overrideReason` or
  `callerLogin`** (assert both empty and non-empty `overrideReason` reject
  identically) — pins that existence can never be overridden (this is the
  logic-level pin for what was previously only Task 4.5's full-handler
  assertion).
- matched (`NewPRVerification(true, true, "backlog/x", github.PRStateOpen, "tstapler")`)
  → accept, no error, regardless of `overrideReason` or `callerLogin` (author
  is never consulted on the fast path).
- mismatch, empty `overrideReason` (`NewPRVerification(true, false, "feature/y", github.PRStateOpen, "tstapler")`,
  `callerLogin="tstapler"`) → reject, AC3-shaped message code (logic-level
  pin for what was previously only Task 4.2/4.3's full-handler assertions) —
  proves the missing-reason check fires even when the author would otherwise
  match.
- **(new)** mismatch, non-empty `overrideReason`, `State == github.PRStateOpen`,
  author mismatch (`NewPRVerification(true, false, "feature/y", github.PRStateOpen, "someone-else")`,
  `callerLogin="tstapler"`) → reject, author-mismatch message code — the
  logic-level pin for the gate this plan-repair pass adds. This is what
  proves requirements.md's "a PR that has no relationship whatsoever to the
  item's work must still be rejected" now holds for the **open** case, not
  just the closed case Task 4.4 already covered.
- **(new)** mismatch, non-empty `overrideReason`, `State == github.PRStateClosed`,
  author mismatch (`NewPRVerification(true, false, "feature/y", github.PRStateClosed, "someone-else")`,
  `callerLogin="tstapler"`) → reject, **author-mismatch** message code, not
  the state-gate code — pins the check ordering from Task 3.1 (author before
  state) explicitly, so a PR that fails both gates surfaces the more
  fundamental rejection reason.
- mismatch, non-empty `overrideReason`, author matches
  (`NewPRVerification(true, false, "feature/y", github.PRStateClosed, "tstapler")`,
  `callerLogin="tstapler"`), `State == github.PRStateClosed` → reject,
  state-gate message code (logic-level pin for what was previously only Task
  4.4's full-handler assertion — now specifically isolates the state check
  from the author check by holding author constant).
- mismatch, non-empty `overrideReason`, author matches
  (`NewPRVerification(true, false, "feature/y", github.PRStateOpen, "tstapler")`,
  `callerLogin="tstapler"`), `State == github.PRStateOpen` → accept.
- mismatch, non-empty `overrideReason`, author matches
  (`NewPRVerification(true, false, "feature/y", github.PRStateMerged, "tstapler")`,
  `callerLogin="tstapler"`), `State == github.PRStateMerged` → accept (the
  confirmed AC1 repro shape).
This test is fast (no I/O, no mocks) and is what makes Tasks
4.2/4.4/4.4a/4.5 below thin wiring-confirmation tests instead of the primary
place the decision logic itself is pinned (architecture-review.md Concern —
Lens 1 items 1 & 4).
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.1** (AC1) — New test
`TestReportPRCreated_should_TransitionToPRPending_When_FallbackBranchWithOverrideReason`
in `server/mcp/tools_backlog_test.go`. Full-handler integration test (needs
real storage/persistence, not just the decision — Task 4.0 doesn't cover
this). Uses the confirmed real repro shape: item fixture via
`setupReportPRCreatedFixture` (status `review`), `resolveSessionBranch` →
`"backlog/stapler-squad-ci-status-diff-viewer"`, `verifyPRMatchesBranch` →
`NewPRVerification(true, false, "feature/ci-status-diff-viewer", github.PRStateMerged, "tstapler"), nil`,
**`resolveCallerGitHubLogin` → `"tstapler", nil`** (**third plan-repair
pass** — the real repro is in fact self-authored: the same account that
opened the fallback-branch PR is the one retrying `report_pr_created`, which
is exactly why the override path exists at all; this fixture now reflects
that truthfully, since authorship is load-bearing).
Request: `pr_url="https://github.com/tstapler/stapler-squad/pull/326"`,
`pr_number=326`, `summary="..."`, `override_reason="tracked branch had unrelated commits from a shared worktree; opened PR from a clean branch instead"`.
Assert: success result contains `"pr_pending"`; fetched item has
`Status == pr_pending`, `PrNumber == 326`; the `log.Warn` audit line (Task
3.1 step 5) was emitted with the expected fields, **including
`pr_author=tstapler`** — this is also the AC2 audit-trail assertion, folded
in here since it's the same call under test (the previous separate
audit-trail test is redundant now that AC2's core "gated by an explicit
reason" logic is pinned by Task 4.0).
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.2** (wiring confirmation — override path is real, not silently
bypassed) — New test
`TestReportPRCreated_should_RejectCall_When_FallbackBranchMissingOverrideReason`.
Decision logic (mismatch + empty `overrideReason` → reject) is already
pinned by Task 4.0's table-driven test; this test's job is narrower —
confirm `reportPRCreated` actually wires `decideOverridePolicy`'s reject
into an untouched item, not a decision-logic re-check. Same fixture as Task
4.1 but `override_reason` omitted from the request. Assert:
`ErrInvalidArgument`; item remains `Status == review`, `PrNumber == 0`
(untouched — proves the relaxed path only ever fires with an explicit,
audited reason, i.e. it behaves as a genuine manual-override mechanism, not
an automatic bypass). **(Third plan-repair pass)** also assert
`resolveCallerGitHubLogin` is **never called** in this case (use a
call-counting stub, mirroring Task 6.6's "finder not called" pattern) — Task
3.1 step 3's guard condition includes `overrideReason != ""`, so a call
that's already doomed to reject for a missing reason must not pay for a
GitHub identity lookup it doesn't need.
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.3** (AC3 — message documents the workaround) — New test
`TestReportPRCreated_should_DocumentOverrideWorkaround_When_BranchMismatchRejected`.
This stays a full-handler test regardless of Task 4.0, because the exact
message text (`prNumber`, actual/tracked branch names) is formatted in
`reportPRCreated`, not in `decideOverridePolicy` (see Task 3.1's
implementation note). Same fixture/request as Task 4.2. Assert the returned
error message contains both `"override_reason"` and the actual head branch
string (`"feature/ci-status-diff-viewer"`) and the tracked branch string
(`"backlog/stapler-squad-ci-status-diff-viewer"`) — i.e. the caller is told
concretely what to retry with, not just that it failed. **(Third plan-repair
pass)** the AC3 message (below) now also names the authorship requirement
up front, so a caller who fixes the missing `override_reason` on retry isn't
surprised by a second, different rejection if the PR isn't theirs — assert
the message also contains the phrase `"authored by your own GitHub
identity"`.
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.4** (wiring confirmation — closed-state unrelated PR still
rejected) — New test
`TestReportPRCreated_should_RejectCall_When_UnrelatedClosedPRWithOverrideReason`
(**renamed, third plan-repair pass, from `..._UnrelatedPRWithOverrideReason`**
— the cross-artifact consistency review that triggered this pass found the
old name implied broader coverage than this test actually had: it only ever
exercised the *closed*-state rejection, never the open/merged case, which is
exactly the gap requirements.md's "no relationship whatsoever" constraint
needed covered and which the old design couldn't have passed — see Task
4.4a below). Decision logic (mismatch + reason + author-match + closed
state → reject) is already pinned by Task 4.0; this test confirms wiring
only. Fixture: `verifyPRMatchesBranch` →
`NewPRVerification(true, false, "totally-unrelated-branch", github.PRStateClosed, "tstapler"), nil`
(a real PR, but closed and not merged), **`resolveCallerGitHubLogin` →
`"tstapler", nil`** (**author deliberately matches** — isolates this test to
the state gate; an author mismatch would also reject a closed PR, but for
the wrong reason, and Task 4.0's table already pins that ordering
separately). Request includes an `override_reason`. Assert: still
`ErrInvalidArgument` (PR state gate rejects it even with a reason and a
matching author); item untouched.

**Task 4.4a** (**third plan-repair pass** — wiring confirmation, open/merged
unrelated PR by a different author) — New test
`TestReportPRCreated_should_RejectCall_When_UnrelatedPRAuthorMismatch`.
Decision logic (mismatch + reason + author mismatch → reject, checked before
the state gate) is already pinned by Task 4.0; this test confirms wiring
only — and is the test requirements.md's constraint actually needed all
along: unlike Task 4.4 (closed state), this fixture uses an **open** PR,
which the pre-repair design would have accepted on the strength of
`override_reason` alone, since existence + correct repo + open state +
non-empty reason were the *only* four conditions the original ADR-001 listed.
Fixture: `verifyPRMatchesBranch` →
`NewPRVerification(true, false, "totally-unrelated-branch", github.PRStateOpen, "a-different-github-user"), nil`
(a real, open, correct-repo PR — everything the pre-repair design required —
but authored by someone else), `resolveCallerGitHubLogin` →
`"tstapler", nil`. Request includes an `override_reason`. Assert:
`ErrInvalidArgument`; the returned message names both `verification.Author`
(`"a-different-github-user"`) and `callerLogin` (`"tstapler"`), per Task
3.1's message text; item untouched (`Status`/`PrNumber` unchanged). This is
the test that closes the cross-artifact consistency gap: before this pass,
no test in this plan could distinguish "the design correctly rejects an open
unrelated PR" from "the design happens to only have been tested against a
closed one."
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.5** (wiring confirmation — nonexistent PR number cannot be
overridden) — New test
`TestReportPRCreated_should_RejectCall_When_PRNumberDoesNotExist`. Decision
logic (not-exists → reject regardless of `overrideReason`) is already pinned
by Task 4.0; this test confirms wiring only. Fixture: `verifyPRMatchesBranch`
→ `NewPRVerification(false, false, "", "", ""), nil`. Request includes an
`override_reason`. Assert: `ErrInvalidArgument` mentioning "does not exist";
item untouched — proves existence can never be overridden, only the
branch-match requirement can. **(Third plan-repair pass)** also assert
`resolveCallerGitHubLogin` is **never called** here (call-counting stub) —
Task 3.1 step 3's guard condition requires `verification.Exists`, which is
`false` here, so `reportPRCreated` never reaches the identity-resolution
step at all.
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.6** (**intermediate checkpoint only — not the plan's shipping
gate; see Definition of Done above, fourth plan-repair pass**) — Run
`go build ./... && go test ./server/mcp/... ./github/...` locally to confirm
Tasks 1.1–4.5 (including Task 4.0 and Task 4.4a) compile and pass together
(the seam-signature change in Task 2.2/2.3 must land atomically with 1.1/2.1
or the package won't build — call this out to whichever session/task-runner
executes this plan; note Task 1.3's `github/client_pr_by_number_live_test.go`
is excluded from this build/run by its `live_github` tag, so this command
does not exercise it). Passing this task confirms Stories 1–4 compile and
pass together, nothing more. **Do not stop here.** Proceed directly to Story
5 and Story 6 — the plan is not done, and must not be shipped, until Task
6.7's checklist (the actual final verification gate) is fully satisfied.
Files: none (verification task).

### Story 5 — Note the reconciler's shared blind spot (no functional change)

**Task 5.1** — In `session/backlog_lifecycle.go`, immediately above line 2656
(`info, prErr := l.getOrphanedPRFinder()(ctx, item.RepoPath, wt.BranchName)`
inside `reconcileOrphanedAgentPRs`), add a one-line comment:
`// NOTE: this still looks up by branch name (github.GetPRForBranch via getOrphanedPRFinder), so it has the same blind spot report_pr_created had before the number-keyed fix in tools_github.go's VerifyPRMatchesBranch — a PR opened from a fallback branch is invisible here too. Not fixed here (out of scope per project_plans/report-pr-created-branch-mismatch/requirements.md); a future fast-follow could reuse VerifyPRMatchesBranch/GetPRByNumber's shape.`
This is documentation only — `reconcileOrphanedAgentPRs`'s actual lookup
function is unchanged. Per `research/features.md`, this reconciler has the
identical blind spot and should reuse whatever predicate this fix builds, in
a future fast-follow, not in this plan.
Files: `session/backlog_lifecycle.go`.

### Story 6 — Guard the reconciliation automation against override-linked
### mismatches (resolves adversarial-review.md's Blocker)

**Context**: `research`/ADR-001 originally framed the blast radius of a wrong
`override_reason` invocation as bounded to "the wrong real PR gets attached
to the one item this session is already authorized to touch." The
adversarial review found this incomplete: once an item reaches `pr_pending`
with `item.PrNumber` set, three automated call sites in
`session/backlog_lifecycle.go` treat that number as ground truth and mutate
GitHub state or complete the item on its strength alone, with no re-check
that the PR actually belongs to this item's work —
`closeIfSupersededByMain` (auto-`ClosePR`s a possibly-unrelated PR),
`ReconcilePRPending`'s merge-detected done transition, and
`reconcileBouncingItems`'s `IsPRMerged`-driven done transition (both
silently mark the item `done` off a possibly-unrelated PR merging). Before
this fix, the strict branch-name match made this collision very unlikely;
Story 3 relaxes that bar, so this story adds a re-check at exactly the three
points where the relaxed trust would otherwise leak into live GitHub
mutations and completion state.

**Task 6.1** — In `session/backlog_lifecycle.go`: add a `prByNumberFinder`
function-field seam, mirroring the existing `orphanedPRFinder` pattern
exactly (struct field + mutex at ~line 350-357, `SetOrphanedPRFinder`/
`getOrphanedPRFinder` at ~line 618-633, `defaultOrphanedPRFinder` at
~line 260-269): add `prByNumberFinderMu sync.RWMutex` and
`prByNumberFinder func(ctx context.Context, repoPath string, prNumber int) (*github.PRInfo, error)`
fields, `SetPRByNumberFinder`/`getPRByNumberFinder` accessor methods, and
`defaultPRByNumberFinder(ctx, repoPath, prNumber)` which resolves
`github.GetOwnerRepoFromRemote(repoPath)`, checks `ref.IsValid()`, and calls
`github.GetPRByNumber(ctx, ref.Owner(), ref.Repo(), prNumber)` (Task 1.1's
new function). Wire the default into `newListenerBase`'s struct literal
(~line 759, next to `orphanedPRFinder: defaultOrphanedPRFinder`).
Files: `session/backlog_lifecycle.go`.

**Task 6.2** — In `session/backlog_lifecycle.go`, immediately above
`closeIfSupersededByMain` (~line 4113): add
`func (l *BacklogLifecycleListener) verifyPRHeadBranchMatchesTracked(ctx context.Context, repoPath, trackedBranch string, prNumber int) (bool, error)`.
Fails closed: `trackedBranch == ""` → `false, fmt.Errorf(...)` immediately
(never treats "couldn't resolve tracked branch" as a match). Otherwise calls
`l.getPRByNumberFinder()(ctx, repoPath, prNumber)`; a non-nil error returns
`false, err` (fail closed on lookup failure too — a transient GitHub error
must never be read as "verified match"); on success returns
`info.HeadRef == trackedBranch, nil`. Doc comment: explains this re-verifies,
via a live GitHub lookup, that `prNumber`'s real head branch still equals the
item's currently-tracked branch immediately before any of Tasks 6.3-6.5's
call sites treats `item.PrNumber` as ground truth for an automated
GitHub-mutating or completing action — the guard this story adds in
response to adversarial-review.md's Blocker.
Files: `session/backlog_lifecycle.go`.

**Task 6.3** — In `closeIfSupersededByMain` (~line 4134-4200), immediately
before `checker.ClosePR(item.PrNumber, closeComment)` (line 4168): resolve
the tracked branch via `l.storage.GetWorktreeDataBySessionUUID(ctx, lastWork.SessionUUID)`
(reusing the `lastWork` variable already in scope from this function's
existing top-of-function session lookup — no duplicate `ListItemSessions`
call) and call `l.verifyPRHeadBranchMatchesTracked(ctx, item.RepoPath, wt.BranchName, item.PrNumber)`.
On `false` or a non-nil error: `log.WarningLog.Printf("[BacklogLifecycle] closeIfSupersededByMain item=%s: PR #%d head branch no longer verifiably matches the tracked branch — skipping auto-close (was this item's PR attached via report_pr_created's override_reason path?)", item.ID, item.PrNumber)`
and `return false` **before** calling `ClosePR` or clearing the PR
fields/transitioning to `done` — the existing caller-side handling of a
`false` return (ReconcilePRPending falls through to its normal CI-fix-spawn
path, lines 3981/4097) is extended by Task 6.3a below, not left as-is.
Files: `session/backlog_lifecycle.go`.

**Task 6.3a** — Second plan-repair pass, closing the residual gap the
adversarial review found in this story's first pass: a `closeIfSupersededByMain`
returning `false` — whether because Task 6.3's new branch-match guard just
tripped, or for any of the function's pre-existing reasons (no work session,
no recorded commit SHA, an `IsCommitOnMain` error, or the commit genuinely
not on `main` yet) — is exactly the trigger for
`l.remediatePRFixWithBackoffGate(...)` → `fixSpawner.AutoReopenForPRFix(...)`,
and until now `fixCtx` was built straight from `item.PrNumber`/`item.PrURL`
with no branch-match check at all — so a PR attached via `report_pr_created`'s
`override_reason` path (by construction, its head branch doesn't match the
tracked branch) gets a freshly spawned session confidently briefed to
"investigate/address" it as fact, with no disclosure that the association is
unverified. In `session/backlog_lifecycle.go`'s `ReconcilePRPending`, at both
call sites immediately downstream of a `closeIfSupersededByMain` → `false`:
- the closed-without-merging path (fixCtx built at ~line 3996, right after
  the `if superseded := ...; superseded { continue }` block at lines
  3981-3983)
- the CI-failing/blocked/conflicting path (fixCtx built at ~line 4106, right
  after the equivalent block at lines 4096-4099)

resolve the item's tracked branch the same way Task 6.5 already does
(`sessions, _ := l.storage.ListItemSessions(ctx, item.ID.String())`,
most-recent `SessionRoleWork` entry, `wt, wtErr :=
l.storage.GetWorktreeDataBySessionUUID(ctx, lastWork.SessionUUID)`), then
call `l.verifyPRHeadBranchMatchesTracked(ctx, item.RepoPath, wt.BranchName,
item.PrNumber)` — Task 6.2's guard, **re-run** here rather than threaded
through `closeIfSupersededByMain`'s return value (see rationale below). Fail
closed identically to the guard's own contract: no work session, a
`GetWorktreeDataBySessionUUID` error, or the guard itself returning an error
all count as "unverified," not "verified." When unverified, prepend this
sentence to `fixCtx` before its existing message text: `"NOTE: this PR's
association with this backlog item could not be verified (its head branch
does not match — or no longer matches — the item's tracked branch, possibly
because it was linked via report_pr_created's override_reason path). Confirm
this PR is actually relevant to this item's work before investigating or
commenting on it. "` When verified, `fixCtx` is byte-for-byte unchanged from
today.

**Rationale for re-running the guard instead of threading it through
`closeIfSupersededByMain`'s return value**: `closeIfSupersededByMain` returns
`false` for several reasons unrelated to branch verification (see above), and
only reaches Task 6.3's branch-match guard call in the one case it's about to
call `ClosePR` (`onMain == true`). A second return value threaded off
`closeIfSupersededByMain` — e.g. widening it from `bool` to `(bool,
verifyOutcome)` — would therefore be unset/meaningless for the majority of
real calls into this fix-spawn path (both call sites are reached far more
often via "no commit on main yet" than via "guard tripped"), which is exactly
the coverage this task needs to add. `closeIfSupersededByMain` has exactly
two call sites (lines 3981, 4097), both already in `ReconcilePRPending`, so
changing its signature would be mechanically cheap — but cheap-and-wrong is
still wrong. `verifyPRHeadBranchMatchesTracked` (Task 6.2) is a read-only
GitHub GET, already fail-closed and idempotent — a second call at the exact
point `fixCtx` is built is the smallest-diff way to get a *correct* answer at
both call sites, matching this plan's existing bias (see Step 3's
"Reconciliation-automation exposure" row) toward a guard-at-consumption-site
over a wider structural change.
Files: `session/backlog_lifecycle.go`.

**Task 6.4** — In `ReconcilePRPending`'s merge-detected path (~line
3867-3929): the `wt` variable is already resolved there (lines ~3903-3907,
for the ship snapshot) before `CaptureShipSnapshot` is called. Immediately
after `wt` is resolved (whether or not `wtErr` is set — treat a resolution
failure identically to a branch mismatch, i.e. `wt == nil` also fails
closed), call
`l.verifyPRHeadBranchMatchesTracked(ctx, item.RepoPath, wtBranchNameOrEmpty, item.PrNumber)`
before `CaptureShipSnapshot` and before the `TransitionBacklogItemStatus(...,
BacklogStatusDone, ...)` call. On `false`/error:
`log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending item=%s: PR #%d head branch no longer verifiably matches the tracked branch — skipping auto-done transition (was this item's PR attached via report_pr_created's override_reason path?)", item.ID, item.PrNumber)`,
skip `CaptureShipSnapshot` and the done transition entirely, and `continue`
to the next item — the item is left at `pr_pending` and re-evaluated next
tick (same "left for next tick" idiom this function already uses for a
failed `TransitionBacklogItemStatus` call, per its own existing comment at
~line 3918-3920).
Files: `session/backlog_lifecycle.go`.

**Task 6.5** — In `reconcileBouncingItems`'s `IsPRMerged`-driven branch
(~line 2817-2837): before the `TransitionBacklogItemStatus(ctx, item.ID,
BacklogStatusDone, precondition, TriggeredBySystem)` call (line 2824),
resolve the tracked branch (this function does not already have `lastWork`
in scope here — add it: `sessions, _ := l.storage.ListItemSessions(ctx,
item.ID)`, find the most-recent `SessionRoleWork` entry mirroring
`closeIfSupersededByMain`'s identical loop at lines 4140-4148, then
`l.storage.GetWorktreeDataBySessionUUID(ctx, lastWork.SessionUUID)`) and
call `l.verifyPRHeadBranchMatchesTracked(ctx, item.RepoPath, wt.BranchName, item.PrNumber)`.
On `false`/error: `log.WarningLog.Printf("[BacklogLifecycle] reconcileBouncingItems item=%s: PR #%d head branch no longer verifiably matches the tracked branch — skipping auto-done transition (was this item's PR attached via report_pr_created's override_reason path?)", item.ID, item.PrNumber)`
and skip straight to the existing `continue` (line 2837) without calling
`TransitionBacklogItemStatus` — the item stays `in_progress`/`review` and is
re-evaluated next tick.
Files: `session/backlog_lifecycle.go`.

**Task 6.6** — New test file `session/backlog_lifecycle_pr_branch_guard_test.go`
(package `session`). Unit tests for `verifyPRHeadBranchMatchesTracked` using
`SetPRByNumberFinder` to stub `github.GetPRByNumber` (no real HTTP call,
mirroring how `SetOrphanedPRFinder` is already stubbed in existing tests —
confirm the exact existing test helper name via `grep -n
SetOrphanedPRFinder session/backlog_lifecycle_test.go` before writing):
- `TestVerifyPRHeadBranchMatchesTracked_should_ReturnTrue_When_HeadBranchMatches`
  — finder returns `&github.PRInfo{HeadRef: "backlog/x"}`, `trackedBranch =
  "backlog/x"` → `true, nil`.
- `TestVerifyPRHeadBranchMatchesTracked_should_ReturnFalse_When_HeadBranchDiffers`
  — finder returns `&github.PRInfo{HeadRef: "feature/y"}`, `trackedBranch =
  "backlog/x"` → `false, nil`.
- `TestVerifyPRHeadBranchMatchesTracked_should_ReturnFalse_When_TrackedBranchEmpty`
  — `trackedBranch = ""` → `false`, non-nil error, **finder not called** (assert
  via a call-counting stub) — proves the fail-closed path short-circuits
  before any GitHub call.
- `TestVerifyPRHeadBranchMatchesTracked_should_ReturnFalse_When_FinderErrors`
  — finder returns `nil, errors.New("transient")` → `false`, non-nil error
  (fail closed on lookup failure, never treated as a match).

Integration-level tests confirming the three call sites actually skip
mutation on a guard failure (using existing fixture patterns from
`backlog_lifecycle_test.go` for `closeIfSupersededByMain`/
`ReconcilePRPending`/`reconcileBouncingItems`, stubbing `prByNumberFinder` to
return a mismatched `HeadRef`):
- `TestCloseIfSupersededByMain_should_NotClosePR_When_HeadBranchMismatchDetected`
  — asserts `checker.ClosePR` (the mock `prPendingChecker`) is never called
  and the item's `PrNumber`/status are unchanged, where before this story's
  fix it would have been auto-closed and marked `done`.
- `TestReconcilePRPending_should_NotTransitionToDone_When_HeadBranchMismatchDetected`
  — asserts the item remains `pr_pending` after a tick where `IsPRMerged`
  returns `true` but the branch guard returns `false`.
- `TestReconcileBouncingItems_should_NotTransitionToDone_When_HeadBranchMismatchDetected`
  — same shape for the `reconcileBouncingItems` call site.

This directly verifies the mitigation the adversarial review's Blocker asked
for: an override-linked item (by construction, its PR's head branch
mismatches the tracked branch) is provably never auto-closed or
auto-completed by the reconciliation automation on the strength of
`item.PrNumber` alone.

**(Second plan-repair pass)** Tests for Task 6.3a's fix-spawn disclaimer,
confirming the fixCtx text handed to `AutoReopenForPRFix` differs between an
unverified and a normal, verified CI-fix scenario — using `fakePRFixSpawner`'s
existing `lastFixContext` recording field (`session/backlog_lifecycle_test.go:683`,
already asserted against by e.g. the merge-conflict test at line 1199), and
`SetPRByNumberFinder` to stub the branch-match result exactly as the
`TestCloseIfSupersededByMain_should_NotClosePR_When_HeadBranchMismatchDetected`
fixture above does:
- `TestReconcilePRPending_should_IncludeUnverifiedDisclaimer_When_ClosedPRHeadBranchMismatchDetected`
  — fixture: PR closed without merging (`IsClosed: true`), `prByNumberFinder`
  stubbed to return a `HeadRef` that doesn't match the item's tracked branch
  (so `closeIfSupersededByMain` also returns `false`, for the ordinary
  not-on-main reason — the mismatch is what Task 6.3a's independent re-check
  must catch). Assert `fakeSpawner.lastFixContext` contains the exact `"NOTE:
  this PR's association with this backlog item could not be verified"` prefix
  from Task 6.3a.
- `TestReconcilePRPending_should_IncludeUnverifiedDisclaimer_When_CIFailingPRHeadBranchMismatchDetected`
  — same shape for the CI-failing/blocked/conflicting fixCtx site (~line
  4106). Same disclaimer assertion.
- `TestReconcilePRPending_should_OmitUnverifiedDisclaimer_When_CIFailingPRHeadBranchMatchesTracked`
  — control case: `prByNumberFinder` stubbed to return a `HeadRef` matching
  the item's tracked branch. Assert `fakeSpawner.lastFixContext` does **not**
  contain `"NOTE:"` and is byte-for-byte identical to what the existing
  pre-Task-6.3a tests already assert for this fixCtx shape (e.g. the format
  string at line 4106) — pins that the normal, verified CI-fix path is
  unchanged by this task.
Files: `session/backlog_lifecycle_pr_branch_guard_test.go`.

**Task 6.7** (**the sole shipping gate for this entire plan — see
Definition of Done above; reworded and expanded into a checklist in the
fourth plan-repair pass, resolving pre-mortem.md P1 #4**) — Run
`go build ./... && go test ./session/... ./server/mcp/... ./github/...`
locally to confirm the **entire plan** — Stories 1 through 6, not Story 6 in
isolation and not Stories 1–4 in isolation (Task 4.6 above is only an
intermediate checkpoint) — compiles and passes together (the
`github.PRStateOpen`/`PRStateMerged`/`PRStateClosed` constants from Task 1.1
and the `GetPRByNumber` function it introduces are shared dependencies of
both `server/mcp` and `session`, so this must be verified together, not in
isolation).

Before considering this plan complete, walk this checklist and confirm each
item explicitly — do not just eyeball a green `go test` run:
- [ ] Task 1.1/1.2 — `GetPRByNumber` + `PRState*` constants added and unit-tested
- [ ] Task 1.3 — the `live_github`-tagged manual verification test has been
      **run separately** (it is excluded from the `go test` command above by
      its build tag) against real PR #326, passed, and its `-v` output
      (including the logged `Author` value) recorded in the PR
      description/session notes
- [ ] Task 2.1/2.2/2.3 — `PRVerification`/`NewPRVerification` added,
      `VerifyPRMatchesBranch` rewritten, existing tests updated to compile
- [ ] Task 3.1/3.2 — `decideOverridePolicy` + `override_reason`/author-match
      wired into `reportPRCreated`, tool schema/hint text updated
- [ ] Task 4.0–4.5 — table-driven decision tests and full-handler wiring
      tests (including Task 4.4a's author-mismatch case) pass
- [ ] Task 5.1 — reconciler blind-spot comment added
- [ ] Task 6.1 — `prByNumberFinder` seam + `defaultPRByNumberFinder` wired
      into `newListenerBase`
- [ ] Task 6.2 — `verifyPRHeadBranchMatchesTracked` guard method added,
      fails closed on both a definitive mismatch and a lookup error
- [ ] Task 6.3 — `closeIfSupersededByMain` calls the guard before `ClosePR`
- [ ] Task 6.3a — **both** `fixCtx`-building call sites in
      `ReconcilePRPending` independently re-verify and prepend the
      "unverified" disclaimer when the guard fails
- [ ] Task 6.4 — `ReconcilePRPending`'s merge-detected done-transition calls
      the guard before `CaptureShipSnapshot`/`TransitionBacklogItemStatus`
- [ ] Task 6.5 — `reconcileBouncingItems`'s `IsPRMerged`-driven
      done-transition calls the guard before `TransitionBacklogItemStatus`
- [ ] Task 6.6 — `session/backlog_lifecycle_pr_branch_guard_test.go` exists
      and covers all of: the guard's 4 unit cases, all 3 mutation-site
      integration tests, and all 3 Task 6.3a disclaimer tests

Any unchecked item means this plan is **not** done and must not be shipped,
regardless of what Task 4.6 showed in isolation.
Files: none (verification task).

## Given-When-Then per Acceptance Criterion

**AC1** (same tool links a fallback-branch PR, GitHub-verified):
- **Given** backlog item `3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d` is `review`
  status with tracked branch `backlog/stapler-squad-ci-status-diff-viewer`,
  and GitHub PR `https://github.com/tstapler/stapler-squad/pull/326` really
  exists with head branch `feature/ci-status-diff-viewer`, state `merged`,
  and — **third plan-repair pass** — author `tstapler`, the same identity
  `github.GetCurrentUserLogin` resolves for the calling session (the real
  repro is in fact self-authored: the confirmed example is a session
  reopening its own work from a clean branch, which is exactly the scenario
  the override path exists for) (so
  `PRVerification{Exists: true, Matched: false, ActualHeadBranch: "feature/ci-status-diff-viewer", State: "merged", Author: "tstapler"}`)
- **When** the linked work session calls `report_pr_created` with
  `pr_url="https://github.com/tstapler/stapler-squad/pull/326"`,
  `pr_number=326`, a `summary`, and
  `override_reason="tracked branch had unrelated commits from a shared worktree; opened PR from a clean branch instead"`
- **Then** `storage.SetBacklogItemPRAndTransition` is called and the item
  transitions `review` → `pr_pending` with `PrNumber == 326` — previously this
  call hard-rejected with `PR #326 does not match this item's branch "backlog/stapler-squad-ci-status-diff-viewer" on GitHub — refusing to record it.`

**AC2** (manual-override path exists, gated, and audited — resolved as the
same tool + mandatory `override_reason` + author-match, not a second tool;
see `decisions/ADR-001-override-reason-security-model.md` for why):
- **Given** the identical state as AC1's Given
- **When** the same call is made with `override_reason` a non-empty reason
- **Then**, in addition to AC1's persisted transition, a `log.Warn` line is
  emitted with `session`, `item=3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d`,
  `pr_number=326`, `actual_head_branch=feature/ci-status-diff-viewer`,
  `tracked_branch=backlog/stapler-squad-ci-status-diff-viewer`,
  **`pr_author=tstapler`** (third plan-repair pass), and the caller's
  `override_reason` verbatim — an operator/reviewer can grep this line after
  the fact even though no human gated the call itself.

**AC3** (rejection message documents the workaround):
- **Given** the identical state as AC1's Given, but the caller omits
  `override_reason`
- **When** `report_pr_created` is called with `pr_url=".../pull/326"`,
  `pr_number=326`
- **Then** the call returns `ErrInvalidArgument` with a message naming the
  actual head branch, the tracked branch, and the exact retry shape:
  `PR #326's head branch on GitHub is "feature/ci-status-diff-viewer", not this item's tracked branch "backlog/stapler-squad-ci-status-diff-viewer" — refusing to record it. If "backlog/stapler-squad-ci-status-diff-viewer" was polluted (e.g. by another session sharing this worktree) and you opened this PR from a clean fallback branch instead, retry this exact call with an additional override_reason argument explaining why, e.g. override_reason="tracked branch had unrelated commits from a shared worktree; opened PR from a clean branch instead". The override path additionally requires that PR #326 was authored by your own GitHub identity — it cannot be used to attach a PR someone/something else opened. If PR #326 is unrelated to this item, do not retry — find and report the correct PR instead.`
  (**bolded sentence added, third plan-repair pass**) — this both names the
  fix (`override_reason`), the authorship precondition, and tells the caller
  when *not* to retry, directly addressing the bug report's "retries the
  identical failing call in a loop" failure mode.

**AC1-open/merged-different-author** (**third plan-repair pass** —
cross-artifact consistency requirement, not a numbered requirements.md AC on
its own, but the case that closes the gap between requirements.md's "a PR
that has no relationship whatsoever to the item's work must still be
rejected" constraint and this plan's own test coverage; see Task 4.4a):
- **Given** the identical state as AC1's Given, except the real GitHub PR is
  authored by `a-different-github-user`, not `tstapler`
  (`PRVerification{Exists: true, Matched: false, ActualHeadBranch: "feature/ci-status-diff-viewer", State: "open", Author: "a-different-github-user"}`)
- **When** the linked work session calls `report_pr_created` with a
  non-empty `override_reason`
- **Then** the call returns `ErrInvalidArgument` naming both the PR's actual
  author and the caller's own resolved GitHub identity, and
  `storage.SetBacklogItemPRAndTransition` is never called — proving the
  open/merged case (not just the closed case AC3's sibling covers) is
  rejected for a PR with no authorship relationship to the calling session,
  regardless of state.

## Observability Plan

- **Unchanged**: the existing single `log.InfoLog.Printf("[mcp:report_pr_created] session=%s item=%s PR #%d %s", ...)` line
  (`tools_backlog.go:721`) still fires on every successful record, fast path
  or fallback path alike.
- **New**: a `log.Warn(...)` structured line (see Task 3.1) fires only when
  the fallback/override path is actually taken and accepted — the
  `pitfalls.md`-identified compensating audit-trail control for a path that
  has no technical human-gate. Fields: `session`, `item`, `pr_number`,
  `actual_head_branch`, `tracked_branch`, `override_reason`, **`pr_author`
  (third plan-repair pass)**.
- **Call-count note (rebuts a `pitfalls.md` §4 concern)**: this fix does
  *not* increase GitHub calls per attempt — it decreases them. Today's fast
  path (`GetPRForBranch`) already makes a REST list call *and* delegates to
  `GetPRInfoCtx`'s `gh pr view` subprocess for full details
  (`github/client.go:427`) — two round trips, one of them a fork/exec. The
  new `GetPRByNumber` is a single REST call, no subprocess, for both the fast
  and fallback path. `pitfalls.md`'s point that `ghHTTPClient` has no actual
  rate-limit-aware transport still stands as a pre-existing, out-of-scope gap
  — but this fix does not make exposure to it worse; it makes it slightly
  better. **(Third plan-repair pass)**: the new author-match gate adds one
  more GitHub call (`GetCurrentUserLogin`, via `resolveCallerGitHubLogin`) —
  but only on the branch-mismatch path, and only once `override_reason` is
  actually non-empty (Task 3.1 step 3's guard condition), so the common fast
  path and a call rejected for a missing reason both pay nothing extra.
- Rejections (missing `override_reason`, PR doesn't exist, PR closed) are
  **not** separately server-logged, consistent with how the pre-existing
  role-check/idempotency/branch-mismatch rejections already behave today —
  the MCP error text itself is the audit surface visible in the session
  transcript. Not changed by this fix; noted so it isn't mistaken for an
  oversight.
- **New (Story 6)**: each of the three reconciliation-automation guard sites
  (`closeIfSupersededByMain`, `ReconcilePRPending`'s merge-detected path,
  `reconcileBouncingItems`'s `IsPRMerged`-driven path) logs a `WarningLog`
  line when it skips an auto-close/auto-done because the PR's head branch no
  longer verifiably matches the item's tracked branch — this is the audit
  trail for Story 6's guard, distinct from Task 3.1's `override_reason`
  audit line (that one records *acceptance* at `report_pr_created` time; this
  one records the reconciler *declining to trust* that acceptance later).
  No per-tick spam concern in practice: the guard only runs once per item at
  the moment of an actual would-be mutation (PR just detected merged, or the
  item's commit just confirmed on `main`), not on every reconciliation tick.
- **New (Story 6, second plan-repair pass — Task 6.3a)**: the same guard is
  now also re-run (not logged with `WarningLog` — silently folded into
  `fixCtx`'s text, since this is advisory context for the spawned session,
  not an automation-declining-to-act event) at the two points in
  `ReconcilePRPending` where `closeIfSupersededByMain` returning `false`
  falls through to building `fixCtx`. Unlike the three sites above, this one
  is **not** bounded to "once per item at the moment of an actual would-be
  mutation" — `fixCtx` (and therefore this guard call) is built unconditionally
  on every tick that reaches the closed-without-merge or CI-failing/blocked/conflicting
  branch for an affected item, *before* `remediatePRFixWithBackoffGate`'s
  internal backoff check runs, so it is not gated by that backoff. This is a
  real, accepted per-tick cost for affected items specifically (one extra
  lightweight REST GET per tick, same call shape as the rest of this guard),
  not a per-tick cost for the whole reconciliation sweep — items with a
  healthy, merged, or verified-matching PR never reach this branch at all.
  Judged acceptable given the low absolute item count typically in this state
  at once and the correctness value of not silently briefing a fresh session
  against a possibly-unrelated PR.

## Risk Control

- **Manual Verification (required before shipping, fourth plan-repair
  pass)**: every test elsewhere in this plan (Tasks 1.2, 2.3, 4.0–4.5, 6.6)
  stubs `verifyPRMatchesBranch`/`prByNumberFinder` or mocks the GitHub REST
  response via `httptest.Server` — none of them touch real GitHub data, so
  none of them can catch a wrong assumption about the actual response shape
  (`base.repo.full_name`, `user.login` vs `author.login`, `head.ref`
  nesting). Task 1.3 closes this gap with a `live_github`-tagged test that
  calls the real `GetPRByNumber` against the real, already-merged, publicly
  visible PR #326 (`https://github.com/tstapler/stapler-squad/pull/326`).
  This is **not optional**: per the Definition of Done section, it must be
  run locally and its output pasted into the PR description or session
  notes before this plan ships — "the mocks pass" is not sufficient proof
  the fix works against live GitHub.
- **No feature flag.** This is an internal MCP tool handler with no
  user-facing surface change outside the agent-facing tool schema/description;
  `.claude/rules/` gives no specific reason to flag-gate a bug fix at this
  scope, and adding one would be its own unjustified complexity per the
  interface-pollution checklist's "unjustified generic" spirit applied to
  process, not just code.
- **Rollback**: standard PR revert. `GetPRByNumber`, `PRVerification`, and
  the `reportPRCreated` branching are all pure/stateless — no persisted data
  format changes, no migration. Reverting the commit fully restores the prior
  (stricter, branch-name-only) behavior with zero cleanup.
- **Residual risk, stated explicitly** (see also
  `decisions/ADR-001-override-reason-security-model.md`): `override_reason`
  is a procedural deterrent for *item relevance*, not a technical one —
  nothing stops the same work-role session that misjudged the original PR
  from also supplying a plausible-sounding `override_reason` for one of its
  **own** other, unrelated PRs (e.g. if that session/account has several
  concurrently open PRs across different backlog items), as long as that PR
  happens to be open/merged in the correct repo. **This is narrower than
  what this paragraph originally described.** A third plan-repair pass
  (closing a cross-artifact consistency gap between requirements.md's "a PR
  that has no relationship whatsoever to the item's work must still be
  rejected" constraint and this plan's own test coverage — see Step 3's new
  row and Task 4.4a) added a technical, GitHub-verified author-match gate to
  the override path: `decideOverridePolicy` now also requires
  `verification.Author == callerLogin`, where `callerLogin` is resolved via
  the existing `github.GetCurrentUserLogin` primitive. This rules out the
  case the requirements.md constraint is actually about — a PR from a
  genuine stranger, with no relationship to this session at all — leaving
  only the narrower residual case of the *same* identity's own misfiled
  work, which author-match structurally cannot catch (it proves authorship,
  not item relevance; GitHub has no first-class concept of "this PR
  addresses that specific backlog item" for a technical check to hang on).
  The bound on this narrower residual risk is unchanged: the pre-existing
  role + item-link check (only a session already trusted to work *this
  specific item* can invoke the fallback path at all), plus the unchanged
  audit log (now additionally recording `pr_author`). This is still a
  narrower guarantee than "GitHub-verified to be the same lineage of work
  for *this item specifically*," accepted deliberately because that stronger
  guarantee would require either an operator/human auth primitive (out of
  scope, no UI changes) or a technical PR-to-item linkage GitHub has no
  native concept of.
  **Narrowed by Story 6**: the adversarial review found the *original*
  version of this paragraph understated the risk — a wrongly-attached PR
  didn't just sit inertly on the item, it was live ground truth for three
  automated GitHub-mutating/completing actions in
  `session/backlog_lifecycle.go` (`closeIfSupersededByMain`'s auto-`ClosePR`,
  and two `IsPRMerged`-driven auto-`done` transitions). Story 6 closes that
  specific gap: all three now re-verify, via a fresh `GetPRByNumber` lookup,
  that the PR's real head branch still matches the item's tracked branch
  immediately before mutating/completing, and skip (fail closed, logged,
  re-evaluated next tick) when it doesn't. What remains residual after Story
  6: an override-linked item (whose PR's head branch mismatches the tracked
  branch *by construction* — that mismatch is exactly why the override path
  was used) will therefore **never** be auto-closed-as-superseded or
  auto-marked-`done` by these three call sites even when it is the
  legitimate, correctly-attached PR — it requires a human/manual resolution
  path instead (existing review/ship flows, unchanged by this plan). This
  trades "automation could silently mutate/complete against the wrong PR"
  for "automation never completes an override-linked item automatically,
  correct or not" — a deliberate fail-safe direction, not a fail-open one.
  A future fast-follow could recover automation for the *correct* case by
  persisting a distinguishable "linked via override, confirmed correct"
  signal once a human has actually looked at it, but that's the "real human
  gate" ADR-001 already puts out of scope for this fix.
  **Further narrowed by Task 6.3a (second plan-repair pass)**: the first pass
  of Story 6 above closed the auto-close/auto-`done` mutation surface but left
  a fourth mechanism uncovered — `closeIfSupersededByMain` returning `false`
  (for *any* reason, including its own new branch-match guard tripping) falls
  through, in `ReconcilePRPending`, to `remediatePRFixWithBackoffGate` →
  `AutoReopenForPRFix`, which spawns a fresh work session briefed via `fixCtx`
  built straight from `item.PrNumber`/`item.PrURL` — with no branch-match
  check and no disclosure. This is lower severity than the mutation/completion
  mechanisms (no live GitHub mutation; the blast radius is a same-class
  work-role session already linked to this item, briefed with a PR reference
  rather than acting on one), but it is not zero: an override-linked item
  would, before this pass, reliably get a fresh session confidently told to
  "investigate/address" a PR that may have nothing to do with this item's
  work. Task 6.3a closes it: both fix-spawn `fixCtx` sites now independently
  re-verify (a second, cheap, idempotent `verifyPRHeadBranchMatchesTracked`
  call — not threaded through `closeIfSupersededByMain`'s return value, see
  Task 6.3a's rationale for why) and, when unverified, prepend an explicit
  "this PR's association could not be verified — confirm relevance before
  investigating" disclaimer to `fixCtx`. The spawned session is now told the
  ground is shaky rather than handed the PR reference as unconditional fact;
  it is not prevented from investigating the PR (there is no technical gate
  that could do that here, mirroring `override_reason`'s own procedural
  nature), only told not to trust the association without checking.

## Unresolved Questions

1. **`SetBacklogItemPRAndTransition` non-atomicity when an item is already
   `pr_pending` with a *different* PR number** (`pitfalls.md` §6): if this
   fallback path is ever invoked against an item that's already `pr_pending`
   (not `review`) with a stale/wrong PR number, the unconditional
   `UpdateBacklogItem` write (`session/storage.go:768`) would land before the
   `ExpectedStatus: review` transition precondition fails
   (`session/storage.go:775`), leaving a partial write. **Not fixed by this
   plan** — the acceptance criteria's confirmed repro item
   (`3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d`) is in `review` status, so this
   plan's Given-When-Then scenarios don't hit it. Blocks a *future* "correct
   an already-wrong PR attachment" story, not this one. Flag as a follow-up
   bug, don't expand this plan's scope to fix it.
2. **Should `override_reason` additionally require `item.PrNumber == 0`** (an
   extra guard limiting the fallback path to items that have genuinely never
   had a PR recorded)? Decided **not** to add this — the role + item-link
   check already scopes who can call the tool for which item, and the
   idempotency check ahead of it already handles the "already succeeded"
   case. Recorded here as a deliberate judgment call in case a future
   reviewer wants it reconsidered.
3. **`reconcileOrphanedAgentPRs`' shared blind spot** (`research/features.md`):
   noted via a one-line comment only (Task 5.1), not fixed — confirmed out of
   scope by requirements.md. A future fast-follow item should migrate it from
   `GetPRForBranch`/`getOrphanedPRFinder` to the same `GetPRByNumber`-based
   check this plan introduces.
4. ~~**Does the reconciliation automation's `IsPRMerged`/`ClosePR`
   call sites re-verify PR ownership before mutating GitHub state or
   completing an item?**~~ **Resolved across two plan-repair passes, no
   longer open.**
   This was not raised during the original planning pass. **First pass**:
   adversarial review surfaced it as a Blocker (ADR-001's stated blast radius
   omitted `closeIfSupersededByMain` and the two `IsPRMerged`-driven `done`
   transitions, all of which treat `item.PrNumber` as ground truth once an
   item is `pr_pending`). Addressed by Story 6 (Tasks 6.1-6.7): all three
   mutation/completion call sites now re-verify the PR's real head branch
   against the item's tracked branch via a fresh `GetPRByNumber` lookup
   immediately before mutating/completing, and fail closed (skip, log, leave
   for next tick) on mismatch or lookup error.
   **Second pass**: a follow-up adversarial review found the first pass's own
   `false`-path — "leave for next tick / normal fix-spawn path," referenced
   directly above — was itself the trigger for a fourth, still-unguarded
   mechanism: `closeIfSupersededByMain` returning `false` falls through, in
   `ReconcilePRPending`, to `remediatePRFixWithBackoffGate` →
   `AutoReopenForPRFix`, spawning a fresh work session briefed via `fixCtx`
   with no branch-match check and no disclosure that the PR association might
   be unverified. Addressed by Task 6.3a: both `fixCtx`-building call sites
   now independently re-run `verifyPRHeadBranchMatchesTracked` and prepend an
   explicit "association could not be verified" disclaimer to `fixCtx` when
   it fails. See the Risk Control section's "Narrowed by Story 6" and
   "Further narrowed by Task 6.3a" notes for the full statement of what
   residual trade-off each pass leaves.
5. ~~**Can `decideOverridePolicy`'s override path accept a real,
   correct-repo, open/merged PR that has no relationship whatsoever to the
   item's work, as long as a plausible `override_reason` is supplied?**~~
   **Resolved in a third plan-repair pass, no longer open.** A cross-artifact
   consistency check found that requirements.md's explicit constraint — "a PR
   that has no relationship whatsoever to the item's work must still be
   rejected" — was not actually proven by this plan's own tests: the only
   test purporting to cover it
   (`TestReportPRCreated_should_RejectCall_When_UnrelatedPRWithOverrideReason`,
   pre-repair) used a *closed* PR, and the pre-repair design would have
   *accepted* the identical PR had it been open or merged, since existence +
   correct repo + open/merged state + a non-empty reason were the only four
   conditions ADR-001 originally listed. The user was asked how to resolve
   this and chose: require the PR's GitHub author to match the calling
   session's own resolved GitHub identity (`github.GetCurrentUserLogin`) —
   narrowing "unrelated PR" from "any real PR in the repo" down to "this
   session's own (possibly misfiled) work," a real, GitHub-verified technical
   check rather than another audit string. Addressed by: `decideOverridePolicy`
   gaining a `callerLogin` parameter and an author-match gate (Task 3.1);
   `PRVerification` gaining an `Author` field and `GetPRByNumber` parsing
   `user.login` into it (Tasks 1.1, 2.1); a new `resolveCallerGitHubLogin`
   seam (Task 2.2); and Task 4.4a's new test, which is the one that actually
   proves the open/merged case now rejects an author-mismatched PR — see the
   Risk Control section's revised "Residual risk" paragraph for what this
   does and does not close.
