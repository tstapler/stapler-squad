package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// FilesFlags is the global flag read FileOps needs; production reads the live config.
type FilesFlags interface {
	Editing(ctx context.Context) bool
	RangeRead(ctx context.Context) bool
	DeltaWrite(ctx context.Context) bool
	EditLarge(ctx context.Context) bool
}

type globalFilesFlags struct{}

func (globalFilesFlags) Editing(ctx context.Context) bool    { return FilesEditingEnabled(ctx) }
func (globalFilesFlags) RangeRead(ctx context.Context) bool  { return FilesRangeReadEnabled(ctx) }
func (globalFilesFlags) DeltaWrite(ctx context.Context) bool { return FilesDeltaWriteEnabled(ctx) }
func (globalFilesFlags) EditLarge(ctx context.Context) bool  { return FilesEditLargeEnabled(ctx) }

// FilePolicy holds the collaborators every file operation shares. Guard is read by
// the FileWriter only; Telemetry serves that writer and the search handler.
type FilePolicy struct {
	Workspace WorkspaceProvider
	Flags     FilesFlags
	Telemetry FilesTelemetry
	// Guard gates writes by session (hidden, archived). Nil refuses every write.
	Guard WriteGuard
}

// FileOps is the single home of file policy (confinement, flags, tokens, limits)
// over a FileBackend; wire handlers are thin adapters around it (ADR-036).
type FileOps struct {
	policy  FilePolicy
	backend FileBackend
	epoch   flagEpoch
	// analysis caches background whole-file passes for range reads and StatFile.
	analysis *analysisCache
}

// NewFileOps builds FileOps over a backend; a nil Flags reads the live global flags
// and a nil Telemetry records nothing.
func NewFileOps(policy FilePolicy, backend FileBackend) *FileOps {
	if policy.Flags == nil {
		policy.Flags = globalFilesFlags{}
	}
	if policy.Telemetry == nil {
		policy.Telemetry = noopFilesTelemetry{}
	}
	return &FileOps{policy: policy, backend: backend, analysis: newAnalysisCache(analysisCacheEntries, analysisDefaultParallel)}
}

// opsRequest is a request after workspace lookup and path confinement.
type opsRequest struct {
	Dir       string // worktree directory the root was opened on
	Root      FileRoot
	Resolved  ResolvedPath
	ViaLink   bool
	Workspace session.Workspace
}

// resolve is the only place a request path becomes a ResolvedPath for the backend.
func (o *FileOps) resolve(ref FileRef, mode AccessMode) (*opsRequest, error) {
	ws, dir, err := workspaceDirFor(o.policy.Workspace, ref, mode)
	if err != nil {
		return nil, err
	}
	root, err := o.backend.OpenRoot(dir)
	if err != nil {
		return nil, openRootError(err)
	}
	resolved, viaLink, err := resolveInWorktreeLinks(root, refPathOrDot(ref), mode)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return &opsRequest{Dir: dir, Root: root, Resolved: resolved, ViaLink: viaLink, Workspace: ws}, nil
}

// GetContent serves a whole-file read; text above maxFileSize is refused.
func (o *FileOps) GetContent(ctx context.Context, sessionID, relPath string) (*sessionv1.GetFileContentResponse, error) {
	if err := o.Require(ctx, sessionv1.FileOp_FILE_OP_READ_ALL); err != nil {
		return nil, err
	}
	fr, err := o.readFileFacts(sessionID, relPath)
	if err != nil {
		return nil, err
	}
	if fr.TooLarge {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("file too large (%d bytes); maximum is %d bytes", fr.Size, maxFileSize))
	}
	return fr.contentResponse(), nil
}

// Stat returns the token, size and read-only reason without content.
func (o *FileOps) Stat(ctx context.Context, sessionID, relPath string) (*sessionv1.StatFileResponse, error) {
	return o.StatWith(ctx, &sessionv1.StatFileRequest{SessionId: sessionID, Path: relPath})
}

// StatWith is Stat plus the whole-file analysis fields the request asks for.
func (o *FileOps) StatWith(ctx context.Context, req *sessionv1.StatFileRequest) (*sessionv1.StatFileResponse, error) {
	if err := o.Require(ctx, sessionv1.FileOp_FILE_OP_STAT); err != nil {
		return nil, err
	}
	fr, err := o.readFileFacts(req.SessionId, req.Path)
	if err != nil {
		return nil, err
	}
	resp := &sessionv1.StatFileResponse{
		VersionToken:   fr.Token.String(),
		Size:           fr.Size,
		ReadOnlyReason: fr.Reason,
	}
	if req.WantAnalysis || req.WantSha256 {
		o.fillAnalysis(ctx, resp, fr, req.WantSha256)
	}
	return resp, nil
}

// fileRead is everything one read of a file establishes; GetContent and Stat project it.
type fileRead struct {
	Size        int64
	ContentType string
	IsBinary    bool
	TooLarge    bool
	Truncated   bool
	HasBOM      bool
	LineEnding  sessionv1.LineEnding
	Reason      sessionv1.ReadOnlyReason
	Token       FileVersionToken
	Body        []byte // raw bytes read (at most truncateSize), BOM stripped
	Src         fileSource
}

func (fr *fileRead) contentResponse() *sessionv1.GetFileContentResponse {
	resp := &sessionv1.GetFileContentResponse{
		IsBinary:       fr.IsBinary,
		Size:           fr.Size,
		ContentType:    fr.ContentType,
		VersionToken:   fr.Token.String(),
		LineEnding:     fr.LineEnding,
		HasBom:         fr.HasBOM,
		ReadOnlyReason: fr.Reason,
	}
	if fr.IsBinary {
		return resp
	}
	resp.Content = strings.ToValidUTF8(string(fr.Body), "�")
	resp.Encoding = "utf-8"
	resp.IsTruncated = fr.Truncated
	return resp
}

// readFileFacts resolves, opens and classifies one file. Binary files are never
// read past the sniff window; text above maxFileSize is reported TooLarge unread.
func (o *FileOps) readFileFacts(sessionID, relPath string) (*fileRead, error) {
	if sessionID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if relPath == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is required"))
	}
	rr, err := o.resolve(FileRef{SessionID: sessionID, Path: relPath}, AccessRead)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rr.Root.Close() }()

	info, err := lstatForRead(rr, relPath)
	if err != nil {
		return nil, err
	}
	f, openErr := rr.Root.Open(rr.Resolved)
	if openErr != nil {
		return nil, openReadError(openErr, relPath)
	}
	defer func() { _ = f.Close() }()
	fr, err := classifyOpenFile(f, rr, relPath, info)
	if err != nil {
		return nil, err
	}
	fr.Src = newFileSource(rr, info)
	return fr, nil
}

// classifyOpenFile sniffs, caps and classifies an opened file.
func classifyOpenFile(f FileHandle, rr *opsRequest, relPath string, info os.FileInfo) (*fileRead, error) {
	fr := &fileRead{Size: info.Size()}
	mtimeNs := info.ModTime().UnixNano()
	fr.Token = NonEditableToken(fr.Size, mtimeNs)
	fr.ContentType, fr.IsBinary = sniffContent(f, isKnownTextFile(relPath))
	if fr.IsBinary {
		fr.Reason = sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY
		return fr, nil
	}
	if fr.Size > maxFileSize {
		fr.TooLarge = true
		fr.Reason = sessionv1.ReadOnlyReason_READ_ONLY_REASON_TOO_LARGE
		return fr, nil
	}
	raw, truncated, readErr := readTextCapped(f, fr.Size)
	if readErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read file content"))
	}
	if truncated {
		raw = truncateOnRuneBoundary(raw)
	}
	fr.HasBOM, fr.Body = detectBOM(raw)
	fr.Truncated = truncated
	fr.LineEnding = detectLineEnding(fr.Body)
	fr.Reason = classifyReadOnly(readFacts{Truncated: truncated, ViaLink: rr.ViaLink, Body: fr.Body})
	if fr.Reason == sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE {
		fr.Token = NewFileVersionToken(fr.Size, mtimeNs, raw)
	}
	return fr, nil
}

// lstatForRead stats the resolved path and maps failures to Connect errors.
func lstatForRead(rr *opsRequest, relPath string) (os.FileInfo, error) {
	info, err := rr.Root.Lstat(rr.Resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("file not found: %s", relPath))
		}
		if os.IsPermission(err) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied: %s", relPath))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to stat file"))
	}
	if info.IsDir() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is a directory, not a file"))
	}
	return info, nil
}
