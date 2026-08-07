package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test host literals: net.Resolver.LookupIPAddr resolves an already-numeric host
// without a real DNS lookup, so these are deterministic and network-independent.
func TestValidateCallbackURL_RejectsNonHTTPScheme(t *testing.T) {
	err := ValidateCallbackURL(context.Background(), "ftp://8.8.8.8/")
	assert.Error(t, err)
}

func TestValidateCallbackURL_RejectsMalformedURL(t *testing.T) {
	err := ValidateCallbackURL(context.Background(), "://not a url")
	assert.Error(t, err)
}

func TestValidateCallbackURL_RejectsLoopback(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/", "http://localhost/", "http://[::1]/"} {
		t.Run(raw, func(t *testing.T) {
			err := ValidateCallbackURL(context.Background(), raw)
			assert.Error(t, err, "expected loopback target to be rejected")
		})
	}
}

func TestValidateCallbackURL_RejectsLinkLocal(t *testing.T) {
	err := ValidateCallbackURL(context.Background(), "http://169.254.1.1/")
	assert.Error(t, err)
}

func TestValidateCallbackURL_RejectsCloudMetadataAddress(t *testing.T) {
	// AC11 calls this address out explicitly, even though it's already covered by
	// the link-local check above — assert it independently so a future refactor of
	// the link-local branch can't silently stop covering it.
	err := ValidateCallbackURL(context.Background(), "http://169.254.169.254/latest/meta-data/")
	assert.Error(t, err)
}

func TestValidateCallbackURL_RejectsPrivateRanges(t *testing.T) {
	for _, raw := range []string{
		"http://10.0.0.5/",
		"http://172.16.5.5/",
		"http://192.168.1.1/",
	} {
		t.Run(raw, func(t *testing.T) {
			err := ValidateCallbackURL(context.Background(), raw)
			assert.Error(t, err, "expected private-range target to be rejected")
		})
	}
}

func TestValidateCallbackURL_AcceptsPublicAddress(t *testing.T) {
	// 8.8.8.8 (Google DNS) is a stable, well-known public IP literal — used here
	// purely as "not loopback/link-local/private", no actual network call is made
	// for a POST, and LookupIPAddr on an IP literal doesn't touch the network either.
	err := ValidateCallbackURL(context.Background(), "https://8.8.8.8/hook")
	assert.NoError(t, err)
}

func TestValidateCallbackURL_RejectsEmptyHost(t *testing.T) {
	err := ValidateCallbackURL(context.Background(), "http:///path")
	assert.Error(t, err)
}
