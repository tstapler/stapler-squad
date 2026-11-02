package tcc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/tstapler/stapler-squad/log"
	"sort"
	"sync"
	"syscall"
)

var ErrUnknownRoot = errors.New("unknown protected root ID")

// ConsentRoot describes app consent, never an authoritative macOS grant.
type ConsentRoot struct {
	ID        string
	Root      string
	Service   Kind
	Paths     []string
	Consented bool
	// Denied is sticky: set when a probe returns EPERM, cleared only by Recheck.
	Denied bool
}

// ConsentGate parks enrichment until the user agrees to its filesystem access.
// Ready may do filesystem I/O; run it only inside the enricher's probe boundary.
type ConsentGate struct {
	classify      func(string) Result
	persist       func(string) error
	mu            sync.Mutex
	roots         map[string]ConsentRoot
	consent       map[string]bool
	denied        map[string]bool
	persistDenied func(id string, denied bool) error
	pending       []deniedOp
	draining      bool
}

func NewConsentGate(classify func(string) Result, consent map[string]bool, persist func(string) error) *ConsentGate {
	g := &ConsentGate{classify: classify, persist: persist, roots: map[string]ConsentRoot{}, consent: map[string]bool{}, denied: map[string]bool{}}
	for id, allowed := range consent {
		g.consent[id] = allowed
	}
	return g
}

func (g *ConsentGate) Ready(path string) bool {
	return g.ReadyDependency(Dependency{Path: path, EnrichmentKey: path})
}

type Dependency struct {
	Path          string
	EnrichmentKey string
}

func (g *ConsentGate) ReadyDependency(dependency Dependency) bool {
	path := dependency.Path
	result := g.classify(path)
	if !result.Protected {
		return true
	}
	sum := sha256.Sum256([]byte(result.Root))
	id := hex.EncodeToString(sum[:])
	g.mu.Lock()
	defer g.mu.Unlock()
	root := g.roots[id]
	root.ID, root.Root, root.Service, root.Consented, root.Denied = id, result.Root, result.Service, g.consent[id], g.denied[id]
	found := false
	for _, p := range root.Paths {
		if p == dependency.EnrichmentKey {
			found = true
			break
		}
	}
	if !found {
		root.Paths = append(root.Paths, dependency.EnrichmentKey)
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
	alreadyConsented := g.consent[id]
	g.mu.Unlock()
	if !known {
		return nil, ErrUnknownRoot
	}
	if alreadyConsented {
		return nil, nil
	}
	if g.persist != nil {
		if err := g.persist(id); err != nil {
			return nil, err
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.consent[id] {
		return nil, nil
	}
	g.consent[id] = true
	root := g.roots[id]
	root.Consented = true
	g.roots[id] = root
	return append([]string(nil), root.Paths...), nil
}

// SetDeniedStore restores persisted denied roots and sets the writer used when
// that state changes. Call before serving requests.
func (g *ConsentGate) SetDeniedStore(initial map[string]bool, persist func(id string, denied bool) error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, d := range initial {
		g.denied[id] = d
	}
	g.persistDenied = persist
}

// IsTCCDenial reports whether err looks like a TCC denial. TCC returns EPERM,
// while ordinary file permissions return EACCES. EPERM can also come from
// immutable flags or MDM/SIP, so this is a hint, never proof of a TCC denial.
func IsTCCDenial(err error) bool { return errors.Is(err, syscall.EPERM) }

// ObserveResult marks the root holding enrichmentKey denied when err is EPERM.
// Only roots already classified as protected by Ready are known here.
func (g *ConsentGate) ObserveResult(enrichmentKey string, err error) {
	if !IsTCCDenial(err) {
		return
	}
	g.mu.Lock()
	var ids []string
	for id, root := range g.roots {
		for _, p := range root.Paths {
			if p == enrichmentKey && !g.denied[id] {
				ids = append(ids, id)
			}
		}
	}
	for _, id := range ids {
		g.denied[id] = true
		root := g.roots[id]
		root.Denied = true
		g.roots[id] = root
		g.enqueuePersistLocked(id, true)
	}
	g.mu.Unlock()
}

type deniedOp struct {
	id     string
	denied bool
}

// enqueuePersistLocked queues a config write behind a single drain goroutine:
// writes stay in state-change order (persist(true) cannot land after
// persist(false)) and config.UpdateConfig disk I/O stays off the enricher worker.
func (g *ConsentGate) enqueuePersistLocked(id string, denied bool) {
	if g.persistDenied == nil {
		return
	}
	g.pending = append(g.pending, deniedOp{id, denied})
	if !g.draining {
		g.draining = true
		go g.drainPersist()
	}
}

func (g *ConsentGate) drainPersist() {
	for {
		g.mu.Lock()
		if len(g.pending) == 0 {
			g.draining = false
			g.mu.Unlock()
			return
		}
		op := g.pending[0]
		g.pending = g.pending[1:]
		persist := g.persistDenied
		g.mu.Unlock()
		if err := persist(op.id, op.denied); err != nil {
			log.Warn("tcc: failed to persist denied state; it will not survive a restart", "root_id", op.id, "denied", op.denied, "err", err)
		}
	}
}

// Recheck clears a sticky denial and returns the root's paths to re-probe, or
// nil when the root was not denied.
func (g *ConsentGate) Recheck(id string) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	root, known := g.roots[id]
	if !known {
		return nil, ErrUnknownRoot
	}
	if !g.denied[id] {
		return nil, nil
	}
	delete(g.denied, id)
	root.Denied = false
	g.roots[id] = root
	g.enqueuePersistLocked(id, false)
	return append([]string(nil), root.Paths...), nil
}
