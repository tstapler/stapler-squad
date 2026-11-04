// Command testsummary prints the sorted passing test names of a run, one "<import path>/<Test>" per
// line, from either Bazel test.xml files or a `gotestsum --jsonfile` stream, so test-set-diff.sh can
// compare the two worlds with diff(1).
//
//	testsummary -testlogs <bazel-testlogs dir> [-module <path>] [-failed]
//	testsummary -gojson <file>                  [-failed]
//
// Bazel names are keyed by the target's package directory under the testlogs tree, prefixed with
// -module (default github.com/tstapler/stapler-squad), matching the Package field of go test JSON.
package main

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type testsuites struct {
	Suites []struct {
		Cases []struct {
			Name    string    `xml:"name,attr"`
			Failure *struct{} `xml:"failure"`
			Error   *struct{} `xml:"error"`
			Skipped *struct{} `xml:"skipped"`
		} `xml:"testcase"`
	} `xml:"testsuite"`
	Cases []struct {
		Name    string    `xml:"name,attr"`
		Failure *struct{} `xml:"failure"`
		Error   *struct{} `xml:"error"`
		Skipped *struct{} `xml:"skipped"`
	} `xml:"testcase"`
}

// parseXML returns the names of passed and failed cases in one test.xml (skipped cases are neither).
func parseXML(r io.Reader) (passed, failed []string, err error) {
	var doc testsuites
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, nil, err
	}
	add := func(name string, bad, skipped bool) {
		switch {
		case skipped:
		case bad:
			failed = append(failed, name)
		default:
			passed = append(passed, name)
		}
	}
	for _, s := range doc.Suites {
		for _, c := range s.Cases {
			add(c.Name, c.Failure != nil || c.Error != nil, c.Skipped != nil)
		}
	}
	for _, c := range doc.Cases {
		add(c.Name, c.Failure != nil || c.Error != nil, c.Skipped != nil)
	}
	return passed, failed, nil
}

// packageDir maps <root>/<pkg>/<target>[/shard_i_of_n]/test.xml to <pkg>.
func packageDir(root, xmlPath string) string {
	rel, _ := filepath.Rel(root, filepath.Dir(xmlPath))
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if n := len(parts); n > 0 && strings.HasPrefix(parts[n-1], "shard_") {
		parts = parts[:n-1]
	}
	if len(parts) > 0 {
		parts = parts[:len(parts)-1] // drop the target directory
	}
	return strings.Join(parts, "/")
}

func fromTestlogs(root, module string, wantFailed bool) ([]string, error) {
	set := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "test.xml" {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		passed, failed, err := parseXML(f)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		names := passed
		if wantFailed {
			names = failed
		}
		pkg := module
		if dir := packageDir(root, p); dir != "" {
			pkg = module + "/" + dir
		}
		for _, n := range names {
			set[pkg+"/"+n] = true
		}
		return nil
	})
	return sorted(set), err
}

func fromGoJSON(r io.Reader, wantFailed bool) ([]string, error) {
	want := "pass"
	if wantFailed {
		want = "fail"
	}
	set := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var ev struct{ Action, Package, Test string }
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Test == "" || ev.Action != want {
			continue
		}
		set[ev.Package+"/"+ev.Test] = true
	}
	return sorted(set), sc.Err()
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func main() {
	testlogs := flag.String("testlogs", "", "bazel-testlogs directory")
	gojson := flag.String("gojson", "", "gotestsum --jsonfile file")
	module := flag.String("module", "github.com/tstapler/stapler-squad", "Go module path prefixed to Bazel package dirs")
	failed := flag.Bool("failed", false, "print failing names instead of passing names")
	flag.Parse()
	var names []string
	var err error
	switch {
	case *testlogs != "" && *gojson == "":
		names, err = fromTestlogs(*testlogs, *module, *failed)
	case *gojson != "" && *testlogs == "":
		var f *os.File
		if f, err = os.Open(*gojson); err == nil {
			defer f.Close()
			names, err = fromGoJSON(f, *failed)
		}
	default:
		err = fmt.Errorf("exactly one of -testlogs or -gojson is required")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "testsummary:", err)
		os.Exit(1)
	}
	w := bufio.NewWriter(os.Stdout)
	for _, n := range names {
		fmt.Fprintln(w, n)
	}
	w.Flush()
}
