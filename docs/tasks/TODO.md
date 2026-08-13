# Task Tracking Index

This file serves as a lightweight index of all feature plans in `docs/tasks/`.

## Active Plans

| Plan | Status | Stories | Priority |
|------|--------|---------|----------|
| [Review Queue Navigation](review-queue-navigation.md) | Draft | 3 stories, 11 tasks | High |
| [Test Coverage: Core Business Logic](test-coverage-core-logic.md) | Draft | 3 stories, 15 tasks | High |
| [History Page Decomposition](history-page-decomposition.md) | Draft | 5 stories, 20 tasks | Medium |
| [Instance God Object Decomposition](instance-god-object-decomposition.md) | Draft | 5 stories | High |
| [Terminal Decomposition](terminal-decomposition.md) | Draft | 5 stories | High |
| [Dependency Initialization Hardening](dependency-initialization-hardening.md) | Draft | 3 stories | Medium |
| [Session Service Decomposition](session-service-decomposition.md) | Draft | 5 stories | Medium |
| [Domain Invariant Enforcement](domain-invariant-enforcement.md) | Draft | 4 stories | Medium |
| [Circuit Breaker Executor](circuit-breaker-executor.md) | Draft | 4 stories | Medium |
| [Frontend Quick Wins](frontend-quick-wins.md) | Draft | 5 atomic tasks | Low |
| [Backlog Pipeline](worktrees/feat/backlog-pipeline-planning/docs/tasks/backlog-pipeline.md) | Planning - Phase 3 Complete | TBD | High |
| [Mobile UX Improvements](mobile-ux-improvements.md) | Partially Implemented (Stories 1-3 done; Task 3.4 iOS auto-zoom fix pending) | 3 stories, 11 tasks | Medium |
| [AskUserQuestion Rich UI](askuserquestion-ui.md) | Draft | 3 stories | Low |

## Completed Plans

| Plan | Status | Completed |
|------|--------|-----------|
| [Permissions Analysis & Auto-Approvals](permissions-analysis-auto-approvals.md) | Implemented | 2026-03 |
| [Notification De-Duplication](notification-deduplication.md) | Implemented | 2026-03 |
| [Claude Code Hook Approval](claude-code-hook-approval.md) | Implemented | 2026-02 |
| [Web UI Enhancements](web-ui-enhancements.md) | Implemented | - |
| [Session Rename/Restart](session-rename-restart.md) | Implemented | - |
| [Full Text Search History](full-text-search-history.md) | Implemented | - |
| [System Service Auto-Start](completed/system-service-autostart.md) | Implemented (Stories 1-4) | 2026-04-20 |
| [Rate Limit Detection](detect-and-address-rate-limits.md) | Implemented (core Stories 1-4; Story 5 config pending) | 2026-04 |
| ssq-mux build/install + from-source installer | Implemented (6518db9) | 2026-04 |
| Classifier: AskUserQuestion escalation + path expansion | Implemented (65b8c8e, 627c3af) | 2026-04 |
| Fork compatibility (dynamic repo owner) | Implemented (a1b0ed6) | 2026-04 |
| Mobile overflow menu | Implemented (ef342b6) | 2026-05-02 |
| [Session Defaults & Profiles](session-defaults.md) | Implemented (d5384192) | 2026-05 |
| [History Page Revamp](history-page-revamp.md) | Implemented (384db26b…cb0fe7cd) | 2026-05-31 |

## Reference Plans

| Plan | Status |
|------|--------|
| [Architecture Refactor](architecture-refactor.md) | Reference |
| [Repository Pattern SQLite Migration](repository-pattern-sqlite-migration.md) | Reference |
| [PTY Discovery Refactoring](pty-discovery-refactoring.md) | Reference |
| [PTY Interception External Claude](pty-interception-external-claude.md) | Reference |
| [Session Search and Sort](session-search-and-sort.md) | Reference |
| [History Page UX Improvements](history-page-ux-improvements.md) | Reference |
| [History Browser Performance](history-browser-performance.md) | Reference |
| [Workspace Status Visualization](workspace-status-visualization.md) | Reference |
| [SQLite Schema Normalization](sqlite-schema-normalization.md) | Reference |
| [Session Restart Functionality](session-restart-functionality.md) | Reference |
| [Fix Test Failures](fix-test-failures.md) | Reference |
| [Rate Limit Detection](detect-and-address-rate-limits.md) | Reference (core implemented; Story 5 + UX pending) |
| [Rate Limit UX Improvements](rate-limit-ux-improvements.md) | Backlog (all tasks pending) |

## Open Bugs

| Bug | Severity | Status | Notes |
|-----|----------|--------|-------|
| [review-queue-gaps](../bugs/open/review-queue-gaps.md) | Low | Open | GAP-001/003/004 remain; BUG-001/002/003 fixed same session |
| [BUG-018](../bugs/open/BUG-018-gob-session-persistence-memory-hotspot.md) | Medium | Open | Gob encoding 35MB heap allocation; consider protobuf migration |
| [BUG-019](../bugs/open/BUG-019-flate-writer-not-pooled.md) | Low | Open | flate writer not pooled; 12MB resident + 18% CPU |
| [BUG-020](../bugs/open/BUG-020-vcs-status-diff-mutex-contention.md) | Medium | Open | VCS status diff mutex contention |
| [BUG-021](../bugs/open/BUG-021-check-gh-auth-mutex-contention.md) | Medium | Open | gh auth check mutex contention |

## Fixed Bugs (recently)

BUG-010 (tmux global registry contamination), BUG-012 (testutil package failures), BUG-013 (xterm.js viewport jump), BUG-015 (EventBus goroutine leak), BUG-016 (WebSocket flate writer), BUG-017 (SQLite eager load 25MB startup) — all fixed, see `docs/bugs/fixed/`.

## Notes

- Session Defaults fully shipped (d5384192); all 11 tasks done including DirectoryRulesManager.
- History Page Revamp fully shipped across 4 commits (384db26b…cb0fe7cd). All 5 stories, 13 tasks complete.
- No critical bugs blocking active feature work.
- Mobile UX Stories 1-3 implemented (viewport, safe-area, 100dvh, ViewportProvider, toolbar overflow menu, keyboard toggle). Task 3.4 (iOS auto-zoom fix for xterm textarea) remains pending.
- Rate Limit Detection core is implemented (`session/detection/ratelimit/`). Story 5 (config/disable toggle) and UX improvements in `rate-limit-ux-improvements.md` remain pending.
- BUG-008, BUG-009, BUG-011 referenced a `ui/` Go package (TUI era) that no longer exists. Move to `docs/bugs/obsolete/` when that directory is created.
