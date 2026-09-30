package streamhub

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
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
//
// The tap writes raw terminal bytes, which can include typed passwords and
// tokens printed by agents. Files are 0600 in a 0700 directory, capped at
// defaultTapMaxBytes per session, and never rotated: delete them when done.
// See docs/how-to/capture-terminal-stream-tap.md.
const CaptureTapDirEnv = "STAPLER_SQUAD_CAPTURE_TAP_DIR"

// defaultTapMaxBytes bounds one session's tap file. On reaching it the tap
// stops for that session (logged once) rather than filling the disk.
const defaultTapMaxBytes = 64 << 20

// TapKind classifies a capture-tap record.
type TapKind string

const (
	TapOutput   TapKind = "output"   // bytes entering the hub or forwarder, before delivery to any subscriber
	TapDrop     TapKind = "drop"     // bytes discarded, see DropCause
	TapResize   TapKind = "resize"   // a negotiated resize began (hub path only)
	TapSnapshot TapKind = "snapshot" // a post-resize snapshot was published (hub path only)
)

// DropCause records why bytes were discarded.
type DropCause string

const (
	DropCauseResizeSettling     DropCause = "resize_settling"
	DropCauseForwardingNotReady DropCause = "forwarding_not_ready"
	// DropCauseSubscriberUndelivered is a frame a subscriber's transport did
	// not accept: its outbound queue was full, or it was already closed. One
	// record per undelivered subscriber, not per frame.
	DropCauseSubscriberUndelivered DropCause = "subscriber_undelivered"
)

// TapSource identifies which stream path wrote a record, so a session served
// by several connections (or both paths) is not double counted.
type TapSource string

const (
	TapSourceHub    TapSource = "hub"
	TapSourceLegacy TapSource = "legacy" // per-connection forwarder; records post-coalesce buffers
)

// TapRecord is one JSONL line of a capture tap.
type TapRecord struct {
	TNs   int64     `json:"t_ns"`
	Kind  TapKind   `json:"kind"`
	Src   TapSource `json:"src,omitempty"`
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

// tapSink is the per-session file shared by every CaptureTap view of it.
type tapSink struct {
	mu       sync.Mutex
	w        io.Writer
	closer   io.Closer // nil for in-memory writers
	now      func() time.Time
	failed   bool
	scratch  []byte
	written  int64
	maxBytes int64
}

// CaptureTap appends TapRecords to one session's JSONL file. A nil
// *CaptureTap is valid and inert. Write errors and the size cap are logged
// once and then swallowed so the tap can never disturb the output hot path.
// Writes are synchronous on the calling goroutine, so a slow disk delays live
// output: use a local directory.
type CaptureTap struct {
	s   *tapSink
	src TapSource
}

func newCaptureTapWriter(w io.Writer) *CaptureTap {
	return &CaptureTap{s: &tapSink{w: w, now: time.Now, maxBytes: defaultTapMaxBytes}}
}

// As returns a view of the same file whose records carry src. Safe on nil.
func (t *CaptureTap) As(src TapSource) *CaptureTap {
	if t == nil {
		return nil
	}
	return &CaptureTap{s: t.s, src: src}
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
	s := t.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed {
		return
	}
	rec.TNs = s.now().UnixNano()
	rec.Src = t.src
	line, err := json.Marshal(rec)
	if err == nil {
		if s.written+int64(len(line))+1 > s.maxBytes {
			err = fmt.Errorf("size cap of %d bytes reached", s.maxBytes)
		} else {
			s.scratch = append(append(s.scratch[:0], line...), '\n')
			var n int
			n, err = s.w.Write(s.scratch)
			s.written += int64(n)
		}
	}
	if err != nil {
		s.failed = true
		if s.closer != nil {
			_ = s.closer.Close()
		}
		log.Warn("streamhub capture tap: disabling tap for this session", "error", err)
	}
}

var (
	tapsMu sync.Mutex
	taps   = map[string]*CaptureTap{}
	// tapActiveWarn logs once per process, at Warn, that the tap is enabled.
	tapActiveWarn sync.Once
)

// CaptureTapFor returns the shared tap for sessionName, or nil when
// STAPLER_SQUAD_CAPTURE_TAP_DIR is unset or the file cannot be opened safely.
// Every stream path for a session shares one file
// (<dir>/<sanitized-session>[-<hash>].jsonl, append mode). The file stays open
// until the size cap or a write error closes it, so descriptors grow with the
// number of distinct sessions seen while the tap is on. A failed open is not
// cached, so fixing the directory does not need a restart.
func CaptureTapFor(sessionName string) *CaptureTap {
	dir := os.Getenv(CaptureTapDirEnv)
	if dir == "" {
		return nil
	}
	tapActiveWarn.Do(func() {
		log.Warn("streamhub capture tap ACTIVE: recording raw terminal output, which may include secrets",
			"dir", dir, "env", CaptureTapDirEnv, "max_bytes_per_session", defaultTapMaxBytes)
	})
	path := filepath.Join(dir, sanitizeTapName(sessionName)+".jsonl")
	tapsMu.Lock()
	defer tapsMu.Unlock()
	if t, ok := taps[path]; ok && !t.s.isFailed() {
		return t
	}
	f, err := openTapFile(path)
	if err != nil {
		log.Warn("streamhub capture tap: cannot open tap file, tap disabled for this stream", "path", path, "error", err)
		return nil
	}
	t := newCaptureTapWriter(f)
	t.s.closer = f
	taps[path] = t
	log.Info("streamhub capture tap enabled", "session", sessionName, "path", path)
	return t
}

func (s *tapSink) isFailed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed
}

// openTapFile creates dir as 0700 (and tightens an existing one we own), refuses
// a symlink at path, and opens the file 0600 in append mode. With the directory
// owner-only, no other user can plant a symlink between the check and the open.
func openTapFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("directory %s is not owned by this user: %w", dir, err)
	}
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to follow symlink at %s", path)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// sanitizeTapName maps name to a safe file stem. A name that needed changes gets
// a short hash of the original so "a/b" and "a b" never share a file.
func sanitizeTapName(name string) string {
	changed := false
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		changed = true
		return '_'
	}, name)
	if !changed {
		return clean
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name)) // hash.Hash.Write never returns an error
	return fmt.Sprintf("%s-%08x", clean, h.Sum32())
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

// PairSettleLatency returns, for each resize record, the time to the next
// snapshot record. A second resize before any snapshot replaces the first (the
// earlier one was aborted and produced no snapshot), a resize with no following
// snapshot is omitted, and a non-positive interval (clock step) is skipped.
// Only the hub path writes resize and snapshot records, so a legacy-only tap
// yields nothing. Timestamps are wall clock, so treat results as approximate.
func PairSettleLatency(r io.Reader) []time.Duration {
	recs, _ := ReadTap(r) // a torn trailing line still yields the records before it
	var out []time.Duration
	var start int64
	pending := false
	for _, rec := range recs {
		switch rec.Kind {
		case TapResize:
			start, pending = rec.TNs, true
		case TapSnapshot:
			if pending && rec.TNs > start {
				out = append(out, time.Duration(rec.TNs-start))
			}
			pending = false
		}
	}
	return out
}
