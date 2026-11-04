package main

import "testing"

func TestCheck(t *testing.T) {
	const head = "07c642d73aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cases := map[string]bool{
		"stapler-squad version dev\n  branch: x  commit: 07c642d73aaa\n": true,
		"stapler-squad version 07c642d73aaa\n":                           true,
		"stapler-squad version 07c642d73aaa-dirty\n":                     true,
		"stapler-squad version dev\n  branch: x  commit: deadbeef1234\n": false,
		"stapler-squad version dev\n":                                    false,
		"":                                                               false,
	}
	for out, ok := range cases {
		if (check(out, head) == nil) != ok {
			t.Errorf("%q: ok=%v want %v", out, check(out, head) == nil, ok)
		}
	}
}
