# Build vs Buy: Up Next tabs, PR data, nudge delivery

Evidence level: VERIFIED = file opened/grepped in this worktree; INFERRED = reasoned, not run. No code was executed.

## (a) Accessible tabs

Existing facts (VERIFIED):
- `@radix-ui/react-tabs ^1.1.13` is already a dependency (`web-app/package.json:77`), used in `web-app/src/app/settings/page.tsx:6,38-40` (`Tabs.Root`/`Tabs.List`).
- Hand-rolled `role="tablist"`/`role="tab"` exists in at least `WindowTabStrip.tsx`, `MobilePaneTabStrip.tsx`, `SessionDetailView.tsx`, `BacklogItemForm.tsx`, `GitHubPRsSection.tsx:445-459`, `logs/page.tsx`, `EscapeAnalyticsPage.tsx`. No shared in-repo Tabs component found (INFERRED from grep; not every file opened).
- No Headless UI or react-aria dependency (grep of package.json).

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Radix Tabs (already installed) | Zero new dependency or bundle cost; roving tabindex, arrow keys, Home/End, `aria-controls`/`aria-labelledby` handled; precedent in settings page; unstyled so works with vanilla-extract; supports controlled `value` for localStorage + `?tab=` sync | Unmounts inactive panels by default (use `forceMount` or accept remount; Stuck/Worktrees hold filter state, so lift state or `forceMount`); settings page uses uncontrolled `defaultValue`, so controlled pattern is new here | **Recommended** |
| Existing hand-rolled strips (WindowTabStrip, etc.) | In-repo styling | Bespoke, session/window-specific; no shared abstraction, so reuse means copy-paste (jscpd risk); a11y behavior unverified | Not recommended |
| Headless UI / react-aria | Battle-tested | New dependency, second tabs idiom next to Radix; no capability Radix lacks here | Not recommended |
| Hand-rolled new | Full control | Re-implements keyboard/ARIA contract; this is the classic place LLM-generated code ships subtly wrong (focus management, `aria-selected` vs tabindex) | Not recommended |

LLM-generated vs battle-tested: the keyboard/ARIA tab pattern is a solved, spec-defined problem; prefer the library the repo already ships. Fork/adapt: nothing to fork. Wrap Radix in a thin `UpNextTabs` component (badge slot, controlled value) rather than adapting another strip.

## (b) PR comment / check data

Existing facts (VERIFIED):
- `github/` package is a native HTTP client: `NewConditionalRequest*`, ETag cache (`etag_cache.go`), rate-limit tracking, `client_graphql.go` with `GetPRInfoGraphQL` mapping reviews and checks (`mapPRInfoGraphQLReviews`, `mapPRInfoGraphQLChecks`), `user_pr_cache.go` feeding `UserPR`. `gh_exec.go` wraps the `gh` CLI as fallback.
- `go-github` is NOT in `go.mod`.
- grep for `reviewThreads`/`isResolved` in `github/*.go` (non-test) found nothing, so unresolved-thread count is not fetched today.
- `session/git/worktree_git.go:799` `PRStatus.render()` already produces failing-check and comment sections (`failedChecks`, `commentReviews`, `generalComments`).
- `.claude/rules/norawghrequest.md` mandates the in-repo request constructors; a lint analyzer enforces it.

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Extend existing server code (add `reviewThreads(isResolved)` + check names to the GraphQL query/mapper; add additive `UserPR` fields) | Inherits ETag/rate-limit/GHE-host handling and the lint rule; `CheckItem` data (failing check names) is probably already mapped by `mapPRInfoGraphQLChecks` (INFERRED, verify); one query extension, no new dependency | GraphQL has no 304/ETag savings (GraphQL POSTs are not conditional), so per-PR detail costs quota; must fetch lazily (on nudge or on card expand) rather than for every PR on every poll | **Recommended** (fetch lazily at nudge time via `GetPRInfoGraphQL`; only add cheap counts to the list poll if it can ride the existing user-PR query) |
| go-github | Typed models, pagination helpers | New dependency; bypasses `NewConditionalRequest*` and rate-limit telemetry unless wrapped in a custom `http.RoundTripper`; duplicates `client.go`; violates the spirit of norawghrequest | Not recommended |
| `gh` CLI via `gh_exec.go` | Already used as fallback; handles auth | Subprocess per call, no ETag, text parsing, no quota awareness, repo has a `prefer-go-git-over-subshells` stance | Viable only as fallback |

LLM-generated vs battle-tested: here "battle-tested" means the repo's own ETag/rate-limit layer, which encodes past incidents; a generated parallel client would lose that. Fork/adapt: adapt `PRStatus.render()` inputs rather than forking a client.

## (c) Nudge delivery

Existing facts (VERIFIED):
- `SessionService.SteerActiveSession(ctx, sessionUUID, message)` at `server/services/session_service.go:1110`; consumed via the `sessionSteerer` interface (`backlog_service.go:107`) with `IsReadyForSteer` gate.
- `backlog_service_pr_fix_steer.go`: `buildSteerMessage(program, fixContext)` (line 190) with truncation and program gating; dedup/cooldown keyed by backlog `itemID` (`steerDedup`, `isDuplicateSteerReason`).
- `fixContext` text comes from `session/git/worktree_git.go` `PRStatus.render()`; header strings are pinned by a test.
- MCP `steer_session` and `diagnose_nudge_session` (`server/mcp/tools_diagnose.go`) exist; no dedicated "steer by session" ConnectRPC found in `session.proto` (grep for `steer` shows only `steer_message` field at line 918), so a new RPC is required for the UI.

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| New thin RPC (`NudgeSessionForPR`) that builds text from fresh PR data using shared render/`buildSteerMessage`, then calls `SteerActiveSession` | Reuses delivery + truncation + readiness gate; consistent prompt wording; additive proto | `render()` is a method on `PRStatus` (unexported fields) in `session/git`; need an exported constructor/adapter from `PRInfo`; dedup state is item-scoped, so manual nudges need their own (or no) cooldown | **Recommended** |
| Call `steerActiveSessionForPRFix` directly | Maximum reuse | Coupled to backlog items (`itemID`, status transitions, notifications, stuck reasons); a PR card may have no backlog item | Not recommended |
| New bespoke prompt builder + `write_to_session` | No coupling | Drifts from fixContext wording; skips readiness/truncation; duplicates logic (jscpd/dupl gates) | Not recommended |

Safeguards to reuse: `IsReadyForSteer` (mid-turn/paused) and the `diagnose_nudge_session` semantics. Manual one-click should skip the auto-steer dedup cooldown (user intent) but still surface "session not ready" to the UI.

LLM-generated vs battle-tested: the prompt text has pinned tests and dedup identity; reusing it keeps that coverage. Fork/adapt: extract an exported `FormatFixContext(PRInfo)` from `render()` instead of copying; keep the header-pin test in step.

## Summary of recommendations
1. Tabs: Radix Tabs (already installed), controlled, thin wrapper.
2. Data: extend existing `github/` GraphQL path lazily; no go-github.
3. Nudge: new thin RPC over `SteerActiveSession` + shared fixContext/`buildSteerMessage`.

## Gaps
- Not verified: whether `UserPR` list poll can cheaply carry unresolved-thread counts; whether `CheckItem` already includes failing check names/URLs.
- Not verified: Radix `forceMount` behavior with current panel components.
