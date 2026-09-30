package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"
)

// BuildSelfSignedTLSFixture generates a single self-signed leaf certificate
// (acting as its own trust root) for hostnames, and returns a server-side
// *tls.Config alongside the client-side *x509.CertPool that trusts it.
//
// This is a fresh, test-only implementation rather than a wrapper around
// server/tls.go's generateCA/generateServerCert: those helpers are
// unexported in package server, and testutil must remain importable from
// packages (like server/services' external services_test package) that
// cannot reach unexported production symbols. Shared by server's and
// server/services' TLS-listener integration tests (StartRemote ALPN/HTTP2
// negotiation, native ConnectRPC streaming) so both packages exercise the
// same fixture-generation logic instead of maintaining duplicate ECDSA key
// gen + x509.CreateCertificate implementations.
func BuildSelfSignedTLSFixture(t *testing.T, hostnames []string) (*tls.Config, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("BuildSelfSignedTLSFixture: generate key: %v", err)
	}

	var dnsNames []string
	var ipAddrs []net.IP
	for _, h := range hostnames {
		if ip := net.ParseIP(h); ip != nil {
			ipAddrs = append(ipAddrs, ip)
		} else {
			dnsNames = append(dnsNames, h)
		}
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("BuildSelfSignedTLSFixture: generate serial: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "stapler-squad-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(2 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("BuildSelfSignedTLSFixture: create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("BuildSelfSignedTLSFixture: parse certificate: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(cert)

	tlsCert := tls.Certificate{Certificate: [][]byte{certDER}, PrivateKey: key}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}, pool
}

// FindFreePort asks the OS for an ephemeral port on localhost and
// immediately releases it, so tests exercising an explicit, non-zero port
// never hardcode a real, recognizable port number -- in particular, never
// the production stapler-squad port, which risks confusion with (or, if
// this test pattern is ever copied into code that actually binds, collision
// with) a real running instance on the developer's machine.
func FindFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("FindFreePort: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
