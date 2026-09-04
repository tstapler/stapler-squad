package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/tstapler/stapler-squad/config"
)

const (
	// suspendedProcessFileName is the JSON file (alongside config.StateFileName
	// and config.InstancesFileName) that durably records every
	// SIGSTOP'd-but-not-yet-resolved external process. This is transient
	// operational state, not domain data, so it deliberately lives in its own
	// config.json-style file rather than the ent DB.
	suspendedProcessFileName = "suspended_processes.json"
	// suspendedProcessLockFileName is the flock coordination file for
	// suspendedProcessFileName, mirroring config.LockFileName.
	suspendedProcessLockFileName = "suspended_processes.lock"
	// suspendedProcessLockTimeout mirrors config.DefaultLockTimeout.
	suspendedProcessLockTimeout = 5 * time.Second
)

// SuspendedProcessRecord durably records an external process this server
// SIGSTOP'd during CommitImportExternalSession, so a server restart can
// reconcile it (see ReconcileSuspendedProcesses) instead of leaving it frozen
// forever.
type SuspendedProcessRecord struct {
	PID          int32                    `json:"pid"`
	CreateTimeMs int64                    `json:"create_time_ms"`
	Candidate    ExternalSessionCandidate `json:"candidate"`
	InstanceID   string                   `json:"instance_id"`
}

// suspendedProcessFile is the on-disk shape of suspended_processes.json.
type suspendedProcessFile struct {
	Records []SuspendedProcessRecord `json:"records"`
}

// SuspendedProcessStore persists SuspendedProcessRecord entries to
// suspended_processes.json using the same exclusive-flock +
// write-tmp-then-rename idiom as config/state.go.
type SuspendedProcessStore struct {
	configDir   string
	lockFile    *flock.Flock
	lockTimeout time.Duration

	// mu provides real intra-process mutual exclusion. flock.Flock guards
	// against other OS processes but a single *flock.Flock value gives no
	// such guarantee against concurrent goroutines within this process --
	// two goroutines can both pass TryLockContext/TryRLockContext (flock is
	// reentrant/no-op within one process on most platforms) and interleave
	// their read-modify-write of the underlying file. mu is taken in
	// addition to, not instead of, the flock calls below.
	mu sync.Mutex
}

// NewSuspendedProcessStore creates a store rooted at the same config
// directory config.GetConfigDir() resolves (workspace-isolated in tests via
// STAPLER_SQUAD_TEST_DIR, matching config/state.go's behavior).
func NewSuspendedProcessStore() (*SuspendedProcessStore, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("suspended process store: failed to get config directory: %w", err)
	}
	lockPath := filepath.Join(configDir, suspendedProcessLockFileName)
	return &SuspendedProcessStore{
		configDir:   configDir,
		lockFile:    flock.New(lockPath),
		lockTimeout: suspendedProcessLockTimeout,
	}, nil
}

func (s *SuspendedProcessStore) path() string {
	return filepath.Join(s.configDir, suspendedProcessFileName)
}

// Add persists a SuspendedProcessRecord under an exclusive lock, using
// upsert semantics: any existing record for the same InstanceID is replaced
// rather than duplicated. CommitImportExternalSession's compensating-delete
// path can retry Add for the same InstanceID (e.g. after a transient write
// failure), and without this dedup a retry would leave two records for one
// instance, which would make ReconcileSuspendedProcesses/Remove behavior
// ambiguous.
func (s *SuspendedProcessStore) Add(record SuspendedProcessRecord) error {
	return s.withWriteLock(func() error {
		file, err := s.readWithoutLocking()
		if err != nil {
			return err
		}
		kept := make([]SuspendedProcessRecord, 0, len(file.Records)+1)
		for _, r := range file.Records {
			if r.InstanceID != record.InstanceID {
				kept = append(kept, r)
			}
		}
		kept = append(kept, record)
		file.Records = kept
		return s.writeWithoutLocking(file)
	})
}

// Remove deletes the record for instanceID, if present. Removing a
// non-existent record is not an error -- callers call this on every
// resume/kill path, some of which race with reconciliation.
func (s *SuspendedProcessStore) Remove(instanceID string) error {
	return s.withWriteLock(func() error {
		file, err := s.readWithoutLocking()
		if err != nil {
			return err
		}
		kept := make([]SuspendedProcessRecord, 0, len(file.Records))
		for _, r := range file.Records {
			if r.InstanceID != instanceID {
				kept = append(kept, r)
			}
		}
		file.Records = kept
		return s.writeWithoutLocking(file)
	})
}

// Get returns the SuspendedProcessRecord for instanceID, if one exists.
func (s *SuspendedProcessStore) Get(instanceID string) (SuspendedProcessRecord, bool, error) {
	records, err := s.List()
	if err != nil {
		return SuspendedProcessRecord{}, false, err
	}
	for _, r := range records {
		if r.InstanceID == instanceID {
			return r, true, nil
		}
	}
	return SuspendedProcessRecord{}, false, nil
}

// List returns every currently-persisted SuspendedProcessRecord.
func (s *SuspendedProcessStore) List() ([]SuspendedProcessRecord, error) {
	var records []SuspendedProcessRecord
	err := s.withReadLock(func() error {
		file, err := s.readWithoutLocking()
		if err != nil {
			return err
		}
		records = file.Records
		return nil
	})
	return records, err
}

func (s *SuspendedProcessStore) withReadLock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), s.lockTimeout)
	defer cancel()
	locked, err := s.lockFile.TryRLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire read lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire read lock within timeout")
	}
	defer func() { _ = s.lockFile.Unlock() }()
	return fn()
}

func (s *SuspendedProcessStore) withWriteLock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), s.lockTimeout)
	defer cancel()
	locked, err := s.lockFile.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire write lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("could not acquire write lock within timeout")
	}
	defer func() { _ = s.lockFile.Unlock() }()
	return fn()
}

func (s *SuspendedProcessStore) readWithoutLocking() (suspendedProcessFile, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return suspendedProcessFile{}, nil
		}
		return suspendedProcessFile{}, fmt.Errorf("failed to read suspended process file: %w", err)
	}
	var file suspendedProcessFile
	if err := json.Unmarshal(data, &file); err != nil {
		return suspendedProcessFile{}, fmt.Errorf("failed to parse suspended process file: %w", err)
	}
	return file, nil
}

func (s *SuspendedProcessStore) writeWithoutLocking(file suspendedProcessFile) error {
	if err := os.MkdirAll(s.configDir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal suspended process file: %w", err)
	}
	targetPath := s.path()
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temporary suspended process file: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to atomically update suspended process file: %w", err)
	}
	return nil
}
