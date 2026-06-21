package config

import (
	"errors"
	"os"
	"testing"
)

func baseConfig() *Config {
	return &Config{
		DefaultProgram: "proxy-claude",
		SessionDefaults: SessionDefaults{
			Profiles:       make(map[string]ProfileDefaults),
			EnvVars:        make(map[string]string),
			Tags:           []string{},
			DirectoryRules: []DirectoryRule{},
		},
	}
}

func TestResolveDefaults_NoRulesNoProfile(t *testing.T) {
	cfg := baseConfig()
	cfg.SessionDefaults.Program = "claude"
	cfg.SessionDefaults.Tags = []string{"global"}

	r := ResolveDefaults(cfg, "/some/dir", "")

	if r.Program != "claude" {
		t.Errorf("expected program=claude, got %q", r.Program)
	}
	if !r.UsedGlobal {
		t.Error("expected UsedGlobal=true")
	}
	if r.UsedDirectory || r.UsedProfile {
		t.Error("unexpected directory or profile usage")
	}
	if len(r.Tags) != 1 || r.Tags[0] != "global" {
		t.Errorf("expected tags=[global], got %v", r.Tags)
	}
}

func TestResolveDefaults_LegacyDefaultProgramFallback(t *testing.T) {
	cfg := baseConfig()
	// Global program is empty; legacy DefaultProgram should apply.
	cfg.SessionDefaults.Program = ""

	r := ResolveDefaults(cfg, "", "")
	if r.Program != "proxy-claude" {
		t.Errorf("expected legacy fallback program=proxy-claude, got %q", r.Program)
	}
}

func TestResolveDefaults_DirectoryRuleExactMatch(t *testing.T) {
	cfg := baseConfig()
	cfg.SessionDefaults.Program = "claude"
	cfg.SessionDefaults.DirectoryRules = []DirectoryRule{
		{
			Path:      "/projects/foo",
			Overrides: ProfileDefaults{Program: "aider", Tags: []string{"foo"}},
		},
	}

	r := ResolveDefaults(cfg, "/projects/foo", "")

	if r.Program != "aider" {
		t.Errorf("expected directory rule to override program: got %q", r.Program)
	}
	if !r.UsedDirectory {
		t.Error("expected UsedDirectory=true")
	}
	if r.MatchedDirectory != "/projects/foo" {
		t.Errorf("expected MatchedDirectory=/projects/foo, got %q", r.MatchedDirectory)
	}
}

func TestResolveDefaults_DirectoryRulePrefixMatch(t *testing.T) {
	cfg := baseConfig()
	cfg.SessionDefaults.DirectoryRules = []DirectoryRule{
		{Path: "/projects", Overrides: ProfileDefaults{Program: "base"}},
		{Path: "/projects/foo", Overrides: ProfileDefaults{Program: "specific"}},
	}

	r := ResolveDefaults(cfg, "/projects/foo/src", "")

	if r.Program != "specific" {
		t.Errorf("expected longest-prefix rule: got %q", r.Program)
	}
}

func TestResolveDefaults_ProfileWinsOverDirectory(t *testing.T) {
	cfg := baseConfig()
	cfg.SessionDefaults.DirectoryRules = []DirectoryRule{
		{Path: "/projects", Overrides: ProfileDefaults{Program: "directory-prog"}},
	}
	cfg.SessionDefaults.Profiles = map[string]ProfileDefaults{
		"Work": {Name: "Work", Program: "work-prog"},
	}

	r := ResolveDefaults(cfg, "/projects/foo", "Work")

	if r.Program != "work-prog" {
		t.Errorf("expected profile to win over directory: got %q", r.Program)
	}
	if !r.UsedProfile {
		t.Error("expected UsedProfile=true")
	}
}

func TestResolveDefaults_TagsUnion(t *testing.T) {
	cfg := baseConfig()
	cfg.SessionDefaults.Tags = []string{"global"}
	cfg.SessionDefaults.DirectoryRules = []DirectoryRule{
		{Path: "/projects", Overrides: ProfileDefaults{Tags: []string{"dir"}}},
	}
	cfg.SessionDefaults.Profiles = map[string]ProfileDefaults{
		"Work": {Name: "Work", Tags: []string{"profile", "global"}}, // "global" is a duplicate
	}

	r := ResolveDefaults(cfg, "/projects/foo", "Work")

	tagSet := make(map[string]bool)
	for _, t := range r.Tags {
		tagSet[t] = true
	}
	for _, expected := range []string{"global", "dir", "profile"} {
		if !tagSet[expected] {
			t.Errorf("expected tag %q in union result %v", expected, r.Tags)
		}
	}
	// Should not have duplicates
	if len(r.Tags) != 3 {
		t.Errorf("expected 3 unique tags, got %d: %v", len(r.Tags), r.Tags)
	}
}

func TestResolveDefaults_EnvVarsMerge(t *testing.T) {
	cfg := baseConfig()
	cfg.SessionDefaults.EnvVars = map[string]string{"BASE": "1", "SHARED": "global"}
	cfg.SessionDefaults.Profiles = map[string]ProfileDefaults{
		"Work": {Name: "Work", EnvVars: map[string]string{"WORK": "2", "SHARED": "profile"}},
	}

	r := ResolveDefaults(cfg, "", "Work")

	if r.EnvVars["BASE"] != "1" {
		t.Errorf("expected BASE=1, got %q", r.EnvVars["BASE"])
	}
	if r.EnvVars["WORK"] != "2" {
		t.Errorf("expected WORK=2, got %q", r.EnvVars["WORK"])
	}
	if r.EnvVars["SHARED"] != "profile" {
		t.Errorf("expected SHARED=profile (profile wins), got %q", r.EnvVars["SHARED"])
	}
}

func TestResolveDefaults_NoMatchReturnsGlobal(t *testing.T) {
	cfg := baseConfig()
	cfg.SessionDefaults.Program = "claude"
	cfg.SessionDefaults.DirectoryRules = []DirectoryRule{
		{Path: "/other/path", Overrides: ProfileDefaults{Program: "aider"}},
	}

	r := ResolveDefaults(cfg, "/projects/foo", "")

	if r.Program != "claude" {
		t.Errorf("expected global defaults when no rule matches: got %q", r.Program)
	}
	if r.UsedDirectory {
		t.Error("expected UsedDirectory=false when no rule matches")
	}
}

func TestResolveDefaults_DirectoryRuleReferencesProfile(t *testing.T) {
	cfg := baseConfig()
	cfg.SessionDefaults.Profiles = map[string]ProfileDefaults{
		"Backend": {Name: "Backend", Program: "aider", Tags: []string{"backend"}},
	}
	cfg.SessionDefaults.DirectoryRules = []DirectoryRule{
		{
			Path:      "/projects/api",
			Profile:   "Backend",
			Overrides: ProfileDefaults{Tags: []string{"api"}},
		},
	}

	r := ResolveDefaults(cfg, "/projects/api", "")

	if r.Program != "aider" {
		t.Errorf("expected program from directory-referenced profile: got %q", r.Program)
	}
	tagSet := make(map[string]bool)
	for _, t := range r.Tags {
		tagSet[t] = true
	}
	if !tagSet["backend"] || !tagSet["api"] {
		t.Errorf("expected tags from profile + overrides: got %v", r.Tags)
	}
}

func TestUnionTags(t *testing.T) {
	result := unionTags([]string{"a", "b"}, []string{"b", "c"})
	if len(result) != 3 {
		t.Errorf("expected 3 tags, got %d: %v", len(result), result)
	}
}

func TestFindAlias_ReturnsAlias_WhenNameMatches(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{{Name: "myproj", Path: "~/code/myproj"}}
	a := FindAlias(cfg, "myproj")
	if a == nil {
		t.Fatal("expected non-nil alias")
	}
	if a.Name != "myproj" {
		t.Errorf("expected name 'myproj', got %q", a.Name)
	}
}

func TestFindAlias_ReturnsNil_WhenNameNotFound(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{{Name: "myproj"}}
	if FindAlias(cfg, "other") != nil {
		t.Error("expected nil for unknown alias")
	}
}

func TestFindAlias_IsCaseInsensitive_WhenUpperCaseProvided(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{{Name: "myproj"}}
	if FindAlias(cfg, "MYPROJ") == nil {
		t.Error("expected case-insensitive match")
	}
}

func TestGetAliasesByGroup_GroupsCorrectly_WhenMixedGroups(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{
		{Name: "a1", Group: "work"},
		{Name: "a2", Group: "tools"},
		{Name: "a3", Group: ""},
	}
	groups := GetAliasesByGroup(cfg)
	if len(groups["work"]) != 1 || groups["work"][0].Name != "a1" {
		t.Error("work group wrong")
	}
	if len(groups["tools"]) != 1 {
		t.Error("tools group wrong")
	}
	if len(groups[""]) != 1 || groups[""][0].Name != "a3" {
		t.Error("ungrouped wrong")
	}
}

func TestGetAliasesByGroup_ReturnsEmpty_WhenNoAliases(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{}
	groups := GetAliasesByGroup(cfg)
	if len(groups) != 0 {
		t.Errorf("expected empty map, got %v", groups)
	}
}

func TestExpandEnvVars_ExpandsSetVar_WhenVarExistsInEnvironment(t *testing.T) {
	t.Setenv("MY_TEST_VAR", "hello")
	m := map[string]string{"key": "${MY_TEST_VAR}"}
	result := ExpandEnvVars(m)
	if result["key"] != "hello" {
		t.Errorf("expected 'hello', got %q", result["key"])
	}
}

func TestExpandEnvVars_OmitsKey_WhenVarNotSetInEnvironment(t *testing.T) {
	// Ensure the var is not set
	os.Unsetenv("UNSET_TEST_VAR_12345") //nolint:tenv
	m := map[string]string{"key": "${UNSET_TEST_VAR_12345}"}
	result := ExpandEnvVars(m)
	if _, ok := result["key"]; ok {
		t.Error("expected key to be omitted when env var is not set")
	}
}

func TestExpandEnvVars_PassesThroughLiteral_WhenNoVarSyntax(t *testing.T) {
	m := map[string]string{"key": "literal_value"}
	result := ExpandEnvVars(m)
	if result["key"] != "literal_value" {
		t.Errorf("expected 'literal_value', got %q", result["key"])
	}
}

func TestResolveAlias_ReturnsError_WhenAliasNotFound(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{}
	_, err := ResolveAlias(cfg, "unknown", "", "", "")
	if err == nil {
		t.Error("expected error for unknown alias")
	}
	if !errors.Is(err, ErrAliasNotFound) {
		t.Errorf("expected ErrAliasNotFound, got %v", err)
	}
}

func TestResolveAlias_PromotesPathIntoResolvedDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{{Name: "myproj", Path: "~/code/myproj"}}
	result, err := ResolveAlias(cfg, "myproj", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Path != "~/code/myproj" {
		t.Errorf("expected Path '~/code/myproj', got %q", result.Path)
	}
}

func TestResolveAlias_IncludesStaticCLIFlags_WhenAliasDefinesThem(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{{Name: "myproj", CLIFlags: "--model haiku"}}
	result, err := ResolveAlias(cfg, "myproj", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CLIFlags != "--model haiku" {
		t.Errorf("expected '--model haiku', got %q", result.CLIFlags)
	}
}

func TestResolveAlias_AppendsExtraFlags_WhenInvocationFlagsProvided(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{{Name: "myproj", CLIFlags: "--model haiku"}}
	result, err := ResolveAlias(cfg, "myproj", "", "", "--verbose")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CLIFlags != "--model haiku --verbose" {
		t.Errorf("expected '--model haiku --verbose', got %q", result.CLIFlags)
	}
}

func TestResolveAlias_UsesOnlyExtraFlags_WhenAliasCLIFlagsEmpty(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{{Name: "quick"}}
	result, err := ResolveAlias(cfg, "quick", "", "", "--foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CLIFlags != "--foo" {
		t.Errorf("expected '--foo', got %q", result.CLIFlags)
	}
	// Must not have leading space
	if len(result.CLIFlags) > 0 && result.CLIFlags[0] == ' ' {
		t.Error("CLIFlags must not have leading space")
	}
}

func TestResolveAlias_PropagatesBranchAndLabel_WhenPassedAsArgs(t *testing.T) {
	cfg := &Config{}
	cfg.SessionDefaults.Aliases = []AliasConfig{{Name: "myproj"}}
	result, err := ResolveAlias(cfg, "myproj", "feature/auth", "working on auth", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Branch != "feature/auth" {
		t.Errorf("expected Branch 'feature/auth', got %q", result.Branch)
	}
	if result.SessionLabel != "working on auth" {
		t.Errorf("expected SessionLabel 'working on auth', got %q", result.SessionLabel)
	}
}
