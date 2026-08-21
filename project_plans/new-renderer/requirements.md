# Requirements: new-renderer

**Date**: 2026-06-24
**Type**: bug fix (with research-first constraint)

## Problem Statement

The Claude Code terminal renderer has changed (new renderer introduced), and as a
result terminal/streaming/rendering is broken. Escape codes that xterm.js depends on
are being accidentally stripped or corrupted somewhere in the display pipeline, causing
xterm.js to choke. There is currently no visibility into which stage of the pipeline
is mangling the sequences, making diagnosis impossible without analytics.

The pipeline under suspicion:
```
[Claude Code output] → [PTY / pty reader] → [Go transport / ConnectRPC] → [xterm.js]
```

A prior project (`terminal-analytics`) designed a comprehensive escape code analytics
system for this pipeline. The new renderer may have introduced a new stripping point
that wasn't accounted for, or changed how bytes flow through an existing stage.

## Users / Consumers

- **End users**: Human operators running AI sessions in stapler-squad who see garbled,
  broken, or incorrectly-rendered terminal output in the browser.
- **xterm.js**: The browser-side terminal renderer that receives the streamed bytes;
  it is the visible failure point when escape codes are missing or corrupted.

## Success Metrics

- Terminal renders correctly in the browser after identifying and fixing the escape code
  stripping culprit introduced by the new renderer.
- Root cause is confirmed: a specific code path in the new renderer is identified as
  the stripping point (not just suspected).
- Regression is prevented: either the fix includes a test, or the escape code analytics
  system (from terminal-analytics) is activated to give permanent visibility.

## Constraints

- **Research-first**: The new Claude Code renderer changes must be understood before
  any fix is attempted. Do not patch blindly.
- The new renderer is an external dependency (Claude Code itself); we can only control
  how stapler-squad consumes its output.
- Existing `stripANSI` in `server/mcp/ansi.go` is intentional and must not be removed
  (it strips for MCP tool output, not display path).

## Scope

### In Scope

1. **Research the new Claude Code renderer**: What changed in the renderer? How does it
   stream terminal output differently? Does it emit, buffer, or transform escape sequences
   differently than before?
2. **Identify the stripping point**: Which stage in the pipeline (PTY reader, Go transport,
   ConnectRPC serialization, new renderer integration layer) is stripping/corrupting
   escape codes?
3. **Fix the stripping**: Once identified, patch the specific code path responsible.
4. **Wire up escape code analytics** (from `project_plans/terminal-analytics/`): If the
   analytics system is already implemented, activate it to validate the fix and provide
   ongoing visibility. If not, implement the minimum viable analytics to confirm the fix.
5. **Regression test**: Add a test that will catch re-introduction of the stripping.

### Out of Scope

- No explicit exclusions — once the root cause is identified, full latitude to fix it
  wherever it lives.
- (But note constraint: do not remove or alter the MCP-path `stripANSI` — that is
  correct behavior.)

## Open Questions

1. What specifically changed in the new Claude Code renderer? Does it use a different
   output format, a different streaming protocol, or different buffering?
2. Is the stripping happening at the Go side (transport/serialization), or has the new
   renderer started emitting output in a form that the existing pipeline wasn't built for?
3. Is the existing `terminal-analytics` escape code analytics system already implemented
   on this branch, or is it still in planning? (Check `session/` and `pkg/analytics/`.)
4. Are there any new intermediate layers introduced by the new renderer between Claude
   Code's raw output and the PTY/circular buffer?
5. What does "xterm.js choking" manifest as in practice? Garbled colors, broken cursor,
   alternate-screen failures, or something else?
