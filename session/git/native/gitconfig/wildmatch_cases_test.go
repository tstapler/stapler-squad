package gitconfig

// wildCase is a (pattern, text) row with git's verified answer under WM_PATHNAME (plan c4).
type wildCase struct {
	pattern, text string
	want          bool
}

func wildCases() []wildCase {
	return []wildCase{
		{"https://h/Foo/**", "https://h/Foo/", true},
		{"https://h/Foo/**", "https://h/Foo", false},
		{"**/repo", "repo", true},
		{"https://h/*/repo", "https://h/a/b/repo", false},
		{"https://h/*/repo", "https://h/a/repo", true},
		{"https://h/**/repo", "https://h/repo", true},
		{"https://h/**/repo", "https://h/a/b/repo", true},
		{"https://h/a**b/repo", "https://h/aX/Yb/repo", false},
		{"https://h/a**b/repo", "https://h/aXYb/repo", true},
		{"https://h/?/repo", "https://h///repo", false},
		{"https://h/?/repo", "https://h/x/repo", true},
		{"*", "https://h/x", false},
		{"**", "https://h/x", true},
		{"https://H/**", "https://h/x", false},
		{"", "", true},
		{"", "a", false},
		{"a", "", false},
		{"*", "", true},
		{"a*", "a", true},
		{"*a", "ba", true},
		{"a?c", "abc", true},
		{"a?c", "a/c", false},
		{"/**", "/", true},
		{"/**", "/a/b", true},
		{"**/", "a/", true},
		{"x/**/y", "x/y", true},
		{"x/**/y", "x/a/b/y", true},
		{"x/**/y", "x/ay", false},
		{"https://**.corp.example/**", "https://git.corp.example/org/repo", true},
		{"https://**.corp.example:*/**", "https://git.corp.example:8443/org/repo", true},
		{"git@host.example:team/**", "git@host.example:team/x.git", true},
		{"https://*.example/x", "https://a/b.example/x", false},
	}
}
