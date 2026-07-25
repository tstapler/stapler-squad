# ADR-003: Normalize Tmux Output Inside `Instance.Preview()`, Not Via a New Interface

**Status**: Accepted (Context/Decision corrected 2026-07-01 after independent adversarial review — see Amendment)
**Date**: 2026-07-01
**Deciders**: Tyler Stapler

## Context

The temporal-coupling audit found `server/services/approval_handler.go` co-changes with `session/tmux/tmux.go` (5 shared commits) despite no static import. Direct code tracing (`research/architecture.md` §5) found the actual mechanism: `approval_handler.go` calls `Instance.Preview()`, which falls back to raw `i.pm().CapturePaneContent()` tmux output with no normalization, while `session/detection` already has a tested `PTYNormalizer` (ANSI-stripping, CR-collapse) that `approval_handler.go` doesn't use.

`pkg/classifier/classifier.go`'s weaker co-change signal (3 shared commits) could not be traced to any actual tmux dependency by direct code reading — treated as unconfirmed per `requirements.md` R2.3, no action taken on it in this ADR.

**Amendment (post-adversarial-review correction)**: The original Context above asserted that `Preview()`'s primary branch (`ctrl.GetRecentOutput()`, the in-memory PTY buffer) is "already normalized ... via `ClaudeController`'s own processing." An independent adversarial review traced this claim directly and found it **false**: `ClaudeController.GetRecentOutput` (`session/claude_controller.go:774`) reads straight from `PTYAccess.GetBuffer()`/`GetRecent()` (`session/pty_access.go`), which is filled by `session/response_stream.go`'s `streamLoop` at the point it copies `readBuf[:n]` into `chunk.Data` and writes that unmodified `chunk.Data` into the circular buffer (`response_stream.go:259-284`) — raw PTY bytes, ANSI codes included, with the escape-code parser call explicitly commented `"passthrough - doesn't modify data"` (`response_stream.go:277`). **Both branches of `Preview()` return raw, unnormalized tmux/PTY bytes today; neither is "already normalized."** This matters because `approval_handler.go`'s autonomous-mode approval path (`approval_handler.go:318-322`, gated on `h.autonomousChecker(sessionID)` returning true) only runs for a live, controller-managed session — i.e. exactly the case where the *primary* branch (`ctrl.GetRecentOutput()`) is taken, not the `CapturePaneContent()` fallback. A fix scoped only to the fallback branch would not normalize the content this call site actually receives.

## Decision

Fix the normalization gap at its source — inside `Instance.Preview()` itself, downstream of both branches, at its single return point — rather than introducing a new Go interface at the `approval_handler.go` call site, and rather than normalizing only the `CapturePaneContent()` fallback branch. `Preview()` already returns `(string, error)`, which is already a reasonable abstraction boundary; the missing piece is that **neither** of its branches applies any normalization before returning. Restructure `Preview()` so both the `ctrl.GetRecentOutput()` branch and the `i.pm().CapturePaneContent()` branch feed into one `content string` local, then route that single value through `session/detection`'s existing `PTYNormalizer{}.Normalize(...)` before returning — one normalization call site covering both branches, not two separate ones.

This was chosen over adding a new `PaneReader`/`CommandOutputSource` interface (the audit's originally-suggested fix) because the interface already exists (`Preview() (string, error)`) and doesn't need duplicating — the actual defect was inconsistent (in fact, entirely absent) normalization behind that interface, not the interface's absence. Fixing it at `Preview()`'s single exit point benefits every current and future caller of `Preview()`, not just `approval_handler.go`, and does not require the caller to know which internal branch was taken.

**Why not normalize at `response_stream.go`'s write point instead** (the other option this review surfaced): the circular buffer `response_stream.go` writes into is also read by real-time terminal display and scrollback-resync paths (`server/services/connectrpc_websocket.go`'s `CapturePaneContentRaw()`/`scrollbackManager`/direct `broadcast()` consumers, and `ClaudeController`'s own status-detection tail read at `claude_controller.go:619,877`) that need the raw ANSI stream intact for correct terminal rendering and status-detection regexes. Stripping ANSI at the buffer-write point would corrupt those unrelated consumers. Verified: none of `Preview()`'s actual callers (`approval_handler.go`, `session/autonomous_driver.go`, `session/session_driver.go`, `session/review_queue_poller.go`, `server/services/session_service.go`'s `GetTerminalSnapshot`, `testutil/session.go`) need raw ANSI — all of them do line-oriented text classification or line-trimming on the result. Normalizing inside `Preview()` itself is therefore the narrowest fix that actually covers `approval_handler.go`'s real call path without affecting the separate raw-byte terminal-streaming path.

## Consequences

### Positive
- Fixes the coupling for all consumers of `Preview()` at once — both branches, including the one `approval_handler.go`'s autonomous-mode path actually exercises — using already-tested normalization code (`normalizer_test.go`) rather than writing new logic.
- No new interface, no new type — smallest diff that closes the actual gap in both branches.
- Does not touch the raw-ANSI-preserving terminal-display/scrollback path (`connectrpc_websocket.go`, `response_stream.go`'s `broadcast`), which has a legitimate need for unmodified bytes.

### Negative / Accepted tradeoffs
- Changes `Preview()`'s output for any caller relying on raw (non-normalized) tmux/PTY bytes in *either* branch — verify no such caller exists before merging (grep all `Preview()` call sites, not just `approval_handler.go`; see Task 2.1a), since this ADR's fix now has a wider blast radius (both branches) than a fallback-only change would have had.
- Does not address `classifier.go`'s reported co-change, since no causal dependency was found — if a future investigation finds one, it needs its own decision, not an extension of this one.
