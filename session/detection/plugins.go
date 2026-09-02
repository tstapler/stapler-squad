// Package detection: this file implements the TOML→DTO parsing layer for
// detector plugins (schema v1, see
// project_plans/detector-plugins/decisions/ADR-003-plugin-toml-schema-v1.md).
// Schema v1 has no `priority` key — ordering within a status category is
// declaration order, matching what PatternSet.MatchLines actually does. The
// DTOs defined here (pluginFile, patternEntry) are an internal parsing
// representation only; they never escape the loader and must not be passed
// to code outside this package.
package detection

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection/dtypes"
)

// patternEntry is the raw TOML representation of one [[patterns]] block.
type patternEntry struct {
	Name        string `toml:"name"`
	Regex       string `toml:"regex"`
	Status      string `toml:"status"`
	Description string `toml:"description"`
}

// pluginFile is the raw TOML representation of one detector plugin file.
type pluginFile struct {
	ID          string         `toml:"id"`
	Version     string         `toml:"version"`
	BinaryNames []string       `toml:"binary_names"`
	Patterns    []patternEntry `toml:"patterns"`
}

// parsePluginFile decodes data (the contents of the file at path, used only
// for error messages) into a pluginFile. Unknown keys — including `priority`,
// which schema v1 deliberately omits (ADR-003) — are rejected loudly via
// DisallowUnknownFields rather than silently ignored.
func parsePluginFile(path string, data []byte) (*pluginFile, error) {
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var pf pluginFile
	if err := dec.Decode(&pf); err != nil {
		// go-toml/v2's StrictMissingError.Error() is a generic "strict mode:
		// fields in the document are missing in the target struct" — it does
		// not name the offending key on its own. StrictMissingError.String()
		// renders per-key human context (source line + highlighted key) that
		// does, so append it when present to give a loud, actionable error
		// naming exactly which key (e.g. binary_name, priority) was rejected.
		var strictErr *toml.StrictMissingError
		if errors.As(err, &strictErr) {
			return nil, fmt.Errorf("failed to parse detector plugin %s: %w\n%s", path, err, strictErr.String())
		}
		return nil, fmt.Errorf("failed to parse detector plugin %s: %w", path, err)
	}
	return &pf, nil
}

// validStatusKeys lists the ten valid `status` values, in the same order as
// the switch in statusField and the fields of dtypes.StatusPatterns. Used to
// build error text when a plugin file's status key doesn't match any of
// them. This slice and statusField's switch must be changed together with
// dtypes.StatusPatterns — there is exactly one table (this one).
var validStatusKeys = []string{
	"ready",
	"processing",
	"needs_approval",
	"input_required",
	"error",
	"tests_failing",
	"idle",
	"active",
	"success",
	"waiting_for_agent",
}

// statusField returns a pointer to the StatusPattern slice on p that
// corresponds to the given status key, and true. If status does not match
// any of the ten valid keys, it returns (nil, false); the caller is
// responsible for building an error message that includes the offending
// value and validStatusKeys.
func statusField(p *dtypes.StatusPatterns, status string) (*[]dtypes.StatusPattern, bool) {
	switch status {
	case "ready":
		return &p.Ready, true
	case "processing":
		return &p.Processing, true
	case "needs_approval":
		return &p.NeedsApproval, true
	case "input_required":
		return &p.InputRequired, true
	case "error":
		return &p.Error, true
	case "tests_failing":
		return &p.TestsFailing, true
	case "idle":
		return &p.Idle, true
	case "active":
		return &p.Active, true
	case "success":
		return &p.Success, true
	case "waiting_for_agent":
		return &p.WaitingForAgent, true
	default:
		return nil, false
	}
}

// toStatusPatterns maps every pattern entry in pf onto the dtypes.StatusPatterns
// field its `status` key names, preserving declaration order within each
// category. Priority is deliberately left at its zero value — schema v1 has
// no `priority` key (ADR-003), and PatternSet.MatchLines (pattern_set.go)
// never reads StatusPattern.Priority for any category.
func toStatusPatterns(pf *pluginFile) (dtypes.StatusPatterns, error) {
	var sp dtypes.StatusPatterns
	for i, e := range pf.Patterns {
		field, ok := statusField(&sp, e.Status)
		if !ok {
			return dtypes.StatusPatterns{}, fmt.Errorf(
				"patterns[%d]: unknown status %q (valid values: %s)",
				i, e.Status, strings.Join(validStatusKeys, ", "),
			)
		}
		*field = append(*field, dtypes.StatusPattern{
			Name:        e.Name,
			Pattern:     e.Regex,
			Description: e.Description,
		})
	}
	return sp, nil
}

// Resource caps on plugin content — see ADR-004
// (project_plans/detector-plugins/decisions/ADR-004-plugin-trust-boundary-and-resource-caps.md).
// These are hygiene bounds against a pasted/generated pathological file, not
// ReDoS protection: Go's regexp package compiles to RE2, which is
// linear-time by construction and forecloses catastrophic backtracking
// outright. Raising any of these later is a one-line change, not a config
// key — no user should need to tune them.
const (
	// maxPatternsPerPlugin caps [[patterns]] blocks per file.
	maxPatternsPerPlugin = 50
	// maxRegexLength caps the byte length of a single pattern's regex value.
	maxRegexLength = 4096
	// maxPluginFileSize caps the bytes read from a single plugin file.
	// Enforced by stat-then-read ordering in LoadPluginDir so an oversized
	// file is rejected before it is ever loaded into memory.
	maxPluginFileSize = 256 * 1024
	// maxPluginCompileTime is the wall-clock budget for regexp.Compile calls
	// across all patterns in one file. Even within maxPatternsPerPlugin and
	// maxRegexLength, a generated file can still be expensive to compile
	// (adversarial-review.md measured 50 patterns of a 4000-byte literal
	// repeated 500 times at 6.1s); this budget bounds that cost.
	maxPluginCompileTime = 500 * time.Millisecond
	// maxPluginFiles caps the number of .toml files LoadPluginDir will
	// process in one directory scan.
	maxPluginFiles = 200
)

// PluginLoadError describes one rejected plugin file, or one rejected field
// within an otherwise-loadable file. LoadPluginDir accumulates these rather
// than treating any single bad file as fatal — one invalid file must not
// prevent other valid plugin files, or the built-in detectors, from loading.
type PluginLoadError struct {
	// Path is the plugin file this error concerns.
	Path string
	// Field is a path expression naming the offending key, e.g. "id",
	// "binary_names[1]", or "patterns[0].regex". A directory-level read
	// failure uses Field "directory"; the total-file-count cap uses the
	// distinct Field "file_count" (not "directory") specifically so callers
	// can treat "directory" as fatal ("scan failed, keep the previous
	// snapshot") without also treating a successful partial scan as fatal.
	Field string
	// Err is the underlying reason. Unwrap returns it so callers can use
	// errors.Is/As against, e.g., the concrete *regexp.Error a bad pattern
	// produced.
	Err error
}

// Error implements the error interface, naming the file, field, and reason.
func (e PluginLoadError) Error() string {
	return fmt.Sprintf("detector plugin %s: field %s: %v", e.Path, e.Field, e.Err)
}

// Unwrap exposes the underlying error for errors.Is/As.
func (e PluginLoadError) Unwrap() error { return e.Err }

// validatePluginFile checks pf for every schema and resource-cap violation
// and returns all of them (never just the first) so a user fixing several
// mistakes at once sees every problem in one pass rather than one edit-run
// cycle per mistake. path is used only to stamp PluginLoadError.Path.
func validatePluginFile(path string, pf *pluginFile) []PluginLoadError {
	var errs []PluginLoadError

	switch pf.Version {
	case "", "1":
		// Absent or "1" is schema v1 (ADR-003).
	default:
		errs = append(errs, PluginLoadError{
			Path:  path,
			Field: "version",
			Err:   fmt.Errorf("unsupported schema version %q (this build supports \"1\")", pf.Version),
		})
	}

	if strings.TrimSpace(pf.ID) == "" {
		errs = append(errs, PluginLoadError{
			Path:  path,
			Field: "id",
			Err:   errors.New("id is required and must be non-empty"),
		})
	}

	if len(pf.BinaryNames) == 0 {
		errs = append(errs, PluginLoadError{
			Path:  path,
			Field: "binary_names",
			Err:   errors.New("binary_names is required and must contain at least one name"),
		})
	} else {
		for i, name := range pf.BinaryNames {
			if strings.TrimSpace(name) == "" {
				errs = append(errs, PluginLoadError{
					Path:  path,
					Field: fmt.Sprintf("binary_names[%d]", i),
					Err:   errors.New("binary name must be non-empty"),
				})
			}
		}
	}

	if len(pf.Patterns) == 0 {
		errs = append(errs, PluginLoadError{
			Path:  path,
			Field: "patterns",
			Err:   errors.New("patterns is required and must contain at least one [[patterns]] block"),
		})
	}
	if n := len(pf.Patterns); n > maxPatternsPerPlugin {
		errs = append(errs, PluginLoadError{
			Path:  path,
			Field: "patterns",
			Err:   fmt.Errorf("%d patterns exceeds the per-file limit of %d", n, maxPatternsPerPlugin),
		})
	}

	// compileElapsed tracks cumulative regexp.Compile wall-clock time across
	// this file's patterns. Once it exceeds maxPluginCompileTime, remaining
	// patterns are still checked for name/status/length but are no longer
	// compiled — this is what keeps a boundary-case file (maxPatternsPerPlugin
	// patterns each at maxRegexLength) from hanging validation.
	var compileElapsed time.Duration
	budgetExceeded := false
	for i, p := range pf.Patterns {
		field := fmt.Sprintf("patterns[%d]", i)

		if strings.TrimSpace(p.Name) == "" {
			errs = append(errs, PluginLoadError{
				Path:  path,
				Field: field + ".name",
				Err:   errors.New("name is required and must be non-empty"),
			})
		}

		if _, ok := statusField(&dtypes.StatusPatterns{}, p.Status); !ok {
			errs = append(errs, PluginLoadError{
				Path:  path,
				Field: field + ".status",
				Err:   fmt.Errorf("unknown status %q (valid values: %s)", p.Status, strings.Join(validStatusKeys, ", ")),
			})
		}

		if strings.TrimSpace(p.Regex) == "" {
			errs = append(errs, PluginLoadError{
				Path:  path,
				Field: field + ".regex",
				Err:   errors.New("regex is required and must be non-empty"),
			})
			continue
		}

		if n := len(p.Regex); n > maxRegexLength {
			errs = append(errs, PluginLoadError{
				Path:  path,
				Field: field + ".regex",
				Err:   fmt.Errorf("regex length %d exceeds the limit of %d bytes", n, maxRegexLength),
			})
			continue
		}

		if budgetExceeded {
			continue
		}

		start := time.Now()
		_, compileErr := regexp.Compile(p.Regex)
		compileElapsed += time.Since(start)
		if compileErr != nil {
			errs = append(errs, PluginLoadError{
				Path:  path,
				Field: field + ".regex",
				Err:   compileErr,
			})
		}

		if compileElapsed > maxPluginCompileTime {
			budgetExceeded = true
			errs = append(errs, PluginLoadError{
				Path:  path,
				Field: "patterns",
				Err:   fmt.Errorf("compiling took longer than %s, rejected", maxPluginCompileTime),
			})
		}
	}

	return errs
}

// PluginDetector implements dtypes.BinaryDetector from a validated plugin
// file: one instance per (file × binary name), sharing that file's compiled
// PatternSet (built once in LoadPluginDir — see CompiledPatternSet). See
// ADR-003 on why id and binary_names are separate concepts — id is
// provenance/collision identity, binary_names are the registry keys.
type PluginDetector struct {
	id         string
	sourcePath string
	binaryName string
	patterns   dtypes.StatusPatterns
	patternSet *PatternSet
}

var _ dtypes.BinaryDetector = (*PluginDetector)(nil)

// Name returns the binary name this detector instance was built for.
func (d *PluginDetector) Name() string { return d.binaryName }

// Patterns returns the status patterns compiled from the plugin file.
func (d *PluginDetector) Patterns() dtypes.StatusPatterns { return d.patterns }

// CompiledPatternSet returns the *PatternSet LoadPluginDir already compiled
// for this file, shared unchanged across every PluginDetector built from the
// same file's binary_names. buildSnapshot (detector_snapshot.go) type-asserts
// for this to avoid re-compiling the same regex strings once per binary
// name — a plugin file declaring N binary_names previously paid N redundant
// NewPatternSet compiles on every rebuild (including every periodic
// safety-net tick) for identical patterns.
func (d *PluginDetector) CompiledPatternSet() *PatternSet { return d.patternSet }

// FilterContent is the identity function: schema v1 plugin content carries
// no binary-specific content filtering (ADR-004 — plugin content is regex
// and identifiers only).
func (d *PluginDetector) FilterContent(content string) string { return content }

// ID returns the plugin's declared id, used for collision detection and log
// messages — not a registry key (see ADR-003).
func (d *PluginDetector) ID() string { return d.id }

// SourcePath returns the absolute path of the .toml file this detector was
// loaded from, for provenance in logs and debugging.
func (d *PluginDetector) SourcePath() string { return d.sourcePath }

// PluginDir returns the directory scanned for user detector plugin files:
// config.GetConfigDir()/detectors. This goes through config.GetConfigDir()
// rather than os.UserHomeDir() directly so plugin scanning honors the same
// STAPLER_SQUAD_TEST_DIR / STAPLER_SQUAD_INSTANCE isolation as every other
// piece of application state (config/config.go) — using the real home
// directory here would leak plugin state across test runs and named
// instances instead of respecting workspace isolation.
func PluginDir() (string, error) {
	cfgDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "detectors"), nil
}

// examplePluginFile is seeded as detectors/example.toml.sample by
// EnsurePluginDir. Every line is commented out on purpose: the file
// documents the schema (including all ten valid `status` values, kept in
// sync with validStatusKeys) without itself being a loadable plugin, even if
// accidentally renamed to end in ".toml". Copying an uncommented, edited
// version to a new *.toml file in this directory is how a user activates a
// plugin.
const examplePluginFile = `# Example detector plugin — stapler-squad detector plugins (schema v1)
#
# This file is named "example.toml.sample" and is NOT loaded as a plugin:
# LoadPluginDir only scans files ending in ".toml", not ".sample". Copy this
# file to a new name ending in ".toml" in this same directory (e.g.
# "my-agent.toml") and uncomment/edit it to activate it.
#
# --- Required top-level keys ---
#
# id = "my-agent"                 # unique id for this file (collision key)
# version = "1"                   # schema version; "1" or absent
# binary_names = ["my-agent"]     # program name(s) these patterns apply to
#
# --- [[patterns]] blocks ---
#
# Each block matches one regex against terminal output for a given status.
# Within a single status category, declaration order IS match order —
# patterns declared earlier are tried first. There is no "priority" key in
# schema v1. Regexes are Go RE2 syntax (https://github.com/google/re2/wiki/Syntax):
# no backreferences, no lookahead/lookbehind.
#
# [[patterns]]
# name = "my_agent_thinking"
# regex = "Thinking\\.\\.\\."
# status = "processing"
# description = "my-agent is generating a response"
#
# [[patterns]]
# name = "my_agent_confirm"
# regex = "Do you want to proceed\\?"
# status = "needs_approval"
# description = "my-agent is asking the user to approve an action"
#
# --- Valid "status" values (all ten) ---
#
#   ready               idle and waiting for a new task
#   processing          actively generating/working
#   needs_approval      asking the user to approve an action
#   input_required      asking the user for free-form input
#   error                an error occurred
#   tests_failing        tests are failing
#   idle                 idle, no work in progress
#   active               actively running
#   success              completed successfully
#   waiting_for_agent    waiting on the agent to respond
`

// EnsurePluginDir ensures the detector plugin directory (PluginDir()) exists
// and, on first run, seeds it with a documented example file
// (example.toml.sample — see examplePluginFile). Returns the directory path.
//
// A failure to create the directory itself is returned to the caller — the
// directory is essential, there's nothing to scan or watch without it. A
// failure to write the seed file is only log.Warn'd and swallowed: the seed
// file is cosmetic documentation, and treating it as fatal would abort the
// caller's subsequent scan+watch steps even when the directory itself (and
// any real, pre-existing plugin files in it) are perfectly usable.
func EnsurePluginDir() (string, error) {
	dir, err := PluginDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create detector plugin directory %s: %w", dir, err)
	}

	examplePath := filepath.Join(dir, "example.toml.sample")
	switch _, statErr := os.Stat(examplePath); {
	case statErr == nil:
		// Already present — never overwrite, so user edits survive.
	case os.IsNotExist(statErr):
		if writeErr := os.WriteFile(examplePath, []byte(examplePluginFile), 0o644); writeErr != nil {
			log.Warn("detector plugins: failed to write example seed file", "path", examplePath, "err", writeErr)
		}
	default:
		log.Warn("detector plugins: failed to stat example seed file", "path", examplePath, "err", statErr)
	}

	return dir, nil
}

// initPluginsOnce guards InitPlugins so a second call in the same process is
// a documented no-op rather than starting a duplicate watcher. It is a
// pointer, rather than a bare sync.Once value, purely so tests can swap in a
// fresh one (see resetInitPluginsForTest in plugins_test.go) — sync.Once has
// no reset API, and package-global state must not leak between independent
// test cases sharing one test binary.
var initPluginsOnce = &sync.Once{}

// InitPlugins bootstraps the user detector plugin directory, loads whatever
// plugins it currently contains, and starts a watcher that hot-reloads the
// active detector snapshot on change. It is safe to call more than once —
// a later call is a no-op via initPluginsOnce — but the intended call site
// is exactly once, from main.go, right after logging is initialized.
//
// Nothing InitPlugins does is fatal: the STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS
// kill switch, an unwritable plugin directory, and a failed watcher start are
// all logged (where relevant) and swallowed, because a detector plugin
// problem must never prevent the daemon or web server from starting. The
// error return exists for API stability/future-proofing; every path today
// returns nil.
func InitPlugins(ctx context.Context) error {
	initPluginsOnce.Do(func() {
		if os.Getenv("STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS") != "" {
			log.Info("detector plugins disabled via STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS")
			return
		}

		dir, err := EnsurePluginDir()
		if err != nil {
			log.Warn("failed to create detector plugin directory", "err", err)
			return
		}

		// Errors are already logged inside rebuildSnapshot; a failed initial
		// scan leaves the built-ins-only snapshot from package init() live.
		_ = rebuildSnapshot(ctx, dir)

		if _, err := StartPluginWatcher(ctx, dir); err != nil {
			log.Warn("failed to start detector plugin watcher", "err", err)
		}
	})
	return nil
}

// pluginTOMLFileNames returns the .toml filenames in dir among entries (an
// os.ReadDir result), skipping subdirectories, non-.toml files, and symlinks
// (symlinks are logged — see ADR-004 on why they are never followed). A file
// that vanishes between ReadDir and this Lstat is silently skipped; there's
// nothing useful to report about a file that's already gone. Extracted from
// LoadPluginDir to keep that function's cognitive complexity under the CI
// gate — this preamble has no interaction with the per-file load/validate
// pipeline below.
func pluginTOMLFileNames(dir string, entries []os.DirEntry) []string {
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".toml") {
			continue
		}

		fullPath := filepath.Join(dir, name)
		info, lstatErr := os.Lstat(fullPath)
		if lstatErr != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			log.Warn("detector plugin skipped: symlinks are not followed", "path", fullPath)
			continue
		}

		names = append(names, name)
	}
	return names
}

// loadPluginFile stats, reads, parses, validates, and compiles fullPath into
// a pluginFile plus its StatusPatterns and compiled PatternSet. The PatternSet
// is compiled exactly once, here, and the returned pointer is shared across
// every PluginDetector the caller builds from this file's binary_names — see
// CompiledPatternSet's doc comment on why that sharing matters. On any
// failure, the returned pluginFile/StatusPatterns/PatternSet are the zero
// value and the returned errs is non-empty; callers must check len(errs)
// rather than any single return value. Extracted from LoadPluginDir to keep
// that function's cognitive complexity under the CI gate — the per-file
// pipeline below has no interaction with the directory-level dedupe logic
// that remains in LoadPluginDir.
func loadPluginFile(fullPath string) (*pluginFile, dtypes.StatusPatterns, *PatternSet, []PluginLoadError) {
	fi, statErr := os.Stat(fullPath)
	if statErr != nil {
		return nil, dtypes.StatusPatterns{}, nil, []PluginLoadError{{Path: fullPath, Field: "file", Err: statErr}}
	}
	if fi.Size() > maxPluginFileSize {
		return nil, dtypes.StatusPatterns{}, nil, []PluginLoadError{{
			Path:  fullPath,
			Field: "file",
			Err:   fmt.Errorf("file size %d exceeds the limit of %d bytes", fi.Size(), maxPluginFileSize),
		}}
	}

	data, readErr := os.ReadFile(fullPath) // #nosec G304 -- fullPath is enumerated from os.ReadDir of the plugin directory with symlinks already rejected above, not externally supplied
	if readErr != nil {
		return nil, dtypes.StatusPatterns{}, nil, []PluginLoadError{{Path: fullPath, Field: "file", Err: readErr}}
	}

	pf, parseErr := parsePluginFile(fullPath, data)
	if parseErr != nil {
		return nil, dtypes.StatusPatterns{}, nil, []PluginLoadError{{Path: fullPath, Field: "file", Err: parseErr}}
	}

	if fieldErrs := validatePluginFile(fullPath, pf); len(fieldErrs) > 0 {
		return nil, dtypes.StatusPatterns{}, nil, fieldErrs
	}

	sp, spErr := toStatusPatterns(pf)
	if spErr != nil {
		return nil, dtypes.StatusPatterns{}, nil, []PluginLoadError{{Path: fullPath, Field: "patterns", Err: spErr}}
	}

	// This should be unreachable in practice: validatePluginFile already
	// compiled every one of these same regex strings successfully (that's
	// how a file gets past the fieldErrs check above). Handled defensively
	// anyway, the same way buildSnapshot treats an analogous "can't happen"
	// compile failure — a rejection here, not a panic.
	ps, psErr := NewPatternSet(sp)
	if psErr != nil {
		return nil, dtypes.StatusPatterns{}, nil, []PluginLoadError{{Path: fullPath, Field: "patterns", Err: psErr}}
	}

	return pf, sp, ps, nil
}

// LoadPluginDir scans dir for *.toml detector plugin files, parses and
// validates each, and returns the resulting detectors plus a list of
// per-file/per-pattern rejections. One invalid file is reported and skipped;
// it never prevents other valid files from loading. A missing directory is
// not an error — it simply yields no plugins, since most users never create
// one. Non-.toml entries, subdirectories, and symlinks are skipped without
// error (symlinks are logged — see ADR-004 on why they are never followed).
//
// ctx is checked once per file in the parse/validate loop (not more finely
// than that — a single file's compile is already time-bounded by
// maxPluginCompileTime, see validatePluginFile). Without this, a cancelled
// context is only observed by the caller (rebuildSnapshot) once, at entry,
// before this function is even called — worst case that delays a graceful
// shutdown or the next legitimate reload by up to maxPluginFiles *
// maxPluginCompileTime (200 * 500ms = 100s). On cancellation, the loop stops
// immediately and returns a fatal "directory"-field PluginLoadError wrapping
// ctx.Err(), the same shape a directory-read failure produces — the caller's
// existing fatal-error handling (rebuildSnapshot) already treats that as
// "leave the previously published snapshot live" (ADR-002), which is exactly
// what a shutdown-in-progress rebuild should do.
func LoadPluginDir(ctx context.Context, dir string) ([]*PluginDetector, []PluginLoadError) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []PluginLoadError{{Path: dir, Field: "directory", Err: err}}
	}

	// os.ReadDir returns entries sorted by filename; this is what makes
	// collision winners below deterministic across repeated scans.
	names := pluginTOMLFileNames(dir, entries)

	var detectors []*PluginDetector
	var loadErrs []PluginLoadError

	if len(names) > maxPluginFiles {
		loadErrs = append(loadErrs, PluginLoadError{
			Path:  dir,
			Field: "file_count",
			Err:   fmt.Errorf("directory contains more than %d .toml files, remainder skipped", maxPluginFiles),
		})
		names = names[:maxPluginFiles]
	}

	seenIDs := make(map[string]string)      // id -> winning filename
	seenBinaries := make(map[string]string) // binary name -> winning filename

	for _, name := range names {
		if err := ctx.Err(); err != nil {
			loadErrs = append(loadErrs, PluginLoadError{Path: dir, Field: "directory", Err: err})
			return detectors, loadErrs
		}

		fullPath := filepath.Join(dir, name)

		pf, sp, ps, fileErrs := loadPluginFile(fullPath)
		if len(fileErrs) > 0 {
			loadErrs = append(loadErrs, fileErrs...)
			continue
		}

		if winner, ok := seenIDs[pf.ID]; ok {
			loadErrs = append(loadErrs, PluginLoadError{
				Path:  fullPath,
				Field: "id",
				Err:   fmt.Errorf("duplicate id %q (already declared by %s)", pf.ID, winner),
			})
			continue
		}

		collided := false
		for _, binName := range pf.BinaryNames {
			if winner, ok := seenBinaries[binName]; ok {
				loadErrs = append(loadErrs, PluginLoadError{
					Path:  fullPath,
					Field: "binary_names",
					Err:   fmt.Errorf("duplicate binary name %q (already claimed by %s)", binName, winner),
				})
				collided = true
				break
			}
		}
		if collided {
			continue
		}

		seenIDs[pf.ID] = name
		for _, binName := range pf.BinaryNames {
			seenBinaries[binName] = name
			detectors = append(detectors, &PluginDetector{
				id:         pf.ID,
				sourcePath: fullPath,
				binaryName: binName,
				patterns:   sp,
				patternSet: ps,
			})
		}
	}

	return detectors, loadErrs
}
