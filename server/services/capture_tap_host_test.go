package services

import "testing"

func TestHostIsLoopback_should_RejectMalformedHosts_When_UserinfoOrEmptyPort(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost:8543", true},
		{"127.0.0.1", true},
		{"[::1]:8543", true},
		{"LOCALHOST", true},
		{"evil.com@localhost:80", false}, // userinfo: url.Parse drops it, so reject explicitly
		{"user@localhost", false},
		{"localhost:", false}, // empty port
		{"localhost.evil.com", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			if got := hostIsLoopback(tc.host); got != tc.want {
				t.Fatalf("hostIsLoopback(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

// A maximum-length id plus the hash suffix, ".jsonl" and ".old" must stay under
// the 255-byte file-name limit, or that session's tap would report failed.
func TestCaptureTapSessionIDLimit_should_LeaveRoomForFileNameSuffixes(t *testing.T) {
	const hashSuffix, jsonl, old = len("-deadbeef"), len(".jsonl"), len(".old")
	if got := maxCaptureTapSessionIDLen + hashSuffix + jsonl + old; got > 255 {
		t.Fatalf("worst-case file name is %d bytes, over the 255-byte limit", got)
	}
}
