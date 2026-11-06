package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	writeFilePath  = "/session.v1.SessionService/WriteFile"
	writeFileLimit = 1536 * 1024 // 1.5 MiB
)

// readAllHandler reads the whole body and reports whether the read hit the limit.
func readAllHandler(readErr *error, read *int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		*readErr, *read = err, len(b)
		w.WriteHeader(http.StatusOK)
	})
}

func post(h http.Handler, path string, size int) {
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(make([]byte, size)))
	h.ServeHTTP(httptest.NewRecorder(), r)
}

func TestBodyLimit_ShouldRefuseOver1_5MiBOnWriteFilePathOnly_And_AcceptOneMiBContent(t *testing.T) {
	var readErr error
	var read int
	h := BodyLimit(writeFilePath, writeFileLimit)(readAllHandler(&readErr, &read))

	t.Run("1.5 MiB + 1 is refused", func(t *testing.T) {
		post(h, writeFilePath, writeFileLimit+1)
		var maxErr *http.MaxBytesError
		require.True(t, errors.As(readErr, &maxErr), "read fails with *http.MaxBytesError, which connect-go maps to ResourceExhausted")
		assert.EqualValues(t, writeFileLimit, maxErr.Limit)
		assert.LessOrEqual(t, read, writeFileLimit, "nothing beyond the limit is read into memory")
	})
	t.Run("1 MiB of content as base64 JSON is accepted", func(t *testing.T) {
		const base64OfOneMiB = (1 << 20) * 4 / 3
		post(h, writeFilePath, base64OfOneMiB+512)
		require.NoError(t, readErr)
		assert.Equal(t, base64OfOneMiB+512, read)
	})
	t.Run("exactly the limit is accepted", func(t *testing.T) {
		post(h, writeFilePath, writeFileLimit)
		require.NoError(t, readErr)
	})
	t.Run("another procedure's large body is unaffected", func(t *testing.T) {
		post(h, "/session.v1.SessionService/WriteToSession", 4<<20)
		require.NoError(t, readErr)
		assert.Equal(t, 4<<20, read)
	})
}
