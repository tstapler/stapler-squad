# ADR-002: Upgrade xterm.js from 5.5.0 to 6.0.0

**Status**: Accepted
**Date**: 2026-04-09
**Deciders**: Tyler Stapler
**Relates to**: Terminal Jank Elimination (Story 1)

## Context

The codebase uses `@xterm/xterm ^5.5.0` and matching 5.x addon packages. Several active bugs in the terminal rendering trace back to xterm.js limitations that are addressed in the 6.0 release:

1. **No native DEC mode 2026 (synchronized output) support**: Claude Code's community has filed a request (issue #37283) for Claude to emit `\x1b[?2026h`/`\x1b[?2026l` around render cycles. xterm.js 5.x ignores these sequences; 6.0 handles them natively, eliminating render tearing when they are eventually emitted.

2. **Alt-buffer scroll bugs**: Issues #5411, #5390, #5127 in xterm.js 5.x cause incorrect scroll behavior when the terminal switches between normal and alternate screen buffers. Claude Code's TUI uses alternate screen (`\x1b[?1049h`) extensively.

3. **Improved IntersectionObserver pause/resume**: The 6.0 `RenderService` has better handling of `_isPaused` state transitions, which is critical for the terminal pool (Story 3) where hidden terminals must pause rendering efficiently.

4. **`@xterm/addon-serialize` availability**: The serialize addon is needed for potential snapshot/restore on pool eviction and for the lazy scrollback loading in Story 4. Ensuring addon version compatibility with the core library avoids runtime errors.

Two options were considered:

1. **Upgrade to 6.0**: Accept the breaking changes, get all fixes.
2. **Stay on 5.5**: Apply the ED3 filter (Story 1, Task 1.1) as a workaround and defer the upgrade.

## Decision

Upgrade all xterm.js packages to 6.0:
- `@xterm/xterm: ^6.0.0`
- `@xterm/addon-fit: ^6.0.0` (or matching minor)
- `@xterm/addon-web-links: ^6.0.0`
- `@xterm/addon-webgl: ^6.0.0`
- `@xterm/addon-search: ^6.0.0`
- `@xterm/addon-serialize: ^6.0.0` (add as new dependency)

The upgrade is done as part of Story 1 to front-load any API compatibility issues before the pool architecture (Story 3) increases the coupling surface with xterm.js internals.

## Consequences

### Positive

- Native DEC mode 2026 support prepares for future Claude Code improvements without code changes on our side.
- Alt-buffer scroll fixes (#5411, #5390, #5127) eliminate a class of scroll position bugs that affect Claude's TUI.
- Improved `RenderService` IntersectionObserver handling makes the CSS visibility pool (ADR-001) more reliable.
- `@xterm/addon-serialize` 6.x ensures version-compatible serialization for snapshot/restore workflows.

### Negative

- xterm.js 6.0 may have breaking API changes in the `Terminal` constructor options, addon loading, or event signatures. These must be identified and addressed during the upgrade task.
- The `allowProposedApi: true` option in `XtermTerminal.tsx` accesses unstable APIs that may have changed between 5.x and 6.x. Specifically, `mouseTracking` is a proposed API that may have been stabilized or renamed.
- Any internal API usage (e.g., `(terminal as any)._core?._renderService?.dimensions` in the debug logging) may break and needs verification.

### Neutral

- The ED3 filter (Task 1.1) remains necessary even with 6.0. xterm.js 6.0 adds DEC 2026 support, but Claude Code does not currently emit DEC 2026 sequences. The ED3 filter handles the current behavior; DEC 2026 support handles the future behavior.

## Patterns Applied

- **Dependency Upgrade as Enabler**: Upgrading the core library before building new architecture (pool) on top of it. This avoids building on a known-buggy foundation and discovering upgrade incompatibilities late.
- **Front-load Risk**: Performing the upgrade in Story 1 (low-risk, small scope) rather than during Story 3 (high-risk, large scope) isolates upgrade issues from architectural changes.
