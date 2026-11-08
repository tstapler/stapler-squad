// Package termlat records terminal-pipeline latency per stage, backend and listener.
//
// Every observation feeds an OTel histogram (terminal_latency.stage_duration, for
// VictoriaMetrics) and a small in-process rolling window, so /actuator/terminal-latency can
// show percentiles with no metrics backend running. All labels are fixed enums: the
// sender of a client-reported stage is untrusted, so Normalize* folds anything else into
// "unknown".
package termlat

import (
	"context"
	"math"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Stages. Duration stages are milliseconds; depth stages are plain counts.
const (
	StageInputTotal     = "input_total"    // input frame read from the socket -> backend send returned
	StageInputSend      = "input_send"     // backend send alone (daemon ack for tymux, CM write for tmux)
	StageInputBookkeep  = "input_bookkeep" // synchronous per-keystroke bookkeeping before the send (should stay ~0)
	StageBookkeepFlush  = "bookkeep_flush" // deferred bookkeeping flush; includes the i.mu wait it used to impose on typing
	StageOutputSend     = "output_send"    // one batch handed to the transport -> write returned
	StageOutputQueue    = "output_queue_depth"
	StageEchoServer     = "echo_server"      // server: keystroke received -> next output write to the socket
	StageWSWriteWait    = "ws_write_wait"    // server: waiting for the per-stream socket write lock
	StageWSWrite        = "ws_write"         // server: one socket write (mux queue or TCP) returning
	StageEchoArrive     = "echo_arrive"      // browser: keystroke sent -> next output frame reaches the component
	StageEchoPaint      = "echo_paint"       // browser: keystroke sent -> next frame painted (input-to-paint)
	StageTTFPConnect    = "ttfp_connect"     // browser: terminal mounted -> stream open
	StageTTFPFirstData  = "ttfp_first_data"  // browser: terminal mounted -> first output frame
	StageTTFPFirstPaint = "ttfp_first_paint" // browser: terminal mounted -> first frame painted
	StageAttachQueue    = "attach_queue"     // server: waiting for an attach slot (reload thundering herd)
	StageAttachOpen     = "attach_open"      // server: backend alive/adopt/restore for one terminal attach
)

const unknown = "unknown"

// NormalizeStage returns s if it is a known stage, else "unknown".
func NormalizeStage(s string) string {
	switch s {
	case StageInputTotal, StageInputSend, StageInputBookkeep, StageBookkeepFlush, StageOutputSend,
		StageOutputQueue, StageEchoServer, StageWSWriteWait, StageWSWrite, StageEchoArrive, StageEchoPaint,
		StageTTFPConnect, StageTTFPFirstData, StageTTFPFirstPaint, StageAttachQueue, StageAttachOpen:
		return s
	}
	return unknown
}

// NormalizeBackend returns b if it is a known backend, else "unknown".
func NormalizeBackend(b string) string {
	switch b {
	case "tmux", "tymux", "native":
		return b
	}
	return unknown
}

// NormalizeListener returns l if it is a known listener, else "unknown".
func NormalizeListener(l string) string {
	switch l {
	case "local", "remote":
		return l
	}
	return unknown
}

// windowSize bounds memory per key (stage x backend x listener); sampleTTL ages out stale data
// so the percentiles describe the recent past, not a session from yesterday.
const (
	windowSize = 512
	sampleTTL  = 5 * time.Minute
)

type key struct{ stage, backend, listener string }

type sample struct {
	v  float64
	at time.Time
}

type window struct {
	mu    sync.Mutex
	buf   [windowSize]sample
	next  int
	n     int
	total uint64
	max   float64
}

func (w *window) add(v float64, at time.Time) {
	w.mu.Lock()
	w.buf[w.next] = sample{v, at}
	w.next = (w.next + 1) % windowSize
	if w.n < windowSize {
		w.n++
	}
	w.total++
	if v > w.max {
		w.max = v
	}
	w.mu.Unlock()
}

// values returns the samples newer than cutoff.
func (w *window) values(cutoff time.Time) (vals []float64, total uint64, max float64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := 0; i < w.n; i++ {
		if s := w.buf[i]; !s.at.Before(cutoff) {
			vals = append(vals, s.v)
		}
	}
	return vals, w.total, w.max
}

// Stats is one row of Report.
type Stats struct {
	Stage    string  `json:"stage"`
	Backend  string  `json:"backend"`
	Listener string  `json:"listener"`
	Count    int     `json:"count"` // samples inside the rolling window
	Total    uint64  `json:"total"` // samples since process start
	P50      float64 `json:"p50"`
	P95      float64 `json:"p95"`
	P99      float64 `json:"p99"`
	Max      float64 `json:"max"` // since process start
}

// Percentile returns the nearest-rank percentile (0-100) of an unsorted slice.
func Percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := append([]float64(nil), vals...)
	sort.Float64s(s)
	rank := int(math.Ceil(p/100*float64(len(s)))) - 1
	if rank < 0 {
		rank = 0
	}
	return s[rank]
}

// Recorder holds the rolling windows. The zero value is not usable; call New.
type Recorder struct {
	now     func() time.Time
	mu      sync.RWMutex
	windows map[key]*window
	hist    metric.Float64Histogram
	alerts  alerter
}

// New returns a Recorder reading time from now (time.Now when nil).
func New(now func() time.Time) *Recorder {
	if now == nil {
		now = time.Now
	}
	r := &Recorder{now: now, windows: map[key]*window{}}
	r.alerts = newAlerter(r)
	h, err := telemetry.GetMeter().Float64Histogram("terminal_latency.stage_duration",
		metric.WithDescription("Terminal pipeline stage latency (ms; *_depth stages are counts) by stage, backend and listener"),
		metric.WithExplicitBucketBoundaries(0.5, 1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000))
	if err != nil {
		log.Error("termlat: failed to register histogram", "error", err)
	} else {
		r.hist = h
	}
	return r
}

// Default is the process-wide recorder.
var Default = New(nil) //nolint:gochecknoglobals // singleton: every stream and the actuator must share one set of windows

// Labels identifies which backend and listener an observation came from.
type Labels struct{ Backend, Listener string }

// Record adds a duration observation.
func (r *Recorder) Record(stage string, l Labels, d time.Duration) {
	r.RecordValue(stage, l, float64(d)/float64(time.Millisecond))
}

// RecordValue adds a raw observation (ms for duration stages, a count for depth stages).
// Negative and non-finite values are dropped.
func (r *Recorder) RecordValue(stage string, l Labels, v float64) {
	if r == nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	k := key{NormalizeStage(stage), NormalizeBackend(l.Backend), NormalizeListener(l.Listener)}
	r.window(k).add(v, r.now())
	if r.hist != nil {
		r.hist.Record(context.Background(), v, metric.WithAttributes(
			attribute.String("stage", k.stage), attribute.String("backend", k.backend), attribute.String("listener", k.listener)))
	}
	r.alerts.observe(k)
}

func (r *Recorder) window(k key) *window {
	r.mu.RLock()
	w := r.windows[k]
	r.mu.RUnlock()
	if w != nil {
		return w
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if w = r.windows[k]; w == nil {
		w = &window{}
		r.windows[k] = w
	}
	return w
}

// Report returns percentile rows for every key seen, sorted by stage, backend, listener.
func (r *Recorder) Report() []Stats {
	cutoff := r.now().Add(-sampleTTL)
	r.mu.RLock()
	keys := make([]key, 0, len(r.windows))
	for k := range r.windows {
		keys = append(keys, k)
	}
	r.mu.RUnlock()
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.stage != b.stage {
			return a.stage < b.stage
		}
		if a.backend != b.backend {
			return a.backend < b.backend
		}
		return a.listener < b.listener
	})
	out := make([]Stats, 0, len(keys))
	for _, k := range keys {
		vals, total, max := r.window(k).values(cutoff)
		out = append(out, Stats{
			Stage: k.stage, Backend: k.backend, Listener: k.listener,
			Count: len(vals), Total: total, Max: max,
			P50: Percentile(vals, 50), P95: Percentile(vals, 95), P99: Percentile(vals, 99),
		})
	}
	return out
}

// percentile95 is the rolling p95 for a key plus its sample count.
func (r *Recorder) percentile95(k key) (p95 float64, n int) {
	vals, _, _ := r.window(k).values(r.now().Add(-sampleTTL))
	return Percentile(vals, 95), len(vals)
}

// Alert thresholds: a stage whose rolling p95 exceeds its limit logs one warning per minute.
// Echo is the user-visible number; input_total is its server-side share.
const (
	EnvEchoP95Ms  = "STAPLER_SQUAD_TERMINAL_ECHO_P95_MS"
	defaultEchoMs = 150.0
	defaultSrvMs  = 100.0
	alertMinN     = 20
	alertEvery    = time.Minute
	alertCheckGap = 5 * time.Second
)

type alerter struct {
	r         *Recorder
	echoLimit float64
	lastCheck atomic.Int64
	mu        sync.Mutex
	lastWarn  map[key]time.Time
	active    map[key]bool
}

func newAlerter(r *Recorder) alerter {
	limit := defaultEchoMs
	if v, err := strconv.ParseFloat(os.Getenv(EnvEchoP95Ms), 64); err == nil && v > 0 {
		limit = v
	}
	return alerter{r: r, echoLimit: limit, lastWarn: map[key]time.Time{}, active: map[key]bool{}}
}

func (a *alerter) limit(stage string) float64 {
	switch stage {
	case StageEchoPaint, StageEchoServer:
		return a.echoLimit
	case StageInputTotal:
		return defaultSrvMs
	}
	return 0
}

func (a *alerter) observe(k key) {
	lim := a.limit(k.stage)
	if lim == 0 {
		return
	}
	nowNs := a.r.now().UnixNano()
	last := a.lastCheck.Load()
	if nowNs-last < int64(alertCheckGap) || !a.lastCheck.CompareAndSwap(last, nowNs) {
		return
	}
	p95, n := a.r.percentile95(k)
	a.mu.Lock()
	defer a.mu.Unlock()
	over := n >= alertMinN && p95 > lim
	a.active[k] = over
	if !over || a.r.now().Sub(a.lastWarn[k]) < alertEvery {
		return
	}
	a.lastWarn[k] = a.r.now()
	log.Warn("terminal latency p95 over threshold", "stage", k.stage, "backend", k.backend,
		"listener", k.listener, "p95_ms", p95, "limit_ms", lim, "samples", n)
}

// Alerting lists the keys currently over their p95 limit as of the last check.
func (r *Recorder) Alerting() []Stats {
	r.alerts.mu.Lock()
	var keys []key
	for k, on := range r.alerts.active {
		if on {
			keys = append(keys, k)
		}
	}
	r.alerts.mu.Unlock()
	var out []Stats
	for _, s := range r.Report() {
		for _, k := range keys {
			if k.stage == s.Stage && k.backend == s.Backend && k.listener == s.Listener {
				out = append(out, s)
			}
		}
	}
	return out
}
