package detection

import (
	"context"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/log"
)

// detectorSnapshot is an immutable, point-in-time view of every per-binary
// StatusDetector currently in effect, plus provenance (which file, if any,
// each binary name's detector came from). byBinary is what DetectForProgram
// consults on every PTY read; provenance backs DetectorProvenance() for
// diagnostics/logging.
type detectorSnapshot struct {
	byBinary   map[string]*StatusDetector
	provenance map[string]string
}

// activeSnapshot holds the currently-published detectorSnapshot.
// snapshotWriteMu is held for the FULL rebuild-then-store sequence (build the
// new snapshot, then Store it) by any future writer (e.g. a later story's
// InitPlugins/reload path) — not just around the Store call itself. This
// mirrors pipelineModeCache's refresh pattern in session/pipeline_engine.go:
// holding the mutex only around the Store would let two concurrent writers'
// Store calls land out of order relative to when each writer *started*
// building its snapshot, so a slower-but-earlier-started rebuild could
// overwrite a faster-but-later-started one's result after the fact — a lost
// update. Serializing the whole build+store sequence guarantees the
// snapshot that ends up published is whichever rebuild started last, never
// stale data clobbering fresher data.
var (
	activeSnapshot  atomic.Pointer[detectorSnapshot]
	snapshotWriteMu sync.Mutex
)

// rebuildCount counts successful rebuildSnapshot calls (one increment per
// Store, not per call — a cancelled-context or directory-scan-failure call
// returns before reaching it). Test-only signal, never read by production
// code: plugin_watcher_test.go uses it to assert that a burst of rapid
// filesystem events collapses into exactly one reload via
// pluginReloadDebounce.
var rebuildCount atomic.Int64

// compiledPatternSetProvider is satisfied by any BinaryDetector that has
// already compiled its patterns into a *PatternSet — currently *PluginDetector
// only, whose PatternSet is compiled once by LoadPluginDir and shared across
// every binary_names entry for that file (see PluginDetector.CompiledPatternSet's
// doc comment). buildSnapshot type-asserts for this to avoid recompiling the
// same regex strings from their string DTOs on every rebuild: without it, a
// plugin file declaring N binary_names paid N redundant NewPatternSet compiles
// per rebuild for identical patterns, including on every periodic safety-net
// tick (see plugin_watcher.go's pluginRescanInterval).
type compiledPatternSetProvider interface {
	CompiledPatternSet() *PatternSet
}

// buildSnapshot constructs a detectorSnapshot from every detector in reg. For
// each registered binary name it reuses an already-compiled PatternSet if the
// detector provides one (compiledPatternSetProvider — plugin detectors), or
// else compiles bd.Patterns() into a fresh PatternSet (built-in detectors,
// whose Patterns() is cheap, code-defined data), wrapping either in a fresh
// *StatusDetector — mirroring the construction shape formerly inlined in
// detector.go's old package-level built-in-detectors cache var. A compile
// failure is logged and that binary name is skipped rather than failing the
// whole snapshot — in practice this can't happen (built-in patterns are
// code-defined and always valid; plugin patterns were already
// regexp-compiled during LoadPluginDir validation) but the snapshot must not
// become unusable for every other binary over one entry.
//
// If provenance is nil, it is built as name -> "" (built-in) for every entry
// in reg — the shape needed when reg is builtins-only, e.g. from init().
func buildSnapshot(reg *DetectorRegistry, provenance map[string]string) *detectorSnapshot {
	byBinary := make(map[string]*StatusDetector, reg.Len())
	names := reg.Names()

	buildProvenance := provenance == nil
	if buildProvenance {
		provenance = make(map[string]string, len(names))
	}

	for _, name := range names {
		bd, ok := reg.Lookup(name)
		if !ok {
			continue
		}

		var ps *PatternSet
		if provider, ok := bd.(compiledPatternSetProvider); ok {
			ps = provider.CompiledPatternSet()
		}
		if ps == nil {
			var err error
			ps, err = NewPatternSet(bd.Patterns())
			if err != nil {
				log.Warn("detector snapshot: skipping binary, pattern compile failed", "binary", name, "error", err)
				continue
			}
		}

		sd := &StatusDetector{}
		sd.patternSet.Store(ps)
		byBinary[name] = sd

		if buildProvenance {
			provenance[name] = ""
		}
	}

	return &detectorSnapshot{byBinary: byBinary, provenance: provenance}
}

// init publishes a built-ins-only snapshot at package load, preserving the
// pre-plugin-system behavior for any code path that never calls a later
// story's InitPlugins: DetectForProgram works out of the box, identically to
// the old package-level built-in-detectors cache var this replaces.
func init() {
	activeSnapshot.Store(buildSnapshot(DefaultRegistry(), nil))
}

// programBinaryName extracts the bare binary name lookupBinaryDetector keys
// its registry lookup on. In real deployments Instance.Program is often not
// a bare binary name: it can be a full command string with arguments (e.g.
// "aider --model ollama_chat/gemma3:1b") or a resolved absolute path (e.g.
// "/usr/local/bin/claude" — see config.GetClaudeCommand()). Both would miss
// an exact-string map lookup keyed by "aider"/"claude", silently falling
// back to the generic detector for every such session.
//
// This mirrors session/instance_tmux.go's isClaude/classifyProgram
// tokenization (split on whitespace, take the first token, filepath.Base()
// it) rather than importing it: package session already imports
// session/detection (see session/claude_controller.go), so the reverse
// import here would create a cycle.
func programBinaryName(program string) string {
	fields := strings.Fields(program)
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

// lookupBinaryDetector returns the *StatusDetector currently published for
// program, and whether one exists. This is the read path DetectForProgram
// uses in place of the old package-level built-in-detectors map index.
func lookupBinaryDetector(program string) (*StatusDetector, bool) {
	snap := activeSnapshot.Load()
	if snap == nil {
		return nil, false
	}
	sd, ok := snap.byBinary[programBinaryName(program)]
	return sd, ok
}

// ResolveDetectorForProgram returns a StatusDetector for program (built-in or user
// plugin) registered in the currently-active snapshot, or (nil, false) if
// none is registered — callers should fall back to NewStatusDetector() in
// that case. This is the first production call site for per-program
// detector resolution (ClaudeController.Start).
//
// The returned detector is a fresh *StatusDetector that shares the
// snapshot's compiled PatternSet (immutable once built — safe to share
// across goroutines, see PatternSet's doc comment) but owns its own
// DetectionEventSink/normalizer. It deliberately does NOT return the
// snapshot's own *StatusDetector pointer: callers call SetSessionID on
// whatever they get back, which mutates DetectionEventSink.sessionID. Since
// every session running the same program (multiple concurrent "claude"
// sessions is the common case) would resolve to that one shared snapshot
// entry, handing out the shared pointer directly would let one session's
// SetSessionID silently reattribute another session's already-in-flight
// detection events.
func ResolveDetectorForProgram(program string) (*StatusDetector, bool) {
	sd, ok := lookupBinaryDetector(program)
	if !ok {
		return nil, false
	}
	fresh := &StatusDetector{normalizer: PTYNormalizer{}}
	fresh.patternSet.Store(sd.patternSet.Load())
	return fresh, true
}

// DetectorProvenance returns a fresh copy of the current snapshot's
// provenance map (binary name -> source: "" for a built-in, a plugin file
// path otherwise). The copy is defensive: callers must never be able to
// mutate the live snapshot's map through the returned value.
func DetectorProvenance() map[string]string {
	snap := activeSnapshot.Load()
	if snap == nil {
		return map[string]string{}
	}
	return maps.Clone(snap.provenance)
}

// asBinaryDetectors adapts LoadPluginDir's []*PluginDetector result to the
// []BinaryDetector shape MergedRegistry expects.
func asBinaryDetectors(pds []*PluginDetector) []BinaryDetector {
	out := make([]BinaryDetector, len(pds))
	for i, pd := range pds {
		out[i] = pd
	}
	return out
}

// rebuildSnapshot rescans dir for detector plugin files and, on success,
// publishes a new detectorSnapshot merging built-ins with whatever plugins
// loaded. The whole directory is rebuilt on every reload rather than
// incrementally patching one changed file, because collision detection
// (duplicate id, duplicate binary name) and override precedence
// (plugin-over-built-in) are directory-global properties: evaluating them
// correctly requires seeing every file in the directory at once, which a
// per-file patch cannot do.
func rebuildSnapshot(ctx context.Context, dir string) error {
	// Cheap up-front check: skip acquiring the write lock and scanning at all
	// if ctx is already cancelled. LoadPluginDir also re-checks ctx once per
	// file in its parse/validate loop (see its doc comment) so a cancellation
	// that lands mid-scan is observed within one file's compile budget rather
	// than only after the whole directory has been processed.
	if err := ctx.Err(); err != nil {
		return err
	}

	snapshotWriteMu.Lock()
	defer snapshotWriteMu.Unlock()

	detectors, errs := LoadPluginDir(ctx, dir)

	for _, e := range errs {
		if e.Field == "directory" {
			// Fatal: the scan itself failed. Leave the previously published
			// snapshot live rather than replacing it with an empty/partial
			// one built from a directory we couldn't even read.
			log.Warn("detector plugin directory scan failed", "dir", e.Path, "field", e.Field, "err", e.Err)
			return e
		}
	}

	for _, e := range errs {
		// "file_count" accompanies a successful partial load (the cap was
		// hit but every file up to it still loaded normally) — it falls
		// through here and is logged like any other per-file rejection,
		// not treated as fatal.
		log.Warn("detector plugin rejected", "file", e.Path, "field", e.Field, "err", e.Err)
	}

	builtins := DefaultRegistry()
	provenance := make(map[string]string, builtins.Len()+len(detectors))
	for _, name := range builtins.Names() {
		provenance[name] = ""
	}
	for _, pd := range detectors {
		provenance[pd.Name()] = pd.SourcePath()
	}

	merged := MergedRegistry(builtins, asBinaryDetectors(detectors))
	snap := buildSnapshot(merged, provenance)
	activeSnapshot.Store(snap)
	rebuildCount.Add(1)

	log.Info("detector plugins loaded", "dir", dir, "plugins", len(detectors), "rejected", len(errs))

	return nil
}
