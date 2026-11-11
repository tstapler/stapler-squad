package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
)

func lookPath(name string) (string, error) { return exec.LookPath(name) }

// Report collects one tool's result keys (the names the plan's jq Verify lines read), its named checks and errors.
type Report struct {
	Keys   map[string]any
	Checks map[string]bool
	Rules  map[string]string // check name -> the plan's PASS-iff rule, quoted
	Errors []string
}

func NewReport() *Report {
	return &Report{Keys: map[string]any{}, Checks: map[string]bool{}, Rules: map[string]string{}}
}

// Check records a rule outcome. An unevaluated check is never recorded, so it cannot read as a Pass.
func (r *Report) Check(name string, ok bool, rule string) {
	r.Checks[name], r.Rules[name] = ok, rule
}

func (r *Report) Errorf(format string, a ...any) {
	r.Errors = append(r.Errors, fmt.Sprintf(format, a...))
}

// ChecksPass is true only when something was checked, nothing errored and every check held. It is an input to
// the S6 verdict, never the verdict row itself.
func (r *Report) ChecksPass() bool {
	if len(r.Errors) > 0 || len(r.Checks) == 0 {
		return false
	}
	for _, ok := range r.Checks {
		if !ok {
			return false
		}
	}
	return true
}

// Output is the printed JSON: the plan's keys at top level plus checks, rules, errors and the noisy tags.
func (r *Report) Output(rs []Reading, allowNoisy bool) map[string]any {
	out := map[string]any{}
	for k, v := range r.Keys {
		out[k] = v
	}
	out["checks"], out["rules"], out["errors"] = r.Checks, r.Rules, r.Errors
	out["checks_pass"] = r.ChecksPass()
	out["not_quiet"] = AnyNoisy(AsBuilds(rs))
	out["allow_noisy_host"] = allowNoisy
	return out
}

// Scalars are the numeric and boolean keys, prefixed s6_, ready for spike-measurements.json.
func (r *Report) Scalars() map[string]any {
	kv := map[string]any{}
	for k, v := range r.Keys {
		switch v.(type) {
		case bool, int, int64, float64:
			kv["s6_"+k] = v
		}
	}
	return kv
}

// Finish prints (or writes) the output and, when update names an inputs file, merges the scalar keys into it
// with the same noisy_inputs tagging the existing collectors use. An errored run writes no measurement keys.
func (r *Report) Finish(out, update, source string, rs []Reading, allowNoisy bool) error {
	if err := writeOut(out, r.Output(rs, allowNoisy)); err != nil {
		return err
	}
	if update == "" || len(r.Errors) > 0 {
		return nil
	}
	b := AsBuilds(rs)
	return UpdateInputs(update, r.Scalars(), AnyNoisy(b), NoisyEntry(b, source))
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
