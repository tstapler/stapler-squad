# Architecture Audit: Temporal Coupling & Hotspots (2026-07-01)

Mining git history for co-change patterns and complexity×churn hotspots, using
the technique behind CodeScene (Adam Tornhill) — the same author's
open-source ancestor tool is `code-maat`. This surfaces coupling that is
invisible to static import graphs: two files that always change together
because a feature spans both, with no compile-time relationship connecting
them.

## Methodology

**Temporal coupling / co-change tool**: `code-maat` was evaluated first but
rejected for this session. `nix run nixpkgs#code-maat` resolves to a
derivation with a ~536 MiB transitive closure (GTK, cairo, alsa, dbus, etc. —
apparently pulled in by a shared build environment, not code-maat itself) and
did not fetch within a reasonable time budget. No prebuilt jar was cached
locally (`~/.m2` empty of it), and `lein` is not installed (`asdf` has no
`lein` plugin). Per the task's stated fallback preference, this was
abandoned in favor of a **self-contained Python script** implementing the
same core algorithm code-maat uses for its `coupling` analysis:

1. `git log -n1000 --name-only --pretty=format:'--%H--'` to get the changed
   file list per commit.
2. For every commit, every unordered pair of files in that commit's file list
   gets `+1` in a co-occurrence counter (`itertools.combinations`).
3. Commits touching more than 60 files are excluded from pairing (but still
   counted toward each file's revision total) to avoid mass-rename/vendor-bump
   commits creating spurious pairs — this is the same "shotgun commit"
   mitigation code-maat's real implementation uses.
4. Pair counts <3 shared commits are dropped as noise.

**Window**: last 1000 commits, spanning **2026-05-02 to 2026-06-30** (~2
months). Of those, **551/1000 commits are `github-actions[bot]`** —
confirmed via `git log --author` to be entirely automated `chore(bench):`
and `chore(demos):` commits touching only `benchmarks/*` and `docs/demos/*`
(baseline JSON/GIF regeneration on every CI run). These files were the
top revision-count entries by a wide margin (167, 161, 153 revisions) but
are pure automation noise, not architectural signal — they are excluded from
both the hotspot ranking and reported co-change pairs below. 885 commits are
human-authored (mostly Tyler Stapler) in the window.

**Complexity proxy**: no `gocyclo` output was found in the scratchpad
directory from a parallel static-analysis run, so **line count** is used as
instructed. Generated ent ORM code (`session/ent/**`, excluding
`session/ent/schema/**` which is hand-written) and generated protobuf
bindings (`gen/proto/**`, `web-app/src/gen/**`) are excluded from the hotspot
ranking for the same reason as the bot commits — they are mechanically
regenerated in bulk on any schema/proto touch and would otherwise dominate
the list with a false complexity signal (e.g. `session/ent/mutation.go` is
23,405 lines and would have topped the list at a score of 514,910 purely from
being regenerated on every ent schema change). They are **kept** in the
co-change pair analysis, because generated-file coupling to its source
(`.proto` ↔ `.pb.go`, schema ↔ mutation.go) is itself a legitimate,
uninteresting-but-confirmed signal, and coupling *through* generated files to
other hand-written files is informative.

Raw data: `revisions.csv`, `coupling.csv`, `hotspots.csv` in
`/tmp/claude-1000/.../scratchpad/` (not committed — regenerate with
`coupling_analysis.py` / `hotspot_score.py` in that directory if needed).

## Top 15 Hotspot Files (revisions × line count)

| Rank | File | Revisions | Lines | Score |
|---|---|---|---|---|
| 1 | `server/services/session_service.go` | 70 | 4,542 | 317,940 |
| 2 | `proto/session/v1/session.proto` | 39 | 2,591 | 101,049 |
| 3 | `session/instance.go` | 47 | 1,293 | 60,771 |
| 4 | `session/tmux/tmux.go` | 26 | 2,105 | 54,730 |
| 5 | `web-app/src/components/sessions/TerminalOutput.tsx` | 29 | 1,739 | 50,431 |
| 6 | `proto/session/v1/types.proto` | 34 | 1,459 | 49,606 |
| 7 | `web-app/src/components/sessions/Omnibar.tsx` | 26 | 1,490 | 38,740 |
| 8 | `web-app/src/components/sessions/SessionList.tsx` | 27 | 1,306 | 35,262 |
| 9 | `server/dependencies.go` | 35 | 1,005 | 35,175 |
| 10 | `server/services/connectrpc_websocket.go` | 23 | 1,524 | 35,052 |
| 11 | `web-app/src/lib/hooks/useSessionService.ts` | 31 | 1,099 | 34,069 |
| 12 | `session/ent_repository.go` | 21 | 1,614 | 33,894 |
| 13 | `web-app/src/components/sessions/SessionDetailView.tsx` | 23 | 1,453 | 33,419 |
| 14 | `server/server.go` | 35 | 870 | 30,450 |
| 15 | `web-app/src/components/sessions/SessionCard.tsx` | 33 | 849 | 28,017 |

Honorable mentions just outside the top 15: `server/services/backlog_service.go`
(17 revs × 1,603 lines = 27,251), `pkg/classifier/classifier.go` (10 revs ×
2,644 lines = 26,440 — low churn but very large, worth watching), `session/storage.go`
(27 × 944 = 25,488), `session/unfinished/gogit_vcs_reader.go` (21 × 1,211 =
25,431), `config/config.go` (27 × 824 = 22,248).

`server/services/session_service.go` is the runaway #1 by more than 3x the
#2 score — 70 commits in ~2 months against a 4,542-line file means it changed
roughly every third working day. This single file is the strongest
hotspot signal in the repo by a wide margin.

## Top Co-Change Pairs (temporal coupling, ≥3 shared commits, noise-filtered)

| Shared commits | Ratio* | File A | File B |
|---|---|---|---|
| 19 | 0.58 | `gen/proto/go/session/v1/session.pb.go` | `proto/session/v1/session.proto` |
| 19 | 0.49 | `proto/session/v1/session.proto` | `server/services/session_service.go` |
| 18 | 0.56 | `gen/proto/go/session/v1/types.pb.go` | `web-app/src/gen/session/v1/types_pb.ts` |
| 16 | 0.48 | `gen/proto/go/session/v1/session.pb.go` | `server/services/session_service.go` |
| 15 | 0.32 | `server/services/session_service.go` | `session/instance.go` |
| 13 | 0.37 | `server/dependencies.go` | `server/services/session_service.go` |
| 11 | 0.35 | `server/services/session_service.go` | `web-app/src/lib/hooks/useSessionService.ts` |
| 11 | 0.42 | `web-app/src/components/sessions/SessionCard.tsx` | `web-app/src/components/sessions/SessionRow.tsx` |
| 11 | 0.41 | `web-app/src/components/sessions/SessionCard.tsx` | `web-app/src/components/sessions/SessionList.tsx` |
| 11 | 0.42 | `web-app/src/components/sessions/SessionList.tsx` | `web-app/src/components/sessions/SessionRow.tsx` |
| 10 | 0.38 | `proto/session/v1/types.proto` | `server/adapters/instance_adapter.go` |
| 8 | 0.31 | `server/adapters/instance_adapter.go` | `server/services/session_service.go` |
| 7 | 0.32 | `server/adapters/instance_adapter.go` | `session/instance.go` |
| 5 | 0.28 | `server/services/approval_handler.go` | `session/tmux/tmux.go` |
| 3 | 0.30 | `pkg/classifier/classifier.go` | `session/tmux/tmux.go` |

*Ratio = shared commits ÷ min(revisions of A, revisions of B) — a rough
proxy for "when the less-frequently-changed file changes, how often does the
other change too."

### Session.Instance family — confirms, does not extend, the God Object suspicion

`session/instance.go` is one of **26 files** matching `session/instance*.go`
(`instance_tmux.go`, `instance_serialization.go`, `instance_controller.go`,
`instance_state.go`, `instance_claude.go`, `instance_status.go`,
`instance_approval.go`, `instance_terminal.go`, `instance_worktree.go`,
`instance_hibernate.go`, `instance_workspace.go`, etc. — presumably all
methods on the same `Instance` struct, split across files but not across
package or type boundaries). Pairwise co-change between any two of these is
individually modest (highest is `instance.go`↔`instance_tmux.go` type
relationships around 4-6 shared commits) because the split already
distributes changes — but in aggregate, **36 of 1000 commits touch 2+ files
in this family simultaneously**, and **42 commits touch the family AND
`server/services/session_service.go` together**. This is exactly the
pattern a file-splitting refactor produces when it splits *files* without
splitting the *type*: temporal coupling moves from "two files always change
together" to "one commit always touches 3-4 files in the same family,"
which is arguably a weaker but still present signal of the same underlying
problem. **This confirms the existing God Object suspicion around
`session.Instance` rather than revealing something new** — the file split
has not decoupled the responsibilities, just spread them across more files
that still co-change as a cluster.

### Surprising within-backend coupling

- **`server/services/approval_handler.go` ↔ `session/tmux/tmux.go`** (5
  shared commits, ratio 0.28) — the approval/review-queue service layer
  changes in lockstep with the low-level tmux pane-capture implementation.
  There is no obvious static reason `approval_handler.go` should need to
  know about tmux internals; this suggests the approval flow is reading
  something (pane content, escape sequences, control-mode state) directly
  from `tmux.go` rather than through a stable abstraction.
- **`pkg/classifier/classifier.go` ↔ `session/tmux/tmux.go`** (3 shared
  commits, ratio 0.30) — the approval-rule classifier (`pkg/classifier`,
  a leaf package with no business reason to depend on tmux specifics)
  co-changes with tmux internals. Combined with the previous pair, there's
  a recurring theme: **tmux output/pane-capture format changes ripple into
  both the classifier and the approval handler**, meaning tmux's internal
  representation is effectively part of the classifier's implicit contract,
  with no interface currently enforcing that.
- **`server/dependencies.go` ↔ `server/services/session_service.go`** (13
  shared commits, ratio 0.37) — expected for a DI/wiring file, but the
  frequency (13 out of only 35 total `dependencies.go` commits) says
  `session_service.go`'s constructor signature/dependencies change often
  enough that `dependencies.go` is essentially a second surface area for
  every `session_service.go` change, not a stable composition root.

### Expected full-stack pairs (proto changes propagating, not surprising)

The generated-file pairs at the top of the table
(`session.pb.go`↔`session.proto`, `types.pb.go`↔`types_pb.ts`,
`session.proto`↔`session_service.go`, etc.) are the expected shape for a
protobuf-first API: a proto edit regenerates Go and TS bindings, and the
handler picks up the new fields. Same for the `SessionCard.tsx` /
`SessionList.tsx` / `SessionRow.tsx` triangle — three views of the same
session-list UI family changing together is normal cohesion, not a smell.
These are included in the raw data for completeness but are not flagged as
findings.

## What This Means / Recommendations

1. **`server/services/session_service.go` is the dominant architectural
   risk in the repo by every measure used here**: highest revision count
   (70), highest hotspot score (3x the runner-up), highest project-plan
   mention count (49 distinct `project_plans/*` directories reference it),
   and it appears at the center of nearly every top co-change pair. It is a
   4,542-line file acting as the RPC handler, DI consumer, and orchestration
   point for nearly every session lifecycle operation. This is the single
   highest-value target for a decomposition pass (e.g. splitting by
   operation family — creation, lifecycle, approval, backlog — into
   separate service files/structs that `session_service.go` composes,
   rather than one file implementing all RPC methods). `architecture-review`
   or `find-refactor-candidates`, targeted specifically at this file, is the
   logical next step.

2. **`session.Instance` file-splitting hasn't decoupled the type** — 26
   `session/instance*.go` files still co-change as a cluster in ~4% of all
   commits, and touch `session_service.go` together in another ~4%. Splitting
   files without splitting the `Instance` struct's responsibilities
   (tmux lifecycle, terminal state, approval state, worktree state, VNC/CDP,
   serialization) preserves the coupling; it just spreads it across more
   files, making `grep`/diff review slightly harder without reducing the
   actual blast radius of a change. Consider whether `Instance` should be
   decomposed into composed sub-objects (e.g. `Instance` holds a
   `TmuxSession`, `ApprovalState`, `TerminalState` as distinct types with
   their own methods) rather than one struct with 26 files of methods.

3. **The approval/classifier ↔ tmux coupling is a missing abstraction
   boundary.** `server/services/approval_handler.go` and
   `pkg/classifier/classifier.go` both co-change with `session/tmux/tmux.go`
   with no compile-time dependency reason found in the static graph search
   done earlier this session — this is exactly the kind of hidden coupling
   temporal analysis is meant to catch. Recommendation: introduce a narrow
   interface (e.g. `PaneReader` / `CommandOutputSource`) that
   `approval_handler.go` and `classifier.go` depend on, with `tmux.go`
   implementing it. That would let tmux's internal format change without
   forcing a ripple through the approval/classification layer, and would
   make the current implicit contract explicit and testable.

4. **`server/dependencies.go` as a secondary session_service.go surface.**
   13 of `dependencies.go`'s 35 total commits are paired with
   `session_service.go` changes — worth checking whether `dependencies.go`
   is doing more than wiring (e.g. constructing derived config specific to
   session service behavior) that could move closer to
   `session_service.go` itself or behind a narrower constructor.

5. **`pkg/classifier/classifier.go` is large (2,644 lines) but
   comparatively low-churn (10 revisions)** — per the hotspot framing, a
   complex-but-stable file isn't urgent, but it's worth a one-time look
   given its size and its coupling to `tmux.go` above; if/when it does need
   to change, its size will make that change slower and riskier than the
   revision count alone suggests.

6. **Cross-reference confirms `session_service.go` and `session/instance.go`
   are genuinely contested, not a false positive from this single window.**
   `session_service.go` is referenced in **49** distinct `project_plans/*`
   directories and `session/instance.go` in **37** — far more than any other
   file checked (next highest: `server/server.go` at 33, `config/config.go`
   at 21, `useSessionService.ts` at 27). Neither has a dedicated ADR yet,
   which is itself notable: a file this central to this many features, with
   no architecture decision record governing its shape, is a gap worth
   closing — an ADR on "how session_service.go / session.Instance
   responsibilities should be partitioned going forward" would give future
   feature work a boundary to build against instead of continuing to widen
   the same file.
