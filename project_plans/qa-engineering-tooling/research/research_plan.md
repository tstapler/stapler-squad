# Research Plan: QA Engineering Tooling

Created: 2026-04-16
Input: project_plans/qa-engineering-tooling/requirements.md

## Subtopics and Agents

| Subtopic | Output file | Search cap | Key axes |
|----------|-------------|------------|----------|
| Stack | findings-stack.md | 5 searches | Language-native vs external tool, Claude API integration patterns, CI compatibility, maintenance burden |
| Features | findings-features.md | 5 searches | Completeness of prior art, license, build-time vs runtime discovery, integration effort |
| Architecture | findings-architecture.md | 4 searches | Scanner→registry→harness data flow, CI integration, registry schema design, extensibility |
| Pitfalls | findings-pitfalls.md | 4 searches | AST false positives, flaky E2E, AI hallucination in UX analysis, headless video capture |

## Decision Context

We are building 5 independent but related tools:
1. Backend feature scanner (Go, ConnectRPC/protobuf AST)
2. Frontend feature scanner (TypeScript/React AST)
3. E2E test harness (Playwright extension)
4. UX analysis automation (Claude API + Playwright screenshots)
5. Feature flow video capture (Playwright video + CI PR attachment)

Existing: Playwright already in repo. Stack: Go backend + TypeScript/React frontend.
Intelligence layer: Claude API.

## Scope Constraints

Each agent uses training knowledge + marks uncertain claims [TRAINING_ONLY - verify].
Each agent appends ## Pending Web Searches with exact queries.
Parent runs all searches in a single pass after all agents complete.

## Synthesis

After agents complete: produce research/synthesis.md in ADR-Ready format.
Synthesis feeds directly into /plan:feature for plan.md.
