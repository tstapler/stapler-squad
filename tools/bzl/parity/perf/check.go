package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/collect"
)

// P10 non-regression bounds (plan Story 7.5, row P10).
const (
	maxWarmNoopS  = 10.0
	maxColdRatio  = 0.30
	maxTTGRatio   = 1.10
	maxDiskRatio  = 1.25
	minTTGSamples = 20
)

// Exit codes of `perf check`.
const (
	exitPass       = 0
	exitFail       = 1
	exitUnmeasured = 4
)

type perfFile struct {
	vals map[string]float64
	meta map[string]collect.Meta
}

func loadPerf(path string) (perfFile, error) {
	pf := perfFile{vals: map[string]float64{}, meta: map[string]collect.Meta{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		return pf, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return pf, err
	}
	for k, v := range all {
		var f float64
		if k == "_meta" {
			_ = json.Unmarshal(v, &pf.meta)
		} else if json.Unmarshal(v, &f) == nil {
			pf.vals[k] = f
		}
	}
	return pf, nil
}

// get returns a key's value; a row recorded NOT QUIET (or unmeasured) is never usable even if a stale number exists.
func (p perfFile) get(key string) (float64, string) {
	v, ok := p.vals[key]
	m := p.meta[key]
	switch {
	case !ok:
		if m.Reason != "" {
			return 0, key + ": " + m.Reason
		}
		return 0, key + ": not collected"
	case m.Quiet != nil && !*m.Quiet:
		return 0, key + ": NOT QUIET"
	}
	return v, ""
}

type bound struct {
	name, key string
	max       float64
}

var checks = map[string][]bound{
	"warm": {{"warm no-op build <= 10 s", "warm_noop_s", maxWarmNoopS}},
	"cold": {{"second-worktree cold <= 30% of make", "second_cold_ratio_make", maxColdRatio}},
	"ttg":  {{"time-to-green p50 <= 1.10 x baseline", "ttg_ratio", maxTTGRatio}, {"per-workflow p50 <= 1.10 x baseline", "ttg_wf_ratio_max", maxTTGRatio}},
	"disk": {{"output_base disk <= 1.25 x baseline", "disk_ratio", maxDiskRatio}},
}

// Check evaluates the named P10 sub-checks ("all" = every one). UNMEASURED beats PASS but FAIL beats UNMEASURED.
func Check(p perfFile, names []string, out io.Writer) int {
	if len(names) == 1 && names[0] == "all" {
		names = []string{"warm", "cold", "ttg", "disk"}
	}
	code := exitPass
	for _, n := range names {
		bs, ok := checks[n]
		if !ok {
			fmt.Fprintf(out, "unknown check %q\n", n)
			return 2
		}
		if n == "ttg" {
			if v, why := p.get("ttg_n"); why != "" || v < minTTGSamples {
				fmt.Fprintf(out, "%s: UNMEASURED fewer than %d qualifying PR runs (%s)\n", n, minTTGSamples, strings.TrimSpace(why))
				code = worstCode(code, exitUnmeasured)
				continue
			}
		}
		for _, b := range bs {
			v, why := p.get(b.key)
			switch {
			case why != "":
				fmt.Fprintf(out, "%s: UNMEASURED %s\n", b.name, why)
				code = worstCode(code, exitUnmeasured)
			case v > b.max:
				fmt.Fprintf(out, "%s: FAIL %.3f > %.3f\n", b.name, v, b.max)
				code = worstCode(code, exitFail)
			default:
				fmt.Fprintf(out, "%s: PASS %.3f\n", b.name, v)
			}
		}
	}
	return code
}

func worstCode(a, b int) int {
	if a == exitFail || b == exitFail {
		return exitFail
	}
	if a == exitUnmeasured || b == exitUnmeasured {
		return exitUnmeasured
	}
	return exitPass
}
