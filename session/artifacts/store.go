package artifacts

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

const (
	artifactWorkerPoolSize = 4
	artifactQueueSize      = 256
	maxScannerTokenSize    = 10 * 1024 * 1024 // 10 MB, matches tokens/parser.go
)

// ArtifactExtractor reads Claude Code JSONL files and extracts structured
// artifacts (PR URLs, commit SHAs, external URLs) using an incremental
// byte-offset scan. Mirrors the TokenStore worker pool pattern.
type ArtifactExtractor struct {
	queue    chan string
	inflight sync.Map // key: filePath, value: struct{}

	// offsets tracks the last scanned byte offset per file.
	offsetsMu sync.Mutex
	offsets   map[string]int64

	// titleMu serializes mergeAndPersist calls for the same session title,
	// preventing lost-update races when multiple JSONL files belong to the same session.
	titleMu sync.Map // key: title string, value: *sync.Mutex

	// storeFn is called with (sessionTitle, jsonBlob) after each successful scan.
	storeFn func(title, blob string) error
	// readFn loads the existing stored blob for a session (for merge on scan).
	readFn func(title string) (string, error)
	// lookupTitle maps a JSONL file path to its session title.
	lookupTitle func(filePath string) (string, bool)
	// OnScanComplete is called after a successful scan with new artifacts.
	// Inject event-bus publish logic here; defaults to no-op.
	// This keeps ArtifactExtractor testable without a live event bus.
	OnScanComplete func(title string, blob *SessionArtifactsBlob)

	cancelFunc context.CancelFunc
}

// NewArtifactExtractor creates an ArtifactExtractor.
// storeFn: persists the JSON blob (wraps storage.UpdateInstanceArtifacts).
// readFn: loads the existing blob (wraps storage.GetInstanceArtifacts).
// lookupTitle: resolves a JSONL path to its session title.
func NewArtifactExtractor(
	storeFn func(title, blob string) error,
	readFn func(title string) (string, error),
	lookupTitle func(filePath string) (string, bool),
) *ArtifactExtractor {
	return &ArtifactExtractor{
		queue:          make(chan string, artifactQueueSize),
		offsets:        make(map[string]int64),
		storeFn:        storeFn,
		readFn:         readFn,
		lookupTitle:    lookupTitle,
		OnScanComplete: func(_ string, _ *SessionArtifactsBlob) {}, // no-op default
	}
}

// Start launches worker goroutines and the startup backfill walk.
func (ae *ArtifactExtractor) Start(ctx context.Context, historyDir string) {
	ctx, cancel := context.WithCancel(ctx)
	ae.cancelFunc = cancel

	for i := 0; i < artifactWorkerPoolSize; i++ {
		go ae.worker(ctx)
	}
	go ae.walkAndEnqueue(ctx, historyDir)
}

// Stop cancels background goroutines.
func (ae *ArtifactExtractor) Stop() {
	if ae.cancelFunc != nil {
		ae.cancelFunc()
	}
}

// OnHistoryFileChanged is the HistoryLinker callback — filters and enqueues.
func (ae *ArtifactExtractor) OnHistoryFileChanged(filePath string) {
	if !strings.HasSuffix(filePath, ".jsonl") {
		return
	}
	if strings.HasPrefix(filepath.Base(filePath), "agent-") {
		return
	}
	ae.enqueue(filePath)
}

func (ae *ArtifactExtractor) enqueue(filePath string) {
	if _, loaded := ae.inflight.LoadOrStore(filePath, struct{}{}); loaded {
		return
	}
	select {
	case ae.queue <- filePath:
	default:
		ae.inflight.Delete(filePath)
		log.Warn("[ArtifactExtractor] queue full, dropping", "path", filePath)
	}
}

func (ae *ArtifactExtractor) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case filePath := <-ae.queue:
			ae.scanFile(filePath)
		}
	}
}

// InstanceInfo provides the minimal session data ArtifactExtractor needs.
// Defined here to avoid a circular import with the session package.
type InstanceInfo struct {
	Title           string
	HistoryFilePath string
}

// SeedOffsets loads each session's existing blob at startup to restore
// the byte offset — so the first scan after restart reads only new bytes.
// Call before Start(). Build InstanceInfo slices in dependencies.go from
// historyLinker.Instances() to avoid a circular import.
func (ae *ArtifactExtractor) SeedOffsets(instances []InstanceInfo) {
	ae.offsetsMu.Lock()
	defer ae.offsetsMu.Unlock()
	for _, inst := range instances {
		if inst.HistoryFilePath == "" {
			continue
		}
		raw, err := ae.readFn(inst.Title)
		if err != nil || raw == "" {
			continue
		}
		var blob SessionArtifactsBlob
		if err := json.Unmarshal([]byte(raw), &blob); err == nil && blob.ScanOffsetBytes > 0 {
			ae.offsets[inst.HistoryFilePath] = blob.ScanOffsetBytes
		}
	}
}

// titleLock returns the per-title mutex for serializing mergeAndPersist calls.
// Using sync.Map.LoadOrStore ensures a single *sync.Mutex is shared across all
// concurrent callers for the same title (C-1 fix).
func (ae *ArtifactExtractor) titleLock(title string) *sync.Mutex {
	mu, _ := ae.titleMu.LoadOrStore(title, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// mergeAndPersist LOADS the existing stored blob via readFn, merges new findings
// with existing artifacts, enforces dedup and caps, then returns the merged blob.
// This ensures prior scan results are never lost on overwrite.
// Serialized per title via titleLock to prevent lost-update races (C-1).
func (ae *ArtifactExtractor) mergeAndPersist(
	title string,
	newOffset int64,
	newPRURLs, newCommitSHAs, newExternalURLs []string,
	newCommands []CommandArtifact,
) *SessionArtifactsBlob {
	mu := ae.titleLock(title)
	mu.Lock()
	defer mu.Unlock()

	existing := &SessionArtifactsBlob{}
	if raw, err := ae.readFn(title); err == nil && raw != "" {
		if umErr := json.Unmarshal([]byte(raw), existing); umErr != nil {
			log.Warn("[ArtifactExtractor] existing blob corrupt, starting fresh", "session", title, "err", umErr)
			existing = &SessionArtifactsBlob{}
		}
	}
	mergedCmds := append(existing.Commands, newCommands...)
	if len(mergedCmds) > maxCommands {
		mergedCmds = mergedCmds[:maxCommands]
	}
	return &SessionArtifactsBlob{
		PRURLs:          dedup(append(existing.PRURLs, newPRURLs...)),
		CommitSHAs:      dedup(append(existing.CommitSHAs, newCommitSHAs...)),
		ExternalURLs:    cap50(dedup(append(existing.ExternalURLs, newExternalURLs...))),
		Commands:        mergedCmds,
		ScanOffsetBytes: newOffset,
		LastScannedAt:   time.Now().UTC(),
	}
}

func (ae *ArtifactExtractor) walkAndEnqueue(ctx context.Context, historyDir string) {
	if historyDir == "" {
		return
	}
	err := filepath.WalkDir(historyDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".jsonl") && !strings.HasPrefix(d.Name(), "agent-") {
			ae.enqueue(path)
		}
		select {
		case <-ctx.Done():
			return filepath.SkipAll
		default:
			return nil
		}
	})
	if err != nil {
		log.Warn("[ArtifactExtractor] walkAndEnqueue error", "dir", historyDir, "err", err)
	}
}

func dedup(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

func cap50(ss []string) []string {
	if len(ss) > maxExternalURLs {
		return ss[:maxExternalURLs]
	}
	return ss
}
