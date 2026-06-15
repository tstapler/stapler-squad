package features_test

import (
	"os"
	"testing"

	// Side-effect import: triggers all init() registrations in server/features
	_ "github.com/tstapler/stapler-squad/server/features"
	"github.com/tstapler/stapler-squad/server/featureregistry"
)

func TestMain(m *testing.M) {
	featureregistry.MustValidate(m)
	os.Exit(m.Run())
}

func TestAllFeaturesHaveRequiredFields(t *testing.T) {
	all := featureregistry.All()
	if len(all) == 0 {
		t.Fatal("no features registered — init() calls may not have run")
	}

	for _, f := range all {
		t.Run(f.ID, func(t *testing.T) {
			if f.ID == "" {
				t.Error("ID is empty")
			}
			if f.Title == "" {
				t.Errorf("Title is empty for feature %q", f.ID)
			}
			if f.Description == "" {
				t.Errorf("Description is empty for feature %q", f.ID)
			}
			if len(f.RPCIDs) == 0 {
				t.Errorf("RPCIDs is empty for feature %q", f.ID)
			}
		})
	}
}

func TestExpectedFeaturesAreRegistered(t *testing.T) {
	expectedIDs := []string{
		"session-create",
		"session-list",
		"session-delete",
		"review-queue-list",
		"review-queue-acknowledge",
		"terminal-render",
		"unfinished-work",
	}

	all := featureregistry.All()
	registered := make(map[string]bool, len(all))
	for _, f := range all {
		registered[f.ID] = true
	}

	for _, id := range expectedIDs {
		if !registered[id] {
			t.Errorf("expected feature %q to be registered, but it was not found", id)
		}
	}
}

func TestLookupRPCReturnsCorrectFeature(t *testing.T) {
	tests := []struct {
		rpcID     string
		featureID string
	}{
		{"session:create", "session-create"},
		{"session:list", "session-list"},
		{"session:delete", "session-delete"},
		{"review-queue:list", "review-queue-list"},
		{"review-queue:acknowledge", "review-queue-acknowledge"},
		{"session:stream-terminal", "terminal-render"},
		{"unfinished:list", "unfinished-work"},
	}

	for _, tt := range tests {
		t.Run(tt.rpcID, func(t *testing.T) {
			f := featureregistry.LookupRPC(tt.rpcID)
			if f == nil {
				t.Fatalf("LookupRPC(%q) returned nil", tt.rpcID)
			}
			if f.ID != tt.featureID {
				t.Errorf("LookupRPC(%q): expected feature ID %q, got %q", tt.rpcID, tt.featureID, f.ID)
			}
		})
	}
}
