package artifacts

import "strings"

// shellFields splits a command line into words, honoring single and double
// quotes. It is a best-effort tokenizer for reading flags out of a logged
// command, not a shell: no escapes beyond quotes, no expansion.
func shellFields(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	inWord := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

// flagValue returns the value of the first of names (e.g. "--base", "-B") in
// args, accepting both "--base v" and "--base=v".
func flagValue(args []string, names ...string) string {
	for i, a := range args {
		for _, n := range names {
			if a == n && i+1 < len(args) {
				return args[i+1]
			}
			if strings.HasPrefix(a, n+"=") {
				return strings.TrimPrefix(a, n+"=")
			}
		}
	}
	return ""
}

// ghSubcommand reports whether args contain the consecutive words "gh", sub...
// and returns the args after them.
func ghSubcommand(args []string, sub ...string) ([]string, bool) {
	for i := range args {
		if args[i] != "gh" || i+len(sub) >= len(args) {
			continue
		}
		match := true
		for j, w := range sub {
			if args[i+1+j] != w {
				match = false
				break
			}
		}
		if match {
			return args[i+1+len(sub):], true
		}
	}
	return nil, false
}
