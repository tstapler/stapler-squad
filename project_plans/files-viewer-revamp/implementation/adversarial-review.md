# Adversarial Review: files-viewer-revamp (re-review after repair)

**Date**: 2026-10-16
**Verdict**: CONCERNS (0 blockers)

Scope: only the previously reported findings plus blockers newly introduced by the repairs. Read `implementation/plan.md` (incl. "Review repair log"), the prior review, `requirements.md`, `decisions/ADR-031`. Code re-checked: `session/types.go:317-355` (`Workspace` semantics), plan text for flags, search registry, draft restore, temp sweep.

## Prior findings

| # | Prior finding | Status | Evidence |
|---|---|---|---|
| B1 | Per-scope flag override by repo path does not exist | RESOLVED | Plan L111/L116: flags global-only, cites `feature_flag_mutation.go:30-36`, `flagcache.go:27-33`, `config.go:1915-1925`; rollout has no per-scope step; AC asserts scoped updates rejected; requirements.md L78 amended as owner-visible deviation; ADR-031 L16 and how-to AC (L990) agree. |
| C1 | Search cancel keyed per session | RESOLVED | Task 1.1.2a: `SearchKey{SessionID, ClientID}`, mutex-guarded swap-and-cancel, process-wide walk semaphore (limit 2); two-client ACs per the repair log. |
| C2 | Token fast path contradicts hash rationale | RESOLVED | ADR-031 L12: always compare sha256; size/mtime only short-circuits toward Conflict; `os.Chtimes` AC listed. |
| C3 | Restored drafts cannot 3-way merge | RESOLVED | `MergeBase = Available | Unavailable`; equal token on restore means disk text is the base (sound, since token embeds sha256); changed token means 2-way, Merge disabled with reason (plan L822-L824, L871, L889). No base persisted. |
| C4 | Remote-access write exposure checked last | RESOLVED | Moved to Story 0.1.3 AC / Task 0.1.3b; 2.1.2d only enforces; ADR-031 L18, L27. |
| C5 | `Rejected` modelled two ways | RESOLVED | One shape: oneof `Saved`/`Conflict`, refusals are Connect errors with typed `ReadOnlyReason` and a code table (ADR-031 L15; plan L719-L721). |
| C6 | Split view no story/deferral | RESOLVED | Explicit deferral at plan L128 and in requirements.md. |
| C7 | Open-latency SLO unmeasured | RESOLVED | Story 1.2.1 AC + Task 1.2.1d (1 MB fixture, <300 ms with 3x CI tolerance). |
| C8 | Orphaned `.ssq-tmp-*` files | PARTIAL | Sweep and `info/exclude` added and the false "git-status noise" claim dropped, but see new concern N1. |
| C9 | Two UI paths behind `files:viewer-v2` | RESOLVED | Retirement condition (4 flag-on weeks), flag-off smoke test only, follow-up AC in 2.5.1. |
| C10 | mobile-webkit CI unverified | RESOLVED | New Task 0.1.4b in Wave 1 with `mobile-chromium` fallback as gating project. |
| Minors | flag naming, `file_metrics.go` missing, spike cadence, BOM on merge, ownership, atomic-write precedent | RESOLVED | Header note; file marked new; Task 1.1.3a re-check; server owns EOL/BOM; ownership mapped to hidden-session guard; ADR-031 cites `settings_file_atomic_write.go`. |

## Checks on the new mechanisms you named

- **Write gate `ActiveDir != "" && ActiveDir == ExistingDir`**: sound. Per `session/types.go:339-353`, `ExistingDir` differs from `ActiveDir` exactly when the active directory is gone (fallback to `RepoRoot`), so the gate blocks the hazard it targets (write aimed at the main checkout after `pause_session`). Directory-mode sessions (`WorktreeDir == ""`, `ActiveDir == RepoRoot`) stay editable, which is the intended behaviour and is flagged as an owner-veto item. Edge: if a directory-mode `RepoRoot` itself vanishes, `ActiveDir == ExistingDir` still holds, but the `os.Root` open then fails, so nothing is written. No blocker. Editing a live main checkout does raise exposure (the user's own IDE or branch may be using it); the symlink-safe `.git` deny is correctly marked mandatory in that case.
- **Global-only flag rollout**: coherent across plan, ADRs, requirements and the how-to. No drift found.
- **Draft-restore base rule**: sound (see C3). The restore-time disk read is the same version because the token includes the hash.
- **Temp-file sweep**: sweep logic is bounded (one `ReadDir`, >10 min, target dir). See N1 for the exclude side effect.

## New findings

No new blockers.

- [ ] **N1 (concern) — `info/exclude` append is an undeclared persistent mutation outside the confinement root.** Tasks 2.1.1a / ADR-031 L13 have the write path append `.*.ssq-tmp-*` to the git common-dir `info/exclude` (via go-git, failure ignored). For linked worktrees that file belongs to the user's main repository and is shared by every worktree; it is written outside the `os.Root` the ADR sells as the single confinement point, it contradicts "FileService/FileWriter writes only the target file", and go-git writers are not crash-safe (project memory). It also makes a leftover temp invisible to `git status`, so the user cannot see it. Recommend: state it in the threat model, do the append with the same temp+rename helper, skip it when the file already contains the line, and add an AC that a failure leaves writes working. Alternative: drop the exclude and rely on the sweep only.
- [ ] **N2 (minor) — walk semaphore queuing is unspecified for users.** Limit 2 across all sessions means a third concurrent search waits on the request ctx with no "queued" signal; specify a bounded wait or a `Queued` progress message so a phone does not show a silent spinner. Not a correctness issue.
- [ ] **N3 (minor) — rollout default for directory-mode editing.** Editing a main checkout is on by default pending owner veto; consider listing it as a distinct line in the Story 2.5.2 sign-off checklist so the owner decides before flipping the global flag, since the flag is global and cannot be narrowed.

## Counts

Blockers: 0. Prior findings: 1 blocker RESOLVED; 10 concerns, 9 RESOLVED and 1 PARTIAL (C8); all minors RESOLVED. New: 1 concern, 2 minors. Not re-verified: story/task counts (repair log says 12 / 37 / 85; the previous review's 81 tasks changed with new tasks 0.1.4b, 1.2.1d, 1.2.2d, 2.1.2b, which is consistent with +4).
