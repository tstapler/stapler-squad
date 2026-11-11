package services

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
)

// maxWorkflowAttachments caps images per run; mirrored client-side in the Run dialog.
const maxWorkflowAttachments = 10

var workflowAttachmentExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
}

// SetAttachmentDir sets the directory RunWorkflow attachment paths must resolve
// into. Unset (empty) rejects every attachment.
func (ws *WorkflowService) SetAttachmentDir(dir string) {
	ws.attachmentDir = dir
}

// validateAttachmentPaths returns the symlink-resolved form of each path, or a
// CodeInvalidArgument error. Each path must be absolute and, after EvalSymlinks,
// lie inside the attachment dir as a regular image file — so `..` segments and
// symlinks cannot reach files elsewhere.
func (ws *WorkflowService) validateAttachmentPaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if len(paths) > maxWorkflowAttachments {
		return nil, invalidAttachment(fmt.Errorf("too many attachments: %d (max %d)", len(paths), maxWorkflowAttachments))
	}
	if ws.attachmentDir == "" {
		return nil, invalidAttachment(errors.New("workflow attachments are not available"))
	}
	resolvedDir, err := filepath.EvalSymlinks(ws.attachmentDir)
	if err != nil {
		return nil, invalidAttachment(fmt.Errorf("attachment directory unavailable: %w", err))
	}
	root := attachmentRoot(resolvedDir)
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		resolved, err := root.resolve(p)
		if err != nil {
			return nil, invalidAttachment(fmt.Errorf("attachment %q: %w", p, err))
		}
		out = append(out, resolved)
	}
	return out, nil
}

// attachmentRoot is the symlink-resolved attachment directory.
type attachmentRoot string

func (root attachmentRoot) resolve(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", errors.New("path must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", errors.New("file not found")
	}
	rel, err := filepath.Rel(string(root), resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("path is outside the attachment directory")
	}
	if !workflowAttachmentExts[strings.ToLower(filepath.Ext(resolved))] {
		return "", errors.New("not an image file")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	return resolved, nil
}

func invalidAttachment(err error) error {
	return connect.NewError(connect.CodeInvalidArgument, err)
}
