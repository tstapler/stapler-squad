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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

const (
	certFileName     = "tls-cert.pem"
	keyFileName      = "tls-key.pem"
	caFileName       = "tls-ca.pem"
	certHashFileName = "tls-cert.hash"
)

// TLSPaths holds the file paths for the generated TLS certificate set.
type TLSPaths struct {
	CertFile string
	KeyFile  string
	CAFile   string
}

// EnsureTLSCerts ensures a stable CA exists and issues/reissues a server
// certificate when the SAN list changes or the server cert nears expiry.
//
// The CA is intentionally kept stable across SAN changes so that phones only
// need to import it once. Only the server cert (signed by the stable CA) is
// replaced when hostnames change — the CA file on disk is never overwritten
// unless it is missing or within 30 days of expiry.
func EnsureTLSCerts(hostnames []string) (*TLSPaths, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get config dir: %w", err)
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	paths := &TLSPaths{
		CertFile: filepath.Join(configDir, certFileName),
		KeyFile:  filepath.Join(configDir, keyFileName),
		CAFile:   filepath.Join(configDir, caFileName),
	}
	hashFile := filepath.Join(configDir, certHashFileName)

	// Step 1: ensure a stable CA (only regenerate if absent or near expiry).
	caKey, caCert, caChanged, err := ensureCA(paths.CAFile)
	if err != nil {
		return nil, fmt.Errorf("ensure CA: %w", err)
	}

	// Step 2: reuse the server cert if SANs and expiry are still valid AND the CA
	// has not been rotated. If the CA changed, the existing server cert is no longer
	// trusted by clients that imported the new CA, so force regeneration.
	if caChanged {
		_ = os.Remove(hashFile) // invalidate cached SAN hash so certCurrent returns false
		log.InfoLog.Printf("tls: CA rotated — forcing server certificate regeneration")
	}
	want := sanHash(hostnames)
	if certCurrent(paths.CertFile, hashFile, want) {
		log.InfoLog.Printf("tls: reusing existing certificate at %s", paths.CertFile)
		return paths, nil
	}

	log.InfoLog.Printf("tls: (re)issuing server certificate for %v", hostnames)

	certPEM, keyPEM, err := generateServerCert(caKey, caCert, hostnames)
	if err != nil {
		return nil, fmt.Errorf("generate server cert: %w", err)
	}
	if err := os.WriteFile(paths.CertFile, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(paths.KeyFile, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}
	if err := os.WriteFile(hashFile, []byte(want), 0644); err != nil {
		return nil, fmt.Errorf("write cert hash: %w", err)
	}

	log.InfoLog.Printf("tls: certificate written to %s", paths.CertFile)
	log.InfoLog.Printf("tls: CA certificate (import once on phones) at %s", paths.CAFile)
	return paths, nil
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
			log.InfoLog.Printf("tls: reusing existing CA (expires %s)", c.NotAfter.Format("2006-01-02"))
			return k, c, false, nil
		}
		log.InfoLog.Printf("tls: CA near expiry (%s), regenerating", c.NotAfter.Format("2006-01-02"))
	}

	log.InfoLog.Printf("tls: generating new CA certificate")
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

// LoadTLSConfig returns a *tls.Config from the given certificate files.
func LoadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load key pair: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
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

	tmpl := &x509.Certificate{
		SerialNumber: newSerial(),
		Subject: pkix.Name{
			Organization: []string{"Stapler Squad"},
			CommonName:   "stapler-squad",
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(2 * 365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		DNSNames:    hostnames,
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
