# Plan: instance DTO lowering and the E11 core split (`session/ent_repository.go`)

Date 2026-11-19, measured at `0b183b1d3` (origin/main). Owner decision 2026-11-19: start the instance DTO lowering that [ent-split-plan.md](ent-split-plan.md) names as the E11 core prerequisite. Every number below came from a command in this worktree; INFERRED marks reasoning.

## Finding: the "instance DTO lowering" is mostly done

The E11 NO-GO text predates `a57266953` ("lower InstanceData, GitWorktreeData, DiffStatsData, ClaudeSessionData, PiSessionData to domain"). Today every instance read and write method on `*EntRepository` already takes or returns `domain.InstanceData` (`session/domain/instance_data.go:52`); `*ent.Session` never crosses the repository boundary through them (`sessionToInstanceData` is unexported). What is still hub-bound is small:

| Hub symbol the instance code needs | Defined in | Prod refs | Test refs | Fix |
|---|---|---|---|---|
| `LoadOptions` (type) | `session/load_options.go` | 28 occ / 24 lines | 17 occ / 17 lines | move to `session/domain` (I1) |
| `LoadMinimal`, `LoadSummary`, `LoadFull` (presets) | `session/load_options.go` | 17 lines | 23 lines | move with the type (I1) |
| `updateSkip` | `session/storage_save_dedup.go:42` | 9 lines | 14 lines | move to `session/domain` (I1) |
| `ErrInstanceDataNotFound` | `session/storage.go:379` | 15 lines | 10 lines | move to `session/domain/errors.go` (I1) |
| `stampItemSessionConversationUUID` | `session/ent_repository_conversation.go` (34 lines, ent-only) | 3 lines | 0 | moves with Delete (I4) |
| `sessionpr.DeleteOwnerPRRows` | leaf `session/sessionpr` | 1 line | 0 | no change (leaf import is allowed) |

Counting method (run on the PR head, excluding `session/ent/`): `grep -rnw <symbol> --include='*.go' session server cmd`, split by `_test.go`; "occ" is `grep -rwo` matches, "lines" is `grep -rn` lines. A reviewer's count of 40 prod lines for `LoadOptions` used a looser match; the order of magnitude (about 25-40 prod, 17 test) and the I1 sizing do not change. I1 touches about 129 lines in total (24+17+17+23 for the load options, plus 9+14 and 15+10), mostly mechanical.

Symbol scan: `python3` identifier scan of `ent_repository.go` against top-level declarations of the other non-test `session/*.go` files (output: those 5 plus constructor/migration items that stay). Before this PR, `AnalyticsData`, `DismissedFindingData`, `SubcommandDecisionCount` were also in `session/repository.go`; I0 lowered them.

## Inventory (`session/ent_repository.go`, 2,022 lines before I0, 1,774 after)

| Block | Lines | Receiver methods | Ent entities | External callers (prod) |
|---|---|---|---|---|
| Create + 8 setters/row helpers (308-566) | 259 | 1 | session, worktree, diffstats, tag, project, claudesession, claudemetadata (`tx.*` calls) | `Storage` (31 `s.Repo.*` sites in storage.go cover the instance and analytics surface together) |
| Update + 14 helpers (567-1006) | 440 | 6 | same | `Storage` incl. `s.Repo.updateSkipping` (storage.go:178, the only unexported cross-file call) |
| Delete (1007-1082) | 76 | 1 | session + item-session rows | `Storage` |
| Get/List/ByStatus/ByTag/ClaudeUUID (1083-1194; `applyLoadOptions` is a plain func) | 112 | 5 | session | `Storage` |
| Single-column updaters, artifacts, metadata (1195-1490) | 296 | 14 | session | `Storage` |
| Mapper + `*WithOptions` readers (1538-1781) | 244 | 6 | session + edges | `Storage` |
| Analytics + dismissed findings (1782-2022) | 241 | 11 | classificationanalytics, dismissedfinding | 5 prod files, 10 exported methods plus `analyticsCreate` (I0, done) |
| Constructor, `initStores`, `GetEntClient`, `MaintenanceDriver` (1-307) and `Close` plus string helpers (1491-1537) | 354 | 4 | all | stays |

The six instance rows sum to 1,427 lines (259+440+76+112+296+244), plus 38 helper lines (`nilIfEmpty`, suppressed-rule-tag converters) inside the 1491-1537 row and 34 in the conversation file. Receiver methods: counted per line range with `awk '/^func \(r \*EntRepository\)/'` on the pre-I0 file; 3+1+6+1+5+14+1+6+11 over the blocks above gives 48 total (37 after I0), the 3 and 1 of the constructor row being `initStores`, `GetEntClient`, `MaintenanceDriver` and `Close`. Instance code touches only `r.Client` (28 uses), `r.driver` (stays in the constructor) and its own helpers; `r.sessionToInstanceData` is read-side only. `GetEntClient()` has 39 prod and 178 test callers; that is a separate ent-containment question (ent-split-plan rejected it as hygiene) and does not block this plan.

## Test seams and guards

- No test reassigns `Client` or builds `&EntRepository{...}` for instance data; the only literal constructors are `ent_repository.go:79` and `:267` (`initStores` runs on both). `TestEntRepository_should_BuildEveryEmbeddedStore_When_ConstructedByAnyPath` (`ent_repository_webhook_registration_test.go:438`) asserts every embedded store is non-nil; I0 mutation-checked it (deleting the `initStores` line fails it).
- Path-keyed guards for `ent_repository.go`: `session/legacy_pr_readers_test.go:42` (row `"session/ent_repository.go": readsInstanceData`) must gain a row for each new instance file in I2; `session/pr_state_writer_guard_test.go:201,206` use it only as a synthetic fixture (no change). Comments citing the path (`wtconsistency/sweep.go:645`, ent schema comments) are cosmetic.
- Per leaf: `.golangci.yml` `isolate_<leaf>`, `tools/bzl/scoping/boundaries.txt` (`!server`, `!github`), `budget.txt`, `make gazelle`, and `//session/<leaf>:go_sources` in the `data` lists of `session/BUILD.bazel` and `server/services/BUILD.bazel`.
- Churn: 82 commits touch `ent_repository.go` since 2026-08-01 (`git log --since=2026-08-01`); the recent ones are this program's own E-series PRs. At measurement time no other open PR touched `ent_repository.go`, `storage.go` or `repository.go` (`gh pr list`, 40 newest); #1707 itself does. Re-check at each step.
- Go receiver rule: a leaf cannot host `func (r *EntRepository)`. Each block becomes methods of `instancestore.Instances`, embedded in `EntRepository` and built in `initStores`; promotion keeps every call site unchanged. `scoping move` does not rewrite receivers, so each step is a scripted rewrite plus a normalised old/new diff (I0: identical, 230 lines).

## Steps

Cost classes: S small (1 reviewer pass), M medium, L large. Waves are PRs in sequence; nothing here runs in parallel because each step edits `ent_repository.go`.

| Step | What | Files / call sites | Risk | Proof of no behaviour change | Rollback | Cost | Verdict |
|---|---|---|---|---|---|---|---|
| I0 | Analytics and dismissed findings to `session/analyticsstore`, 3 DTOs to `domain` (this PR) | 29 files changed; 8 external call-site files | low: Client-only, no hooks | existing session tests unchanged and passing; 4 leaf tests; 8 leaf mutations killed; normalised diff identical | revert the commit | M | DONE |
| I1 | Lower `LoadOptions` + presets, `updateSkip`, `ErrInstanceDataNotFound` to `domain` (no aliases) | about 50 prod and 60 test lines (sum of the rows above); scripted rewrite Done: 22 files, qualifier-only diff; `LoadDiffOnly`/`LoadForReviewQueue` moved too; `UpdateSkip` is the exported name; presets, `With*` and not-found contract pinned by new tests (10 own mutations all killed; M2-M4 survived the existing tests until `session/domain/load_options_test.go`, the Registry.Acquire not-found path (M9) was untested until `session/instance_data_not_found_test.go`); `session` budget 115/25662 to 114/25556. `session/domain` has no budget row (informational; no row added here).  | low-medium: wide but mechanical | `go build ./...`, `go vet` test compile, test-name diff empty | revert | M | DONE|
| I2 | `session/instancestore` read side: Get/List*/`*WithOptions`/mapper/`applyLoadOptions`/artifact getters, about 450 lines; new embedded `*instancestore.Instances` | 2 literal constructors, 1 guard row, 6 config files | medium: mapper is the hottest read path | `storage_test.go` and `ent_repository_test.go` unchanged and green; mapper normalised diff; mutation on each filter in `applyLoadOptions` | revert; promotion keeps callers | L | DONE |
| I3 | Single-column updaters, artifacts writers, `UpdateSessionMetadata`, `UpdateGitHubPR*` (296 lines) into `Instances` | `Storage` only | low | same tests; mutation per column | revert | M | GO after I2 (needs the type) |
| I4 | Create + helpers, Delete, `stampItemSessionConversationUUID` (about 370 lines) | `Storage`; Delete imports `sessionpr` | medium: Delete runs the item-session stamp in a tx | `TestEntRepository*` create/delete tests, tx-rollback mutation | revert | M | GO after I2 |
| I5 | Update + 14 helpers (440 lines); export `UpdateSkipping(ctx, data, domain.UpdateSkip)` for `storage.go:178` | `Storage` save path; fingerprint dedup | highest: single writer, section-skip logic | `storage_save_dedup_test.go`, section mutation per skip flag, race run on `./session -run Dedup` only | revert | L | GO after I3 and I4; NO-GO if a concurrent PR edits `Update` |
| I6 | Remove emptied `ent_repository.go` bodies, keep constructor in `session`; update the plan docs and the E11 row | docs, budget | low | `make gazelle` clean, budget lowered | revert | S | GO after I5 |

Expected end state (INFERRED): `ent_repository.go` about 300 lines; `session` loses about 1,700 non-test lines of the 25,662 now measured (`tools/bzl/scoping budget --init session`).

## Start today

I1 has no prerequisite and can start now; I0 is in this PR. Open owner decisions: none block I1-I4. I5 should wait for a quiet window on `session/storage.go`.

The GO/NO-GO verdicts above are inferred from the measurements; none was tested by attempting the step beyond I0.
