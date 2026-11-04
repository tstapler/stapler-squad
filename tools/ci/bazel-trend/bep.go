// Package main summarises Bazel Build Event Protocol (BEP) JSON files into the CI driver metrics
// (time_to_green_s, cache_hit_ratio, affected_target_count) plus cache-credential health (Task 7.1a).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// authFailure matches the E11 signatures of an expired or rejected cache credential.
var authFailure = regexp.MustCompile(`HTTP/?[0-9.]* ?(401|403)|\b(401|403)\b.*(Unauthorized|Forbidden)|UNAUTHENTICATED|PERMISSION_DENIED|HTTP (401|403)`)

// Summary is the per-job result; it sums every BEP file of the job (eviction retries re-run the build).
type Summary struct {
	TimeToGreenS        float64 `json:"time_to_green_s"`
	CacheHitRatio       float64 `json:"cache_hit_ratio"`
	AffectedTargetCount int     `json:"affected_target_count"`
	Attempts            int     `json:"attempts"`
	EvictionRetries     int     `json:"eviction_retries"`
	CacheHits           int64   `json:"cache_hits"`
	Spawns              int64   `json:"spawns"`
	AuthFailures        int     `json:"bazel_cache_auth_failures_total"`
}

type bepEvent struct {
	Progress *struct {
		Stderr string `json:"stderr"`
		Stdout string `json:"stdout"`
	} `json:"progress"`
	BuildMetrics *struct {
		ActionSummary struct {
			RunnerCount []struct {
				Name  string      `json:"name"`
				Count json.Number `json:"count"`
			} `json:"runnerCount"`
		} `json:"actionSummary"`
		TimingMetrics struct {
			WallTimeInMs json.Number `json:"wallTimeInMs"`
		} `json:"timingMetrics"`
	} `json:"buildMetrics"`
}

// ParseBEP adds one BEP file (newline-delimited JSON) to s. Unparseable lines are an error: a silently
// truncated file would understate failures.
func (s *Summary) ParseBEP(r io.Reader) error {
	s.Attempts++
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev bepEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return fmt.Errorf("BEP line %d: %w", n, err)
		}
		if ev.Progress != nil {
			s.AuthFailures += countAuth(ev.Progress.Stderr)
		}
		if m := ev.BuildMetrics; m != nil {
			ms, _ := strconv.ParseFloat(m.TimingMetrics.WallTimeInMs.String(), 64)
			s.TimeToGreenS += ms / 1000
			for _, rc := range m.ActionSummary.RunnerCount {
				c, _ := rc.Count.Int64()
				switch {
				case rc.Name == "remote cache hit" || rc.Name == "disk cache hit":
					s.CacheHits += c
					s.Spawns += c
				case rc.Name != "internal":
					s.Spawns += c
				}
			}
		}
	}
	return sc.Err()
}

// AddLog counts auth-failure lines of a command.log (or any text log).
func (s *Summary) AddLog(r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		s.AuthFailures += countAuth(sc.Text())
	}
	return sc.Err()
}

func countAuth(text string) int {
	n := 0
	for _, l := range strings.Split(text, "\n") {
		if authFailure.MatchString(l) {
			n++
		}
	}
	return n
}

// Finish derives the ratio and retry count after every file is added.
func (s *Summary) Finish() {
	if s.Spawns > 0 {
		s.CacheHitRatio = float64(s.CacheHits) / float64(s.Spawns)
	}
	if s.Attempts > 1 {
		s.EvictionRetries = s.Attempts - 1
	}
}

// Prom renders the metrics as "name value" lines plus a ::warning:: annotation on auth failures.
func (s *Summary) Prom() string {
	var b strings.Builder
	fmt.Fprintf(&b, "bazel_time_to_green_s %.1f\n", s.TimeToGreenS)
	fmt.Fprintf(&b, "bazel_cache_hit_ratio %.4f\n", s.CacheHitRatio)
	fmt.Fprintf(&b, "bazel_affected_target_count %d\n", s.AffectedTargetCount)
	fmt.Fprintf(&b, "bazel_eviction_retries %d\n", s.EvictionRetries)
	fmt.Fprintf(&b, "bazel_cache_auth_failures_total %d\n", s.AuthFailures)
	if s.AuthFailures > 0 {
		fmt.Fprintf(&b, "::warning title=Bazel cache auth::%d cache auth failures (HTTP 401/403, UNAUTHENTICATED or PERMISSION_DENIED); the cache credential may have expired\n", s.AuthFailures)
	}
	return b.String()
}

// Median returns the median of the last n values (all of them if fewer); ok is false when empty.
func Median(vals []float64, n int) (float64, bool) {
	if len(vals) == 0 {
		return 0, false
	}
	if n > 0 && len(vals) > n {
		vals = vals[len(vals)-n:]
	}
	c := append([]float64(nil), vals...)
	sort.Float64s(c)
	if len(c)%2 == 1 {
		return c[len(c)/2], true
	}
	return (c[len(c)/2-1] + c[len(c)/2]) / 2, true
}

// TrendRow is one line of the trend artifact (JSON lines).
type TrendRow struct {
	RunID string `json:"run_id"`
	Ref   string `json:"ref,omitempty"`
	Warm  bool   `json:"warm,omitempty"`
	Summary
}

// HitRatioFloor evaluates the P10 / Task 7.4c rule on warm `main` rows: median of the last n >= floor.
func HitRatioFloor(rows []TrendRow, n int, floor float64) (median float64, ok bool, enough bool) {
	var v []float64
	for _, r := range rows {
		if r.Warm && r.Ref == "refs/heads/main" {
			v = append(v, r.CacheHitRatio)
		}
	}
	m, has := Median(v, n)
	if !has {
		return 0, false, false
	}
	return m, m >= floor, len(v) >= n
}
