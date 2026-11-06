package gitwiring

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/backend"
)

// captureWarns routes log.Warn output to the returned buffer for the test's duration.
func captureWarns(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })
	return &buf
}

func assertModes(t *testing.T, got backend.CohortMap, want map[backend.Cohort]backend.BackendMode) {
	t.Helper()
	for _, c := range backend.AllCohorts() {
		w := want[c] // missing entry means the default, cli
		if got.Mode(c) != w {
			t.Errorf("cohort %v = %v, want %v", c, got.Mode(c), w)
		}
	}
}

func TestParseCohortMap_NilAndEmptyDefaultToCLI(t *testing.T) {
	warns := captureWarns(t)
	assertModes(t, parseCohortMap(nil, ""), nil)
	assertModes(t, parseCohortMap(map[string]string{}, ""), nil)
	if warns.Len() != 0 {
		t.Errorf("unexpected WARN: %s", warns)
	}
}

func TestParseCohortMap_ValidAndInvalidValues(t *testing.T) {
	warns := captureWarns(t)
	got := parseCohortMap(map[string]string{"refs": "gogit", "network": "bogus"}, "")
	assertModes(t, got, map[backend.Cohort]backend.BackendMode{backend.CohortRefs: backend.BackendGoGit})
	if out := warns.String(); !strings.Contains(out, "network") || !strings.Contains(out, "level=WARN") {
		t.Errorf("want a WARN naming key network, got %q", out)
	}
}

func TestParseCohortMap_ShadowRejectedForLocalWrite(t *testing.T) {
	warns := captureWarns(t)
	got := parseCohortMap(map[string]string{"localwrite": "shadow", "diffstatus": "shadow"}, "")
	assertModes(t, got, map[backend.Cohort]backend.BackendMode{backend.CohortDiffStatus: backend.BackendShadow})
	if out := warns.String(); !strings.Contains(out, "localwrite") || strings.Contains(out, "diffstatus") {
		t.Errorf("want a WARN for localwrite only, got %q", out)
	}
}

func TestParseCohortMap_LocalWriteAcceptsGoGit(t *testing.T) {
	got := parseCohortMap(map[string]string{"localwrite": "gogit"}, "")
	assertModes(t, got, map[backend.Cohort]backend.BackendMode{backend.CohortLocalWrite: backend.BackendGoGit})
}

func TestParseCohortMap_NetworkShadowAccepted(t *testing.T) {
	// Cohort-level shadow is legal for network; the router restricts it to fetch/ls-remote.
	got := parseCohortMap(map[string]string{"network": "shadow"}, "")
	assertModes(t, got, map[backend.Cohort]backend.BackendMode{backend.CohortNetwork: backend.BackendShadow})
}

func TestParseCohortMap_UnknownKeysWarnAndAreIgnored(t *testing.T) {
	warns := captureWarns(t)
	got := parseCohortMap(map[string]string{"nonsense": "gogit", "refs/ResolveRef": "gogit", "worktree": "gogit"}, "")
	assertModes(t, got, map[backend.Cohort]backend.BackendMode{backend.CohortWorktree: backend.BackendGoGit})
	out := warns.String()
	for _, key := range []string{"nonsense", "refs/ResolveRef"} {
		if !strings.Contains(out, key) {
			t.Errorf("want a WARN naming %q, got %q", key, out)
		}
	}
}

func TestParseCohortMap_EnvOverrideForcesCLI(t *testing.T) {
	warns := captureWarns(t)
	got := parseCohortMap(map[string]string{"refs": "gogit", "network": "shadow"}, "cli")
	assertModes(t, got, nil)
	if warns.Len() != 0 {
		t.Errorf("unexpected WARN: %s", warns)
	}
}

func TestParseCohortMap_EnvOverrideFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		env      string
		wantWarn bool
	}{
		{"cli", false}, {"CLI", false}, {" cli ", false},
		{"off", true}, {"0", true}, {"gogit", true},
	} {
		t.Run(tc.env, func(t *testing.T) {
			warns := captureWarns(t)
			got := parseCohortMap(map[string]string{"refs": "gogit"}, tc.env)
			assertModes(t, got, nil)
			if tc.wantWarn != strings.Contains(warns.String(), EnvGitBackend) {
				t.Errorf("wantWarn=%v naming %s, got %q", tc.wantWarn, EnvGitBackend, warns)
			}
			if tc.wantWarn && !strings.Contains(warns.String(), tc.env) {
				t.Errorf("WARN should name the bad value %q, got %q", tc.env, warns)
			}
		})
	}
}

// The os.Getenv lookup is the only part not covered by the pure-function tests above.
func TestParseCohortMap_ReadsEnvVar(t *testing.T) {
	t.Setenv(EnvGitBackend, "cli")
	assertModes(t, ParseCohortMap(map[string]string{"refs": "gogit"}), nil)
}

// A malformed cohort value must not fail the whole config load (which would reset every
// other setting to defaults); it must reach ParseCohortMap, which WARNs naming the key.
func TestParseCohortMap_MalformedConfigValueKeepsOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"listen_address":"localhost:9999","git_backend_cohorts":{"refs":1,"diffstatus":"gogit"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("LoadConfigFromPath: %v", err)
	}
	if cfg.ListenAddress != "localhost:9999" {
		t.Errorf("ListenAddress = %q, other settings were dropped", cfg.ListenAddress)
	}
	warns := captureWarns(t)
	got := ParseCohortMap(cfg.GitBackendCohorts)
	assertModes(t, got, map[backend.Cohort]backend.BackendMode{backend.CohortDiffStatus: backend.BackendGoGit})
	if !strings.Contains(warns.String(), "refs") {
		t.Errorf("want a WARN naming refs, got %q", warns)
	}
}
