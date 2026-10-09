# Research: Feature Landscape — up-next-tabs-pr-nudge

Confidence labels: VERIFIED = opened in this repo; INFERRED = reasoned, not run.

## 1. Existing building blocks (VERIFIED)

| Need | Existing piece | Location |
|---|---|---|
| Tab a11y pattern | `role="tablist"`/`role="tab"`/`aria-selected` with arrow-key nav | `web-app/src/components/sessions/SessionDetailView.tsx:877-909`; also `GitHubPRsSection.tsx:445` (auth tabs), `WindowTabStrip.tsx`, `MobilePaneTabStrip.tsx`. No shared `<Tabs>` primitive found; a new one risks jscpd duplication, so extract or reuse. |
| PR fix prompt | `fixContext` string built in `session/git/worktree_git.go:803-837` with `## Merge conflict`, `## Failing CI checks`, `## Review: changes requested by @x`, `## Reviewer comments` sections | Already holds failing checks + comment bodies; a server-side RPC can reuse it rather than duplicating in the client. |
| Safe delivery | `steerActiveSessionForPRFix` (`server/services/backlog_service_pr_fix_steer.go:245-316`): per-item `steerInFlight` guard, `SessionProgram` check, `IsReadyForSteer` (busy/unknown degrades, never guesses), signature+cooldown dedup, `buildSteerMessage` (program-specific suffix, UTF-8-safe truncation `:190-230`) | Keyed by backlog item, not PR; its dedup/cooldown is automation-oriented and should NOT apply to an explicit user click. |
| Write-time gate | `session.VerifyNudgeSafeToWrite` = `CheckNudgeEligible` (idle, settled for a window) then `VerifyPaneOwnershipBeforeWrite` (tmux `STAPLER_SESSION_UUID` check) — `session/nudge_gate.go:42,102,132` | Best candidate gate for the click path. Refusal semantic ("decline, don't retry") is in `server/mcp/tools_diagnose.go:62`. |
| MCP steer | `steer_session` always appends newline, not rate-limited, behind diagnose role gate (`server/mcp/tools_terminal.go:147-158`) | MCP-only; the web UI needs a ConnectRPC method instead. |
| PR data | `UserPR` proto (`types.proto:1769-1787`): `check_conclusion`, `approved_count`, `changes_req_count`, `session_ids`, `local_worktree_path`. No failing-check names, no unresolved-thread count. |
| Related plans | `project_plans/pr-fix-steering/` (research/{features,pitfalls,ux,architecture}.md, ADR-001 exact program match, ADR-002 SteerFailed stuck reason); `github-pr-status/`; `pr-comment-check-runs/` (policy about *posting* comments, prefers check runs — relevant only in that check runs are the failure source of truth); `pr-event-webhooks/`; `github-call-optimization/`, `github-api-usage-tracking/` (quota). |

Gap: no `reviewThreads`/`isResolved` GraphQL anywhere in Go (grep of `*.go` returned nothing), so "unresolved comments" is genuinely new data. Existing "reviewer comments" are what `worktree_git.go` already fetches for fixContext — check how it sources them before adding a second fetch.

## 2. Industry reference points (INFERRED, not web-verified)
- GitHub's own PR list: status icon + review decision + checks summary; "Needs your attention" style grouping (Graphite, Reviewable, Trunk) with counts per tab.
- Copilot/Cursor "fix CI" buttons: one-click, send failing job log excerpt to the agent. Pattern: prompt includes check name, a link, and a log tail, not just "CI failed".
- Tab-with-badge dashboards (Linear inbox, GitHub notifications): persisted last tab, count badges that mean "needs action" rather than "total".

## 3. Edge cases and failure modes the design must handle

Nudge delivery
1. Session busy/mid-turn, paused, stopped, or tmux gone: reuse `VerifyNudgeSafeToWrite`; UI must show a distinct non-error "session busy, try again" result, not a generic failure. Never queue silently.
2. Pane identity changed between click and write (tmux server restart): ownership check handles it; surface refusal text.
3. Non-Claude programs (aider, etc.): `buildSteerMessage` suffix is program-specific and ADR-001 uses exact program match; decide whether the click path honors the same gating or sends plain text.
4. Prompt injection: fixContext embeds unauthenticated PR comment bodies verbatim into a PTY (the existing code calls this out as a security concern). A one-click, un-editable prompt removes the human review step, so keep the idle-only gate, truncation, and consider a "from GitHub, untrusted" framing line. Control characters/escape sequences in comment bodies should be stripped (check whether `truncateUTF8Bytes` does this; INFERRED not).
5. Size: long comment threads — reuse truncation with pointer suffix.
6. Double click / two tabs / webhook-steer racing: `steerInFlight` is per backlog item; a PR-keyed click could race an automated steer to the same session. Need a shared per-session in-flight guard, and the click should record into dedup state so automation does not immediately re-steer with the same reason.
7. Stale data: PR went green or merged between render and click. Rebuild the prompt server-side at click time and refuse with "nothing to fix" if empty, instead of trusting client-rendered state.
8. Multiple sessions per PR: default = most recently active; define "active" (last output vs last user input) and exclude paused/stopped from the default. Picker must not mis-target; show session title + status in the chooser.
9. No session: "+ Session" seeds the prompt; confirm the seed path (`create_session_for_pr`) accepts an initial prompt and behaves for fork PRs.

Data/mapping
10. PR→session by branch: renamed branches, fork PRs (same branch name, different owner), same branch in two repos, detached HEAD, paused worktree (path deleted, branch kept — see `instance-lock-free-reads.md` ActiveDir vs ExistingDir). Use `Workspace().RepoRoot` + branch + owner/repo match, not branch alone.
11. Draft PRs, PRs from other authors where the user is only a reviewer, bot PRs: define what "needs attention" counts (draft failing CI probably should not badge).
12. Pending vs failed vs unknown checks: only `failure`/`timed_out`/`action_required` should offer a nudge; `pending` must not.
13. Rate limit/ETag: per-PR check-run + thread fetch multiplies calls (tens of PRs). Fetch lazily (on card expand/hover or on nudge click) or batch via one GraphQL query with `reviewThreads`+`statusCheckRollup`; route through `NewConditionalRequest*` (norawghrequest lint). GHE host support (`newGHRequestForHostWithToken`).
14. "Connecting to GitHub…" / unauthenticated / rate-limited state: PRs tab is default, so these states are now the first thing seen; need an in-tab auth prompt and last-known data with a stale marker.
15. Outdated/resolved threads, comments from the user themself, and bot comments should not count as "unresolved needing action".

Tabs/state
16. localStorage vs `?tab=` vs `?item=`: precedence should be `?item=` > `?tab=` > localStorage > default(PRs). Do not write `?tab=` into history on every click (use `replace`) or Back becomes a trap. Invalid/legacy values fall back to default; SSR/hydration mismatch with localStorage (read in effect, not render).
17. Badge semantics: `UnfinishedNavBadge` counts today's items; changing meaning silently confuses the existing nav. Keep nav badge unchanged, add per-tab badges separately (open question in requirements resolved toward "unchanged").
18. Badges for inactive tabs need data for inactive tabs: keep hooks mounted (or lift data) so counts do not flash 0 → N; unmounting tab panels loses filter/scroll state — preserve filters across tab switches.
19. Keyboard/a11y: arrow-key roving tabindex, `aria-controls`, `tabpanel` labelled; e2e locators by role per `e2e-test-conventions`.
20. Zero-state tabs: empty PRs tab as default should say so and offer a link to the next non-empty tab.

## 4. Unstated user needs
- Trust: the user can't edit the prompt, so they need to see what was sent (toast with first lines, or a note in the session) and an undo-less but clear confirmation of which session received it.
- Result feedback: "sent" vs "agent started working" — at minimum a toast linking to the session; ideally status flips to working on the card.
- Avoid repeat nudges: show "nudged 3m ago" on the card and disable/confirm on re-click within the cooldown, same reason signature.
- Per-reason granularity: separate CI-only / comments-only / conflict nudge, or one combined prompt; default to everything outstanding but the card should show what the prompt will contain (counts chips act as the preview).
- Merge conflict is a third failure mode already in fixContext; the card's "needs attention" should include it even though requirements list only CI and comments.
- Sorting: PRs needing attention first; filters (has session / no session) already exist at `GitHubPRsSection.tsx:763` and must survive.
- Cross-repo scale: PRs grouped or labeled by repo; hundreds of worktrees tab needs the existing filters unchanged.
- Audit/observability: log nudge click, target session, reason signature (the request-logging-only requirement is enough, but a notification-history entry like `notifyActiveSessionSteered` keeps parity).
- Permissions: ConnectRPC endpoint writes into a PTY from the browser; confirm it is behind the same local-auth as other write RPCs and that remote-access mode doesn't expose it more widely than `write_to_session`.

## 5. Answers toward the open questions
- Source of unresolved comments: GraphQL `reviewThreads(isResolved,isOutdated)` is the accurate source; REST review comments cannot tell resolved from unresolved. Failing check names: `statusCheckRollup`/check-runs. Cost is the open issue (item 13). UNVERIFIED: whether current polling already fetches either.
- Reuse fixContext server-side: yes, recommended — one builder, one truncation/sanitization path. Caveat: it is built in `session/git/worktree_git.go` from a worktree context; verify it can be called with only owner/repo/number.
- `UnfinishedNavBadge`: keep unchanged.

## 6. Risks to flag for planning
- Highest: prompt injection via un-editable one-click send (item 4) and automation/click races (item 6).
- Medium: GitHub quota from per-PR detail fetches; PR→session mapping accuracy.
- Low: tab state precedence; jscpd from new tab tests (mock block growth is expected).
