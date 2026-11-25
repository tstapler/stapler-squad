package claudesettings

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// claudeSettingsGoldenPath holds the parser, rule-conversion, classifier-effect and
// watcher-reload outputs over a scripted scenario matrix. It was recorded from the
// pre-move code in server/services (origin/main at 03f52ebcc), so a pass means the move
// changed no output byte for byte. Regenerate (only for an intended behaviour change) with
// CLAUDE_SETTINGS_GOLDEN_OUT=<path>.
const claudeSettingsGoldenPath = "testdata/claude_settings.golden.gz"

func TestClaudeSettings_ShouldMatchTheRecordedGolden_WhenTheScenariosAreReplayed(t *testing.T) {
	got := dumpClaudeSettingsScenarios(t)
	if out := os.Getenv("CLAUDE_SETTINGS_GOLDEN_OUT"); out != "" {
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		require.NoError(t, err)
		_, err = zw.Write([]byte(got))
		require.NoError(t, err)
		require.NoError(t, zw.Close())
		require.NoError(t, os.WriteFile(out, buf.Bytes(), 0o644))
		return
	}
	f, err := os.Open(claudeSettingsGoldenPath)
	require.NoError(t, err)
	defer f.Close()
	zr, err := gzip.NewReader(f)
	require.NoError(t, err)
	raw, err := io.ReadAll(zr)
	require.NoError(t, err)
	// The one intended difference: encoding/json names the target types' package in its
	// error text (claudeSettingsFile and ClaudePermissions), and they moved from services to claudesettings.
	want := []byte(strings.NewReplacer(
		"services.claudeSettingsFile", "claudesettings.claudeSettingsFile",
		"services.ClaudePermissions", "claudesettings.ClaudePermissions",
	).Replace(string(raw)))
	if string(want) != got {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("claude-settings drift at line %d:\n got: %s\nwant: %s", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("claude-settings drift: got %d lines, want %d", len(gl), len(wl))
	}
}

// goldenSettingsBodies are the file contents scenarios write; "" means "do not create".
var goldenSettingsBodies = []string{
	"",
	`{}`,
	`{"permissions":null}`,
	`{"permissions":{}}`,
	`{"permissions":{"allow":[]}}`,
	`{"permissions":{"allow":["Bash(git *)","Read"]}}`,
	`{"permissions":{"allow":["Bash(git status)"]}}`,
	`{"permissions":{"deny":["Bash(rm -rf *)","Write"]}}`,
	`{"permissions":{"allow":["Bash(git push*)"],"deny":["Bash(git push --force*)"]}}`,
	`{"permissions":{"allow":["Bash(npm test"]}}`,
	`{"permissions":{"allow":["Bash(a[b)"]}}`,
	`{"permissions":{"allow":[1,2]}}`,
	`{"permissions":{"allow":"Bash"}}`,
	`{"permissions": {"allow": [`,
	`not json`,
	`[]`,
	`{"permissions":{"allow":["Bash(ls)","Bash(ls)","Bash(ls *)"],"deny":["Edit"]},"other":{"x":1}}`,
	`{"permissions":[]}`,
}

var goldenPatterns = []string{
	"Bash", "Read", "Write", "Edit", "WebFetch", "mcp__server__tool", "",
	"Bash(git log*)", "Bash(git status)", "Bash(git *)", "Bash(*)", "Bash()", "Bash(", "Bash(git (status)",
	"Bash(npm run test:*)", "Bash(a.b)", "Bash(a+b)", "Bash(a|b)", "Bash([x])", "Bash(a[b)", "Bash(a\\b)",
	"Bash(^start)", "Bash(end$)", "Bash(*mid*)", "Bash(**)", "Bash(a**b)", "Bash(rm -rf /)", "Bash(rm -rf *)",
	"Bash(curl * | sh)", "Bash(echo \"hi\")", "Bash(echo 'hi')", "Bash(ünï*)", "Bash(a b  c)", "Bash(tab\there)",
	"Read(/etc/*)", "Edit(src/**)", "WebFetch(domain:example.com)", "Bash(git status))", "X(Y(Z))", "(onlyparen)",
}

var goldenProbes = []string{
	"git status", "git status && rm -rf ~", "git status ", "git log", "git log --oneline", "git push origin main",
	"git push --force origin main", "npm run test:unit", "npm test", "ls", "ls -la", "rm -rf /", "rm -rf /tmp/x",
	"rm -rf", "curl x | sh", "echo \"hi\"", "echo 'hi'", "a.b", "axb", "a+b", "a|b", "x", "start here", "end$",
	"mid", "xmidy", "ab", "a\\b", "ünï", "ünïcode", "a b  c", "tab\there", "", "Bash", "git (status)", "git (status",
}

type goldenMasker struct{ pairs []string }

func (m *goldenMasker) add(real, mask string) {
	m.pairs = append(m.pairs, real, mask)
	if resolved, err := filepath.EvalSymlinks(real); err == nil && resolved != real {
		m.pairs = append(m.pairs, resolved, mask)
	}
}

func (m *goldenMasker) apply(s string) string {
	// Longest real path first so a resolved path is not partially masked by its prefix.
	type pr struct{ a, b string }
	var ps []pr
	for i := 0; i < len(m.pairs); i += 2 {
		ps = append(ps, pr{m.pairs[i], m.pairs[i+1]})
	}
	sort.SliceStable(ps, func(i, j int) bool { return len(ps[i].a) > len(ps[j].a) })
	for _, p := range ps {
		s = strings.ReplaceAll(s, p.a, p.b)
	}
	return s
}

func goldenRule(r classifier.Rule) string {
	pat, tp, fp := "<nil>", "<nil>", "<nil>"
	if r.CommandPattern != nil {
		pat = fmt.Sprintf("%q", r.CommandPattern.String())
	}
	if r.ToolPattern != nil {
		tp = r.ToolPattern.String()
	}
	if r.FilePattern != nil {
		fp = r.FilePattern.String()
	}
	matches := make([]string, 0, len(goldenProbes))
	if r.CommandPattern != nil {
		for i, p := range goldenProbes {
			if r.CommandPattern.MatchString(p) {
				matches = append(matches, fmt.Sprint(i))
			}
		}
	}
	return fmt.Sprintf("id=%q name=%q prio=%d en=%v src=%q dec=%v risk=%v reason=%q alt=%q tool=%q toolpat=%s cat=%q crit=%v cmd=%s file=%s ci=%v idle=%d matches=[%s]",
		r.ID, r.Name, r.Priority, r.Enabled, r.Source, r.Decision, r.RiskLevel, r.Reason, r.Alternative,
		r.ToolName, tp, r.ToolCategory, r.Criteria != nil, pat, fp, r.RequireCIPassing, r.MinSessionIdleMinutes, strings.Join(matches, ","))
}

func goldenRules(rs []classifier.Rule) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  rules=%d\n", len(rs))
	for _, r := range rs {
		b.WriteString("    " + goldenRule(r) + "\n")
	}
	return b.String()
}

func goldenWrite(t *testing.T, path, body string) {
	t.Helper()
	if body == "" {
		return
	}
	writeSettingsFile(t, path, body)
}

func goldenClassify(rules []classifier.Rule) string {
	c := classifier.NewRuleBasedClassifier()
	c.ReplaceRules(rules)
	var b strings.Builder
	tools := []string{"Bash", "Read", "Write", "Edit", "WebFetch"}
	for _, tool := range tools {
		for _, cmd := range goldenProbes {
			in := map[string]interface{}{"command": cmd}
			res := c.Classify(classifier.PermissionRequestPayload{ToolName: tool, ToolInput: in}, classifier.ClassificationContext{})
			fmt.Fprintf(&b, "    %s %q -> dec=%v risk=%v rule=%q src=%q\n", tool, cmd, res.Decision, res.RiskLevel, res.RuleID, res.Source)
		}
	}
	return b.String()
}

func dumpClaudeSettingsScenarios(t *testing.T) string {
	var b strings.Builder

	// 1. ParseClaudeSettings over every body.
	b.WriteString("== ParseClaudeSettings\n")
	for i, body := range goldenSettingsBodies {
		dir := t.TempDir()
		m := &goldenMasker{}
		m.add(dir, "<DIR>")
		p := filepath.Join(dir, "settings.json")
		goldenWrite(t, p, body)
		perms, err := ParseClaudeSettings(p)
		fmt.Fprintf(&b, "body[%d]=%q\n", i, body)
		fmt.Fprintf(&b, "  err=%s notfound=%v\n", m.apply(fmt.Sprint(err)), err == ErrSettingsNotFound)
		if perms == nil {
			b.WriteString("  perms=<nil>\n")
		} else {
			fmt.Fprintf(&b, "  allow=%q deny=%q allownil=%v denynil=%v\n", perms.Allow, perms.Deny, perms.Allow == nil, perms.Deny == nil)
		}
	}
	// A directory where the file should be is a read error that is not "not found".
	dir := t.TempDir()
	m := &goldenMasker{}
	m.add(dir, "<DIR>")
	_, err := ParseClaudeSettings(dir)
	fmt.Fprintf(&b, "dir-as-file err=%s notfound=%v\n", m.apply(fmt.Sprint(err)), err == ErrSettingsNotFound)

	// 2. Pattern -> rule conversion at several priorities and labels.
	b.WriteString("== pattern conversion\n")
	for _, base := range []int{0, 150, 160, 170, 180} {
		for _, label := range []string{"global", "project-local"} {
			fmt.Fprintf(&b, "allow base=%d label=%s\n", base, label)
			b.WriteString(goldenRules(claudeAllowsToRules(goldenPatterns, base, label)))
			fmt.Fprintf(&b, "deny base=%d label=%s\n", base, label)
			b.WriteString(goldenRules(claudeDeniesToRules(goldenPatterns, base, label)))
		}
	}
	fmt.Fprintf(&b, "nil allow=%d nil deny=%d\n", len(claudeAllowsToRules(nil, 1, "x")), len(claudeDeniesToRules(nil, 1, "x")))
	for _, g := range []string{"", "*", "a", "a*", "*a", "a*b", "a.b", "a(b", "a b *", "**", "[x]", "ünï*", "git status"} {
		fmt.Fprintf(&b, "globToRegex(%q)=%q\n", g, globToRegex(g))
	}

	// 3. LoadClaudeSettingsRulesDetailed / LoadClaudeSettingsRules over file layouts.
	b.WriteString("== load detailed\n")
	layouts := [][4]int{
		{0, 0, 0, 0}, {5, 0, 0, 0}, {0, 5, 0, 0}, {0, 0, 5, 0}, {0, 0, 0, 5}, {5, 5, 5, 5},
		{5, 7, 8, 16}, {8, 8, 8, 8}, {13, 5, 0, 0}, {5, 13, 14, 0}, {9, 10, 11, 12}, {7, 8, 7, 8},
		{14, 15, 3, 2}, {16, 16, 16, 16}, {4, 1, 2, 0},
	}
	for li, lay := range layouts {
		for _, mode := range []string{"none", "home", "project", "projectsymlink"} {
			home, proj := t.TempDir(), t.TempDir()
			m := &goldenMasker{}
			m.add(home, "<HOME>")
			m.add(proj, "<PROJ>")
			svctest.WithFakeHomePath(t, home)
			projectDir := ""
			switch mode {
			case "home":
				projectDir = home
			case "project":
				projectDir = proj
			case "projectsymlink":
				real := filepath.Join(t.TempDir(), "real")
				require.NoError(t, os.MkdirAll(real, 0o755))
				m.add(real, "<REAL>")
				projectDir = proj
				require.NoError(t, os.MkdirAll(filepath.Join(proj, ".claude"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(real, "shared.json"), []byte(goldenSettingsBodies[5]), 0o644))
				require.NoError(t, os.Symlink(filepath.Join(real, "shared.json"), filepath.Join(proj, ".claude", "settings.json")))
				lay[2] = 0
			}
			goldenWrite(t, filepath.Join(home, ".claude", "settings.json"), goldenSettingsBodies[lay[0]])
			goldenWrite(t, filepath.Join(home, ".claude", "settings.local.json"), goldenSettingsBodies[lay[1]])
			if mode == "project" || mode == "projectsymlink" {
				goldenWrite(t, filepath.Join(proj, ".claude", "settings.json"), goldenSettingsBodies[lay[2]])
				goldenWrite(t, filepath.Join(proj, ".claude", "settings.local.json"), goldenSettingsBodies[lay[3]])
			}
			fmt.Fprintf(&b, "layout[%d]=%v mode=%s\n", li, lay, mode)
			var flat []classifier.Rule
			for _, r := range LoadClaudeSettingsRulesDetailed(projectDir) {
				fmt.Fprintf(&b, "  path=%s prio=%d label=%s err=%s notfound=%v\n", m.apply(r.Path), r.Priority, r.Label, m.apply(fmt.Sprint(r.Err)), r.Err == ErrSettingsNotFound)
				b.WriteString(m.apply(goldenRules(r.Rules)))
				flat = append(flat, r.Rules...)
			}
			load := LoadClaudeSettingsRules(projectDir)
			fmt.Fprintf(&b, "  flat=%d equalToDetailedFlatten=%v\n", len(load), goldenRulesText(load) == goldenRulesText(flat))
			if li%3 == 0 {
				b.WriteString("  classify:\n" + goldenClassify(load))
			}
		}
	}

	// 4. computeReloadOrigin over every label subset; rulesEqual over rule pairs.
	b.WriteString("== reload origin\n")
	labels := []string{"global", "global-local", "project", "project-local", "other", ""}
	for mask := 0; mask < 1<<len(labels); mask++ {
		set := map[string]bool{}
		for i, l := range labels {
			if mask&(1<<i) != 0 {
				set[l] = true
			}
		}
		fmt.Fprintf(&b, "mask=%d origin=%s\n", mask, computeReloadOrigin(set))
	}
	fmt.Fprintf(&b, "nil origin=%s\n", computeReloadOrigin(nil))
	b.WriteString("== rulesEqual\n")
	corpus := make([][]classifier.Rule, 0, 17)
	corpus = append(corpus, nil, []classifier.Rule{})
	for _, set := range [][]string{{"Bash(git *)"}, {"Bash(git *)", "Read"}, {"Bash(git status)"}, {"Read"}, {"Bash(git *)", "Write"}} {
		corpus = append(corpus, claudeAllowsToRules(set, 150, "global"))
		corpus = append(corpus, claudeAllowsToRules(set, 170, "project"))
		corpus = append(corpus, claudeDeniesToRules(set, 150, "global"))
	}
	for i := range corpus {
		for j := range corpus {
			fmt.Fprintf(&b, "%d,%d=%v ", i, j, rulesEqual(corpus[i], corpus[j]))
		}
		b.WriteString("\n")
	}

	// 5. Watcher reload sequences (no filesystem notifications involved).
	b.WriteString("== watcher reload sequences\n")
	type step struct {
		file int // 0..3 index into the four settings files
		body int // goldenSettingsBodies index; -1 deletes
	}
	scripts := [][]step{
		{{0, 5}}, {{0, 5}, {0, 13}}, {{0, 5}, {0, -1}}, {{0, 5}, {2, 8}, {0, 13}, {2, -1}},
		{{0, 5}, {0, 6}}, {{1, 7}, {3, 8}, {1, 14}, {3, 14}, {1, -1}}, {{2, 5}, {2, 5}}, {{0, 13}, {0, 5}},
		{{0, 5}, {2, 5}, {1, 9}}, {{3, 10}, {3, 11}, {3, 12}, {3, 16}},
	}
	for si, script := range scripts {
		for _, mode := range []string{"none", "home", "project"} {
			home, proj := t.TempDir(), t.TempDir()
			m := &goldenMasker{}
			m.add(home, "<HOME>")
			m.add(proj, "<PROJ>")
			svctest.WithFakeHomePath(t, home)
			files := []string{
				filepath.Join(home, ".claude", "settings.json"), filepath.Join(home, ".claude", "settings.local.json"),
				filepath.Join(proj, ".claude", "settings.json"), filepath.Join(proj, ".claude", "settings.local.json"),
			}
			projectDir := map[string]string{"none": "", "home": home, "project": proj}[mode]
			spy := &reloadSpy{}
			w := NewClaudeSettingsWatcher(projectDir, spy.callback())
			fmt.Fprintf(&b, "script[%d] mode=%s\n", si, mode)
			for k, s := range script {
				if s.body < 0 {
					_ = os.Remove(files[s.file])
				} else {
					writeSettingsFile(t, files[s.file], goldenSettingsBodies[s.body])
				}
				n, failed := w.Reload(context.Background())
				for i := range failed {
					failed[i] = m.apply(failed[i])
				}
				last := spy.last()
				fmt.Fprintf(&b, "  step[%d] file=%d body=%d -> count=%d failed=%v calls=%d cb(count=%d origin=%s notify=%v)\n",
					k, s.file, s.body, n, failed, spy.count(), last.ruleCount, last.origin, last.notify)
				keys := make([]string, 0, len(w.lastGood))
				for p := range w.lastGood {
					keys = append(keys, m.apply(p))
				}
				sort.Strings(keys)
				fmt.Fprintf(&b, "    lastGood=%v\n", keys)
			}
		}
	}
	// A nil callback must be tolerated.
	{
		home := t.TempDir()
		svctest.WithFakeHomePath(t, home)
		writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"), goldenSettingsBodies[5])
		n, failed := NewClaudeSettingsWatcher("", nil).Reload(context.Background())
		fmt.Fprintf(&b, "nil callback count=%d failed=%d\n", n, len(failed))
	}

	// 6. Start's priming reload never notifies, and the watcher stops with its context.
	b.WriteString("== start priming\n")
	for _, body := range []int{0, 5, 13} {
		home := t.TempDir()
		svctest.WithFakeHomePath(t, home)
		goldenWrite(t, filepath.Join(home, ".claude", "settings.json"), goldenSettingsBodies[body])
		spy := &reloadSpy{}
		w := NewClaudeSettingsWatcher("", spy.callback())
		ctx, cancel := context.WithCancel(context.Background())
		w.Start(ctx)
		fmt.Fprintf(&b, "body=%d calls=%d count=%d origin=%s notify=%v\n", body, spy.count(), spy.last().ruleCount, spy.last().origin, spy.last().notify)
		cancel()
		select {
		case <-w.Stopped():
			b.WriteString("  stopped=true\n")
		case <-time.After(5 * time.Second):
			b.WriteString("  stopped=false\n")
		}
	}

	// 7. handlePendingDirCreate branches that never reach the filesystem watcher.
	b.WriteString("== pending dir create\n")
	{
		w := NewClaudeSettingsWatcher("", nil)
		w.pendingDirs = map[string]map[string]bool{"/p": {".claude": true}}
		for _, ev := range []fsnotify.Event{
			{Name: "/p/.claude", Op: fsnotify.Write}, {Name: "/p/.claude", Op: fsnotify.Remove},
			{Name: "/q/.claude", Op: fsnotify.Create}, {Name: "/p/other", Op: fsnotify.Create},
			{Name: "/p/.claude", Op: fsnotify.Chmod},
		} {
			fmt.Fprintf(&b, "%s %v -> %v pending=%v\n", ev.Name, ev.Op, w.handlePendingDirCreate(ev), w.pendingDirs)
		}
	}
	fmt.Fprintf(&b, "debounce=%v\n", claudeSettingsWatchDebounce)
	fmt.Fprintf(&b, "denyOffset=%d\n", claudeSettingsDenyPriorityOffset)

	// 8. Concurrent reloads converge on the same state as serial ones.
	b.WriteString("== concurrent reload\n")
	{
		home := t.TempDir()
		svctest.WithFakeHomePath(t, home)
		writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"), goldenSettingsBodies[8])
		spy := &reloadSpy{}
		w := NewClaudeSettingsWatcher("", spy.callback())
		var wg sync.WaitGroup
		counts := make([]int, 8)
		for i := range counts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				counts[i], _ = w.Reload(context.Background())
			}()
		}
		wg.Wait()
		fmt.Fprintf(&b, "counts=%v calls=%d\n", counts, spy.count())
	}
	return b.String()
}

func goldenRulesText(rs []classifier.Rule) string { return goldenRules(rs) }
