# Architecture Research: session-classifier-pipeline

This doc builds directly on two prior research artifacts rather than re-deriving what they
already established:

- `project_plans/dynamic-rule-reload/research/architecture.md` (`§1`, lines 3-23) — confirms
  `RuleBasedClassifier`'s shape: a single flat `[]Rule` behind `deadlock.RWMutex`
  (`pkg/classifier/classifier.go:414-417`), sorted descending by `Priority`, with no per-source
  partitioning — sources are distinguished only by the `Rule.Source` string field. `AddRules`
  (append+resort), `ReplaceRules` (atomic full-slice swap), and `Rules()` (defensive-copy read)
  are the only mutation/read primitives. `NewRuleBasedClassifier()` seeds from `SeedRules()`
  only. Where a `rebuildMu`-style serialization lock belongs (`RulesService`, not the classifier
  itself) is established there (`§3`, lines 78-153) and applies unchanged to a tagging-rule
  reload path — see §5 below.
- `project_plans/core-domain-decomposition/research/architecture.md:86` — `classifier.go` has
  **no tmux dependency**; its only `.Output()` calls are `safeexec`-wrapped `git`/`jj` subprocess
  calls. A reported 3-commit/0.30-ratio co-change signal with another file could not be confirmed
  as causal and is not re-litigated here — no action needed on that basis for this project.

Verified directly in this research (all via targeted reads, not re-derivation):
`pkg/classifier/classifier.go:365-778` (Rule/RuleBasedClassifier/matchesRule/classifySingle),
`session/pr_status_poller.go` (full file, 467 lines), `session/worktree_pr_poller.go` (full
file, 374 lines), `session/ent/schema/approvalrule.go` (full file, 100 lines),
`session/instance_tags.go` (full file, 78 lines), `session/headless/features.go` (grep +
targeted reads for `FeatureKey`/`CallOptions`/`GenerateSessionCompletionNarrative`).

## 1. Rule struct design — shared base vs. one unified struct

`pkg/classifier.Rule` (`classifier.go:365-399`) is 15 fields, all tool-use-shaped:
`ToolName`, `ToolPattern`, `ToolCategory`, `Criteria *CommandCriteria`, `CommandPattern`,
`FilePattern`, `RequireCIPassing`, `MinSessionIdleMinutes`, `Decision`, `RiskLevel`, `Reason`,
`Alternative`, plus the domain-agnostic `ID`/`Name`/`Priority`/`Enabled`/`Source`. Every match
predicate in `matchesRule` (`:723-778`) reads a `PermissionRequestPayload` and
`ClassificationContext` — there is no session-shaped input anywhere in the type.

**Recommendation: factor out a shared `RuleMeta` base, do not cram both domains into one
`Rule` struct.** Concretely:

```go
// RuleMeta holds fields common to every rule type in this package, independent of what
// domain (tool-use approval, session tagging) the rule matches against.
type RuleMeta struct {
    ID       string
    Name     string
    Priority int // higher evaluated first
    Enabled  bool
    Source   string // SourceSeed / SourceUser / SourceClaudeSettings / SourceGenerated
}

// Rule is unchanged (tool-use / approval rule). Existing field layout, existing behavior.
type Rule struct {
    RuleMeta
    ToolName              string
    ToolPattern           *regexp.Regexp
    ToolCategory          string
    Criteria              *CommandCriteria
    CommandPattern        *regexp.Regexp
    FilePattern           *regexp.Regexp
    RequireCIPassing      bool
    MinSessionIdleMinutes int32
    Decision              ClassificationDecision
    RiskLevel             RiskLevel
    Reason                string
    Alternative           string
}

// TaggingRule matches session metadata and produces a tag, not a decision.
type TaggingRule struct {
    RuleMeta
    NamePattern    *regexp.Regexp // session title
    BranchPattern  *regexp.Regexp
    PathPattern    *regexp.Regexp
    ProgramPattern *regexp.Regexp
    // RequiredTags: this rule only fires if the session already has ALL of these tags
    // (from an earlier fixpoint iteration or manual tagging). Empty = no dependency.
    RequiredTags []string
    // DependsOnTags is an alias-free name for the same concept, kept as one field
    // (RequiredTags) rather than two — avoids "which one wins" ambiguity the
    // requirements doc's field list (RequiredTags/DependsOnTags) would otherwise invite.
    OutputTag string
}
```

Why not embed both tool-use and tagging fields on one `Rule`: `matchesRule` already has to
special-case which fields are set (`ToolName != ""` else `ToolPattern != nil` else
`ToolCategory != ""`, `:725-742`) — that's tolerable within one domain because every field
*is* tool-use-relevant. Adding `NamePattern`/`BranchPattern`/`PathPattern`/`ProgramPattern`/
`RequiredTags`/`OutputTag` onto the same struct means every `Rule` value in memory carries six
always-nil/always-zero fields when used for approval classification and eight when used for
tagging — a `RuleSpec`/ent-schema/UI-form combinatorial explosion (`rules_service.go`'s
`allRuleSpecs()` aggregation, `ai-rule-generation/research/architecture.md:84`) that has to
explain to a rule author which fields apply to which "kind" of rule. That is exactly the leaky
abstraction the requirements doc's Rabbit Holes section (`requirements.md:135-140`) called out.
A shared `RuleMeta` embed keeps `ID`/`Priority`/`Source`/`Enabled` — the fields every consumer
(`RulesService.rebuildClassifier`-equivalent, ent persistence, seed-rule authoring, the rules UI)
actually needs generically — genuinely shared, while keeping domain-specific match/output fields
on domain-specific types. This mirrors the `interface-pollution-checklist`/`primitive-obsession-
checklist` skills' guidance against one struct serving two unrelated shapes.

`RuleSource` (`classifier.go:401-411`) is already domain-agnostic (a plain string-backed type)
and needs no change beyond adding `SourceGenerated RuleSource = "generated"` if the tagging
side wants an LLM-authored provenance tier distinct from `SourceUser` (not required by this
project's scope, but cheap to add now since `ApprovalRule`'s ent schema already has a bare
`source` string column with no enum constraint — see §5).

## 2. Evaluation loop — separate `TaggingEngine`, not a `RuleBasedClassifier` extension

`RuleBasedClassifier.Classify` (`:462-544`) → `classifySingle`/`classifyCompound` is deeply
tool-use-specific: compound-command splitting (`&&`/`|`/`;`), recursive-eval unwrapping
(`xargs`/`sudo`/`rtk`, `maxRecursionDepth`), `AuditCommand` AST security scanning, and a
first-match-wins `Escalate`-on-no-match fallback. None of that generalizes to "does this
session's name/branch/path/program match a regex" — a `TaggingRule` evaluation has no
compound-command concept, no recursion, and (per the fixpoint requirement) is designed to
match potentially **many** rules per pass, not stop at first match. Retrofitting `Classify`
to serve both would mean threading a type-switch or two divergent code paths through the one
already-2857-line file.

**Recommendation**: a new `TaggingEngine` type in `pkg/classifier` (new file, e.g.
`pkg/classifier/tagging.go`), reusing only the genuinely shared primitives:

```go
type TaggingEngine struct {
    mu    deadlock.RWMutex
    rules []TaggingRule // sorted by Priority descending, same convention as RuleBasedClassifier
}

func NewTaggingEngine() *TaggingEngine { ... }         // seeds from SeedTaggingRules()
func (e *TaggingEngine) ReplaceRules(rules []TaggingRule) { ... } // same shape as RuleBasedClassifier
func (e *TaggingEngine) AddRules(rules []TaggingRule) { ... }
func (e *TaggingEngine) Rules() []TaggingRule { ... }

// SessionTaggingContext is the minimal match input — deliberately not the full Instance,
// to keep the classifier package free of a session package import (avoids the import-cycle
// shape session/unfinished already has to route around for WorktreeSource, worktree_pr_poller.go:15-20).
type SessionTaggingContext struct {
    Name    string
    Branch  string
    Path    string
    Program string
    Tags    []string // tags already applied, for RequiredTags matching
}

// EvalOnce runs every enabled rule once against ctx and returns newly-matched tags
// (tags in ctx.Tags are not re-returned). Pure function of (rules, ctx) — no fixpoint
// looping here; that's the caller's job (§3), so EvalOnce stays trivially testable.
func (e *TaggingEngine) EvalOnce(ctx SessionTaggingContext) []string { ... }
```

`matchesRule`'s pattern-matching idiom (nil-pattern-means-any-value, `:725-767`) is worth
copying structurally into a new `matchesTaggingRule` helper, but not sharing by function
extraction — the input types (`PermissionRequestPayload`/`ClassificationContext` vs.
`SessionTaggingContext`) are different enough that a generic-over-both helper would need an
interface or reflection, which is worse than two ~15-line near-duplicate functions once dupl's
new-code-only gate (`CLAUDE.md`'s "Duplication and hotspot checks") is considered — two small,
independently-readable match functions is the right call, not a shared generic.

## 3. Fixpoint evaluation design

Requirements (`requirements.md:68-70`, `:130-133`) call for eager sync evaluation to a stable
point on session mutation, with an explicit iteration cap since tag-dependency chains can cycle
(rule A: tag Y present → add X; rule B: tag X present → add Y).

```go
// pkg/classifier/tagging.go
const maxTaggingFixpointIterations = 10 // authoring-bug guard, not a legitimate depth need

// ApplyToFixpoint runs EvalOnce repeatedly, folding newly-matched tags into ctx.Tags between
// iterations, until either no new tag is added or the iteration cap is hit. Returns the full
// set of newly-added tags (for logging/observability) and whether the cap was hit.
func (e *TaggingEngine) ApplyToFixpoint(ctx SessionTaggingContext) (newTags []string, capHit bool) {
    seen := make(map[string]bool, len(ctx.Tags))
    for _, t := range ctx.Tags {
        seen[t] = true
    }
    var added []string
    for iter := 0; iter < maxTaggingFixpointIterations; iter++ {
        newlyMatched := e.EvalOnce(ctx)
        progressed := false
        for _, t := range newlyMatched {
            if !seen[t] {
                seen[t] = true
                added = append(added, t)
                ctx.Tags = append(ctx.Tags, t)
                progressed = true
            }
        }
        if !progressed {
            return added, false
        }
    }
    return added, true // cap hit — log.Warn per Observability Requirements, requirements.md:180
}
```

This is a plain "re-run until no new fact" fixpoint (standard fact-saturation, not the
mathematical fixpoint-with-lattice machinery TLA+ would model) — a bounded loop is sufficient
and matches the Rabbit Holes note that an iteration cap, not cycle *detection*, is the
pragmatic fix (`requirements.md:132-133`): cycles A→X→Y→X→Y... simply stop making progress
after each tag is added once (the `seen` map), so true infinite cycles are impossible by
construction — the cap only guards against a long *chain* (A adds X, X-rule adds Y, Y-rule
adds Z, ...) exceeding a sane depth, which is the authoring-bug case the requirements doc wants
surfaced via a `Warn` log.

**Hook point**: `session/instance_actor_setters.go` has one clear integration seam — every
mutation setter (`setProgramLocked`, `setTitleDirectLocked`, and whichever setter carries
branch/path — `applyWorktreeDetectionLocked` at `:767` and `setGitHubResolutionLocked` at
`:733` are the closest name/branch/path mutators found) already follows the
`set*Locked(s *instanceState, ...)` + `(i *Instance) Set*(...)` two-layer pattern, with
`i.snapshot.Store(buildSnapshot(i))` republished after every locked mutation (per the
`instance-lock-free-reads.md` rule already in this repo's `.claude/rules/`). Rather than adding
fixpoint-evaluation calls to every individual setter (`SetProgram`, `SetTitleDirect`,
worktree-detection, GitHub-resolution), the requirements' "on session mutation" scope is best
served by **one funnel point**: a `(i *Instance) reclassifyTagsLocked(engine *classifier.
TaggingEngine)` called from the *end* of each name/branch/path/program-mutating `Set*Locked`
function, mirroring how `bumpCreationEpochLocked` (`:525`) is already a small reusable
locked-helper called from multiple setters. This keeps the fixpoint call itself trivial
(build a `SessionTaggingContext` from the in-progress `instanceState`, call `ApplyToFixpoint`,
merge `newTags` into `s.tags` via the existing `TagManager.Set`-equivalent locked path) while
avoiding a second, parallel "notify on any mutation" event-bus subscription that would run
async and violate the eager/synchronous requirement (`requirements.md:69`, "run eagerly...
before returning").

Because `TaggingEngine` needs to be reachable from `session/instance_actor_setters.go`, and
`pkg/classifier` must not import `session` (no cycle), the `*Instance` needs a reference to a
`*classifier.TaggingEngine` (or a small `SessionTagger` interface wrapping it) injected the same
way `RuleBasedClassifier` is injected into `RulesService` today — a constructor parameter /
setter on `Instance` or its owning `InstanceManager`, not a global.

## 4. Poller hook — sibling copy, not a shared base

Read in full: `session/pr_status_poller.go` (467 lines) and `session/worktree_pr_poller.go`
(374 lines). Both share a **structural idiom**, not a common base type — there is no
`Poller` interface or embedded struct either one derives from; each independently declares
`ctx`/`cancel`/`wg`, a `Start(ctx)`/`Stop()` pair, a `pollLoop()` goroutine, an
`atomic.Value`-backed auth/callback cache, and a `checkAll*`/`pollWorktrees` fan-out with a
semaphore-bounded worker pool. `WorktreePRPoller` additionally reacts to an event channel
(`p.source.ScanDone()`) *and* a fallback ticker (`pollLoop`, `:163-181`) — this is the closest
existing precedent for a poller that's triggered by upstream state changes rather than pure
time, which matters because the new LLM poller's trigger is "a session's classification-
relevant content hash changed," not a fixed schedule.

**Important terminology correction against the requirements doc**: neither existing poller
implements a debounce mechanism internally (no coalescing timer, no burst-suppression window).
`PRStatusPoller` is a pure fixed-interval ticker (`pollLoop:208-225`). `WorktreePRPoller`'s
"debounce" is delegated entirely to its upstream source's own coalescing
(`session/unfinished/scanner.go`'s `fsnotifyLoop`, per the `dynamic-rule-reload` doc's §2 table)
— the poller itself just selects on `ScanDone()` or the ticker, whichever fires first. For the
LLM classification poller, "debounced" per the requirements (`requirements.md:71-75`) really
means: **poll on a ticker, but skip any session whose content hash is unchanged since the last
LLM call** — i.e., the debounce is the content-hash cache, not a timer-coalescing mechanism.
This matches the requirements' own success metric wording ("without re-invoking the LLM on
every render/poll when the session's classification-relevant content hash hasn't changed",
`requirements.md:49-51`) more precisely than a literal fsnotify-style debounce would.

**Recommendation: sibling copy of the `PRStatusPoller` shape**, not a shared base type or
interface extraction. Rationale:
- The two existing pollers already tolerate ~70% structural duplication (ticker loop,
  semaphore fan-out, atomic auth cache) without a shared base — that's an established,
  accepted convention in this codebase, not an oversight; retrofitting a `Poller` interface
  now would touch both existing pollers for a third caller's benefit, which is out of this
  project's scope (`requirements.md`'s Out of Scope doesn't mention poller refactoring, and
  the Rabbit Holes section frames this as "reuse vs. duplicate is a real decision" without
  mandating reuse).
- The new poller's actual work (compute content hash, check cache, call `headless` LLM pool,
  parse a tag decision, call `inst.SetTags`/`AddTag`, apply `Unclassified` on failure) shares
  no fetch/ETag/GitHub-specific logic with either existing poller — the only genuinely shared
  shape is "ticker + semaphore-bounded fan-out over `[]*Instance` + atomic-cached auth-like
  gate," which is exactly the ~30 lines both existing pollers already re-implement per-file.
- A new `SessionTagClassificationPoller` (name mirrors `PRStatusPoller`) in
  `session/session_tag_poller.go`:
  ```go
  type SessionTagClassificationPoller struct {
      instances []*Instance
      pool      headless.PoolClient
      engine    *classifier.TaggingEngine // for RequiredTags context on LLM-derived tags, if needed
      config    SessionTagPollerConfig    // PollInterval, CallTimeout, ConcurrentCalls
      cache     sync.Map                  // key: session title, value: cachedTagResult{hash, tag, at}
      ctx       context.Context
      cancel    context.CancelFunc
      wg        sync.WaitGroup
  }
  ```
  wired into `wireDepsIntoServer` (`server/server.go:140-186`) exactly like
  `deps.PRStatusPoller.Start(serverCtx)` (`:145`), registered via the same `warren.Set` DI
  pattern in `server/dependencies.go` — this reuses the *lifecycle* convention the
  `dynamic-rule-reload` doc already documented (§2) without inventing a fourth pattern.
- Risk Control's "disable independently" requirement (`requirements.md:192-193`) is satisfied
  for free by this shape: not calling `deps.SessionTagClassificationPoller.Start(serverCtx)`
  in `wireDepsIntoServer` is sufficient to disable it, identical to how any of the three
  existing background components could be disabled today.

Content-hash input (an Open Question in requirements.md:206): given the Security
classification constraint (`requirements.md:88-91`, "scope the LLM prompt to session metadata
unless research determines richer content is needed and safe") and that no other component in
this codebase currently feeds diff/transcript content into a debounce-cache key, the
lowest-risk default is `sha256(name + "\x00" + branch + "\x00" + path + "\x00" + program +
"\x00" + sorted(tags))` — cheap to compute, matches the metadata-only prompt scope, and
naturally invalidates on any of the same mutation triggers that fire the sync fixpoint (§3).
Widening the hash to include diff/transcript content is a planning-phase decision this doc
defers, per the requirements doc's own Rabbit Holes framing.

## 5. Ent schema for user-editable tagging rules

`session/ent/schema/approvalrule.go` (100 lines, read in full) is a flat entity: every `Rule`
field maps 1:1 to a column (`tool_name`, `tool_pattern`, `command_pattern`, `decision`,
`risk_level`, `priority`, `enabled`, `source`, plus `programs`/`subcommands`/etc. as
`field.JSON([]string{})` for the structured `CommandCriteria` sub-object), with three indexes
(`rule_id`, `priority`, `enabled`) and no edges.

**Recommendation: a new sibling schema `TaggingRule`, not a shared/polymorphic entity.**
Mirror the *pattern* (flat fields, JSON columns for slice fields, same three-index shape,
same `created_at`/`updated_at` convention), diverge on the field list per §1's struct design:

```go
// session/ent/schema/taggingrule.go
type TaggingRule struct{ ent.Schema }

func (TaggingRule) Fields() []ent.Field {
    return []ent.Field{
        field.String("rule_id").Unique().NotEmpty(),
        field.String("name").NotEmpty(),
        field.String("name_pattern").Optional(),
        field.String("branch_pattern").Optional(),
        field.String("path_pattern").Optional(),
        field.String("program_pattern").Optional(),
        field.JSON("required_tags", []string{}).Optional().Default([]string{}),
        field.String("output_tag").NotEmpty(),
        field.Int("priority").Default(0),
        field.Bool("enabled").Default(true),
        field.String("source").Default("user"),
        field.Time("created_at").Default(time.Now).Immutable(),
        field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
    }
}

func (TaggingRule) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("rule_id"),
        index.Fields("priority"),
        index.Fields("enabled"),
    }
}
```

Why not one polymorphic `rules` table with a `kind` discriminator column and nullable
approval-only/tagging-only columns: ent's generated code (and the CRUD/RPC/UI surface built on
top of it — `RulesService`, `ListApprovalRulesRequest`'s `sourceFilter`, the rules panel) would
need a runtime `kind` check sprinkled through every consumer to know which nullable-field subset
is meaningful, reproducing exactly the leaky-struct problem §1 avoided at the Go-type level, just
pushed into SQL/ent instead. A second entity is one more `make ent-gen` target and one more
CRUD service method (`UpsertTaggingRule`/`DeleteTaggingRule`, modeled directly on
`UpsertApprovalRule`/`DeleteApprovalRule`), which is small relative to this project's Large
appetite and keeps `ApprovalRule`'s existing schema/tests/migrations completely untouched —
directly serving the "zero regressions to the existing approval-rule path" success metric
(`requirements.md:52-55`).

Per repo `CLAUDE.md`'s ent workflow: edit only `session/ent/schema/taggingrule.go`, run
`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`,
confirm `go build ./...`, and do not commit generated `session/ent/*.go` output (gitignored,
regenerated by `make ent-gen` on every build/test/lint target already).

## 6. Tech Debt Disposition

**Extend as-is (with the new-file/new-type additions above), not Refactor-first or Isolate-
via-seam.** `RuleBasedClassifier`/`Rule`/`Source` is not itself debt to be paid down —
`dynamic-rule-reload`'s research confirms it's a clean, working, actively-relied-on primitive
(atomic replace/add/read, correct locking) — and `core-domain-decomposition`'s research found
no hidden coupling (no tmux dependency) that would force an isolation seam before touching it.
The project's own constraint ("generalize... not build parallel infrastructure," `requirements.
md:65-67`) is satisfied by extending the *package* (new `TaggingRule`/`TaggingEngine` types,
shared `RuleMeta`, shared `Source`/priority conventions) while leaving the existing `Rule`/
`RuleBasedClassifier` code paths — and every line of `SeedRules()` — completely untouched,
which is what keeps the zero-regression success metric achievable without a parallel isolation
layer.

## Event-Command-Policy table

Evaluated per the instructions: this domain does involve multiple actors/systems (sync rule
engine, async LLM poller, user rule editor) with real ordering/timing rules, so the table adds
clarity beyond the linear flow prose above (unlike the `dynamic-rule-reload` doc, which
correctly judged its config-reload feature didn't need one).

| Domain Event | Policy trigger | Command | Actor/System |
|---|---|---|---|
| `SessionCreated` / `SessionRenamed` / `SessionBranchChanged` / `SessionPathChanged` / `SessionProgramChanged` | Always (synchronous, eager) | `ApplyToFixpoint(ctx)` → `SetTags`/`AddTag` | `Instance` (via `instance_actor_setters.go` funnel, §3) |
| `SyncTagApplied` | Fixpoint made progress this iteration | Re-run `EvalOnce` (next fixpoint iteration) | `TaggingEngine` |
| `FixpointIterationCapHit` | Iteration count reaches `maxTaggingFixpointIterations` | `log.Warn` (rule-authoring-bug signal, `requirements.md:180`) | `TaggingEngine` |
| `SessionMutated` (any of the above) with no sync rule match | Content hash differs from poller's cache | Enqueue for next poll tick (implicit — no explicit command, the poller's own tick picks it up) | `SessionTagClassificationPoller` |
| `PollTick` | Session's content hash not in cache / cache stale | `headless.ClassifySessionTag(ctx, pool, meta)` | `SessionTagClassificationPoller` → `session/headless` pool |
| `LLMClassificationSucceeded` | — | `SetTags`/`AddTag` with LLM-derived tag; cache result by hash | `SessionTagClassificationPoller` |
| `LLMClassificationFailed` (timeout/error) | — | `AddTag("Unclassified")`; cache result by hash (so failures don't hot-loop-retry every tick) | `SessionTagClassificationPoller` |
| `TaggingRuleCreated` / `TaggingRuleUpdated` / `TaggingRuleDeleted` | User edits a rule via CRUD surface | `UpsertTaggingRule`/`DeleteTaggingRule` → `TaggingEngine.ReplaceRules` (rebuildMu-guarded, per `dynamic-rule-reload/research/architecture.md §3`'s pattern applied to the tagging engine) | User (via rules UI/API) |
| `TaggingRulesReloaded` | `ReplaceRules` completed | Existing/future sessions re-evaluated against new rules on their *next* mutation only — no retroactive re-run (`requirements.md:124-126`, backfill explicitly out of scope) | `TaggingEngine` |

Notable ordering rule this table surfaces: `LLMClassificationFailed`'s `Unclassified` tag and a
later `SyncTagApplied` event are not mutually exclusive — if a sync rule fires *after* the LLM
poller already applied `Unclassified` (e.g., a subsequent rename triggers the eager fixpoint),
the session should end up with both the sync-derived tag and `Unclassified` unless planning
decides `Unclassified` should be removed once any real tag exists. This interaction is not
specified in requirements.md and should be resolved as a planning-phase decision, not assumed
here.
