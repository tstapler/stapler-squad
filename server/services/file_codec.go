package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// isKnownTextFile reports whether the file's extension (or, for extension-less
// names like Makefile, its basename) is on the always-text allowlist.
func isKnownTextFile(fullPath string) bool {
	ext := strings.ToLower(filepath.Ext(fullPath))
	if ext == "" {
		base := strings.ToLower(filepath.Base(fullPath))
		if knownTextExtensions["."+base] {
			ext = "." + base
		}
	}
	return knownTextExtensions[ext]
}

// sniffContent returns the sniffed content type and whether the file is binary:
// MIME sniff first, then a null-byte scan of the first 8000 bytes (which overrides
// a text MIME). Known-text files skip both checks. Leaves f's offset unspecified.
func sniffContent(f *os.File, isText bool) (contentType string, isBinary bool) {
	sniffBuf := make([]byte, 512)
	sniffN, _ := f.Read(sniffBuf)
	sniffBuf = sniffBuf[:sniffN]
	contentType = http.DetectContentType(sniffBuf)
	if isText {
		return contentType, false
	}
	if !strings.HasPrefix(contentType, "text/") {
		return contentType, true
	}
	scanBuf := sniffBuf
	if len(sniffBuf) < 8000 {
		_, _ = f.Seek(0, 0)
		scanBuf = make([]byte, 8000)
		n, _ := f.Read(scanBuf)
		scanBuf = scanBuf[:n]
	}
	for _, b := range scanBuf {
		if b == 0 {
			return contentType, true
		}
	}
	return contentType, false
}

// readTextCapped reads f from the start, capped at truncateSize; truncated reports
// that the file was longer than the cap. The caller has already enforced maxFileSize.
func readTextCapped(f *os.File, size int64) (buf []byte, truncated bool, err error) {
	readLimit := size
	if size > truncateSize {
		readLimit = truncateSize
		truncated = true
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, false, err
	}
	buf = make([]byte, readLimit)
	n, readErr := readFull(f, buf)
	if readErr != nil && n == 0 {
		return nil, false, readErr
	}
	return buf[:n], truncated, nil
}

// FileVersionToken identifies one on-disk version: "v1:<size>:<mtime_ns>:<sha256hex>".
// An empty SHA256 is the "-" form of a non-editable read; size and mtime are
// hints, the hash is the write identity.
type FileVersionToken struct {
	Size    int64
	MtimeNs int64
	SHA256  string
}

const (
	tokenPrefix  = "v1"
	tokenNoHash  = "-"
	sha256HexLen = 64
)

// NewFileVersionToken hashes the raw on-disk bytes (BOM and line endings included).
func NewFileVersionToken(size, mtimeNs int64, raw []byte) FileVersionToken {
	sum := sha256.Sum256(raw)
	return FileVersionToken{Size: size, MtimeNs: mtimeNs, SHA256: hex.EncodeToString(sum[:])}
}

// NonEditableToken is the "-" form carried by reads that cannot be edited.
func NonEditableToken(size, mtimeNs int64) FileVersionToken {
	return FileVersionToken{Size: size, MtimeNs: mtimeNs}
}

// Editable reports whether the token carries a write identity.
func (t FileVersionToken) Editable() bool { return t.SHA256 != "" }

func (t FileVersionToken) String() string {
	h := t.SHA256
	if h == "" {
		h = tokenNoHash
	}
	return fmt.Sprintf("%s:%d:%d:%s", tokenPrefix, t.Size, t.MtimeNs, h)
}

// ParseFileVersionToken parses a v1 token; a well-formed "-" token parses and is not Editable.
func ParseFileVersionToken(s string) (FileVersionToken, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 4 || parts[0] != tokenPrefix {
		return FileVersionToken{}, fmt.Errorf("malformed version token")
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || size < 0 {
		return FileVersionToken{}, fmt.Errorf("malformed version token size")
	}
	mtime, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return FileVersionToken{}, fmt.Errorf("malformed version token mtime")
	}
	tok := FileVersionToken{Size: size, MtimeNs: mtime}
	if parts[3] == tokenNoHash {
		return tok, nil
	}
	if !isLowerHex(parts[3], sha256HexLen) {
		return FileVersionToken{}, fmt.Errorf("malformed version token hash")
	}
	tok.SHA256 = parts[3]
	return tok, nil
}

func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// detectLineEnding classifies raw bytes. A lone CR (not followed by LF) makes the
// file Mixed so the LF-normalize/re-apply round trip can never change bytes.
func detectLineEnding(b []byte) sessionv1.LineEnding {
	var crlf, lf, cr int
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '\n':
			lf++
		case '\r':
			if i+1 < len(b) && b[i+1] == '\n' {
				crlf++
				i++
			} else {
				cr++
			}
		}
	}
	switch {
	case cr > 0, crlf > 0 && lf > 0:
		return sessionv1.LineEnding_LINE_ENDING_MIXED
	case crlf > 0:
		return sessionv1.LineEnding_LINE_ENDING_CRLF
	default:
		return sessionv1.LineEnding_LINE_ENDING_LF
	}
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// detectBOM reports a leading UTF-8 BOM and returns b without it.
func detectBOM(b []byte) (hasBOM bool, rest []byte) {
	if bytes.HasPrefix(b, utf8BOM) {
		return true, b[len(utf8BOM):]
	}
	return false, b
}

var (
	errContainsNUL = errors.New("contains NUL")
	errNotUTF8     = errors.New("not valid UTF-8")
)

// validateEditableUTF8 is the strict editability check: no NUL and valid UTF-8.
func validateEditableUTF8(b []byte) error {
	if bytes.IndexByte(b, 0) >= 0 {
		return errContainsNUL
	}
	if !utf8.Valid(b) {
		return errNotUTF8
	}
	return nil
}

// truncateOnRuneBoundary drops a trailing incomplete UTF-8 sequence (at most 3 bytes).
func truncateOnRuneBoundary(b []byte) []byte {
	end := len(b)
	for back := 1; back <= utf8.UTFMax && back <= end; back++ {
		if utf8.RuneStart(b[end-back]) {
			if !utf8.FullRune(b[end-back : end]) {
				return b[:end-back]
			}
			return b
		}
	}
	return b
}

// readFacts is what classifyReadOnly needs; Body is the raw bytes read, BOM stripped.
type readFacts struct {
	Binary    bool
	Truncated bool
	ViaLink   bool
	Body      []byte
}

// classifyReadOnly picks the first applicable reason, or NONE when editable.
func classifyReadOnly(f readFacts) sessionv1.ReadOnlyReason {
	switch {
	case f.Binary:
		return sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY
	case f.Truncated:
		return sessionv1.ReadOnlyReason_READ_ONLY_REASON_TRUNCATED
	}
	switch err := validateEditableUTF8(f.Body); {
	case errors.Is(err, errContainsNUL):
		return sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY
	case err != nil:
		return sessionv1.ReadOnlyReason_READ_ONLY_REASON_NON_UTF8
	}
	switch {
	case detectLineEnding(f.Body) == sessionv1.LineEnding_LINE_ENDING_MIXED:
		return sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL
	case f.ViaLink:
		return sessionv1.ReadOnlyReason_READ_ONLY_REASON_SYMLINK
	}
	return sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE
}
