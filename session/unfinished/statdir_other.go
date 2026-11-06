//go:build !unix

package unfinished

import (
	"os"
	"path/filepath"
	"time"
)

// dirStatter is the portable fallback: a plain os.Lstat per file.
type dirStatter struct{ root string }

func newDirStatter(root string) *dirStatter { return &dirStatter{root: root} }

func (d *dirStatter) close() {}

func (d *dirStatter) lstat(rel string) (size int64, mtime time.Time, err error) {
	info, err := os.Lstat(filepath.Join(d.root, rel))
	if err != nil {
		return 0, time.Time{}, err
	}
	return info.Size(), info.ModTime(), nil
}
