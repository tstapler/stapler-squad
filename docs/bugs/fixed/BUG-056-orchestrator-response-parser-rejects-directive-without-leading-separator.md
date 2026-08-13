# BUG-056: Autonomous-Mode Orchestrator Response Parser Rejects a Directive With No Leading Separator, Wasting Turns [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-08-01)
**Discovered**: 2026-08-01, live — while checking current `ListStuckBacklogItems` state after an unrelated user-shared screenshot turned out to be stale. Two currently-stuck items showed `"max turns reached (N malformed orchestrator responses)"` (8/20 and 1/20 turns wasted).
**Fixed**: 2026-08-01 — `session/autonomous_driver.go`.
**Impact**: `AutonomousDriver`'s orchestrator LLM call (the internal call that decides what to inject into a live autonomous work session, or when to stop) routinely produces a response the exact-prefix parser cannot parse, discarding the entire response and burning that turn with zero progress — up to 40% of one item's 20-turn budget in the confirmed live case.

## Problem Description

`parseOrchestrationResponse` (`session/autonomous_driver.go`) required the LLM's response to start with the *exact literal* prefix `"DONE:"` or `"NEXT_MESSAGE:"`:

```go
resp = strings.TrimSpace(resp)
if after, ok := strings.CutPrefix(resp, "DONE:"); ok { ... }
if after, ok := strings.CutPrefix(resp, "NEXT_MESSAGE:"); ok { ... }
return "", false, "", fmt.Errorf("unrecognized orchestration response: %q", resp)
```

The system prompt instructs the model: `"Reply with exactly one of: NEXT_MESSAGE: <message> / DONE: <reason>. No other text."` — but real-world model output doesn't reliably comply. The actual malformed response captured live from `~/.stapler-squad/logs/` for item `04089969` (session `stapler-squad-phantom-input-replay-r3`, turn 20):

```
This is the final turn (20/20). The agent has made solid, verified progress: it
correctly aborted a risky merge, investigated the actual codebase [...] and
committed project_plans/phantom-keystroke-replay/requirements.md (commit
1711a7acb) reflecting that reality. It has just started the sdd:2-research
phase.DONE: Reached the 20-turn limit for this supervision session. [...]
```

The model wrote a full free-text explanation and appended `DONE:` directly onto the end of its last sentence with **no separating newline or space at all** (`"...reality.DONE: Reached..."`). `strings.CutPrefix` on the whole trimmed string only ever checks the first few characters, so this — and, per the live evidence (8 of 20 malformed on one item, 1 of 20 on another), apparently many similar responses — was rejected outright as "unrecognized," logged, and the turn wasted: no message was ever sent to the underlying session, and `turnCount` still incremented (the `for` loop's `continue` still runs the post-statement), consuming budget for zero work.

**Root cause**: the parser encoded a false assumption that the orchestrator model would comply exactly with "no other text" formatting instructions. It has no tolerance for the model's much more common behavior of reasoning in prose first and appending the directive at the end, sometimes without any separator.

## Fix Applied

Replaced the exact-prefix match with a marker search:

- `orchestrationDirectiveMarker` — a case-insensitive regex (`(?i)(DONE|NEXT_MESSAGE)\s*:`) — finds every occurrence of a directive marker anywhere in the (trimmed) response.
- `parseOrchestrationResponse` takes the **last** occurrence found (not the first): a model that echoes part of its own instructions (which literally contain both keywords) before giving its real answer must not have that echo mistaken for the actual directive — the model's authoritative answer is reliably the final one in its response.
- Everything from right after that marker to the end of the response becomes the payload, preserving a multi-line `NEXT_MESSAGE` body exactly as the old whole-string `CutPrefix` did.

This directly fixes the captured real failure (directive glued onto the end of a sentence with no separator) and additionally tolerates: markdown-wrapped responses, different casing, and a preamble line restating the instructions before the real answer — all plausible variants of the same underlying "the model doesn't format exactly as asked" problem, none of which needed separate handling once the parser searches for the marker instead of requiring it to open the string.

**Scope note on searching "anywhere" vs. requiring a line start**: an anywhere-in-text search does technically widen where a spoofed directive could appear compared to a strict prefix match, but this is a low-stakes internal orchestration decision, not a privilege boundary — a spoofed `DONE:` only ends the loop early (fail-safe, matching what already happens on any genuine completion), and a spoofed `NEXT_MESSAGE:` would require quoted content from the session's own already-visible output, which the model already has full read access to and could otherwise act on directly. The existing `<goal>`/`<session_output>` XML-tag wrapping (`buildOrchestrationPrompt`'s doc comment) is unaffected — it protects the *input* prompt from user content escaping its section, a separate concern from this response-side parse.

## Regression Tests

`session/autonomous_driver_test.go`:

- `TestParseOrchestrationResponse_DirectiveWithNoLeadingSeparator` — the exact captured live response; must parse as `DONE` with the correct reason.
- `TestParseOrchestrationResponse_PreferLastDirective_When_ModelEchoesInstructionsFirst` — a response that restates the instructions (containing both keywords) before giving its real answer; the last occurrence must win.
- `TestParseOrchestrationResponse_CaseInsensitiveDirective` — lowercase `next_message:` still parses.
- `TestParseOrchestrationResponse_PreservesMultilineNextMessage` — a multi-line `NEXT_MESSAGE` body must not be truncated by the marker-search approach.

Existing tests (`TestParseOrchestrationResponse_NextMessage`, `_Done`, `_Malformed`) verified unaffected — the happy-path exact-prefix case and the genuinely-unparseable case both still behave identically.

## Verification

```
$ go build ./...
(clean)

$ gofmt -l session/autonomous_driver.go session/autonomous_driver_test.go
(clean — no output)

$ go test ./session/ -run 'TestParseOrchestrationResponse' -v
=== RUN   TestParseOrchestrationResponse_NextMessage
--- PASS
=== RUN   TestParseOrchestrationResponse_Done
--- PASS
=== RUN   TestParseOrchestrationResponse_Malformed
--- PASS
=== RUN   TestParseOrchestrationResponse_DirectiveWithNoLeadingSeparator
--- PASS
=== RUN   TestParseOrchestrationResponse_PreferLastDirective_When_ModelEchoesInstructionsFirst
--- PASS
=== RUN   TestParseOrchestrationResponse_CaseInsensitiveDirective
--- PASS
=== RUN   TestParseOrchestrationResponse_PreservesMultilineNextMessage
--- PASS
PASS
ok  	github.com/tstapler/stapler-squad/session	1.450s

$ golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet ./session/...
0 issues.
```

```
$ go test ./session/...
ok  	github.com/tstapler/stapler-squad/session	(cached)
ok  	github.com/tstapler/stapler-squad/session/git	(cached)
ok  	github.com/tstapler/stapler-squad/session/headless	(cached)
ok  	github.com/tstapler/stapler-squad/session/memory	(cached)
ok  	github.com/tstapler/stapler-squad/session/mux	(cached)
ok  	github.com/tstapler/stapler-squad/session/namegen	(cached)
ok  	github.com/tstapler/stapler-squad/session/prompts	(cached)
ok  	github.com/tstapler/stapler-squad/session/queue	(cached)
ok  	github.com/tstapler/stapler-squad/session/scrollback	(cached)
ok  	github.com/tstapler/stapler-squad/session/search	0.017s
ok  	github.com/tstapler/stapler-squad/session/tmux	(cached)
ok  	github.com/tstapler/stapler-squad/session/tokens	(cached)
ok  	github.com/tstapler/stapler-squad/session/unfinished	2.003s
ok  	github.com/tstapler/stapler-squad/session/unfinished/gogitstore	59.394s
ok  	github.com/tstapler/stapler-squad/session/vc	(cached)
ok  	github.com/tstapler/stapler-squad/session/vnc	(cached)
ok  	github.com/tstapler/stapler-squad/session/workspace	0.062s
```

18 packages exercised, zero `FAIL` lines.

## Related

- Not directly related to BUG-053/054/055 (today's other fixes) — those were about the *backlog triage* headless call path; this is the *autonomous-mode orchestrator* call path, a completely different subsystem (`session/autonomous_driver.go` vs `server/services/backlog_service_triage.go`). Coincidentally surfaced in the same investigation session after a user asked to "make sure we have a way to work through triage failures" and shared a screenshot that turned out to be stale — the current, actually-live equivalent problem was this one.

## Reflection (Phase D — fix the class, not the instance)

**Classification**: Semantic/Intent gap. The code's assumption ("the model will comply with 'no other text'") doesn't match how LLMs actually behave under a formatting instruction when they also have substantive reasoning to express — this is a recurring category across LLM-driven systems, not specific to this codebase, but the fix (parse defensively rather than trust the instruction) is a general principle worth remembering: any code parsing structured output from an LLM call that also permits/encourages reasoning text should search for the directive rather than requiring it to be the entire response.

**Earliest achievable enforcement**: The regression tests are the earliest practical level — this is fundamentally about tolerating real-world model output variance, which isn't expressible as a type or lint rule. `TestParseOrchestrationResponse_DirectiveWithNoLeadingSeparator` in particular pins the exact failure shape to a concrete input, so a future change to this parser can't silently regress against the one shape already confirmed to happen in production.

**Recurring shape**: Not one of `docs/tasks/backlog-feature-improvement.md`'s named backlog-reconciliation shapes (this is a different subsystem) — but worth flagging as its own reusable pattern for this codebase: **"a parser for LLM-generated structured output assumes exact format compliance and has no fallback for the model reasoning first and directing last."** If another headless/orchestrator call site in this codebase (or a future one) parses a similarly rigid "reply with exactly X" contract, check whether it has the same brittleness before assuming it's fine.
