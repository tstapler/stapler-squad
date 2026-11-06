package gitconfig

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrNotRegular reports a path that is not a regular file (a directory, FIFO or device).
var ErrNotRegular = errors.New("not a regular file")

// ErrTooLarge reports a file over the caller's size limit.
var ErrTooLarge = errors.New("file too large")

// ReadRegularFile reads at most max bytes of a regular file, deciding "regular" and the size on
// the open descriptor so a path swapped to a FIFO or a growing file between a stat and the read
// cannot block or exhaust memory. A missing file returns an error satisfying errors.Is(fs.ErrNotExist).
func ReadRegularFile(path string, max int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|openFlags, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	if fi.Size() > max {
		return nil, fmt.Errorf("%w: %d bytes, limit %d", ErrTooLarge, fi.Size(), max)
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%w: limit %d", ErrTooLarge, max)
	}
	return data, nil
}
