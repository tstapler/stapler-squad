package services

import (
	"context"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// configFileRulesPollInterval is how often the server re-stats shared_rules.yaml. A poll (not
// fsnotify) because editors often replace the file via rename, which loses a file-level watch.
const configFileRulesPollInterval = 3 * time.Second

// configFileRulesReloader hot-reloads shared_rules.yaml into the live classifier so the
// resident classifier uses the same rules ssq-hooks' local path loads at startup. Any load
// problem after a successful load keeps the last-good rules, so an edit can never silently drop
// a deny; a file missing for two consecutive polls clears them (delete-then-write saves and
// relinks are transient).
type configFileRulesReloader struct {
	path         string
	apply        func([]classifier.Rule)
	primed       bool
	loaded       bool // rules from the file have been applied at least once
	pendingClear bool // file seen missing once
	lastSig      fileSignature
}

// fileSignature is the cheap change detector: existence, mtime and size.
type fileSignature struct {
	exists  bool
	modTime time.Time
	size    int64
}

func statSignature(path string) fileSignature {
	fi, err := os.Stat(path)
	if err != nil {
		return fileSignature{}
	}
	return fileSignature{exists: true, modTime: fi.ModTime(), size: fi.Size()}
}

// reloadIfChanged reloads and applies the rules when the file changed since the last call (or on
// the first call). It reports whether rules were applied.
func (r *configFileRulesReloader) reloadIfChanged() bool {
	sig := statSignature(r.path)
	if r.primed && sig == r.lastSig && !r.pendingClear {
		return false
	}
	if r.primed && r.loaded && !sig.exists && !r.pendingClear {
		r.pendingClear = true
		return false
	}
	r.pendingClear = false
	r.primed = true
	r.lastSig = sig

	rules, err := classifier.LoadConfigFileRules(r.path)
	if err != nil {
		if rules == nil || r.loaded {
			log.Warn("[ConfigFileRules] keeping last-good rules, reload failed", "path", r.path, "err", err)
			return false
		}
		log.Warn("[ConfigFileRules] applying valid rules, some were skipped", "path", r.path, "err", err)
	}
	r.loaded = len(rules) > 0 || sig.exists
	r.apply(rules)
	log.Info("[ConfigFileRules] loaded shared rules", "path", r.path, "rule_count", len(rules))
	return true
}

// run polls until ctx is cancelled.
func (r *configFileRulesReloader) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reloadIfChanged()
		}
	}
}

// StartConfigFileRulesReload loads shared_rules.yaml synchronously, then keeps it hot-reloaded
// in the background until ctx is cancelled.
func (rs *RulesService) StartConfigFileRulesReload(ctx context.Context, path string, interval time.Duration) {
	r := &configFileRulesReloader{path: path, apply: rs.rebuildConfigFileRules}
	r.reloadIfChanged()
	go r.run(ctx, interval)
}

// rebuildConfigFileRules hot-swaps the config-file-sourced rules in the classifier, keeping
// seed, user and claude-settings rules unchanged. See rebuildClassifier for the rebuildMu and
// reconciliation rationale.
func (rs *RulesService) rebuildConfigFileRules(newRules []classifier.Rule) {
	func() {
		rs.rebuildMu.Lock()
		defer rs.rebuildMu.Unlock()

		existing := rs.classifier.Rules()
		rs.afterRebuildReadHook() // test-only: see field doc comment
		kept := filterRulesBySource(existing, classifier.SourceSeed, classifier.SourceUser, classifier.SourceClaudeSettings)
		rs.classifier.ReplaceRules(append(kept, newRules...))
	}()
	go rs.reconcilePendingApprovalsSafe()
}
