package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

// RecordKind tags a record's variant in the stored JSON.
type RecordKind string

const (
	KindPR      RecordKind = "pr"
	KindBranch  RecordKind = "branch"
	KindCommit  RecordKind = "commit"
	KindIssue   RecordKind = "issue"
	KindPlanDoc RecordKind = "plan_doc"
	KindCILink  RecordKind = "ci_link"
	KindCommand RecordKind = "command"
)

// ArtifactRecord is one typed finding. The set of variants is closed (the
// methods are unexported), so a variant carries only the fields its kind has:
// a CommitRecord has no Host, a PRRecord has no SHA.
type ArtifactRecord interface {
	Kind() RecordKind
	firstSeen() time.Time
	// key identifies the finding for dedup; equal keys describe the same thing.
	key() string
}

// PRSource says whether the session authored a PR or only saw it.
type PRSource string

const (
	PRSourceCreated   PRSource = "created"
	PRSourceMentioned PRSource = "mentioned"
)

type PRRecord struct {
	Host        string    `json:"host"`
	Owner       string    `json:"owner"`
	Repo        string    `json:"repo"`
	Number      int       `json:"number"`
	Source      PRSource  `json:"source"`
	Branch      string    `json:"branch,omitempty"`
	Base        string    `json:"base,omitempty"`
	FirstSeenAt time.Time `json:"first_seen_at"`
}

type BranchRecord struct {
	Name        string    `json:"name"`
	Action      string    `json:"action,omitempty"` // "created" or "pushed"
	FirstSeenAt time.Time `json:"first_seen_at"`
}

type CommitRecord struct {
	SHA         string    `json:"sha"`
	Subject     string    `json:"subject,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	FirstSeenAt time.Time `json:"first_seen_at"`
}

type IssueRecord struct {
	Host        string    `json:"host,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	Repo        string    `json:"repo,omitempty"`
	Number      int       `json:"number"`
	Relation    string    `json:"relation,omitempty"` // e.g. "fixes", "closes"
	FirstSeenAt time.Time `json:"first_seen_at"`
}

type PlanDocRecord struct {
	Path        string    `json:"path"`
	FirstSeenAt time.Time `json:"first_seen_at"`
}

type CILinkRecord struct {
	URL         string    `json:"url"`
	FirstSeenAt time.Time `json:"first_seen_at"`
}

// CommandRecord pairs a bash command with the text its tool_result returned.
type CommandRecord struct {
	Type        string    `json:"type"`
	Command     string    `json:"command"`
	Detail      string    `json:"detail,omitempty"`
	Result      string    `json:"result,omitempty"` // at most MaxResultBytes
	FirstSeenAt time.Time `json:"first_seen_at"`
}

func (PRRecord) Kind() RecordKind      { return KindPR }
func (BranchRecord) Kind() RecordKind  { return KindBranch }
func (CommitRecord) Kind() RecordKind  { return KindCommit }
func (IssueRecord) Kind() RecordKind   { return KindIssue }
func (PlanDocRecord) Kind() RecordKind { return KindPlanDoc }
func (CILinkRecord) Kind() RecordKind  { return KindCILink }
func (CommandRecord) Kind() RecordKind { return KindCommand }

func (r PRRecord) firstSeen() time.Time      { return r.FirstSeenAt }
func (r BranchRecord) firstSeen() time.Time  { return r.FirstSeenAt }
func (r CommitRecord) firstSeen() time.Time  { return r.FirstSeenAt }
func (r IssueRecord) firstSeen() time.Time   { return r.FirstSeenAt }
func (r PlanDocRecord) firstSeen() time.Time { return r.FirstSeenAt }
func (r CILinkRecord) firstSeen() time.Time  { return r.FirstSeenAt }
func (r CommandRecord) firstSeen() time.Time { return r.FirstSeenAt }

func (r PRRecord) key() string     { return fmt.Sprintf("%s/%s/%s#%d", r.Host, r.Owner, r.Repo, r.Number) }
func (r BranchRecord) key() string { return r.Name + "|" + r.Action }
func (r CommitRecord) key() string { return r.SHA }
func (r IssueRecord) key() string {
	return fmt.Sprintf("%s/%s/%s#%d|%s", r.Host, r.Owner, r.Repo, r.Number, r.Relation)
}
func (r PlanDocRecord) key() string { return r.Path }
func (r CILinkRecord) key() string  { return r.URL }
func (r CommandRecord) key() string { return r.Type + "|" + r.Command }

// unknownRecords counts stored records skipped because their kind is not known
// to this binary (a newer writer, or corruption).
var unknownRecords atomic.Uint64

// UnknownRecordsSkipped is how many unparseable records this process has skipped.
func UnknownRecordsSkipped() uint64 { return unknownRecords.Load() }

// recordSkipObserver lets a higher layer log a skipped record; the domain
// package cannot import the logger.
var recordSkipObserver atomic.Pointer[func(error)]

// SetRecordSkipObserver registers fn to be told about each unparseable record.
func SetRecordSkipObserver(fn func(error)) { recordSkipObserver.Store(&fn) }

// ParseRecord is the single decode point for stored records, shared by every
// reader. It returns the variant its "kind" names; fields that variant does
// not declare are dropped.
func ParseRecord(raw []byte) (ArtifactRecord, error) {
	var head struct {
		Kind RecordKind `json:"kind"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, err
	}
	switch head.Kind {
	case KindPR:
		return decodeRecord[PRRecord](raw)
	case KindBranch:
		return decodeRecord[BranchRecord](raw)
	case KindCommit:
		return decodeRecord[CommitRecord](raw)
	case KindIssue:
		return decodeRecord[IssueRecord](raw)
	case KindPlanDoc:
		return decodeRecord[PlanDocRecord](raw)
	case KindCILink:
		return decodeRecord[CILinkRecord](raw)
	case KindCommand:
		return decodeRecord[CommandRecord](raw)
	}
	return nil, fmt.Errorf("unknown record kind %q", head.Kind)
}

func decodeRecord[T ArtifactRecord](raw []byte) (ArtifactRecord, error) {
	var r T
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return r, nil
}

// MarshalRecord encodes r with its "kind" tag first.
func MarshalRecord(r ArtifactRecord) ([]byte, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	tag := fmt.Sprintf(`{"kind":%q`, r.Kind())
	if len(body) <= 2 { // "{}"
		return []byte(tag + "}"), nil
	}
	return append([]byte(tag+","), body[1:]...), nil
}

// RecordList is the stored form of a blob's records: a JSON array of tagged records.
type RecordList []ArtifactRecord

func (l RecordList) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, r := range l {
		if i > 0 {
			buf.WriteByte(',')
		}
		b, err := MarshalRecord(r)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

// UnmarshalJSON parses every element and skips (counted, and reported to the skip observer) the ones
// no variant claims, so one bad record never loses the blob.
func (l *RecordList) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}
	out := make(RecordList, 0, len(raws))
	for _, raw := range raws {
		r, err := ParseRecord(raw)
		if err != nil {
			unknownRecords.Add(1)
			if fn := recordSkipObserver.Load(); fn != nil {
				(*fn)(err)
			}
			continue
		}
		out = append(out, r)
	}
	*l = out
	return nil
}

// MergeRecords appends add to existing, keeping the first of each key in order.
// A PR seen again keeps its earliest time, and a "created" sighting upgrades a
// "mentioned" one and fills its empty branch and base.
func MergeRecords(existing, add []ArtifactRecord) []ArtifactRecord {
	out := make([]ArtifactRecord, 0, len(existing)+len(add))
	index := make(map[string]int, len(existing)+len(add))
	for _, r := range append(append([]ArtifactRecord{}, existing...), add...) {
		k := string(r.Kind()) + ":" + r.key()
		i, seen := index[k]
		if !seen {
			index[k] = len(out)
			out = append(out, r)
			continue
		}
		switch cur := out[i].(type) {
		case PRRecord:
			out[i] = mergePR(cur, r.(PRRecord))
		case CommandRecord:
			if cur.Result == "" { // the result may arrive in a later scan pass
				cur.Result = r.(CommandRecord).Result
				out[i] = cur
			}
		}
	}
	return out
}

func mergePR(cur, other PRRecord) PRRecord {
	if other.Source == PRSourceCreated && cur.Source != PRSourceCreated {
		cur.Source = PRSourceCreated
	}
	if cur.Branch == "" {
		cur.Branch = other.Branch
	}
	if cur.Base == "" {
		cur.Base = other.Base
	}
	if !other.FirstSeenAt.IsZero() && (cur.FirstSeenAt.IsZero() || other.FirstSeenAt.Before(cur.FirstSeenAt)) {
		cur.FirstSeenAt = other.FirstSeenAt
	}
	return cur
}
