# Build vs. Buy: Context-Compaction Detection

## Question

Should "detect Claude Code is actively auto-compacting context" be built by
extending the existing hand-rolled PTY regex package
(`session/detection/`), or sourced/adapted from something else?

## Option 1: Existing OSS library for "detect AI CLI state from terminal output"

**Search performed:** WebSearch for terminal-output/PTY-based AI-CLI state
detection libraries; none found. This is a narrow, tool-specific problem
(parsing one vendor's spinner/status-line text) with no generic ecosystem —
unsurprising, and the requirements doc already assumed this.

- Pros: N/A — nothing exists.
- Cons: N/A.
- **Verdict: Not applicable / not recommended** (nothing to adopt).

### Adjacent finding: Claude Code hooks as a structured alternative channel

`claude --help` confirms `--output-format=stream-json` **only works with
`--print`** (non-interactive one-shot mode) — not applicable here, since
stapler-squad drives Claude Code interactively inside a tmux PTY. So the
"structured event stream" escape hatch that exists for scripted/CI usage does
**not** apply to this codebase's PTY-based session model without a much
larger architecture change (replacing the tmux+PTY session model with a
`--print --output-format=stream-json` subprocess model — far outside this
feature's scope and a non-goal).

However, Claude Code **does** expose two dedicated, documented hook events
for this exact transition, confirmed via the current hooks reference
(`https://code.claude.com/docs/en/hooks`, fetched 2026-08-06):

- **`PreCompact`** — "Runs before Claude Code is about to run a compact
  operation." Matcher `auto` fires specifically for auto-compact (vs.
  `manual` for `/compact`). Receives `trigger` and `custom_instructions` on
  stdin.
- **`PostCompact`** — "Runs after Claude Code completes a compact
  operation." Same `auto`/`manual` matcher. Receives `trigger` and
  `compact_summary`.

Critically, hooks "run wherever Claude Code runs: sessions in the terminal,
IDE extensions, the Desktop app, and Claude Code on the web all fire the same
hook events" — i.e. they **do** fire for the interactive terminal sessions
stapler-squad manages, unlike `stream-json`.

stapler-squad already has first-class infrastructure for exactly this
pattern: `server/services/hook_injector.go` + `hook_receivers.go` inject
per-session HTTP hooks (curl → `POST {base}/api/hooks/<event>` with
`X-CS-Session-ID`) into a session's `.claude/settings.local.json`, and
already registers `PreToolUse`, `PostToolUse`, `Stop`, `UserPromptSubmit`,
plus a bespoke `HookGitDriftCheck` (its own dedicated `PostToolUse` endpoint,
scoped only to autonomous backlog sessions — see the doc comment at
`server/services/hook_injector.go:23-31`). Adding `HookPreCompact`/
`HookPostCompact` following that exact template would give an **exact,
unambiguous** compaction-start/compaction-end signal — no regex, no
false-positive risk, no dependency on capturing the right spinner string —
at the cost of: a new proto/Go hook constant, a new receiver endpoint, and
(unlike existing fire-and-forget hooks) actually threading the event through
to session state instead of just logging it.

- Pros: Deterministic, vendor-documented signal; zero regex/spinner-text
  fragility; direct architectural precedent already in this codebase
  (`HookGitDriftCheck`); works for interactive sessions.
- Cons: Larger touch surface than a pattern addition (new hook constant,
  injector wiring, new receiver endpoint, session-state plumbing from HTTP
  callback → `DetectedStatus`, whereas today's detector pipeline only reads
  PTY bytes); introduces a second, HTTP-based path into the same
  `DetectedStatus` state machine that regex detection currently owns
  exclusively, which the requirements doc's non-goals implicitly assume
  won't happen; **not** what the requirements/acceptance-criteria already
  scoped (they explicitly commit to `session/detection/binaries/claude.go`
  regex patterns, proto enum, `detector_test.go` fixtures).
- **Verdict: Viable, but out of scope for this pass.** This is a legitimate,
  arguably *better* long-term signal and worth flagging as a follow-up/ADR
  candidate, but replacing the acceptance criteria's already-committed
  regex-based design mid-research would be scope creep beyond what Phase 2
  is chartered to decide. Recommend: build the regex-based detector now (per
  requirements), and file the hook-based approach as a fast-follow
  backlog/ADR item, since it would also generalize better if compaction
  detection is ever needed for non-Claude binaries (a stated non-goal today
  but explicitly called out as "unless research finds it's trivially the
  same mechanism" — hooks are Claude Code-specific, so this doesn't change
  that non-goal for other binaries).

## Option 2: SaaS/managed API

Not applicable — this is local PTY/terminal-output classification, no
network service is involved. Confirmed no disagreement with requirements.md
framing.

- **Verdict: Not applicable.**

## Option 3: Custom regex (stdlib `regexp`) vs. something more robust

The existing package already solves the two hard problems a compaction
message would present:

1. **ANSI-code-laden output** — `session/detection/normalizer.go`'s
   `PTYNormalizer.Normalize()` runs `stripANSI(collapseCarriageReturns(...))`
   on all PTY content *before* any pattern ever sees it. Patterns are never
   written against raw escape sequences — this already generalizes to a
   compaction message.
2. **Spinner/animation frames** — `claude_thinking_verb` in
   `session/detection/binaries/claude.go:126-130` is the exact template:
   `(?m)^[ \t]*[·✢✳✶✻✽●*✦][ \t]+[A-Z][a-zA-Z'\-éèêàâùûôîïëüöäÿæœ]*(?:…|\.{1,3})`
   — a character class of spinner glyphs + capitalized verb + ellipsis,
   matched per-line after normalization, at `Active` priority 26. The
   `progress_indicators` pattern (line 138-142,
   `[✓✔⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏★].*(?:ing|Processing|...)`) is a second working example of
   the same "spinner-glyph + verb" shape.

Go's stdlib `regexp` (RE2, no backtracking, linear-time, no catastrophic
backtracking risk) is already proven sufficient for this exact class of
input across ~10 other status groups and dozens of patterns in this package.
A compacting-context message is very likely to follow the *same* shape
Claude Code already uses for its other transient status lines (spinner glyph
+ verb, single status line, possibly multi-line with a token-count summary
line similar to `claude_active.txt`'s `esc to interrupt ... % until
auto-compact` line) — nothing about it requires backtracking regex,
multiline lookahead, or a parser generator. `pattern_set.go`'s existing
YAML-configurable `StatusPattern{Name, Pattern, Priority}` model, and
`detector.go`'s priority-ordered first-match evaluation, already handle
"this is one of several similarly-shaped Active-state lines" — exactly this
feature's shape.

- Pros: Zero new dependency; matches 10+ existing patterns in the same file;
  RE2 has no ReDoS risk; already tested at scale in this package
  (`pattern_set_test.go`, `detector_test.go`, `asterism_test.go`); the exact
  live string just needs confirming (per requirements' Open Question) and
  then it's a same-shaped addition to `Active` (or a new group, per ACT 2).
- Cons: None specific to this case — a genuinely more complex pattern (e.g.
  matching balanced structures, or needing lookahead across widely-separated
  lines) would be a reason to reach for something heavier, but nothing in
  the confirmed hook payloads or existing fixtures suggests that.
- **Verdict: Recommended.** stdlib `regexp` following the
  `claude_thinking_verb` template is both sufficient and the only choice
  consistent with the rest of the package's convention (no case exists in
  this package for a heavier parsing library, and introducing one for a
  single pattern would violate the interface-pollution / unjustified-tooling
  spirit of this repo's engineering conventions).

## Option 4: Fork/adapt existing prior art in this codebase

`StatusWaitingForAgent` is the closest and most recent precedent — a new
`DetectedStatus` value added in the same "unify session status pipeline"
change (`684e0b498` / PR #132) that this feature should mirror structurally:

- New iota value in `session/detection/detector.go`'s `DetectedStatus` enum
  (line 31: `StatusWaitingForAgent // Waiting for one or more background
  agents to finish`).
- A new pattern group (`WaitingForAgent: []StatusPattern{...}` at
  `detector.go:574`) rather than overloading `Active`.
- Explicit priority-ordering interaction documented at `detector.go:819`
  ("WaitingForAgent is more specific (higher priority in single-line
  matching)") and a case in the terminal single-line resolution switch
  (`detector.go:832`).
- A `case StatusWaitingForAgent:` added in **both** functions in
  `proto_mapping.go` (lines 30 and 46) — confirming the requirements doc's
  claim that `proto_mapping.go` needs two updates, not one.
- A matching new `sessionv1.DetectedStatus` proto enum value in
  `proto/session/v1/types.proto` (enum defined at line 380).

This is a direct, low-risk template: add `StatusContextCompacting` the same
way, as its own pattern group (not folded into `Active`, matching how
`WaitingForAgent` got its own group rather than reusing `Active` despite
being conceptually "the AI is doing something") with an explicit priority
relative to `esc_to_interrupt`/`claude_thinking_verb`/the "N% until
auto-compact" line so compaction-in-progress doesn't get shadowed by the
more general "active" patterns already matching on the same output.

- Pros: Directly analogous, recent (same PR/era), same author intent
  (distinguish a specific AI-busy substate from generic
  Processing/Executing), all four touch points (`detector.go`,
  `proto_mapping.go` ×2, `types.proto`, binary pattern file) already
  demonstrated end-to-end in the same codebase.
- Cons: None found — this is the intended extension point.
- **Verdict: Recommended.** Mirror `StatusWaitingForAgent`'s structure
  exactly: new enum value, own pattern group (not `Active`), explicit
  priority/precedence documented at the point where it could conflict with
  `esc_to_interrupt`/`claude_thinking_verb`, both `proto_mapping.go` cases,
  and a new proto enum value.

## Summary

| Option | Verdict |
|---|---|
| 1a. Existing OSS "AI CLI state" library | Not applicable — nothing exists |
| 1b. Claude Code hooks (`PreCompact`/`PostCompact`) as structured signal | Viable, but out of scope for this pass — recommend filing as a fast-follow ADR/backlog item, not adopting now (requirements/acceptance-criteria already commit to the regex path; hooks would add a second event-driven path into the same state machine) |
| 2. SaaS/managed API | Not applicable |
| 3. stdlib `regexp` vs. heavier parsing | Recommended — matches 10+ existing patterns, RE2 is safe by construction, `claude_thinking_verb` is the exact template |
| 4. Fork/adapt `StatusWaitingForAgent` | Recommended — direct, recent, four-touch-point precedent to mirror structurally |

**Overall recommendation:** Build from scratch within `session/detection/`,
following the `StatusWaitingForAgent` precedent exactly (own pattern group,
own proto enum value, both `proto_mapping.go` cases, explicit priority vs.
neighboring `Active` patterns), using stdlib `regexp` with the
`claude_thinking_verb` spinner-glyph-class template once the literal
compaction-in-progress string is confirmed against live output (the
requirements doc's Open Question / ACT 1 — still unresolved by this
research pass, since it requires capturing real Claude Code output during
compaction, not desk research). Separately, flag the `PreCompact`/
`PostCompact` hook-based approach to whoever owns the ADR backlog as a
stronger long-term signal worth an explicit build-vs-buy revisit — it isn't
adopted here only because it's a larger architectural change than this
feature's committed scope, not because it's inferior.

## Sources

- `claude --help` (local binary, 2026-08-06) — confirms `--output-format
  stream-json` is `--print`-only.
- [Hooks reference — Claude Code Docs](https://code.claude.com/docs/en/hooks) (fetched 2026-08-06) — `PreCompact`/`PostCompact` event definitions, matcher semantics, input schema.
- Local repo: `session/detection/binaries/claude.go`,
  `session/detection/detector.go`, `session/detection/normalizer.go`,
  `session/detection/proto_mapping.go`,
  `proto/session/v1/types.proto`, `server/services/hook_injector.go`,
  `server/services/hook_receivers.go`, `internal/claudehooks/claudehooks.go`.
- `git log --oneline -- session/detection/proto_mapping.go` — identifies
  `684e0b498` (PR #132, "unify session status pipeline with type-safe
  DetectedStatus enum") as the commit that introduced `StatusWaitingForAgent`,
  used as the Option 4 precedent.
