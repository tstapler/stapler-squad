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
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	pkgRe  = regexp.MustCompile(`^[✓✖∅]\s+(\S+)\s+\(([0-9.]+(?:h|m|s|ms|µs)[0-9.hmsµ]*)\)`)
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
		line := ansiRe.ReplaceAllString(sc.Text(), "") // CI gotestsum colours the status mark
		if m := targetRe.FindStringSubmatch(line); m != nil {
			t := now()
			closeCur(t)
			cur, curStart = m[1], t
		} else if m := pkgRe.FindStringSubmatch(line); m != nil {
			if d, perr := time.ParseDuration(m[2]); perr == nil {
				rep.Packages = append(rep.Packages, Entry{cur + ": " + m[1], d.Seconds()}) // same package runs under test, test-race and test-integration
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
	b.WriteString(table("Target (slowest first)", slowest(r.Targets, top), r.TotalSeconds, true))
	if len(r.Packages) > 0 {
		b.WriteString(table("Go package, parallel so times overlap (slowest first)", slowest(r.Packages, top), r.TotalSeconds, false))
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

func table(head string, es []Entry, total float64, pct bool) string {
	var b strings.Builder
	if pct {
		fmt.Fprintf(&b, "| %s | Time | %% of run |\n|---|---:|---:|\n", head)
	} else {
		fmt.Fprintf(&b, "| %s | Time |\n|---|---:|\n", head)
	}
	for _, e := range es {
		if !pct {
			fmt.Fprintf(&b, "| `%s` | %s |\n", e.Name, dur(e.Seconds))
			continue
		}
		share := 0.0
		if total > 0 {
			share = 100 * e.Seconds / total
		}
		fmt.Fprintf(&b, "| `%s` | %s | %.1f%% |\n", e.Name, dur(e.Seconds), share)
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

// followReader reads a growing file and reports EOF only after the done file exists and a further read is empty,
// so citime can time a log that make writes to a regular file (a pipe shared with Bun/Node children can go
// non-blocking and make then fails with "write error: stdout").
type followReader struct {
	f    *os.File
	done string
}

func (r followReader) Read(p []byte) (int, error) {
	for {
		n, err := r.f.Read(p)
		if n > 0 {
			return n, nil
		}
		if err != nil && err != io.EOF {
			return 0, err
		}
		if _, statErr := os.Stat(r.done); statErr == nil {
			if m, _ := r.f.Read(p); m > 0 { // drain what landed between the EOF and the done marker
				return m, nil
			}
			return 0, io.EOF
		}
		time.Sleep(200 * time.Millisecond)
	}
}
