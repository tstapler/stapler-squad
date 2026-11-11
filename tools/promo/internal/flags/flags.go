// Package flags loads .github/promo-flags.json. On pull requests the file is read
// from the base blob so a PR cannot flip its own gate.
package flags

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
)

// Path is the repo-relative location of the flags file.
const Path = ".github/promo-flags.json"

// Flags are the three rollout switches.
type Flags struct {
	FreshnessEnforce    bool `json:"freshnessEnforce"`
	DocsCoverageEnforce bool `json:"docsCoverageEnforce"`
	GuardsEnforce       bool `json:"guardsEnforce"`
}

// Defaults are used when the file is absent (bootstrap window before it lands).
func Defaults() Flags {
	return Flags{FreshnessEnforce: false, DocsCoverageEnforce: false, GuardsEnforce: true}
}

// Parse reads a flags file. All three keys must be present booleans; anything else
// is an error, never the defaults.
func Parse(data []byte) (Flags, error) {
	var raw struct {
		Freshness *bool `json:"freshnessEnforce"`
		Docs      *bool `json:"docsCoverageEnforce"`
		Guards    *bool `json:"guardsEnforce"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&raw); err != nil {
		return Flags{}, fmt.Errorf("parse %s: %w", Path, err)
	}
	if raw.Freshness == nil || raw.Docs == nil || raw.Guards == nil {
		return Flags{}, fmt.Errorf("parse %s: freshnessEnforce, docsCoverageEnforce and guardsEnforce must all be booleans", Path)
	}
	return Flags{FreshnessEnforce: *raw.Freshness, DocsCoverageEnforce: *raw.Docs, GuardsEnforce: *raw.Guards}, nil
}

// FromBase reads the flags from the base blob; absent means Defaults.
func FromBase(r *gitbase.Repo, base string) (Flags, error) {
	data, err := r.ReadBlob(base, Path)
	if errors.Is(err, gitbase.ErrNotFound) {
		return Defaults(), nil
	}
	if err != nil {
		return Flags{}, err
	}
	return Parse(data)
}

// FromTree reads the flags from the working tree, for push, schedule and
// workflow_dispatch runs where the tree is already merged; absent means Defaults.
func FromTree(path string) (Flags, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Flags{}, err
	}
	return Parse(data)
}
