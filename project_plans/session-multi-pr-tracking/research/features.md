# Research: Features audit of single-PR assumptions

Scope: every read/write of `GitHubPRNumber` / `GitHubPRURL` (session side) and `PrURL` / `PrNumber` (backlog item side), plus proto, MCP and web-app equivalents. Method: `grep -rnE` over `*.go|*.proto|*.ts|*.tsx` excluding tests, generated code and `session/ent/*.go` (169 non-test hits across 41 files; recount with the grep in the Method line), then reading the flow code. Confidence: VERIFIED = opened the cited lines; INFERRED = from grep absence only.

## 1. Two independent single-PR stores

There are **two** places a PR lives, written by different flows and never reconciled:

- **Session** (`Instance.GitHubPRNumber/GitHubPRURL`, ent `Session.github_pr_url/github_pr_number`).
- **Backlog item** (`BacklogItem.pr_url/pr_number`, `session/ent/schema/backlog_item.go:98-100`), plus `ItemSession` rows (item to sessions, many). The item side is what gates `pr_pending` and completion.

Key finding: `report_pr_created` (MCP), Jules, `pushAndCreatePR` and the manual-link RPC write **only the item**. The only callers of `Instance.SetGitHubPR` are `server/services/pr_creation_service.go:305` and `server/services/session_service_lifecycle.go:868` (VERIFIED by grep). So a session whose agent runs `gh pr create` and then `report_pr_created` has the PR on the item but not on the session, unless the poller's branch discovery or the terminal scanner fills it in.

## 2. Session-side sites (file:line, assumption)

### Writes
| Site | What it does | Single-PR assumption |
|---|---|---|
| `session/instance_actor_setters.go:276-291` `setGitHubPRURLLocked` / `SetGitHubPR` | Overwrites URL+number unconditionally | Last writer wins; old PR is dropped, `GitHubPRStatusTerminal` and PR status fields are NOT reset, so a new PR inherits a stale "merged" terminal flag |
| `instance_actor_setters.go:296-310` `SetGitHubPRNumber` | Number only (URL left untouched) | URL and number can desync (new number, old URL). Used by poller discovery and artifact extractor |
| `instance_actor_setters.go:938-955` `setGitHubResolutionLocked` | Sets PR from a `/pull/N` URL at deferred GitHub resolution, only if `r.PRNumber > 0` | Session created from a PR URL is "about" that one PR |
| `server/services/session_service_create.go:895-898`, `session_creation_pipeline.go:185-197`, `session/instance.go:1222-1223` | Set PR on create from a PR URL | Same as above |
| `session/session_driver.go:952-969` `scanAndLinkPRURL` | Raw-writes PR from terminal `git push` output; guarded by `prURLLinked` bool seeded at `session_driver.go:396` (`inst.GitHubPRNumber > 0`) | "Link once": after the first PR is linked, a later pushed PR URL is never scanned. Also writes fields directly under `inst.mu`, bypassing the actor setter |
| `session/pr_status_poller.go:419-427` | Branch-based discovery: only when `prNumber == 0`; persists via `UpdateInstancePRNumber` | Discovery runs only for sessions with no PR. Once set, never rediscovers |
| `server/dependencies.go:1875-1890` (ArtifactExtractor `OnScanComplete`) | Takes the **first** PR URL from transcript artifacts if `!HasGitHubPR()`, then `break` | First PR in transcript wins; later PRs in the blob are ignored; also trusts transcript text (any PR mentioned, not necessarily one the session opened) |
| `server/services/pr_creation_service.go:165-169, 279-281, 305` | Create-PR RPC: if URL already set, returns `ExistingPrUrl` (early exit); else creates and `SetGitHubPR` | "Session already has a PR, do not create another" is a hard gate. This blocks a legitimate second/follow-up/stacked PR from the UI |
| `server/review_queue_manager.go:553, 593` (`maybeAutoCreatePR`) | Skips if `inst.GitHubPRURL != ""` (re-checked after in-flight guard) | Same gate for auto-create-PR |
| `server/services/session_service_lifecycle.go:855-872` | After `RunOneShot` output contains a PR URL (`extractPRURL`) sets session PR and calls `RecordPRCreatedOutOfBand` | One-shot "create PR" overwrites any existing PR |
| `session/instance_terminal.go:425-449` | `PRStatusUpdate` including `Terminal` written to the instance | Status fields are per-session, not per-PR |
| `session/session.go:254-258, 338-339` | `Session` DTO to/from Instance (`Git.PRNumber/PRURL`) | Single PR in DTO |
| `session/instance_serialization.go:148-149, 164, 344-345, 359`; `session/storage.go:66-67, 84, 792, 800`; `ent_repository.go:406-412, 752-758, 1136-1160, 1559-1561` | Persistence: only non-empty/`>0` values are written (`if data.GitHubPRURL != ""`) | **Cannot clear** a PR through the normal save path, and a stale value persists; `UpdateGitHubPRNumber`/`UpdateGitHubPRStatusTerminal` keyed by session **title**, not UUID |
| `session/ent/schema/session.go:136-139` | Two scalar columns | No per-PR state, role or timestamps; no host/owner/repo on the PR (owner/repo/host are separate session columns) |
| `session/github_pr_url_backfill.go:35-66`, registered `ent_repository_migrations.go:81` | One-time: fills URL from number+owner+repo+host | Identity of a PR = session's owner/repo/host + number. Works only because there is one PR per session |

### Reads
| Site | Assumption |
|---|---|
| `session/instance.go:1443-1444` `HasGitHubPR()` | Boolean "has a PR"; callers treat true as "no need to look for more" (`dependencies.go:1875`) |
| `session/pr_tracking.go:21-220` (refresh info, comments, diff, post comment, set commit status, merge, close, context prompt) | All act on `gh.PRNumber` from `i.GitHub()`; **no PR parameter**. After PR A merges and PR B opens, merge/close/comment still hit whatever the single field holds |
| `session/pr_status_poller.go:162` (`InvalidateAndRefresh`, webhook) | Matches `(owner, repo, number)` against the single field; a webhook for PR B never matches a session tracking A. Doc comment notes two instances can track the same PR (many sessions to one PR already occurs) |
| `pr_status_poller.go:319-321` | `GitHubPRStatusTerminal` true: skip session forever. **Merged PR A makes the poller stop, so PR B is never discovered** (prNumber != 0 AND terminal). This is the core "wrong/stale PR" bug |
| `pr_status_poller.go:375-440` | Direct-number path; discovery only when 0 |
| `session/instance_terminal.go:361-362` | PR fields copied to the terminal/status update |
| `server/pr_annotations.go:76-109` | Reads number, falls back to parsing URL; one PR for diff annotations |
| `server/prwatch_sessions.go:100`, `prwatch_snapshot.go:31` | PR-watch inbox: one `(PR, Terminal)` entry per session |
| `server/services/session_retention_sweeper.go:132` | Session eligible for retention only if `GitHubPRNumber == 0 || terminal`. With two PRs, "terminal" of the current one decides; an open older PR is invisible, an archived session with an unmerged PR is protected |
| `server/services/unfinished_work_service.go:128-135` | One PR per worktree row (`Number`, `URL`); skipped if number 0 |
| `server/services/approval_service.go:142-149`, `approval_handler.go:464` | CI-failure gating on `GitHubPRNumber > 0` and `GitHubCheckConclusion` (single check conclusion) |
| `server/adapters/instance_adapter.go:103-104` | Maps to proto `github_pr_number` / `github_pr_url` |
| `server/services/debug_snapshot.go:65, 171` | Debug dump, one PR |

## 3. Backlog-item-side sites

| Site | Assumption |
|---|---|
| `session/storage.go:1240-1310` `setPRAndTransition` | Item has one PR. If `pr_pending` and same number: idempotent no-op. If `pr_pending` and different number: **reassignment**, requires `PRReassignmentGuard` (override reason, current PR not merged, new PR author verified); a merged current PR **cannot be changed** (`ErrPRReassignmentNotAllowed`). So a second PR for an item is rejected by design today |
| `server/mcp/tools_backlog_pr.go:178-399` `report_pr_created` | Verifies PR head branch equals the session branch (`sessionBranch`, `verifyPR`); mismatch rejected unless `override_reason` + author match; reassignment path `isReassignment`. Writes only `PrURL/PrNumber` + `pr_pending`; never touches the session |
| `server/services/backlog_service_lifecycle.go:386-508` manual-link RPC | Same one-PR field pair; "pr_url and pr_number must both be set or both unset"; no GitHub verification |
| `session/backlog_lifecycle_pr.go:665-734` `pushAndCreatePR` | If `item.PrNumber > 0 && item.PrURL != ""` reuse that PR instead of creating (line 665), else `CreatePR` and write item fields |
| `backlog_lifecycle_pr.go:543-596` `shipViaAgentOrFallback`, `:1329-1366` `RecordPRCreatedOutOfBand` | Agent/one-shot PR written to item (`PrNumber <= 0` is rejected, BUG-063) |
| `backlog_lifecycle_pr.go:190, 263, 1028` + `storage_backlog.go:1129, 1177` | `FindPRPendingItems` uses `PrNumberGT(0)`; `BackfillMissingPRNumbers`; items with `pr_pending` but `pr_number = 0` are a stuck class (`pr_pending_no_pr`, BUG-040). Every reconciler keys on the single item PR |
| `session/backlog_lifecycle.go:2058-2082` `reconcileBouncingItems` | Item to done when **the** item PR `IsPRMerged`; verifies head branch still matches. With 2 PRs, merging the first would complete the item |
| `backlog_lifecycle_pr.go:21-24, 1573-1600, 1800-1850` PR-fix spawner / auto-rework / conflict retry | Reopen item for rework when its PR has failing checks/conflicts; rework session works on the existing branch (PR). Counters (`conflictRetryMaxAttempts`, rework cap) are per item, not per PR |
| `server/services/backlog_github_progress_sync.go:89-156` | Posts a progress comment linking the item's one PR; re-fetches because the event snapshot can lack the PR |
| `session/jules_session_poller.go:505-560, 585-604` | `completedSessionPRURL` takes the **first** valid PR in `s.Outputs`; with `len(Outputs) > 1` only logs `jules multiple pr outputs` (open question in the Jules plan: whether outputs[] ever carry more than one PR). Writes item only |
| `session/ent_repository_backlog.go:2012-2041` | Single UPDATE writes status + `PrURL` + `PrNumber` atomically (CAS). Any multi-PR design must keep this atomicity or relocate it |
| `server/services/backlog_service_ship.go:105-117` (`TriggerShipPR`) | Returns one `PrUrl` |

## 4. Proto

- `proto/session/v1/types.proto:93` `int32 github_pr_number = 25`, `:96` `string github_pr_url = 26` (Session message; github_pr_state, check conclusion etc. sit alongside as scalars).
- `types.proto:831` `string github_pr_url = 18` (review-queue item; has no number, web parses it with `parseGitHubRef`).
- `types.proto:1704-1706` `github_pr_number = 21`, `github_pr_url = 22`, `github_pr_state = 23` (unfinished-worktree item).
- Backlog item message and `TriggerShipPRResponse.pr_url`, `ExistingPrUrl/ExistingPrNumber` in create-PR response: single-valued.
- Adding `repeated` PR messages is wire-compatible; the legacy scalars must stay populated for old clients (mobile app).

## 5. MCP tools

`report_pr_created` (`server/mcp/tools_backlog.go:3138-3143` schema; `tools_backlog_pr.go`), plus the prompt text that tells agents to call it after `gh pr create` (`tools_backlog.go:557`, `session/backlog_commands.go:263`). Params `pr_url`, `pr_number`, `override_reason` are single-PR. Prompt wording says "the PR", not "a PR". `get_backlog_item` and the debug-seed handlers (`backlog_debug_seed_handler.go:240, 751`) expose `pr_url` singly.

## 6. Web-app (TS/TSX)

| File:line | Behaviour |
|---|---|
| `web-app/src/components/sessions/SessionCard.tsx:715-716, 1093-1105` | Single `#N` badge link, aria-label uses `githubOwner/githubRepo` |
| `SessionRow.tsx:449-450` | Badge props |
| `SessionActionsOverflow.tsx:924-935` | "View PR #N" menu item vs "Create PR" fallback (hidden if URL set) |
| `SessionDetailView.tsx:1580-1585` | `#N (state)` using `githubPrState` |
| `DiffViewer.tsx:25-30` | PR panel only when `githubPrNumber` truthy |
| `ReviewQueuePanel.tsx:546-548, 1255-1267` | Filter "has PR" by URL; number parsed from URL |
| `lib/vcs/adapters.ts:73-74, 220-226` | Adapter flattens to one `prUrl/prNumber`; unfinished-worktree adapter derives owner/repo by parsing the URL |
| `components/unfinished/UnfinishedItemDetail.tsx:89-90` | Same |
| `tests/e2e/create-pull-request.spec.ts`, `ci-status-badge.spec.ts` (+ steer-focus) | Mock `githubPrUrl/githubPrNumber`; must be updated |

## 7. Flows where the PR changes or a second PR can appear

1. **Session PR creation (`session-pr-creation`)**: create-PR RPC early-returns the existing PR (`pr_creation_service.go:165, 279`); auto-create skips on `GitHubPRURL != ""`. A second PR from the same session can only arise via `gh pr create` by the agent or the one-shot path, which then **overwrites** (`session_service_lifecycle.go:868`).
2. **`report_pr_created`**: item-level only; second call with a different number is a "reassignment" needing `override_reason`, author check, and a not-merged current PR. Session is never updated.
3. **Branch mismatch (`report-pr-created-branch-mismatch`)**: PR head must equal the session's branch; a PR opened from a clean fallback branch needs the override. Stacked PRs (different head branches, same session) fail this check by default.
4. **Rework sessions**: PR-fix spawner reopens the item; the new session typically pushes to the same branch (same PR). INFERRED: it does not create a new PR unless the agent decides to. A rework session is a *different* Instance, so it starts with `GitHubPRNumber == 0` and relies on branch discovery to find the same PR (this is how two instances come to track one PR).
5. **Jules multi-output**: first PR wins, extras logged and dropped (`jules_session_poller.go:511-516, 597`).
6. **`pr_pending` respawn** (`project_plans/backlog-pr-pending-session-respawn`): a replacement session for a `pr_pending` item has no PR on the session; it is rediscovered by branch name only if the branch matches. If the original session was deleted, session-side PR history is gone even though the item still has the PR.
7. **`github_pr_url_backfill`**: fills missing URLs from number+owner/repo/host; ent_repository_migrations.go:81. Idempotent. Any new relation needs the same legacy-field backfill so rows with number but no URL migrate into the new table with a correct URL.
8. **Terminal scan (`scanAndLinkPRURL`)** and **artifact extractor**: both are "first PR seen wins" opportunistic linkers that run only when no PR is linked.
9. **Merged-then-new-PR**: PR A merges, poller marks `GitHubPRStatusTerminal=true`, session never polls again. Agent opens PR B: `SetGitHubPR` overwrites URL/number but the terminal flag stays true unless `applyPRUpdate` runs, and it won't (skipped by line 319). INFERRED from code reading; worth a failing test to confirm.
10. **Created from a PR URL**: session starts with that PR; if the agent then pushes a new branch/PR, nothing records the original (source) PR as history.

## 8. Edge cases and unstated user needs

- **Identity key**: `(host, owner, repo, number)`. Today host/owner/repo live on the session, so a PR in a different repo (cross-repo follow-up, fork PR, GHE host) cannot be represented. The webhook path already guesses host (`InvalidateAndRefresh` comment: "github.com is the safe default"). Number reuse across repos is real.
- **Fork sessions**: poller skips `GitHubIsFork` sessions entirely (`pr_status_poller.go:322-325`). A fork PR to upstream is out of tracking.
- **Stacked PRs**: PR2 targets PR1's branch; when PR1 merges GitHub retargets PR2 to base. Needs `base_ref/head_ref` per PR to show stack order and a defined "active PR" (leaf or earliest open).
- **Merged-then-new-PR / follow-up PR**: history row for the merged PR must remain (user need: "show everything this session shipped"); polling stops for it, resumes for the new one.
- **Closed-then-reopened and closed-then-replaced** (close PR, open a fixed one): state transitions per PR; a closed PR should not block completion.
- **Draft to ready** and **auto-merge enabled**: per-PR flags (`IsDraft`, `Mergeable`, `ApprovedCount`, `Checks` today live as scalars on the instance via `PRStatusUpdate`); these are per-PR data and move with the relation.
- **Archived / paused sessions**: poller skips suspended sessions (`IsSuspended()`); an archived session with an open PR keeps stale state. Retention sweeper protects it until terminal (`session_retention_sweeper.go:132`), so a second open PR must also count. Any auto path touching PRs on archived sessions must respect `noarchivedrevival`.
- **Two sessions, one PR** (worktree session plus parent; rework session plus original): relation is many-to-many (PR can belong to several sessions; session to several PRs). The webhook refresh already fans out to all matching instances.
- **Backlog item with several sessions each opening a PR**: `ItemSession` already models item to many sessions; which PR does the item track? Completion rule needs a decision (all merged/closed vs. primary PR merged).
- **Un-linking / correction**: today persistence cannot clear a PR (only non-empty writes). A wrongly linked PR (artifact extractor can pick any mentioned PR) needs a remove/unlink action, or at least "not mine" marking, plus audit.
- **Trust**: sources differ (agent-claimed, terminal-scanned, transcript-mined, API-created, GitHub-verified). The relation should record provenance and verified flag; `report_pr_created` already distinguishes verified vs override.
- **Title-keyed updates** (`UpdateGitHubPRNumber(title)`): renames or duplicate titles can mis-target; new relation should key on session UUID.
- **Actions need a target**: merge/close/comment/ask-to-fix with multiple PRs must take a PR id; the default needs to be unambiguous (most recent open, else most recent), and UI must say which PR an action will hit.
- **Mobile**: session card currently shows one `#N` chip; with N PRs need a compact "N PRs" chip with expansion. Review-queue "has PR" filter semantics (any open PR?).
- **Notifications/inbox** (`inbox-pr-watch-consumer`, `prwatch_*`): one entry per session today; with multiple PRs entries should be per PR to avoid collapsing a merged PR's state over an open one.
- **Rollout parity**: requirements say legacy fields stay written in parallel; the legacy "current PR" must be defined by the same rule the UI uses or the flag-off view diverges.
- **Unresolved from code**: whether Jules outputs[] ever carry more than one PR (the Jules plan lists this as open); whether `RunOneShot` PR overwrite occurs in practice.

## 9. Answers to requirements Open Questions

1. *Where is the single field overwritten?* `SetGitHubPR` (create-PR RPC, one-shot lifecycle), `scanAndLinkPRURL` (only when unlinked), `setGitHubResolutionLocked` (PR-URL sessions), poller/artifact discovery via `SetGitHubPRNumber` (only when 0). Only the two `SetGitHubPR` call sites truly overwrite an existing PR. No flow records a second PR anywhere.
2. *What does a backlog item store?* One `pr_url` + `pr_number` on the item; `pr_pending` and every reconciler key on `PrNumber > 0`; reassignment is guarded and refused once the current PR is merged.
3. *Explicit vs derived active PR?* Evidence favors derived default (most recent open, tie-break by newest) with explicit PR id on every action; agents have no reliable way to name the active PR today.
4. *Jules / up-next-tabs-pr-nudge need the relation?* Jules: record all `Outputs[]` PRs through it (currently drops extras). Nudge/PR-watch consume `prwatch_sessions.go:100` per-session; should become per-PR.
