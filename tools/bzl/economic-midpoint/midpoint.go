package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Thresholds mirror tools/bzl/economic-gate.py mid_t1/mid_t3/mid_t4 (Story 2.7); keep them in step.
const (
	t1Cap           = 0.30
	t1RA1Factor     = 1.5
	t3SecondOfFirst = 0.25
	t4Factor        = 1.25
	t4SlackS        = 5.0
	quietLoad1      = 2.0
)

// Verdict is one midpoint criterion result.
type Verdict struct {
	ID     string
	Pass   bool
	Detail string
}

func (v Verdict) Line() string {
	if v.Pass {
		return v.ID + ": PASS"
	}
	return v.ID + ": FAIL"
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// Ratio is selected/total; a "//..." selection means the whole graph, so 1.
func Ratio(selected []string, total int) float64 {
	if total <= 0 {
		return math.NaN()
	}
	n := 0
	for _, t := range selected {
		if t == "//..." {
			return 1
		}
		if t != "" && !strings.HasPrefix(t, "job:") {
			n++
		}
	}
	return math.Min(1, float64(n)/float64(total))
}

func EvalT1(ratio, ra1 float64) Verdict {
	if math.IsNaN(ratio) || math.IsNaN(ra1) {
		return Verdict{"T1", false, "UNMEASURED (cannot be evaluated, counted as FAIL)"}
	}
	lim := math.Min(t1Cap, t1RA1Factor*ra1)
	return Verdict{"T1", ratio <= lim, fmt.Sprintf("affected ratio %.3f vs target %.3f", ratio, lim)}
}

func EvalT3(second, first float64) Verdict {
	if math.IsNaN(second) || math.IsNaN(first) {
		return Verdict{"T3", false, "UNMEASURED (cannot be evaluated, counted as FAIL)"}
	}
	return Verdict{"T3", second <= t3SecondOfFirst*first, fmt.Sprintf("second %.0f s vs 0.25 x first = %.0f s", second, t3SecondOfFirst*first)}
}

func EvalT4(edit, base float64) Verdict {
	if math.IsNaN(edit) || math.IsNaN(base) {
		return Verdict{"T4", false, "UNMEASURED (cannot be evaluated, counted as FAIL)"}
	}
	lim := math.Max(t4Factor*base, base+t4SlackS)
	return Verdict{"T4", edit <= lim, fmt.Sprintf("latency %.1f s vs limit %.1f s", edit, lim)}
}

// NameStatus renders `git diff --name-status` lines: M for paths that exist in the current tree, D otherwise.
func NameStatus(paths []string, exists func(string) bool) string {
	var b strings.Builder
	for _, p := range paths {
		st := "D"
		if exists(p) {
			st = "M"
		}
		b.WriteString(st + "\t" + p + "\n")
	}
	return b.String()
}

// LoadAvg1 parses the first field of /proc/loadavg content.
func LoadAvg1(s string) (float64, error) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, fmt.Errorf("empty loadavg")
	}
	return strconv.ParseFloat(f[0], 64)
}

func IsQuiet(load float64) bool { return load <= quietLoad1 }

// MergeInputs sets keys in the JSON object at path (creating it) and writes it back, keeping other keys.
func MergeInputs(path string, kv map[string]any) error {
	cur := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &cur); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	for k, v := range kv {
		cur[k] = v
	}
	out, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// Num reads a finite number from a decoded JSON object, NaN when absent or not a number.
func Num(m map[string]any, k string) float64 {
	if v, ok := m[k].(float64); ok {
		return v
	}
	return math.NaN()
}

// noisyEnv is the owner-authorised override of the quiet-host wait; the default stays "wait for load <= 2.0".
const noisyEnv = "BZL_ALLOW_NOISY_HOST"

// AllowNoisy reports whether the loud override is on (flag or BZL_ALLOW_NOISY_HOST=1).
func AllowNoisy(flag bool, env string) bool { return flag || env == "1" }

// IsNoisy is true when either end of a run saw a 1-minute load above the quiet limit (NaN counts as noisy).
func IsNoisy(pre, post float64) bool {
	return math.IsNaN(pre) || math.IsNaN(post) || pre > quietLoad1 || post > quietLoad1
}

// MergeNoisy merges kv into the inputs JSON and records (noisy) or clears (quiet) noisy_inputs[key] for each key in kv.
func MergeNoisy(path string, kv map[string]any, noisy bool, entry map[string]any) error {
	cur := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &cur); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	side, _ := cur["noisy_inputs"].(map[string]any)
	if side == nil {
		side = map[string]any{}
	}
	for k, v := range kv {
		cur[k] = v
		if noisy {
			side[k] = entry
		} else {
			delete(side, k)
		}
	}
	if len(side) > 0 {
		cur["noisy_inputs"] = side
	} else {
		delete(cur, "noisy_inputs")
	}
	out, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// NoisyTag is " [NOISY]" when key is listed under noisy_inputs in the decoded inputs.
func NoisyTag(inputs map[string]any, key string) string {
	if side, ok := inputs["noisy_inputs"].(map[string]any); ok && side[key] != nil {
		return " [NOISY]"
	}
	return ""
}
