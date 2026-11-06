package services

import (
	"bytes"
	"errors"
	"io"
	"sort"
)

// ErrLineOutOfRange reports a line number past the end of the analyzed file.
var ErrLineOutOfRange = errors.New("line out of range") //nolint:gochecknoglobals

const lineScanChunkBytes = 64 * 1024

// LineIndex is a sparse line index: Checkpoints[k] is the raw byte offset where
// line k*Interval (0-based) starts. Checkpoints[0] is always 0.
type LineIndex struct {
	Interval    int
	Checkpoints []int64
}

// checkpointFor returns the nearest checkpoint at or before line.
func (ix *LineIndex) checkpointFor(line int64) (startLine, offset int64) {
	k := line / int64(ix.Interval)
	if k >= int64(len(ix.Checkpoints)) {
		k = int64(len(ix.Checkpoints)) - 1
	}
	return k * int64(ix.Interval), ix.Checkpoints[k]
}

// LineToByte returns the raw byte offset where 0-based line starts; line ==
// LineCount yields Size. At most Interval lines are scanned from a checkpoint.
func (a *Analysis) LineToByte(r io.ReaderAt, line int64) (int64, error) {
	if line < 0 || line > a.LineCount {
		return 0, ErrLineOutOfRange
	}
	if line == a.LineCount {
		return a.Size, nil
	}
	cpLine, off := a.Index.checkpointFor(line)
	buf := make([]byte, lineScanChunkBytes)
	for cpLine < line {
		n, err := r.ReadAt(buf[:min(int64(len(buf)), a.Size-off)], off)
		if n == 0 && err != nil {
			return 0, err
		}
		chunk := buf[:n]
		for cpLine < line {
			i := bytes.IndexByte(chunk, '\n')
			if i < 0 {
				break
			}
			chunk = chunk[i+1:]
			off += int64(i) + 1
			cpLine++
		}
		if cpLine < line {
			off += int64(len(chunk))
		}
	}
	return off, nil
}

// ByteToLine returns the 0-based line containing raw offset off (the number of
// newline bytes before it); off must lie in [0, Size].
func (a *Analysis) ByteToLine(r io.ReaderAt, off int64) (int64, error) {
	if off < 0 || off > a.Size {
		return 0, ErrLineOutOfRange
	}
	k := sort.Search(len(a.Index.Checkpoints), func(i int) bool { return a.Index.Checkpoints[i] > off }) - 1
	line := int64(k) * int64(a.Index.Interval)
	pos := a.Index.Checkpoints[k]
	buf := make([]byte, lineScanChunkBytes)
	for pos < off {
		n, err := r.ReadAt(buf[:min(int64(len(buf)), off-pos)], pos)
		if n == 0 && err != nil {
			return 0, err
		}
		line += int64(bytes.Count(buf[:n], []byte{'\n'}))
		pos += int64(n)
	}
	return line, nil
}
