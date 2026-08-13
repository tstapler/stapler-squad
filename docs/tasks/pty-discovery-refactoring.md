# PTY Discovery System Refactoring

## Epic Overview

**Goal**: Refactor the PTY (Pseudo Terminal) discovery system to use an interface-based architecture with platform-specific implementations, improving maintainability, testability, and error handling.

**Value Proposition**:
- Cleaner platform separation for easier maintenance
- Better error handling preventing UI noise and log spam
- Improved testability through mock interfaces
- Easier addition of new platform support
- Resolution of "(error)" text appearing in UI

**Success Metrics**:
- Zero log spam for expected PTY discovery failures
- Clean UI without error text artifacts
- 80%+ test coverage for new code
- Support for macOS, Linux, and BSD platforms
- Mock implementations enable comprehensive testing

**Current Status**:
- ✅ Immediate fixes deployed (macOS lsof-based detection works)
- ✅ Log spam reduced for nil gitWorktree errors
- ⚠️ "(error)" text still appearing in UI between title and branch lines
- 🔄 Refactoring planned but not started

## Story Breakdown

### Story 1: Core Interface Architecture (8h total)

Establish the foundational interface-based architecture for platform-agnostic PTY discovery.

**Objectives**:
- Define clean interfaces for PTY discovery
- Create service orchestrator for managing discoverers
- Establish data structures for PTY information
- Enable dependency injection for testing

### Story 2: Platform Implementations (12h total)

Implement platform-specific PTY discoverers for all supported operating systems.

**Objectives**:
- Support macOS with lsof-based discovery
- Support Linux with /proc filesystem
- Support BSD variants
- Provide tmux-based fallback for all platforms

### Story 3: Error Management System (8h total)

Create comprehensive error handling with context, categorization, and rate limiting.

**Objectives**:
- Structured errors with full debugging context
- Prevent log spam through rate limiting
- Clean UI with no error text artifacts
- Differentiate transient vs permanent failures

### Story 4: Integration and Testing (10h total)

Migrate existing code to new system and establish comprehensive testing.

**Objectives**:
- Seamless migration from old implementation
- Mock-based testing infrastructure
- Platform-specific integration tests
- Resolution of "(error)" UI bug

## Atomic Tasks

### Story 1: Core Interface Architecture

#### Task 1.1: Define Core Interfaces (2h)

**Scope**: Create the foundational interfaces and data structures for PTY discovery.

**Files**:
- session/pty/interfaces.go (create)
- session/pty/types.go (create)
- session/pty/doc.go (create)

**Context**:
- Current implementation mixes platform code in session/pty_discovery.go
- Need clean separation of concerns
- Must support dependency injection for testing

**Implementation**:
```go
// interfaces.go
type PTYDiscoverer interface {
    DiscoverPTY(pid int) (*PTYInfo, error)
    DiscoverPTYFromTmux(sessionName string) (*PTYInfo, error)
    GetProcessState(pid int) (ProcessState, error)
    Priority() int // Lower number = higher priority
    Name() string
}

// types.go
type PTYInfo struct {
    Path      string
    PID       int
    PPID      int
    Command   string
    State     ProcessState
    Platform  string
    Method    string // How it was discovered
}

type ProcessState int
const (
    StateRunning ProcessState = iota
    StateSleeping
    StateStopped
    StateZombie
    StateUnknown
)
```

**Success Criteria**:
- Interfaces compile without errors
- Types are well-documented
- Supports all required PTY operations

**Testing**: Unit tests for type methods and constants

**Dependencies**: None

**Status**: ⏳ Pending

---

#### Task 1.2: Create PTYDiscoveryService (3h)

**Scope**: Implement the service orchestrator that manages multiple discoverers.

**Files**:
- session/pty/service.go (create)
- session/pty/service_test.go (create)

**Context**:
- Service tries discoverers in priority order
- Falls back to next discoverer on failure
- Caches successful discoverers per PID

**Implementation**:
```go
type PTYDiscoveryService struct {
    discoverers []PTYDiscoverer
    cache       map[int]PTYDiscoverer // PID -> successful discoverer
    mu          sync.RWMutex
}

func (s *PTYDiscoveryService) DiscoverPTY(pid int) (*PTYInfo, error) {
    // Try cached discoverer first
    // Then try all discoverers in priority order
    // Cache successful discoverer
}
```

**Success Criteria**:
- Service correctly orchestrates multiple discoverers
- Caching improves performance for repeated queries
- Thread-safe for concurrent use

**Testing**: Unit tests with mock discoverers

**Dependencies**: Task 1.1

**Status**: ⏳ Pending

---

#### Task 1.3: Platform Detection and Registration (3h)

**Scope**: Implement automatic platform detection and discoverer registration.

**Files**:
- session/pty/platform.go (create)
- session/pty/platform_darwin.go (create)
- session/pty/platform_linux.go (create)
- session/pty/platform_bsd.go (create)

**Context**:
- Use build tags for platform-specific code
- Register appropriate discoverers at runtime
- Provide factory function for service creation

**Implementation**:
```go
// platform.go
func NewPTYDiscoveryService() *PTYDiscoveryService {
    service := &PTYDiscoveryService{}
    registerPlatformDiscoverers(service)
    return service
}

// platform_darwin.go
// +build darwin
func registerPlatformDiscoverers(s *PTYDiscoveryService) {
    s.Register(NewDarwinPTYDiscoverer())
    s.Register(NewTmuxPTYDiscoverer())
}
```

**Success Criteria**:
- Correct discoverers registered per platform
- Build tags prevent compilation of wrong platform code
- Factory function provides ready-to-use service

**Testing**: Platform-specific integration tests

**Dependencies**: Task 1.2

**Status**: ⏳ Pending

---

### Story 2: Platform Implementations

#### Task 2.1: Darwin (macOS) PTY Discoverer (3h)

**Scope**: Implement macOS-specific PTY discovery using lsof.

**Files**:
- session/pty/darwin.go (create)
- session/pty/darwin_test.go (create)

**Context**:
- macOS requires lsof command for PTY discovery
- Current implementation in session/pty_discovery.go:368-409
- Must handle lsof command failures gracefully

**Implementation**:
```go
type DarwinPTYDiscoverer struct {
    lsofPath string
}

func (d *DarwinPTYDiscoverer) DiscoverPTY(pid int) (*PTYInfo, error) {
    // Use lsof -p PID to find PTY
    // Parse output for /dev/ttys* devices
    // Extract process information
}
```

**Success Criteria**:
- Works on macOS 10.15+
- Handles missing lsof gracefully
- Correctly parses PTY information

**Testing**: Unit tests with mocked command output

**Dependencies**: Task 1.1

**Status**: ⏳ Pending

---

#### Task 2.2: Linux PTY Discoverer (3h)

**Scope**: Implement Linux-specific PTY discovery using /proc filesystem.

**Files**:
- session/pty/linux.go (create)
- session/pty/linux_test.go (create)

**Context**:
- Linux provides /proc/[pid]/fd for file descriptors
- Can read /proc/[pid]/stat for process state
- Fallback to lsof if /proc unavailable

**Implementation**:
```go
type LinuxPTYDiscoverer struct {
    procPath string // Default: /proc
}

func (d *LinuxPTYDiscoverer) DiscoverPTY(pid int) (*PTYInfo, error) {
    // Check /proc/[pid]/fd/0 for PTY
    // Read /proc/[pid]/stat for state
    // Parse /proc/[pid]/cmdline for command
}
```

**Success Criteria**:
- Works on Linux 3.x+
- Efficiently uses /proc filesystem
- Falls back to lsof when needed

**Testing**: Unit tests with mock filesystem

**Dependencies**: Task 1.1

**Status**: ⏳ Pending

---

#### Task 2.3: BSD PTY Discoverer (3h)

**Scope**: Implement BSD-specific PTY discovery.

**Files**:
- session/pty/bsd.go (create)
- session/pty/bsd_test.go (create)

**Context**:
- BSD variants have different /proc implementations
- May need fstat or similar commands
- Should support FreeBSD, OpenBSD, NetBSD

**Implementation**:
```go
type BSDPTYDiscoverer struct {
    variant string // freebsd, openbsd, netbsd
}

func (d *BSDPTYDiscoverer) DiscoverPTY(pid int) (*PTYInfo, error) {
    // Use fstat -p PID on FreeBSD
    // Use appropriate command per variant
    // Parse output for PTY devices
}
```

**Success Criteria**:
- Works on major BSD variants
- Handles command differences between variants
- Graceful degradation when commands unavailable

**Testing**: Unit tests with variant-specific mocks

**Dependencies**: Task 1.1

**Status**: ⏳ Pending

---

#### Task 2.4: Tmux PTY Discoverer (3h)

**Scope**: Implement cross-platform tmux-based PTY discovery.

**Files**:
- session/pty/tmux.go (create)
- session/pty/tmux_test.go (create)

**Context**:
- Tmux provides PTY information via list-panes
- Works on all platforms where tmux is available
- Should be lowest priority discoverer

**Implementation**:
```go
type TmuxPTYDiscoverer struct {
    tmuxPath string
}

func (d *TmuxPTYDiscoverer) DiscoverPTYFromTmux(sessionName string) (*PTYInfo, error) {
    // Use tmux list-panes -F "#{pane_tty}"
    // Parse tmux output for PTY path
    // Get PID from tmux pane_pid
}
```

**Success Criteria**:
- Works with tmux 2.0+
- Handles missing tmux gracefully
- Correctly parses tmux format strings

**Testing**: Unit tests with mock tmux output

**Dependencies**: Task 1.1

**Status**: ⏳ Pending

---

### Story 3: Error Management System

#### Task 3.1: Structured Error Types (2h)

**Scope**: Create comprehensive error types with context.

**Files**:
- session/pty/errors.go (create)
- session/pty/errors_test.go (create)

**Context**:
- Errors need full context for debugging
- Must differentiate error categories
- Should support error wrapping

**Implementation**:
```go
type PTYError struct {
    Op       string // Operation that failed
    Path     string // PTY path if known
    PID      int    // Process ID
    Platform string // Platform where error occurred
    Err      error  // Underlying error
    Category ErrorCategory
}

type ErrorCategory int
const (
    ErrPTYNotFound ErrorCategory = iota
    ErrPTYNotSupported
    ErrCommandFailed
    ErrPermissionDenied
    ErrTransient
)

func (e *PTYError) IsTransient() bool
func (e *PTYError) ShouldLog() bool
```

**Success Criteria**:
- Errors provide full debugging context
- Category system enables smart handling
- Compatible with errors.Is/As

**Testing**: Unit tests for all error methods

**Dependencies**: None

**Status**: ⏳ Pending

---

#### Task 3.2: Rate Limiting for Error Logs (3h)

**Scope**: Implement rate limiting to prevent log spam.

**Files**:
- session/pty/ratelimit.go (create)
- session/pty/ratelimit_test.go (create)

**Context**:
- Same errors repeat during polling
- Need to suppress duplicate logs
- Should allow periodic warnings

**Implementation**:
```go
type RateLimiter struct {
    seen     map[string]time.Time
    duration time.Duration // Suppress duration
    mu       sync.Mutex
}

func (r *RateLimiter) ShouldLog(key string) bool {
    // Check if we've seen this error recently
    // Update last seen time
    // Return true if should log
}
```

**Success Criteria**:
- Prevents log spam for repeated errors
- Allows periodic warnings (e.g., every 30s)
- Thread-safe for concurrent use

**Testing**: Unit tests with time manipulation

**Dependencies**: None

**Status**: ⏳ Pending

---

#### Task 3.3: Error Handler Integration (3h)

**Scope**: Integrate error handling throughout PTY discovery.

**Files**:
- session/pty/handler.go (create)
- session/pty/handler_test.go (create)

**Context**:
- Centralized error handling logic
- Determines logging and UI behavior
- Integrates with rate limiting

**Implementation**:
```go
type ErrorHandler struct {
    limiter *RateLimiter
    logger  Logger
}

func (h *ErrorHandler) Handle(err error) error {
    // Categorize error
    // Apply rate limiting
    // Log if appropriate
    // Return sanitized error for UI
}
```

**Success Criteria**:
- Consistent error handling across system
- No error text leaks to UI
- Appropriate logging levels used

**Testing**: Unit tests with mock logger

**Dependencies**: Tasks 3.1, 3.2

**Status**: ⏳ Pending

---

### Story 4: Integration and Testing

#### Task 4.1: Migrate Existing Implementation (3h)

**Scope**: Replace old PTY discovery with new system.

**Files**:
- session/pty_discovery.go (modify)
- session/instance.go (modify)
- session/instance_test.go (modify)

**Context**:
- Current code in session/pty_discovery.go
- Multiple callers throughout codebase
- Must maintain backward compatibility

**Implementation**:
```go
// pty_discovery.go - simplified wrapper
var defaultService = pty.NewPTYDiscoveryService()

func DiscoverPTY(pid int) (string, error) {
    info, err := defaultService.DiscoverPTY(pid)
    if err != nil {
        return "", err
    }
    return info.Path, nil
}
```

**Success Criteria**:
- All existing functionality preserved
- No breaking changes to public API
- Old platform-specific code removed

**Testing**: Existing tests continue to pass

**Dependencies**: Stories 1, 2, 3

**Status**: ⏳ Pending

---

#### Task 4.2: Fix "(error)" UI Bug (2h)

**Scope**: Identify and fix the mysterious "(error)" text in UI.

**Files**:
- ui/list.go (modify)
- session/instance.go (modify)
- ui/overlay/sessionSetup.go (investigate)

**Context**:
- Error appears between title and branch lines
- Not coming from obvious error returns
- May be lipgloss or terminal capability issue

**Implementation**:
```go
// Add debug logging to trace error source
// Check all string concatenations
// Verify error handling in rendering pipeline
// Sanitize any error returns to UI
```

**Success Criteria**:
- "(error)" text no longer appears
- Root cause documented
- Test prevents regression

**Testing**: Visual testing and regression test

**Dependencies**: None (can start immediately)

**Status**: ⏳ Pending

---

#### Task 4.3: Mock Infrastructure (2h)

**Scope**: Create comprehensive mock implementations for testing.

**Files**:
- session/pty/mock/mock.go (create)
- session/pty/mock/discoverer.go (create)
- session/pty/mock/filesystem.go (create)

**Context**:
- Tests need to work without real PTYs
- Should simulate various failure modes
- Enable deterministic testing

**Implementation**:
```go
type MockDiscoverer struct {
    DiscoverFunc func(pid int) (*PTYInfo, error)
    CallCount    int
}

func (m *MockDiscoverer) DiscoverPTY(pid int) (*PTYInfo, error) {
    m.CallCount++
    return m.DiscoverFunc(pid)
}
```

**Success Criteria**:
- Mocks cover all interface methods
- Can simulate success and failure
- Enable comprehensive unit testing

**Testing**: Tests using the mocks

**Dependencies**: Task 1.1

**Status**: ⏳ Pending

---

#### Task 4.4: Integration Tests (3h)

**Scope**: Create platform-specific integration tests.

**Files**:
- session/pty/integration_test.go (create)
- session/pty/integration_darwin_test.go (create)
- session/pty/integration_linux_test.go (create)

**Context**:
- Need to verify actual PTY discovery works
- Should run on CI for each platform
- Test fallback mechanisms

**Implementation**:
```go
// +build integration

func TestPTYDiscoveryOnPlatform(t *testing.T) {
    // Start a process with known PTY
    // Verify discovery finds it
    // Test error cases
    // Verify fallback works
}
```

**Success Criteria**:
- Tests pass on all platforms
- Cover success and failure paths
- Verify fallback mechanisms work

**Testing**: Run on CI for each platform

**Dependencies**: All other tasks

**Status**: ⏳ Pending

---

## Dependency Visualization

```
Story 1: Core Interface Architecture
├─ Task 1.1: Define Core Interfaces (2h) ──────────┐
├─ Task 1.2: Create PTYDiscoveryService (3h) ──────┤
└─ Task 1.3: Platform Detection (3h) ──────────────┤
                                                    │
Story 2: Platform Implementations                  │
├─ Task 2.1: Darwin Discoverer (3h) ───────────────┤
├─ Task 2.2: Linux Discoverer (3h) ────────────────┤
├─ Task 2.3: BSD Discoverer (3h) ──────────────────┤
└─ Task 2.4: Tmux Discoverer (3h) ─────────────────┤
                                                    │
Story 3: Error Management                          │
├─ Task 3.1: Error Types (2h) ─────────────────────┤
├─ Task 3.2: Rate Limiting (3h) ───────────────────┤
└─ Task 3.3: Error Handler (3h) ───────────────────┤
                                                    ▼
Story 4: Integration ─────────────── Integration Checkpoint
├─ Task 4.1: Migrate Implementation (3h)
├─ Task 4.2: Fix UI Bug (2h) [INDEPENDENT - can start now]
├─ Task 4.3: Mock Infrastructure (2h)
└─ Task 4.4: Integration Tests (3h)
```

## Context Preparation

### Required Understanding
- Go interface patterns and dependency injection
- Platform-specific build tags
- tmux command-line interface
- /proc filesystem on Linux
- lsof command output format
- Error wrapping in Go 1.13+

### Key Files to Review
- `session/pty_discovery.go` - Current implementation
- `ui/list.go` - UI rendering that shows errors
- `session/instance.go` - Main consumer of PTY discovery
- `session/tmux/tmux.go` - Tmux integration

### Testing Approach
- Unit tests with mocks for platform-specific code
- Integration tests on actual platforms
- Manual testing for UI bug fix
- Benchmark tests for performance validation

## Progress Tracking

**Overall Status**: 0% Complete (0/14 tasks)

### Story Completion
- Story 1 (Core Interface): 0% (0/3 tasks)
- Story 2 (Platform Implementations): 0% (0/4 tasks)
- Story 3 (Error Management): 0% (0/3 tasks)
- Story 4 (Integration): 0% (0/4 tasks)

### Time Tracking
- Estimated: 38 hours
- Completed: 0 hours
- Remaining: 38 hours

### Next Actions
1. **Immediate**: Start Task 4.2 (Fix UI Bug) - independent, high visibility
2. **Next Sprint**: Begin Story 1 (Core Interface) - foundation for all other work
3. **Parallel Work**: Multiple developers can work on platform implementations

## Risk Mitigation

### Technical Risks
- **Platform Compatibility**: Test on CI for all target platforms
- **Performance Impact**: Benchmark before/after migration
- **Breaking Changes**: Maintain wrapper functions for compatibility

### Mitigation Strategies
- Incremental migration with fallback to old code
- Feature flag to enable/disable new implementation
- Comprehensive testing before full rollout
- Keep old implementation until new one proven stable

## Success Metrics

### Quantitative
- Zero log spam for expected failures
- <10ms average PTY discovery time
- 80%+ test coverage for new code
- Support for 3+ platforms

### Qualitative
- Clean separation of platform code
- Easy to add new platform support
- Improved developer experience
- Better debugging with structured errors

## Notes

- Current immediate fixes are working but not architecturally clean
- The "(error)" UI bug is priority as it affects user experience
- Can be implemented incrementally without breaking existing code
- Consider feature flag for gradual rollout
- Platform-specific code should use build tags consistently