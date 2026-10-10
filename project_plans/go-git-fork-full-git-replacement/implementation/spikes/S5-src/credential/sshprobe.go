package credential

import (
	"os"
	"sort"
	"strings"

	"github.com/kevinburke/ssh_config"
)

// Directives OpenSSH honours that go-git v5.19.2's ssh transport does not
// (it reads only Hostname and Port from ssh_config; see ssh/common.go
// doGetHostWithPortFromSSHConfig).
var unsupportedKeys = []string{
	"ProxyCommand", "ProxyJump", "IdentityFile", "IdentityAgent", "CertificateFile",
	"UserKnownHostsFile", "GlobalKnownHostsFile", "StrictHostKeyChecking", "HostKeyAlias",
	"User", "CanonicalizeHostname", "ControlPath", "KnownHostsCommand", "AddKeysToAgent",
}

// ProbeSSHConfig lists unsupported directives that apply to host in the config
// file at path, including via Include. A Match block makes the whole file
// unparseable for go-git; that is reported as the single entry "Match".
func ProbeSSHConfig(path, host string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, err := ssh_config.Decode(f)
	if err != nil {
		if strings.Contains(err.Error(), "Match directive") {
			// kevinburke/ssh_config hard-fails on Match, and go-git's Get() then returns "" for
			// EVERY key, so Hostname/Port aliases are silently dropped too.
			return []string{"Match"}, nil
		}
		return nil, err
	}
	var out []string
	for _, k := range unsupportedKeys {
		if v, _ := cfg.Get(host, k); v != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}
