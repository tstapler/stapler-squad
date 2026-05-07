package session

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/linkdata/deadlock"
	"github.com/tstapler/stapler-squad/log"
)

const historyLinkerPollInterval = 5 * time.Second

// HistoryLinker is a background service that correlates running sessions with
// their Claude JSONL history files. It populates Instance.claudeSession.ConversationUUID
// and Instance.HistoryFilePath when a conversation file is detected.
//
// Detection uses two complementary paths:
//   - Polling (every 5 s): scans all running sessions via proc_pidinfo open-files
//   - fsnotify (fast path): watcher callback fires as soon as a new JSONL is created
//
// Both paths call the same correlateSession helper, which is idempotent.
type HistoryLinker struct {
	detector *HistoryFileDetector
	watcher  *HistoryFileWatcher

	mu        deadlock.RWMutex
	instances []*Instance
}

// NewHistoryLinkerFromRealInspector creates a HistoryLinker backed by the real
// gopsutil-based process inspector and an fsnotify watcher on ~/.claude/projects/.
// This is the production constructor; use NewHistoryLinker in tests.
func NewHistoryLinkerFromRealInspector() *HistoryLinker {
	detector := NewHistoryFileDetectorWithRealInspector()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.WarningLog.Printf("HistoryLinker: failed to get home dir, watcher disabled: %v", err)
		return &HistoryLinker{
			detector:  detector,
			instances: make([]*Instance, 0),
		}
	}
	watchDir := filepath.Join(homeDir, ".claude", "projects")

	// Build the linker first so the watcher callback can close over it.
	hl := &HistoryLinker{
		detector:  detector,
		instances: make([]*Instance, 0),
	}
	hl.watcher = NewHistoryFileWatcher(watchDir, func(_ string) {
		hl.ScanAll()
	})
	return hl
}

// Instances returns a snapshot of the currently monitored instances.
// Used by shutdown hooks that need the live set (including externally added sessions).
func (hl *HistoryLinker) Instances() []*Instance {
	hl.mu.RLock()
	defer hl.mu.RUnlock()
	snap := make([]*Instance, len(hl.instances))
	copy(snap, hl.instances)
	return snap
}

// NewHistoryLinker creates a HistoryLinker backed by the given detector and watcher.
// Call SetInstances (or AddInstance) to register sessions before starting.
func NewHistoryLinker(detector *HistoryFileDetector, watcher *HistoryFileWatcher) *HistoryLinker {
	return &HistoryLinker{
		detector:  detector,
		watcher:   watcher,
		instances: make([]*Instance, 0),
	}
}

// SetInstances replaces the full instance list.
func (hl *HistoryLinker) SetInstances(instances []*Instance) {
	hl.mu.Lock()
	defer hl.mu.Unlock()
	hl.instances = instances
}

// AddInstance adds a single instance for monitoring.
func (hl *HistoryLinker) AddInstance(instance *Instance) {
	hl.mu.Lock()
	defer hl.mu.Unlock()
	hl.instances = append(hl.instances, instance)
}

// RemoveInstance stops monitoring the named instance.
func (hl *HistoryLinker) RemoveInstance(title string) {
	hl.mu.Lock()
	defer hl.mu.Unlock()
	filtered := make([]*Instance, 0, len(hl.instances))
	for _, inst := range hl.instances {
		if !inst.MatchesID(title) {
			filtered = append(filtered, inst)
		}
	}
	hl.instances = filtered
}

// Start performs an initial synchronous scan and then runs a background poll
// loop until ctx is cancelled. The fsnotify watcher is also started here so
// that new JSONL files trigger instant correlation.
func (hl *HistoryLinker) Start(ctx context.Context) {
	// Story 1.2.3: initial scan before first poll interval.
	hl.scanAllSessions()

	// Register watcher callback for fast-path detection.
	if hl.watcher != nil {
		if err := hl.watcher.Start(ctx); err != nil {
			log.WarningLog.Printf("HistoryLinker: failed to start watcher: %v", err)
		}
	}

	go hl.run(ctx)
}

// run is the polling loop goroutine.
func (hl *HistoryLinker) run(ctx context.Context) {
	ticker := time.NewTicker(historyLinkerPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hl.scanAllSessions()
		}
	}
}

// ScanAll triggers an immediate correlation pass over all monitored instances.
// Exported for use by HistoryFileWatcher callbacks.
func (hl *HistoryLinker) ScanAll() {
	hl.scanAllSessions()
}

// scanAllSessions iterates all monitored instances and attempts history correlation
// for those with a live tmux session.
func (hl *HistoryLinker) scanAllSessions() {
	hl.mu.RLock()
	snapshot := make([]*Instance, len(hl.instances))
	copy(snapshot, hl.instances)
	hl.mu.RUnlock()

	for _, inst := range snapshot {
		hl.correlateSession(inst)
	}
}

// correlateSession detects a history file for inst and updates its fields if found.
// Skips instances that already have a UUID (idempotent).
func (hl *HistoryLinker) correlateSession(inst *Instance) {
	// Skip if we already know the UUID — avoid unnecessary proc_pidinfo calls.
	if inst.HasClaudeSession() {
		return
	}

	var info *HistoryFileInfo

	// Fast path: inspect open files of the live tmux pane process.
	pid, err := inst.GetPanePID()
	if err == nil {
		info, err = hl.detector.Detect(pid)
		if err != nil {
			log.WarningLog.Printf("HistoryLinker: detect error for '%s' (pid=%d): %v", inst.Title, pid, err)
		}
	}

	// Fallback: scan the project directory by path (works after reboot / tmux kill).
	if info == nil && inst.Path != "" {
		info, err = hl.detector.DetectByPath(inst.Path)
		if err != nil {
			log.WarningLog.Printf("HistoryLinker: path-based detect error for '%s': %v", inst.Title, err)
		}
	}

	if info == nil {
		log.DebugLog.Printf("HistoryLinker: no JSONL found for '%s' (path=%q)", inst.Title, inst.Path)
		return
	}

	log.InfoLog.Printf("HistoryLinker: linked '%s' → conv UUID %s", inst.Title, info.ConversationUUID)
	inst.SetHistoryInfo(info.ConversationUUID, info.HistoryFilePath)
}
