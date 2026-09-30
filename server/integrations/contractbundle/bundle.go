// Package contractbundle builds and verifies a signed contract bundle: a directory of
// schemas and fixtures plus a SHA-256 manifest (checksums.sha256) and a detached Ed25519
// signature over the manifest's exact bytes (checksums.sig).
//
// Verification fails closed. Any problem -- unknown or revoked signer, bad signature, a
// hash mismatch, a duplicate or unsafe manifest path, a file the manifest does not list, a
// listed file that is missing -- returns an error wrapping ErrVerification and no result, so
// a caller can never use part of a bundle it has not fully verified.
//
// Not implemented yet: trust-root rotation chains (a new root signed by the old one) and
// two-root emergency revocation. A TrustRoot here is pinned out of band and replaced whole.
package contractbundle

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Bundle metadata file names. Both are excluded from the manifest they describe.
const (
	ManifestFile  = "checksums.sha256"
	SignatureFile = "checksums.sig"

	maxMetaFileBytes = 1 << 20
	trustRootVersion = 1
)

// ErrVerification wraps every failure to build trust in a bundle.
var ErrVerification = errors.New("contract bundle verification failed")

var hexSHA256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

func fail(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrVerification, fmt.Sprintf(format, args...))
}

// TrustKey is one authorised signing key.
type TrustKey struct {
	ID        string    `json:"id"`
	PublicKey string    `json:"public_key"` // base64 (std) of the 32-byte Ed25519 public key
	NotBefore time.Time `json:"not_before"`
	Revoked   bool      `json:"revoked"`
}

// TrustRoot is the pinned set of keys allowed to sign bundles.
type TrustRoot struct {
	Version int        `json:"version"`
	Keys    []TrustKey `json:"keys"`
}

// Signature is the content of checksums.sig.
type Signature struct {
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"` // base64 (std) Ed25519 signature over the manifest bytes
}

// Verified is the result of a successful Verify.
type Verified struct {
	KeyID          string
	ManifestSHA256 string
	Files          []string
}

// ParseTrustRoot strictly decodes a trust root and rejects malformed or ambiguous ones.
func ParseTrustRoot(data []byte) (*TrustRoot, error) {
	var root TrustRoot
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&root); err != nil {
		return nil, fail("trust root: %v", err)
	}
	if root.Version != trustRootVersion {
		return nil, fail("trust root: unsupported version %d", root.Version)
	}
	if len(root.Keys) == 0 {
		return nil, fail("trust root: no keys")
	}
	seen := map[string]bool{}
	for _, k := range root.Keys {
		raw, err := base64.StdEncoding.DecodeString(k.PublicKey)
		if k.ID == "" || err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fail("trust root: invalid key %q", k.ID)
		}
		if seen[k.ID] {
			return nil, fail("trust root: duplicate key id %q", k.ID)
		}
		seen[k.ID] = true
	}
	return &root, nil
}

// BuildManifest hashes every regular file under dir (except the bundle metadata files) and
// returns the manifest: one "<sha256 hex>  <relative path>" line per file, sorted by path.
func BuildManifest(dir string) ([]byte, error) {
	files, err := listBundleFiles(dir)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	for _, rel := range files {
		sum, err := hashFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&buf, "%s  %s\n", sum, rel)
	}
	if buf.Len() == 0 {
		return nil, fail("bundle has no files")
	}
	return buf.Bytes(), nil
}

// Sign produces the checksums.sig content for manifest, naming keyID as the signer.
func Sign(manifest []byte, keyID string, priv ed25519.PrivateKey) ([]byte, error) {
	if keyID == "" {
		return nil, fail("signer key id is required")
	}
	return json.Marshal(Signature{KeyID: keyID, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, manifest))})
}

// Verify checks the bundle in dir against root. at is when the bundle was issued; a key
// whose NotBefore is later than at is not yet authorised to have signed it.
func Verify(dir string, root *TrustRoot, at time.Time) (*Verified, error) {
	manifest, err := readMeta(dir, ManifestFile)
	if err != nil {
		return nil, err
	}
	keyID, err := verifySignature(dir, manifest, root, at)
	if err != nil {
		return nil, err
	}
	entries, err := parseManifest(manifest)
	if err != nil {
		return nil, err
	}
	onDisk, err := listBundleFiles(dir)
	if err != nil {
		return nil, err
	}
	if err := requireSameFileSet(entries, onDisk); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for rel, want := range entries {
		got, err := hashFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		if got != want {
			return nil, fail("hash mismatch for %q", rel)
		}
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	sum := sha256.Sum256(manifest)
	return &Verified{KeyID: keyID, ManifestSHA256: hex.EncodeToString(sum[:]), Files: paths}, nil
}

func verifySignature(dir string, manifest []byte, root *TrustRoot, at time.Time) (string, error) {
	sigBytes, err := readMeta(dir, SignatureFile)
	if err != nil {
		return "", err
	}
	var sig Signature
	dec := json.NewDecoder(bytes.NewReader(sigBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sig); err != nil {
		return "", fail("signature file: %v", err)
	}
	key := findKey(root, sig.KeyID)
	switch {
	case key == nil:
		return "", fail("unknown signer %q", sig.KeyID)
	case key.Revoked:
		return "", fail("signer %q is revoked", sig.KeyID)
	case at.Before(key.NotBefore):
		return "", fail("signer %q was not yet authorised at issuance", sig.KeyID)
	}
	pub, _ := base64.StdEncoding.DecodeString(key.PublicKey) // length validated by ParseTrustRoot
	raw, err := base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil || len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, manifest, raw) {
		return "", fail("invalid signature")
	}
	return sig.KeyID, nil
}

func findKey(root *TrustRoot, id string) *TrustKey {
	if root == nil {
		return nil
	}
	for i := range root.Keys {
		if root.Keys[i].ID == id {
			return &root.Keys[i]
		}
	}
	return nil
}

// parseManifest strictly decodes manifest bytes into path -> sha256 hex.
func parseManifest(manifest []byte) (map[string]string, error) {
	if len(manifest) == 0 || manifest[len(manifest)-1] != '\n' {
		return nil, fail("manifest must be non-empty and end with a newline")
	}
	entries := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(manifest), "\n"), "\n") {
		sum, rel, ok := strings.Cut(line, "  ")
		if !ok || !hexSHA256Re.MatchString(sum) {
			return nil, fail("malformed manifest line %q", line)
		}
		if !safeRelPath(rel) {
			return nil, fail("unsafe manifest path %q", rel)
		}
		if _, dup := entries[rel]; dup {
			return nil, fail("duplicate manifest path %q", rel)
		}
		entries[rel] = sum
	}
	return entries, nil
}

func safeRelPath(p string) bool {
	switch {
	case p == "" || p == ManifestFile || p == SignatureFile:
		return false
	case strings.ContainsAny(p, "\\\x00") || path.IsAbs(p):
		return false
	}
	return path.Clean(p) == p && p != "." && !strings.HasPrefix(p, "../")
}

func requireSameFileSet(entries map[string]string, onDisk []string) error {
	onDiskSet := make(map[string]bool, len(onDisk))
	for _, rel := range onDisk {
		onDiskSet[rel] = true
		if _, listed := entries[rel]; !listed {
			return fail("file %q is present but not listed in the manifest", rel)
		}
	}
	for rel := range entries {
		if !onDiskSet[rel] {
			return fail("manifest lists %q but it is missing", rel)
		}
	}
	return nil
}

// listBundleFiles returns the sorted slash-separated relative paths of every file under
// dir except the metadata files. Anything that is not a plain file or directory (a
// symlink, say) is rejected so a bundle cannot point outside itself.
func listBundleFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fail("%q is not a regular file", p)
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		if rel != ManifestFile && rel != SignatureFile {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrVerification) {
			return nil, err
		}
		return nil, fail("walk bundle: %v", err)
	}
	sort.Strings(files)
	return files, nil
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", fail("open %q: %v", filepath.Base(p), err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fail("read %q: %v", filepath.Base(p), err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readMeta(dir, name string) ([]byte, error) {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil, fail("open %s: %v", name, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxMetaFileBytes+1))
	if err != nil || len(data) > maxMetaFileBytes {
		return nil, fail("%s is unreadable or too large", name)
	}
	return data, nil
}
