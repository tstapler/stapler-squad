// Package model holds the types shared by the docs-coverage checker.
package model

import "strings"

// FeatureID is a registry feature id (for example "alias:invoke").
type FeatureID string

// DocSlug is an in-app help slug listed in docLoader.ts DOC_FILES.
type DocSlug string

// WebsiteDocID is a website docs-collection id (for example "help/getting-started").
type WebsiteDocID string

// Grandfathered is the sidecar status value for a pre-existing docs gap recorded by the baseline.
const Grandfathered = "grandfathered"

// Sidecar is one docs/registry/docs-coverage/<encoded-id>.json file.
type Sidecar struct {
	ID         FeatureID `json:"id"`
	UserFacing *bool     `json:"userFacing,omitempty"`
	Help       string    `json:"help,omitempty"`
	Website    string    `json:"website,omitempty"`
	Tour       string    `json:"tour,omitempty"`
	Exempt     string    `json:"exempt,omitempty"`
	Approver   string    `json:"approver,omitempty"`
	Status     string    `json:"status,omitempty"`
}

// HelpRef splits "slug#anchor".
func (s Sidecar) HelpRef() (DocSlug, string) {
	base, anchor := SplitRef(s.Help)
	return DocSlug(base), anchor
}

// WebsiteRef splits "docs/id#anchor".
func (s Sidecar) WebsiteRef() (WebsiteDocID, string) {
	base, anchor := SplitRef(s.Website)
	return WebsiteDocID(base), anchor
}

// SplitRef splits "target#anchor" into its parts.
func SplitRef(ref string) (target, anchor string) {
	target, anchor, _ = strings.Cut(ref, "#")
	return target, anchor
}

// Feature is the subset of a registry entry the gate reads.
type Feature struct {
	ID          FeatureID
	Type        string
	Name        string
	Description string
	UserFacing  *bool // explicit registry value, nil when absent
	Files       []string
	// InBackendDir is true when the entry lives under features/backend/.
	InBackendDir bool
}

// DefaultUserFacing is the registry-only default, decided by location (ADR-005):
// entries under frontend/ and the root are user facing, entries under backend/
// are not, whatever their "type" field says.
func (f Feature) DefaultUserFacing() bool {
	if f.UserFacing != nil {
		return *f.UserFacing
	}
	return !f.InBackendDir
}

// Status is a feature's coverage state.
type Status string

// Coverage states.
const (
	StatusCovered       Status = "covered"
	StatusExempt        Status = "exempt"
	StatusGrandfathered Status = "grandfathered-gap"
	StatusGap           Status = "gap"
)
