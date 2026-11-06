package gitconfig

// tokenizerCase is one row of the tokenizer contract. wantErr rows are ones where git dies
// with "bad config line" (git config -f f.cfg --list exits non-zero).
type tokenizerCase struct {
	name    string
	content string
	// want is the expected listing: "name=value" for a valued entry, "name" for a bare key.
	want    []string
	wantErr bool
}

func tokenizerCases() []tokenizerCase {
	return []tokenizerCase{
		{name: "empty file", content: ""},
		{name: "plain entries", content: "[core]\n\tbare = false\n\teditor = vim\n", want: []string{"core.bare=false", "core.editor=vim"}},
		{name: "section and key case", content: "[Core]\n\tHooksPath = /a\n[CORE]\n\thookspath = /b\n", want: []string{"core.hookspath=/a", "core.hookspath=/b"}},
		{name: "subsection case is kept", content: "[remote \"Origin\"]\n\tURL = u\n", want: []string{"remote.Origin.url=u"}},
		{name: "subsection escapes", content: "[a \"x\\\"y\\\\z\\q\"]\n\tk = v\n", want: []string{"a.x\"y\\zq.k=v"}},
		{name: "deprecated dotted subsection is lowercased", content: "[a.Foo]\n\tk = v\n", want: []string{"a.foo.k=v"}},
		{name: "empty subsection is present", content: "[a \"\"]\n\tk = v\n", want: []string{"a..k=v"}},
		{name: "entry on the header line", content: "[a] k = 1\n", want: []string{"a.k=1"}},
		{name: "comment after header", content: "[a] ; note\nk = 1\n", want: []string{"a.k=1"}},
		{name: "semicolon and hash comments", content: "[a]\n; whole line\n# another\nk = 1 ; trailing\nj = 2 # trailing\n", want: []string{"a.k=1", "a.j=2"}},
		{name: "comment markers inside quotes", content: "[a]\nk = \"x ; y\" # c\nj = \"p # q\"\n", want: []string{"a.k=x ; y", "a.j=p # q"}},
		{name: "bare key is true, key = is empty", content: "[a]\nbare\nempty =\nquoted = \"\"\n", want: []string{"a.bare", "a.empty=", "a.quoted="}},
		{name: "value whitespace", content: "[a]\nk =   x   y\t z  \n", want: []string{"a.k=x   y\t z"}},
		{name: "quoted whitespace is kept", content: "[a]\nk = \"  x  \"\n", want: []string{"a.k=  x  "}},
		{name: "escapes", content: "[a]\nk = a\\tb\\nc\\\\d\\\"e\\b\n", want: []string{"a.k=a\tb\nc\\d\"e\b"}},
		{name: "bad escape", content: "[a]\nk = a\\qb\n", wantErr: true},
		{name: "unquoted backslash continuation", content: "[a]\nk = x\\\n  y\n", want: []string{"a.k=x  y"}},
		{name: "quoted backslash continuation", content: "[a]\nk = \"x\\\n  y\"\n", want: []string{"a.k=x  y"}},
		{name: "unterminated quote", content: "[a]\nk = \"x\n", wantErr: true},
		{name: "CRLF line ends", content: "[a]\r\nk = 1\r\nj = \"2\"\r\n", want: []string{"a.k=1", "a.j=2"}},
		{name: "CRLF continuation", content: "[a]\r\nk = x\\\r\ny\r\n", want: []string{"a.k=xy"}},
		{name: "UTF-8 BOM", content: "\xef\xbb\xbf[a]\nk = 1\n", want: []string{"a.k=1"}},
		{name: "partial BOM", content: "\xef\xbbX[a]\nk = 1\n", wantErr: true},
		{name: "no final newline", content: "[a]\nk = 1", want: []string{"a.k=1"}},
		{name: "bare key without final newline", content: "[a]\nk", want: []string{"a.k"}},
		{name: "key before any section", content: "k = 1\n[a]\nj = 2\n", want: []string{"k=1", "a.j=2"}},
		{name: "key with digit and dash", content: "[a]\nk-1 = v\n", want: []string{"a.k-1=v"}},
		{name: "key starting with a digit", content: "[a]\n1k = v\n", wantErr: true},
		{name: "section with spaces", content: "[ a ]\nk = 1\n", wantErr: true},
		{name: "section with trailing space", content: "[a ]\nk = 1\n", wantErr: true},
		{name: "unterminated header", content: "[a\nk = 1\n", wantErr: true},
		{name: "subsection without closing bracket", content: "[a \"x\"\nk = 1\n", wantErr: true},
		{name: "subsection junk after quote", content: "[a \"x\" y]\nk = 1\n", wantErr: true},
		{name: "key then junk", content: "[a]\nk.j = 1\n", wantErr: true},
		{name: "key without equals then word", content: "[a]\nk v\n", wantErr: true},
		{name: "include between same section", content: "[core]\nhooksPath = /a\n[include]\npath = x\n[core]\nhooksPath = /b\n",
			want: []string{"core.hookspath=/a", "include.path=x", "core.hookspath=/b"}},
		{name: "multi-valued key", content: "[a]\nk = 1\nk = 2\n", want: []string{"a.k=1", "a.k=2"}},
		{name: "equals inside value", content: "[a]\nk = x=y\n", want: []string{"a.k=x=y"}},
		{name: "tab before equals", content: "[a]\nk\t=\tv\n", want: []string{"a.k=v"}},
		{name: "unquoted trailing backslash then comment", content: "[a]\nk = x\\\n; c\n", want: []string{"a.k=x"}},
	}
}
