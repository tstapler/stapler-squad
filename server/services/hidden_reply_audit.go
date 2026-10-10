package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/server/deliverygate"
)

// The audit sink is the one append-only JSONL file behind hidden-session
// controls (ADR-010 decision 6). This file holds its core: flag_change lines
// land here from the first PR in which the gate can flip, Reply and the steer
// extend it later.
const (
	auditFileName    = "hidden-session-replies.jsonl"
	auditBootSeqName = "boot_seq"
	auditDirMode     = 0o700
	auditFileMode    = 0o600
	auditRotateBytes = 5 << 20
	auditFilesKept   = 3
	auditQueueSize   = 64
	auditAppendBound = 2 * time.Second
	auditKindFlagChg = "flag_change"
	// Steer line kinds (plan Story 5.2). Both carry a `requested` line before the
	// write and a `result` line after it.
	auditKindBacklogSteer = "backlog_steer"
	auditKindGuardBypass  = "guard_bypass"
	auditPhaseRequest     = "requested"
	auditPhaseResult      = "result"
)

// Degraded-mode label values of hidden_session_audit_degraded_total.
const (
	auditModeQueued      = "flag_change_queued"
	auditModeFallbackLog = "flag_change_fallback_log"
	auditModeSteerLog    = "steer_fallback_log"
)

// Outcomes of a flag flip, recorded on the result line.
const (
	flagOutcomeApplied          = "applied"
	flagOutcomePersistFailed    = "aborted_persist_failed"
	flagOutcomeControllerFailed = "aborted_controller_failed"
	flagOutcomeRolledBack       = "aborted_rolled_back"
)

// Errors a caller maps to Internal (nothing was written or persisted).
var (
	ErrAuditFailed  = errors.New("audit append failed")
	ErrAuditTimeout = errors.New("audit append timed out")
)

// AuditLine is one JSONL record. Pointer and omitempty fields keep a lone
// `requested` line from carrying a previous value it cannot know.
type AuditLine struct {
	TS        string `json:"ts"`
	Kind      string `json:"kind"`
	Phase     string `json:"phase,omitempty"`
	ChangeID  string `json:"change_id,omitempty"`
	Flag      string `json:"flag,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Mutation  string `json:"mutation,omitempty"`
	Previous  *bool  `json:"previous,omitempty"`
	New       *bool  `json:"new,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
	Seq       int64  `json:"seq,omitempty"`
	BootID    string `json:"boot_id"`
	BootTS    string `json:"boot_ts"`
	BootSeq   string `json:"boot_seq"`
	Listener  string `json:"listener,omitempty"`
	PeerAddr  string `json:"peer_addr,omitempty"`
	Host      string `json:"host,omitempty"`
	Origin    string `json:"origin,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	AuthMode  string `json:"auth_mode,omitempty"`

	// Steer lines (kind backlog_steer, guard_bypass). The preview is the first
	// 80 characters only; the full text is never recorded.
	SessionUUID    string `json:"session_uuid,omitempty"`
	SessionTitle   string `json:"session_title,omitempty"`
	ItemID         string `json:"item_id,omitempty"`
	MessageLen     int    `json:"message_len,omitempty"`
	MessageSHA256  string `json:"message_sha256,omitempty"`
	MessagePreview string `json:"message_preview,omitempty"`
	PeerLoopback   *bool  `json:"peer_loopback,omitempty"`
	Proxied        *bool  `json:"proxied,omitempty"`
}

// AuditSink appends to <config dir>/audit/hidden-session-replies.jsonl. It does
// no I/O until first use, so a server that never flips a gate flag never
// creates the directory.
type AuditSink struct {
	fs       fsOps
	dirFn    func() (string, error)
	now      func() time.Time
	logger   *slog.Logger
	after    func(time.Duration) <-chan time.Time
	degraded func(mode string)

	openOnce  sync.Once
	openErr   error
	dir       string
	bootID    string    // fixed at construction: identity of this process start
	bootTS    time.Time // label only; readers order by (boot_seq, seq)
	metaMu    sync.Mutex
	bootSeq   string // decimal, or "unknown" until the counter is persisted
	startOnce sync.Once

	mu   sync.Mutex // process-wide: append and rotate
	file appendFile
	size int64

	queue     chan AuditLine
	stop      chan struct{}
	drained   sync.WaitGroup
	closeOnce sync.Once
	closed    chan struct{}
}

// AuditSinkOption configures NewAuditSink.
type AuditSinkOption func(*AuditSink)

// WithAuditFS injects the file system.
func WithAuditFS(f fsOps) AuditSinkOption { return func(s *AuditSink) { s.fs = f } }

// WithAuditClock injects the clock.
func WithAuditClock(now func() time.Time) AuditSinkOption { return func(s *AuditSink) { s.now = now } }

// WithAuditLogger injects the logger used for the fallback record.
func WithAuditLogger(l *slog.Logger) AuditSinkOption { return func(s *AuditSink) { s.logger = l } }

// WithAuditAfter injects the append-bound timer (tests never sleep).
func WithAuditAfter(f func(time.Duration) <-chan time.Time) AuditSinkOption {
	return func(s *AuditSink) { s.after = f }
}

// WithAuditDegradedCounter wires hidden_session_audit_degraded_total{mode}.
func WithAuditDegradedCounter(f func(mode string)) AuditSinkOption {
	return func(s *AuditSink) { s.degraded = f }
}

// NewAuditSink builds a sink whose directory is resolved once, on first use.
func NewAuditSink(dirFn func() (string, error), opts ...AuditSinkOption) *AuditSink {
	s := &AuditSink{
		fs: osFS{}, dirFn: dirFn, now: time.Now, logger: deliverygate.NewRepoLogger(),
		after: time.After, degraded: func(string) {},
		queue: make(chan AuditLine, auditQueueSize), stop: make(chan struct{}), closed: make(chan struct{}),
	}
	for _, o := range opts {
		o(s)
	}
	s.bootTS = s.now().UTC()
	s.bootSeq = "unknown"
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		s.bootID = hex.EncodeToString(b[:])
	}
	return s
}

// open creates the directory and takes the next boot_seq. A fault here is the
// same fault as an unwritable sink.
func (s *AuditSink) open() error {
	s.openOnce.Do(func() {
		base, err := s.dirFn()
		if err != nil {
			s.openErr = err
			return
		}
		s.dir = filepath.Join(base, "audit")
		if err := s.fs.MkdirAll(s.dir, auditDirMode); err != nil {
			s.openErr = err
			return
		}
		seq, err := s.nextBootSeq()
		if err != nil {
			s.openErr = fmt.Errorf("boot_seq: %w", err)
			return
		}
		s.metaMu.Lock()
		s.bootSeq = strconv.FormatInt(seq, 10)
		s.metaMu.Unlock()
	})
	return s.openErr
}

// nextBootSeq increments the persisted counter atomically (temp file, fsync, rename).
func (s *AuditSink) nextBootSeq() (int64, error) {
	path := filepath.Join(s.dir, auditBootSeqName)
	var cur int64
	if b, err := s.fs.ReadFile(path); err == nil {
		cur, _ = strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	}
	next := cur + 1
	tmp := path + ".tmp"
	_ = s.fs.Remove(tmp)
	if err := s.fs.WriteFileSync(tmp, []byte(strconv.FormatInt(next, 10)), auditFileMode); err != nil {
		return 0, err
	}
	if err := s.fs.Rename(tmp, path); err != nil {
		_ = s.fs.Remove(tmp)
		return 0, err
	}
	return next, nil
}

func (s *AuditSink) bootSeqString() string {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	return s.bootSeq
}

func (s *AuditSink) stamp(l AuditLine) AuditLine {
	l.TS = s.now().UTC().Format(time.RFC3339Nano)
	l.BootID = s.bootID
	l.BootTS = s.bootTS.Format(time.RFC3339Nano)
	l.BootSeq = s.bootSeqString()
	return l
}

// Append writes one line durably (write plus fsync) and returns only after the
// fsync. A fault returns ErrAuditFailed with nothing written.
func (s *AuditSink) Append(l AuditLine) error {
	if err := s.open(); err != nil {
		return fmt.Errorf("%w: %v", ErrAuditFailed, err)
	}
	b, err := json.Marshal(s.stamp(l))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAuditFailed, err)
	}
	return s.write(append(b, '\n'))
}

func (s *AuditSink) write(line []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil || s.size+int64(len(line)) > auditRotateBytes {
		if err := s.rotateLocked(int64(len(line))); err != nil {
			return fmt.Errorf("%w: %v", ErrAuditFailed, err)
		}
	}
	if _, err := s.file.Write(line); err != nil {
		return fmt.Errorf("%w: %v", ErrAuditFailed, err)
	}
	if err := s.file.Sync(); err != nil {
		return fmt.Errorf("%w: %v", ErrAuditFailed, err)
	}
	s.size += int64(len(line))
	return nil
}

// rotateLocked opens the file, first shifting .1 -> .2 and the live file -> .1
// when it is full. Any failure leaves nothing written.
func (s *AuditSink) rotateLocked(incoming int64) error {
	path := filepath.Join(s.dir, auditFileName)
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	s.size = 0
	if info, err := s.fs.Stat(path); err == nil {
		s.size = info.Size()
		if s.size+incoming > auditRotateBytes {
			if err := s.shiftRotated(path); err != nil {
				return err
			}
			s.size = 0
		}
	}
	f, err := s.fs.OpenAppend(path, auditFileMode)
	if err != nil {
		return err
	}
	s.file = f
	return nil
}

// shiftRotated renames path.(n-1) to path.n from the oldest down, then the live
// file to path.1, so auditFilesKept files remain.
func (s *AuditSink) shiftRotated(path string) error {
	for i := auditFilesKept - 1; i >= 1; i-- {
		from := path
		if i > 1 {
			from = fmt.Sprintf("%s.%d", path, i-1)
		}
		if _, err := s.fs.Stat(from); err != nil {
			continue
		}
		if err := s.fs.Rename(from, fmt.Sprintf("%s.%d", path, i)); err != nil {
			return fmt.Errorf("rotate %s: %w", from, err)
		}
	}
	return nil
}

// AppendBounded runs Append in a helper goroutine and waits at most 2s, so a
// stalled fsync cannot hold the caller (and with it the flag update mutex). A
// timeout returns ErrAuditTimeout; the abandoned append may still complete
// later, which readers treat as indeterminate.
func (s *AuditSink) AppendBounded(ctx context.Context, l AuditLine) error {
	res := make(chan error, 1)
	go func() { res <- s.Append(l) }()
	select {
	case err := <-res:
		return err
	case <-s.after(auditAppendBound):
		return ErrAuditTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Enqueue is the tightening path: it never waits on the sink mutex or an
// fsync. A full queue or a closed sink writes the fallback record instead.
func (s *AuditSink) Enqueue(l AuditLine) {
	s.startOnce.Do(func() {
		s.drained.Add(1)
		go s.drain()
	})
	select {
	case <-s.closed:
		s.fallback(l, errors.New("sink closed"))
		return
	default:
	}
	select {
	case s.queue <- l:
		if len(s.queue) > 1 {
			s.degraded(auditModeQueued)
		}
	default:
		s.fallback(l, errors.New("queue full"))
	}
}

func (s *AuditSink) drain() {
	defer s.drained.Done()
	for {
		select {
		case l := <-s.queue:
			s.drainOne(l)
		case <-s.stop:
			s.drainRemaining()
			return
		}
	}
}

func (s *AuditSink) drainRemaining() {
	for {
		select {
		case l := <-s.queue:
			s.drainOne(l)
		default:
			return
		}
	}
}

func (s *AuditSink) drainOne(l AuditLine) {
	err := s.open()
	var b []byte
	if err == nil {
		b, err = json.Marshal(s.stamp(l))
	}
	if err == nil {
		err = s.write(append(b, '\n'))
	}
	if err != nil {
		s.fallback(l, err)
	}
}

// Fallback writes the line's fields to the main log as one WARN record and
// counts it; the main log is not tamper-evident, which the ADR states.
func (s *AuditSink) fallback(l AuditLine, cause error) {
	bootID, bootSeq := s.bootID, s.bootSeqString()
	if l.Kind == auditKindBacklogSteer || l.Kind == auditKindGuardBypass {
		s.degraded(auditModeSteerLog)
		s.logger.Warn("steer_audit_degraded",
			"cause", cause.Error(), "kind", l.Kind, "phase", l.Phase, "change_id", l.ChangeID, "outcome", l.Outcome,
			"session_uuid", l.SessionUUID, "session_title", l.SessionTitle, "item_id", l.ItemID,
			"message_len", l.MessageLen, "message_sha256", l.MessageSHA256, "message_preview", l.MessagePreview,
			"boot_id", bootID, "boot_seq", bootSeq, "listener", l.Listener, "peer_addr", l.PeerAddr,
			"peer_loopback", boolPtrString(l.PeerLoopback), "proxied", boolPtrString(l.Proxied),
			"host", l.Host, "origin", l.Origin, "user_agent", l.UserAgent, "auth_mode", l.AuthMode)
		return
	}
	s.degraded(auditModeFallbackLog)
	s.logger.Warn("flag_change_audit_degraded",
		"cause", cause.Error(), "phase", l.Phase, "change_id", l.ChangeID, "flag", l.Flag, "scope", l.Scope,
		"previous", boolPtrString(l.Previous), "new", boolPtrString(l.New), "outcome", l.Outcome, "seq", l.Seq,
		"boot_id", bootID, "boot_seq", bootSeq, "listener", l.Listener, "peer_addr", l.PeerAddr, "host", l.Host,
		"origin", l.Origin, "user_agent", l.UserAgent, "auth_mode", l.AuthMode)
}

func boolPtrString(b *bool) string {
	if b == nil {
		return "unknown"
	}
	return strconv.FormatBool(*b)
}

// Close stops the drain goroutine after it empties the queue and joins it.
func (s *AuditSink) Close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		close(s.stop)
		s.drained.Wait()
		s.mu.Lock()
		if s.file != nil {
			_ = s.file.Close()
			s.file = nil
		}
		s.mu.Unlock()
	})
}
