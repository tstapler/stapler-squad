package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
