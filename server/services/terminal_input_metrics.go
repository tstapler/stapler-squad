package services

import (
	"container/list"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
)

// Terminal input observability (ADR-005). Counters carry class-only labels
// (kind, listener, path, stage); the per-session view is a bounded in-process
// tally behind /actuator/terminal-input. Input content is never stored or logged.

// InputClass is the coarse shape of an input frame.
type InputClass uint8

const (
	InputClassText InputClass = iota
	InputClassEscape
	InputClassOther
	inputClassCount
)

var inputClassNames = [inputClassCount]string{"text", "escape", "other"}

func (c InputClass) String() string { return inputClassNames[c] }

// InputPath names the server-side input entry point that saw a frame.
type InputPath uint8

const (
	PathCM InputPath = iota
	PathShell
	PathCapture
	PathStream
	inputPathCount
)

var inputPathNames = [inputPathCount]string{"cm", "shell", "capture", "stream"}

func (p InputPath) String() string { return inputPathNames[p] }

// InputStage is where in the pipeline a frame was counted.
type InputStage uint8

const (
	// StageReceived is counted at the entry, before permission/read-only checks.
	StageReceived InputStage = iota
	// StageForwarded is counted only after the gates pass and the write is issued.
	StageForwarded
	inputStageCount
)

var inputStageNames = [inputStageCount]string{"received", "forwarded"}

func (s InputStage) String() string { return inputStageNames[s] }

const (
	inputListenerLocal = iota
	inputListenerRemote
	inputListenerUnknown
	inputListenerCount
)

var inputListenerNames = [inputListenerCount]string{ListenerLocal, ListenerRemote, "unknown"}

// normalizeInputListener bounds the listener label: only the two server-side
// constants are ever label values.
func normalizeInputListener(listener string) int {
	switch listener {
	case ListenerLocal:
		return inputListenerLocal
	case ListenerRemote:
		return inputListenerRemote
	default:
		return inputListenerUnknown
	}
}

const (
	terminalInputTallySize     = 256
	terminalInputClassifyLimit = 64
	terminalInputSummaryEvery  = time.Minute
)

// classifyInput looks at no more than the first 64 bytes: ESC-first is escape;
// printable UTF-8 plus \r \t \n \x7f is text; anything else is other.
func classifyInput(data []byte) InputClass {
	if len(data) == 0 {
		return InputClassOther
	}
	if data[0] == 0x1b {
		return InputClassEscape
	}
	head := data
	truncated := len(head) > terminalInputClassifyLimit
	if truncated {
		head = head[:terminalInputClassifyLimit]
	}
	for len(head) > 0 {
		r, size := utf8.DecodeRune(head)
		if r == utf8.RuneError && size <= 1 {
			// A rune split by the scan limit is not evidence of non-text.
			if truncated && !utf8.FullRune(head) {
				return InputClassText
			}
			return InputClassOther
		}
		if !isTextRune(r) {
			return InputClassOther
		}
		head = head[size:]
	}
	return InputClassText
}

func isTextRune(r rune) bool {
	if r == '\r' || r == '\t' || r == '\n' || r == 0x7f {
		return true
	}
	return r >= 0x20 && (r < 0x80 || r >= 0xa0)
}

var (
	terminalInputFramesCounter = sync.OnceValue(counterNamed("terminal_input_frames_total", "Terminal input frames by class, listener, entry path and stage (received|forwarded); never carries session or content"))
	terminalInputBytesCounter  = sync.OnceValue(counterNamed("terminal_input_bytes_total", "Terminal input bytes by class, listener, entry path and stage; never carries session or content"))
)

type inputSeries [inputClassCount][inputListenerCount][inputPathCount][inputStageCount]struct {
	frames atomic.Int64
	bytes  atomic.Int64
	opt    metric.MeasurementOption
}

// terminalInputMetrics owns the in-process counters, the session tally and the
// logging clock. Production uses defaultTerminalInput; tests build their own.
type terminalInputMetrics struct {
	series inputSeries
	tally  *inputTally
	now    func() time.Time
	logf   func(msg string, args ...any)
}

func newTerminalInputMetrics(now func() time.Time, logf func(string, ...any)) *terminalInputMetrics {
	m := &terminalInputMetrics{tally: newInputTally(terminalInputTallySize), now: now, logf: logf}
	for k := range m.series {
		for l := range m.series[k] {
			m.initSeriesCells(InputClass(k), l)
		}
	}
	return m
}

func (m *terminalInputMetrics) initSeriesCells(class InputClass, listener int) {
	for p := range m.series[class][listener] {
		for s := range m.series[class][listener][p] {
			m.series[class][listener][p][s].opt = metric.WithAttributeSet(attribute.NewSet(
				attribute.String("kind", class.String()),
				attribute.String("listener", inputListenerNames[listener]),
				attribute.String("path", InputPath(p).String()),
				attribute.String("stage", InputStage(s).String()),
			))
		}
	}
}

var defaultTerminalInput = newTerminalInputMetrics(time.Now, func(msg string, args ...any) { log.Info(msg, args...) })

// inputSource identifies one input stream (one WebSocket or Connect stream) for
// counting and log throttling. It is safe for concurrent use; a nil source records nothing.
type inputSource struct {
	m         *terminalInputMetrics
	path      InputPath
	listener  int
	sessionID string

	mu          sync.Mutex
	sawFirst    bool
	lastSummary time.Time
	sumFrames   int64
	sumBytes    int64
}

func (m *terminalInputMetrics) source(path InputPath, listener, sessionID string) *inputSource {
	return &inputSource{m: m, path: path, listener: normalizeInputListener(listener), sessionID: sessionID}
}

func newInputSource(path InputPath, listener, sessionID string) *inputSource {
	return defaultTerminalInput.source(path, listener, sessionID)
}

// recordTerminalInput counts one frame at the given stage. The frame is only
// classified and measured; its bytes are never retained.
func recordTerminalInput(src *inputSource, data []byte, stage InputStage) {
	if src == nil {
		return
	}
	src.m.record(src, data, stage)
}

func (m *terminalInputMetrics) record(src *inputSource, data []byte, stage InputStage) {
	class := classifyInput(data)
	cell := &m.series[class][src.listener][src.path][stage]
	cell.frames.Add(1)
	cell.bytes.Add(int64(len(data)))
	if c := terminalInputFramesCounter(); c != nil {
		c.Add(context.Background(), 1, cell.opt)
	}
	if n := len(data); n > 0 {
		if c := terminalInputBytesCounter(); c != nil {
			c.Add(context.Background(), int64(n), cell.opt)
		}
	}
	now := m.now()
	m.tally.record(inputObservation{session: src.sessionID, class: class, path: src.path, stage: stage, bytes: len(data), at: now})
	if stage == StageReceived {
		src.logReceived(now, len(data))
	}
}

// logReceived emits an Info line on the stream's first input and then at most
// one summary per minute. Fields are class/size only, never content.
func (s *inputSource) logReceived(now time.Time, n int) {
	s.mu.Lock()
	s.sumFrames++
	s.sumBytes += int64(n)
	var msg string
	var frames, bytes int64
	switch {
	case !s.sawFirst:
		s.sawFirst = true
		s.lastSummary = now
		msg, frames, bytes = "terminal input: first frame on stream", 1, int64(n)
		s.sumFrames, s.sumBytes = 0, 0
	case now.Sub(s.lastSummary) >= terminalInputSummaryEvery:
		s.lastSummary = now
		msg, frames, bytes = "terminal input: summary", s.sumFrames, s.sumBytes
		s.sumFrames, s.sumBytes = 0, 0
	}
	s.mu.Unlock()
	if msg == "" {
		return
	}
	s.m.logf(msg, "session", s.sessionID, "path", s.path.String(), "listener", inputListenerNames[s.listener], "frames", frames, "bytes", bytes)
}

// count reads one in-process series cell; tests and diagnostics use it instead of a metric reader.
func (m *terminalInputMetrics) count(class InputClass, listener string, path InputPath, stage InputStage) (frames, bytes int64) {
	cell := &m.series[class][normalizeInputListener(listener)][path][stage]
	return cell.frames.Load(), cell.bytes.Load()
}

// inputSessionEntry holds one session's counters; fields are atomic so readers never block recorders.
type inputSessionEntry struct {
	id       string
	received atomic.Int64
	bytes    atomic.Int64
	byKind   [inputClassCount]atomic.Int64
	lastAt   atomic.Int64 // unix nanos
	paths    [inputPathCount][inputStageCount]atomic.Int64
}

// inputTally is an LRU of per-session entries bounded to cap; it never stores data.
type inputTally struct {
	mu      sync.Mutex
	cap     int
	order   *list.List // front = most recent
	entries map[string]*list.Element
}

func newInputTally(capacity int) *inputTally {
	return &inputTally{cap: capacity, order: list.New(), entries: map[string]*list.Element{}}
}

func (t *inputTally) entry(id string) *inputSessionEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	if el, ok := t.entries[id]; ok {
		t.order.MoveToFront(el)
		return el.Value.(*inputSessionEntry)
	}
	e := &inputSessionEntry{id: id}
	t.entries[id] = t.order.PushFront(e)
	for t.order.Len() > t.cap {
		oldest := t.order.Back()
		t.order.Remove(oldest)
		delete(t.entries, oldest.Value.(*inputSessionEntry).id)
	}
	return e
}

type inputObservation struct {
	session string
	class   InputClass
	path    InputPath
	stage   InputStage
	bytes   int
	at      time.Time
}

func (t *inputTally) record(o inputObservation) {
	e := t.entry(o.session)
	e.paths[o.path][o.stage].Add(1)
	if o.stage != StageReceived {
		return
	}
	e.received.Add(1)
	e.bytes.Add(int64(o.bytes))
	e.byKind[o.class].Add(1)
	e.lastAt.Store(o.at.UnixNano())
}

func (t *inputTally) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

// TerminalInputPathCounts is one entry path's received/forwarded frame counts for a session.
type TerminalInputPathCounts struct {
	Received  int64 `json:"received"`
	Forwarded int64 `json:"forwarded"`
}

// TerminalInputSession is one session's row in the /actuator/terminal-input payload.
type TerminalInputSession struct {
	SessionID string                             `json:"sessionId"`
	Frames    int64                              `json:"frames"`
	Bytes     int64                              `json:"bytes"`
	ByKind    map[string]int64                   `json:"byKind"`
	ByPath    map[string]TerminalInputPathCounts `json:"byPath"`
	LastAt    time.Time                          `json:"lastAt"`
}

func (t *inputTally) snapshot() []TerminalInputSession {
	t.mu.Lock()
	entries := make([]*inputSessionEntry, 0, len(t.entries))
	for el := t.order.Front(); el != nil; el = el.Next() {
		entries = append(entries, el.Value.(*inputSessionEntry))
	}
	t.mu.Unlock()

	out := make([]TerminalInputSession, 0, len(entries))
	for _, e := range entries {
		row := TerminalInputSession{
			SessionID: e.id, Frames: e.received.Load(), Bytes: e.bytes.Load(),
			ByKind: map[string]int64{}, ByPath: map[string]TerminalInputPathCounts{},
		}
		for k := range e.byKind {
			if n := e.byKind[k].Load(); n > 0 {
				row.ByKind[InputClass(k).String()] = n
			}
		}
		for p := range e.paths {
			r, f := e.paths[p][StageReceived].Load(), e.paths[p][StageForwarded].Load()
			if r > 0 || f > 0 {
				row.ByPath[InputPath(p).String()] = TerminalInputPathCounts{Received: r, Forwarded: f}
			}
		}
		if ns := e.lastAt.Load(); ns > 0 {
			row.LastAt = time.Unix(0, ns).UTC()
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastAt.After(out[j].LastAt) })
	return out
}

// TerminalInputReport returns the per-session input tally, most recently active first.
func TerminalInputReport() []TerminalInputSession { return defaultTerminalInput.tally.snapshot() }

// RenderTerminalInputText formats rows for a terminal, one session per line.
func RenderTerminalInputText(rows []TerminalInputSession) string {
	if len(rows) == 0 {
		return "no terminal input recorded\n"
	}
	var b strings.Builder
	for _, r := range rows {
		last := "never"
		if !r.LastAt.IsZero() {
			last = r.LastAt.Format(time.RFC3339)
		}
		fmt.Fprintf(&b, "%s frames=%d bytes=%d text=%d escape=%d other=%d last=%s\n",
			r.SessionID, r.Frames, r.Bytes, r.ByKind["text"], r.ByKind["escape"], r.ByKind["other"], last)
	}
	return b.String()
}
