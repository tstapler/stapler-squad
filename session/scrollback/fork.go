package scrollback

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// ForkScrollback copies scrollback entries with sequence numbers <= upToSeq from srcPath
// to dstPath. The destination is always written as uncompressed JSONL regardless of
// whether the source is compressed. Entries are copied in order, preserving original
// timestamps and sequence numbers so the forked session can resume playback from any point.
//
// If srcPath does not exist (session has no scrollback yet), dstPath is created as an
// empty file. If upToSeq is 0, an empty file is created (no entries qualify).
// If upToSeq exceeds the maximum sequence in src, all entries are copied.
func ForkScrollback(srcPath string, upToSeq uint64, dstPath string) error {
	// Ensure destination directory exists.
	if err := os.MkdirAll(filepath.Dir(dstPath), 0750); err != nil {
		return fmt.Errorf("fork scrollback: create dst dir: %w", err)
	}

	// Open source (handle missing gracefully — create empty dst).
	// srcPath/dstPath are caller-constructed from session titles; the only
	// caller (Instance.ForkFromCheckpoint, session/instance_checkpoint.go)
	// verifies both stay under the trusted config directory via
	// isPathUnderDir before ever calling this function.
	srcFile, err := os.Open(srcPath) // #nosec G304 -- caller validates containment under configDir before calling (see ForkFromCheckpoint)
	if err != nil {
		if os.IsNotExist(err) {
			// Create an empty destination file.
			f, createErr := os.OpenFile(dstPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600) // #nosec G304 -- caller validates containment under configDir before calling (see ForkFromCheckpoint)
			if createErr != nil {
				return fmt.Errorf("fork scrollback: create empty dst: %w", createErr)
			}
			return f.Close()
		}
		return fmt.Errorf("fork scrollback: open src: %w", err)
	}
	defer srcFile.Close()

	// Wrap reader with decompressor based on file extension.
	var reader io.Reader = srcFile
	switch {
	case strings.HasSuffix(srcPath, ".gz"):
		gz, gzErr := gzip.NewReader(srcFile)
		if gzErr != nil {
			return fmt.Errorf("fork scrollback: init gzip reader: %w", gzErr)
		}
		defer gz.Close()
		reader = gz
	case strings.HasSuffix(srcPath, ".zst"):
		dec, zstdErr := zstd.NewReader(srcFile)
		if zstdErr != nil {
			return fmt.Errorf("fork scrollback: init zstd reader: %w", zstdErr)
		}
		defer dec.Close()
		reader = dec
	}

	// Write to a temp file alongside dst, then atomically rename. Unique temp name
	// (not dstPath+".tmp") to match the write-tmp-then-rename shape used by the
	// other atomic writers in this codebase (config.go's saveConfigLocked, etc.).
	tmpFile, err := os.CreateTemp(filepath.Dir(dstPath), filepath.Base(dstPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("fork scrollback: create tmp: %w", err)
	}
	tmpPath := tmpFile.Name()

	enc := json.NewEncoder(tmpFile)
	scanner := bufio.NewScanner(reader)
	// Increase the buffer limit to handle large terminal lines.
	const maxLine = 4 * 1024 * 1024 // 4 MiB
	scanner.Buffer(make([]byte, 64*1024), maxLine)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry storedEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Skip lines that can't be parsed (e.g., partial writes at end of file).
			continue
		}
		if entry.Sequence > upToSeq {
			// Past the desired checkpoint — stop copying.
			// When upToSeq == 0, this triggers on the first entry (seq starts at 1).
			break
		}
		if err := enc.Encode(entry); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("fork scrollback: encode entry: %w", err)
		}
	}

	if scanErr := scanner.Err(); scanErr != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("fork scrollback: scan src: %w", scanErr)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("fork scrollback: close tmp: %w", err)
	}

	// Atomic rename.
	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("fork scrollback: rename tmp→dst: %w", err)
	}

	return nil
}
