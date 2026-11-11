package scenario

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const maxRouteFixturesPerScenario = 2

var (
	protoPackageRE = regexp.MustCompile(`(?m)^package\s+([\w.]+);`)
	protoServiceRE = regexp.MustCompile(`^service\s+(\w+)\s*\{`)
	protoRPCRE     = regexp.MustCompile(`^\s*rpc\s+(\w+)\s*\(`)
)

// protoRoutes lists every "/api/<package>.<Service>/<Method>" route declared in
// proto/session/v1. Generated message types are gitignored and absent without
// `make build`, so the declared RPC surface is checked from the .proto text.
func protoRoutes(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("../../../proto/session/v1/*.proto")
	if err != nil || len(files) == 0 {
		t.Fatalf("proto files not found: %v", err)
	}
	routes := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		m := protoPackageRE.FindSubmatch(data)
		if m == nil {
			continue
		}
		service := ""
		for _, line := range strings.Split(string(data), "\n") {
			if s := protoServiceRE.FindStringSubmatch(line); s != nil {
				service = s[1]
			} else if r := protoRPCRE.FindStringSubmatch(line); r != nil && service != "" {
				routes["/api/"+string(m[1])+"."+service+"/"+r[1]] = true
			}
		}
	}
	return routes
}

func TestRouteFixturesNameDeclaredRPCsAndStayWithinCap(t *testing.T) {
	t.Parallel()
	declared := protoRoutes(t)
	for _, n := range Names() {
		if n == DefaultName {
			continue
		}
		b, err := BuildBundle(n, refA())
		if err != nil {
			t.Fatal(err)
		}
		if len(b.Routes) > maxRouteFixturesPerScenario {
			t.Errorf("%s: %d route fixtures, cap is %d per tour", n, len(b.Routes), maxRouteFixturesPerScenario)
		}
		for _, r := range b.Routes {
			if !declared[r.Route] {
				t.Errorf("%s/%s: route %q is not an RPC declared in proto/session/v1", n, r.ID, r.Route)
			}
		}
	}
}

func TestRouteFixturesHaveWellFormedJSONRows(t *testing.T) {
	t.Parallel()
	for _, n := range Names() {
		if n == DefaultName {
			continue
		}
		b, _ := BuildBundle(n, refA())
		for _, r := range b.Routes {
			raw, err := json.Marshal(r)
			if err != nil {
				t.Fatalf("%s/%s: marshal: %v", n, r.ID, err)
			}
			var back struct {
				ID, Route, Seeded string
				Rows              []Row
			}
			if err := json.Unmarshal(raw, &back); err != nil || len(back.Rows) == 0 {
				t.Errorf("%s/%s: not round-trippable JSON with rows (err %v)", n, r.ID, err)
			}
			for _, row := range r.Rows {
				if row.ID == "" || row.Kind == "" || row.Title == "" {
					t.Errorf("%s/%s: row %+v needs id, kind and title", n, r.ID, row)
				}
			}
		}
	}
}
