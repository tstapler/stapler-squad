package launchcmd

import (
	"errors"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// ErrNeedsShell reports program text that is more than plain words and
// assignments (expansions, globs, pipelines, redirects, several commands).
var ErrNeedsShell = errors.New("launchcmd: program text needs a shell")

// ParseProgram splits a configured program string such as
// `FOO=1 claude --model "sonnet 4"` into leading env assignments and argv words,
// with the quoting a shell would apply already resolved. Text with any
// expansion, glob, tilde, redirect, pipe or second command yields ErrNeedsShell
// so the caller can keep it as RawShell instead of guessing.
func ParseProgram(program string) (ExecCommand, error) {
	file, err := syntax.NewParser().Parse(strings.NewReader(program), "")
	if err != nil {
		return ExecCommand{}, errors.Join(ErrNeedsShell, err)
	}
	if len(file.Stmts) != 1 {
		return ExecCommand{}, ErrNeedsShell
	}
	stmt := file.Stmts[0]
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || stmt.Background || stmt.Coprocess || stmt.Negated || len(stmt.Redirs) > 0 || len(call.Args) == 0 {
		return ExecCommand{}, ErrNeedsShell
	}
	env, err := parseAssigns(call.Assigns)
	if err != nil {
		return ExecCommand{}, err
	}
	cmd := ExecCommand{Env: env}
	for _, w := range call.Args {
		text, err := plainWord(w)
		if err != nil {
			return ExecCommand{}, err
		}
		cmd.Argv = append(cmd.Argv, Word(text))
	}
	return cmd, nil
}

func parseAssigns(assigns []*syntax.Assign) ([]EnvVar, error) {
	var env []EnvVar
	for _, as := range assigns {
		if as.Append || as.Array != nil || as.Index != nil || as.Naked {
			return nil, ErrNeedsShell
		}
		value := ""
		if as.Value != nil {
			var err error
			if value, err = plainWord(as.Value); err != nil {
				return nil, err
			}
		}
		e, err := NewEnvVar(as.Name.Value, value)
		if err != nil {
			return nil, errors.Join(ErrNeedsShell, err)
		}
		env = append(env, e)
	}
	return env, nil
}

// plainWord returns w's value when it is built only from literals and quoted
// literals; anything a shell would expand makes it ErrNeedsShell.
func plainWord(w *syntax.Word) (string, error) {
	for _, part := range w.Parts {
		if !plainPart(part) {
			return "", ErrNeedsShell
		}
	}
	fields, err := expand.Fields(&expand.Config{}, w)
	if err != nil {
		return "", errors.Join(ErrNeedsShell, err)
	}
	if len(fields) != 1 {
		return "", ErrNeedsShell
	}
	return fields[0], nil
}

// plainPart reports whether a word part is a literal, a single-quoted string,
// or a double-quoted string containing only literals.
func plainPart(part syntax.WordPart) bool {
	switch p := part.(type) {
	case *syntax.Lit:
		return !strings.ContainsAny(p.Value, "*?[~")
	case *syntax.SglQuoted:
		return true
	case *syntax.DblQuoted:
		for _, inner := range p.Parts {
			if _, lit := inner.(*syntax.Lit); !lit {
				return false
			}
		}
		return true
	default:
		return false
	}
}
