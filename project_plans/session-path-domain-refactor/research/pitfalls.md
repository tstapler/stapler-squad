# Research: Pitfalls / Risks — session-path-domain-refactor

Scope: risks specific to renaming/clarifying session path-like fields across Go
structs, proto messages, persisted JSON state, and the React/Redux frontend.
Verified against worktree HEAD `6692f5abb` (branched from `main`, 2026-09-12).

## 1. Proto field rename pitfalls

- **Wire safety**: protobuf wire encoding keys on field *number*, not name.
  Renaming `path`/`working_dir` in the `.proto` source while keeping their
  numbers (3, 4) is wire-compatible — no risk there, and the requirements doc's
  additive-only constraint (new fields alongside old, same numbers retained)
  sidesteps this entirely. The real risk is generated-identifier churn, not the
  wire.
- **protobuf-es (`web-app/src/gen/`) generates TS property names from proto
  field names** (camelCase), so a *rename* (not an addition) changes the
  TS property every consumer destructures — e.g. `session.path` →
  `session.identityPath`. This is compiler-enforced in `.ts`/`.tsx` (verified:
  every found consumer accesses the field as a typed property on the
  generated `Session` message class, not via bracket/dynamic string access —
  `grep -rn "session\[.path.\]"` and similar found nothing), so an incomplete
  migration **fails the build**, not a silent runtime `undefined`. The
  "silent `undefined`" failure mode the research question raises would only
  occur if a consumer read the field dynamically (`session["path"]`) or if
  the backend populated the field with an actual `undefined`/empty string
  while the frontend still expected data — neither observed in this codebase's
  current consumers.
- **No wire/JSON-name mismatch risk found**: `protojson`/`FieldMask` are not
  used anywhere against `Session` (only one unrelated comment mentions
  `protojson` in `server/services/notification_service.go:301`, about a
  `Metadata` map, not path fields). So proto3 JSON field-name compatibility
  (relevant if any consumer serialized `Session` to JSON string-keyed and
  parsed it back) is not a live concern for this refactor.
- **No normalization/mapping layer in the Redux store** — this is the most
  important finding for dimension 1. `web-app/src/lib/store/sessionsSlice.ts`
  stores the raw protobuf-es `Session` message object directly via
  `createEntityAdapter<Session, string>`; `setSessions`/`upsertSession` pass
  proto objects straight into the store with no field remapping. Every
  consumer (`SessionRow.tsx`, `SessionCard.tsx`, `SessionDetailView.tsx`,
  `WorkspacePeersPanel.tsx`, `ResumeSessionModal.tsx`,
  `OmnibarSessionResult.tsx`, `ImportExternalSessionsPanel.tsx`,
  `Omnibar.tsx`, plus additional ones found beyond the requirements doc's
  list: `useFilteredGroupedSessions.ts`, `lib/grouping/strategies.ts`,
  `useSessionRepoPaths.ts`, `lib/utils/frecency.ts`,
  `useSessionService.ts`, `NativeGitRolloutPanel.tsx`, `UnfinishedItem.tsx`,
  `AliasesManager.tsx`, `AliasPalette.tsx`, `app/page.tsx`,
  `app/history/page.tsx`, `HistoryDetailPanel.tsx`,
  `HistorySearchResults.tsx`) reads `session.path`/`session.workingDir`
  directly off the store entity, not through a selector or adapter function.
  **Consequence**: there is no single choke point to update — every one of
  these ~24 files is an independent migration call site. This raises the
  call-site count materially above what the requirements doc's seed list
  implied, which matters for both Phase 3 planning and the `jscpd` risk below.
- **Rollout-order risk is structurally mitigated, not eliminated**: this repo
  ships as one binary (`make install-service` builds web UI + Go binary
  together, single systemd unit) — there is no independent frontend/backend
  deploy lag in the normal path, so the "backend stops populating before
  frontend migrates" scenario the research question raises mostly can't
  happen through normal deployment. Two residual gaps: (a) a browser tab left
  open across a deploy runs stale JS against the new backend — safe here only
  *because* the constraint mandates additive fields (old field values keep
  flowing), so pin down in Phase 3 that no PR in this series drops population
  of an old field before every consumer (frontend *and* MCP) has migrated;
  (b) external MCP/agent consumers (`mcp__stapler-squad__list_sessions`,
  `get_session`) are not controlled by this repo's deploy and may pin an
  older field name indefinitely — explicitly out of scope for removal per the
  requirements doc, but worth a tracked follow-up note in the plan.

## 2. Test coverage gaps

- **`server/adapters/instance_adapter_test.go` (456 lines, 19 test functions)
  has zero assertions on `Session.Path` or `Session.WorkingDir`** — confirmed
  by grep for `.Path`, `WorkingDir`, `EffectivePath` in that file (no
  matches). `InstanceToProto`'s population of these two fields
  (`server/adapters/instance_adapter.go:65-66`) is currently untested at the
  adapter layer. This is a real gap: adding new snapshot/proto fields
  alongside these without adapter-level tests would let a population bug
  (wrong source field, swapped assignment) ship silently, since nothing here
  would catch it. Regression coverage for the new field(s) must add adapter
  tests, not just extend the existing `session/instance_worktree_test.go`
  suite.
- `session/instance_worktree_test.go` **does** cover the underlying resolution
  logic directly (`TestGetEffectiveRootDir_ReturnsWorktreePath_EvenWhenMissingFromDisk`,
  `TestGetEffectiveRootDir_ConcurrentWithSetGitHubResolution_NoRace`) — the
  race-safety and worktree-path-vs-missing-disk behavior is well covered at
  the `Instance` method level. The gap is specifically at the proto-adapter
  boundary, not the domain logic.
- **No snapshot/golden-file tests found for `Session` or any path-carrying
  proto message** (`grep -rln "toMatchSnapshot|toMatchInlineSnapshot"` in
  `web-app/src` combined with "session" hits nothing). No golden-file
  regeneration risk from this refactor.
- **No reflection/JSON-string-keyed assertions found** on path fields in
  either Go or TS tests — the e2e helper `tests/e2e/helpers/session-client.ts`
  types `path`/`workingDir` as real TS properties (compiler-checked), and Go
  tests access `Session.Path`/`.WorkingDir` as typed struct fields, also
  compiler-checked. A rename that misses a call site fails to compile; it
  does not silently pass with a stale assertion. The originating research
  question's specific worry (reflection/JSON-snapshot tests masking a rename)
  does not apply to this codebase's current test suite.
- **A separate, previously-invisible persistence-layer gap** (not asked about
  directly but found while tracing the fields): `session/storage.go:26-27`'s
  `InstanceData` struct — the type serialized to/from `~/.stapler-squad/sessions.json`
  — has `Path string \`json:"path"\`` and `WorkingDir string \`json:"working_dir"\`}`.
  This is a **third backward-compat surface** beyond proto and the Go struct,
  with **no schema-version/migration mechanism** (`grep` for
  `SchemaVersion`/`migrat` in `storage.go` found none). If the JSON tags
  (not just the Go field names) are renamed to match a new domain vocabulary,
  every existing on-disk session loses that field's value on the next load
  (`encoding/json` silently zero-values missing keys) — a real, user-visible
  data-loss risk on upgrade, not just a test gap. **Recommendation for Phase
  3: keep `InstanceData`'s JSON tags exactly as-is (`"path"`, `"working_dir"`)
  even if the Go field names or accessor names change**, and treat any JSON
  tag rename as requiring an explicit load-time migration path, tested against
  a fixture written with the old tag names.

## 3. This exact class of bug elsewhere

- **`.claude/rules/instance-lock-free-reads.md`** documents one prior
  confirmed race (raw `i.Path` read racing `setGitHubResolutionLocked`'s
  write) — already known context, not re-derived here.
- **A second, more directly on-point incident is *in flight right now*,
  concurrently with this research, in a sibling worktree** (branch
  `worktree-agent-a444663aef6c88205`, commit `2fba774a6`, authored
  2026-09-12 12:37:26 — after this worktree's HEAD `6692f5abb` at 12:08:16,
  and **not yet merged to `main`** as of this research: `git merge-base
  --is-ancestor 2fba774a6 origin/main` → not an ancestor). Commit message:
  > "WorkspacePeersPanel compared raw `session.path` to detect sibling
  > sessions in the same directory, but path is the original repo path set
  > once at session creation and never updated for worktree sessions — so
  > two sessions in separate, properly isolated worktrees of the same repo
  > reported identical path and were falsely flagged as colliding.
  > Compare `gitWorktree.worktreePath` (falling back to `path`) instead..."
  >
  > Backlog: `e2430349-8891-4067-a6c3-0d1383ff2fd4`

  This is exactly backlog item `e2430349` — the same bug the requirements doc
  calls "still unfixed on `main`" (true) but a fix is actively landing in
  parallel. **Two important consequences for this refactor's plan**:
  1. **Coordination risk, not just a note-in-PR-description**: if this
     refactor's PR(s) touch `WorkspacePeersPanel.tsx` (they will, per the
     consumer list) before `2fba774a6` merges, there is a real merge-conflict/
     duplicate-work risk on the same two files
     (`WorkspacePeersPanel.tsx`, `WorkspacePeersPanel.test.tsx`) — check
     `e2430349`'s live status before Phase 3/5 touch that component, not just
     at requirements time.
  2. **The two investigations disagree about what `session.path` currently
     means**, and that disagreement is itself the strongest evidence for why
     this refactor matters: the requirements doc (this session, verified
     against `instance_adapter.go:65`) states `Session.path` = `EffectivePath`
     (worktree-aware, resolved). The concurrent fix's commit message states
     the opposite — "path is the original repo path set once at session
     creation and never updated for worktree sessions." Both cite the same
     backend line. **This is not yet reconciled by this research pass**
     (reconciling it is Open Question 1, assigned elsewhere in Phase 2) — but
     the fact that two careful, close readings of the same code reached
     opposite conclusions about an ambiguously-named field is itself a
     concrete, dated instance of the exact failure this refactor sets out to
     prevent, independent of which reading is correct.
- **A related but distinct incident**, `2588b9328`/`f4601c6cc` "native merge
  dirty-worktree check false-positives on unrelated changes" (#742,
  2026-09-08): a blanket `status.IsClean()` check refused merges whenever any
  path in the worktree was dirty, even paths the merge never touched. Not a
  naming-ambiguity bug (it's a scoping bug in dirty-check logic, explicitly
  out of scope per the requirements doc's "no change to worktree
  detection/creation logic" constraint), but it's the same *shape* of failure
  — an operation reasoning about "the worktree" at the wrong granularity —
  and worth citing as evidence that worktree-path reasoning in this codebase
  has a track record of subtle bugs, reinforcing the case for a structural
  fix over a documentation-only one.
- **Earlier history of the same panel's scope oscillating**: `ab511c42b`
  ("scope workspace peers to literal path", predates this session) had
  *already* moved `WorkspacePeersPanel` from matching on `workspaceKey`
  (repo identity, worktree siblings included) to `session.path` (literal) —
  the opposite direction from what `2fba774a6` just did. This one component
  has flipped its path-matching semantics at least twice, each time in
  response to a real bug report, which is a strong signal that whatever
  naming/API shape this refactor lands on should make the identity-vs-
  resolved choice obvious enough that a third flip doesn't happen — the
  requirements doc's success metric ("a reader should not need to check the
  backend implementation to know which one to use") is directly aimed at
  this exact history.

## 4. Duplication/complexity gate risk

- **Go side (`dupl`, threshold 150, `--new-from-rev=origin/main` diff-scoped)**:
  low risk. The rename's Go-side surface is backend producer code
  (`instance_adapter.go`, `session/instance_snapshot.go`,
  `session/instance_worktree.go`) plus call sites — these are single-line
  field accesses/assignments, not new repeated logic blocks, and `dupl` only
  flags newly-introduced duplication in the diff. No new shared helper
  pattern is anticipated across many files (the resolution logic already
  lives once, in `Workspace()`/`GetEffectiveRootDir()`/`GetWorkingDirectory()`;
  this refactor names/exposes it, it doesn't reimplement it per call site).
- **web-app side (`jscpd`, absolute 0.12% threshold, no diff scoping) is the
  real exposure, and it is higher than the requirements doc's framing
  suggests**, given the consumer-count finding in §1: ~24 files each doing an
  isolated `session.path` → `session.<newName>` substitution is genuinely
  low-duplication-risk *if* it stays that shape (single-token rename,
  `jscpd`'s `minLines: 20`/`minTokens: 200` floor is well above a one-line
  read). But several of the actual consumers are not simple reads:
  - `WorkspacePeersPanel.tsx` and `useSessionRepoPaths.ts` implement
    *comparison/matching logic* over path fields (per the `2fba774a6` finding
    above, `gitWorktree.worktreePath` with a fallback to `path` — conditional
    fallback logic, not a bare read).
  - `lib/grouping/strategies.ts` and `useFilteredGroupedSessions.ts` likely
    group/filter by path (session grouping is documented as one of 8
    strategies including "Path" per the root `CLAUDE.md`), which is exactly
    the kind of small-but-nontrivial logic block that, if the *same*
    fallback/comparison snippet gets copy-pasted into 2-3 of these files
    instead of extracted into one shared helper, would tip `jscpd`'s
    absolute threshold (it has no way to know only your diff should count,
    unlike `dupl`).
  - **Mitigation**: extract any comparison/fallback logic (e.g. "resolved
    path, falling back to identity path" — the exact pattern `2fba774a6`
    just introduced in `WorkspacePeersPanel.tsx`) into one shared TS helper
    (mirroring the backend's `GetEffectiveRootDir()` pattern) before this
    refactor's frontend PR(s), and have every comparison site call it. This
    is the same fix class the repo's own `dupl` gate already pushes Go code
    toward (Extract/Move Function) — Phase 3 should budget for one small
    shared frontend utility, not assume every consumer is a bare rename.
  - Note also the **jscpd threshold was already raised once** (0.10% →
    0.12%, 2026-09-12, same day as this research) after one new test file's
    irreducible `jest.mock()` blocks tripped it — meaning the current headroom
    against 0.12% is unknown/thin at the moment this refactor lands scope
    should confirm current jscpd % via `pnpm run lint:duplicates` in Phase 3
    before assuming margin exists.

## 5. `session/ent/*` and feature-registry staleness risk

- **`session/ent/*` generated code**: no risk found. `session/ent/schema/`
  (the hand-written source) has no field related to `Instance.Path`/
  `WorkingDir`/`EffectivePath` — the only `Path`-named field in any ent
  schema is `backlog_item.go`'s `ShippedFileStat{Path,...}` JSON blob, an
  unrelated per-diff-file-path record captured at ship time, not session
  identity/location. Session `Instance` state is persisted via
  `session/storage.go`'s own JSON encoding (see §2's `InstanceData` finding),
  not through ent at all. This refactor has no ent-regeneration surface.
- **Feature registry (`docs/registry/features/*.json`)**: no risk found.
  These files are generated by `make registry-generate` scanning `// +api:`
  and `// +feature:` marker *comments* (RPC/component names), not proto field
  names — confirmed by inspecting `workspace-peers-panel.json`'s schema
  (`id`, `component`, `path`/`filePath` = the **source file's** path, not any
  session-path field) and by grepping every `+api:`/`+feature:` marker in
  `server/services/*.go` for "path"/"working" — the only hit,
  `session_service.go:2913`'s `// +api: session:preview-destination-path`,
  names the `PreviewDestinationPath` RPC itself (an unrelated
  destination-preview feature for session creation), not the `Session.path`
  field this refactor touches. Renaming `Session.path`/`working_dir` will not
  make any registry file stale and does not require a `make
  registry-generate` run as part of this work (only run it if a marker
  comment itself changes, which this refactor isn't expected to touch).

## Summary of findings not explicitly asked for but material to Phase 3

1. The Redux store has no normalization layer — every one of ~24 consumer
   files is an independent call site (§1), which is materially more than the
   requirements doc's seed list and should reset the call-site-count
   assumption feeding the "is a lint rule proportionate" question.
2. `session/storage.go`'s persisted `InstanceData` JSON tags are an
   unguarded, unversioned third backward-compat surface (§2) — keep the JSON
   tags stable regardless of what the Go field/accessor names become.
3. A live, concurrent, unmerged fix for backlog `e2430349` exists right now
   in a sibling worktree touching the exact file this refactor must also
   touch (§3) — check its merge status before Phase 5 implementation starts.
