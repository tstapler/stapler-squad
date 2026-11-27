package webhooks

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This golden pins the webhook trust-boundary behaviour (SSRF validation, the auth-failure limiter, the pinned
// outbound dial) over a hostile matrix. Its body is identical to the copy that ran in package services on
// origin/main 0830eb23e (before the leaf move) to record testdata/webhooks.golden.txt, so a green run here
// means the move changed none of these answers. Regenerate only with UPDATE_WEBHOOKS_GOLDEN=1 and review the diff.

const goldenPath = "testdata/webhooks.golden.txt"

var goldenHosts = map[string][]string{
	"public.example":      {"93.184.216.34"},
	"public6.example":     {"2606:2800:220:1:248:1893:25c8:1946"},
	"loopback.example":    {"127.0.0.1"},
	"loopback6.example":   {"::1"},
	"mapped-lo.example":   {"::ffff:127.0.0.1"},
	"mapped-priv.example": {"::ffff:10.0.0.1"},
	"metadata.example":    {"169.254.169.254"},
	"cgnat.example":       {"100.64.0.1"},
	"unspec.example":      {"0.0.0.0"},
	"unspec6.example":     {"::"},
	"ula.example":         {"fd00::1"},
	"linklocal6.example":  {"fe80::1"},
	"mcast-ll.example":    {"ff02::1"},
	"mixed.example":       {"93.184.216.34", "10.0.0.1"},
	"mixed-last.example":  {"10.0.0.1", "93.184.216.34"},
	"two-public.example":  {"93.184.216.34", "8.8.8.8"},
	"empty.example":       {},
	"localhost":           {"127.0.0.1"},
	"localhost.":          {"127.0.0.1"},
}

func goldenResolver(_ context.Context, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}
	ips, ok := goldenHosts[host]
	if !ok {
		return nil, fmt.Errorf("fake resolver: no such host %q", host)
	}
	out := []net.IPAddr{}
	for _, s := range ips {
		out = append(out, net.IPAddr{IP: net.ParseIP(s)})
	}
	return out, nil
}

func goldenErr(err error) string {
	if err == nil {
		return "ok"
	}
	return "err: " + err.Error()
}

func TestWebhooksGolden(t *testing.T) {
	// Not t.Parallel(): swaps the package-level lookupIPAddr resolver seam.
	orig := lookupIPAddr
	lookupIPAddr = goldenResolver
	t.Cleanup(func() { lookupIPAddr = orig })

	var b strings.Builder
	goldenSSRFURLs(&b)
	goldenDisallowedIPs(&b)
	goldenLimiterKeys(&b)
	goldenLimiterWindows(t, &b)
	goldenPinnedDial(t, &b)

	got := b.String()
	if os.Getenv("UPDATE_WEBHOOKS_GOLDEN") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}

func goldenSSRFURLs(b *strings.Builder) {
	urls := []string{
		"http://public.example/", "https://public.example:8443/x?y=1", "HTTP://public.example/", "http://PUBLIC.EXAMPLE/",
		"http://public6.example/", "http://[2606:2800:220:1:248:1893:25c8:1946]/",
		"http://loopback.example/", "http://loopback6.example/", "http://mapped-lo.example/", "http://mapped-priv.example/",
		"http://metadata.example/latest/meta-data", "http://169.254.169.254/", "http://[::ffff:169.254.169.254]/",
		"http://cgnat.example/", "http://100.64.0.1/", "http://100.127.255.254/", "http://100.128.0.1/", "http://100.63.255.255/",
		"http://unspec.example/", "http://unspec6.example/", "http://0.0.0.0/", "http://[::]/", "http://0/",
		"http://ula.example/", "http://[fd00::1]/", "http://linklocal6.example/", "http://[fe80::1%25eth0]/", "http://mcast-ll.example/",
		"http://mixed.example/", "http://mixed-last.example/", "http://two-public.example/", "http://empty.example/",
		"http://localhost/", "http://localhost./", "http://localhost:80/",
		"http://127.0.0.1/", "http://127.0.0.1:8543/", "http://127.1/", "http://2130706433/", "http://0x7f000001/", "http://017700000001/",
		"http://[::1]/", "http://[::1]:8543/", "http://[0:0:0:0:0:0:0:1]/",
		"http://10.0.0.1/", "http://10.255.255.255/", "http://172.16.0.1/", "http://172.31.255.255/", "http://172.32.0.1/", "http://172.15.0.1/",
		"http://192.168.0.1/", "http://192.169.0.1/", "http://169.254.1.1/", "http://169.253.1.1/", "http://8.8.8.8/", "http://1.1.1.1/",
		"http://user:pass@public.example/", "http://user:pass@127.0.0.1/", "http://public.example@127.0.0.1/", "http://127.0.0.1@public.example/",
		"http://public.example#@127.0.0.1/", "http://public.example\\@127.0.0.1/", "http://public.example:80@127.0.0.1/",
		"http://public.example:notaport/", "http://public.example:99999/", "http://public.example:/",
		"ftp://public.example/", "file:///etc/passwd", "gopher://public.example/", "javascript:alert(1)", "data:text/plain,x",
		"ws://public.example/", "wss://public.example/", "https:public.example", "http:/public.example", "http:public.example",
		"//public.example/", "public.example", "", " ", "http://", "http:///path", "http://%20/", "http://pub lic.example/", "http://public.example\n/",
		"http://[public.example]/", "http://public.example./", "http://unknown.example/", "http://xn--e1afmkfd.example/", "http://пример.example/",
		"http://public.example/redirect?to=http://127.0.0.1/", "http://%31%32%37.0.0.1/",
	}
	fmt.Fprintf(b, "== ssrf resolveAndValidateCallbackHost / ValidateCallbackURL\n")
	for _, u := range urls {
		ip, err := resolveAndValidateCallbackHost(context.Background(), u)
		vErr := ValidateCallbackURL(context.Background(), u)
		fmt.Fprintf(b, "%q -> ip=%v %s | validate=%s\n", u, ip, goldenErr(err), goldenErr(vErr))
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := resolveAndValidateCallbackHost(cancelled, "http://public.example/")
	fmt.Fprintf(b, "cancelled-ctx -> %s\n", goldenErr(err))
}

func goldenDisallowedIPs(b *strings.Builder) {
	ips := []string{
		"0.0.0.0", "::", "127.0.0.1", "127.255.255.254", "::1", "10.0.0.0", "10.255.255.255", "11.0.0.0", "172.15.255.255", "172.16.0.0",
		"172.31.255.255", "172.32.0.0", "192.167.255.255", "192.168.0.0", "192.168.255.255", "192.169.0.0", "169.254.0.0", "169.254.169.254",
		"169.254.255.255", "169.255.0.0", "100.63.255.255", "100.64.0.0", "100.127.255.255", "100.128.0.0", "224.0.0.1", "255.255.255.255",
		"fc00::", "fd00::1", "fdff:ffff::1", "fe00::1", "fe80::1", "febf::1", "fec0::1", "ff02::1", "ff01::1", "ff05::1", "2001:db8::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:169.254.169.254", "::ffff:100.64.0.1", "::ffff:8.8.8.8", "8.8.8.8", "93.184.216.34",
		"2606:4700::1111", "64:ff9b::7f00:1", "::127.0.0.1", "fd00:ec2::254",
	}
	fmt.Fprintf(b, "== checkDisallowedCallbackIP\n")
	for _, s := range ips {
		fmt.Fprintf(b, "%s -> %s\n", s, goldenErr(checkDisallowedCallbackIP(net.ParseIP(s))))
	}
	fmt.Fprintf(b, "nil-ip -> %s\n", goldenErr(checkDisallowedCallbackIP(nil)))
}

func goldenLimiterKeys(b *strings.Builder) {
	addrs := []string{
		"203.0.113.9:4444", "203.0.113.9", "[2001:db8:1:2:aaaa:bbbb:cccc:dddd]:80", "[2001:db8:1:2:1111:2222:3333:4444]:81",
		"2001:db8:1:2::1", "[2001:db8:1:3::1]:1", "[::ffff:203.0.113.9]:1", "::ffff:203.0.113.9", "[fe80::1%eth0]:5", "fe80::1%eth0",
		"[fe80::1%eth1]:5", "", "garbage", "garbage:with:colons", "[::1]:80", "127.0.0.1:80", "999.1.1.1:1", "1.2.3.4:", ":80", "[::]:0",
	}
	fmt.Fprintf(b, "== clientLimiterKey\n")
	for _, a := range addrs {
		fmt.Fprintf(b, "%q -> %q\n", a, clientLimiterKey(a))
	}
}

func goldenLimiterWindows(t *testing.T, b *strings.Builder) {
	t.Helper()
	clock := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newAuthFailureLimiter()
	l.now = func() time.Time { return clock }
	state := func(key string) string {
		d, blocked := l.blocked(key)
		return fmt.Sprintf("blocked=%v retry=%s", blocked, d)
	}
	fmt.Fprintf(b, "== limiter defaults max=%d window=%s maxKeys=%d\n", l.max, l.window, l.maxKeys)
	for i := 1; i <= l.max+2; i++ {
		just := l.recordFailure("a")
		fmt.Fprintf(b, "a failure %d just=%v %s\n", i, just, state("a"))
		clock = clock.Add(time.Second)
	}
	fmt.Fprintf(b, "b untouched %s\n", state("b"))
	clock = clock.Add(l.window - time.Duration(l.max+2)*time.Second - time.Nanosecond)
	fmt.Fprintf(b, "a one ns before window end %s\n", state("a"))
	clock = clock.Add(time.Nanosecond)
	fmt.Fprintf(b, "a at window end %s\n", state("a"))
	just := l.recordFailure("a")
	fmt.Fprintf(b, "a after reset just=%v %s\n", just, state("a"))
	for i := 0; i < l.max-2; i++ {
		l.recordFailure("a")
	}
	fmt.Fprintf(b, "a at max-1 %s\n", state("a"))
	fmt.Fprintf(b, "a hits max just=%v %s\n", l.recordFailure("a"), state("a"))
	clock = clock.Add(l.window / 2)
	fmt.Fprintf(b, "a half window %s\n", state("a"))

	small := newAuthFailureLimiter()
	small.maxKeys = 3
	small.now = func() time.Time { return clock }
	for _, k := range []string{"k1", "k2", "k3"} {
		small.recordFailure(k)
		clock = clock.Add(time.Second)
	}
	small.recordFailure("k4")
	fmt.Fprintf(b, "evict oldest live: len=%d k1=%v k4=%v\n", len(small.entries), small.entries["k1"] != nil, small.entries["k4"] != nil)
	clock = clock.Add(small.window)
	small.recordFailure("k5")
	fmt.Fprintf(b, "evict expired: len=%d keys=%d\n", len(small.entries), len(small.entries))
}

func goldenPinnedDial(t *testing.T, b *strings.Builder) {
	t.Helper()
	fmt.Fprintf(b, "== pinned dial / redirects\n")
	for _, status := range []int{200, 204, 299, 300, 302, 307, 400, 500} {
		var gotMethod, gotCT, gotBody, gotHost string
		var hits int
		var mu sync.Mutex
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			mu.Lock()
			hits++
			first := hits == 1
			gotMethod, gotCT, gotHost, gotBody = r.Method, r.Header.Get("Content-Type"), r.Host, string(raw)
			mu.Unlock()
			if !first {
				w.WriteHeader(http.StatusOK)
				return
			}
			if status >= 300 && status < 400 {
				// Same host and port: a followed redirect is visible as a second hit on this server.
				w.Header().Set("Location", "http://"+r.Host+"/followed")
			}
			w.WriteHeader(status)
		}))
		d := NewCallbackDispatcher(nil)
		_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
		require.NoError(t, err)
		// Host name does not resolve anywhere: success proves the dial was pinned to the supplied IP.
		url := "http://pin.invalid:" + port + "/cb"
		ok := d.attempt(context.Background(), url, []byte(`{"k":1}`), net.ParseIP("127.0.0.1"))
		srv.Close()
		mu.Lock()
		fmt.Fprintf(b, "status=%d ok=%v hits=%d method=%s ct=%q body=%q hostHdrIsURLHost=%v\n", status, ok, hits, gotMethod, gotCT, gotBody, gotHost == "pin.invalid:"+port)
		mu.Unlock()
	}
	d := NewCallbackDispatcher(nil)
	fmt.Fprintf(b, "dial refused: %v\n", d.attempt(context.Background(), "http://pin.invalid:1/cb", []byte("{}"), net.ParseIP("127.0.0.1")))
	d.Dispatch("session_complete", map[string]string{"a": "b"})
}
