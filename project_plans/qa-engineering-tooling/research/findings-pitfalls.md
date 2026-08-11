# Findings: Pitfalls

## Summary

QA engineering tooling compounds complexity at each layer. AST scanners create false positive/negative traps through generated code and language edge cases. E2E test suites become maintenance burdens through timing-dependent flakiness in CI environments. AI-assisted UX analysis risks confident hallucinations that waste human review time. Video capture in headless CI hits codec, container networking, and artifact size walls. The highest risk is not technical failure — it's building systems nobody maintains because they're too fragile or noisy to trust.

Key tension: lean tooling (fewer false positives, easier to maintain) trades off against comprehensive tooling (catches more issues but generates more maintenance work). For a solo practitioner, this tradeoff is critical.

---

## Options Surveyed

### Approach 1: Do Nothing
- No scanner automation; manual feature tracking via documentation or spreadsheets
- Manual QA verification for each feature before ship
- No systematic UX review
- No video capture for PR context

### Approach 2: Lean Tooling (Minimal False Positives, High Maintenance Tolerance)
- Scanner: Go AST scanner that only captures _explicitly decorated_ handlers (via marker comments); TypeScript scanner captures only router-registered components
- E2E: Hand-written critical-path tests only; no generated test suites; explicit wait strategies with generous timeouts
- UX Analysis: Manual Claude API calls on demand, not automated; high-touch, low-noise
- Video: Capture only on merge-to-main or explicit PR request; not automated on all PRs
- Trade-off: fewer false positives, less maintenance, lower coverage confidence

### Approach 3: Comprehensive Tooling (High Coverage, High Maintenance Risk)
- Scanner: Full AST traversal for all handlers and components; attempts to filter out generated code via heuristics
- E2E: Auto-generated tests from feature registry; aggressive timeout tuning; retries on failure
- UX Analysis: Automated per-PR, batch evaluation of screenshots, prompt templates applied to all flows
- Video: Auto-capture all PRs, attach to all feature-touching PRs, archive to storage
- Trade-off: higher coverage confidence, significant maintenance burden, high false positive/alarm fatigue

---

## Trade-off Matrix

| Dimension | Do Nothing | Lean Tooling | Comprehensive Tooling |
|-----------|-----------|--------------|----------------------|
| Maintenance Burden | None | Low (~5–10 hrs/month) | High (~15–25 hrs/month) |
| Coverage Confidence | None | Moderate (critical paths known) | High (but degraded by noise) |
| Failure Cost | Regressions reach users undetected | Fewer regressions; some edge cases miss | Many regressions caught, but noise erodes trust |
| False Positive Rate | N/A | ~5–10% | ~20–40% |
| Solo Practitioner Fit | Poor | Good (high signal, low noise) | Poor (maintenance burden outweighs benefits) |

---

## Risk and Failure Modes

### AST Scanner Pitfalls

#### False Positives in Go Handler Detection

- **Generated code**: Protobuf-generated `.pb.go` files contain struct definitions and helper functions that the scanner may classify as handlers. [TRAINING_ONLY - verify]
- **Unexported handlers**: Private methods (lowercase names) that match handler patterns but are internal utilities
- **Build tag exclusion**: Go code with `// +build integration` or `// +build test` tags runs conditionally; scanner may include/exclude inconsistently
- **Middleware and decorators**: Higher-order functions that wrap handlers; naive AST may double-count or miss

Manifestation: Feature registry lists non-existent or duplicate endpoints; registry diverges from reality.
Cost: Developer ignores scanner output as noise; registry becomes untrusted; manual verification still required.

#### False Negatives in Go Handler Detection

- **Functional composition**: Handlers built dynamically via function composition or builder patterns
- **Interface-based registration**: If handlers are registered via interface methods, scanner must infer the relationship
- **ConnectRPC middleware**: Custom interceptors that conditionally register routes based on runtime config

Cost: Feature inventory is incomplete; QA and monitoring gaps go unnoticed.

#### Go Generated Code Staleness

If `make generate` hasn't run before CI scan, `.pb.go` files and generated interfaces don't match reality. Scanner must run _after_ code generation, or CI must ensure `make generate` is up-to-date.

### False Positives in TypeScript/React Component Detection

- **HOCs and dynamic wrapping**: `export default withRouter(MyComponent)` — unclear whether MyComponent or wrapped result is the "real" component
- **Dynamic imports**: `const Component = lazy(() => import('./path'))` — can't be statically resolved; scanner may miss
- **Barrel exports**: `export * from './components'` — requires recursive traversal; can cause double-counting
- **Runtime routing**: Routes defined at runtime (from a route config object) aren't discoverable via static AST
- **Generated TS files**: Protobuf-generated `*_pb.ts` files may contain type definitions that look like components

### E2E Test Flakiness

#### Timing Failures
- Hardcoded `waitForTimeout(500)` works on dev machine, fails in CI (slower container network)
- `waitForLoadState('networkidle')` assumes consistent network; bursty CI network fails intermittently
- Tests wait for elements visually ready but CSS animations haven't cleared

#### State Leakage Between Tests
- Browser cache/cookies/localStorage from previous test pollute next test
- Tmux sessions not cleaned up between test runs (Stapler Squad-specific: stale sessions interfere)
- Database/file state created by tests and not torn down bleeds into subsequent tests

#### Container-Specific CI Flakiness
- **Port conflicts**: If tests run in parallel with hardcoded port 8543, one fails
- **Disk I/O**: CI containers have slower disk; file operations timeout
- **Memory pressure**: Playwright browser instances OOM-killed silently in constrained runners
- **Container DNS**: Localhost resolution slower in container networks

#### Retry Logic and Compounded Failure
- Retried failed tests may fail for different reasons (port still in use from failed previous run)
- Retry limits too low = masks real timing issues; too high = wastes CI time

### Playwright Video Capture in Headless CI

#### Codec and Container Issues [TRAINING_ONLY - verify]
- Missing codec libraries: headless browsers in minimal CI containers lack libavcodec or libvpx
- No GPU acceleration for H.264/VP9 encoding → slow or failed encoding
- Frame rate/DPI inconsistency causes codec mismatches

#### Artifact Size and Storage Limits [TRAINING_ONLY - verify]
- GitHub Actions artifact size limit ~5 GB per run; 30-min E2E suite with all-test video can exceed this
- 30–90 day retention; large video artifacts exhaust quota
- PR comment links to artifacts expire; brittle QA documentation

#### Video Capture Blocking Test Execution
- Synchronous encoding: test waits for video encoding before teardown → CI pipeline serializes
- Async encoding adds complexity; can lose frames or fail silently

### AI UX Analysis Hallucinations

#### Confident But Wrong Assertions
- False accessibility violations: "This button should have aria-label" when button already has proper semantic HTML
- False design inconsistency: flags intentional design hierarchy differences as errors
- Hallucinated best practices: "Add a loading spinner here" when operation completes instantly

Cost: Developer spends time verifying non-issues → stops trusting the tool.

#### Token Explosion on Large UIs [TRAINING_ONLY - verify]
- Complex dashboard with 10 state variations = 10 screenshots = 1,000–2,000 tokens per analysis
- Claude vision tokens are more expensive than text tokens
- Running UX analysis on every PR can cost $5–20/month even at small scale; scales poorly

#### Prompt Design Failure Modes
- **Too generic**: "Evaluate the UX quality" → "Colors look good. Layout is clear." Not actionable
- **Too specific**: "Check buttons are blue with rounded corners" → misses real issues
- **Misaligned expectations**: Prompt doesn't specify project context (accessibility? conversion? clarity?)

#### Rate Limits in CI
Multiple parallel PRs analyzed simultaneously can hit Claude API requests-per-minute limits → some analyses timeout → CI becomes unreliable.

### Maintenance Traps

#### Registry Drift
- New feature added with different naming convention → scanner misses it → registry incomplete
- Structural refactor (handler moved to different package) → scanner double-counts or drops
- Manual registry edits to "fix" scanner bugs → re-running scanner overwrites manual fixes

Cost: Registry becomes a maintenance burden; developers stop updating it.

#### Flaky Test Suite as Maintenance Burden
- Developer runs locally, passes. CI, fails intermittently. 2 hours debugging CI networking.
- "Fix" is arbitrary sleep or raised retry limits → test slower, harder to debug
- New developers assume test suite is unreliable → stop relying on it for signal

#### Scanner Noise Erodes Trust
Every PR, scanner reports 5 new "handlers" that are internal utilities or generated code. Developer dismisses output without reading. Real issues hidden in noise.

#### QA Tooling Harder to Update Than the App
- Scanner depends on specific Go conventions or file naming → app refactors break scanner
- E2E tests use brittle selectors (`nth-child(3)`) that break when DOM changes
- UX analysis prompt calibrated to current design → design changes require prompt updates

---

## Migration and Adoption Cost

| Component | Initial Effort | False Positive Triage | Ongoing/Month |
|-----------|---------------|----------------------|---------------|
| Go AST scanner | 2–4 weeks | 20–30 hrs calibration | 2–4 hrs |
| TS/React AST scanner | 2–3 weeks | 20–30 hrs calibration | 2–4 hrs |
| E2E critical-path tests | 1 week scaffolding + 1–2 hrs/test | N/A | 3–5 hrs flaky triage |
| Claude API UX integration | 2–3 days + 1 week prompt tuning | N/A | 1–2 hrs monitoring |
| Video capture + CI | 2–3 days + 1 week CI debug | N/A | 2–3 hrs mgmt |

**Comprehensive tooling total**: 8–10 weeks first month, 15–25 hrs/month ongoing.
**Lean tooling total**: 4–5 weeks first month, 5–10 hrs/month ongoing.

---

## Operational Concerns

**CI reliability**: E2E test failures should block merge (indicate real problems). Video capture and UX analysis failures should warn but not block (informational).

**Data retention and PII**: Videos may capture sensitive UI data. GitHub artifact retention = data persists 30–90 days. Mitigation: anonymize video data or use private S3 with shorter retention.

**Cost at scale**: Claude API UX analysis — 1–2 analyses/day ≈ $2–5/month [TRAINING_ONLY]; 10 PRs/day ≈ $20–50/month. Start manual/on-demand.

**Noise fatigue threshold**: If false positive rate exceeds ~10%, developers start ignoring output. Start with lean (high precision, lower coverage); expand as FP rate is proven <5%.

---

## Prior Art and Lessons Learned

1. **Swagger/OpenAPI Auto-Generation from Go** [TRAINING_ONLY]: Tools like swagger-go require explicit code markers (`// @Router`) to disambiguate. Pure AST without markers has high false positive rate. Annotation/marker-based discovery has lower FP rate than pure AST.

2. **Storybook Component Auto-Discovery** [TRAINING_ONLY]: Filesystem conventions are more reliable than AST analysis. HOCs and dynamic imports still require manual registration.

3. **Spring Boot Actuator** [TRAINING_ONLY]: Annotation-based discovery (`@RestController`, `@RequestMapping`) has low FP rate. Go lacks strong annotation conventions (comments not attributes) — requires marker discipline.

4. **Playwright Official Guidance**: Explicit waits, avoid `waitForTimeout`, retry for specific error types. Tests written without these patterns fail 30–50% of the time in CI.

5. **Google's Flaky Test Task Force** [TRAINING_ONLY]: 10–15% of test failures are timing-related, not logic errors. Flakiness is structural, requires deliberate mitigation.

6. **Datadog E2E Flakiness Analysis** [TRAINING_ONLY]: State leakage = 35% of flaky failures, timing = 40%, resource conflicts = 20%. Most flakiness is infrastructure/environment variance, not test design.

7. **LLM code review hallucination**: Generic prompts produce generic feedback; domain-specific prompts with project conventions are more accurate. Human review mandatory before acting on AI feedback.

8. **Cypress Docker Video** [TRAINING_ONLY]: Official guidance requires specific base image with ffmpeg. Generic CI images don't have all codecs.

9. **YouTube internal testing** [TRAINING_ONLY]: Offloads video storage to GCS with 24-hour signed URLs. Scalable approach is external storage, not CI/GitHub artifacts.

---

## Open Questions

- [ ] **AST Scanner Accuracy Target**: What FP rate is acceptable? Target <5% FP for lean, <10% for comprehensive. Blocks: scanner design decisions.
- [ ] **E2E Coverage Threshold**: Which flows are "critical" enough? Session creation, history persistence, branch switching, output streaming? Blocks: test writing scope.
- [ ] **Video Storage Policy**: GitHub artifacts (temporary, free) vs. external (permanent, cost)? Start with artifacts; move to S3 if >1 GB/month. Blocks: CI architecture.
- [ ] **UX Analysis Automation Frequency**: Every PR vs. on-demand? Start on-demand; automate if token cost <$10/month. Blocks: CI integration design.
- [ ] **Flakiness Tolerance**: >5% flakiness = refactor; >10% = remove. Blocks: CI gate configuration.
- [ ] **Registry Trust Threshold**: >10% FP rate = developers stop trusting → tool failed. Blocks: scanner calibration strategy.

---

## Recommendation

**Start lean. Expand only after proving FP rate <5% at each layer.**

A solo practitioner cannot sustain 15–25 hours/month maintenance burden. False positives compound — scanner noise → ignored registry → missed issues; flaky tests → ignored CI; AI hallucinations → ignored UX feedback. Each noisy tool makes the others less trusted.

**Phased lean implementation:**

1. **Weeks 1–4**: Backend scanner with explicit marker comments (`// +scan:api`). Excludes generated code (`.pb.go` suffix), private methods, test packages. Validate against manual review before expanding.
2. **Weeks 5–8**: Frontend scanner capturing only components in `/routes` or explicitly tagged. Cross-reference against backend registry for gap detection.
3. **Weeks 9–12**: 5–10 hand-written E2E tests on critical flows. Playwright best practices. Target <2% flakiness before declaring stable.
4. **Weeks 13–16**: Claude API UX analysis on-demand (not automated). Calibrate prompt to project context. Move to automated only after validating <10% noise rate.
5. **Weeks 17–20**: Video capture on PR request (not automatic). Debug CI codec/container issues. GitHub artifacts only initially; add S3 if needed.

**Decision gates:**
- After Phase 1: Is registry accurate enough to trust? If FP >5%, add marker requirement and re-calibrate.
- After Phase 3: Are E2E tests stable (<5% flakiness)? If not, pause Phases 4–5 and fix infrastructure.
- After Phase 4: Is UX analysis cost <$10/month and noise <10%? If not, stay manual-only.

---

## Pending Web Searches

1. `"Go AST protobuf generated code false positives handling"` — how existing tools handle generated code filtering
2. `"Playwright test flakiness GitHub Actions CI timing issues 2024"` — current best practices and reported flakiness rates
3. `"Claude vision API UX analysis hallucination rate costs 2026"` — token costs and hallucination rates for visual analysis
4. `"Playwright video recording Docker container codec ffmpeg issues"` — current codec support and workarounds in headless environments
5. `"TypeScript React HOC dynamic import static analysis false positives"` — current AST tool limitations for component detection
6. `"E2E test flakiness measurement state leakage database cleanup"` — isolation strategies and reported causes
7. `"GitHub Actions artifact size limits retention policy 2026"` — current limits and retention policies
8. `"AST-based feature scanner comparison Storybook Swagger OpenAPI"` — false positive rates across approaches
