# Requirements: QA Engineering Tooling

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-16

## Problem Statement

The product (Stapler Squad) has grown complex enough that the team can no longer
confidently track which features exist, verify they're implemented correctly, or catch
regressions before they reach users. Three compounding problems:

1. **Visibility gap**: No living inventory of backend or frontend features. Hard to know
   what exists, what's tested, or what changed in a PR.
2. **QA bottleneck**: Manual verification of feature flows before shipping is slow and
   incomplete. Features ship without structured test coverage.
3. **UX drift**: UI patterns and quality degrade over time without systematic review.
   Catching UX regressions is expensive and reactive.

Primary users: the solo developer (owner) who is simultaneously dev, QA, and PM.

## Success Criteria

At 3-month horizon, success looks like:

- A living feature inventory exists for both backend (Go/ConnectRPC) and frontend
  (React) that updates automatically from source code changes.
- Critical user flows have E2E test coverage via Playwright that runs in CI and catches
  regressions before merge.
- New PRs that introduce features include auto-generated video captures of the flow,
  attached to the PR for human QA review.
- A UX analysis tool can evaluate a running build and produce actionable UX feedback
  within minutes (not hours of manual review).

## Scope

### Must Have (MoSCoW)

- **Backend feature scanner**: Automatically discovers and documents all backend
  API endpoints and ConnectRPC handlers from Go source code. Produces a structured
  feature registry.
- **Frontend feature scanner**: Automatically discovers UI features, components, and
  flows from TypeScript/React source. Cross-references against the backend registry
  to identify implementation gaps.
- **E2E test harness**: Playwright-based framework (extending existing setup) for
  writing and running end-to-end tests covering critical user flows. Integrates with CI.
- **UX analysis automation**: AI-assisted (Claude API) tool that evaluates a running
  build's UI for UX quality, accessibility, consistency, and pattern drift. Returns
  structured findings.
- **Feature flow video capture**: Playwright-powered video recording of user flows,
  automatically triggered on PRs that touch feature code. Videos attached to PR
  for QA and bug identification.

### Out of Scope

Nothing is explicitly excluded. All dimensions of QA tooling are in scope.
Load/performance testing is lower priority but not excluded.

## Constraints

**Tech stack**: Go (backend) + TypeScript/React (frontend). Scanners and harness stay
in the repo's existing stack. AI-assisted analysis uses the Claude API as the
intelligence layer.

**Existing work**: Playwright is already in use in the repo. New tooling extends and
formalizes the existing setup rather than replacing it.

**Timeline**: Not fixed. Deliverables are phase-gated; each tool is independently
shippable.

**Dependencies**: Stapler Squad Go backend (ConnectRPC/protobuf), React frontend,
existing Playwright setup, Claude API for AI analysis.

## Context

### Existing Work

- Playwright is already present in the repo - E2E harness builds on this.
- No backend feature scanner exists today.
- No frontend feature scanner exists today.
- No automated UX review tooling exists today.
- No video capture / PR attachment workflow exists today.

### Stakeholders

- Solo developer / owner: primary user of all tooling.
- Future contributors: will benefit from living documentation and test coverage.
- PR reviewers (human or AI): consumers of video captures and UX reports.

## Research Dimensions Needed

- [ ] Stack - evaluate technology options: Claude API integration patterns, AST-based
  Go/TS scanner approaches, Playwright video capture APIs, CI attachment mechanisms
- [ ] Features - survey comparable tools: existing feature registry tools, API doc
  generators (swagger/grpc-gateway), Storybook feature catalogs, Playwright test
  reporting tools, automated UX review SaaS
- [ ] Architecture - design patterns and tradeoffs: how scanner output feeds E2E
  harness, how UX analysis integrates with CI, registry data model design
- [ ] Pitfalls - known failure modes: AST scanner false positives, flaky E2E tests,
  AI UX analysis hallucinations, video capture in headless CI environments
