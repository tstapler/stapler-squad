# ADR-004: UX Analysis Pipeline

Status: Accepted
Date: 2026-04-17
Deciders: Solo developer (owner)

---

## Context

The UX analysis pipeline must evaluate the Stapler Squad web UI for accessibility compliance, performance, and subjective quality — automatically, on PRs that touch UI code — without generating so much noise that the output is ignored.

The core tension: LLM-based UX analysis is capable of nuanced feedback but has a non-trivial hallucination rate. Deterministic tools (Axe, Lighthouse) are reliable but limited to compliance checks. Pure LLM = too noisy. Pure deterministic = misses real UX issues. Neither alone is sufficient.

The solo developer constraint is critical here. If the UX analysis step generates false positives on more than ~10% of runs, the developer stops reading the output. A noisy advisory is worse than no advisory — it trains the developer to ignore the channel entirely.

---

## Options Considered

### Option A: SaaS Visual Regression (Percy, Chromatic, Applitools)

Screenshot-diff-based visual regression using a hosted SaaS platform.

Pros:
- Pixel-perfect regression detection
- Established tooling with Playwright integration
- No LLM hallucinations

Cons:
- $100-500+/month subscription cost; unjustifiable for solo developer
- Requires human review of every screenshot diff (approval workflow)
- High maintenance: UI changes require screenshot baseline updates for every intentional change
- Does not provide actionable UX feedback; only detects visual changes
- Rejected on cost and maintenance grounds

### Option B: Deterministic Tools Only (Axe Core + Lighthouse)

Axe Core for WCAG 2.1 AA compliance; Lighthouse for performance and basic best practices.

Pros:
- Zero hallucinations; deterministic and reproducible
- Low false positive rate (~2-5% for Axe at WCAG 2.1 AA level)
- Free; well-maintained; Playwright integration exists
- Can block CI on violations without noise concerns

Cons:
- No subjective UX feedback (cluttered layout, confusing navigation, inconsistent spacing)
- Misses real UX issues that don't map to a compliance rule
- Lighthouse performance metrics are noisy in CI environments (network variance)

### Option C: LLM-Only (Claude Vision)

Send screenshots to Claude API on every PR; Claude produces UX analysis report.

Pros:
- Catches subjective quality issues that Axe misses
- Can reason about design consistency and pattern drift

Cons:
- High hallucination rate on generic prompts; research indicates 20-40% noise without domain context
- Cost scales with PR frequency; $5-50/month at typical solo developer cadence
- Rate limits can cause CI failures under parallel PRs
- Cannot be used as a blocking CI gate (too unreliable)
- If used as advisory and noise rate exceeds 10%, developers stop reading it

### Option D: Hybrid (Axe Core blocking + Claude vision advisory) — Chosen

Axe Core is the CI gate: it blocks PRs on WCAG 2.1 AA `critical` and `serious` violations. Lighthouse runs alongside and posts a performance score as an informational PR comment (warning only, not blocking, because Lighthouse scores are CI-environment noisy).

Claude vision runs as an advisory step on UI-touching PRs. It is not a CI gate. Its output is a PR comment with the top 3 findings (not an exhaustive list). The Claude prompt includes design system context (CSS custom properties from `globals.css`), accessibility target, and explicit instructions to skip terminal-rendering UI elements.

Pros:
- Deterministic gate (Axe) blocks real violations reliably
- Advisory layer (Claude) surfaces non-compliance quality issues without risking false gate failures
- Cost is bounded: max 2 API calls per PR, max 3 screenshots per call
- Claude step degrades gracefully: skipped if API key absent; warned if API errors
- Separating "blocking" from "advisory" matches the relative reliability of each tool

Cons:
- Two separate tools to maintain
- Claude noise still possible; requires monitoring and prompt tuning over first month
- `ANTHROPIC_API_KEY` must be configured as a GitHub Secret

---

## Decision

**Hybrid: Axe Core blocking CI gate + Claude vision advisory PR comment.**

Axe Core (via `@axe-core/playwright`) runs as part of the Playwright E2E suite. Any WCAG 2.1 AA `critical` or `serious` violation causes the test to fail, blocking the PR. Lighthouse CI runs after build and posts a performance score comment; score below 70 is a warning with a link to the Lighthouse report.

Claude vision analysis runs as a separate CI job on PRs touching `web-app/src/**`. It is non-blocking. Its output is written to `docs/qa/ux-findings-{pr-number}.md` and summarized in a PR comment (top 3 findings with confidence scores). The step is skipped silently if `ANTHROPIC_API_KEY` is not set.

---

## Claude Prompt Design

The Claude prompt must include project-specific context to reduce hallucinations. The prompt template:

```
You are evaluating the UX quality of the Stapler Squad web application.

Context:
- This is a terminal session management tool for developers. The primary users are engineers.
- The application renders terminal output inside <pre> elements using monospace fonts and ANSI colors. These areas are intentional design choices and should NOT be flagged for contrast or font accessibility.
- The design system uses these tokens: [CSS custom properties from globals.css, truncated to 500 tokens]
- Accessibility target: WCAG 2.1 AA (excluding terminal rendering areas described above)

You are reviewing screenshots of the feature: {feature-id} ({feature-description}).

Instructions:
1. Identify the top 3 UX issues only. Do not list more than 3.
2. For each issue: state what it is, where it is, and a specific actionable fix.
3. Do not flag terminal output areas (pre elements with monospace content).
4. Assign a confidence score (high/medium/low) to each finding.
5. Format as: Finding 1: [confidence: high] [issue] [location] [fix]
```

Monitoring: after first 10 analyses, manually review all findings. If more than 1 in 10 findings required no action (false positive), revise the prompt or narrow the screenshot scope before continuing automated analysis.

---

## Cost Bounds

- Max screenshots per PR: 3 (one per critical flow affected by the PR)
- Max Claude API calls per PR: 2
- Estimated cost per PR: $0.05-0.15 at Sonnet 4.6 pricing
- Monthly budget estimate at 20 PRs/month: $1-3 (well within $10 target)
- If monthly cost exceeds $10 (anomaly), post a Slack/notification alert and pause automation pending review

---

## Consequences

- Axe violations blocking CI requires `data-testid` attributes or ARIA landmarks on all testable UI elements — this is a good practice that Story 4 also requires
- Claude analysis is advisory; developers MUST be trained to treat it as "suggestions to consider" not "violations to fix"
- The `ANTHROPIC_API_KEY` secret enables Claude analysis; its absence is not an error
- Lighthouse performance scores in CI will vary by 5-10 points run-to-run due to network variance; the 70-point threshold is a floor, not a target

---

## References

- `project_plans/qa-engineering-tooling/research/findings-architecture.md` — Options 6A through 6D
- `project_plans/qa-engineering-tooling/research/findings-features.md` — Options 4A through 4C
- `project_plans/qa-engineering-tooling/research/findings-pitfalls.md` — LLM hallucination analysis and cost estimates
- `web-app/src/app/globals.css` — Design token definitions for prompt context
