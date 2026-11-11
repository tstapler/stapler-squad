// Package soak validates soak-ledger.yaml and evaluates the Story 7.6 soak rule.
//
// Rule: over the last N PR runs there are zero unattributed failures and zero bazel-real failures in the last
// K runs, and the parity gate passes. Attributed flakes do not reset the count; a bazel-real failure does.
package soak

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Attribution classes for a failed run.
const (
	Flake             = "flake"
	BazelReal         = "bazel-real"
	Infra             = "infra"
	GeneratedFileRace = "generated-file-race"
)

// Defaults are the owner-confirmed values (owner-attestations.yaml `defaults`).
const (
	DefaultN            = 30
	DefaultK            = 10
	DefaultQueueTimeout = 900
)

var bugID = regexp.MustCompile(`^BUG-[0-9]+$`)

// Run is one PR run.
type Run struct {
	RunID        string `yaml:"run_id"`
	Result       string `yaml:"result"` // pass | fail
	Attribution  string `yaml:"attribution,omitempty"`
	Bug          string `yaml:"bug,omitempty"`
	QueueSeconds int    `yaml:"queue_seconds,omitempty"`
}

// Ledger is the file schema.
type Ledger struct {
	Schema              int   `yaml:"schema"`
	N                   int   `yaml:"soak_runs_n"`
	K                   int   `yaml:"soak_consecutive_green_k"`
	QueueTimeoutSeconds int   `yaml:"queue_timeout_seconds"`
	Runs                []Run `yaml:"runs"`
}

// Parse reads and defaults a ledger. Unknown keys are rejected so schema typos surface.
func Parse(r io.Reader) (*Ledger, error) {
	var l Ledger
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&l); err != nil && err != io.EOF {
		return nil, err
	}
	if l.N == 0 {
		l.N = DefaultN
	}
	if l.K == 0 {
		l.K = DefaultK
	}
	if l.QueueTimeoutSeconds == 0 {
		l.QueueTimeoutSeconds = DefaultQueueTimeout
	}
	return &l, nil
}

// Validate returns every schema problem (empty when the ledger is well formed).
func (l *Ledger) Validate() []string {
	var errs []string
	if l.Schema != 1 {
		errs = append(errs, fmt.Sprintf("schema must be 1, got %d", l.Schema))
	}
	if l.K > l.N {
		errs = append(errs, fmt.Sprintf("soak_consecutive_green_k (%d) exceeds soak_runs_n (%d)", l.K, l.N))
	}
	seen := map[string]bool{}
	for i, r := range l.Runs {
		id := fmt.Sprintf("runs[%d] (%s)", i, r.RunID)
		switch {
		case r.RunID == "":
			errs = append(errs, id+": run_id is required")
		case seen[r.RunID]:
			errs = append(errs, id+": duplicate run_id")
		}
		seen[r.RunID] = true
		errs = append(errs, l.checkRun(id, r)...)
	}
	return errs
}

func (l *Ledger) checkRun(id string, r Run) []string {
	var errs []string
	switch r.Result {
	case "pass":
		if r.Attribution != "" || r.Bug != "" {
			errs = append(errs, id+": a passing run takes no attribution")
		}
		if r.QueueSeconds > l.QueueTimeoutSeconds {
			errs = append(errs, fmt.Sprintf("%s: queue wait %d s exceeds the %d s timeout, so the run cannot be a pass", id, r.QueueSeconds, l.QueueTimeoutSeconds))
		}
	case "fail":
		errs = append(errs, checkAttribution(id, r)...)
	default:
		errs = append(errs, fmt.Sprintf("%s: result must be pass or fail, got %q", id, r.Result))
	}
	return errs
}

func checkAttribution(id string, r Run) []string {
	switch r.Attribution {
	case "", BazelReal, Infra, GeneratedFileRace:
		if r.Bug != "" {
			return []string{id + ": bug is only valid for attribution flake"}
		}
	case Flake:
		if !bugID.MatchString(r.Bug) {
			return []string{id + ": flake attribution needs bug: BUG-<n>"}
		}
	default:
		return []string{fmt.Sprintf("%s: unknown attribution %q", id, r.Attribution)}
	}
	return nil
}

// Report is the evaluation outcome.
type Report struct {
	Met          bool
	Reasons      []string
	Window       int
	Unattributed int
	BazelReal    int // within the last K runs
	ByClass      map[string]int
}

// Evaluate applies the soak rule to the last N runs. parityPass is the current `PARITY: PASS` state.
func (l *Ledger) Evaluate(parityPass bool) Report {
	rep := Report{ByClass: map[string]int{}}
	runs := l.Runs
	if len(runs) > l.N {
		runs = runs[len(runs)-l.N:]
	}
	rep.Window = len(runs)
	for i, r := range runs {
		if r.Result != "fail" {
			continue
		}
		cls := r.Attribution
		if cls == "" {
			rep.Unattributed++
			continue
		}
		rep.ByClass[cls]++
		if cls == BazelReal && i >= len(runs)-l.K {
			rep.BazelReal++
		}
	}
	if rep.Window < l.N {
		rep.Reasons = append(rep.Reasons, fmt.Sprintf("only %d of %d runs recorded", rep.Window, l.N))
	}
	if rep.Unattributed > 0 {
		rep.Reasons = append(rep.Reasons, fmt.Sprintf("%d unattributed failure(s)", rep.Unattributed))
	}
	if rep.BazelReal > 0 {
		rep.Reasons = append(rep.Reasons, fmt.Sprintf("%d bazel-real failure(s) in the last %d runs", rep.BazelReal, l.K))
	}
	if !parityPass {
		rep.Reasons = append(rep.Reasons, "PARITY: PASS does not hold")
	}
	rep.Met = len(rep.Reasons) == 0
	return rep
}

// String renders the one-line verdict plus the per-class counts (generated-file-race is reported separately).
func (r Report) String() string {
	verdict := "SOAK: MET"
	if !r.Met {
		verdict = "SOAK: NOT MET (" + strings.Join(r.Reasons, "; ") + ")"
	}
	return fmt.Sprintf("%s\nwindow=%d flake=%d bazel-real=%d infra=%d generated-file-race=%d unattributed=%d",
		verdict, r.Window, r.ByClass[Flake], r.ByClass[BazelReal], r.ByClass[Infra], r.ByClass[GeneratedFileRace], r.Unattributed)
}
