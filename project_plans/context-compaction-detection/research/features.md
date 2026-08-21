# Research: Feature Landscape — Context-Compaction Detection

## 1. Precedent: how `StatusWaitingForAgent` was added (the closest prior art)

`StatusWaitingForAgent` is the most recent precedent for adding a brand-new
`DetectedStatus` value (PR #115, commit `079de957c`, 2026-06-15, later refined in
`da994e589`/#121 and `bb85cf198`). Tracing it end to end shows the **full touchpoint
list is larger than requirements.md's Existing-System section implies** — see
§4 below for the gap analysis.

Key precedent decisions worth reusing:

- **New pattern group, not a reused one.** `WaitingForAgent` got its own field in
  `dtypes.StatusPatterns` (`session/detection/dtypes/dtypes.go:15-23`) rather than
  being folded into `Active`/`Processing`. Compaction should follow the same shape —
  a distinct `Compacting` pattern group, not a flag on `Active`.
- **Priority placement was deliberate and documented.** In
  `session/detection/pattern_set.go:99-104`, `WaitingForAgent` is checked
  **before `Success`**, with an inline comment explaining exactly why: a line like
  `"✻ Churned for 52s · 1 shell still running"` would otherwise match the `Success`
  verb-duration pattern first. This is the template to follow for the new state:
  whatever regex Claude Code prints during compaction must be checked against every
  existing pattern group for accidental overlap, and the winning priority slot must
  carry the same kind of inline "why here, not there" comment.
- **The commit added a Go test using a new fixture file**, following exactly the
  pattern AC4 asks for (`session/detection/testdata/*.txt` + `detector_test.go`).

## 2. Industry precedent: maki-agent's `AgentEvent::AutoCompacting`

Confirmed via `gh api search/code` against `tontinton/maki`
(https://github.com/tontinton/maki — a Rust TUI coding agent unrelated to
Anthropic):

- `maki-agent/src/types.rs:561-562` — `AgentEvent` enum has dedicated
  `AutoCompacting` and `CompactionDone` variants.
- `maki-agent/src/agent/run.rs:476-478` — the agent's own control loop fires
  `AgentEvent::AutoCompacting` *before* calling `do_compact()`, i.e. it's an
  **intentional, structured signal the agent emits about itself**, not something
  inferred from parsing its own terminal output.
- `maki-ui/src/chat.rs:109-118` — the UI reacts to that structured event by pushing
  a literal `"Auto-compacting conversation..."` display message, and clears/flushes
  on `CompactionDone`.

**Why this precedent only partially transfers:** maki controls its own agent loop,
so it can emit `AutoCompacting`/`CompactionDone` as first-class, unambiguous
lifecycle events with a clean start/end boundary. stapler-squad does not control
Claude Code's internals — it can only regex-scrape whatever text Claude Code
happens to print to its own PTY, with the same false-positive/missed-window risks
every other `DetectedStatus` pattern already has. The architectural takeaway is
narrower than "add an enum variant": maki's clean `AutoCompacting → CompactionDone`
pair is the *ideal* we can't fully get — stapler-squad will detect only "compaction
text is currently visible," not a guaranteed start/end transition, since there is
no equivalent of a `CompactionDone` event to key off unless Claude Code also prints
a distinct "compaction finished" string (unconfirmed — same gap as AC1's open
question).

No public source for the *exact* Claude Code terminal string during active
compaction was found. A community research gist
(https://gist.github.com/badlogic/cd2ef65b0697c4dbe2d13fbecb0a0a5f, compares Claude
Code/Codex CLI/OpenCode/Amp compaction mechanisms) and multiple
`anthropics/claude-code` GitHub issues (e.g. #5243, #17808, #26518) confirm the
*mechanism* (auto-triggers near 95% context capacity, replaces history with an
LLM-generated summary) but never quote the literal in-progress status-line text.
**This reinforces requirements.md's own flagged open question (AC1): the
`Compacting context...` / `[context window compressed]` phrasing in the original
issue is still unverified and must come from live/recent Claude Code output before
any pattern is written.**

## 3. False-positive risk: the existing "N% until auto-compact" indicator

Quantified check against the current fixture corpus
(`session/detection/testdata/*.txt`): **3 of the 26 Claude fixtures already contain
the literal word "compact"** — `claude_active.txt:5`, `claude_thinking_verb.txt:5`,
`claude_asterism_active.txt:5`, all reading
`esc to interrupt   10% until auto-compact` (a 4th match, `opencode_input_required.txt`,
is an unrelated `compact` command-palette menu entry for a different binary).

This means the approaching-threshold indicator is present on **essentially every
normal active Claude turn**, not an edge case. Any regex looser than an exact match
on the confirmed in-progress string (e.g. a naive `(?i)compact`) would immediately
regress 3 existing fixtures from `Executing`/`Processing` to the new `Compacting`
status the moment implementation lands — a direct violation of AC7's "additive
only" requirement. The new pattern must positively distinguish "N% until
auto-compact" (approaching, still `esc to interrupt`-driven `Active`) from whatever
the actual in-progress string turns out to be — e.g. by requiring the confirmed
string's exact wording and *rejecting* lines that also contain a `%` + `until`
structure, or by scoping the match to a line that lacks `esc to interrupt` entirely
(compaction likely isn't interruptible the same way, but that's also unconfirmed).

## 4. Gap: requirements.md undercounts the touchpoints (the actual blast radius)

AC2/AC3 name `detector.go`, `binaries/claude.go`, and `proto_mapping.go`. Tracing
the real data flow from `DetectedStatus` to the rendered badge surfaces **four
additional files that must change**, three of which are duplicate/parallel
switches not mentioned anywhere in requirements.md:

1. **`session/detection/dtypes/dtypes.go:15-23`** — `StatusPatterns` struct has one
   fixed field per status group (`Ready`, `Processing`, ..., `WaitingForAgent`).
   A new `Compacting` pattern group needs a new field here first; `claude.go`
   can't declare compaction patterns without it existing on the struct.
2. **`session/detection/pattern_set.go`** — three separate spots, not one:
   the `PatternSet` struct's regex-slice fields (`:11-22`), the `compile()`
   groups table (`:39-56`), and a new priority slot inserted into the
   `MatchLines()` if-chain (`:71-141`) — this last one is the actual behavioral
   decision (§1 above).
3. **`server/adapters/instance_adapter.go:240-272`,
   `toProtoSubStatusFromInfo`** — this is the function *actually wired* to the
   outgoing `SubStatus` proto field (called from `instance_adapter.go:167`). It
   duplicates the `switch` in `proto_mapping.go`'s `DetectedStatusToSubStatus`
   almost line for line, **despite that function's own doc comment reading "this
   is the single authoritative mapping; do not duplicate this logic in adapters
   or converters — call this function instead."** A repo-wide grep confirms
   `DetectedStatusToSubStatus` (`proto_mapping.go:44`) has **zero call sites**
   outside its own file and tests — it is dead/duplicate code, and
   `toProtoSubStatusFromInfo` is the one that actually needs the new
   `case detection.StatusCompacting:` for the badge to appear via the primary
   (`SubStatus`) path. Missing this file specifically would make the feature
   silently no-op in production despite `detector.go`/`proto_mapping.go`/tests
   all being "correctly" updated — the frontend's `SubStatusChip` (see below)
   would just never receive the new value.
4. **`proto/session/v1/types.proto`** needs a new value in **both** the
   `DetectedStatus` enum (per AC3) and the separate `SubStatus` enum — the two are
   independent enums, and `deriveWorkingState.ts`'s primary switch keys off
   `SubStatus`, falling back to `DetectedStatus` only when `SubStatus` is
   `UNSPECIFIED` (`deriveWorkingState.ts:27-43`). Adding only a `DetectedStatus`
   value (as AC3's literal wording suggests) leaves the state reachable solely via
   the fallback path, which the requirements don't call out as intentional.
5. **`web-app/src/components/sessions/SubStatusChip.tsx:30-169`** is the actual
   badge-rendering component the AC6 "session card UI shows a visually distinct
   badge/spinner" targets — not `SessionCard.tsx` as the Existing-System section's
   generic "Session card component(s)" phrasing might suggest. It renders per
   `SubStatus` (not `WorkingState`/`DetectedStatus` directly), and its `default`
   case uses an `_exhaustive: never` compile-time guard (`:157-166`) — a genuinely
   useful safety net: forgetting this file is a **TypeScript compile error**, not a
   silent gap, once the `SubStatus` enum gains the new value. This mirrors the
   `assertNever` guard already in `deriveWorkingState.ts:65` for `DetectedStatus`.

Net effect: the real touchpoint list is roughly double what AC2/AC3's prose implies
— `dtypes.go`, `pattern_set.go` (×3 spots), `detector.go`, `claude.go`,
`proto/session/v1/types.proto` (×2 enums), `proto_mapping.go`,
**`instance_adapter.go`** (the one that actually matters at runtime),
`deriveWorkingState.ts`, and `SubStatusChip.tsx`. The plan phase should enumerate
these explicitly rather than relying on AC3's shorthand, specifically flagging
`instance_adapter.go` since it's the one place a correct-looking implementation
could still ship a no-op badge.

## 5. Other binaries: no analogous state found

`grep`ing all binary detectors (`session/detection/binaries/{aider,opencode,gemini,agy}.go`)
for "compact" turns up nothing except the unrelated opencode command-palette entry
noted in §3. `aider.go:26` even declares an empty `Active: []dtypes.StatusPattern{}`
— Aider has no PTY-visible active-state signal at all today, let alone a
compaction one. This confirms the requirements' Non-Goal ("out of scope unless
research finds it's trivially the same mechanism") — it is not trivially the same
mechanism; no other binary has any surfaced compaction indicator to key a pattern
off, and each would need independent verification against live output the same way
Claude does. Scope confirmed as Claude-only.

## 6. TOML plugin system as a possible faster/lower-risk path

`session/detection/plugins.go` (added in `3c25e94f9`, "user-extensible TOML
agent-detector plugins") lets users drop a TOML file into `<config-dir>/detectors/`
to override a built-in binary's *pattern text* without a rebuild. This does **not**
help here: the TOML plugin system overrides pattern strings within the existing
fixed `StatusPatterns` struct fields (`dtypes.go:15-23`), it cannot add a wholly new
status *group* (there's no `Compacting` field for a plugin to target) or a new
`DetectedStatus`/`SubStatus` enum value. It's mentioned here only to rule it out as
a shortcut: given AC1's string is still unverified, hot-reloadable TOML overrides
*would* be useful for iterating on the regex itself once the base `Compacting`
group exists in code — but the initial enum/struct/proto plumbing is unavoidably a
compiled-code change.

## 7. Edge cases and failure modes to carry into planning

- **Regex/priority overlap (confirmed risk, quantified in §3).** The compaction
  pattern must not fire on the existing "N% until auto-compact" line already
  present in ~12% of the Claude fixture corpus (3/26).
- **Missed transient message (PTY snapshot mechanics, confirmed via code trace).**
  Detection reads a 4096-byte tail / last-15-lines window of a circular buffer,
  triggered on `pty.Read()` with a 100ms deadline, deduped by content hash
  (`session/detection/detector.go:241`, `session/claude_controller.go:64,643-711`).
  A message that both appears *and* is overwritten within a single ~100ms read
  cycle could be missed entirely — but since compaction reportedly lasts 30-60s
  (per requirements.md's Problem section), the *sustained* in-progress message is
  very unlikely to be a sub-100ms flash; the higher-probability miss is the exact
  moment of the state *transition* (Active → Compacting or Compacting → Active)
  landing between two reads, which only matters if a future consumer needs precise
  timing/duration — not for the AC's "show a badge while it's happening" bar.
- **No `CompactionDone` signal exists to key off (see §2).** Unlike maki-agent,
  there's no confirmed distinct "compaction finished" string yet either. Without
  one, the compacting badge will naturally clear only because the *next* PTY read
  no longer matches the compacting pattern (falls through to whatever state the
  post-compaction screen shows) — which is actually fine and matches how every
  other transient `DetectedStatus` (e.g. `Executing`) already works, but should be
  named explicitly as "no explicit end-of-compaction detection, state clears
  implicitly" rather than assumed to need its own end-event.
- **Non-Claude binaries: none applicable today (§5)** — no code changes needed
  outside Claude's detector, confirmed rather than assumed.
- **Unstated need — duration/history tracking:** requirements.md's ACs describe a
  badge only (matching the issue's own framing: "session is not blocked... it's
  self-managing"). There is no existing precedent in this codebase for tracking
  *duration* of a transient `DetectedStatus` (e.g. no "how long has this session
  been `Executing`" stored anywhere found in `session/detection/` or
  `server/services/`) — a "compacting for Ns" duration counter would be new
  infrastructure, not a small addition, and should be treated as explicitly out of
  scope unless separately requested; it isn't implied by any AC.
- **Unstated need — toast/notification vs. badge only:** every other transient
  `DetectedStatus` (`WaitingForAgent`, `NeedsApproval`, etc.) is badge-only, no
  toast — `SubStatusChip.tsx` has no companion notification dispatch anywhere in
  its diff history. A toast for compaction would be inconsistent with how every
  comparable state is currently surfaced and isn't asked for by any AC; recommend
  treating it as an explicit non-goal in the plan rather than silently adding it.

## Sources

- Repo git history: `git log --oneline --all -- session/detection/detector.go`
  (commit `079de957c`, PR #115, `da994e589` PR #121, `bb85cf198`)
- `session/detection/detector.go`, `session/detection/pattern_set.go`,
  `session/detection/dtypes/dtypes.go`, `session/detection/binaries/*.go`,
  `session/detection/proto_mapping.go`, `server/adapters/instance_adapter.go`,
  `web-app/src/lib/utils/deriveWorkingState.ts`,
  `web-app/src/components/sessions/SubStatusChip.tsx`,
  `session/detection/testdata/*.txt` (all read directly in this repo)
- [maki-agent (tontinton/maki) — types.rs](https://github.com/tontinton/maki/blob/main/maki-agent/src/types.rs#L561) and [run.rs](https://github.com/tontinton/maki/blob/main/maki-agent/src/agent/run.rs#L476), [maki-ui/src/chat.rs](https://github.com/tontinton/maki/blob/main/maki-ui/src/chat.rs#L109), found via `gh api search/code`
- [Context Compaction Research gist (badlogic)](https://gist.github.com/badlogic/cd2ef65b0697c4dbe2d13fbecb0a0a5f) — compares Claude Code/Codex CLI/OpenCode/Amp compaction mechanisms, no literal in-progress string
- `anthropics/claude-code` GitHub issues referencing compaction behavior (#5243, #17808, #26518, #23751) — confirm mechanism/threshold, not the literal terminal string
