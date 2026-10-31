package server

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestAcceptErrorKind_should_ClassifyErrors(t *testing.T) {
	assert.Equal(t, "closed", acceptErrorKind(net.ErrClosed))
	assert.Equal(t, "fd_exhausted", acceptErrorKind(&net.OpError{Op: "accept", Err: os.NewSyscallError("accept", syscall.EMFILE)}))
	assert.Equal(t, "other", acceptErrorKind(errors.New("boom")))
}

func TestParseTcpExtCounter_should_ReadNamedCounter(t *testing.T) {
	data := []byte("TcpExt: SyncookiesSent ListenOverflows ListenDrops\nTcpExt: 0 7 9\nIpExt: InNoRoutes\nIpExt: 1\n")
	n, ok := parseTcpExtCounter(data, "ListenOverflows")
	assert.True(t, ok)
	assert.EqualValues(t, 7, n)
	n, ok = parseTcpExtCounter(data, "ListenDrops")
	assert.True(t, ok)
	assert.EqualValues(t, 9, n)
	_, ok = parseTcpExtCounter(data, "Missing")
	assert.False(t, ok)
}

func TestMeteredListener_should_PassThroughAccept(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ln := meterListener(listenerMain, inner)
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, aerr := ln.Accept()
		if aerr == nil {
			_ = c.Close()
		}
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	_ = c.Close()
	<-done
	require.NoError(t, ln.Close())
	_, err = ln.Accept()
	assert.Equal(t, "closed", acceptErrorKind(err))
}

func TestTLSHandshakeWriter_should_SwallowHandshakeLinesOnly(t *testing.T) {
	w := tlsHandshakeWriter{name: listenerRemote}
	n, err := w.Write([]byte("http: TLS handshake error from 127.0.0.1:1: EOF\n"))
	require.NoError(t, err)
	assert.Positive(t, n)
}

func TestRegisterOSSocketMetrics_should_ExportFDGauges(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	require.NoError(t, registerOSSocketMetrics(mp.Meter("t")))
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	got := map[string]float64{}
	for _, m := range rm.ScopeMetrics[0].Metrics {
		if g, ok := m.Data.(metricdata.Gauge[int64]); ok && len(g.DataPoints) > 0 {
			got[m.Name] = float64(g.DataPoints[0].Value)
		}
	}
	assert.Positive(t, got["process.open_fds"])
	assert.Greater(t, got["process.fd_limit"], got["process.open_fds"])
}
