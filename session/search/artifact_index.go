package search

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/artifacts"
)

// ArtifactIndex is the in-memory index over what sessions produced (PRs, branches,
// commits, issues, plan docs, CI links, commands). It is a second corpus, apart
// from the history SearchEngine, so artifact documents never reach
// SearchClaudeHistory and never skew its avgdl or idf (ADR-004). It is rebuilt
// from the stored blobs at start and never persisted.
//
// It lives beside SearchEngine, not in package session, because this package
// imports session (a session-side file would be an import cycle).
type ArtifactIndex struct {
	links PRLinkSource

	mu     sync.RWMutex
	index  *InvertedIndex
	docs   *DocumentStore
	tok    *Tokenizer
	scorer *BM25Scorer

	prSessions map[session.PRRef]map[string]struct{} // artifact PR records -> sessions
	shas       map[string][]shaEntry                 // 7-char prefix -> commits
	bySession  map[string]sessionKeys                // what to unwind on Update/Remove
}

// PRLinkSource lists the sessions that hold a SessionPR link to a PR.
type PRLinkSource interface {
	SessionUUIDsByPR(ctx context.Context, ref session.PRRef) ([]string, error)
}

type shaEntry struct{ sha, sessionID string }

type sessionKeys struct {
	prs     []session.PRRef
	shaKeys []string
}

// ArtifactHit is one matching artifact document.
type ArtifactHit struct {
	SessionID string
	Type      artifacts.RecordKind
	Score     float64
	// Text is the display form: a PR ref, SHA, branch name, path or URL.
	Text string
}

// PRSource says where a session-for-PR hit came from.
type PRSource string

const (
	PRSourceSessionPR PRSource = "session_pr" // an attached link (Tracked)
	PRSourceArtifact  PRSource = "artifact"   // seen in the transcript
)

// SessionPRHit is one session found for a PR. A session matched by both sources
// is reported once, as session_pr.
type SessionPRHit struct {
	SessionID string
	Source    PRSource
}

// NewArtifactIndex returns an empty index. links may be nil (artifact records only).
func NewArtifactIndex(links PRLinkSource) *ArtifactIndex {
	idx := NewInvertedIndex()
	return &ArtifactIndex{
		links:      links,
		index:      idx,
		docs:       NewDocumentStore(),
		tok:        NewTokenizer(),
		scorer:     NewBM25Scorer(idx),
		prSessions: map[session.PRRef]map[string]struct{}{},
		shas:       map[string][]shaEntry{},
		bySession:  map[string]sessionKeys{},
	}
}

// shaPrefixLen is the abbreviated-SHA length the lookup map is keyed by.
const shaPrefixLen = 7

// Update replaces everything indexed for sessionID with blob's records. A nil
// blob just removes the session.
func (x *ArtifactIndex) Update(sessionID string, blob *artifacts.SessionArtifactsBlob) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.removeLocked(sessionID)
	if blob == nil {
		x.afterChange()
		return
	}
	var keys sessionKeys
	// Indexed redacted: legacy blobs may predate redaction.
	for i, rec := range artifacts.RedactedRecords(blob.Records) {
		doc, ok := artifactDocument(rec)
		if !ok {
			continue
		}
		tokens := x.tok.TokenizeArtifact(doc.search)
		if len(tokens) == 0 {
			continue
		}
		id := x.docs.Add(&Document{
			SessionID: sessionID, MessageIndex: i, MessageRole: string(rec.Kind()),
			Content: doc.display, WordCount: len(tokens),
		})
		x.index.AddDocumentSimple(id, tokens)
		x.recordExact(sessionID, rec, &keys)
	}
	if len(keys.prs)+len(keys.shaKeys) > 0 {
		x.bySession[sessionID] = keys
	}
	x.afterChange()
}

// Remove drops everything indexed for sessionID.
func (x *ArtifactIndex) Remove(sessionID string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.removeLocked(sessionID)
	x.afterChange()
}

func (x *ArtifactIndex) afterChange() {
	x.scorer.UpdateAvgDocLength()
	recordArtifactIndexDocs(x.docs.Count())
}

func (x *ArtifactIndex) removeLocked(sessionID string) {
	for _, id := range x.docs.GetDocIDsBySession(sessionID) {
		x.index.RemoveDocument(id)
	}
	x.docs.RemoveBySession(sessionID)
	keys, ok := x.bySession[sessionID]
	if !ok {
		return
	}
	for _, ref := range keys.prs {
		delete(x.prSessions[ref], sessionID)
		if len(x.prSessions[ref]) == 0 {
			delete(x.prSessions, ref)
		}
	}
	for _, k := range keys.shaKeys {
		x.shas[k] = slicesDeleteSession(x.shas[k], sessionID)
		if len(x.shas[k]) == 0 {
			delete(x.shas, k)
		}
	}
	delete(x.bySession, sessionID)
}

func slicesDeleteSession(es []shaEntry, sessionID string) []shaEntry {
	out := es[:0]
	for _, e := range es {
		if e.sessionID != sessionID {
			out = append(out, e)
		}
	}
	return out
}

// recordExact fills the exact-lookup maps (never SessionPR: ADR-004).
func (x *ArtifactIndex) recordExact(sessionID string, rec artifacts.ArtifactRecord, keys *sessionKeys) {
	switch r := rec.(type) {
	case artifacts.PRRecord:
		ref, err := session.NewPRRef(r.Host, r.Owner, r.Repo, r.Number)
		if err != nil {
			return
		}
		if x.prSessions[ref] == nil {
			x.prSessions[ref] = map[string]struct{}{}
		}
		x.prSessions[ref][sessionID] = struct{}{}
		keys.prs = append(keys.prs, ref)
	case artifacts.CommitRecord:
		sha := strings.ToLower(r.SHA)
		if len(sha) < shaPrefixLen {
			return
		}
		k := sha[:shaPrefixLen]
		x.shas[k] = append(x.shas[k], shaEntry{sha: sha, sessionID: sessionID})
		keys.shaKeys = append(keys.shaKeys, k)
	}
}

// DocCount is the number of indexed artifact documents.
func (x *ArtifactIndex) DocCount() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.docs.Count()
}

// Search ranks artifact documents for a free-text, branch or SHA query, best first.
// limit <= 0 means no limit.
func (x *ArtifactIndex) Search(query string, limit int) []ArtifactHit {
	x.mu.RLock()
	defer x.mu.RUnlock()
	terms := x.tok.TokenizeArtifact(query)
	if len(terms) == 0 {
		return nil
	}
	scored := x.scorer.ScoreAll(terms)
	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}
	hits := make([]ArtifactHit, 0, len(scored))
	for _, sd := range scored {
		if d := x.docs.Get(sd.DocID); d != nil {
			hits = append(hits, ArtifactHit{
				SessionID: d.SessionID, Type: artifacts.RecordKind(d.MessageRole), Score: sd.Score, Text: d.Content,
			})
		}
	}
	return hits
}

// SessionsForSHAPrefix returns the sessions with a commit whose SHA starts with
// prefix (at least 7 hex characters), in a stable order.
func (x *ArtifactIndex) SessionsForSHAPrefix(prefix string) []string {
	prefix = strings.ToLower(prefix)
	if len(prefix) < shaPrefixLen {
		return nil
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	set := map[string]struct{}{}
	for _, e := range x.shas[prefix[:shaPrefixLen]] {
		if strings.HasPrefix(e.sha, prefix) {
			set[e.sessionID] = struct{}{}
		}
	}
	return sortedKeys(set)
}

// SessionsForPR unions the sessions holding a SessionPR link to ref with the
// sessions whose transcript artifacts name it. A session in both is returned once,
// as session_pr. Rows are not checked against live sessions; the caller filters
// orphans. Artifact records never seed SessionPR.
func (x *ArtifactIndex) SessionsForPR(ctx context.Context, ref session.PRRef) ([]SessionPRHit, error) {
	if ref.IsZero() {
		return nil, session.ErrZeroPRRef
	}
	var linked []string
	if x.links != nil {
		var err error
		if linked, err = x.links.SessionUUIDsByPR(ctx, ref); err != nil {
			return nil, fmt.Errorf("sessions linked to %s: %w", ref, err)
		}
	}
	hits := make([]SessionPRHit, 0, len(linked))
	seen := make(map[string]struct{}, len(linked))
	for _, id := range linked {
		if _, dup := seen[id]; !dup {
			seen[id] = struct{}{}
			hits = append(hits, SessionPRHit{SessionID: id, Source: PRSourceSessionPR})
		}
	}
	x.mu.RLock()
	artifactSessions := sortedKeys(x.prSessions[ref])
	x.mu.RUnlock()
	for _, id := range artifactSessions {
		if _, dup := seen[id]; !dup {
			seen[id] = struct{}{}
			hits = append(hits, SessionPRHit{SessionID: id, Source: PRSourceArtifact})
		}
	}
	return hits, nil
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// artifactDoc is what one record contributes: the text that is tokenized and the
// short form shown in results.
type artifactDoc struct{ search, display string }

func artifactDocument(rec artifacts.ArtifactRecord) (artifactDoc, bool) {
	switch r := rec.(type) {
	case artifacts.PRRecord:
		ref := fmt.Sprintf("%s/%s#%d", r.Owner, r.Repo, r.Number)
		return artifactDoc{
			search:  strings.Join([]string{"pr", ref, r.Host + "/" + ref, fmt.Sprintf("https://%s/%s/%s/pull/%d", r.Host, r.Owner, r.Repo, r.Number), r.Branch, r.Base}, " "),
			display: ref,
		}, true
	case artifacts.BranchRecord:
		return artifactDoc{search: "branch " + r.Name + " " + r.Action, display: r.Name}, true
	case artifacts.CommitRecord:
		short := r.SHA
		if len(short) > shaPrefixLen {
			short = short[:shaPrefixLen]
		}
		return artifactDoc{search: strings.Join([]string{"commit", r.SHA, short, r.Subject, r.Branch}, " "), display: r.SHA}, true
	case artifacts.IssueRecord:
		ref := fmt.Sprintf("%s/%s#%d", r.Owner, r.Repo, r.Number)
		if r.Owner == "" || r.Repo == "" {
			ref = fmt.Sprintf("#%d", r.Number)
		}
		return artifactDoc{search: "issue " + ref + " " + r.Relation, display: ref}, true
	case artifacts.PlanDocRecord:
		return artifactDoc{search: "plan " + r.Path, display: r.Path}, true
	case artifacts.CILinkRecord:
		return artifactDoc{search: "ci " + r.URL, display: r.URL}, true
	case artifacts.CommandRecord:
		return artifactDoc{search: "command " + r.Type + " " + r.Command + " " + r.Detail, display: r.Command}, true
	}
	return artifactDoc{}, false
}
