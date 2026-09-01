package jules

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeSourceLister is a sourceLister test double that counts ListSources
// calls and returns a fixed source list.
type fakeSourceLister struct {
	sources        []JulesSource
	listSources    int
	listSourcesErr error
}

func (f *fakeSourceLister) ListSources(ctx context.Context) ([]JulesSource, error) {
	f.listSources++
	if f.listSourcesErr != nil {
		return nil, f.listSourcesErr
	}
	return f.sources, nil
}

func TestJulesSourceRegistry_Resolve_should_ServeFromCacheWithoutSecondCall_When_CalledWithinTTL(t *testing.T) {
	client := &fakeSourceLister{
		sources: []JulesSource{
			{Name: "sources/github-tstapler-stapler-squad", ID: "github-tstapler-stapler-squad"},
		},
	}
	registry := NewJulesSourceRegistry(client)

	first, err := registry.Resolve(context.Background(), "tstapler", "stapler-squad")
	if err != nil {
		t.Fatalf("first Resolve: unexpected error: %v", err)
	}

	second, err := registry.Resolve(context.Background(), "tstapler", "stapler-squad")
	if err != nil {
		t.Fatalf("second Resolve: unexpected error: %v", err)
	}

	if first != second {
		t.Errorf("expected the same JulesSourceName from both calls, got %q then %q", first, second)
	}
	if want := JulesSourceName("sources/github-tstapler-stapler-squad"); first != want {
		t.Errorf("Resolve() = %q, want %q", first, want)
	}
	if client.listSources != 1 {
		t.Errorf("client.listSources = %d, want exactly 1 ListSources call", client.listSources)
	}
}

func TestJulesSourceRegistry_Resolve_should_ReturnErrJulesSourceNotRegistered_When_RepoAbsentFromSources(t *testing.T) {
	client := &fakeSourceLister{
		sources: []JulesSource{
			{Name: "sources/github-tstapler-dotfiles", ID: "github-tstapler-dotfiles"},
		},
	}
	registry := NewJulesSourceRegistry(client)

	_, err := registry.Resolve(context.Background(), "tstapler", "stapler-squad")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, ErrJulesSourceNotRegistered) {
		t.Errorf("errors.Is(err, ErrJulesSourceNotRegistered) = false, err = %v", err)
	}
	if !strings.Contains(err.Error(), "tstapler/stapler-squad") {
		t.Errorf("error %q does not name tstapler/stapler-squad", err.Error())
	}
	if !strings.Contains(err.Error(), "jules.google.com") {
		t.Errorf("error %q does not instruct the user to connect the repo at jules.google.com", err.Error())
	}
}

func TestJulesSourceRegistry_Resolve_should_RefetchSources_When_TTLExpired(t *testing.T) {
	client := &fakeSourceLister{
		sources: []JulesSource{
			{Name: "sources/github-tstapler-stapler-squad", ID: "github-tstapler-stapler-squad"},
		},
	}
	registry := NewJulesSourceRegistry(client)
	registry.TTL = 10 * time.Minute

	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return now }

	_, err := registry.Resolve(context.Background(), "tstapler", "stapler-squad")
	if err != nil {
		t.Fatalf("first Resolve: unexpected error: %v", err)
	}
	if client.listSources != 1 {
		t.Fatalf("client.listSources = %d after first Resolve, want 1", client.listSources)
	}

	now = now.Add(11 * time.Minute)

	_, err = registry.Resolve(context.Background(), "tstapler", "stapler-squad")
	if err != nil {
		t.Fatalf("second Resolve: unexpected error: %v", err)
	}
	if client.listSources != 2 {
		t.Errorf("client.listSources = %d after TTL expiry, want 2 (a second ListSources call)", client.listSources)
	}
}

func TestParseGitHubSourceName_should_SplitOwnerAndRepo_When_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		source    JulesSourceName
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{
			name:      "simple repo name",
			source:    "sources/github-tstapler-dotfiles",
			wantOwner: "tstapler",
			wantRepo:  "dotfiles",
			wantOK:    true,
		},
		{
			name:      "hyphenated repo name",
			source:    "sources/github-tstapler-stapler-squad",
			wantOwner: "tstapler",
			wantRepo:  "stapler-squad",
			wantOK:    true,
		},
		{
			name:   "missing sources/github- prefix",
			source: "sources/gitlab-tstapler-dotfiles",
			wantOK: false,
		},
		{
			name:   "no repo segment",
			source: "sources/github-tstapler",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := parseGitHubSourceName(tt.source)
			if ok != tt.wantOK {
				t.Fatalf("parseGitHubSourceName(%q) ok = %v, want %v", tt.source, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("parseGitHubSourceName(%q) = (%q, %q), want (%q, %q)", tt.source, owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}
