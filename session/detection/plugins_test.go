package detection

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/detection/dtypes"
)

func Test_parsePluginFile(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    *pluginFile
		wantErr string // substring expected in the error, empty means no error
	}{
		{
			name: "parsePluginFile_should_decodeAllFields_When_fileIsWellFormed",
			data: `
id = "my-agent"
version = "1"
binary_names = ["my-agent"]

[[patterns]]
name = "my_agent_thinking"
regex = "Thinking\\.\\.\\."
status = "processing"
description = "my-agent is thinking"
`,
			want: &pluginFile{
				ID:          "my-agent",
				Version:     "1",
				BinaryNames: []string{"my-agent"},
				Patterns: []patternEntry{
					{
						Name:        "my_agent_thinking",
						Regex:       `Thinking\.\.\.`,
						Status:      "processing",
						Description: "my-agent is thinking",
					},
				},
			},
		},
		{
			name: "parsePluginFile_should_returnErrorNamingKey_When_binaryNamesKeyIsMisspelled",
			data: `
id = "my-agent"
version = "1"
binary_name = ["my-agent"]
`,
			wantErr: "binary_name",
		},
		{
			name: "parsePluginFile_should_returnErrorNamingKey_When_priorityKeyIsPresent",
			data: `
id = "my-agent"
version = "1"
binary_names = ["my-agent"]

[[patterns]]
name = "my_agent_thinking"
regex = "Thinking\\.\\.\\."
status = "processing"
priority = 10
`,
			wantErr: "priority",
		},
		{
			name: "parsePluginFile_should_returnError_When_tomlIsSyntacticallyInvalid",
			data: `
id =
`,
			wantErr: "failed to parse detector plugin",
		},
		{
			name:    "parsePluginFile_should_returnZeroValuePluginFile_When_fileIsEmpty",
			data:    "",
			want:    &pluginFile{},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePluginFile("/tmp/detectors/test.toml", []byte(tt.data))

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parsePluginFile() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parsePluginFile() error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("parsePluginFile() unexpected error = %v", err)
			}

			if got.ID != tt.want.ID {
				t.Errorf("ID = %q, want %q", got.ID, tt.want.ID)
			}
			if got.Version != tt.want.Version {
				t.Errorf("Version = %q, want %q", got.Version, tt.want.Version)
			}
			if len(got.BinaryNames) != len(tt.want.BinaryNames) {
				t.Fatalf("BinaryNames = %v, want %v", got.BinaryNames, tt.want.BinaryNames)
			}
			for i := range got.BinaryNames {
				if got.BinaryNames[i] != tt.want.BinaryNames[i] {
					t.Errorf("BinaryNames[%d] = %q, want %q", i, got.BinaryNames[i], tt.want.BinaryNames[i])
				}
			}
			if len(got.Patterns) != len(tt.want.Patterns) {
				t.Fatalf("Patterns = %+v, want %+v", got.Patterns, tt.want.Patterns)
			}
			for i := range got.Patterns {
				if got.Patterns[i] != tt.want.Patterns[i] {
					t.Errorf("Patterns[%d] = %+v, want %+v", i, got.Patterns[i], tt.want.Patterns[i])
				}
			}
		})
	}
}

func Test_statusField(t *testing.T) {
	cases := []struct {
		status string
		get    func(*dtypes.StatusPatterns) *[]dtypes.StatusPattern
	}{
		{"ready", func(p *dtypes.StatusPatterns) *[]dtypes.StatusPattern { return &p.Ready }},
		{"processing", func(p *dtypes.StatusPatterns) *[]dtypes.StatusPattern { return &p.Processing }},
		{"needs_approval", func(p *dtypes.StatusPatterns) *[]dtypes.StatusPattern { return &p.NeedsApproval }},
		{"input_required", func(p *dtypes.StatusPatterns) *[]dtypes.StatusPattern { return &p.InputRequired }},
		{"error", func(p *dtypes.StatusPatterns) *[]dtypes.StatusPattern { return &p.Error }},
		{"tests_failing", func(p *dtypes.StatusPatterns) *[]dtypes.StatusPattern { return &p.TestsFailing }},
		{"idle", func(p *dtypes.StatusPatterns) *[]dtypes.StatusPattern { return &p.Idle }},
		{"active", func(p *dtypes.StatusPatterns) *[]dtypes.StatusPattern { return &p.Active }},
		{"success", func(p *dtypes.StatusPatterns) *[]dtypes.StatusPattern { return &p.Success }},
		{"waiting_for_agent", func(p *dtypes.StatusPatterns) *[]dtypes.StatusPattern { return &p.WaitingForAgent }},
	}

	for _, c := range cases {
		t.Run("statusField_should_resolveTargetSlice_When_statusIs_"+c.status, func(t *testing.T) {
			var p dtypes.StatusPatterns
			got, ok := statusField(&p, c.status)
			if !ok {
				t.Fatalf("statusField(%q) ok = false, want true", c.status)
			}
			want := c.get(&p)
			if got != want {
				t.Errorf("statusField(%q) returned pointer %p, want %p", c.status, got, want)
			}
		})
	}

	t.Run("statusField_should_returnFalse_When_statusIsUnknown", func(t *testing.T) {
		var p dtypes.StatusPatterns
		got, ok := statusField(&p, "processsing")
		if ok {
			t.Fatalf("statusField() ok = true, want false")
		}
		if got != nil {
			t.Errorf("statusField() = %v, want nil", got)
		}
	})
}

func Test_toStatusPatterns(t *testing.T) {
	t.Run("toStatusPatterns_should_preserveDeclarationOrder_When_multiplePatternsShareACategory", func(t *testing.T) {
		pf := &pluginFile{
			ID:          "my-agent",
			BinaryNames: []string{"my-agent"},
			Patterns: []patternEntry{
				{Name: "first", Regex: "a", Status: "processing"},
				{Name: "second", Regex: "b", Status: "processing"},
			},
		}

		sp, err := toStatusPatterns(pf)
		if err != nil {
			t.Fatalf("toStatusPatterns() unexpected error = %v", err)
		}
		if len(sp.Processing) != 2 {
			t.Fatalf("len(Processing) = %d, want 2", len(sp.Processing))
		}
		if sp.Processing[0].Name != "first" {
			t.Errorf("Processing[0].Name = %q, want %q", sp.Processing[0].Name, "first")
		}
		if sp.Processing[1].Name != "second" {
			t.Errorf("Processing[1].Name = %q, want %q", sp.Processing[1].Name, "second")
		}
	})

	t.Run("toStatusPatterns_should_returnErrorListingAllTenValidKeys_When_statusIsUnknown", func(t *testing.T) {
		pf := &pluginFile{
			ID:          "my-agent",
			BinaryNames: []string{"my-agent"},
			Patterns: []patternEntry{
				{Name: "typo", Regex: "a", Status: "processsing"},
			},
		}

		_, err := toStatusPatterns(pf)
		if err == nil {
			t.Fatal("toStatusPatterns() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "processsing") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "processsing")
		}
		for _, key := range validStatusKeys {
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error = %q, want it to contain valid key %q", err.Error(), key)
			}
		}
	})
}

// findErrByField returns the first PluginLoadError in errs whose Field
// matches, and whether one was found.
func findErrByField(errs []PluginLoadError, field string) (PluginLoadError, bool) {
	for _, e := range errs {
		if e.Field == field {
			return e, true
		}
	}
	return PluginLoadError{}, false
}

func Test_validatePluginFile(t *testing.T) {
	t.Run("validatePluginFile_should_returnFieldId_When_idIsMissing", func(t *testing.T) {
		pf := &pluginFile{
			BinaryNames: []string{"my-agent"},
			Patterns: []patternEntry{
				{Name: "p", Status: "processing", Regex: "a"},
			},
		}
		errs := validatePluginFile("/tmp/detectors/noid.toml", pf)
		got, ok := findErrByField(errs, "id")
		if !ok {
			t.Fatalf("validatePluginFile() errs = %+v, want one with Field \"id\"", errs)
		}
		if !strings.Contains(got.Error(), "id is required and must be non-empty") {
			t.Errorf("Error() = %q, want it to contain %q", got.Error(), "id is required and must be non-empty")
		}
	})

	t.Run("validatePluginFile_should_returnFieldBinaryNames_When_binaryNamesIsEmpty", func(t *testing.T) {
		pf := &pluginFile{
			ID:          "my-agent",
			BinaryNames: []string{},
			Patterns: []patternEntry{
				{Name: "p", Status: "processing", Regex: "a"},
			},
		}
		errs := validatePluginFile("/tmp/detectors/nobinaries.toml", pf)
		got, ok := findErrByField(errs, "binary_names")
		if !ok {
			t.Fatalf("validatePluginFile() errs = %+v, want one with Field \"binary_names\"", errs)
		}
		if !strings.Contains(got.Error(), "binary_names is required and must contain at least one name") {
			t.Errorf("Error() = %q, want it to contain %q", got.Error(), "binary_names is required and must contain at least one name")
		}
	})

	t.Run("validatePluginFile_should_returnFieldPatterns0Regex_When_regexDoesNotCompile", func(t *testing.T) {
		pf := &pluginFile{
			ID:          "my-agent",
			BinaryNames: []string{"my-agent"},
			Patterns: []patternEntry{
				{Name: "p", Status: "processing", Regex: `Thinking(\.\.\.`},
			},
		}
		errs := validatePluginFile("/tmp/detectors/badregex.toml", pf)
		got, ok := findErrByField(errs, "patterns[0].regex")
		if !ok {
			t.Fatalf("validatePluginFile() errs = %+v, want one with Field \"patterns[0].regex\"", errs)
		}
		if !strings.Contains(got.Error(), "missing closing )") {
			t.Errorf("Error() = %q, want it to contain the regexp.Compile error", got.Error())
		}
	})

	t.Run("validatePluginFile_should_returnFieldVersion_When_versionIsUnsupported", func(t *testing.T) {
		pf := &pluginFile{
			ID:          "my-agent",
			Version:     "2",
			BinaryNames: []string{"my-agent"},
			Patterns: []patternEntry{
				{Name: "p", Status: "processing", Regex: "a"},
			},
		}
		errs := validatePluginFile("/tmp/detectors/badversion.toml", pf)
		got, ok := findErrByField(errs, "version")
		if !ok {
			t.Fatalf("validatePluginFile() errs = %+v, want one with Field \"version\"", errs)
		}
		want := `unsupported schema version "2" (this build supports "1")`
		if !strings.Contains(got.Error(), want) {
			t.Errorf("Error() = %q, want it to contain %q", got.Error(), want)
		}
	})

	t.Run("validatePluginFile_should_accept_When_versionKeyIsAbsent", func(t *testing.T) {
		pf := &pluginFile{
			ID:          "my-agent",
			BinaryNames: []string{"my-agent"},
			Patterns: []patternEntry{
				{Name: "p", Status: "processing", Regex: "a"},
			},
		}
		errs := validatePluginFile("/tmp/detectors/noversion.toml", pf)
		if len(errs) != 0 {
			t.Fatalf("validatePluginFile() errs = %+v, want none", errs)
		}
	})

	t.Run("validatePluginFile_should_returnThreeErrors_When_fileHasThreeSeparateProblems", func(t *testing.T) {
		pf := &pluginFile{
			// ID missing, BinaryNames missing, and the one pattern has an
			// unclosed character class — three independent problems.
			Patterns: []patternEntry{
				{Name: "p", Status: "processing", Regex: "["},
			},
		}
		errs := validatePluginFile("/tmp/detectors/threeproblems.toml", pf)
		if len(errs) != 3 {
			t.Fatalf("validatePluginFile() returned %d errors, want 3: %+v", len(errs), errs)
		}
	})

	t.Run("validatePluginFile_should_rejectOnPatternsField_When_cumulativeCompileTimeExceedsBudget", func(t *testing.T) {
		// Boundary case from adversarial-review.md: maxPatternsPerPlugin
		// patterns, each at maxRegexLength, shaped to be expensive to
		// compile — a 4000-byte literal group repeated 500 times, which
		// forces Go's regexp/syntax to expand the bounded repetition into
		// ~500 copies of the group at compile time. Must reject cheaply —
		// this test itself must not hang.
		longLiteral := "(" + strings.Repeat("x", 4000) + "){500}"
		if len(longLiteral) > maxRegexLength {
			t.Fatalf("test setup: longLiteral length %d exceeds maxRegexLength %d", len(longLiteral), maxRegexLength)
		}

		patterns := make([]patternEntry, maxPatternsPerPlugin)
		for i := range patterns {
			patterns[i] = patternEntry{
				Name:   fmt.Sprintf("p%d", i),
				Status: "processing",
				Regex:  longLiteral,
			}
		}
		pf := &pluginFile{
			ID:          "my-agent",
			BinaryNames: []string{"my-agent"},
			Patterns:    patterns,
		}

		start := time.Now()
		errs := validatePluginFile("/tmp/detectors/expensive.toml", pf)
		elapsed := time.Since(start)

		if elapsed > 5*time.Second {
			t.Fatalf("validatePluginFile() took %s, want it to reject well before hanging", elapsed)
		}

		got, ok := findErrByField(errs, "patterns")
		if !ok {
			t.Fatalf("validatePluginFile() errs = %+v, want one with Field \"patterns\"", errs)
		}
		if !strings.Contains(got.Error(), "compiling took longer than") {
			t.Errorf("Error() = %q, want it to mention the compile budget", got.Error())
		}
	})

	t.Run("validatePluginFile_should_returnFieldPatterns_When_patternCountExceedsCap", func(t *testing.T) {
		patterns := make([]patternEntry, maxPatternsPerPlugin+1)
		for i := range patterns {
			patterns[i] = patternEntry{Name: fmt.Sprintf("p%d", i), Status: "processing", Regex: "a"}
		}
		pf := &pluginFile{
			ID:          "my-agent",
			BinaryNames: []string{"my-agent"},
			Patterns:    patterns,
		}
		errs := validatePluginFile("/tmp/detectors/toomanypatterns.toml", pf)
		got, ok := findErrByField(errs, "patterns")
		if !ok {
			t.Fatalf("validatePluginFile() errs = %+v, want one with Field \"patterns\"", errs)
		}
		want := fmt.Sprintf("%d patterns exceeds the per-file limit of %d", maxPatternsPerPlugin+1, maxPatternsPerPlugin)
		if !strings.Contains(got.Error(), want) {
			t.Errorf("Error() = %q, want it to contain %q", got.Error(), want)
		}
	})

	t.Run("validatePluginFile_should_returnFieldPatterns0Regex_When_regexExceedsLengthCap", func(t *testing.T) {
		longRegex := strings.Repeat("a", maxRegexLength+1)
		pf := &pluginFile{
			ID:          "my-agent",
			BinaryNames: []string{"my-agent"},
			Patterns: []patternEntry{
				{Name: "p", Status: "processing", Regex: longRegex},
			},
		}
		errs := validatePluginFile("/tmp/detectors/toolongregex.toml", pf)
		got, ok := findErrByField(errs, "patterns[0].regex")
		if !ok {
			t.Fatalf("validatePluginFile() errs = %+v, want one with Field \"patterns[0].regex\"", errs)
		}
		want := fmt.Sprintf("regex length %d exceeds the limit of %d bytes", maxRegexLength+1, maxRegexLength)
		if !strings.Contains(got.Error(), want) {
			t.Errorf("Error() = %q, want it to contain %q", got.Error(), want)
		}
	})
}

// writePluginFile writes content to dir/filename and returns the full path.
func writePluginFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
	return path
}

// validPluginTOML returns a minimal, schema-valid plugin file body declaring
// id and binaryNames, with one "processing" pattern matching "Thinking...".
func validPluginTOML(id string, binaryNames []string) string {
	quoted := make([]string, len(binaryNames))
	for i, n := range binaryNames {
		quoted[i] = fmt.Sprintf("%q", n)
	}
	return fmt.Sprintf(`id = %q
binary_names = [%s]

[[patterns]]
name = "thinking"
regex = "Thinking\\.\\.\\."
status = "processing"
`, id, strings.Join(quoted, ", "))
}

func Test_LoadPluginDir(t *testing.T) {
	t.Run("LoadPluginDir_should_returnDetectorAndSkipInvalid_When_directoryHasValidAndInvalidFiles", func(t *testing.T) {
		dir := t.TempDir()
		writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))
		writePluginFile(t, dir, "broken.toml", `id = "broken"
binary_names = ["broken"]

[[patterns]]
name = "thinking"
regex = "Thinking(\\.\\.\\."
status = "processing"
`)

		detectors, errs := LoadPluginDir(context.Background(), dir)

		if len(detectors) != 1 {
			t.Fatalf("LoadPluginDir() detectors = %+v, want exactly 1", detectors)
		}
		if detectors[0].Name() != "my-agent" {
			t.Errorf("detectors[0].Name() = %q, want %q", detectors[0].Name(), "my-agent")
		}
		if len(errs) != 1 {
			t.Fatalf("LoadPluginDir() errs = %+v, want exactly 1", errs)
		}
		if !strings.HasSuffix(errs[0].Path, "broken.toml") {
			t.Errorf("errs[0].Path = %q, want it to end in %q", errs[0].Path, "broken.toml")
		}
	})

	t.Run("LoadPluginDir_should_returnOneDetectorPerBinaryName_When_fileDeclaresMultipleBinaryNames", func(t *testing.T) {
		dir := t.TempDir()
		path := writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent", "my-agent-beta"}))

		detectors, errs := LoadPluginDir(context.Background(), dir)

		if len(errs) != 0 {
			t.Fatalf("LoadPluginDir() errs = %+v, want none", errs)
		}
		if len(detectors) != 2 {
			t.Fatalf("LoadPluginDir() detectors = %+v, want exactly 2", detectors)
		}
		names := map[string]*PluginDetector{}
		for _, d := range detectors {
			names[d.Name()] = d
		}
		for _, want := range []string{"my-agent", "my-agent-beta"} {
			d, ok := names[want]
			if !ok {
				t.Fatalf("LoadPluginDir() detectors = %+v, want one named %q", detectors, want)
			}
			if d.SourcePath() != path {
				t.Errorf("detector %q SourcePath() = %q, want %q", want, d.SourcePath(), path)
			}
			if len(d.Patterns().Processing) != 1 || d.Patterns().Processing[0].Pattern != `Thinking\.\.\.` {
				t.Errorf("detector %q Patterns().Processing = %+v, want one pattern %q", want, d.Patterns().Processing, `Thinking\.\.\.`)
			}
		}
	})

	t.Run("LoadPluginDir_should_rejectLaterFile_When_twoFilesShareAnId", func(t *testing.T) {
		dir := t.TempDir()
		writePluginFile(t, dir, "a.toml", validPluginTOML("my-agent", []string{"a-bin"}))
		writePluginFile(t, dir, "b.toml", validPluginTOML("my-agent", []string{"b-bin"}))

		detectors, errs := LoadPluginDir(context.Background(), dir)

		if len(detectors) != 1 || detectors[0].Name() != "a-bin" {
			t.Fatalf("LoadPluginDir() detectors = %+v, want exactly 1 named \"a-bin\"", detectors)
		}
		got, ok := findErrByField(errs, "id")
		if !ok {
			t.Fatalf("LoadPluginDir() errs = %+v, want one with Field \"id\"", errs)
		}
		if !strings.HasSuffix(got.Path, "b.toml") {
			t.Errorf("errs Path = %q, want it to end in %q", got.Path, "b.toml")
		}
		want := `duplicate id "my-agent" (already declared by a.toml)`
		if !strings.Contains(got.Error(), want) {
			t.Errorf("Error() = %q, want it to contain %q", got.Error(), want)
		}
	})

	t.Run("LoadPluginDir_should_rejectLaterFile_When_twoFilesShareABinaryName", func(t *testing.T) {
		dir := t.TempDir()
		writePluginFile(t, dir, "a.toml", validPluginTOML("id-a", []string{"my-agent"}))
		writePluginFile(t, dir, "z.toml", validPluginTOML("id-z", []string{"my-agent"}))

		detectors, errs := LoadPluginDir(context.Background(), dir)

		if len(detectors) != 1 || detectors[0].Name() != "my-agent" {
			t.Fatalf("LoadPluginDir() detectors = %+v, want exactly 1 named \"my-agent\"", detectors)
		}
		if !strings.HasSuffix(detectors[0].SourcePath(), "a.toml") {
			t.Errorf("detectors[0].SourcePath() = %q, want it to end in %q", detectors[0].SourcePath(), "a.toml")
		}
		got, ok := findErrByField(errs, "binary_names")
		if !ok {
			t.Fatalf("LoadPluginDir() errs = %+v, want one with Field \"binary_names\"", errs)
		}
		if !strings.HasSuffix(got.Path, "z.toml") {
			t.Errorf("errs Path = %q, want it to end in %q", got.Path, "z.toml")
		}
		want := `duplicate binary name "my-agent" (already claimed by a.toml)`
		if !strings.Contains(got.Error(), want) {
			t.Errorf("Error() = %q, want it to contain %q", got.Error(), want)
		}
	})

	t.Run("LoadPluginDir_should_pickSameWinnerEveryTime_When_calledTenTimesOnCollidingFiles", func(t *testing.T) {
		dir := t.TempDir()
		writePluginFile(t, dir, "a.toml", validPluginTOML("id-a", []string{"my-agent"}))
		writePluginFile(t, dir, "z.toml", validPluginTOML("id-z", []string{"my-agent"}))

		for i := 0; i < 10; i++ {
			detectors, _ := LoadPluginDir(context.Background(), dir)
			if len(detectors) != 1 {
				t.Fatalf("iteration %d: LoadPluginDir() detectors = %+v, want exactly 1", i, detectors)
			}
			if !strings.HasSuffix(detectors[0].SourcePath(), "a.toml") {
				t.Fatalf("iteration %d: winner SourcePath() = %q, want it to end in %q", i, detectors[0].SourcePath(), "a.toml")
			}
		}
	})

	t.Run("LoadPluginDir_should_skipNonTomlSubdirsAndSymlinks_When_directoryContainsThem", func(t *testing.T) {
		dir := t.TempDir()
		writePluginFile(t, dir, "example.toml.sample", validPluginTOML("my-agent", []string{"my-agent"}))
		if err := os.Mkdir(filepath.Join(dir, "archive"), 0o755); err != nil {
			t.Fatalf("failed to create archive/ subdirectory: %v", err)
		}
		if runtime.GOOS != "windows" {
			target := writePluginFile(t, dir, "target.toml", validPluginTOML("target", []string{"target-agent"}))
			if err := os.Symlink(target, filepath.Join(dir, "linked.toml")); err != nil {
				t.Fatalf("failed to create symlink: %v", err)
			}
			// Remove the real target file so this only proves the symlink
			// itself was never followed/read, not merely that its target
			// happened to be a valid plugin.
			_ = os.Remove(target)
		}

		detectors, errs := LoadPluginDir(context.Background(), dir)

		if len(detectors) != 0 {
			t.Fatalf("LoadPluginDir() detectors = %+v, want none", detectors)
		}
		if len(errs) != 0 {
			t.Fatalf("LoadPluginDir() errs = %+v, want none", errs)
		}
	})

	t.Run("LoadPluginDir_should_returnNilNil_When_directoryDoesNotExist", func(t *testing.T) {
		detectors, errs := LoadPluginDir(context.Background(), filepath.Join(t.TempDir(), "does-not-exist"))
		if detectors != nil {
			t.Errorf("LoadPluginDir() detectors = %+v, want nil", detectors)
		}
		if errs != nil {
			t.Errorf("LoadPluginDir() errs = %+v, want nil", errs)
		}
	})

	t.Run("LoadPluginDir_should_capAt200Detectors_When_directoryHas201TomlFiles", func(t *testing.T) {
		dir := t.TempDir()
		const total = maxPluginFiles + 1
		for i := 0; i < total; i++ {
			id := fmt.Sprintf("agent-%03d", i)
			writePluginFile(t, dir, fmt.Sprintf("p%03d.toml", i), validPluginTOML(id, []string{id}))
		}

		detectors, errs := LoadPluginDir(context.Background(), dir)

		if len(detectors) != maxPluginFiles {
			t.Fatalf("LoadPluginDir() returned %d detectors, want exactly %d", len(detectors), maxPluginFiles)
		}
		got, ok := findErrByField(errs, "file_count")
		if !ok {
			t.Fatalf("LoadPluginDir() errs = %+v, want one with Field \"file_count\"", errs)
		}
		want := fmt.Sprintf("more than %d .toml files", maxPluginFiles)
		if !strings.Contains(got.Error(), want) {
			t.Errorf("Error() = %q, want it to contain %q", got.Error(), want)
		}
	})

	// LoadPluginDir_should_stopEarlyAndReportFatal_When_contextCancelledMidLoop
	// is the regression guard for the MAJOR finding in pre-ship review: the
	// per-file parse/validate loop never checked ctx, so a cancelled context
	// (e.g. shutdown) was only observed by rebuildSnapshot's single check at
	// entry, up to maxPluginFiles * maxPluginCompileTime later in the worst
	// case. An already-cancelled context must stop the loop before processing
	// any file and report a fatal "directory"-field error — the same shape a
	// directory-read failure produces, so rebuildSnapshot's existing fatal
	// handling (keep the previous snapshot live) applies unchanged.
	t.Run("LoadPluginDir_should_stopEarlyAndReportFatal_When_contextCancelledMidLoop", func(t *testing.T) {
		dir := t.TempDir()
		for i := 0; i < 5; i++ {
			id := fmt.Sprintf("agent-%d", i)
			writePluginFile(t, dir, fmt.Sprintf("p%d.toml", i), validPluginTOML(id, []string{id}))
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		detectors, errs := LoadPluginDir(ctx, dir)

		if len(detectors) != 0 {
			t.Errorf("LoadPluginDir(cancelled ctx) detectors = %+v, want none — cancellation must be observed before the first file is processed", detectors)
		}
		e, ok := findErrByField(errs, "directory")
		if !ok {
			t.Fatalf("LoadPluginDir(cancelled ctx) errs = %+v, want one with Field \"directory\"", errs)
		}
		if !errors.Is(e, context.Canceled) {
			t.Errorf("LoadPluginDir(cancelled ctx) directory error = %v, want it to wrap context.Canceled", e)
		}
	})
}

func Test_PluginDir(t *testing.T) {
	t.Run("PluginDir_should_returnDetectorsSubdirOfConfigDir_When_testDirIsSet", func(t *testing.T) {
		testDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

		got, err := PluginDir()
		if err != nil {
			t.Fatalf("PluginDir() unexpected error = %v", err)
		}
		want := filepath.Join(testDir, "detectors")
		if got != want {
			t.Errorf("PluginDir() = %q, want %q", got, want)
		}
	})
}

func Test_EnsurePluginDir(t *testing.T) {
	t.Run("EnsurePluginDir_should_createDetectorsSubdirWithMode0755_When_absent", func(t *testing.T) {
		testDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

		dir, err := EnsurePluginDir()
		if err != nil {
			t.Fatalf("EnsurePluginDir() unexpected error = %v", err)
		}
		want := filepath.Join(testDir, "detectors")
		if dir != want {
			t.Fatalf("EnsurePluginDir() = %q, want %q", dir, want)
		}
		info, statErr := os.Stat(dir)
		if statErr != nil {
			t.Fatalf("os.Stat(%q) unexpected error = %v", dir, statErr)
		}
		if !info.IsDir() {
			t.Fatalf("%q is not a directory", dir)
		}
		if perm := info.Mode().Perm(); perm != 0o755 {
			t.Errorf("dir mode = %o, want %o", perm, 0o755)
		}
	})

	t.Run("EnsurePluginDir_should_seedExampleFileContainingAllTenStatusKeys_When_absent", func(t *testing.T) {
		testDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

		dir, err := EnsurePluginDir()
		if err != nil {
			t.Fatalf("EnsurePluginDir() unexpected error = %v", err)
		}
		examplePath := filepath.Join(dir, "example.toml.sample")
		data, readErr := os.ReadFile(examplePath)
		if readErr != nil {
			t.Fatalf("os.ReadFile(%q) unexpected error = %v", examplePath, readErr)
		}
		content := string(data)
		for _, key := range validStatusKeys {
			if !strings.Contains(content, key) {
				t.Errorf("example.toml.sample content does not contain status key %q", key)
			}
		}
	})

	t.Run("EnsurePluginDir_should_yieldNoDetectorsOrErrors_When_exampleFileIsScannedByLoadPluginDir", func(t *testing.T) {
		testDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

		dir, err := EnsurePluginDir()
		if err != nil {
			t.Fatalf("EnsurePluginDir() unexpected error = %v", err)
		}
		detectors, errs := LoadPluginDir(context.Background(), dir)
		if len(detectors) != 0 {
			t.Errorf("LoadPluginDir(%q) detectors = %+v, want none (example.toml.sample must not be treated as a plugin)", dir, detectors)
		}
		if len(errs) != 0 {
			t.Errorf("LoadPluginDir(%q) errs = %+v, want none", dir, errs)
		}
	})

	t.Run("EnsurePluginDir_should_notClobberUserEdits_When_calledAgainOnExistingExampleFile", func(t *testing.T) {
		testDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

		dir, err := EnsurePluginDir()
		if err != nil {
			t.Fatalf("EnsurePluginDir() unexpected error = %v", err)
		}
		examplePath := filepath.Join(dir, "example.toml.sample")
		if writeErr := os.WriteFile(examplePath, []byte("# edited"), 0o644); writeErr != nil {
			t.Fatalf("failed to simulate user edit: %v", writeErr)
		}

		if _, err := EnsurePluginDir(); err != nil {
			t.Fatalf("second EnsurePluginDir() unexpected error = %v", err)
		}

		data, readErr := os.ReadFile(examplePath)
		if readErr != nil {
			t.Fatalf("os.ReadFile(%q) unexpected error = %v", examplePath, readErr)
		}
		if string(data) != "# edited" {
			t.Errorf("example.toml.sample content = %q, want %q (must not be clobbered by a second EnsurePluginDir() call)", string(data), "# edited")
		}
	})

	t.Run("EnsurePluginDir_should_stillReturnDirectory_When_seedFileWriteFails", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: directory permission bits do not restrict writes, so this failure cannot be simulated")
		}
		testDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

		dir := filepath.Join(testDir, "detectors")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("test setup: failed to pre-create detectors dir: %v", err)
		}
		// Drop a real, valid plugin file *before* locking the directory down,
		// so we can prove afterward that the directory is still genuinely
		// scannable, not just that EnsurePluginDir returns without error.
		writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))

		// example.toml.sample does not exist yet, so EnsurePluginDir's os.Stat
		// check takes the os.IsNotExist branch and attempts os.WriteFile — make
		// that write fail by stripping the directory's write bit (read+execute
		// only). Restore it in Cleanup so t.TempDir()'s own removal succeeds.
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("test setup: failed to chmod detectors dir read-only: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(dir, 0o755)
		})

		gotDir, err := EnsurePluginDir()
		if err != nil {
			t.Fatalf("EnsurePluginDir() unexpected error = %v, want nil (a seed-file write failure must be swallowed, not returned)", err)
		}
		if gotDir != dir {
			t.Fatalf("EnsurePluginDir() = %q, want %q", gotDir, dir)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "example.toml.sample")); !os.IsNotExist(statErr) {
			t.Fatalf("example.toml.sample stat err = %v, want IsNotExist (the write should have failed, not silently succeeded)", statErr)
		}

		detectors, errs := LoadPluginDir(context.Background(), gotDir)
		if len(errs) != 0 {
			t.Errorf("LoadPluginDir(%q) errs = %+v, want none", gotDir, errs)
		}
		if len(detectors) != 1 || detectors[0].Name() != "my-agent" {
			t.Errorf("LoadPluginDir(%q) detectors = %+v, want exactly one PluginDetector named my-agent", gotDir, detectors)
		}
	})
}

// resetInitPluginsForTest swaps in a fresh *sync.Once for the duration of
// the calling test, restoring the previous one in t.Cleanup.
// initPluginsOnce is package-global; sync.Once has no reset API, so without
// this every test after the first real InitPlugins() call in this test
// binary would silently no-op.
func resetInitPluginsForTest(t *testing.T) {
	t.Helper()
	old := initPluginsOnce
	initPluginsOnce = &sync.Once{}
	t.Cleanup(func() {
		initPluginsOnce = old
	})
}

func Test_InitPlugins(t *testing.T) {
	builtinNames := []string{"claude", "gemini", "aider", "opencode", "agy"}

	t.Run("InitPlugins_should_bootstrapLoadAndStartWatcher_When_noPluginDirExistsYet", func(t *testing.T) {
		resetInitPluginsForTest(t)
		resetSnapshotAfterTest(t)

		testDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		if err := InitPlugins(ctx); err != nil {
			t.Fatalf("InitPlugins() unexpected error = %v", err)
		}

		dir := filepath.Join(testDir, "detectors")
		if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
			t.Fatalf("detectors dir stat = (%v, %v), want an existing directory", info, statErr)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "example.toml.sample")); statErr != nil {
			t.Fatalf("example.toml.sample missing after InitPlugins(): %v", statErr)
		}

		prov := DetectorProvenance()
		if len(prov) != len(builtinNames) {
			t.Fatalf("DetectorProvenance() = %+v, want exactly the %d built-ins", prov, len(builtinNames))
		}

		// Prove the watcher goroutine InitPlugins started is actually
		// running — not just that bootstrap happened — by writing a plugin
		// file after InitPlugins returned and confirming it hot-reloads
		// without a restart.
		writePluginFile(t, dir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))
		require.Eventually(t, func() bool {
			_, ok := DetectorProvenance()["my-agent"]
			return ok
		}, eventuallyTimeout, eventuallyPoll, "watcher started by InitPlugins() did not pick up a new plugin file")
	})

	t.Run("InitPlugins_should_disableEverything_When_killSwitchEnvVarIsSet", func(t *testing.T) {
		resetInitPluginsForTest(t)
		resetSnapshotAfterTest(t)

		testDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
		t.Setenv("STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS", "1")

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		if err := InitPlugins(ctx); err != nil {
			t.Fatalf("InitPlugins() unexpected error = %v", err)
		}

		if _, statErr := os.Stat(filepath.Join(testDir, "detectors")); !os.IsNotExist(statErr) {
			t.Fatalf("detectors dir stat err = %v, want IsNotExist (kill switch must prevent directory creation)", statErr)
		}

		prov := DetectorProvenance()
		if len(prov) != len(builtinNames) {
			t.Fatalf("DetectorProvenance() = %+v, want exactly the %d built-ins", prov, len(builtinNames))
		}
	})

	t.Run("InitPlugins_should_logAndReturnNil_When_pluginDirCannotBeCreated", func(t *testing.T) {
		resetInitPluginsForTest(t)
		resetSnapshotAfterTest(t)

		testDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
		// Block the "detectors" subdir from ever being creatable by
		// pre-placing a regular file at that exact path — os.MkdirAll fails
		// when a path component that must be a directory already exists as
		// a plain file.
		if err := os.WriteFile(filepath.Join(testDir, "detectors"), []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("test setup: failed to write blocking file: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		if err := InitPlugins(ctx); err != nil {
			t.Fatalf("InitPlugins() unexpected error = %v", err)
		}

		prov := DetectorProvenance()
		if len(prov) != len(builtinNames) {
			t.Fatalf("DetectorProvenance() = %+v, want exactly the %d built-ins (plugin dir failure must not affect the active snapshot)", prov, len(builtinNames))
		}
	})

	t.Run("InitPlugins_should_beNoOp_When_calledTwice", func(t *testing.T) {
		resetInitPluginsForTest(t)
		resetSnapshotAfterTest(t)

		firstDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", firstDir)

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		if err := InitPlugins(ctx); err != nil {
			t.Fatalf("first InitPlugins() unexpected error = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(firstDir, "detectors")); statErr != nil {
			t.Fatalf("detectors dir missing after first InitPlugins(): %v", statErr)
		}

		// Point STAPLER_SQUAD_TEST_DIR at a brand-new, never-touched directory
		// and call InitPlugins again with the same ctx. If the sync.Once guard
		// works, the second call's body never runs at all -- so PluginDir()
		// re-resolving to this new directory is irrelevant, and critically
		// EnsurePluginDir() is never invoked against it. If the guard were
		// broken (e.g. accidentally reset, or InitPlugins didn't actually gate
		// on it), the second call would bootstrap a *second* detectors/ dir and
		// a *second* watcher goroutine here -- a duplicate this test would
		// observe directly, rather than just trusting the doc comment.
		secondDir := t.TempDir()
		t.Setenv("STAPLER_SQUAD_TEST_DIR", secondDir)

		if err := InitPlugins(ctx); err != nil {
			t.Fatalf("second InitPlugins() unexpected error = %v", err)
		}

		if _, statErr := os.Stat(filepath.Join(secondDir, "detectors")); !os.IsNotExist(statErr) {
			t.Fatalf("detectors dir under the second STAPLER_SQUAD_TEST_DIR stat err = %v, want IsNotExist -- the second InitPlugins() call must be a true no-op (no duplicate bootstrap/watcher), not just return nil", statErr)
		}

		// The original watcher (from the first call) must still be the only
		// one running, and must still be watching firstDir: a plugin file
		// dropped there is still picked up live.
		firstDetectorsDir := filepath.Join(firstDir, "detectors")
		writePluginFile(t, firstDetectorsDir, "my-agent.toml", validPluginTOML("my-agent", []string{"my-agent"}))
		require.Eventually(t, func() bool {
			_, ok := DetectorProvenance()["my-agent"]
			return ok
		}, eventuallyTimeout, eventuallyPoll, "the original watcher from the first InitPlugins() call is no longer running after a second InitPlugins() call")
	})
}
