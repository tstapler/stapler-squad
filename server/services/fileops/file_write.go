package fileops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services/fsops"
)

// The WriteFile validation pipeline (ADR-031). Order: flag, arguments, session guard,
// confinement, token parse, then, under the per-path mutex, the token compare against
// the bytes on disk, the pre-rename re-check and the atomic replace. The sha256 is the
// identity: size and mtime never accept a write on their own.

var errChangedBeforeRename = errors.New("file changed before rename")

type roReason = sessionv1.ReadOnlyReason

// writeCall is the state of one WriteFile call.
type writeCall struct {
	req      *sessionv1.WriteFileRequest
	token    fsops.FileVersionToken
	badToken bool
	eol      fsops.EOLLabel
	bom      fsops.BOMLabel
}

func newWriteCall(req *sessionv1.WriteFileRequest) *writeCall {
	return &writeCall{req: req, eol: fsops.EOLLF, bom: fsops.BOMNo}
}

// diskFile is the current state of the target, read under the per-path mutex.
type diskFile struct {
	Missing  bool
	TooLarge bool
	Raw      []byte
	MtimeNs  int64
}

func (d diskFile) sha256Hex() string {
	sum := sha256.Sum256(d.Raw)
	return hex.EncodeToString(sum[:])
}

// body is the content without its BOM.
func (d diskFile) body() []byte {
	_, body := fsops.DetectBOM(d.Raw)
	return body
}

// classify is the editability of the bytes on disk (NONE when editable).
func (d diskFile) classify() roReason {
	return fsops.ClassifyReadOnly(fsops.ReadFacts{Body: d.body()})
}

func (d diskFile) token() fsops.FileVersionToken {
	return fsops.NewFileVersionToken(int64(len(d.Raw)), d.MtimeNs, d.Raw)
}

// lfText is the body with CRLF folded to LF.
func (d diskFile) lfText() string {
	return strings.ReplaceAll(string(d.body()), "\r\n", "\n")
}

func (w *FileWriter) write(ctx context.Context, call *writeCall) (*sessionv1.WriteFileResponse, error) {
	if !w.ops.policy.Flags.Editing(ctx) {
		return nil, reasonRefusal(connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_FLAG_OFF,
			errors.New("editing disabled"))
	}
	if err := validateWriteArgs(call.req); err != nil {
		return nil, err
	}
	if err := w.checkGuard(call.req.SessionId); err != nil {
		return nil, err
	}
	rr, err := w.ops.resolve(fsops.FileRef{SessionID: call.req.SessionId, Path: call.req.Path}, fsops.AccessWrite)
	if err != nil {
		return nil, typeResolveError(err)
	}
	defer func() { _ = rr.Root.Close() }()

	tok, perr := fsops.ParseFileVersionToken(call.req.ExpectedToken)
	if perr != nil {
		call.badToken = true
		return nil, connect.NewError(connect.CodeInvalidArgument, perr)
	}
	call.token = tok

	unlock := w.locks.Lock(fsops.PathKey{ActiveDir: rr.Workspace.ActiveDir, Path: rr.Resolved})
	defer unlock()
	return w.writeLocked(call, rr)
}

// validateWriteArgs rejects what no valid LF-normalized save can contain, before any I/O.
func validateWriteArgs(req *sessionv1.WriteFileRequest) error {
	switch {
	case req.SessionId == "":
		return connect.NewError(connect.CodeInvalidArgument, errors.New("session_id is required"))
	case req.Path == "":
		return connect.NewError(connect.CodeInvalidArgument, errors.New("path is required"))
	case bytes.IndexByte(req.Content, 0) >= 0:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("content contains NUL"))
	case bytes.IndexByte(req.Content, '\r') >= 0:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("content must be LF-normalized (found CR)"))
	case len(req.Content) > fsops.TruncateSize:
		return reasonRefusal(connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_TOO_LARGE,
			fmt.Errorf("content is %d bytes; the editable maximum is %d", len(req.Content), fsops.TruncateSize))
	}
	return nil
}

// checkGuard applies the session-level gate; a missing guard refuses every write.
func (w *FileWriter) checkGuard(sessionID string) error {
	if w.ops.policy.Guard == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("write guard is not configured"))
	}
	return w.ops.policy.Guard.CheckWriteAccess(sessionID)
}

// typeResolveError attaches the typed reason to resolver refusals that lack one.
func typeResolveError(err error) error {
	switch {
	case errors.Is(err, fsops.ErrGitInternal):
		return reasonRefusal(connect.CodePermissionDenied, sessionv1.ReadOnlyReason_READ_ONLY_REASON_GIT_INTERNAL, fsops.ErrGitInternal)
	case errors.Is(err, fsops.ErrWorktreeMissing):
		return reasonRefusal(connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_WORKTREE_MISSING, fsops.ErrWorktreeMissing)
	}
	return err
}

func (w *FileWriter) writeLocked(call *writeCall, rr *opsRequest) (*sessionv1.WriteFileResponse, error) {
	disk, err := readDisk(rr)
	if err != nil {
		return nil, err
	}
	if !call.token.Editable() {
		return nil, refuseNonEditable(disk)
	}
	if conflict := conflictIfChanged(disk, call.token); conflict != nil {
		return conflict, nil
	}
	if reason := disk.classify(); reason != sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE {
		return nil, reasonRefusal(connect.CodeFailedPrecondition, reason, errors.New("file is not editable"))
	}
	out := call.encodeForDisk(disk)
	rerr := rr.Root.Replace(rr.Resolved, out, func() error { return w.recheck(rr, call.token) })
	switch {
	case errors.Is(rerr, errChangedBeforeRename):
		return w.conflictNow(rr)
	case rerr != nil:
		log.Error("file write failed", "session", call.req.SessionId, "err", rerr)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to write file"))
	}
	return w.saved(call, rr, out)
}

// readDisk reads the target under the cap. A file that vanished is reported, not an error.
func readDisk(rr *opsRequest) (diskFile, error) {
	info, err := rr.Root.Lstat(rr.Resolved)
	if errors.Is(err, fs.ErrNotExist) {
		return diskFile{Missing: true}, nil
	}
	if err != nil {
		return diskFile{}, fsops.InternalIOError("stat", rr.Resolved, err, "failed to stat file")
	}
	if info.IsDir() {
		return diskFile{}, connect.NewError(connect.CodeInvalidArgument, errors.New("path is a directory, not a file"))
	}
	if info.Size() > fsops.TruncateSize {
		return diskFile{TooLarge: true}, nil
	}
	f, err := rr.Root.Open(rr.Resolved)
	if errors.Is(err, fs.ErrNotExist) {
		return diskFile{Missing: true}, nil
	}
	if err != nil {
		return diskFile{}, fsops.InternalIOError("open", rr.Resolved, err, "failed to open file")
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, fsops.TruncateSize+1))
	if err != nil {
		return diskFile{}, fsops.InternalIOError("read", rr.Resolved, err, "failed to read file")
	}
	if len(raw) > fsops.TruncateSize {
		return diskFile{TooLarge: true}, nil
	}
	fi, err := f.Stat()
	if err != nil {
		return diskFile{}, fsops.InternalIOError("fstat", rr.Resolved, err, "failed to stat file")
	}
	return diskFile{Raw: raw, MtimeNs: fi.ModTime().UnixNano()}, nil
}

// refuseNonEditable is the refusal for a well-formed "-" token: the live classification
// names why, with Truncated as the default (the read that produced the token was cut short).
func refuseNonEditable(disk diskFile) error {
	reason := sessionv1.ReadOnlyReason_READ_ONLY_REASON_TRUNCATED
	switch {
	case disk.TooLarge:
		reason = sessionv1.ReadOnlyReason_READ_ONLY_REASON_TOO_LARGE
	case !disk.Missing:
		if live := disk.classify(); live != sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE {
			reason = live
		}
	}
	return reasonRefusal(connect.CodeFailedPrecondition, reason, errors.New("version token is not editable"))
}

// conflictIfChanged returns the Conflict response when disk is no longer the version
// token names, or nil when the hash matches (an mtime-only change is not a conflict).
func conflictIfChanged(disk diskFile, tok fsops.FileVersionToken) *sessionv1.WriteFileResponse {
	if !disk.Missing && !disk.TooLarge && disk.sha256Hex() == tok.SHA256 {
		return nil
	}
	return conflictResponse(disk)
}

// conflictResponse shows the current version only when it can be shown whole and safely.
func conflictResponse(disk diskFile) *sessionv1.WriteFileResponse {
	reason := disk.classify()
	switch {
	case disk.Missing:
		reason = sessionv1.ReadOnlyReason_READ_ONLY_REASON_DELETED
	case disk.TooLarge:
		reason = sessionv1.ReadOnlyReason_READ_ONLY_REASON_TOO_LARGE
	}
	if reason != sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE {
		return &sessionv1.WriteFileResponse{Outcome: &sessionv1.WriteFileResponse_Conflict{
			Conflict: &sessionv1.WriteFileConflict{Theirs: &sessionv1.WriteFileConflict_Unavailable{
				Unavailable: &sessionv1.WriteFileUnavailable{Reason: reason},
			}},
		}}
	}
	return &sessionv1.WriteFileResponse{Outcome: &sessionv1.WriteFileResponse_Conflict{
		Conflict: &sessionv1.WriteFileConflict{Theirs: &sessionv1.WriteFileConflict_FullText{
			FullText: &sessionv1.WriteFileFullText{Content: disk.lfText(), Token: disk.token().String()},
		}},
	}}
}

// recheck is the pre-rename verification: re-read and re-hash, so a content change
// aborts the save while an mtime-only change does not. Residual TOCTOU: ADR-031.
func (w *FileWriter) recheck(rr *opsRequest, tok fsops.FileVersionToken) error {
	if w.preRenameHook != nil {
		w.preRenameHook()
	}
	disk, err := readDisk(rr)
	if err != nil {
		return err
	}
	if conflictIfChanged(disk, tok) != nil {
		return errChangedBeforeRename
	}
	return nil
}

// conflictNow builds the Conflict response from what is on disk after an aborted rename.
func (w *FileWriter) conflictNow(rr *opsRequest) (*sessionv1.WriteFileResponse, error) {
	disk, err := readDisk(rr)
	if err != nil {
		return nil, err
	}
	return conflictResponse(disk), nil
}

// encodeForDisk applies the on-disk file's own BOM and line ending to the LF content,
// which also records them for telemetry.
func (c *writeCall) encodeForDisk(disk diskFile) []byte {
	hasBOM, body := fsops.DetectBOM(disk.Raw)
	crlf := fsops.DetectLineEnding(body) == sessionv1.LineEnding_LINE_ENDING_CRLF
	out := c.req.Content
	if crlf {
		out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
		c.eol = fsops.EOLCRLF
	}
	if hasBOM {
		out = append(append(make([]byte, 0, len(fsops.UTF8BOM)+len(out)), fsops.UTF8BOM...), out...)
		c.bom = fsops.BOMYes
	}
	return out
}

// saved builds the Saved response and evicts the VCS cache once.
func (w *FileWriter) saved(call *writeCall, rr *opsRequest, written []byte) (*sessionv1.WriteFileResponse, error) {
	info, err := rr.Root.Lstat(rr.Resolved)
	if err != nil {
		log.Error("file write: stat after rename failed", "session", call.req.SessionId, "err", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("saved, but the new version could not be read back"))
	}
	if w.invalidator != nil {
		w.invalidator.InvalidateVCSStatus(rr.Workspace.ActiveDir)
	}
	tok := fsops.NewFileVersionToken(int64(len(written)), info.ModTime().UnixNano(), written)
	return &sessionv1.WriteFileResponse{Outcome: &sessionv1.WriteFileResponse_Saved{
		Saved: &sessionv1.WriteFileSaved{Token: tok.String()},
	}}, nil
}

// classify maps a finished call to the shared slog and analytics outcome and reason.
func (c *writeCall) classify(resp *sessionv1.WriteFileResponse, err error) (fsops.WriteOutcome, fsops.WriteReason) {
	switch {
	case err == nil && resp.GetConflict() != nil:
		return fsops.WriteOutcomeConflictRejected, fsops.WriteReasonNone
	case err == nil:
		return fsops.WriteOutcomeOK, fsops.WriteReasonNone
	}
	fallback := fsops.WriteReasonBadContent
	if c.badToken {
		fallback = fsops.WriteReasonBadToken
	}
	reason := writeReasonOf(err, fallback)
	switch {
	case reason == fsops.WriteReasonFlagOff:
		return fsops.WriteOutcomeDisabled, reason
	case connect.CodeOf(err) == connect.CodePermissionDenied:
		return fsops.WriteOutcomeDenied, reason
	case connect.CodeOf(err) == connect.CodeInternal:
		return fsops.WriteOutcomeError, reason
	}
	return fsops.WriteOutcomeRejected, reason
}

var writeReasonByDetail = map[roReason]fsops.WriteReason{
	sessionv1.ReadOnlyReason_READ_ONLY_REASON_FLAG_OFF:         fsops.WriteReasonFlagOff,
	sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY:           fsops.WriteReasonBinary,
	sessionv1.ReadOnlyReason_READ_ONLY_REASON_TRUNCATED:        fsops.WriteReasonTruncated,
	sessionv1.ReadOnlyReason_READ_ONLY_REASON_TOO_LARGE:        fsops.WriteReasonTooLarge,
	sessionv1.ReadOnlyReason_READ_ONLY_REASON_NON_UTF8:         fsops.WriteReasonNonUTF8,
	sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL:        fsops.WriteReasonMixedEOL,
	sessionv1.ReadOnlyReason_READ_ONLY_REASON_SYMLINK:          fsops.WriteReasonSymlink,
	sessionv1.ReadOnlyReason_READ_ONLY_REASON_WORKTREE_MISSING: fsops.WriteReasonWorktreeMissing,
	sessionv1.ReadOnlyReason_READ_ONLY_REASON_HIDDEN_SESSION:   fsops.WriteReasonHiddenSession,
	sessionv1.ReadOnlyReason_READ_ONLY_REASON_GIT_INTERNAL:     fsops.WriteReasonGitInternal,
}

// detailReason reads the typed ReadOnlyReasonDetail off a refusal; no free-form text
// ever reaches a label.
func detailReason(err error) (fsops.WriteReason, bool) {
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return "", false
	}
	for _, d := range ce.Details() {
		v, verr := d.Value()
		if verr != nil {
			continue
		}
		if rd, ok := v.(*sessionv1.ReadOnlyReasonDetail); ok {
			r, found := writeReasonByDetail[rd.Reason]
			return r, found
		}
	}
	return "", false
}

// writeReasonOf picks the label for a refusal; without a typed detail the fallback applies.
func writeReasonOf(err error, fallback fsops.WriteReason) fsops.WriteReason {
	if errors.Is(err, errSessionArchived) {
		return fsops.WriteReasonArchived
	}
	if r, ok := detailReason(err); ok {
		return r
	}
	return fallback
}
