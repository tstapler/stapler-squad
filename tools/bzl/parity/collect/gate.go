// Package collect holds what every parity/improvement collector shares: the quiet-host gate, medians and the
// measurements document the improvement evaluator reads.
//
// A timing collector never emits a verdict number from a noisy host: when the 1-minute load average is above
// QuietMaxLoad before or after a sample, the row is recorded as UNMEASURED with the reason "NOT QUIET".
package collect

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// QuietMaxLoad is the plan's quiet-host bound on the 1-minute load average.
const QuietMaxLoad = 2.0

// Host reports the current 1-minute load average; fakes make the gate testable.
type Host interface{ Load1() (float64, error) }

// ProcHost reads /proc/loadavg.
type ProcHost struct{}

// Load1 implements Host.
func (ProcHost) Load1() (float64, error) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}
	return ParseLoad1(string(b))
}

// ParseLoad1 extracts the first field of /proc/loadavg.
func ParseLoad1(s string) (float64, error) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, fmt.Errorf("empty loadavg")
	}
	return strconv.ParseFloat(f[0], 64)
}

// Reading is the load recorded around one timed sample.
type Reading struct {
	Pre   float64 `json:"load_pre"`
	Post  float64 `json:"load_post"`
	Quiet bool    `json:"quiet"`
}

// Gate measures load around samples. Require (the default of every collector) means a NOT QUIET series yields no number.
type Gate struct {
	Host    Host
	Require bool
	Max     float64
}

// NewGate returns a gate with the plan bound.
func NewGate(h Host, require bool) Gate { return Gate{Host: h, Require: require, Max: QuietMaxLoad} }

func (g Gate) load() float64 {
	l, err := g.Host.Load1()
	if err != nil {
		return math.Inf(1) // an unreadable load is not evidence of quiet
	}
	return l
}

// Begin records the load before a sample.
func (g Gate) Begin() Reading { return Reading{Pre: g.load()} }

// End records the load after a sample and decides Quiet.
func (g Gate) End(r Reading) Reading {
	r.Post = g.load()
	r.Quiet = r.Pre <= g.Max && r.Post <= g.Max
	return r
}

// PreOK reports whether a timed run may start at all (load before is within bounds).
func (g Gate) PreOK() bool { return g.load() <= g.Max }

// AllQuiet is true when every reading is quiet and there is at least one.
func AllQuiet(rs []Reading) bool {
	for _, r := range rs {
		if !r.Quiet {
			return false
		}
	}
	return len(rs) > 0
}

// MaxLoad returns the highest load seen across readings.
func MaxLoad(rs []Reading) float64 {
	m := 0.0
	for _, r := range rs {
		m = math.Max(m, math.Max(r.Pre, r.Post))
	}
	return m
}

// NotQuietReason renders the recorded reason for an UNMEASURED row.
func NotQuietReason(rs []Reading, max float64) string {
	return fmt.Sprintf("NOT QUIET: 1-minute load reached %.2f (limit %.1f) before or after a sample", MaxLoad(rs), max)
}

// Median of xs, NaN when empty.
func Median(xs []float64) float64 { return Percentile(xs, 0.5) }

// Percentile is the nearest-rank percentile used by ci-time-to-green.sh (ceil(p*n)-th smallest) for p != 0.5,
// and the usual midpoint median for p == 0.5.
func Percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if p == 0.5 {
		if n%2 == 1 {
			return s[n/2]
		}
		return (s[n/2-1] + s[n/2]) / 2
	}
	i := int(math.Ceil(p*float64(n))) - 1
	if i < 0 {
		i = 0
	}
	return s[i]
}
