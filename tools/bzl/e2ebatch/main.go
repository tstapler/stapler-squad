// Command e2ebatch drives e2ecapture over every spec, one at a time, resume-ably (spike S14, Task 0.15a).
// It pauses while the host is over the load or memory gate, reruns a failing spec once alone, and keeps
// progress.json current so an interrupted run continues where it stopped.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Attempt is one capture invocation of one spec.
type Attempt struct {
	Exit       int     `json:"exit"`
	WallSec    float64 `json:"wall_seconds"`
	LoadAtRun  float64 `json:"load_at_start"`
	MemAvailGB float64 `json:"mem_available_gb_at_start"`
	TimedOut   bool    `json:"timed_out,omitempty"`
	Note       string  `json:"note,omitempty"`
}

// Record is a spec's state: done (captured, passing), failed (both attempts failed), excluded (not capturable).
type Record struct {
	State    string    `json:"state"`
	Excluded string    `json:"excluded,omitempty"`
	Attempts []Attempt `json:"attempts"`
}

// Progress is progress.json.
type Progress struct {
	Specs map[string]*Record `json:"specs"`
}

func readLoad(proc string) float64 {
	b, err := os.ReadFile(proc)
	if err != nil {
		return 0
	}
	f, _ := strconv.ParseFloat(strings.Fields(string(b))[0], 64)
	return f
}

func readMemGB(proc string) float64 {
	b, err := os.ReadFile(proc)
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "MemAvailable:") {
			kb, _ := strconv.ParseFloat(strings.Fields(l)[1], 64)
			return kb / 1048576
		}
	}
	return 0
}

// gateOK reports whether the host is within the load and memory limits.
func gateOK(load, memGB, maxLoad, minMemGB float64) bool { return load <= maxLoad && memGB >= minMemGB }

// classify turns the runner output of an attempt into excluded (no test in the chromium project) or ok/failed.
func classify(exit int, output string) (excluded string, ok bool) {
	if exit != 0 && strings.Contains(output, "No tests found") {
		return "no test matches the chromium project (runs only in another Playwright project: visual, mobile, perf or secondary browser)", false
	}
	return "", exit == 0
}

func save(path string, p *Progress) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func main() {
	root := flag.String("root", ".", "repository root")
	captureBin := flag.String("capture", "", "e2ecapture binary")
	serverBin := flag.String("binary", "", "cover-instrumented server binary")
	scratch := flag.String("scratch", "", "scratch dir on disk (never /tmp)")
	progress := flag.String("progress", "", "progress.json path")
	capsDir := flag.String("caps", "", "directory for per-spec capture JSON")
	maxLoad := flag.Float64("max-load", 24, "pause while 1-minute load is above this")
	minMem := flag.Float64("min-mem-gb", 12, "pause while MemAvailable (GB) is below this")
	timeout := flag.Duration("timeout", 15*time.Minute, "per-spec timeout")
	limit := flag.Int("limit", 0, "stop after this many specs this invocation (0 = all)")
	only := flag.String("only", "", "comma-separated spec basenames to (re)run")
	redo := flag.Bool("redo-failed", false, "rerun once, alone, every spec whose capture JSON records a failing Playwright run")
	flag.Parse()
	if *captureBin == "" || *serverBin == "" || *scratch == "" || *progress == "" || *capsDir == "" ||
		strings.HasPrefix(*scratch, "/tmp") || strings.HasPrefix(*scratch, "/dev/shm") {
		fmt.Fprintln(os.Stderr, "usage: e2ebatch -capture BIN -binary SERVER -scratch DIR -progress FILE -caps DIR (scratch must not be /tmp)")
		os.Exit(2)
	}
	specs, _ := filepath.Glob(filepath.Join(*root, "tests/e2e/*.spec.ts"))
	prog := &Progress{Specs: map[string]*Record{}}
	if b, err := os.ReadFile(*progress); err == nil {
		_ = json.Unmarshal(b, prog)
	}
	onlySet := map[string]bool{}
	for _, o := range strings.Split(*only, ",") {
		if o != "" {
			onlySet[o] = true
		}
	}
	ran := 0
	for _, abs := range specs {
		rel, _ := filepath.Rel(*root, abs)
		rel = filepath.ToSlash(rel)
		if len(onlySet) > 0 && !onlySet[filepath.Base(rel)] {
			continue
		}
		prev := prog.Specs[rel]
		if *redo {
			if prev == nil || prev.State == "excluded" || playwrightExit(filepath.Join(*capsDir, capName(rel))) == 0 {
				continue
			}
		} else if prev != nil && len(onlySet) == 0 && prev.State != "" {
			continue
		}
		if *limit > 0 && ran >= *limit {
			break
		}
		rec := &Record{}
		maxAttempts := 2
		if *redo {
			rec = prev
			maxAttempts = 1 // the first-pass failure is already recorded; this is the single rerun alone
			rec.Attempts[len(rec.Attempts)-1].Exit = playwrightExit(filepath.Join(*capsDir, capName(rel)))
			rec.Attempts[len(rec.Attempts)-1].Note = "first pass: playwright reported failures (wrapper exit 0); " + rec.Attempts[len(rec.Attempts)-1].Note
		}
		prog.Specs[rel] = rec
		for attempt := 0; attempt < maxAttempts; attempt++ {
			for !gateOK(readLoad("/proc/loadavg"), readMemGB("/proc/meminfo"), *maxLoad, *minMem) {
				time.Sleep(20 * time.Second)
			}
			a := Attempt{LoadAtRun: readLoad("/proc/loadavg"), MemAvailGB: readMemGB("/proc/meminfo")}
			out := filepath.Join(*capsDir, capName(rel))
			var buf bytes.Buffer
			//nolint:norawexec short-lived run-to-completion spike driver (own timeout/kill)
			cmd := exec.Command("nice", "-n", "19", "ionice", "-c3", *captureBin, "-spec", rel, "-binary", *serverBin,
				"-root", *root, "-tmp", *scratch, "-out", out)
			cmd.Stdout, cmd.Stderr = &buf, &buf
			start := time.Now()
			if err := cmd.Start(); err != nil {
				a.Exit, a.Note = 1, err.Error()
			} else {
				done := make(chan error, 1)
				go func() { done <- cmd.Wait() }()
				select {
				case err := <-done:
					a.Exit = exitCode(err)
				case <-time.After(*timeout):
					_ = cmd.Process.Kill()
					<-done
					a.Exit, a.TimedOut = 1, true
				}
			}
			a.WallSec = time.Since(start).Seconds()
			text := buf.String()
			if len(text) > 400 {
				a.Note = a.Note + text[len(text)-400:]
			}
			if pw := playwrightExit(out); a.Exit == 0 && pw != 0 { // e2ecapture exits 0 whatever Playwright did
				a.Exit = pw
			}
			rec.Attempts = append(rec.Attempts, a)
			if ex, _ := classify(a.Exit, text); ex != "" {
				rec.State, rec.Excluded = "excluded", ex
				break
			}
			if _, ok := classify(a.Exit, text); ok {
				rec.State = "done"
				break
			}
			rec.State = "failed"
		}
		_ = save(*progress, prog)
		ran++
	}
}

func capName(spec string) string {
	return strings.TrimSuffix(filepath.Base(spec), ".spec.ts") + ".json"
}

// playwrightExit reads playwright_exit from a capture JSON; a missing or unreadable file counts as failed.
func playwrightExit(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 1
	}
	var c struct {
		Exit int `json:"playwright_exit"`
	}
	if json.Unmarshal(b, &c) != nil {
		return 1
	}
	return c.Exit
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}
