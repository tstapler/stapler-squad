package server

import "testing"

func TestResolvePRBodyBaseURL(t *testing.T) {
	t.Parallel()
	const lan = "https://host.lan:8444"
	tests := []struct {
		name string
		src  prBaseURLSources
		want string
	}{
		{"configured wins", prBaseURLSources{configured: "https://ssq.example.com/", remoteHTTPSURL: lan}, "https://ssq.example.com"},
		{"remote-only", prBaseURLSources{remoteHTTPSURL: lan}, lan},
		{"none", prBaseURLSources{listenAddr: "127.0.0.1:8543"}, ""},
		{"loopback configured falls through to remote", prBaseURLSources{configured: "http://127.0.0.1:8543", remoteHTTPSURL: lan}, lan},
		{"loopback everywhere", prBaseURLSources{configured: "http://localhost:8543", remoteHTTPSURL: "https://127.0.0.1:8444", listenAddr: "127.0.0.1:8543", hostnames: []string{"onyx.lan"}}, ""},
		{"advertised hostname when listener is bound beyond loopback", prBaseURLSources{listenAddr: "[::]:8543", hostnames: []string{"onyx.lan"}}, "http://onyx.lan:8543"},
		{"advertised hostname with specific LAN bind", prBaseURLSources{listenAddr: "192.168.1.5:8543", hostnames: []string{"onyx.lan"}}, "http://onyx.lan:8543"},
		{"hostname ignored when listener is loopback-only", prBaseURLSources{listenAddr: "127.0.0.1:8543", hostnames: []string{"onyx.lan"}}, ""},
		{"localhost hostname never emitted", prBaseURLSources{listenAddr: "0.0.0.0:8543", hostnames: []string{"localhost"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolvePRBodyBaseURL(tt.src); got != tt.want {
				t.Fatalf("resolvePRBodyBaseURL(%+v) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}
