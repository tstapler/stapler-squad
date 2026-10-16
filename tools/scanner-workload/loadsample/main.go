// Command loadsample records host load average and one process's cumulative
// CPU-seconds at a fixed interval, as CSV, for the DIAGNOSTIC outcome capture
// of plan Tasks 0.1.1b, 5.2.1a and 5.2.2b (requirements.md "Outcome metric").
// Every output is labelled DIAGNOSTIC: it never passes or fails a story.
//
//	loadsample -pid 12345 [-interval 60s] [-duration 30m] [-out samples.csv]
//	loadsample -summarize samples.csv
//
// No subprocesses: load average comes from gopsutil's sysctl (macOS) or
// /proc/loadavg (Linux) reader, process CPU time from libproc (macOS) or
// /proc/<pid>/stat (Linux), both read in-process. macOS has no sysctl-only
// path for per-process CPU time (kinfo_proc's tick fields are all zero there,
// verified on Darwin 25.6), hence the gopsutil dependency the repo already has.
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	diagnosticBanner = "# DIAGNOSTIC: host load and process CPU samples; not a gate"
	timeLayout       = time.RFC3339
)

var csvHeader = []string{"timestamp_utc", "load1", "load5", "load15", "proc_cpu_user_s", "proc_cpu_sys_s", "proc_cpu_total_s"}

// sample is one row of the output.
type sample struct {
	At                   time.Time
	Load1, Load5, Load15 float64
	CPUUser, CPUSys      float64
}

func (s sample) CPUTotal() float64 { return s.CPUUser + s.CPUSys }

func (s sample) record() []string {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }
	return []string{s.At.UTC().Format(timeLayout), f(s.Load1), f(s.Load5), f(s.Load15), f(s.CPUUser), f(s.CPUSys), f(s.CPUTotal())}
}

// sampler reads the two sources; both are injectable so tests need no live process.
type sampler struct {
	loadFn func(context.Context) (l1, l5, l15 float64, err error)
	cpuFn  func(context.Context) (user, sys float64, err error)
	now    func() time.Time
}

func newSampler(pid int32) (*sampler, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("pid %d: %w", pid, err)
	}
	return &sampler{
		loadFn: func(ctx context.Context) (float64, float64, float64, error) {
			a, err := load.AvgWithContext(ctx)
			if err != nil {
				return 0, 0, 0, err
			}
			return a.Load1, a.Load5, a.Load15, nil
		},
		cpuFn: func(ctx context.Context) (float64, float64, error) {
			t, err := p.TimesWithContext(ctx)
			if err != nil {
				return 0, 0, err
			}
			return t.User, t.System, nil
		},
		now: time.Now,
	}, nil
}

func (s *sampler) sample(ctx context.Context) (sample, error) {
	out := sample{At: s.now()}
	var err error
	if out.Load1, out.Load5, out.Load15, err = s.loadFn(ctx); err != nil {
		return out, fmt.Errorf("load average: %w", err)
	}
	if out.CPUUser, out.CPUSys, err = s.cpuFn(ctx); err != nil {
		return out, fmt.Errorf("process cpu: %w", err)
	}
	return out, nil
}

// run writes the banner and header, then one row per tick (plus one
// immediately) until ctx ends or ticks closes. A failed sample is reported to
// stderr and skipped, so a dead target does not erase the load-only history.
func (s *sampler) run(ctx context.Context, ticks <-chan time.Time, w io.Writer, stderr io.Writer) error {
	if _, err := fmt.Fprintln(w, diagnosticBanner); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	emit := func() error {
		sm, err := s.sample(ctx)
		if err != nil {
			fmt.Fprintln(stderr, "loadsample: skipped sample:", err)
			return nil
		}
		if err := cw.Write(sm.record()); err != nil {
			return err
		}
		cw.Flush()
		return cw.Error()
	}
	if err := emit(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-ticks:
			if !ok {
				return nil
			}
			if err := emit(); err != nil {
				return err
			}
		}
	}
}

// summary is the DIAGNOSTIC rollup recorded in baseline.md.
type summary struct {
	Samples                      int
	Window                       time.Duration
	LoadMin, LoadMedian, LoadMax float64 // load1
	CPUSeconds                   float64 // last minus first proc_cpu_total_s
	CPUSecondsPerHour            float64
}

// parseCSV reads loadsample output (banner and '#' lines ignored).
func parseCSV(r io.Reader) ([]sample, error) {
	var kept strings.Builder
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		kept.WriteString(line + "\n")
	}
	rows, err := csv.NewReader(strings.NewReader(kept.String())).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || strings.Join(rows[0], ",") != strings.Join(csvHeader, ",") {
		return nil, errors.New("missing or unexpected CSV header")
	}
	var out []sample
	for i, row := range rows[1:] {
		if len(row) != len(csvHeader) {
			return nil, fmt.Errorf("row %d: %d fields, want %d", i+1, len(row), len(csvHeader))
		}
		at, err := time.Parse(timeLayout, row[0])
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		nums := make([]float64, 5)
		for j := range nums {
			if nums[j], err = strconv.ParseFloat(row[j+1], 64); err != nil {
				return nil, fmt.Errorf("row %d col %s: %w", i+1, csvHeader[j+1], err)
			}
		}
		out = append(out, sample{At: at, Load1: nums[0], Load5: nums[1], Load15: nums[2], CPUUser: nums[3], CPUSys: nums[4]})
	}
	return out, nil
}

func summarize(ss []sample) (summary, error) {
	if len(ss) == 0 {
		return summary{}, errors.New("no samples")
	}
	loads := make([]float64, len(ss))
	for i, s := range ss {
		loads[i] = s.Load1
	}
	sort.Float64s(loads)
	med := loads[len(loads)/2]
	if len(loads)%2 == 0 {
		med = (loads[len(loads)/2-1] + loads[len(loads)/2]) / 2
	}
	first, last := ss[0], ss[len(ss)-1]
	sum := summary{
		Samples: len(ss), Window: last.At.Sub(first.At),
		LoadMin: loads[0], LoadMedian: med, LoadMax: loads[len(loads)-1],
		CPUSeconds: last.CPUTotal() - first.CPUTotal(),
	}
	if h := sum.Window.Hours(); h > 0 {
		sum.CPUSecondsPerHour = sum.CPUSeconds / h
	}
	return sum, nil
}

func (s summary) String() string {
	return fmt.Sprintf("DIAGNOSTIC samples=%d window=%s load1_min=%.2f load1_median=%.2f load1_max=%.2f proc_cpu_seconds=%.1f proc_cpu_seconds_per_hour=%.1f",
		s.Samples, s.Window.Round(time.Second), s.LoadMin, s.LoadMedian, s.LoadMax, s.CPUSeconds, s.CPUSecondsPerHour)
}

func main() {
	if err := runMain(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "loadsample:", err)
		os.Exit(1)
	}
}

func runMain(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("loadsample", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pid := fs.Int("pid", 0, "target process id (the instance under test)")
	interval := fs.Duration("interval", 60*time.Second, "sampling interval")
	duration := fs.Duration("duration", 0, "stop after this long (0 = until interrupted)")
	out := fs.String("out", "", "CSV output file (default stdout)")
	sumPath := fs.String("summarize", "", "print the DIAGNOSTIC summary of an existing CSV and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *sumPath != "" {
		f, err := os.Open(*sumPath)
		if err != nil {
			return err
		}
		defer f.Close()
		ss, err := parseCSV(f)
		if err != nil {
			return err
		}
		sm, err := summarize(ss)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, sm)
		return nil
	}
	if *pid <= 0 || *interval <= 0 {
		return errors.New("-pid and a positive -interval are required")
	}
	s, err := newSampler(int32(*pid))
	if err != nil {
		return err
	}
	w := stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}
	t := time.NewTicker(*interval)
	defer t.Stop()
	return s.run(ctx, t.C, w, stderr)
}
