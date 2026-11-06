// Package contractarchive builds, and strictly checks, the release archive of the generic
// webhook-management contract (contracts/webhook-management/v1).
//
// The archive is a tar.gz whose first member is a root MANIFEST.json listing every other member
// with its SHA-256 and size. It carries no signature: the released bytes are attested externally
// (GitHub keyless Sigstore), so the only integrity data inside is the manifest.
//
// Output is byte-identical for identical contract content on a given Go toolchain: stable member
// order, zeroed metadata and fixed gzip settings. The compressed bytes can differ across Go
// versions (compress/flate is not frozen), so a release must be built by one pinned toolchain
// and then attested; do not rebuild and expect the same digest.
package contractarchive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// ManifestName is the archive's first member.
	ManifestName = "MANIFEST.json"
	// FormatVersion is the manifest schema version this package reads and writes.
	FormatVersion = 1
	// ContractName and ContractVersion identify the contract the archive carries.
	ContractName    = "stapler-squad.generic-webhook-management"
	ContractVersion = "v1"

	maxFileBytes     = 1 << 20
	maxManifestBytes = 64 << 10
	maxMembers       = 64
	archiveMode      = 0o644
)

// ErrInvalidArchive is returned (wrapped) for every Verify rejection.
var ErrInvalidArchive = errors.New("invalid contract archive")

// Manifest is the strict v1 MANIFEST.json. MANIFEST.json itself is never listed.
type Manifest struct {
	FormatVersion int         `json:"format_version"`
	Contract      string      `json:"contract"`
	Version       string      `json:"version"`
	Files         []FileEntry `json:"files"`
}

// FileEntry describes one archive member.
type FileEntry struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

// PinnedFiles returns the exact set of contract files an archive contains, in archive order.
// Adding or removing a contract artifact is a deliberate change to this list.
func PinnedFiles() []string {
	return []string{
		"fixtures/capability-response.json",
		"fixtures/emergency-cleanup-request.delete.json",
		"fixtures/error.unauthenticated.json",
		"fixtures/error.version-conflict.json",
		"fixtures/lifecycle-request.disable.json",
		"fixtures/reconcile-request.create.json",
		"fixtures/reconcile-response.created.json",
		"schemas/capability-response.schema.json",
		"schemas/emergency-cleanup-request.schema.json",
		"schemas/error-response.schema.json",
		"schemas/lifecycle-request.schema.json",
		"schemas/reconcile-request.schema.json",
		"schemas/reconcile-response.schema.json",
		"schemas/registration.schema.json",
	}
}

// EncodeManifest renders m canonically (two-space indent, trailing newline).
func EncodeManifest(m Manifest) ([]byte, error) {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return append(raw, '\n'), nil
}

// Build packs the pinned contract files found under dir. It refuses a directory holding any
// other file, any missing pinned file, or anything that is not a regular file.
func Build(dir string) ([]byte, error) {
	pinned := PinnedFiles()
	if err := requireExactTree(dir, pinned); err != nil {
		return nil, err
	}
	manifest := Manifest{FormatVersion: FormatVersion, Contract: ContractName, Version: ContractVersion, Files: []FileEntry{}}
	contents := make(map[string][]byte, len(pinned))
	for _, rel := range pinned {
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		if len(raw) > maxFileBytes {
			return nil, fmt.Errorf("%s exceeds %d bytes", rel, maxFileBytes)
		}
		sum := sha256.Sum256(raw)
		contents[rel] = raw
		manifest.Files = append(manifest.Files, FileEntry{Path: rel, SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(raw))})
	}
	encoded, err := EncodeManifest(manifest)
	if err != nil {
		return nil, err
	}
	return writeArchive(encoded, pinned, contents)
}

func requireExactTree(dir string, pinned []string) error {
	found := map[string]bool{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", p)
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		found[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan contract directory: %w", err)
	}
	var problems []string
	for _, rel := range pinned {
		if !found[rel] {
			problems = append(problems, "missing "+rel)
		}
		delete(found, rel)
	}
	for rel := range found {
		problems = append(problems, "unexpected "+rel)
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("contract directory does not match the pinned file set: %s", strings.Join(problems, ", "))
	}
	return nil
}

func writeArchive(manifest []byte, names []string, contents map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("gzip writer: %w", err)
	}
	tw := tar.NewWriter(gz)
	put := func(name string, data []byte) error {
		hdr := &tar.Header{
			Typeflag: tar.TypeReg, Name: name, Mode: archiveMode, Size: int64(len(data)),
			ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}
	if err := put(ManifestName, manifest); err != nil {
		return nil, fmt.Errorf("write %s: %w", ManifestName, err)
	}
	for _, name := range names {
		if err := put(name, contents[name]); err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}
	return buf.Bytes(), nil
}

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Verify checks an archive against the strict v1 rules and returns its manifest:
// MANIFEST.json first, unknown manifest fields rejected, and every other member a regular file
// listed exactly once with a matching digest and size. Directories, links, devices, FIFOs,
// duplicate, absolute or traversal paths, unlisted members and omissions all fail.
func Verify(archive []byte) (*Manifest, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, invalid("not a gzip stream: %v", err)
	}
	limit := int64(maxMembers+1) * (maxFileBytes + 2*512)
	tr := tar.NewReader(io.LimitReader(gz, limit))

	manifest, err := readManifest(tr)
	if err != nil {
		return nil, err
	}
	if err := readMembers(tr, manifest); err != nil {
		return nil, err
	}
	if err := requireZeroTail(gz); err != nil {
		return nil, err
	}
	return manifest, nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidArchive, fmt.Sprintf(format, args...))
}

func readManifest(tr *tar.Reader) (*Manifest, error) {
	hdr, err := tr.Next()
	if err != nil {
		return nil, invalid("no first member: %v", err)
	}
	if hdr.Typeflag != tar.TypeReg || hdr.Name != ManifestName {
		return nil, invalid("first member must be the regular file %s", ManifestName)
	}
	raw, err := io.ReadAll(io.LimitReader(tr, maxManifestBytes+1))
	if err != nil || len(raw) > maxManifestBytes {
		return nil, invalid("%s unreadable or too large", ManifestName)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, invalid("%s: %v", ManifestName, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, invalid("%s has data after the JSON object", ManifestName)
	}
	if err := validateManifest(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func validateManifest(m *Manifest) error {
	if m.FormatVersion != FormatVersion || m.Contract != ContractName || m.Version != ContractVersion {
		return invalid("unsupported manifest identity (format_version=%d contract=%q version=%q)", m.FormatVersion, m.Contract, m.Version)
	}
	if m.Files == nil || len(m.Files) > maxMembers {
		return invalid("files must be present and at most %d entries", maxMembers)
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if err := validatePath(f.Path); err != nil {
			return err
		}
		if seen[f.Path] {
			return invalid("duplicate manifest entry %q", f.Path)
		}
		seen[f.Path] = true
		if !sha256Re.MatchString(f.SHA256) {
			return invalid("%q: sha256 must be 64 lowercase hex characters", f.Path)
		}
		if f.SizeBytes < 0 || f.SizeBytes > maxFileBytes {
			return invalid("%q: size_bytes out of range", f.Path)
		}
	}
	return nil
}

func validatePath(p string) error {
	switch {
	case p == "" || p == ManifestName:
		return invalid("path %q is not allowed", p)
	case path.IsAbs(p) || strings.Contains(p, `\`):
		return invalid("path %q must be relative and use forward slashes", p)
	case p == ".." || strings.HasPrefix(p, "../") || path.Clean(p) != p:
		return invalid("path %q is not a clean relative path", p)
	}
	return nil
}

func readMembers(tr *tar.Reader, m *Manifest) error {
	want := make(map[string]FileEntry, len(m.Files))
	for _, f := range m.Files {
		want[f.Path] = f
	}
	seen := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid("corrupt tar: %v", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			return invalid("member %q is not a regular file", hdr.Name)
		}
		entry, listed := want[hdr.Name]
		switch {
		case !listed:
			return invalid("member %q is not listed in the manifest", hdr.Name)
		case seen[hdr.Name]:
			return invalid("duplicate member %q", hdr.Name)
		}
		seen[hdr.Name] = true
		data, err := io.ReadAll(io.LimitReader(tr, maxFileBytes+1))
		if err != nil {
			return invalid("read %q: %v", hdr.Name, err)
		}
		sum := sha256.Sum256(data)
		if int64(len(data)) != entry.SizeBytes || hex.EncodeToString(sum[:]) != entry.SHA256 {
			return invalid("member %q does not match its manifest digest or size", hdr.Name)
		}
	}
	for name := range want {
		if !seen[name] {
			return invalid("manifest entry %q has no member", name)
		}
	}
	return nil
}

// requireZeroTail drains what follows the tar end marker; only zero padding is allowed, and
// draining also forces gzip to check its own trailer.
func requireZeroTail(gz io.Reader) error {
	rest, err := io.ReadAll(gz)
	if err != nil {
		return invalid("corrupt gzip: %v", err)
	}
	for _, b := range rest {
		if b != 0 {
			return invalid("unexpected data after the tar end marker")
		}
	}
	return nil
}
