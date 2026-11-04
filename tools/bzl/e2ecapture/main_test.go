package main

import (
	"reflect"
	"testing"
)

func TestExecutedPackagesKeepsOnlyRunStatements(t *testing.T) {
	out := "\tgithub.com/tstapler/stapler-squad/session\tcoverage: 12.5% of statements\n" +
		"\tgithub.com/tstapler/stapler-squad/server/services\tcoverage: 3.0% of statements\n" +
		"\tgithub.com/tstapler/stapler-squad/github\tcoverage: 0.0% of statements\n" +
		"\tgolang.org/x/sys\tcoverage: 50.0% of statements\n" +
		"noise line\n"
	want := []string{"github.com/tstapler/stapler-squad/server/services", "github.com/tstapler/stapler-squad/session"}
	if got := executedPackages(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFilterEnvDropsUserAgent(t *testing.T) {
	got := filterEnv([]string{"A=1", "npm_config_user_agent=npm/9", "B=2"}, "npm_config_user_agent")
	if !reflect.DeepEqual(got, []string{"A=1", "B=2"}) {
		t.Fatal(got)
	}
}
