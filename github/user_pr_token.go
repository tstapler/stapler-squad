package github

import "strings"

// TokenForPR returns the token and login of the account whose poll produced
// the cached PR for key. Dedup in fetch() keeps the first account in
// resolveAllLogins order, so the owner is stable across polls. There is
// deliberately no fallback to another account or the host default token: an
// unknown PR, or an owner that is no longer connected, yields ok=false so
// each nudge acts as one auditable identity. The token never leaves the
// process; callers must not log or return it.
func (c *UserPRCache) TokenForPR(key PRKey) (token, login string, ok bool) {
	if !key.IsValid() {
		return "", "", false
	}
	owner, found := c.cachedPRAccount(key)
	if !found {
		return "", "", false
	}
	v := c.multiLogin.Load()
	if v == nil {
		return "", "", false
	}
	state, _ := v.(*multiLoginState)
	if state == nil {
		return "", "", false
	}
	for _, acc := range state.accounts {
		if NormalizeHost(acc.host) == key.Host() && strings.EqualFold(acc.login, owner) {
			return acc.token, acc.login, true
		}
	}
	return "", "", false
}

// cachedPRAccount finds key in the latest snapshot and returns its owning
// account login.
func (c *UserPRCache) cachedPRAccount(key PRKey) (string, bool) {
	v := c.snapshot.Load()
	if v == nil {
		return "", false
	}
	snap, _ := v.(*userPRSnapshot)
	if snap == nil {
		return "", false
	}
	for i := range snap.prs {
		pr := &snap.prs[i]
		if pr.Number != key.Number() || pr.AccountLogin == "" {
			continue
		}
		if NormalizeHost(pr.Host) != key.Host() ||
			!strings.EqualFold(pr.Owner, key.Owner()) ||
			!strings.EqualFold(pr.Repo, key.RepoName()) {
			continue
		}
		return pr.AccountLogin, true
	}
	return "", false
}
