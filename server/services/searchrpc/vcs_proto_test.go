package searchrpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/vc"
)

// TestFileChangeToProto verifies that fileChangeToProto correctly threads
// every vc.FileChange field — including the Additions/Deletions numstat
// counts added alongside per-file insertion/deletion stats — through to the
// sessionv1.FileChange proto.
func TestFileChangeToProto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   vc.FileChange
		want *sessionv1.FileChange
	}{
		{
			name: "modified file threads additions and deletions",
			in: vc.FileChange{
				Path:      "main.go",
				Status:    vc.FileModified,
				IsStaged:  false,
				Additions: 12,
				Deletions: 3,
			},
			want: &sessionv1.FileChange{
				Path:      "main.go",
				Status:    sessionv1.FileStatus_FILE_STATUS_MODIFIED,
				IsStaged:  false,
				OldPath:   "",
				Additions: 12,
				Deletions: 3,
			},
		},
		{
			name: "staged rename threads old path and stats",
			in: vc.FileChange{
				Path:      "renamed.txt",
				OldPath:   "old.txt",
				Status:    vc.FileRenamed,
				IsStaged:  true,
				Additions: 5,
				Deletions: 1,
			},
			want: &sessionv1.FileChange{
				Path:      "renamed.txt",
				OldPath:   "old.txt",
				Status:    sessionv1.FileStatus_FILE_STATUS_RENAMED,
				IsStaged:  true,
				Additions: 5,
				Deletions: 1,
			},
		},
		{
			name: "untracked file has zero additions/deletions",
			in: vc.FileChange{
				Path:      "new.txt",
				Status:    vc.FileUntracked,
				IsStaged:  false,
				Additions: 0,
				Deletions: 0,
			},
			want: &sessionv1.FileChange{
				Path:      "new.txt",
				Status:    sessionv1.FileStatus_FILE_STATUS_UNTRACKED,
				IsStaged:  false,
				Additions: 0,
				Deletions: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fileChangeToProto(tt.in)

			assert.Equal(t, tt.want.Path, got.Path)
			assert.Equal(t, tt.want.OldPath, got.OldPath)
			assert.Equal(t, tt.want.Status, got.Status)
			assert.Equal(t, tt.want.IsStaged, got.IsStaged)
			assert.Equal(t, tt.want.Additions, got.Additions)
			assert.Equal(t, tt.want.Deletions, got.Deletions)
		})
	}
}
