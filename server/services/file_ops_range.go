package services

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

const (
	// rangeIndexWait is how long a line anchor waits for the background index.
	rangeIndexWait     = 250 * time.Millisecond
	maxRangesPerClient = 4
	nulSniffBytes      = 8000
)

// rangeRead is one ReadRange's resolved file and analysis, shared by its helpers.
type rangeRead struct {
	rr       *opsRequest
	file     FileHandle
	info     os.FileInfo
	src      fileSource
	analysis *Analysis // nil until the background pass is ready
	state    sessionv1.FileAnalysisState
}

func rangeOp(req *sessionv1.ReadFileRangeRequest) sessionv1.FileOp {
	if req.GetBytes() != nil {
		return sessionv1.FileOp_FILE_OP_READ_BYTES
	}
	return sessionv1.FileOp_FILE_OP_READ_LINES
}

// ReadRange serves one window of a file without reading the rest (ADR-036).
func (o *FileOps) ReadRange(ctx context.Context, req *sessionv1.ReadFileRangeRequest) (*sessionv1.ReadFileRangeResponse, error) {
	if req.GetLines() == nil && req.GetBytes() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("an anchor (lines or bytes) is required"))
	}
	if err := o.Require(ctx, rangeOp(req)); err != nil {
		return nil, err
	}
	if req.SessionId == "" || req.Path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id and path are required"))
	}
	rr, err := o.resolve(FileRef{SessionID: req.SessionId, Path: req.Path}, AccessRead)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rr.Root.Close() }()
	info, err := lstatForRead(rr, req.Path)
	if err != nil {
		return nil, err
	}
	if info.Size() > rangeReadCeilingBytes {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("file too large for range reads (%d bytes)", info.Size()))
	}
	f, err := rr.Root.Open(rr.Resolved)
	if err != nil {
		return nil, openReadError(err, req.Path)
	}
	defer func() { _ = f.Close() }()

	rd := &rangeRead{rr: rr, file: f, info: info, src: newFileSource(rr, info)}
	resp, err := o.serveRange(ctx, rd, req)
	if err != nil {
		return nil, err
	}
	resp.CapsEpoch = o.epoch.observe(o.flagMask(ctx))
	resp.Validator = rangeValidator(info)
	resp.Size = info.Size()
	return resp, nil
}

func (o *FileOps) serveRange(ctx context.Context, rd *rangeRead, req *sessionv1.ReadFileRangeRequest) (*sessionv1.ReadFileRangeResponse, error) {
	size := rd.info.Size()
	if _, binary := sniffContent(rd.file, isKnownTextFile(req.Path)); binary {
		return &sessionv1.ReadFileRangeResponse{
			IsBinary:       true,
			VersionToken:   NonEditableToken(size, rd.info.ModTime().UnixNano()).String(),
			ReadOnlyReason: sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY,
		}, nil
	}
	if req.PreferFullIfLe > 0 && size <= min(req.PreferFullIfLe, truncateSize) {
		return o.rangeWhole(rd, req.Path)
	}
	o.attachAnalysis(rd)
	lim := clampRangeLimits(req.GetLines().GetWantLines(), req.MaxBytes)
	var win rangeWindow
	var err error
	if a := req.GetBytes(); a != nil {
		win, err = o.byteRangeWindow(rd, a, lim)
	} else {
		win, err = o.lineRangeWindow(ctx, rd, req.GetLines(), lim)
	}
	if err != nil {
		return nil, err
	}
	return o.finishRange(rd, win), nil
}

// attachAnalysis starts the background pass (first range read of a validator)
// and picks up the result when it is already there.
func (o *FileOps) attachAnalysis(rd *rangeRead) {
	if rd.info.Size() > rangeReadCeilingBytes {
		return
	}
	rd.state, rd.analysis = o.analysis.Start(rd.src.Key, o.analysisRunnerFor(rd.src))
}

// lineRangeWindow resolves a line anchor. Only line 1 needs no index; any other
// start waits briefly for the background pass and is refused if it is still pending.
func (o *FileOps) lineRangeWindow(ctx context.Context, rd *rangeRead, a *sessionv1.LineAnchor, lim rangeLimits) (rangeWindow, error) {
	size := rd.info.Size()
	first := firstLineFor(a.Dir, a.AnchorLine, lim.Lines)
	if first > 1 && rd.analysis == nil {
		waitCtx, cancel := context.WithTimeout(ctx, rangeIndexWait)
		defer cancel()
		if res, err := o.analysis.Await(waitCtx, rd.src.Key, o.analysisRunnerFor(rd.src)); err == nil {
			rd.analysis, rd.state = res, analysisStateReady
		}
	}
	if first > 1 && rd.analysis == nil {
		return rangeWindow{}, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("line index is still being built; retry or read from line 1"))
	}
	if rd.analysis != nil {
		first = min(first, max(rd.analysis.LineCount-int64(lim.Lines)+1, 1))
	}
	var start int64
	if first > 1 {
		off, err := rd.analysis.LineToByte(rd.file, first-1)
		if err != nil {
			return rangeWindow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to locate line"))
		}
		start = off
	}
	raw, err := readAtFull(rd.file, start, min(lim.Bytes, size-start))
	if err != nil {
		return rangeWindow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read file range"))
	}
	raw, partial := cutAtLines(raw, lim.Lines, start+int64(len(raw)) >= size)
	return rangeWindow{Start: start, Raw: raw, StartLine: first, Partial: partial}, nil
}

func (o *FileOps) byteRangeWindow(rd *rangeRead, a *sessionv1.ByteAnchor, lim rangeLimits) (rangeWindow, error) {
	size := rd.info.Size()
	start, length := byteWindowStart(a, size, lim)
	win, err := readByteWindow(rd.file, size, start, length)
	if err != nil {
		return rangeWindow{}, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to read file range"))
	}
	switch {
	case win.Start == 0:
		win.StartLine = 1
	case rd.analysis != nil:
		if line, lerr := rd.analysis.ByteToLine(rd.file, win.Start); lerr == nil {
			win.StartLine = line + 1
		}
	}
	return win, nil
}

// rangeWhole serves a small file in one response with the normal editable token.
func (o *FileOps) rangeWhole(rd *rangeRead, relPath string) (*sessionv1.ReadFileRangeResponse, error) {
	fr, err := classifyOpenFile(rd.file, rd.rr, relPath, rd.info)
	if err != nil {
		return nil, err
	}
	raw := fr.Body
	if fr.HasBOM {
		raw = append(append([]byte(nil), utf8BOM...), raw...)
	}
	resp := o.windowResponse(rd, rangeWindow{Start: 0, Raw: raw, StartLine: 1})
	resp.VersionToken = fr.Token.String()
	resp.ReadOnlyReason = fr.Reason
	resp.LineCountKnown, resp.Analysis = true, analysisStateReady
	resp.LineCount = resp.EndLine
	return resp, nil
}

// finishRange adds the analysis-dependent fields and the post-read torn check.
func (o *FileOps) finishRange(rd *rangeRead, win rangeWindow) *sessionv1.ReadFileRangeResponse {
	resp := o.windowResponse(rd, win)
	resp.Analysis = rd.state
	token := NonEditableToken(rd.info.Size(), rd.info.ModTime().UnixNano())
	if rd.analysis != nil {
		resp.LineCountKnown, resp.LineCount = true, rd.analysis.LineCount
		if rd.info.Size() <= rangeHashCapBytes {
			token.SHA256 = rd.analysis.SHA256
		}
	}
	resp.VersionToken = token.String()
	_, body := detectBOM(win.Raw)
	resp.ReadOnlyReason = classifyReadOnly(readFacts{
		Truncated: !(resp.Bof && resp.Eof) || rd.info.Size() > truncateSize || resp.LongLine,
		ViaLink:   rd.rr.ViaLink,
		Body:      body,
	})
	if post, err := rd.rr.Root.Lstat(rd.rr.Resolved); err != nil ||
		post.Size() != rd.info.Size() || !post.ModTime().Equal(rd.info.ModTime()) {
		resp.Torn = true
	}
	return resp
}

// windowResponse describes win: text, offsets, line span and encoding facts.
func (o *FileOps) windowResponse(rd *rangeRead, win rangeWindow) *sessionv1.ReadFileRangeResponse {
	size := rd.info.Size()
	end := win.Start + int64(len(win.Raw))
	body := win.Raw
	resp := &sessionv1.ReadFileRangeResponse{
		StartByte:       win.Start,
		EndByte:         end,
		Bof:             win.Start == 0,
		Eof:             end >= size,
		ChunkCrc32C:     crc32.Checksum(win.Raw, crc32.MakeTable(crc32.Castagnoli)),
		EolClass:        detectLineEnding(win.Raw),
		NonUtf8:         !utf8.Valid(body),
		PartialLastLine: win.Partial,
		LongLine:        win.Partial || maxLineLen(body) > longLineBytes,
	}
	if win.Start == 0 {
		resp.HasBom, body = detectBOM(body)
	}
	if i := bytes.IndexByte(win.Raw, 0); i >= 0 {
		resp.ContainsNulAt = proto64(win.Start + int64(i))
	}
	resp.Content = strings.ToValidUTF8(strings.ReplaceAll(string(body), "\r\n", "\n"), "�")
	if win.StartLine > 0 && len(win.Raw) > 0 {
		resp.StartLine = win.StartLine
		resp.EndLine = win.StartLine + int64(bytes.Count(win.Raw, []byte{'\n'})) - 1
		if !bytes.HasSuffix(win.Raw, []byte{'\n'}) {
			resp.EndLine++
		}
	}
	return resp
}

func proto64(v int64) *int64 { return &v }

// rangeValidator is the (size, mtime_ns, inode) identity a window was read from.
func rangeValidator(info os.FileInfo) string {
	return strconv.FormatInt(info.Size(), 10) + ":" + strconv.FormatInt(info.ModTime().UnixNano(), 10) +
		":" + strconv.FormatUint(fileInode(info), 10)
}

// openReadError maps an Open failure to the same Connect codes GetContent uses.
func openReadError(err error, relPath string) error {
	if os.IsNotExist(err) {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("file no longer exists: %s", relPath))
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to open file"))
}
