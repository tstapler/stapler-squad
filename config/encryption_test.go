package config

import (
	"encoding/base64"
	"testing"
)

// TestGetOrCreateEncryptionKey verifies key generation and persistence
func TestGetOrCreateEncryptionKey(t *testing.T) {
	cfg := &Config{}

	// First call should generate a key
	key1, err := cfg.GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("GetOrCreateEncryptionKey: %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("expected 32-byte key, got %d", len(key1))
	}

	if cfg.MachineEncryptionKey == "" {
		t.Error("MachineEncryptionKey should be set after GetOrCreateEncryptionKey")
	}

	// Verify it's valid base64
	decoded, err := base64.StdEncoding.DecodeString(cfg.MachineEncryptionKey)
	if err != nil {
		t.Errorf("MachineEncryptionKey is not valid base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("decoded key should be 32 bytes, got %d", len(decoded))
	}

	// Second call should return the same key
	key2, err := cfg.GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("second GetOrCreateEncryptionKey: %v", err)
	}

	if string(key1) != string(key2) {
		t.Error("second call returned a different key")
	}
}

// TestGetOrCreateEncryptionKeyWithExistingKey verifies reuse of existing key
func TestGetOrCreateEncryptionKeyWithExistingKey(t *testing.T) {
	// Simulate a key already being stored
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	encodedKey := base64.StdEncoding.EncodeToString(key)

	cfg := &Config{
		MachineEncryptionKey: encodedKey,
	}

	retrieved, err := cfg.GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("GetOrCreateEncryptionKey: %v", err)
	}

	if string(retrieved) != string(key) {
		t.Error("retrieved key does not match original")
	}
}
