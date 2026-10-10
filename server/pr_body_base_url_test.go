package server

import "testing"

func TestResolvePRBodyBaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, configured, remote, want string
	}{
		{"configured wins", "https://ssq.example.com/", "https://host.lan:8444", "https://ssq.example.com"},
		{"remote-only", "", "https://host.lan:8444", "https://host.lan:8444"},
		{"none omits", "", "", ""},
		{"loopback configured falls through to remote", "http://127.0.0.1:8543", "https://host.lan:8444", "https://host.lan:8444"},
		{"loopback everywhere omits", "http://localhost:8543", "https://127.0.0.1:8444", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolvePRBodyBaseURL(tt.configured, tt.remote); got != tt.want {
				t.Fatalf("resolvePRBodyBaseURL(%q, %q) = %q, want %q", tt.configured, tt.remote, got, tt.want)
			}
		})
	}
}
