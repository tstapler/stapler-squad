# Research: Feature Landscape — Session Auto-Classification/Tagging

## 1. Existing auto-classification in this codebase

**There is no existing generalized auto-tagging engine for sessions.** What looks like
auto-tagging today is narrow, hardcoded logic, not a rule engine:

- `session.TagBacklogWork` / `TagBacklogReview` (`session/backlog.go:95,97`) are plain string
  constants. The *only* place either is ever applied is
  `server/services/backlog_service_triage.go:1064`, where `spawnTags := []string{session.TagBacklogWork}`
  is hardcoded at the moment a session is spawned from a backlog item — i.e. a single
  special-cased call site, not a rule evaluated against session attributes. `HasTag(TagBacklogWork)`
  is then read in `server/dependencies.go:801` and `session/session_driver.go:767,773` to drive
  nudge/archive behavior, but nothing ever *removes* or *re-evaluates* these tags.
- The `Category` field auto-migrates to `Tags` once, on first load
  (`session/instance_serialization.go:253`) — a one-shot backward-compat shim, not ongoing
  classification.
- Tag mutation primitives (`session/instance_tags.go`: `AddTag`, `RemoveTag`, `HasTag`, `SetTags`)
  are otherwise only driven by direct user action through the Tag Editor Modal
  (`docs/reference/tag-organization.md`).

**The mature, reusable engine this project must generalize is `pkg/classifier`**, built for
tool-use approval classification, not session tagging:

- `classifier.Rule` (`pkg/classifier/classifier.go:365-399`) is a flat struct mixing generic
  fields (`ID`, `Name`, `Priority`, `Enabled`, `Source`) with tool-use-specific match fields
  (`ToolName`, `ToolPattern`, `ToolCategory`, `Criteria *CommandCriteria`, `CommandPattern`,
  `FilePattern`, `RequireCIPassing`, `MinSessionIdleMinutes`) and tool-use-specific outputs
  (`Decision ClassificationDecision`, `RiskLevel`, `Reason`, `Alternative`).
- `RuleBasedClassifier` (`classifier.go:414-445`) holds `rules []Rule` sorted by `Priority`
  descending; `NewRuleBasedClassifier()` seeds from `SeedRules()`, `ReplaceRules`/`AddRules`
  re-sort on mutation. `Classify` → `classifyInternal` → `classifySingle`/`classifyCompound`
  orchestrate compound-command splitting; `matchesRule` (`classifier.go:723-774`) is the actual
  per-rule predicate — first-match-wins by priority, no fixpoint/iteration concept at all today.
- `RuleSource` (seed/user/claude-settings) provenance (`classifier.go:401-410`) is the exact
  provenance model this project is told to reuse for tagging rules.
- Persistence/CRUD precedent: `server/services/rules_store.go` defines `RuleSpec` — a
  JSON-serializable mirror of `classifier.Rule` (regexes stored as strings, compiled on load) —
  backed by SQLite via `session.Storage.UpsertRule`/`DeleteRule` (`session/storage.go:774,779`,
  ent-backed). `RulesService` (`server/services/rules_service.go`) wraps this with
  `ListApprovalRules`/`UpsertApprovalRule`/`DeleteApprovalRule`/`BulkUpsertRules`/`ExportRules`/
  `ValidateRules`, exposed over ConnectRPC via `SessionService` (`server/services/session_service.go:5137+`).
  This is the "same surface" the requirements doc points integration at — a new tagging-rule type
  most plausibly becomes a sibling `RuleSpec`-shaped table/service reusing this CRUD/provenance
  scaffolding, not a new schema built from scratch.
- Notably, `RulesService.GenerateSuggestedRule` (`rules_service.go:909`) already has an
  LLM-assisted flow — but it uses AI to *author* candidate rules for human approval (read-only,
  never auto-upserts), which is a different LLM-use pattern than this project's *ongoing,
  per-session* LLM fallback classification.

**Poller precedent**: `session/pr_status_poller.go` and `session/worktree_pr_poller.go` are
near-identical `time.NewTicker`-based loops (`PollInterval` default 60s) iterating over a held
`[]*Instance` list, with ETag-based conditional-fetch caching and an auth-cache/backoff guard —
directly the shape to mirror for the LLM poller, but neither has any debounce-by-content-hash
concept today (that would be new). No existing shared poller base/registration point was found —
each poller is a standalone struct wired up independently, so a new poller likely means another
independent copy-following-precedent rather than slotting into shared infrastructure.

## 2. Industry patterns

**GitHub-ecosystem issue/PR labelers** (`actions/labeler`, `github/issue-labeler`,
`srvaroa/labeler`, `coder/labeler`) converge on a YAML-configured include/exclude regex rule set
with AND/OR boolean composition (any/all keys) over multiple signals (title, body, changed files,
branch name). Two points transfer directly to this project:
- `github/issue-labeler` embeds a *version identifier* in the regex config so historic issues
  aren't silently re-classified when regex rules change later — directly relevant to this
  project's "rename/re-branch after initial tagging" edge case: rules should be versioned/pinned
  or at minimum the tagging pipeline needs to decide explicitly whether a rule-set change
  retroactively re-tags existing sessions (out of scope here per the retroactive-reclassification
  exclusion, but the *precedent* for why that's hard is worth citing).
- `coder/labeler` is the closest industry precedent to this project's exact hybrid: regex handles
  cheap/obvious exclusions, an LLM (GPT-4o) does the actual classification call — explicitly
  chosen over a vector-DB/embedding approach for simplicity, matching this project's regex-first/
  cheap-LLM-fallback shape.

**Zendesk/Linear-style support-ticket auto-triage** is explicitly a three-tier pattern industry
has converged on:
1. Deterministic rule/trigger layer for the "easy 30%" (keyword/regex → category).
2. LLM classification layer for ambiguous cases, returning category + confidence.
3. **Confidence-threshold gate**: below-threshold LLM results route to a human/fallback queue
   with reasoning attached, rather than forcing a guess — directly analogous to this project's
   `Unclassified` fallback tag on LLM failure/timeout, except Zendesk's pattern extends the same
   idea to *low-confidence* results, not just outright failures. Worth considering whether the
   LLM step here should also emit a confidence signal and fall back to `Unclassified` (or a
   distinct `LowConfidence` tag) below a threshold, not only on hard failure/timeout.
   Zendesk also always writes *some* classification "even if earlier steps encountered errors" —
   i.e., the tagging write must be the last, most-robust step in the pipeline, matching this
   project's "apply Unclassified on failure" requirement.
4. Corrections-as-feedback: manual re-tags by a human are treated as training signal, reviewed
   periodically. This project's "user pins a tag" edge case is one increment short of that full
   loop — worth flagging as a natural fast-follow (feed manual corrections back into rule/prompt
   tuning) even though full retraining is out of scope.

**Gmail filters** are the cautionary counter-example on rule-conflict semantics: Gmail applies
*all* matching filters cumulatively rather than first-match-wins, with **no documented, reliable
ordering guarantee** and **no way for a user to see why a label was applied or to reorder rules in
the UI** — sources report contradictory actions (star vs. delete) resolving unpredictably, and the
only user recourse is manually deleting/recreating filters. This is strong evidence *against*
adopting an "apply-all-matching, no visible precedence" model for session tagging: this project's
existing `Priority`-ordered, single-classifier-instance model (already used for approval rules) is
the right base to extend, and the fixpoint design should preserve deterministic, inspectable
ordering (priority + iteration number) rather than converging to an unpredictable "whatever fired
last" state, matching Gmail's exact known failure mode.

## 3. Edge cases and failure modes to design for

- **Rename/re-branch after initial tagging.** Today, nothing re-evaluates tags after creation
  (confirmed above: `TagBacklogWork` is spawn-time-only). The requirements explicitly require
  sync rules to run on *every* mutation (name/branch/path/program change), not just creation — so
  this must be wired at the actor-setter level (`session/instance_actor_setters.go`'s
  `SetTitleDirect`, `SetProgram`, `SetWorkingDir`, `SetGitHubResolution`, etc.), each triggering a
  fixpoint re-run. A rule-added tag from a *stale* branch name must not persist once the branch
  changes and no longer matches — implying rule-added tags need provenance (which rule/rule-ID
  added them) so they can be *retracted* when their triggering condition no longer holds, distinct
  from tags a rule never touches.
- **Rule-added vs. user-added tag ownership.** None of the existing `AddTag`/`RemoveTag`/`SetTags`
  primitives distinguish tag origin (`session/instance_tags.go`) — a `[]string` has no per-tag
  metadata today. Per the Gmail lesson above (unpredictable stacking of automatic actions) and the
  Zendesk "agents can override AI classification" precedent, the design needs at minimum a
  same-session-persisted marker of *which tags are rule-derived* so that: (a) a rule can safely
  remove a tag it previously added when conditions change, without touching a user's manually
  added identical-string tag; (b) a user's manual removal of a rule-derived tag should "stick" —
  i.e. be remembered as a pin/exclusion so the next fixpoint pass doesn't silently re-add it,
  mirroring Zendesk's "agents can update field values if necessary" without the AI immediately
  overwriting the correction. This likely requires extending the tag data model beyond a bare
  `[]string` (e.g. tag → {source: rule-id|user, pinned: bool}), which is a larger surface change
  than "add a rule engine" alone and should be flagged explicitly in planning.
- **Rules firing on transient state.** A branch-name rule firing mid-rebase/mid-rename window, or
  a program-detection rule firing before a worktree resolves, could apply then immediately need
  retraction — the eager-fixpoint-on-every-mutation design already covers *convergence*, but the
  observability requirement ("log when a sync rule fires") should also log *retractions*, not just
  applications, so flapping is visible rather than silently oscillating.
- **Fixpoint non-termination.** Explicitly called out in Rabbit Holes: tag-dependency cycles (rule
  A: has-tag-Y → add-X; rule B: has-tag-X → add-Y) need an iteration cap with a loud log line when
  hit (already planned) — `classifier.go` has zero prior art for iterative/fixpoint evaluation
  (`Classify` is single-pass, first-match-wins per priority), so this is wholly new logic, not a
  generalization of an existing loop.
- **Sync-fixpoint vs. async-LLM-poller write race.** Both paths ultimately call into
  `AddTag`/`RemoveTag`/`SetTags` on the same `*Instance`. `session/instance_tags.go`'s current
  methods should already hold `i.mu` for the mutation itself (consistent with the
  `instance-lock-free-reads.md` rule that mutations happen under `i.mu.Lock()` and republish via
  `Snapshot()`), so single-tag-add/remove races are likely already safe at the field level —  the
  real risk is *logical* races: the LLM poller reads a content hash, computes a classification
  tag, and by the time it writes, a sync-rule fixpoint (triggered by a concurrent rename) has
  already changed the session's tag set out from under the assumption the LLM's decision was based
  on. The LLM write should be conditioned on the content hash still matching current session state
  at write time (compare-and-set style), not a blind `AddTag` after the async call returns.
- **LLM cache invalidation granularity.** The content-hash definition (open question, deferred to
  planning) determines whether a rename that doesn't change classification-relevant content
  (e.g. a cosmetic title tweak) triggers a wasted re-classification — worth scoping the hash to
  exactly the fields sync rules already key on (name/branch/path/program) plus whatever richer
  content research decides is needed, so hash churn tracks classification-relevant churn only.

## 4. Unstated user needs

- **Tag provenance visibility ("why was this tag applied?").** Every industry precedent surveyed
  (Zendesk confidence scores + reasoning attached to human-escalated tickets, GitHub labeler
  configs being inspectable YAML, `coder/labeler`'s explicit rule-vs-LLM split) treats
  explainability as core, not optional — and this codebase already has a UI precedent for it:
  `RulesService.attachConflictInfo` / `GetApprovalAnalytics`'s `TopTriggeredRules` show which
  approval rule fired and how often. Users will expect the same for tags — hovering a tag pill to
  see "applied by rule `branch-bugfix-prefix`" or "LLM classification, cached Tue 2pm" — even
  though the requirements doc doesn't explicitly ask for it. This should be scoped into planning
  as a near-free addition given the existing analytics pattern, not deferred as UI-redesign scope.
- **Per-rule enable/disable.** `classifier.Rule.Enabled` already exists as a field and
  `RulesService`/`RulesStore` already support toggling it for approval rules — the tagging-rule
  CRUD surface should expose the same toggle from day one; it would be a regression in
  expressiveness (compared to the approval-rule UI users already have) not to.
- **Pinning/override immunity.** Directly related to the rule-vs-user tag ownership edge case
  above: users will want a way to say "this tag is mine, no rule may ever remove or re-add over
  it" — the Zendesk pattern's "agents can update field values" implies correction sticks, and
  without an explicit pin mechanism, the fixpoint re-run on every mutation (a hard requirement)
  will otherwise fight a user's manual edit on the very next session rename.
- **Seeing *why* the LLM chose Unclassified vs. a real tag** (e.g. timeout vs. low-confidence vs.
  genuinely ambiguous) — the observability requirements call for logging outcome/cost/latency,
  but users watching the UI will want at least a coarse surfaced reason (distinct from a bare
  `Unclassified` pill) mirroring Zendesk's confidence-score-on-the-ticket pattern, so a stalled/
  misconfigured LLM poller doesn't look identical to "the session is genuinely hard to classify."
</content>
