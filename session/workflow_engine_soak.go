package session

// workflow_engine_soak.go — the one soak evaluator behind `make
// workflow-engine-soak-status`, /actuator/workflow-engine and the readiness
// evidence other stories read. READY can never be vacuous: it needs evidence
// (floor), history (span W), per-edge coverage (N or an owner ack) and zero
// seeded divergence. The owner-approved parameters live here as constants so a
// verdict cannot be tuned to pass.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// Owner-approved soak parameters (plan Story 1.1.3).
const (
	SoakWindowOperatorAgent = 7 * 24 * time.Hour
	SoakWindowSystem        = 14 * 24 * time.Hour
	SoakMinTransitionTotal  = 100 // floor on path=transition samples
	SoakMinEdgeSamples      = 20  // N per expected edge
	SoakReviewAfter         = 14 * 24 * time.Hour
)

// VerdictStatus is the outcome of one class's evaluation.
type VerdictStatus string

const (
	VerdictReady         VerdictStatus = "READY"
	VerdictNotReady      VerdictStatus = "NOT_READY"
	VerdictNeedsOwnerAck VerdictStatus = "NEEDS_OWNER_ACK"
	VerdictUnavailable   VerdictStatus = "UNAVAILABLE"
)

// SoakAcks are the owner acknowledgements recorded in the runbook.
type SoakAcks struct {
	// Edges are expected edges accepted with fewer than N samples.
	Edges []string `json:"edges,omitempty"`
	// DivergentEdges are edges whose divergence under an edited config maps to
	// an acknowledged --config-diff entry.
	DivergentEdges []string `json:"divergent_edges,omitempty"`
	// StatefulEdges are stateful-gate edges the owner accepts as unverified.
	StatefulEdges []string `json:"stateful_edges,omitempty"`
	// GuardNew are the "caller|from->to" triples whose guard_new divergence
	// (a Default guard the legacy path never asked) the owner accepts.
	GuardNew []string `json:"guard_new,omitempty"`
	// LowVolume is the four-part acknowledgement that stands in for the floor
	// and the per-edge N on a low-volume class (Risk Control).
	LowVolume LowVolumeAck `json:"low_volume"`
}

// LowVolumeAck is the owner's acknowledgement of the four artifacts: the
// --config-diff output, the stranded-item preview, the Story 1.5.9b
// every-registered-caller table and the Story 1.5.9d replay report.
type LowVolumeAck struct {
	ConfigDiff      bool `json:"config_diff"`
	StrandedPreview bool `json:"stranded_preview"`
	CallerTable     bool `json:"caller_table"`
	ReplayReport    bool `json:"replay_report"`
}

// Complete reports whether all four artifacts are acknowledged.
func (a LowVolumeAck) Complete() bool {
	return a.ConfigDiff && a.StrandedPreview && a.CallerTable && a.ReplayReport
}

// SoakAckFile is the --ack-file document: one SoakAcks per caller class.
type SoakAckFile struct {
	Classes map[string]SoakAcks `json:"classes"`
}

// ParseSoakAckFile decodes an ack file strictly: an unknown field or class is
// an error, so a typo can never silently acknowledge nothing.
func ParseSoakAckFile(data []byte) (SoakAckFile, error) {
	var f SoakAckFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return SoakAckFile{}, fmt.Errorf("ack file: %w", err)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return SoakAckFile{}, errors.New("ack file: unexpected data after the JSON object")
	}
	for c := range f.Classes {
		switch CallerClass(c) {
		case ClassOperator, ClassAgent, ClassSystem:
		default:
			return SoakAckFile{}, fmt.Errorf("ack file: unknown class %q", c)
		}
	}
	return f, nil
}

// For returns the acknowledgements recorded for class.
func (f SoakAckFile) For(class CallerClass) SoakAcks { return f.Classes[string(class)] }

// Merge returns a with b's entries appended; each LowVolume part holds if
// either side acknowledges it.
func (a SoakAcks) Merge(b SoakAcks) SoakAcks {
	a.Edges = append(append([]string(nil), a.Edges...), b.Edges...)
	a.DivergentEdges = append(append([]string(nil), a.DivergentEdges...), b.DivergentEdges...)
	a.StatefulEdges = append(append([]string(nil), a.StatefulEdges...), b.StatefulEdges...)
	a.GuardNew = append(append([]string(nil), a.GuardNew...), b.GuardNew...)
	a.LowVolume = LowVolumeAck{
		ConfigDiff:      a.LowVolume.ConfigDiff || b.LowVolume.ConfigDiff,
		StrandedPreview: a.LowVolume.StrandedPreview || b.LowVolume.StrandedPreview,
		CallerTable:     a.LowVolume.CallerTable || b.LowVolume.CallerTable,
		ReplayReport:    a.LowVolume.ReplayReport || b.LowVolume.ReplayReport,
	}
	return a
}

// SoakParams are the inputs EvaluateSoak needs besides the ledger.
type SoakParams struct {
	Now              time.Time
	Window           time.Duration
	Floor            int64
	MinEdgeSamples   int64
	ExpectedEdges    []SystemEdge
	Acks             SoakAcks
	AnyClassEnforced bool
	ReproPath        string // where the redacted repro records live, printed on alarm
}

// ExpectedEdgesForClass lists the edges the caller registry says class can
// request, wildcard archive edges expanded over the built-in statuses and
// forwarding entries (which declare none) contributing nothing. Sorted, unique.
func ExpectedEdgesForClass(class CallerClass) []SystemEdge {
	seen := map[SystemEdge]bool{}
	var out []SystemEdge
	add := func(e SystemEdge) {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	for _, c := range transitionCallers {
		if c.Class != class || c.Forwarding {
			continue
		}
		for _, e := range c.Edges {
			if e.From != AnyStatus {
				add(e)
				continue
			}
			for _, from := range builtInStageOrder {
				add(SystemEdge{From: from, To: e.To})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

// DefaultSoakParams returns the owner-approved parameters for class, with the
// expected-edge list taken from the caller registry.
func DefaultSoakParams(class CallerClass, now time.Time) SoakParams {
	w := SoakWindowOperatorAgent
	if class == ClassSystem {
		w = SoakWindowSystem
	}
	return SoakParams{
		Now: now, Window: w, Floor: SoakMinTransitionTotal, MinEdgeSamples: SoakMinEdgeSamples,
		ExpectedEdges: ExpectedEdgesForClass(class),
	}
}

// Verdict is one class's soak result.
type Verdict struct {
	Class             CallerClass       `json:"class"`
	Status            VerdictStatus     `json:"status"`
	Reasons           []string          `json:"reasons,omitempty"`
	ConfigWindow      string            `json:"config_window"`
	TransitionSamples int64             `json:"transition_samples"`
	Floor             int64             `json:"floor"`
	SpanHours         float64           `json:"span_hours"`
	WindowHours       float64           `json:"window_hours"`
	Alarms            map[string]int64  `json:"alarms,omitempty"`
	Unverified        []string          `json:"unverified_in_shadow,omitempty"`
	CoveredOffline    []string          `json:"covered_offline,omitempty"`
	Review            string            `json:"review,omitempty"`
	ReproPath         string            `json:"repro_path,omitempty"`
	ExitCode          int               `json:"exit_code"`
	Detail            map[string]string `json:"detail,omitempty"`
}

func exitCodeFor(s VerdictStatus) int {
	switch s {
	case VerdictReady:
		return 0
	case VerdictUnavailable:
		return 2
	default:
		return 1
	}
}

// EvaluateSoakFromSource loads the ledger and evaluates; a load error is
// UNAVAILABLE, never READY.
func EvaluateSoakFromSource(class CallerClass, load func() (ShadowLedger, error), p SoakParams) Verdict {
	l, err := load()
	if err != nil {
		return Verdict{
			Class: class, Status: VerdictUnavailable, ExitCode: exitCodeFor(VerdictUnavailable),
			Reasons: []string{fmt.Sprintf("ledger unavailable: %v", err)},
		}
	}
	return EvaluateSoak(class, l, p)
}

// EvaluateSoak scores one class's ledger against the flip criteria.
func EvaluateSoak(class CallerClass, l ShadowLedger, p SoakParams) Verdict {
	v := Verdict{
		Class: class, Floor: p.Floor, WindowHours: p.Window.Hours(),
		Review: reviewNotice(l, p), ReproPath: p.ReproPath,
	}
	cl := l.Classes[string(class)]
	if cl == nil || totalCalls(cl, "") == 0 {
		v.Status, v.ConfigWindow = VerdictNotReady, ConfigUnlabeled
		v.Reasons = []string{"no evidence: zero shadowed calls recorded (is shadow on?)"}
		v.ExitCode = exitCodeFor(v.Status)
		return v
	}
	ev := soakEvaluation{v: &v, cl: cl, p: p, l: l}
	ev.run()
	return v
}

type soakEvaluation struct {
	v        *Verdict
	cl       *ClassLedger
	p        SoakParams
	l        ShadowLedger
	notReady []string
	needsAck []string
	stateful map[string]int64
}

func (e *soakEvaluation) run() {
	e.labelWindow()
	e.checkAlarms()
	e.checkGuardNew()
	e.collectStateful()
	e.checkFloor()
	e.checkSpan()
	e.checkExpectedEdges()
	e.listCoveredOffline()
	e.finish()
}

func (e *soakEvaluation) labelWindow() {
	e.v.ConfigWindow = ConfigSeeded
	if e.cl.ConfigSeen[ConfigEdited] > 0 || e.cl.ConfigSeen[ConfigUnlabeled] > 0 {
		e.v.ConfigWindow = ConfigEdited
	}
}

var alarmClasses = map[string]bool{
	string(ClassGraph): true, string(ClassGateMissing): true, string(ClassGateExtra): true, string(ClassErrorKind): true,
}

// checkAlarms applies rules (a) and (b): seeded windows alarm on any
// divergence on either path; edited windows need every divergent edge acked.
func (e *soakEvaluation) checkAlarms() {
	counts, seeded := map[string]int64{}, map[string]int64{}
	for key, n := range e.cl.Divergence {
		parts := strings.Split(key, "|") // Method|class|config|path
		if len(parts) != 4 || !alarmClasses[parts[1]] {
			continue
		}
		counts[parts[1]] += n
		if parts[2] == ConfigSeeded {
			seeded[parts[1]] += n
		}
	}
	if len(counts) == 0 {
		return
	}
	e.v.Alarms = counts
	// A divergence recorded under the seeded config is a bug whatever the window
	// label says; only edited-config divergence can be acknowledged.
	if e.v.ConfigWindow == ConfigSeeded || len(seeded) > 0 {
		shown := counts
		if e.v.ConfigWindow != ConfigSeeded {
			shown = seeded
		}
		e.notReady = append(e.notReady, fmt.Sprintf("divergence under the seeded config %s; repro records: %s", formatCounts(shown), e.reproPath()))
		if e.v.ConfigWindow == ConfigSeeded {
			return
		}
	}
	acked := toSet(e.p.Acks.DivergentEdges)
	for _, k := range sortedKeys(e.cl.Edges) {
		if e.cl.Edges[k].Divergences > 0 && !acked[k] {
			e.needsAck = append(e.needsAck, fmt.Sprintf("divergence on %s under an edited config is not acknowledged against --config-diff", k))
		}
	}
}

// checkGuardNew requires an owner acknowledgement for every (caller,from,to)
// with a guard_new divergence: a newly governed Unchecked caller (Risk Control).
func (e *soakEvaluation) checkGuardNew() {
	acked := toSet(e.p.Acks.GuardNew)
	suffix := "|" + string(ClassGuardNew)
	for _, key := range sortedKeys(e.cl.CallerEdgeDivergence) {
		if !strings.HasSuffix(key, suffix) || e.cl.CallerEdgeDivergence[key] == 0 {
			continue
		}
		triple := strings.TrimSuffix(key, suffix)
		if !acked[triple] {
			e.notReady = append(e.notReady, fmt.Sprintf("guard_new on %s (%d) is not acknowledged in the ack file", triple, e.cl.CallerEdgeDivergence[key]))
		}
	}
}

func (e *soakEvaluation) reproPath() string {
	if e.p.ReproPath == "" {
		return "(ledger path unknown)"
	}
	return e.p.ReproPath
}

func (e *soakEvaluation) collectStateful() {
	e.stateful = map[string]int64{}
	for k, es := range e.cl.Edges {
		if es.StatefulSkips > 0 {
			e.stateful[k] = es.StatefulSkips
		}
	}
	acked := toSet(e.p.Acks.StatefulEdges)
	for _, k := range sortedKeys(e.cl.Edges) {
		n, ok := e.stateful[k]
		if !ok {
			continue
		}
		e.v.Unverified = append(e.v.Unverified, fmt.Sprintf("%s UNVERIFIED in shadow (not_compared{stateful_gate}) skipped=%d", k, n))
		if !acked[k] {
			e.needsAck = append(e.needsAck, fmt.Sprintf("stateful edge %s is unverified in shadow and not acknowledged", k))
		}
	}
}

// checkFloor counts only path=transition samples, and not those on stateful
// edges (which were not fully compared).
func (e *soakEvaluation) checkFloor() {
	total := totalCalls(e.cl, string(PathTransition))
	for k := range e.stateful {
		total -= e.cl.Edges[k].TransitionSamples
	}
	if total < 0 {
		total = 0
	}
	e.v.TransitionSamples = total
	if total < e.p.Floor {
		if e.p.Acks.LowVolume.Complete() {
			e.waive("floor", fmt.Sprintf("%d of %d path=transition samples, waived by the four-part low-volume acknowledgement", total, e.p.Floor))
			return
		}
		e.notReady = append(e.notReady, fmt.Sprintf("only %d path=transition samples, floor is %d", total, e.p.Floor))
	}
}

func (e *soakEvaluation) checkSpan() {
	earliest := earliestStart(e.l, e.cl)
	var span time.Duration
	if !earliest.IsZero() {
		span = e.p.Now.Sub(earliest)
	}
	e.v.SpanHours = span.Hours()
	if span < e.p.Window {
		e.notReady = append(e.notReady, fmt.Sprintf("history spans %.1fh, window W is %.0fh", span.Hours(), e.p.Window.Hours()))
	}
}

func (e *soakEvaluation) checkExpectedEdges() {
	acked := toSet(e.p.Acks.Edges)
	for _, edge := range e.p.ExpectedEdges {
		key := EdgeKey(edge.From, edge.To)
		if _, stateful := e.stateful[key]; stateful {
			continue // reported through the UNVERIFIED rule above
		}
		n := e.cl.Edges[key].TransitionSamples
		if n > 0 && n < e.p.MinEdgeSamples && e.p.Acks.LowVolume.Complete() {
			e.waive("edge "+key, fmt.Sprintf("%d of %d samples, waived by the four-part low-volume acknowledgement", n, e.p.MinEdgeSamples))
			continue
		}
		if n < e.p.MinEdgeSamples && !acked[key] {
			e.notReady = append(e.notReady, fmt.Sprintf("expected edge %s has %d of %d path=transition samples and no owner acknowledgement", key, n, e.p.MinEdgeSamples))
		}
	}
}

func (e *soakEvaluation) waive(what, why string) {
	if e.v.Detail == nil {
		e.v.Detail = map[string]string{}
	}
	e.v.Detail[what] = why
}

func (e *soakEvaluation) listCoveredOffline() {
	expected := map[string]bool{}
	for _, edge := range e.p.ExpectedEdges {
		expected[EdgeKey(edge.From, edge.To)] = true
	}
	for from, targets := range domain.ValidTransitions() {
		for to := range targets {
			key := EdgeKey(from, to)
			if !expected[key] && e.cl.Edges[key].TransitionSamples == 0 {
				e.v.CoveredOffline = append(e.v.CoveredOffline, key)
			}
		}
	}
	sort.Strings(e.v.CoveredOffline)
}

func (e *soakEvaluation) finish() {
	e.v.Reasons = append(append([]string(nil), e.notReady...), e.needsAck...)
	switch {
	case len(e.notReady) > 0:
		e.v.Status = VerdictNotReady
	case len(e.needsAck) > 0:
		e.v.Status = VerdictNeedsOwnerAck
	default:
		e.v.Status = VerdictReady
	}
	e.v.ExitCode = exitCodeFor(e.v.Status)
}

func totalCalls(cl *ClassLedger, path string) int64 {
	var n int64
	for k, c := range cl.Calls {
		if path == "" || strings.HasSuffix(k, "|"+path) {
			n += c
		}
	}
	return n
}

func earliestStart(l ShadowLedger, cl *ClassLedger) time.Time {
	var earliest time.Time
	consider := func(t time.Time) {
		if !t.IsZero() && (earliest.IsZero() || t.Before(earliest)) {
			earliest = t
		}
	}
	for _, t := range l.BinaryStarts {
		consider(t)
	}
	if cl != nil {
		consider(cl.FirstSampleAt)
	}
	return earliest
}

// SoakReviewDueAt is the structured form of reviewNotice's date: the earliest
// binary start plus SoakReviewAfter. ok is false when no start is recorded.
func SoakReviewDueAt(l ShadowLedger) (due time.Time, ok bool) {
	earliest := earliestStart(l, nil)
	if earliest.IsZero() {
		return time.Time{}, false
	}
	return earliest.Add(SoakReviewAfter), true
}

// reviewNotice is the dated forcing function (requirements Metric 6): advisory
// only, it never changes a verdict or exit code.
func reviewNotice(l ShadowLedger, p SoakParams) string {
	earliest := earliestStart(l, nil)
	if earliest.IsZero() {
		return ""
	}
	due := earliest.Add(SoakReviewAfter)
	out := "REVIEW DUE " + due.UTC().Format("2006-01-02")
	if p.Now.After(due) && !p.AnyClassEnforced {
		out += "\nREVIEW OVERDUE: no class is enforced; run the Outcome review"
	}
	return out
}

func toSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, s := range items {
		out[s] = true
	}
	return out
}

func formatCounts(m map[string]int64) string {
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}

// Text renders the verdict for a terminal.
func (v Verdict) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "workflow engine soak: class=%s verdict=%s exit=%d config_window=%s\n", v.Class, v.Status, v.ExitCode, v.ConfigWindow)
	fmt.Fprintf(&b, "  path=transition samples: %d (floor %d)\n", v.TransitionSamples, v.Floor)
	fmt.Fprintf(&b, "  history span: %.1fh (window %.0fh)\n", v.SpanHours, v.WindowHours)
	for _, r := range v.Reasons {
		fmt.Fprintf(&b, "  - %s\n", r)
	}
	for _, u := range v.Unverified {
		fmt.Fprintf(&b, "  %s\n", u)
	}
	if len(v.CoveredOffline) > 0 {
		fmt.Fprintf(&b, "  covered offline by harness (non-blocking): %d edges\n", len(v.CoveredOffline))
	}
	if v.Review != "" {
		b.WriteString(v.Review + "\n")
	}
	return b.String()
}

// OverallSoakExit is the exit code for a set of verdicts: 2 when any class is
// unavailable, else 1 when any is not READY, else 0.
func OverallSoakExit(vs []Verdict) int {
	if len(vs) == 0 {
		return 1
	}
	code := 0
	for _, v := range vs {
		switch {
		case v.ExitCode == 2:
			return 2
		case v.ExitCode != 0:
			code = 1
		}
	}
	return code
}
