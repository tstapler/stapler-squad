package featureregistry_test

import (
	"testing"

	"github.com/tstapler/stapler-squad/server/featureregistry"
)

func TestRegister_DuplicatePanics(t *testing.T) {
	featureregistry.ResetForTest()
	t.Cleanup(featureregistry.ResetForTest)

	f := featureregistry.Feature{
		ID:     "test-dedup",
		Title:  "Test Dedup",
		RPCIDs: []string{"test:dedup"},
		Status: featureregistry.StatusStable,
		Since:  "1.0.0",
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration, got none")
		}
	}()

	featureregistry.Register(f)
	featureregistry.Register(f) // should panic
}

func TestLookupRPC_UnknownReturnsNil(t *testing.T) {
	featureregistry.ResetForTest()
	t.Cleanup(featureregistry.ResetForTest)

	result := featureregistry.LookupRPC("nonexistent:rpc")
	if result != nil {
		t.Errorf("expected nil for unknown RPC, got %+v", result)
	}
}

func TestLookupRPC_KnownReturnsFeature(t *testing.T) {
	featureregistry.ResetForTest()
	t.Cleanup(featureregistry.ResetForTest)

	f := featureregistry.Feature{
		ID:     "lookup-test-feature",
		Title:  "Lookup Test Feature",
		RPCIDs: []string{"lookup-test:get"},
		Status: featureregistry.StatusStable,
		Since:  "1.0.0",
	}
	featureregistry.Register(f)

	result := featureregistry.LookupRPC("lookup-test:get")
	if result == nil {
		t.Fatal("expected feature, got nil")
	}
	if result.ID != "lookup-test-feature" {
		t.Errorf("expected ID %q, got %q", "lookup-test-feature", result.ID)
	}
}

func TestAll_ReturnsRegisteredFeatures(t *testing.T) {
	featureregistry.ResetForTest()
	t.Cleanup(featureregistry.ResetForTest)

	f := featureregistry.Feature{
		ID:     "all-test-feature",
		Title:  "All Test Feature",
		RPCIDs: []string{"all-test:list"},
		Status: featureregistry.StatusStable,
		Since:  "1.0.0",
	}
	featureregistry.Register(f)

	all := featureregistry.All()
	found := false
	for _, registered := range all {
		if registered.ID == f.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("feature %q not found in All() results", f.ID)
	}
}

func TestMustValidate_PanicsOnEmptyTitle(t *testing.T) {
	featureregistry.ResetForTest()
	t.Cleanup(featureregistry.ResetForTest)

	// Register a feature with an empty title to trigger validation failure
	featureregistry.Register(featureregistry.Feature{
		ID:     "empty-title-feature",
		Title:  "", // intentionally empty
		RPCIDs: []string{"empty-title:get"},
		Status: featureregistry.StatusStable,
		Since:  "1.0.0",
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustValidate to panic on empty title, got none")
		}
	}()

	featureregistry.MustValidate(nil)
}
