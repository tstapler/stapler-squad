package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// ForkClaudeConversation copies the first lineCount non-empty lines from srcConvPath
// into a new JSONL file named {newUUID}.jsonl inside dstDir. The new UUID is returned so
// the caller can set it as the forked session's ResumeId.
//
// If lineCount is 0 an empty file is created and the new UUID is still returned.
// If lineCount exceeds the number of lines in the source all lines are copied without error.
// If srcConvPath does not exist an error is returned.
func ForkClaudeConversation(srcConvPath string, lineCount uint64, dstDir string) (string, error) {
	newUUID := uuid.New().String()
	dstPath := filepath.Join(dstDir, newUUID+".jsonl")

	if err := os.MkdirAll(dstDir, 0750); err != nil {
		return "", fmt.Errorf("fork claude conversation: create dst dir: %w", err)
	}

	srcFile, err := os.Open(srcConvPath) // #nosec G304 -- callers resolve srcConvPath via FindConversationFilePath's safe directory walk or internal Instance.HistoryFilePath, never raw request input
	if err != nil {
		return "", fmt.Errorf("fork claude conversation: open src: %w", err)
	}
	defer srcFile.Close()

	// Unique temp name (not dstPath+".tmp"): dstPath is already unique per call via
	// newUUID, so this can't collide with another ForkClaudeConversation call, but
	// os.CreateTemp keeps the write-tmp-then-rename shape consistent with the other
	// atomic writers in this codebase (config.go's saveConfigLocked, etc.).
	tmpFile, err := os.CreateTemp(dstDir, newUUID+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("fork claude conversation: create tmp: %w", err)
	}
	tmpPath := tmpFile.Name()

	scanner := bufio.NewScanner(srcFile)
	const maxLine = 4 * 1024 * 1024 // 4 MiB — conversation entries can be large
	scanner.Buffer(make([]byte, 64*1024), maxLine)

	var copied uint64
	for lineCount > 0 && scanner.Scan() {
		if copied >= lineCount {
			break
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Validate JSON before copying — skip malformed lines.
		if !json.Valid(line) {
			continue
		}
		if _, err := tmpFile.Write(line); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("fork claude conversation: write line: %w", err)
		}
		if _, err := tmpFile.Write([]byte("\n")); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("fork claude conversation: write newline: %w", err)
		}
		copied++
	}

	if scanErr := scanner.Err(); scanErr != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("fork claude conversation: scan src: %w", scanErr)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("fork claude conversation: close tmp: %w", err)
	}

	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("fork claude conversation: rename tmp→dst: %w", err)
	}

	return newUUID, nil
}
