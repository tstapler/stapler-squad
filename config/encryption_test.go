package config

import (
	"encoding/base64"
	"sync"
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

// TestGetOrCreateEncryptionKey_ConcurrentCallers_AllReturnTheSameKey guards
// against the race between concurrent first-callers reading/writing
// MachineEncryptionKey unprotected (each could generate and persist its own
// key, and callers could observe a torn/partial value) — see lazyMu on
// Config.
func TestGetOrCreateEncryptionKey_ConcurrentCallers_AllReturnTheSameKey(t *testing.T) {
	cfg := &Config{}

	const goroutines = 32
	keys := make([][]byte, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			keys[i], errs[i] = cfg.GetOrCreateEncryptionKey()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: GetOrCreateEncryptionKey: %v", i, err)
		}
	}
	for i := 1; i < goroutines; i++ {
		if string(keys[i]) != string(keys[0]) {
			t.Errorf("goroutine %d got a different key than goroutine 0", i)
		}
	}
}

// TestGetOrCreateClaimantHostID_ConcurrentCallers_AllReturnTheSameID guards
// against the same unprotected-race pattern as
// TestGetOrCreateEncryptionKey_ConcurrentCallers_AllReturnTheSameKey, applied
// to ClaimantHostID.
func TestGetOrCreateClaimantHostID_ConcurrentCallers_AllReturnTheSameID(t *testing.T) {
	cfg := &Config{}

	const goroutines = 32
	ids := make([]string, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			ids[i], errs[i] = cfg.GetOrCreateClaimantHostID()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: GetOrCreateClaimantHostID: %v", i, err)
		}
	}
	for i := 1; i < goroutines; i++ {
		if ids[i] != ids[0] {
			t.Errorf("goroutine %d got a different claimant host id than goroutine 0", i)
		}
	}
}
