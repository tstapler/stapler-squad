package notify

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
)

var testFlagRegistry = sync.OnceValue(func() *featureflags.Registry { return flagsvc.FlagRegistry() })

var errDiskFull = errors.New("no space left on device")

func lines(b []byte) [][]byte {
	return bytes.Split(bytes.TrimSpace(b), []byte("\n"))
}

// StampedContext runs the real stamp middleware and returns the context a
// handler behind it would see.
func StampedContext(t *testing.T, cfg *middleware.VerdictConfig, listener string, requiresAuth bool, host string, hdr map[string]string) context.Context {
	t.Helper()
	var got context.Context
	var h http.Handler = http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Context()
	})
	if cfg != nil {
		h = access.WithLocalWriteVerdictConfig(*cfg)(h)
	}
	h = access.WithRequestRecord(listener, requiresAuth)(h)
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Host = host
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	h.ServeHTTP(httptest.NewRecorder(), r)
	require.NotNil(t, got)
	return got
}
