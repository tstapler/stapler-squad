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
	"time"

	"github.com/tstapler/stapler-squad/internal/history"
)

// BashParser implements a streaming state-machine parser for Bash history files.
type BashParser struct{}

// NewBashParser creates a new BashParser.
func NewBashParser() *BashParser {
	return &BashParser{}
}

// Parse parses the Bash history from an io.Reader and returns a channel of canonical Events.
func (p *BashParser) Parse(ctx context.Context, r io.Reader) (<-chan *history.Event, <-chan error) {
	out := make(chan *history.Event, 100)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)

		scanner := bufio.NewScanner(r)
		const maxCapacity = 10 * 1024 * 1024
		buf := make([]byte, maxCapacity)
		scanner.Buffer(buf, maxCapacity)

		var currentTimestamp time.Time
		sequenceNumber := 0
		var currentCmd bytes.Buffer
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

			// Bash timestamp marker: #1600000000
			if !inCmd && line[0] == '#' {
				ts, err := strconv.ParseInt(string(line[1:]), 10, 64)
				if err == nil {
					currentTimestamp = time.Unix(ts, 0)
					continue
				}
			}

			// Accumulate command
			currentCmd.Write(line)

			// If ends with a backslash, it's a continuation line (unless escaped)
			if bytes.HasSuffix(currentCmd.Bytes(), []byte{'\\'}) {
				inCmd = true
				currentCmd.Truncate(currentCmd.Len() - 1)
				currentCmd.WriteByte('\n')
				continue
			}

			// Command is complete
			if currentTimestamp.IsZero() {
				// Fallback if no timestamp
				currentTimestamp = time.Now()
			}

			e := &history.Event{
				Command:       currentCmd.String(),
				Timestamp:     currentTimestamp,
				ProgramSource: "bash",
			}

			sequenceNumber++
			// For Bash without timestamps (or with), sequenceNumber helps disambiguate
			e.GenerateID(sequenceNumber)

			out <- e

			currentCmd.Reset()
			currentTimestamp = time.Time{}
			inCmd = false
		}

		if err := scanner.Err(); err != nil {
			errs <- fmt.Errorf("scanner error: %w", err)
		}
	}()

	return out, errs
}

// LocateBashHistory resolves the standard Bash history file path using environment variables.
func LocateBashHistory() (string, error) {
	if h := os.Getenv("HISTFILE"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not resolve home directory: %w", err)
	}
	return filepath.Join(home, ".bash_history"), nil
}
