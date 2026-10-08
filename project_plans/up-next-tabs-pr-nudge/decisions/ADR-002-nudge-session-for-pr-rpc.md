# ADR-002: `NudgeSessionForPR` RPC, server-built prompt, per-session guard

**Status**: Accepted | **Date**: 2026-10-05

## Context
A one-click, un-editable prompt injects untrusted PR comment text into an agent PTY. Existing steering (`steerActiveSessionForPRFix`) is keyed by backlog item and has an item-keyed `steerInFlight`; MCP `steer_session`/`diagnose_nudge_session` are role-gated and not UI-callable. `session_ids` holds titles, and `FindLiveInstance` -> `MatchesID` accepts a title (`session/instance_terminal.go:72`).

## Decision
- New RPC `GitHubUserService.NudgeSessionForPR(PRKey, session_id)`; client never supplies text.
- Handler order: `parsePRKey` -> PR in cache by host+owner+repo+number -> session in PR's `LinkedSessions` (keys are host+owner+repo+branch; fixed in Task 1.3.1a, since `github/repo_ref.go:51,55` keys omit host and repo today) -> resolve the instance once -> token via PR's owning account -> fresh detail fetch -> `BuildPRNudgePrompt` (pure; empty = NOTHING_TO_FIX) -> `SessionService.SteerInstanceGuarded` on the resolved instance: atomic `TryBegin` (in-flight + 60s duplicate window under one lock) -> single idle gate `IsReadyForSteer` -> write. `VerifyNudgeSafeToWrite` is not a second gate; one gate shared with auto-steer avoids disagreeing idle notions and the re-resolve-by-title TOCTOU.
- Safety order inside the guarded steer: `TryBegin` -> readiness gate -> `VerifyPaneOwnershipBeforeWrite` (unconditional; `steerInstance` does not perform it) -> write. Auto-steer reaches it via UUID method `SteerSessionGuarded`; a guard BUSY/DUPLICATE there skips the tick (never `degradeToRespawnBlocked`), and the signature is the full reason signature. Sessions with no active controller get BUSY with distinct copy.
- Typed outcomes: DELIVERED, BUSY, PAUSED, DUPLICATE, NOTHING_TO_FIX, SESSION_NOT_LINKED, PR_NOT_FOUND. Rate limit = `ResourceExhausted`; unsupported program = `FailedPrecondition`.
- New `sessionNudgeGuard` keyed by session stable ID, owned by `SessionService` behind the existing `SessionSteerer` seam (one added interface method; no new `BacklogService` field), shared with `steerActiveSessionForPRFix` so click and automation cannot interleave and a click suppresses an identical auto-steer. Manual click bypasses the auto-steer 5-minute cooldown but not the 60s duplicate window.
- Prompt carries thread URL + path + author and failing check names only; **no third-party comment body** and **no appended `/github:pr-ship`** (a manual nudge keeps the user in the loop; untrusted text plus an auto-executing command widens injection blast radius). All GitHub-sourced strings pass `sanitizeUntrusted` (shared in `session/`), are framed as data, and the total is bounded by `session.MaxSteerMessageLength` (`session/instance.go:200`).
- The Nudge button shows only when `nudgeable` (failing checks, unresolved threads or merge conflict); changes-requested alone is hidden, matching server `NOTHING_TO_FIX`.
- Merge conflicts are a third `NudgeReason`.
- Prompt builder is new and pure over `PRNudgeDetail`, not `PRStatus.render()` (needs a worktree), but reuses `buildSteerMessage` for program suffix and truncation.

- **Access guard (write-capable RPC; VERIFIED middleware, `server/server.go:1678-1700`, `server/middleware/probeguard.go`, `auth.go`):** the local :8543 chain has no auth unless configured, and only `ProbeProgram` is protected there (ProbeGuard: POST-only, loopback-bound, loopback/allowed Host and Origin; the Host check stops DNS rebinding, which CORS does not). CORS only reflects allowed origins and there is no CSRF token. `NudgeSessionForPR` types into a live PTY, so it is added to a generalized ProbeGuard path set; on :8444 the existing `authMW` (WebAuthn cookie or Bearer) is the boundary, asserted by a test. No extra per-RPC rate limit: the in-flight + 60 s per-(session, signature) window caps writes and the single GraphQL query is limited by `AdmitOrigin` (Task 2.3.1f).
- **PR visible to two accounts:** `fetch()` dedups by URL and the first account in `resolveAllLogins` order wins (stable across polls); the nudge uses that account's token and never falls back to the other account (`PR_NOT_FOUND` if it cannot see the PR). Reorder accounts to change the owner (Task 1.4.1c).
- **State lifetime:** the guard's `last` map expires entries after 10 min and caps at 1,024; it, `firstSeenAttention` and the `nudge_followup` map are in-memory and lost on restart (a nudge just before a restart may repeat once). Code placement: new `SessionService` methods in `session_service_guarded_steer.go`, the guard in `session_nudge_guard.go`, the handler in `github_user_nudge.go`; the 6.8k-line `session_service.go` gains one field and a delegation.

## Alternatives rejected
Client-composed text via `UpdateSession` steer (no gate, stale data); calling `steerActiveSessionForPRFix` (item-coupled, status side effects); exposing the MCP nudge (role-gated).

## Open items
Seed path for "+ Session": decided in Phase 1 Task 1.4.2a (timeboxed 15 min; at most one additive optional `initial_prompt` + `PRFixSeeder` port, else deferred; fork PRs get a plain session). "Most recently active" field (`UpdatedAt` proxy).

### Readiness-gate spike (Task 2.0.1a, 2026-10-07)
**Method.** Read-only: MCP `list_github_prs` (113 open PRs; `existing_session_id`/`worktree_path` link fields) joined to `search_sessions`/`list_sessions` lifecycle status (171 sessions). No live service restart, no writes. The in-process idle/controller verdict is not exposed over MCP, so the gate was bounded from lifecycle status plus code: controllers start only for instances that are started, not paused and not Stopped (`server/dependencies.go:1008-1020`), and `IsReadyForSteer` (`server/services/session_service.go:1093`) returns false without an active controller.

**Measurement (7 sessions linked to an open PR):** Hibernated 5 (corp-compute-nop-pr-534, compute-docs-pr-226, pr-619, pr-579, traffic-capacitron pr-456), PermanentlyFailed 1 (pr-487), Active 1 (stapler-squad-terminal-corruption, PR 937). Upper bound on the idle pass rate: 1/7 = 14% (and only if that one is idle at click time). Failing for `NoStatusSource` specifically (live but no controller): 0 observed; all 6 failures are non-live sessions (would surface as PAUSED, not BUSY-no-controller). Caveats: n=7, one developer instance, snapshot only, idle state of the Active session unmeasured. Branch-linked PRs without `existing_session_id` (e.g. #932/#933/#936) were not counted.

**Decision: below the 70% gate, choose (b).** Widening the manual gate (a) would not help: CheckNudgeEligible still needs a live controller/pane, and the dominant failure is a hibernated/paused session with no pane at all. So keep the single shared gate unchanged and add an in-card "Open session" (resume/open) action for non-live linked sessions, keeping the distinct PAUSED/BUSY-no-controller `detail` copy (Story 4.3.1). Task 2.2.1b's gate step stays as written; Story 2.3.1's parity AC is unchanged; the UI must surface PAUSED prominently since it is the common outcome, not an edge case.

### Link-stability replay (Task 6.1.1d step 1, PR A, 2026-10-07)
**Result: passed.** `TestAnnotate_should_LinkSameSessionsBeforeAndAfterKeyChange_When_SessionSnapshotReplayed` (`github/user_pr_cache_annotate_test.go`) links `github/testdata/session_snapshot.json` with the old owner-only key and with the host+owner+repo key (legacy fallback index on): 8 links before, 8 after, none lost. **Gap:** the fixture is synthetic and real-shaped (https, ssh, `.git`, GHE, PR-number-only sessions), not an export of the live `list_sessions` (the stapler-squad MCP was unreachable when this was written), so it does not cover every real remote. The manual live-instance confirmation is still open; the legacy owner-only fallback index must stay until it is recorded here.

### Measurement recipe (Task 6.1.1f, PR A part)
No pre-ship baseline exists (see `requirements.md` Baseline); the first two weeks after ship are the baseline. Revisit at 2 weeks, kill-criterion check at 4 weeks (Core Value Hypothesis).

Tab usage counters (browser console on `/unfinished`; counters only, never PR content):
```js
const s = JSON.parse(localStorage["up-next-tab-stats"] ?? "null");
s && { ...s, prsLeaveRate: s.visits ? +(s.leftPrsWithin5s / s.visits).toFixed(2) : null };
```
`leftPrsWithin5s / visits` is the PRs-tab leave rate. Nudge log queries. The log (`~/.stapler-squad/logs/staplersquad.log`, JSON lines) carries one `nudge_outcome` line per RPC call (`server/services/github_user_nudge.go`, `logNudgeExit`: `pr`, `host`, `account`, `session_id`, `outcome`, `reasons`, `latency_ms`, `prompt_bytes`, `session_live`, and `attention_age_s` only when the poll had seen the PR needing attention) and one `nudge_followup` line per settled nudge (`github/user_pr_nudge_track.go`, `trackNudges`: `pr`, `host`, `state` of `resolved|closed|expired`, `resolved_after_s`). Nudges per ISO week, outcome mix, and median attention age (save as `nudge.jq`, run `jq -c -s -f nudge.jq ~/.stapler-squad/logs/staplersquad.log`; the week is taken from the first 19 characters of `time`, so a non-UTC offset shifts a line near a week boundary):
```jq
[ .[] | select(.msg == "nudge_outcome" and .session_live == true)
  | . + {week: ((.time[0:19] + "Z") | fromdateiso8601 | strftime("%G-W%V"))} ]
| group_by(.week)[]
| { week: .[0].week, total: length,
    by_outcome: (group_by(.outcome) | map({(.[0].outcome): length}) | add),
    median_attention_age_s: ([.[].attention_age_s | numbers] | sort
        | if length == 0 then null
          elif length % 2 == 1 then .[length / 2 | floor]
          else (.[length / 2 - 1] + .[length / 2]) / 2 end) }
```
Funnel (counts per snapshot sent; read ratios): `jq -c 'select(.msg == "up_next_funnel")' ~/.stapler-squad/logs/staplersquad.log` gives `prs_needing_attention`, `with_linked_session`, `with_live_session`; clicks are `nudge_request` lines and deliveries are `nudge_outcome` with `outcome == "DELIVERED"`. "Open session" clicks are in the browser counter (`openSessionClicks` in the `up-next-tab-stats` snippet above).

Share of nudges resolved within 24 h:
```sh
jq -c -s '[.[] | select(.msg == "nudge_followup")]
  | {total: length, resolved_within_24h: ([.[] | select(.state == "resolved" and .resolved_after_s <= 86400)] | length)}
  | . + {share: (.resolved_within_24h / .total)}' ~/.stapler-squad/logs/staplersquad.log
```
Self-test (measurementRecipe_should_CountOutcomesPerWeekAndMedianAge_When_RunAgainstSampleLog, run 2026-10-07 with jq 1.7): three fabricated `nudge_outcome` lines (2026-10-05 `DELIVERED` age 100, 2026-10-06 `DELIVERED` age 300, 2026-10-14 `DUPLICATE` no age) plus one `nudge_request` distractor gave `{"week":"2026-W41","total":2,"by_outcome":{"DELIVERED":2},"median_attention_age_s":200}` and `{"week":"2026-W42","total":1,"by_outcome":{"DUPLICATE":1},"median_attention_age_s":null}`; three fabricated `nudge_followup` lines (resolved 3600 s, resolved 172800 s, expired) gave `{"total":3,"resolved_within_24h":1,"share":0.333...}`. Not yet run against a real log: no `nudge_*` lines exist before PR B ships.

5-minute self-test, first two weeks: (1) open `/unfinished` cold and note which tab lands and whether the first PR card is visible without scrolling; (2) find a PR with failing CI and open its linked session from the card; (3) reload and confirm the tab and filters persist; (4) note anything confusing, and tally how often you still hand-type a PR-fix instruction into a session. Feed the notes into the hypothesis review.

### Dual-visibility token owner (Task 1.4.1c, PR B, 2026-10-07)
**Result: confirmed by test.** `UserPRCache.TokenForPR` (`github/user_pr_token.go`) returns the token of the account named by the cached PR's `AccountLogin` on the PR's host, and `ok=false` when the PR is unknown or its owning account is no longer connected; there is no fallback to another account or the host default. `TestTokenForPR_should_ReturnFirstAccountInOrderAndNeverFallBack_When_PRVisibleToTwoAccounts` (`github/user_pr_token_test.go`) drives `fetch()` three times against an httptest GitHub where `alice` and `carol` both see `acme/api#42`: `alice` (first in account order) owns it on every poll and `carol`'s token is never returned, including after `alice` is disconnected. The "alice's fresh fetch returns not-found, so the RPC yields `PR_NOT_FOUND`" half of the AC is handler behaviour and is covered by Story 2.3.1, not here. Tests live in `user_pr_token_test.go` (internal package) rather than `user_pr_cache_test.go` (external `github_test` package, cannot seed private cache state).

### "+ Session" seed path (Task 1.4.2a, PR B, 2026-10-07; time spent about 10 minutes)
**Decision: (c) deferred; plain "+ Session" ships.**

Evidence (read in this worktree):
- `PRCard.tsx` "+ Session" is a `<Link href="/?pr=<htmlUrl>">` (`web-app/src/components/unfinished/prs/PRCard.tsx`, `data-testid="create-session-button"`); `web-app/src/app/page.tsx` handles `?pr=` by calling `openOmnibar(prUrl)`. There is no create-for-PR RPC on the web path; creation is the generic `CreateSession` fired from the Omnibar.
- `CreateSessionRequest` already has `initial_prompt = 15` (`proto/session/v1/session.proto:757`), consumed at `server/services/session_service.go:2788`. So the transport exists, but the text must be server-built (fresh fetch with the PR's owning-account token, sanitized, no comment bodies). The client cannot build it, and `CreateSession` has no PR identity to build it from.
- Seeding therefore needs (1) an additive PR-identity field on `CreateSessionRequest`, (2) a `PRFixSeeder` port wired into `SessionService`, (3) Omnibar changes so the `?pr=` flow carries host/owner/repo/number instead of only a URL (new session-creation mode touchpoints, see `docs/reference/session-creation-registry.md`), (4) a re-check for an existing linked session at create time and a "created, nothing to fix" response flag. That is wider than "one additive optional field plus a call to a small port", which is the plan's gate for (a)/(b).

Consequence: Story 4.3.2 drops its CONDITIONAL rows and keeps only `prCard_should_ShipPlainPlusSession_When_Task142aOutcomeC`; the "Open session" action for non-live linked sessions (readiness spike above) is unaffected. Revisit as a follow-up only if the post-ship hypothesis review shows people hand-typing the fix instruction into new sessions.

## Decision update (2026-10-07, user decision): readiness-led primary action
The nudge gate stays strict, but the card no longer offers the nudge to sessions the server cannot confirm idle. `LinkedSession.steer_ready` (additive proto field 4) is computed per snapshot with the same gate `SteerInstanceGuarded` applies (live, linked by title, not suspended, steerable program, idle via `instanceReadyForSteer`); unknown is false. When no linked session is steer-ready, "Open session" is the primary action and the ask button is disabled and de-emphasized. Readiness is as fresh as the last poll, so a click may still return `BUSY`; the typed outcomes are unchanged.
Metrics follow: the `nudge_outcome` denominator is nudges attempted on live sessions (`session_live=true`, set once the session resolves to a live unsuspended instance); each client snapshot logs `up_next_funnel`; "Open session" clicks count as a success signal in the existing `up-next-tab-stats` counter. Revisit and kill criteria in `requirements.md` were updated to match.
