# Research: Pitfalls & Risks — Persistent Operator Memory

Agent 4 (Pitfalls), SDD Phase 2 research for `project_plans/operator-memory/`.

## 1. Prompt injection scan: right-sized for a manual-edit-only v1

**Finding**: this repo already has two live precedents for handling untrusted
text destined for a system/tool prompt, and neither is a heavyweight
"injection scanner":

- `server/services/rule_prompt_builder.go:169` wraps untrusted command text in
  `<command>...</command>` XML delimiters specifically "to prevent prompt
  injection from command content," combined with a **secret** scan
  (`ScanForSecrets`, `server/services/secret_scanner.go:53`) as "defense-in-depth"
  before the text is interpolated (`rule_prompt_builder.go:163-168`).
- `session/backlog_context.go:14-35` distinguishes two untrusted-text
  strategies: `SanitizeForAgentContext`/`sanitizeField` strips HTML tags for
  free-text fields, while `truncateField` deliberately does **not** strip,
  with the comment: "the envelope context renders injection payloads inert
  and stripping content would be destructive" — i.e. delimiting/quoting
  untrusted content is treated as sufficient in some cases, not always
  scan-and-reject.

**Recommendation**: for this issue's actual scope (`OPERATOR.md`/`REPO.md` are
manual-edit-only — no automated writer ships here per requirements.md line
28-32), a full pattern-matching "injection scanner" (heuristics for
`ignore previous instructions`, role-markers, etc.) is over-engineering for
what is currently operator-trusted content the operator typed themselves.
Two proportionate options, in order of preference:

1. **Delimit, don't scan.** Wrap the loaded memory snapshot in an explicit
   tagged block (e.g. `<operator_memory>...</operator_memory>`) when
   appending to the system prompt tail, mirroring the `<command>` precedent.
   This is cheap, matches existing style, and defangs the "operator's own
   file quietly becomes an attack vector for a future automated writer"
   concern without pretending to detect adversarial content today.
2. **Reuse the existing secret scanner, not a new injection scanner**, on the
   *read* path (before injecting into the prompt) as defense-in-depth against
   an operator accidentally pasting a credential into `OPERATOR.md` — this is
   a real, cheap, already-built primitive (`ScanForSecrets`), unlike a novel
   prompt-injection classifier.

The acceptance criterion "Injection scan runs before any write" should be
scoped explicitly, per the issue's own instruction (requirements.md line
53-57): if this issue ships no write path (view/CLI-read only), state plainly
that the scan requirement is **deferred to the companion automatic-writer
item**, where it is actually load-bearing (untrusted diff/PR content flowing
into a file that gets replayed into every future prompt is a real escalation
path — that's when a real scanner earns its cost, not now). Do not build an
unused scanner just to satisfy the letter of the AC — `plan.md`'s existing
open question about `memory edit`/`memory add` is the right place to make
this call explicit.

## 2. Prefix-cache invalidation: tail-only append is required, not optional

**Finding**: `session/headless/features.go` and `session/headless/pool.go`
both document prefix-caching as a load-bearing design constraint, not
incidental:

- `features.go:69`: "Stable prompts enable prefix-caching across repeated
  calls."
- `features.go:101`: system-prompt role/instruction text "is separated from
  the per-call data payload (item, diff) to enable prefix-caching."
- `pool.go:32-33`: `Pool` "manages a map of named LLM feature sessions,
  providing session reuse **for prefix-cache optimization** and bounded
  concurrency" — i.e. the pool itself assumes repeated calls share a stable
  prefix across a session's lifetime (`MaxCallsPerSession`, default 25,
  `pool.go:21`).

**Implication for this feature**: the reference design's own stated
mitigation — "snapshot is the final, volatile tier ... so a snapshot change
doesn't invalidate the cacheable stable prefix — only the tail" — is not
just a nice-to-have, it's the specific property that keeps this feature
compatible with the pool's existing prefix-cache assumption. If memory were
instead spliced into the *middle* of the stable prompt (e.g. inserted before
`reviewSystemPrompt`'s body), every snapshot change would silently invalidate
the shared-prefix cache benefit `pool.go` is built around, increasing latency
and cost per call without any test catching it (cache hit/miss isn't
currently asserted anywhere in this codebase). **Concretely**: implementation
must append the memory block strictly after the existing constants
(`reviewSystemPrompt`, `headlessReviewSystemPrompt`, etc.) with no
interpolation into their body text, and a unit test should assert the stable
prefix bytes are byte-identical whether or not memory content changes (i.e.
`stablePrompt + memoryBlock`, never `fmt.Sprintf(stablePromptWithHole, memory)`).

## 3. Concurrency: read path is the only path in scope, but plan for lost-write risk in the companion item

**Finding**: this repo has a documented pattern
(`.claude/rules/go-double-checked-locking.md`) for returning the
locally-computed value rather than re-reading a shared slot after a
lock-release/re-acquire race, plus a cluster of *fixed* mutex-contention bugs
(`docs/bugs/fixed/BUG-020` through `BUG-024`) whose common lesson is: **don't
hold a lock across I/O**, and **prefer `sync.Map`/lock-free reads for
read-mostly, write-rarely data** (BUG-022 explicitly: "the new preference...
is `sync.Map` or `xsync.Map` for concurrent maps, not `map + mutex`").

**Applied to this feature**:
- The read path (loading `OPERATOR.md`/`REPO.md` once per headless call) is
  read-mostly and should follow the `sync.Map`/lock-free precedent if any
  in-process caching of the snapshot is added — plain `os.ReadFile` per call
  with no shared mutable cache sidesteps the whole class of bug cheaply for
  v1, since headless calls are already bounded by `Pool.concurrencySem`
  (`pool.go:26,45`, default `MaxConcurrentSessions` = 5) and a handful of
  small file reads per call is not a hot path worth optimizing prematurely.
- **This issue has no write path** (manual edit only), so the classic
  concurrent-writer race (two headless calls or a human editor and a headless
  call writing `OPERATOR.md` simultaneously, one write clobbering the other)
  is out of scope *for this issue* but is a real risk the companion
  auto-writer item must handle — recommend it use an atomic
  read-modify-write (temp file + rename, the same pattern already used
  elsewhere in this codebase for state files — see
  `server/services/approval_store.go` / config persistence) rather than an
  in-place append, and should NOT assume it's the only writer (an operator
  could hand-edit `OPERATOR.md` while the auto-writer is mid-append).

## 4. Unbounded growth: byte cap has direct precedent, follow it exactly

**Finding**: `session/headless/features.go:59-66` already defines the exact
pattern requirements.md asks for ("start with a byte cap, not a
summarization pipeline"):

```go
const MaxDiffSizeReview = 40_000     // review prompt diffs
const maxDiffSizePR = 40_000         // DraftPRDescription
const maxDiffSizeCommit = 20_000     // SuggestCommitMessage
```

with truncation applied as a hard slice + explicit marker, e.g.
`features.go:284-285`: `if len(diff) > maxDiffSizePR { diff =
diff[:maxDiffSizePR] }` (see also `truncateField`'s `" [truncated]"` suffix
convention in `session/backlog_context.go:33-36`, and
`docs/bugs/fixed/BUG-018-gob-session-persistence-memory-hotspot.md` /
`BUG-017-sqlite-eager-load-difftats-25mb-startup.md` as prior incidents where
unbounded stored content became a real production cost).

**Recommendation**: define e.g. `maxOperatorMemoryBytes` /
`maxRepoMemoryBytes` constants alongside the existing `MaxDiffSize*` group in
`features.go`, truncate with the same `"[truncated]"` marker convention on
read (not silently), and log a warning when truncation occurs so growth is
visible before it becomes BUG-017/BUG-018's "why is memory/DB usage climbing"
surprise. A raw token-cost note: at ~4 bytes/token, even a generous 20KB cap
per file (2 files = 40KB) adds ~10K tokens to every headless call's prompt —
worth stating in the plan as the actual per-call cost being accepted, since
nothing here amortizes it the way prefix-caching amortizes the *stable*
prompt tier (memory is the volatile tail, so it's paid in full — cache-miss
cost — on every single call, not just the first in a pool session).

## 5. Silent failure: must degrade, must not block triage/review

**Finding**: `config.GetConfigDirForDir` (`config/config.go:123-136`) — the
function this feature is required to reuse for `REPO.md`'s path
(requirements.md line 39-41) — can itself fail: it calls
`os.UserHomeDir()` and returns a hard error if that fails (`config.go:133-136`,
`"failed to get config home directory: %w"`), which is exactly the "home dir
resolution fails under a different user/CI context" scenario named in the
task. This is a real, already-encoded failure mode in the dependency this
feature is required to build on, not a hypothetical.

**Recommendation** (matches the requirement's own empty-state rule,
requirements.md line 65-66, "empty memory files produce no system prompt
noise"): generalize that rule to **any read failure** — missing file,
permission-denied, non-UTF8/corrupted content, or `GetConfigDirForDir`
erroring outright — must all resolve to "no memory block appended," never a
failed/blocked headless call. A triage or review call's job (assess a diff
against acceptance criteria) has nothing to do with whether memory happened
to load, so treating a memory read failure as fatal would be a scope
violation of its own reliability contract. Concretely: the loader function
should return `(string, bool)` or equivalent ("content, ok") and any `error`
path collapses to `ok=false` with a single low-noise log line (not a returned
`error` that a caller might propagate) — consistent with how
`session/claude_session_manager.go:102`'s `os.ReadFile(metadataPath)` and
`session/backlog_commands.go:262`'s `os.ReadFile(excludeFile)` both already
discard the read error and fall back to a zero-value rather than failing the
caller.

## 6. Naming collision: `session/memory` already exists — pick a different package path

**Finding**: `session/memory/` is an **existing, unrelated** package
(`reader.go`) that "provides session memory measurement for the hibernation
sweeper" — i.e. system/process RAM usage (gopsutil-based `SystemMemoryPct`,
`SessionRSSMB`), nothing to do with LLM operator memory. A new package
literally named `memory` anywhere near `session/` will either collide on
import path or, worse, survive as two same-named-but-unrelated concepts that
future readers (and grep-based audits, per `ai_interfaces.go:6`'s own stated
convention) will confuse. Recommend a distinct name in the plan — e.g.
`session/headless/operatormemory` or `config/memory` (co-located with
`GetConfigDirForDir`, which the plan already leans on) — and call this out
explicitly so `plan.md`'s task breakdown doesn't silently redefine
`session/memory`.

## 7. Related prior incidents worth cross-checking against (docs/bugs/)

- `BUG-052-github-keychain-userprcache-data-race.md` — another concurrent
  read/write-to-shared-state incident; worth a skim if the companion
  auto-writer item reuses any shared in-process cache pattern for the memory
  snapshot.
- `BUG-017`/`BUG-018` (large-state-file-size, gob-persistence memory
  hotspot) — direct precedent for why the byte cap (finding #4) must be
  enforced from day one, not "later once it's a real problem" — both were
  exactly that failure mode for other state files in this repo.
- No existing bug in `docs/bugs/{open,fixed}/` currently mentions
  "operator memory" or "OPERATOR.md" by name — this is genuinely new
  surface area, not a repeat of a previously-logged incident.

## Summary of concrete recommendations for plan.md

1. Injection scan: delimit with an XML-ish tag on inject, reuse
   `ScanForSecrets` (not a new scanner) as read-side defense-in-depth; state
   explicitly that a full injection classifier is deferred to the companion
   auto-writer item where untrusted diff content is actually the input.
2. Append memory strictly as a tail block after existing stable prompt
   constants; add a test asserting the stable-prefix bytes are unchanged
   across memory content changes.
3. No shared in-process cache for v1 (plain per-call `os.ReadFile`); if one
   is added later, follow the `sync.Map`/no-lock-across-I/O precedent from
   BUG-020–024, not `map + mutex`.
4. Add `maxOperatorMemoryBytes`/`maxRepoMemoryBytes` constants next to
   `MaxDiffSizeReview` et al. in `features.go`, truncate with the existing
   `"[truncated]"` marker + a log line on truncation.
5. Loader must never fail a headless call — any read/path-resolution error
   (including `GetConfigDirForDir`'s `os.UserHomeDir()` failure mode)
   collapses to "no memory block," logged once, not propagated as `error`.
6. Do not name the new package/dir `memory` under `session/` —
   `session/memory` already exists for unrelated hibernation-sweeper RSS
   measurement.
