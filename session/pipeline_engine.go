// PipelineEngine is the seam that WriteSlashCommands, headless-triage prompt
// construction, review-gate prompt construction, initial-session-prompt
// construction, and mode-content-hash lookup consult instead of calling the
// pre-existing hardcoded functions directly.
//
// PipelineEngine is a SIBLING of WorkflowEngine (session/workflow_engine.go),
// not an extension or wrapper of it. WorkflowEngine governs which backlog
// *status transitions* are structurally/gate-legal; PipelineEngine governs
// *what content* (slash commands, prompts) drives an item's pipeline within
// whatever status it's already in. The two interfaces have disjoint call-site
// sets and disjoint reasons to change — coupling them (e.g. having
// PipelineEngine call into WorkflowEngine, or extending WorkflowEngine with
// pipeline methods) would pull unrelated concerns together for no benefit.
// Both are held as independent fields by their callers (BacklogService,
// BacklogLifecycleListener) and composed by the caller, never by each other.
// See project_plans/backlog-configurable-pipeline/implementation/plan.md's
// Pattern Decisions table ("PipelineEngine ↔ WorkflowEngine relationship")
// and research/architecture.md §1 for the full reasoning.
package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/log"
)

// PipelineMode identifies which PipelineMode definition (by slug) drives a
// backlog item's triage/work/review content.
type PipelineMode string

// PipelineModeDefault is the sentinel PipelineMode value meaning "no mode
// chosen — use the pre-existing hardcoded pipeline." Resolving this value is
// guaranteed, by construction, to never touch pipelineModeCache or
// PipelineModeRepository — this is the concrete mechanism that keeps "no
// uncached DB read on the hot path for the common case" true without needing
// runtime feature-flagging.
const PipelineModeDefault PipelineMode = ""

// ResolvedModeLabel renders a raw BacklogItemData.PipelineMode string for
// [PipelineEngine]-prefixed log lines: the empty string (PipelineModeDefault)
// becomes "default" for log readability, any other slug is passed through
// unchanged. Exported so both server/services (TriggerTriage) and
// session (ReviewGateRunner.Run) call sites use one shared rendering — see
// Story 1.7.2's observability acceptance criteria.
func ResolvedModeLabel(mode string) string {
	if mode == "" {
		return "default"
	}
	return mode
}

// PipelineEngine is the narrow (5-method) seam described in the package doc
// comment above. CachingPipelineEngine is its single concrete implementation.
//
// The method count (5) intentionally exceeds the usual 1-3-method interface-
// segregation guidance: each method shares the same resolve-and-render
// mechanism and has a genuine, independently-verified caller elsewhere in the
// codebase (see plan.md's Pattern Decisions row on method count) — splitting
// them into multiple interfaces would be exactly the kind of speculative
// interface-pollution .claude/rules/interface-pollution-checklist.md warns
// against, not less of it.
type PipelineEngine interface {
	// SlashCommandSet returns the filename→rendered-content map that
	// WriteSlashCommands writes to .claude/commands/backlog/ for item.
	SlashCommandSet(item *BacklogItemData) (map[string]string, error)
	// TriagePromptFor builds the headless-triage prompt for item.
	TriagePromptFor(item *BacklogItemData, artifactAbsPath string) string
	// ReviewPromptFor builds the headless-review prompt for item.
	ReviewPromptFor(item *BacklogItemData, acSnapshot []AcCriterion, diff string, diffTruncated bool, verificationNotes string, extras ReviewContextExtras) string
	// InteractiveReviewPromptFor builds the tool-call-style review prompt used
	// by the automatic review gate's real, hidden session.Instance (see
	// ReviewGateRunner.Run) — the review path most items actually go through.
	// Unlike ReviewPromptFor (JSON-output style, for headless callers with no
	// tool access), this asks the reviewer to call submit_review_verdict.
	// PipelineModeDefault renders BuildReviewPrompt; a resolved custom mode
	// renders the same ReviewPromptTemplate field ReviewPromptFor uses — mode
	// authors must include verdict-tool-call instructions in that template if
	// they want it to drive the real review gate too.
	InteractiveReviewPromptFor(item *BacklogItemData, acSnapshot []AcCriterion, diff string, diffTruncated bool, itemSessionID string, verificationNotes string) string
	// InitialPromptFor builds the interactive/autonomous session's initial
	// prompt (inst.Prompt).
	InitialPromptFor(item *BacklogItemData, priorSessions []ItemSessionSummary) string
	// ContentHashFor returns the content hash of a resolved mode's 9 raw
	// content-template fields. ok is false for PipelineModeDefault (code-
	// backed content can't drift without a redeploy — nothing to hash) or an
	// unresolved slug.
	ContentHashFor(mode PipelineMode) (hash string, ok bool)
}

// resolvedPipelineMode is an unexported, immutable snapshot of one enabled
// PipelineMode's content, held inside pipelineModeCache. It is deep-copied
// (by value, field by field) from *ent.PipelineMode at cache-load time so
// concurrent readers never observe a partially-updated ent object during a
// cache swap — mirrors NewDefaultWorkflowEngine's deep-copy-on-construct
// discipline (session/workflow_engine.go).
type resolvedPipelineMode struct {
	Slug string
	Name string

	StatusCommandTemplate string
	DoneCommandTemplate   string
	FailCommandTemplate   string
	ReviewCommandTemplate string
	ShipCommandTemplate   string
	HelpCommandTemplate   string
	TriagePromptTemplate  string
	ReviewPromptTemplate  string
	InitialPromptTemplate string

	// ContentHash is computed once at load time: SHA-256, hex, truncated to
	// 16 characters, over the 9 content-template fields above concatenated in
	// the fixed declaration order shown here (see ComputeContentHash).
	ContentHash string
}

// pipelineModeCache is an in-process, copy-on-write cache of enabled pipeline
// modes keyed by slug.
//
// Get is lock-free: a single atomic Load + map lookup, and it never touches
// writeMu — so a reader is never blocked behind a concurrent writer.
//
// Load/Invalidate share one writer-serialized sequence (acquire writeMu →
// DB read → build new map → atomic Store → release writeMu). Holding writeMu
// across the DB read itself (not just around the Store) is what prevents a
// slower concurrent caller's Store from landing after a faster, later-started
// caller's Store and silently reverting the cache to stale data — a lost
// update that a bare atomic.Pointer does not prevent on its own. See
// project_plans/backlog-configurable-pipeline/implementation/plan.md's
// Pattern Decisions row on cache concurrency for the full rationale.
type pipelineModeCache struct {
	ptr     atomic.Pointer[map[string]resolvedPipelineMode]
	writeMu sync.Mutex
}

// Load populates the cache from repo.ListEnabled. Shares its implementation
// (and writeMu serialization) with Invalidate — see refresh.
func (c *pipelineModeCache) Load(ctx context.Context, repo PipelineModeRepository) error {
	return c.refresh(ctx, repo)
}

// Invalidate re-fetches enabled modes from repo and swaps the cache wholesale.
// Exposed as a distinct method name from Load for call-site clarity at RPC
// write handlers (Epic 2.2) — the implementation is identical to Load beyond
// both sharing the writeMu-guarded refresh sequence.
func (c *pipelineModeCache) Invalidate(ctx context.Context, repo PipelineModeRepository) error {
	return c.refresh(ctx, repo)
}

// refresh is the shared, writer-serialized read-then-store sequence used by
// both Load and Invalidate. writeMu is held for the FULL sequence, including
// the DB read — this is what serializes concurrent Invalidate calls against
// each other (and against the initial Load) so the cache always ends up
// reflecting whichever call's DB read *began last*, never an earlier call's
// data lingering because its Store happened to land after a later call's.
func (c *pipelineModeCache) refresh(ctx context.Context, repo PipelineModeRepository) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	modes, err := repo.ListEnabled(ctx)
	if err != nil {
		return fmt.Errorf("pipelineModeCache: ListEnabled: %w", err)
	}

	next := make(map[string]resolvedPipelineMode, len(modes))
	for _, m := range modes {
		next[m.Slug] = resolvedPipelineMode{
			Slug:                  m.Slug,
			Name:                  m.Name,
			StatusCommandTemplate: m.StatusCommandTemplate,
			DoneCommandTemplate:   m.DoneCommandTemplate,
			FailCommandTemplate:   m.FailCommandTemplate,
			ReviewCommandTemplate: m.ReviewCommandTemplate,
			ShipCommandTemplate:   m.ShipCommandTemplate,
			HelpCommandTemplate:   m.HelpCommandTemplate,
			TriagePromptTemplate:  m.TriagePromptTemplate,
			ReviewPromptTemplate:  m.ReviewPromptTemplate,
			InitialPromptTemplate: m.InitialPromptTemplate,
			ContentHash: ComputeContentHash(
				m.StatusCommandTemplate,
				m.DoneCommandTemplate,
				m.FailCommandTemplate,
				m.ReviewCommandTemplate,
				m.ShipCommandTemplate,
				m.HelpCommandTemplate,
				m.TriagePromptTemplate,
				m.ReviewPromptTemplate,
				m.InitialPromptTemplate,
			),
		}
	}

	c.ptr.Store(&next)
	log.DebugLog().Printf("[PipelineEngine] cache refreshed: %d enabled modes", len(modes))
	return nil
}

// Get returns the resolved mode for slug, or (resolvedPipelineMode{}, false)
// if slug is not present in the current cache snapshot — including the case
// where the cache has never been loaded at all. Lock-free: a single atomic
// Load + map lookup. The caller (CachingPipelineEngine) is responsible for
// any Warn-log-and-fallback behavior on a miss; Get itself never logs.
func (c *pipelineModeCache) Get(slug string) (resolvedPipelineMode, bool) {
	m := c.ptr.Load()
	if m == nil {
		return resolvedPipelineMode{}, false
	}
	rm, ok := (*m)[slug]
	return rm, ok
}

// ComputeContentHash returns a SHA-256 hex digest, truncated to 16
// characters, over fields concatenated in the order given by the caller. Used
// to detect when a PipelineMode's persisted content has changed since a
// session snapshotted it (see plan.md's ItemSessionSummary.PipelineModeSnapshotHash
// entry, Epic 1.6). Callers must always pass the 9 content-template fields in
// the same fixed declaration order so hashes are comparable across loads.
//
// Exported (Epic 2.2) so server/services can compute content_hash for
// CreatePipelineMode/UpdatePipelineMode/GetPipelineMode/ListPipelineModes RPC
// responses directly from a row's current field values — including rows for
// disabled modes, which never enter pipelineModeCache (only ListEnabled-backed
// modes do) and therefore have no resolvedPipelineMode.ContentHash to reuse.
func ComputeContentHash(fields ...string) string {
	h := sha256.New()
	for _, f := range fields {
		h.Write([]byte(f))
	}
	sum := hex.EncodeToString(h.Sum(nil))
	return sum[:16]
}

// recognizedPlaceholders is the allow-list of {{...}} placeholder names
// renderTemplate substitutes. Story 2.3.1 (Phase 2) introduces
// ValidatePipelineModeContent, which must agree with this same allow-list at
// write time — see plan.md's Domain Glossary entry for renderTemplate.
var recognizedPlaceholders = []string{
	"item_id",
	"item_title",
	"item_description",
	"criteria_index",
	"criteria_count",
	"criteria_text",
	"repo_path",
}

// renderTemplate performs fixed-placeholder substitution on tmpl using
// strings.NewReplacer — deliberately NOT text/template: no conditionals, no
// loops, not Turing-complete, to resist the "templating engine" rabbit hole
// (see plan.md's Domain Glossary entry for renderTemplate).
//
// Unrecognized {{...}} tokens are left un-substituted. This is a TEMPORARY,
// Phase-1-ONLY passthrough: no CRUD write path exists yet in Phase 1 for an
// operator to persist a template containing an unrecognized token, so this
// branch is unreachable in practice today. Phase 2's Story 2.3.1 supersedes
// it with write-time allow-list rejection — once that ships, every persisted
// template field is guaranteed to contain only recognized placeholder names.
func renderTemplate(tmpl string, placeholders map[string]string) string {
	pairs := make([]string, 0, len(recognizedPlaceholders)*2)
	for _, name := range recognizedPlaceholders {
		val, ok := placeholders[name]
		if !ok {
			continue
		}
		pairs = append(pairs, "{{"+name+"}}", val)
	}
	return strings.NewReplacer(pairs...).Replace(tmpl)
}

// itemPlaceholders returns the item-level (not-per-criterion) placeholder
// values recognized by renderTemplate for item: item_id, item_title,
// item_description, repo_path. Callers add criteria_index/criteria_count/
// criteria_text themselves where applicable.
func itemPlaceholders(item *BacklogItemData) map[string]string {
	return map[string]string{
		"item_id":          item.ID,
		"item_title":       item.Title,
		"item_description": item.Description,
		"repo_path":        item.RepoPath,
	}
}

// CachingPipelineEngine is the single concrete implementation of
// PipelineEngine. PipelineModeDefault resolves for free (no cache/DB touch);
// any other slug resolves via pipelineModeCache; any unresolvable/malformed
// slug falls back to the default behavior and emits exactly one
// [PipelineEngine]-prefixed Warn log line naming the item and the unresolved
// slug (never a silent no-op, never a panic) — see Story 1.3.3's fail-closed
// acceptance criteria.
type CachingPipelineEngine struct {
	repo  PipelineModeRepository
	cache *pipelineModeCache
}

var _ PipelineEngine = (*CachingPipelineEngine)(nil)

// NewPipelineEngine constructs a CachingPipelineEngine backed by repo, doing
// one synchronous cache.Load at construction time.
//
// Unlike NewDefaultWorkflowEngine's zero-arg, infallible, pure in-memory
// construction, this constructor performs a real DB call and can fail (DB
// unavailable, migration race, transient connection error). Per plan.md's
// Risk Control section ("NewPipelineEngine startup-failure behavior"), a
// cache.Load failure here NEVER aborts construction: it is logged at Warn and
// NewPipelineEngine returns a valid, usable engine backed by an empty cache.
// The signature still returns an error for future-proofing (e.g. a future
// validation error genuinely worth failing construction on), but this Phase 1
// implementation never returns a non-nil error for a cache.Load failure
// specifically — PipelineEngine is purely additive/opt-in, so a transient DB
// hiccup at boot must never crash the whole server for a feature most items
// don't use yet.
func NewPipelineEngine(repo PipelineModeRepository) (*CachingPipelineEngine, error) {
	e := &CachingPipelineEngine{repo: repo, cache: &pipelineModeCache{}}
	if err := e.cache.Load(context.Background(), repo); err != nil {
		log.WarningLog().Printf("[PipelineEngine] cache.Load failed at startup, continuing with an empty cache: %v", err)
	}
	return e, nil
}

// InvalidateCache re-fetches enabled pipeline modes from the repository and
// swaps the cache wholesale. Exported for the RPC write handlers (Epic 2.2)
// that must invalidate the cache after every Create/Update/Delete/Enable/
// Disable of a PipelineMode. Not yet called by any production code path in
// this epic — added now because it is cheap and Epic 2.2 needs it.
func (e *CachingPipelineEngine) InvalidateCache(ctx context.Context) error {
	return e.cache.Invalidate(ctx, e.repo)
}

// SlashCommandSet implements PipelineEngine.
func (e *CachingPipelineEngine) SlashCommandSet(item *BacklogItemData) (map[string]string, error) {
	mode := PipelineMode(item.PipelineMode)
	if mode == PipelineModeDefault {
		return buildDefaultSlashCommandSet(item)
	}

	rm, ok := e.cache.Get(string(mode))
	if !ok {
		log.WarningLog().Printf("[PipelineEngine] unresolved pipeline_mode=%q item=%s — falling back to default", mode, item.ID)
		return buildDefaultSlashCommandSet(item)
	}

	criteria, err := ParseAcCriteria(item.AcceptanceCriteria)
	if err != nil {
		return nil, fmt.Errorf("CachingPipelineEngine.SlashCommandSet: failed to parse AC criteria: %w", err)
	}

	base := itemPlaceholders(item)
	base["criteria_count"] = strconv.Itoa(len(criteria))

	files := make(map[string]string, len(criteria)*2+4)
	files["status.md"] = renderTemplate(rm.StatusCommandTemplate, base)
	files["review.md"] = renderTemplate(rm.ReviewCommandTemplate, base)
	files["ship.md"] = renderTemplate(rm.ShipCommandTemplate, base)
	files["help.md"] = renderTemplate(rm.HelpCommandTemplate, base)

	for _, c := range criteria {
		ph := make(map[string]string, len(base)+2)
		for k, v := range base {
			ph[k] = v
		}
		ph["criteria_index"] = strconv.Itoa(c.Index)
		ph["criteria_text"] = c.Text

		files[fmt.Sprintf("done-%d.md", c.Index)] = renderTemplate(rm.DoneCommandTemplate, ph)
		files[fmt.Sprintf("fail-%d.md", c.Index)] = renderTemplate(rm.FailCommandTemplate, ph)
	}

	return files, nil
}

// TriagePromptFor implements PipelineEngine.
func (e *CachingPipelineEngine) TriagePromptFor(item *BacklogItemData, artifactAbsPath string) string {
	mode := PipelineMode(item.PipelineMode)
	if mode == PipelineModeDefault {
		return BuildHeadlessTriagePrompt(item, artifactAbsPath)
	}

	rm, ok := e.cache.Get(string(mode))
	if !ok {
		log.WarningLog().Printf("[PipelineEngine] unresolved pipeline_mode=%q item=%s — falling back to default", mode, item.ID)
		return BuildHeadlessTriagePrompt(item, artifactAbsPath)
	}

	return renderTemplate(rm.TriagePromptTemplate, itemPlaceholders(item))
}

// ReviewPromptFor implements PipelineEngine.
//
// Deviation from plan.md's Story 1.3.1 interface text: an extras
// ReviewContextExtras parameter was added. BuildHeadlessReviewPrompt requires
// it (PriorSessions/ProgressNotes/ItemDescription), and the real call sites
// this engine replaces (session/review_gate.go, backlog_service_triage.go's
// TriggerReReview) always populate it — dropping it silently would have been
// a real behavior regression on the default path, not just an interface
// nicety. Epic 1.5's call sites should pass their real extras value here.
func (e *CachingPipelineEngine) ReviewPromptFor(item *BacklogItemData, acSnapshot []AcCriterion, diff string, diffTruncated bool, verificationNotes string, extras ReviewContextExtras) string {
	mode := PipelineMode(item.PipelineMode)
	if mode == PipelineModeDefault {
		return BuildHeadlessReviewPrompt(item, acSnapshot, diff, diffTruncated, verificationNotes, extras)
	}

	rm, ok := e.cache.Get(string(mode))
	if !ok {
		log.WarningLog().Printf("[PipelineEngine] unresolved pipeline_mode=%q item=%s — falling back to default", mode, item.ID)
		return BuildHeadlessReviewPrompt(item, acSnapshot, diff, diffTruncated, verificationNotes, extras)
	}

	ph := itemPlaceholders(item)
	ph["criteria_count"] = strconv.Itoa(len(acSnapshot))
	return renderTemplate(rm.ReviewPromptTemplate, ph)
}

// InteractiveReviewPromptFor implements PipelineEngine.
//
// Shares ReviewPromptTemplate with ReviewPromptFor rather than introducing a
// second template field: the PipelineMode schema's review_prompt_template
// comment ("Prompt template used for review under this pipeline mode") is
// already style-agnostic, and splitting headless-JSON vs. tool-call variants
// into two DB fields/UI inputs for one logical "review prompt" concept would
// be speculative until a real mode author needs genuinely different content
// for the two paths. Custom-mode authors who want their template to drive
// the real review gate (this method) must include submit_review_verdict
// call instructions themselves — see the interface doc comment.
func (e *CachingPipelineEngine) InteractiveReviewPromptFor(item *BacklogItemData, acSnapshot []AcCriterion, diff string, diffTruncated bool, itemSessionID string, verificationNotes string) string {
	mode := PipelineMode(item.PipelineMode)
	if mode == PipelineModeDefault {
		return BuildReviewPrompt(item, acSnapshot, diff, diffTruncated, itemSessionID, verificationNotes)
	}

	rm, ok := e.cache.Get(string(mode))
	if !ok {
		log.WarningLog().Printf("[PipelineEngine] unresolved pipeline_mode=%q item=%s — falling back to default", mode, item.ID)
		return BuildReviewPrompt(item, acSnapshot, diff, diffTruncated, itemSessionID, verificationNotes)
	}

	ph := itemPlaceholders(item)
	ph["criteria_count"] = strconv.Itoa(len(acSnapshot))
	return renderTemplate(rm.ReviewPromptTemplate, ph)
}

// InitialPromptFor implements PipelineEngine.
func (e *CachingPipelineEngine) InitialPromptFor(item *BacklogItemData, priorSessions []ItemSessionSummary) string {
	mode := PipelineMode(item.PipelineMode)
	if mode == PipelineModeDefault {
		return BuildTokenBudgetedPrompt(item, priorSessions)
	}

	rm, ok := e.cache.Get(string(mode))
	if !ok {
		log.WarningLog().Printf("[PipelineEngine] unresolved pipeline_mode=%q item=%s — falling back to default", mode, item.ID)
		return BuildTokenBudgetedPrompt(item, priorSessions)
	}

	return renderTemplate(rm.InitialPromptTemplate, itemPlaceholders(item))
}

// ContentHashFor implements PipelineEngine.
//
// No Warn log is emitted for an unresolved slug here — documented exemption:
// this method is only ever called from the Epic 1.6 snapshot-write path
// immediately after a successful resolution already logged its own outcome
// via one of the 4 methods above; a caller invoking ContentHashFor for an
// already-unresolved slug independently of that flow would be a pre-existing
// bug elsewhere, not a new failure mode worth logging again here.
func (e *CachingPipelineEngine) ContentHashFor(mode PipelineMode) (string, bool) {
	if mode == PipelineModeDefault {
		return "", false
	}
	rm, ok := e.cache.Get(string(mode))
	if !ok {
		return "", false
	}
	return rm.ContentHash, true
}
