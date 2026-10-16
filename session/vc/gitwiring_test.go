package vc

// The production backend registers itself with session/gitprovider from gitwiring's init;
// session/vc cannot import gitwiring outside tests (depguard git_no_gitwiring_import).
import _ "github.com/tstapler/stapler-squad/session/gitwiring"
