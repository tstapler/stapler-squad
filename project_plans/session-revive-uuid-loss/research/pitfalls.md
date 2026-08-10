# Pitfalls Research: session-revive-uuid-loss

Phase 2 (Pitfalls) research for the fix described in
`project_plans/session-revive-uuid-loss/requirements.md`: call `DetectByPath`-based
UUID recovery *before* the resume-vs-fresh decision in `startLocked`/`start`
(`session/instance.go`), instead of only after.

## Summary of what's already true in this codebase (don't re-derive, don't regress)

1. **The DetectByPath ambiguity bug is already fixed, but only in `HistoryLinker`,
   not in `tryExtractConversationUUID`.** `session/history_linker.go:274-275`
   (`pathFallbackAllowed`) guards `DetectByPath` so it only runs for a
   **not-yet-linked** session, or an already-linked session that is not
   `Paused`/`Hibernated`/`Stopped`. `session/instance_claude.go:308-349`
   (`tryExtractConversationUUID`, the function this bug's fix must call earlier)
   has **no equivalent guard** — it already skips entirely if a UUID is set
   (line 310), so in practice it's only ever reached when `alreadyLinked == false`
   in `HistoryLinker`'s terms. That happens to make it safe today, *but only
   because every caller already checks `HasClaudeSession()` first*. Any new
   call site this fix adds must preserve that invariant (only call recovery
   when there is genuinely no UUID), or it resurrects the exact ambiguity bug
   `project_plans/session-resume-uuid-fix/` fixed: `DetectByPath` picks the
   *most recently modified* JSONL in the directory with **no ownership check**
   (`session/history_detector.go:189-193`), so a second session sharing the
   same working directory would silently donate its UUID.

2. **`DetectByPath` does not validate file content, only the filename.** It
   filters by `.jsonl` suffix, excludes `agent-*` prefixed files, and requires
   the basename to be a valid UUID (`session/history_detector.go:159-183`) —
   but never opens or parses the file. A 0-byte file created the instant
   before a crash (mid-`os.Create`, before Claude's writer flushes any bytes)
   is indistinguishable from a real conversation and will be returned as the
   "best" candidate purely by `ModTime`.

3. **`ClearConversationState` wipes both fields the requirements' Goal 2/3
   "had history before" signal would naively read.** `session/instance_claude.go:278-296`
   zeroes `claudeSession.ConversationUUID` **and** `i.HistoryFilePath` together,
   with no separate durable marker left behind. Three call sites do this today:
   `recoverFromStaleResume` (`instance_claude.go:83`, auto-recovery from a bad
   `--resume` UUID), the `ClearConversationState` RPC
   (`server/services/checkpoint_service.go:161-195`, user-initiated "start
   over"), and `instance.go:1556-1560` (worktree recreation after a paused
   session's worktree was torn down — the encoded project path changes, so the
   old UUID is deliberately invalidated). **All three are intentional,
   already-fresh-start cases** — exactly what Goal 4 says must not regress. If
   the fix's "was this an unexpected loss" signal is implemented by checking
   "is `HistoryFilePath` currently empty," it cannot distinguish these
   intentional clears from the unexpected watchdog-restart-raced-capture case
   the bug report describes, because both leave the same empty state. A
   correct signal needs to be set *at the clear call site* (e.g., only the
   watchdog/restart path marks "lost unexpectedly"), not inferred after the
   fact from field emptiness.

4. **The launch command is already frozen by the time the resume/fresh
   decision runs — recovering the UUID *after* that point doesn't help.**
   `initTmuxSession()` (`session/instance_tmux.go:249-279`) is called
   unconditionally near the top of both `startLocked` (`instance.go:858`) and
   `start` (`instance.go:1040`), and it calls
   `i.buildLaunchCommand(claudeSessionID)` — reading `i.claudeSession.ConversationUUID`
   **at that moment** — to build the `tmux new-session` command (including
   `--resume <uuid>` or its absence) that gets stored on the `TmuxSession`
   object. The resume/fresh decision block (`instance.go:878-935` and
   `:1068-1141`) runs *after* `initTmuxSession()` already captured the command.
   **This is the single most important pitfall for the implementer**: simply
   moving `tryExtractConversationUUID()` (or an equivalent recovery call)
   earlier than the `log.Info("cold restoring...")`/`log.Warn("cold start...")`
   branch is not sufficient — if it runs after `initTmuxSession()` (as it
   would if inserted at the obvious spot, right before the `if i.HasClaudeSession()`
   check), the recovered UUID is not yet in `i.claudeSession` when
   `buildLaunchCommand` reads it, so Claude launches without `--resume` even
   though the decision *log line* says "cold restoring." The recovery call
   must run **before** `initTmuxSession()`, or `initTmuxSession()` must be
   called again (or made lazy) after recovery.

## (a) Races: recovery scanning the filesystem while another goroutine/session touches the same JSONL/project dir

- `DetectByPath` does a plain `os.ReadDir` + per-entry `Stat` with no locking
  against writers (`session/history_detector.go:146-183`). Claude itself (a
  separate process) is the writer; there's no cross-process lock, so a
  concurrent read of a JSONL mid-append is possible but generally safe for
  JSON Lines (readers only care about the last complete line, and the
  recovery path only reads the *filename*, not content, so a torn write to
  content doesn't matter here — only mtime).
- Real risk is **the "picked one, but the picked one changed" window**:
  `DetectByPath` returns a `(uuid, path)` pair; nothing pins that UUID as
  "the one" atomically with the caller's next action (building `--resume
  <uuid>`, or persisting it via `SetHistoryInfo`). Between the scan and the
  actual `claude --resume` process launch, a different concurrent restart of
  the *same instance* (e.g. a duplicate `start()` call racing past `startMu`,
  or the actor's serialized command queue admitting two `Start` commands back
  to back due to a bug elsewhere) could interleave. The existing `startMu`
  lock in `start()` (`instance.go:1027-1028`) and the actor's single-goroutine
  command queue for `startLocked` are the existing mitigations — the fix must
  keep the recovery call *inside* that same serialization, not spawn it on a
  separate goroutine for "speed," or two concurrent starts could both scan,
  both decide "resume," and race to launch two `claude --resume <uuid>`
  processes against the same conversation file.
- Cross-session risk (two different `Instance`s sharing a working directory —
  e.g. two directory-mode sessions on the same repo path) is the (b) concern
  below, not really a data race in the Go sense, but a logical one: nothing
  prevents two `Instance` objects from computing the same `effectivePath` and
  both reading the same directory concurrently. That's fine for `DetectByPath`
  itself (read-only), but see (b) for why it's semantically wrong regardless
  of timing.

## (b) DetectByPath ambiguity when multiple sessions share a working directory

- This is exactly the bug `project_plans/session-resume-uuid-fix/requirements.md`
  fixed for `HistoryLinker.correlateSession`. That fix's guard
  (`!alreadyLinked || (Status not in {Paused,Hibernated,Stopped})`) is scoped
  to `HistoryLinker`, which runs continuously via polling/fsnotify — a
  fundamentally different call pattern than a one-shot recovery at
  cold-restore decision time.
- **Does this fix's "try harder to recover" loosen that guard?** Not
  structurally, if scoped correctly: the bug report's precondition is
  `HasClaudeSession() == false` (i.e., genuinely unlinked, `alreadyLinked ==
  false` in `HistoryLinker`'s terms), which is exactly the case the older fix
  says *should* use the path fallback (its own R3: "Sessions that have NO
  stored UUID... MUST still use the DetectByPath fallback"). So the two fixes
  are consistent *by construction*, provided the new call site is gated the
  same way `tryExtractConversationUUID` already is (line 310: skip if UUID
  already set).
- **Where it could go wrong**: if the implementer "helps" by also invoking
  recovery for sessions where `HasClaudeSession()` is true but the tmux pane
  is dead and the UUID "looks stale" (e.g. trying to be extra-robust against
  a corrupt in-memory UUID), that reintroduces exactly the ambiguity bug for
  a **Paused/Hibernated/Stopped, already-linked** session, since
  `tryExtractConversationUUID` has no per-session-directory ownership
  disambiguation — it just takes the newest file. Scope the new call
  strictly to the `!HasClaudeSession()` branch, matching the existing
  early-return in `tryExtractConversationUUID` itself, and do not extend it
  to "verify" an existing UUID.
- **Directory-mode sessions are the highest-risk case**, since worktree
  sessions get a unique path per session (no sharing), but two directory-mode
  sessions can legitimately point at the same `Path`/`GetEffectiveRootDir()`
  (e.g. a personal wiki opened from two different Stapler Squad sessions).
  If *both* lose their UUID simultaneously (e.g. both hit by the same
  watchdog-restart race) and both call recovery, `DetectByPath` on the same
  directory could hand both sessions the **same** recovered UUID (whichever
  JSONL is newest), silently merging two users' conversations into one
  `--resume` target. There's no code today that detects or prevents this —
  worth an explicit test case, and worth checking whether `GetEffectiveRootDir()`
  disambiguates directory-mode sessions at all (it does not appear to beyond
  the raw path).

## (c) False positives: attaching to the wrong conversation's UUID vs. starting fresh

- Per the analysis in (b), the systemic failure mode is not "detector returns
  garbage" but "detector returns *someone else's real, valid conversation*"
  — worse than fresh because it looks completely legitimate to the user
  (real messages, real tool calls, just not theirs/not what they expect next).
  A fresh start is visibly a fresh start; a wrong-conversation resume is only
  detectable by reading the content, which most users won't do until
  something is confusingly wrong.
- Requirements' Goal 3/AC3 (a user-visible signal for "lost & restarted
  fresh") does **not** currently have a symmetric counterpart for "resumed,
  but here's the confidence level of that resume" — recovered-via-fallback
  is inherently lower confidence than a UUID that was captured live via PID
  inspection (`Detect(pid)`) or persisted from `SetHistoryInfo`'s
  `HistoryLinker`-observed detection. Consider whether the acceptance
  criteria should also distinguish "resumed with a live-observed UUID" from
  "resumed with a filesystem-guessed UUID" in the visible signal, even though
  the requirements only explicitly ask for the fresh-start signal — the risk
  asymmetry (wrong-conversation-looks-right vs. fresh-looks-obviously-fresh)
  argues for treating recovered-via-path resumes as worth a lighter-weight
  note too, not just silence-on-success.
- No content-level verification exists anywhere in this path (see the
  "content not validated" point in the summary) — there's no cheap check like
  "does the last line's `cwd` field in the JSONL match this session's
  `effectivePath`" that could catch a same-directory collision before
  committing to `--resume`. Worth scoping as a possible mitigation during
  planning (Phase 3), not required by current requirements but directly
  relevant to (c)/(b).

## (d) Added latency to session revive from a filesystem scan on every cold-restore

- `DetectByPath` is a single `os.ReadDir` plus `os.Stat`-per-entry on a
  directory that in practice holds a small number of JSONL files per project
  (one per distinct conversation ever had in that path) — not expected to be
  a hot-path performance problem in isolation. The requirements' own AC2
  explicitly calls out "no added latency that matters" for the *no-recovery-needed*
  case (first-time setup / genuine fresh start) — meaning the fix must not
  call `DetectByPath` (or must make it cheap enough not to matter) even when
  it's certain to return nothing, e.g. first-time setup should not scan at
  all (it already skips this whole block — `firstTimeSetup` short-circuits
  before reaching the decision, per `instance.go:850,863,878` — so this is
  already satisfied structurally as long as the new call stays inside the
  `!firstTimeSetup && !i.pm().IsAlive()` block, not hoisted above it).
- **Where latency could bite**: this scan runs **inside the actor's serialized
  command execution** (`startLocked`/`start` hold `startMu` and, per the
  `runActor` doc comments referenced throughout `instance.go`, are on the
  single-goroutine actor command queue for that instance). A slow or hung
  filesystem (network home directory, degraded disk, a `~/.claude/projects/`
  directory with an unexpectedly large number of stale JSONL files from a
  long-lived path) would block that instance's entire actor loop — and by
  extension every other command queued for that instance — for the duration
  of the scan. There is no context/timeout passed into `DetectByPath` today
  (`session/history_detector.go:137` takes only a `projectPath string`, no
  `ctx`) so a slow scan cannot be cancelled. Not necessarily a blocker for
  this fix (the existing lazy call site has the same property), but the
  *"only after the tmux pane is confirmed dead"* framing masks that this
  already-existing risk is being moved earlier in the sequence, onto the
  critical path of every revive rather than a background lazy trigger.
- Consider whether `os.ReadDir` ordering + `Stat`-per-file cost scales badly
  for directories with hundreds of old JSONL files (e.g. a long-lived
  frequently-restarted repo path never cleaned up) — worth a quick check
  during planning of whether Claude/the project ever prunes old JSONL files,
  since this scan's cost is proportional to *all* conversations ever had at
  that path, not just recent ones.

## (e) Test flakiness risks from time-based/filesystem-based detection

- `DetectByPath`'s "most recent" logic is `ModTime`-based
  (`session/history_detector.go:189-193`), and the existing test suite
  already had to work around mtime-resolution flakiness:
  `session/history_linker_test.go:94` ("pin oldUUID 1s in the past so
  DetectByPath reliably...") and `:321` (same pattern) — i.e., tests must
  explicitly separate file mtimes by whole seconds (or use `os.Chtimes`) to
  get deterministic ordering, because two files written in the same test
  function call can land in the same filesystem mtime bucket (especially on
  filesystems with 1-second mtime resolution, or in CI containers with
  degraded clock resolution). Any new test for this fix's "recovery finds the
  right UUID" path must follow the same pattern (explicit `os.Chtimes` /
  sleep-then-write, not "write two files in a row and assume ordering") or it
  will be flaky exactly the way `.claude/rules/fix-flaky-tests-dont-defer.md`
  warns about — this is a pre-identified risk class in this exact code area,
  not a hypothetical one.
- `session/instance_workspace_test.go:166` already documents "no live tmux
  pane, so the fast path is skipped and DetectByPath runs" as a test
  precondition — a useful existing pattern to mirror for the new cold-restore
  test (mock/stub `pm().IsAlive()` to force the path-fallback branch
  deterministically rather than relying on a real dead tmux session).
- The new call sites live inside `startLocked`/`start`, which construct real
  `tmux.TmuxSession` objects and (per the surrounding code) start VNC
  displays, allocate CDP ports, etc. — heavier integration-style setup than
  the unit-level `tryExtractConversationUUID`/`DetectByPath` tests. Prefer
  testing the recovery-before-decision *ordering* at the smallest unit that
  can observe it (e.g. a table test on the extracted decision logic, if the
  fix factors it into a testable helper) rather than only asserting through a
  full `startLocked` run, to avoid flakiness from the heavier dependencies.

## (f) Recovery finds a UUID but the JSONL is stale/corrupt/truncated (partial write)

- Already covered in the Summary: `DetectByPath` never opens the file, so a
  0-byte or truncated JSONL from an interrupted write is a valid "winning"
  candidate purely on `ModTime`. If Claude's writer creates the file before
  its first flush (common for line-buffered or newly-`os.Create`d files),
  a crash between file-creation and first-write leaves an empty file that
  outranks a real, complete conversation file with an older mtime.
- **What happens downstream if this fix resumes with that UUID**: `claude
  --resume <uuid>` against an empty/corrupt JSONL is Claude CLI's problem to
  handle, not this codebase's — but this codebase already has a documented
  recovery path for "resume was rejected/stale":
  `recoverFromStaleResume` (`session/instance_claude.go:75-90`) clears
  conversation state and restarts fresh specifically to avoid "loop[ing]
  forever on the same bad UUID." Confirm (during planning, not required to
  re-derive here) that whatever triggers `recoverFromStaleResume` today
  (likely output-pattern detection of a Claude CLI error) also fires for a
  UUID recovered via this fix's new call site, not just for UUIDs that came
  from the normal `--resume` flow — otherwise a corrupt-file false positive
  from recovery could hang rather than self-heal.
- Related durability question flagged by the requirements' own Non-goals
  section ("if research finds the existing persistence path has its own bug
  ... that's a candidate finding but out of scope") — worth naming
  explicitly rather than silently working around: a truncated JSONL is
  itself evidence of exactly the "save not reaching disk before a crash"
  scenario the Non-goals section anticipates. This fix's job is not to fix
  that, but its recovery path is the first place in the codebase that would
  actually *encounter* a truncated file as a live input rather than a
  historical curiosity — worth a defensive check (e.g., treat a 0-byte or
  suspiciously-small JSONL as equivalent to "not found" rather than a valid
  candidate) even though nothing in the current code does this today.

## Design-against checklist (carried into Phase 3 planning)

1. Gate the new recovery call the same way `tryExtractConversationUUID`
   already gates itself (only when no UUID is currently set) — do not extend
   it to "double-check" an existing UUID, or (b)'s ambiguity bug returns.
2. Insert the recovery call **before** `initTmuxSession()` runs (or re-run/
   defer `initTmuxSession()` after recovery) — recovering the UUID after the
   launch command is already built is a no-op in effect. See Summary point 4.
3. Do not infer "had history before" from `HistoryFilePath`/`ConversationUUID`
   being empty at decision time — all three existing `ClearConversationState`
   call sites deliberately leave that same empty state for legitimate
   fresh-start cases (Goal 4's regression risk). Set the "unexpectedly lost"
   signal at the point of loss (e.g. only the watchdog/race path marks it),
   not inferred after the fact.
4. Keep the recovery call inside the existing `startMu`/actor serialization —
   don't parallelize it "for speed," since two concurrent starts racing past
   an unserialized scan-and-resume is a duplicate-process risk (a).
5. Treat a same-directory multi-session collision (b) as an explicit test
   case: two unlinked sessions sharing an `effectivePath`, both losing their
   UUID, must not silently converge on the same recovered UUID — or at
   minimum this risk should be named as accepted/out-of-scope in the plan
   rather than silently untested.
6. Use `os.Chtimes`-pinned mtimes (not write-order-implies-mtime-order) in
   any new test exercising "recovery picks the right/most-recent file" (e).
7. Consider a minimal sanity check on the recovered file (non-zero size, or
   at least one parseable JSON line) before trusting it as a resume target
   (f) — no such check exists in `DetectByPath` today, and this fix is the
   first caller to hit this on the critical revive path rather than a lazy
   background correlation loop.
