package domainage

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var updateDomainGolden = flag.Bool("update-domainage-golden", false, "rewrite testdata/domainage.golden")

type goldenRDAP struct {
	calls  []string
	status int
	body   string
	err    error
}

func (g *goldenRDAP) RoundTrip(r *http.Request) (*http.Response, error) {
	var headers []string
	for k, v := range r.Header {
		headers = append(headers, k+"="+strings.Join(v, ","))
	}
	sort.Strings(headers)
	g.calls = append(g.calls, fmt.Sprintf("%s %s %v", r.Method, r.URL, headers))
	if g.err != nil {
		return nil, g.err
	}
	return &http.Response{StatusCode: g.status, Body: io.NopCloser(strings.NewReader(g.body)), Header: http.Header{}, Request: r}, nil
}

// The golden was recorded from the pre-move code in server/services at 385c81549.
func TestDomainAgeGolden(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	for _, cmd := range []string{
		"",
		"ls -la",
		"curl https://example.com/a",
		"curl HTTP://upper.example.org",
		"curl https://a.b.Sketchy.XYZ:8443/x https://sketchy.xyz/y",
		"wget http://sub.domain.co.uk/file && curl https://other.io",
		"curl http://127.0.0.1:8080/ http://[::1]:9/ http://10.0.0.1/",
		"curl http://localhost:3000/ http://intranet/",
		"echo 'https://quoted.dev' \"https://dq.app\\n\"",
		"curl ftp://nope.com https://user@auth.net/p",
		"curl https://github.io/x https://foo.github.io/y",
	} {
		line("extract %q = %#v", cmd, ExtractDomainsFromCommand(cmd))
	}

	def := NewDomainAgeChecker(true)
	line("defaults timeout=%s ttl=%s thresh=%s enabled=%t getter=%s transportNil=%t",
		def.httpClient.Timeout, def.cacheTTL, def.newThresh, def.enabled, def.NewDomainThreshold(), def.httpClient.Transport == nil)

	safe := make([]string, 0, len(knownSafeHosts))
	for h := range knownSafeHosts {
		safe = append(safe, h)
	}
	sort.Strings(safe)
	line("safe=%v", safe)

	event := func(action, date string) string {
		return fmt.Sprintf(`{"events":[{"eventAction":"last changed","eventDate":"2000-01-01T00:00:00Z"},{"eventAction":%q,"eventDate":%q}]}`, action, date)
	}
	recent := time.Now().Add(-5 * 24 * time.Hour).UTC()
	justInside := time.Now().Add(-30*24*time.Hour + time.Hour).UTC()
	justOutside := time.Now().Add(-30*24*time.Hour - time.Hour).UTC()
	for _, c := range []struct {
		name, domain string
		enabled      bool
		rt           goldenRDAP
	}{
		{"new", "new.xyz", true, goldenRDAP{status: 200, body: event("registration", recent.Format(time.RFC3339))}},
		{"just inside", "in.xyz", true, goldenRDAP{status: 200, body: event("registration", justInside.Format(time.RFC3339))}},
		{"just outside", "out.xyz", true, goldenRDAP{status: 200, body: event("registration", justOutside.Format(time.RFC3339))}},
		{"alt format upper action", "alt.xyz", true, goldenRDAP{status: 200, body: event("REGISTRATION", recent.Format("2006-01-02T15:04:05Z0700"))}},
		{"bad date", "bad.xyz", true, goldenRDAP{status: 200, body: event("registration", "yesterday")}},
		{"no registration event", "none.xyz", true, goldenRDAP{status: 200, body: `{"events":[]}`}},
		{"404", "gone.xyz", true, goldenRDAP{status: 404}},
		{"500", "err.xyz", true, goldenRDAP{status: 500}},
		{"429", "limit.xyz", true, goldenRDAP{status: 429}},
		{"bad json", "json.xyz", true, goldenRDAP{status: 200, body: "{"}},
		{"transport error", "down.xyz", true, goldenRDAP{err: errors.New("offline")}},
		{"disabled", "new.xyz", false, goldenRDAP{status: 200, body: event("registration", recent.Format(time.RFC3339))}},
		{"safe host", "github.com", true, goldenRDAP{status: 200, body: event("registration", recent.Format(time.RFC3339))}},
	} {
		rt := c.rt
		d := NewDomainAgeChecker(c.enabled)
		d.httpClient = &http.Client{Transport: &rt}
		for i := range 2 {
			isNew, err := d.IsNewlyRegistered(context.Background(), c.domain)
			line("%s #%d: new=%t err=%v calls=%d", c.name, i, isNew, err, len(rt.calls))
		}
		d.mu.RLock()
		entry, ok := d.cache[c.domain]
		d.mu.RUnlock()
		line("  cached=%t notFound=%t zero=%t", ok, entry.notFound, entry.registrationDate.IsZero())
		if ok {
			d.mu.Lock()
			entry.cachedAt = time.Now().Add(-25 * time.Hour)
			d.cache[c.domain] = entry
			d.mu.Unlock()
			isNew, err := d.IsNewlyRegistered(context.Background(), c.domain)
			line("  expired: new=%t err=%v calls=%d", isNew, err, len(rt.calls))
		}
		if len(rt.calls) > 0 {
			line("  request %s", rt.calls[0])
		}
	}

	path := filepath.Join("testdata", "domainage.golden")
	if *updateDomainGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o750))
		require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o600))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), b.String())
}
