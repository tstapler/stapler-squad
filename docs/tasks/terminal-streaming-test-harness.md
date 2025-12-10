# Terminal Streaming Test Harness

**Epic Overview**: Create a comprehensive test harness using Playwright and xterm.js to stress test terminal streaming under high-volume data conditions (ASCII video rendering, rapid log output, large text dumps).

## Epic Overview

### User Value
Enable reliable detection of terminal streaming performance issues before they reach production:
- Validate terminal can handle rapid updates without memory leaks
- Detect frame drops or rendering glitches during high-volume streaming
- Ensure flow control mechanisms work correctly under load
- Provide reproducible benchmarks for terminal performance regression testing

### Success Metrics
| Metric | Target | Measurement Method |
|--------|--------|-------------------|
| Frame Rate | ≥50 FPS during 30+ FPS ASCII video | Playwright performance timeline |
| Memory Growth | <50MB over 60 second test | Chrome DevTools memory profiling |
| Dropped Frames | <1% during normal operation | Custom frame counter in xterm |
| Latency P99 | <100ms from data send to render | Timestamp correlation |
| Test Coverage | 90%+ of streaming code paths | Integration test scenarios |

### Scope

**In Scope:**
- ASCII video playback stress test (multiple frame rates)
- High-volume log output simulation (10K+ lines/sec)
- Large text dump tests (1MB+ single writes)
- Color-heavy output (rainbow ANSI sequences)
- Flow control validation (backpressure testing)
- Memory leak detection over extended runs
- Performance metrics collection and reporting
- CI/CD integration for regression detection

**Out of Scope:**
- Production monitoring/observability (separate concern)
- Performance optimization implementation (test reveals issues, separate task to fix)
- Mobile browser testing (desktop focus first)
- Accessibility testing (separate concern)

### Constraints
- Must use existing Playwright infrastructure in `tests/e2e/`
- Must integrate with existing xterm.js terminal components
- Tests must complete within CI time limits (5 min max per test suite)
- Must work with production server at localhost:8543

---

## Architecture Decisions

### ADR-001: Test Data Generation Strategy

**Context:** Need to generate realistic high-volume terminal data for stress testing without requiring external dependencies.

**Decision:** Use embedded test data generators that produce various terminal output patterns:
- ASCII video frames from embedded frame data (no external files)
- Programmatic log line generation with realistic patterns
- ANSI escape sequence generators for color stress testing

**Rationale:**
- Self-contained tests don't depend on external assets
- Predictable, reproducible test data
- Easy to version control and maintain

**Consequences:**
- Need to implement efficient frame generators
- Test data patterns may not perfectly match real-world output
- Can add recorded real-world data later as extension

**Patterns Applied:** Test Data Builder, Factory Pattern

### ADR-002: Performance Measurement Architecture

**Context:** Need to accurately measure terminal rendering performance in browser context.

**Decision:** Use three-layer measurement approach:
1. **Browser Performance API**: High-resolution timestamps, RAF timing
2. **xterm.js Hooks**: Custom metrics via terminal options/addons
3. **WebSocket Metrics**: Round-trip timing from Go server

**Rationale:**
- Browser APIs provide accurate client-side timing
- xterm.js hooks catch rendering-specific issues
- Server metrics validate end-to-end flow

**Consequences:**
- More complex test setup
- Need to correlate metrics across layers
- Provides comprehensive performance picture

**Patterns Applied:** Observer Pattern, Metrics Collector

### ADR-003: Test Server Mode

**Context:** Need dedicated test endpoint for controlled terminal streaming without session management overhead.

**Decision:** Add `/api/test/terminal-stream` endpoint that accepts test configuration and streams specified patterns.

**Rationale:**
- Isolates test from session management complexity
- Allows precise control over streaming parameters
- Can be disabled in production builds

**Consequences:**
- Additional API surface to maintain
- Clear separation between test and production code
- Easy to extend with new test patterns

**Patterns Applied:** Test Endpoint Pattern, Feature Flags

---

## Story Breakdown

### Story 1: Test Data Generators [1 week]

**User Value:** Provide reusable data generation utilities for all stress test scenarios.

**Acceptance Criteria:**
- [ ] ASCII frame generator produces valid terminal frames at configurable FPS
- [ ] Log line generator produces 10K+ lines/second with realistic patterns
- [ ] Color sequence generator produces full ANSI 256-color spectrum
- [ ] Large text generator produces configurable size payloads
- [ ] All generators are deterministic with seed support

#### Task 1.1: ASCII Frame Generator [2h]

**Objective:** Create generator that produces ASCII art animation frames suitable for terminal playback.

**Context Boundary:**
- **Files:** `tests/e2e/generators/ascii-frames.ts` (new), `tests/e2e/generators/types.ts` (new)
- **Lines:** ~200 lines
- **Concepts:** Frame buffer, terminal escape sequences, animation timing

**Prerequisites:**
- Understanding of ANSI cursor positioning (`\x1b[H`, `\x1b[{row};{col}H`)
- Understanding of terminal clear sequences

**Implementation Approach:**
1. Define frame interface with content, dimensions, cursor position
2. Create simple animation patterns (scrolling text, bouncing ball, progress bars)
3. Add timing metadata for playback synchronization
4. Include checksum for frame validation

**Validation Strategy:**
- **Unit Tests:** Frame dimensions match specification
- **Integration Tests:** Frames render correctly in xterm
- **Success Criteria:** Can generate 1000 unique frames at 60 FPS target

**INVEST Check:**
- [x] Independent: No external dependencies
- [x] Negotiable: Animation patterns can be adjusted
- [x] Valuable: Foundation for stress tests
- [x] Estimable: Clear scope
- [x] Small: Single responsibility
- [x] Testable: Output is verifiable

#### Task 1.2: Log Line Generator [2h]

**Objective:** Create high-throughput log line generator with realistic patterns.

**Context Boundary:**
- **Files:** `tests/e2e/generators/log-lines.ts` (new), `tests/e2e/generators/types.ts`
- **Lines:** ~150 lines
- **Concepts:** String templating, rate limiting, pattern variation

**Prerequisites:**
- Understanding of common log formats (timestamp, level, message)
- Understanding of ANSI color codes for log levels

**Implementation Approach:**
1. Define log line templates with color codes
2. Create timestamp generator with configurable format
3. Implement rate-limited stream that respects target throughput
4. Add pattern variations (errors, warnings, info, debug)

**Validation Strategy:**
- **Unit Tests:** Line format matches specification
- **Integration Tests:** Can sustain 10K lines/sec for 10 seconds
- **Success Criteria:** Generator doesn't become bottleneck

**INVEST Check:**
- [x] Independent: No external dependencies
- [x] Negotiable: Log patterns can be customized
- [x] Valuable: Simulates real build output
- [x] Estimable: Clear throughput target
- [x] Small: Single responsibility
- [x] Testable: Throughput is measurable

#### Task 1.3: Color Stress Generator [2h]

**Objective:** Create generator that produces maximum ANSI color complexity.

**Context Boundary:**
- **Files:** `tests/e2e/generators/color-stress.ts` (new), `tests/e2e/generators/types.ts`
- **Lines:** ~150 lines
- **Concepts:** ANSI 256-color, RGB colors, color transitions

**Prerequisites:**
- Understanding of ANSI color escape sequences
- Understanding of 256-color and true-color modes

**Implementation Approach:**
1. Define color palette (256-color and true-color)
2. Create gradient generators for smooth transitions
3. Implement rainbow text generator
4. Add rapid color switching patterns

**Validation Strategy:**
- **Unit Tests:** Generated escape sequences are valid
- **Integration Tests:** Colors render correctly in xterm
- **Success Criteria:** Can stress color parsing without corruption

**INVEST Check:**
- [x] Independent: No external dependencies
- [x] Negotiable: Color patterns can vary
- [x] Valuable: Tests parser edge cases
- [x] Estimable: Clear color coverage target
- [x] Small: Single responsibility
- [x] Testable: Visual verification possible

#### Task 1.4: Large Payload Generator [1h]

**Objective:** Create generator for testing large single-write scenarios.

**Context Boundary:**
- **Files:** `tests/e2e/generators/large-payload.ts` (new), `tests/e2e/generators/types.ts`
- **Lines:** ~100 lines
- **Concepts:** Buffer management, chunking strategy

**Prerequisites:**
- Understanding of xterm.js write buffer behavior
- Understanding of flow control watermarks

**Implementation Approach:**
1. Define payload size configurations (1KB, 10KB, 100KB, 1MB)
2. Create content patterns (random, repeated, mixed)
3. Add checksums for integrity validation
4. Implement chunked vs single-write modes

**Validation Strategy:**
- **Unit Tests:** Payloads match requested size
- **Integration Tests:** Large writes don't crash terminal
- **Success Criteria:** 1MB payload completes in <5 seconds

**INVEST Check:**
- [x] Independent: No external dependencies
- [x] Negotiable: Size thresholds adjustable
- [x] Valuable: Tests buffer limits
- [x] Estimable: Clear size targets
- [x] Small: Simple generation logic
- [x] Testable: Size is verifiable

---

### Story 2: Test Server Streaming Endpoint [1 week]

**User Value:** Dedicated endpoint for controlled terminal streaming without session overhead.

**Acceptance Criteria:**
- [ ] `/api/test/terminal-stream` endpoint accepts streaming configuration
- [ ] Endpoint streams generated data at configured rate
- [ ] Supports all generator types (ASCII, logs, colors, large)
- [ ] Returns performance metrics after streaming completes
- [ ] Can be disabled in production via build flag

#### Task 2.1: Define Test Streaming Protocol [2h]

**Objective:** Design protobuf messages for test streaming configuration and control.

**Context Boundary:**
- **Files:** `proto/test/v1/test.proto` (new), `proto/session/v1/events.proto` (reference)
- **Lines:** ~100 lines
- **Concepts:** Protobuf message design, streaming RPC

**Prerequisites:**
- Understanding of existing protobuf patterns in project
- Understanding of streaming requirements from Task 1.x

**Implementation Approach:**
1. Define TestStreamRequest with generator type, rate, duration
2. Define TestStreamResponse with metrics
3. Add control messages (pause, resume, stop)
4. Generate Go and TypeScript bindings

**Validation Strategy:**
- **Unit Tests:** Messages serialize/deserialize correctly
- **Integration Tests:** Round-trip through WebSocket
- **Success Criteria:** Protocol supports all generator types

**INVEST Check:**
- [x] Independent: Self-contained protocol definition
- [x] Negotiable: Message fields adjustable
- [x] Valuable: Foundation for endpoint
- [x] Estimable: Clear message structure
- [x] Small: Protocol definition only
- [x] Testable: Serialization testable

#### Task 2.2: Implement Go Streaming Handler [3h]

**Objective:** Create server-side handler that streams test data to clients.

**Context Boundary:**
- **Files:** `server/services/test_streaming.go` (new), `server/server.go` (modify registration), `tests/e2e/generators/` (reference)
- **Lines:** ~250 lines
- **Concepts:** Go streaming, rate limiting, goroutine management

**Prerequisites:**
- Understanding of existing streaming patterns in session_service.go
- Completion of Task 2.1 (protocol definition)

**Implementation Approach:**
1. Create TestStreamingService implementing generated interface
2. Implement generator factory based on request type
3. Add rate limiting with time.Ticker
4. Implement graceful shutdown on context cancellation
5. Collect and return performance metrics

**Validation Strategy:**
- **Unit Tests:** Generator factory returns correct types
- **Integration Tests:** Stream delivers data at configured rate
- **Success Criteria:** Sustained 10KB/s streaming without drift

**INVEST Check:**
- [x] Independent: Uses defined protocol
- [x] Negotiable: Rate limiting strategy flexible
- [x] Valuable: Enables controlled testing
- [x] Estimable: Similar to existing handlers
- [x] Small: Single service responsibility
- [x] Testable: Rate accuracy measurable

#### Task 2.3: Implement TypeScript Client [2h]

**Objective:** Create client-side code to connect to test streaming endpoint.

**Context Boundary:**
- **Files:** `web-app/src/lib/hooks/useTestStream.ts` (new), `web-app/src/lib/hooks/useTerminalStream.ts` (reference)
- **Lines:** ~150 lines
- **Concepts:** WebSocket client, React hooks, streaming state

**Prerequisites:**
- Understanding of existing useTerminalStream hook
- Completion of Task 2.1 (protocol definition)

**Implementation Approach:**
1. Create useTestStream hook mirroring useTerminalStream patterns
2. Add configuration methods for generator type and rate
3. Implement metrics collection (bytes received, frame timing)
4. Add error handling and reconnection logic

**Validation Strategy:**
- **Unit Tests:** Hook state transitions correctly
- **Integration Tests:** Connects and receives stream
- **Success Criteria:** Can sustain streaming for 60 seconds

**INVEST Check:**
- [x] Independent: Uses defined protocol
- [x] Negotiable: Hook API flexible
- [x] Valuable: Enables test page integration
- [x] Estimable: Similar to existing hook
- [x] Small: Client-side only
- [x] Testable: Stream metrics verifiable

---

### Story 3: Stress Test Page Implementation [1 week]

**User Value:** Interactive test page for manual and automated stress testing.

**Acceptance Criteria:**
- [ ] Test page at `/test/terminal-stress` with configuration controls
- [ ] Real-time performance metrics display
- [ ] Preset configurations for common test scenarios
- [ ] Screenshot-friendly layout for visual regression testing
- [ ] Export metrics to JSON for analysis

#### Task 3.1: Create Test Page Layout [2h]

**Objective:** Build React component structure for stress test page.

**Context Boundary:**
- **Files:** `web-app/src/app/test/terminal-stress/page.tsx` (new), `web-app/src/components/sessions/TerminalOutput.tsx` (reference)
- **Lines:** ~200 lines
- **Concepts:** Next.js pages, component composition, CSS modules

**Prerequisites:**
- Understanding of existing test-terminal page structure
- Understanding of TerminalOutput component

**Implementation Approach:**
1. Create page with configuration panel and terminal area
2. Add test preset buttons (ASCII video, log flood, etc.)
3. Create metrics display area with live updates
4. Add export button for metrics JSON
5. Style for dark/light mode support

**Validation Strategy:**
- **Unit Tests:** Components render without errors
- **Integration Tests:** Page loads and is interactive
- **Success Criteria:** Clean layout matching design system

**INVEST Check:**
- [x] Independent: UI only, uses existing components
- [x] Negotiable: Layout details flexible
- [x] Valuable: Enables test interaction
- [x] Estimable: Standard page structure
- [x] Small: Layout and structure only
- [x] Testable: Renders correctly

#### Task 3.2: Implement Configuration Controls [2h]

**Objective:** Create interactive controls for test configuration.

**Context Boundary:**
- **Files:** `web-app/src/components/test/StressTestControls.tsx` (new), `web-app/src/components/test/types.ts` (new)
- **Lines:** ~200 lines
- **Concepts:** Form controls, state management, validation

**Prerequisites:**
- Completion of Task 3.1 (page layout)
- Understanding of test generator parameters

**Implementation Approach:**
1. Create generator type selector (dropdown)
2. Add rate configuration (slider with numeric input)
3. Add duration configuration (seconds)
4. Implement preset buttons with predefined configs
5. Add validation for parameter combinations

**Validation Strategy:**
- **Unit Tests:** Controls update state correctly
- **Integration Tests:** Configuration reaches server
- **Success Criteria:** All generator types configurable

**INVEST Check:**
- [x] Independent: UI controls only
- [x] Negotiable: Control types flexible
- [x] Valuable: Enables test customization
- [x] Estimable: Standard form controls
- [x] Small: Configuration UI only
- [x] Testable: State changes verifiable

#### Task 3.3: Implement Live Metrics Display [3h]

**Objective:** Create real-time metrics visualization.

**Context Boundary:**
- **Files:** `web-app/src/components/test/MetricsDisplay.tsx` (new), `web-app/src/lib/hooks/usePerformanceMetrics.ts` (new)
- **Lines:** ~300 lines
- **Concepts:** Performance measurement, RAF timing, statistics

**Prerequisites:**
- Completion of Task 3.1 (page layout)
- Understanding of browser Performance API

**Implementation Approach:**
1. Create metrics hook that tracks frame timing
2. Implement rolling statistics (min, max, avg, p95, p99)
3. Create visual display with gauges/charts
4. Add memory usage tracking via performance.memory
5. Implement exportable metrics structure

**Validation Strategy:**
- **Unit Tests:** Statistics calculated correctly
- **Integration Tests:** Metrics update during streaming
- **Success Criteria:** Sub-ms timing accuracy

**INVEST Check:**
- [x] Independent: Uses browser APIs
- [x] Negotiable: Visualization style flexible
- [x] Valuable: Core test output
- [x] Estimable: Statistics are standard
- [x] Small: Metrics display only
- [x] Testable: Known inputs produce known stats

---

### Story 4: Playwright Test Suites [1 week]

**User Value:** Automated tests that catch performance regressions.

**Acceptance Criteria:**
- [ ] ASCII video playback test at 30 and 60 FPS
- [ ] Log flood test at 1K, 5K, 10K lines/sec
- [ ] Large payload test at 100KB, 500KB, 1MB
- [ ] Memory leak detection over 60 second run
- [ ] All tests produce metrics artifacts

#### Task 4.1: ASCII Video Playback Tests [3h]

**Objective:** Create Playwright tests for ASCII video streaming.

**Context Boundary:**
- **Files:** `tests/e2e/terminal-stress/ascii-video.spec.ts` (new), `tests/e2e/terminal-stress/helpers.ts` (new)
- **Lines:** ~250 lines
- **Concepts:** Playwright testing, performance assertions, frame analysis

**Prerequisites:**
- Completion of Story 3 (test page)
- Understanding of existing terminal-flickering.spec.ts patterns

**Implementation Approach:**
1. Create test helper for stress page interaction
2. Implement 30 FPS playback test with frame rate assertions
3. Implement 60 FPS playback test with more stringent timing
4. Add screenshot capture for visual regression
5. Collect and export performance metrics

**Validation Strategy:**
- **Unit Tests:** Helpers work correctly
- **Integration Tests:** Tests pass on CI
- **Success Criteria:** Can detect 10% frame rate degradation

**INVEST Check:**
- [x] Independent: Uses test page
- [x] Negotiable: Thresholds adjustable
- [x] Valuable: Catches regressions
- [x] Estimable: Similar to existing tests
- [x] Small: ASCII video scenarios only
- [x] Testable: Performance measurable

#### Task 4.2: Log Flood Tests [2h]

**Objective:** Create Playwright tests for high-volume log output.

**Context Boundary:**
- **Files:** `tests/e2e/terminal-stress/log-flood.spec.ts` (new), `tests/e2e/terminal-stress/helpers.ts`
- **Lines:** ~200 lines
- **Concepts:** Throughput testing, streaming stability

**Prerequisites:**
- Completion of Task 4.1 (test helpers)
- Understanding of log generator parameters

**Implementation Approach:**
1. Create 1K lines/sec test (baseline)
2. Create 5K lines/sec test (moderate load)
3. Create 10K lines/sec test (stress load)
4. Assert no dropped data (checksum validation)
5. Assert frame rate stays above minimum

**Validation Strategy:**
- **Unit Tests:** Rate calculations correct
- **Integration Tests:** All scenarios complete
- **Success Criteria:** Detect when throughput causes drops

**INVEST Check:**
- [x] Independent: Uses test page
- [x] Negotiable: Rate thresholds adjustable
- [x] Valuable: Catches throughput issues
- [x] Estimable: Parameterized scenarios
- [x] Small: Log flood scenarios only
- [x] Testable: Throughput measurable

#### Task 4.3: Large Payload Tests [2h]

**Objective:** Create Playwright tests for large single-write scenarios.

**Context Boundary:**
- **Files:** `tests/e2e/terminal-stress/large-payload.spec.ts` (new), `tests/e2e/terminal-stress/helpers.ts`
- **Lines:** ~150 lines
- **Concepts:** Buffer management, flow control

**Prerequisites:**
- Completion of Task 4.1 (test helpers)
- Understanding of flow control watermarks

**Implementation Approach:**
1. Create 100KB payload test (below watermark)
2. Create 500KB payload test (triggers flow control)
3. Create 1MB payload test (extended flow control)
4. Assert payload integrity via checksum
5. Assert no browser freeze during write

**Validation Strategy:**
- **Unit Tests:** Checksum logic correct
- **Integration Tests:** All sizes complete successfully
- **Success Criteria:** Detect buffer overflow issues

**INVEST Check:**
- [x] Independent: Uses test page
- [x] Negotiable: Size thresholds adjustable
- [x] Valuable: Catches buffer issues
- [x] Estimable: Simple scenario structure
- [x] Small: Payload scenarios only
- [x] Testable: Integrity verifiable

#### Task 4.4: Memory Leak Detection [3h]

**Objective:** Create Playwright tests that detect memory leaks over time.

**Context Boundary:**
- **Files:** `tests/e2e/terminal-stress/memory-leak.spec.ts` (new), `tests/e2e/terminal-stress/helpers.ts`
- **Lines:** ~200 lines
- **Concepts:** Memory profiling, heap snapshots, trend analysis

**Prerequisites:**
- Completion of Task 4.1 (test helpers)
- Understanding of Chrome DevTools Protocol for memory

**Implementation Approach:**
1. Create 60-second streaming test
2. Capture heap snapshots at intervals (0s, 15s, 30s, 45s, 60s)
3. Analyze memory trend (should be flat or decreasing)
4. Assert growth under threshold (50MB)
5. Report detailed memory breakdown

**Validation Strategy:**
- **Unit Tests:** Trend analysis logic correct
- **Integration Tests:** Detects intentional memory leak
- **Success Criteria:** Can detect 5MB/min leak rate

**INVEST Check:**
- [x] Independent: Uses standard APIs
- [x] Negotiable: Thresholds adjustable
- [x] Valuable: Catches memory issues
- [x] Estimable: Clear measurement approach
- [x] Small: Memory analysis only
- [x] Testable: Comparison with baseline

---

### Story 5: CI/CD Integration [3 days]

**User Value:** Automated regression detection on every PR.

**Acceptance Criteria:**
- [ ] Stress tests run as part of PR checks
- [ ] Performance metrics stored as artifacts
- [ ] Baseline comparison for regression detection
- [ ] Clear pass/fail criteria with actionable messages

#### Task 5.1: GitHub Actions Workflow [2h]

**Objective:** Create workflow that runs stress tests on PRs.

**Context Boundary:**
- **Files:** `.github/workflows/stress-tests.yml` (new), `.github/workflows/test.yml` (reference)
- **Lines:** ~100 lines
- **Concepts:** GitHub Actions, artifact storage, job dependencies

**Prerequisites:**
- Understanding of existing CI workflows
- Completion of Story 4 (Playwright tests)

**Implementation Approach:**
1. Create workflow triggered on PR to main
2. Add job to build and start test server
3. Add job to run stress test suite
4. Upload metrics as artifacts
5. Add failure comments with metrics summary

**Validation Strategy:**
- **Unit Tests:** N/A (workflow file)
- **Integration Tests:** Workflow runs successfully
- **Success Criteria:** Tests run on every PR

**INVEST Check:**
- [x] Independent: Workflow is self-contained
- [x] Negotiable: Triggers adjustable
- [x] Valuable: Enables automation
- [x] Estimable: Standard workflow patterns
- [x] Small: CI configuration only
- [x] Testable: Workflow execution visible

#### Task 5.2: Baseline Comparison [2h]

**Objective:** Implement comparison against stored performance baselines.

**Context Boundary:**
- **Files:** `tests/e2e/terminal-stress/baseline.ts` (new), `.github/workflows/stress-tests.yml`
- **Lines:** ~150 lines
- **Concepts:** Statistical comparison, threshold alerts

**Prerequisites:**
- Completion of Task 5.1 (workflow)
- Understanding of metrics structure from Story 3

**Implementation Approach:**
1. Define baseline metrics structure (stored in repo)
2. Implement comparison function with tolerance
3. Generate comparison report (better/worse/same)
4. Fail check if regression exceeds threshold
5. Update baseline command for releases

**Validation Strategy:**
- **Unit Tests:** Comparison logic correct
- **Integration Tests:** Detects intentional regression
- **Success Criteria:** 10% regression triggers failure

**INVEST Check:**
- [x] Independent: Uses stored baselines
- [x] Negotiable: Thresholds adjustable
- [x] Valuable: Catches regressions
- [x] Estimable: Statistical comparison
- [x] Small: Comparison logic only
- [x] Testable: Known regressions detectable

---

## Known Issues

### Bug 001: RAF Timing Variance in CI [SEVERITY: Medium]

**Description:** RequestAnimationFrame timing may vary significantly in CI environments due to lack of display, causing flaky performance assertions.

**Mitigation:**
- Use wider tolerance thresholds for CI vs local
- Add environment detection to adjust assertions
- Consider using virtual framebuffer (xvfb) for CI

**Files Likely Affected:**
- `tests/e2e/terminal-stress/*.spec.ts` - Assertion thresholds
- `.github/workflows/stress-tests.yml` - xvfb setup

**Prevention Strategy:**
- Run tests with xvfb in CI from start
- Design assertions with CI variance in mind

**Related Tasks:** Task 4.1, Task 5.1

### Bug 002: Memory Measurement Inconsistency [SEVERITY: Low]

**Description:** `performance.memory` API is Chrome-specific and may not be available in all Playwright browsers.

**Mitigation:**
- Restrict memory tests to Chromium project only
- Add fallback using heap snapshot API
- Document limitation in test README

**Files Likely Affected:**
- `tests/e2e/terminal-stress/memory-leak.spec.ts` - Memory API usage
- `playwright.config.ts` - Project configuration

**Prevention Strategy:**
- Check API availability before use
- Provide alternative measurement methods

**Related Tasks:** Task 4.4

---

## Dependency Visualization

```
Story 1: Test Data Generators
├── Task 1.1: ASCII Frame Generator
├── Task 1.2: Log Line Generator (can parallel with 1.1)
├── Task 1.3: Color Stress Generator (can parallel with 1.1, 1.2)
└── Task 1.4: Large Payload Generator (can parallel with others)

Story 2: Test Server Endpoint (depends on Story 1)
├── Task 2.1: Protocol Definition
├── Task 2.2: Go Streaming Handler (depends on 2.1)
└── Task 2.3: TypeScript Client (depends on 2.1, can parallel with 2.2)

Story 3: Stress Test Page (depends on Story 2)
├── Task 3.1: Test Page Layout
├── Task 3.2: Configuration Controls (depends on 3.1)
└── Task 3.3: Live Metrics Display (depends on 3.1, can parallel with 3.2)

Story 4: Playwright Test Suites (depends on Story 3)
├── Task 4.1: ASCII Video Tests (creates helpers)
├── Task 4.2: Log Flood Tests (depends on 4.1 helpers)
├── Task 4.3: Large Payload Tests (depends on 4.1 helpers)
└── Task 4.4: Memory Leak Detection (depends on 4.1 helpers)

Story 5: CI/CD Integration (depends on Story 4)
├── Task 5.1: GitHub Actions Workflow
└── Task 5.2: Baseline Comparison (depends on 5.1)
```

**Parallel Execution Opportunities:**
- Tasks 1.1-1.4 can all run in parallel
- Tasks 2.2 and 2.3 can run in parallel after 2.1
- Tasks 3.2 and 3.3 can run in parallel after 3.1
- Tasks 4.2, 4.3, 4.4 can run in parallel after 4.1

---

## Integration Checkpoints

### After Story 1
- [ ] All generators produce valid output
- [ ] Generators maintain target throughput
- [ ] Unit tests pass for all generators

### After Story 2
- [ ] Test endpoint streams all generator types
- [ ] TypeScript client receives streams correctly
- [ ] Rate limiting accurate to within 5%

### After Story 3
- [ ] Test page renders and is interactive
- [ ] All presets produce expected output
- [ ] Metrics display updates in real-time
- [ ] Export produces valid JSON

### After Story 4
- [ ] All Playwright tests pass locally
- [ ] Tests complete within time limits
- [ ] Metrics artifacts are generated

### Final Checkpoint
- [ ] CI workflow runs on PRs
- [ ] Baseline comparison works
- [ ] Documentation complete
- [ ] Performance baselines established

---

## Context Preparation Guide

### Task 1.1: ASCII Frame Generator
**Files to Load:**
- `tests/e2e/terminal-flickering.spec.ts` - Existing test patterns
- `web-app/src/lib/terminal/StateApplicator.ts` - Frame structure reference

**Concepts to Understand:**
- ANSI escape sequences for cursor positioning
- Terminal coordinate system (1-indexed rows/cols)

### Task 2.2: Go Streaming Handler
**Files to Load:**
- `server/services/session_service.go` - Existing streaming patterns
- `session/response_stream.go` - Response streaming logic
- Generated protobuf from Task 2.1

**Concepts to Understand:**
- Go context cancellation patterns
- gRPC streaming lifecycle

### Task 3.3: Live Metrics Display
**Files to Load:**
- `web-app/src/components/sessions/TerminalOutput.tsx` - Flow control metrics
- Browser Performance API documentation

**Concepts to Understand:**
- RequestAnimationFrame timing
- Rolling statistics calculation

### Task 4.4: Memory Leak Detection
**Files to Load:**
- Playwright CDP (Chrome DevTools Protocol) documentation
- `web-app/src/lib/hooks/useTerminalStream.ts` - Memory-sensitive code

**Concepts to Understand:**
- Heap snapshot analysis
- Memory trend detection algorithms

---

## Success Criteria

- [ ] All atomic tasks completed and validated
- [ ] All acceptance criteria met for each story
- [ ] Test coverage meets requirements (>80% of streaming code paths)
- [ ] Performance baselines established and documented
- [ ] Documentation complete and accurate
- [ ] CI integration working on PRs
- [ ] Code review approved
