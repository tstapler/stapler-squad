# Implementation Plan: session-enter-not-sent

**Feature**: Live smoke-test PR #832's already-merged two-write submit fix for `write_to_session`/`steer_session`/`run_command`, then close backlog item `f7201b49-8318-43fb-a22f-0ce1926f9980` citing PR #832 and PR #837.
**Date**: 2026-09-28
**Status**: Ready for implementation
**ADRs**: None — no non-standard technology choice is being made here.

---

## Domain Glossary
N/A — complexity 1, no new domain types.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall shape of remaining work | Single plan/backlog item covering both phases, with an explicit go/no-go gate task (Task 1.3.4a) between them — smoke test must PASS before Phase 2 (closure) executes. Phase 1 and Phase 2 are nonetheless executed by two temporally *separate sessions* (see "Session boundary for closure" row): this triage session runs Phase 1 and then ends; Phase 2 is picked up later by a fresh session with no open link on the item | Step 0.5 creative pass (below); revised per adversarial-review.md Blocker 2 | (a) Skip the smoke test, close on static evidence alone. (b) Split the smoke test into a fully separate backlog item from closure. (c) Execute Phase 2 in the same still-running session/process that ran Phase 1 | (a) rejected — requirements.md and pitfalls.md §1 both cite BUG-031's own postmortem stating static/unit-test evidence structurally cannot prove the Ink-TUI paste-detector race is fixed; closing without a live check risks a false PASS. (b) rejected — appetite is "small, well under a day" (requirements.md); build-vs-buy.md calls the smoke test a "~15-minute manual check," and splitting into two backlog items adds coordination overhead disproportionate to that size, without adding rigor the in-session gate task doesn't already provide. (c) rejected — `report_duplicate`'s linked-caller path requires "work" role and this session is triage-role, so it returns `PERMISSION_DENIED`; its unclaimed-archive fallback separately refuses while any `ItemSession.EndedAt == nil`, which includes this session's own still-open link (`server/mcp/tools_backlog.go:2205-2214`, `:2420`) — a same-session Phase 2 call fails structurally, it doesn't just risk failing |
| Smoke-test transport/environment | Manual second instance (`PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server`), MCP client registered directly via `claude mcp add --transport http <name> http://localhost:62871/mcp` (Streamable HTTP). Note: `STAPLER_SQUAD_INSTANCE` isolates config/DB/session-list state only — the manual instance's tmux sessions run on the SAME shared default tmux server as the live production instance (`config.IsNamedInstance()`'s doc comment, `config/config.go:57-70`); teardown must be session-name-scoped, never socket- or server-scoped (see Epic 1.4) | research/stack.md "Manual second-instance mechanics", "Connecting a real MCP client for the smoke test"; tmux-sharing correction per adversarial-review.md Blocker 1 | (a) Reuse the live registered MCP entry pointed at `:8543`. (b) Use the `--mcp` stdio proxy path against the manual instance | (a) rejected — pitfalls.md §2 calls wrong-port MCP registration "the highest-severity mistake here": every tool call would land on the live production instance and could inject text into a real running session. (b) rejected — stack.md's stdio caveat: `main.go`'s `--mcp` proxy fast path resolves the target port from the resolved instance's *persisted* `config.json`, not a fresh `PORT` env var read at that point, so it can silently proxy to the live `:8543` instance unless the manual instance's config was already primed — the HTTP transport sidesteps this entirely by naming the port directly |
| Closure mechanism | From a FRESH session with no open `ItemSession` link on the item (see "Session boundary for closure" row), check current item status/linkage via `get_backlog_item` (Task 2.1.2a), then branch three ways: (1) unlinked (no `ItemSession` with `EndedAt == nil`) and status is one of idea/refining/ready/queued → `report_duplicate`'s unclaimed-archive path (`reportDuplicateUnclaimed`) with `duplicate_ref` = the PR #832 GitHub URL and `reason` citing PR #837; (2) a work-role `ItemSession` is linked → the review-gate path applies and BUG-032's empty-diff risk must be flagged before proceeding; (3) a triage-role `ItemSession` (e.g. this item's own Phase-1 session) is still linked with `EndedAt == nil` → do NOT call `report_duplicate` yet, wait for that session to end and re-check later | research/pitfalls.md §3; branch (3) added per adversarial-review.md Blocker 2 | Assuming the unclaimed-archive path applies unconditionally, without checking live status/linkage first; or assuming only a binary unclaimed-vs-work-role-linked branch exists | pitfalls.md §3: `reportDuplicateUnclaimed` explicitly refuses to act while any `ItemSession` — including this triage session itself — is still open on the item (`server/mcp/tools_backlog.go:2408-2426`); which path is valid can only be determined by a live `get_backlog_item` call, not assumed from this planning pass. The binary branch missed the linked-*triage*-role case, which is this item's actual, most likely case (`tools_backlog.go:2205-2214` returns `PERMISSION_DENIED` for a linked non-"work" role) |
| Session boundary for closure | Phase 2 (`report_duplicate`) is executed by a session holding no open `ItemSession` link on this item — either (a) this triage session's own link ends naturally once its Phase-1 work (and Epic 1.4 teardown) completes, and a later, fresh session with no open link performs Phase 2, or (b) explicit operator (Tyler) handoff | adversarial-review.md Blocker 2 | Executing Phase 2 from this same still-open triage-role session, as Pattern Decision row 1 originally assumed | `report_duplicate`'s linked-caller path returns `PERMISSION_DENIED` unless the linked role is "work" (this session is triage-role, `server/mcp/tools_backlog.go:2205-2214`); its unclaimed-archive fallback separately refuses whenever any `ItemSession.EndedAt == nil` (`tools_backlog.go:2420`), which includes this session's own still-open link. Both make same-session closure fail, not merely risk failing — see new Epic 2.0 |

---

## Tech Debt Disposition

None identified — no `research/architecture.md` was produced for this complexity-1 item; the tymux gap noted in requirements.md/build-vs-buy.md is an explicitly deferred, separately-tracked concern (BUG-113), not a hotspot this item's scope touches.

---

## Migration Plan
N/A — complexity 1, no schema or data changes.

## Observability Plan
N/A — complexity 1, no new service boundaries or operations added.

## Risk Control
N/A — complexity 1, no feature flag or rollout surface.

## Unresolved Questions
- [ ] Has this triage session's own `ItemSession` link on `f7201b49` fully ended (`EndedAt` set) by the time a session picks up Phase 2? — blocks Task 2.0.1b and, transitively, all of Phase 2 — owner: whichever fresh session picks up Phase 2; cannot be resolved from static planning since it depends on when/how this session's tmux process terminates. If still open, wait and re-check later rather than proceeding.
- [ ] Does item `f7201b49`'s current status/session-linkage permit the unclaimed-archive `report_duplicate` path, does the linked-work-role review-gate path apply, or is a linked triage-role `ItemSession` still open (requiring the wait in Task 2.0.1b/2.1.2b's third branch)? — blocks Story 2.1.2 — owner: the (fresh, unlinked) execution session, via `get_backlog_item` at run time (cannot be resolved from static planning; backlog-store state is not repo-resident).
- [ ] Is this session's GitHub authentication (`GITHUB_TOKEN`/`GH_TOKEN`/connected account) valid for `report_duplicate`'s GitHub-verification step? — blocks Task 2.2.1a — owner: execution session; if `ErrNotAuthenticated` is returned, leave the item as-is and note it for Tyler (the operator) rather than retrying.
- [ ] Is BUG-113 still open at execution time (it could have shipped between this planning pass and execution)? — blocks the premise of Task 2.1.1a and this item's out-of-scope framing for the tymux gap — owner: execution session, via a fresh read of `docs/bugs/open/BUG-113-tymux-backend-never-passes-target-program-launches-default-shell.md`.
- [ ] Does the live smoke test confirm the fix holds for genuinely long/multi-line input, per requirements.md's own open question? — this is the central question Phase 1 exists to answer; not resolvable ahead of execution by design.

## Dependency Visualization

```
Phase 1: Live Smoke Test
  Epic 1.1 Stand up manual instance (tmux server SHARED w/ prod; only config/DB isolated)
    1.1.1a build binary -> 1.1.1b start instance -> 1.1.1c confirm config/DB isolation -> 1.1.1d record baseline tmux session list
                                                          |
  Epic 1.2 Connect MCP client + create target session     v
    1.2.1a register client -> 1.2.1b triple-check port -> 1.2.2a create_session -> 1.2.2b confirm ready
                                                                                          |
  Epic 1.3 Exercise the three tools                                                      v
    1.3.1a prep payload -> 1.3.1b write_to_session -> 1.3.1c verify
                                                            |
    1.3.2a respect rate limit -> 1.3.2b steer_session -> 1.3.2c verify
                                                            |
    1.3.3a run_command -> 1.3.3b verify
                                                            |
    1.3.4a record go/no-go verdict  <---- (gates Phase 2) -+
                                                            v
  Epic 1.4 Teardown (session-NAME-scoped only; tmux kill-server is FORBIDDEN, with or without -L)
    1.4.1a kill process -> 1.4.1b kill only this test's named tmux session(s) -> 1.4.1c remove MCP registration -> 1.4.1d diff live tmux sessions vs. 1.1.1d baseline + confirm live instance unaffected

                          [ gate: 1.3.4a must be PASS to proceed ]
                                                            |
                                                            v
Phase 2: Closure — MUST run from a fresh session with no open ItemSession link (never the same session that ran Phase 1)
  Epic 2.0 Confirm session boundary
    2.0.1a this session ends without calling report_duplicate -> 2.0.1b (fresh session) confirm no open ItemSession link
                                                                       |
  Epic 2.1 Determine closure path                                     v
    2.1.1a re-check BUG-113 -> 2.1.2a get_backlog_item -> 2.1.2b branch on status/linkage (unclaimed / linked-work / linked-triage-still-open)
                                                                       |
  Epic 2.2 Execute closure                                            v
    2.2.1a report_duplicate -> 2.2.1b handle ErrNotAuthenticated / PERMISSION_DENIED / active-session refusal -> 2.2.1c verify mutation read-back
                                                                       |
  Epic 2.3 Clean up stale branch                                      v
    2.3.1a confirm branch unmerged -> 2.3.1b delete branch
```

---

## Phase 1: Live Smoke Test

### Epic 1.1: Stand Up a Manual Instance (Config/DB Isolated, tmux SHARED With Production)
**Goal**: A second stapler-squad instance is running on the documented manual-instance port block, with its own config/DB/session-list state isolated from the live `:8543` deployment via `STAPLER_SQUAD_INSTANCE`. Its tmux server is **not** isolated: per `config.IsNamedInstance()`'s doc comment (`config/config.go:57-70`), a named instance shares the default tmux server/socket with every other instance on the machine, including the live production one. This is a hard constraint on Epic 1.4's teardown design, not an implementation detail — see that epic for why `tmux kill-server` (with or without `-L`) must never be used.

#### Story 1.1.1: Manual instance running, with config/DB isolated from live `:8543` (tmux is shared, not isolated)
**As a** triage session verifying PR #832's fix, **I want** a second, disposable stapler-squad instance with its own config/DB/session-list state, **so that** I can call the three affected MCP tools without risking the live deployed instance's sessions or backlog *state* — while being explicit that tmux itself is a shared resource requiring session-name-scoped (not socket-scoped) care in Epic 1.4.
**Acceptance Criteria**:
- The manual instance is reachable on port 62871 and the live instance on 8543 is unaffected.
  - *Given* no manual instance is currently running, *When* `PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server &` is run, *Then* `curl -sf http://localhost:62871/` succeeds and `curl -sf http://localhost:8543/` continues to succeed independently (proving the live instance was never restarted).
- The manual instance's config/DB/session-list state is isolated under its own instance directory.
  - *Given* the manual instance started with `STAPLER_SQUAD_INSTANCE=claude-manual-test`, *When* `~/.stapler-squad/instances/claude-manual-test/config.json` is read, *Then* its `ListenAddress` is `localhost:62871`, distinct from the live instance's `~/.stapler-squad/config.json` (`localhost:8543`).
- The manual instance's tmux sessions are NOT socket-isolated from production — this is recorded explicitly, not assumed away.
  - *Given* `config.IsNamedInstance()`'s doc comment (`config/config.go:57-70`) states a named instance shares the default tmux server, *When* the manual instance creates a session in Epic 1.2, *Then* that session's tmux process runs on the SAME default/shared tmux server as the live production instance — teardown (Epic 1.4) must therefore target it by session name, never by killing the shared server.
**Files**: None (runtime-only).

##### Task 1.1.1a: Build the manual-instance binary (~3 min)
- `mkdir -p ~/.stapler-squad/manual-builds/manual-1`
- `go build -o ~/.stapler-squad/manual-builds/manual-1/stapler-squad .` (run from this worktree's repo root)
- Do **not** build to `./stapler-squad` (the live systemd unit's `ExecStart` target) or a bare `/tmp` path.
- Files: None (runtime-only).

##### Task 1.1.1b: Start the manual instance (~2 min)
- `PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server &`
- Confirm the process started: check `~/.stapler-squad/instances/claude-manual-test/logs/staplersquad.log` for a startup line, or `curl -sf http://localhost:62871/`.
- Files: None (runtime-only).

##### Task 1.1.1c: Confirm config/DB isolation from the live instance (~2 min)
- Read `~/.stapler-squad/instances/claude-manual-test/config.json`; confirm `ListenAddress` is `localhost:62871`.
- `curl -sf http://localhost:8543/` still succeeds, confirming the live instance was not restarted or disturbed.
- This confirms config/DB/session-list isolation only — it says nothing about tmux, which is shared (see Epic 1.1's goal and Task 1.1.1d).
- Files: None (runtime-only).

##### Task 1.1.1d: Record a baseline tmux session list before any smoke-test session exists (~1 min)
- `tmux list-sessions` (default/shared socket — there is no distinct manual-instance socket) — record the full list of session names now, before Epic 1.2 creates the smoke-test target session.
- This baseline is what Task 1.4.1d diffs against after teardown, to confirm the live production tmux sessions were genuinely unaffected (per Blocker 1 in the adversarial review) — a check an HTTP-port probe alone cannot make.
- Files: None (runtime-only).

### Epic 1.2: Connect an MCP Client and Create a Real Target Session
**Goal**: An MCP client is registered against the manual instance only, and a real Claude Code session exists in that instance ready to receive input.

#### Story 1.2.1: MCP client registered only against the manual instance
**As a** triage session, **I want** an MCP client pointed exclusively at `http://localhost:62871/mcp`, **so that** no tool call can accidentally reach the live `:8543` instance.
**Acceptance Criteria**:
- The registered MCP server's URL names port 62871, not 8543.
  - *Given* the manual instance is listening on port 62871, *When* `claude mcp add --transport http ssq-manual-smoke http://localhost:62871/mcp` is run, *Then* `claude mcp list` shows `ssq-manual-smoke -> http://localhost:62871/mcp`.
**Files**: None (runtime-only; mutates the local MCP client config, e.g. `~/.claude.json`).

##### Task 1.2.1a: Register the HTTP MCP client (~2 min)
- `claude mcp add --transport http ssq-manual-smoke http://localhost:62871/mcp`
- Files: None (runtime-only).

##### Task 1.2.1b: Triple-check the registered port before the first tool call (~1 min)
- `claude mcp list` — re-read the output and confirm the URL contains `62871`, not `8543`, per pitfalls.md §2's "highest-severity mistake" warning. Do not proceed to Epic 1.3 until this is confirmed.
- Files: None (runtime-only).

#### Story 1.2.2: Real Claude Code target session ready for input
**As a** triage session, **I want** a live Claude Code session running inside the manual instance, **so that** the smoke test exercises a real Ink-TUI target, not a synthetic pane.
**Acceptance Criteria**:
- `create_session` produces a running session whose pane shows Claude Code's startup UI.
  - *Given* the manual instance has no sessions yet, *When* `create_session` is called (via `ssq-manual-smoke`) with `program: "claude"` and a working directory such as `/tmp/ssq-smoke-test-repo`, *Then* the response returns `success: true` with a new `session_id`, and `read_session_output` on that `session_id` shows Claude Code's Ink-TUI banner/prompt rendered in the pane.
**Files**: None (runtime-only).

##### Task 1.2.2a: Create the target session (~3 min)
- Call `create_session` on `ssq-manual-smoke` with `program: "claude"` (confirm the exact registered program name first if `claude` is not accepted — see `server/mcp/tools_session.go`'s program registry) and a scratch working directory.
- Record the returned `session_id` for use in Epic 1.3.
- Files: None (runtime-only).

##### Task 1.2.2b: Confirm the target session is ready for input (~2 min)
- Call `read_session_output` on the `session_id` from 1.2.2a.
- *Given* the session was just created, *When* `read_session_output` is called, *Then* the pane shows Claude Code's ready prompt (not a loading spinner or blank pane) before any write attempt.
- Files: None (runtime-only).

### Epic 1.3: Exercise the Three Tools With Long/Multi-line Input
**Goal**: Each of `write_to_session`, `steer_session`, and `run_command` is called with genuinely long/multi-line content — the shape that triggers the Ink-TUI paste-detector race — and actual submission (not just tool-return success) is confirmed via `read_session_output`.

#### Story 1.3.1: `write_to_session` confirms real submission
**As a** triage session, **I want** to send a long/multi-line payload via `write_to_session` and observe it actually get processed, **so that** PR #832's fix is confirmed live, not just by static review.
**Acceptance Criteria**:
- A ~3000-byte multi-line payload (under the 4096-byte `maxInputBytes` cap) is accepted and actually submitted.
  - *Given* the target `session_id` from Task 1.2.2a is ready for input, *When* `write_to_session` is called with a 40-line, ~3000-byte payload containing embedded `\n` characters, *Then* it returns `success: true` (not `INPUT_TOO_LONG`, not `SUBMIT_NOT_CONFIRMED`, not `RATE_LIMITED`), and a subsequent `read_session_output` shows Claude Code has begun processing the content (e.g. a new response/turn appears), not the raw multi-line text still sitting unsubmitted in the input box.
**Files**: None (runtime-only).

##### Task 1.3.1a: Prepare the long/multi-line test payload (~2 min)
- Construct a ~3000-byte, 40-line string with embedded `\n` characters — well under `maxInputBytes = 4096` (`server/mcp/tools_terminal.go:19`).
- Files: None (runtime-only).

##### Task 1.3.1b: Call `write_to_session` with the payload (~3 min)
- Call `write_to_session` with `session_id` from 1.2.2a and `input` = the payload from 1.3.1a.
- *Given* the payload is ~3000 bytes, *When* `write_to_session` is called, *Then* the response is `success: true` (any of `INPUT_TOO_LONG`, `SUBMIT_NOT_CONFIRMED`, or `RATE_LIMITED` indicates the call needs investigation before continuing — see the errors' meanings in pitfalls.md §1).
- Files: None (runtime-only).

##### Task 1.3.1c: Verify actual submission via `read_session_output` (~3 min)
- Call `read_session_output` on the same `session_id`.
- *Given* `write_to_session` returned `success: true` in 1.3.1b, *When* `read_session_output` is called, *Then* the pane shows Claude Code actively processing or having processed the payload (e.g., visible progress/response text), confirming the tool's `success: true` reflected real submission, not a false positive.
- Files: None (runtime-only).

#### Story 1.3.2: `steer_session` confirms real submission
**As a** triage session, **I want** to steer the in-progress session with a second long/multi-line message, **so that** `steer_session`'s use of the same submit path is confirmed independently of `write_to_session`.
**Acceptance Criteria**:
- A second long/multi-line payload sent via `steer_session` while Claude Code is mid-task is actually incorporated.
  - *Given* Claude Code is actively working from Task 1.3.1c's submission, and at least 1 second has elapsed since the last `write_to_session` call on this `session_id` (per the 1-call/sec rate limit at `server/mcp/tools_terminal.go:49`), *When* `steer_session` is called with a new ~3000-byte multi-line steering message, *Then* it returns `success: true` (not `SUBMIT_NOT_CONFIRMED` or `RATE_LIMITED`), and `read_session_output` shows the steering message was incorporated, not left as unsent pasted text.
**Files**: None (runtime-only).

##### Task 1.3.2a: Wait out the per-session rate limit before calling `steer_session` (~2 min)
- *Given* `write_to_session` was called on this `session_id` in Task 1.3.1b, *When* less than 1 second has elapsed, *Then* wait until at least 1 second has passed before calling `steer_session`, to avoid a `RATE_LIMITED` response being misread as a submit-path failure.
- Files: None (runtime-only).

##### Task 1.3.2b: Call `steer_session` with a second long/multi-line payload (~3 min)
- Call `steer_session` with `session_id` from 1.2.2a and a fresh ~3000-byte multi-line message.
- *Given* Claude Code is mid-task, *When* `steer_session` is called, *Then* the response is `success: true`.
- Files: None (runtime-only).

##### Task 1.3.2c: Verify `steer_session`'s submission landed (~3 min)
- Call `read_session_output`; confirm the steering content was actually incorporated into Claude Code's behavior/response, not sitting unsent.
- Files: None (runtime-only).

#### Story 1.3.3: `run_command` confirms real submission
**As a** triage session, **I want** to run a long/multi-line command via `run_command` and observe its actual output, **so that** `run_command`'s use of the same submit path is confirmed independently.
**Acceptance Criteria**:
- A long/multi-line command submitted via `run_command` actually executes and its output is observable.
  - *Given* the target `session_id` is available, *When* `run_command` is called with a multi-line command string (e.g. a shell heredoc or multi-statement script under whatever cap `run_command` enforces — confirmed separately per Task 1.3.3a), *Then* it returns `success: true` (not `SUBMIT_NOT_CONFIRMED`), and `read_session_output` shows the command's actual output (e.g. a distinguishing marker string echoed back), not just the command text sitting unsubmitted.
**Files**: None (runtime-only).

##### Task 1.3.3a: Confirm `run_command`'s own input-size/rate-limit caps before reusing the 1.3.1a payload shape (~2 min)
- Per pitfalls.md §1: `steer_session`/`run_command` may have their own separate byte caps distinct from `write_to_session`'s `maxInputBytes = 4096` — check `server/mcp/tools_terminal.go`'s `runCommand` validation before assuming the same payload size is safe.
- Files: `server/mcp/tools_terminal.go` (read-only reference, no edits).

##### Task 1.3.3b: Call `run_command` with a long/multi-line command and verify via `read_session_output` (~4 min)
- Call `run_command` with `session_id` from 1.2.2a and a multi-line command containing a unique marker string (e.g. `echo "SMOKE_TEST_MARKER_<timestamp>"`).
- *Given* the command was submitted, *When* `read_session_output` is called afterward, *Then* the marker string appears in the pane output, confirming Enter registered and the command actually ran (not just `success: true` with no observable effect).
- Files: None (runtime-only).

#### Story 1.3.4: Go/no-go gate recorded before Phase 2 proceeds
**As a** triage session, **I want** an explicit pass/fail verdict recorded after Epic 1.3, **so that** closure (Phase 2) only proceeds if the live fix is actually confirmed.
**Acceptance Criteria**:
- A clear PASS or FAIL verdict is recorded before any closure action is taken.
  - *Given* Stories 1.3.1-1.3.3 have each been exercised, *When* all three showed `success: true` with `read_session_output`-confirmed real submission and no unresolved `SUBMIT_NOT_CONFIRMED`, *Then* the verdict is PASS and Phase 2 may proceed; *if* any tool showed a swallowed submission (input sat unprocessed) or an unresolved `SUBMIT_NOT_CONFIRMED` after the built-in retry, *then* the verdict is FAIL, Phase 2 is skipped, and a new, narrowly-scoped follow-up bug is filed instead (fixing it is out of this item's appetite). The follow-up is filed as (1) a new backlog item via the `create_backlog_item` MCP tool, titled with the failing tool name and error code, and (2) a `docs/bugs/open/BUG-<next>-<slug>.md` file matching the existing BUG-NNN convention, citing the tool call, its `read_session_output`/`tmux capture-pane` evidence, and PR #832 as the prior fix.

**Recorded verdict (2026-09-28): PASS for the Enter-submission fix; separate read-side gap found.** On a manual instance (port 62875, `STAPLER_SQUAD_INSTANCE=claude-manual-test`; 62871 was occupied by another session's instance) against a real Claude Code target: `write_to_session` (3030 B, 42 lines) returned `success:true` and Claude replied `WRITE_MARKER_7431`; `steer_session` (3071 chars) returned `method:"send_keys"` and Claude replied `STEER_MARKER_5582`; `run_command` (multi-line) submitted and Claude replied `RUN_MARKER_9917`. Submission was confirmed from the pane via `tmux capture-pane`, because `read_session_output` and `run_command`'s read half returned `SESSION_NOT_READY` (scrollback sequence stayed 0 with no terminal stream subscriber, `server/mcp/tools_terminal.go:187-194`). That read-side behavior is a distinct defect and is NOT fixed by PR #832.
**Files**: None (runtime-only).

##### Task 1.3.4a: Record the smoke-test verdict (~2 min)
- Summarize each of the three tool calls' outcome (success/failure, any error codes seen) and state PASS or FAIL explicitly before moving to Epic 1.4/Phase 2.
- Files: None (runtime-only; this verdict is carried forward into the closure step's `reason` field in Task 2.2.1a, not written to a repo file).

### Epic 1.4: Clean, Session-Name-Scoped Teardown (Never `tmux kill-server`)
**Goal**: The manual instance's process, its own smoke-test tmux session(s) (torn down by name, not by killing the shared server), and the MCP client registration are all removed; the live `:8543` instance's tmux sessions are confirmed byte-for-byte unaffected (same names, same count) before vs. after teardown — not just that its HTTP port keeps responding.

**Hard constraint** (per adversarial-review.md Blocker 1 and Epic 1.1's goal): the manual instance does **not** have its own tmux socket — `STAPLER_SQUAD_INSTANCE` isolates config/DB/session-list state only. `tmux kill-server`, with or without `-L <anything>`, is **forbidden** anywhere in this teardown: there is no isolated socket to safely target, so any such call kills the shared default tmux server and every real session on it, including live production ones (this is a confirmed prior-incident failure mode, not a theoretical one).

#### Story 1.4.1: No stale processes or registrations, and zero impact on the live instance's tmux sessions
**As a** triage session, **I want** every artifact of the manual instance removed after the smoke test, without ever touching the shared tmux server itself, **so that** nothing is left behind to confuse a future session, and the live production instance's tmux sessions are provably untouched.
**Acceptance Criteria**:
- No manual-instance process, named smoke-test tmux session(s), or MCP registration survives the smoke test.
  - *Given* the manual instance was started in Task 1.1.1b and registered as `ssq-manual-smoke` in Task 1.2.1a, *When* teardown completes, *Then* `curl -sf http://localhost:62871/` fails (connection refused), the specific tmux session name(s) created for this smoke test (recorded from `create_session`'s response in Task 1.2.2a) no longer appear in `tmux list-sessions` (default/shared socket — never `tmux -L <socket>`, since no such distinct socket exists), and `claude mcp list` no longer shows `ssq-manual-smoke`.
- The live `:8543` instance's tmux sessions are provably unchanged, not just its HTTP port.
  - *Given* Task 1.1.1d recorded a baseline tmux session list before Epic 1.2 created any smoke-test session, *When* teardown (Task 1.4.1d) completes and `tmux list-sessions` is run again, *Then* the resulting list is identical in names and count to the baseline — confirming the shared tmux server, and every session on it, was genuinely unaffected.
- The live `:8543` instance shows no trace of the smoke test at the application level too.
  - *Given* teardown is complete, *When* the live instance's session list is checked, *Then* it contains no sessions created during Epic 1.2/1.3 (all smoke-test sessions were created in the isolated `claude-manual-test` instance, per Story 1.1.1's config/DB isolation).
**Files**: None (runtime-only).

##### Task 1.4.1a: Kill the manual instance process (~1 min)
- `kill %1` (or the specific PID of the manual instance's `stapler-squad` process if not running as the shell's job 1).
- Files: None (runtime-only).

##### Task 1.4.1b: Kill only this smoke test's own tmux session(s), by name (~2 min)
- `--tmux-keep-server` was passed in Task 1.1.1b, so tmux sessions survive `kill %1`.
- Identify the exact tmux session name(s) created for the smoke test — recorded from `create_session`'s response / session state in Task 1.2.2a. This is a session *name*, not a socket name: no isolated manual-instance socket exists.
- For each such session name, run `tmux kill-session -t <session-name>` — scoped to exactly that session, on the default/shared socket.
- Do **not** run `tmux kill-server` or `tmux -L <anything> kill-server` under any circumstances — the default socket is shared with the live production instance, and killing it kills every real session on it, not just this test's.
- *Given* the smoke-test session name(s) are killed individually, *When* `tmux list-sessions` is checked, *Then* only those specific names are gone; every other pre-existing session (including production ones) remains listed.
- Files: None (runtime-only).

##### Task 1.4.1c: Remove the MCP client registration (~1 min)
- `claude mcp remove ssq-manual-smoke`
- *Given* `ssq-manual-smoke` was registered in Task 1.2.1a, *When* `claude mcp remove ssq-manual-smoke` is run and `claude mcp list` is checked, *Then* it no longer appears.
- Files: None (runtime-only).

##### Task 1.4.1d: Diff live tmux sessions against the 1.1.1d baseline and confirm the live instance is unaffected (~3 min)
- `curl -sf http://localhost:8543/` succeeds; the live instance's own session list (via its API, not tmux) shows no new sessions from this smoke test.
- `tmux list-sessions` (default/shared socket) is compared against the baseline recorded in Task 1.1.1d: same session names, same count. This is the authoritative check that the shared tmux server — and every session on it, including live production ones — was genuinely unaffected; an HTTP-port check alone cannot establish that, since it says nothing about the tmux layer.
- Files: None (runtime-only).

---

## Phase 2: Closure

**Preconditions**:
1. Task 1.3.4a's verdict is PASS. If FAIL, stop here — do not execute Phase 2; file a follow-up bug instead per Story 1.3.4's acceptance criteria.
2. Phase 2 is executed by a session holding **no open `ItemSession` link** on this item — see Epic 2.0. It must **not** be executed by the same still-open triage-role session that ran Phase 1: `report_duplicate`'s role/link gating makes a same-session call fail (see Pattern Decisions' "Session boundary for closure" row).

### Epic 2.0: Confirm Session Boundary Before Closure
**Goal**: Phase 2 executes from a session that holds no open `ItemSession` link on this item — never the same still-open triage-role session that ran Phase 1 — because `report_duplicate`'s role/link gating makes a same-session call fail structurally, not just risk failing.

#### Story 2.0.1: Closure runs from a fresh, unlinked session
**As a** future session picking up this plan after Phase 1's smoke test passed, **I want** to confirm I hold no open `ItemSession` link on this item before calling `report_duplicate`, **so that** the call doesn't fail via `PERMISSION_DENIED` (role mismatch — this triage session's link, if still open, isn't "work" role, `server/mcp/tools_backlog.go:2205-2214`) or the unclaimed-archive path's "active session" refusal (`tools_backlog.go:2420`).
**Acceptance Criteria**:
- The session executing Phase 2 is confirmed distinct from the session that ran Phase 1, with no open link on the item.
  - *Given* Phase 1 (this triage session) has recorded a PASS verdict in Task 1.3.4a and Epic 1.4's teardown is complete, *When* this triage session's own work finishes, *Then* this session ends WITHOUT calling `report_duplicate` — Phase 2 is picked up later by a fresh session.
  - *Given* a fresh session picks up this plan for Phase 2, *When* `get_backlog_item` is called on `f7201b49-8318-43fb-a22f-0ce1926f9980`, *Then* no `ItemSession` entry shows `EndedAt == nil` before proceeding to Epic 2.1 — if one is found (e.g. the Phase 1 triage session hasn't fully ended yet), wait and re-check later rather than proceeding.
**Files**: None (runtime-only).

##### Task 2.0.1a: End this session after Phase 1 completes, without calling `report_duplicate` (~1 min)
- *Given* Task 1.3.4a recorded a PASS verdict and Epic 1.4's teardown is complete, *When* this triage session's own work is done, *Then* stop here — do not proceed to Epic 2.1 in this same session. Phase 2 resumes in a later, fresh session once this session has naturally ended (its `ItemSession` link's `EndedAt` is set when the underlying tmux process terminates).
- Files: None (runtime-only).

##### Task 2.0.1b: (Fresh session only) Confirm no open `ItemSession` link remains before proceeding (~2 min)
- Call `get_backlog_item` on `f7201b49-8318-43fb-a22f-0ce1926f9980`.
- *Given* the result, *When* any `ItemSession` entry shows `EndedAt == nil`, *Then* do not proceed to Epic 2.1 yet — some session (possibly the Phase 1 triage session, if it hasn't fully ended) still owns the item; wait and re-check later.
- *Given* no `ItemSession` shows `EndedAt == nil`, *When* confirmed, *Then* proceed to Epic 2.1.
- Files: None (runtime-only).

### Epic 2.1: Determine the Correct Closure Path
**Goal**: The item's current status/session-linkage and BUG-113's current status are both confirmed live, so the correct `report_duplicate` path is chosen deliberately rather than assumed.

#### Story 2.1.1: BUG-113's open status re-confirmed immediately before closing
**As a** triage session, **I want** to re-check BUG-113's status right before closing this item, **so that** this item's "tymux gap is low-risk because BUG-113 is open" framing hasn't silently gone stale.
**Acceptance Criteria**:
- BUG-113 is confirmed still open before proceeding.
  - *Given* `docs/bugs/open/BUG-113-tymux-backend-never-passes-target-program-launches-default-shell.md` was the last-known location of BUG-113's status, *When* the file is re-read at execution time, *Then* it is still present under `docs/bugs/open/` (not moved to `docs/bugs/fixed/`), confirming this item's tymux-gap scoping argument still holds.
**Files**: `docs/bugs/open/BUG-113-tymux-backend-never-passes-target-program-launches-default-shell.md` (read-only reference).

##### Task 2.1.1a: Re-check BUG-113's status (~1 min)
- Read `docs/bugs/open/BUG-113-tymux-backend-never-passes-target-program-launches-default-shell.md` (or check whether it has moved to `docs/bugs/fixed/`).
- Files: `docs/bugs/open/BUG-113-tymux-backend-never-passes-target-program-launches-default-shell.md`.

#### Story 2.1.2: Item status/linkage determines the `report_duplicate` path
**As a** triage session, **I want** to check item `f7201b49`'s live status and session linkage, **so that** the unclaimed-archive path is only used when it actually applies.
**Acceptance Criteria**:
- The item's status and linkage are read before any closure mutation.
  - *Given* item `f7201b49-8318-43fb-a22f-0ce1926f9980`, *When* `get_backlog_item` is called, *Then* its `status` field and any `ItemSession` linkage (role, ended/active) are recorded, and the closure path is chosen accordingly: unclaimed-archive if status is idea/refining/ready/queued with no `ItemSession` showing `EndedAt == nil`; the review-gate path if a work-role `ItemSession` is active; or wait-and-recheck-later if a triage-role `ItemSession` (e.g. this item's own Phase-1 session) is still active — see Task 2.1.2b's three-way branch.
**Files**: None (runtime-only).

##### Task 2.1.2a: Call `get_backlog_item` on `f7201b49` (~2 min)
- Call `get_backlog_item` with `item_id: f7201b49-8318-43fb-a22f-0ce1926f9980`.
- Record `status` and any `ItemSession` entries (role, ended/active).
- Files: None (runtime-only).

##### Task 2.1.2b: Branch on the status/linkage result (~2 min)
- *Given* the result from 2.1.2a, *When* status is one of idea/refining/ready/queued and no `ItemSession` with `EndedAt == nil` remains linked, *Then* proceed to Epic 2.2's unclaimed-archive path (`reportDuplicateUnclaimed`).
- *Given* the result instead shows status in_progress/review with an active work-role `ItemSession`, *When* this is observed, *Then* the review-gate path applies instead — flag BUG-032's empty-diff risk (pitfalls.md §3) before calling `report_duplicate`, since a citation-only `verification_notes` with no accompanying diff risks either an UNVERIFIABLE spiral or a weakly-grounded PASS.
- *Given* the result instead shows an active (`EndedAt == nil`) triage-role `ItemSession` still linked — including this item's own Phase-1 session, if Epic 2.0's wait was somehow skipped — *When* this is observed, *Then* do **not** call `report_duplicate` in this session: `report_duplicate`'s linked-caller path requires "work" role and returns `PERMISSION_DENIED` for a triage-role link (`server/mcp/tools_backlog.go:2205-2214`). Wait for that session to end, then re-check via `get_backlog_item` in a fresh session (per Epic 2.0) before proceeding. This is expected to be the case immediately after Phase 1 finishes, until this session's own link has ended.
- Files: None (runtime-only).

### Epic 2.2: Execute Closure
**Goal**: `report_duplicate` is called with PR #832 as `duplicate_ref` and PR #837 cited in `reason`, and the resulting state mutation is verified by reading it back — not just trusted from the return value.

#### Story 2.2.1: Item closed citing PR #832 and PR #837
**As** Tyler (the operator), **I want** this backlog item closed with a citation to the PRs that actually fixed it, **so that** the duplicate/stale planning artifacts on `backlog/stapler-squad-item-f7201b49` are not left as the only record of this conclusion.
**Acceptance Criteria**:
- `report_duplicate` archives the item with both PRs cited.
  - *Given* the unclaimed-archive path applies (per Task 2.1.2b) and the smoke test verdict is PASS (per Task 1.3.4a), *When* `report_duplicate` is called with `item_id: f7201b49-8318-43fb-a22f-0ce1926f9980`, `duplicate_ref: "https://github.com/tstapler/stapler-squad/pull/832"`, and `reason` stating the item was fixed by PR #832 (two-write submit + confirm/retry) with PR #837 ("https://github.com/tstapler/stapler-squad/pull/837") also cited for the related #819 empty-output fix, plus the live smoke-test result, *Then* the item's status becomes archived/done directly, without a review-gate detour.
- The mutation is verified by reading it back, not assumed from the call's return value.
  - *Given* `report_duplicate` returned success, *When* `get_backlog_item` is called again on `f7201b49`, *Then* its status reflects archived/closed and `VerificationNotes` contains the exact-line marker `duplicate_ref=https://github.com/tstapler/stapler-squad/pull/832`.
**Files**: None (runtime-only).

##### Task 2.2.1a: Call `report_duplicate` with PR #832 as `duplicate_ref` (~2 min)
- `report_duplicate(item_id: "f7201b49-8318-43fb-a22f-0ce1926f9980", duplicate_ref: "https://github.com/tstapler/stapler-squad/pull/832", reason: "<summary citing PR #832's two-write submit fix, PR #837 at https://github.com/tstapler/stapler-squad/pull/837 for the related #819 fix, and the live smoke-test PASS verdict from Task 1.3.4a>")`.
- Use the exact `duplicate_ref` string verbatim if this call is ever retried (idempotency is keyed on that exact line in `VerificationNotes`).
- Files: None (runtime-only).

##### Task 2.2.1b: Handle non-success results from `report_duplicate` (~3 min)
- **`ErrNotAuthenticated`**: *Given* `report_duplicate`'s GitHub-ref verification requires working credentials, *When* the call returns `ErrNotAuthenticated` (no `GITHUB_TOKEN`/`GH_TOKEN`/connected account), *Then* leave the item as-is and note it for Tyler rather than retrying in a loop.
- **`PERMISSION_DENIED`**: *Given* the calling session turned out to still be linked with a non-"work" role (`server/mcp/tools_backlog.go:2205-2214`), *When* the call returns `PERMISSION_DENIED`, *Then* do **not** retry in place — this confirms Epic 2.0's session-boundary check was not actually satisfied (the caller still has an open non-work `ItemSession` link). Re-run Task 2.0.1b's check from a genuinely fresh, unlinked session before trying again.
- **"active `ItemSession` still open" refusal** (unclaimed-archive path, `tools_backlog.go:2420`): *Given* some `ItemSession` on the item still has `EndedAt == nil`, *When* `reportDuplicateUnclaimed` refuses for this reason, *Then* the resolution is the same as above — wait for that session to end, then retry from a fresh session once `get_backlog_item` confirms no open link remains.
- Files: None (runtime-only).

##### Task 2.2.1c: Verify the closure mutation by reading it back (~2 min)
- Call `get_backlog_item` on `f7201b49-8318-43fb-a22f-0ce1926f9980` again.
- *Given* Task 2.2.1a reported success, *When* the item is re-read, *Then* its status is archived/closed and `VerificationNotes` contains `duplicate_ref=https://github.com/tstapler/stapler-squad/pull/832` — confirming the mutation actually took effect, per the "read a mutation back before claiming it happened" discipline.
- Files: None (runtime-only).

### Epic 2.3: Clean Up the Stale Abandoned Branch
**Goal**: `backlog/stapler-squad-item-f7201b49` (the abandoned prior-triage branch, commit `12d2849de`, never merged) no longer sits as a stale "fix still pending" signal for a future triage pass.

#### Story 2.3.1: Stale branch no longer misleads future triage
**As a** future triage session encountering this item's history, **I want** the abandoned branch removed or clearly marked resolved, **so that** I don't re-triage an already-closed item from scratch.
**Acceptance Criteria**:
- The stale branch is deleted (or explicitly annotated) after confirming it never merged.
  - *Given* `backlog/stapler-squad-item-f7201b49` at commit `12d2849de` is confirmed absent from `main`'s history, *When* `git push origin --delete backlog/stapler-squad-item-f7201b49` is run, *Then* the branch no longer exists on `origin`, and a future `git branch -r` or GitHub branch listing no longer shows it as an unresolved lead.
**Files**: None (runtime-only; a remote git operation, no repo file changes).

##### Task 2.3.1a: Confirm the branch is still unmerged (~2 min)
- `git fetch origin backlog/stapler-squad-item-f7201b49`
- `git merge-base --is-ancestor 12d2849de main` (or `git log main..origin/backlog/stapler-squad-item-f7201b49`) — confirm `12d2849de` is not reachable from `main`.
- Files: None (runtime-only).

##### Task 2.3.1b: Delete the stale branch (~2 min)
- *Given* the branch is confirmed unmerged and superseded by this item's closure (Task 2.2.1a), *When* `git push origin --delete backlog/stapler-squad-item-f7201b49` is run, *Then* the branch no longer exists on `origin`.
- Files: None (runtime-only).
