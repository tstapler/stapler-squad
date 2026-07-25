# Requirements: session-history-metadata

**Date**: 2026-06-22
**Type**: feature addition

## Problem Statement

Session users (developers running AI agents) have no way to see what a session actually *produced* — which PRs were opened, which commits were made, which external URLs were referenced — without manually reading raw terminal scrollback or running `git log` themselves. The session detail view shows status and terminal output but surfaces no structured artifacts. This makes it hard to quickly review what happened in a session or hand off work to another person.

The JSONL conversation files already exist (Claude Code writes them to `~/.claude/projects/<encoded-path>/<uuid>.jsonl`), and `HistoryLinker` already correlates sessions to their conversation files. The gap is that nothing reads and extracts structured metadata from those files.

## Users / Consumers

- **Human operators / developers**: Review what a session accomplished, share session summaries, navigate to PRs or commits created during a session.
- **The system itself**: The PR status poller and session driver already benefit from knowing PR numbers; the metadata extractor would feed that same pipeline.

## Success Metrics

- Session detail view (or a new metadata panel) displays at minimum: PR links, commit SHAs, and external URLs found in the session's conversation history
- Extraction runs automatically in the background — no user action required
- Metadata persists across app restarts (survives session reload from DB)
- Extraction adds < 100 ms to session load time for typical JSONL files (≤ 10k lines)
- At least one PR URL discovered via JSONL scanning feeds the existing PR status poller (closes the retroactive recovery gap identified in PR integration debugging)

## Constraints

- Must not block the main request path — extraction runs asynchronously
- Must gracefully handle missing or unreadable JSONL files (file not yet created, process still running, file corrupted)
- Must not re-read unchanged JSONL files on every tick — use file modification time or line count delta to gate re-extraction
- No deadline; ship when ready

## Scope

### In Scope

- **JSONL scanner**: reads a session's conversation JSONL file and extracts:
  - GitHub PR URLs (`github.com/*/pull/*`) → PR number + owner + repo
  - Commit SHAs from `git commit` / `git push` tool output (40-char hex strings following "commit" keyword)
  - External URLs (http/https) from assistant text and tool outputs
  - File paths mentioned in tool calls that match known patterns (optional, lower priority)
- **Persistence**: store extracted metadata in the ent DB (or as a JSON blob field) so it survives restarts
- **Frontend panel**: a "Session Artifacts" or "History" section on the session detail page showing the extracted items as rich links (PR badge with status, commit link, external URL list)
- **PR poller integration**: if a PR URL is found via JSONL scan and the session's `GitHubPRNumber == 0`, auto-populate it (closing the retroactive recovery gap)
- **Incremental re-scan**: re-scan when JSONL file grows (new lines appended), not on a fixed timer

### Out of Scope

- Parsing agent sub-conversation files (`agent-*.jsonl`)
- Full-text search across all sessions' histories
- Summarization or AI-generated summaries of session content
- Modifying the JSONL format or Claude Code's write behavior
- Session-to-session linking (e.g. detecting that two sessions worked on the same PR)

## Open Questions

1. **Storage**: Should extracted metadata be stored as typed ent fields (one column per artifact type) or as a single JSON blob (`session_artifacts TEXT`)? Blob is simpler but less queryable; typed fields are more structured but require schema migrations for each new artifact type.
2. **Trigger**: Should scanning be triggered by the `HistoryLinker` (already fires when a JSONL file is detected/modified) or by a new dedicated watcher? Reusing HistoryLinker's fsnotify watcher avoids a second filesystem watch per session.
3. **Deduplication**: If the same PR URL appears 50 times in a conversation, should we store one artifact or all occurrences? (Recommendation: one, keyed by URL.)
4. **Frontend component**: Where does the metadata panel live — a new tab on session detail, a sidebar panel, or inline beneath the terminal? The existing `session-detail` page uses a tab layout; adding a "Artifacts" tab is the least-invasive option.
5. **HeadRef validation**: Should JSONL-extracted PR URLs be validated against the GitHub API to confirm `HeadRef` matches `inst.Branch` before being accepted? Or trust the URL as-is and let the poller validate on next tick?
