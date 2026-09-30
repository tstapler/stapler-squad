package server

import (
	"crypto/tls"
	"net"
	"sync"
	"testing"
)

func TestNetworkCertStore_StoreThenLoad_ReflectsUpdate(t *testing.T) {
	initial := map[string]*NetworkCert{
		"192.168.1.5": {Key: "192.168.1.5"},
	}
	store := NewNetworkCertStore(initial)

	if got := store.Load(); len(got) != 1 || got["192.168.1.5"] == nil {
		t.Fatalf("Load() before Store() = %v, want the initial map", got)
	}

	updated := map[string]*NetworkCert{
		"192.168.1.5": {Key: "192.168.1.5"},
		"192.168.1.9": {Key: "192.168.1.9"},
	}
	store.Store(updated)

	got := store.Load()
	if len(got) != 2 || got["192.168.1.9"] == nil {
		t.Fatalf("Load() after Store() = %v, want the updated map containing 192.168.1.9", got)
	}
}

// testLocalAddr is a minimal net.Addr for exercising GetCertificateByLocalAddr
// without a real connection.
type testLocalAddr string

func (a testLocalAddr) Network() string { return "tcp" }
func (a testLocalAddr) String() string  { return string(a) }

// testConn is a minimal net.Conn stub that only implements LocalAddr, which
// is all GetCertificateByLocalAddr's closure reads from tls.ClientHelloInfo.Conn.
type testConn struct {
	net.Conn
	localAddr net.Addr
}

func (c testConn) LocalAddr() net.Addr { return c.localAddr }

// certWithByte builds a NetworkCert whose leaf cert bytes let a test tell
// which Store() call produced the cert a closure invocation returned.
func certWithByte(key string, b byte) *NetworkCert {
	return &NetworkCert{Key: key, Cert: tls.Certificate{Certificate: [][]byte{{b}}}}
}

func assertCertByte(t *testing.T, cert *tls.Certificate, err error, want byte) {
	t.Helper()
	if err != nil {
		t.Fatalf("getCert() = error %v, want nil", err)
	}
	if len(cert.Certificate) == 0 || cert.Certificate[0][0] != want {
		t.Fatalf("getCert() = %v, want cert byte %d", cert, want)
	}
}

func TestGetCertificateByLocalAddr_ReadsLatestStore(t *testing.T) {
	// Only "192.168.1.5" is registered initially, so a connection on
	// "192.168.1.9" falls back to whatever single cert exists (see
	// GetCertificateByLocalAddr's fallback loop) rather than erroring.
	store := NewNetworkCertStore(map[string]*NetworkCert{"192.168.1.5": certWithByte("192.168.1.5", 1)})
	getCert := GetCertificateByLocalAddr(store)
	chi := &tls.ClientHelloInfo{Conn: testConn{localAddr: testLocalAddr("192.168.1.9:443")}}

	cert, err := getCert(chi)
	assertCertByte(t, cert, err, 1)

	// Publishing a new map must be visible immediately -- no restart of the
	// listener or cached closure state.
	store.Store(map[string]*NetworkCert{"192.168.1.5": {Key: "192.168.1.5"}, "192.168.1.9": certWithByte("192.168.1.9", 2)})
	cert, err = getCert(chi)
	assertCertByte(t, cert, err, 2)
}

func TestGetCertificateByLocalAddr_ConcurrentStoreAndLoadIsRaceFree(t *testing.T) {
	store := NewNetworkCertStore(map[string]*NetworkCert{
		"192.168.1.9": {Key: "192.168.1.9", Cert: tls.Certificate{Certificate: [][]byte{{1}}}},
	})
	getCert := GetCertificateByLocalAddr(store)
	chi := &tls.ClientHelloInfo{
		Conn: testConn{localAddr: testLocalAddr("192.168.1.9:443")},
	}

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			store.Store(map[string]*NetworkCert{
				"192.168.1.9": {Key: "192.168.1.9", Cert: tls.Certificate{Certificate: [][]byte{{byte(i)}}}},
			})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if _, err := getCert(chi); err != nil {
				t.Errorf("getCert() = error %v, want nil", err)
			}
		}
	}()

	wg.Wait()
}
