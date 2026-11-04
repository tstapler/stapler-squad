package artifacts

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/scanqueue"
	"github.com/tstapler/stapler-squad/session/scanstate"
)

// keepOffset tells mergeAndPersist to leave the stored scan offset unchanged.
const keepOffset int64 = -1

const (
	maxScannerTokenSize = 10 * 1024 * 1024 // 10 MB, matches tokens/parser.go

	flushInterval = 5 * time.Second
	flushBatch    = 200
)

// StateStore persists scan positions across restarts. *scanstate.Scoped implements it.
type StateStore interface {
	Load(ctx context.Context) (map[string]scanstate.Record, error)
	Save(ctx context.Context, recs []scanstate.Record) error
	Delete(ctx context.Context, paths []string) error
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
	restored   map[string]struct{} // paths loaded from states that the walk has not yet seen
	writer     *scanstate.Writer[trackedPos]
	flushEvery time.Duration

	// titleMu serializes mergeAndPersist calls for the same session title,
	// preventing lost-update races when multiple JSONL files belong to the same session.
	titleMu sync.Map // key: title string, value: *sync.Mutex

	// verified holds titles whose stored blob is known to carry a scan state, so
	// an unchanged transcript needs no re-check (key: title, value: struct{}).
	verified sync.Map

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
		restored:       make(map[string]struct{}),
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
			return scanstate.Record{Position: t.Position, State: encodeOwner(t)}, nil
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
	// verified: the owner's stored blob carried a scan state when this position
	// was recorded, so an unchanged file needs no re-check. Positions persisted
	// before scan states existed lack it and are re-checked once.
	verified bool
}

const verifiedMarker = "\x00verified"

func encodeOwner(t trackedPos) []byte {
	if t.verified {
		return []byte(t.owner + verifiedMarker)
	}
	return []byte(t.owner)
}

func decodeOwner(state []byte) (owner string, verified bool) {
	owner, verified = strings.CutSuffix(string(state), verifiedMarker)
	return owner, verified
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
	t := trackedPos{Position: p, owner: owner, verified: ae.stateVerified(owner)}
	ae.posMu.Lock()
	ae.positions[filePath] = t
	delete(ae.restored, filePath) // freshly scanned: not a stale restored row
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
		cur, ok := ae.positions[path]
		if ok && cur.Size != 0 {
			continue
		}
		// The blob (written synchronously) can be ahead of the batched position
		// write; resuming from the older persisted offset would re-extract and
		// duplicate what the blob already holds.
		if ok && cur.Offset > rec.Offset {
			continue
		}
		owner, verified := decodeOwner(rec.State)
		ae.positions[path] = trackedPos{Position: rec.Position, owner: owner, verified: verified}
		ae.restored[path] = struct{}{}
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
// newOffset == keepOffset keeps the stored offset: a subagent transcript's own
// offset must not replace the parent conversation's, which SeedOffsets reads.
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
	if newOffset == keepOffset {
		newOffset = existing.ScanOffsetBytes
	}
	blob := &SessionArtifactsBlob{
		PRURLs:          dedup(append(existing.PRURLs, newPRURLs...)),
		CommitSHAs:      dedup(append(existing.CommitSHAs, newCommitSHAs...)),
		ExternalURLs:    capNewest(dedup(append(existing.ExternalURLs, newExternalURLs...)), maxExternalURLs),
		Commands:        capNewest(dedupCommands(append(existing.Commands, newCommands...)), maxCommands),
		ScanOffsetBytes: newOffset,
		LastScannedAt:   time.Now().UTC(),
	}
	blob.ScanState = ScanStateNoneFound
	if blob.HasData() {
		blob.ScanState = ScanStateHasData
	}
	return blob
}

func (ae *ArtifactExtractor) walkAndEnqueue(ctx context.Context, historyDir string) {
	if historyDir == "" {
		return
	}
	walkClean := true
	err := filepath.WalkDir(historyDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			walkClean = false // part of the tree is unreadable: absence proves nothing
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".jsonl") {
			ae.posMu.Lock()
			delete(ae.restored, path)
			ae.posMu.Unlock()
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
		return
	}
	if walkClean && ctx.Err() == nil {
		ae.dropUnseenRestored(ctx)
	}
}

// dropUnseenRestored forgets persisted positions of transcripts the (complete) walk did not find.
func (ae *ArtifactExtractor) dropUnseenRestored(ctx context.Context) {
	ae.posMu.Lock()
	gone := make([]string, 0, len(ae.restored))
	for path := range ae.restored {
		gone = append(gone, path)
		delete(ae.positions, path)
	}
	ae.restored = make(map[string]struct{})
	ae.posMu.Unlock()
	if len(gone) > 0 && ae.states != nil {
		if err := ae.states.Delete(ctx, gone); err != nil {
			log.Warn("[ArtifactExtractor] delete stale scan state failed", "count", len(gone), "err", err)
		}
	}
}

// dedupCommands drops repeats (a rescan after truncation or an owner change
// re-extracts commands the blob already holds), keeping first occurrences.
func dedupCommands(cs []CommandArtifact) []CommandArtifact {
	seen := make(map[CommandArtifact]struct{}, len(cs))
	out := make([]CommandArtifact, 0, len(cs))
	for _, c := range cs {
		if _, ok := seen[c]; !ok {
			seen[c] = struct{}{}
			out = append(out, c)
		}
	}
	return out
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

// capNewest keeps the last n items: callers append in discovery order, so the tail is the newest.
func capNewest[T any](items []T, n int) []T {
	if len(items) > n {
		return items[len(items)-n:]
	}
	return items
}

func isSubagentFile(filePath string) bool {
	return strings.HasPrefix(filepath.Base(filePath), "agent-")
}

// parentTranscript maps a subagent transcript to its parent conversation file.
// Claude Code nests them as <project>/<conv-uuid>/subagents/agent-X.jsonl; older
// layouts put agent-X.jsonl beside the parent and name the parent in the
// first entry's sessionId.
func parentTranscript(filePath string) (string, bool) {
	dir := filepath.Dir(filePath)
	if filepath.Base(dir) == "subagents" {
		convDir := filepath.Dir(dir)
		return convDir + ".jsonl", true
	}
	f, err := os.Open(filePath) // #nosec G304 -- same enumerated transcript path scanFile opens
	if err != nil {
		return "", false
	}
	defer f.Close()
	var first struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(io.LimitReader(f, maxScannerTokenSize)).Decode(&first); err != nil || first.SessionID == "" {
		return "", false
	}
	return filepath.Join(dir, first.SessionID+".jsonl"), true
}

// ownerOf resolves the session that owns filePath; sub reports a subagent
// transcript, whose findings belong to its parent conversation's session.
func (ae *ArtifactExtractor) ownerOf(filePath string) (title string, sub, ok bool) {
	if !isSubagentFile(filePath) {
		title, ok = ae.lookupTitle(filePath)
		return title, false, ok
	}
	parent, ok := parentTranscript(filePath)
	if !ok {
		return "", true, false
	}
	title, ok = ae.lookupTitle(parent)
	return title, true, ok
}

func (ae *ArtifactExtractor) stateVerified(title string) bool {
	if title == "" {
		return false
	}
	_, ok := ae.verified.Load(title)
	return ok
}

// stateRecorded reports whether title's stored blob already carries a usable
// scan state, remembering a positive answer so the check runs once per title.
func (ae *ArtifactExtractor) stateRecorded(title string) bool {
	if ae.stateVerified(title) {
		return true
	}
	raw, err := ae.readFn(title)
	if err != nil || raw == "" {
		return false
	}
	var b SessionArtifactsBlob
	if json.Unmarshal([]byte(raw), &b) != nil || b.ScanState == "" || b.ScanState == ScanStateFailed {
		return false
	}
	ae.verified.Store(title, struct{}{})
	return true
}

// markFailed records a failed scan while keeping the last good data. Best
// effort: a failure to record it is only logged.
func (ae *ArtifactExtractor) markFailed(filePath, title string) {
	ae.verified.Delete(title)
	ae.posMu.Lock()
	if t, ok := ae.positions[filePath]; ok {
		t.verified = false
		ae.positions[filePath] = t
	}
	ae.posMu.Unlock()
	mu := ae.titleLock(title)
	mu.Lock()
	defer mu.Unlock()
	existing := &SessionArtifactsBlob{}
	if raw, err := ae.readFn(title); err == nil && raw != "" {
		if json.Unmarshal([]byte(raw), existing) != nil {
			existing = &SessionArtifactsBlob{}
		}
	}
	failed := *existing
	failed.ScanState = ScanStateFailed
	failed.LastScannedAt = time.Now().UTC()
	encoded, err := json.Marshal(&failed)
	if err != nil {
		return
	}
	if err := ae.storeFn(title, string(encoded)); err != nil {
		log.Warn("[ArtifactExtractor] persist failed state failed", "session", title, "err", err)
		return
	}
	ae.OnScanComplete(title, &failed)
}
