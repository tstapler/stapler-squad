package credential

import (
	"errors"
	"net/http"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// StripAuthOnHostChange removes credentials from a redirected request whose
// host:port differs from the original. net/http itself compares Hostname()
// only (ports ignored), so 127.0.0.1:A -> 127.0.0.1:B would forward them.
func StripAuthOnHostChange(req *http.Request, via []*http.Request) error {
	if len(via) > 0 && req.URL.Host != via[0].URL.Host {
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("Proxy-Authorization")
	}
	return nil
}

// NewHTTPClient builds the custom client installed for http/https.
func NewHTTPClient(rt http.RoundTripper, cacheEntries int) transport.Transport {
	hc := &http.Client{Transport: rt, CheckRedirect: StripAuthOnHostChange}
	return githttp.NewClientWithOptions(hc, &githttp.ClientOptions{
		CacheMaxEntries: cacheEntries,
		RedirectPolicy:  githttp.FollowInitialRedirects,
	})
}

// Install replaces the process-global go-git http(s) protocol handlers.
// Call once at startup: client.Protocols is an unsynchronised map.
func Install(t transport.Transport) {
	client.InstallProtocol("http", t)
	client.InstallProtocol("https", t)
}

// IsAuthFailure reports whether err is go-git's 401/403 mapping.
func IsAuthFailure(err error) bool {
	return errors.Is(err, transport.ErrAuthenticationRequired) || errors.Is(err, transport.ErrAuthorizationFailed)
}

// installFile swaps the file:// handler (test seam; production would call this
// to make local fetch/push spawn-free).
func installFile(t transport.Transport) { client.InstallProtocol("file", t) }
