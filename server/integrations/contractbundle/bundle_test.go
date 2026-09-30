package contractbundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const signerID = "release-key-1"

var issuedAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

type signedBundle struct {
	dir  string
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
	root *TrustRoot
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func trustRootFor(t *testing.T, id string, pub ed25519.PublicKey, mutate func(*TrustKey)) *TrustRoot {
	t.Helper()
	key := TrustKey{ID: id, PublicKey: base64.StdEncoding.EncodeToString(pub), NotBefore: issuedAt.Add(-time.Hour)}
	if mutate != nil {
		mutate(&key)
	}
	raw, err := json.Marshal(TrustRoot{Version: 1, Keys: []TrustKey{key}})
	require.NoError(t, err)
	root, err := ParseTrustRoot(raw)
	require.NoError(t, err)
	return root
}

func newSignedBundle(t *testing.T) *signedBundle {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	dir := t.TempDir()
	writeFile(t, dir, "schemas/a.schema.json", `{"type":"object"}`)
	writeFile(t, dir, "fixtures/a.json", `{"x":1}`)
	b := &signedBundle{dir: dir, pub: pub, priv: priv, root: trustRootFor(t, signerID, pub, nil)}
	b.resign(t)
	return b
}

func (b *signedBundle) resign(t *testing.T) {
	t.Helper()
	manifest, err := BuildManifest(b.dir)
	require.NoError(t, err)
	sig, err := Sign(manifest, signerID, b.priv)
	require.NoError(t, err)
	writeFile(t, b.dir, ManifestFile, string(manifest))
	writeFile(t, b.dir, SignatureFile, string(sig))
}

func TestVerify_should_ReturnKeyAndManifestHash_When_BundleIsIntact(t *testing.T) {
	b := newSignedBundle(t)

	got, err := Verify(b.dir, b.root, issuedAt)

	require.NoError(t, err)
	assert.Equal(t, signerID, got.KeyID)
	assert.Regexp(t, `^[0-9a-f]{64}$`, got.ManifestSHA256)
	assert.Equal(t, []string{"fixtures/a.json", "schemas/a.schema.json"}, got.Files)
}

func TestVerify_should_FailClosed_When_BundleIsTamperedWith(t *testing.T) {
	cases := []struct {
		name    string
		tamper  func(t *testing.T, b *signedBundle)
		wantErr string
	}{
		{"artifact content altered", func(t *testing.T, b *signedBundle) {
			writeFile(t, b.dir, "fixtures/a.json", `{"x":2}`)
		}, "hash mismatch"},
		{"extra unlisted file added", func(t *testing.T, b *signedBundle) {
			writeFile(t, b.dir, "fixtures/sneaky.json", `{}`)
		}, "not listed"},
		{"listed file removed", func(t *testing.T, b *signedBundle) {
			require.NoError(t, os.Remove(filepath.Join(b.dir, "fixtures/a.json")))
		}, "missing"},
		{"manifest altered after signing", func(t *testing.T, b *signedBundle) {
			m, err := os.ReadFile(filepath.Join(b.dir, ManifestFile))
			require.NoError(t, err)
			writeFile(t, b.dir, ManifestFile, strings.Replace(string(m), "a", "b", 1))
		}, "invalid signature"},
		{"signature bytes corrupted", func(t *testing.T, b *signedBundle) {
			writeFile(t, b.dir, SignatureFile, `{"key_id":"`+signerID+`","signature":"AAAA"}`)
		}, "invalid signature"},
		{"signed by a different key under the trusted key id", func(t *testing.T, b *signedBundle) {
			_, other, err := ed25519.GenerateKey(rand.Reader)
			require.NoError(t, err)
			b.priv = other
			b.resign(t)
		}, "invalid signature"},
		{"signature file names an unknown signer", func(t *testing.T, b *signedBundle) {
			manifest, err := os.ReadFile(filepath.Join(b.dir, ManifestFile))
			require.NoError(t, err)
			sig, err := Sign(manifest, "someone-else", b.priv)
			require.NoError(t, err)
			writeFile(t, b.dir, SignatureFile, string(sig))
		}, "unknown signer"},
		{"signature file has unknown fields", func(t *testing.T, b *signedBundle) {
			writeFile(t, b.dir, SignatureFile, `{"key_id":"`+signerID+`","signature":"AAAA","extra":1}`)
		}, "signature file"},
		{"signature file missing", func(t *testing.T, b *signedBundle) {
			require.NoError(t, os.Remove(filepath.Join(b.dir, SignatureFile)))
		}, "open"},
		{"a symlink is smuggled in", func(t *testing.T, b *signedBundle) {
			require.NoError(t, os.Symlink("/etc/hosts", filepath.Join(b.dir, "fixtures/link.json")))
		}, "not a regular file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newSignedBundle(t)
			tc.tamper(t, b)

			got, err := Verify(b.dir, b.root, issuedAt)

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrVerification)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Nil(t, got, "a failed verification must return no result")
		})
	}
}

func TestVerify_should_RefuseSigner_When_KeyIsRevokedOrNotYetEffective(t *testing.T) {
	b := newSignedBundle(t)

	revoked := trustRootFor(t, signerID, b.pub, func(k *TrustKey) { k.Revoked = true })
	_, errRevoked := Verify(b.dir, revoked, issuedAt)

	future := trustRootFor(t, signerID, b.pub, func(k *TrustKey) { k.NotBefore = issuedAt.Add(time.Hour) })
	_, errFuture := Verify(b.dir, future, issuedAt)

	assert.ErrorContains(t, errRevoked, "revoked")
	assert.ErrorContains(t, errFuture, "not yet authorised")
	assert.ErrorIs(t, errRevoked, ErrVerification)
	assert.ErrorIs(t, errFuture, ErrVerification)
}

func TestVerify_should_RejectManifest_When_PathsAreDuplicateOrUnsafe(t *testing.T) {
	zero := strings.Repeat("0", 64)
	cases := map[string]string{
		"duplicate path":     zero + "  a.json\n" + zero + "  a.json\n",
		"parent traversal":   zero + "  ../escape.json\n",
		"absolute path":      zero + "  /etc/passwd\n",
		"unclean path":       zero + "  a/../b.json\n",
		"lists the manifest": zero + "  " + ManifestFile + "\n",
		"short hash":         "abcd  a.json\n",
		"uppercase hash":     strings.Repeat("A", 64) + "  a.json\n",
		"missing newline":    zero + "  a.json",
		"empty":              "",
		"blank line":         zero + "  a.json\n\n",
	}
	for name, manifest := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseManifest([]byte(manifest))
			assert.ErrorIs(t, err, ErrVerification)
		})
	}
}

func TestParseTrustRoot_should_RejectAmbiguousOrMalformedRoots(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	b64 := base64.StdEncoding.EncodeToString(pub)
	cases := map[string]string{
		"wrong version":    `{"version":2,"keys":[{"id":"k","public_key":"` + b64 + `"}]}`,
		"no keys":          `{"version":1,"keys":[]}`,
		"duplicate ids":    `{"version":1,"keys":[{"id":"k","public_key":"` + b64 + `"},{"id":"k","public_key":"` + b64 + `"}]}`,
		"short public key": `{"version":1,"keys":[{"id":"k","public_key":"AAAA"}]}`,
		"empty id":         `{"version":1,"keys":[{"id":"","public_key":"` + b64 + `"}]}`,
		"unknown field":    `{"version":1,"keys":[{"id":"k","public_key":"` + b64 + `","admin":true}]}`,
		"not json":         `nope`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseTrustRoot([]byte(raw))
			assert.ErrorIs(t, err, ErrVerification)
		})
	}
}

func TestVerify_should_Fail_When_TrustRootIsNil(t *testing.T) {
	b := newSignedBundle(t)

	_, err := Verify(b.dir, nil, issuedAt)

	assert.ErrorIs(t, err, ErrVerification)
}
