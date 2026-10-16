# Pitfalls and risks: up-next-tabs-pr-nudge

Confidence: VERIFIED = opened/ran in this repo; INFERRED = general framework knowledge, not re-checked here.

## 1. Tab state: SSR/hydration, localStorage, URL

- VERIFIED: `web-app/src/app/unfinished/page.tsx` already wraps `UnfinishedTab` in `<Suspense>` (needed because `useSearchParams` in a client component opts the route into CSR bailout during static build). Keep the Suspense boundary; any new component that calls `useSearchParams` must sit beneath it. Tests mock `next/navigation` (`UnfinishedTab.test.tsx:7`).
- INFERRED: Reading `localStorage` in a `useState` initializer renders a different tab on the client than the server HTML, which causes a hydration mismatch warning and a flash of the wrong tab. Design: server and first client render use the deterministic default (PRs), then a `useEffect` (or `useSyncExternalStore` with a server snapshot) applies the stored tab. Accept one-frame flicker, or render a neutral skeleton until resolved. Do not branch on `typeof window` during render.
- Precedence must be a single pure function: `?item=` (forces Stuck) > `?tab=` > localStorage > default `prs`. Write it once, unit-test the table.
- Two writers fight: URL and localStorage. Make the URL the source of truth when present; write localStorage only on user tab clicks, not on programmatic tab selection from `?item=` (otherwise a one-off deep link permanently changes the remembered tab).
- Tab clicks should `router.replace` (not `push`) with `?tab=` to avoid polluting history, and must preserve unrelated params. Do not drop `?item=` silently: after the user leaves the Stuck tab, clear `item` or the deep link re-forces Stuck on the next render.
- Invalid or stale values (`?tab=bogus`, a tab removed later, corrupt localStorage JSON, localStorage throwing in private mode or when blocked) must fall back to default without throwing. Wrap storage access in try/catch.
- Hidden tabs: `StuckItemsSection`'s `focusItemId` expand/scroll logic only works when mounted. If inactive tabs unmount, deep link scroll must happen after the tab switch mounts it. Decide mount-all-hidden vs lazy-mount; lazy-mount also avoids running `useGitHubPRs` polling for a tab nobody sees, but badge counts then need data from a hook that stays mounted.
- Badge counts (PRs needing attention, stuck count) need data from tabs that are not mounted; hoist the hooks to the parent so counts do not flicker to 0 on tab change. Audit `UnfinishedNavBadge` for shared hooks to avoid doubling requests (the requirements leave its semantics unchanged, so do not alter it).
- A11y: use ARIA `tablist`/`tab`/`tabpanel`, `aria-selected`, roving tabindex, arrow-key navigation. The e2e suite runs Axe and blocks on WCAG AA (`tests/e2e/a11y-route-sweep.spec.ts`, `accessibility.spec.ts`); badge chips need AA contrast.

## 2. GitHub data cost, rate limits, GHE

- VERIFIED: `github/` has `etag_cache.go`, `rate_limit.go` (`RateLimiter`, `AdmitOrigin(origin, resource)`, per-resource quota), `hosts.go` (`RestBaseURLForHost`, `graphQLURLForHost`, GHE override). `.claude/rules/norawghrequest.md` forbids raw `http.NewRequest` to GH URLs; use `NewConditionalRequest*`/`newGHRequestForHostWithToken`/`newGHGraphQLRequestForHostWithToken`.
- VERIFIED: no `reviewThreads`/`isResolved` appears in `github/*.go` today. Unresolved-thread counts are new GraphQL surface. REST has no "resolved" state for review comments, so REST review comments cannot answer "unresolved"; GraphQL `reviewThreads { isResolved isOutdated }` is the only accurate source (INFERRED from GitHub API design).
- GraphQL has no ETag/304 savings (INFERRED); it is costed by node count against a separate 5000-point/hour budget. Design against N+1: one batched query per repo (or aliased multi-PR query) with `reviewThreads(first: 50)` plus `statusCheckRollup`, not one query per PR per poll. Cap pagination; if a PR has more than the cap, show "50+" instead of paging.
- Better: fetch thread/check detail lazily (on card expand or on nudge click) and refresh only PRs whose REST ETag changed, so unchanged PRs cost zero. Counts on badges then come from already-fetched `UserPR` fields; detail is on demand.
- Route through `AdmitOrigin` with a distinct `CallOrigin` so UI-triggered detail fetches cannot starve background pollers (`github/call_origin.go`, `call_site.go`). When limited (`IsLimited`), degrade to cached/stale data with a visible "rate limited until HH:MM" state, not an error or blank.
- GHE: every new call must take the PR's host, not assume github.com. Use `graphQLURLForHost(host)`. GHE servers can be older and lack fields (e.g. `statusCheckRollup` shape, check suites); tolerate missing fields with partial results. Per-host tokens (`newGHRequestForHostWithToken`). Test with a GHE fixture host (`SetEnterpriseBaseURLOverride`).
- Check-run names: `statusCheckRollup.contexts` mixes `CheckRun` and legacy `StatusContext`; handle both, and cap the failing-name list (matrix builds can produce dozens) before putting it in a prompt.
- Proto: additive fields only, new field numbers, never renumber. Zero-value ambiguity: `unresolved_comment_count = 0` is indistinguishable from "unknown". Use `optional` or a separate `details_loaded` flag so the UI shows "?" rather than "0 unresolved" when data is missing or rate limited.

## 3. PR to session mapping

- PRs map by branch checkout (requirements). Failure modes: fork PRs (head repo differs, same branch name collides with an unrelated local branch), renamed branches, paused sessions whose worktree was deleted (`pause_session` deletes the worktree but keeps the branch; see `.claude/rules/instance-lock-free-reads.md`), two sessions on the same branch, and same branch name in different repos. Match on (host, owner/repo, branch), not branch alone. Read paths via `Workspace()` fields, `RepoRoot`/`WorkspaceKey()` for "same repository", not `ExistingDir`.
- "Default most recently active" must be defined by a concrete field and handle ties and missing timestamps deterministically. Exclude archived or stopped sessions from the picker, or show them disabled with the reason.

## 4. Nudging a busy, paused, or dead session

- VERIFIED: existing safeguards live in `server/mcp/tools_diagnose.go` (`nudgeSession`, `checkNudgeCapForWrite`, `claimNudgeAttemptForWrite`, `resolveNudgeTarget`, `performNudge`) and `server/services/backlog_service_pr_fix_steer.go` (`steerActiveSessionForPRFix`, `buildSteerMessage`, `isClaudeCodeProgram`, `truncateUTF8Bytes`, `degradeToRespawnBlocked`). Reuse the steering primitive; do not write a second `write_to_session` path.
- Mid-turn: writing keystrokes into a running agent either queues the text or interleaves with its output depending on the program. Check status first; for a busy session either queue with explicit UI text ("will be delivered when idle") or refuse with a clear reason. Do not silently send. Program-specific behavior is already encoded via `isClaudeCodeProgram`; non-Claude programs (aider, etc.) need explicit handling, not an assumed prompt format.
- Paused/stopped/no tmux: return a typed error and the UI offers Resume, not a generic failure toast. The RPC response must say what happened (delivered, queued, rejected-busy, rejected-paused, duplicate-suppressed), and the UI must reflect it (read back, do not trust a 200).
- Prompt size and injection: PR comment text is untrusted input injected into an agent as instructions. Truncate (existing `truncateUTF8Bytes`), strip control characters and terminal escape sequences, and frame comment bodies as quoted data with links rather than inline imperatives. Prefer thread URLs plus short excerpts. Out of scope says "no editing the prompt", which raises the importance of sanitizing since the user cannot review it first. At minimum show the generated prompt in a tooltip or toast after sending.
- Authorization: the RPC must be allowed only for the local user's own sessions; do not expose a generic "write arbitrary text to session X" endpoint. The RPC takes (pr identity, session id) and builds the prompt server-side; the client never supplies the text.

## 5. Duplicate nudges and staleness

- VERIFIED: automated steering has a 5-minute cooldown with a reason signature (`steerCooldown`, `isDuplicateSteerReason`, `nextLastSteerReason` advance only on successful delivery) and `session/nudge_dedup.go`. Manual nudges can collide with the automatic path for backlog-item sessions: the user clicks, then `pr-fix-steering` sends the same reason. Share the dedup state (or record the manual send in it) so the auto path suppresses a repeat, and decide whether a manual click bypasses the cooldown (probably yes, but with a UI debounce).
- Double click / two tabs / retry: disable the button while in flight, and make the RPC idempotent per (session, reason signature) within a short window. Show "Nudged 2m ago" state on the card so the user does not re-send.
- Stale data: the card can show failing CI that has since turned green (polling interval) or comments that were resolved. Re-fetch the PR's current state server-side at nudge time and build the prompt from fresh data; if nothing is failing now, return "nothing to fix" instead of sending an obsolete prompt. Show data age on the PR tab and keep stale data visible when refresh fails.
- Sessions list and PR list update on different schedules; the linked-session set can change between render and click. Resolve the session by id server-side and reject if it no longer matches the PR.
- "+ Session" seeded with a fix prompt: the same create-session duplicate risk (two clicks create two worktrees on one branch); disable on click and check for an existing linked session at the server.

## 6. Repo gates

- jscpd: VERIFIED `web-app/.jscpd.json` threshold is currently **0.14** (not 0.12 as requirements and CLAUDE.md state); update the docs or confirm which is authoritative. Gate is absolute, not diff-scoped. Mitigate: put new tab shell, storage hook, and PR-card sub-parts into shared components; in tests, share fixtures via a helper module (not copy-pasted blocks); expect `jest.mock` registration repeats to be irreducible. Run `pnpm run lint:duplicates` in `web-app/` before pushing. Go side: `dupl` is diff-scoped against `origin/main`; keep the nudge RPC from copy-pasting `performNudge`/steer logic (extract and share).
- e2e (VERIFIED existing `tests/e2e/unfinished-work.spec.ts`, `backlog-stuck-items.spec.ts`, `nav-navigation.spec.ts`, `a11y-route-sweep.spec.ts` touch this page): moving sections into tabs will break specs that expect Stuck/PR content visible by default. Update them to click the tab or set `?tab=`. Conventions: `// @feature` header, no `waitForTimeout`, only `data-testid` or ARIA roles (tabs get `role=tab` with name), page helpers in `tests/e2e/pages/`. localStorage persists across tests in one browser context; clear it in setup or tests become order dependent. Test server is isolated and has no GitHub, so PR-tab e2e needs a stubbed/mocked PR source or focus on unit tests for PR cards.
- Feature registry: `make registry-generate` and commit changed files after adding an RPC or `// +feature:` / `// +api:` markers. Proto: `make proto-gen`; generated output is gitignored per repo policy, commit only proto sources.
- Manual testing on a separate instance (port block 62871+), never `make install-service`.

## 7. Explicitly design against (checklist)

1. Hydration mismatch: deterministic first render, apply stored tab after mount.
2. Single precedence function for `?item=` > `?tab=` > storage > default; deep link must not overwrite stored preference.
3. Suspense boundary preserved around all `useSearchParams` users.
4. Badge counts independent of which tab is mounted; no duplicated polling.
5. GraphQL cost: batch per repo, cap pagination, lazy detail, distinct call origin, honor `IsLimited`.
6. GHE: host-aware URLs/tokens everywhere; tolerate missing fields.
7. Unknown vs zero in proto (`optional` / loaded flag).
8. Session match on (host, repo, branch); handle forks, paused worktrees, multiple sessions.
9. Server-built prompt only; sanitize and truncate untrusted comment text; re-fetch fresh state at send time.
10. Typed nudge outcomes (delivered / busy / paused / duplicate / nothing-to-fix) surfaced in UI.
11. Shared dedup/cooldown with `pr-fix-steering`; in-flight button disable; idempotency window.
12. jscpd 0.14 threshold (docs say 0.12), existing e2e specs updated for tabbed layout, localStorage reset in e2e setup.
