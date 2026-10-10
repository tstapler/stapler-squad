package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/server/deliverygate"
)

const (
	statsFileName       = "delivery-gate-stats.json"
	statsFileMaxBytes   = 1 << 20
	statsFileMode       = 0o600
	foreignWriterWindow = 2 * time.Minute
	statsWarnEvery      = 10 * time.Minute
)

// FileStatsStore is the deliverygate.StatsStore backed by
// <config dir>/delivery-gate-stats.json. It never blocks the publish path and
// never fails startup: every fault ends in a WARN and Status().Writable=false.
type FileStatsStore struct {
	fs     fsOps
	dir    string
	pid    int
	now    func() time.Time
	logger *slog.Logger

	saveMu   sync.Mutex // serializes Save (writer tick and shutdown flush share the temp file)
	mu       sync.Mutex // guards status and warnAt
	status   deliverygate.StatsFileStatus
	warnedAt map[string]time.Time
}

// StatsStoreOption configures NewFileStatsStore.
type StatsStoreOption func(*FileStatsStore)

// WithStatsFS injects the file system (fault-injecting fake in tests).
func WithStatsFS(f fsOps) StatsStoreOption { return func(s *FileStatsStore) { s.fs = f } }

// WithStatsClock injects the clock.
func WithStatsClock(now func() time.Time) StatsStoreOption {
	return func(s *FileStatsStore) { s.now = now }
}

// WithStatsPID injects the writer pid.
func WithStatsPID(pid int) StatsStoreOption { return func(s *FileStatsStore) { s.pid = pid } }

// WithStatsLogger injects the logger (default: the repo logger).
func WithStatsLogger(l *slog.Logger) StatsStoreOption {
	return func(s *FileStatsStore) { s.logger = l }
}

// NewFileStatsStore builds a store over dir, the config directory resolved once
// by the caller (the test-mode directory when STAPLER_SQUAD_TEST_DIR is set).
func NewFileStatsStore(dir string, opts ...StatsStoreOption) *FileStatsStore {
	s := &FileStatsStore{
		fs: osFS{}, dir: dir, pid: os.Getpid(), now: time.Now, logger: deliverygate.NewRepoLogger(),
		warnedAt: map[string]time.Time{},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *FileStatsStore) path() string { return filepath.Join(s.dir, statsFileName) }

func (s *FileStatsStore) tmpPath() string {
	return fmt.Sprintf("%s.%d.tmp", s.path(), s.pid)
}

// warnEvery logs at most one WARN per key per statsWarnEvery.
func (s *FileStatsStore) warnEvery(key, msg string, args ...any) {
	s.mu.Lock()
	last, ok := s.warnedAt[key]
	now := s.now()
	if ok && now.Sub(last) < statsWarnEvery {
		s.mu.Unlock()
		return
	}
	s.warnedAt[key] = now
	s.mu.Unlock()
	s.logger.Warn(msg, args...)
}

func (s *FileStatsStore) setStatus(f func(*deliverygate.StatsFileStatus)) {
	s.mu.Lock()
	f(&s.status)
	s.mu.Unlock()
}

// Status reports the file state for the RPC.
func (s *FileStatsStore) Status() deliverygate.StatsFileStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func statsChecksum(p deliverygate.PersistedStats) (string, error) {
	p.Checksum = ""
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Load removes stale temp files, then reads and verifies the file. Any fault
// (oversize, invalid JSON, unknown version, bad checksum) quarantines it and
// returns false so the process starts empty.
func (s *FileStatsStore) Load() (deliverygate.PersistedStats, bool) {
	s.removeStaleTemps()
	info, err := s.fs.Stat(s.path())
	if err != nil {
		return deliverygate.PersistedStats{}, false
	}
	if info.Size() > statsFileMaxBytes {
		s.quarantine("file over 1 MiB")
		return deliverygate.PersistedStats{}, false
	}
	data, err := s.fs.ReadFile(s.path())
	if err != nil {
		s.warnEvery("read", "delivery_gate_stats_read_failed", "err", err)
		return deliverygate.PersistedStats{}, false
	}
	var p deliverygate.PersistedStats
	if err := json.Unmarshal(data, &p); err != nil {
		s.quarantine("invalid JSON")
		return deliverygate.PersistedStats{}, false
	}
	if p.Version != deliverygate.StatsFileVersion {
		s.quarantine(fmt.Sprintf("unknown version %d", p.Version))
		return deliverygate.PersistedStats{}, false
	}
	if want, err := statsChecksum(p); err != nil || want != p.Checksum {
		s.quarantine("checksum mismatch")
		return deliverygate.PersistedStats{}, false
	}
	s.setStatus(func(st *deliverygate.StatsFileStatus) { st.Loaded = true })
	return p, true
}

func (s *FileStatsStore) removeStaleTemps() {
	matches, _ := s.fs.Glob(s.path() + ".*.tmp")
	for _, m := range matches {
		if err := s.fs.Remove(m); err != nil {
			s.warnEvery("tmp:"+m, "delivery_gate_stats_stale_temp_undeletable", "path", m, "err", err)
		}
	}
}

// quarantine renames the bad file aside, keeping at most one such file.
func (s *FileStatsStore) quarantine(why string) {
	old, _ := s.fs.Glob(s.path() + ".corrupt-*")
	for _, o := range old {
		_ = s.fs.Remove(o)
	}
	dst := s.path() + ".corrupt-" + s.now().UTC().Format("20060102T150405Z")
	if err := s.fs.Rename(s.path(), dst); err != nil {
		s.logger.Warn("delivery_gate_stats_corrupt", "reason", why, "quarantine_failed", err)
	} else {
		s.logger.Warn("delivery_gate_stats_corrupt", "reason", why, "moved_to", dst)
	}
	s.setStatus(func(st *deliverygate.StatsFileStatus) { st.Quarantined = true })
}

// errForeignWriter reports that another live process wrote the file recently.
var errForeignWriter = errors.New("delivery gate stats file is being written by another process")

// ForeignWriterActive reports whether the file was written by a different pid within
// foreignWriterWindow.
func (s *FileStatsStore) ForeignWriterActive() bool {
	data, err := s.fs.ReadFile(s.path())
	if err != nil {
		return false
	}
	var p deliverygate.PersistedStats
	if json.Unmarshal(data, &p) != nil {
		return false
	}
	return p.WriterPID != 0 && p.WriterPID != s.pid && s.now().Sub(p.WrittenAt) < foreignWriterWindow
}

// Save writes atomically: pid-named temp file, fsync, rename, mode 0600. A
// foreign recent writer makes it a no-op that returns errForeignWriter.
func (s *FileStatsStore) Save(p deliverygate.PersistedStats) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if s.ForeignWriterActive() {
		s.warnEvery("foreign", "delivery_gate_stats_foreign_writer",
			"hint", "another process wrote the stats file within 2 minutes; keeping stats in memory")
		return errForeignWriter
	}
	p.Version = deliverygate.StatsFileVersion
	p.WriterPID = s.pid
	p.WrittenAt = s.now().UTC()
	sum, err := statsChecksum(p)
	if err != nil {
		return err
	}
	p.Checksum = sum
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := s.writeAtomic(data); err != nil {
		s.setStatus(func(st *deliverygate.StatsFileStatus) { st.Writable = false })
		s.warnEvery("write", "delivery_gate_stats_write_failed", "path", s.path(), "err", err)
		return err
	}
	s.setStatus(func(st *deliverygate.StatsFileStatus) { st.Writable = true })
	return nil
}

func (s *FileStatsStore) writeAtomic(data []byte) error {
	tmp := s.tmpPath()
	_ = s.fs.Remove(tmp)
	if err := s.fs.WriteFileSync(tmp, data, statsFileMode); err != nil {
		_ = s.fs.Remove(tmp)
		return err
	}
	if err := s.fs.Rename(tmp, s.path()); err != nil {
		_ = s.fs.Remove(tmp)
		return err
	}
	return nil
}
