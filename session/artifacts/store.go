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
	"github.com/tstapler/stapler-squad/session/scanqueue"
	"github.com/tstapler/stapler-squad/session/scanstate"
)

const (
	maxScannerTokenSize = 10 * 1024 * 1024 // 10 MB, matches tokens/parser.go

	// stateConsumer names this extractor's rows in the scan-state table.
	stateConsumer = "artifacts"

	flushInterval = 5 * time.Second
	flushBatch    = 200
)

// StateStore persists scan positions across restarts. *scanstate.Scoped implements it.
type StateStore interface {
	Load(ctx context.Context) (map[string]scanstate.Record, error)
	Save(ctx context.Context, recs []scanstate.Record) error
}

// ArtifactExtractor reads Claude Code JSONL files and extracts structured
// artifacts (PR URLs, commit SHAs, external URLs) using an incremental
// byte-offset scan. Mirrors the TokenStore worker pool pattern.
type ArtifactExtractor struct {
	// queue coalesces by path and paces startup backfill; nothing is dropped.
	queue *scanqueue.Queue

	// positions tracks where each file was last scanned to.
	posMu     sync.Mutex
	positions map[string]trackedPos

	states     StateStore
	writer     *scanstate.Writer[trackedPos]
	flushEvery time.Duration

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
	opts ...Option,
) *ArtifactExtractor {
	ae := &ArtifactExtractor{
		positions:      make(map[string]trackedPos),
		flushEvery:     flushInterval,
		storeFn:        storeFn,
		readFn:         readFn,
		lookupTitle:    lookupTitle,
		OnScanComplete: func(_ string, _ *SessionArtifactsBlob) {}, // no-op default
	}
	var qc scanqueue.Config
	for _, o := range opts {
		o(ae, &qc)
	}
	ae.queue = scanqueue.New(qc, ae.scanFile)
	if ae.states != nil {
		ae.writer = scanstate.NewWriter(ae.states, func(_ string, t trackedPos) (scanstate.Record, error) {
			return scanstate.Record{Position: t.Position, State: []byte(t.owner)}, nil
		})
	}
	return ae
}

// Option configures an ArtifactExtractor.
type Option func(*ArtifactExtractor, *scanqueue.Config)

// WithStateStore persists scan positions in s so a restart resumes at the
// appended bytes.
func WithStateStore(s StateStore) Option {
	return func(ae *ArtifactExtractor, _ *scanqueue.Config) { ae.states = s }
}

// WithQueueConfig sets worker count, startup delay and backfill throttle.
func WithQueueConfig(c scanqueue.Config) Option {
	return func(_ *ArtifactExtractor, qc *scanqueue.Config) { *qc = c }
}

// QueueStats reports scan-queue activity (enqueued, coalesced, processed).
func (ae *ArtifactExtractor) QueueStats() scanqueue.Stats { return ae.queue.Stats() }

// FlushState writes every pending scan position to the StateStore.
func (ae *ArtifactExtractor) FlushState(ctx context.Context) error {
	if ae.writer == nil {
		return nil
	}
	return ae.writer.Flush(ctx, 0)
}

// trackedPos is a scan position plus the session title it was scanned for. The
// owner is persisted so a position is never trusted for a different session:
// that session's artifacts would be skipped, not rescanned. Empty owner means
// unknown (a SeedOffsets seed), which is trusted as before.
type trackedPos struct {
	scanstate.Position
	owner string
}

func (ae *ArtifactExtractor) position(filePath string) (scanstate.Position, bool) {
	t, ok := ae.tracked(filePath)
	return t.Position, ok
}

func (ae *ArtifactExtractor) tracked(filePath string) (trackedPos, bool) {
	ae.posMu.Lock()
	defer ae.posMu.Unlock()
	t, ok := ae.positions[filePath]
	return t, ok
}

func (ae *ArtifactExtractor) setPosition(filePath, owner string, p scanstate.Position) {
	t := trackedPos{Position: p, owner: owner}
	ae.posMu.Lock()
	ae.positions[filePath] = t
	ae.posMu.Unlock()
	if ae.writer != nil {
		ae.writer.Mark(filePath, t)
	}
}

// restoreState loads persisted positions; they supersede SeedOffsets' coarser ones.
func (ae *ArtifactExtractor) restoreState(ctx context.Context) {
	if ae.states == nil {
		return
	}
	recs, err := ae.states.Load(ctx)
	if err != nil {
		log.Warn("[ArtifactExtractor] load scan state failed; scanning from scratch", "err", err)
		return
	}
	ae.posMu.Lock()
	for path, rec := range recs {
		// A live scan already recorded a real position (Size > 0), newer than the
		// persisted one; a SeedOffsets entry (no Size) is the coarser one to replace.
		if cur, ok := ae.positions[path]; ok && cur.Size != 0 {
			continue
		}
		ae.positions[path] = trackedPos{Position: rec.Position, owner: string(rec.State)}
	}
	ae.posMu.Unlock()
}

// Start launches worker goroutines and the startup backfill walk.
func (ae *ArtifactExtractor) Start(ctx context.Context, historyDir string) {
	ctx, cancel := context.WithCancel(ctx)
	ae.cancelFunc = cancel

	ae.queue.Start(ctx)
	if ae.writer != nil {
		go ae.writer.Run(ctx, ae.flushEvery, flushBatch, func(err error) {
			log.Warn("[ArtifactExtractor] scan-state flush failed", "err", err)
		})
	}
	go func() {
		ae.restoreState(ctx)
		ae.walkAndEnqueue(ctx, historyDir)
	}()
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

// enqueue schedules a live scan of filePath, coalescing with any pending one.
func (ae *ArtifactExtractor) enqueue(filePath string) { ae.queue.Enqueue(filePath) }

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
	ae.posMu.Lock()
	defer ae.posMu.Unlock()
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
			ae.positions[inst.HistoryFilePath] = trackedPos{Position: scanstate.Position{Offset: blob.ScanOffsetBytes}}
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
			ae.queue.EnqueueBackfill(path)
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
