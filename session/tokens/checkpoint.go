package tokens

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/scanstate"
)

// Checkpoint is a resumable parser position in one transcript: the parse state
// after the last newline-terminated line, plus the file position it covers.
// It is immutable once returned; resuming works on a clone.
type Checkpoint struct {
	state *parseState
	Pos   scanstate.Position
}

// Result is the ParseResult as of the checkpoint (excluding any unterminated tail line).
func (c *Checkpoint) Result() *ParseResult { return c.state.result }

// Encode serializes the parser state for persistence alongside Pos.
func (c *Checkpoint) Encode() ([]byte, error) { return c.state.encode() }

// DecodeCheckpoint restores a Checkpoint persisted via Encode.
func DecodeCheckpoint(blob []byte, pos scanstate.Position) (*Checkpoint, error) {
	st, err := decodeParseState(blob)
	if err != nil {
		return nil, err
	}
	st.finalize()
	return &Checkpoint{state: st, Pos: pos}, nil
}

// ParseFileFrom parses filePath, resuming from prev when the file only grew
// since prev was taken; a shrunk, truncated, replaced or rewritten file (see
// scanstate.Position.Resumable) is reparsed from byte 0. The returned result
// equals ParseFile's, except that a final line with no terminating newline is
// held back until its newline arrives (it may be mid-write) — then included in
// the result, but not in the returned checkpoint. The result is a fresh
// snapshot, never mutated afterwards; prev is not modified.
func (p *Parser) ParseFileFrom(filePath string, prev *Checkpoint) (*ParseResult, *Checkpoint, error) {
	f, err := os.Open(filePath) // #nosec G304 -- filePath comes from an internal directory walk over historyDir, not user input
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}

	st := newParseState()
	var start int64
	if prev != nil && prev.Pos.Resumable(f, fi.Size()) {
		st, start = prev.state.clone(), prev.Pos.Offset
	}
	if _, err := f.Seek(start, 0); err != nil {
		return nil, nil, err
	}

	consumed, tail, scanErr := scanstate.ScanComplete(f, maxScannerTokenSize, func(line []byte) { p.feed(st, line) })
	if scanErr != nil && !errors.Is(scanErr, bufio.ErrTooLong) {
		return nil, nil, scanErr
	}
	pos, err := scanstate.Capture(f, fi, start+consumed)
	if err != nil {
		return nil, nil, err
	}

	p.stamp(st, filePath, fi.ModTime())
	cp := &Checkpoint{state: st, Pos: pos}
	st.finalize()
	if len(tail) == 0 {
		return st.result, cp, nil
	}
	// Publish checkpoint+tail without letting the tail into the checkpoint.
	withTail := st.clone()
	p.feed(withTail, tail)
	return withTail.finalize(), cp, nil
}

// stamp sets the per-file fields ParseFile derives from the path and stat.
func (p *Parser) stamp(st *parseState, filePath string, modTime time.Time) {
	st.result.FileModTime = modTime
	st.result.ParsedAt = time.Now()
	if base := filepath.Base(filePath); strings.HasSuffix(base, ".jsonl") {
		st.result.SessionUUID = strings.TrimSuffix(base, ".jsonl")
	}
	st.result.ProjectPath = decodeProjectDirName(filepath.Base(filepath.Dir(filePath)))
}
