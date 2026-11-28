package fileops

import (
	"context"
	"fmt"
	"os"
	"strings"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/fsops"
	"github.com/tstapler/stapler-squad/session/domain"
)

// FilesFlags is the global flag read FileOps needs; production reads the live config.
type FilesFlags interface {
	Editing(ctx context.Context) bool
	RangeRead(ctx context.Context) bool
	DeltaWrite(ctx context.Context) bool
	EditLarge(ctx context.Context) bool
}

// GlobalFilesFlags reads the Files flags from the live global config.
type GlobalFilesFlags struct{ reader flagsvc.FlagReader }

// NewGlobalFilesFlags reads the flags' defaults from reg.
func NewGlobalFilesFlags(reg *featureflags.Registry) GlobalFilesFlags {
	return GlobalFilesFlags{reader: flagsvc.FlagReader{Reg: reg}}
}

func (f GlobalFilesFlags) enabled(name string) bool {
	return f.reader.Enabled(config.LoadConfig(), name)
}

func (f GlobalFilesFlags) Editing(context.Context) bool {
	return f.enabled(flagsvc.FilesEditingFlagName)
}
func (f GlobalFilesFlags) ViewerV2(context.Context) bool {
	return f.enabled(flagsvc.FilesViewerV2FlagName)
}
func (f GlobalFilesFlags) MergePhone(context.Context) bool {
	return f.enabled(flagsvc.FilesMergePhoneFlagName)
}
func (f GlobalFilesFlags) RangeRead(context.Context) bool {
	return f.enabled(flagsvc.FilesRangeReadFlagName)
}
func (f GlobalFilesFlags) DeltaWrite(context.Context) bool {
	return f.enabled(flagsvc.FilesDeltaWriteFlagName)
}
func (f GlobalFilesFlags) EditLarge(context.Context) bool {
	return f.enabled(flagsvc.FilesEditLargeFlagName)
}

// FilePolicy holds the collaborators every file operation shares. Guard is read by
// the FileWriter only; Telemetry serves that writer and the search handler.
type FilePolicy struct {
	Workspace fsops.WorkspaceProvider
	Flags     FilesFlags
	Telemetry fsops.FilesTelemetry
	// Guard gates writes by session (hidden, archived). Nil refuses every write.
	Guard WriteGuard
}

// FileOps is the single home of file policy (confinement, flags, tokens, limits)
// over a FileBackend; wire handlers are thin adapters around it (ADR-036).
type FileOps struct {
	policy  FilePolicy
	backend fsops.FileBackend
	epoch   flagEpoch
	// analysis caches background whole-file passes for range reads and StatFile.
	analysis *analysisCache
}

// NewFileOps builds FileOps over a backend; a nil Flags reads the live global flags through the default FlagRegistry
// and a nil Telemetry records nothing.
func NewFileOps(policy FilePolicy, backend fsops.FileBackend) *FileOps {
	if policy.Flags == nil {
		policy.Flags = NewGlobalFilesFlags(flagsvc.FlagRegistry())
	}
	if policy.Telemetry == nil {
		policy.Telemetry = fsops.NoopFilesTelemetry{}
	}
	return &FileOps{policy: policy, backend: backend, analysis: newAnalysisCache(analysisCacheEntries, analysisDefaultParallel)}
}

// opsRequest is a request after workspace lookup and path confinement.
type opsRequest struct {
	Dir       string // worktree directory the root was opened on
	Root      fsops.FileRoot
	Resolved  fsops.ResolvedPath
	ViaLink   bool
	Workspace domain.Workspace
}

// resolve is the only place a request path becomes a ResolvedPath for the backend.
func (o *FileOps) resolve(ref fsops.FileRef, mode fsops.AccessMode) (*opsRequest, error) {
	ws, dir, err := fsops.WorkspaceDirFor(o.policy.Workspace, ref, mode)
	if err != nil {
		return nil, err
	}
	root, err := o.backend.OpenRoot(dir)
	if err != nil {
		return nil, fsops.OpenRootError(err)
	}
	resolved, viaLink, err := fsops.ResolveInWorktreeLinks(root, fsops.RefPathOrDot(ref), mode)
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
	Token       fsops.FileVersionToken
	Body        []byte // raw bytes read (at most fsops.TruncateSize), BOM stripped
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
	rr, err := o.resolve(fsops.FileRef{SessionID: sessionID, Path: relPath}, fsops.AccessRead)
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
func classifyOpenFile(f fsops.FileHandle, rr *opsRequest, relPath string, info os.FileInfo) (*fileRead, error) {
	fr := &fileRead{Size: info.Size()}
	mtimeNs := info.ModTime().UnixNano()
	fr.Token = fsops.NonEditableToken(fr.Size, mtimeNs)
	fr.ContentType, fr.IsBinary = fsops.SniffContent(f, fsops.IsKnownTextFile(relPath))
	if fr.IsBinary {
		fr.Reason = sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY
		return fr, nil
	}
	if fr.Size > maxFileSize {
		fr.TooLarge = true
		fr.Reason = sessionv1.ReadOnlyReason_READ_ONLY_REASON_TOO_LARGE
		return fr, nil
	}
	raw, truncated, readErr := fsops.ReadTextCapped(f, fr.Size)
	if readErr != nil {
		return nil, fsops.InternalIOError("read_content", rr.Resolved, readErr, "failed to read file content")
	}
	if truncated {
		raw = fsops.TruncateOnRuneBoundary(raw)
	}
	fr.HasBOM, fr.Body = fsops.DetectBOM(raw)
	fr.Truncated = truncated
	fr.LineEnding = fsops.DetectLineEnding(fr.Body)
	fr.Reason = fsops.ClassifyReadOnly(fsops.ReadFacts{Truncated: truncated, ViaLink: rr.ViaLink, Body: fr.Body})
	if fr.Reason == sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE {
		fr.Token = fsops.NewFileVersionToken(fr.Size, mtimeNs, raw)
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
		return nil, fsops.InternalIOError("stat", rr.Resolved, err, "failed to stat file")
	}
	if info.IsDir() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("path is a directory, not a file"))
	}
	return info, nil
}
