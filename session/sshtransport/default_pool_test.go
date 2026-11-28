package sshtransport

import (
	"testing"

	"golang.org/x/crypto/ssh"
)

// session/sshremote and the health prober attach to the pool a session's runner
// dialed through, so a runner built without WithSSHClientPool must use it.
func TestNewSSHRunner_DefaultsToTheProcessWidePool(t *testing.T) {
	t.Parallel()
	r := NewSSHRunner(SSHTarget{Name: "default-pool", Addr: "127.0.0.1:1"}, ssh.ClientConfig{})
	if r.pool == nil || r.pool != DefaultSSHClientPool() {
		t.Fatalf("NewSSHRunner pool = %p, want DefaultSSHClientPool() %p", r.pool, DefaultSSHClientPool())
	}
}
