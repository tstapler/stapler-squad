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
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// CaptureTapDirEnv is an operator override: it sets the tap directory and
// enables the tap for every session at startup with no TTL. Otherwise the tap
// is off until enabled at runtime through TapRegistry (the SetCaptureTap RPC)
// and writes under <state dir>/tap. See TapRegistry.
//
// The tap writes raw terminal bytes, which can include typed passwords and
// tokens printed by agents. Files are 0600 in a 0700 directory, capped at
// defaultTapMaxBytes per file, and never rotated: delete them when done.
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

// tapSink is the per-session state shared by every CaptureTap view of it. The
// file opens lazily on the first write while enabled and closes on disable,
// cap or error.
type tapSink struct {
	mu       sync.Mutex
	enabled  atomic.Bool  // hot-path gate; written under mu or by the registry
	deadline atomic.Int64 // unix nanos at which the TTL ends; 0 = none
	reg      *TapRegistry // nil for in-memory writers
	name     string
	path     string
	w        io.Writer
	closer   io.Closer // nil for in-memory writers
	now      func() time.Time
	failed   bool // a write or open error stopped the tap
	capped   bool // the size cap stopped the tap
	scratch  []byte
	written  int64 // size of the file, including earlier appends
	maxBytes int64
}

// CaptureTap appends TapRecords to one session's JSONL file. A nil
// *CaptureTap is valid and inert, as is a handle whose tap is currently
// disabled: Record then costs one atomic load. Write errors and the size cap
// are logged once and then swallowed so the tap can never disturb the output
// hot path. Writes are synchronous on the calling goroutine, so a slow disk
// delays live output: use a local directory.
type CaptureTap struct {
	s   *tapSink
	src TapSource
}

func newCaptureTapWriter(w io.Writer) *CaptureTap {
	s := &tapSink{w: w, now: time.Now, maxBytes: defaultTapMaxBytes}
	s.enabled.Store(true)
	return &CaptureTap{s: s}
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
	if t == nil || !t.s.enabled.Load() {
		return
	}
	t.write(TapRecord{Kind: kind, Cause: cause, B64: base64.StdEncoding.EncodeToString(data)})
}

// RecordResize appends a resize record carrying the requested size.
func (t *CaptureTap) RecordResize(size TerminalSize) {
	if t == nil || !t.s.enabled.Load() {
		return
	}
	t.write(TapRecord{Kind: TapResize, Cols: size.cols, Rows: size.rows})
}

func (t *CaptureTap) write(rec TapRecord) {
	s := t.s
	// Checked before taking s.mu: sweep locks the registry and then the sink.
	if dl := s.deadline.Load(); dl != 0 && s.reg != nil && s.now().UnixNano() >= dl {
		s.reg.sweep()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled.Load() {
		return
	}
	if s.w == nil {
		if err := s.openLocked(); err != nil {
			s.stopLocked(err, false)
			return
		}
	}
	rec.TNs = s.now().UnixNano()
	rec.Src = t.src
	line, err := json.Marshal(rec)
	capped := false
	if err == nil {
		if s.written+int64(len(line))+1 > s.maxBytes {
			err = fmt.Errorf("size cap of %d bytes reached", s.maxBytes)
			capped = true
		} else {
			s.scratch = append(append(s.scratch[:0], line...), '\n')
			var n int
			n, err = s.w.Write(s.scratch)
			s.written += int64(n)
		}
	}
	if err != nil {
		s.stopLocked(err, capped)
	}
}

// openLocked opens the session's file for appending and seeds written from its
// current size so the cap holds across disable and re-enable.
func (s *tapSink) openLocked() error {
	path := filepath.Join(s.reg.resolveDir(), sanitizeTapName(s.name)+".jsonl")
	f, err := openTapFile(path)
	if err != nil {
		return fmt.Errorf("cannot open tap file %s: %w", path, err)
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	s.path, s.w, s.closer, s.written = path, f, f, fi.Size()
	log.Info("streamhub capture tap file opened", "session", s.name, "path", path)
	return nil
}

// closeLocked closes the file; the next enabled write reopens it in append mode.
func (s *tapSink) closeLocked() {
	if s.closer != nil {
		_ = s.closer.Close()
		s.w, s.closer = nil, nil
	}
}

// stopLocked turns the tap off for this session after an error or the cap.
func (s *tapSink) stopLocked(err error, capped bool) {
	s.enabled.Store(false)
	s.failed, s.capped = !capped, capped
	s.closeLocked()
	log.Warn("streamhub capture tap: disabling tap for this session", "session", s.name, "error", err)
}

// apply sets whether the tap is on, and until when. A sink stopped by an error
// or the cap stays stopped until reset.
func (s *tapSink) apply(on bool, deadline time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !on {
		s.enabled.Store(false)
		s.deadline.Store(0)
		s.closeLocked()
		return
	}
	if deadline.IsZero() {
		s.deadline.Store(0)
	} else {
		s.deadline.Store(deadline.UnixNano())
	}
	if !s.failed && !s.capped {
		s.enabled.Store(true)
	}
}

// reset clears a stop caused by an error or the cap, on an explicit re-enable.
func (s *tapSink) reset() {
	s.mu.Lock()
	s.failed, s.capped = false, false
	s.mu.Unlock()
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
