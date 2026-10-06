# Build vs. Buy — Cold-Restart UUID Recovery

## Scope reminder

This fix is a **reordering of existing first-party code**, not a new capability.
`session/instance.go`'s `startLocked()` must call `HistoryFileDetector.DetectByPath()`
*before* `initTmuxSession()`/`buildLaunchCommand()` instead of after. No new parser,
algorithm, or data structure is being introduced. The build-vs-buy questions below
are answered against that reality rather than padded with hypothetical alternatives.

## 1. External library for "detect/resume a CLI subprocess's prior session"

**Conclusion: none exists, and none would plausibly exist.**

Checked `go.mod` (`grep -n "jsonl\|history\|resume\|session" go.mod`) — no dependency
name suggests this. The detection logic is inherently coupled to Claude Code's own,
undocumented on-disk conventions:

- The directory-naming encoding in `ClaudeProjectDirName()`
  (`session/history_detector.go:118-129`) — replace every non-alphanumeric byte in
  the absolute project path with `-` — is a private implementation detail of the
  `claude` CLI, not a published spec.
- The matching regex `claudeProjectsPattern` (`session/history_detector.go:55`,
  `` `/\.claude/projects/([^/]+)/([^/]+)\.jsonl$` ``) and the `agent-*.jsonl`
  exclusion filter encode more undocumented behavior specific to how Claude Code
  writes agent-subprocess transcripts alongside the main conversation file.

No OSS package can reasonably target this format because it isn't a public
interface — it's the internal layout of one specific CLI tool's local state
directory. This is fully first-party knowledge already captured in this repo. There
is nothing to "buy."

## 2. SaaS / managed API angle

**None.** This is local filesystem state (`~/.claude/projects/*.jsonl`) plus local
subprocess/tmux state — there is no network call, hosted service, or managed API
involved anywhere in the detection or recovery path. Forcing a build-vs-buy SaaS
comparison onto a local-disk scan would be manufacturing a false choice.

## 3. LLM-generated vs. battle-tested library — reframed as reuse vs. reimplement

The actual "algorithm" here — scan a directory, filter to `*.jsonl`, exclude
`agent-*` prefixed files, validate UUID-shaped basenames, pick the newest by
mtime — is not a hypothetical to be written or sourced. It's already implemented
and already tested:

- `session/history_detector.go:137-199` — `DetectByPath()`.
- `session/history_detector_test.go` — 10 test functions total, of which 3
  exercise `DetectByPath()` directly:
  - `TestHistoryFileDetector_DetectByPath_MissingDirReturnsNil` (line 177) —
    nonexistent project directory → `nil, nil`.
  - `TestHistoryFileDetector_DetectByPath_FiltersAgentFiles` (line 185) —
    an `agent-*.jsonl`-only directory → `nil` (correctly excluded).
  - `TestHistoryFileDetector_DetectByPath_PicksMostRecentWhenMultiple` (line 202) —
    two valid UUID `.jsonl` files with explicit `os.Chtimes` — asserts the newer
    one wins.
  - `TestClaudeProjectDirName` (line 155) covers the path-encoding function
    `DetectByPath` depends on.

  The remaining 6 tests cover the PID-based `Detect()` sibling method (process-file
  inspection path), which this fix does not touch.

**Coverage gap for planning to note (not a build-vs-buy concern, but relevant to
Agent 2/4):** none of the existing tests write JSONL content that is empty,
malformed, or truncated — `DetectByPath()` never reads file *contents* at all, it
only matches on filename pattern + mtime, so a zero-byte or corrupt `.jsonl` file
would still be picked as a valid candidate today. That's an existing, pre-existing
characteristic of the function being reused, not something introduced by this fix.
Worth flagging as a possible edge case for the regression test in Acceptance
Criterion 4, but it is not a reason to alter or reimplement `DetectByPath()` — the
requirements only call for *reordering when* it's invoked, not changing what it
does.

**Recommendation: 100% reuse.** `DetectByPath()` is mature relative to the scope of
this fix (dedicated, passing tests for exactly the missing-dir, agent-filtering, and
multi-candidate-newest-wins cases the reordered call site will hit) and requires
zero modification — only a different call site and ordering in `startLocked()`.

## 4. Fork/adapt

N/A. Nothing external is being adopted, forked, or vendored. This is first-party
code (`session/history_detector.go`, already in this repo, already exercised by
`session/instance_cold_restore_test.go`) being invoked earlier in an existing
first-party function (`session/instance.go`'s `startLocked()`).

## Bottom line

There is no real build-vs-buy decision in this task. Zero new dependencies, zero
new algorithms, zero forking. The only engineering work is moving an existing,
tested function call to an earlier point in `startLocked()` and threading its
result into the launch-command decision that currently runs before it.
