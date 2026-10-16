package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunThenParseThenSummarize_ShouldRoundTrip(t *testing.T) {
	t.Parallel()
	loads := []float64{10, 30, 20, 40}
	cpu := []float64{100, 160, 220, 400}
	i := -1
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	s := &sampler{
		now:    func() time.Time { i++; return base.Add(time.Duration(i) * time.Minute) },
		loadFn: func(context.Context) (float64, float64, float64, error) { return loads[i], 0, 0, nil },
		cpuFn:  func(context.Context) (float64, float64, error) { return cpu[i] * 0.75, cpu[i] * 0.25, nil },
	}
	ticks := make(chan time.Time, len(loads)-1)
	for range loads[1:] {
		ticks <- time.Time{}
	}
	close(ticks)

	var buf, errBuf bytes.Buffer
	if err := s.run(context.Background(), ticks, &buf, &errBuf); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "# DIAGNOSTIC") {
		t.Fatalf("output must start with the DIAGNOSTIC banner, got %q", buf.String()[:20])
	}
	got, err := parseCSV(&buf)
	if err != nil {
		t.Fatalf("parseCSV: %v", err)
	}
	sum, err := summarize(got)
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if sum.Samples != 4 || sum.Window != 3*time.Minute {
		t.Fatalf("samples/window = %d/%s, want 4/3m", sum.Samples, sum.Window)
	}
	if sum.LoadMin != 10 || sum.LoadMax != 40 || sum.LoadMedian != 25 {
		t.Fatalf("load min/median/max = %v/%v/%v, want 10/25/40", sum.LoadMin, sum.LoadMedian, sum.LoadMax)
	}
	if sum.CPUSeconds != 300 || sum.CPUSecondsPerHour != 6000 {
		t.Fatalf("cpu = %v s, %v s/h; want 300, 6000", sum.CPUSeconds, sum.CPUSecondsPerHour)
	}
}

func TestRun_ShouldSkipFailedSampleAndKeepGoing(t *testing.T) {
	t.Parallel()
	calls := 0
	s := &sampler{
		now:    func() time.Time { return time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC) },
		loadFn: func(context.Context) (float64, float64, float64, error) { return 1, 1, 1, nil },
		cpuFn: func(context.Context) (float64, float64, error) {
			calls++
			if calls == 2 {
				return 0, 0, errors.New("process gone")
			}
			return 1, 1, nil
		},
	}
	ticks := make(chan time.Time, 2)
	ticks <- time.Time{}
	ticks <- time.Time{}
	close(ticks)
	var buf, errBuf bytes.Buffer
	if err := s.run(context.Background(), ticks, &buf, &errBuf); err != nil {
		t.Fatalf("run: %v", err)
	}
	rows, err := parseCSV(&buf)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%d err=%v, want 2 rows (one skipped)", len(rows), err)
	}
	if !strings.Contains(errBuf.String(), "process gone") {
		t.Fatalf("skip not reported on stderr: %q", errBuf.String())
	}
}

func TestParseCSV_ShouldRejectBadInput(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"no header":  "2026-10-15T00:00:00Z,1,1,1,1,1,2\n",
		"short row":  strings.Join(csvHeader, ",") + "\n2026-10-15T00:00:00Z,1,1\n",
		"bad number": strings.Join(csvHeader, ",") + "\n2026-10-15T00:00:00Z,x,1,1,1,1,2\n",
		"bad time":   strings.Join(csvHeader, ",") + "\nyesterday,1,1,1,1,1,2\n",
	}
	for name, in := range cases {
		if _, err := parseCSV(strings.NewReader(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := summarize(nil); err == nil {
		t.Error("summarize(nil) should error")
	}
}

func TestNewSampler_ShouldReadLiveSelf(t *testing.T) {
	t.Parallel()
	s, err := newSampler(int32(os.Getpid()))
	if err != nil {
		t.Fatalf("newSampler(self): %v", err)
	}
	sm, err := s.sample(context.Background())
	if err != nil {
		t.Fatalf("sample(self): %v", err)
	}
	if sm.CPUTotal() <= 0 || sm.Load1 < 0 {
		t.Fatalf("implausible self sample: cpu=%v load1=%v", sm.CPUTotal(), sm.Load1)
	}
}
