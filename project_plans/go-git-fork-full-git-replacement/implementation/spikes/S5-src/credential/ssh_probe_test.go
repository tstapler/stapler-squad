package credential

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/kevinburke/ssh_config"
	gossh "golang.org/x/crypto/ssh"
)

// cfgReader adapts a parsed file to go-git's sshConfig interface (Get(alias,key) string).
// go-git's default is ssh_config.DefaultUserSettings (reads ~/.ssh/config once, cached).
type cfgReader struct{ c *ssh_config.Config }

func (r cfgReader) Get(alias, key string) string { v, _ := r.c.Get(alias, key); return v }

func useSSHConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config")
	writeFile(t, p, body)
	f, _ := os.Open(p)
	defer f.Close()
	c, err := ssh_config.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	old := gitssh.DefaultSSHConfig
	gitssh.DefaultSSHConfig = cfgReader{c}
	t.Cleanup(func() { gitssh.DefaultSSHConfig = old })
	return p
}

func fetchOnce(t *testing.T, url string) error {
	repo, _ := git.PlainInit(t.TempDir(), false)
	repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{url}})
	err := repo.FetchContext(context.Background(), &git.FetchOptions{RemoteName: "origin"})
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}
	return err
}

func TestSSH_should_UseAgentAndKnownHosts_Matrix(t *testing.T) {
	root, _ := seedBare(t)
	key := startAgent(t)
	ss := newSSHServer(t, root, key)
	url := "ssh://git@" + ss.Addr + "/repo.git"
	good := knownHostsLine(ss.Addr, ss.HostKey.PublicKey())
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	otherSigner, _ := gossh.NewSignerFromKey(otherPriv)
	bad := knownHostsLine(ss.Addr, otherSigner.PublicKey())

	writeKnownHosts(t)
	err := fetchOnce(t, url)
	t.Logf("no known_hosts entry -> %v", err)
	if err == nil {
		t.Fatal("unknown host accepted")
	}
	writeKnownHosts(t, bad)
	err = fetchOnce(t, url)
	t.Logf("mismatched key -> %v", err)
	if err == nil {
		t.Fatal("mismatched host key accepted")
	}
	writeKnownHosts(t, good)
	if err := fetchOnce(t, url); err != nil {
		t.Fatalf("plain known_hosts entry rejected: %v", err)
	}
	// hashed entry (HashKnownHosts yes)
	kh := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
	if out, err := exec.Command("ssh-keygen", "-H", "-f", kh).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen -H: %v %s", err, out)
	}
	if b := mustRead(t, kh); strings.Contains(b, "127.0.0.1") {
		t.Fatalf("known_hosts not hashed: %s", b)
	}
	if err := fetchOnce(t, url); err != nil {
		t.Fatalf("hashed known_hosts entry rejected: %v", err)
	}
	// endpoint without user -> go-git defaults to "git"
	_ = fetchOnce(t, "ssh://"+ss.Addr+"/repo.git")
	cur, _ := user.Current()
	if len(ss.Users) == 0 || ss.Users[len(ss.Users)-1] != cur.Username {
		t.Fatalf("users seen: %v (want OS user %q for a user-less ssh:// URL)", ss.Users, cur.Username)
	}
	t.Logf("user-less ssh:// URL authenticates as the OS user %q, not \"git\"", cur.Username)
	// no agent at all
	t.Setenv("SSH_AUTH_SOCK", "")
	err = fetchOnce(t, url)
	t.Logf("no SSH_AUTH_SOCK -> %v", err)
	if err == nil {
		t.Fatal("expected failure without agent")
	}
}

func TestSSH_should_HonourHostnamePortButIgnoreUserStrictHostKeyChecking(t *testing.T) {
	root, _ := seedBare(t)
	key := startAgent(t)
	ss := newSSHServer(t, root, key)
	host, port, _ := net.SplitHostPort(ss.Addr)
	useSSHConfig(t, fmt.Sprintf("Host s5alias\n  Hostname %s\n  Port %s\n  User someoneelse\n  StrictHostKeyChecking no\n", host, port))
	writeKnownHosts(t) // empty
	url := "ssh://git@s5alias/repo.git"
	err := fetchOnce(t, url)
	t.Logf("alias + StrictHostKeyChecking=no, empty known_hosts -> %v", err)
	if err == nil || strings.Contains(err.Error(), "refused") || strings.Contains(err.Error(), "no such host") {
		t.Fatalf("alias not resolved or strict=no honoured: %v", err)
	}
	// resolved address in known_hosts -> success (go-git checks the RESOLVED host:port, not the alias)
	writeKnownHosts(t, knownHostsLine(ss.Addr, ss.HostKey.PublicKey()))
	if err := fetchOnce(t, url); err != nil {
		t.Fatalf("alias fetch with resolved known_hosts entry failed: %v", err)
	}
	// alias-only known_hosts entry (what OpenSSH would check) -> rejected
	writeKnownHosts(t, "s5alias "+strings.TrimSpace(string(gossh.MarshalAuthorizedKey(ss.HostKey.PublicKey()))))
	err = fetchOnce(t, url)
	t.Logf("alias-only known_hosts entry -> %v", err)
	if err == nil {
		t.Fatal("alias-named known_hosts entry accepted; expected resolved-name lookup")
	}
	if last := ss.Users[len(ss.Users)-1]; last != "git" {
		t.Fatalf("ssh_config User leaked into auth: %q", last)
	}
}

func TestSSH_should_IgnoreIdentityFile_AndProxyCommand(t *testing.T) {
	root, _ := seedBare(t)
	agentKey := startAgent(t)
	_ = agentKey
	// Server only accepts a key that is NOT in the agent but IS named by IdentityFile.
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := gossh.NewSignerFromKey(priv)
	blk, _ := gossh.MarshalPrivateKey(priv, "")
	idFile := filepath.Join(t.TempDir(), "id_s5")
	if err := os.WriteFile(idFile, pem.EncodeToMemory(blk), 0o600); err != nil {
		t.Fatal(err)
	}
	ss := newSSHServer(t, root, signer.PublicKey())
	host, port, _ := net.SplitHostPort(ss.Addr)
	writeKnownHosts(t, knownHostsLine(ss.Addr, ss.HostKey.PublicKey()))
	useSSHConfig(t, fmt.Sprintf("Host keyed\n  Hostname %s\n  Port %s\n  IdentityFile %s\n  IdentitiesOnly yes\n", host, port, idFile))
	err := fetchOnce(t, "ssh://git@keyed/repo.git")
	t.Logf("IdentityFile-only key, agent lacks it -> %v", err)
	if err == nil {
		t.Fatal("go-git honoured IdentityFile?")
	}
	// Design: resolver reads IdentityFile itself and passes a PublicKeys AuthMethod -> works.
	auth, err := gitssh.NewPublicKeysFromFile("git", idFile, "")
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := git.PlainInit(t.TempDir(), false)
	repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{"ssh://git@keyed/repo.git"}})
	if err := repo.Fetch(&git.FetchOptions{RemoteName: "origin", Auth: auth}); err != nil && err != git.NoErrAlreadyUpToDate {
		t.Fatalf("explicit PublicKeys auth failed: %v", err)
	}

	// ProxyCommand: Hostname/Port point at a closed port; ProxyCommand would route to the real server.
	useSSHConfig(t, fmt.Sprintf("Host proxied\n  Hostname 127.0.0.1\n  Port 1\n  ProxyCommand nc %s %s\n", host, port))
	err = fetchOnce(t, "ssh://git@proxied/repo.git")
	t.Logf("ProxyCommand host -> %v", err)
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("expected direct dial to 127.0.0.1:1 (ProxyCommand ignored), got %v", err)
	}
}

func TestSSHProbe_should_ListUnsupportedDirectives_When_ProxyCommandIdentityFileInclude(t *testing.T) {
	dir := t.TempDir()
	inc := filepath.Join(dir, "extra.conf")
	writeFile(t, inc, "Host included\n  IdentityFile ~/.ssh/id_included\n")
	cfg := filepath.Join(dir, "config")
	writeFile(t, cfg, fmt.Sprintf(`Include %s

Host proxied
  Hostname p.invalid
  ProxyCommand nc -X 5 -x proxy.invalid:1080 %%h %%p

Host jumped
  Hostname j.invalid
  ProxyJump bastion.invalid

Host keyed
  Hostname k.invalid
  IdentityFile ~/.ssh/id_k
  User deploy

Host plain
  Hostname plain.invalid
  Port 2222

`, inc))
	want := map[string][]string{
		"proxied":  {"ProxyCommand"},
		"jumped":   {"ProxyJump"},
		"keyed":    {"IdentityFile", "User"},
		"included": {"IdentityFile"}, // via Include
		"plain":    nil,
	}
	for host, exp := range want {
		got, err := ProbeSSHConfig(cfg, host)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, exp) && !(len(got) == 0 && len(exp) == 0) {
			t.Errorf("%s: got %v want %v", host, got, exp)
		}
		t.Logf("probe %-9s -> %v", host, got)
	}
	// Oracle: OpenSSH agrees these directives apply.
	for host, key := range map[string]string{"proxied": "proxycommand", "jumped": "proxyjump", "keyed": "identityfile", "included": "identityfile"} {
		out, err := exec.Command("ssh", "-G", "-F", cfg, host).CombinedOutput()
		if err != nil {
			t.Fatalf("ssh -G %s: %v %s", host, err, out)
		}
		if !strings.Contains("\n"+string(out), "\n"+key+" ") {
			t.Errorf("ssh -G %s lacks %s", host, key)
		}
	}
	// Match block: whole file becomes unparseable for go-git.
	mcfg := filepath.Join(dir, "config-match")
	writeFile(t, mcfg, "Host alias\n  Hostname real.invalid\n  Port 2200\n\nMatch host matched.invalid\n  ProxyCommand nc m.invalid 22\n")
	got, err := ProbeSSHConfig(mcfg, "alias")
	if err != nil || !reflect.DeepEqual(got, []string{"Match"}) {
		t.Fatalf("match probe: %v %v", got, err)
	}
	fakeHome := t.TempDir()
	writeFile(t, filepath.Join(fakeHome, ".ssh", "config"), mustRead(t, mcfg))
	t.Setenv("HOME", fakeHome)
	us := &ssh_config.UserSettings{} // same type and code path as go-git's DefaultSSHConfig
	if v := us.Get("alias", "Hostname"); v != "" {
		t.Fatalf("expected go-git-style Get to return empty on a Match file, got %q", v)
	}
	_, serr := us.GetStrict("alias", "Hostname")
	t.Logf("Match file: UserSettings.Get(alias,Hostname)=\"\" (silently dropped), GetStrict err = %v", serr)
	out, _ := exec.Command("ssh", "-G", "-F", mcfg, "matched.invalid").CombinedOutput()
	if !strings.Contains(string(out), "proxycommand nc m.invalid 22") {
		t.Errorf("ssh -G did not apply Match block: %s", out)
	}
	// go-git's own reader (what actually runs) never looks at these:
	t.Log("go-git consults only Hostname and Port: see plumbing/transport/ssh/common.go doGetHostWithPortFromSSHConfig")
}

func contains_unused(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
