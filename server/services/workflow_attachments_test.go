package services

import (
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeAttachmentFile(t *testing.T, path string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(path, pngMagic(), 0o600))
	return path
}

func TestValidateAttachmentPaths(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	dir := filepath.Join(base, "att")
	require.NoError(t, os.Mkdir(dir, 0o750))
	outside := writeAttachmentFile(t, filepath.Join(base, "secret.png"))
	good := writeAttachmentFile(t, filepath.Join(dir, "good.png"))
	upper := writeAttachmentFile(t, filepath.Join(dir, "UP.JPG"))
	notImage := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(notImage, []byte("hi"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "folder.png"), 0o750))
	linkOut := filepath.Join(dir, "link.png")
	require.NoError(t, os.Symlink(outside, linkOut))
	linkDir := filepath.Join(dir, "linkdir")
	require.NoError(t, os.Symlink(base, linkDir))

	// A symlinked attachment dir must still work: the root is resolved too.
	dirLink := filepath.Join(base, "att-link")
	require.NoError(t, os.Symlink(dir, dirLink))

	tests := []struct {
		name    string
		dir     string
		paths   []string
		wantErr bool
	}{
		{"none", dir, nil, false},
		{"valid", dir, []string{good}, false},
		{"valid uppercase ext", dir, []string{upper}, false},
		{"valid via symlinked root", dirLink, []string{good}, false},
		{"outside dir", dir, []string{outside}, true},
		{"dotdot escape", dir, []string{filepath.Join(dir, "..", "secret.png")}, true},
		{"symlink file escape", dir, []string{linkOut}, true},
		{"symlink dir escape", dir, []string{filepath.Join(linkDir, "secret.png")}, true},
		{"non-image", dir, []string{notImage}, true},
		{"missing", dir, []string{filepath.Join(dir, "gone.png")}, true},
		{"directory named like image", dir, []string{filepath.Join(dir, "folder.png")}, true},
		{"relative", dir, []string{"good.png"}, true},
		{"empty string", dir, []string{""}, true},
		{"the dir itself", dir, []string{dir}, true},
		{"one bad among good", dir, []string{good, outside}, true},
		{"no dir configured", "", []string{good}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ws := &WorkflowService{attachmentDir: tt.dir}
			got, err := ws.validateAttachmentPaths(tt.paths)
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, len(tt.paths))
		})
	}
}

func TestValidateAttachmentPaths_ResolvesSymlinkedFileInsideDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	real := writeAttachmentFile(t, filepath.Join(dir, "real.png"))
	link := filepath.Join(dir, "alias.png")
	require.NoError(t, os.Symlink(real, link))
	ws := &WorkflowService{attachmentDir: dir}
	got, err := ws.validateAttachmentPaths([]string{link})
	require.NoError(t, err)
	resolvedReal, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	assert.Equal(t, []string{resolvedReal}, got)
}

func TestValidateAttachmentPaths_TooMany(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	good := writeAttachmentFile(t, filepath.Join(dir, "a.png"))
	ws := &WorkflowService{attachmentDir: dir}

	atLimit := make([]string, maxWorkflowAttachments)
	for i := range atLimit {
		atLimit[i] = good
	}
	_, err := ws.validateAttachmentPaths(atLimit)
	require.NoError(t, err)

	_, err = ws.validateAttachmentPaths(append(atLimit, good))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
