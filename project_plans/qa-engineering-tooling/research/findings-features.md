# Findings: Features (Comparable Tools & Prior Art)

## Summary

The QA engineering tooling project needs to deliver five integrated capabilities: backend feature discovery, frontend component cataloging, E2E test harness, automated UX analysis, and video capture/PR attachment. The feature landscape splits into two distinct categories:

1. Code discovery & documentation (API doc generators, AST scanners, registry tools) - mature market with strong open-source options
2. Testing & quality automation (Playwright reporters, Allure, automated visual review) - rapid innovation with both SaaS and open-source solutions

Key finding: No single tool ecosystem covers all five requirements. The market provides excellent point solutions but minimal integration between discovery tools (what features exist) and test harnesses (coverage validation). This creates an opportunity for Stapler Squad to own the cross-cutting orchestration layer.

For a Go + TypeScript/React + ConnectRPC stack, the recommended approach is:
- Use existing open-source scanners for API discovery (buf.build CLI, protoc-gen-doc, or custom AST reflection)
- Build custom frontend scanner leveraging TypeScript AST (babel/typescript parser) - no off-shelf solution captures React feature flows well
- Extend existing Playwright setup (already present) with structured test reporting
- Integrate Claude API for UX analysis (avoid SaaS visual review tools due to cost and lack of domain-specific rules)
- Adopt native Playwright video recording with custom GitHub Actions integration for PR attachment

---

## Options Surveyed

### 1. Backend Feature Discovery & API Documentation

#### Option 1A: buf.build CLI + Protobuf Reflection
[TRAINING_ONLY - verify] buf.build is the modern package management layer for Protocol Buffers, with native tooling for code generation and documentation.

- Capabilities: Scans .proto files, generates OpenAPI/gRPC-JSON Gateway docs, validates against breaking changes
- Output formats: JSON schema, OpenAPI 3.0, or custom plugins via protoc
- Integration effort: Low - direct integration with existing proto/ directory
- Maintenance: Minimal; buf ecosystem is actively maintained by Buf Technologies
- ConnectRPC support: Full support; ConnectRPC is protobuf-native
- Trade-offs:
  - Focuses on proto definitions, not runtime handlers (doesn't capture middleware, custom routing)
  - Missing implementation-specific details (auth requirements, rate limits)
  - Requires handlers to be registered in proto definitions (can miss ad-hoc HTTP routes)

#### Option 1B: grpc-gateway + Swagger Generation
[TRAINING_ONLY - verify] grpc-gateway is an older but stable ecosystem for exposing gRPC services via JSON/HTTP.

- Capabilities: Generates OpenAPI/Swagger from proto files, creates reverse-proxy between HTTP and gRPC
- Output formats: OpenAPI 2.0/3.0, human-readable HTML docs
- Integration effort: Medium - requires proto annotations (google.api.http) on every RPC
- Maintenance: Actively maintained but slower release cycles than buf
- ConnectRPC support: Limited - not native; requires translation layer
- Trade-offs:
  - Heavy weight; brings HTTP reverse-proxy functionality not needed for internal tooling
  - Requires invasive proto annotations
  - Better for external API documentation than internal registry

#### Option 1C: Custom Go Reflection + AST Scanner
Manual implementation approach using Go's built-in reflection on ConnectRPC handler registration.

- Capabilities: Scan registered handlers at runtime, extract method signatures, optional static AST analysis of handler code
- Output formats: JSON registry, JSONL event stream, or live HTTP endpoint
- Integration effort: Medium - requires understanding of ConnectRPC handler registration pattern
- Maintenance: High - scanner logic must adapt to codebase structure changes
- ConnectRPC support: Native - scanners run directly against server codebase
- Trade-offs:
  - Captures runtime reality (only documents what's actually registered)
  - Can extract implementation details (middleware usage, error types, response formats)
  - Requires code changes if handler registration pattern changes
  - No off-shelf tooling; learning curve for new contributors

#### Option 1D: Spectacle, Restish, or Standalone Swagger/OpenAPI Tools
Third-party API documentation and discovery platforms.

- Capabilities: Generate docs from running API endpoint, support multiple formats
- Integration effort: Low - point at a running server and extract metadata
- Maintenance: Minimal
- ConnectRPC support: Limited; designed for REST APIs
- Trade-offs:
  - Requires a running server to scan
  - ConnectRPC isn't REST-friendly (uses Connect protocol)
  - Better suited for REST APIs than RPC protocols

**Recommendation (Backend):** buf.build CLI + custom Go reflection scanner. Use buf for proto-level documentation (completeness, validation), supplement with custom Go scanner that walks ConnectRPC handler registry to capture runtime state.

---

### 2. Frontend Feature Discovery & Component Cataloging

#### Option 2A: Storybook 8+ (React)
[TRAINING_ONLY - verify] Storybook is the industry-standard component documentation tool for React/Vue/Angular projects.

- Capabilities: Isolated component rendering, auto-generated props docs from TypeScript types, visual regression testing integration, interaction testing via play() functions, built-in a11y auditing (Axe integration)
- Output formats: Interactive web UI, static export, JSON manifest of components
- Integration effort: Medium - requires Storybook setup, story files for each component, TypeScript types on props
- Maintenance: Low - Storybook team provides excellent docs and update paths
- TypeScript support: First-class; infers prop types from interfaces
- Trade-offs:
  - Requires developer discipline (story files must be written and maintained)
  - Focused on component-level coverage, not feature-flow coverage
  - Large build artifact; adds 50-100MB to bundle if published
  - Requires running a separate Storybook server or static build

#### Option 2B: React Component Doc Generator (docz, Styleguidist)
Lighter-weight alternatives to Storybook focusing on documentation generation.

- Capabilities: Auto-generate docs from component source code and TypeScript types
- Output formats: Markdown, HTML site, JSON
- Integration effort: Low - minimal setup, works with existing components
- Maintenance: Low - but smaller community support
- Trade-offs:
  - No interaction testing or live preview
  - Limited visual regression testing
  - Smaller ecosystem of plugins

#### Option 2C: Custom TypeScript/Babel AST Scanner
Manual implementation using TypeScript compiler API or Babel to parse React components.

- Capabilities: Extract component names, props definitions, hooks used, parent-child relationships
- Output formats: JSON registry, feature map, dependency graph
- Integration effort: Medium-high - requires learning AST APIs
- Maintenance: High - must handle new React patterns (server components, use client, etc.)
- TypeScript support: Native
- Trade-offs:
  - Full control over what to catalog
  - Can capture custom metadata via comments or decorators
  - No visual preview or interactive testing
  - Misses runtime behavior (dynamic routing, conditional rendering)

#### Option 2D: AST-grep (via sg CLI)
[TRAINING_ONLY - verify] AST-grep is a modern AST search tool supporting multiple languages including TypeScript.

- Capabilities: Pattern-based AST queries to find components, hooks, and patterns
- Output formats: JSON, YAML, or text
- Integration effort: Low - command-line tool, no SDK integration
- Maintenance: Minimal - external tool
- TypeScript support: Full
- Trade-offs:
  - Powerful for searching (finding all usages of a component), not cataloging
  - Not designed for comprehensive feature discovery
  - Better as a search layer than a discovery layer

**Recommendation (Frontend):** Custom TypeScript AST scanner + optional Storybook. Build a scanner that walks the React component tree, extracts props definitions from TypeScript types, and identifies feature entry points (pages, hooks that expose features). The scanner produces a feature registry JSON that cross-references backend APIs.

---

### 3. E2E Test Harness & Reporting

#### Option 3A: Playwright (Existing)
Stapler Squad already has Playwright integrated. Extending it is the natural path.

- Capabilities: Declarative test syntax, multi-browser support, built-in video recording, parallel execution, full page tracing
- Reporting: HTML report with video/trace playback
- Integration effort: Low - already present
- Maintenance: Low

#### Option 3B: Playwright + Allure Reporter
Extend Playwright with Allure Framework for test reporting and analytics.

- Capabilities: Rich test report UI (test history, flakiness trends, failure categorization), test execution timeline, CI integration
- Integration effort: Low - lightweight Playwright integration (npm package)
- Maintenance: Minimal
- Cost: Free (open-source Allure)
- Trade-offs: Allure TestOps (hosted) has pricing; open-source is excellent and free

#### Option 3C: Cypress (with Cypress Cloud)
- Not recommended: replacement cost, slower headless execution, Cypress Cloud subscription required

**Recommendation (E2E Harness):** Playwright + Allure Reporter (open-source) + custom test helpers. Keep Playwright as the test runner. Layer Allure for flakiness tracking. Build a small test utility library (`tests/fixtures/`) with session creation, page object models, and common assertion helpers.

---

### 4. Automated UX Analysis

#### Option 4A: Claude API (Text + Vision)
- Capabilities: Evaluate UI consistency, accessibility heuristics, identify UX pattern drift
- Integration effort: Low - API-based
- Cost: ~$0.01-0.05 per UX review
- Trade-offs: Non-deterministic, can hallucinate; pair with deterministic checks

#### Option 4B: Axe Core + Lighthouse
- Capabilities: Comprehensive a11y scanning (WCAG 2.1 AA), performance audits (FCP, LCP, CLS)
- Integration effort: Low - npm packages, Playwright plugins available
- Cost: Free
- Trade-offs: No subjective UX feedback; focused on compliance, not quality

#### Option 4C: Percy / Chromatic / Applitools
[TRAINING_ONLY - verify] Visual regression SaaS platforms.
- Cost: $100-500+/month
- Trade-offs: Expensive for solo developer; designed for teams; overkill for personal QA

**Recommendation (UX Analysis):** Claude API (text + vision) + Axe Core + Lighthouse. Claude handles subjective quality review; Axe and Lighthouse handle objective compliance checks. Combined into a single structured report.

---

### 5. Feature Flow Video Capture & PR Attachment

#### Option 5A: Playwright Native Video + GitHub Actions Artifact Upload
- Playwright built-in video recording (MP4); GitHub workflow artifact storage; custom PR comment with link
- Integration effort: Low
- Cost: Free (GitHub artifact storage) to $50-100/month (S3 for long-term archival)

#### Option 5B: SaaS Recording (Loom, Wistia)
- Cost: $100+/month; over-engineered for personal use case

**Recommendation (Video Capture):** Playwright native video + GitHub Actions artifact upload + optional S3. Record on test failure; upload as artifact; post PR comment with link. Add S3 if longer retention needed.

---

## Trade-off Matrix

| Tool | Coverage | Cost | Setup Time | Maintenance | Go+TS Fit |
|------|----------|------|------------|-------------|-----------|
| buf.build CLI | Proto-level (8/10) | Free | 1 week | Low | Native |
| Custom Go scanner | Runtime (9/10) | Free | 2-3 weeks | Medium | Native |
| Storybook | Components (7/10) | Free | 2-3 weeks | Medium | Excellent |
| Custom TS AST scanner | Flows (8/10) | Free | 2 weeks | Medium | Excellent |
| Playwright (existing) | E2E flows (8/10) | Free | 0 (exists) | Low | Excellent |
| Playwright + Allure | E2E + analytics (9/10) | Free | 1 week | Minimal | Excellent |
| Claude API | Subjective UX (7/10) | ~$50/mo | 1 week | Low | Any |
| Axe + Lighthouse | Objective a11y (6/10) | Free | 3 days | Low | Any |
| Percy/Chromatic | Visual regression (8/10) | $200+/mo | 1 week | Medium | Any |
| Playwright video + GH | PR attachment (7/10) | Free | 2 days | Low | Excellent |

Key observations:
- Custom scanners offer best completeness but highest maintenance burden
- Playwright baseline is already present; extending always cheaper than replacement
- UX analysis: Claude API offers best quality-to-cost ratio for solo developer
- SaaS options (Percy, Chromatic) are overkill for solo developer use case

---

## Risk and Failure Modes

### Backend Scanner Risks
- **False negatives**: Ad-hoc HTTP routing missed by proto scanner → Mitigation: combine buf + Go reflection
- **Stale documentation**: Handlers added without proto updates → CI validation cross-referencing proto vs. handler registry
- **ConnectRPC metadata lost**: buf is generic, misses interceptors → supplement with custom Go scanner

### Frontend Scanner Risks
- **React pattern detection miss**: New patterns (Server Components, Suspense) not recognized → rule-based scanner ignores implementation details, focuses on exports
- **False positives**: Internal utilities cataloged as features → explicit feature markers (comments/decorators)
- **Staleness after refactoring**: Deleted components still in registry → "last seen" timestamp with stale-entry CI flag

### E2E Test Risks
- **Flaky tests**: Timing/network/state leakage → Playwright retry logic, mock external services, Allure flakiness tracking
- **Local-pass/CI-fail**: Headless environment differences → run tests in Docker locally; match Node version in CI
- **Coverage-registry mismatch**: Scanner shows feature but no test exists → CI gate cross-referencing registry vs. test coverage

### UX Analysis Risks
- **Claude hallucinations**: Confident but wrong accessibility feedback → Axe is source of truth; Claude is advisory
- **Cost explosion**: Scanning every page on every commit → only run on PRs that touch UI files; cache results
- **High false positive rate**: Generic rules on domain-specific UI → provide rich context (design system, brand guidelines in system prompt)

### Video Capture Risks
- **Artifact size**: Full test execution videos can be 100MB+ → video compression, failures-only recording
- **Privacy exposure**: Videos capture sensitive test data → use non-production test data; sanitize fixtures
- **CI storage quota**: Videos accumulate → set 7-day retention; permanent only for main branch

---

## Migration and Adoption Cost

| Phase | Effort | Cost | Adoption Blocker | Rollback Risk |
|-------|--------|------|------------------|---------------|
| Backend scanner | 60-80h | Dev time only | None; read-only | Low |
| Frontend scanner | 40-60h | Dev time only | None; registry is advisory | Low |
| E2E harness extension | 20-30h | Dev time only | Tests must pass in CI | Medium |
| UX analysis | 20-30h | Claude API ~$50-100/mo | None; advisory initially | Low |
| Video capture | 15-20h | Optional S3 $50-100/mo | None; supplementary | Low |

**Total**: ~200-250 hours developer time over 6 weeks. Ongoing SaaS: ~$100-200/month.

---

## Operational Concerns

**Monitoring targets:**
- Backend registry: endpoint count vs. server request logs
- E2E: test pass rate (threshold <90% = alert), flakiness index, execution time (>10 min = investigate)
- Claude API: monthly token cost (>$200 = warning), analysis latency (<5 sec target)
- Artifact storage: GitHub artifact usage (>50GB = archive/cleanup)

**Critical dependencies:**
- Playwright: Update monthly; breaking changes quarterly
- Claude API SDK: Pin to specific model version; backward-compatible updates
- buf CLI: Update quarterly; vendor in repo if critical
- Node.js version: Affects TypeScript scanner performance

**Security:** Claude API key in GitHub Secrets; S3 IAM write-only scope; scanner outputs committed to version control for audit trail.

---

## Prior Art and Lessons Learned

1. **Feature registries are underutilized**: Teams maintain them for compliance but rarely use them for test coverage tracking. Stapler Squad's registry→tests cross-reference is a differentiator.

2. **E2E test flakiness is endemic**: Every team struggles. Plan for 10-15% of CI time on retries. Allure's flakiness tracking is key to managing this.

3. **LLM-based UX review is nascent** [TRAINING_ONLY]: Industry consensus (as of 2024) - pair LLM analysis with deterministic checks (Axe, Lighthouse), don't rely on LLM alone.

4. **Video attachment to PRs is rare**: Most teams record for debugging but don't attach to PRs due to storage cost and lack of native embedding. Solo-developer model makes this more viable.

5. **Scanners vs. runtime analysis**: AST scanner = what should exist; runtime scanner = what actually exists. Best results combine both. [Stripe/Netflix case studies - TRAINING_ONLY - verify]

---

## Open Questions

- [ ] **Frontend feature definition**: What qualifies as a "feature"? Page? Component? User flow? Needs explicit definition before scanner can work. Blocks: frontend scanner design.
- [ ] **E2E coverage target**: 20-30% coverage of high-value flows vs. 80%? 80% is expensive to maintain. Blocks: CI gate thresholds.
- [ ] **UX analysis as CI gate**: Should findings block CI or report-only? If report-only, will developers ignore them? Blocks: CI integration architecture.
- [ ] **Video retention policy**: 7 days? 1 month? 1 year? Blocks: S3 vs. GitHub artifacts decision.
- [ ] **Cross-feature dependencies**: Should registry track which features depend on others for impact analysis? Blocks: registry schema design.

---

## Recommendation

**Phased rollout (6 weeks):**

1. **Week 1-2**: Backend feature scanner (buf.build + custom Go reflection → `docs/backend-registry.json`)
2. **Week 2-3**: Frontend component scanner (TypeScript AST → `docs/frontend-registry.json`)
3. **Week 3-4**: E2E harness (Allure + test fixtures + 10-15 critical flow tests; CI gate: feature with no test = warn)
4. **Week 4-5**: UX analysis (Claude API + Axe + Lighthouse → structured JSON report on UI-touching PRs)
5. **Week 5-6**: Video capture (Playwright video-on-failure + GitHub artifact upload + PR comment with link)

**Success criteria:**
- Registries exist, auto-update on code changes, human-readable
- 20-30% of critical flows have E2E coverage; registry shows which features are covered
- UX analysis runs on PRs; developers review top 5 severity findings
- Failed tests include video; developers can replay without local reproduction
- Flaky test rate <5%; scanner false positives <2%

---

## Pending Web Searches

1. `"buf.build ConnectRPC support 2025"` - verify buf CLI supports ConnectRPC as of 2026
2. `"Playwright video recording CI performance overhead 2025"` - quantify overhead of native video recording
3. `"Claude API vision pricing 2026"` - confirm current image token cost
4. `"Percy vs Chromatic visual regression 2025"` - verify current pricing and feature matrix
5. `"Allure TestOps pricing 2025"` - confirm pricing for open-source vs. hosted
6. `"axe-core 2025 WCAG coverage"` - verify coverage and false positive rate
7. `"TypeScript AST scanner tools 2025"` - confirm no new mainstream alternatives
8. `"GitHub Actions artifacts storage quota 2026"` - verify retention and quota limits
9. `"React Server Components AST detection"` - confirm TypeScript parser handles use client directive
10. `"Storybook 8 React adoption 2025"` - verify current adoption and maintenance burden
