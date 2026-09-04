package sshremote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/tstapler/stapler-squad/config"
)

// configDirContainsBytes walks config.GetConfigDir()'s directory and reports
// whether any file under it contains needle. Not necessarily test-exclusive:
// GetConfigDirForDir honors an ambient STAPLER_SQUAD_TEST_DIR override, which
// can point at a directory another process is also using -- see the WalkDir
// callback's ENOENT handling below. A missing directory means nothing was
// written there, so the answer is trivially false.
func configDirContainsBytes(t *testing.T, needle []byte) bool {
	t.Helper()
	dir, err := config.GetConfigDir()
	if err != nil {
		t.Fatalf("config.GetConfigDir() failed: %v", err)
	}
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		return false
	}

	found := false
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// STAPLER_SQUAD_TEST_DIR can point at a live, shared state dir
			// (config.GetConfigDirForDir honors it), so a file can vanish
			// between WalkDir listing it and stat-ing it here. Benign race
			// for this best-effort scan -- skip it, don't abort the walk.
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil // best-effort scan; unreadable files can't contain the secret via this code path
		}
		if bytes.Contains(content, needle) {
			found = true
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking config dir %s: %v", dir, walkErr)
	}
	return found
}

func TestSetIdentity_And_GetIdentity_RoundTrip(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	ctx := context.Background()

	keyPEM := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nfake\n-----END OPENSSH PRIVATE KEY-----\n")
	if err := ks.SetIdentity(ctx, "prod-box", IdentityKindPrivateKey, keyPEM); err != nil {
		t.Fatalf("SetIdentity failed: %v", err)
	}

	kind, value, err := ks.GetIdentity(ctx, "prod-box")
	if err != nil {
		t.Fatalf("GetIdentity failed: %v", err)
	}
	if kind != IdentityKindPrivateKey {
		t.Errorf("GetIdentity kind = %q, want %q", kind, IdentityKindPrivateKey)
	}
	if !bytes.Equal(value, keyPEM) {
		t.Errorf("GetIdentity value = %q, want %q", value, keyPEM)
	}
}

func TestSetIdentity_And_GetIdentity_RoundTrip_Passphrase(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	ctx := context.Background()

	passphrase := []byte("correct horse battery staple")
	if err := ks.SetIdentity(ctx, "prod-box", IdentityKindPassphrase, passphrase); err != nil {
		t.Fatalf("SetIdentity failed: %v", err)
	}

	kind, value, err := ks.GetIdentity(ctx, "prod-box")
	if err != nil {
		t.Fatalf("GetIdentity failed: %v", err)
	}
	if kind != IdentityKindPassphrase {
		t.Errorf("GetIdentity kind = %q, want %q", kind, IdentityKindPassphrase)
	}
	if !bytes.Equal(value, passphrase) {
		t.Errorf("GetIdentity value = %q, want %q", value, passphrase)
	}
}

// TestSetIdentity_WritesToKeychain_NotToDisk is the Story 3.2.1 primary
// acceptance criterion: SetIdentity writes to the OS keychain (via
// zalando/go-keyring), never to a file under ~/.stapler-squad/.
func TestSetIdentity_WritesToKeychain_NotToDisk(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	ctx := context.Background()

	keyPEM := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nAC1-marker-bytes-should-never-touch-disk\n-----END OPENSSH PRIVATE KEY-----\n")
	if err := ks.SetIdentity(ctx, "prod-box", IdentityKindPrivateKey, keyPEM); err != nil {
		t.Fatalf("SetIdentity failed: %v", err)
	}

	// keyring.Get on the raw envelope key must succeed and, once decoded,
	// must equal keyPEM -- the envelope wraps it (JSON-marshaling a []byte
	// field base64-encodes it), it doesn't lose it.
	raw, err := keyring.Get(keychainService, identityKey("prod-box"))
	if err != nil {
		t.Fatalf("keyring.Get(%q, %q) failed: %v", keychainService, identityKey("prod-box"), err)
	}
	var envelope identityEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("decoding stored envelope failed: %v", err)
	}
	if !bytes.Equal(envelope.Value, keyPEM) {
		t.Errorf("stored keyring envelope value = %q, want %q", envelope.Value, keyPEM)
	}

	if configDirContainsBytes(t, keyPEM) {
		t.Errorf("found private key bytes on disk under the config dir; SetIdentity must only write to the OS keychain")
	}
}

func TestDeleteIdentity_RemovesEntry(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	ctx := context.Background()

	if err := ks.SetIdentity(ctx, "prod-box", IdentityKindPrivateKey, []byte("key-bytes")); err != nil {
		t.Fatalf("SetIdentity failed: %v", err)
	}
	if err := ks.DeleteIdentity(ctx, "prod-box"); err != nil {
		t.Fatalf("DeleteIdentity failed: %v", err)
	}

	if _, _, err := ks.GetIdentity(ctx, "prod-box"); !errors.Is(err, ErrIdentityNotFound) {
		t.Errorf("GetIdentity after delete: err = %v, want wrapping ErrIdentityNotFound", err)
	}
}

func TestGetIdentity_Returns_ErrIdentityNotFound_When_NoneStored(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	ctx := context.Background()

	if _, _, err := ks.GetIdentity(ctx, "never-onboarded"); !errors.Is(err, ErrIdentityNotFound) {
		t.Errorf("GetIdentity err = %v, want wrapping ErrIdentityNotFound", err)
	}
}

// withKeyringFuncOverride swaps the package-level keyring func-var seams for
// the duration of the test, restoring them afterward under keyringMu.
//
// Locking keyringMu *before* restoring the vars (rather than after) is
// load-bearing, not decorative: in the hang-simulation test, the background
// goroutine reads keyringGetFunc while holding keyringMu and then blocks
// inside the override for an arbitrary time. GetIdentity itself gives up on
// ctx expiry without ever synchronizing with that goroutine (it takes the
// ctx.Done() branch of the select, not the channel-receive branch), so
// nothing else orders "goroutine read the func var" before "cleanup writes
// it" except the mutex -- restoring the vars without first acquiring the
// same lock the reader used would be a data race under `go test -race`,
// separate from (and in addition to) the goroutine-leak/next-test
// contamination concern that motivates waiting for the lock at all.
func withKeyringFuncOverride(t *testing.T, get func(service, key string) (string, error), set func(service, key, value string) error, del func(service, key string) error) {
	t.Helper()
	keyringMu.Lock()
	origGet, origSet, origDelete := keyringGetFunc, keyringSetFunc, keyringDeleteFunc
	if get != nil {
		keyringGetFunc = get
	}
	if set != nil {
		keyringSetFunc = set
	}
	if del != nil {
		keyringDeleteFunc = del
	}
	keyringMu.Unlock()

	t.Cleanup(func() {
		keyringMu.Lock()
		keyringGetFunc, keyringSetFunc, keyringDeleteFunc = origGet, origSet, origDelete
		keyringMu.Unlock()
	})
}

// TestSetIdentity_FailsLoud_When_NoKeyringBackendAvailable covers Task
// 3.2.1d's immediate-error half: per research/build-vs-buy.md §3, there is
// deliberately no on-disk fallback. If go-keyring reports "no backend
// available," SetIdentity/GetIdentity must surface that error rather than
// silently succeeding or falling back to a file.
func TestSetIdentity_FailsLoud_When_NoKeyringBackendAvailable(t *testing.T) {
	noBackendErr := errors.New("no backend available: org.freedesktop.secrets was not provided by any .service files")
	withKeyringFuncOverride(t,
		func(service, key string) (string, error) { return "", noBackendErr },
		func(service, key, value string) error { return noBackendErr },
		func(service, key string) error { return noBackendErr },
	)

	ks := NewKeyStore()
	ctx := context.Background()
	keyPEM := []byte("fail-loud-marker-bytes")

	err := ks.SetIdentity(ctx, "prod-box", IdentityKindPrivateKey, keyPEM)
	if err == nil {
		t.Fatal("SetIdentity returned nil error when the keyring backend is unavailable; must fail loud, not silently succeed")
	}
	if !errors.Is(err, noBackendErr) {
		t.Errorf("SetIdentity err = %v, want it to wrap the backend's \"no backend available\" error", err)
	}

	if _, _, getErr := ks.GetIdentity(ctx, "prod-box"); getErr == nil {
		t.Fatal("GetIdentity returned nil error when the keyring backend is unavailable; must fail loud, not silently succeed")
	}

	if configDirContainsBytes(t, keyPEM) {
		t.Errorf("found private key bytes on disk under the config dir after a failed SetIdentity; there must be no on-disk fallback")
	}
}

// TestGetIdentity_ReturnsDeadlineError_When_KeyringHangs covers Task
// 3.2.1d's hang half (pre-mortem.md Failure #3): a headless Secret Service
// D-Bus unlock prompt can block go-keyring's call indefinitely. GetIdentity
// must not block CreateSession forever -- it must return a context-deadline
// error at its timeout budget. Uses a small injected budget (via
// withIdentityTimeout) instead of the production 5s so this test runs fast;
// the mechanism under test (ctx-racing, per session/tmux/ssh_runner.go's
// newSession technique) is identical regardless of the budget's size.
func TestGetIdentity_ReturnsDeadlineError_When_KeyringHangs(t *testing.T) {
	release := make(chan struct{})
	withKeyringFuncOverride(t,
		func(service, key string) (string, error) {
			<-release // never returns until the test unblocks it in cleanup
			return "", errors.New("unblocked after test completed")
		},
		nil,
		nil,
	)
	t.Cleanup(func() { close(release) })

	const testTimeout = 50 * time.Millisecond
	ks := NewKeyStore(withIdentityTimeout(testTimeout))

	start := time.Now()
	_, _, err := ks.GetIdentity(context.Background(), "prod-box")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("GetIdentity returned nil error against a hanging keyring backend; must time out")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("GetIdentity err = %v, want it to wrap context.DeadlineExceeded", err)
	}
	// Generous slack for scheduler jitter -- the point is "bounded by the
	// timeout," not "bounded exactly at it."
	if elapsed > testTimeout+2*time.Second {
		t.Errorf("GetIdentity took %s against a %s timeout budget; did not bound the hang", elapsed, testTimeout)
	}
}

// TestKeyStore_ConcurrentGetSet_NoRace_When_DifferentRemoteNames is the
// Task 3.2.1e concurrency test: 10 goroutines concurrently calling
// SetIdentity/GetIdentity for different remote names must be race-free
// under `go test -race`, per the keyringMu discipline mirrored from
// github/keychain.go.
func TestKeyStore_ConcurrentGetSet_NoRace_When_DifferentRemoteNames(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	ctx := context.Background()

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n * 2)

	for i := 0; i < n; i++ {
		i := i
		remoteName := "remote-" + strconv.Itoa(i)
		go func() {
			defer wg.Done()
			if err := ks.SetIdentity(ctx, remoteName, IdentityKindPrivateKey, []byte(fmt.Sprintf("key-%d", i))); err != nil {
				t.Errorf("SetIdentity(%q) failed: %v", remoteName, err)
			}
		}()
		go func() {
			defer wg.Done()
			// May race ahead of the corresponding SetIdentity above and
			// legitimately observe ErrIdentityNotFound -- this goroutine
			// only exists to exercise concurrent Get/Set under -race, not
			// to assert ordering.
			_, _, _ = ks.GetIdentity(ctx, remoteName)
		}()
	}
	wg.Wait()

	// Sanity: every write actually landed, proving the mutex serializes
	// access without silently dropping writes.
	for i := 0; i < n; i++ {
		remoteName := "remote-" + strconv.Itoa(i)
		_, value, err := ks.GetIdentity(ctx, remoteName)
		if err != nil {
			t.Errorf("GetIdentity(%q) after concurrent run failed: %v", remoteName, err)
			continue
		}
		want := fmt.Sprintf("key-%d", i)
		if string(value) != want {
			t.Errorf("GetIdentity(%q) = %q, want %q", remoteName, value, want)
		}
	}
}

func TestSetIdentity_RejectsEmptyRemoteName(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	if err := ks.SetIdentity(context.Background(), "", IdentityKindPrivateKey, []byte("x")); err == nil {
		t.Error("SetIdentity(\"\") returned nil error, want a validation error")
	}
}

func TestSetIdentity_RejectsUnknownKind(t *testing.T) {
	keyring.MockInit()
	ks := NewKeyStore()
	if err := ks.SetIdentity(context.Background(), "prod-box", IdentityKind("bogus"), []byte("x")); err == nil {
		t.Error("SetIdentity with an unknown IdentityKind returned nil error, want a validation error")
	}
}
