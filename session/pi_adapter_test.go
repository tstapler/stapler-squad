package session

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPiAdapter_ShouldDecodeEveryKnownEventType_WhenFedCapturedTranscript
// feeds the real, live-captured transcript from Story 1.1.1
// (session/detection/testdata/pi/basic_session.jsonl, pi 0.84.4) through
// PiEventReader line-by-line and asserts every line decodes into a known
// typed event with no "unrecognized type" error, and that the session
// header's fields are read correctly.
func TestPiAdapter_ShouldDecodeEveryKnownEventType_WhenFedCapturedTranscript(t *testing.T) {
	file, err := os.Open("detection/testdata/pi/basic_session.jsonl")
	require.NoError(t, err)
	defer file.Close()

	reader := NewPiEventReader(file)

	var (
		events   []any
		sawFirst bool
	)
	for {
		event, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err, "every line in the captured transcript must decode into a known event type")
		events = append(events, event)

		if !sawFirst {
			sessionEvent, ok := event.(PiSessionEvent)
			require.True(t, ok, "first event must be a session header, got %T", event)
			assert.Equal(t, 3, sessionEvent.Version)
			assert.Equal(t, "01a065a6-28ae-7973-8b45-c633ab1b138f", sessionEvent.ID)
			assert.Equal(t, "/private/tmp/pi-spike", sessionEvent.CWD)
			sawFirst = true
		}
	}

	require.NotEmpty(t, events, "expected at least one decoded event")

	// Sanity-check that the known event types from Story 1.1.1's capture
	// were all actually observed at least once, not just that decoding
	// didn't error.
	var (
		sawAgentStart    bool
		sawToolExecStart bool
		sawToolExecEnd   bool
		sawMessageUpdate bool
		sawAgentEnd      bool
		sawAgentSettled  bool
	)
	for _, event := range events {
		switch event.(type) {
		case PiAgentStartEvent:
			sawAgentStart = true
		case PiToolExecutionStartEvent:
			sawToolExecStart = true
		case PiToolExecutionEndEvent:
			sawToolExecEnd = true
		case PiMessageUpdateEvent:
			sawMessageUpdate = true
		case PiAgentEndEvent:
			sawAgentEnd = true
		case PiAgentSettledEvent:
			sawAgentSettled = true
		}
	}
	assert.True(t, sawAgentStart, "expected an agent_start event")
	assert.True(t, sawToolExecStart, "expected a tool_execution_start event")
	assert.True(t, sawToolExecEnd, "expected a tool_execution_end event")
	assert.True(t, sawMessageUpdate, "expected a message_update event")
	assert.True(t, sawAgentEnd, "expected an agent_end event")
	assert.True(t, sawAgentSettled, "expected an agent_settled event")
}

// TestPiAdapter_ShouldReturnUnrecognizedTypeError_WhenEventTypeIsUnknown
// asserts a line with an unrecognized `type` discriminator is reported via a
// distinct error (so callers can count it per Story 6.1.1) rather than being
// silently dropped or causing a panic.
func TestPiAdapter_ShouldReturnUnrecognizedTypeError_WhenEventTypeIsUnknown(t *testing.T) {
	input := `{"type":"totally_unknown_type","foo":"bar"}` + "\n"
	reader := NewPiEventReader(strings.NewReader(input))

	event, err := reader.Next()

	require.Error(t, err)
	assert.Nil(t, event)

	var unrecognized *piUnrecognizedTypeError
	require.True(t, errors.As(err, &unrecognized), "expected a *piUnrecognizedTypeError, got %T: %v", err, err)
	assert.Equal(t, "totally_unknown_type", unrecognized.eventType)
}

// TestPiAdapter_ShouldNotSplitLine_WhenLineContainsEmbeddedU2028Character is
// a regression test for PITFALL-2: some "line-aware" text splitters treat
// Unicode line separators (U+2028) as line breaks, which would mis-split a
// JSON string value containing a literal U+2028 character into garbled,
// non-JSON fragments. PiEventReader must use bufio.ScanLines (LF-only), so a
// buffer containing one JSON line with an embedded U+2028 followed by a
// real newline-terminated second line must produce exactly two events, not
// three.
func TestPiAdapter_ShouldNotSplitLine_WhenLineContainsEmbeddedU2028Character(t *testing.T) {
	// U+2028 (LINE SEPARATOR) embedded inside a JSON string value.
	firstLine := "{\"type\":\"agent_start\",\"note\":\"before after\"}"
	secondLine := `{"type":"agent_settled"}`
	input := firstLine + "\n" + secondLine + "\n"

	reader := NewPiEventReader(strings.NewReader(input))

	var events []any
	for {
		event, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		events = append(events, event)
	}

	require.Len(t, events, 2, "a line with an embedded U+2028 must not be split into extra events")
	first, ok := events[0].(PiAgentStartEvent)
	require.True(t, ok, "expected first event to decode as PiAgentStartEvent, got %T", events[0])
	assert.Equal(t, "agent_start", first.Type)
	second, ok := events[1].(PiAgentSettledEvent)
	require.True(t, ok, "expected second event to decode as PiAgentSettledEvent, got %T", events[1])
	assert.Equal(t, "agent_settled", second.Type)
}
