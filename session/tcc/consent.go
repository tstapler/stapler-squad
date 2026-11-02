package tcc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
)

var ErrUnknownRoot = errors.New("unknown protected root ID")

// ConsentRoot describes app consent, never an authoritative macOS grant.
type ConsentRoot struct {
	ID        string
	Root      string
	Service   Kind
	Paths     []string
	Consented bool
}

// ConsentGate parks enrichment until the user agrees to its filesystem access.
// Ready may do filesystem I/O; run it only inside the enricher's probe boundary.
type ConsentGate struct {
	classify func(string) Result
	persist  func(string) error
	mu       sync.Mutex
	roots    map[string]ConsentRoot
	consent  map[string]bool
}

func NewConsentGate(classify func(string) Result, consent map[string]bool, persist func(string) error) *ConsentGate {
	g := &ConsentGate{classify: classify, persist: persist, roots: map[string]ConsentRoot{}, consent: map[string]bool{}}
	for id, allowed := range consent {
		g.consent[id] = allowed
	}
	return g
}

func (g *ConsentGate) Ready(path string) bool {
	result := g.classify(path)
	if !result.Protected {
		return true
	}
	sum := sha256.Sum256([]byte(result.Root))
	id := hex.EncodeToString(sum[:])
	g.mu.Lock()
	defer g.mu.Unlock()
	root := g.roots[id]
	root.ID, root.Root, root.Service, root.Consented = id, result.Root, result.Service, g.consent[id]
	found := false
	for _, p := range root.Paths {
		if p == path {
			found = true
			break
		}
	}
	if !found {
		root.Paths = append(root.Paths, path)
	}
	g.roots[id] = root
	return root.Consented
}

func (g *ConsentGate) Roots() []ConsentRoot {
	g.mu.Lock()
	defer g.mu.Unlock()
	roots := make([]ConsentRoot, 0, len(g.roots))
	for _, root := range g.roots {
		root.Paths = append([]string(nil), root.Paths...)
		sort.Strings(root.Paths)
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].ID < roots[j].ID })
	return roots
}

// Continue accepts only a root discovered by Ready. Persistence must succeed
// before any caller can observe consent and unpark its enrichment paths.
func (g *ConsentGate) Continue(id string) ([]string, error) {
	g.mu.Lock()
	_, known := g.roots[id]
	g.mu.Unlock()
	if !known {
		return nil, ErrUnknownRoot
	}
	if g.persist != nil {
		if err := g.persist(id); err != nil {
			return nil, err
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.consent[id] = true
	root := g.roots[id]
	root.Consented = true
	g.roots[id] = root
	return append([]string(nil), root.Paths...), nil
}
