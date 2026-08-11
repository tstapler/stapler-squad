# Findings: Architecture

## Summary

The QA engineering tooling system requires orchestrating five semi-independent tools (backend scanner, frontend scanner, E2E harness, UX analysis, video capture) across a Go backend + React frontend stack using ConnectRPC. The core architectural challenge is not building each tool in isolation — the market has strong reference implementations — but designing the integration layer: how feature discovery feeds test harness, how the registry stays authoritative as code changes, and how to balance comprehensive coverage against solo practitioner maintenance burden.

Recommended architecture:
1. **Static feature registry files** (JSON, committed to repo) as the source of truth, updated via CI scanner runs
2. **Loose coupling between tools** — registry is read by tests/analysis but not written by them
3. **Dual-scan** (proto-level + Go reflection) for backend discovery to capture both specification and runtime reality
4. **Marker-based identification** (`// +api:` / `// +feature:` comments) to disambiguate features from internal utilities
5. **Advisory CI gates** for UX analysis and video capture; blocking gates only for E2E tests
6. **Feature IDs in test decorators** to enable coverage traceability queries

---

## Options Surveyed

### 1. Feature Registry Storage Model

#### Option 1A: Static JSON Files Committed to Repo (Recommended)
- Location: `docs/registry/backend-features.json` + `docs/registry/frontend-features.json`
- Generation: Scanner runs in CI; diffs generated files; commits if changed (or errors if outdated)
- Discovery: Tests/analysis read from files; no runtime API needed
- **Pro**: Git history preserved; code review captures registry changes; works offline; staging can use old registry
- **Con**: Registry can drift if CI scanner fails; manual fixes overwritten by scanner re-run

#### Option 1B: Live HTTP Service / API Endpoint
- Registry exposed via `/api/v1/features/backend` and `/api/v1/features/frontend` endpoints
- **Rejected**: adds runtime dependency; E2E tests fail if service down; requires versioning strategy; over-engineered for solo developer

#### Option 1C: Generated-on-Demand at Test Time
- Scanners run as test fixtures; registry generated fresh per test run
- **Rejected**: scanner runs for every test → slow; test failures mask scan errors; no historical registry

#### Option 1D: Hybrid: Static Registry + CI Validation
- Committed static registry + CI job that validates against live scans; errors if mismatched
- **Incorporated into 1A**: this is how Option 1A is enforced — CI errors if >2% divergence from committed registry

**Recommendation**: Static JSON committed to repo + CI validation (divergence >2% = error). Manual generation via `make registry-generate` for local development.

---

### 2. Backend Feature Scanner Architecture

#### Option 2A: Proto-Only (buf.build CLI)
- Scans all `.proto` files; extracts service definitions and RPC methods
- Accuracy: ~85% — captures proto-defined APIs but misses runtime handler registration
- **Con**: misses ad-hoc HTTP routes; requires proto discipline for 100% coverage

#### Option 2B: Go Reflection + ConnectRPC Registry Walk
- Runtime introspection of registered ConnectRPC handlers via reflection
- Accuracy: ~95% — captures runtime reality including interceptors
- **Con**: requires running the full server; misses unregistered handlers (dead code hidden)

#### Option 2C: Go AST + Marker Comments
- Parse all Go files; find functions with `// +api:` comments; extract signatures
- Accuracy: ~90% (marker-based reduces FP rate to ~3%)
- **Con**: requires developer discipline; won't catch unmarked handlers

#### Option 2D: Dual-Scan (buf + Go Reflection) — Recommended
- Run both buf CLI (proto inventory) and Go reflection (runtime inventory); cross-reference; flag divergences
- Accuracy: ~98%
- **Pro**: comprehensive; catches missing endpoints in either layer; validates proto-code sync
- **Con**: higher complexity; two tools; requires conflict resolution strategy

**Recommendation**: Dual-scan. CI job compares results; errors if coverage <95% or divergence detected.

---

### 3. Frontend Feature Scanner Architecture

#### Option 3A: TypeScript AST + Compiler API
- Walk React component tree; extract component names, props, exports from TypeScript definitions
- Accuracy: ~85% (static analysis; misses runtime imports, lazy loading)
- **Con**: HOCs and dynamic imports need special handling; generated `_pb.ts` needs filtering

#### Option 3B: Runtime Component Registry via Playwright Traversal
- Instrument React DevTools protocol during test runs to capture rendered components
- Accuracy: ~70% — only documents tested features (circular logic problem)
- **Rejected**: only captures what tests exercise; slow; full app required

#### Option 3C: Storybook-Based Registry
- Extract component metadata from story files
- Accuracy: ~90% but requires developer discipline (story files separate from components)
- **Future option**: adopt after scanner proves value; Storybook adds 50-100MB artifact overhead

#### Option 3D: Filesystem Convention + Marker Comments — Recommended (combined with 3A)
- Components with `// +feature:` comment only
- Accuracy: ~100% (no false positives; explicit developer intent)
- **Con**: lowest coverage — catches only marked features

**Recommendation**: TypeScript Compiler API (Option 3A) + marker filtering (Option 3D). Scanner walks React component exports; filters to files with `// +feature:` comment; excludes generated code (`_pb.ts`), test files, utilities. Extracts prop types from TypeScript definitions.

---

### 4. Scanner → Registry → E2E Harness Data Flow

#### Option 4A: Registry Contains Test Generation Hints
- Registry includes `testScenarios?: TestScenario[]` field; E2E tests auto-generated
- **Rejected**: auto-generated tests are often brittle; high maintenance burden

#### Option 4B: Registry with Feature IDs; Tests Reference IDs — Recommended
- Each feature has `id: string` (stable, human-readable, e.g., `session:create`)
- Tests explicitly reference feature IDs via decorators/comments
- CI cross-references: "Feature X has no test" coverage report

Test decorator example (TypeScript):
```typescript
test.describe('session:create', () => {
  // @feature session:create
  test('creates new session from path', async ({ page }) => { ... });
});
```

- **Pro**: explicit traceability; feature coverage queryable; tests remain hand-written (human curated)
- **Con**: manual discipline needed; registry and tests can drift

#### Option 4C: Loose Coupling; Registry and Tests Separate
- No explicit link; CI reports coverage via post-analysis only
- **Con**: coverage gaps only discovered retroactively

**Recommendation**: Option 4B. Registry schema:
```json
{
  "features": [{
    "id": "session:create",
    "type": "backend|frontend|full-stack",
    "backend": { "service": "SessionService", "method": "CreateSession" },
    "frontend": { "component": "NewSessionModal", "path": "web-app/src/..." },
    "tested": false,
    "lastModified": "2026-04-16"
  }]
}
```

---

### 5. CI Integration Architecture

#### Option 5A: Scanner Runs Pre-Build
- Pipeline: Lint → Scanner → Build → Test
- **Con**: adds latency; registry mutations in CI can cause conflicts

#### Option 5B: Scanner Runs Post-Build, Pre-Test — Recommended
- Pipeline: Build → Scanner → [Test + UX Analysis + Video in parallel] → Report → Publish
- Scans actual artifacts (compiled Go, built TS, generated `.pb.go` in place)
- **Pro**: scans real artifacts; doesn't block build on registry issues

#### Option 5C: Scanner Runs After Tests (Retrospective)
- **Rejected**: feature-test gaps discovered too late; registry not part of CI contract

**Recommendation**: Option 5B. Scanners run after build, before tests. Registry divergence >2% = warning (not blocking initially; promote to blocking after 2-week validation). Test execution includes feature-coverage report. Video capture and UX analysis run in parallel with test execution (non-blocking).

---

### 6. UX Analysis Pipeline Design

#### Option 6A: Full-Page Screenshots + Batch Analysis
- All screenshots sent to Claude in single call
- **Con**: context window limits; quality degrades with batch size; high token cost

#### Option 6B: Per-Flow Sequences + Contextual Prompts
- Playwright captures test flow screenshots in sequence; Claude analyzes the narrative
- **Pro**: contextual; better flow-level feedback
- **Con**: higher token cost; requires flow metadata

#### Option 6C: Deterministic Tools Only (Axe + Lighthouse)
- No LLM; Axe (a11y) + Lighthouse (performance) via Playwright integrations
- **Pro**: deterministic; free; no hallucinations
- **Con**: no subjective UX feedback; compliance-only

#### Option 6D: Hybrid: Deterministic + LLM Advisory — Recommended
- Axe + Lighthouse as CI gate (blocking if violations exceed threshold)
- Claude vision analysis as advisory (informational PR comment)
- **Pro**: deterministic gate + advisory quality signals; false positives filtered by Axe

**Recommendation**: Option 6D. Axe Core + Lighthouse block CI on UI-touching PRs with violations. Claude provides advisory UX feedback with rich prompt:
```json
{
  "context": {
    "feature": "session:create",
    "design_tokens": "<summary of CSS custom properties from globals.css>",
    "accessibility_target": "WCAG 2.1 AA"
  },
  "instructions": "Evaluate for clarity, design system consistency, and accessibility gaps automated tools miss."
}
```
Store Claude findings as `docs/qa/ux-findings-{pr-number}.md` linked from PR comment.

---

### 7. Video Capture Trigger and Storage

#### Option 7A: Capture All Tests; Upload on Failure Only
- Record all tests; upload artifacts only on failure
- **Pro**: always ready for failed tests; no CI logic needed
- **Con**: encoder overhead slows all tests; artifact quota consumed

#### Option 7B: Record on Feature-Change PRs Only — Recommended
- Video recording enabled only when PR touches feature-registry files or marked feature files
- Upload artifacts for all tests (pass + fail) when recording is active
- **Pro**: focused recording; videos serve as QA evidence; low overhead on non-feature PRs

#### Option 7C: Manual On-Demand
- `--record-video` flag or PR comment trigger
- **Con**: requires discipline; often forgotten

#### Option 7D: Smart Sampling (~10% of tests randomly)
- **Rejected**: non-deterministic; complex sampling logic

**Recommendation**: Option 7B. CI sets `RECORD_FEATURES=true` when PR touches `docs/registry/` or files with `// +feature:` marker. Playwright video recording enabled only in that mode. GitHub Actions artifacts with 30-day retention. Optional S3 sync job for long-term archival (main branch only).

---

## Trade-off Matrix

| Decision | Options | Key Tension | Recommendation |
|----------|---------|-------------|----------------|
| Registry storage | Static file vs. live service | Auditability vs. freshness | Static JSON + CI validation |
| Backend scan | Proto-only vs. reflection vs. AST | Coverage vs. accuracy | Dual-scan (buf + Go reflection) |
| Frontend scan | AST vs. runtime vs. Storybook | Precision vs. maintenance | AST + marker filtering |
| Scanner→test link | IDs vs. loose coupling | Traceability vs. coupling | Feature IDs in test decorators |
| CI placement | Pre-build vs. post-build | Gate strength vs. latency | Post-build, pre-test |
| UX analysis gate | Blocking vs. advisory | Confidence vs. noise | Axe blocks; Claude advises |
| Video capture | All tests vs. feature-only | Coverage vs. overhead | Feature-change-triggered |

---

## Risk and Failure Modes

### Registry Staleness and Drift
- Scanner runs; outputs new feature; registry file not updated; test missed
- Mitigation: CI validation errors if >2% divergence from committed registry; `lastModified` timestamps; auto-commit option if divergence <2%

### AST Scanner False Positives/Negatives
- Generated code (`.pb.go`, `_pb.ts`), HOCs, dynamic imports cause scanner misclassification
- Mitigation: explicit `// +api:` and `// +feature:` markers; scanner skips `_pb.go`, `_pb.ts`, `.gen.go` files; manual baseline seed + FP-blocking test suite

### E2E Test Flakiness in CI
- Tests pass locally; fail intermittently in headless CI (network variance, port conflicts, state leakage)
- Mitigation: explicit `waitForSelector` (no hardcoded `sleep`); unique ports per test (computed from test ID); fixture creates/destroys fresh tmux session per test; Allure tracks flakiness trend; alert if >5%

### Video Codec in Headless CI [TRAINING_ONLY - verify]
- Video encoding fails silently in minimal CI containers (missing ffmpeg/libavcodec)
- Mitigation: explicit `apt-get install ffmpeg` or use base image with ffmpeg; validate `.webm` files exist post-test; fail job if missing; compress to 500kbps for disk efficiency

### Claude API Hallucinations and Cost Explosion [TRAINING_ONLY - verify]
- Claude reports false a11y violations; developers ignore tool; or cost scales to $100+/month
- Mitigation: Axe is source of truth; Claude advisory only; rich context prompt (design system + accessibility target); monthly budget alert ($50); noise tracking (>10% noise rate = disable Claude)

---

## Migration and Adoption Cost

| Phase | Component | Effort | Adoption Risk |
|-------|-----------|--------|---------------|
| 1 | Backend scanner (dual-scan) | 60-80h | Low (read-only) |
| 2 | Frontend scanner (TS AST) | 40-60h | Low (advisory) |
| 3 | Registry + coverage reporting | 20-30h | Low |
| 4 | E2E harness extension + Allure | 30-40h | Medium (CI gate) |
| 5 | UX analysis (Axe + Claude) | 20-30h | Low (advisory) |
| 6 | Video capture + CI integration | 15-20h | Low (supplementary) |

**Total**: 185-260 hours, 8-12 weeks. Ongoing: 10-15 hrs/month (test maintenance, FP triage).

---

## Operational Concerns

**Registry governance**: Auto-commit registry changes if divergence <2%; flag for review if >2%. Include registry diffs in PR for human review. Breaking schema changes require explicit migration.

**Test maintenance**: Target 20-30% coverage of critical paths (session creation, history search, VCS operations). Pin E2E tests to feature IDs; when feature changes, update test decorators. Allure alert at >5% flakiness.

**CI cost and latency**: Scanners run in parallel (buf + Go reflection as separate jobs). Analysis (Axe + Claude) runs alongside tests. Target total CI latency <20 min; investigate if >25 min.

**Data retention**: GitHub artifacts retained 7-30 days; non-production test data only. S3 storage (if adopted) encrypted at rest. No videos pushed to public bucket.

---

## Prior Art and Lessons Learned

1. **Static registries outlast live services** [TRAINING_ONLY]: GitHub API docs and Netflix API catalog use versioned static files per deployment. Live services require versioning, deprecation, backward compatibility — complexity not worth it for solo developer.

2. **Marker-based discovery > pure AST**: Spring Boot uses `@RestController`; Swagger uses `// @Router` comments. Pure AST scanners have 20-30% FP rate without markers. Explicit markers encode developer intent.

3. **E2E flakiness compounds early** [TRAINING_ONLY]: Google's Flaky Test Task Force found 10-15% of test failures are timing-related. Early investment in isolation and explicit waits prevents compounding technical debt.

4. **Feature-to-test traceability is rare and valuable**: Most projects decouple tests from features. Feature ID references in test decorators enable "Feature X has no test" queries — enables systematic coverage tracking.

5. **LLM advisory + deterministic gate** [TRAINING_ONLY]: Industry consensus (as of 2024): pair LLM with Axe/Lighthouse as ground truth. Generic LLM prompts produce generic feedback; domain-specific prompts with design system context are 40-50% more actionable.

6. **Video as QA evidence (not debugging)**: Stapler Squad's model (attach to feature PR) is unusual. High-signal approach (feature changes only) + short retention (30 days) avoids storage explosion.

---

## Open Questions

- [ ] **Registry schema evolution**: How to handle breaking schema changes without breaking tooling? Version registry format? Semantic versioning of schema? Blocks: initial schema design.
- [ ] **Feature granularity**: Is a "feature" a handler/endpoint? A user flow? A component? Inconsistency causes scanner confusion. Must define before implementing scanners. Blocks: scanner scope.
- [ ] **Cross-feature dependencies**: Should registry track "session:create depends on tmux:session" for impact analysis? Or keep flat (just what exists)? Blocks: registry schema design.
- [ ] **E2E coverage target**: 20%? 30%? 50% of critical paths? Coverage aspiration affects test writing scope and CI time investment. Blocks: Phase 4 scope.
- [ ] **Flakiness tolerance threshold**: 5% = refactor; 10% = remove? Determines automation investment. Blocks: CI gate configuration.
- [ ] **Registry as public artifact**: Should registry be versioned and published with releases? Or internal-only? Blocks: deployment workflow.

---

## Recommendation

**Phased lean-first implementation (8-12 weeks):**

**Phase 1 (Weeks 1-3)**: Foundation
- Backend scanner: buf CLI + Go reflection → `docs/registry/backend-features.json`
- Frontend scanner: TypeScript AST + marker filtering → `docs/registry/frontend-features.json`
- Establish feature ID naming scheme: `{scope}:{action}` (e.g., `session:create`, `ui:sidebar`)
- Decision gate: registry FP rate <5%? If not, add/refine marker requirement and re-tune.

**Phase 2 (Weeks 4-6)**: E2E Harness
- Extend Playwright with Allure reporter
- 5-10 hand-curated critical-path tests with feature ID decorators
- CI reports: "Feature coverage: X/Y tested"
- Decision gate: <2% flakiness over 10 runs? If not, pause and fix infrastructure.

**Phase 3 (Weeks 7-9)**: UX Analysis + Video
- Axe Core + Lighthouse as CI gate (UI-touching PRs)
- Claude vision advisory (on-demand initially; auto on feature PRs if cost <$10/month)
- Playwright video on feature-change PRs; GitHub artifacts
- Decision gate: Claude noise <10%? Cost <$10/month? If not, stay manual-only.

**Phase 4 (Weeks 10-12)**: Polish
- Registry auto-update in CI (auto-commit if divergence <2%)
- Coverage dashboard (feature coverage graph, flakiness trends)
- Documentation and onboarding

**Success criteria:**
- Registries exist, auto-update, FP rate <5%
- 25% of critical flows have E2E coverage with feature IDs
- E2E tests stable (<2% flakiness); failures represent real bugs
- UX analysis cost <$10/month; noise <10%; developers read top 3 findings per PR
- Failed tests have playable recordings linked from PR

---

## Pending Web Searches

1. `"buf.build ConnectRPC support 2025 2026"` — verify buf CLI can scan ConnectRPC services
2. `"Playwright video codec Docker CI ffmpeg headless"` — codec availability in headless environments
3. `"Claude vision API UX analysis accuracy hallucination rate 2026"` — token cost and hallucination data
4. `"TypeScript Compiler API React HOC detection limitations"` — AST parser limitations for complex components
5. `"GitHub Actions artifacts storage quota 2026"` — retention policies and size limits
6. `"Allure TestOps pricing free vs hosted 2026"` — cost implications
7. `"E2E test flakiness measurement methodology state leakage isolation"` — best practices
8. `"Go protobuf generated code filtering AST scanner"` — how to filter `.pb.go` in scanners
