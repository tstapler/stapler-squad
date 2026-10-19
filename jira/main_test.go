package jira

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestMain swaps in go-keyring's in-memory provider so tests never touch the
// real OS keychain.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}
