package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

const caFileName = "tls-ca.pem"

// NetworkCert is one leaf certificate scoped to a single network (e.g. the
// loopback interface or a single LAN IP), signed by the shared local CA.
type NetworkCert struct {
	Key  string // stable identifier for this network, e.g. "192.168.1.135"
	SANs []string
	Cert tls.Certificate
}

func networkCertFileName(key string) string { return "tls-cert-" + sanitizeKey(key) + ".pem" }
func networkKeyFileName(key string) string  { return "tls-key-" + sanitizeKey(key) + ".pem" }
func networkHashFileName(key string) string { return "tls-cert-" + sanitizeKey(key) + ".hash" }

// sanitizeKey makes a network key safe to use as a filename component (IPv6
// addresses contain ':', which is not valid in a path segment).
func sanitizeKey(key string) string {
	return strings.NewReplacer(":", "_", "/", "_").Replace(key)
}

// EnsureNetworkTLSCerts ensures a stable CA exists and issues/reissues one
// leaf certificate per network, keyed by the map key (typically the IP the
// server listens on for that network). Each leaf cert's SAN list contains
// only that network's own hostnames/IP — never another network's — so
// adding, removing, or renaming one network never forces regeneration of
// another network's cert.
//
// The CA is intentionally kept stable across SAN changes so that phones only
// need to import it once. Leaf certs (signed by the stable CA) are replaced
// only when their own network's SANs change or they near expiry — the CA
// file on disk is never overwritten unless it is missing or within 30 days
// of expiry.
func EnsureNetworkTLSCerts(networks map[string][]string) (caFile string, certs map[string]*NetworkCert, err error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", nil, fmt.Errorf("get config dir: %w", err)
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", nil, fmt.Errorf("create config dir: %w", err)
	}
	caFile = filepath.Join(configDir, caFileName)

	// Step 1: ensure a stable CA (only regenerate if absent or near expiry).
	caKey, caCert, caChanged, err := ensureCA(caFile)
	if err != nil {
		return "", nil, fmt.Errorf("ensure CA: %w", err)
	}

	certs = make(map[string]*NetworkCert, len(networks))
	for key, sans := range networks {
		certFile := filepath.Join(configDir, networkCertFileName(key))
		keyFile := filepath.Join(configDir, networkKeyFileName(key))
		hashFile := filepath.Join(configDir, networkHashFileName(key))

		// If the CA was rotated, every leaf cert is now untrusted by clients
		// that imported the new CA, so force regeneration of each network's cert.
		if caChanged {
			_ = os.Remove(hashFile)
		}

		want := sanHash(sans)
		if certCurrent(certFile, hashFile, want) {
			log.Info("tls: reusing existing certificate", "network", key, "cert", certFile)
			cert, loadErr := tls.LoadX509KeyPair(certFile, keyFile)
			if loadErr != nil {
				return "", nil, fmt.Errorf("load cert for network %s: %w", key, loadErr)
			}
			certs[key] = &NetworkCert{Key: key, SANs: sans, Cert: cert}
			continue
		}

		log.Info("tls: (re)issuing certificate for network", "network", key, "sans", sans)

		certPEM, keyPEM, genErr := generateServerCert(caKey, caCert, sans)
		if genErr != nil {
			return "", nil, fmt.Errorf("generate cert for network %s: %w", key, genErr)
		}
		if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
			return "", nil, fmt.Errorf("write cert for network %s: %w", key, err)
		}
		if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
			return "", nil, fmt.Errorf("write key for network %s: %w", key, err)
		}
		if err := os.WriteFile(hashFile, []byte(want), 0644); err != nil {
			return "", nil, fmt.Errorf("write cert hash for network %s: %w", key, err)
		}

		cert, pairErr := tls.X509KeyPair(certPEM, keyPEM)
		if pairErr != nil {
			return "", nil, fmt.Errorf("parse generated cert for network %s: %w", key, pairErr)
		}
		certs[key] = &NetworkCert{Key: key, SANs: sans, Cert: cert}
	}

	log.Info("tls: CA certificate (import once on phones)", "ca", caFile)
	return caFile, certs, nil
}

// GetCertificateByLocalAddr returns a tls.Config.GetCertificate callback that
// selects the leaf certificate matching the local IP a connection was
// accepted on. The server binds one listener across all interfaces, but each
// network still only ever presents the certificate scoped to it.
func GetCertificateByLocalAddr(certs map[string]*NetworkCert) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if chi.Conn != nil {
			if host, _, err := net.SplitHostPort(chi.Conn.LocalAddr().String()); err == nil {
				if nc, ok := certs[host]; ok {
					return &nc.Cert, nil
				}
			}
		}
		for _, nc := range certs {
			return &nc.Cert, nil
		}
		return nil, fmt.Errorf("no TLS certificate available")
	}
}

// ensureCA loads the CA from disk if it exists and is not nearing expiry.
// Otherwise it generates a new CA, writes it to disk, and returns it.
// The CA private key file is stored alongside the CA cert as tls-ca-key.pem.
const caKeyFileName = "tls-ca-key.pem"

// ensureCA loads the CA from disk if it exists and is not nearing expiry.
// Otherwise it generates a new CA, writes it to disk, and returns it.
// caChanged is true when a new CA was generated; the caller must then
// regenerate the server certificate to keep them in sync.
func ensureCA(caFile string) (caKey *ecdsa.PrivateKey, caCert *x509.Certificate, caChanged bool, err error) {
	configDir := filepath.Dir(caFile)
	caKeyFile := filepath.Join(configDir, caKeyFileName)

	// Try to load existing CA.
	if k, c, ok := loadCA(caFile, caKeyFile); ok {
		// Regenerate only if within 30 days of expiry.
		if time.Now().Add(30 * 24 * time.Hour).Before(c.NotAfter) {
			log.Info("tls: reusing existing CA", "expires", c.NotAfter.Format("2006-01-02"))
			return k, c, false, nil
		}
		log.Info("tls: CA near expiry, regenerating", "expires", c.NotAfter.Format("2006-01-02"))
	}

	log.Info("tls: generating new CA certificate")
	caKey, caCert, caCertPEM, err := generateCA()
	if err != nil {
		return nil, nil, false, err
	}
	if err := os.WriteFile(caFile, caCertPEM, 0644); err != nil {
		return nil, nil, false, fmt.Errorf("write CA cert: %w", err)
	}

	caKeyDER, err := x509.MarshalECPrivateKey(caKey)
	if err != nil {
		return nil, nil, false, fmt.Errorf("marshal CA key: %w", err)
	}
	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: caKeyDER})
	if err := os.WriteFile(caKeyFile, caKeyPEM, 0600); err != nil {
		return nil, nil, false, fmt.Errorf("write CA key: %w", err)
	}

	return caKey, caCert, true, nil
}

// loadCA reads the CA cert and key from disk. Returns (nil, nil, false) on any error.
func loadCA(caFile, caKeyFile string) (*ecdsa.PrivateKey, *x509.Certificate, bool) {
	certData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, nil, false
	}
	block, _ := pem.Decode(certData)
	if block == nil {
		return nil, nil, false
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, false
	}

	keyData, err := os.ReadFile(caKeyFile)
	if err != nil {
		return nil, nil, false
	}
	keyBlock, _ := pem.Decode(keyData)
	if keyBlock == nil {
		return nil, nil, false
	}
	caKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, false
	}

	return caKey, caCert, true
}

// sanHash returns a stable hex hash of the sorted hostname list. Any change to
// the set of hostnames produces a different hash, triggering regeneration.
func sanHash(hostnames []string) string {
	sorted := make([]string, len(hostnames))
	copy(sorted, hostnames)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(h[:])
}

// certCurrent returns true if the cert file exists, is not nearing expiry, and
// the stored SAN hash matches want.
func certCurrent(certFile, hashFile, want string) bool {
	// Check stored hash first — cheapest test.
	stored, err := os.ReadFile(hashFile)
	if err != nil || strings.TrimSpace(string(stored)) != want {
		return false
	}

	// Check cert expiry.
	data, err := os.ReadFile(certFile)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	return time.Now().Add(7 * 24 * time.Hour).Before(cert.NotAfter)
}

func generateCA() (*ecdsa.PrivateKey, *x509.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: newSerial(),
		Subject: pkix.Name{
			Organization: []string{"Stapler Squad Local CA"},
			CommonName:   "Stapler Squad CA",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err
	}

	parsed, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return key, parsed, certPEM, nil
}

func generateServerCert(caKey *ecdsa.PrivateKey, caCert *x509.Certificate, hostnames []string) (certPEM, keyPEM []byte, err error) {
	key, genErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if genErr != nil {
		return nil, nil, genErr
	}

	// RFC 6125 / browser hostname validation matches a literal IP address
	// against the certificate's iPAddress SAN entries only, never its dNSName
	// entries -- a dotted-decimal string stuffed into DNSNames (as this used
	// to do for every entry) is silently ignored when the client connects by
	// IP, causing a hostname-mismatch error.
	var dnsNames []string
	var ipAddrs []net.IP
	for _, h := range hostnames {
		if ip := net.ParseIP(h); ip != nil {
			ipAddrs = append(ipAddrs, ip)
		} else {
			dnsNames = append(dnsNames, h)
		}
	}

	tmpl := &x509.Certificate{
		SerialNumber: newSerial(),
		Subject: pkix.Name{
			Organization: []string{"Stapler Squad"},
			CommonName:   "stapler-squad",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(2 * 365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		DNSNames:    dnsNames,
		IPAddresses: ipAddrs,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, createErr := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if createErr != nil {
		return nil, nil, createErr
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, marshalErr := x509.MarshalECPrivateKey(key)
	if marshalErr != nil {
		return nil, nil, marshalErr
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func newSerial() *big.Int {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return new(big.Int).SetBytes(b)
}
