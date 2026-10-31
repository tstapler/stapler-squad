package safeexec

import (
	"context"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// Metric names for the tmux spawn attribution counters. Both count tmux client processes built
// through CommandContext; the control-mode attach (a long-lived exec.CommandContext) is not one.
const (
	// MetricTmuxExec is labelled {subcommand, caller}: caller is the goroutine's entry function,
	// i.e. the logical owner (a poller loop, an RPC handler, a sweeper).
	MetricTmuxExec = "tmux_exec_total"
	// MetricTmuxExecSite is labelled {subcommand, site}: site is the function that built the command.
	MetricTmuxExecSite = "tmux_exec_site_total"
)

// maxSpawnLabelValues bounds each of the caller and site label sets. Function names come from
// the binary so they are finite, but a closed cap keeps series count safe against refactors.
const maxSpawnLabelValues = 64

const spawnLabelOverflow = "other"

var (
	tmuxExecTotal     = newSpawnCounter(MetricTmuxExec, metric.WithDescription("tmux client processes spawned, by tmux subcommand and goroutine-root caller"))
	tmuxExecSiteTotal = newSpawnCounter(MetricTmuxExecSite, metric.WithDescription("tmux client processes spawned, by tmux subcommand and spawning function"))

	callerLabels = newLabelSet(maxSpawnLabelValues)
	siteLabels   = newLabelSet(maxSpawnLabelValues)
)

func newSpawnCounter(name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	c, err := telemetry.GetMeter().Int64Counter(name, opts...)
	if err != nil {
		panic(err) // malformed build-time constant, as in newBackstopCounter
	}
	return c
}

// labelSet admits at most max distinct values; later new values collapse to "other".
type labelSet struct {
	mu   sync.Mutex
	max  int
	seen map[string]struct{}
}

func newLabelSet(max int) *labelSet { return &labelSet{max: max, seen: make(map[string]struct{}, max)} }

func (s *labelSet) admit(v string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seen[v]; ok {
		return v
	}
	if len(s.seen) >= s.max {
		return spawnLabelOverflow
	}
	s.seen[v] = struct{}{}
	return v
}

// isTmux matches by base name, so a TMUX_BIN wrapper not named tmux* is not counted.
func isTmux(name string) bool { return strings.HasPrefix(filepath.Base(name), "tmux") }

// recordTmuxSpawn counts one tmux client process. Like recordGitSpawn it runs when the command
// is built; every caller in this repo runs what it builds.
func recordTmuxSpawn(ctx context.Context, name string, args []string) {
	if !isTmux(name) {
		return
	}
	sub := TmuxSubcommand(args)
	site, root := spawnCallers()
	tmuxExecTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("subcommand", sub), attribute.String("caller", callerLabels.admit(root))))
	tmuxExecSiteTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("subcommand", sub), attribute.String("site", siteLabels.admit(site))))
}

var closureSuffix = regexp.MustCompile(`(\.func\d+|\.gowrap\d+|\.deferwrap\d+|-fm)(\.\d+)*$`)

const modulePrefix = "github.com/tstapler/stapler-squad/"

// shortFunc turns a runtime function name into "pkg.(*T).Method", closure suffixes removed.
func shortFunc(fn string) string {
	fn = strings.TrimPrefix(fn, modulePrefix)
	for {
		trimmed := closureSuffix.ReplaceAllString(fn, "")
		if trimmed == fn {
			break
		}
		fn = trimmed
	}
	if i := strings.Index(fn, "["); i >= 0 {
		fn = fn[:i]
	}
	return fn
}

// spawnCallers returns the first function above the spawn plumbing (site) and the outermost
// function on the goroutine's stack (root), skipping the runtime's own frames.
func spawnCallers() (site, root string) {
	pcs := make([]uintptr, 128)
	n := runtime.Callers(3, pcs) // skip Callers, spawnCallers, recordTmuxSpawn
	frames := runtime.CallersFrames(pcs[:n])
	site, root = "unknown", "unknown"
	for {
		fr, more := frames.Next()
		if fr.Function != "" && !strings.HasPrefix(fr.Function, "runtime.") {
			if site == "unknown" && !isSpawnPlumbing(fr.Function) && !isTmuxCommandBuilder(fr.Function) {
				site = shortFunc(fr.Function)
			}
			root = shortFunc(fr.Function)
		}
		if !more {
			return site, root
		}
	}
}

// tmuxFlagsWithArg are tmux's global options that consume the next argument.
var tmuxFlagsWithArg = map[string]bool{"-L": true, "-S": true, "-f": true, "-T": true}

// tmuxSubcommands is the closed label set; anything else is "other".
var tmuxSubcommands = map[string]bool{
	"attach-session": true, "bind-key": true, "break-pane": true, "capture-pane": true,
	"choose-tree": true, "clear-history": true, "detach-client": true, "display-message": true,
	"has-session": true, "kill-pane": true, "kill-server": true, "kill-session": true,
	"kill-window": true, "list-clients": true, "list-commands": true, "list-panes": true,
	"list-sessions": true, "list-windows": true, "load-buffer": true, "new-session": true,
	"new-window": true, "paste-buffer": true, "pipe-pane": true, "refresh-client": true,
	"rename-session": true, "resize-pane": true, "resize-window": true, "respawn-pane": true,
	"select-pane": true, "select-window": true, "send-keys": true, "set-environment": true,
	"set-option": true, "set-window-option": true, "set-buffer": true, "show-environment": true,
	"show-options": true, "show-window-options": true, "source-file": true, "start-server": true,
	"wait-for": true, "ls": true, "new": true, "has": true,
}

// TmuxSubcommand returns the tmux subcommand named by args, skipping global options. It returns
// "none" when there is no subcommand (`tmux -V`) and "other" for one outside the closed list.
func TmuxSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case tmuxFlagsWithArg[a]:
			i++
		case strings.HasPrefix(a, "-"):
		case tmuxSubcommands[a]:
			return a
		default:
			return "other"
		}
	}
	return "none"
}

// isTmuxCommandBuilder matches session/tmux's own command-construction helpers, which sit
// between every tmux call and safeexec and would otherwise be the site for all of them.
func isTmuxCommandBuilder(fn string) bool {
	return strings.HasPrefix(fn, modulePrefix+"session/tmux.(*TmuxSession).buildTmuxCommand")
}
