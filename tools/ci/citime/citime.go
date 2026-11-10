package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Entry is one timed unit: a make target or a Go package.
type Entry struct {
	Name    string  `json:"name"`
	Seconds float64 `json:"seconds"`
}

// Report is the machine-readable result; targets are in execution order, packages sorted slowest first.
type Report struct {
	TotalSeconds float64 `json:"total_seconds"`
	Targets      []Entry `json:"targets"`
	Packages     []Entry `json:"packages"`
}

var (
	// make --trace: "Makefile:709: update target 'test' due to: target is .PHONY"
	targetRe = regexp.MustCompile(`^\S+:\d+: update target '([^']+)'`)
	// gotestsum --format pkgname-and-test-fails: "✓  session (12.3s)", "✓  x (3ms)", "✓  y (1m2s)"; "(cached)" has no time
	pkgRe = regexp.MustCompile(`^[✓✖∅]\s+(\S+)\s+\(([0-9.]+(?:h|m|s|ms|µs)[0-9.hmsµ]*)\)`)
)

// scan reads in and times targets and packages; it never writes the log (tee does), so a broken stdout cannot
// lose build output. A target runs until the next target starts, so a target's time includes its recipe and
// any prerequisites it triggers after its own trace line. Targets under minTarget seconds (tmux's per-object
// sub-make) are folded into one row.
func scan(in io.Reader, now func() time.Time) (Report, error) {
	var rep Report
	start := now()
	var cur string
	curStart := start
	closeCur := func(t time.Time) {
		if cur != "" {
			rep.Targets = append(rep.Targets, Entry{cur, t.Sub(curStart).Seconds()})
		}
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		line := sc.Text()
		if m := targetRe.FindStringSubmatch(line); m != nil {
			t := now()
			closeCur(t)
			cur, curStart = m[1], t
		} else if m := pkgRe.FindStringSubmatch(line); m != nil {
			if d, perr := time.ParseDuration(m[2]); perr == nil {
				rep.Packages = append(rep.Packages, Entry{m[1], d.Seconds()})
			}
		}
	}
	end := now()
	closeCur(end)
	rep.TotalSeconds = end.Sub(start).Seconds()
	rep.Targets = foldShort(rep.Targets)
	sort.SliceStable(rep.Packages, func(i, j int) bool { return rep.Packages[i].Seconds > rep.Packages[j].Seconds })
	return rep, sc.Err()
}

const minTarget = 1.0

func foldShort(es []Entry) []Entry {
	var out []Entry
	var n int
	var sum float64
	for _, e := range es {
		if e.Seconds < minTarget {
			n++
			sum += e.Seconds
			continue
		}
		out = append(out, e)
	}
	if n > 0 {
		out = append(out, Entry{fmt.Sprintf("(%d targets under %.0fs)", n, minTarget), sum})
	}
	return out
}

func (r Report) markdown(top int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### make timing (total %s)\n\n", dur(r.TotalSeconds))
	b.WriteString(table("Target (slowest first)", slowest(r.Targets, top), r.TotalSeconds))
	if len(r.Packages) > 0 {
		b.WriteString(table("Go package (slowest first)", slowest(r.Packages, top), r.TotalSeconds))
	}
	return b.String()
}

func slowest(es []Entry, n int) []Entry {
	c := append([]Entry(nil), es...)
	sort.SliceStable(c, func(i, j int) bool { return c[i].Seconds > c[j].Seconds })
	if len(c) > n {
		c = c[:n]
	}
	return c
}

func table(head string, es []Entry, total float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| %s | Time | %% of run |\n|---|---:|---:|\n", head)
	for _, e := range es {
		pct := 0.0
		if total > 0 {
			pct = 100 * e.Seconds / total
		}
		fmt.Fprintf(&b, "| `%s` | %s | %.1f%% |\n", e.Name, dur(e.Seconds), pct)
	}
	b.WriteString("\n")
	return b.String()
}

func dur(s float64) string {
	if s < 60 {
		return fmt.Sprintf("%.1fs", s)
	}
	return fmt.Sprintf("%dm%02ds", int(s)/60, int(s)%60)
}

func writeJSON(path string, r Report) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func appendFile(path, s string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(s)
	return err
}
