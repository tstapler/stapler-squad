// Package slowops records slow terminal backend operations: a still-running goroutine dump
// (Recorder), a per-session slow-call counter and span (ReportIfSlow), and a hang watchdog (Watch).
// It imports no session code; callers pass the backend as a plain string.
package slowops

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// A terminal operation (capture_pane, resize, write_stdin) that is still running after
// slowOpThreshold gets its blocked goroutines logged while it is still blocked: the duration
// histogram only says it was slow after the fact, never what it waited on.

// EnvSlowOpMs overrides the threshold in milliseconds.
const EnvSlowOpMs = "STAPLER_SQUAD_SLOW_OP_MS"

const (
	defaultSlowOpThreshold = 2 * time.Second
	slowOpReportEvery      = 30 * time.Second // at most one goroutine dump per window
	slowOpMaxGoroutines    = 12
	slowOpMaxStackBytes    = 2048
)

// DumpReport is what a firing watch hands its sink.
type DumpReport struct {
	Op      string
	Backend string
	Stacks  string
}

// Recorder arms a timer per watched operation; the timer's cost on a healthy call is one
// allocation and a Stop.
type Recorder struct {
	threshold time.Duration
	now       func() time.Time
	sink      func(DumpReport)
	lastNs    atomic.Int64
	counter   metric.Int64Counter
	once      sync.Once
}

// NewRecorder returns a Recorder that hands at most one DumpReport per rate-limit window to sink.
func NewRecorder(threshold time.Duration, sink func(DumpReport)) *Recorder {
	return &Recorder{threshold: threshold, now: time.Now, sink: sink}
}

// Watch starts watching one operation and returns the function that ends the watch.
func (r *Recorder) Watch(op, backend string) (stop func()) {
	t := time.AfterFunc(r.threshold, func() { r.fire(op, backend) })
	return func() { t.Stop() }
}

func (r *Recorder) fire(op, backend string) {
	r.once.Do(func() {
		r.counter, _ = telemetry.GetMeter().Int64Counter("terminal_slow_op_total",
			metric.WithDescription("Terminal backend operations still running after the slow-op threshold, by operation and backend"))
	})
	if r.counter != nil {
		r.counter.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("operation", op), attribute.String("backend", backend)))
	}
	nowNs := r.now().UnixNano()
	last := r.lastNs.Load()
	if last != 0 && nowNs-last < int64(slowOpReportEvery) || !r.lastNs.CompareAndSwap(last, nowNs) {
		return
	}
	r.sink(DumpReport{Op: op, Backend: backend, Stacks: blockedStacks()})
}

// blockedStacks returns the goroutines running this module's code, minus the recorder itself,
// ranked before capping so the blocked op's goroutine survives a busy process.
func blockedStacks() string {
	var buf bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&buf, 2); err != nil {
		return err.Error()
	}
	return rankBlockedStacks(buf.String())
}

// slowOpFrames mark a goroutine inside a timed backend op; those always outrank the rest.
var slowOpFrames = []string{"timeBackendOp", "timedBackendOp"} //nolint:gochecknoglobals // constant list

// rankBlockedStacks keeps module goroutines from a debug=2 dump: timed-op goroutines first,
// then the longest-waiting, capped in count and size so a dump stays a log line.
func rankBlockedStacks(dump string) string {
	type ranked struct {
		stack string
		inOp  bool
		wait  int // minutes from the "[chan receive, 5 minutes]" header; 0 when unstated
	}
	var gs []ranked
	for _, g := range strings.Split(dump, "\n\n") {
		if !strings.Contains(g, "stapler-squad/") || strings.Contains(g, "slowops.(*Recorder)") {
			continue
		}
		r := ranked{stack: g, wait: goroutineWaitMinutes(g)}
		for _, f := range slowOpFrames {
			if strings.Contains(g, f) {
				r.inOp = true
				break
			}
		}
		gs = append(gs, r)
	}
	sort.SliceStable(gs, func(i, j int) bool {
		if gs[i].inOp != gs[j].inOp {
			return gs[i].inOp
		}
		return gs[i].wait > gs[j].wait
	})
	if len(gs) > slowOpMaxGoroutines {
		gs = gs[:slowOpMaxGoroutines]
	}
	out := make([]string, len(gs))
	for i, r := range gs {
		out[i] = r.stack
		if len(out[i]) > slowOpMaxStackBytes {
			out[i] = out[i][:slowOpMaxStackBytes] + "...(truncated)"
		}
	}
	return strings.Join(out, "\n\n")
}

var goroutineWaitRe = regexp.MustCompile(`^goroutine \d+ \[[^\]]*?, (\d+) minutes`)

func goroutineWaitMinutes(g string) int {
	m := goroutineWaitRe.FindStringSubmatch(strings.TrimLeft(g, "\n"))
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func logSlowOp(r DumpReport) {
	log.Warn("terminal backend operation still running past the slow-op threshold; goroutines in this module follow",
		"operation", r.Op, "backend", r.Backend, "stacks", r.Stacks)
}

func slowOpThresholdFromEnv() time.Duration { return thresholdFromMs(os.Getenv(EnvSlowOpMs)) }

func thresholdFromMs(v string) time.Duration {
	if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return defaultSlowOpThreshold
}

// terminalSlowOps is the process-wide recorder behind WatchTerminalOp.
var terminalSlowOps = NewRecorder(slowOpThresholdFromEnv(), logSlowOp) //nolint:gochecknoglobals // singleton rate limit across all backends

// WatchTerminalOp watches one terminal backend operation on the process-wide recorder and
// returns the function that ends the watch.
func WatchTerminalOp(op, backend string) (stop func()) { return terminalSlowOps.Watch(op, backend) }
