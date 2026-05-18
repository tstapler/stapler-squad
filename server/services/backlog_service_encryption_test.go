package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"testing"

	"connectrpc.com/connect"
	"crypto/rand"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// newTestEncryptionService builds a BacklogService wired to a narrow source backend for encryption tests.
func newTestEncryptionService(backend itemSourceBackend, cfg *config.Config) *BacklogService {
	return &BacklogService{sourceBackend: backend, cfg: cfg}
}

// testStorageRecorder captures calls to backlog methods
type testStorageRecorder struct {
	createdData map[string]session.ItemSourceData
	updatedData map[string]session.ItemSourceData
}

func (tsr *testStorageRecorder) CreateItemSource(ctx context.Context, data session.ItemSourceData) (*session.ItemSourceData, error) {
	if tsr.createdData == nil {
		tsr.createdData = make(map[string]session.ItemSourceData)
	}
	tsr.createdData[data.PluginID] = data
	return &data, nil
}

func (tsr *testStorageRecorder) UpdateItemSource(ctx context.Context, id string, update session.ItemSourceUpdate) (*session.ItemSourceData, error) {
	if tsr.updatedData == nil {
		tsr.updatedData = make(map[string]session.ItemSourceData)
	}
	data := session.ItemSourceData{}
	if update.Config != nil {
		data.Config = *update.Config
	}
	tsr.updatedData[id] = data
	return &data, nil
}

// TestCreateItemSourceEncryptsToken verifies tokens are encrypted in CreateItemSource
func TestCreateItemSourceEncryptsToken(t *testing.T) {
	// Create a test config with encryption key
	cfg := &config.Config{}
	key, err := cfg.GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("GetOrCreateEncryptionKey: %v", err)
	}

	storage := &testStorageRecorder{}
	svc := newTestEncryptionService(storage, cfg)

	req := &connect.Request[sessionv1.CreateItemSourceRequest]{
		Msg: &sessionv1.CreateItemSourceRequest{
			PluginId:    "github_issues",
			DisplayName: "My GitHub",
			Token:       "ghp_test1234567890abcdefghijk",
			ConfigJson:  `{"owner":"myorg","repo":"myrepo"}`,
		},
	}

	resp, err := svc.CreateItemSource(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateItemSource: %v", err)
	}

	if resp == nil || resp.Msg == nil {
		t.Fatal("response is nil")
	}

	// Check that the stored config has encrypted flag
	stored, ok := storage.createdData["github_issues"]
	if !ok {
		t.Fatal("no item source was stored")
	}

	var cfg_data map[string]interface{}
	if err := json.Unmarshal([]byte(stored.Config), &cfg_data); err != nil {
		t.Fatalf("parse stored config: %v", err)
	}

	encrypted, ok := cfg_data["encrypted"].(bool)
	if !ok {
		t.Error("stored config does not have 'encrypted' field")
	}
	if !encrypted {
		t.Error("encrypted flag should be true")
	}

	// Verify token is base64 (encrypted)
	token_str, ok := cfg_data["token"].(string)
	if !ok {
		t.Fatal("token is not a string")
	}

	if _, err := base64.StdEncoding.DecodeString(token_str); err != nil {
		t.Errorf("token is not valid base64: %v", err)
	}

	// Verify we can decrypt it with the key
	decrypted, err := session.DecryptToken(key, token_str)
	if err != nil {
		t.Fatalf("decrypt token: %v", err)
	}

	if decrypted != req.Msg.Token {
		t.Errorf("decrypted token %q does not match original %q", decrypted, req.Msg.Token)
	}
}

// TestUpdateItemSourceEncryptsToken verifies tokens are encrypted in UpdateItemSource
func TestUpdateItemSourceEncryptsToken(t *testing.T) {
	cfg := &config.Config{}
	key, err := cfg.GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("GetOrCreateEncryptionKey: %v", err)
	}

	storage := &testStorageRecorder{}
	svc := newTestEncryptionService(storage, cfg)

	req := &connect.Request[sessionv1.UpdateItemSourceRequest]{
		Msg: &sessionv1.UpdateItemSourceRequest{
			SourceId: "source-123",
			Token:    "ghp_newsecret7890abcdefghijklmno",
		},
	}

	resp, err := svc.UpdateItemSource(context.Background(), req)
	if err != nil {
		t.Fatalf("UpdateItemSource: %v", err)
	}

	if resp == nil || resp.Msg == nil {
		t.Fatal("response is nil")
	}

	// Check that the updated config has encrypted flag
	updated, ok := storage.updatedData["source-123"]
	if !ok {
		t.Fatal("no item source was updated")
	}

	var cfg_data map[string]interface{}
	if err := json.Unmarshal([]byte(updated.Config), &cfg_data); err != nil {
		t.Fatalf("parse updated config: %v", err)
	}

	encrypted, ok := cfg_data["encrypted"].(bool)
	if !ok {
		t.Error("updated config does not have 'encrypted' field")
	}
	if !encrypted {
		t.Error("encrypted flag should be true")
	}

	token_str, ok := cfg_data["token"].(string)
	if !ok {
		t.Fatal("token is not a string")
	}

	decrypted, err := session.DecryptToken(key, token_str)
	if err != nil {
		t.Fatalf("decrypt token: %v", err)
	}

	if decrypted != req.Msg.Token {
		t.Errorf("decrypted token %q does not match original %q", decrypted, req.Msg.Token)
	}
}

// TestCreateItemSourceWithoutConfigDoesNotEncrypt verifies backward compatibility
func TestCreateItemSourceWithoutConfigDoesNotEncrypt(t *testing.T) {
	// Service without config should store tokens unencrypted
	storage := &testStorageRecorder{}
	svc := newTestEncryptionService(storage, nil)

	req := &connect.Request[sessionv1.CreateItemSourceRequest]{
		Msg: &sessionv1.CreateItemSourceRequest{
			PluginId:    "github_issues",
			DisplayName: "My GitHub",
			Token:       "ghp_test1234567890abcdefghijk",
		},
	}

	resp, err := svc.CreateItemSource(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateItemSource: %v", err)
	}

	if resp == nil {
		t.Fatal("response is nil")
	}

	stored, ok := storage.createdData["github_issues"]
	if !ok {
		t.Fatal("no item source was stored")
	}

	var cfg_data map[string]interface{}
	if err := json.Unmarshal([]byte(stored.Config), &cfg_data); err != nil {
		t.Fatalf("parse stored config: %v", err)
	}

	// Should NOT have encrypted flag
	if encrypted, ok := cfg_data["encrypted"].(bool); ok && encrypted {
		t.Error("unencrypted service should not set encrypted flag")
	}

	// Token should be plaintext
	if token, ok := cfg_data["token"].(string); ok {
		if token != req.Msg.Token {
			t.Errorf("token should be stored plaintext, got %q", token)
		}
	}
}

// TestCreateItemSourceEncryptionRoundTrip verifies the full store-and-retrieve cycle:
// a token stored via CreateItemSource can be decrypted back to the original plaintext.
func TestCreateItemSourceEncryptionRoundTrip(t *testing.T) {
	cfg := &config.Config{}
	key, err := cfg.GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("GetOrCreateEncryptionKey: %v", err)
	}

	storage := &testStorageRecorder{}
	svc := newTestEncryptionService(storage, cfg)

	originalToken := "ghp_roundtrip_secret_abc123"

	req := &connect.Request[sessionv1.CreateItemSourceRequest]{
		Msg: &sessionv1.CreateItemSourceRequest{
			PluginId:    "github_issues",
			DisplayName: "Round-trip Test",
			Token:       originalToken,
			ConfigJson:  `{"owner":"testorg","repo":"testrepo"}`,
		},
	}

	_, err = svc.CreateItemSource(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateItemSource: %v", err)
	}

	// Retrieve the stored record.
	stored, ok := storage.createdData["github_issues"]
	if !ok {
		t.Fatal("no item source was stored")
	}

	// Parse stored config.
	var cfgData map[string]interface{}
	if err := json.Unmarshal([]byte(stored.Config), &cfgData); err != nil {
		t.Fatalf("parse stored config: %v", err)
	}

	// Extract the encrypted token.
	tokenStr, ok := cfgData["token"].(string)
	if !ok {
		t.Fatal("token field missing or not a string")
	}

	// Decrypt the token using the same key and verify it matches the original.
	decrypted, err := session.DecryptToken(key, tokenStr)
	if err != nil {
		t.Fatalf("decrypt stored token: %v", err)
	}

	if decrypted != originalToken {
		t.Errorf("round-trip decrypted token %q does not match original %q", decrypted, originalToken)
	}
}

// TestSyncLoopDecryptsToken verifies SyncLoop decrypts tokens before passing to plugins
func TestSyncLoopDecryptsToken(t *testing.T) {
	// Generate encryption key
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plainToken := "ghp_verysecrettoken123456789"
	encrypted, err := session.EncryptToken(key, plainToken)
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}

	// Build encrypted config
	encryptedConfig := map[string]interface{}{
		"token":     encrypted,
		"encrypted": true,
		"owner":     "myorg",
		"repo":      "myrepo",
	}
	encryptedConfigJSON, _ := json.Marshal(encryptedConfig)

	// Create a mock SyncLoop
	syncLoop := session.NewSyncLoopWithKeyProvider(nil, nil, func() ([]byte, error) {
		return key, nil
	})

	// Test decryption
	decrypted, err := syncLoop.TestDecryptConfigToken(string(encryptedConfigJSON))
	if err != nil {
		t.Fatalf("decryptConfigToken: %v", err)
	}

	// Verify plaintext token is in decrypted config
	var decryptedCfg map[string]interface{}
	if err := json.Unmarshal([]byte(decrypted), &decryptedCfg); err != nil {
		t.Fatalf("parse decrypted config: %v", err)
	}

	if decryptedCfg["token"] != plainToken {
		t.Errorf("decrypted token %q does not match original %q", decryptedCfg["token"], plainToken)
	}

	// Encrypted flag should be removed
	if _, ok := decryptedCfg["encrypted"].(bool); ok {
		t.Error("encrypted flag should be removed from decrypted config")
	}
}
