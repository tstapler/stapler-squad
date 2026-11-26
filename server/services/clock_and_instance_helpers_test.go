package services

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/session/instance"
)

// manualClock is a settable clock for tests that must not sleep.
type manualClock struct {
	mu sync.Mutex
	t  time.Time
}

func newManualClock() *manualClock { return &manualClock{t: time.Unix(1_800_000_000, 0)} }

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// namedPM gives the test instance a tmux session name.
type namedPM struct {
	*replyPM
	name string
}

func (n namedPM) GetSessionIdentifier() string { return n.name }

func hiddenInstance(t *testing.T, hidden bool) *instance.Instance {
	t.Helper()
	inst, err := instance.NewInstance(instance.InstanceOptions{
		Title:   "access-" + t.Name(),
		Path:    t.TempDir(),
		Program: "claude",
		Hidden:  hidden,
	})
	require.NoError(t, err)
	return inst
}

func VerdictServiceConfig(listen string, verified []string) middleware.VerdictConfig {
	return middleware.VerdictConfig{
		VerifiedHosts:  func() []string { return verified },
		ListenAddr:     func() string { return listen },
		AllowedOrigins: func() []string { return []string{"https://ui.example"} },
		LocalIPs:       func() []net.IP { return []net.IP{net.ParseIP("192.168.1.50")} },
	}
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
