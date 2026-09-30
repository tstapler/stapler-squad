# Research: Pitfalls — session-enter-not-sent

Scope reminder (per requirements.md): the only remaining work is (a) a live smoke test of
`write_to_session`/`steer_session`/`run_command` against PR #832's fix, and (b) closing backlog
item `f7201b49-8318-43fb-a22f-0ce1926f9980` citing PR #832/#837 — not new engineering, not the
tymux gap, not re-litigating `sendkeysguard`. All findings below are pitfalls *in that remaining
work*, not the original defect.

---

## 1. "PR merged claiming to fix X" vs. "verified fixed in practice"

**What static review + unit tests actually cover today**, per `session/pane_submit_test.go` and
`server/mcp/tools_terminal_test.go` (per requirements.md's own evidence list): the two-write shape,
swallowed-submit retry/failure, context-cancellation, and wedged-write timeout, plus
`TestMCPPackage_NoDirectSendKeysPlusEnterConcatenation` (an AST-level structural check, not a
runtime one). **None of this exercises a real Claude Code Ink-TUI process.** `waitForPaneUpdate`/
`HasUpdated()` are tested against fakes that report scripted "changed"/"unchanged" sequences — the
tests prove the retry/error-surfacing logic is correct *given* a pane-change signal, not that a real
Ink TUI actually produces that signal at the right time for genuinely large/multi-line input. This
mirrors BUG-031's own "Verification" section, which states outright: *"this is a real tmux/PTY
timing bug that cannot be forced deterministically in a unit test... A full end-to-end repro would
require driving a real tmux pane with a large payload and asserting on Claude Code's own
paste-detection behavior, which is out of this repo's control to assert against reliably."* That
gap is exactly what a live smoke test exists to close — and exactly why static code reading (which
is all the requirements.md evidence trail is, by its own admission) is not sufficient proof.

**Concrete gaps a live test could catch that the existing suite structurally cannot:**
- Claude Code CLI version drift changing the paste-detector's actual timing/heuristics since
  `waitForPaneSettle`'s constants (`DefaultPaneSettlePollInterval = 150ms`,
  `DefaultPaneSettleMaxWait = 2s`, `session/pane_submit.go:18-20`) were tuned. The unit tests can't
  detect "the real CLI's window changed shape," only that the Go retry logic still does what it's
  told.
- The **`maxInputBytes = 4096`** cap (`server/mcp/tools_terminal.go:19`) on `write_to_session`'s
  `input` param. `steer_session`/`run_command` may have their own separate caps — check before
  assuming the same ceiling. A smoke-test payload sized to reliably trigger the Ink-TUI
  paste-detection window (per requirements.md's own open question: "does the fix hold for
  genuinely long/multi-line input... not just short single-line input that may never have been
  affected") must stay under whatever cap applies, or the test exercises `INPUT_TOO_LONG` instead
  of the submit path at all — a false "pass" for the wrong reason if the tester doesn't notice the
  early rejection.
- `write_to_session` is rate-limited to **1 call/sec per session** (`th.writeLim`,
  `server/mcp/tools_terminal.go:49,297`). A smoke-test script that fires multiple attempts in quick
  succession (e.g., retrying after a perceived failure, or looping over the three tools against the
  same session) will hit `RATE_LIMITED` and can be misread as the submit fix failing.
- A trivial/short single-line smoke input (e.g. `"echo hi"`) most likely never entered the
  paste-detector's burst window even pre-fix (per BUG-031's own root-cause note: short messages
  already worked with a bare `"\r"`) — a passing smoke test with short input proves nothing about
  the actual regression class. The test must use long/multi-line content specifically.

## 2. Manually driving a second instance without disturbing `:8543`

CLAUDE.md's "Manual/interactive testing" section already documents the mechanics (port block
62871-62880, `~/.stapler-squad/manual-builds/manual-N/`, `STAPLER_SQUAD_INSTANCE=<name>`,
`--tmux-keep-server`) — the pitfalls below are the ways to get that *procedure* wrong specifically
for an MCP-tool smoke test (as opposed to just clicking around the web UI, which is the doc's
primary worked example):

- **Wrong-port MCP registration is the highest-severity mistake here.** The MCP endpoint is mounted
  at `/mcp` on the *same port* the instance's web server binds (`srv.mux.Handle("/mcp", ...)`,
  `server/server.go:978`) — there is no separate MCP-only port. If the test client (e.g. `claude mcp
  add --transport http <name> http://localhost:<port>/mcp`) is pointed at `:8543` instead of the
  manual instance's port (e.g. `62871`), every `write_to_session`/`steer_session`/`run_command` call
  lands on the **live production instance** and can inject text into a real, currently-running
  session — silent data corruption of someone's actual work, not a test artifact. Triple-check the
  port in the registration command before sending the first tool call.
- **A stale/leftover MCP client registration is easy to leave behind.** `claude mcp add` (or
  equivalent for whatever MCP client drives the smoke test) writes to a *persistent* client config
  (e.g. `~/.claude.json` or project `.mcp.json`), not something scoped to the manual instance's
  lifetime. Killing the manual `stapler-squad` process (`kill %1`) does not remove the registration
  — a later, unrelated session that still has this entry configured will get connection-refused
  errors (harmless but noisy) or, worse, silently succeed against a *different* process if the same
  port later gets reused by something else. Explicitly `claude mcp remove <name>` (or the
  equivalent) after the smoke test, not just kill the process.
- **`write_to_session`'s `session_id` targets a session inside whatever instance the MCP client is
  connected to** — there's no cross-instance session ID collision risk in practice (session UUIDs
  are unique), but if the smoke-test client has *two* MCP servers registered simultaneously (e.g.
  the live `:8543` one from normal daily use, plus a newly-added manual-instance one) and the tool
  call is dispatched to the wrong registered server by an ambiguous/duplicate tool name, the same
  wrong-instance risk as above applies via a different path. Prefer registering only the manual
  instance's MCP server for the duration of the test, or use a client that lets you pin the server
  explicitly per call.
- **Stale processes**: per CLAUDE.md, the manual instance defaults to `--tmux-keep-server`
  guidance — if the smoke test's own tmux session (the one running the target Claude Code TUI being
  written to) is left attached to a tmux server that outlives the `kill %1` of the Go process, a
  zombie tmux server can persist and hold the manual instance's chosen ports/sockets, causing a
  *second* manual-instance run later to collide confusingly. Kill the tmux server explicitly
  (`tmux -L <manual-socket> kill-server` or equivalent) as part of teardown, not just the Go binary.
- **Binary path discipline**: build to `~/.stapler-squad/manual-builds/manual-1/stapler-squad`, never
  `./stapler-squad` (the live systemd unit's `ExecStart` target) and never a bare `/tmp` path — this
  is already called out explicitly in CLAUDE.md's own "Manual/interactive testing" section; the
  pitfall is skipping it because the smoke test "is just a quick check" and reaching for `go run .`
  or overwriting the repo-root binary out of habit.
- **`STAPLER_SESSION_UUID` is not required for `write_to_session`/`steer_session`/`run_command`
  themselves** (confirmed: `writeToSession` in `server/mcp/tools_terminal.go:277` reads
  `session_id` from the call args, not from caller-identity context) — but *other* MCP tools used
  incidentally during the smoke test (e.g. `request_review`, `report_duplicate` for the closure step
  below) do require it via `callerSessionUUID`. Don't conflate "this tool needs
  `STAPLER_SESSION_UUID`" across tools; check each one, since a manually-invoked MCP client (plain
  terminal, no Stapler-Squad-spawned session) legitimately has no `STAPLER_SESSION_UUID` set at all.

## 3. Closing the backlog item correctly

**The closing mechanism is `report_duplicate` (`server/mcp/tools_backlog.go:2158` /
`reportDuplicateUnclaimed` at line 2391), not a generic "mark done."** This matters because its
behavior branches sharply on whether the calling session is *linked* to the item:

- **If a session has a `work`-role `ItemSession` link to the item**: `reportDuplicate` routes the
  item to **`review`** status (never directly to `done`/`archived` — the code comment cites this as
  "ADR-001": *"a work session must never unilaterally close out its own work"*), and a real review
  gate call fires against whatever diff exists.
- **If the caller is unlinked (a passerby, e.g. this triage session)** and the item is in one of
  `unclaimedDuplicateSourceStatuses` (idea/refining/ready/queued — **not** `in_progress`/`review`),
  `reportDuplicateUnclaimed` **archives the item directly**, bypassing the review gate entirely. Its
  own doc comment explains why: an unclaimed item "has no diff, no commits, nothing for a reviewer
  to look at," and routing it through the normal review-gate pipeline would hit the
  `committedDiffEmpty` guardrail and loop the item between `review`/`in_progress` forever.

**Concrete pitfalls in picking/using this tool for this item:**

- **Path selection is a status-and-linkage decision, not a free choice.** If item `f7201b49` is
  currently `in_progress` with *this very triage session* as its linked `ItemSession`, that session
  is by definition **not** unclaimed — `reportDuplicateUnclaimed`'s own guard rejects acting on any
  item with an active (unended) `ItemSession`, explicitly *including* an active triage-role session
  analyzing it (`server/mcp/tools_backlog.go:2408-2426`: *"idea/refining items can carry an active
  (not yet ended) triage-role ItemSession... Refuse rather than archive out from under it"*). Check
  the item's actual current status and session linkage via `get_backlog_item` before assuming either
  path applies — do not guess.
- **If routed through the linked-session `review` path, BUG-032's leftover risk applies directly.**
  BUG-032 (`docs/bugs/fixed/BUG-032-review-cycles-against-already-merged-empty-diff.md`) is the
  *exact* prior incident of "an item whose fix already merged into `main` gets bounced through
  review against an empty/irrelevant diff." That bug's own "Not Fixed" section explicitly leaves two
  factors open: **no distinct "diff is empty/irrelevant" review outcome**, and **a PASS is reachable
  on self-report alone with an empty diff** — i.e., citing PR #832/#837 in `verification_notes` with
  no accompanying diff risks either an UNVERIFIABLE spiral (this item's own
  `project_plans/backlog-already-implemented/` documents that exact 3-cycle-then-silently-parked
  failure mode) or a rubber-stamped PASS with weak grounding, per that project's pitfalls research
  (`project_plans/backlog-already-implemented/research/pitfalls.md`, "False negative risk /
  leniency bias" section) — neither of which is a clean close.
- **`duplicate_ref` accepts exactly one GitHub PR/issue/commit URL, ≤500 characters, and is
  GitHub-verified before any mutation** (`resolveDuplicateRef`, `tools_backlog.go:2341` →
  `verifyGitHubRefExists`/`GetPR`/`GetIssue`/`GetCommit`). Two pitfalls follow: (a) it must be a
  **full GitHub URL** (`ParseGitHubRefWithHosts`), not a bare `"PR #832"` string, or it fails
  `ErrInvalidArgument` before any GitHub call; (b) since this item cites **two** PRs (#832 for the
  submit fix, #837 for the related #819 empty-output fix) but the tool takes a single ref, put the
  primary/on-point one (#832) in `duplicate_ref` and mention #837 in the `reason` field (≤1000
  chars) — don't assume the tool supports multiple refs, and don't drop the #837 citation because
  there's nowhere obvious to put it.
- **GitHub verification requires the calling session to have working GitHub credentials.** If this
  session has no `GITHUB_TOKEN`/`GH_TOKEN`/connected account, `resolveDuplicateRef` returns a
  non-retryable `ErrNotAuthenticated` result, explicitly instructing the caller to *leave the item
  as-is and note it for an operator* rather than retry — don't loop on this error expecting it to
  eventually succeed.
- **Idempotency is keyed on an exact-line marker (`"duplicate_ref=" + duplicateRef`) in
  `VerificationNotes`, not on item identity.** If the closure step is retried after a partial
  failure, use the *same* `duplicate_ref` string verbatim to get the safe no-op path — a
  differently-formatted URL for the same PR (e.g. with vs. without `https://`) is treated as a
  distinct call and will hit `ADR-004`'s "reject a differing second ref" rejection instead.
- **The stale abandoned branch is a separate cleanup, not automatic.** requirements.md notes prior
  planning artifacts exist on `backlog/stapler-squad-item-f7201b49` (commit `12d2849de`, never
  merged). Closing the backlog item via `report_duplicate`/archive does **not** delete or comment on
  that branch — if leaving it around as a stale, confusing "the fix must still be pending" signal
  for a future triage pass matters (requirements.md's Success Metrics explicitly call out "the
  duplicate/stale planning artifacts... are not left as the only record of this conclusion"), that
  branch needs its own explicit cleanup (delete, or a comment pointing at this item's resolution) —
  don't assume the backlog tool handles it.

## 4. Other open bugs that interact with or could be confused for this one

- **BUG-113** (`docs/bugs/open/BUG-113-tymux-backend-never-passes-target-program-launches-default-shell.md`,
  still open, confirmed via this pass) — the tymux backend launches `$SHELL` instead of the
  configured program, which is *why* the tymux submit-confirmation gap (permanently-stubbed
  `HasUpdated()`, `session/tymux/session.go:719-721`) is currently low-risk: there's no Claude Code
  Ink-TUI running on that backend to swallow a submit today. **Pitfall**: if BUG-113 ships before
  this item is smoke-tested/closed, the tymux gap becomes live and this item's "explicitly
  out-of-scope, mitigated by BUG-113 being open" framing silently stops being true. Worth a
  one-line check of BUG-113's status immediately before closing this item, not just at
  requirements-write time.
- **Backlog item `e08ad709-6ade-48cf-81f9-f577b53c1b10`** (referenced in requirements.md, not a
  `docs/bugs/` doc — it only exists in the live backlog store, not as a repo-resident file) covers
  the `INPUT_REQUIRED`/numbered-selection prompt delivery failure mode that PR #832's own commit
  message explicitly calls out as **not** fixed by that PR. This is the single easiest thing to
  conflate with this item during a live smoke test: if a test scenario happens to involve Claude
  Code presenting a numbered-choice/permission prompt and the smoke test observes *that* failing to
  register, it is evidence for `e08ad709`, not a regression in this item's scope. Keep smoke-test
  scenarios to plain text input/Enter, not interactive prompt selection, to avoid muddying the
  verdict.
- **BUG-032** (`docs/bugs/fixed/BUG-032-review-cycles-against-already-merged-empty-diff.md`,
  fixed) — already covered in depth under §3; flagged here again because it's the single most
  on-point prior incident for "how does this specific repo's automation mishandle an
  already-shipped fix during closure," and its "Not Fixed" factors are exactly what makes the
  `reportDuplicateUnclaimed` (bypass review-gate) path preferable to the linked-session (`review`)
  path for this item, if the status/linkage check in §3 permits it.
- **BUG-031** (`docs/bugs/fixed/BUG-031-autonomous-driver-large-prompt-paste-not-submitted.md`,
  fixed) — same root-cause shape as this item's original defect, but in `AutonomousDriver.run`
  (`session/autonomous_driver.go`), a *different* call site than the three MCP tools this item is
  about. Its "Recurring shape" reflection note (*"an action is taken but its actual effect is never
  verified"*) is the reason `sendkeysguard` was generalized across `session/`, `server/mcp/`,
  `server/services/`, and `session/tymux` rather than patched once more at a fourth call site — worth
  citing as background if the smoke test surfaces yet another undetected call site, but not
  in-scope to fix here.
- **BUG-047** (`docs/bugs/fixed/BUG-047-write-to-session-uses-newline-instead-of-carriage-return.md`,
  fixed 2026-07-24) — the `\n`-vs-`\r` half of this same defect family, at the same three call
  sites this item is about. Superseded by PR #832's two-write fix, but useful context: if a smoke
  test somehow observes a `\n` being sent instead of `\r`, that would indicate a *regression* of
  BUG-047, not a new finding — check `session/pane_submit.go`'s use of
  `session.EnterKeySequence` before assuming it's novel.
