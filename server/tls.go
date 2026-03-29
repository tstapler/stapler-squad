package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

const (
	certFileName = "tls-cert.pem"
	keyFileName  = "tls-key.pem"
	caFileName   = "tls-ca.pem"
)

// TLSPaths holds the file paths for the generated TLS certificate set.
type TLSPaths struct {
	CertFile string
	KeyFile  string
	CAFile   string
}

// EnsureTLSCerts generates a self-signed CA and server certificate if they do
// not already exist, and returns their paths.  The certificate is valid for the
// provided hostname/IP SANs.
func EnsureTLSCerts(hostnames []string) (*TLSPaths, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get config dir: %w", err)
	}

	paths := &TLSPaths{
		CertFile: filepath.Join(configDir, certFileName),
		KeyFile:  filepath.Join(configDir, keyFileName),
		CAFile:   filepath.Join(configDir, caFileName),
	}

	// Reuse existing cert only if it is unexpired AND covers all required SANs.
	if tlsValidForHosts(paths.CertFile, hostnames) {
		log.InfoLog.Printf("tls: reusing existing certificate at %s", paths.CertFile)
		return paths, nil
	}
	if tlsValid(paths.CertFile) {
		log.InfoLog.Printf("tls: existing certificate does not cover all required SANs %v — regenerating", hostnames)
	}

	log.InfoLog.Printf("tls: generating self-signed certificate for %v", hostnames)

	// 1. Generate CA key + cert
	caKey, caCert, caCertPEM, err := generateCA()
	if err != nil {
		return nil, fmt.Errorf("generate CA: %w", err)
	}

	// 2. Generate server key + cert signed by the CA
	certPEM, keyPEM, err := generateServerCert(caKey, caCert, hostnames)
	if err != nil {
		return nil, fmt.Errorf("generate server cert: %w", err)
	}

	// 3. Write files
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(paths.CAFile, caCertPEM, 0644); err != nil {
		return nil, fmt.Errorf("write CA: %w", err)
	}
	if err := os.WriteFile(paths.CertFile, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(paths.KeyFile, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}

	log.InfoLog.Printf("tls: certificate written to %s", paths.CertFile)
	log.InfoLog.Printf("tls: CA certificate (for import on phones) at %s", paths.CAFile)
	return paths, nil
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

// tlsValid returns true if certFile exists and its leaf certificate has not
// expired (with a 7-day safety margin).
func tlsValid(certFile string) bool {
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

// tlsValidForHosts returns true if the cert at certFile is unexpired AND
// contains all of the required hostnames/IPs in its SANs.
func tlsValidForHosts(certFile string, required []string) bool {
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
	// Check expiry first.
	if !time.Now().Add(7 * 24 * time.Hour).Before(cert.NotAfter) {
		return false
	}
	// Verify every required hostname/IP is covered by the cert.
	for _, h := range required {
		if ip := net.ParseIP(h); ip != nil {
			found := false
			for _, certIP := range cert.IPAddresses {
				if certIP.Equal(ip) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		} else {
			if err := cert.VerifyHostname(h); err != nil {
				return false
			}
		}
	}
	return true
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
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(2 * 365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	for _, h := range hostnames {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
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
