# Adversarial Review: google-jules-integration

**Date**: 2026-09-01
**Verdict**: CONCERNS

## Blockers

None. This plan is unusually well grounded — code citations (`session/backlog.go:73`,
`session/ent/schema/item_session.go:23-24`, `session/storage.go:898`,
`session/worktree_pr_poller.go`, `github/keychain.go`) were spot-checked against
the actual repo and all matched. The `jules/` package isolation constraint is
enforced by an actual CI test (`go list -deps ./jules`, Story 1.1.2 AC4 + DoD),
not just asserted, and the data-residency opt-in is a real backend guard
(`checkEgressConsent`) plus a real frontend gate (`JulesDispatchDialog`
confirmation, e2e coverage in Story 4.1.3), not just a mention. No issue found
rises to "must resolve before writing code."

## Concerns

- [ ] **No ordering guarantee that a successful `CreateSession` is durably
      recorded before a later step can fail** — Task 2.2.1b's sequence is
      reserve → `CreateSession` → `UpdateItemSessionSessionUUID` → transition,
      and "on any error after the reservation" just ends the row as
      `dispatch_failed`. If `CreateSession` succeeds at Jules but the response
      is lost (network partition) or the subsequent DB write fails, a real
      billed Jules session now exists with no `session_uuid` recorded anywhere
      locally — the user's only recovery path is "go check jules.google.com,"
      and nothing prevents them from re-dispatching and creating a genuine
      duplicate, since Jules' `CreateSession` request carries no
      client-supplied idempotency key (Task 1.1.2b's body shape has none). —
      **Recommendation**: require the `jules session created` log line
      (carrying `jules_session`) to be emitted immediately on a successful
      `CreateSession` response, before the DB write that can still fail, so
      the real session name is always recoverable from logs even on a
      downstream failure.
- [ ] **Cross-item TOCTOU race in the spend guards** — `MaxConcurrentJulesSessions`
      and `MaxJulesSessionsPerDay` are checked via `ListOpenJulesItemSessions`/
      `CountJulesItemSessionsSince` under a *per-item* lock (Task 2.2.1a/b), which
      only serializes concurrent dispatches to the *same* item (as Story 2.2.1's
      own concurrency test proves). Two different items dispatched at the same
      moment can both read the cap as not-yet-reached and both proceed, so the
      "enforced *before* any billed API call" ceiling in Risk Control can be
      exceeded by a small margin. Blast radius is bounded (off-by-a-few, not
      unbounded), so this doesn't need to block implementation, but the plan
      states the guard as a hard ceiling without noting the race.
      — **Recommendation**: either serialize the guard-check-then-reserve
      sequence behind a single global mutex (cheap at this scale), or note the
      race explicitly and accept it as a known, bounded gap.
- [ ] **GitHub push-state races are named in requirements.md's Rabbit Holes but
      have no corresponding story** — nothing in the plan addresses what
      happens if the dispatched branch is force-pushed or deleted after
      `CreateSession` while Jules is still working from it. Jules' behavior in
      that case is unknown and unrecorded (no golden fixture, no poller
      handling). — **Recommendation**: at minimum, document the assumption
      ("branch must stay stable for the session's duration") in the
      how-to doc (Story 4.1.2), and consider surfacing it as a warning in
      `JulesDispatchDialog`.
- [ ] **Possible double-ingestion of a Jules-opened PR** — the plan's own
      Alternative B rejection text notes `GitHubPRsPlugin`
      (`session/backlog_plugin_github_prs.go`) "already ingests any PR
      regardless of author." Jules' `automationMode: AUTO_CREATE_PR` means
      Jules' own GitHub App opens the PR, which is exactly the kind of PR that
      plugin's sweep would also notice. No story verifies that a Jules-opened
      PR gets attached to the *originating* backlog item by
      `applyJulesState`/`SetBacklogItemPRAndTransition` and is **not** also
      separately imported as an unrelated new backlog item by
      `GitHubPRsPlugin`'s ordinary sweep — which would directly violate the
      stated success metric ("the same backlog UI ... not a separate
      surface"). A recent PR (#663, "PR deep links and import dedup with
      progress") suggests dedup logic may already cover this, but the plan
      doesn't cite or test against it. — **Recommendation**: add an explicit
      acceptance criterion/test in Story 2.3.2 or 4.1.3 proving a
      Jules-created PR is not independently re-imported.
- [ ] **Rollback procedure doesn't address items already `in_progress` with an
      open Jules session at the moment of disabling** — Risk Control's
      rollback says setting `jules.enabled=false` stops the poller "on its
      next tick." But any `jules_work` `ItemSession` still open at that moment
      is then never polled again, so its backlog item stays `in_progress`
      indefinitely with no mechanism to reconcile it back to `ready`/`done`.
      Contrast with the age/staleness sweeps in Story 2.3.3, which exist
      specifically to prevent this class of stuck-forever state — but they
      only run while the poller is running. — **Recommendation**: either keep
      polling already-open Jules sessions to completion even when `Enabled`
      flips to `false` (only gating *new* dispatches), or document the manual
      recovery step in the how-to doc.

## Minors

- `session/ent/schema/item_session.go:26`'s doc comment ("One of: work,
  triage, review") will go stale once `jules_work` exists as a fourth role;
  no task in Epic 2.1 touches it.
- Task 2.2.1b ("resolve owner/repo from `item.RepoPath`") doesn't cite the
  existing `github.GetOwnerRepoFromRemote` helper (`github/client.go:811`),
  which does exactly this — worth naming explicitly so the implementer reuses
  it instead of reinventing remote-URL parsing.
- `JulesSessionPoller` polls open sessions sequentially within one tick
  (Task 2.3.1b). With `CallTimeout=20s` per session and
  `MaxConcurrentJulesSessions` configurable up to its hard ceiling of 10, a
  tick with several slow/timing-out sessions could exceed the 60s
  `PollInterval`, degrading poll cadence and potentially tripping the
  frontend's "stale" badge state (Story 3.3.1 AC4) under load that isn't
  actually a failure.
- The Pattern Decisions table frames `CredentialChain`
  (`server/services/credentials.go:100`) as mirroring "the... approach used
  for GitHub tokens" — but GitHub token storage (`github/keychain.go`) is a
  separate, standalone package with no `CredentialChain`/`CredentialSource`
  concept at all; `CredentialChain` is actually the existing AI/LLM-provider
  (Anthropic/Google/OpenAI) credential-resolution chain. The design choice
  itself is sound (reuse the AI-provider chain's env-var-override behavior,
  borrow keychain storage mechanics from `session/sshremote/keystore.go`), but
  the stated precedent is inaccurate and should be corrected in ADR-003.
