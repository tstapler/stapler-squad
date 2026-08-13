# Research Plan: Stapler Squad Pain Points

**Date**: 2026-04-16
**Input**: `project_plans/stapler-squad-painpoints/requirements.md`

## Subtopics and Scope

### 1. Stack (`findings-stack.md`)
Evaluate concrete library/API choices for each must-have feature.

**Search strategy**: xterm.js lazy scrollback API docs; OTel JS SDK bundle size comparisons; combobox/autocomplete headless libraries for React; git list-branches API patterns in Go.

**Axes**: bundle size, API completeness, maintenance status, integration effort with existing stack.

**Search cap**: 5 searches max.

Key questions:
- Does xterm.js have a first-party lazy/virtual scrollback addon, or does it require custom implementation?
- OTel JS SDK (`@opentelemetry/sdk-web`) vs Sentry browser SDK vs lightweight custom — what are the bundle sizes and feature gaps?
- Best headless combobox for React with keyboard navigation baked in (Radix Combobox, Downshift, Headless UI Combobox)?
- What Go API does `git branch -r` / `git for-each-ref` expose, and how fast is it on large repos?

---

### 2. Features (`findings-features.md`)
Survey how comparable tools solve the same UX problems.

**Search strategy**: VS Code terminal virtual scrollback architecture; ttyd mobile terminal; GitHub/Linear branch autocomplete dialogs; analytics in single-user dev tools.

**Axes**: user-facing quality, implementation complexity, applicability to our stack.

**Search cap**: 5 searches max.

Key questions:
- How does VS Code's terminal handle large scrollback without loading everything upfront?
- How do ttyd / Wetty handle touch scroll on mobile web terminals?
- What patterns do VS Code/JetBrains use for branch selection autocomplete?
- How does Linear/GitHub handle inline rename/retag for list items?

---

### 3. Architecture (`findings-architecture.md`)
Design the integration points and data-flow for the top must-haves.

**Search strategy**: xterm.js ITerminalAddon interface; ConnectRPC server streaming patterns; OTel browser OTLP export; touch-action CSS for embedded scroll elements.

**Axes**: consistency with existing patterns, implementation risk, incremental shippability.

**Search cap**: 4 searches max.

Key questions:
- How should lazy scrollback work end-to-end? (trigger: scroll-to-top → RPC `GetScrollback(fromSeq, limit)` → write to xterm buffer → update scroll position)
- Should frontend OTel send to the existing Go OTLP endpoint, or does it need its own collector?
- How do we isolate touch scroll inside the xterm.js div so it doesn't conflict with page scroll (`touch-action: none` vs `overscroll-behavior`)?
- Should branch autocomplete use a new RPC (`ListBranches(repoPath)`) or extend the existing path-completion API?

---

### 4. Pitfalls (`findings-pitfalls.md`)
Known failure modes, gotchas, and risks.

**Search strategy**: xterm.js GitHub issues for virtual scrollback; OTel JS production bundle size problems; mobile Safari touch scroll in iframes/fixed elements; git branch listing performance.

**Axes**: severity, likelihood, mitigation availability.

**Search cap**: 4 searches max.

Key questions:
- What breaks when you write historical ANSI data into xterm.js out of order or after initial render?
- What are known OTel JS SDK production issues (bundle size, CORS on OTLP endpoint, sampling)?
- What are known touch scroll pitfalls for xterm.js on iOS Safari?
- What are the git performance cliffs for `for-each-ref` on repos with thousands of branches?
