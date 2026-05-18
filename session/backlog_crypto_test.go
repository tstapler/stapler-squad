package session

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEncryptDecryptToken verifies round-trip encryption and decryption
func TestEncryptDecryptToken(t *testing.T) {
	// Generate a test key
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plaintext := "ghp_test1234567890abcdefghijk"

	// Encrypt
	encrypted, err := EncryptToken(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Verify base64 encoding
	if _, err := base64.StdEncoding.DecodeString(encrypted); err != nil {
		t.Fatalf("encrypted value is not valid base64: %v", err)
	}

	// Decrypt
	decrypted, err := DecryptToken(key, encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypted %q does not match plaintext %q", decrypted, plaintext)
	}
}

// TestDecryptWithWrongKey verifies that decryption fails with wrong key
func TestDecryptWithWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key1); err != nil {
		t.Fatalf("generate key1: %v", err)
	}

	key2 := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key2); err != nil {
		t.Fatalf("generate key2: %v", err)
	}

	plaintext := "secret_token_123"

	// Encrypt with key1
	encrypted, err := EncryptToken(key1, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Try to decrypt with key2 (should fail)
	_, err = DecryptToken(key2, encrypted)
	if err == nil {
		t.Error("decrypt with wrong key should have failed")
	}
}

// TestKeySize verifies that functions reject invalid key sizes with key-size errors.
func TestKeySize(t *testing.T) {
	plaintext := "test"

	// Test with 16-byte key (too small)
	badKey := make([]byte, 16)
	_, err := EncryptToken(badKey, plaintext)
	require.Error(t, err, "encrypt with 16-byte key should have failed")
	require.Contains(t, err.Error(), "key", "encrypt error should mention key size")

	// Test with 48-byte key (too large)
	badKey = make([]byte, 48)
	_, err = EncryptToken(badKey, plaintext)
	require.Error(t, err, "encrypt with 48-byte key should have failed")
	require.Contains(t, err.Error(), "key", "encrypt error should mention key size")

	// Same for decrypt
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key) //nolint:errcheck
	encrypted, _ := EncryptToken(key, plaintext)

	_, err = DecryptToken(make([]byte, 16), encrypted)
	require.Error(t, err, "decrypt with 16-byte key should have failed")
	require.Contains(t, err.Error(), "key", "decrypt error should mention key size")
}

// TestEmptyToken verifies encryption of empty strings
func TestEmptyToken(t *testing.T) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plaintext := ""

	encrypted, err := EncryptToken(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt empty string: %v", err)
	}

	decrypted, err := DecryptToken(key, encrypted)
	if err != nil {
		t.Fatalf("decrypt empty string: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypted %q does not match plaintext %q", decrypted, plaintext)
	}
}
