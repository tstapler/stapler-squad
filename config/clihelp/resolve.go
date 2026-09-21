// Package clihelp discovers what a user-configured CLI program supports:
// it resolves a raw command string to one binary, checks that it is a safe
// executable, and (in later milestones) reads its --help output.
package clihelp

import (
	"path/filepath"
	"strings"
)

// ResolveError says why a command string could not be reduced to one binary.
// The zero value means success.
type ResolveError int

const (
	ResolveOK ResolveError = iota
	ResolveEmpty
	ResolveOnlyEnvAssignments
	ResolveUnbalancedQuote
	// ResolveInvalidToken covers NUL, newline and a leading '-'.
	ResolveInvalidToken
	// ResolveRelativePath covers any token with a separator that is not absolute, and ~user.
	ResolveRelativePath
)

// wrappers run another program; their own --help says nothing about the wrapped one.
var wrappers = map[string]struct{}{
	"env": {}, "sudo": {}, "npx": {}, "uv": {}, "uvx": {}, "nice": {},
	"time": {}, "exec": {}, "nohup": {}, "xargs": {}, "command": {},
}

// Target is the single binary named by a command. Only meaningful when Resolve
// returned ResolveOK; arguments are never retained.
type Target struct {
	Name       string
	IsAbsolute bool
	Wrapper    bool
}

// Resolve reduces command to its first non-env-assignment token. The token is
// never handed to a shell: it must be a bare name or an absolute path (after
// expanding a leading ~ or ~/ with home).
func Resolve(command, home string) (Target, ResolveError) {
	if strings.ContainsRune(command, 0) {
		return Target{}, ResolveInvalidToken
	}
	tokens, ok := splitTokens(command)
	if !ok {
		return Target{}, ResolveUnbalancedQuote
	}
	if len(tokens) == 0 {
		return Target{}, ResolveEmpty
	}
	rest := skipEnvAssignments(tokens)
	if len(rest) == 0 {
		return Target{}, ResolveOnlyEnvAssignments
	}
	name, errCode := expandHome(rest[0], home)
	if errCode != ResolveOK {
		return Target{}, errCode
	}
	if name == "" || name[0] == '-' || strings.ContainsRune(name, '\n') {
		return Target{}, ResolveInvalidToken
	}
	abs := filepath.IsAbs(name)
	if !abs && (strings.ContainsRune(name, '/') || name == "." || name == "..") {
		return Target{}, ResolveRelativePath
	}
	_, isWrapper := wrappers[filepath.Base(name)]
	return Target{Name: name, IsAbsolute: abs, Wrapper: isWrapper}, ResolveOK
}

// splitTokens splits on spaces and tabs, honouring single quotes, double
// quotes and backslash escapes. ok is false on an unterminated quote.
func splitTokens(s string) (tokens []string, ok bool) {
	var cur strings.Builder
	inToken := false
	var quote rune
	escaped := false
	flush := func() {
		if inToken {
			tokens = append(tokens, cur.String())
			cur.Reset()
			inToken = false
		}
	}
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped, inToken = true, true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inToken = r, true
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
			inToken = true
		}
	}
	if quote != 0 || escaped {
		return nil, false
	}
	flush()
	return tokens, true
}

// skipEnvAssignments drops leading NAME=value tokens.
func skipEnvAssignments(tokens []string) []string {
	for len(tokens) > 0 && isEnvAssignment(tokens[0]) {
		tokens = tokens[1:]
	}
	return tokens
}

func isEnvAssignment(tok string) bool {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range tok[:eq] {
		isAlpha := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !isAlpha && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// expandHome expands "~" and "~/..." only; "~user" is not supported and is
// reported as a relative path, as is "~" when home is unknown.
func expandHome(tok, home string) (string, ResolveError) {
	if !strings.HasPrefix(tok, "~") {
		return tok, ResolveOK
	}
	if home == "" || (tok != "~" && !strings.HasPrefix(tok, "~/")) {
		return "", ResolveRelativePath
	}
	return filepath.Join(home, tok[1:]), ResolveOK
}
