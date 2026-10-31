package session

// workflow_engine_replay.go replays the caller registry against a COPY of the
// database (Task 1.5.9d): for every registered caller and declared edge it
// prints the enforced decision under the stage configuration stored in the copy
// and again with a synthetic human_approval gate on that edge. It is the
// evidence for low-volume edges where shadow sample counts cannot be reached.
//
// The copy is opened read-only and immutable, and a path inside a live state
// directory is refused, so a replay can never write the live database. The
// synthetic gate lives in an in-memory overlay of the stage repository.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/internal/sqlitedsn"
	"github.com/tstapler/stapler-squad/session/ent"
)

// ErrReplayLiveDatabase is returned for a path inside a live state directory.
var ErrReplayLiveDatabase = errors.New("refusing to replay a database inside a live state directory: copy it elsewhere first (sqlite3 <live> \".backup <copy>\")")

// LiveStateDirs are the directories a replay input may never live in.
func LiveStateDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home, ".stapler-squad"), filepath.Join(home, ".claude-squad")}
}

// ValidateReplayDBPath resolves path (symlinks included) and refuses a missing
// file, a directory, a non-empty -wal sidecar (the copy would read stale) and
// anything inside liveDirs.
func ValidateReplayDBPath(path string, liveDirs []string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("no database path given")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", path, err)
	}
	for _, dir := range liveDirs {
		if lexicallyInside(abs, dir) { // refuse before any filesystem access under the live directory
			return "", fmt.Errorf("%w (%s)", ErrReplayLiveDatabase, abs)
		}
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat %q: %w", resolved, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory, not a database file", resolved)
	}
	for _, dir := range liveDirs {
		if insideDir(resolved, dir) {
			return "", fmt.Errorf("%w (%s)", ErrReplayLiveDatabase, resolved)
		}
		if liveFile, ok := sameFileUnder(info, dir); ok {
			return "", fmt.Errorf("%w (%s is the same file as %s)", ErrReplayLiveDatabase, resolved, liveFile)
		}
	}
	if wal, err := os.Stat(resolved + "-wal"); err == nil && wal.Size() > 0 {
		return "", fmt.Errorf("%s-wal is not empty: the copy is not self-contained, make it with sqlite3 \".backup\"", resolved)
	}
	return resolved, nil
}

func lexicallyInside(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func insideDir(path, dir string) bool {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// sameFileUnder reports a regular file directly or one level inside dir that is
// the same inode as info (a hard link to the live database).
func sameFileUnder(info os.FileInfo, dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		candidate := filepath.Join(dir, e.Name())
		if cInfo, err := os.Stat(candidate); err == nil && os.SameFile(info, cInfo) {
			return candidate, true
		}
	}
	return "", false
}

// OpenReplayCopy validates path and opens it read-only and immutable. The
// returned function closes the client.
func OpenReplayCopy(path string, liveDirs []string) (*ent.Client, func(), error) {
	resolved, err := ValidateReplayDBPath(path, liveDirs)
	if err != nil {
		return nil, nil, err
	}
	dsn := sqlitedsn.New("file:" + resolved + "?mode=ro&immutable=1").WithQueryOnly().WithEntTimeCompat().Build()
	db, err := sql.Open("sqlite", dsn) //nolint:norawdb read-only immutable replay file
	if err != nil {
		return nil, nil, fmt.Errorf("open %s read-only: %w", resolved, err)
	}
	db.SetMaxOpenConns(1)
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	return client, func() { _ = client.Close() }, nil
}

// ReplayDecision is one registered caller's enforced answer on one edge.
type ReplayDecision struct {
	Caller    TransitionCaller
	Class     CallerClass
	Edge      SystemEdge
	Live      string // "allowed" or "denied:<reason>"
	Synthetic string // answer with a human_approval gate on the edge; "-" when not applicable
}

// ReplayReport is the replay result.
type ReplayReport struct {
	Config       string // seeded | edited
	Decisions    []ReplayDecision
	Stranded     map[BacklogStatus]int
	StrandedAll  int
	LiveItems    int
	Failures     []string
	SyntheticRan int
}

// Failed reports whether a fixed agent or system edge is denied by the stored
// configuration with no gate involved.
func (r *ReplayReport) Failed() bool { return len(r.Failures) > 0 }

type replayEnforceAll struct{}

func (replayEnforceAll) Mode() EngineMode               { return ModeEnforce }
func (replayEnforceAll) ModeFor(CallerClass) EngineMode { return ModeEnforce }

// replayGateOverlay adds one unsatisfied human_approval gate to the from->to
// transition on every read; nothing is written.
type replayGateOverlay struct {
	StageConfigRepository
	edge SystemEdge
}

func (o replayGateOverlay) ListEnabledTransitions(ctx context.Context) ([]*ent.StageTransition, error) {
	transitions, err := o.StageConfigRepository.ListEnabledTransitions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*ent.StageTransition, 0, len(transitions))
	for _, t := range transitions {
		if t.Edges.FromStage == nil || t.Edges.ToStage == nil ||
			t.Edges.FromStage.Slug != string(o.edge.From) || t.Edges.ToStage.Slug != string(o.edge.To) {
			out = append(out, t)
			continue
		}
		clone := *t
		clone.Edges.Gates = append(append([]*ent.TransitionGate(nil), t.Edges.Gates...), &ent.TransitionGate{
			ID: uuid.New(), Kind: string(GateKindHumanApproval), Config: map[string]interface{}{},
			Stateful: true, OrderIndex: 1 << 20, Enabled: true,
		})
		out = append(out, &clone)
	}
	return out, nil
}

func newReplaySelector(repo StageConfigRepository) (*SwitchableWorkflowEngine, error) {
	cfg, err := NewConfiguredWorkflowEngine(repo, nil, nil)
	if err != nil {
		return nil, err
	}
	if loadErr := cfg.loadErr.Load(); loadErr != nil {
		return nil, fmt.Errorf("load stage configuration: %w", *loadErr)
	}
	return NewSwitchableWorkflowEngine(NewDefaultWorkflowEngine(), cfg, replayEnforceAll{}), nil
}

// replayEdges expands a caller's declared edges: the archive wildcard becomes
// one edge per built-in stage.
func replayEdges(spec TransitionCallerSpec) []SystemEdge {
	var out []SystemEdge
	for _, e := range spec.Edges {
		if e.From != AnyStatus {
			out = append(out, e)
			continue
		}
		for _, from := range builtInStageOrder {
			if from != e.To {
				out = append(out, SystemEdge{From: from, To: e.To})
			}
		}
	}
	return out
}

// replayAnswer is the enforced decision for caller on edge, Default guards off
// (the recorded reasons are the graph and Configured gates).
func replayAnswer(sel *SwitchableWorkflowEngine, spec TransitionCallerSpec, edge SystemEdge) string {
	item := &BacklogItemData{ID: "replay-item", Status: string(edge.From)}
	v := decideTransition(sel.ForClass(spec.Class, GuardsNone), item, TransitionRequest{Caller: spec.Name, To: edge.To})
	if v.Err == nil {
		return "allowed"
	}
	return replayDeniedPrefix + v.Reason
}

const replayDeniedPrefix = "denied:"

// replayRun holds the selectors one replay builds: the stored configuration and
// one overlay per system-required edge that gets a synthetic gate.
type replayRun struct {
	repo           StageConfigRepository
	live           *SwitchableWorkflowEngine
	systemRequired map[SystemEdge]bool
	synthetic      map[SystemEdge]*SwitchableWorkflowEngine
	stored         map[SystemEdge]bool
	report         *ReplayReport
}

// replayNoStoredTransition marks a system-required edge with no stage
// transition row: it is allowed by the system-required union, and no operator
// gate can attach to it, so configuration cannot change its answer.
const replayNoStoredTransition = "n/a:no_stored_transition"

func replayStoredEdges(ctx context.Context, repo StageConfigRepository) (map[SystemEdge]bool, error) {
	transitions, err := repo.ListEnabledTransitions(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[SystemEdge]bool, len(transitions))
	for _, t := range transitions {
		if t.Edges.FromStage != nil && t.Edges.ToStage != nil {
			out[SystemEdge{From: BacklogStatus(t.Edges.FromStage.Slug), To: BacklogStatus(t.Edges.ToStage.Slug)}] = true
		}
	}
	return out, nil
}

func (r *replayRun) syntheticFor(edge SystemEdge) (*SwitchableWorkflowEngine, error) {
	if sel, ok := r.synthetic[edge]; ok {
		return sel, nil
	}
	sel, err := newReplaySelector(replayGateOverlay{r.repo, edge})
	if err != nil {
		return nil, err
	}
	r.synthetic[edge] = sel
	return sel, nil
}

func (r *replayRun) decide(spec TransitionCallerSpec, edge SystemEdge) (ReplayDecision, error) {
	d := ReplayDecision{Caller: spec.Name, Class: spec.Class, Edge: edge, Live: replayAnswer(r.live, spec, edge), Synthetic: "-"}
	if !r.systemRequired[edge] {
		return d, nil
	}
	if !r.stored[edge] {
		d.Synthetic = replayNoStoredTransition
		return d, nil
	}
	sel, err := r.syntheticFor(edge)
	if err != nil {
		return d, err
	}
	d.Synthetic = replayAnswer(sel, spec, edge)
	r.report.SyntheticRan++
	return d, nil
}

// failure describes an agent or system edge the stored configuration denies
// with no gate involved; operator callers choose their edge, so a denial there
// is the configuration working.
func (d ReplayDecision) failure() (string, bool) {
	if d.Class == ClassOperator || (d.Live != replayDeniedPrefix+DeniedConfigGraph && d.Live != replayDeniedPrefix+DeniedTargetStageRemoved) {
		return "", false
	}
	return fmt.Sprintf("%s (%s) %s->%s is %s by the stored configuration with no gate involved", d.Caller, d.Class, d.Edge.From, d.Edge.To, d.Live), true
}

// BuildWorkflowEngineReplay evaluates the registry against the stage
// configuration and items in client. It only reads.
func BuildWorkflowEngineReplay(ctx context.Context, client *ent.Client) (*ReplayReport, error) {
	repo := NewEntStageConfigRepository(client)
	live, err := newReplaySelector(repo)
	if err != nil {
		return nil, fmt.Errorf("replay: %w", err)
	}
	diff, err := StageConfigDiff(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("compare stage configuration with the seed: %w", err)
	}
	stored, err := replayStoredEdges(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("list stored transitions: %w", err)
	}
	run := &replayRun{
		repo: repo, live: live, stored: stored, systemRequired: map[SystemEdge]bool{},
		synthetic: map[SystemEdge]*SwitchableWorkflowEngine{}, report: &ReplayReport{Config: diff.Label},
	}
	for _, e := range SystemRequiredEdges() {
		run.systemRequired[e] = true
	}
	for _, spec := range TransitionCallers() {
		for _, edge := range replayEdges(spec) {
			d, err := run.decide(spec, edge)
			if err != nil {
				return nil, fmt.Errorf("replay %s %s->%s: %w", spec.Name, edge.From, edge.To, err)
			}
			if msg, failed := d.failure(); failed {
				run.report.Failures = append(run.report.Failures, msg)
			}
			run.report.Decisions = append(run.report.Decisions, d)
		}
	}
	return run.report, replayStranded(ctx, client, repo, run.report)
}

func replayStranded(ctx context.Context, client *ent.Client, repo StageConfigRepository, report *ReplayReport) error {
	cfg, err := NewConfiguredWorkflowEngine(repo, nil, nil)
	if err != nil {
		return err
	}
	items, disabled, err := ListItemStatusRefs(ctx, client)
	if err != nil {
		return err
	}
	report.LiveItems = len(items)
	report.Stranded = map[BacklogStatus]int{}
	for _, s := range StrandedItemPreview(NewDefaultWorkflowEngine(), cfg, items, disabled) {
		report.Stranded[s.Status]++
		report.StrandedAll++
	}
	return nil
}

// Render prints the report: one row per registered caller and edge, then the
// stranded-item preview and the failures.
func (r *ReplayReport) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "workflow engine replay (copy opened read-only; gates applied in memory only)\nconfig: %s\n", r.Config)
	fmt.Fprintf(&b, "decisions: %d caller edges, %d with a synthetic human_approval gate\n", len(r.Decisions), r.SyntheticRan)
	fmt.Fprintf(&b, "%-34s %-9s %-26s %-30s %s\n", "CALLER", "CLASS", "EDGE", "LIVE", "WITH SYNTHETIC GATE")
	for _, d := range r.Decisions {
		fmt.Fprintf(&b, "%-34s %-9s %-26s %-30s %s\n", d.Caller, d.Class, string(d.Edge.From)+"->"+string(d.Edge.To), d.Live, d.Synthetic)
	}
	fmt.Fprintf(&b, "stranded-item preview: %d of %d live item(s)\n", r.StrandedAll, r.LiveItems)
	statuses := make([]string, 0, len(r.Stranded))
	for s := range r.Stranded {
		statuses = append(statuses, string(s))
	}
	sort.Strings(statuses)
	for _, s := range statuses {
		fmt.Fprintf(&b, "  %-12s %d\n", s, r.Stranded[BacklogStatus(s)])
	}
	b.WriteString("detector-guard table: pinned by the session tests TestEveryStuckDetectorIsClassified, TestDeniedGuards_should_AllShareOnePredicate and TestDetectorLocalGuards_should_CallTheSharedHelper; not derived from the database\n")
	if r.Failed() {
		fmt.Fprintf(&b, "FAIL: %d fixed agent or system edge(s) denied with no gate:\n", len(r.Failures))
		for _, f := range r.Failures {
			fmt.Fprintf(&b, "  %s\n", f)
		}
	} else {
		b.WriteString("OK: no fixed agent or system edge is denied without a gate\n")
	}
	return b.String()
}

// RunWorkflowEngineReplay is the entry point of `make workflow-engine-replay`:
// it opens the copy at path and returns the rendered report and whether it
// failed.
func RunWorkflowEngineReplay(ctx context.Context, path string, liveDirs []string) (string, bool, error) {
	client, closeFn, err := OpenReplayCopy(path, liveDirs)
	if err != nil {
		return "", false, err
	}
	defer closeFn()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	report, err := BuildWorkflowEngineReplay(ctx, client)
	if err != nil {
		return "", false, err
	}
	return report.Render(), report.Failed(), nil
}
