// Command checkformula asserts that a goreleaser-rendered Homebrew formula agrees with the
// release assets of the same snapshot: every url names an archive that exists in dist, every
// sha256 equals both checksums.txt and the archive's real digest, and every darwin/linux
// archive is referenced by the formula. Exit 0 means the formula resolves its artifacts.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	urlRe    = regexp.MustCompile(`^\s*url\s+"([^"]+)"`)
	sha256Re = regexp.MustCompile(`^\s*sha256\s+"([0-9a-f]{64})"`)
)

type asset struct{ name, sha string }

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("checkformula", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dist := fs.String("dist", "dist", "goreleaser dist directory holding the archives and checksums.txt")
	formula := fs.String("formula", "", "formula to check (default <dist>/homebrew/Formula/stapler-squad.rb)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *formula == "" {
		*formula = filepath.Join(*dist, "homebrew", "Formula", "stapler-squad.rb")
	}
	problems, n, err := check(*dist, *formula)
	if err != nil {
		fmt.Fprintln(stderr, "checkformula:", err)
		return 2
	}
	for _, p := range problems {
		fmt.Fprintln(stderr, "checkformula: FAIL:", p)
	}
	if len(problems) > 0 {
		return 1
	}
	fmt.Fprintf(stdout, "checkformula: %d formula assets match %s\n", n, *dist)
	return 0
}

// check returns the mismatches between formula and dist, and how many formula assets it checked.
func check(dist, formulaPath string) ([]string, int, error) {
	f, err := os.Open(formulaPath)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	assets, problems := parseFormula(f)

	sums, err := readChecksums(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		return nil, 0, err
	}
	if len(assets) == 0 {
		problems = append(problems, "formula has no url/sha256 pairs")
	}
	seen := map[string]bool{}
	for _, a := range assets {
		seen[a.name] = true
		want, ok := sums[a.name]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: not listed in checksums.txt", a.name))
			continue
		case want != a.sha:
			problems = append(problems, fmt.Sprintf("%s: formula sha256 %s != checksums.txt %s", a.name, a.sha, want))
		}
		got, err := fileSHA(filepath.Join(dist, a.name))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", a.name, err))
		} else if got != a.sha {
			problems = append(problems, fmt.Sprintf("%s: formula sha256 %s != archive digest %s", a.name, a.sha, got))
		}
	}
	for name := range sums {
		if brewPlatform(name) && !seen[name] {
			problems = append(problems, fmt.Sprintf("%s: release asset missing from the formula", name))
		}
	}
	return problems, len(assets), nil
}

// brewPlatform reports whether goreleaser's brews publisher consumes the archive (darwin/linux tar.gz).
func brewPlatform(name string) bool {
	return strings.HasSuffix(name, ".tar.gz") && (strings.Contains(name, "_darwin_") || strings.Contains(name, "_linux_"))
}

func parseFormula(r io.Reader) ([]asset, []string) {
	var assets []asset
	var problems []string
	pending := ""
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if m := urlRe.FindStringSubmatch(line); m != nil {
			if pending != "" {
				problems = append(problems, fmt.Sprintf("%s: url has no sha256", pending))
			}
			u, err := url.Parse(m[1])
			if err != nil || !strings.Contains(u.Path, "/releases/download/") {
				problems = append(problems, fmt.Sprintf("url %q is not a GitHub release download", m[1]))
			}
			pending = path.Base(m[1])
		} else if m := sha256Re.FindStringSubmatch(line); m != nil {
			if pending == "" {
				problems = append(problems, "sha256 "+m[1]+" has no preceding url")
				continue
			}
			assets = append(assets, asset{pending, m[1]})
			pending = ""
		}
	}
	if pending != "" {
		problems = append(problems, fmt.Sprintf("%s: url has no sha256", pending))
	}
	return assets, problems
}

func readChecksums(p string) (map[string]string, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sums := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) == 2 {
			sums[fields[1]] = fields[0]
		}
	}
	return sums, sc.Err()
}

func fileSHA(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
