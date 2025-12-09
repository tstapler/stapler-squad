# Omnibar Session Creation - Quick Reference

**Feature Plan**: [omnibar-session-creation.md](./omnibar-session-creation.md)  
**Status**: Planning Complete  
**Estimated Effort**: 2-3 weeks (1 developer), 8 days (3 developers parallel)

---

## What is the Omnibar?

The Omnibar is an intelligent single-input session creation interface that auto-detects input type and creates sessions with minimal user interaction. It replaces the current multi-step wizard with a modern, streamlined workflow.

### Supported Input Types

| Input Example | Detected As | Action |
|--------------|------------|--------|
| `/Users/tyler/projects/repo` | Local Path | Create session in directory |
| `~/projects/repo@feature` | Path + Branch | Create session on branch |
| `https://github.com/owner/repo` | GitHub Repo | Clone and create session |
| `https://github.com/owner/repo/tree/branch` | GitHub Branch | Clone branch and create session |
| `https://github.com/owner/repo/pull/123` | GitHub PR | Clone PR branch with context |
| `owner/repo` | GitHub Shorthand | Clone repo |
| `owner/repo:branch` | GitHub Shorthand + Branch | Clone branch |

---

## Key Features

### 1. Smart Input Detection
- **Real-time detection**: Type indicator updates as you type
- **Priority-based matching**: GitHub PRs > Branches > Repos > Local paths
- **Validation feedback**: Instant error messages with helpful suggestions

### 2. Contextual Field Expansion
- **Dynamic fields**: Additional fields appear only when needed
- **GitHub PR metadata**: Displays PR title, status, files changed
- **Auto-suggestions**: Session name auto-generated from input

### 3. Performance Optimizations
- **Debounced validation**: 300ms delay prevents excessive API calls
- **GitHub API caching**: 5-minute LRU cache for PR metadata
- **Async operations**: Non-blocking UI during validation

### 4. Keyboard-First Workflow
- `Tab` / `Shift+Tab` - Navigate fields
- `Enter` - Submit (when valid)
- `Esc` - Cancel
- `Ctrl+L` - Focus omnibar

---

## Architecture Overview

### Component Structure
```
OmnibarOverlay
├── Input Detection Engine
│   ├── LocalPathDetector (priority: 100)
│   ├── PathWithBranchDetector (priority: 50)
│   ├── GitHubPRDetector (priority: 10)
│   ├── GitHubBranchDetector (priority: 20)
│   ├── GitHubRepoDetector (priority: 30)
│   └── GitHubShorthandDetectors (priority: 40)
├── Validation Engine
│   ├── Path validation (reuses existing ValidatePathEnhanced)
│   ├── GitHub API validation (with caching)
│   └── Git command validation (branch existence)
└── UI Components
    ├── Type Indicator (shows detected type)
    ├── Validation Feedback (errors/warnings/success)
    ├── Contextual Fields (branch selector, PR info)
    └── Action Buttons (Create, Cancel)
```

### Key Design Patterns
- **Detector Priority Pattern**: Ordered list of detectors, first match wins
- **Debounced Validation**: 300ms delay for expensive operations
- **LRU Caching**: 5-minute cache for GitHub API responses
- **Callback Pattern**: OnComplete/OnCancel for parent integration

---

## Implementation Plan

### Epic Breakdown (2-3 weeks)

#### Week 1: Detection Engine & Base Overlay (5 days)
- **Story 1**: Input Detection Engine (3 days)
  - Task 1.1: Detector interface & registry (4h)
  - Task 1.2: Local path detector (3h)
  - Task 1.3: Path+branch detector (3h)
  - Task 1.4: GitHub URL detectors (5h)
  - Task 1.5: Detection orchestration (3h)
  
- **Story 2**: Omnibar Overlay Component (2 days)
  - Task 2.1: Base overlay structure (4h)
  - Task 2.2: Type indicator component (3h)
  - Task 2.3: Validation feedback component (3h)
  - Task 2.4: Keyboard navigation (4h)
  - Task 2.5: Integration with app handler (2h)

#### Week 2: Contextual Fields & Auto-Suggestions (5 days)
- **Story 3**: Contextual Field Expansion (3 days)
  - Task 3.1: Conditional field manager (4h)
  - Task 3.2: GitHub branch selector (4h)
  - Task 3.3: PR metadata display (5h)
  - Task 3.4: PR prompt generation toggle (3h)
  - Task 3.5: Smooth animations (3h)

- **Story 4**: Auto-Suggestions & Defaults (2 days)
  - Task 4.1: Session name auto-suggestion (4h)
  - Task 4.2: Program & category defaults (2h)
  - Task 4.3: Branch default selection (3h)
  - Task 4.4: Override mechanism (3h)

#### Week 3: Testing & Polish (5 days)
- **Story 5**: Testing & Polish
  - Task 5.1: Unit tests for detectors (6h)
  - Task 5.2: Integration tests (8h)
  - Task 5.3: Error handling & edge cases (6h)
  - Task 5.4: Performance optimization (4h)
  - Task 5.5: Documentation & examples (4h)
  - Task 5.6: User acceptance testing (4h)

### Critical Path
```
1.1 → 1.2/1.3/1.4 → 1.5 → 2.1 → 2.5 → 3.1 → 3.2/3.3 → 5.2 → 5.6
```
**Duration**: 11-12 days (sequential), 8 days (with parallel work)

---

## Known Issues & Mitigations

### High Priority

1. **GitHub API Rate Limiting**
   - **Risk**: 60 requests/hour limit for unauthenticated users
   - **Mitigation**: 5-minute LRU cache, debounced validation, optional token auth

2. **Git Clone Failures**
   - **Risk**: Network errors, auth required, repo deleted
   - **Mitigation**: Retry logic (3 attempts), clear error messages, fallback to error state

3. **Command Injection via Branch Names**
   - **Risk**: Malicious branch names like `; rm -rf /`
   - **Mitigation**: Strict validation, exec.CommandContext with args, no shell interpolation

### Medium Priority

4. **Tmux Session Name Conflicts**
   - **Risk**: Auto-suggested names conflict with existing sessions
   - **Mitigation**: Check existing names, auto-append number suffix

5. **Slow Network Path Validation**
   - **Risk**: SMB/NFS paths may timeout (>5s)
   - **Mitigation**: 500ms context timeout, network path detection, warning messages

6. **Path Traversal Attacks**
   - **Risk**: Input like `../../sensitive` accesses unauthorized directories
   - **Mitigation**: filepath.Clean(), show full expanded path for confirmation

---

## Testing Strategy

### Unit Tests (>90% coverage target)
- Detector registration & priority ordering
- Input detection for all types
- Validation logic (path, GitHub, branch names)
- Error handling (network, API, filesystem)

### Integration Tests
- End-to-end session creation workflows
- Error recovery (API failures, network timeouts)
- Keyboard navigation
- Async validation with debouncing

### Performance Tests
- Detection latency (<50ms)
- Validation latency (<100ms local, <500ms GitHub)
- UI responsiveness (<16ms per keystroke)
- Stress test: 150 WPM typing speed

### User Acceptance Testing
- First-time user: understands omnibar in <30s
- Power user: creates 10 sessions in <2 minutes
- Error scenarios: helpful error messages

---

## Files to Create

### New Files (15 total)
```
ui/overlay/omnibar/
├── detector.go                      # Detector interface
├── registry.go                      # Detector registry
├── types.go                         # Core types (InputType, DetectionResult, etc.)
├── detector_local_path.go           # Local path detector
├── detector_local_path_test.go
├── detector_path_with_branch.go     # Path@branch detector
├── detector_path_with_branch_test.go
├── detector_github_pr.go            # GitHub PR detector
├── detector_github_pr_test.go
├── detector_github_branch.go        # GitHub branch detector
├── detector_github_branch_test.go
├── detector_github_repo.go          # GitHub repo detector
├── detector_github_repo_test.go
├── detector_github_shorthand.go     # Shorthand detectors
├── detector_github_shorthand_test.go
├── omnibar_overlay.go               # Main overlay component
├── omnibar_overlay_test.go
├── type_indicator.go                # Type indicator UI
├── validation_feedback.go           # Validation feedback UI
├── conditional_fields.go            # Contextual field manager
└── auto_suggestions.go              # Auto-suggestion logic
```

### Modified Files (3 total)
```
app/handleAdvancedSessionSetup.go   # Update to use OmnibarOverlay
ui/overlay/sessionSetup.go          # Optional: keep as fallback or remove
app/services/ui_coordination.go     # Register omnibar overlay
```

---

## Success Metrics

**Quantitative KPIs**:
- Session creation time: < 10 seconds (vs 30-60s current) [80-90% improvement]
- GitHub session adoption: 40%+ of sessions from GitHub URLs
- User error rate: < 5% invalid inputs
- Detection accuracy: 99%+ correct type detection
- Performance: < 100ms detection latency

**Qualitative Indicators**:
- User feedback: "much faster session creation"
- Increased PR-based workflow adoption
- Reduced support questions about GitHub integration

---

## Quick Start for Developers

### 1. Read the Full Plan
Start with [`omnibar-session-creation.md`](./omnibar-session-creation.md) for comprehensive details.

### 2. Review Existing Code
- `ui/overlay/sessionSetup.go` - Current session creation wizard
- `ui/overlay/pathutils.go` - Path validation utilities
- `github/url_parser.go` - GitHub URL parsing
- `ui/overlay/fuzzyInput.go` - Async input patterns

### 3. Start with Story 1
Begin with the input detection engine (Tasks 1.1-1.5). This is the foundation for all other features.

### 4. Follow Atomic Tasks
Each task is designed to be:
- **Bounded**: 2-5 files, 1-4 hours
- **Testable**: Clear acceptance criteria
- **Independent**: Minimal concurrent dependencies

### 5. Use Context Guides
Each task has a "Context Preparation Guide" with:
- Files to read before starting
- Key questions to answer
- Patterns to follow

---

## Architecture Decision Records (ADRs)

### ADR-001: Detector Priority Pattern
**Decision**: Use priority-ordered detectors, first match wins  
**Rationale**: Simpler, faster, more predictable than exhaustive matching

### ADR-002: Debounced Validation
**Decision**: 300ms debounce for expensive operations  
**Rationale**: Balance responsiveness with performance, avoid API rate limits

### ADR-003: Single Overlay vs Multi-Step Wizard
**Decision**: Single overlay with contextual fields  
**Rationale**: Faster workflow, lower cognitive load, modern UX pattern

### ADR-004: GitHub API Caching
**Decision**: 5-minute LRU cache for PR metadata  
**Rationale**: Instant repeat lookups, rate limit protection, acceptable staleness

### ADR-005: Path+Branch @-notation
**Decision**: Use `@` separator (e.g., `/path@branch`)  
**Rationale**: Unambiguous, familiar to git users, cross-platform compatible

---

## Next Steps

1. **Review this plan** with team and stakeholders
2. **Set up development branch**: `git checkout -b feature/omnibar-session-creation`
3. **Start with Task 1.1**: Detector interface & registry (foundational)
4. **Implement in sequence**: Follow the story breakdown
5. **Test incrementally**: Write tests alongside each task
6. **Deploy for UAT**: Week 3 - user acceptance testing

---

**Document Version**: 1.0  
**Last Updated**: 2025-12-09  
**Author**: Claude (Architecture Planning Specialist)
