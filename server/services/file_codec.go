package services

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
