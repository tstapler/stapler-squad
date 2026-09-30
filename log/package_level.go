package log

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// modulePrefix is stripped from resolved package paths so overrides can be
// written as "session/tmux" instead of the full module-qualified import path.
const modulePrefix = "github.com/tstapler/stapler-squad/"

// packageLevels holds the current per-package level overrides, keyed by
// package path (module prefix stripped, e.g. "session/tmux"). Swapped
// atomically so hot-path reads in PackageLevelHandler.Handle never block on
// a mutex. nil means "no overrides configured" (the common case).
var packageLevels atomic.Pointer[map[string]slog.Level] //nolint:gochecknoglobals

// packageLevelsMu serializes writers (SetPackageLevel/ClearPackageLevel/
// ClearAllPackageLevels) so a load->copy->mutate->store sequence from one
// goroutine can't be lost to a concurrent one. Readers stay lock-free via
// packageLevels.Load().
var packageLevelsMu sync.Mutex //nolint:gochecknoglobals

// SetPackageLevel sets a minimum log level for one package (and, by prefix,
// its subpackages unless they have a more specific override — the same
// hierarchical-logger convention as Java's log4j/logback). Pass a path
// relative to the module root, e.g. "session/tmux" or "server/services".
func SetPackageLevel(pkg string, level LogLevel) {
	pkg = strings.TrimPrefix(pkg, modulePrefix)
	packageLevelsMu.Lock()
	defer packageLevelsMu.Unlock()
	next := copyPackageLevels()
	next[pkg] = toSlogLevel(level)
	packageLevels.Store(&next)
}

// ClearPackageLevel removes a package's override, falling back to the global
// runtime level (or a less-specific ancestor override) for that package.
func ClearPackageLevel(pkg string) {
	pkg = strings.TrimPrefix(pkg, modulePrefix)
	packageLevelsMu.Lock()
	defer packageLevelsMu.Unlock()
	next := copyPackageLevels()
	delete(next, pkg)
	packageLevels.Store(&next)
}

// ClearAllPackageLevels removes every override.
func ClearAllPackageLevels() {
	packageLevelsMu.Lock()
	defer packageLevelsMu.Unlock()
	empty := make(map[string]slog.Level)
	packageLevels.Store(&empty)
}

// GetPackageLevels returns a snapshot of the current overrides, keyed by
// package path, for display (e.g. the debug API / web UI).
func GetPackageLevels() map[string]LogLevel {
	m := copyPackageLevels()
	out := make(map[string]LogLevel, len(m))
	for pkg, lvl := range m {
		out[pkg] = fromSlogLevel(lvl)
	}
	return out
}

func copyPackageLevels() map[string]slog.Level {
	cur := packageLevels.Load()
	next := make(map[string]slog.Level, len(derefOrEmpty(cur))+1)
	for k, v := range derefOrEmpty(cur) {
		next[k] = v
	}
	return next
}

func derefOrEmpty(m *map[string]slog.Level) map[string]slog.Level {
	if m == nil {
		return nil
	}
	return *m
}

func fromSlogLevel(level slog.Level) LogLevel {
	switch {
	case level <= slog.LevelDebug:
		return DEBUG
	case level <= slog.LevelInfo:
		return INFO
	case level <= slog.LevelWarn:
		return WARNING
	default:
		return ERROR
	}
}

// LoadPackageLevelsFromEnv parses STAPLER_SQUAD_LOG_LEVELS, a comma-separated
// list of "package=level" pairs (e.g. "session/tmux=debug,server/services=warn"),
// and installs them as package overrides. Call once at startup; safe to call
// again to re-parse. Malformed entries are skipped with a warning rather than
// failing startup.
func LoadPackageLevelsFromEnv() {
	raw := os.Getenv("STAPLER_SQUAD_LOG_LEVELS")
	if raw == "" {
		return
	}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		pkg, levelStr, ok := strings.Cut(pair, "=")
		if !ok {
			Warn("STAPLER_SQUAD_LOG_LEVELS: skipping malformed entry (want pkg=level)", "entry", pair)
			continue
		}
		SetPackageLevel(strings.TrimSpace(pkg), ParseLogLevel(strings.TrimSpace(levelStr)))
	}
}

// PackageLevelHandler wraps a slog.Handler and drops records that fall below
// the calling package's configured minimum level, resolved from the log
// call's program counter. Enabled fast-paths on the global level when no
// override is active; Handle does the precise per-package check otherwise.
type PackageLevelHandler struct {
	next slog.Handler
}

// NewPackageLevelHandler wraps next with per-package level filtering.
func NewPackageLevelHandler(next slog.Handler) *PackageLevelHandler {
	return &PackageLevelHandler{next: next}
}

// Enabled restores slog's normal fast path (level >= global threshold) for
// the common no-override case, so logAt's own Enabled check can still skip
// runtime.Callers/NewRecord/Add for a disabled level. When a per-package
// override is active, it reports true unconditionally — Handle is the only
// place with enough information (the record's PC) to resolve the precise
// per-package threshold.
func (h *PackageLevelHandler) Enabled(_ context.Context, level slog.Level) bool {
	overrides := packageLevels.Load()
	if overrides == nil || len(*overrides) == 0 {
		return level >= slogLevel.Level()
	}
	return true
}

func (h *PackageLevelHandler) Handle(ctx context.Context, r slog.Record) error {
	overrides := packageLevels.Load()
	if overrides == nil || len(*overrides) == 0 {
		// No overrides configured — fall back to the single global level,
		// same as before this handler existed.
		if r.Level < slogLevel.Level() {
			return nil
		}
		return h.next.Handle(ctx, r)
	}
	threshold, ok := lookupPackageLevel(*overrides, resolvePackage(r.PC))
	if !ok {
		threshold = slogLevel.Level()
	}
	if r.Level < threshold {
		return nil
	}
	return h.next.Handle(ctx, r)
}

func (h *PackageLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &PackageLevelHandler{next: h.next.WithAttrs(attrs)}
}

func (h *PackageLevelHandler) WithGroup(name string) slog.Handler {
	return &PackageLevelHandler{next: h.next.WithGroup(name)}
}

// PackageForPC resolves a program counter to its module-relative package
// path, e.g. "session/tmux" — the same resolution PackageLevelHandler uses
// internally. Exposed for tests and for debug tooling that wants to preview
// how a given call site would be classified for STAPLER_SQUAD_LOG_LEVELS.
func PackageForPC(pc uintptr) string { return resolvePackage(pc) }

// resolvePackage turns a Record's PC into a module-relative package path,
// e.g. "session/tmux". Returns "" if it can't be resolved (pc == 0, or a
// function outside this module).
func resolvePackage(pc uintptr) string {
	if pc == 0 {
		return ""
	}
	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()
	if frame.Function == "" {
		return ""
	}
	// frame.Function looks like:
	//   "github.com/tstapler/stapler-squad/session/tmux.(*TmuxSession).Close"
	// The package path is everything before the last "/"-delimited segment's
	// first "." (receiver/function names never appear before the package's
	// final path segment).
	funcName := frame.Function
	slash := strings.LastIndex(funcName, "/")
	searchFrom := 0
	if slash != -1 {
		searchFrom = slash
	}
	dot := strings.Index(funcName[searchFrom:], ".")
	if dot == -1 {
		return ""
	}
	pkgPath := funcName[:searchFrom+dot]
	return strings.TrimPrefix(pkgPath, modulePrefix)
}

// lookupPackageLevel finds the most specific configured level for pkg,
// walking up its path segments (log4j/logback-style hierarchical lookup):
// "session/tmux/inner" checks "session/tmux/inner", then "session/tmux",
// then "session".
func lookupPackageLevel(overrides map[string]slog.Level, pkg string) (slog.Level, bool) {
	for pkg != "" {
		if level, ok := overrides[pkg]; ok {
			return level, true
		}
		idx := strings.LastIndex(pkg, "/")
		if idx == -1 {
			break
		}
		pkg = pkg[:idx]
	}
	return 0, false
}
