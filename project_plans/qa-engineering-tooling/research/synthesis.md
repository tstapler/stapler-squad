# Research Synthesis: QA Engineering Tooling

Created: 2026-04-16
Sources: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md

---

## Decision Required

How to build a maintainable, low-noise QA tooling system for a solo practitioner that provides feature visibility (what exists), test coverage (what's verified), and quality feedback (UX and accessibility) — without creating a maintenance burden that exceeds the value it delivers.

---

## Context

Stapler Squad is a Go + TypeScript/React web app that has outgrown informal QA. Three compounding problems: no living feature inventory, manual QA bottleneck before shipping, and UX drift over time. The solo developer is simultaneously engineer, QA, and PM. The tooling must provide signal without generating noise — if false positive rate exceeds ~10% at any layer, developers stop trusting that layer entirely.

Five tools to build:
1. Backend feature scanner (Go/ConnectRPC handler discovery)
2. Frontend feature scanner (React/TypeScript component discovery)
3. E2E test harness (Playwright extension with coverage tracking)
4. UX analysis automation (Claude API + Axe + Lighthouse)
5. Feature flow video capture (Playwright + GitHub Actions)

Existing: Playwright already in repo. Stack: Go + TypeScript/React + ConnectRPC. AI layer: Claude API.

---

## Options Considered

| Dimension | Option A | Option B | Option C |
|-----------|----------|----------|----------|
| Registry storage | Static JSON in repo | Live HTTP service | Generated per test run |
| Backend scan | Dual-scan (buf + reflection) | buf only (proto-level) | Go AST + markers |
| Frontend scan | TypeScript AST + markers | Storybook-based | Runtime via Playwright |
| E2E harness | Playwright + Allure + feature IDs | Playwright only (no IDs) | Cypress (replacement) |
| UX analysis | Hybrid (Axe + Claude advisory) | SaaS (Percy/Chromatic) | Deterministic only (Axe) |
| Video trigger | Feature-change PRs only | All PRs | On-demand only |
| Implementation pace | Lean-first, phase-gated | Comprehensive from start | Defer until team grows |

---

## Dominant Trade-off

**Coverage confidence vs. maintenance burden.**

Comprehensive tooling (full AST scan, all-PR video, automated LLM analysis) provides higher coverage but generates 20-40% false positive rate — causing alarm fatigue that makes the tools self-defeating. Lean tooling (marker-based scanners, hand-written tests, on-demand analysis) has lower coverage but higher signal-to-noise ratio, building developer trust before expanding scope.

For a solo practitioner, maintenance burden is the binding constraint. A tool that takes 15-25 hours/month to maintain is worse than no tool. A tool that takes 5 hours/month and is trusted delivers continuous value.

---

## Recommendation

**Choose: Lean-first, phase-gated implementation with explicit markers, static registry, advisory AI.**

**Because:**

1. **Markers over magic for discovery**: Require `// +api:` in Go handlers and `// +feature:` in React components before indexing them. This eliminates the 20-30% false positive rate of pure AST scanning. Static discovery is the right call — annotation-based discovery has proved reliable (Spring Boot, Swagger). Each marked item represents explicit developer intent.

2. **Static JSON registry over live service**: Committed `docs/registry/*.json` files give git history, code review visibility, offline operation, and no runtime dependency. CI validation (divergence >2% = error) catches drift. This is how GitHub, Netflix, and most mature teams handle API catalogs.

3. **Feature IDs in test decorators for traceability**: Tests reference registry feature IDs (e.g., `session:create`). CI reports "Feature coverage: 18/42 tested." This cross-reference is rare but high-value — enables systematic gap analysis without coupling scanners to test generation.

4. **Hybrid UX analysis (Axe blocking + Claude advisory)**: Axe Core + Lighthouse provide deterministic, reproducible compliance checks with ~2-5% false positive rate. Claude vision adds subjective quality review but is advisory only. This combination avoids the trap of either pure LLM (hallucinations undermine trust) or pure deterministic (misses real UX issues).

5. **Feature-triggered video capture**: Record only on PRs that touch feature-marked files. Avoids encoding overhead on every test run while still providing video evidence for feature additions. GitHub artifact storage (30-day retention) is sufficient for MVP.

**Accept these costs:**
- Lower initial coverage (marker-based = only marked features discovered; target 20-30% critical path E2E coverage, not exhaustive)
- Manual marking discipline required from developer
- No auto-generated tests; all E2E tests hand-written

**Reject these alternatives:**

- **Comprehensive tooling from start**: 20-40% false positive rate creates alarm fatigue; 15-25 hrs/month maintenance is unsustainable for solo practitioner. Must prove <5% FP rate at each layer before expanding scope.
- **SaaS visual regression (Percy, Chromatic)**: $100-500+/month subscription is unjustifiable for solo developer. Claude API + Axe delivers comparable value at ~$10-50/month with full data ownership.
- **Cypress (replacing Playwright)**: Playwright is already in repo, faster in headless CI, and has equivalent video recording. Migration cost (full rewrite of existing tests) produces zero value.
- **Live registry service**: Adds runtime dependency, versioning complexity, and operational overhead. No benefits over static JSON at this scale.

---

## Implementation Roadmap (6 phases, 8-12 weeks)

| Phase | Weeks | Deliverable | Decision Gate |
|-------|-------|-------------|---------------|
| 1 | 1-3 | Backend scanner (buf + Go reflection) → `docs/registry/backend-features.json` | FP rate <5%? If not, add markers. |
| 2 | 3-5 | Frontend scanner (TS AST + markers) → `docs/registry/frontend-features.json`; gap report | FP rate <5%? |
| 3 | 5-7 | 5-10 E2E tests with feature IDs; Allure reporter; CI coverage report | <2% flakiness over 10 runs? If not, pause and fix. |
| 4 | 7-9 | Axe + Lighthouse CI gate; Claude advisory on feature PRs | Claude noise <10%? Cost <$10/mo? If not, stay manual. |
| 5 | 9-10 | Video capture on feature-change PRs; GitHub artifacts; PR comment | Videos work in CI headless? |
| 6 | 10-12 | Registry auto-update in CI; coverage dashboard; docs | All gates passing? |

---

## Open Questions Before Committing

- [ ] **Feature granularity definition**: What qualifies as a "feature"? Handler? Flow? Component? Must agree before scanners can be scoped. Blocks: Phase 1 scanner design.
- [ ] **E2E coverage target**: 20% vs. 30% vs. 50% of critical paths? Affects Phase 3 scope significantly. Blocks: test writing scope.
- [ ] **Registry schema version strategy**: How to evolve schema without breaking tooling? Blocks: initial schema design (Phase 1).
- [ ] **buf.build + ConnectRPC compatibility** [TRAINING_ONLY - verify via web search]: Does buf CLI natively scan ConnectRPC services as of 2026? If not, dual-scan reduces to Go reflection only.
- [ ] **GitHub Actions artifact limits** [TRAINING_ONLY - verify]: Current retention default and quota limits. Affects: whether 30-day retention is feasible on free tier.
- [ ] **Claude API vision pricing** [TRAINING_ONLY - verify]: Current per-image token cost for Opus 4.6. Affects: $10/month budget estimate.

If the first three questions are unresolved, schedule a 30-minute scoping session before Phase 1 begins. The TRAINING_ONLY items can be verified with web searches and don't block planning.

---

## Sources

- `research/findings-stack.md` — technology options for each tool layer
- `research/findings-features.md` — comparable tools, prior art, phased rollout plan
- `research/findings-architecture.md` — registry schema design, CI integration, data flow
- `research/findings-pitfalls.md` — failure modes, lean-vs-comprehensive trade-off, maintenance analysis
