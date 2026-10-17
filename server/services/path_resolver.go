package services

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// maxSymlinkHops bounds symlink expansion so a link cycle cannot loop forever.
const maxSymlinkHops = 40

// AccessMode says whether a file request reads or writes.
type AccessMode int

const (
	AccessRead AccessMode = iota
	AccessWrite
)

// ResolvedPath is a slash-separated, root-relative path that resolveInWorktree
// has confined to the worktree: no "..", no absolute form, no ".git" component
// and no symlink that leaves the root. "." names the root itself.
type ResolvedPath string

// SymlinkDeniedError reports a symlink whose target leaves the worktree or
// points into .git. Target is the link's raw Readlink string (metadata only).
type SymlinkDeniedError struct {
	Link   string
	Target string
}

func (e *SymlinkDeniedError) Error() string {
	return fmt.Sprintf("symlink %q points outside the worktree or into .git (target %q)", e.Link, e.Target)
}

// symlinkDenied builds the PermissionDenied error for a refused link, carrying
// the typed ReadOnlyReasonDetail (SYMLINK plus the raw target string).
func symlinkDenied(link, target string) *connect.Error {
	ce := connect.NewError(connect.CodePermissionDenied, &SymlinkDeniedError{Link: link, Target: target})
	if d, err := connect.NewErrorDetail(&sessionv1.ReadOnlyReasonDetail{
		Reason:     sessionv1.ReadOnlyReason_READ_ONLY_REASON_SYMLINK,
		LinkTarget: target,
	}); err == nil {
		ce.AddDetail(d)
	}
	return ce
}

// WorktreeRoot confines filesystem access to one worktree directory via os.Root.
type WorktreeRoot struct {
	dir  string
	root *os.Root
}

// OpenWorktreeRoot opens dir as a confinement root. The caller must Close it.
func OpenWorktreeRoot(dir string) (*WorktreeRoot, error) {
	// #nosec G304 -- dir is a session workspace directory from WorkspaceProvider, not request input.
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &WorktreeRoot{dir: dir, root: r}, nil
}

// Close releases the underlying directory handle.
func (w *WorktreeRoot) Close() error { return w.root.Close() }

// Dir returns the absolute directory the root was opened on.
func (w *WorktreeRoot) Dir() string { return w.dir }

// Open opens a resolved path for reading, confined by os.Root.
func (w *WorktreeRoot) Open(p ResolvedPath) (*os.File, error) { return w.root.Open(string(p)) }

// Lstat stats a resolved path without following a final symlink.
func (w *WorktreeRoot) Lstat(p ResolvedPath) (os.FileInfo, error) { return w.root.Lstat(string(p)) }

// Stat stats a resolved path, following symlinks inside the root.
func (w *WorktreeRoot) Stat(p ResolvedPath) (os.FileInfo, error) { return w.root.Stat(string(p)) }

// ReadDir lists a resolved directory through the root.
func (w *WorktreeRoot) ReadDir(p ResolvedPath) ([]os.DirEntry, error) {
	f, err := w.root.Open(string(p))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return f.ReadDir(-1)
}

// resolveInWorktree confines rel to the root. It rejects NUL, absolute and
// escaping paths, any ".git" component (before and after symlink expansion) and
// symlinks that leave the root or point into .git. In-tree symlinks are followed
// in AccessRead mode and refused in AccessWrite mode, so a write can never be
// redirected through a link. A missing leaf is not an error here: the open
// reports NotFound, and a write may create it.
func resolveInWorktree(root *WorktreeRoot, rel string, mode AccessMode) (ResolvedPath, error) {
	resolved, _, err := resolveInWorktreeLinks(root, rel, mode)
	return resolved, err
}

// resolveInWorktreeLinks is resolveInWorktree that also reports whether any
// component was a symlink, which callers surface as ReadOnlyReason=Symlink.
func resolveInWorktreeLinks(root *WorktreeRoot, rel string, mode AccessMode) (ResolvedPath, bool, error) {
	if strings.ContainsRune(rel, 0) {
		return "", false, connect.NewError(connect.CodeInvalidArgument, errors.New("path contains NUL"))
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", false, connect.NewError(connect.CodeInvalidArgument, errors.New("absolute paths are not allowed"))
	}
	clean := path.Clean(filepath.ToSlash(rel))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false, connect.NewError(connect.CodeInvalidArgument, errors.New("path traversal detected"))
	}
	if hasGitComponent(clean) {
		return "", false, connect.NewError(connect.CodePermissionDenied, errors.New(".git is not accessible"))
	}
	return expandLinks(root, clean, mode)
}

func hasGitComponent(p string) bool {
	for _, c := range strings.Split(p, "/") {
		if strings.EqualFold(c, ".git") {
			return true
		}
	}
	return false
}

// expandLinks walks p component by component, replacing each symlink by its
// target (re-checked for escape and .git) until no link remains.
func expandLinks(root *WorktreeRoot, p string, mode AccessMode) (ResolvedPath, bool, error) {
	pending := splitComponents(p)
	var cur []string
	viaLink := false
	for hops := 0; len(pending) > 0; {
		comp := pending[0]
		pending = pending[1:]
		next := append(append([]string(nil), cur...), comp)
		nextPath := strings.Join(next, "/")
		fi, err := root.root.Lstat(nextPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
				return finishResolved(append(next, pending...), viaLink), viaLink, nil
			}
			return "", false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to stat path"))
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			cur = next
			continue
		}
		if mode == AccessWrite {
			return "", false, symlinkDenied(nextPath, "")
		}
		if hops++; hops > maxSymlinkHops {
			return "", false, connect.NewError(connect.CodeInvalidArgument, errors.New("too many symlinks"))
		}
		viaLink = true
		target, err := root.root.Readlink(nextPath)
		if err != nil {
			return "", false, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read symlink"))
		}
		denied := symlinkDenied(nextPath, target)
		if filepath.IsAbs(target) || strings.HasPrefix(target, "/") {
			return "", false, denied
		}
		joined := path.Clean(path.Join(strings.Join(cur, "/"), filepath.ToSlash(target)))
		if joined == ".." || strings.HasPrefix(joined, "../") || hasGitComponent(joined) {
			return "", false, denied
		}
		cur = nil
		pending = append(splitComponents(joined), pending...)
	}
	return finishResolved(cur, viaLink), viaLink, nil
}

func splitComponents(p string) []string {
	if p == "." || p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func finishResolved(parts []string, _ bool) ResolvedPath {
	if len(parts) == 0 {
		return "."
	}
	return ResolvedPath(path.Clean(strings.Join(parts, "/")))
}

// FileRef is a session-relative file reference, parsed once at the proto boundary.
type FileRef struct {
	SessionID string
	Path      string
}

// resolvedRequest is what resolveFileRequest hands to a handler. The caller
// must Close Root.
type resolvedRequest struct {
	Root      *WorktreeRoot
	Resolved  ResolvedPath
	ViaLink   bool
	Workspace session.Workspace
}

// resolveFileRequest is the shared request preamble: workspace lookup, root
// selection per mode, and path confinement. Read uses ExistingDir. Write uses
// ActiveDir and requires it to equal ExistingDir, which blocks the silent
// fallback to the repo root after a worktree has been cleaned up.
func resolveFileRequest(_ context.Context, provider WorkspaceProvider, ref FileRef, mode AccessMode) (*resolvedRequest, error) {
	if ref.SessionID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session_id is required"))
	}
	ws, err := provider.GetWorkspace(ref.SessionID)
	if err != nil {
		return nil, err
	}
	dir := ws.ExistingDir
	if mode == AccessWrite {
		if ws.ActiveDir == "" || ws.ActiveDir != ws.ExistingDir {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("WorktreeMissing: session worktree is not on disk"))
		}
		dir = ws.ActiveDir
	}
	if dir == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("session has no working directory"))
	}
	root, err := OpenWorktreeRoot(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("working directory not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to open working directory"))
	}
	rel := ref.Path
	if rel == "" {
		rel = "."
	}
	resolved, viaLink, err := resolveInWorktreeLinks(root, rel, mode)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return &resolvedRequest{Root: root, Resolved: resolved, ViaLink: viaLink, Workspace: ws}, nil
}

// FS returns an fs.FS rooted at the worktree for walking; it cannot follow a link out of the root.
func (w *WorktreeRoot) FS() fs.FS { return w.root.FS() }

// Readlink reads a symlink's raw target (metadata only) through the root.
func (w *WorktreeRoot) Readlink(rel string) (string, error) { return w.root.Readlink(rel) }
