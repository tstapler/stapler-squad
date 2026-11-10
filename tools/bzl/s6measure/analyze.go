package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Percentile is the nearest-rank percentile (p in 0..100) of xs, -1 when empty.
func Percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return -1
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	rank := int(math.Ceil(p / 100 * float64(len(s))))
	return s[min(max(rank, 1), len(s))-1]
}

// Spawn is the subset of a SpawnExec record in --execution_log_json_file output that the checks read.
type Spawn struct {
	TargetLabel   string   `json:"targetLabel"`
	Mnemonic      string   `json:"mnemonic"`
	Runner        string   `json:"runner"`
	CacheHit      bool     `json:"cacheHit"`
	Cacheable     bool     `json:"cacheable"`
	ListedOutputs []string `json:"listedOutputs"`
}

// ParseSpawns reads newline-delimited JSON spawn records, skipping lines that are not JSON objects.
func ParseSpawns(r io.Reader) []Spawn {
	var out []Spawn
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var s Spawn
		if json.Unmarshal(sc.Bytes(), &s) == nil && (s.Mnemonic != "" || s.Runner != "") {
			out = append(out, s)
		}
	}
	return out
}

func isHit(s Spawn) bool {
	return s.CacheHit || s.Runner == "disk cache hit" || s.Runner == "remote cache hit"
}

// executed reports a spawn that really ran an action (not a hit, not an internal symlink action).
func executed(s Spawn) bool { return !isHit(s) && s.Runner != "internal" && s.Runner != "" }

func (s Spawn) onAllowList(allow []string) bool {
	hay := strings.ToLower(s.TargetLabel + " " + s.Mnemonic + " " + strings.Join(s.ListedOutputs, " "))
	for _, a := range allow {
		if a != "" && strings.Contains(hay, strings.ToLower(a)) {
			return true
		}
	}
	return false
}

// SpawnStats is the E1/E2 view of one build's execution log.
type SpawnStats struct {
	Cacheable, Hits    int
	NonAllowMisses     []Spawn // cacheable, not a hit, not on the allow-list
	NonAllowExecuted   int     // executed spawns not on the allow-list (E2 counts these)
	HitRatio           float64 // -1 when there were no cacheable spawns
	UniqueExecutedKeys int
	ExecutedTotal      int
}

// AnalyzeSpawns applies E1's definition: hit ratio = hits over cacheable spawns, internal symlink actions excluded.
func AnalyzeSpawns(sp []Spawn, allow []string) SpawnStats {
	st := SpawnStats{HitRatio: -1}
	uniq := map[string]bool{}
	for _, s := range sp {
		if s.Runner == "internal" {
			continue
		}
		if s.Cacheable {
			st.Cacheable++
			if isHit(s) {
				st.Hits++
			} else if !s.onAllowList(allow) {
				st.NonAllowMisses = append(st.NonAllowMisses, s)
			}
		}
		if executed(s) {
			st.ExecutedTotal++
			uniq[s.Mnemonic+"|"+s.TargetLabel+"|"+strings.Join(s.ListedOutputs, ",")] = true
			if !s.onAllowList(allow) && (s.Runner == "linux-sandbox" || s.Runner == "worker" || s.Runner == "processwrapper-sandbox" || s.Runner == "local") {
				st.NonAllowExecuted++
			}
		}
	}
	st.UniqueExecutedKeys = len(uniq)
	if st.Cacheable > 0 {
		st.HitRatio = float64(st.Hits) / float64(st.Cacheable)
	}
	return st
}

// CPUMultiple is executed actions over distinct executed actions across several builds' logs: the
// no-cross-server-dedup cost of a concurrent cold start (1.0 means every action ran once).
func CPUMultiple(logs [][]Spawn) float64 {
	total, uniq := 0, map[string]bool{}
	for _, sp := range logs {
		for _, s := range sp {
			if executed(s) {
				total++
				uniq[s.Mnemonic+"|"+s.TargetLabel+"|"+strings.Join(s.ListedOutputs, ",")] = true
			}
		}
	}
	if len(uniq) == 0 {
		return -1
	}
	return float64(total) / float64(len(uniq))
}

var absPathRE = regexp.MustCompile(`(^|[\s='":(])/(home|tmp|root|var|Users|mnt|opt)/`)

// CompareAquery implements E3: byte-identical output and no absolute path (the worktree roots or any
// /home, /tmp, ... prefix). It returns the verdict and a reason on failure.
func CompareAquery(a, b string, roots ...string) (bool, string) {
	if a != b {
		return false, "command lines differ between worktrees"
	}
	for _, r := range roots {
		if r != "" && strings.Contains(a, r) {
			return false, "absolute worktree path in command line: " + r
		}
	}
	if absPathRE.MatchString(a) {
		return false, "absolute path in command line"
	}
	return true, ""
}

// HashTree sha256s every regular file under root (symlinks followed once at the root only), keyed by
// relative path. Bazel's bazel-bin/bazel-out are symlink farms, so symlink entries are hashed by target content.
func HashTree(root string) (map[string]string, error) {
	out := map[string]string{}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		f, e := os.Open(p)
		if e != nil {
			return nil
		}
		defer f.Close()
		h := sha256.New()
		if _, e := io.Copy(h, f); e != nil {
			return nil
		}
		rel, _ := filepath.Rel(real, p)
		out[rel] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	return out, err
}

// DiffDigests lists relative paths whose digest differs or that exist on one side only.
func DiffDigests(a, b map[string]string) []string {
	var d []string
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			d = append(d, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			d = append(d, k)
		}
	}
	sort.Strings(d)
	return d
}

// ParseFilefragShared reports whether any extent line of `filefrag -v` output carries the `shared` flag.
func ParseFilefragShared(s string) bool {
	for _, ln := range strings.Split(s, "\n") {
		f := strings.Fields(ln)
		if len(f) == 0 {
			continue
		}
		for _, tok := range strings.Split(f[len(f)-1], ",") {
			if tok == "shared" {
				return true
			}
		}
	}
	return false
}

// CountPattern counts lines of text matching any of the lowercase needles (E-series corruption grep).
func CountPattern(text string, needles ...string) int {
	n := 0
	for _, ln := range strings.Split(text, "\n") {
		l := strings.ToLower(ln)
		for _, nd := range needles {
			if strings.Contains(l, nd) {
				n++
				break
			}
		}
	}
	return n
}

// CountErrorLines counts lines starting with "ERROR:" (the GC-under-reader check expects zero).
func CountErrorLines(text string) int {
	n := 0
	for _, ln := range strings.Split(text, "\n") {
		if strings.HasPrefix(ln, "ERROR:") {
			n++
		}
	}
	return n
}

// PctChange is (to-from)/from*100, NaN-safe (0 when from is 0).
func PctChange(from, to float64) float64 {
	if from == 0 {
		return 0
	}
	return (to - from) / from * 100
}

// WorkerMnemonics lists mnemonics whose aquery jsonproto action carries supports-workers / supports-multiplex-workers.
func WorkerMnemonics(aqueryJSON []byte) (workers, multiplex []string, err error) {
	var doc struct {
		Actions []map[string]any `json:"actions"`
	}
	if err = json.Unmarshal(aqueryJSON, &doc); err != nil {
		return nil, nil, err
	}
	w, m := map[string]bool{}, map[string]bool{}
	for _, a := range doc.Actions {
		mn, _ := a["mnemonic"].(string)
		raw, _ := json.Marshal(a)
		s := string(raw)
		if strings.Contains(s, "supports-multiplex-workers") {
			m[mn], w[mn] = true, true
		} else if strings.Contains(s, "supports-workers") {
			w[mn] = true
		}
	}
	return sortedKeys(w), sortedKeys(m), nil
}
