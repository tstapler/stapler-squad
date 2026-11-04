package collect

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
)

// CacheCount is spawns versus cache hits of one build (hit = remote cache hit or disk cache hit, Task 0.10c).
type CacheCount struct{ Hits, Spawns int64 }

// Ratio is Hits/Spawns, NaN-free: ok is false when there were no spawns.
func (c CacheCount) Ratio() (float64, bool) {
	if c.Spawns == 0 {
		return 0, false
	}
	return float64(c.Hits) / float64(c.Spawns), true
}

// ParseExecLog counts spawns in a `--execution_log_json_file` (one JSON object per line). A spawn is a hit when
// its runner is a cache runner or its cacheHit flag is set; "internal" runners are not spawns.
func ParseExecLog(r io.Reader) (CacheCount, error) {
	var c CacheCount
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var e struct {
			Runner   string `json:"runner"`
			CacheHit bool   `json:"cacheHit"`
		}
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return c, err
		}
		switch {
		case e.Runner == "internal":
		case e.CacheHit || e.Runner == "remote cache hit" || e.Runner == "disk cache hit":
			c.Hits++
			c.Spawns++
		default:
			c.Spawns++
		}
	}
	return c, sc.Err()
}

// TrendRow is the slice of a bazel-trend JSONL row used for the warm-main hit ratio.
type TrendRow struct {
	Ref           string  `json:"ref"`
	Warm          bool    `json:"warm"`
	CacheHitRatio float64 `json:"cache_hit_ratio"`
}

// WarmMainMedian is the median hit ratio of the last n warm `main` rows (T2b); ok is false with fewer than n rows.
func WarmMainMedian(path string, n int) (median float64, rows int, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false, err
	}
	defer f.Close()
	var v []float64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r TrendRow
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Warm && r.Ref == "refs/heads/main" {
			v = append(v, r.CacheHitRatio)
		}
	}
	rows = len(v)
	if len(v) > n {
		v = v[len(v)-n:]
	}
	return Median(v), rows, rows >= n, sc.Err()
}

// BEPSummary is the JSON the bazel-trend tool prints for a job.
type BEPSummary struct {
	CacheHitRatio   float64 `json:"cache_hit_ratio"`
	CacheHits       int64   `json:"cache_hits"`
	Spawns          int64   `json:"spawns"`
	Attempts        int     `json:"attempts"`
	EvictionRetries int     `json:"eviction_retries"`
	TimeToGreenS    float64 `json:"time_to_green_s"`
}

// RunBazelTrend summarises BEP files with the built bazel-trend binary (reused, not reimplemented).
func RunBazelTrend(bin string, extra []string, bep ...string) (BEPSummary, error) {
	var s BEPSummary
	out, err := exec.Command(bin, append(append([]string(nil), extra...), bep...)...).Output() //nolint:norawexec // dev-time collector, waited on
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(out, &s)
}
