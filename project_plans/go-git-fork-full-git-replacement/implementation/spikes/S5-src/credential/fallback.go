package credential

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gosshtransport "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// Fetcher is the application-level HTTPS->SSH fallback (ADR-005): on an HTTPS
// auth failure, retry exactly once over SSH with agent auth. The HTTPS
// credential is never attached to the SSH attempt (different AuthMethod type,
// URL has no userinfo).
type Fetcher struct {
	Provider *Provider
	Rewriter *Rewriter
	Fallback bool

	// SSHURLFn overrides SSH URL derivation (tests; GHE hosts with a non-default SSH host).
	SSHURLFn func(httpsURL string) (string, bool)

	HTTPAttempts, SSHAttempts, Rejects int
	lastSSHURL                         string
}

// SSHURL derives git@host:path from https://host/path (no userinfo, no token).
func SSHURL(httpsURL string) (string, bool) {
	u, err := url.Parse(httpsURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return "", false
	}
	return fmt.Sprintf("git@%s:%s", u.Hostname(), strings.TrimPrefix(u.Path, "/")), true
}

func (f *Fetcher) Fetch(ctx context.Context, repo *git.Repository, remote string) error {
	rem, err := repo.Remote(remote)
	if err != nil {
		return err
	}
	raw := rem.Config().URLs[0]
	target := f.Rewriter.Fetch(raw)
	u, perr := url.Parse(target)
	if perr != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return f.fetchSSH(ctx, repo, remote, target)
	}
	res, ferr := f.Provider.Fill(ctx, Request{Protocol: u.Scheme, Host: u.Host})
	var auth transport.AuthMethod
	if ferr == nil {
		auth = NewHostAuth(u.Host, res.Credential)
	}
	f.HTTPAttempts++
	err = repo.FetchContext(ctx, &git.FetchOptions{RemoteName: remote, RemoteURL: target, Auth: auth})
	if err == nil || err == git.NoErrAlreadyUpToDate {
		return nil
	}
	if !IsAuthFailure(err) {
		return err
	}
	if res.Helper != nil { // rejected helper creds are erased, Go-native sources are not
		f.Rejects++
		_ = res.Helper.Reject(ctx, Request{Protocol: u.Scheme, Host: u.Host, Username: res.Username, Password: res.Password}, f.Provider.Timeout)
	}
	if !f.Fallback {
		return err
	}
	derive := f.SSHURLFn
	if derive == nil {
		derive = SSHURL
	}
	sshURL, ok := derive(target)
	if !ok {
		return err
	}
	return f.fetchSSH(ctx, repo, remote, sshURL)
}

func (f *Fetcher) fetchSSH(ctx context.Context, repo *git.Repository, remote, url string) error {
	f.SSHAttempts++
	f.lastSSHURL = url
	auth, err := gosshtransport.NewSSHAgentAuth("git")
	if err != nil {
		return fmt.Errorf("ssh fallback: %w", err)
	}
	err = repo.FetchContext(ctx, &git.FetchOptions{RemoteName: remote, RemoteURL: url, Auth: auth})
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}
	return err
}
