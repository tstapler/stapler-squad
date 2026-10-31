package telemetry

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTcpExt_should_ExtractNamedCounters(t *testing.T) {
	t.Parallel()
	const netstat = `TcpExt: SyncookiesSent ListenOverflows ListenDrops TCPTimeouts
TcpExt: 1 42 57 9
IpExt: InNoRoutes InOctets
IpExt: 3 1000
`
	got := parseTcpExt(strings.NewReader(netstat), "ListenOverflows", "ListenDrops")
	assert.Equal(t, map[string]int64{"ListenOverflows": 42, "ListenDrops": 57}, got)
}

func TestParseTcpExt_should_ReturnEmpty_When_NoTcpExtSection(t *testing.T) {
	t.Parallel()
	assert.Empty(t, parseTcpExt(strings.NewReader("IpExt: A\nIpExt: 1\n"), "ListenDrops"))
}
