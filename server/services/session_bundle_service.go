package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"connectrpc.com/connect"
	kzip "github.com/klauspost/compress/zip"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/artifacts"
	"github.com/tstapler/stapler-squad/session/contexthistory"
)

// BundleStorage is the narrow persistence interface SessionBundleService needs
// to assemble a RUM session diagnostic bundle. *session.Storage satisfies this
// interface; tests inject a fake.
type BundleStorage interface {
	// ListAnalyticsBySessionID returns up to 2000 analytics events for the
	// given session UUID, ordered by created_at DESC.
	ListAnalyticsBySessionID(ctx context.Context, sessionID string) ([]session.AnalyticsData, error)
	// GetItemSessionBySessionUUID returns the most-recent backlog item session
	// linked to this session, or session.ErrNotFound when none exists.
	GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (session.ItemSessionSummary, error)
	// GetSessionArtifactsByUUID returns the artifact blob, or (nil, nil) when none.
	GetSessionArtifactsByUUID(ctx context.Context, sessionUUID string) (*artifacts.SessionArtifactsBlob, error)
}

// ContextHistoryReader is the narrow contexthistory interface SessionBundleService
// needs. *contexthistory.Store satisfies this interface; nil is safe and disables
// the context_history section. Tests inject a fake.
type ContextHistoryReader interface {
	SummarizeSession(ctx context.Context, uuid string) (contexthistory.SessionSummary, error)
	Series(ctx context.Context, uuid string) ([]contexthistory.Sample, error)
	Compactions(ctx context.Context, uuid string) ([]contexthistory.Compaction, error)
}

// ScrollbackReader is the narrow scrollback interface SessionBundleService needs.
// *scrollback.ScrollbackManager satisfies this interface; tests inject a fake.
type ScrollbackReader interface {
	// FlushSession drains any unflushed in-memory scrollback to disk so a
	// subsequent GetRecentLines call sees the full tail.
	FlushSession(sessionUUID string) error
	// GetRecentLines returns the most-recent n lines of scrollback.
	GetRecentLines(sessionID string, n int) ([]byte, error)
}

// BundleManifest describes the contents of a RUM session diagnostic bundle ZIP.
type BundleManifest struct {
	BundleFormatVersion int              `json:"bundle_format_version"`
	SessionUUID         string           `json:"session_uuid"`
	GeneratedAt         time.Time        `json:"generated_at"`
	ServerVersion       string           `json:"server_version"`
	ServerOS            string           `json:"server_os"`
	IncludedSections    []string         `json:"included_sections"`
	OmittedSections     []omittedSection `json:"omitted_sections"`
	Errors              []string         `json:"errors,omitempty"`
}

type omittedSection struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// SessionBundleService backs the SessionBundleService RPCs (RUM session
// diagnostic bundle export). ExportSessionBundle streams the ZIP to the
// browser in fixed-size BundleChunk messages; SaveSessionBundle writes it to
// a server-side directory and returns the path + SHA-256 content hash.
type SessionBundleService struct {
	storage      BundleStorage
	ctxHistory   ContextHistoryReader // may be nil when analytics disabled
	scrollback   ScrollbackReader
	poller       *session.ReviewQueuePoller
	extDiscovery *session.ExternalSessionDiscovery
	// exportSem caps concurrent in-memory ZIP assemblies at 3 to bound OOM risk
	// (assembleBundle materialises the full ZIP before streaming).
	exportSem chan struct{}
}

// NewSessionBundleService creates a SessionBundleService. ctxHistory may be
// nil (context_history section is omitted). Wire poller / extDiscovery after
// construction via SetPoller / SetExternalDiscovery.
func NewSessionBundleService(storage BundleStorage, ctxHistory ContextHistoryReader, scrollback ScrollbackReader) *SessionBundleService {
	return &SessionBundleService{
		storage:    storage,
		ctxHistory: ctxHistory,
		scrollback: scrollback,
		exportSem:  make(chan struct{}, 3),
	}
}

// SetPoller wires the live-instance poller for session lookup.
func (s *SessionBundleService) SetPoller(p *session.ReviewQueuePoller) {
	s.poller = p
}

// SetExternalDiscovery wires external session discovery (mux-enabled sessions).
func (s *SessionBundleService) SetExternalDiscovery(disc *session.ExternalSessionDiscovery) {
	s.extDiscovery = disc
}

// findInstance mirrors DiagnosticService.findInstance's poller ->
// external-discovery fallback chain.
func (s *SessionBundleService) findInstance(id string) *session.Instance {
	if s.poller != nil {
		if inst := s.poller.FindInstance(id); inst != nil {
			return inst
		}
	}
	if s.extDiscovery != nil {
		if inst := s.extDiscovery.GetSession(id); inst != nil {
			return inst
		}
	}
	return nil
}

// sectionEntry holds a named section's uncompressed bytes for ZIP assembly.
type sectionEntry struct {
	name string
	data []byte
}

// sectionFileName maps logical section names to their ZIP entry filenames.
var sectionFileName = map[string]string{
	"session_metadata": "session_metadata.json",
	"server_info":      "server_info.json",
	"backlog_linkage":  "backlog_linkage.json",
	"scrollback":       "scrollback.txt",
	"analytics":        "analytics.json",
	"context_history":  "context_history.json",
	"artifacts":        "artifacts.json",
}

// dropOrder is the reverse-priority order for dropping sections when the
// total uncompressed size exceeds maxUncompressedBytes (45 MB). Sections
// listed first are dropped first.
var dropOrder = []string{
	"analytics",
	"scrollback",
	"context_history",
	"artifacts",
	"backlog_linkage",
}

const maxUncompressedBytes = 45 * 1024 * 1024 // 45 MB

// assembleBundle collects all data sources for the given session into a ZIP.
// inst must not be nil. maxScrollbackLines is capped at 500 internally.
func (s *SessionBundleService) assembleBundle(
	ctx context.Context,
	inst *session.Instance,
	maxScrollbackLines int,
) ([]byte, BundleManifest, error) {
	snap := inst.Snapshot()
	sessionUUID := snap.UUID

	manifest := BundleManifest{
		BundleFormatVersion: 1,
		SessionUUID:         sessionUUID,
		GeneratedAt:         time.Now().UTC(),
		ServerVersion:       runtime.Version(),
		ServerOS:            runtime.GOOS,
	}

	var sections []sectionEntry
	var errs []string

	addSection := func(name string, data []byte) {
		sections = append(sections, sectionEntry{name: name, data: data})
		manifest.IncludedSections = append(manifest.IncludedSections, name)
	}
	omit := func(name, reason string) {
		manifest.OmittedSections = append(manifest.OmittedSections, omittedSection{Name: name, Reason: reason})
	}

	// Step 1: Flush scrollback so the subsequent read sees the full tail.
	// Non-fatal: log at DEBUG and continue.
	if s.scrollback != nil {
		if err := s.scrollback.FlushSession(sessionUUID); err != nil {
			slog.Debug("session bundle: flush scrollback", "uuid", sessionUUID, "err", err)
		}
	}

	// Step 2: Session metadata (required).
	metaPayload := struct {
		ID          string    `json:"id"`
		UUID        string    `json:"uuid"`
		Title       string    `json:"title"`
		Status      int       `json:"status"`
		Program     string    `json:"program"`
		Path        string    `json:"path"`
		Branch      string    `json:"branch"`
		Category    string    `json:"category"`
		Tags        []string  `json:"tags"`
		AutoYes     bool      `json:"auto_yes"`
		AutoApprove bool      `json:"auto_approve"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
	}{
		ID:          snap.ID,
		UUID:        snap.UUID,
		Title:       snap.Title,
		Status:      int(snap.Status),
		Program:     snap.Program,
		Path:        snap.Path,
		Branch:      snap.Branch,
		Category:    snap.Category,
		Tags:        snap.Tags,
		AutoYes:     snap.AutoYes,
		AutoApprove: snap.AutoApprove,
		CreatedAt:   snap.CreatedAt,
		UpdatedAt:   snap.UpdatedAt,
	}
	if metaBytes, err := json.MarshalIndent(metaPayload, "", "  "); err == nil {
		addSection("session_metadata", metaBytes)
	} else {
		errs = append(errs, fmt.Sprintf("session_metadata: %v", err))
	}
	if ctx.Err() != nil {
		return nil, BundleManifest{}, ctx.Err()
	}

	// Step 3: Server info (required).
	serverInfo := struct {
		OS        string `json:"os"`
		GoVersion string `json:"go_version"`
		PID       int    `json:"pid"`
	}{
		OS:        runtime.GOOS,
		GoVersion: runtime.Version(),
		PID:       os.Getpid(),
	}
	if infoBytes, err := json.MarshalIndent(serverInfo, "", "  "); err == nil {
		addSection("server_info", infoBytes)
	} else {
		errs = append(errs, fmt.Sprintf("server_info: %v", err))
	}
	if ctx.Err() != nil {
		return nil, BundleManifest{}, ctx.Err()
	}

	// Step 4: Backlog linkage (omittable).
	if s.storage != nil {
		if item, err := s.storage.GetItemSessionBySessionUUID(ctx, sessionUUID); err == nil {
			if data, merr := json.MarshalIndent(item, "", "  "); merr == nil {
				addSection("backlog_linkage", data)
			} else {
				omit("backlog_linkage", fmt.Sprintf("marshal error: %v", merr))
			}
		} else {
			omit("backlog_linkage", fmt.Sprintf("not available: %v", err))
		}
	} else {
		omit("backlog_linkage", "storage unavailable")
	}
	if ctx.Err() != nil {
		return nil, BundleManifest{}, ctx.Err()
	}

	// Step 5: Scrollback tail (omittable).
	// GetRecentLines returns the most-recent N lines — NOT GetScrollback which
	// returns the oldest N lines (ascending from seq=0).
	if s.scrollback != nil {
		lines := maxScrollbackLines
		if lines <= 0 || lines > 500 {
			lines = 500
		}
		if scrollBytes, err := s.scrollback.GetRecentLines(sessionUUID, lines); err == nil {
			addSection("scrollback", scrollBytes)
		} else {
			omit("scrollback", fmt.Sprintf("read error: %v", err))
		}
	} else {
		omit("scrollback", "scrollback reader unavailable")
	}
	if ctx.Err() != nil {
		return nil, BundleManifest{}, ctx.Err()
	}

	// Step 6: Analytics (omittable).
	if s.storage != nil {
		if analytics, err := s.storage.ListAnalyticsBySessionID(ctx, sessionUUID); err == nil {
			if data, merr := json.MarshalIndent(analytics, "", "  "); merr == nil {
				addSection("analytics", data)
			} else {
				omit("analytics", fmt.Sprintf("marshal error: %v", merr))
			}
		} else {
			omit("analytics", fmt.Sprintf("query error: %v", err))
		}
	} else {
		omit("analytics", "storage unavailable")
	}
	if ctx.Err() != nil {
		return nil, BundleManifest{}, ctx.Err()
	}

	// Step 7: Context history (omittable; nil-guard on ctxHistory).
	if s.ctxHistory != nil {
		summary, err := s.ctxHistory.SummarizeSession(ctx, sessionUUID)
		if err == nil {
			series, serErr := s.ctxHistory.Series(ctx, sessionUUID)
			if serErr == nil {
				compactions, compErr := s.ctxHistory.Compactions(ctx, sessionUUID)
				if compErr == nil {
					ctxHistPayload := struct {
						Summary     contexthistory.SessionSummary `json:"summary"`
						Series      []contexthistory.Sample       `json:"series"`
						Compactions []contexthistory.Compaction   `json:"compactions"`
					}{Summary: summary, Series: series, Compactions: compactions}
					if data, merr := json.MarshalIndent(ctxHistPayload, "", "  "); merr == nil {
						addSection("context_history", data)
					} else {
						omit("context_history", fmt.Sprintf("marshal error: %v", merr))
					}
				} else {
					omit("context_history", fmt.Sprintf("compactions query error: %v", compErr))
				}
			} else {
				omit("context_history", fmt.Sprintf("series query error: %v", serErr))
			}
		} else {
			omit("context_history", fmt.Sprintf("summary query error: %v", err))
		}
	} else {
		omit("context_history", "context history store unavailable (analytics disabled or store not wired)")
	}
	if ctx.Err() != nil {
		return nil, BundleManifest{}, ctx.Err()
	}

	// Step 8: Session artifacts (omittable).
	if s.storage != nil {
		if artBlob, err := s.storage.GetSessionArtifactsByUUID(ctx, sessionUUID); err == nil {
			if artBlob != nil {
				if data, merr := json.MarshalIndent(artBlob, "", "  "); merr == nil {
					addSection("artifacts", data)
				} else {
					omit("artifacts", fmt.Sprintf("marshal error: %v", merr))
				}
			} else {
				omit("artifacts", "no artifacts recorded for this session")
			}
		} else {
			omit("artifacts", fmt.Sprintf("query error: %v", err))
		}
	} else {
		omit("artifacts", "storage unavailable")
	}
	if ctx.Err() != nil {
		return nil, BundleManifest{}, ctx.Err()
	}

	// Step 9: Size cap — drop omittable sections in reverse priority order
	// until total uncompressed bytes <= 45 MB.
	totalUncompressed := func() int {
		n := 0
		for _, sec := range sections {
			n += len(sec.data)
		}
		return n
	}
	for totalUncompressed() > maxUncompressedBytes {
		dropped := false
		for _, name := range dropOrder {
			for i, sec := range sections {
				if sec.name == name {
					sections = append(sections[:i], sections[i+1:]...)
					for j, n := range manifest.IncludedSections {
						if n == name {
							manifest.IncludedSections = append(manifest.IncludedSections[:j], manifest.IncludedSections[j+1:]...)
							break
						}
					}
					manifest.OmittedSections = append(manifest.OmittedSections, omittedSection{
						Name:   name,
						Reason: "dropped: total uncompressed size exceeded 45 MB",
					})
					dropped = true
					break
				}
			}
			if dropped {
				break
			}
		}
		if !dropped {
			// Nothing left to drop — exit to avoid infinite loop.
			break
		}
	}

	manifest.Errors = errs

	// Step 10: Write ZIP using kzip (klauspost/compress/zip).
	var buf bytes.Buffer
	zw := kzip.NewWriter(&buf)

	// manifest.json always first.
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, BundleManifest{}, fmt.Errorf("marshal manifest: %w", err)
	}
	mw, err := zw.Create("manifest.json")
	if err != nil {
		return nil, BundleManifest{}, fmt.Errorf("create manifest.json in zip: %w", err)
	}
	if _, err := mw.Write(manifestBytes); err != nil {
		return nil, BundleManifest{}, fmt.Errorf("write manifest.json: %w", err)
	}

	// Per-section files.
	for _, sec := range sections {
		fname, ok := sectionFileName[sec.name]
		if !ok {
			fname = sec.name + ".bin"
		}
		fw, err := zw.Create(fname)
		if err != nil {
			return nil, BundleManifest{}, fmt.Errorf("create %s in zip: %w", fname, err)
		}
		if _, err := fw.Write(sec.data); err != nil {
			return nil, BundleManifest{}, fmt.Errorf("write %s: %w", fname, err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, BundleManifest{}, fmt.Errorf("close zip: %w", err)
	}

	return buf.Bytes(), manifest, nil
}

// acquireExportSlot caps concurrent in-memory ZIP assemblies (export and save
// share the cap, since both hold the whole archive in memory).
func (s *SessionBundleService) acquireExportSlot() (release func(), err error) {
	select {
	case s.exportSem <- struct{}{}:
		return func() { <-s.exportSem }, nil
	default:
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("too many concurrent bundle exports"))
	}
}

// ExportSessionBundle assembles a RUMSessionBundle ZIP and streams it to the
// browser as fixed-size 64 KB BundleChunk messages. The final chunk has
// Final=true.
//
// +api: session:export-bundle
func (s *SessionBundleService) ExportSessionBundle(
	ctx context.Context,
	req *connect.Request[sessionv1.ExportSessionBundleRequest],
	stream *connect.ServerStream[sessionv1.BundleChunk],
) error {
	release, err := s.acquireExportSlot()
	if err != nil {
		return err
	}
	defer release()

	inst := s.findInstance(req.Msg.SessionUuid)
	if inst == nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionUuid))
	}

	maxLines := int(req.Msg.MaxScrollbackLines)
	if maxLines <= 0 {
		maxLines = 500
	} else if maxLines > 500 {
		maxLines = 500
	}

	zipBytes, _, err := s.assembleBundle(ctx, inst, maxLines)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return connect.NewError(connect.CodeCanceled, err)
		}
		return connect.NewError(connect.CodeInternal, fmt.Errorf("assemble bundle: %w", err))
	}

	const chunkSize = 64 * 1024
	totalChunks := (len(zipBytes) + chunkSize - 1) / chunkSize
	if totalChunks == 0 {
		totalChunks = 1
	}

	for chunkIdx := 0; chunkIdx < totalChunks; chunkIdx++ {
		start := chunkIdx * chunkSize
		end := start + chunkSize
		if end > len(zipBytes) {
			end = len(zipBytes)
		}
		chunk := zipBytes[start:end]
		final := chunkIdx == totalChunks-1
		if err := stream.Send(&sessionv1.BundleChunk{
			Data:       chunk,
			ChunkIndex: int32(chunkIdx),
			Final:      final,
		}); err != nil {
			return err
		}
	}

	return nil
}

// SaveSessionBundle assembles a RUMSessionBundle ZIP, writes it atomically to
// the output_dir (or ~/.stapler-squad/diagnostics/ by default), and returns
// the file path, size, and SHA-256 content hash.
//
// +api: session:save-bundle
func (s *SessionBundleService) SaveSessionBundle(
	ctx context.Context,
	req *connect.Request[sessionv1.SaveSessionBundleRequest],
) (*connect.Response[sessionv1.SaveSessionBundleResponse], error) {
	release, err := s.acquireExportSlot()
	if err != nil {
		return nil, err
	}
	defer release()

	inst := s.findInstance(req.Msg.SessionUuid)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionUuid))
	}

	// Resolve output directory.
	outputDir := req.Msg.OutputDir
	if outputDir == "" {
		configDir, err := config.GetConfigDir()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("could not determine config dir: %w", err))
		}
		outputDir = filepath.Join(configDir, "diagnostics")
	} else {
		// Path traversal prevention: output_dir must be under the config dir.
		configDir, err := config.GetConfigDir()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("could not determine config dir: %w", err))
		}
		absDir, err := filepath.Abs(filepath.Clean(outputDir))
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid output_dir: %w", err))
		}
		// Resolve symlinks on both sides (nearest existing ancestor for absDir) so a
		// symlink inside the config dir cannot redirect the write elsewhere.
		if resolved, rerr := filepath.EvalSymlinks(configDir); rerr == nil {
			configDir = resolved
		}
		absDir = resolveExistingPrefix(absDir)
		// Ensure prefix check includes a separator so /foo/bar doesn't match /foo/baz.
		if !strings.HasPrefix(absDir+string(filepath.Separator), configDir+string(filepath.Separator)) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("output_dir must be within the server config directory"))
		}
		outputDir = absDir
	}

	// Ensure diagnostics directory exists.
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create diagnostics dir: %w", err))
	}

	// Check available disk space (100 MB minimum).
	if free, err := diskFreeBytes(outputDir); err != nil {
		slog.Warn("session bundle: disk space check failed", "dir", outputDir, "err", err)
		// Non-fatal: a real write failure will surface below.
	} else if free < 100*1024*1024 {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("diagnostics directory has less than 100MB free"))
	}

	zipBytes, _, err := s.assembleBundle(ctx, inst, 500)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, connect.NewError(connect.CodeCanceled, err)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("assemble bundle: %w", err))
	}

	// Compute SHA-256 content hash.
	h := sha256.New()
	h.Write(zipBytes)
	contentHash := hex.EncodeToString(h.Sum(nil))

	// Build filename: <slug>-<YYYYMMDD-HHMMSS>.zip.
	snap := inst.Snapshot()
	slug := bundleSlugify(snap.Title)
	if slug == "" {
		slug = snap.UUID
	}
	ts := time.Now().UTC().Format("20060102-150405")
	filename := slug + "-" + ts + "-" + shortID(snap.UUID) + ".zip"

	// Atomic write: write to .tmp, then rename.
	tmpPath := filepath.Join(outputDir, filename+".tmp")
	finalPath := filepath.Join(outputDir, filename)

	if err := os.WriteFile(tmpPath, zipBytes, 0o640); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write bundle tmp file: %w", err))
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("rename bundle file: %w", err))
	}

	slog.Info("session bundle saved", "path", finalPath, "size", len(zipBytes), "hash", contentHash)

	return connect.NewResponse(&sessionv1.SaveSessionBundleResponse{
		FilePath:      finalPath,
		FileSizeBytes: int64(len(zipBytes)),
		ContentHash:   contentHash,
	}), nil
}

// resolveExistingPrefix evaluates symlinks on the longest existing ancestor of
// path and re-appends the not-yet-created remainder.
func resolveExistingPrefix(path string) string {
	rest := ""
	for p := path; ; p = filepath.Dir(p) {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(resolved, rest)
		}
		if filepath.Dir(p) == p {
			return path
		}
		rest = filepath.Join(filepath.Base(p), rest)
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// RunDiagnosticsRetention deletes *.zip files in dir whose mtime is older than
// maxAge. Returns nil (logging at DEBUG) if dir does not exist.
func RunDiagnosticsRetention(dir string, maxAge time.Duration) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Debug("diagnostics retention: directory does not exist, skipping", "dir", dir)
			return nil
		}
		return fmt.Errorf("read diagnostics dir: %w", err)
	}
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			slog.Warn("diagnostics retention: stat file", "file", e.Name(), "err", err)
			continue
		}
		age := now.Sub(info.ModTime())
		if age > maxAge {
			path := filepath.Join(dir, e.Name())
			if removeErr := os.Remove(path); removeErr != nil {
				slog.Warn("diagnostics retention: remove file", "file", path, "err", removeErr)
			} else {
				slog.Info("diagnostics retention: removed stale bundle", "file", path, "age", age.Round(time.Hour))
			}
		}
	}
	return nil
}

// ExportBundleForTest calls assembleBundle and exposes it for package-level tests
// that cannot call the private method directly. It is not part of the public API.
func (s *SessionBundleService) ExportBundleForTest(ctx context.Context, inst *session.Instance, maxScrollbackLines int) ([]byte, BundleManifest, error) {
	return s.assembleBundle(ctx, inst, maxScrollbackLines)
}

// bundleSlugify converts s to a lowercase alphanumeric-plus-hyphen string
// suitable for use in a filename, at most 40 characters long.
func bundleSlugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 && rune(b.String()[b.Len()-1]) != '-' {
			b.WriteByte('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > 40 {
		result = result[:40]
	}
	return result
}
