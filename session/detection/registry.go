package detection

import (
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection/binaries"
)

// DefaultRegistry returns a DetectorRegistry pre-populated with all built-in binary detectors.
func DefaultRegistry() *DetectorRegistry {
	r := NewDetectorRegistry()
	r.Register(binaries.NewClaudeDetector())
	r.Register(binaries.NewGeminiDetector())
	r.Register(binaries.NewAiderDetector())
	r.Register(binaries.NewOpencodeDetector())
	r.Register(binaries.NewAgyDetector())
	r.Register(binaries.NewPiDetector())
	return r
}

// sourcePathProvider is satisfied by any BinaryDetector that can report the
// file it was loaded from (e.g. a plugin detector loaded from a TOML file).
// It is declared inline rather than referencing a concrete plugin type so
// this file has no dependency on how/where plugin detectors are defined.
type sourcePathProvider interface {
	SourcePath() string
}

// MergedRegistry returns a new DetectorRegistry containing every detector
// from builtins plus every detector in plugins. A plugin whose Name()
// matches a built-in (or another plugin already merged) replaces it in the
// result rather than growing the registry. builtins is never mutated — the
// caller's registry is left untouched.
func MergedRegistry(builtins *DetectorRegistry, plugins []BinaryDetector) *DetectorRegistry {
	merged := NewDetectorRegistry()
	for _, name := range builtins.Names() {
		if d, ok := builtins.Lookup(name); ok {
			merged.Upsert(d)
		}
	}

	for _, d := range plugins {
		name := d.Name()
		if _, overriding := merged.Lookup(name); overriding {
			if sp, ok := d.(sourcePathProvider); ok {
				log.Info("detector plugin overrides built-in", "binary", name, "file", sp.SourcePath())
			} else {
				log.Info("detector plugin overrides built-in", "binary", name)
			}
		}
		merged.Upsert(d)
	}

	return merged
}
