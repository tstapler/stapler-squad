package unfinished

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	gitsync "github.com/go-git/go-git/v5/utils/sync"
)

// zeroCommitTime is what object.Commit reports when a committer line is absent
// or its timestamp does not parse.
var zeroCommitTime = time.Time{}.Unix()

// stockParentsReader parses only the header prefix go-git's decoder also
// honours: `tree`, the contiguous `parent` block, an optional `author`, then
// `committer`, stopping at the first blank line or after the committer. Stopping
// by position (never by pattern) keeps ` parent ...` text inside gpgsig
// continuation lines or the message from being read as parents.
type stockParentsReader struct{}

func (stockParentsReader) Parents(s storer.EncodedObjectStorer, h plumbing.Hash, buf []plumbing.Hash) ([]plumbing.Hash, int64, error) {
	o, err := s.EncodedObject(plumbing.CommitObject, h)
	if err != nil {
		return buf[:0], zeroCommitTime, err
	}
	RecordDecodedCommits(1)
	return parseCommitHeader(o, buf)
}

func parseCommitHeader(o plumbing.EncodedObject, buf []plumbing.Hash) (parents []plumbing.Hash, when int64, err error) {
	parents, when = buf[:0], zeroCommitTime
	if o.Type() != plumbing.CommitObject {
		return parents, when, object.ErrUnsupportedObject
	}
	rc, err := o.Reader()
	if err != nil {
		return parents, when, err
	}
	defer func() {
		if cerr := rc.Close(); err == nil {
			err = cerr
		}
	}()
	br := gitsync.GetBufioReader(rc)
	defer gitsync.PutBufioReader(br)
	return scanCommitHeader(&headerReader{r: br}, parents)
}

// scanCommitHeader runs the tree, parents, author, committer sequence.
func scanCommitHeader(hr *headerReader, parents []plumbing.Hash) ([]plumbing.Hash, int64, error) {
	when := zeroCommitTime
	line, eof, err := hr.next()
	if err != nil {
		return parents, when, err
	}
	if len(line) == 0 || isBlankHeader(line) {
		return parents, when, fmt.Errorf("%w: missing tree header", object.ErrMalformedCommit)
	}
	key, data := splitHeaderLine(line)
	var tree plumbing.Hash
	switch {
	case string(key) != "tree":
		return parents, when, fmt.Errorf("%w: tree header must be first", object.ErrMalformedCommit)
	case !parseHexHash(data, &tree):
		return parents, when, fmt.Errorf("%w: bad tree hash", object.ErrMalformedCommit)
	case eof:
		return parents, when, nil
	}

	parents, key, data, done, err := scanParentBlock(hr, parents)
	if done || err != nil {
		return parents, when, err
	}
	if string(key) == "author" {
		if line, _, err = hr.next(); err != nil || len(line) == 0 || isBlankHeader(line) {
			return parents, when, err
		}
		key, data = splitHeaderLine(line)
	}
	if string(key) == "committer" {
		when = committerUnix(data)
	}
	return parents, when, nil
}

// scanParentBlock reads the contiguous parent lines. It returns the first
// non-parent line's header (done=false) or done=true when the header ended.
func scanParentBlock(hr *headerReader, parents []plumbing.Hash) (_ []plumbing.Hash, key, data []byte, done bool, err error) {
	for {
		line, eof, rerr := hr.next()
		if rerr != nil {
			return parents, nil, nil, true, rerr
		}
		if len(line) == 0 || isBlankHeader(line) {
			return parents, nil, nil, true, nil
		}
		key, data = splitHeaderLine(line)
		if string(key) != "parent" {
			return parents, key, data, false, nil
		}
		var p plumbing.Hash
		if !parseHexHash(data, &p) {
			return parents, nil, nil, true, fmt.Errorf("%w: bad parent hash", object.ErrMalformedCommit)
		}
		parents = append(parents, p)
		if eof {
			return parents, nil, nil, true, nil
		}
	}
}

// headerReader reads newline-terminated lines. A line longer than the bufio
// buffer is accumulated in long, which is allocated only in that rare case.
type headerReader struct {
	r    *bufio.Reader
	long []byte
}

// next returns one line (with its newline when present). eof reports that the
// line was the last one, with or without a trailing newline. The result is
// valid until the next call.
func (hr *headerReader) next() (line []byte, eof bool, err error) {
	line, err = hr.r.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		hr.long = append(hr.long[:0], line...)
		for errors.Is(err, bufio.ErrBufferFull) {
			line, err = hr.r.ReadSlice('\n')
			hr.long = append(hr.long, line...)
		}
		line = hr.long
	}
	if errors.Is(err, io.EOF) {
		return line, true, nil
	}
	return line, false, err
}

// isBlankHeader reports the canonical header/body separator: a lone newline.
func isBlankHeader(line []byte) bool { return len(line) == 1 && line[0] == '\n' }

func splitHeaderLine(line []byte) (key, value []byte) {
	trimmed := bytes.TrimRight(line, "\n")
	key, value, _ = bytes.Cut(trimmed, []byte{' '})
	return key, value
}

func parseHexHash(data []byte, out *plumbing.Hash) bool {
	if len(data) != hex.EncodedLen(len(out)) {
		return false
	}
	_, err := hex.Decode(out[:], data)
	return err == nil
}

// committerUnix mirrors object.Signature.Decode for the timestamp only:
// `Name <email> <unix> <tz>`, anything unparseable leaves the zero time.
func committerUnix(b []byte) int64 {
	open, closeIdx := bytes.LastIndexByte(b, '<'), bytes.LastIndexByte(b, '>')
	if open == -1 || closeIdx == -1 || closeIdx < open || closeIdx+2 >= len(b) {
		return zeroCommitTime
	}
	ts := b[closeIdx+2:]
	if space := bytes.IndexByte(ts, ' '); space != -1 {
		ts = ts[:space]
	}
	n, err := strconv.ParseInt(string(ts), 10, 64) // <= 32 bytes: the conversion stays on the stack
	if err != nil {
		return zeroCommitTime
	}
	return n
}
