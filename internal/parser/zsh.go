package parser

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/internal/history"
)

const zshMeta = 0x83

// ZshParser implements a streaming state-machine parser for Zsh history files.
type ZshParser struct{}

// NewZshParser creates a new ZshParser.
func NewZshParser() *ZshParser {
	return &ZshParser{}
}

// unmetafy reverses Zsh's proprietary escaping (metafication).
func unmetafy(b []byte) []byte {
	res := make([]byte, 0, len(b))
	meta := false
	for _, c := range b {
		if c == zshMeta {
			meta = true
			continue
		}
		if meta {
			res = append(res, c^32)
			meta = false
		} else {
			res = append(res, c)
		}
	}
	return res
}

// Parse parses the Zsh history from an io.Reader and returns a channel of canonical Events.
func (p *ZshParser) Parse(ctx context.Context, r io.Reader) (<-chan *history.Event, <-chan error) {
	out := make(chan *history.Event, 100)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)

		// Read byte by byte or line by line. Since Zsh might have multi-line commands
		// that use escaped newlines, we read using a scanner.
		// However, unmetafy should be applied carefully. We will read line by line.
		scanner := bufio.NewScanner(r)
		// Increase buffer size for large commands
		const maxCapacity = 10 * 1024 * 1024
		buf := make([]byte, maxCapacity)
		scanner.Buffer(buf, maxCapacity)

		// Zsh history commands can span multiple lines if there's a trailing backslash
		// before a newline (or if inside quotes, but Zsh often escapes those newlines).
		var currentCmd bytes.Buffer
		var currentTimestamp time.Time
		inCmd := false

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			default:
			}

			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			// Check if this is the start of a new extended history entry
			if !inCmd && bytes.HasPrefix(line, []byte(": ")) {
				// Parse timestamp and duration
				parts := bytes.SplitN(line[2:], []byte(";"), 2)
				if len(parts) == 2 {
					meta := bytes.SplitN(parts[0], []byte(":"), 2)
					if len(meta) >= 1 {
						ts, err := strconv.ParseInt(strings.TrimSpace(string(meta[0])), 10, 64)
						if err == nil {
							currentTimestamp = time.Unix(ts, 0)
						}
					}
					// Unmetafy the command part
					unmetafiedCmd := unmetafy(parts[1])
					currentCmd.Write(unmetafiedCmd)
				} else {
					// Fallback for malformed or non-extended lines
					currentCmd.Write(unmetafy(line))
				}

				// Check for trailing backslash to see if command continues
				if bytes.HasSuffix(currentCmd.Bytes(), []byte{'\\'}) {
					inCmd = true
					currentCmd.Truncate(currentCmd.Len() - 1)
					currentCmd.WriteByte('\n')
				} else {
					// Command is complete
					e := &history.Event{
						Command:       currentCmd.String(),
						Timestamp:     currentTimestamp,
						ProgramSource: "zsh",
					}
					// Generate unique ID based on zero sequence since Zsh has timestamps
					e.GenerateID(0)
					out <- e
					currentCmd.Reset()
					currentTimestamp = time.Time{}
					inCmd = false
				}
			} else if inCmd {
				unmetafiedCmd := unmetafy(line)
				currentCmd.Write(unmetafiedCmd)

				if bytes.HasSuffix(currentCmd.Bytes(), []byte{'\\'}) {
					currentCmd.Truncate(currentCmd.Len() - 1)
					currentCmd.WriteByte('\n')
				} else {
					e := &history.Event{
						Command:       currentCmd.String(),
						Timestamp:     currentTimestamp,
						ProgramSource: "zsh",
					}
					e.GenerateID(0)
					out <- e
					currentCmd.Reset()
					currentTimestamp = time.Time{}
					inCmd = false
				}
			} else {
				// Treat as plain command without extended timestamp
				currentCmd.Write(unmetafy(line))
				e := &history.Event{
					Command:       currentCmd.String(),
					Timestamp:     time.Now(),
					ProgramSource: "zsh",
				}
				e.GenerateID(0)
				out <- e
				currentCmd.Reset()
			}
		}

		if err := scanner.Err(); err != nil {
			errs <- fmt.Errorf("scanner error: %w", err)
		}
	}()

	return out, errs
}

// LocateZshHistory resolves the standard Zsh history file path using environment variables.
func LocateZshHistory() (string, error) {
	if h := os.Getenv("HISTFILE"); h != "" {
		return h, nil
	}
	if zd := os.Getenv("ZDOTDIR"); zd != "" {
		return filepath.Join(zd, ".zsh_history"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not resolve home directory: %w", err)
	}
	return filepath.Join(home, ".zsh_history"), nil
}
