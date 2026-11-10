package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type listPkg struct {
	ImportPath   string
	Dir          string
	GoFiles      []string
	TestGoFiles  []string
	XTestGoFiles []string
	Imports      []string
	TestImports  []string
	XTestImports []string
}

// decodeList parses the concatenated JSON objects `go list -json` prints.
func decodeList(r io.Reader) ([]listPkg, error) {
	dec := json.NewDecoder(r)
	var pkgs []listPkg
	for {
		var p listPkg
		if err := dec.Decode(&p); err == io.EOF {
			return pkgs, nil
		} else if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, p)
	}
}

func countLines(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return bytes.Count(b, []byte("\n"))
}

// BuildModel assembles the Model from `go list` output. `go list -e` tolerates the gitignored gen/ and session/ent
// packages being absent: their import edges are dropped by IsGenerated anyway.
func BuildModel(pkgs []listPkg) *Model {
	m := &Model{
		Lib: map[string]map[string]bool{}, Test: map[string]map[string]bool{}, HasTest: map[string]bool{},
		LOC: map[string]int{}, TestLOC: map[string]int{},
	}
	edges := func(ims []string, self string) map[string]bool {
		out := map[string]bool{}
		for _, i := range ims {
			if d := RelDir(i); inModule(i) && !IsGenerated(d) && d != self {
				out[d] = true
			}
		}
		return out
	}
	for _, p := range pkgs {
		d := RelDir(p.ImportPath)
		if !inModule(p.ImportPath) || IsGenerated(d) {
			continue
		}
		m.Lib[d] = edges(p.Imports, d)
		t := edges(append(append([]string{}, p.TestImports...), p.XTestImports...), d)
		m.Test[d] = t
		if len(p.TestGoFiles)+len(p.XTestGoFiles) > 0 {
			m.HasTest[d] = true
		}
		for _, f := range p.GoFiles {
			m.LOC[d] += countLines(filepath.Join(p.Dir, f))
		}
		for _, f := range append(append([]string{}, p.TestGoFiles...), p.XTestGoFiles...) {
			m.TestLOC[d] += countLines(filepath.Join(p.Dir, f))
		}
	}
	return m
}

// LoadModel runs `go list -e -json ./...` in root.
func LoadModel(root string) (*Model, error) {
	cmd := exec.Command("go", "list", "-e", "-json", "./...") //nolint:norawexec short-lived offline CLI; Run waits for exit
	cmd.Dir = root
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list: %w: %s", err, strings.TrimSpace(errb.String()))
	}
	pkgs, err := decodeList(&out)
	if err != nil {
		return nil, err
	}
	return BuildModel(pkgs), nil
}
