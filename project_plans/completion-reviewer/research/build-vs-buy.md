# Build vs. Buy — Completion Reviewer

Agent 6, Phase 2 research. Scope: is there an existing library/service worth adopting for
any part of this feature, or is it a "just write the Go" case. Requirements:
`project_plans/completion-reviewer/requirements.md`.

## 1. OSS agent-memory framework (mem0 / Letta-MemGPT / LangChain memory)

Researched current (2026) state: Mem0 (Apache-2.0, ~48K GitHub stars, 2026 LOCOMO-benchmark
update) and Letta/MemGPT (Apache-2.0, ~13K stars, funded, full agent runtime built around
tiered/self-editing memory) are the two mature options; LangChain Memory is the
ecosystem-modular option tied to the LangChain orchestration stack.

**Pros**
- Mature, maintained, permissively licensed (Apache-2.0), solve a harder problem than this
  feature needs (semantic recall, consolidation, tiered/vector memory, conflict resolution).
- Would remove the need to design a memory *store* at all if adopted for both this item and
  the upstream operator-memory item (#116).

**Cons**
- **Language mismatch, stated explicitly**: mem0, Letta, and LangChain Memory are all
  Python-first libraries/services (Letta ships a Python SDK + optional server; mem0 is a
  Python/TS SDK against a hosted or self-hosted backend). This codebase is a single Go binary
  (`server/`, `session/`) with no Python runtime, no Python dependency management, and no
  existing FFI/subprocess bridge to a Python process. Adopting any of them means either (a)
  running a separate Python service and talking to it over HTTP/gRPC — a new deployable, a new
  failure domain, a new thing to keep alive alongside the existing single-binary + tmux
  architecture — or (b) shelling out to a Python CLI, which reintroduces exactly the
  subprocess/parsing overhead `.claude/rules/prefer-go-git-over-subshells.md` argues against
  for git and would be worse here (cold-start a Python interpreter per write).
- The problem this feature actually has (append a tagged, structured text learning; read it
  back by item ID or in aggregate for future prompts) does not need vector embeddings,
  semantic similarity search, or tiered context-window memory — the machinery these libraries
  exist for. Pulling in Letta's full agent runtime, in particular, would mean **running a
  second agent framework alongside stapler-squad's own tmux-backed Claude Code sessions**,
  duplicating a responsibility the app already owns.
- The requirements doc explicitly scopes the memory *store* itself out of this item (#116,
  "Out of Scope") and asks this item to define only the interface shape it needs. Committing
  to a specific Python library here would prejudge that separate, not-yet-planned item.

**Verdict: Not recommended.** Name the mismatch plainly: these are Python-ecosystem
tools being evaluated for adoption into a Go codebase with no existing Python runtime
dependency. Nothing here is Go-native or embeddable without a network hop to a separate
process. Recommend the operator-memory item (#116) build a small Go-native store instead (see
§2) and reassess mem0/Letta only if/when the memory feature set actually grows into
semantic recall across a large corpus — not before.

## 2. Managed/SaaS memory store vs. thin wrapper over existing local infra

Checked this repo's existing local persistence patterns before considering any hosted option:

- `config/state.go` — `AppState`/`UIStateAccess`, JSON file (`state.json`) protected by
  `github.com/gofrs/flock` for cross-process file locking, atomic write pattern. Precedent for
  small, low-volume structured JSON persisted to `~/.stapler-squad/`.
- `docs/registry/features/{backend,frontend}/*.json` — one JSON file per entry (per RPC/UI
  feature), keyed by ID, aggregated by `make registry-generate`. This is the closest structural
  analog to "one memory entry per backlog item, tagged by source ID": small, human-readable,
  diffable, no schema migration machinery needed.
- `session/ent/schema/*.go` — ent/SQLite is already the established pattern for structured,
  queryable, backlog-adjacent data with exactly this shape: `review_verdict.go`,
  `backlogprogressnote.go`, `approvalrule.go`, `backlog_status_event.go` are all small tables
  tied to a backlog item ID, versioned via the existing
  `.claude/rules/ent-schema-generation.md` codegen flow. A `memory_entry` ent schema
  (`id`, `backlog_item_id` FK/tag, `content`, `pinned bool`, `created_at`) would sit naturally
  next to these and get transactional writes, indexing by item ID, and existing migration
  tooling for free.

**Pros of local file/DB wrapper**
- Zero new infrastructure, zero new credentials/secrets, zero network dependency for a
  background hook that must "never block the main workflow" (AC) — a SaaS call adds a network
  round-trip and a new failure mode (auth expiry, rate limit, outage) to a path whose own
  acceptance criteria demand it degrade silently on failure.
- Matches every existing low-volume structured-data pattern in this repo (state.json,
  per-feature registry JSON, ent/SQLite tables) — no architectural precedent-setting needed.
- Trivially satisfies "append/update only, never delete" (AC) and "pinned entries bypass
  auto-transition" (AC) as plain field/query logic, whether the backing store is a JSON file or
  an ent table.

**Cons**
- None specific to this feature's actual volume/query needs (single-digit writes per
  completed backlog item, read-back by item ID or a bounded recent-N scan for prompt context).
  A hosted memory API's value proposition (semantic search, cross-session recall at scale)
  isn't a need this feature has yet.

**Verdict: Recommended (local store, no SaaS).** Explicitly recommend *against* any hosted
memory API for this feature. Whether the operator-memory item (#116) ultimately picks the
JSON-per-entry pattern (matching `docs/registry/`) or an ent/SQLite table (matching
`review_verdict`/`backlogprogressnote`) is that item's call, not this one's — but both are
Go-native, in-repo, zero-new-infra options that already exist as precedent. This item's job is
only to define the write call's shape (`WriteMemory(ctx, backlogItemID, content string) error`
or similar) against whichever primitive #116 lands with.

## 3. Bespoke Go implementation vs. battle-tested library for the reviewer logic itself

Walked the actual control flow this feature needs, per the requirements doc:

1. Trigger on `BacklogStatusDone` transition (a switch/if in the existing lifecycle state
   machine, `session/backlog_lifecycle.go`).
2. Gate on non-trivial content (`description != "" && len(acceptanceCriteria) > 0`) — a boolean
   check, not an algorithm.
3. Assemble a text context blob (title, description, AC statuses, triage/review notes) — string
   formatting.
4. Fire a background goroutine (`go func() { ... }()` with panic recovery and logged, swallowed
   errors) so it never blocks the caller — a well-understood, already-used pattern in this
   codebase (nothing new to invent).
5. Make one LLM call with a hard-restricted tool surface.
6. Append the result to the memory store, tagged with the item ID.

**No non-trivial algorithm or data structure is present in this item's scope.** There is no
dedup, no ranking, no consolidation, no similarity matching here — the requirements doc
explicitly places that class of problem ("periodic curator pass — staleness/dedup/pruning") in
**Out of Scope**, deferring it to a separate future item modeled on Hermes's weekly Curator job.
That is exactly where "battle-tested library vs. bespoke" would become a live question (dedup
and consolidation are real algorithmic problems); this item doesn't reach it.

**Verdict: Just write the ~150–200 lines of Go.** This is the plain "battle-tested library
doesn't really apply" case: assemble context → one restricted LLM call → append to store, gated
by a trivial boolean and fired off a goroutine. Importing a library to do string assembly and a
single API call would be over-engineering relative to the actual problem size. The one piece of
this that genuinely deserves care is *not* an algorithm — it's the tool-restriction enforcement
mechanism (see finding below), which is a correctness/security property, not a build-vs-buy
question.

**Adjacent finding directly relevant to the "enforced in code, not prompt" AC** (surfaced while
checking this repo's existing tool-restriction code, not part of the build-vs-buy question
proper, but load-bearing for whoever implements this): `session/backlog_review.go:406-414` and
its ADR-001 2026-07-15 addendum record that `TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed`
**empirically proved `AllowedTools`/`DisallowedTools` provide no real technical enforcement**
for Bash under `--permission-mode bypassPermissions` when invoking the `claude` CLI. This means
spawning a real `claude` CLI process (tmux-backed session or headless) and passing
`--allowedTools memory-write` is **not sufficient** to satisfy this item's AC that restriction
be "enforced at the session-builder level ... not by prompt instruction alone" — the flag has
already been shown, in this exact codebase, to be advisory rather than a hard boundary for at
least one tool class. Whoever implements this item should resolve the requirements doc's open
question ("real spawned session vs. a lighter-weight direct API call") in favor of a direct
Anthropic Messages API call from Go with a **single hard-coded tool definition in the request
body** (no `claude` CLI subprocess at all) — enforcement then comes from "the Go code literally
never implements or exposes any tool but memory-write," which is a property of what code exists,
not a flag any CLI might or might not honor.

## 4. Hermes Agent (`background_review.py` / `curator.py`) — fork/adapt or reference only

Confirmed via web search: Hermes Agent is a real, active open-source project
([NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent)), and the cited
pattern is real — the review fork runs on its own thread with a fresh context, uses a
`ContextVar`-based provenance tag (`"foreground"` vs `"background_review"`) so the two contexts
never cross-contaminate, and is restricted to only `memory`/`skill_manage` tools by construction
of which tools that fork's tool registry exposes — not by a runtime flag.

**Pros**
- The *architecture* is directly relevant and validated by a shipping project: fork a
  restricted context off a completion event, write only to memory, never let the fork touch
  the tools the main agent has. That shape maps cleanly onto this item's ACs.
- Confirms the "restrict by not implementing/exposing the tool" pattern is the right one
  (reinforcing the finding in §3) rather than a flag-based restriction.

**Cons**
- It is Python: in-process thread + `ContextVar` + a Python tool-registry object. None of that
  is portable code — there's no module to `go get`, no adapter to write against, because the
  entire mechanism is Python's threading/contextvars model applied to a Python-native tool
  dispatch table. stapler-squad's equivalent of a "tool registry" is the `claude` CLI's own
  tool set (invoked via subprocess or the Messages API), not an in-process Go object Hermes's
  design could be dropped into.
- Hermes's fork model relies on sharing the parent's prefix cache in-process (same Python
  process, same loaded model context) — an optimization this codebase's process-per-session
  (tmux) or per-call (headless) architecture doesn't have an equivalent for.

**Verdict: Reference pattern only, not adaptable/portable code.** Cite Hermes in the
implementation plan for its architecture (fork-with-a-restricted-tool-set-triggered-by-a-
completion-event), but treat it purely as prior art to imitate the *shape* of, the same way the
requirements doc already does. There is nothing to fork, vendor, or port — the Go implementation
plan should describe the equivalent Go-native mechanism (a direct Messages API call with a
hard-coded single-tool request body, per §3) from scratch, using Hermes only to validate that the
overall design pattern is sound and has prior production use.

## Summary Table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| mem0 / Letta / LangChain Memory | Mature, Apache-2.0, solves harder memory problems | Python-only; no Go runtime dependency exists in this repo; solves problems this feature doesn't have (semantic/vector recall); would prejudge the separate #116 item | **Not recommended** |
| Local file/SQLite wrapper (matches `config/state.go`, `docs/registry/`, `session/ent/schema/`) | Zero new infra, matches 3 existing in-repo patterns, no network failure mode on a must-never-block path | None material at this feature's volume | **Recommended** |
| Hosted/SaaS memory API | N/A (no known fit) | Network dependency on a must-never-block background path; no existing precedent in repo | **Not recommended** |
| Bespoke ~150–200 line Go implementation (assemble context → 1 LLM call → append) | Matches actual problem size exactly; no real algorithm present in scope | None — over-engineering risk runs the *other* direction (importing a library for this would be the mistake) | **Recommended** |
| Fork/port Hermes Agent's Python code | Validates the architecture against shipping prior art | Not portable — Python threading/contextvars/tool-registry model, no adapter surface | **Reference pattern only, not a fork target** |

## Key cross-cutting finding for the implementation plan

`session/backlog_review.go`'s ADR-001 addendum is directly load-bearing for this item's
hardest AC ("enforced at the session-builder level ... not by prompt instruction alone"):
`--allowedTools`/`--disallowedTools` passed to the `claude` CLI have already been proven, in
this codebase, not to provide real enforcement for at least one tool class under
`bypassPermissions`. The plan phase should design the restricted call as a direct Anthropic
Messages API request from Go with a single hard-coded tool in the request body — not a spawned
`claude` CLI/tmux session gated by an allow-list flag.
