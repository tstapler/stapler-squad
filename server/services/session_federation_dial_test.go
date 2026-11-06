package services

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func allowLoopbackFederationDialsForTest(t *testing.T) {
	t.Helper()
	federationAllowLoopbackDials = true
	t.Cleanup(func() { federationAllowLoopbackDials = false })
}

func TestForbiddenPeerIP(t *testing.T) {
	for ip, want := range map[string]bool{
		"127.0.0.1": true, "::1": true, "169.254.169.254": true, "fe80::1": true,
		"0.0.0.0": true, "224.0.0.1": true,
		"192.168.1.5": false, "10.0.0.2": false, "93.184.216.34": false,
	} {
		assert.Equal(t, want, forbiddenPeerIP(net.ParseIP(ip)), ip)
	}
}

func TestFederationTransportRefusesMetadataAndLoopback(t *testing.T) {
	tr := federationTransport()
	for _, addr := range []string{"169.254.169.254:80", "127.0.0.1:1", "localhost:1"} {
		conn, err := tr.DialContext(t.Context(), "tcp", addr)
		if conn != nil {
			conn.Close()
		}
		assert.ErrorContains(t, err, errForbiddenPeerAddress.Error(), addr)
	}
}
