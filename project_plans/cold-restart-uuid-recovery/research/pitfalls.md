# Pitfalls: Cold-Restart UUID Recovery Before Fresh-Start Decision

Agent 4 (Pitfalls) — SDD research phase for `cold-restart-uuid-recovery`.

## 1. Reintroducing a variant of the session-resume-uuid-fix bug

`project_plans/session-resume-uuid-fix/requirements.md` documents a real production bug:
`correlateSession()` in `session/history_linker.go` called `DetectByPath()` — "pick the
newest JSONL in the directory" — unconditionally whenever no live PID could be found,
including for **already-linked** paused sessions. If a different session (B, C) ran in
the same directory after session A was paused, B/C's newer JSONL silently overwrote A's
correct stored UUID. The fix (already shipped, `session/history_linker.go:264-273`) gates
the fallback: `pathFallbackAllowed := !alreadyLinked || (Status not in {Paused,
Hibernated, Stopped})`. Critically, the fix's own requirements doc scopes the
**unlinked-session, shared-directory** case explicitly **out**:

> Out of Scope: Handling the case where a paused session has NO stored UUID and multiple
> sessions have run in the same directory (this is a pre-existing limitation, best effort
> only).

`cold-restart-uuid-recovery` targets exactly that carved-out case: `ConversationUUID ==
""` with dead tmux. A naive "call `DetectByPath` before `initTmuxSession` whenever the
in-memory UUID is empty" reintroduces the same class of bug through three concrete
variants, none of which the existing `correlateSession` guard protects against (that
guard only fires on `alreadyLinked`; here the UUID is empty by construction):

- **Two sessions cold-starting in the same directory near-simultaneously.** Two distinct
  `Instance`s (different `Title`, same `Path`/worktree — legal for directory-mode
  sessions) can both be dead-tmux with empty UUID after e.g. a host reboot. Each
  `Instance` has its own actor goroutine (ADR-025), so both `startLocked` calls can run
  concurrently. `DetectByPath` is a pure read (`os.ReadDir`+stat, no locking across
  instances) so there's no data race, but there IS a logical race: both could resolve to
  the **same** "most recent" JSONL and each embed `--resume <same-uuid>` in their launch
  command. Two concurrent `claude --resume <uuid>` processes against the same
  conversation JSONL is a correctness hazard (concurrent append/rewrite of one JSONL,
  unpredictable which process's `/clear` or turn "wins"), independent of whether the UUID
  was ever wrong.
- **A session that intentionally has no prior conversation.** `Instance.ClearConversationState()`
  (`session/instance_claude.go:278`) exists specifically so a user-triggered "start new
  conversation" (the `ClearConversationState` RPC, wired through
  `server/services/checkpoint_service.go:186` and `session/instance_program.go:66` on
  program switch) makes the **next** start begin fresh rather than resuming a stale UUID.
  Its own doc comment says so explicitly: "so that the next Resume starts a fresh
  conversation rather than attempting `--resume` with a potentially stale... UUID." A
  naive empty-UUID-triggers-recovery fix defeats this by design: the directory/worktree
  still has the just-cleared (or an older, unrelated) JSONL sitting on disk, and
  `DetectByPath` will happily resurrect it the moment the session cold-starts again —
  exactly the outcome `ClearConversationState` exists to prevent.
- **A stale worktree path reused for a new purpose.** Worktree paths get recycled
  (pruned and recreated at the same filesystem path, or a directory-mode session pointed
  at a path a *different*, unrelated prior session used). `~/.claude/projects/<encoded-path>/`
  is keyed purely on the encoded absolute path (`ClaudeProjectDirName`), not on any
  Stapler Squad session identity — it has no notion of "this JSONL belongs to Instance X."
  A brand-new session (legitimately no UUID, first-ever start) landing on a path with old
  JSONL history from a different, unrelated Claude conversation gets silently merged with
  that conversation's history.

**There is no in-memory signal today that distinguishes "never had a conversation, safe
to recover" from "had one, intentionally cleared" or "shares a path with a stale/sibling
conversation."** `ConversationUUID == ""` is the same bit pattern for all three. Any
design for this fix needs an explicit way to tell them apart — candidates worth
validating in the plan phase: only attempt recovery on the very first cold-start after
process death (not after an explicit clear — may need a "cleared" flag or timestamp
distinct from "never set"), compare JSONL mtime against a session lifecycle timestamp
(e.g. only trust a JSONL newer than the instance's own creation/last-active time), or
scope recovery to worktree-mode sessions only (where path uniqueness is much stronger
than directory-mode) and treat directory-mode as out of scope the same way
session-resume-uuid-fix did.

## 2. Filesystem / timing pitfalls in `DetectByPath`

`DetectByPath` (`session/history_detector.go:137-199`) is a synchronous
`os.ReadDir(dir)` followed by `entry.Info()` (stat) per candidate file, done entirely on
the calling goroutine with no timeout.

- **Slow/networked home directory.** If `~` is on NFS, a FUSE mount, or a syncing cloud
  drive, `os.ReadDir` + N stats can block for seconds with no cancellation path — the
  function takes no `context.Context` and has no deadline. Since this call would now run
  inside `startLocked` (see §3), there's no independent timeout protecting the caller.
- **Concurrent writes to the JSONL mid-flush.** Claude appends to the JSONL as the
  conversation progresses. `entry.Info()` reading `ModTime()` mid-write is fine (mtime
  updates are effectively atomic at the OS level), but there's a narrower race: if a
  *second* Claude process (the scenario in §1's first bullet) is actively appending to
  the file at the exact moment `DetectByPath` is choosing it as the resume target, the
  file is not corrupted by the read, but the choice itself is meaningless — "most
  recently modified" during an in-progress write is not a reliable signal of "the right
  conversation for this session," it's a signal of "something is writing to this
  directory right now."
- **Directory accumulation.** `~/.claude/projects/<dir>/` has no eviction — JSONL files
  accumulate for the life of the machine. A long-lived dev path (e.g. a personal wiki or
  frequently-reused directory-mode session) can accumulate hundreds to thousands of
  stale JSONLs from `/clear`-created conversations, old sessions, and abandoned agents.
  `DetectByPath` does an `entry.Info()` syscall per `.jsonl` file with no early exit —
  it's O(n) stats even though only the single newest file's UUID is used. This cost is
  already paid today by `HistoryLinker`'s background poll (`session/history_linker.go`),
  but that path runs off the caller's critical path; moving an equivalent call into
  `startLocked` puts it on a path a human or automated caller is synchronously waiting
  on (see §3).

## 3. Concurrency pitfall: actor mailbox blocking

`startLocked` runs only inside a `sendSyncErr` closure
(`session/instance.go:818-819`), and `sendSyncErr` (`session/actor.go:34-56`) is
**synchronous**: the caller blocks on `<-reply` until the actor goroutine finishes
executing the closure. Per ADR-025
(`project_plans/instance-actor-concurrency/decisions/ADR-025-one-actor-per-instance-not-shared-actor.md`),
each `Instance` has its own actor goroutine and mailbox — so a slow `DetectByPath` call
inside one `Instance`'s `startLocked` does **not** stall other `Instance`s' actors or
their `Start`/`Pause`/etc. calls (no head-of-line blocking across sessions, by
construction). But ADR-025's own "Negative / Accepted tradeoffs" section is explicit that
this per-instance isolation does not excuse slow work *within* one instance's command
handler (R2.5/R2.8 in `project_plans/instance-actor-concurrency/requirements.md:61,64`
codify "keep each command handler fast" as a still-live constraint). Concretely, adding
`DetectByPath` to `startLocked` risks:

- **Caller-visible latency for the specific request that triggered `Start(false)`.**
  Callers of `Start(false)` include `session/instance_hibernate.go:110,154` (user-facing
  wake-from-hibernation), `server/mcp/tools_lifecycle.go:347,450,466` (MCP tool calls —
  an agent or IDE integration synchronously awaiting the RPC), and
  `session/session_driver.go:541` (autonomous-driver auto-restart). Any of these now
  waits on a filesystem scan with no timeout before getting a reply.
- **Serialized cost across `session/health.go`'s recovery loop.** `CheckAllSessions()`
  (`session/health.go:59-98`) iterates `instances` in a plain `for` loop and calls
  `checkSingleSession` — which calls `instance.Start(false)` for every dead-tmux session
  it finds — **synchronously, one at a time** (`session/health.go:83,98,179,229`). This
  loop is not per-instance-actor-parallel; it's the driver, and it's sequential. After a
  reboot or a mass tmux-server kill, many sessions can be simultaneously dead-tmux with
  empty UUID. Even though each `DetectByPath` call is individually isolated to its own
  instance's actor, the health-check *pass* itself pays for N sequential filesystem scans
  back-to-back, extending the total time before the last session in the list gets
  attention, and risking overlap with the next health-check tick if the interval is
  short relative to N × scan-time.
- **Any queued command for that same instance still waits.** If a UI action is queued on
  the same instance's mailbox right after `Start(false)` was enqueued (e.g., a rapid
  double-click, or a `Snapshot`-driven UI poll that happens to route through the actor
  rather than the atomic snapshot), it now waits behind the filesystem scan too — this is
  within-instance queuing, which is expected/by-design for the actor model, but worth
  confirming against R2.5/R2.8's "keep handlers fast" intent before shipping.

## 4. Testing pitfalls

`session/instance_cold_restore_test.go`'s existing tests
(`TestColdRestore_WithUUID`, `TestColdRestore_WithoutUUID`,
`TestHotRestore_ExistingSession`) are real-tmux integration tests, skipped under
`-short` (`if testing.Short() { t.Skip(...) }`), and none of them writes a JSONL file to
disk or overrides the home directory — `TestColdRestore_WithoutUUID` in particular
(lines 99-142) is the closest existing analogue to the new scenario, but it asserts only
that `Start(false)` succeeds and the instance reaches `Running`, with no on-disk
conversation history in play at all.

Two structural gaps make a new regression test for "dead-tmux + empty-UUID +
JSONL-present" risky to write correctly:

- **No test-only injection point for `historyDetector` exists yet.** `Instance.historyDetector`
  (`session/instance.go:266-268`, used by `tryExtractConversationUUID`,
  `session/instance_claude.go:314`) has no constructor parameter, no setter, and — per a
  repo-wide grep — **no existing test sets it**. When nil, both the current
  `tryExtractConversationUUID` fallback and any new call site this fix adds would resolve
  to `NewHistoryFileDetectorWithRealInspector()`, i.e. the developer's/CI runner's actual
  `~/.claude/projects/`. A regression test that doesn't plumb a `NewHistoryFileDetectorWithHomeDir`
  override into the `Instance` under test either (a) can't hermetically prove the fix (no
  fixture JSONL will be found in the real home dir, so the test can't distinguish "fix
  works" from "fix never ran"), or worse (b) if it *does* write into the real
  `~/.claude/projects/<encoded-tmpdir-path>/` to force a match, it pollutes the
  developer's or CI runner's real Claude project history — a correctness and hygiene
  problem, and a source of test flakiness if a stray file collides with a real
  conversation dir on a shared CI box. The fix's implementation must add a way to inject
  `historyDetector` (or reuse the field a new constructor path already can set) before a
  hermetic test is possible at all.
- **mtime-resolution races if the test relies on wall-clock ordering instead of explicit
  timestamps.** `history_detector_test.go`'s own `TestHistoryFileDetector_DetectByPath_PicksMostRecentWhenMultiple`
  (lines 202-228) avoids this by writing both files and then calling `os.Chtimes` with
  explicit `past`/`future` `time.Time` values rather than relying on write-order +
  real-clock delay — some filesystems have coarse (1s+) mtime granularity, so two files
  written microseconds apart in a fast test can get identical mtimes and make
  `DetectByPath`'s "most recent" comparison nondeterministic. A new cold-restart
  regression test that writes a fixture JSONL and expects it to be picked up must follow
  the same `os.Chtimes`-with-explicit-times pattern, not "sleep a bit between writes."
- **The real-tmux nature of `instance_cold_restore_test.go` makes it expensive to add
  many scenario variants there.** Since the fix's dangerous edge cases (§1) are about
  *decision logic* (should recovery run at all, given UUID-empty + JSONL-present +
  {intentionally-cleared, shared-directory, ...}), the safer place to pin those decisions
  down is a fast unit test close to `history_detector_test.go`'s style (fake
  `ProcessFileInspector`/fixed home dir, no real tmux), reserving the real-tmux
  integration test in `instance_cold_restore_test.go` for the end-to-end "does `--resume
  <uuid>` actually get embedded and does the process come up" assertion — mirroring how
  that file's own doc comment already defers `--resume` flag-injection testing to
  `claude_command_builder_test.go` rather than re-proving it at the tmux level.

## 5. Log/observability pitfall

Today, `startLocked`'s cold-restore branch (`session/instance.go:867-874`) already logs
unconditionally on every dead-tmux restart:

```go
if i.HasClaudeSession() {
    log.Info("cold restoring with --resume", ...)
} else {
    log.Warn("cold start: tmux dead, no conversation UUID, starting fresh", ...)
}
```

This `log.Warn` fires for **every** dead-tmux restart where the in-memory UUID happens to
be empty, regardless of whether that's actually anomalous. Layering the new requirement
(R3: distinguish "never had a conversation" vs "recovery failed") on top of this existing
line risks two failure modes:

- **Over-logging / WARN spam.** If the new code path logs at WARN whenever
  `DetectByPath` is attempted-and-empty, every genuinely first-time cold-start (brand-new
  session, crashes before ever talking to Claude, then restarts) — a completely normal
  event — produces a WARN. At fleet scale (a few hundred long-lived instances per
  ADR-025's own framing), this could become permanent background noise that trains
  operators to ignore WARNs from this subsystem entirely, defeating the point of adding
  the distinction.
- **Under-logging / losing the signal.** Conversely, if "recovery attempted but found
  nothing" and "recovery attempted, found something, and used it" both log at the same
  level with similar wording, an operator debugging "why did this session lose its
  history" can't grep the logs to tell "no JSONL ever existed" from "a JSONL existed but
  something went wrong extracting/validating it" (e.g., `DetectByPath` erroring on a
  transient stat failure vs. legitimately finding zero candidates) — collapsing exactly
  the distinction R3 asks for.

The safer split, consistent with existing conventions elsewhere in this file (e.g.
`correlateSession`'s `log.Info` only on an actual UUID *change*, not on every no-op
match, `session/history_linker.go:295-304`): log at INFO (not WARN) when recovery
succeeds (a genuine, useful event — "found and resumed a conversation the in-memory
state had lost"), log at DEBUG for "no candidates found, starting genuinely fresh" (the
common, unremarkable case), and reserve WARN for actual failures (`DetectByPath`
returning a non-nil `error`, e.g. a real I/O error, as opposed to its documented `nil,
nil` "not found" contract) or for the "found a candidate but a safety guard (§1) declined
to use it" case, since *that* is the situation an operator most wants visibility into
before it's mistaken for silent data loss.
