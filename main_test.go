package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
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

func Test_formatKnownHosts_should_PrintNoKnownHostsMessage_When_GivenEmptySnapshot(t *testing.T) {
	var buf bytes.Buffer

	formatKnownHosts(&buf, nil)

	got := buf.String()
	if !strings.Contains(got, "No known hosts") {
		t.Fatalf("expected a 'no known hosts' message, got: %q", got)
	}
}

func Test_formatKnownHosts_should_PrintHostIDAddressesAndLastSeen_When_GivenEntries(t *testing.T) {
	id, err := session.NewHostID()
	if err != nil {
		t.Fatalf("NewHostID() error = %v", err)
	}
	lastSeen := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	entries := []session.RegistryEntry{
		{
			HostID:            id,
			AdvertisedAddress: []string{"192.168.1.42:8543", "10.0.0.5:8543"},
			LastSeenAt:        lastSeen,
		},
	}

	var buf bytes.Buffer
	formatKnownHosts(&buf, entries)

	got := buf.String()
	for _, want := range []string{id.String(), "192.168.1.42:8543", "10.0.0.5:8543"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got: %s", want, got)
		}
	}
	if !strings.Contains(got, lastSeen.Local().Format(time.RFC3339)) {
		t.Errorf("expected output to contain formatted last-seen time, got: %s", got)
	}
}

func Test_formatKnownHosts_should_SortEntriesByHostID_When_GivenMultipleEntries(t *testing.T) {
	idA, err := session.NewHostID()
	if err != nil {
		t.Fatalf("NewHostID() error = %v", err)
	}
	idB, err := session.NewHostID()
	if err != nil {
		t.Fatalf("NewHostID() error = %v", err)
	}
	// Ensure a deterministic expected order regardless of generation order.
	first, second := idA, idB
	if first.String() > second.String() {
		first, second = second, first
	}

	entries := []session.RegistryEntry{
		{HostID: second, AdvertisedAddress: []string{"host-b:8543"}},
		{HostID: first, AdvertisedAddress: []string{"host-a:8543"}},
	}

	var buf bytes.Buffer
	formatKnownHosts(&buf, entries)

	got := buf.String()
	idxFirst := strings.Index(got, first.String())
	idxSecond := strings.Index(got, second.String())
	if idxFirst == -1 || idxSecond == -1 {
		t.Fatalf("expected both host IDs present in output, got: %s", got)
	}
	if idxFirst > idxSecond {
		t.Errorf("expected %q to appear before %q in sorted output, got: %s", first.String(), second.String(), got)
	}
}
