package services

import (
	"context"
	"net"
)

// hermeticLookupIPAddr replaces the real DNS resolver for the whole test
// binary: hostnames resolve to a fixed public address so callback-URL
// validation tests need neither network access nor a working resolver.
// Tests that need other answers still swap lookupIPAddr via withFakeResolver.
func hermeticLookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}
	if host == "localhost" {
		return []net.IPAddr{{IP: net.IPv4(127, 0, 0, 1)}}, nil
	}
	return []net.IPAddr{{IP: net.IPv4(93, 184, 216, 34)}}, nil
}
