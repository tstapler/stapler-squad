package streamhub

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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
// defaultTapMaxBytes per file; a full file is rotated once to .old on re-enable.
// Delete them when done.
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

// Values of tapSink.stop.
const (
	tapRunning int32 = iota
	tapFailed        // a write or open error stopped the tap
	tapCapped        // the size cap stopped the tap
)

// tapSink is the per-session state shared by every CaptureTap view of it. The
// file opens lazily on the first write while enabled and closes on disable,
// cap or error.
//
// mu guards the file (w, closer, scratch) and is held across a disk write, so a
// hung disk can hold it indefinitely. Everything the registry touches (enable,
// disable, status) is therefore an atomic that never waits on mu.
type tapSink struct {
	mu       sync.Mutex
	enabled  atomic.Bool  // hot-path gate
	deadline atomic.Int64 // unix nanos at which the TTL ends; 0 = none
	written  atomic.Int64 // size of the file, including earlier appends; changed under mu
	stop     atomic.Int32 // tapRunning, tapFailed or tapCapped
	rotate   atomic.Bool  // capped before an explicit re-enable: rotate on next open
	rotated  atomic.Bool  // the file was rotated to .old by the latest enable
	// closePending asks the next writer (or a successful TryLock) to close the
	// file once the tap is off, since the disabler must not wait for mu.
	closePending atomic.Bool
	pathv        atomic.Pointer[string]

	reg      *TapRegistry // nil for in-memory writers
	name     string
	w        io.Writer
	closer   io.Closer // nil for in-memory writers
	now      func() time.Time
	scratch  []byte
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
	// Checked before taking s.mu so the sweep never runs under a sink lock.
	if dl := s.deadline.Load(); dl != 0 && s.reg != nil && s.now().UnixNano() >= dl {
		s.reg.onTimer()
	}
	s.mu.Lock()
	s.appendLocked(rec, t.src)
	s.closeIfPendingLocked()
	s.mu.Unlock()
	// A disable that lost the TryLock race above may have set closePending
	// after our check; finish the close it could not.
	s.tryClosePending()
}

func (s *tapSink) appendLocked(rec TapRecord, src TapSource) {
	if !s.enabled.Load() || s.stop.Load() != tapRunning {
		return
	}
	if s.w == nil {
		if err := s.openLocked(); err != nil {
			s.stopLocked(err, false)
			return
		}
	}
	rec.TNs = s.now().UnixNano()
	rec.Src = src
	line, err := json.Marshal(rec)
	if err != nil {
		s.stopLocked(err, false)
		return
	}
	if s.written.Load()+int64(len(line))+1 > s.maxBytes {
		s.stopLocked(fmt.Errorf("size cap of %d bytes reached", s.maxBytes), true)
		return
	}
	s.scratch = append(append(s.scratch[:0], line...), '\n')
	n, err := s.w.Write(s.scratch)
	s.written.Add(int64(n))
	if err != nil {
		s.stopLocked(err, false)
	}
}

// openLocked opens the session's file for appending and seeds written from its
// current size so the cap holds across disable and re-enable. A file at the cap,
// or one that stopped the tap before an explicit re-enable, is first rotated to
// <name>.jsonl.old, replacing any earlier .old file.
func (s *tapSink) openLocked() error {
	dir, err := s.reg.resolveDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, sanitizeTapName(s.name)+".jsonl")
	forceRotate := s.rotate.Swap(false)
	f, rotated, err := openTapFile(dir, path, s.maxBytes, forceRotate)
	if err != nil {
		if forceRotate {
			s.rotate.Store(true) // a failed open must not lose the pending rotation
		}
		return fmt.Errorf("cannot open tap file %s: %w", path, err)
	}
	fi, err := f.Stat()
	if err != nil {
		if forceRotate {
			s.rotate.Store(true)
		}
		_ = f.Close()
		return err
	}
	s.w, s.closer = f, f
	s.pathv.Store(&path)
	s.written.Store(fi.Size())
	if rotated {
		s.rotated.Store(true)
		log.Warn("streamhub capture tap: rotated full file", "session", s.name, "old", path+tapRotatedSuffix)
	}
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

func (s *tapSink) closeIfPendingLocked() {
	if s.closePending.Swap(false) && !s.enabled.Load() {
		s.closeLocked()
	}
}

// tryClosePending closes the file if a disable asked for it and no write is in
// flight. It never blocks: an in-flight writer closes the file itself.
func (s *tapSink) tryClosePending() {
	if s.closePending.Load() && s.mu.TryLock() {
		s.closeIfPendingLocked()
		s.mu.Unlock()
	}
}

// stopLocked turns the tap off for this session after an error or the cap.
func (s *tapSink) stopLocked(err error, capped bool) {
	if capped {
		s.stop.Store(tapCapped)
	} else {
		s.stop.Store(tapFailed)
	}
	s.enabled.Store(false)
	s.closeLocked()
	log.Warn("streamhub capture tap: disabling tap for this session", "session", s.name, "error", err)
}

// apply sets whether the tap is on, and until when. It never blocks, so the
// registry can call it while a writer is stuck on a hung disk. A sink stopped
// by an error or the cap stays stopped until reset.
func (s *tapSink) apply(on bool, deadline time.Time) {
	if !on {
		s.enabled.Store(false)
		s.deadline.Store(0)
		s.closePending.Store(true)
		s.tryClosePending()
		return
	}
	if deadline.IsZero() {
		s.deadline.Store(0)
	} else {
		s.deadline.Store(deadline.UnixNano())
	}
	s.enabled.Store(s.stop.Load() == tapRunning)
}

// reset clears a stop caused by an error or the cap, on an explicit re-enable.
// A capped file is rotated when it next opens.
func (s *tapSink) reset() {
	if s.stop.Swap(tapRunning) == tapCapped {
		s.rotate.Store(true)
	}
	s.rotated.Store(false)
}

// tapRotatedSuffix names the previous file after a rotation.
const tapRotatedSuffix = ".old"

// openTapFile creates dir as 0700 if missing, refuses a symlink at path,
// rotates a full file to path+".old", and opens the file 0600 in append mode,
// terminating a torn last line so later records stay readable.
func openTapFile(dir, path string, maxBytes int64, forceRotate bool) (f *os.File, rotated bool, err error) {
	if err := ensureTapDir(dir, os.Getuid()); err != nil {
		return nil, false, err
	}
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, false, fmt.Errorf("refusing to follow symlink at %s", path)
		}
		if fi.Size() >= maxBytes || (forceRotate && fi.Size() > 0) {
			if err := os.Rename(path, path+tapRotatedSuffix); err != nil {
				return nil, false, fmt.Errorf("rotating full tap file: %w", err)
			}
			rotated = true
		}
	}
	f, err = os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return nil, false, err
	}
	if err := terminateTornTail(f); err != nil {
		_ = f.Close()
		return nil, false, err
	}
	return f, rotated, nil
}

// terminateTornTail appends a newline when the file's last byte is not one, as
// after a crash mid-write, so the next record starts on its own line.
func terminateTornTail(f *os.File) error {
	fi, err := f.Stat()
	if err != nil || fi.Size() == 0 {
		return err
	}
	var last [1]byte
	if _, err := f.ReadAt(last[:], fi.Size()-1); err != nil {
		return err
	}
	if last[0] == '\n' {
		return nil
	}
	_, err = f.Write([]byte{'\n'})
	return err
}

// ensureTapDir creates dir 0700 when missing. An existing directory is never
// chmodded (it may be the operator's home or temp dir); it is refused unless it
// is owned by uid and not group- or world-writable.
func ensureTapDir(dir string, uid int) error {
	fi, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		if err = os.MkdirAll(dir, 0o700); err == nil {
			fi, err = os.Stat(dir)
		}
	}
	if err != nil {
		return err
	}
	switch {
	case !fi.IsDir():
		return fmt.Errorf("%s is not a directory", dir)
	case runtime.GOOS != "windows" && fi.Mode().Perm()&0o022 != 0: // Windows reports 0777 for every directory
		return fmt.Errorf("refusing tap directory %s: group- or world-writable (%v)", dir, fi.Mode().Perm())
	case !ownedByUID(fi, uid):
		return fmt.Errorf("refusing tap directory %s: not owned by uid %d", dir, uid)
	}
	return nil
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
