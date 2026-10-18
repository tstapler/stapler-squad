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

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
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
	//nolint:entfullscan the fingerprint must cover every row of the stage configuration tables.
	stages, err := client.BacklogStage.Query().All(ctx)
	if err != nil {
		return "", fmt.Errorf("StageConfigFingerprint: query stages: %w", err)
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
		return "", fmt.Errorf("StageConfigFingerprint: query transitions: %w", err)
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
		return "", fmt.Errorf("StageConfigFingerprint: query gates: %w", err)
	}
	for _, g := range gates {
		cfg, _ := json.Marshal(g.Config) // map keys marshal sorted: stable
		c.Gates = append(c.Gates, fmt.Sprintf("%s|kind=%s|stateful=%t|order=%d|enabled=%t|config=%s", edgeByID[g.TransitionID], g.Kind, g.Stateful, g.OrderIndex, g.Enabled, cfg))
	}

	//nolint:entfullscan the fingerprint must cover every liveness row.
	liveness, err := client.StageLivenessDefinition.Query().All(ctx)
	if err != nil {
		return "", fmt.Errorf("StageConfigFingerprint: query liveness: %w", err)
	}
	for _, l := range liveness {
		c.Liveness = append(c.Liveness, livenessLine(l))
	}
	return c.hash(), nil
}
