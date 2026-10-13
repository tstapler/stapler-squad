package gitwiring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/backend"
)

// Flip criteria for a shadow cohort (plan Story 2.1.3, Gate G8). They are constants, not config,
// so the verdict cannot be tuned to pass.
const (
	SoakMinWindow         = 7 * 24 * time.Hour
	SoakMinCallsPerOp     = 500
	SoakDetectErrorBudget = 0.01 // day-one capability checklist ceiling (Story 1.3.4 h)

	soakLedgerFile    = "git-backend-soak.json"
	soakFlushInterval = time.Minute
)

// ledger persists the cumulative soak counters, so a service restart (make install-service) does
// not reset the 7-day window. It is written only while some cohort is in shadow.
type ledger struct {
	Since  time.Time  `json:"since"`
	Sample soakSample `json:"sample"`
}

type soakState struct {
	mu      sync.Mutex
	path    string // empty: no persistence
	cohorts backend.CohortMap
	base    ledger
	stop    func()
}

var (
	soakMu     sync.Mutex
	soakActive *soakState
)

// startSoak loads (or, when no cohort is shadowed, discards) the persisted ledger and starts the
// periodic flush. It is a no-op when no cohort is in shadow, so the default path adds nothing.
func startSoak(cohorts backend.CohortMap, dir string) {
	if !anyShadow(cohorts) {
		if dir != "" {
			// Leaving shadow ends the window: the next one starts a fresh clock.
			_ = os.Remove(filepath.Join(dir, soakLedgerFile))
		}
		return
	}
	st := &soakState{cohorts: cohorts}
	if dir != "" {
		st.path = filepath.Join(dir, soakLedgerFile)
		st.base = loadLedger(st.path)
	}
	if st.base.Since.IsZero() {
		st.base.Since = time.Now().UTC()
	}
	st.flush(context.Background())
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(soakFlushInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				st.flush(context.Background())
			case <-done:
				return
			}
		}
	}()
	st.stop = func() { close(done); st.flush(context.Background()) }
	soakMu.Lock()
	soakActive = st
	soakMu.Unlock()
}

// stopSoak ends the periodic flush after a last write. Production never calls it; tests do.
func stopSoak() {
	soakMu.Lock()
	st := soakActive
	soakActive = nil
	soakMu.Unlock()
	if st != nil && st.stop != nil {
		st.stop()
	}
}

func anyShadow(c backend.CohortMap) bool {
	for _, co := range backend.AllCohorts() {
		if c.Mode(co) == backend.BackendShadow {
			return true
		}
	}
	return false
}

func loadLedger(path string) ledger {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the config dir's own ledger file
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warn("git soak ledger unreadable, starting a new window", "err", err)
		}
		return ledger{}
	}
	var l ledger
	if err := json.Unmarshal(data, &l); err != nil {
		log.Warn("git soak ledger corrupt, starting a new window", "err", err)
		return ledger{}
	}
	return l
}

func (s *soakState) merged(ctx context.Context) ledger {
	return ledger{Since: s.base.Since, Sample: mergeSamples(s.base.Sample, readLive(ctx))}
}

// flush folds the live counters into the base and writes the ledger atomically.
func (s *soakState) flush(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return
	}
	l := s.merged(ctx)
	data, err := json.MarshalIndent(l, "", " ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		log.Warn("git soak ledger not written", "err", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Warn("git soak ledger not written", "err", err)
	}
}

// soakConfigDir is where the ledger and shadow-mismatch records live.
func soakConfigDir() string {
	dir, err := config.GetConfigDir()
	if err != nil {
		log.Warn("no config dir for the git soak ledger; counters reset on restart", "err", err)
		return ""
	}
	return dir
}

// OpSoak is one operation's shadow tally.
type OpSoak struct {
	Operation   string           `json:"operation"`
	ShadowCalls int64            `json:"shadow_calls"`
	Mismatch    map[string]int64 `json:"mismatch"`
	CLIP50Ms    *float64         `json:"cli_p50_ms,omitempty"`
	GoGitP50Ms  *float64         `json:"gogit_p50_ms,omitempty"`
}

// CapabilitySoak is the day-one checklist input (git_capability_detect_total).
type CapabilitySoak struct {
	Total            int64            `json:"total"`
	DetectError      int64            `json:"detect_error"`
	DetectErrorRatio float64          `json:"detect_error_ratio"`
	ByOutcome        map[string]int64 `json:"by_outcome"`
	SubReasons       map[string]int64 `json:"detect_error_sub_reasons"`
}

// SoakVerdict is the mechanical flip decision. Ready never flips anything; it only says the
// recorded criteria hold. G8 (gates.md) is a document gate this code cannot check.
type SoakVerdict struct {
	Ready    bool     `json:"ready"`
	Blockers []string `json:"blockers,omitempty"`
}

// SoakReport is what /actuator/git-backend serves.
type SoakReport struct {
	GeneratedAt time.Time              `json:"generated_at"`
	Cohorts     map[string]string      `json:"cohorts"`
	Active      bool                   `json:"shadow_active"`
	Since       *time.Time             `json:"window_since,omitempty"`
	WindowHours float64                `json:"window_hours"`
	Operations  []OpSoak               `json:"operations"`
	Capability  CapabilitySoak         `json:"capability"`
	Spawns      map[string]int64       `json:"cli_spawns"`
	Fallbacks   map[string]int64       `json:"fallbacks"`
	Verdict     map[string]SoakVerdict `json:"verdict"`
}

// SoakStatus reports the soak counters of the process-wide backend. It never starts git.
func SoakStatus(ctx context.Context) SoakReport {
	Shared() // ensure the router (and with it the ledger) is built
	soakMu.Lock()
	st := soakActive
	soakMu.Unlock()
	if st == nil {
		return buildReport(time.Now(), backend.CohortMap{}, ledger{}, false)
	}
	st.mu.Lock()
	l := st.merged(ctx)
	st.mu.Unlock()
	return buildReport(time.Now(), st.cohorts, l, true)
}

func label(p counterPoint, k string) string { return p.Labels[k] }

func buildReport(now time.Time, cohorts backend.CohortMap, l ledger, active bool) SoakReport {
	rep := SoakReport{
		GeneratedAt: now.UTC(), Cohorts: map[string]string{}, Active: active,
		Spawns: map[string]int64{}, Fallbacks: map[string]int64{}, Verdict: map[string]SoakVerdict{},
		Capability: CapabilitySoak{ByOutcome: map[string]int64{}, SubReasons: map[string]int64{}},
	}
	for _, c := range backend.AllCohorts() {
		rep.Cohorts[c.String()] = cohorts.Mode(c).String()
	}
	if !l.Since.IsZero() {
		since := l.Since.UTC()
		rep.Since = &since
		rep.WindowHours = now.Sub(l.Since).Hours()
	}

	ops := map[string]*OpSoak{}
	opOf := func(name string) *OpSoak {
		if o, ok := ops[name]; ok {
			return o
		}
		o := &OpSoak{Operation: name, Mismatch: map[string]int64{}}
		ops[name] = o
		return o
	}
	for _, p := range l.Sample.Counters {
		switch p.Name {
		case backend.MetricShadowCallsTotal:
			opOf(label(p, "operation")).ShadowCalls += p.Value
		case backend.MetricShadowMismatchTotal:
			opOf(label(p, "operation")).Mismatch[label(p, "class")] += p.Value
		case backend.MetricCapabilityDetectTotal:
			out := label(p, "outcome")
			rep.Capability.Total += p.Value
			rep.Capability.ByOutcome[out] += p.Value
			if out == "detect_error" {
				rep.Capability.DetectError += p.Value
				rep.Capability.SubReasons[label(p, "sub_reason")] += p.Value
			}
		case backend.MetricCLISpawnTotal:
			rep.Spawns[label(p, "operation")+"/"+label(p, "reason")] += p.Value
		case backend.MetricFallbackTotal:
			rep.Fallbacks[label(p, "cohort")+"/"+label(p, "operation")+"/"+label(p, "reason")] += p.Value
		}
	}
	for _, h := range l.Sample.Histograms {
		if h.Name != backend.MetricShadowDurationMS {
			continue
		}
		v, ok := h.p50()
		if !ok {
			continue
		}
		o := opOf(h.Labels["operation"])
		switch h.Labels["implementation"] {
		case string(backend.ImplCLI):
			o.CLIP50Ms = &v
		case string(backend.ImplGoGit):
			o.GoGitP50Ms = &v
		}
	}
	// Every shadowable operation of every shadowed cohort is listed, even at zero calls: an
	// operation the app never exercises must hold the window open, not vanish from it.
	for _, op := range backend.AllOperations() {
		if !op.Mutating() && cohorts.Mode(backend.CohortOf(op)) == backend.BackendShadow {
			opOf(string(op))
		}
	}
	for _, o := range ops {
		rep.Operations = append(rep.Operations, *o)
	}
	sort.Slice(rep.Operations, func(i, j int) bool { return rep.Operations[i].Operation < rep.Operations[j].Operation })
	if rep.Capability.Total > 0 {
		rep.Capability.DetectErrorRatio = float64(rep.Capability.DetectError) / float64(rep.Capability.Total)
	}
	for _, c := range backend.AllCohorts() {
		if cohorts.Mode(c) == backend.BackendShadow {
			rep.Verdict[c.String()] = evaluate(rep, c)
		}
	}
	return rep
}

// evaluate applies the flip criteria to one shadowed cohort.
func evaluate(rep SoakReport, cohort backend.Cohort) SoakVerdict {
	var blockers []string
	if rep.WindowHours < SoakMinWindow.Hours() {
		blockers = append(blockers, fmt.Sprintf("window is %.1f h, need %.0f h", rep.WindowHours, SoakMinWindow.Hours()))
	}
	for _, o := range rep.Operations {
		op := backend.OperationName(o.Operation)
		if backend.CohortOf(op) != cohort || op.Mutating() {
			continue
		}
		if o.ShadowCalls < SoakMinCallsPerOp {
			blockers = append(blockers, fmt.Sprintf("%s: %d shadow calls, need %d", o.Operation, o.ShadowCalls, SoakMinCallsPerOp))
		}
		if n := o.Mismatch[string(backend.MismatchReal)] + o.Mismatch[string(backend.MismatchFalseClean)]; n > 0 {
			blockers = append(blockers, fmt.Sprintf("%s: %d real mismatch(es); fix from shadow-mismatches/ and restart the window", o.Operation, n))
		}
		switch {
		case o.CLIP50Ms == nil || o.GoGitP50Ms == nil:
			if o.ShadowCalls > 0 {
				blockers = append(blockers, o.Operation+": no p50 recorded")
			}
		case *o.GoGitP50Ms > *o.CLIP50Ms:
			blockers = append(blockers, fmt.Sprintf("%s: gogit p50 %.2f ms is slower than cli p50 %.2f ms", o.Operation, *o.GoGitP50Ms, *o.CLIP50Ms))
		}
	}
	if rep.Capability.DetectErrorRatio >= SoakDetectErrorBudget {
		blockers = append(blockers, fmt.Sprintf("capability detect_error ratio %.2f%% is at or above %.0f%%", 100*rep.Capability.DetectErrorRatio, 100*SoakDetectErrorBudget))
	}
	return SoakVerdict{Ready: len(blockers) == 0, Blockers: blockers}
}

// Text renders the report for a terminal.
func (r SoakReport) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "git backend soak at %s\n", r.GeneratedAt.Format(time.RFC3339))
	for _, c := range backend.AllCohorts() {
		fmt.Fprintf(&b, "  cohort %-10s %s\n", c, r.Cohorts[c.String()])
	}
	if r.Since == nil {
		b.WriteString("no cohort is in shadow: nothing is being soaked\n")
		return b.String()
	}
	fmt.Fprintf(&b, "window since %s (%.1f h of %.0f h)\n\n", r.Since.Format(time.RFC3339), r.WindowHours, SoakMinWindow.Hours())
	fmt.Fprintf(&b, "%-16s %8s %6s %6s %12s %12s %12s\n", "operation", "calls", "racy", "real", "cli p50 ms", "gogit p50 ms", "false_clean")
	for _, o := range r.Operations {
		fmt.Fprintf(&b, "%-16s %8d %6d %6d %12s %12s %12d\n", o.Operation, o.ShadowCalls,
			o.Mismatch[string(backend.MismatchRacy)], o.Mismatch[string(backend.MismatchReal)],
			fmtMs(o.CLIP50Ms), fmtMs(o.GoGitP50Ms), o.Mismatch[string(backend.MismatchFalseClean)])
	}
	c := r.Capability
	fmt.Fprintf(&b, "\ncapability preflights: %d, detect_error %d (%.2f%%, budget < %.0f%%)\n", c.Total, c.DetectError, 100*c.DetectErrorRatio, 100*SoakDetectErrorBudget)
	for _, k := range sortedKeys(c.SubReasons) {
		fmt.Fprintf(&b, "  detect_error sub_reason %-28s %d\n", k, c.SubReasons[k])
	}
	for _, k := range sortedKeys(r.Fallbacks) {
		fmt.Fprintf(&b, "  fallback %-52s %d\n", k, r.Fallbacks[k])
	}
	for _, name := range sortedKeys(r.Verdict) {
		v := r.Verdict[name]
		if v.Ready {
			fmt.Fprintf(&b, "\nVERDICT %s: criteria met (G8 in gates.md is still a manual check)\n", name)
			continue
		}
		fmt.Fprintf(&b, "\nVERDICT %s: NOT READY\n", name)
		for _, bl := range v.Blockers {
			fmt.Fprintf(&b, "  - %s\n", bl)
		}
	}
	return b.String()
}

func fmtMs(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.2f", *v)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
