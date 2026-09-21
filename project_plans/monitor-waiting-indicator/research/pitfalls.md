# Pitfalls: detecting Claude Code's "N shell(s)/N monitor(s)" status footer

## Load-bearing finding: this is largely already built

Before listing pitfalls, note that most of this feature already exists in
`session/detection/detector.go` and `session/detection/binaries/claude.go`. Any
new work should extend these, not duplicate them:

- **`session/detection/detector.go:35`** — `autoModeFooterRegex` already
  parses the persistent bottom-of-pane footer: `` `auto mode on\s*·\s*(\d+)\s+shells?(?:,\s*(\d+)\s+monitors?)?` ``,
  matching e.g. `"⏵⏵ auto mode on · 2 shells, 1 monitor · ← for agents"`.
- **`footerAgentCount`** (detector.go:40) scans lines bottom-up, sums shell +
  monitor counts, returns `(count, desc, ok)`.
- **`applyFooterIdleOverride`** (detector.go:68) promotes `StatusIdle`/`StatusUnknown`
  to `StatusWaitingForAgent` using that count — but explicitly *never*
  overrides a more urgent status (Error/NeedsApproval/Executing).
- **`session/detection/binaries/claude.go:361-379`** — a second, distinct
  pattern pair (`shells_still_running`, `monitors_still_running`) matches the
  *mid-turn* spinner-line variant: `"✻ Churned for 52s · 1 shell still
  running"` — different from the always-present footer bar.
- UI already renders this: `web-app/src/components/sessions/SubStatusChip.tsx`
  has a `chipWaitingForAgent` chip driven by the count.
- Proto mapping already exists: `session/detection/proto_mapping.go` maps
  `StatusWaitingForAgent`.

If the backlog item is asking for something not covered by this, it's likely:
(a) the polling-mode path not re-running this scan as often/at all, (b) tmux
control-mode raw stream not having the footer line surfaced before
ANSI-normalization, or (c) the item's literal wording appearing in scrollback
(the item's own description) causing a spurious match — see #3 below. Verify
which gap actually motivated the item before assuming a parser needs to be
written from scratch.

## 1. Parsing brittleness (CLI wording is not a stable API)

- The repo already treats this defensively: `autoModeFooterRegex`'s doc
  comment states it was "verified against a live pane capture of a real
  session" — i.e., empirically confirmed, not guessed — and is kept **out of**
  the main `PatternSet`/`StatusPatterns` priority chain specifically so a
  format change here can't silently swallow every other status.
- The established drift-detection idiom in this codebase is a **canary**:
  `session/detection/ratelimit/detector.go`'s `maybeLogUndetectedWording`
  (fires a one-time `log.Warn` if a generic keyword like "rate limit" appears
  but none of the specific regexes matched) and `detector.go`'s
  `compactingCanary` (same idea for compaction wording, explicitly marked
  TEMPORARY with a removal note). Any new shell/monitor parsing should add an
  analogous canary: if a line contains `"shell"` or `"monitor"` near `"auto
  mode"` or `"running"` but the specific regex doesn't match, log once per
  session — this is how a future Claude Code CLI rewording gets caught instead
  of silently going idle-forever or never-idle.
- Regex tolerance: the existing pattern already handles optional pluralization
  (`shells?`, `monitors?`) and an optional second clause
  (`(?:,\s*(\d+)\s+monitors?)?`) — keep that shape; do not hardcode both
  clauses as always-present or always in that order.
- Never let this detector's failure mode be "crash" — `FindStringSubmatch`
  returning `nil` must always degrade to "no override," never a panic on a
  nonexistent capture group. `footerAgentCount` already does this correctly
  (`m == nil` → `continue`).

## 2. ANSI / box-drawing stripping

The repo has **at least four separate ANSI-stripping implementations** —
consolidate onto the newest, don't add a fifth:

- `pkg/ansi` (`ansi.StripCSI`, referenced by `session/detection/ratelimit/detector.go`
  and imported by `session/detection/detector.go`) — the shared package,
  prefer this for any new detector code.
- `session/tmux/banner_filter.go`'s `stripANSICodes` — has a
  zero-alloc fast path for plain text (`TestStripANSICodes_ZeroAllocsOnPlainText`)
  and its own benchmark; this is the pattern to copy if perf matters (it will,
  see #5).
- `session/review_transcript.go`'s `stripANSI`/`reviewTranscriptANSIRegex`.
- `session/instance_claude.go`'s `stripANSISimple`.

Gotchas specific to the footer line:
- The footer is drawn with a horizontal-rule/box-drawing separator above it
  (`session/tmux/tmux.go`'s `filterStatusLine`/`isStatusLine` treats a
  run of `─` as a tmux status-line marker) — a naive regex over raw pane text
  will see `──────...⏵⏵ auto mode on · 2 shells...` on possibly the *same*
  line as trailing box-drawing, not just before it. Strip ANSI **and** don't
  assume the footer occupies its own clean line; `autoModeFooterRegex` avoids
  this by matching a substring (`auto mode on\s*·...`), not anchoring `^...$`.
- `·` (U+00B7, middle dot) and the `⏵⏵` glyphs are multi-byte UTF-8 — any
  future line-splitting/truncation logic must not slice mid-rune (see
  `session/tmux/tmux.go:3308`'s explicit UTF-8-safe handling in its own ANSI
  scanner for the established idiom).
- `capture-pane -e` (used by `external_tmux_streamer.go:477`) preserves color
  codes; `capture-pane -p` without `-e` (used elsewhere, e.g.
  `session/mux/multiplexer.go:445`) does not. Whichever capture path feeds
  this detector determines whether ANSI-stripping is even necessary — check
  which flag the actual call site uses before assuming escape codes are
  present at all.

## 3. False positives / false negatives — anchor to the *live* status bar

This is the sharpest risk, and the task description itself demonstrates it:
the literal string `"N shell, N monitor still running"` appearing in a
backlog item's own title/description, if that description is ever displayed
inside a Claude Code session's transcript (e.g. pasted into a prompt, echoed
back by the agent, or shown in a code block during an unrelated task), would
match a naive `regexp.MatchString` over full scrollback with no anchoring.

How the existing code avoids this:
- **Scans backward from the bottom, stops at first match** — `footerAgentCount`
  iterates `for i := len(lines) - 1; i >= 0; i--`, so it finds the most recent
  occurrence, not the first. This isn't full anchoring to "the actual live
  status bar position" though — it's anchoring to *recency*, which is a good
  proxy but not airtight (see below).
- **`statusDetectionLinesWindow`** (`session/claude_controller.go:756`,`:1072`)
  bounds the scan to only the last N lines via `lastNLines`, not full
  scrollback — this is the real anchor: old scrollback (including a pasted
  backlog description from minutes ago) scrolls out of the window entirely.
  Any new detection logic MUST reuse this windowing, not scan the full
  captured buffer.
- **Never override a more specific/urgent status** — `applyFooterIdleOverride`
  only fires when the rest of the scan already concluded Idle/Unknown. A
  match inside a code block being actively discussed would, in practice,
  co-occur with other active-turn signals (spinner text, "esc to interrupt")
  that take priority in the per-line chain and would already win before the
  footer override is even consulted.
- **Residual risk**: if the false-positive text appears on a line *within*
  the trailing N-line window while the pane is genuinely idle with no other
  signal (e.g. Claude just printed a code sample containing the exact footer
  wording, then returned to an idle prompt within the window), the override
  could still fire. This is a real, not just theoretical, gap — worth a
  targeted test case (render the fixture from `waitingForAgentFooterContent`
  in `session/review_queue_determiner_test.go:671` but with the count-bearing
  text inside a quoted/code-fenced block above the real prompt, and assert no
  false positive). A tighter anchor to consider: require the footer's
  known-fixed neighboring bar characters (`⏵⏵`, `← for agents`) rather than
  just the shells/monitors count clause alone, since those are far less
  likely to appear in ordinary transcript content.

## 4. Race conditions — Snapshot() vs raw field reads

- `.claude/rules/instance-lock-free-reads.md` (this repo, glob-scoped to
  `session/instance*.go`) mandates reading mutable `*Instance` fields through
  `Snapshot()`, never the raw field, because actor setters mutate under
  `i.mu.Lock()` from background goroutines while other goroutines read
  concurrently (`go test -race` already caught exactly this class of bug once,
  per that doc's postmortem).
- **The existing shell/monitor-count code sidesteps this entirely** — worth
  preserving that design, not "fixing" it into a stored field. `footerAgentCount`
  is computed fresh, on every call, from the PTY content string passed in;
  nothing is cached on `*Instance`. `StatusContext` similarly flows through
  `session/instance_status.go`'s `GetStatus()` computed on demand
  (`info.StatusContext = statusContext`), never stored as a raced field. This
  is why it needs no `Snapshot()` plumbing today.
- **The risk is introduced if a UI-indicator feature adds a cached field**
  (e.g. `i.shellMonitorCount int` set by a background poller, read elsewhere
  for rendering) instead of recomputing on read. If that's the design chosen:
  add the field to `InstanceSnapshot` (`session/instance_snapshot.go` — "the
  single authoritative field list," per the project rule) and read it via
  `Snapshot()`, exactly like `GitHubApprovedCount`/`GitHubChangesReqCount`
  already do in that same struct. Do not add an ad hoc `i.mu.RLock()` accessor
  for this — the rule reserves that only for fields deliberately excluded from
  the snapshot (manager/dependency objects), and a plain int count doesn't
  qualify.
- Prefer the "recompute on read" design if at all workable — it's what the
  rest of this exact feature area already does, avoids adding another
  snapshot field, and eliminates a whole class of staleness bugs (count stuck
  at N after shells actually exit, if the background write path has any gap).

## 5. Performance — full-scrollback re-parse vs incremental

- Control mode path (`external_tmux_streamer.go`'s `readControlMode` /
  `debounceCaptures`) already **debounces rapid `%output` events into a
  single capture-pane call** rather than firing a capture per byte received —
  reuse this debounce, don't add a second, competing one.
- Legacy polling path (same file's capture-pane polling fallback, `pollInterval`,
  default 500ms) and `session/tmux_process_manager.go`'s **capture-pane
  cache** (`cacheCaptureContent`, guarded by its own `mu`) exist specifically
  to avoid "a capture-pane subprocess on every single poll cycle indefinitely"
  (see `session/review_queue_poller.go:706`'s comment on the identical
  concern for its own poller) — any new per-tick shell/monitor scan should key
  off the *existing* cached/debounced capture, not add its own independent
  capture-pane invocation per session per tick.
- The actual regex work itself is cheap (`autoModeFooterRegex` is one
  `FindStringSubmatch` per line, bounded by `statusDetectionLinesWindow`, not
  full scrollback) — the real per-session-count cost at scale is the
  **subprocess fork** for capture-pane, not the parsing. `TestStripANSICodes_ZeroAllocsOnPlainText`'s
  existence signals allocation-per-scan is already a tracked concern here too;
  don't introduce a second full-string ANSI-strip pass if one has already run
  upstream in the same tick (check what `detectFromLines`'s caller already
  normalized before adding a redundant strip).
- With many concurrent sessions, prefer computing the shell/monitor count as
  part of the *existing* per-tick status-detection pass (which already runs
  once per session per tick) rather than a second independent
  scan/goroutine/timer per session — that would multiply subprocess/lock
  contention linearly with session count for no new information (the footer
  line is already inside the same captured content the status detector reads).

## 6. Testing pitfalls — no real tmux, no real sleeps

- `session/review_queue_determiner_test.go:671` already has the exact fixture
  to model new tests on: `waitingForAgentFooterContent`, a plain Go string
  constant containing the footer text, fed directly into
  `determiner.Determine(inst, waitingForAgentFooterContent, statusInfo,
  detector)` — no tmux process, no PTY, no sleep. Any parsing-logic test
  should follow this exact shape: hardcode the captured-pane string (ANSI
  already stripped or not, per what's being tested) and call the detection
  function directly.
- Per the `deterministic-fast-tests` skill and `fix-flaky-tests-dont-defer`
  skill (both referenced in this repo's CLAUDE.md): do not spin up a real
  tmux session with a real Claude process and poll/sleep waiting for the
  footer to appear — that's exactly the flaky-by-construction pattern those
  skills warn about. If an integration-level test is genuinely needed for the
  control-mode vs. polling-mode wiring (not just the regex), fake the
  capture-pane/control-mode data source (there's already a `piCommandFactory`-style
  seam pattern in `session/pi_status_source.go` for substituting a fake
  subprocess in tests — mirror that: inject a fake pane-content source rather
  than a real `exec.Cmd`/`tmux` binary).
- Cover both wording variants as distinct test cases, since they're genuinely
  different regexes today: the persistent footer (`autoModeFooterRegex`,
  "auto mode on · N shells...") and the mid-turn spinner line
  (`shells_still_running`/`monitors_still_running` in `binaries/claude.go`,
  "✻ Churned for 52s · 1 shell still running"). A test suite that only
  fixtures one of the two will pass while the other silently regresses.
- Add the false-positive regression case named in #3 (footer wording present
  in transcript content but outside the live status-bar window / superseded
  by an active-turn signal) as an explicit test, not just an informal check —
  it's the one path that doesn't have an existing test to model after.
