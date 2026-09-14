# Architecture Research: session-path-domain-refactor

Scope: how to structure the naming fix and its staged rollout. Does not enumerate every
consumer (a sibling research agent owns the full audit) — file references below are the
minimum needed to justify each architectural call.

## 1. Naming scheme

### 1.1 The one correct abstraction already in the codebase

`session/instance_worktree.go`'s `Workspace()` (types not shown here are defined alongside
it) is the domain's own, already-correct vocabulary:

```go
type Workspace struct {
	EffectivePath string // resolved, worktree-aware current location
	RepoRoot      string // stable, creation-time identity path
}
```

`RepoRoot`/`EffectivePath` are unambiguous, already documented (`Workspace()`'s doc comment:
"the single source of truth for path resolution"), and already the terms the requirements
doc's Problem Statement uses to describe the two concepts. **Every other layer should adopt
this vocabulary rather than invent a third.** No rename needed at this layer — it's the
anchor the rest of the scheme should key off, not another rename target.

### 1.2 Proposed renames, by layer

| Concept | Current name(s) | Proposed name | Layer | Rename risk |
|---|---|---|---|---|
| Identity path | `Instance.Path`, `InstanceSnapshot.Path`, `InstanceData.Path` (Go field) | `Instance.RepoRoot`, `InstanceSnapshot.RepoRoot`, `InstanceData.RepoRoot` | Go internal | Low — compiler-checked, no wire/JSON change if `InstanceData.RepoRoot` keeps `json:"path"` (see §1.3) |
| Identity path | *(not exposed generically today — only `GitHubIntegration.MainRepoPath`, GitHub-scoped and only set when GitHub detection ran)* | new proto field `Session.repo_root_path` | proto/wire | New field, additive |
| Resolved path | `Session.path` (proto field 3) — **already populated from `Workspace().EffectivePath`**, confirmed at [`server/adapters/instance_adapter.go:65`](server/adapters/instance_adapter.go#L65) | new proto field `Session.effective_path`, dual-populated with `path` during migration | proto/wire | New field, additive; `path` deprecated after consumers migrate |
| Resolved path (near-duplicate) | `Session.working_dir` (proto field 4), populated from `GetWorkingDirectory()` — computes almost the same value as `EffectivePath` but skips `Workspace()`'s disk-existence fallback | deprecate in favor of `effective_path`; **the proto doc comment ("Directory within repository to start in") is itself wrong** — it describes `Instance.WorkingDir`, the relative-subdirectory *config input*, not what's actually on the wire | proto/wire | See §1.4 — this is a doc-drift bug independent of the rename |
| Same ambiguity, second message | `ReviewItem.path` / `ReviewItem.working_dir` (proto fields 11/12, "Session details for rich display (matching Session message fields)") | same treatment: add `effective_path`/`repo_root_path`, deprecate old, in lockstep with the `Session` message so the two never drift again | proto/wire | Same as above |
| Workspace-identity grouping key | `WorkspaceKey()` (`session/repo_path.go:675`) | **keep as-is** — already unambiguous, already a non-filesystem identity string (`"gh:owner/repo"` / `"path:<repo root>"`), not part of this ambiguity | Go internal | None |
| Workspace peer's path | `WorkspacePeer.Path` (`session/workspace_peers.go:29`, populated from raw `InstanceData.Path`, i.e. the **identity** path, not resolved) | `WorkspacePeer.RepoRoot` | Go internal | Low, same as InstanceData |
| MCP tool JSON contract | `SessionSummary.Path` / `SessionDetail.WorkingDir` (`server/mcp/types.go:26,36`) | `SessionSummary.RepoRootPath` (`json:"repo_root_path"`) / add `SessionDetail.EffectivePath` (`json:"effective_path"`) | MCP wire | See §2.3 — this is its own independent wire contract, not derived from the proto `Session` |
| Insights token summary | `SessionTokenSummary.project_path` (`proto/session/v1/insights.proto:72`) | **do not fold into this dichotomy** — rename to `decoded_project_path` (or add a doc comment) making clear it's a best-effort value decoded from Claude Code's history-directory naming convention (`session/tokens/parser.go:69`'s `decodeProjectDirName`), used only for prefix-matching against the identity path (`session/tokens/association.go:81-83`), not a resolution of either concept | proto/wire (Insights-scoped) | Rename or comment only — this is a third, structurally different kind of path |

### 1.3 Why `InstanceData.RepoRoot` doesn't need a data migration

`InstanceData` (`session/storage.go:23`) is the JSON-persisted DTO (`sessions.json` / ent).
Its `Path` field carries `json:"path"`. Renaming the *Go identifier* to `RepoRoot` while
keeping the tag `json:"path"` is a zero-migration change — the field name in Go source and
the key on disk are independent. Recommend doing exactly that: rename the identifier
everywhere for reading clarity, keep the wire/disk key `"path"` unchanged.

### 1.4 A finding that changes the shape of this refactor: `working_dir`'s doc comment is wrong

`proto/session/v1/types.proto`'s `Session.working_dir` (field 4) is documented as "Directory
within repository to start in" — that description matches `Instance.WorkingDir`, the
relative-subdirectory **override a user configures at session creation** (consumed by
`resolveStartPath` in `session/instance_worktree.go:340`). But the value actually placed on
the wire is `GetWorkingDirectory()` (`instance_adapter.go:66`), which returns the **resolved
worktree-or-repo root** — a different concept with the same field name colliding at both the
Go-method and proto-field level. This isn't just an ambiguous name; the doc comment actively
lies about what's on the wire today. Fixing this is in-scope for this refactor (it's the same
disease as `Session.path`), but Phase 3 should decide whether `GetWorkingDirectory()` is kept
as a near-duplicate of `Workspace().EffectivePath` (documented as deprecated, same as the
proto field) or consolidated to call `Workspace().EffectivePath` directly — the latter is a
behavior change to resolution logic (the disk-existence fallback), which brushes against the
Constraints' "no change to worktree detection/creation logic" boundary, so flag it as a
decision point rather than pre-deciding it here.

## 2. Data flow

```
git worktree creation
  git.NewGitWorktreeWithBranch / NewGitWorktreeFromExisting
        │  (session/instance_worktree.go: setupFirstTimeWorktree)
        ▼
  i.gitManager.SetWorktree(gitWorktree)         ◄── excluded from InstanceSnapshot
        │                                            (behavior, not data — see
        │                                            instance_snapshot.go's exclusion list)
        │
  i.Path set (session creation) or mutated later under i.mu.Lock()
        │  (session/instance_actor_setters.go, e.g. setGitHubResolutionLocked)
        ▼
  buildSnapshot(i) under i.mu.Lock()             (session/instance_snapshot.go)
        │  i.snapshot.Store(&InstanceSnapshot{Path: i.Path, ...})
        ▼
  ┌─────────────────────────────┬───────────────────────────────┬─────────────────────────┐
  │ i.Snapshot().Path            │ i.gitManager (live, unsnapshotted, own locking)          │
  │ → GetPath() [identity]       │ → HasWorktree()/GetWorktreePath() [resolved]             │
  └─────────────────────────────┴───────────────────────────────────────────────────────────┘
        │                                       │
        └──────────────┬────────────────────────┘
                        ▼
              i.Workspace() { RepoRoot, EffectivePath }
              i.GetEffectiveRootDir(), i.GetWorkingDirectory()
                        │
        ┌───────────────┼────────────────────────────────┐
        ▼                                                 ▼
  InstanceToProto (ConnectRPC)                    instanceToSummary/Detail (MCP)
  server/adapters/instance_adapter.go:65-66       server/mcp/tools_discovery.go:43-71
    Path: Workspace().EffectivePath                 Path: inst.Path        ◄── RAW, unguarded
    WorkingDir: GetWorkingDirectory()                WorkingDir: inst.WorkingDir ◄── RAW, unguarded
        │                                                 │  (see §2.3 — a real, separate bug)
        ▼                                                 ▼
  proto Session (wire, ConnectRPC)                 SessionSummary/Detail (wire, MCP JSON)
        │
        ▼
  generated TS type (web-app/src/gen/session/v1/types_pb.ts — gitignored, regenerated)
        │
        ▼
  Redux: sessionsSlice.setSessions(Session[])   (web-app/src/lib/store/sessionsSlice.ts:59)
    createEntityAdapter<Session> stores the WHOLE generated message verbatim —
    no per-field mapping/normalization layer exists.
        │
        ▼
  selectAllSessions / selectSessionById selectors
        │
        ▼
  Components read session.path / session.workingDir directly
  (SessionRow.tsx, WorkspacePeersPanel.tsx, ..., full list = sibling audit's job)
```

### 2.1 Where a rename must be threaded (nothing may be silently dropped)

1. `session/instance_actor_setters.go` — every `i.Path =` write site (compiler-enforced once renamed).
2. `session/instance_snapshot.go` — `InstanceSnapshot` field + `buildSnapshot`.
3. `session/instance_worktree.go` — `GetPath()`, `Workspace()`, `GetEffectiveRootDir()`, `GetWorkingDirectory()`, and `DetectAndPopulateWorktreeInfo`'s deliberately-raw `i.Path` read (its comment explains why it must stay raw, not `GetPath()` — same caveat applies to the renamed field).
4. `session/repo_path.go` — `WorkspaceKey()`'s two overloads (`*Instance`, `InstanceData`).
5. `session/storage.go` — `InstanceData.Path` (rename identifier, keep `json:"path"` tag — §1.3).
6. `session/instance_serialization.go` — `ToInstanceData`/`FromInstanceData` round trip (`instance_serialization.go:88,266`).
7. `session/workspace_peers.go` — `WorkspacePeer.Path` field and its population in `ListWorkspacePeers` (line 80).
8. `server/adapters/instance_adapter.go` — `InstanceToProto`: add new proto field assignments (`RepoRootPath: snap.RepoRoot`, `EffectivePath: inst.Workspace().EffectivePath`) alongside the existing `Path`/`WorkingDir` writes (dual-populate, don't replace).
9. `server/mcp/types.go` + `server/mcp/tools_discovery.go` — a wire contract **independent of the proto Session**, needs the same two fields added and its raw reads fixed (§2.3).
10. `proto/session/v1/types.proto` — new fields on `Session` *and* `ReviewItem`; `[deprecated = true]` on the old ones once migrated; regenerate via `make proto-gen` (gen/ output never committed, per this repo's convention — verified via `.gitignore` lines 31/33).
11. Frontend: no slice-level change needed (see §2.2) — only the per-component read sites (sibling audit's job).

### 2.2 Redux/RTK carries both old and new fields for free

Because `sessionsSlice.ts` stores the entire generated `Session` protobuf-es object as one
entity (`createEntityAdapter<Session>`, no field-level mapping — verified by reading
`sessionsSlice.ts:1-53`), adding new proto fields requires **zero extra plumbing** in the
Redux layer during the dual-populate phase: `session.effectivePath` and `session.path` will
simply both exist on every entity in the store the moment `make proto-gen` regenerates the TS
type and a build picks it up. This significantly de-risks the "phased rollout" requirement —
there's no normalization/selector-shape migration to design, only per-component read-site
swaps, which is exactly the kind of mechanical, low-coordination-cost migration a phased PR
split wants.

### 2.3 A concrete, pre-existing violation this refactor should also fix

`server/mcp/tools_discovery.go:43-71` (`instanceToSummary`/`instanceToDetail`, backing the
`list_sessions`/`get_session`/`search_sessions` MCP tools) reads `inst.Path`, `inst.Branch`,
`inst.Tags`, `inst.CreatedAt`, `inst.UpdatedAt`, `inst.Program`, `inst.SessionType`, and
`inst.WorkingDir` **directly off the raw `*Instance` struct**, bypassing `Snapshot()`
entirely — the exact class of bug `.claude/rules/instance-lock-free-reads.md` documents (a
background actor-setter write racing an unguarded read). This is a second, independently
discovered real violation of that rule (the rule's own example was `GetEffectiveRootDir`,
already fixed) — found empirically while tracing this refactor's data flow, not hypothesized.
It strengthens the case (Constraints: "lint-rule enforcement... Phase 3 decides
proportionality") for a `tools/lint/norawinstanceread`-style analyzer mirroring
`norawghrequest`: two real production violations of the same documented rule, found in two
unrelated code reviews months apart, is exactly the pattern that convention-only enforcement
fails to catch (per `norawghrequest.md`'s own stated rationale for why it graduated from a
rule doc to a lint analyzer). Recommend Phase 3 fix this call site regardless of whether a
lint rule is built (it's the MCP-facing equivalent of the bug the rule already exists for),
and use its rediscovery as the tie-breaker if the call-site count is borderline for justifying
the analyzer.

## 3. Backward-compatibility staging, modeled on IAC Epic 1 → Epic 2

Verified via `git log --oneline --all | grep -i "IAC Epic"` and `git show --stat e16ab45e1`:
the snapshot migration was staged as clearly separated, sequential commits, not one big-bang
change:

- **IAC Epic 1** (`447caed00`/`e8cfafc44`) — *additive infrastructure only*: introduce
  `InstanceSnapshot` + `atomic.Pointer`, publish a snapshot in every mutator. No reader
  migrated yet; old raw-field reads still work unchanged.
- **IAC Epic 2** (`e16ab45e1`/`f491483a7`, "migrate all unguarded readers to
  snapshot.Load()") — *consumer migration*, one commit enumerating every touched reader by
  name in the commit message (`InstanceToProto`, `capacity_monitor.go`,
  `review_queue_poller.go`, `pr_status_poller.go`, `connectrpc_websocket.go`).
- Later epics (2.5, 3, 4, 7) build further structure on top once the read path was safe —
  none of them re-touch the Epic 1/2 boundary.

Apply the same two-phase split here:

- **Phase A (additive, low-risk PR)**: rename Go-internal identifiers (`Path` →
  `RepoRoot`, compiler-checked, §1.2/§2.1 items 1-7); add the new proto fields
  (`repo_root_path`, `effective_path`) to `Session` and `ReviewItem`, dual-populated
  alongside the existing `path`/`working_dir` in `InstanceToProto` (item 8) and the MCP
  types (item 9, plus fixing the raw reads per §2.3 in the same pass since it's the same
  functions). Old fields untouched in meaning, still populated. `make proto-gen` regenerates
  `gen/`; nothing consumer-facing breaks because nothing consumer-facing changed yet.
- **Phase B (consumer migration, can split further per-surface)**: migrate each identified
  frontend/MCP-tool-consumer/backlog-ship-status consumer from `path`/`working_dir` to
  `effective_path`/`repo_root_path` (sibling audit's enumeration drives this list). Because
  of §2.2, this phase has no Redux-shape risk — it's a pure per-file find/replace plus a
  behavior check.
- **Phase C (out of scope for this project, per Constraints)**: mark old fields
  `[deprecated = true]` and eventually delete — a separate, later breaking-change PR once
  every consumer (including external MCP/agent consumers not controlled by this repo) has
  migrated.

This mirrors IAC Epic 1/2's justification for *why* two PRs beat one: Epic 1 alone is
reviewable in isolation (pure additive plumbing, hard to get wrong, easy to verify via
`go build`), and Epic 2 alone is reviewable in isolation (a mechanical, enumerable list of
call sites) — bundling both into one diff would make either kind of review-mistake (a missed
dual-populate site, or a consumer migrated to a field that doesn't exist yet) much harder to
catch in review. Phase A/B here has the identical shape.

### 3.1 `make registry-generate` — not required for this rename

Checked `docs/reference/feature-registry.md`: the registry's `// +api:`/`// +feature:`
markers are scoped to **RPC names and component identity** (`markerFound` on a handler,
`// +feature: <slug>` in a React file's first 10 lines), not to individual proto field names.
Adding/renaming fields on the existing `Session`/`ReviewItem` messages, or on the MCP
`SessionSummary`/`SessionDetail` structs, doesn't touch any RPC or component the registry
tracks, and no existing feature JSON in `docs/registry/features/backend/` references these
field names directly (checked — none found for `GetSession`/`ListSessions`). `make
registry-generate` is **not needed** for this work unless Phase 3's plan also introduces a
new RPC or component (e.g., a dedicated `GetWorkspacePeers` public endpoint per Open Question
3) — in that case, the *new RPC* needs a marker and a registry regen, independent of the field
rename itself.

## 4. Integration risk: `WorkspacePeersPanel.tsx` (backlog `e2430349`)

Read `web-app/src/components/sessions/WorkspacePeersPanel.tsx` in full. Its own doc comment
(lines 63-69) states the intended design precisely: peers are scoped to **the literal
directory** (`session.path`), deliberately *not* `workspaceKey` — "a peer editing a different
worktree isn't touching this directory's files." Given `session.path` is confirmed to already
be `Workspace().EffectivePath` (resolved, worktree-aware), the panel's `s.path === session.path`
comparison (line 82) is *already doing the semantically correct thing* for its stated goal —
two sessions with the same resolved effective path genuinely are working in the same
directory. This means:

- **The bug (if it reproduces) is not a naming problem** — it's a worktree-detection gap:
  something making `GetEffectiveRootDir()` fall back to the identity path (`i.gitManager.
  HasWorktree()` returning false, or `GetWorktreePath()` returning `""`) for a session that
  actually has a distinct worktree, so two genuinely-separate worktree sessions collapse onto
  the same `EffectivePath` and falsely appear to collide. Confirming/root-causing this is
  explicitly Open Question 1, owned by Phase 2's other research thread(s), not this one — but
  it changes what "trivial fix" means once this refactor's naming lands (see below).

### 4.1 How the new naming makes the eventual fix trivial

Once `Session.effective_path` exists as an explicitly-named field (§1.2), `e2430349`'s fix
becomes a rename with zero semantic change: `s.path === session.path` →
`s.effectivePath === session.effectivePath`. The literal comparison logic doesn't change at
all — only the field name, which stops a future reader from mistakenly "fixing" the panel by
swapping to `workspaceKey`-based matching (a real risk today, since `path` reads as
ambiguous and a future engineer skimming the panel's intent might reasonably reach for the
repo-identity key instead of the resolved path, silently breaking the "same literal directory"
guarantee the comment explains). If the actual root cause is a detection gap (not a naming
issue), that fix is orthogonal and lands in the worktree-detection code this refactor
explicitly excludes (Constraints: "no change to worktree detection/creation logic").

### 4.2 Sequencing risk

- **File overlap**: `e2430349`'s fix and this refactor's Phase B (frontend consumer
  migration) both touch `WorkspacePeersPanel.tsx` — the *same one line* (82) — via
  potentially concurrent PRs. `e2430349` is currently `idea` status (unstarted, per
  requirements.md's verification), so there's no PR to conflict with yet.
- **Safest order**: land this refactor's Phase A+B **first** (adds `effective_path`, migrates
  the panel's read from `path` to `effective_path` with the identical comparison — a
  same-semantics rename, not a functional change, so it's safe to include in this refactor's
  own consumer-migration pass rather than waiting). Then `e2430349`'s own fix — if it turns
  out to be a detection-gap fix rather than a naming fix — lands cleanly against the new field
  name with no merge conflict, since the rename will have already happened.
- **If `e2430349` is picked up first** (e.g., a parallel session starts on it before this
  refactor merges): the safest coordination is a one-line note in this refactor's PR
  description flagging the shared line, per the Out-of-Scope section's own requirement
  ("if this refactor's PR(s) land first, note in the PR description what the `e2430349` fix
  should now use") — reversed: if `e2430349` lands first using `session.path`, this
  refactor's Phase B migration of that same line becomes a trivial rebase (rename
  `path`→`effectivePath` in whatever `e2430349` already wrote), not a real conflict, since
  both changes touch the same one line with compatible intent.
- **Proto/backend overlap**: `e2430349` as scoped (per requirements.md) is a frontend-only
  fix (`WorkspacePeersPanel.tsx`), so it doesn't touch `proto/session/v1/types.proto` or
  `instance_adapter.go` — no backend merge risk. The only real risk is the single shared
  frontend line, which is low-severity (a rename collision, not a logic collision) and
  trivially resolved either direction.
