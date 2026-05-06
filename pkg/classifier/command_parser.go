package classifier

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// CommandInfo contains parsed information extracted from a Bash command string.
type CommandInfo struct {
	// Program is the primary executable being invoked (first non-env-var, non-wrapper token).
	Program string
	// Subcommand is the first positional argument after the program, if it looks like a
	// subcommand (i.e., does not start with '-').
	Subcommand string
	// Category classifies Program into a high-level category (e.g., "vcs", "runtime").
	Category string
	// AllPrograms contains all distinct programs found across the full command line,
	// including across pipes, semicolons, and logical operators.
	AllPrograms []string
}

// ParsedCommand is a single simple command extracted from a (potentially compound) shell command.
type ParsedCommand struct {
	// Program is the primary executable (path-stripped).
	Program string
	// Args is the list of remaining tokens.
	Args []string
	// Raw is the reconstructed "program arg1 arg2 …" string for pattern matching.
	Raw string
	// Redirections lists the targets of any shell redirections (e.g., "> file", ">> file").
	Redirections []string
	// FromCmdSubst is true when this command was extracted from inside a $(...) or `...`
	// substitution. Such commands are evaluated as argument values for the outer command,
	// not as independent top-level commands.
	FromCmdSubst bool
	// HasShellExpansionProgram is true when the program token itself is an unresolvable
	// shell expansion — either a simple variable ($VAR, ${VAR}) or a command substitution
	// ($(cmd)) that could not be resolved to a known program name by path-stripping.
	// Examples: "$SCRIPT", "$(which python)", "${CMD}".
	HasShellExpansionProgram bool
}

// ExtractAllCommands parses cmd with mvdan.cc/sh and recursively walks the AST,
// returning all CallExpr nodes — including those inside $(), backticks, and process
// substitutions. Falls back to splitCommandParts() on parse error.
//
// Commands extracted from inside $(...) substitutions have FromCmdSubst=true.
// Commands whose program token is itself a shell expansion ($VAR, $(cmd)) have
// HasShellExpansionProgram=true.
func ExtractAllCommands(cmd string) []ParsedCommand {
	r := strings.NewReader(cmd)
	f, err := syntax.NewParser().Parse(r, "")
	if err != nil {
		// Fallback: split on shell operators and treat each part as a raw command.
		// Skip parts with no executable (env-var-only assignments like OWNER=val).
		parts := splitCommandParts(cmd)
		result := make([]ParsedCommand, 0, len(parts))
		for _, p := range parts {
			prog, _ := extractProgramAndSubcommand(p)
			if prog == "" {
				continue // bare env-var assignment or empty fragment — no-op
			}
			result = append(result, ParsedCommand{Program: prog, Raw: p})
		}
		return result
	}

	// First pass: collect all Stmt nodes that appear directly inside a CmdSubst
	// (i.e., inside $(...) or `...`). These are argument-value producers, not
	// independent commands.
	cmdSubstStmts := make(map[*syntax.Stmt]bool)
	syntax.Walk(f, func(node syntax.Node) bool {
		cs, ok := node.(*syntax.CmdSubst)
		if !ok {
			return true
		}
		for _, s := range cs.Stmts {
			cmdSubstStmts[s] = true
		}
		return true
	})

	var cmds []ParsedCommand
	syntax.Walk(f, func(node syntax.Node) bool {
		stmt, ok := node.(*syntax.Stmt)
		if !ok || stmt.Cmd == nil {
			return true
		}

		call, ok := stmt.Cmd.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}

		// Reconstruct words into string tokens.
		var tokens []string
		printer := syntax.NewPrinter()
		for _, word := range call.Args {
			var sb strings.Builder
			if printErr := printer.Print(&sb, word); printErr == nil {
				// Strip surrounding quotes from simple quoted words.
				tok := sb.String()
				tok = stripOuterQuotes(tok)
				tokens = append(tokens, tok)
			}
		}

		// Capture redirections from the Stmt node.
		var redirects []string
		for _, redir := range stmt.Redirs {
			if redir.Word != nil {
				var sb strings.Builder
				if printErr := printer.Print(&sb, redir.Word); printErr == nil {
					redirects = append(redirects, stripOuterQuotes(sb.String()))
				}
			}
		}

		if len(tokens) == 0 {
			return true
		}

		prog := tokens[0]
		// Strip path prefix (/usr/bin/git → git, $HOME/bin/git → git).
		if idx := strings.LastIndex(prog, "/"); idx >= 0 {
			prog = prog[idx+1:]
		}

		// Detect when the program itself is an unresolvable shell expansion.
		// After path-stripping, a program that still starts with '$' means the
		// executable name cannot be determined statically.
		// Examples: "$SCRIPT" → prog="$SCRIPT"; "$(which python)" → prog="$(which python)"
		// Contrast: "$HOME/bin/git" → prog="git" (path-stripped; NOT an expansion).
		hasExpansionProg := len(prog) > 0 && prog[0] == '$'

		// wrapperCommands is now empty; this loop is a no-op and exists for forward
		// compatibility. Recursive-eval programs (sudo, xargs, rtk, etc.) are handled
		// by classifyInternal via ExtractInnerCommand after the AST walk completes.
		startIdx := 1
		for wrapperCommands[strings.ToLower(prog)] && startIdx < len(tokens) {
			// Skip env-var assignments (KEY=VALUE) before the real program.
			for startIdx < len(tokens) && envVarPattern.MatchString(tokens[startIdx]) {
				startIdx++
			}
			if startIdx >= len(tokens) {
				break // bare wrapper with no following command
			}
			prog = tokens[startIdx]
			if idx := strings.LastIndex(prog, "/"); idx >= 0 {
				prog = prog[idx+1:]
			}
			startIdx++
		}

		raw := strings.Join(tokens, " ")
		cmds = append(cmds, ParsedCommand{
			Program:                  prog,
			Args:                     tokens[startIdx:],
			Raw:                      raw,
			Redirections:             redirects,
			FromCmdSubst:             cmdSubstStmts[stmt],
			HasShellExpansionProgram: hasExpansionProg,
		})
		return true
	})

	if len(cmds) == 0 {
		// Command had no callable expressions (e.g. pure redirections).
		parts := splitCommandParts(cmd)
		for _, p := range parts {
			prog, _ := extractProgramAndSubcommand(p)
			cmds = append(cmds, ParsedCommand{Program: prog, Raw: p})
		}
	}
	return cmds
}

// envVarRefPattern matches $VAR and ${VAR} shell variable references.
// Group 1 captures the name from ${VAR}; group 2 captures the name from $VAR.
var envVarRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// ExpandEnvVars replaces $VAR and ${VAR} references in cmd, mirroring Python's
// os.path.expandvars behaviour:
//  1. env map (caller overrides) — checked first.
//  2. OS environment (os.LookupEnv) — fallback, so real env vars work without
//     having to enumerate them in the map.
//  3. Unknown variables are left verbatim (original $VAR / ${VAR} form preserved).
//
// Command substitutions ($(...)) are not expanded — only simple variable references.
func ExpandEnvVars(cmd string, env map[string]string) string {
	return envVarRefPattern.ReplaceAllStringFunc(cmd, func(m string) string {
		var name string
		if strings.HasPrefix(m, "${") {
			name = m[2 : len(m)-1]
		} else {
			name = m[1:]
		}
		if val, ok := env[name]; ok {
			return val
		}
		if val, ok := os.LookupEnv(name); ok {
			return val
		}
		return m // leave unexpanded, preserving original $VAR / ${VAR} form
	})
}

// stripOuterQuotes removes a single layer of surrounding single or double quotes.
func stripOuterQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// PythonInfo contains information extracted from a Python command invocation.
type PythonInfo struct {
	// Imports contains top-level module names imported in inline Python code.
	// Only populated when -c is used (inline code), not for script files.
	Imports []string
	// IsInline is true when code was passed via the -c flag.
	IsInline bool
	// Code is the extracted Python source passed via -c, with surrounding quotes stripped.
	Code string
	// CodeWithoutComments is Code with whole-line Python comments (lines whose first
	// non-whitespace character is '#') removed. Use this for banned-pattern detection
	// so that a comment like "# open() is dangerous" does not trigger a false positive.
	CodeWithoutComments string
}

// shellKeywords is the set of Bash/POSIX shell flow-control keywords. When the
// naive splitCommandParts fallback is used, these tokens should never be treated
// as a program name; extractProgramAndSubcommand skips them.
var shellKeywords = map[string]bool{
	"for": true, "while": true, "until": true, "if": true,
	"then": true, "else": true, "elif": true, "fi": true,
	"do": true, "done": true, "case": true, "esac": true,
	"in": true, "select": true, "function": true,
}

// bgOperatorPattern matches a standalone & (background operator) that is not
// part of a redirect pattern like 2>&1, >&2, or &>file. It matches & preceded
// by a non-> character and surrounded by whitespace or string boundaries.
var bgOperatorPattern = regexp.MustCompile(`([^>])&([^>&])`)

var (
	// envVarPattern matches shell environment variable assignments like FOO=bar or FOO="bar".
	envVarPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

	// pythonImportPattern matches Python import statements in two groups:
	//   group 1: module name from "from X import ..."
	//   group 2: module list from "import X, Y, Z"
	pythonImportPattern = regexp.MustCompile(`(?m)(?:from\s+(\S+)\s+import|import\s+([^#\n;]+))`)
)

// wrapperCommands formerly held programs that wrap an inner command via simple
// token-stripping. All entries have been migrated to recursiveEvalPrograms, which
// classifies inner commands through the full rule engine instead of a static
// allowlist. The map is kept empty so the AST walker loop below is a no-op without
// requiring structural changes to ExtractAllCommands.
var wrapperCommands = map[string]bool{}

// recursiveEvalSpec describes how to extract the inner command from a wrapper program.
type recursiveEvalSpec struct {
	// flagArgs maps flag names (e.g. "-n", "--max-args") to true when that flag
	// consumes the next token as its value. Boolean flags are handled automatically
	// by the generic "-" prefix skip and need not be listed here.
	flagArgs map[string]bool
	// skipPositionals is the number of positional (non-flag) arguments to consume
	// and discard after all flags, before the inner command begins.
	// Example: timeout has a mandatory DURATION positional before the command.
	skipPositionals int
	// skipEnvAssignments, when true, skips any KEY=value tokens immediately after
	// the flags/positionals before the inner command begins.
	// Required for `env VAR=val cmd` where env-var assignments precede the command.
	skipEnvAssignments bool
	// passthroughSubcmds lists first-token subcommand names that are themselves
	// transparent pass-through modes of the wrapper. When the first inner token
	// matches an entry here it is skipped so the actual command can be found.
	// Example: "rtk proxy git status" — proxy is rtk's pass-through sub-mode;
	// skip "proxy" to get the actual inner command "git status".
	passthroughSubcmds map[string]bool
}

// recursiveEvalPrograms defines programs whose inner command should be extracted and
// classified through the full rule engine rather than matched against a static allowlist.
// Adding a program here enables transparent wrapper support: `xargs git push` escalates
// because `git push` escalates; `sudo go test ./...` auto-allows because `go test` does.
var recursiveEvalPrograms = map[string]recursiveEvalSpec{

	// ── stdin parallelism ─────────────────────────────────────────────────────
	"xargs": {flagArgs: map[string]bool{
		"-n": true, "--max-args": true,
		"-P": true, "--max-procs": true,
		"-s": true, "--max-chars": true,
		"-L": true, "-l": true, "--max-lines": true,
		"-I": true, "--replace": true,
		"-d": true, "--delimiter": true,
		"-E": true, "--eof": true,
		"-a": true, "--arg-file": true,
	}},
	// GNU parallel — same role as xargs but with richer job-control flags.
	"parallel": {flagArgs: map[string]bool{
		"-j": true, "--jobs": true,
		"-n": true, "--max-args": true,
		"-N": true,
		"-I": true, "--replace": true,
		"-d": true, "--delimiter": true,
		"--timeout":  true,
		"--sshlogin": true, "-S": true,
		"--sshloginfile": true,
		"--basefile":     true, "--trc": true,
		"--results": true, "--joblog": true,
		"--delay": true, "--retries": true,
	}},

	// ── timing / resource limits ──────────────────────────────────────────────
	// timeout DURATION COMMAND [ARGS] — skipPositionals=1 skips the DURATION value.
	"timeout": {
		flagArgs: map[string]bool{
			"-k": true, "--kill-after": true,
			"-s": true, "--signal": true,
		},
		skipPositionals: 1,
	},
	// nice [-n ADJUSTMENT] COMMAND [ARGS]
	"nice": {flagArgs: map[string]bool{
		"-n": true, "--adjustment": true,
	}},

	// ── I/O buffering ─────────────────────────────────────────────────────────
	"stdbuf": {flagArgs: map[string]bool{
		"-i": true, "--input": true,
		"-o": true, "--output": true,
		"-e": true, "--error": true,
	}},

	// ── virtual display ───────────────────────────────────────────────────────
	"xvfb-run": {flagArgs: map[string]bool{
		"-n": true, "--server-num": true,
		"-e": true, "--error-file": true,
		"-f": true, "--auth-file": true,
		"-s": true, "--server-args": true,
		"-w": true, "--wait": true,
	}},

	// ── I/O scheduling ────────────────────────────────────────────────────────
	// ionice [-c class] [-n level] COMMAND [ARGS]  (when not in -p/--pid mode)
	"ionice": {flagArgs: map[string]bool{
		"-c": true, "--class": true,
		"-n": true, "--classdata": true,
	}},

	// ── session / process control ─────────────────────────────────────────────
	"setsid":    {flagArgs: map[string]bool{}}, // boolean flags only (-w/-c/-f)
	"catchsegv": {flagArgs: map[string]bool{}}, // no flags before command
	"nohup":     {flagArgs: map[string]bool{}}, // no flags before command

	// ── environment control ───────────────────────────────────────────────────
	// env [-i] [-0] [-u NAME] [-C DIR] [VAR=val...] COMMAND [ARGS]
	"env": {
		flagArgs: map[string]bool{
			"-u": true, "--unset": true,
			"-C": true, "--chdir": true,
		},
		skipEnvAssignments: true,
	},

	// ── periodic execution ────────────────────────────────────────────────────
	// watch [-n secs] [-d] [-t] ... COMMAND
	"watch": {flagArgs: map[string]bool{
		"-n": true, "--interval": true,
	}},

	// ── timing (GNU time / bash builtin) ─────────────────────────────────────
	// GNU time: time [-apqvV] [-f FORMAT] [-o FILE] COMMAND
	"time": {flagArgs: map[string]bool{
		"-f": true, "--format": true,
		"-o": true, "--output": true,
	}},

	// ── shell execution control ───────────────────────────────────────────────
	// exec [-cl] [-a name] COMMAND [ARGS]
	"exec": {flagArgs: map[string]bool{
		"-a": true, // exec -a name cmd: sets argv[0]
	}},
	// command [-pVv] COMMAND [ARGS]  — POSIX shell builtin, boolean flags only
	"command": {flagArgs: map[string]bool{}},

	// ── privilege escalation ──────────────────────────────────────────────────
	// sudo [-u user] [-g group] ... COMMAND [ARGS]
	"sudo": {flagArgs: map[string]bool{
		"-u": true, "--user": true,
		"-g": true, "--group": true,
		"-C": true, "--close-from": true,
		"-D": true, "--chdir": true,
		"-T": true, "--command-timeout": true,
		"-p": true, "--prompt": true,
		"-h": true, "--host": true,
		"-r": true, "--role": true,
		"-t": true, "--type": true,
		"-U": true, "--other-user": true,
	}},
	// doas [-u user] [-C config] COMMAND [ARGS]
	"doas": {flagArgs: map[string]bool{
		"-u": true, "--user": true,
		"-C": true,
	}},
	// run0 (systemd's sudo replacement) uses --key=value form; inline "=" is handled
	// automatically so no explicit flagArgs entries are needed.
	"run0": {flagArgs: map[string]bool{}},

	// ── rtk transparent proxy ─────────────────────────────────────────────────
	// rtk is a token-saving CLI proxy: `rtk git status` → git status.
	// `rtk proxy git status` is rtk's explicit pass-through sub-mode; skip "proxy"
	// to reach the actual inner command.
	"rtk": {
		flagArgs:           map[string]bool{},
		passthroughSubcmds: map[string]bool{"proxy": true},
	},
}

// ExtractInnerCommand extracts the inner command string from a recursive-eval wrapper
// invocation (e.g. xargs, parallel, timeout, sudo, nice, env, rtk). It skips the
// wrapper's own flags (and value tokens for flagArgs entries), then any skipPositionals
// tokens, then any env-var assignments (when skipEnvAssignments is true), then any
// leading passthroughSubcmds token. Returns the remaining tokens joined as a string.
//
// Returns "" when:
//   - prog is not in recursiveEvalPrograms
//   - no inner command remains after skipping flags/positionals/env-vars
//   - a parallel input separator (:::) appears before any inner command token
func ExtractInnerCommand(prog string, args []string) string {
	spec, ok := recursiveEvalPrograms[strings.ToLower(prog)]
	if !ok {
		return ""
	}

	i := 0

	// Phase 1: skip the wrapper's own flags (and their value tokens).
	for i < len(args) {
		arg := args[i]

		// "--" explicitly ends this program's own flags; inner command follows.
		if arg == "--" {
			i++
			break
		}

		// parallel uses ::: / :::: to separate command from input sources.
		// If ::: appears before any non-flag token, there is no explicit inner command.
		if arg == ":::" || arg == "::::" {
			return ""
		}

		if strings.HasPrefix(arg, "-") {
			// Long flags with inline value (e.g. --max-args=5): the value is in the
			// same token — only skip this one token.
			flagKey := arg
			if eqIdx := strings.Index(arg, "="); eqIdx > 0 {
				flagKey = arg[:eqIdx]
			}
			if spec.flagArgs[flagKey] && flagKey != arg {
				i++ // inline value form
				continue
			}
			if spec.flagArgs[flagKey] {
				i += 2 // two-token form: flag + value
				continue
			}
			// Boolean flag or combined short flag with inline value (e.g. -n1, -I{}):
			// self-contained, skip just this token.
			i++
			continue
		}

		// First non-flag token marks the end of the flags section.
		break
	}

	// Phase 2: skip positional arguments that belong to the wrapper itself
	// (e.g. the DURATION in "timeout 30 git status").
	for skip := 0; skip < spec.skipPositionals && i < len(args); skip++ {
		i++
	}

	// Phase 3: skip env-var assignments that belong to the wrapper
	// (e.g. "env KEY=val git status" — skip KEY=val).
	if spec.skipEnvAssignments {
		for i < len(args) && envVarPattern.MatchString(args[i]) {
			i++
		}
	}

	if i >= len(args) {
		return ""
	}

	// Collect inner command tokens, stopping at a parallel input separator.
	var parts []string
	for j := i; j < len(args); j++ {
		if args[j] == ":::" || args[j] == "::::" {
			break
		}
		parts = append(parts, args[j])
	}

	// Phase 4: skip pass-through subcommands (e.g. "proxy" in "rtk proxy git status").
	if len(parts) > 0 && spec.passthroughSubcmds[strings.ToLower(parts[0])] {
		parts = parts[1:]
	}

	return strings.Join(parts, " ")
}

// deepSubcommandPrograms is the set of programs that use two-level subcommand hierarchies
// (e.g., "gh pr create", "aws s3 cp", "kubectl get pods"). For these programs,
// extractProgramAndSubcommand captures up to 2 positional subcommand tokens.
var deepSubcommandPrograms = map[string]bool{
	"gh":       true, // gh pr create, gh repo clone, gh workflow run
	"aws":      true, // aws s3 cp, aws ec2 describe-instances
	"gcloud":   true, // gcloud compute instances list
	"az":       true, // az vm list, az group create
	"doctl":    true, // doctl compute droplet list
	"fly":      true, // fly apps list
	"flyctl":   true, // flyctl apps list
	"kubectl":  true, // kubectl get pods, kubectl apply
	"docker":   true, // docker container run, docker image pull
	"heroku":   true, // heroku apps:info, heroku config:set
	"ip":       true, // ip route show, ip addr add, ip link set
	"asdf":     true, // asdf plugin list, asdf plugin add, asdf list all
	"localdev": true, // localdev ai-setup status, localdev ai-setup config get
}

// prefixFlagArgs maps programs to the set of flags that each consume one additional
// argument as their value. When scanning for subcommand tokens, these flag+value pairs
// are skipped so that subcommands appearing after them (e.g., git -C /repo status) are
// correctly identified.
var prefixFlagArgs = map[string]map[string]bool{
	"git": {
		"-C":          true, // git -C <path> <subcmd>
		"--git-dir":   true,
		"--work-tree": true,
		"-c":          true, // git -c key=val <subcmd>
		"--namespace": true,
	},
	"ssh": {"-i": true, "-p": true, "-o": true, "-l": true, "-J": true},
}

// isSubcommandLike returns true if tok looks like a subcommand name rather than a flag or
// path argument. A subcommand starts with a letter, contains only letters/digits/hyphens/
// underscores, and is at most 25 characters — ruling out paths, globs, and URLs.
func isSubcommandLike(tok string) bool {
	if len(tok) == 0 || len(tok) > 25 {
		return false
	}
	c := tok[0]
	if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
		return false
	}
	for _, r := range tok {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// PythonPrograms is the set of program names that invoke a Python interpreter.
var PythonPrograms = map[string]bool{
	"python": true, "python3": true, "python2": true,
	"pypy": true, "pypy3": true,
}

// extractSubcommand returns the subcommand portion from a parsed command's argument list.
// It handles prefix flags (e.g., git -C <path>) by skipping flag+value pairs defined in
// prefixFlagArgs. Other flags are also skipped so that subcommands following any flags
// are correctly identified (fixing the git -C /repo status issue).
// For programs in deepSubcommandPrograms, up to 2 subcommand tokens are captured.
func extractSubcommand(prog string, args []string) string {
	skipFlags := prefixFlagArgs[prog]
	maxSub := 1
	if deepSubcommandPrograms[prog] {
		maxSub = 2
	}

	var subParts []string
	i := 0
	for i < len(args) && len(subParts) < maxSub {
		arg := args[i]
		// Skip prefix flags and their value argument (e.g., -C /repo).
		if skipFlags != nil && skipFlags[arg] {
			i += 2
			continue
		}
		// Skip any other flag without consuming the next token.
		if strings.HasPrefix(arg, "-") {
			i++
			continue
		}
		// Must look like a subcommand name (not a path, glob, URL, etc.).
		if !isSubcommandLike(arg) {
			break
		}
		subParts = append(subParts, arg)
		i++
	}
	return strings.Join(subParts, " ")
}

// isPythonProgram returns true if prog is a Python interpreter, including versioned
// variants like python3.11, python3.9, pypy3.10, etc.
func isPythonProgram(prog string) bool {
	if PythonPrograms[prog] {
		return true
	}
	for base := range PythonPrograms {
		if strings.HasPrefix(prog, base+".") {
			return true
		}
	}
	return false
}

// detectPythonMode classifies how a Python interpreter is being invoked.
// Returns one of: "inline" (-c), "module" (-m), "version" (-V/--version),
// "script" (*.py file), or "" (unknown/other).
func detectPythonMode(prog string, args []string) string {
	if !isPythonProgram(prog) {
		return ""
	}
	for i, arg := range args {
		switch arg {
		case "-c":
			// Return "inline-multiline" when the code block contains newlines so
			// that rules can distinguish interactive one-liners from embedded scripts
			// and recommend more reviewable alternatives (e.g. a temp .py file).
			if i+1 < len(args) && strings.ContainsRune(args[i+1], '\n') {
				return "inline-multiline"
			}
			return "inline"
		case "-m":
			return "module"
		case "-V", "--version":
			return "version"
		}
		if strings.HasSuffix(arg, ".py") {
			return "script"
		}
	}
	return ""
}

// matchesProgram checks whether prog matches any entry in the programs slice.
// It performs exact matching and also handles versioned interpreters:
// a base name like "python3" matches "python3.11", "python3.9", etc.
func matchesProgram(programs []string, prog string) bool {
	for _, p := range programs {
		if prog == p {
			return true
		}
		// Prefix match for versioned interpreters (python3 → python3.11).
		if strings.HasPrefix(prog, p+".") {
			return true
		}
	}
	return false
}

// ParseBashCommand extracts structured categorization information from a Bash command.
// It uses the mvdan.cc/sh AST parser (via ExtractAllCommands) as the primary path,
// which correctly handles subshells, pipelines, compound commands, and env-var prefixes.
// On parse error, ExtractAllCommands falls back to splitCommandParts automatically.
//
// The primary program and subcommand are taken from the first CallExpr in the AST.
// AllPrograms collects all distinct programs across the full command.
func ParseBashCommand(command string) CommandInfo {
	cmds := ExtractAllCommands(command)
	if len(cmds) == 0 {
		return CommandInfo{}
	}

	first := cmds[0]
	prog := first.Program
	sub := extractSubcommand(prog, first.Args)

	// Collect all distinct programs across the full command.
	seen := make(map[string]bool)
	var allProgs []string
	for _, c := range cmds {
		if c.Program != "" && !seen[c.Program] {
			seen[c.Program] = true
			allProgs = append(allProgs, c.Program)
		}
	}

	return CommandInfo{
		Program:     prog,
		Subcommand:  sub,
		Category:    categorizeProgram(prog),
		AllPrograms: allProgs,
	}
}

// ParsePythonCommand extracts Python import information from a python/python3 invocation.
// Only parses inline code passed via the -c flag; script files are not read.
func ParsePythonCommand(command string) PythonInfo {
	// Locate the -c flag.
	idx := strings.Index(command, " -c ")
	if idx == -1 {
		return PythonInfo{}
	}

	info := PythonInfo{IsInline: true}
	code := strings.TrimSpace(command[idx+4:])

	// Strip surrounding single or double quotes.
	if len(code) >= 2 {
		first, last := code[0], code[len(code)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			code = code[1 : len(code)-1]
		}
	}

	info.Code = code
	info.CodeWithoutComments = stripPythonCommentLines(code)
	// Extract imports from comment-stripped code so that commented-out import
	// statements (e.g. "# import os") are not treated as real imports.
	info.Imports = extractPythonImports(info.CodeWithoutComments)
	return info
}

// splitCommandParts splits a shell command string into individual simple commands
// by tokenizing on |, ;, &&, ||, &, and newlines. This is intentionally simple and
// does not handle quoted strings or subshell constructs.
func splitCommandParts(cmd string) []string {
	// Normalize line continuations: backslash-newline joins continuation lines.
	cmd = strings.ReplaceAll(cmd, "\\\n", " ")

	// Replace && and || with a single sentinel before splitting on remaining separators.
	cmd = strings.ReplaceAll(cmd, "&&", "\x00")
	cmd = strings.ReplaceAll(cmd, "||", "\x00")

	// Replace standalone & (background operator) with the sentinel.
	// bgOperatorPattern avoids splitting redirect patterns like 2>&1, >&2, &>file.
	// We pad with spaces to ensure the regex can match at boundaries, then strip.
	cmd = bgOperatorPattern.ReplaceAllStringFunc(" "+cmd+" ", func(m string) string {
		// Preserve the non-> char before & and the char after, replacing & with sentinel.
		return string(m[0]) + "\x00" + string(m[2])
	})
	cmd = strings.TrimSpace(cmd)

	parts := strings.FieldsFunc(cmd, func(r rune) bool {
		return r == '|' || r == ';' || r == '\n' || r == '\x00'
	})

	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// Skip empty parts and shell comment lines.
		if p != "" && !strings.HasPrefix(p, "#") {
			result = append(result, p)
		}
	}
	return result
}

// extractProgramAndSubcommand returns the primary program name and the subcommand
// (if any) from a single simple command. For programs in deepSubcommandPrograms,
// it captures up to 2 positional subcommand tokens (e.g., "gh pr create" → "pr create").
// For all other programs it captures at most 1 token.
// Prefix flags (e.g., git -C <path>) are skipped so that subcommands following them
// are correctly identified.
func extractProgramAndSubcommand(cmd string) (prog, sub string) {
	tokens := strings.Fields(cmd)
	// Build a slice of args (tokens after env vars and wrappers are stripped).
	var args []string

	for _, tok := range tokens {
		// Skip environment variable assignments.
		if envVarPattern.MatchString(tok) {
			continue
		}

		// Strip leading path prefix (/usr/local/bin/git → git).
		bare := tok
		if slashIdx := strings.LastIndex(bare, "/"); slashIdx >= 0 {
			bare = bare[slashIdx+1:]
		}

		if prog == "" {
			// Skip shell flow-control keywords (for, while, if, …).
			if shellKeywords[bare] {
				continue
			}
			// Skip wrapper/recursive-eval programs so the underlying command is returned.
			if wrapperCommands[bare] {
				continue
			}
			if _, ok := recursiveEvalPrograms[bare]; ok {
				continue
			}
			prog = bare
		} else {
			args = append(args, tok)
		}
	}

	sub = extractSubcommand(prog, args)
	return
}

// categorizeProgram maps a program name to a high-level category string.
func categorizeProgram(prog string) string {
	switch prog {
	case "git", "hg", "svn", "fossil", "jj":
		return "vcs"
	case "npm", "npx", "yarn", "pnpm", "bun", "node":
		return "node"
	case "pip", "pip3", "uv", "poetry", "pipenv", "conda", "mamba", "pdm":
		return "python_pkg"
	case "go", "gofmt":
		return "go"
	case "cargo", "rustup", "rust-analyzer":
		return "rust"
	case "brew", "apt", "apt-get", "yum", "dnf", "pacman", "snap", "flatpak", "port":
		return "system_pkg"
	case "docker", "podman", "nerdctl", "buildah":
		return "container"
	case "kubectl", "helm", "kustomize", "k9s", "flux", "argocd":
		return "kubernetes"
	case "terraform", "tofu", "pulumi", "cdktf":
		return "iac"
	case "python", "python3", "python2", "pypy", "pypy3":
		return "python"
	case "ruby", "gem", "bundle", "rake":
		return "ruby"
	case "java", "javac", "mvn", "gradle", "gradlew":
		return "java"
	case "make", "cmake", "ninja", "meson", "bazel", "buck", "just":
		return "build"
	case "curl", "wget", "httpie", "http", "xh":
		return "network"
	case "ssh", "scp", "rsync", "sftp", "mosh":
		return "remote"
	case "ls", "ll", "find", "locate", "fd", "tree":
		return "filesystem"
	case "cat", "head", "tail", "less", "more", "bat", "view":
		return "file_view"
	case "cp", "mv", "rm", "mkdir", "rmdir", "touch", "ln", "install":
		return "file_ops"
	case "grep", "rg", "ag", "ack":
		return "search"
	case "sed", "awk", "tr", "cut", "sort", "uniq", "wc", "paste":
		return "text_proc"
	case "echo", "printf", "read", "export", "source", ".", "cd", "pwd", "which":
		return "shell_builtin"
	case "bash", "sh", "zsh", "fish", "dash", "ksh":
		return "shell"
	case "jq", "yq", "dasel", "fx", "gron":
		return "data_proc"
	case "psql", "mysql", "sqlite3", "mongosh", "redis-cli", "pgcli":
		return "database"
	case "pytest", "jest", "vitest", "mocha", "jasmine", "rspec", "karma":
		return "testing"
	case "aws", "gcloud", "az", "doctl", "heroku", "flyctl":
		return "cloud_cli"
	case "gh", "lab", "glab":
		return "git_hosting"
	case "tar", "zip", "unzip", "gzip", "bzip2", "xz", "7z":
		return "archive"
	case "openssl", "gpg", "age", "pass":
		return "crypto"
	case "kill", "pkill", "ps", "top", "htop", "lsof", "strace", "dtrace":
		return "process"
	default:
		return "other"
	}
}

// stripPythonCommentLines removes whole-line Python comments from code. A line is
// treated as a comment when its first non-whitespace character is '#'. Inline
// comments (# after code on the same line) and '#' inside string literals are
// intentionally kept to avoid incorrectly altering string content.
func stripPythonCommentLines(code string) string {
	lines := strings.Split(code, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			out = append(out, "") // preserve line count for readability
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// extractPythonImports parses Python import statements from source code using regex.
// It handles: import X, import X as Y, from X import Y, from X.Y import Z.
func extractPythonImports(code string) []string {
	matches := pythonImportPattern.FindAllStringSubmatch(code, -1)
	seen := make(map[string]bool)
	var imports []string

	for _, m := range matches {
		fromModule := strings.TrimSpace(m[1])
		importList := strings.TrimSpace(m[2])

		if fromModule != "" {
			// "from X.Y import Z" → top-level package is X
			pkg := topLevelPackage(fromModule)
			if pkg != "" && !seen[pkg] {
				seen[pkg] = true
				imports = append(imports, pkg)
			}
		} else if importList != "" {
			// "import X, Y as Z, W" → extract X, Y, W (stripping " as alias")
			for _, part := range strings.Split(importList, ",") {
				part = strings.TrimSpace(part)
				if asIdx := strings.Index(part, " as "); asIdx >= 0 {
					part = strings.TrimSpace(part[:asIdx])
				}
				pkg := topLevelPackage(part)
				if pkg != "" && !seen[pkg] {
					seen[pkg] = true
					imports = append(imports, pkg)
				}
			}
		}
	}
	return imports
}

// SecurityFinding represents a potential security risk discovered during deep AST analysis.
type SecurityFinding struct {
	ID          string
	Name        string
	RiskLevel   RiskLevel
	Reason      string
	Alternative string
}

// AuditCommand performs deep AST analysis on a shell command to identify dangerous patterns.
func AuditCommand(cmd string, cwd string) []SecurityFinding {
	r := strings.NewReader(cmd)
	f, err := syntax.NewParser().Parse(r, "")
	if err != nil {
		return nil
	}

	var findings []SecurityFinding

	syntax.Walk(f, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.BinaryCmd:
			// Detect dangerous pipelines: <download> | <interpreter>
			if n.Op == syntax.Pipe {
				if isDownloadCommand(n.X) && isInterpreterCommand(n.Y) {
					findings = append(findings, SecurityFinding{
						ID:          "audit-download-execute",
						Name:        "Download and Execute Pipeline",
						RiskLevel:   RiskCritical,
						Reason:      "Piping a download directly into an interpreter is a high-security risk.",
						Alternative: "Download the script first, inspect it, and then execute it manually.",
					})
				}
			}

		case *syntax.Stmt:
			// Detect dangerous redirections
			for _, redir := range n.Redirs {
				if redir.Word != nil {
					var sb strings.Builder
					if err := syntax.NewPrinter().Print(&sb, redir.Word); err == nil {
						path := stripOuterQuotes(sb.String())
						if isSensitivePath(path) {
							findings = append(findings, SecurityFinding{
								ID:          "audit-sensitive-redirect",
								Name:        "Sensitive Path Redirection",
								RiskLevel:   RiskCritical,
								Reason:      fmt.Sprintf("Redirecting output to sensitive path %q is highly dangerous.", path),
								Alternative: "Review the target path and ensure you have permission to modify it.",
							})
						}
					}
				}
			}
		}
		return true
	})

	// Detect rm with recursive+force flags targeting root or home directory.
	syntax.Walk(f, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		var progSB strings.Builder
		if err := syntax.NewPrinter().Print(&progSB, call.Args[0]); err != nil {
			return true
		}
		prog := stripOuterQuotes(progSB.String())
		if idx := strings.LastIndex(prog, "/"); idx >= 0 {
			prog = prog[idx+1:]
		}
		if prog != "rm" {
			return true
		}
		hasRecursiveForce := false
		for _, arg := range call.Args[1:] {
			var sb strings.Builder
			if err := syntax.NewPrinter().Print(&sb, arg); err != nil {
				continue
			}
			a := stripOuterQuotes(sb.String())
			if strings.HasPrefix(a, "-") && strings.ContainsAny(a, "r") && strings.ContainsAny(a, "f") {
				hasRecursiveForce = true
			}
		}
		if !hasRecursiveForce {
			return true
		}
		for _, arg := range call.Args[1:] {
			var sb strings.Builder
			if err := syntax.NewPrinter().Print(&sb, arg); err != nil {
				continue
			}
			target := expandPathForAudit(stripOuterQuotes(sb.String()), cwd)
			if target == "/" || target == homeDir {
				findings = append(findings, SecurityFinding{
					ID:          "audit-rm-rf-critical-path",
					Name:        "Recursive Force Delete on Critical Path",
					RiskLevel:   RiskCritical,
					Reason:      fmt.Sprintf("rm -rf targeting %q would cause irreversible data loss.", target),
					Alternative: "Specify a precise subdirectory path instead.",
				})
			}
		}
		return true
	})

	return findings
}

// expandPathForAudit expands ~, $HOME, and relative paths for security audit.
// cwd is the working directory used to resolve relative paths like "." or "..".
func expandPathForAudit(path string, cwd string) string {
	// Strip trailing slashes for comparison (rm -rf ~/ == rm -rf ~).
	cleaned := strings.TrimRight(path, "/")
	if cleaned == "~" || cleaned == "$HOME" {
		return homeDir
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Clean(homeDir + path[1:])
	}
	if strings.HasPrefix(path, "$HOME/") {
		return filepath.Clean(homeDir + path[5:])
	}
	// Resolve relative paths (., .., etc.) against the working directory.
	if !filepath.IsAbs(path) && cwd != "" {
		return filepath.Clean(filepath.Join(cwd, path))
	}
	return filepath.Clean(path)
}

// homeDir caches the user's home directory for audit path expansion.
var homeDir = func() string {
	h, _ := os.UserHomeDir()
	return h
}()

func isDownloadCommand(n syntax.Node) bool {
	// For simplicity, we check if the command name is curl or wget.
	// This can be expanded to check arguments as well.
	var prog string
	syntax.Walk(n, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok && len(call.Args) > 0 {
			var sb strings.Builder
			if err := syntax.NewPrinter().Print(&sb, call.Args[0]); err == nil {
				prog = stripOuterQuotes(sb.String())
				return false // stop walking
			}
		}
		return true
	})

	// Strip path
	if idx := strings.LastIndex(prog, "/"); idx >= 0 {
		prog = prog[idx+1:]
	}

	return prog == "curl" || prog == "wget"
}

func isInterpreterCommand(n syntax.Node) bool {
	var prog string
	syntax.Walk(n, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok && len(call.Args) > 0 {
			var sb strings.Builder
			if err := syntax.NewPrinter().Print(&sb, call.Args[0]); err == nil {
				prog = stripOuterQuotes(sb.String())
				return false // stop walking
			}
		}
		return true
	})

	// Strip path
	if idx := strings.LastIndex(prog, "/"); idx >= 0 {
		prog = prog[idx+1:]
	}

	interpreters := map[string]bool{
		"bash": true, "sh": true, "zsh": true, "dash": true, "ksh": true,
		"python": true, "python3": true, "python2": true,
		"perl": true, "ruby": true, "node": true, "php": true,
	}
	return interpreters[prog]
}

func isSensitivePath(path string) bool {
	sensitive := []string{
		"/etc/passwd", "/etc/shadow", "/etc/group", "/etc/sudoers",
		"authorized_keys", "known_hosts", "id_rsa", "id_ed25519",
	}

	lower := strings.ToLower(path)
	for _, s := range sensitive {
		if strings.Contains(lower, s) {
			// Additional check to ensure it's a full path component or exact match
			if strings.HasSuffix(lower, "/"+s) || lower == s {
				return true
			}
		}
	}
	return false
}

// safeStdlibModules is the set of Python standard-library top-level package names
// that are considered safe for inline (-c) execution without manual review.
//
// Inclusion criterion: pure computation with no direct file-system writes, no network
// I/O, and no subprocess spawning. Modules that expose system-call wrappers (os,
// subprocess, socket, ctypes, multiprocessing, pty) are intentionally excluded even
// though many of their sub-functions are safe — the risk of os.system/os.popen/
// socket.connect makes the entire top-level package ineligible.
//
// In addition to this list, SafePythonImportsOnly in CommandCriteria also checks for
// banned builtin call patterns (eval, exec, open, __import__, compile).
var safeStdlibModules = map[string]bool{
	// Data & text processing
	"json": true, "re": true, "csv": true, "xml": true, "html": true, "glob": true,
	// Filesystem — read-only subset allowed; write methods are blocked by bannedInlinePythonPatterns.
	// pathlib.Path arithmetic, .exists(), .read_text(), .iterdir(), .glob() etc. are safe.
	// pathlib.Path.write_text(), .unlink(), .mkdir() etc. are blocked below.
	"pathlib": true,
	"string":  true, "textwrap": true, "pprint": true, "struct": true,
	"codecs": true, "unicodedata": true, "io": true, "difflib": true,
	// Math & numerics
	"math": true, "cmath": true, "decimal": true, "fractions": true,
	"numbers": true, "statistics": true, "random": true,
	// Data structures & algorithms
	"collections": true, "heapq": true, "bisect": true, "array": true,
	"queue": true, "itertools": true, "functools": true, "operator": true,
	// Hashing & encoding (no network)
	"hashlib": true, "hmac": true, "base64": true, "binascii": true, "secrets": true,
	// Type system & introspection (no side effects)
	"typing": true, "types": true, "abc": true, "dataclasses": true,
	"enum": true, "copy": true, "contextlib": true,
	// Parsing (no execution side-effects)
	"ast": true, "tokenize": true, "keyword": true, "token": true,
	// Time (read-only)
	"datetime": true, "time": true, "calendar": true,
	// System info readable by any process (no write/exec access)
	"sys": true,
	// Logging & diagnostics (no network)
	"logging": true, "warnings": true, "traceback": true,
	// CLI argument parsing (no I/O side effects)
	"argparse": true, "shlex": true,
	// Formatting
	"uuid": true, "locale": true,
}

// bannedInlinePythonPatterns lists substrings that, if present in a python -c command,
// indicate dangerous behaviour regardless of which modules are imported.
//
// Two categories:
//  1. Dangerous builtins: can execute arbitrary code or open files without an import.
//  2. Filesystem write methods: Path/io methods that mutate the filesystem; read
//     methods (read_text, iterdir, exists, glob, …) are safe and excluded.
var bannedInlinePythonPatterns = []string{
	// Dangerous builtins
	"eval(", "exec(", "open(", "__import__(", "compile(",
	// pathlib.Path write methods
	".write_text(", ".write_bytes(",
	".unlink(", ".rmdir(", ".mkdir(",
	".touch(", ".rename(", ".replace(", // .replace() is Path rename, not str.replace
	".symlink_to(", ".hardlink_to(", ".link_to(",
	".chmod(", ".chown(",
	// open() with write/append modes (catches pathlib .open('w') and io.open)
	".open('w'", ".open('a'", ".open('wb'", ".open('ab'",
	`.open("w"`, `.open("a"`, `.open("wb"`, `.open("ab"`,
	// io write wrappers
	"io.FileIO(", "io.open(", "io.TextIOWrapper(",
}

// topLevelPackage returns the top-level package name from a potentially dotted module path.
// "os.path" → "os", "pathlib" → "pathlib", "" → "".
func topLevelPackage(module string) string {
	module = strings.TrimSpace(module)
	// Reject anything with spaces (malformed or not a module name).
	if module == "" || strings.ContainsAny(module, " \t\n") {
		return ""
	}
	if dot := strings.IndexByte(module, '.'); dot >= 0 {
		return module[:dot]
	}
	return module
}
