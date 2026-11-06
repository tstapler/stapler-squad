package envtest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The guard mutates the process-wide http.DefaultTransport, so these cases run
// serially inside one test and restore the original dialer afterward.
func TestDenyNonLoopbackNetwork(t *testing.T) {
	t.Setenv(AllowNetworkEnv, "")
	tr := http.DefaultTransport.(*http.Transport)
	origDial, origTLS := tr.DialContext, tr.DialTLSContext
	t.Cleanup(func() { tr.DialContext, tr.DialTLSContext = origDial, origTLS })

	guard := DenyNonLoopbackNetwork()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("loopback request must pass: %v", err)
	}
	_ = resp.Body.Close()
	if got := guard.Violations(); len(got) != 0 {
		t.Fatalf("loopback dial recorded as violation: %+v", got)
	}

	req, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid:8080/", nil)
	_, err = http.DefaultClient.Do(req)
	if !errors.Is(err, ErrNonLoopbackDial) {
		t.Fatalf("non-loopback request: err = %v, want ErrNonLoopbackDial", err)
	}
	got := guard.Violations()
	if len(got) != 1 || got[0].Addr != "example.invalid:8080" || guard.Report() == "" {
		t.Fatalf("violations = %+v, want one for example.invalid:8080 with a report", got)
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:80":        true,
		"[::1]:443":           true,
		"localhost:8080":      true,
		"api.example.com:443": false,
		"10.0.0.5:80":         false,
	} {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}
