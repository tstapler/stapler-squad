# Feature Research: session-history-metadata

**Date**: 2026-06-22
**Researcher**: Claude Sonnet 4.6

---

## 1. Comparable "Session Artifact" Features in Developer Tools

### GitHub Actions Run Summaries

GitHub Actions introduced Job Summaries (2022) allowing steps to write structured Markdown to `$GITHUB_STEP_SUMMARY`. The finished run shows a single summary page with: duration, status per step, custom artifact tables, and log links. Key design choices:
- **Categorized, not raw**: structured tables/callouts rather than raw log scrollback
- **Persistent after expiry**: job logs expire in 90 days, summaries persist longer
- **Linkable**: each job summary has a stable URL the developer can share
- **Artifacts separate from logs**: the Artifacts panel lists downloadable files, independent from the log viewer

Takeaway: users expect artifacts (files, links, outputs) in a dedicated panel separated from raw logs — not buried inline.

### Linear Activity Feeds

Linear shows a per-issue activity timeline: who changed what, when, and to what value. For GitHub-integrated projects it inlines PR state changes (opened, merged, closed) as timeline entries with PR title + number. The timeline interleaves code events with human comments, giving a causal narrative.

Takeaway: a chronological timeline view (not just a categorized list) helps users understand *what happened in order*, which is valuable for AI sessions where the agent may have created several PRs or commits sequentially.

### Datadog Continuous Profiler / Trace Explorer

For long-running agent tasks, Datadog's session replay / trace model is instructive: a waterfall of spans with start/end times gives an at-a-glance picture of where time was spent. They surface "most expensive" sub-operations up front.

Takeaway: for multi-hour sessions, a time-distribution view (phase 1: research, phase 2: implementation) can answer "what took so long?" faster than a list of commits.

### Buildkite / CircleCI Artifact Panels

Both show artifacts as a flat file list with download links and MIME-type icons. CircleCI additionally shows test result XML parsed into a "Tests" tab with pass/fail/skip counts. Buildkite shows "Annotations" (free-form HTML injected by steps) alongside artifact links.

Takeaway: separate tabs for different artifact types (test results vs files vs URLs) reduce cognitive load compared to one mixed list.

### Claude Code's Own `/cost` and Usage Output

Claude Code itself writes token-usage summaries to its JSONL files. The `usage` field on assistant messages contains `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`. The existing `TokenStore` in this codebase already tracks this. 

Takeaway: the JSONL files already contain structured usage metadata; a scanner can trivially derive session duration (first/last `timestamp` fields) and cost estimates without any AI inference.

### JetBrains Space / GitLab Activity Feeds

Both tools show "recent activity" on a project: commits, MR events, pipeline runs, and comments in a unified timeline. GitLab specifically enriches each MR link with its current state (open / merged / closed), live-refreshed.

Takeaway: artifact links should show live state (PR merged vs open) not just the URL captured at creation time — aligning with the PR poller integration already in scope.

---

## 2. Edge Cases and Failure Modes

### 2.1 JSONL File Still Being Written (Live Session)

**Risk**: scanning a file that is actively being appended can read a partial last line, causing JSON parse errors.

**Mitigation**: the existing `readAllMessagesFromFile` function uses `bufio.Scanner` which handles partial lines gracefully (they produce a JSON unmarshal error that is silently skipped). For incremental re-scan, track the last byte offset scanned and start from there — but only process lines followed by a newline character, never the last incomplete line.

**Design decision**: store `jsonl_scan_offset int64` and `jsonl_scan_complete bool` in the DB. For live sessions, re-scan from `jsonl_scan_offset` on each trigger. Mark `jsonl_scan_complete = true` only after the session stops and a final scan finds no new content.

### 2.2 Very Large JSONL Files (Multi-Hour Sessions, 100k+ Lines)

**Risk**: scanning 100k lines inline with a request will block the response for seconds.

**Observations from codebase**: `readLastNMessagesFromFile` already implements efficient reverse-read in 64 KiB chunks, avoiding full file load. However, URL/SHA extraction requires a forward pass.

**Mitigation**: 
- Run the scanner entirely in a background goroutine, not in the request path
- Wire into the existing `HistoryLinker.RegisterFileCallback` hook — this is exactly the right injection point
- For a file that changes, re-scan only from `jsonl_scan_offset`, not from the beginning
- Cap extraction at 500k lines per scan pass with a configurable limit; log a warning and set a `truncated` flag on the metadata record if exceeded

### 2.3 Multiple JSONL Files for One Session

Claude Code creates a new JSONL file when the user runs `/clear` — a new conversation UUID, a new file, but the same session in stapler-squad. The `HistoryLinker` already handles UUID rotation by updating `HistoryFilePath` on `ScanAll()`. 

**Risk**: after `/clear`, the old file had artifacts; the new file has different (or no) artifacts. A naive approach loses the old scan.

**Mitigation**:
- Persist artifacts per JSONL file path (`jsonl_file_path` column on the artifact record), not per session ID alone
- When `/clear` triggers a new UUID and new file, start a fresh scan; keep old artifact records tagged with their source file
- The UI should show artifacts from all files associated with a session, unioned together

### 2.4 Sessions With No JSONL File (Non-Claude Agents, e.g., Aider)

**Risk**: if `HistoryFilePath` is empty (Aider sessions, external sessions), the scanner has no file to read.

**Mitigation**: the scanner must gate on `instance.HistoryFilePath != ""`. No file → no scan → no error. The "Session Artifacts" panel should gracefully render an empty state: "No conversation history available for this session type."

### 2.5 PR URLs from Draft PRs, Closed PRs, PRs in Forks

**Risk**: a PR URL extracted from a JSONL file may reference a draft or closed PR. The existing `PRStatusPoller` already polls for `github_pr_state` and `github_pr_is_draft`, but only for sessions created from a PR URL (IsPRSession check).

**Mitigation**: 
- When a PR URL is extracted from JSONL, store it in the artifact record with an `artifact_type = "github_pr"` row
- If the PR URL matches the session's working repo, trigger a PRStatusPoller refresh to populate live state
- Display PR state with visual indicators (draft badge, merged badge, closed badge) — re-use `GitHubBadge.tsx` which already exists
- For fork PRs (URL host = github.com but different owner than session repo), display as a plain link without live state polling, to avoid cross-org auth issues

### 2.6 Commit SHAs in Code Snippets vs. Actual Commits

**Risk**: if a user asks Claude to "explain commit abc1234" or references a SHA in a doc comment, that SHA will match a 7–40 hex regex but is not a commit *made by this session*.

**High-confidence signals** that a SHA is "made by this session":
- It appears in the output of a `tool_result` where the `tool_use` `name` field is `"Bash"` and the command contains `git commit` or `git push`
- It appears in a `git push` stdout line that matches `[branch sha]` or the GitHub "remote: Create a pull request" URL pattern
- It appears in a GitHub PR URL that was created (not just mentioned) in this session

**Low-confidence signals to exclude**:
- SHAs that appear inside `tool_use` `input` blocks (Claude is referencing, not creating)
- SHAs that appear inside `type: "user"` messages (user is telling Claude about them, not Claude creating them)

**Design**: extract SHAs only from `tool_result` content blocks where the parent `tool_use` is a `Bash` or `computer` tool call containing commit/push-related commands.

### 2.7 False-Positive URL Extraction

**Risk**: URLs in code comments, documentation strings, test fixtures, or example code will be extracted and shown as "session artifacts."

**Mitigation strategies**:
- Only extract URLs from `tool_result` content (not from `user` messages or Claude's `text` content — those are inputs/explanations, not outputs)
- Apply a domain allowlist for "interesting" external URLs: `github.com`, `gitlab.com`, `npm.js`, `pypi.org`, `docs.*`, `api.*` — filter out `localhost`, `127.0.0.1`, `0.0.0.0`, and clearly-documentation URLs
- De-duplicate by normalized URL (strip trailing slashes, UTM params, fragments)
- Require a minimum of 2 occurrences across different messages OR appearance in a `Bash` result with an HTTP success response, to filter out URLs that were only ever mentioned once in passing

---

## 3. Unstated User Needs

### 3.1 Files Changed (Not Just Commits)

The existing `VcsPanel` (`VcsPanel.tsx`) already shows `git status` output for the working tree, and the `FilesTab` shows the file tree. However, these are point-in-time views — they show current state, not "what was changed during *this* session."

**Unstated need**: users want "what files did this agent touch" as a session-scoped summary. This is different from current diff stats (added/removed line counts) — it's a file list.

**Option**: extract this from JSONL `tool_use` calls where `name` is `"Edit"`, `"Write"`, or `"Bash"` with commands containing file paths. Alternatively, compute it from `git log --name-only --pretty=format: SHA1..SHA2` using the base commit SHA already stored in `session.gitWorktree.baseCommitSha`.

**Recommendation**: prefer git-derived file list (more accurate, already anchored to base commit) over JSONL extraction. The JSONL scanner should only be responsible for commit SHAs, PR URLs, and external URLs. File changes can be surfaced via a `git diff --name-only` call, which avoids false positives from file references in code comments.

### 3.2 Timeline View vs. Categorized List

**Unstated need**: for debugging "why did this session fail?" or "at what point did the PR get created?", a chronological timeline of events is more useful than a flat categorized list.

The existing `DetectionEventsPanel.tsx` demonstrates this pattern — it shows timestamped events in a list.

**Recommendation**: the "Session Artifacts" panel should offer two views:
1. **Timeline** (default for active/recently completed sessions): events in time order — "12:03: commit abc1234", "12:07: PR #42 opened", "12:15: URL https://... referenced"
2. **Categorized** (default for older sessions): PRs / Commits / External URLs in separate sections

The DB schema should store `artifact_timestamp` (extracted from the JSONL `timestamp` field) so both views are possible without rescanning.

### 3.3 Jump-to-Terminal-Moment from Artifact

**Unstated need**: when a user sees "commit abc1234 created at 12:03", they want to click it and jump to that moment in the terminal scrollback to understand the context.

**Feasibility assessment**: 
- Stapler-squad stores scrollback via the existing terminal buffer, but it does not store a time-indexed scrollback position
- The JSONL `timestamp` field maps to wall-clock time, not a terminal buffer offset
- Implementing click-to-jump would require correlating JSONL timestamps with terminal output timestamps — complex and out of scope for V1

**Recommendation**: defer to V2. For V1, show the `timestamp` as a human-readable time label (e.g., "12:03") next to each artifact. This gives users enough context to manually scroll to that area.

### 3.4 Shareable Session Summary Link

**Unstated need**: developers often want to share "what the agent did" with teammates for code review or retrospectives.

**Feasibility**: stapler-squad is a local tool (localhost:8543); there is no multi-user backend or sharing infrastructure. Export to clipboard (as Markdown or plain text) is feasible.

**Recommendation**: add a "Copy summary" button that formats artifacts as Markdown: PR links, commit SHAs, and external URLs in a structured list. This covers the sharing use case without requiring a multi-user server.

### 3.5 Artifact Notifications (Toast When PR Created Mid-Session)

**Unstated need**: if a session creates a PR while the user is watching the terminal, they want an immediate notification ("PR #42 opened") without having to check the Info tab.

**Feasibility**: the JSONL scanner is event-driven via `HistoryLinker.RegisterFileCallback`, which fires on every fsnotify event. This means new artifacts are detected within seconds of being written.

**Implementation**: when a new `github_pr` artifact is discovered for a running session, emit an event on the existing `EventBus` (already used for approval notifications). The frontend's existing notification system (`NotificationContext.tsx`) can consume this and show a toast.

**Recommendation**: include this in V1 — it uses existing infrastructure and provides high value for live monitoring.

---

## 4. Additional Extraction Patterns Worth Considering

### 4.1 npm/pip Package Installs

**Pattern**: in `tool_result` content from `Bash` calls, look for:
- `added N packages` (npm)
- `Successfully installed package==version` (pip)
- `go get github.com/foo/bar@v1.2.3` (Go modules)

**Value**: tells users what dependencies the agent added without reading the full diff. Useful for security audits.

**Recommendation**: include as `artifact_type = "package_install"` — low extraction complexity, high signal.

### 4.2 Docker Image References

**Pattern**: in `tool_result` content from `Bash` calls:
- `docker pull image:tag` or `docker build -t image:tag`
- `FROM image:tag` in written Dockerfiles (via `Write` tool results)

**Value**: moderate; mainly useful for infrastructure-heavy sessions.

**Recommendation**: defer to V2. Adds complexity without clear priority.

### 4.3 API Endpoint Patterns

**Pattern**: HTTP method + path patterns in `tool_result` content:
- `curl -X POST https://api.example.com/v1/resource`
- `fetch('https://...')`

**Value**: low for typical sessions; relevant mainly for API integration work.

**Recommendation**: defer. Overlap with the general URL extraction.

### 4.4 Error Messages / Exception Summary

**Pattern**: in `tool_result` content, look for:
- `Error:`, `Exception:`, `FAILED`, `exit code N` patterns
- Stack trace fragments (lines starting with `at `, `File "`, goroutine lines)

**Value**: a "what went wrong" summary for sessions that ended in failure (session status `Stopped` with non-zero exit code). This directly answers "why did the session fail?" without reading scrollback.

**Recommendation**: include as `artifact_type = "error_event"` with truncated content (first 200 chars of error message). Store error timestamp so timeline view can show error events inline with commits/PRs. Note: the existing `error_event` ent schema already exists — may be able to reuse it.

### 4.5 Session Duration

**Pattern**: `first_message_timestamp` = earliest `timestamp` field in any JSONL message; `last_message_timestamp` = latest.

**Value**: high. "This session ran for 47 minutes" is a useful at-a-glance metric visible in the session list or summary panel.

**Implementation**: these two timestamps are trivially derived during the scan with no regex needed. Store as `jsonl_first_ts` and `jsonl_last_ts` on the `ClaudeSession` ent entity (or a new `SessionArtifactMeta` record).

**Recommendation**: include in V1. Minimal implementation cost, high informational value.

---

## 5. Current Session Detail: What's Already Shown

The `SessionDetailView` Info tab (lines 800–1211 in `SessionDetailView.tsx`) already surfaces:

| Field | Source |
|---|---|
| Session ID, Status, Type | `session` proto |
| Created/Updated timestamps | `session` proto |
| Branch, Path, Working Dir | `session` proto |
| Category, Tags, Auto Yes | `session` proto |
| Program, Launch Command | `session` proto |
| Claude Conversation UUID | `session.claudeSession.sessionId` |
| Claude Project | `session.claudeSession.projectName` |
| History File Path | `session.historyFilePath` |
| Diff Stats (+/- lines) | `session.diffStats` |
| GitHub PR URL/Number/State | `session.githubPrUrl`, `.githubPrNumber`, `.githubPrState` |
| GitHub Reviews | `session.githubApprovedCount`, `.githubChangesReqCount` |
| CI Status | `session.githubCheckConclusion` |
| Launch Prompt, Terminal Prompt | `session.prompt`, `.initialPrompt` |
| Workflow metadata | `session.workflowId`, `.workflowName` |

**Gap**: none of these fields come from JSONL scanning. The Info tab shows what was *configured* for the session, not what it *produced*. The planned "Session Artifacts" panel fills this gap with JSONL-derived outputs.

**Placement recommendation**: add "Artifacts" as a new tab in `SessionDetailView` alongside Terminal, Diff, VCS, Files, Logs, Info. This avoids cluttering the Info tab (already dense) and gives artifacts first-class navigation status. The tab should show a badge count (e.g., "Artifacts (3)") when artifacts exist.

---

## 6. Summary of Key Design Decisions for Implementation

1. **Scanner injection point**: use `HistoryLinker.RegisterFileCallback` (already exists) rather than a second fsnotify watcher. This keeps the infrastructure footprint minimal.

2. **Incremental scanning**: store `jsonl_scan_offset` and only re-scan from that offset on each trigger. Never re-scan the full file on a change event.

3. **Artifact confidence filtering**: extract from `tool_result` content only, not from `user` messages or Claude's explanation text. This is the single highest-impact false-positive filter.

4. **Multiple JSONL files**: store `jsonl_file_path` on artifact records. When a session's JSONL file changes (due to `/clear`), start a new scan from offset 0 for the new file while retaining old artifacts.

5. **Non-Claude sessions**: gate the entire scanner on `HistoryFilePath != ""`. Aider sessions, external sessions, and sessions without a detected JSONL file get an empty artifacts panel, not an error.

6. **Live PR notifications**: emit on EventBus when a new `github_pr` artifact is discovered for a running session. Reuse existing `NotificationContext.tsx` for toast display.

7. **UI placement**: new "Artifacts" tab in `SessionDetailView`, not inside the Info tab. Show badge count when artifacts exist.

8. **Timeline vs. categorized**: store `artifact_timestamp` in DB. Default to timeline view for active sessions, categorized for completed sessions.
