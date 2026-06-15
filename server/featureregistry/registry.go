// Package featureregistry is the Go counterpart of the TypeScript feature catalog.
// Features are registered from init() functions in server/features/*.go.
package featureregistry

import (
	"fmt"
	"log"
	"sync"
	"testing"
)

// FeatureStatus mirrors the TypeScript catalog's status field.
type FeatureStatus string

const (
	StatusStable       FeatureStatus = "stable"
	StatusExperimental FeatureStatus = "experimental"
	StatusDeprecated   FeatureStatus = "deprecated"
)

// Feature is the Go counterpart of the TypeScript Feature interface.
// Every field is required — zero values are invalid.
type Feature struct {
	ID          string
	Title       string
	Description string
	RPCIDs      []string // proto scope:action IDs this feature implements
	Status      FeatureStatus
	Since       string // semver e.g. "1.4.0"
}

var (
	mu       sync.RWMutex
	registry = map[string]Feature{}
	rpcIndex = map[string]string{} // rpcID → featureID
)

// Register adds a Feature to the global registry. Panics on duplicate ID.
// Call from init() in feature files (server/features/*.go).
func Register(f Feature) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[f.ID]; exists {
		panic(fmt.Sprintf("featureregistry: duplicate feature ID %q", f.ID))
	}
	registry[f.ID] = f
	for _, rpcID := range f.RPCIDs {
		if existing, conflict := rpcIndex[rpcID]; conflict {
			panic(fmt.Sprintf("featureregistry: duplicate RPC ID %q claimed by both %q and %q",
				rpcID, existing, f.ID))
		}
		rpcIndex[rpcID] = f.ID
	}
}

// All returns a copy of all registered features.
func All() []Feature {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Feature, 0, len(registry))
	for _, f := range registry {
		out = append(out, f)
	}
	return out
}

// LookupRPC returns the Feature that declares the given RPC ID, or nil.
func LookupRPC(rpcID string) *Feature {
	mu.RLock()
	defer mu.RUnlock()
	if fid, ok := rpcIndex[rpcID]; ok {
		f := registry[fid]
		return &f
	}
	return nil
}

// MustValidate asserts registry completeness in tests.
// Call from TestMain: featureregistry.MustValidate(m).
func MustValidate(m *testing.M) {
	mu.RLock()
	defer mu.RUnlock()
	var errs []string
	for _, f := range registry {
		if f.ID == "" {
			errs = append(errs, "feature with empty ID")
		}
		if f.Title == "" {
			errs = append(errs, fmt.Sprintf("feature %q has empty Title", f.ID))
		}
		if len(f.RPCIDs) == 0 {
			errs = append(errs, fmt.Sprintf("feature %q has no RPCIDs", f.ID))
		}
	}
	if len(errs) > 0 {
		for _, e := range errs {
			log.Printf("featureregistry: VALIDATION ERROR: %s", e)
		}
		panic("featureregistry: validation failed — see errors above")
	}
}
