# Architecture Research: OSC Title/Progress as High-Priority Status Signals

Scope note: per requirements, this is a straightforward internal data-flow
integration, not a multi-actor business domain — no EventStorming table.

## 1. Data flow: raw PTY bytes → `DetectedStatus`

Two parallel byte-storage/consumption paths exist. Both originate from the
same raw PTY read loop but diverge before status detection:

```
tmux PTY (or control-mode socket)
  │
  ├─ session/response_stream.go:278-305  (readLoop, local session)
  │    data := readBuf[:n]                         ← raw bytes, escapes intact
  │    rs.onOutput()                                → IdleDetector.RecordActivity()
  │    rs.escapeParser.Parse(data, sessionSeq)       → pkg/analytics.EscapeCodeParser (gated, see §6)
  │    rs.ptyAccess.buffer.Write(data)               → session/circular_buffer.go (raw bytes stored)
  │    rs.broadcast(data)                            → WebSocket subscribers (xterm.js in browser)
  │
  └─ session/external_streamer.go:436-443  (external/mux path, same shape)
       s.buffer.Write(msg.Data); s.broadcast(msg.Data)
```

`session/circular_buffer.go` stores **raw, unstripped** bytes (OSC sequences
included) — confirmed by `PTYAccess.GetRecentOutput`/`GetRecentOutputInto`/
`GetRecentHash` in `session/pty_access.go:86-111`, which are thin wrappers
over the buffer with no filtering.

Status detection is pulled (not pushed) from that buffer on each poll tick,
in `session/claude_controller.go`:

```
ClaudeController.GetCurrentStatus() / GetStatusAndIdleInfo()   (claude_controller.go:631, 955)
  │
  tail := pa.GetRecentOutputInto(...)               ← last 4096 raw bytes (statusDetectionTailBytes)
  filtered, _ := filterTmuxMetadata(tail)             ← strips tmux status-bar lines only
  lines := lastNLines(filtered, ...)
  │
  ├─ sd.DetectWithContextFromLines(lines)             ← sd = cc.statusDetector (detection.StatusDetector)
  │     └─ detectFromLines → detectWithContextFromString → detectFromText(text, rawPTY)
  │           text := sd.normalizer.Normalize(line)    ← strips ANSI incl. OSC (ansiStripRegex, detector.go:129)
  │           ps.MatchLines(text, rawPTY)               ← rawPTY only used for hasScreenOverwrite (bare \r / cursor-up)
  │
  └─ id.DetectStateFromContent(filtered)               ← id = cc.idleDetector (detection.IdleDetector)
        status := id.statusDetector.DetectFromLines(lines)   ← a SEPARATE StatusDetector instance, same patterns
        mapStatusToIdleState(status)                          ← debounce gate lives here (see §4)
```

**Key finding — OSC content never reaches the matching pattern set.**
`ansiStripRegex` in `session/detection/detector.go:129`
(`\x1b\][^\x07]*\x07` alternative) deletes OSC sequences during
`stripANSI`/`Normalize` before `MatchLines` ever runs. The title text itself
is discarded, matching the requirements doc's problem statement exactly.
`hasScreenOverwrite` receives `rawPTY` (pre-strip) but only scans for bare
`\r` / cursor-up CSI — it has no OSC awareness.

**Key finding — `cc.statusDetector` is NOT the `binaries/claude.go`
detector.** `claude_controller.go:224` builds it via
`detection.NewStatusDetector()`, which loads `getDefaultPatterns()`
(`detector.go:304`) — a hand-duplicated copy of the same pattern set as
`ClaudeDetector.Patterns()` in `session/detection/binaries/claude.go`, not
that struct itself. The `dtypes.BinaryDetector` / `DetectorRegistry` /
`DetectForProgram` path (§3) is **not called anywhere in production code**
(`grep` shows only `session/detection/*_test.go` reference it) — it exists
but is currently dead for the live Claude session status pipeline. This
matters for acceptance criterion #2 ("threaded into the pipeline as a
distinct input reaching `binaries/claude.go`'s detector"): today nothing
reaches that file at runtime, so "the detector" in AC2 should be read as
"the Claude-specific classification logic," which can live in
`binaries/claude.go` as a plain function even though the struct method
(`Patterns()`) it currently exposes isn't the one driving
`claude_controller.go`.

## 2. `PatternSet` / `BinaryDetector` plumbing

- `dtypes.BinaryDetector` (`session/detection/dtypes/dtypes.go:29-33`): `Name() string`, `Patterns() StatusPatterns`, `FilterContent(content string) string`. Five implementations (claude, gemini, aider, opencode, agy) — all `FilterContent` are no-ops except presumably future ones.
- `session/detection/registry.go`: `DefaultRegistry()` registers all five into a `DetectorRegistry` (name → `BinaryDetector`).
- `session/detection/detector.go:733-746`: `builtBinaryDetectors` is a package-level map built once from `DefaultRegistry()`, each wrapped in its own `*StatusDetector` (compiled `PatternSet`). Consumed only by `DetectForProgram` (`detector.go:753`), which itself has no production callers.
- `PatternSet.MatchLines(text string, rawPTY []byte)` (`pattern_set.go:69`) already takes **two** inputs — normalized text for regex matching, and raw bytes for the screen-overwrite heuristic. This is direct precedent in this codebase for "thread a second raw-bytes-derived signal alongside the text signal through the same call chain" — the natural place to add a third: OSC title string.

## 3. Registry/dispatch

`session/detection/registry.go` — `DefaultRegistry()` is the only
registration point; confirmed no other runtime dispatch exists
(`BinaryDetector` values are never looked up by program name outside
`DetectForProgram`, which is unused). No changes needed here unless the
recommendation is implemented as a `BinaryDetector` interface extension
(not recommended — see §5).

## 4. Existing debounce/stabilization mechanism

Lives entirely in `session/detection/idle.go`, **not** in
`detector.go`/`pattern_set.go`. `IdleDetector` (`idle.go:48-64`) holds
`currentState IdleState`, `lastStateChange time.Time`, and
`config.DebounceDelay` (default 500ms, `idle.go:34`).

The gate, duplicated identically in `DetectState()` (idle.go:141-147,
deprecated) and `DetectStateFromContent()` (idle.go:173-179, the live
path called from `claude_controller.go:911,1050`):

```go
newState := id.mapStatusToIdleState(status)
if newState != id.currentState {
    if id.currentState == IdleStateUnknown || id.timeNow().Sub(id.lastStateChange) >= id.config.DebounceDelay {
        id.currentState = newState
        id.lastStateChange = id.timeNow()
    }
}
return id.currentState
```

I.e., a state change is accepted immediately only if the current state is
`IdleStateUnknown`; otherwise it's held until 500ms have elapsed since the
last accepted change — pending state is simply dropped (not queued), so a
transition that arrives during the debounce window and doesn't recur is
lost. `DetectedStatus` (from `detector.go`/`pattern_set.go`) and
`IdleState` (from `idle.go`) are two different enums;
`mapStatusToIdleState` (idle.go:185-239) converts one to the other and is
where `lastActivity` gets bumped for active-ish statuses.

Separately, `session/claude_controller.go:900-917` and `:978-999` also
cache the mapped status/idle-state keyed by a hash of the 4096-byte tail
(`statusCache`/`idleCache`, `atomic.Pointer`) — this is a content-hash
memoization, not a time debounce, but it means an OSC bypass must either
(a) participate in that same hash (OSC title bytes must be part of what's
hashed, or a separate cache key), or (b) be read fresh every call outside
the cache. Since the OSC title is carried in the same raw tail bytes
already fed to `GetRecentHash`, option (a) falls out naturally *if* the
title is parsed from that same tail slice rather than from a
separately-tracked "last seen title" variable — see §5 Option A vs Option
C tradeoff.

**Bypass integration point:** the debounce gate is a single `if` in
`idle.go`, called from exactly two methods. The cleanest bypass is a
sibling method/parameter that skips the `DebounceDelay` check — e.g.
`DetectStateFromContent(content string, immediate bool)` (or a wrapper
`ForceState(state IdleState)` that only touches
`currentState`/`lastStateChange`, no pattern matching) — rather than
threading a bypass flag through `mapStatusToIdleState` and every
`DetectedStatus` case, since only two callers need to opt in.

## 5. Integration point options

### Option A — New parameter threaded through the existing text+raw-bytes call chain (recommended)

Add OSC title as a third input alongside `text`/`rawPTY`, mirroring the
existing `MatchLines(text, rawPTY)` precedent:

1. New free function in `session/detection/binaries/claude.go` (not a
   `BinaryDetector` interface method — see rejection rationale below):
   ```go
   // ClassifyOSCTitle inspects a Claude Code OSC window-title string and
   // returns a definitive status if the title contains an unambiguous
   // spinner/idle marker, or ok=false if the title is empty/unrecognized.
   func ClassifyOSCTitle(title string) (status dtypes.OSCStatus, ok bool)
   ```
   `dtypes.OSCStatus` is a new tiny enum in `dtypes.go`
   (`OSCStatusExecuting`, `OSCStatusIdle`, `OSCStatusNone`) — kept in
   `dtypes` (not `detection`) for the same import-cycle reason every other
   shared type lives there, and mapped to `detection.DetectedStatus` /
   `detection.IdleState` by the caller in `session/`.
2. Extraction: a small dedicated regex/scanner (not reusing
   `pkg/analytics.EscapeCodeParser` — see Option C rejection) run in
   `session/response_stream.go`'s existing read loop
   (`response_stream.go:278-305`), same call site as
   `rs.escapeParser.Parse(data, sessionSeq)`, storing the latest OSC-0/2
   title into an `atomic.Pointer[string]` on `ResponseStream` (or
   `PTYAccess`). Cheap: one pass over each chunk, same place raw bytes are
   already being touched.
3. `claude_controller.go`'s `GetCurrentStatus`/`GetStatusAndIdleInfo`
   read that title alongside the tail bytes, call
   `binaries.ClassifyClaudeOSCTitle`, and if it returns a definitive
   status, use it directly — skipping `sd.DetectWithContextFromLines` for
   that field — then pass an "immediate" flag into `IdleDetector` (§4
   bypass) so the debounce gate is skipped for that transition only.
4. Tradeoff: introduces a second small parser (title extraction) alongside
   the existing one in `pkg/analytics`; acceptable because that one is
   config-gated (§6) and pulling OSC out of it would be riskier than a
   ~15-line dedicated scanner.

### Option B — New parallel signal channel (rejected)

A pub/sub channel from the PTY read loop to a dedicated "OSC status"
goroutine feeding `ClaudeController` asynchronously. Rejected: adds a
concurrency surface (another goroutine + channel lifecycle to manage
alongside `runStatusChangeLoop`, `debounceCaptures`, etc.) for a value that
is naturally pull-based today (status is polled from the tail buffer on
demand, not pushed). The existing `statusCheckCh` signal
(`claude_controller.go:318-322`) already exists to trigger a poll on new
output — reuse that trigger rather than adding a second async pipe.

### Option C — Extend `dtypes.BinaryDetector` interface with an OSC method (rejected)

E.g. `ClassifyOSCTitle(title string) (StatusHint, bool)` added to the
`BinaryDetector` interface itself. Rejected per
`.claude/rules/interface-pollution-checklist.md`: although
`BinaryDetector` already has 5 implementations (not "speculative" in the
literal single-implementation sense), the requirements' own Non-Goals
section explicitly excludes "extending to binaries other than Claude
Code" — so 4 of the 5 implementations (`gemini`, `aider`, `opencode`,
`agy`) would get a dead no-op method solely to satisfy the interface,
which is exactly the forwarding-only/speculative-surface smell the
checklist flags. A free function in `binaries/claude.go`, called directly
by Claude-specific code (`claude_controller.go` is already
Claude-only — it hardcodes Claude patterns), avoids polluting the shared
interface and needs no registry/dispatch change.

### Reusing `pkg/analytics.EscapeCodeParser` for OSC extraction (considered, rejected as primary source)

`session/response_stream.go:294-296` already runs
`rs.escapeParser.Parse(data, sessionSeq)` on every raw PTY chunk, and
`pkg/analytics/escape_code_parser.go:451` (`parseOSC`) already implements
correct BEL/ST-terminated OSC boundary detection — attractive to reuse
rather than writing a second scanner. **However**, `Parse()`
(`escape_code_parser.go:163-168`) no-ops whenever
`p.captureLevel == "off"`, and `newEscapeParserForSession`
(`response_stream.go:56-73`) sets `captureLevel = "off"` whenever the
global escape-event writer is a `NoopEscapeEventWriter` (i.e. **no
analytics DB configured** — the common case for most deployments/dev
setups). That means today this parser silently does zero work for most
sessions. Depending on it for status detection would make a
correctness-relevant feature (false-idle prevention) contingent on an
unrelated, optional analytics subsystem being configured — a hidden
coupling. Decoupling `EscapeCodeParser`'s OSC-extraction from its
capture-level gate is a reasonable follow-up refactor but is
out-of-scope/riskier for this feature; Option A's small dedicated scanner
avoids the coupling entirely.

## Recommendation

Option A: dedicated lightweight OSC-title scanner at the
`response_stream.go` read-loop call site (parallel to, not reusing,
`escapeParser.Parse`), a plain `ClassifyOSCTitle` function in
`binaries/claude.go` (not an interface method), a new small `dtypes.OSCStatus`
enum to avoid import cycles, and a debounce-bypass entry point added
alongside (not inside) the existing gate in `idle.go`'s
`DetectStateFromContent`. This satisfies AC1–AC7 without touching the
`BinaryDetector` interface or the currently-dead `DetectorRegistry`/
`DetectForProgram` path, and without taking on a hidden dependency on the
analytics subsystem's capture-level config.
