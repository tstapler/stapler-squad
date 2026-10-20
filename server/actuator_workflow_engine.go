package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// WorkflowEngineSoakSource is everything /actuator/workflow-engine reads. Wire
// it with Server.SetWorkflowEngineSoakSource; nil fields degrade to "not
// available" rather than a guess.
type WorkflowEngineSoakSource struct {
	// Ledger returns the merged shadow ledger (live counts plus the persisted window).
	Ledger func() (session.ShadowLedger, error)
	// ConfigDiff returns the live-versus-seed diff and the stranded-item preview.
	ConfigDiff func(ctx context.Context) (session.ConfigDiff, []session.StrandedItem, error)
	// Acks returns the owner acknowledgements recorded in the runbook.
	Acks func() session.SoakAcks
	// AnyClassEnforced reports whether any class's enforce flag is on.
	AnyClassEnforced func() bool
	// ReproPath is where the redacted repro records live; printed on alarm.
	ReproPath string
	// Now is the clock (a test seam); nil uses real time.
	Now func() time.Time
}

// SetWorkflowEngineSoakSource wires the actuator's soak data.
func (s *Server) SetWorkflowEngineSoakSource(src *WorkflowEngineSoakSource) {
	s.workflowEngineSoak.Store(src)
}

func (s *Server) handleActuatorWorkflowEngine(w http.ResponseWriter, r *http.Request) {
	writeWorkflowEngineSoak(w, r, s.workflowEngineSoak.Load())
}

var soakClasses = []session.CallerClass{session.ClassOperator, session.ClassAgent, session.ClassSystem}

// soakSeries is one labeled counter in the JSON body.
type soakSeries struct {
	Class      string `json:"class"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Divergence string `json:"divergence,omitempty"`
	Config     string `json:"config,omitempty"`
	Value      int64  `json:"value"`
}

type workflowEngineReport struct {
	GeneratedAt       time.Time              `json:"generated_at"`
	BinaryStartedAt   []time.Time            `json:"binary_started_at"`
	ExitCode          int                    `json:"exit_code"`
	Verdicts          []session.Verdict      `json:"verdicts"`
	ShadowCalls       []soakSeries           `json:"shadow_calls_total"`
	DivergenceTotal   []soakSeries           `json:"divergence_total"`
	NotComparedTotal  map[string]int64       `json:"not_compared_total"`
	ConfigDiff        *session.ConfigDiff    `json:"config_diff,omitempty"`
	StrandedItems     []session.StrandedItem `json:"stranded_items,omitempty"`
	ConfigDiffMessage string                 `json:"config_diff_error,omitempty"`
}

func writeWorkflowEngineSoak(w http.ResponseWriter, r *http.Request, src *WorkflowEngineSoakSource) {
	if src == nil || src.Ledger == nil {
		w.Header().Set("X-Workflow-Engine-Soak-Exit", "2")
		http.Error(w, "workflow engine soak is not wired in this process", http.StatusServiceUnavailable)
		return
	}
	fileAcks, err := readSoakAckBody(w, r)
	if err != nil {
		w.Header().Set("X-Workflow-Engine-Soak-Exit", "2")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now()
	if src.Now != nil {
		now = src.Now()
	}
	classes := soakClasses
	if c := r.URL.Query().Get("class"); c != "" {
		classes = []session.CallerClass{session.CallerClass(c)}
	}
	ledger, ledgerErr := src.Ledger()
	rep := workflowEngineReport{GeneratedAt: now.UTC(), BinaryStartedAt: ledger.BinaryStarts}
	for _, c := range classes {
		p := session.DefaultSoakParams(c, now)
		p.ReproPath = src.ReproPath
		if src.Acks != nil {
			p.Acks = src.Acks()
		}
		p.Acks = p.Acks.Merge(fileAcks.For(c))
		if src.AnyClassEnforced != nil {
			p.AnyClassEnforced = src.AnyClassEnforced()
		}
		rep.Verdicts = append(rep.Verdicts, session.EvaluateSoakFromSource(c, func() (session.ShadowLedger, error) { return ledger, ledgerErr }, p))
	}
	rep.ExitCode = session.OverallSoakExit(rep.Verdicts)
	rep.ShadowCalls, rep.DivergenceTotal, rep.NotComparedTotal = soakSeriesFrom(ledger)
	if r.URL.Query().Get("config_diff") != "" {
		fillConfigDiff(r.Context(), src, &rep)
		if len(rep.StrandedItems) > 0 || rep.ConfigDiffMessage != "" {
			rep.ExitCode = 1
		}
	}

	w.Header().Set("X-Workflow-Engine-Soak-Exit", strconv.Itoa(rep.ExitCode))
	if r.URL.Query().Get("format") == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(rep.text()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rep)
}

// maxSoakAckBytes bounds a posted --ack-file.
const maxSoakAckBytes = 64 << 10

// readSoakAckBody parses the --ack-file a POST carries; a GET has none.
func readSoakAckBody(w http.ResponseWriter, r *http.Request) (session.SoakAckFile, error) {
	if r.Method != http.MethodPost {
		return session.SoakAckFile{}, nil
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSoakAckBytes))
	if err != nil {
		return session.SoakAckFile{}, fmt.Errorf("ack file unreadable: %w", err)
	}
	return session.ParseSoakAckFile(data)
}

func fillConfigDiff(ctx context.Context, src *WorkflowEngineSoakSource, rep *workflowEngineReport) {
	if src.ConfigDiff == nil {
		rep.ConfigDiffMessage = "config diff is not wired in this process"
		return
	}
	d, stranded, err := src.ConfigDiff(ctx)
	if err != nil {
		rep.ConfigDiffMessage = err.Error()
		return
	}
	rep.ConfigDiff, rep.StrandedItems = &d, stranded
}

func soakSeriesFrom(l session.ShadowLedger) (calls, divergences []soakSeries, notCompared map[string]int64) {
	notCompared = map[string]int64{}
	classNames := make([]string, 0, len(l.Classes))
	for c := range l.Classes {
		classNames = append(classNames, c)
	}
	sort.Strings(classNames)
	for _, class := range classNames {
		cl := l.Classes[class]
		for k, v := range cl.Calls {
			if parts := strings.Split(k, "|"); len(parts) == 2 {
				calls = append(calls, soakSeries{Class: class, Method: parts[0], Path: parts[1], Value: v})
			}
		}
		for k, v := range cl.Divergence {
			if parts := strings.Split(k, "|"); len(parts) == 4 {
				divergences = append(divergences, soakSeries{Class: class, Method: parts[0], Divergence: parts[1], Config: parts[2], Path: parts[3], Value: v})
			}
		}
		for k, v := range cl.NotCompared {
			notCompared[k] += v
		}
	}
	less := func(s []soakSeries) {
		sort.Slice(s, func(i, j int) bool {
			return fmt.Sprint(s[i].Class, s[i].Method, s[i].Path, s[i].Divergence, s[i].Config) <
				fmt.Sprint(s[j].Class, s[j].Method, s[j].Path, s[j].Divergence, s[j].Config)
		})
	}
	less(calls)
	less(divergences)
	return calls, divergences, notCompared
}

func (r workflowEngineReport) text() string {
	var b strings.Builder
	for _, v := range r.Verdicts {
		b.WriteString(v.Text())
	}
	if r.ConfigDiff != nil {
		b.WriteString(configDiffText(*r.ConfigDiff, r.StrandedItems))
	}
	if r.ConfigDiffMessage != "" {
		fmt.Fprintf(&b, "config diff unavailable: %s\n", r.ConfigDiffMessage)
	}
	fmt.Fprintf(&b, "exit=%d\n", r.ExitCode)
	return b.String()
}

func configDiffText(d session.ConfigDiff, stranded []session.StrandedItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "live config: %s (vs built-in seed)\n", d.Label)
	for _, sec := range []struct {
		name string
		diff session.SectionDiff
	}{{"stages", d.Stages}, {"edges", d.Edges}, {"gates", d.Gates}, {"liveness", d.Liveness}} {
		for _, l := range sec.diff.Added {
			fmt.Fprintf(&b, "  + %s: %s\n", sec.name, l)
		}
		for _, l := range sec.diff.Removed {
			fmt.Fprintf(&b, "  - %s: %s\n", sec.name, l)
		}
	}
	fmt.Fprintf(&b, "stranded item preview: %d item(s)\n", len(stranded))
	for _, it := range stranded {
		fmt.Fprintf(&b, "  item %s status=%s lost=%s no_outgoing_edges=%t in_disabled_stage=%t\n",
			it.ItemHash, it.Status, strings.Join(it.LostEdges, ","), it.NoOutgoingEdges, it.InDisabledStage)
	}
	if len(stranded) > 0 {
		b.WriteString("  the flip is blocked until the owner acknowledges every previewed item\n")
	}
	return b.String()
}

// NewWorkflowEngineConfigDiff builds the ConfigDiff func of a
// WorkflowEngineSoakSource: the live stage configuration versus the seed, plus
// the stranded-item preview under the enforce composition. Read-only.
func NewWorkflowEngineConfigDiff(client *ent.Client, cfg *session.ConfiguredWorkflowEngine) func(ctx context.Context) (session.ConfigDiff, []session.StrandedItem, error) {
	return func(ctx context.Context) (session.ConfigDiff, []session.StrandedItem, error) {
		diff, err := session.StageConfigDiff(ctx, client)
		if err != nil {
			return session.ConfigDiff{}, nil, err
		}
		items, disabled, err := session.ListItemStatusRefs(ctx, client)
		if err != nil {
			return session.ConfigDiff{}, nil, err
		}
		return diff, session.StrandedItemPreview(session.NewDefaultWorkflowEngine(), cfg, items, disabled), nil
	}
}
