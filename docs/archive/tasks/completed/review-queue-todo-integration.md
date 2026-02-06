# Review Queue Feature - TODO.md Integration

## Summary for TODO.md

Add the following section to your TODO.md file to track the review queue feature implementation:

```markdown
## Review Queue Feature

A system to help users identify and prioritize sessions requiring their attention (pending approval dialogs, idle sessions, error states).

**Documentation**: See `docs/tasks/review-queue-feature.md` for complete AIC breakdown

### Quick Start Tasks

#### Phase 1 - Foundation (Ready to Start)
- [ ] Extend DetectedStatus enum with attention states (2h)
- [ ] Create attention pattern YAML definitions (3h)
- [ ] Implement idle detection logic (3h)
- [ ] Build priority scoring algorithm (4h)

#### Phase 2 - Core Logic (Blocked by Phase 1)
- [ ] Define ReviewQueueEntry data model (2h)
- [ ] Implement thread-safe QueueManager (4h)
- [ ] Integrate with session monitoring (3h)

#### Phase 3 - User Interface (Blocked by Phase 2)
- [ ] Create QueueView component (3h)
- [ ] Add 'r' key binding for queue toggle (2h)
- [ ] Implement queue entry rendering (3h)
- [ ] Add queue count status indicator (2h)

#### Phase 4 - Advanced Features (Blocked by Phase 3)
- [ ] Add queue filtering options (3h)
- [ ] Implement quick actions menu (3h)
- [ ] Enable batch operations (2h)

#### Phase 5 - Optimization (Blocked by Phase 4)
- [ ] Implement queue caching strategy (3h)
- [ ] Add background status monitoring (4h)
- [ ] Complete integration testing (3h)

### Key Integration Points

**Existing Systems to Modify**:
- `session/status_detector.go` - Extend status detection
- `session/instance.go` - Add priority scoring
- `ui/list.go` - Add queue indicator
- `app/app.go` - Integrate queue view mode

**New Components to Create**:
- `session/review_queue.go` - Core queue logic
- `ui/queue_view.go` - Queue UI component
- `session/idle_detector.go` - Idle detection
- `session/priority_scorer.go` - Priority algorithm

### Success Criteria
- [ ] Queue updates within 2 seconds of status change
- [ ] < 100ms UI rendering with 100+ sessions
- [ ] Zero false positives in attention detection
- [ ] 80%+ test coverage
- [ ] Complete user documentation
```

## Development Workflow

### Getting Started

1. **Review the complete documentation**:
   ```bash
   cat docs/tasks/review-queue-feature.md
   ```

2. **Start with Phase 1 tasks** - these have no dependencies:
   - Extend status enum (Task 1.1)
   - Create pattern definitions (Task 1.2)

3. **Set up development branch**:
   ```bash
   git checkout -b feature/review-queue
   ```

4. **Run existing tests** to ensure baseline:
   ```bash
   go test ./session/...
   go test ./ui/...
   ```

### Testing Strategy

**Unit Tests** (write alongside each task):
```bash
# After each component
go test ./session/... -run TestReviewQueue
go test ./ui/... -run TestQueueView
```

**Integration Tests** (Phase 5):
```bash
# With real tmux sessions
go test ./integration/... -run TestQueueIntegration
```

**Performance Benchmarks**:
```bash
# Queue operations
go test -bench=BenchmarkQueue ./session/...

# UI rendering
go test -bench=BenchmarkQueueView ./ui/...
```

### Code Review Checklist

Before submitting PRs, ensure:

- [ ] All tests pass
- [ ] Performance benchmarks meet targets
- [ ] Thread-safety verified for concurrent operations
- [ ] Documentation updated
- [ ] Backward compatibility maintained
- [ ] Configuration options exposed
- [ ] Error handling comprehensive

### Implementation Tips

1. **Start Small**: Begin with status detection enhancements before UI
2. **Mock Early**: Create mock data for UI development
3. **Test Patterns**: Validate regex patterns with real Claude outputs
4. **Performance First**: Profile early, optimize throughout
5. **User Feedback**: Get early feedback on UI/UX decisions

## Quick Reference

### File Structure
```
session/
  ├── status_detector.go      # Extend with new statuses
  ├── review_queue.go         # NEW - Queue entry model
  ├── queue_manager.go        # NEW - Queue operations
  ├── idle_detector.go        # NEW - Idle detection
  ├── priority_scorer.go      # NEW - Priority algorithm
  └── background_monitor.go   # NEW - Background monitoring

ui/
  ├── list.go                 # Modify - Add queue indicator
  ├── queue_view.go          # NEW - Queue view component
  ├── queue_renderer.go      # NEW - Entry rendering
  ├── queue_filter.go        # NEW - Filtering logic
  └── queue_actions.go       # NEW - Quick actions

app/
  └── app.go                 # Modify - Integrate queue mode
```

### Key Commands

```bash
# Build and test
make build
make test

# Run with queue feature
./claude-squad

# Toggle queue view (in app)
Press 'r' key

# Run benchmarks
make benchmark
```

### Dependencies

The feature integrates with:
- BubbleTea TUI framework
- tmux session management
- Git worktree system
- Existing status detection

No new external dependencies required.

## Next Steps

1. **Create feature branch**
2. **Implement Task 1.1** (Extend Status Enum)
3. **Write tests for new statuses**
4. **Submit first PR for review**
5. **Continue with Phase 1 tasks**

The complete implementation should take approximately 4-5 weeks with one developer, or 2-3 weeks with parallel development of independent tasks.