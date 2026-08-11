# Research: Build vs. Buy — Operator Memory

Agent: 6 (Build vs. Buy) · Source: `project_plans/operator-memory/requirements.md`

## Scope recap

The feature is: read `~/.stapler-squad/memory/OPERATOR.md` + `<workspace>/memory/REPO.md`
(two flat Markdown files), concatenate their non-empty contents as a new tail
block on an existing system-prompt string, and provide a `memory show` CLI
command. Writing is explicitly out of scope this issue except for the
gate itself: "injection scan runs before any write" must exist, scoped to
whatever write path this issue actually introduces. Per the requirements'
own non-goals, there is no summarization, no embedding search, and no
eviction policy to design for.

---

## Option 1 — Existing OSS "LLM agent memory" library

**Examples considered:** `mem0`, LangChain-Go memory modules, vector-store-backed
memory frameworks (Milvus/Chroma/Pinecone Go clients + a retrieval layer).

- **Pros:** Handles semantic recall, summarization, and eviction out of the
  box if the project ever needs those.
- **Cons:** Every one of these frameworks is built around the problem this
  issue explicitly does *not* have — semantic search over many memory
  fragments. Adopting one means pulling in a vector-store dependency (or an
  embedded one), an embedding-model call path, and a query/retrieval API to
  read two Markdown files whose entire contents already fit in a prompt
  tail. `go.mod` currently has zero RAG/vector/embedding dependencies
  (checked: no `langchain`, `vector`, `embed`, `rag`, or `memory` framework
  imports) — this would be the first. It also conflicts with the reference
  design's own "frozen snapshot" semantics: these frameworks are built to
  mutate/query state *during* a conversation, whereas the spec requires a
  read-once-at-call-start snapshot that explicitly does *not* reflect
  mid-call writes.
- **Verdict: Not recommended.** No library-shaped problem exists here — this
  is `os.ReadFile` + `strings.TrimSpace` + a `Sprintf` tail block. Pulling in
  a memory/RAG framework for two flat files is the textbook case this repo's
  own `ponytail` convention (cited directly in requirements.md's non-goals:
  "start with a byte cap, not a summarization pipeline") warns against.

## Option 2 — SaaS / managed "agent memory" API

**Examples considered:** hosted memory-as-a-service offerings (e.g. Mem0's
hosted tier, Zep, other managed conversational-memory APIs).

- **Pros:** Offloads storage, versioning, and (if ever needed) semantic
  search to a vendor; no local file management code to write.
- **Cons:** Stapler Squad is a single-operator local tool — `~/.stapler-squad/`
  already holds all state (`config.json`, `sessions.json`) as local files per
  `.claude/docs/state-isolation.md`, with no existing pattern of phoning
  home to a SaaS backend for core state. A managed memory API would add:
  network latency on every headless triage/review call (a call path that
  currently has zero external dependencies beyond the LLM call itself), a
  new vendor credential to provision and rotate, an availability dependency
  (headless triage breaks if the memory vendor is down, even though the
  underlying data — two Markdown files a human edits by hand per the
  read-side-only scope — has no reason to leave the machine), and a privacy
  concern (OPERATOR.md/REPO.md are explicitly meant to hold "team
  conventions, recurring failure patterns, codebase quirks" — operator/repo
  knowledge that has no obvious reason to transit a third party for a tool
  whose entire state model is local-first).
- **Verdict: Not recommended.** No requirement here needs a network round
  trip. It inverts the project's local-first architecture for two files an
  operator edits by hand.

## Option 3 — Injection scan: heuristic vs. LLM-classifier vs. library

The acceptance criteria require "injection scan runs before any write."
Three shapes were compared:

1. **Deterministic heuristic/denylist (regex or substring check)** — e.g.
   reject content containing role-override phrases ("ignore previous
   instructions", "you are now..."), unescaped fence-breaking sequences, or
   embedded system/assistant role markers.
   - **Pros:** Cheap (microseconds), deterministic, trivially unit-testable
     (the requirements explicitly call for a "write-scan rejection" unit
     test — a heuristic gives a stable, non-flaky assertion; an LLM-based
     check would make that test either mocked-and-therefore-not-testing-the-
     real-path, or nondeterministic/costly to run in CI), no new runtime
     dependency, no added latency or cost on the write path, no new failure
     mode (no network call that can time out or rate-limit).
   - **Cons:** Not semantically robust — a sufficiently indirect or novel
     phrasing could slip through a fixed denylist, and it needs upkeep as
     new injection patterns are discovered.
   - **Verdict: Recommended.** This is the correct proportional choice: the
     write surface here (per this issue's own scope note) is an
     operator/CLI-driven manual edit path, not an untrusted external input
     channel like a scraped web page or third-party issue body — a bounded,
     testable heuristic gate matches the actual threat model without
     over-building.

2. **Second LLM call to classify content as safe/unsafe.**
   - **Pros:** More semantically robust against novel phrasing than a fixed
     denylist.
   - **Cons:** Adds latency, an API cost, and a new failure mode (the
     classifier call itself can fail, time out, or be wrong) to what is
     otherwise a synchronous local file write. Non-deterministic — directly
     at odds with the unit-testable "write-scan rejection" acceptance
     criterion, which wants a stable pass/fail assertion, not something that
     depends on model behavior/mocking. Also a layering smell: using an LLM
     to guard content that will itself be fed to an LLM doesn't remove the
     trust boundary, it just adds a second copy of it.
   - **Verdict: Not recommended** for this issue's scope. Worth
     reconsidering only if/when the future automatic writer (companion
     backlog item, out of scope here) starts accepting less-trusted input
     (e.g. summarized diff content) where a fixed denylist proves
     insufficient in practice.

3. **Dedicated Go prompt-injection-detection library.**
   - **Cons:** No mature, widely-used Go library for this exists in the
     ecosystem today (the space is dominated by Python tooling, e.g.
     `rebuff`, `llm-guard`, none with a first-class Go port). Pulling in an
     unmaintained or immature dependency for a single denylist check is the
     same over-engineering problem as Option 1, just smaller.
   - **Verdict: Not recommended.** Nothing to buy; write it as a ~20-30 line
     function.

**Recommendation:** a small deterministic heuristic function (denylist of
role-override phrases + delimiter-breaking sequences), analogous in spirit
to `SanitizeDiff` (see Option 4) but rejecting/flagging rather than
escaping, since memory content is operator-authored prose, not an
executable diff that must be preserved verbatim.

## Option 4 — Fork/adapt something already in this repo

Searched for existing "scan for X before write" or prompt-injection-adjacent
patterns:

- **`session/backlog_review.go:614` `SanitizeDiff`** — neutralizes ` ``` `
  sequences in diffs before interpolating them into an LLM prompt, so
  injected content can't close a markdown code fence and get the model to
  treat subsequent diff text as instructions. This is a real precedent in
  this codebase for "sanitize untrusted content before it enters a prompt,"
  but it solves a different, narrower problem (delimiter escaping to
  preserve diff fidelity), not content classification/rejection. It is
  *adjacent*, not directly forkable for a "reject unsafe content" gate — but
  its `strings.ReplaceAll`-based simplicity is exactly the calibration the
  new injection-scan function should match (no new dependency, one file, a
  handful of lines).
- **`server/services/rule_prompt_builder.go:169`** — wraps rule/command
  content in XML delimiters "to prevent prompt injection from command
  content" — same escaping-not-classifying pattern as `SanitizeDiff`, again
  adjacent but not a scanner to fork.
- No existing "reject write if content matches denylist" function was found
  anywhere in the Go tree (`grep -rn "injection"` across `*.go` returns only
  these two escaping call sites plus unrelated hits like MCP/hook
  "injection" naming that mean dependency injection, not prompt injection).
- **Verdict: Nothing to fork wholesale.** Write the new heuristic scanner as
  its own small function (e.g. `session/memory` or wherever the
  storage/injection layer lands), following the same "small, local,
  dependency-free helper" shape as `SanitizeDiff` and the rule-prompt
  delimiter wrap, rather than generalizing either of those (they solve a
  different problem: escape-to-preserve vs. reject-on-match).

---

## Summary table

| Option | Verdict |
|---|---|
| OSS agent-memory/RAG library | Not recommended — no semantic-search problem exists here |
| SaaS managed memory API | Not recommended — breaks local-first architecture, adds latency/vendor risk for no benefit |
| Injection scan: heuristic/denylist | **Recommended** |
| Injection scan: second LLM call | Not recommended for this issue's scope (revisit if the future writer ingests less-trusted input) |
| Injection scan: dedicated Go library | Not recommended — no mature option exists, and it's overkill regardless |
| Fork `SanitizeDiff` / rule-prompt XML-wrap | Not directly forkable (different problem shape) but sets the size/complexity bar for the new scanner |

**Bottom line:** build this from scratch as plain Go (`os.ReadFile` +
string assembly for the read/inject path, a small deterministic
denylist/heuristic function for the write-scan gate). No dependency, OSS
library, or SaaS integration is justified at this scope — consistent with
the requirements' own non-goals section and the repo's stated preference
for the smallest solution that satisfies the actual (not speculative)
requirement.
