# Task-001: Mock External Dependencies for Test Infrastructure

## Task ID
AIC-001-MOCK-EXTERNAL-DEPS

## Priority
**CRITICAL** - Blocking development workflow

## Context
Tests are failing due to real external dependencies (`claude --version`, `tmux` commands) causing timeouts and unreliable test execution. This prevents reliable CI/CD and development workflows.

## Problem Statement
- `TestDefaultConfig` takes 2.82s because `GetClaudeCommand()` executes real shell commands
- `session/tmux` tests timeout after 10s waiting for real tmux sessions
- Tests fail in environments without claude CLI or tmux installed
- CI/CD pipeline is unreliable due to external dependency timeouts

## Atomic Work Unit (AIC Framework)

### INVEST Criteria
- **Independent**: Can be completed without other task dependencies
- **Valuable**: Immediately unblocks development and CI/CD workflows
- **Estimable**: Clear scope - mock 2 external commands in 3-5 files
- **Small**: 1-4 hour focused task with single responsibility
- **Testable**: Verify by running test suites that currently fail

### Context Boundary (5 files max)
1. `config/config.go` - Add dependency injection for command execution
2. `config/config_test.go` - Implement mocks for GetClaudeCommand
3. `session/tmux/tmux.go` - Use existing executor interface consistently
4. `testutil/mocks.go` - Create shared mock utilities (new file)
5. `session/tmux/tmux_test.go` - Enhance existing mock infrastructure

## Implementation Plan

### Step 1: Create Shared Mock Utilities (20 min)
**File**: `testutil/mocks.go` (new)
- Create `MockCommandExecutor` interface
- Implement configurable mock responses for shell commands
- Add helper functions for common test scenarios

### Step 2: Refactor Config External Dependencies (45 min)
**File**: `config/config.go`
- Add `CommandExecutor` interface field to Config struct
- Modify `GetClaudeCommand()` to use injected executor
- Maintain backward compatibility with default real executor

### Step 3: Update Config Tests with Mocks (30 min)
**File**: `config/config_test.go`
- Replace real command execution with mocks in tests
- Add test cases for mock command responses
- Verify `TestDefaultConfig` runs under 100ms

### Step 4: Enhance tmux Mock Infrastructure (45 min)
**File**: `session/tmux/tmux.go` & `session/tmux/tmux_test.go`
- Ensure consistent use of existing `executor.Executor` interface
- Add timeout configuration to prevent real tmux calls in tests
- Verify existing `MockCmdExec` handles all tmux command scenarios

### Step 5: Integration Validation (20 min)
- Run `go test ./config` - should complete under 1s
- Run `go test ./session/tmux` - should complete under 5s
- Verify no real external commands executed during tests

## Success Criteria

### Functional Requirements
- [ ] `go test ./config` completes in <1s (currently 2.82s for one test)
- [ ] `go test ./session/tmux` completes in <10s (currently times out)
- [ ] No real `claude --version` or `tmux` commands executed during tests
- [ ] All existing test functionality preserved

### Technical Requirements
- [ ] Dependency injection pattern implemented for external commands
- [ ] Shared mock utilities in `testutil/` package
- [ ] Backward compatibility maintained for production code
- [ ] Mock configurations are easily testable and maintainable

### Quality Requirements
- [ ] Code follows existing patterns in project
- [ ] Mock utilities are reusable across test files
- [ ] Clear error messages when mocks are misconfigured
- [ ] Documentation updated in relevant files

## Risk Mitigation

### Risk: Breaking existing production functionality
**Mitigation**: Use dependency injection with default real executors, maintain full backward compatibility

### Risk: Incomplete mock coverage
**Mitigation**: Start with failing tests, add mock scenarios incrementally based on actual test requirements

### Risk: Overly complex mock infrastructure
**Mitigation**: Keep mocks simple, focus on the specific commands that are causing test failures

## Files to Modify

1. **testutil/mocks.go** (new) - Shared mock utilities
2. **config/config.go** - Add dependency injection interface
3. **config/config_test.go** - Implement mocks for failing tests
4. **session/tmux/tmux.go** - Ensure consistent executor usage
5. **session/tmux/tmux_test.go** - Enhance existing mock infrastructure

## Success Metrics
- Test execution time: `config` tests <1s (from 2.82s)
- Test reliability: `tmux` tests complete without timeout
- CI/CD stability: Tests pass consistently in automation environments
- Developer experience: Local test runs are fast and reliable

## Definition of Done
- [ ] All config tests run in <1s with mocked dependencies
- [ ] All tmux tests run in <10s without real tmux sessions
- [ ] Shared mock utilities created and documented
- [ ] Production code maintains full backward compatibility
- [ ] CI/CD pipeline can run tests reliably

---

**Estimated Effort**: 2.5 hours
**Context Boundary**: 5 files max
**Single Responsibility**: Mock external command dependencies for test reliability