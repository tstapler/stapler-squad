// Package check applies the docs-coverage ratchet to a registry, its sidecars
// and the resolved documentation (ADR-005).
package check

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/changed"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/registry"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/resolve"
)

// Budgets bound `exempt` use (tools/docscoverage/budgets.json).
type Budgets struct {
	ExemptRatioCeiling float64 `json:"exemptRatioCeiling"`
	MaxExemptIncrease  int     `json:"maxExemptIncrease"`
}

// Base is the registry and sidecars at the base commit.
type Base struct {
	Registry registry.Registry
	Sidecars registry.Sidecars
}

// Input is everything Assess needs. Base is nil when the base is unresolved.
type Input struct {
	Registry registry.Registry
	Sidecars registry.Sidecars
	Resolver *resolve.Resolver
	Budgets  Budgets
	Base     *Base
}

// Entry is one in-scope feature's coverage.
type Entry struct {
	ID      model.FeatureID
	Status  model.Status
	Missing []string // fields to add, for a gap
	Change  changed.Kind
	Notes   []string
}

// Result is the outcome of Assess.
type Result struct {
	Entries    []Entry
	Violations []string // fail the gate when enforcing
	Warnings   []string
	Counts     Counts
}

// Counts are shown next to each other, never collapsed into a percentage.
type Counts struct {
	InScope, Covered, Exempt, Grandfathered, Gap int
}

var githubLogin = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)

// Assess evaluates every in-scope feature and every sidecar.
func Assess(in Input) Result {
	res := Result{Warnings: append([]string(nil), in.Registry.Warnings...)}
	res.Warnings = append(res.Warnings, in.Resolver.Warnings...)
	res.Violations = append(res.Violations, in.Resolver.Problems...)
	res.Violations = append(res.Violations, in.Sidecars.Problems...)

	var kinds map[model.FeatureID]changed.Kind
	if in.Base != nil {
		kinds = changed.Detect(in.Base.Registry, in.Registry)
	}
	for _, id := range in.Sidecars.IDs() {
		res.checkSidecar(in, id)
	}
	for _, id := range in.Registry.IDs() {
		f := in.Registry.Features[id]
		sc, hasSC := in.Sidecars.ByID[id]
		if !inScope(f, sc, hasSC) {
			continue
		}
		res.evaluate(in, f, sc, hasSC, kinds)
	}
	res.checkBudgets(in)
	res.checkTourBacklinks(in)
	return res
}

func inScope(f model.Feature, sc model.Sidecar, hasSC bool) bool {
	if f.UserFacing != nil || !f.InBackendDir {
		return f.DefaultUserFacing()
	}
	return hasSC && sc.UserFacing != nil && *sc.UserFacing
}

func (res *Result) violate(format string, args ...any) {
	res.Violations = append(res.Violations, fmt.Sprintf(format, args...))
}

// checkSidecar applies the rules that hold for any sidecar, in scope or not.
func (res *Result) checkSidecar(in Input, id model.FeatureID) {
	sc := in.Sidecars.ByID[id]
	if _, ok := in.Registry.Features[id]; !ok {
		res.violate("%s: orphan sidecar, id is in no registry file under %s", id, registry.FeaturesDir)
		return
	}
	if sc.Exempt != "" {
		if strings.TrimSpace(sc.Exempt) == "" || !githubLogin.MatchString(sc.Approver) {
			res.violate("%s: exempt needs a reason and an approver (a GitHub login)", id)
		}
		if sc.Help != "" || sc.Website != "" || sc.Tour != "" {
			res.violate("%s: exempt and help/website/tour are mutually exclusive", id)
		}
	} else if sc.Approver != "" {
		res.violate("%s: approver is only valid together with exempt", id)
	}
	if sc.Status != "" && sc.Status != model.Grandfathered {
		res.violate("%s: unknown status %q", id, sc.Status)
	}
	if sc.Status == model.Grandfathered && in.Base != nil {
		if _, ok := in.Base.Registry.Features[id]; !ok {
			res.violate("%s: grandfathered only valid for ids existing on base", id)
		}
	}
}

func (res *Result) evaluate(in Input, f model.Feature, sc model.Sidecar, hasSC bool, kinds map[model.FeatureID]changed.Kind) {
	e := Entry{ID: f.ID, Change: kinds[f.ID]}
	problems, missing, covered := docState(in.Resolver, f.ID, sc, hasSC)
	for _, p := range problems {
		res.violate("%s: %s", f.ID, p)
	}
	switch {
	case hasSC && strings.TrimSpace(sc.Exempt) != "":
		e.Status = model.StatusExempt
	case covered:
		e.Status = model.StatusCovered
		if sc.Status == model.Grandfathered {
			res.violate("%s: now has help and website; drop status \"grandfathered\" (baseline shrink)", f.ID)
		}
	case hasSC && sc.Status == model.Grandfathered:
		e.Status = model.StatusGrandfathered
		if e.Change == changed.Modified {
			e.Notes = append(e.Notes, "changed, still grandfathered")
		}
	default:
		e.Status = model.StatusGap
		e.Missing = missing
		if e.Change != "" {
			res.violate("%s: missing %s", f.ID, strings.Join(missing, ", "))
		}
	}
	res.Entries = append(res.Entries, e)
	res.Counts.InScope++
	switch e.Status {
	case model.StatusCovered:
		res.Counts.Covered++
	case model.StatusExempt:
		res.Counts.Exempt++
	case model.StatusGrandfathered:
		res.Counts.Grandfathered++
	case model.StatusGap:
		res.Counts.Gap++
	}
}

// docState resolves a sidecar's links. Covered means a resolved help and website
// pair, or a resolved tour and website pair (a tour page is its own website page).
func docState(r *resolve.Resolver, id model.FeatureID, sc model.Sidecar, hasSC bool) (problems, missing []string, covered bool) {
	if !hasSC {
		return nil, []string{"help", "website"}, false
	}
	var helpOK, webOK, tourOK bool
	if sc.Help != "" {
		p := r.CheckHelp(sc.Help)
		problems, helpOK = append(problems, p...), len(p) == 0
	}
	if sc.Website != "" {
		p := r.CheckWebsite(sc.Website)
		problems, webOK = append(problems, p...), len(p) == 0
	}
	if sc.Tour != "" {
		p := r.CheckTour(sc.Tour, id)
		problems, tourOK = append(problems, p...), len(p) == 0
	}
	covered = webOK && (helpOK || tourOK)
	if sc.Website == "" {
		missing = append(missing, "website")
	}
	if sc.Help == "" && sc.Tour == "" {
		missing = append([]string{"help"}, missing...)
	}
	return problems, missing, covered
}

func (res *Result) checkBudgets(in Input) {
	exempt := countExempt(in.Sidecars)
	if res.Counts.InScope > 0 && in.Budgets.ExemptRatioCeiling > 0 {
		if ratio := float64(exempt) / float64(res.Counts.InScope); ratio > in.Budgets.ExemptRatioCeiling {
			res.violate("exempt ratio %d/%d exceeds the ceiling %.2f in tools/docscoverage/budgets.json", exempt, res.Counts.InScope, in.Budgets.ExemptRatioCeiling)
		}
	}
	if in.Base == nil {
		return
	}
	if d := exempt - countExempt(in.Base.Sidecars); d > in.Budgets.MaxExemptIncrease {
		res.violate("this change raises the exempt count by %d (limit %d)", d, in.Budgets.MaxExemptIncrease)
	}
	// An empty base sidecar set is the bootstrap PR that creates the baseline.
	if len(in.Base.Sidecars.ByID) > 0 {
		if d := countGrandfathered(in.Sidecars) - countGrandfathered(in.Base.Sidecars); d > 0 {
			res.violate("this change raises the grandfathered count by %d; the baseline can only shrink", d)
		}
	}
}

func countExempt(s registry.Sidecars) (n int) {
	for _, sc := range s.ByID {
		if strings.TrimSpace(sc.Exempt) != "" {
			n++
		}
	}
	return n
}

func countGrandfathered(s registry.Sidecars) (n int) {
	for _, sc := range s.ByID {
		if sc.Status == model.Grandfathered {
			n++
		}
	}
	return n
}

// checkTourBacklinks warns when a tour lists a feature whose sidecar links to no
// tour that lists it. A sidecar holds one tour, so a feature listed by several
// tours is linked back by whichever of them its sidecar names.
func (res *Result) checkTourBacklinks(in Input) {
	var out []string
	for _, tid := range in.Resolver.TourIDs() {
		for _, fid := range in.Resolver.TourFeatures(tid) {
			sc, ok := in.Sidecars.ByID[model.FeatureID(fid)]
			if !ok || !in.Resolver.TourLists(sc.Tour, fid) {
				out = append(out, fmt.Sprintf("tour %s lists %s but that feature's sidecar has no tour: %q", tid, fid, tid))
			}
		}
	}
	sort.Strings(out)
	res.Warnings = append(res.Warnings, out...)
}
