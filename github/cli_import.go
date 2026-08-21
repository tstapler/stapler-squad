package github

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"gopkg.in/yaml.v3"
)

// CLIHost describes a GitHub host the `gh` CLI is already authenticated to.
type CLIHost struct {
	Host     string
	Username string
}

// ghConfigDir returns gh CLI's config directory, honoring GH_CONFIG_DIR the
// same way `gh` itself does, falling back to XDG_CONFIG_HOME / ~/.config.
func ghConfigDir() (string, error) {
	if dir := os.Getenv("GH_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("determine home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "gh"), nil
}

// ListCLIHosts reads gh CLI's hosts.yml to discover which GitHub hosts the
// user has already run `gh auth login` against. It only reads the host and
// associated username — safe to display in the UI — never a token: gh 2.x
// stores tokens in the OS keyring, not in this file, so retrieving the
// actual token still requires GetCLIToken. Returns an empty slice (not an
// error) when gh has never been configured on this machine.
func ListCLIHosts() ([]CLIHost, error) {
	dir, err := ghConfigDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "hosts.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read gh hosts.yml: %w", err)
	}

	var raw map[string]struct {
		User string `yaml:"user"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse gh hosts.yml: %w", err)
	}

	hosts := make([]CLIHost, 0, len(raw))
	for host, entry := range raw {
		hosts = append(hosts, CLIHost{Host: NormalizeHost(host), Username: entry.User})
	}
	return hosts, nil
}

// GetCLIToken shells out to `gh auth token --hostname <host>` to retrieve
// the token gh has stored (keyring or hosts.yml) for host. Requires the gh
// CLI to be installed and already authenticated to that host.
func GetCLIToken(ctx context.Context, host string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "gh", "auth", "token", "--hostname", NormalizeHost(host))
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh auth token --hostname %s: %w", host, err)
	}
	return strings.TrimSpace(string(out)), nil
}
