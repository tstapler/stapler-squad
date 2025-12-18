# Feature Plan: ANSI Escape Code Test Harness

## Executive Summary

Create a comprehensive test harness that validates the web UI's TerminalOutput component can properly render all ANSI escape codes observed in production Claude sessions. The harness will use the exported escape-codes.json file (77 unique codes, 26,974 occurrences) as the source of truth for coverage, generating test frames that exercise all codes and providing visual verification capabilities.

## Problem Statement

### Current Gaps
- Web UI may not properly handle all escape codes sent from the server
- Current test generators only cover a subset of escape codes
- Missing critical codes like Erase to End of Line (`\x1b[K`), default colors, and character sets
- No systematic way to verify rendering correctness for all production escape codes
- Risk of terminal corruption or visual artifacts in production

### Impact
- **User Experience**: Improperly rendered escape codes cause visual corruption in terminal output
- **Data Integrity**: Some codes affect text positioning and could misrepresent command outputs
- **Testing Coverage**: Current stress tests don't exercise 70% of production escape codes
- **Debugging Difficulty**: Hard to reproduce escape code issues without systematic testing

## Requirements

### Functional Requirements

**FR1: Escape Code Coverage**
- Generate test frames using ALL 77 unique escape codes from production
- Prioritize high-frequency codes (>1000 occurrences)
- Handle code combinations as they appear in real sessions
- Support parameterized codes with various argument values

**FR2: Test Frame Generation**
- Create deterministic, reproducible test sequences
- Generate frames that isolate individual escape codes for targeted testing
- Produce combination frames that mix multiple codes realistically
- Support configurable frame rates and data volumes

**FR3: Visual Verification**
- Provide visual test page for manual verification
- Display expected vs actual rendering comparisons
- Show escape code metadata (name, category, frequency)
- Highlight rendering differences or errors

**FR4: Automated Testing**
- Integrate with existing Playwright E2E test suite
- Validate terminal state after each escape code
- Check for visual corruption or unexpected behavior
- Generate coverage reports showing tested codes

**FR5: Performance Testing**
- Measure rendering performance for each escape code category
- Test high-volume scenarios with rapid code sequences
- Monitor memory usage during extended test runs
- Identify performance bottlenecks for specific codes

### Non-Functional Requirements

**NFR1: Maintainability**
- Automatically update when escape-codes.json changes
- Clear separation between test data and test logic
- Well-documented code generation patterns
- Easy to add new test scenarios

**NFR2: Performance**
- Test harness should not impact normal development workflow
- Frame generation must be fast enough for 60 FPS testing
- Memory-efficient for long-running stress tests
- Minimal CPU overhead during test execution

**NFR3: Usability**
- Simple interface for selecting test scenarios
- Clear visual feedback during test execution
- Exportable test results and metrics
- Easy debugging when tests fail

**NFR4: Reliability**
- Deterministic test results (same input → same output)
- Graceful handling of rendering failures
- Timeout protection for hanging tests
- Recovery from terminal corruption

## Architecture & Design

### Component Architecture

```
┌─────────────────────────────────────────────────┐
│                Test Harness UI                  │
│  ┌──────────────┐  ┌──────────────────────┐   │
│  │ Test Selector│  │  Visual Verification  │   │
│  └──────────────┘  └──────────────────────┘   │
└─────────────────────────────────────────────────┘
                         │
┌─────────────────────────────────────────────────┐
│            Escape Code Test Engine              │
│  ┌──────────────┐  ┌──────────────────────┐   │
│  │ Code Library │  │  Frame Generator      │   │
│  └──────────────┘  └──────────────────────┘   │
└─────────────────────────────────────────────────┘
                         │
┌─────────────────────────────────────────────────┐
│              Terminal Rendering                 │
│  ┌──────────────┐  ┌──────────────────────┐   │
│  │   XTerm.js   │  │  TerminalOutput      │   │
│  └──────────────┘  └──────────────────────┘   │
└─────────────────────────────────────────────────┘
                         │
┌─────────────────────────────────────────────────┐
│            Validation & Metrics                 │
│  ┌──────────────┐  ┌──────────────────────┐   │
│  │   Playwright │  │  Coverage Reports    │   │
│  └──────────────┘  └──────────────────────┘   │
└─────────────────────────────────────────────────┘
```

### Data Model

```typescript
interface EscapeCodeDefinition {
  code: string;           // Hex representation
  sequence: string;       // Actual escape sequence
  humanReadable: string;  // Description
  category: EscapeCategory;
  count: number;          // Production frequency
  params?: ParamSpec[];   // Parameter specifications
  testPriority: 'critical' | 'high' | 'medium' | 'low';
}

interface TestScenario {
  id: string;
  name: string;
  description: string;
  codes: EscapeCodeDefinition[];
  pattern: 'isolated' | 'sequential' | 'mixed' | 'stress';
  frameCount: number;
  frameRate: number;
}

interface TestFrame {
  sequence: number;
  content: string;
  codes: string[];        // Escape codes used in this frame
  validation: ValidationRules;
  expectedState: TerminalState;
}

interface ValidationRules {
  cursorPosition?: { row: number; col: number };
  textContent?: string[];
  colors?: ColorExpectation[];
  attributes?: AttributeExpectation[];
}
```

### Key Design Patterns

**1. Code Priority System**
```typescript
// Prioritize based on production frequency
const priorityMap = {
  critical: codes.filter(c => c.count > 1000),   // 5 codes
  high: codes.filter(c => c.count > 100),        // 12 codes  
  medium: codes.filter(c => c.count > 10),       // 25 codes
  low: codes.filter(c => c.count <= 10)          // 35 codes
};
```

**2. Test Pattern Types**
- **Isolated**: Single escape code per frame for targeted testing
- **Sequential**: Codes in sequence as they appear in production
- **Mixed**: Realistic combinations based on production patterns
- **Stress**: Rapid-fire sequences to test performance limits

**3. Frame Generation Strategy**
```typescript
class EscapeCodeFrameGenerator {
  generateIsolatedFrame(code: EscapeCodeDefinition): TestFrame
  generateSequentialFrames(codes: EscapeCodeDefinition[]): TestFrame[]
  generateMixedFrame(scenario: TestScenario): TestFrame
  generateStressFrames(rate: number, duration: number): TestFrame[]
}
```

## Known Issues & Mitigation

### 🐛 Race Condition: Rapid Escape Code Processing [SEVERITY: High]

**Description**: Terminal may drop or misinterpret codes when processing >1000 codes/second.

**Mitigation**:
- Implement frame rate limiting in test harness
- Add inter-code delays for stress testing
- Monitor terminal buffer state
- Validate each code is processed before sending next

**Prevention**:
- Default to 60 FPS max in test scenarios
- Queue codes with backpressure handling
- Add performance metrics for code processing time

### 🐛 Memory Leak: Terminal Buffer Growth [SEVERITY: High]

**Description**: Long-running tests may cause unbounded terminal buffer growth.

**Mitigation**:
- Implement scrollback limits in test terminal
- Periodically clear terminal during long tests
- Monitor memory usage and alert on growth
- Add test duration limits

**Prevention**:
- Configure reasonable scrollback limits (10,000 lines)
- Clear terminal between test scenarios
- Use memory profiling in stress tests

### 🐛 Visual Corruption: Unsupported Escape Codes [SEVERITY: Medium]

**Description**: XTerm.js may not support all escape codes, causing visual artifacts.

**Mitigation**:
- Document unsupported codes
- Provide fallback rendering for critical codes
- Skip unsupported codes in automated tests
- Log warnings for rendering failures

**Prevention**:
- Test against XTerm.js compatibility matrix
- Implement escape code filtering
- Provide graceful degradation

### 🐛 State Inconsistency: Cursor Position Drift [SEVERITY: Medium]

**Description**: Cursor position may drift over time due to accumulated errors.

**Mitigation**:
- Reset terminal state between test scenarios
- Validate cursor position after each frame
- Implement cursor position correction
- Add position tracking metrics

**Prevention**:
- Use absolute positioning where possible
- Validate all cursor movement codes
- Add position assertion helpers

## Implementation Roadmap

### Phase 1: Core Infrastructure (MVP)
**Goal**: Basic test harness with critical escape codes

**Tasks**:
1. Create `EscapeCodeLibrary` from escape-codes.json
2. Implement `IsolatedFrameGenerator` for single-code testing
3. Build test UI page at `/test/escape-codes`
4. Add 5 critical escape codes (>1000 occurrences)
5. Create basic Playwright test for critical codes
6. Implement visual verification interface

**Deliverables**:
- Working test page with critical codes
- Basic E2E test coverage
- Visual verification capability

### Phase 2: Comprehensive Coverage
**Goal**: Full escape code coverage with all 77 codes

**Tasks**:
1. Implement remaining 72 escape codes
2. Add parameterized code support
3. Create `SequentialFrameGenerator` for code sequences
4. Build code combination patterns from production data
5. Add coverage reporting
6. Implement test result persistence

**Deliverables**:
- Complete escape code library
- Coverage reports showing 100% code testing
- Production-like test scenarios

### Phase 3: Advanced Testing
**Goal**: Stress testing and performance validation

**Tasks**:
1. Implement `StressFrameGenerator` for high-volume testing
2. Add performance metrics collection
3. Create memory leak detection
4. Build automated regression suite
5. Add visual diff testing
6. Implement test result comparison

**Deliverables**:
- Performance benchmarks for all codes
- Memory usage profiles
- Automated regression tests

### Phase 4: Integration & Polish
**Goal**: Full integration with development workflow

**Tasks**:
1. Integrate with CI/CD pipeline
2. Add pre-commit hooks for escape code validation
3. Create developer documentation
4. Build escape code playground for debugging
5. Add real-time escape code monitoring
6. Implement test result dashboards

**Deliverables**:
- CI/CD integration
- Developer tools and documentation
- Production monitoring capabilities

## Testing Strategy

### Unit Tests
```typescript
// Test escape code parsing
describe('EscapeCodeLibrary', () => {
  test('parses escape-codes.json correctly')
  test('categorizes codes by priority')
  test('generates valid escape sequences')
  test('handles parameterized codes')
});

// Test frame generation
describe('FrameGenerators', () => {
  test('generates isolated frames for each code')
  test('creates valid sequential patterns')
  test('produces deterministic output')
  test('handles edge cases gracefully')
});
```

### Integration Tests
```typescript
// Test terminal rendering
describe('TerminalRendering', () => {
  test('renders all critical escape codes')
  test('handles rapid code sequences')
  test('maintains cursor position accuracy')
  test('preserves text attributes correctly')
});
```

### E2E Tests
```typescript
// Playwright tests
describe('EscapeCodeE2E', () => {
  test('critical codes render without corruption')
  test('high-volume sequences perform acceptably')
  test('memory usage remains bounded')
  test('visual output matches expectations')
});
```

### Performance Tests
```typescript
// Benchmark tests
describe('EscapeCodePerformance', () => {
  benchmark('render 1000 codes/second')
  benchmark('process 10MB of escape sequences')
  benchmark('handle 100 concurrent code types')
  benchmark('maintain 60 FPS with stress load')
});
```

## File Structure

```
web-app/
├── src/
│   ├── lib/
│   │   ├── test-generators/
│   │   │   ├── escape-codes/
│   │   │   │   ├── library.ts         # Escape code definitions
│   │   │   │   ├── generators.ts      # Frame generators
│   │   │   │   ├── validators.ts      # Validation logic
│   │   │   │   ├── patterns.ts        # Production patterns
│   │   │   │   └── index.ts
│   │   │   └── index.ts
│   │   └── escape-codes/
│   │       ├── parser.ts              # Parse escape-codes.json
│   │       ├── categories.ts          # Code categorization
│   │       └── coverage.ts            # Coverage tracking
│   ├── app/
│   │   └── test/
│   │       └── escape-codes/
│   │           ├── page.tsx           # Test UI
│   │           ├── components/
│   │           │   ├── CodeSelector.tsx
│   │           │   ├── VisualVerifier.tsx
│   │           │   ├── MetricsPanel.tsx
│   │           │   └── CoverageReport.tsx
│   │           └── layout.tsx
│   └── data/
│       └── escape-codes.json          # Production data
└── tests/
    └── e2e/
        └── escape-codes/
            ├── critical.spec.ts        # Critical codes
            ├── comprehensive.spec.ts   # All codes
            ├── stress.spec.ts         # Performance
            └── regression.spec.ts     # Regression suite
```

## Success Metrics

### Coverage Metrics
- **Code Coverage**: 100% of 77 production escape codes tested
- **Frequency Coverage**: 100% of top 20 most-used codes validated
- **Combination Coverage**: 80% of common code combinations tested
- **Parameter Coverage**: All parameter variations for critical codes

### Performance Metrics
- **Rendering Speed**: <16ms per frame (60 FPS) with escape codes
- **Memory Stability**: <10% memory growth over 10-minute test
- **Code Processing**: >1000 codes/second without drops
- **Visual Accuracy**: Zero corruption in critical code rendering

### Quality Metrics
- **Test Reliability**: <1% flake rate in E2E tests
- **Bug Detection**: Catch 100% of escape code regressions
- **Documentation**: All 77 codes documented with examples
- **Developer Adoption**: Test harness used in >80% of terminal PRs

## Risk Assessment

### Technical Risks
- **XTerm.js Limitations**: Some escape codes may not be supported
  - *Mitigation*: Document unsupported codes, provide workarounds
- **Performance Impact**: Test harness may slow down development
  - *Mitigation*: Optimize generators, provide quick test modes
- **Maintenance Burden**: Keeping tests updated with new codes
  - *Mitigation*: Automate from escape-codes.json

### Schedule Risks
- **Scope Creep**: Adding more test scenarios than needed
  - *Mitigation*: Focus on production-observed codes only
- **Integration Delays**: Playwright test complexity
  - *Mitigation*: Start with simple visual tests

## Documentation Requirements

### Developer Guide
- How to run escape code tests
- Adding new test scenarios
- Debugging rendering issues
- Understanding test results

### API Documentation
- EscapeCodeLibrary API
- Frame generator interfaces
- Validation rule format
- Coverage report structure

### User Documentation
- Visual verification guide
- Interpreting test results
- Common escape code issues
- Performance tuning tips

## Appendix: Critical Escape Codes

Based on production data, these are the most critical codes to test first:

1. **`\x1b[K`** (4,767 uses) - Erase to End of Line
   - Critical for line clearing in progress indicators
   - Must handle correctly to avoid text overlap

2. **`\x1b[39m`** (3,678 uses) - Default Foreground Color
   - Resets text color to terminal default
   - Essential for color state management

3. **`\x1b(B`** (3,366 uses) - G0 ASCII Character Set
   - Sets standard ASCII character encoding
   - Critical for text display accuracy

4. **`\x1b[48;5;{n}m`** (1,452 uses) - 256-color Background
   - Extended color palette for backgrounds
   - Important for syntax highlighting

5. **`\x1b[49m`** (1,100 uses) - Default Background Color
   - Resets background to terminal default
   - Pairs with foreground reset for clean state

These five codes account for 53% of all escape code usage in production and must be the first priority for testing.
