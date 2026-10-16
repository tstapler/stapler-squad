package capability

import (
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/native/gitconfig"
)

// signing reports commit, tag or push signing configured anywhere in the effective config:
// commit.gpgsign and tag.gpgsign by git's boolean rules (a bare key is true), push.gpgSign when
// true or "if-asked", and gpg.format, gpg.program or user.signingkey by presence (any value).
// go-git honours none of them. Never cached.
func (f *facts) signing() ([]backend.Capability, error) {
	cfg, err := f.config()
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"commit.gpgsign", "tag.gpgsign"} {
		if v, set, ok := cfg.Bool(key); set && (v || !ok) { // an invalid boolean makes git die: route there
			return one(backend.CapGPGSign, key), nil
		}
	}
	if e, ok := cfg.Lookup("push.gpgsign"); ok {
		// "if-asked" is not a boolean, so it is invalid here and routes like a true value.
		if v, valid := gitconfig.ParseBool(e.Value, e.HasValue); v || !valid {
			return one(backend.CapGPGSign, "push.gpgsign"), nil
		}
	}
	for _, key := range []string{"gpg.format", "gpg.program", "user.signingkey"} {
		if _, ok := cfg.Lookup(key); ok {
			return one(backend.CapGPGSign, key), nil
		}
	}
	return nil, nil
}
