package streamhub

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// CaptureTapDirEnv names the env var that enables the capture tap. When unset
// or empty, CaptureTapFor returns nil and every (*CaptureTap).Record is a
// no-op, so the tap costs nothing on the hot path.
const CaptureTapDirEnv = "STAPLER_SQUAD_CAPTURE_TAP_DIR"

// TapKind classifies a capture-tap record.
type TapKind string

const (
	TapOutput   TapKind = "output"   // bytes delivered into the hub/forwarder
	TapDrop     TapKind = "drop"     // bytes discarded, see DropCause
	TapResize   TapKind = "resize"   // a negotiated resize began
	TapSnapshot TapKind = "snapshot" // a post-resize snapshot was published
)

// DropCause records why bytes were discarded.
type DropCause string

const (
	DropCauseResizeSettling     DropCause = "resize_settling"
	DropCauseForwardingNotReady DropCause = "forwarding_not_ready"
)

// TapRecord is one JSONL line of a capture tap.
type TapRecord struct {
	TNs   int64     `json:"t_ns"`
	Kind  TapKind   `json:"kind"`
	Cause DropCause `json:"cause,omitempty"`
	Cols  int       `json:"cols,omitempty"`
	Rows  int       `json:"rows,omitempty"`
	B64   string    `json:"b64,omitempty"`
}

// Bytes decodes the record's payload; nil if it has none or is malformed.
func (r TapRecord) Bytes() []byte {
	b, err := base64.StdEncoding.DecodeString(r.B64)
	if err != nil {
		return nil
	}
	return b
}

// CaptureTap appends TapRecords to one session's JSONL file. A nil
// *CaptureTap is valid and inert. Write errors are logged once and then
// swallowed so the tap can never disturb the output hot path.
type CaptureTap struct {
	mu      sync.Mutex
	w       io.Writer
	now     func() time.Time
	failed  bool
	scratch []byte
}

func newCaptureTapWriter(w io.Writer) *CaptureTap {
	return &CaptureTap{w: w, now: time.Now}
}

// Record appends one record. Safe on a nil receiver.
func (t *CaptureTap) Record(kind TapKind, cause DropCause, data []byte) {
	if t == nil {
		return
	}
	t.write(TapRecord{Kind: kind, Cause: cause, B64: base64.StdEncoding.EncodeToString(data)})
}

// RecordResize appends a resize record carrying the requested size.
func (t *CaptureTap) RecordResize(size TerminalSize) {
	if t == nil {
		return
	}
	t.write(TapRecord{Kind: TapResize, Cols: size.cols, Rows: size.rows})
}

func (t *CaptureTap) write(rec TapRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failed {
		return
	}
	rec.TNs = t.now().UnixNano()
	line, err := json.Marshal(rec)
	if err == nil {
		t.scratch = append(append(t.scratch[:0], line...), '\n')
		_, err = t.w.Write(t.scratch)
	}
	if err != nil {
		t.failed = true
		log.Warn("streamhub capture tap: write failed, disabling tap for this session", "error", err)
	}
}

var (
	tapsMu sync.Mutex
	taps   = map[string]*CaptureTap{}
)

// CaptureTapFor returns the shared tap for sessionName, or nil when
// STAPLER_SQUAD_CAPTURE_TAP_DIR is unset. Hub and legacy forwarder share one
// file per session (<dir>/<sanitized-session>.jsonl, append mode). The file
// stays open for the process lifetime.
func CaptureTapFor(sessionName string) *CaptureTap {
	dir := os.Getenv(CaptureTapDirEnv)
	if dir == "" {
		return nil
	}
	path := filepath.Join(dir, sanitizeTapName(sessionName)+".jsonl")
	tapsMu.Lock()
	defer tapsMu.Unlock()
	if t, ok := taps[path]; ok {
		return t
	}
	var t *CaptureTap
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Warn("streamhub capture tap: cannot create dir", "dir", dir, "error", err)
	} else if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err != nil {
		log.Warn("streamhub capture tap: cannot open file", "path", path, "error", err)
	} else {
		t = newCaptureTapWriter(f)
		log.Info("streamhub capture tap enabled", "session", sessionName, "path", path)
	}
	taps[path] = t
	return t
}

func sanitizeTapName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, name)
}

// ReadTap parses a JSONL capture tap. A malformed line stops the read and
// returns the records parsed so far along with the error.
func ReadTap(r io.Reader) ([]TapRecord, error) {
	br := bufio.NewReader(r)
	var out []TapRecord
	for {
		line, err := br.ReadBytes('\n')
		if s := strings.TrimSpace(string(line)); s != "" {
			var rec TapRecord
			if uerr := json.Unmarshal([]byte(s), &rec); uerr != nil {
				return out, uerr
			}
			out = append(out, rec)
		}
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
}

// PairSettleLatency returns, for each run of resize_settling drops, the time
// from the run's first drop to the next snapshot record. A run with no
// following snapshot is omitted.
func PairSettleLatency(r io.Reader) []time.Duration {
	recs, _ := ReadTap(r) // a torn trailing line still yields the records before it
	var out []time.Duration
	var firstDrop int64
	pending := false
	for _, rec := range recs {
		switch {
		case rec.Kind == TapDrop && rec.Cause == DropCauseResizeSettling:
			if !pending {
				firstDrop, pending = rec.TNs, true
			}
		case rec.Kind == TapSnapshot && pending:
			out = append(out, time.Duration(rec.TNs-firstDrop))
			pending = false
		}
	}
	return out
}
