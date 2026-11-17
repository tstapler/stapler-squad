package artifacts

import (
	"regexp"

	"github.com/tstapler/stapler-squad/session/domain"
)

const redactedValue = "[redacted]"

// secretName matches a parameter, flag or variable name that holds a secret.
const secretName = `(?:token|secret|password|passwd|pwd|api[-_]?key|apikey|access[-_]?key|private[-_]?key|auth|credential|signature|sig|bearer|cookie)`

// quotedOrBare is a value: "double", 'single' or an unquoted run.
const quotedOrBare = `(?:"[^"]*"|'[^']*'|[^\s"']+)`

// assignValue is quotedOrBare that stops at "&", so a query string already
// scrubbed by reQuerySecret is not extended past its own value.
const assignValue = `(?:"[^"]*"|'[^']*'|[^\s"'&]+)`

// secretFlagValue is quotedOrBare minus a bare value starting with "-", so
// "--token --env prod" does not swallow the next flag.
const secretFlagValue = `(?:"[^"]*"|'[^']*'|[^\s"'-][^\s"']*)`

var (
	// ?token=v or &api_key=v in a URL query
	reQuerySecret = regexp.MustCompile(`(?i)([?&][\w.-]*` + secretName + `[\w.-]*=)[^&\s"'#]+`)
	// --token v, --token "v" or --api-key=v on a command line
	reFlagSecret = regexp.MustCompile(`(?i)(--[\w-]*` + secretName + `[\w-]*(?:=|\s+))` + secretFlagValue)
	// GITHUB_TOKEN=v, GITHUB_TOKEN="v"
	reEnvSecret = regexp.MustCompile(`\b([A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|API_?KEY|PRIVATE_?KEY)[A-Z0-9_]*=)` + quotedOrBare)
	// name = v / name: v / "name": "v" for names that end in a secret word
	// (aws_secret_access_key = v, X-Api-Key: v, _authToken=v, password: v).
	reAssignSecret = regexp.MustCompile(`(?i)(\b[\w.-]*(?:token|secret|password|passwd|api[-_]?key|apikey|access[-_]?key|private[-_]?key|authtoken|credential)["']?[ \t]*[:=][ \t]*)` + assignValue)
	// scheme://user:pass@host for any scheme
	reURLUserinfo = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^\s/@:"']+:[^\s/@"']+@`)
	// curl -u user:pass / --user user:pass
	reBasicUser = regexp.MustCompile(`(\s(?:-u[ \t]*|--user[ \t=]+))(?:"[^"]*:[^"]*"|'[^']*:[^']*'|[^\s"':]+:[^\s"']+)`)
	// mysql -p pw / sshpass -p pw
	rePasswordFlag = regexp.MustCompile(`(?i)(\b(?:mysql|mysqldump|mysqladmin|sshpass)\b[^\n|;&]*?\s-p[ \t]*)` + secretFlagValue)
	// Authorization: <scheme> <value>
	reAuthHeader = regexp.MustCompile(`(?i)((?:proxy-)?authorization["']?[ \t]*[:=][ \t]*["']?)(?:(?:Bearer|Basic|Digest|Token|Negotiate|NTLM|ApiKey)[ \t]+)?[^\s"']+`)
	// well-known token shapes
	reTokenShape = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|npm_[A-Za-z0-9]{30,})\b`)
	// JWT: three base64url segments, the first starting {"
	reJWT = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]*`)
	// Authorization: Bearer v
	reBearer = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
)

// Redact replaces secret-looking values (query parameters, flag values,
// environment assignments, URL credentials, well-known token shapes) with
// "[redacted]", keeping the name so the text still reads.
func Redact(s string) string {
	s = reQuerySecret.ReplaceAllString(s, "${1}"+redactedValue)
	s = reURLUserinfo.ReplaceAllString(s, "${1}"+redactedValue+"@")
	s = reAuthHeader.ReplaceAllString(s, "${1}"+redactedValue)
	for _, re := range []*regexp.Regexp{reFlagSecret, reEnvSecret, reAssignSecret, reBasicUser, rePasswordFlag, reBearer} {
		s = re.ReplaceAllString(s, "${1}"+redactedValue)
	}
	s = reJWT.ReplaceAllString(s, redactedValue)
	return reTokenShape.ReplaceAllString(s, redactedValue)
}

// RedactedRecords returns a scrubbed copy of rs; the input is not modified.
func RedactedRecords(rs domain.RecordList) domain.RecordList {
	out := make(domain.RecordList, len(rs))
	for i, r := range rs {
		out[i] = redactRecord(r)
	}
	return out
}

func redactAll(ss []string) []string {
	if ss == nil {
		return nil
	}
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = Redact(s)
	}
	return out
}

func redactCommandArtifact(c domain.CommandArtifact) domain.CommandArtifact {
	c.Command, c.Detail = Redact(c.Command), Redact(c.Detail)
	return c
}

func redactRecord(r domain.ArtifactRecord) domain.ArtifactRecord {
	switch v := r.(type) {
	case domain.PRRecord:
		v.Branch, v.Base = Redact(v.Branch), Redact(v.Base)
		return v
	case domain.BranchRecord:
		v.Name = Redact(v.Name)
		return v
	case domain.CommitRecord:
		v.Subject, v.Branch = Redact(v.Subject), Redact(v.Branch)
		return v
	case domain.PlanDocRecord:
		v.Path = Redact(v.Path)
		return v
	case domain.CILinkRecord:
		v.URL = Redact(v.URL)
		return v
	case domain.CommandRecord:
		v.Command, v.Detail, v.Result = Redact(v.Command), Redact(v.Detail), Redact(v.Result)
		return v
	}
	return r
}

// redactFindings scrubs everything one scan pass collected before it is stored.
func (f *scanFindings) redactFindings() {
	f.prURLs = redactAll(f.prURLs)
	f.externalURLs = redactAll(f.externalURLs)
	for i, c := range f.commands {
		f.commands[i] = redactCommandArtifact(c)
	}
	for i, r := range f.records {
		f.records[i] = redactRecord(r)
	}
}

// RedactBlob returns a scrubbed copy of the blob, for export. It cleans blobs
// stored before redaction existed; the receiver is not modified.
func RedactBlob(b *domain.SessionArtifactsBlob) *domain.SessionArtifactsBlob {
	out := *b
	out.PRURLs, out.ExternalURLs = redactAll(b.PRURLs), redactAll(b.ExternalURLs)
	out.Commands = make([]domain.CommandArtifact, len(b.Commands))
	for i, c := range b.Commands {
		out.Commands[i] = redactCommandArtifact(c)
	}
	out.Records = RedactedRecords(b.Records)
	return &out
}
