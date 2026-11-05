package artifacts

import "regexp"

const redactedValue = "[redacted]"

// secretName matches a parameter, flag or variable name that holds a secret.
const secretName = `(?:token|secret|password|passwd|pwd|api[-_]?key|apikey|access[-_]?key|private[-_]?key|auth|credential|signature|sig|bearer|cookie)`

var (
	// ?token=v or &api_key=v in a URL query
	reQuerySecret = regexp.MustCompile(`(?i)([?&][\w.-]*` + secretName + `[\w.-]*=)[^&\s"'#]+`)
	// --token v or --api-key=v on a command line
	reFlagSecret = regexp.MustCompile(`(?i)(--[\w-]*` + secretName + `[\w-]*(?:=|\s+))[^\s"'-][^\s"']*`)
	// GITHUB_TOKEN=v
	reEnvSecret = regexp.MustCompile(`\b([A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|API_?KEY|PRIVATE_?KEY)[A-Z0-9_]*=)[^\s"']+`)
	// https://user:pass@host
	reURLUserinfo = regexp.MustCompile(`(https?://)[^\s/@:"']+:[^\s/@"']+@`)
	// well-known token shapes
	reTokenShape = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16})\b`)
	// Authorization: Bearer v
	reBearer = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
)

// Redact replaces secret-looking values (query parameters, flag values,
// environment assignments, URL credentials, well-known token shapes) with
// "[redacted]", keeping the name so the text still reads.
func Redact(s string) string {
	s = reQuerySecret.ReplaceAllString(s, "${1}"+redactedValue)
	s = reFlagSecret.ReplaceAllString(s, "${1}"+redactedValue)
	s = reEnvSecret.ReplaceAllString(s, "${1}"+redactedValue)
	s = reURLUserinfo.ReplaceAllString(s, "${1}"+redactedValue+"@")
	s = reBearer.ReplaceAllString(s, "${1}"+redactedValue)
	return reTokenShape.ReplaceAllString(s, redactedValue)
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

func redactCommandArtifact(c CommandArtifact) CommandArtifact {
	c.Command, c.Detail = Redact(c.Command), Redact(c.Detail)
	return c
}

func redactRecord(r ArtifactRecord) ArtifactRecord {
	switch v := r.(type) {
	case PRRecord:
		v.Branch, v.Base = Redact(v.Branch), Redact(v.Base)
		return v
	case BranchRecord:
		v.Name = Redact(v.Name)
		return v
	case CommitRecord:
		v.Subject, v.Branch = Redact(v.Subject), Redact(v.Branch)
		return v
	case PlanDocRecord:
		v.Path = Redact(v.Path)
		return v
	case CILinkRecord:
		v.URL = Redact(v.URL)
		return v
	case CommandRecord:
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

// Redacted returns a scrubbed copy of the blob, for export. It cleans blobs
// stored before redaction existed; the receiver is not modified.
func (b *SessionArtifactsBlob) Redacted() *SessionArtifactsBlob {
	out := *b
	out.PRURLs, out.ExternalURLs = redactAll(b.PRURLs), redactAll(b.ExternalURLs)
	out.Commands = make([]CommandArtifact, len(b.Commands))
	for i, c := range b.Commands {
		out.Commands[i] = redactCommandArtifact(c)
	}
	out.Records = make(RecordList, len(b.Records))
	for i, r := range b.Records {
		out.Records[i] = redactRecord(r)
	}
	return &out
}
