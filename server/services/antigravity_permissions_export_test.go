package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

func allowRule(opts ...func(*classifier.Rule)) classifier.Rule {
	r := classifier.Rule{
		ID:        "r1",
		Decision:  classifier.AutoAllow,
		Enabled:   true,
		RiskLevel: classifier.RiskLow,
	}
	for _, opt := range opts {
		opt(&r)
	}
	return r
}

func TestAntigravityCommandEntries_should_ExpandProgramsAndSubcommands_When_CriteriaSet(t *testing.T) {
	rule := allowRule(func(r *classifier.Rule) {
		r.Criteria = &classifier.CommandCriteria{
			Programs:    []string{"git"},
			Subcommands: []string{"status", "log"},
		}
	})

	entries, skip := antigravityCommandEntries(rule)

	if skip != "" {
		t.Fatalf("expected no skip reason, got %q", skip)
	}
	want := map[string]bool{"command(git status)": true, "command(git log)": true}
	if len(entries) != len(want) {
		t.Fatalf("entries = %v, want %v", entries, want)
	}
	for _, e := range entries {
		if !want[e] {
			t.Errorf("unexpected entry %q", e)
		}
	}
}

func TestAntigravityCommandEntries_should_ExportBareProgram_When_NoSubcommands(t *testing.T) {
	rule := allowRule(func(r *classifier.Rule) {
		r.Criteria = &classifier.CommandCriteria{Programs: []string{"pwd"}}
	})

	entries, skip := antigravityCommandEntries(rule)

	if skip != "" || len(entries) != 1 || entries[0] != "command(pwd)" {
		t.Fatalf("entries=%v skip=%q", entries, skip)
	}
}

func TestAntigravityCommandEntries_should_EmbedRegexSource_When_CommandPatternSet(t *testing.T) {
	rule := allowRule(func(r *classifier.Rule) {
		r.CommandPattern = regexp.MustCompile("mcp__claude_.*")
	})

	entries, skip := antigravityCommandEntries(rule)

	if skip != "" || len(entries) != 1 || entries[0] != "command(mcp__claude_.*)" {
		t.Fatalf("entries=%v skip=%q", entries, skip)
	}
}

func TestAntigravityCommandEntries_should_ExportBareToolName_When_NonBashToolNameSet(t *testing.T) {
	rule := allowRule(func(r *classifier.Rule) {
		r.ToolName = "Read"
	})

	entries, skip := antigravityCommandEntries(rule)

	if skip != "" || len(entries) != 1 || entries[0] != "command(Read)" {
		t.Fatalf("entries=%v skip=%q", entries, skip)
	}
}

func TestAntigravityCommandEntries_should_Skip_When_ToolNameIsBashWithNoScoping(t *testing.T) {
	rule := allowRule(func(r *classifier.Rule) {
		r.ToolName = "Bash"
	})

	entries, skip := antigravityCommandEntries(rule)

	if entries != nil || skip == "" {
		t.Fatalf("expected a skip reason and no entries for unscoped Bash, got entries=%v skip=%q", entries, skip)
	}
}

func TestAntigravityCommandEntries_should_Skip_When_RequireCIPassingSet(t *testing.T) {
	rule := allowRule(func(r *classifier.Rule) {
		r.ToolName = "Bash"
		r.Criteria = &classifier.CommandCriteria{Programs: []string{"git"}, Subcommands: []string{"push"}}
		r.RequireCIPassing = true
	})

	entries, skip := antigravityCommandEntries(rule)

	if entries != nil || skip == "" {
		t.Fatalf("expected RequireCIPassing to force a skip, got entries=%v skip=%q", entries, skip)
	}
}

func TestAntigravityCommandEntries_should_Skip_When_FilePatternSet(t *testing.T) {
	rule := allowRule(func(r *classifier.Rule) {
		r.ToolName = "Read"
		r.FilePattern = regexp.MustCompile(`^/home/.*`)
	})

	entries, skip := antigravityCommandEntries(rule)

	if entries != nil || skip == "" {
		t.Fatalf("expected FilePattern to force a skip, got entries=%v skip=%q", entries, skip)
	}
}

func TestAntigravityCommandEntries_should_Skip_When_ForbiddenFlagsWouldBeDropped(t *testing.T) {
	rule := allowRule(func(r *classifier.Rule) {
		r.ToolName = "Bash"
		r.Criteria = &classifier.CommandCriteria{
			Programs:       []string{"git"},
			Subcommands:    []string{"reset"},
			ForbiddenFlags: []string{"--hard"},
		}
	})

	entries, skip := antigravityCommandEntries(rule)

	if entries != nil || skip == "" {
		t.Fatalf("exporting command(git reset) would allow --hard too; expected a skip, got entries=%v skip=%q", entries, skip)
	}
}

func TestAntigravityCommandEntries_should_Skip_When_CommandPatternMatchesEverything(t *testing.T) {
	rule := allowRule(func(r *classifier.Rule) {
		r.ToolName = "Bash"
		r.CommandPattern = regexp.MustCompile(".*")
	})

	entries, skip := antigravityCommandEntries(rule)

	if entries != nil || skip == "" {
		t.Fatalf("expected the shared overbreadCommandPatterns guard to force a skip, got entries=%v skip=%q", entries, skip)
	}
}

func TestAntigravityCommandEntries_should_Skip_When_ToolCategoryOnly(t *testing.T) {
	rule := allowRule(func(r *classifier.Rule) {
		r.ToolCategory = classifier.ToolCategoryMCPRead
	})

	entries, skip := antigravityCommandEntries(rule)

	if entries != nil || skip == "" {
		t.Fatalf("expected a structural ToolCategory to force a skip, got entries=%v skip=%q", entries, skip)
	}
}

func TestExportableAntigravityPermissions_should_IgnoreDisabledAndNonAllowRules(t *testing.T) {
	rules := []classifier.Rule{
		allowRule(func(r *classifier.Rule) { r.ToolName = "Read" }),
		allowRule(func(r *classifier.Rule) { r.ToolName = "Write"; r.Enabled = false }),
		allowRule(func(r *classifier.Rule) { r.ToolName = "Edit"; r.Decision = classifier.AutoDeny }),
	}

	got := exportableAntigravityPermissions(rules)

	if len(got) != 1 || got[0] != "command(Read)" {
		t.Fatalf("got %v, want only command(Read)", got)
	}
}

func TestSyncAntigravityPermissionsAt_should_PreserveManualEntries_And_TrackOwnSeparately(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	statePath := filepath.Join(dir, ".managed.json")

	manual := map[string]any{"permissions": map[string]any{"allow": []string{"command(ls)"}}, "toolPermission": "request-review"}
	data, err := json.Marshal(manual)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	rules := []classifier.Rule{allowRule(func(r *classifier.Rule) { r.ToolName = "Read" })}
	if err := syncAntigravityPermissionsAt(settingsPath, statePath, rules); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	raw, _, allow, err := readSettingsPermissionsAllow(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, allow, "command(ls)")
	assertContains(t, allow, "command(Read)")

	var toolPermission string
	if err := json.Unmarshal(raw["toolPermission"], &toolPermission); err != nil || toolPermission != "request-review" {
		t.Fatalf("expected unrelated top-level key preserved, got %q (err=%v)", toolPermission, err)
	}
}

func TestSyncAntigravityPermissionsAt_should_RemoveOnlyOwnStaleEntry_When_RuleDeleted(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	statePath := filepath.Join(dir, ".managed.json")

	// First sync with two rules.
	first := []classifier.Rule{
		allowRule(func(r *classifier.Rule) { r.ID = "a"; r.ToolName = "Read" }),
		allowRule(func(r *classifier.Rule) { r.ID = "b"; r.ToolName = "Write" }),
	}
	if err := syncAntigravityPermissionsAt(settingsPath, statePath, first); err != nil {
		t.Fatal(err)
	}

	// A manual entry added by hand between syncs must survive the next sync.
	appendManualAllowEntry(t, settingsPath, "command(ls)")

	// Second sync: rule "b" (Write) is gone.
	second := []classifier.Rule{
		allowRule(func(r *classifier.Rule) { r.ID = "a"; r.ToolName = "Read" }),
	}
	if err := syncAntigravityPermissionsAt(settingsPath, statePath, second); err != nil {
		t.Fatal(err)
	}

	_, _, finalAllow, err := readSettingsPermissionsAllow(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, finalAllow, "command(Read)")
	assertContains(t, finalAllow, "command(ls)")
	assertNotContains(t, finalAllow, "command(Write)")
}

func appendManualAllowEntry(t *testing.T, settingsPath, entry string) {
	t.Helper()
	_, _, allow, err := readSettingsPermissionsAllow(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	allow = append(allow, entry)

	manual := map[string]any{"permissions": map[string]any{"allow": allow}}
	data, err := json.Marshal(manual)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, list []string, want string) {
	t.Helper()
	for _, v := range list {
		if v == want {
			return
		}
	}
	t.Errorf("expected %v to contain %q", list, want)
}

func assertNotContains(t *testing.T, list []string, unwanted string) {
	t.Helper()
	for _, v := range list {
		if v == unwanted {
			t.Errorf("expected %v to NOT contain %q", list, unwanted)
		}
	}
}
