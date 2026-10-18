package session

// workflow_stage_seed.go — boot-time, idempotent seed of the built-in 9-stage
// workflow graph (BacklogStage + StageTransition rows). Mirrors
// pipeline_mode_seed.go's create-if-missing discipline: an operator's later
// hand-edit via Epic 2.3's CRUD surface must survive every subsequent server
// restart, so this seed writes the built-in rows exactly once and never
// touches them again. See Epic 2.2,
// project_plans/backlog-custom-workflow-stages/implementation/plan.md.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
)

// builtInStageOrder is the fixed order the 9 built-in BacklogStatus values
// are seeded in. Order has no semantic meaning to ConfiguredWorkflowEngine
// (Epic 2.3 looks stages up by slug) but keeps seeded rows deterministic.
var builtInStageOrder = []BacklogStatus{
	BacklogStatusIdea,
	BacklogStatusRefining,
	BacklogStatusReady,
	BacklogStatusQueued,
	BacklogStatusInProgress,
	BacklogStatusReview,
	BacklogStatusPRPending,
	BacklogStatusDone,
	BacklogStatusArchived,
}

// builtInStageNames gives each built-in stage a human-readable display name.
var builtInStageNames = map[BacklogStatus]string{
	BacklogStatusIdea:       "Idea",
	BacklogStatusRefining:   "Refining",
	BacklogStatusReady:      "Ready",
	BacklogStatusQueued:     "Queued",
	BacklogStatusInProgress: "In Progress",
	BacklogStatusReview:     "Review",
	BacklogStatusPRPending:  "PR Pending",
	BacklogStatusDone:       "Done",
	BacklogStatusArchived:   "Archived",
}

// builtInEntryStages marks the built-in stages a new item may be created
// directly into. Only "idea" qualifies today.
var builtInEntryStages = map[BacklogStatus]bool{
	BacklogStatusIdea: true,
}

// builtInTerminalStages marks the built-in stages that end an item's
// lifecycle, mirroring Epic 2.1's IsTerminalStatus-equivalent check.
var builtInTerminalStages = map[BacklogStatus]bool{
	BacklogStatusDone:     true,
	BacklogStatusArchived: true,
}

// EnsureBuiltInWorkflowStages seeds the 9 built-in BacklogStatus values as
// BacklogStage rows, plus one StageTransition row per domain.ValidTransitions()
// edge, exactly once. It is a pure no-op — it never calls Update — once any
// backlog_stages row exists, so a later operator hand-edit is never reverted
// by a restart.
//
// Mirrors EnsureDefaultSDDPipelineMode's non-fatal-boot posture: errors are
// returned for the caller to log-and-continue rather than aborting server
// startup, since ConfiguredWorkflowEngine isn't wired into production until
// Epic 2.3 lands and nothing yet depends on these rows existing.
func EnsureBuiltInWorkflowStages(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return nil
	}

	count, err := client.BacklogStage.Query().Count(ctx)
	if err != nil {
		return fmt.Errorf("EnsureBuiltInWorkflowStages: count existing stages: %w", err)
	}
	if count > 0 {
		return nil
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("EnsureBuiltInWorkflowStages: begin tx: %w", err)
	}

	stageIDs := make(map[BacklogStatus]uuid.UUID, len(builtInStageOrder))
	for _, slug := range builtInStageOrder {
		row, createErr := tx.BacklogStage.Create().
			SetSlug(string(slug)).
			SetName(builtInStageNames[slug]).
			SetIsEntry(builtInEntryStages[slug]).
			SetIsTerminal(builtInTerminalStages[slug]).
			SetEnabled(true).
			Save(ctx)
		if createErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("EnsureBuiltInWorkflowStages: create stage %q: %w", slug, createErr)
		}
		stageIDs[slug] = row.ID
	}

	transitionsCreated := 0
	for from, targets := range domain.ValidTransitions() {
		fromID, ok := stageIDs[from]
		if !ok {
			continue // defensive: only seed edges between the 9 built-in stages just created
		}
		for to := range targets {
			toID, ok := stageIDs[to]
			if !ok {
				continue
			}
			if _, createErr := tx.StageTransition.Create().
				SetFromStageID(fromID).
				SetToStageID(toID).
				SetEnabled(true).
				Save(ctx); createErr != nil {
				_ = tx.Rollback()
				return fmt.Errorf("EnsureBuiltInWorkflowStages: create transition %s->%s: %w", from, to, createErr)
			}
			transitionsCreated++
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("EnsureBuiltInWorkflowStages: commit: %w", err)
	}

	log.InfoLog().Printf("[ConfiguredWorkflowEngine] seeded %d built-in stage(s) and %d transition(s)", len(stageIDs), transitionsCreated)
	return nil
}

// stageConfigFingerprint is the canonical, order-insensitive description of a
// stage configuration (stages, edges, liveness rows, gates). SeedFingerprint
// hashes the built-in seed's; StageConfigFingerprint hashes a live database's.
// An operator who adds a gate or changes a liveness value without touching an
// edge therefore still reads as "edited" rather than "seeded".
type stageConfigFingerprint struct {
	Stages   []string
	Edges    []string
	Liveness []string
	Gates    []string
}

func (c stageConfigFingerprint) hash() string {
	h := sha256.New()
	for _, section := range []struct {
		name  string
		lines []string
	}{{"stages", c.Stages}, {"edges", c.Edges}, {"liveness", c.Liveness}, {"gates", c.Gates}} {
		lines := append([]string(nil), section.lines...)
		sort.Strings(lines)
		_, _ = fmt.Fprintf(h, "[%s]\n", section.name)
		for _, l := range lines {
			_, _ = fmt.Fprintln(h, l)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

type stageRow struct {
	Slug, Name               string
	Entry, Terminal, Enabled bool
}

func (r stageRow) line() string {
	return fmt.Sprintf("%s|%s|entry=%t|terminal=%t|enabled=%t", r.Slug, r.Name, r.Entry, r.Terminal, r.Enabled)
}

const nilText = "nil"

func ptrText[T any](v *T) string {
	if v == nil {
		return nilText
	}
	return fmt.Sprint(*v)
}

func livenessLine(l *ent.StageLivenessDefinition) string {
	return fmt.Sprintf("%s|mode=%s|kind=%s|expected=%s|margin=%s|noprogress=%s|threshold=%s|lookback=%s|enabled=%t",
		l.StageSlug, ptrText(l.PipelineMode), l.Kind, ptrText(l.ExpectedDurationMs), ptrText(l.StalenessMarginMs),
		ptrText(l.MaxNoProgressDurationMs), ptrText(l.CycleThreshold), ptrText(l.CycleLookbackMs), l.Enabled)
}

// builtInSeedFingerprintInput builds the fingerprint input from the same
// sources EnsureBuiltInWorkflowStages writes. The built-in seed writes no
// liveness rows and no gates, so those sections are empty.
func builtInSeedFingerprintInput() stageConfigFingerprint {
	var c stageConfigFingerprint
	for _, slug := range builtInStageOrder {
		c.Stages = append(c.Stages, stageRow{string(slug), builtInStageNames[slug], builtInEntryStages[slug], builtInTerminalStages[slug], true}.line())
	}
	for from, targets := range domain.ValidTransitions() {
		for to := range targets {
			c.Edges = append(c.Edges, fmt.Sprintf("%s->%s|enabled=true", from, to))
		}
	}
	return c
}

// SeedFingerprint returns a stable hash of the built-in seed: stage set,
// edges, liveness rows and gates. It labels `config=seeded|edited` in the
// workflow-engine metrics.
func SeedFingerprint() string {
	return builtInSeedFingerprintInput().hash()
}

// StageConfigFingerprint hashes the live stage configuration in client with
// the same canonical form as SeedFingerprint, so equality means "still seeded".
func StageConfigFingerprint(ctx context.Context, client *ent.Client) (string, error) {
	c, err := liveStageConfigInput(ctx, client)
	if err != nil {
		return "", err
	}
	return c.hash(), nil
}

func liveStageConfigInput(ctx context.Context, client *ent.Client) (stageConfigFingerprint, error) {
	//nolint:entfullscan the fingerprint must cover every row of the stage configuration tables.
	stages, err := client.BacklogStage.Query().All(ctx)
	if err != nil {
		return stageConfigFingerprint{}, fmt.Errorf("StageConfigFingerprint: query stages: %w", err)
	}
	slugByID := make(map[uuid.UUID]string, len(stages))
	var c stageConfigFingerprint
	for _, s := range stages {
		slugByID[s.ID] = s.Slug
		c.Stages = append(c.Stages, stageRow{s.Slug, s.Name, s.IsEntry, s.IsTerminal, s.Enabled}.line())
	}

	//nolint:entfullscan the fingerprint must cover every edge.
	transitions, err := client.StageTransition.Query().All(ctx)
	if err != nil {
		return stageConfigFingerprint{}, fmt.Errorf("StageConfigFingerprint: query transitions: %w", err)
	}
	edgeByID := make(map[uuid.UUID]string, len(transitions))
	for _, t := range transitions {
		edge := fmt.Sprintf("%s->%s", slugByID[t.FromStageID], slugByID[t.ToStageID])
		edgeByID[t.ID] = edge
		c.Edges = append(c.Edges, fmt.Sprintf("%s|enabled=%t", edge, t.Enabled))
	}

	//nolint:entfullscan the fingerprint must cover every gate.
	gates, err := client.TransitionGate.Query().All(ctx)
	if err != nil {
		return stageConfigFingerprint{}, fmt.Errorf("StageConfigFingerprint: query gates: %w", err)
	}
	for _, g := range gates {
		cfg, _ := json.Marshal(g.Config) // map keys marshal sorted: stable
		c.Gates = append(c.Gates, fmt.Sprintf("%s|kind=%s|stateful=%t|order=%d|enabled=%t|config=%s", edgeByID[g.TransitionID], g.Kind, g.Stateful, g.OrderIndex, g.Enabled, cfg))
	}

	//nolint:entfullscan the fingerprint must cover every liveness row.
	liveness, err := client.StageLivenessDefinition.Query().All(ctx)
	if err != nil {
		return stageConfigFingerprint{}, fmt.Errorf("StageConfigFingerprint: query liveness: %w", err)
	}
	for _, l := range liveness {
		c.Liveness = append(c.Liveness, livenessLine(l))
	}
	return c, nil
}

// SectionDiff lists the lines present only in the live configuration (Added)
// and only in the seed (Removed).
type SectionDiff struct {
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

// ConfigDiff is the live stage configuration versus the built-in seed.
type ConfigDiff struct {
	Label    string      `json:"config"` // seeded | edited
	Stages   SectionDiff `json:"stages"`
	Edges    SectionDiff `json:"edges"`
	Gates    SectionDiff `json:"gates"`
	Liveness SectionDiff `json:"liveness"`
}

// StageConfigDiff compares the live configuration in client with the seed.
func StageConfigDiff(ctx context.Context, client *ent.Client) (ConfigDiff, error) {
	live, err := liveStageConfigInput(ctx, client)
	if err != nil {
		return ConfigDiff{}, err
	}
	return diffStageConfig(builtInSeedFingerprintInput(), live), nil
}

func diffStageConfig(seed, live stageConfigFingerprint) ConfigDiff {
	d := ConfigDiff{
		Stages:   diffLines(seed.Stages, live.Stages),
		Edges:    diffLines(seed.Edges, live.Edges),
		Gates:    diffLines(redactGateConfigs(seed.Gates), redactGateConfigs(live.Gates)),
		Liveness: diffLines(seed.Liveness, live.Liveness),
	}
	d.Label = ConfigSeeded
	if seed.hash() != live.hash() {
		d.Label = ConfigEdited
	}
	return d
}

// redactGateConfigs replaces each gate's free-form config with a short hash so
// the diff names what changed without echoing prompts or commands.
func redactGateConfigs(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if at := strings.Index(l, "|config="); at >= 0 {
			sum := sha256.Sum256([]byte(l[at+len("|config="):]))
			l = l[:at] + "|config_hash=" + hex.EncodeToString(sum[:4])
		}
		out[i] = l
	}
	return out
}

func diffLines(seed, live []string) SectionDiff {
	inSeed, inLive := make(map[string]bool, len(seed)), make(map[string]bool, len(live))
	for _, l := range seed {
		inSeed[l] = true
	}
	for _, l := range live {
		inLive[l] = true
	}
	var d SectionDiff
	for _, l := range live {
		if !inSeed[l] {
			d.Added = append(d.Added, l)
		}
	}
	for _, l := range seed {
		if !inLive[l] {
			d.Removed = append(d.Removed, l)
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	return d
}

// ItemStatusRef is the minimum the stranded-item preview needs about an item.
type ItemStatusRef struct {
	ID     string
	Status BacklogStatus
}

// StrandedItem is a live item that loses outgoing edges once enforce is on.
// The id is hashed; no title or id leaves the process.
type StrandedItem struct {
	ItemHash        string        `json:"item_hash"`
	Status          BacklogStatus `json:"status"`
	LostEdges       []string      `json:"lost_edges"`
	NoOutgoingEdges bool          `json:"no_outgoing_edges"`
	InDisabledStage bool          `json:"in_disabled_stage"`
}

// StrandedItemPreview lists, read-only, every item whose enforce-mode allowed
// transitions are empty or smaller than Default's. A non-empty OverrideReason
// skips operator gates only, never the graph, so it cannot free a lost edge.
func StrandedItemPreview(def WorkflowEngine, cfg *ConfiguredWorkflowEngine, items []ItemStatusRef, disabledStages map[BacklogStatus]bool) []StrandedItem {
	enf := enforceAnswer{def: def, cfg: cfg}
	var out []StrandedItem
	for _, it := range items {
		before := def.AllowedTransitions(it.Status, nil)
		after, _ := enf.allowedTransitions(it.Status, nil)
		kept := make(map[BacklogStatus]bool, len(after))
		for _, to := range after {
			kept[to] = true
		}
		var lost []string
		for _, to := range before {
			if !kept[to] {
				lost = append(lost, EdgeKey(it.Status, to))
			}
		}
		if len(lost) == 0 && (len(after) > 0 || len(before) == 0) {
			continue
		}
		sort.Strings(lost)
		out = append(out, StrandedItem{
			ItemHash: HashItemID(it.ID), Status: it.Status, LostEdges: lost,
			NoOutgoingEdges: len(after) == 0, InDisabledStage: disabledStages[it.Status],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ItemHash < out[j].ItemHash })
	return out
}

// ListItemStatusRefs reads the non-archived items' ids and statuses, plus the
// slugs of disabled stages, for StrandedItemPreview. Read-only.
func ListItemStatusRefs(ctx context.Context, client *ent.Client) ([]ItemStatusRef, map[BacklogStatus]bool, error) {
	//nolint:entfullscan the preview must cover every live item.
	rows, err := client.BacklogItem.Query().Where(backlogitem.StatusNEQ(string(BacklogStatusArchived))).
		Select(backlogitem.FieldID, backlogitem.FieldStatus).All(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("ListItemStatusRefs: query items: %w", err)
	}
	items := make([]ItemStatusRef, 0, len(rows))
	for _, r := range rows {
		items = append(items, ItemStatusRef{ID: r.ID.String(), Status: BacklogStatus(r.Status)})
	}
	//nolint:entfullscan the preview must cover every stage.
	stages, err := client.BacklogStage.Query().Where(backlogstage.Enabled(false)).All(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("ListItemStatusRefs: query disabled stages: %w", err)
	}
	disabled := make(map[BacklogStatus]bool, len(stages))
	for _, s := range stages {
		disabled[BacklogStatus(s.Slug)] = true
	}
	return items, disabled, nil
}
