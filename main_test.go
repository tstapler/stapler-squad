package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// captureLogWarn temporarily redirects the default slog handler to a text
// handler writing into the returned buffer, restoring the previous handler
// via the returned cleanup func. Used to assert that log.Warn was actually
// invoked (and named the offending entry) rather than only checking the
// returned slice.
func captureLogWarn(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(prev)
	})
	return &buf
}

func Test_parseExtraOrigins_should_AcceptEntry_When_GivenWellFormedHttpLocalhostOrigin(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{"http localhost", "http://localhost:54212"},
		{"https 127.0.0.1", "https://127.0.0.1:9999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, rejected := parseExtraOrigins(tt.entry)

			if len(rejected) != 0 {
				t.Fatalf("expected no rejected entries, got %v", rejected)
			}
			if len(valid) != 1 || valid[0] != tt.entry {
				t.Fatalf("expected valid=[%q], got %v", tt.entry, valid)
			}
		})
	}
}

func Test_parseExtraOrigins_should_RejectAndLogWarning_When_GivenMalformedEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{"not a URL", "not-a-valid-origin"},
		{"wildcard", "http://*"},
		{"has path", "http://localhost:1234/path"},
		{"non-localhost host", "http://example.com:1234"},
		{"missing port", "http://localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogWarn(t)

			valid, rejected := parseExtraOrigins(tt.entry)

			if len(valid) != 0 {
				t.Fatalf("expected no valid entries, got %v", valid)
			}
			if len(rejected) != 1 || rejected[0] != tt.entry {
				t.Fatalf("expected rejected=[%q], got %v", tt.entry, rejected)
			}
			if !strings.Contains(buf.String(), tt.entry) {
				t.Fatalf("expected a warning log line naming the offending entry %q, got log output: %s", tt.entry, buf.String())
			}
		})
	}
}

func Test_parseExtraOrigins_should_AcceptValidEntry_And_RejectInvalidEntry_When_BothPresentInOneCommaSeparatedList(t *testing.T) {
	buf := captureLogWarn(t)

	valid, rejected := parseExtraOrigins("http://localhost:54212,not-a-valid-origin")

	if len(valid) != 1 || valid[0] != "http://localhost:54212" {
		t.Fatalf(`expected valid=["http://localhost:54212"], got %v`, valid)
	}
	if len(rejected) != 1 || rejected[0] != "not-a-valid-origin" {
		t.Fatalf(`expected rejected=["not-a-valid-origin"], got %v`, rejected)
	}
	if !strings.Contains(buf.String(), "not-a-valid-origin") {
		t.Fatalf("expected exactly one warning logged naming the offending entry, got log output: %s", buf.String())
	}
}
