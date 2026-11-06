package contractarchive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	contractDir    = "../../../contracts/webhook-management/v1"
	goldenManifest = "../../../contracts/webhook-management/bundle/v1/MANIFEST.json"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden MANIFEST.json from the checked-in contract")

type member struct {
	hdr  tar.Header
	data []byte
}

func extractMembers(t *testing.T, archive []byte) []member {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	var out []member
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out
		}
		require.NoError(t, err)
		data, err := io.ReadAll(tr)
		require.NoError(t, err)
		out = append(out, member{hdr: *hdr, data: data})
	}
}

func copyDir(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	for _, rel := range PinnedFiles() {
		raw, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(rel)))
		require.NoError(t, err)
		target := filepath.Join(dst, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(t, os.WriteFile(target, raw, 0o600))
	}
	return dst
}

func TestBuild_should_ProduceTheCanonicalLayout_When_BuiltFromTheCheckedInContract(t *testing.T) {
	archive, err := Build(contractDir)
	require.NoError(t, err)

	members := extractMembers(t, archive)

	require.Len(t, members, len(PinnedFiles())+1)
	assert.Equal(t, ManifestName, members[0].hdr.Name, "MANIFEST.json must be the first member")
	for i, name := range PinnedFiles() {
		assert.Equal(t, name, members[i+1].hdr.Name, "members follow the pinned lexical order")
	}
	for _, m := range members {
		assert.Equal(t, byte(tar.TypeReg), m.hdr.Typeflag, m.hdr.Name)
		assert.EqualValues(t, 0o644, m.hdr.Mode, m.hdr.Name)
		assert.Zero(t, m.hdr.Uid, m.hdr.Name)
		assert.Zero(t, m.hdr.Gid, m.hdr.Name)
		assert.Empty(t, m.hdr.Uname+m.hdr.Gname, m.hdr.Name)
		assert.True(t, m.hdr.ModTime.Equal(time.Unix(0, 0)), "%s mtime must be the epoch", m.hdr.Name)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	require.NoError(t, err)
	assert.Empty(t, gz.Name)
	assert.True(t, gz.ModTime.IsZero(), "gzip header must not carry a timestamp")
}

func TestBuild_should_BeByteIdentical_When_SourceFilesHaveDifferentModTimesAndLocations(t *testing.T) {
	first := copyDir(t, contractDir)
	second := copyDir(t, contractDir)
	require.NoError(t, os.Chtimes(filepath.Join(second, PinnedFiles()[0]), time.Now(), time.Now().Add(-48*time.Hour)))

	a, err := Build(first)
	require.NoError(t, err)
	b, err := Build(second)
	require.NoError(t, err)

	assert.Equal(t, a, b)
}

func TestBuild_should_MatchTheGoldenManifest_When_BuiltFromTheCheckedInContract(t *testing.T) {
	archive, err := Build(contractDir)
	require.NoError(t, err)
	got := extractMembers(t, archive)[0].data
	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenManifest), 0o755))
		require.NoError(t, os.WriteFile(goldenManifest, got, 0o644))
	}

	want, err := os.ReadFile(goldenManifest)
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got),
		"the contract changed: review the diff, then regenerate with `go test ./server/integrations/contractarchive -run Golden -update`")
}

func TestBuild_should_RefuseTheDirectory_When_ItDoesNotMatchThePinnedSet(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"extra file": func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "fixtures", "extra.json"), []byte("{}"), 0o600))
		},
		"missing file": func(t *testing.T, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, PinnedFiles()[0])))
		},
		"symlinked file": func(t *testing.T, dir string) {
			p := filepath.Join(dir, PinnedFiles()[0])
			require.NoError(t, os.Remove(p))
			require.NoError(t, os.Symlink(filepath.Join(dir, PinnedFiles()[1]), p))
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			dir := copyDir(t, contractDir)
			mutate(t, dir)

			_, err := Build(dir)

			require.Error(t, err)
		})
	}
}

// spec describes a hand-built archive so each rejection rule can be exercised on its own.
type spec struct {
	manifest []byte
	members  []tar.Header
	data     map[string][]byte
	trailer  []byte
}

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func goodManifestJSON(files ...FileEntry) []byte {
	raw, err := EncodeManifest(Manifest{FormatVersion: 1, Contract: ContractName, Version: ContractVersion, Files: files})
	if err != nil {
		panic(err)
	}
	return raw
}

func regular(name string, data []byte) tar.Header {
	return tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(data))}
}

func (s spec) bytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	first := regular(ManifestName, s.manifest)
	require.NoError(t, tw.WriteHeader(&first))
	_, err := tw.Write(s.manifest)
	require.NoError(t, err)
	for _, hdr := range s.members {
		hdr := hdr
		body := s.data[hdr.Name]
		if hdr.Typeflag == tar.TypeReg {
			hdr.Size = int64(len(body))
		}
		require.NoError(t, tw.WriteHeader(&hdr))
		if hdr.Typeflag == tar.TypeReg {
			_, err = tw.Write(body)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	_, err = gz.Write(s.trailer)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func goodSpec() spec {
	a, b := []byte(`{"a":1}`), []byte(`{"b":2}`)
	return spec{
		manifest: goodManifestJSON(
			FileEntry{Path: "fixtures/a.json", SHA256: sha(a), SizeBytes: int64(len(a))},
			FileEntry{Path: "schemas/b.json", SHA256: sha(b), SizeBytes: int64(len(b))}),
		members: []tar.Header{regular("fixtures/a.json", a), regular("schemas/b.json", b)},
		data:    map[string][]byte{"fixtures/a.json": a, "schemas/b.json": b},
	}
}

func TestVerify_should_Accept_When_ArchiveFollowsTheV1Rules(t *testing.T) {
	m, err := Verify(goodSpec().bytes(t))

	require.NoError(t, err)
	assert.Len(t, m.Files, 2)
}

func TestVerify_should_AcceptTheProducersOutput_When_BuiltFromTheCheckedInContract(t *testing.T) {
	archive, err := Build(contractDir)
	require.NoError(t, err)

	m, err := Verify(archive)

	require.NoError(t, err)
	require.Len(t, m.Files, len(PinnedFiles()))
	for i, name := range PinnedFiles() {
		assert.Equal(t, name, m.Files[i].Path)
	}
}

// nonRegular lists a single empty file in the manifest, so its digest and size would match, and
// ships it as the given non-regular entry: only the member-type rule can reject it.
func nonRegular(hdr tar.Header) func(s *spec) {
	return func(s *spec) {
		hdr.Name, hdr.Mode = "fixtures/a.json", 0o644
		s.manifest = goodManifestJSON(FileEntry{Path: hdr.Name, SHA256: sha(nil), SizeBytes: 0})
		s.members = []tar.Header{hdr}
		s.data = map[string][]byte{}
	}
}

func TestVerify_should_Reject_When_ArchiveBreaksAV1Rule(t *testing.T) {
	cases := map[string]func(s *spec){
		"unknown manifest field": func(s *spec) {
			s.manifest = bytes.Replace(s.manifest, []byte(`"version": "v1",`), []byte(`"version": "v1", "signature": "x",`), 1)
		},
		"trailing manifest data": func(s *spec) { s.manifest = append(s.manifest, []byte(`{}`)...) },
		"wrong format version": func(s *spec) {
			s.manifest = []byte(`{"format_version":2,"contract":"` + ContractName + `","version":"v1","files":[]}`)
		},
		"wrong contract": func(s *spec) {
			s.manifest = []byte(`{"format_version":1,"contract":"other","version":"v1","files":[]}`)
		},
		"missing files key": func(s *spec) {
			s.manifest = []byte(`{"format_version":1,"contract":"` + ContractName + `","version":"v1"}`)
		},
		"unlisted member": func(s *spec) {
			s.members = append(s.members, regular("fixtures/extra.json", []byte("x")))
			s.data["fixtures/extra.json"] = []byte("x")
		},
		"omitted member": func(s *spec) { s.members = s.members[:1] },
		"digest mismatch": func(s *spec) {
			s.data["fixtures/a.json"] = []byte(`{"a":9}`)
		},
		"size mismatch": func(s *spec) {
			a := s.data["fixtures/a.json"]
			s.manifest = goodManifestJSON(
				FileEntry{Path: "fixtures/a.json", SHA256: sha(a), SizeBytes: int64(len(a)) + 1},
				FileEntry{Path: "schemas/b.json", SHA256: sha(s.data["schemas/b.json"]), SizeBytes: int64(len(s.data["schemas/b.json"]))})
		},
		"duplicate member": func(s *spec) { s.members = append(s.members, s.members[0]) },
		"duplicate manifest entry": func(s *spec) {
			a := s.data["fixtures/a.json"]
			e := FileEntry{Path: "fixtures/a.json", SHA256: sha(a), SizeBytes: int64(len(a))}
			s.manifest = goodManifestJSON(e, e)
		},
		"directory member":   nonRegular(tar.Header{Typeflag: tar.TypeDir}),
		"symlink member":     nonRegular(tar.Header{Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}),
		"hard link member":   nonRegular(tar.Header{Typeflag: tar.TypeLink, Linkname: "fixtures/a.json"}),
		"fifo member":        nonRegular(tar.Header{Typeflag: tar.TypeFifo}),
		"char device member": nonRegular(tar.Header{Typeflag: tar.TypeChar}),
		"manifest lists itself": func(s *spec) {
			s.manifest = goodManifestJSON(FileEntry{Path: ManifestName, SHA256: sha(nil), SizeBytes: 0})
			s.members = []tar.Header{regular(ManifestName, nil)}
			s.data = map[string][]byte{}
		},
		"uppercase digest": func(s *spec) {
			s.manifest = goodManifestJSON(FileEntry{Path: "fixtures/a.json", SHA256: "ABCDEF" + sha(nil)[6:], SizeBytes: 0})
			s.members = s.members[:1]
		},
		"negative size": func(s *spec) {
			s.manifest = goodManifestJSON(FileEntry{Path: "fixtures/a.json", SHA256: sha(nil), SizeBytes: -1})
			s.members = s.members[:1]
		},
		"data after the tar end marker": func(s *spec) { s.trailer = []byte("garbage") },
	}
	for _, p := range []string{"../escape.json", "/abs.json", "a/../b.json", "./a.json", "a//b.json", "dir\\file.json", "", ".."} {
		p := p
		cases["bad path "+p] = func(s *spec) {
			body := []byte("x")
			s.manifest = goodManifestJSON(FileEntry{Path: p, SHA256: sha(body), SizeBytes: 1})
			s.members = []tar.Header{regular(p, body)}
			s.data = map[string][]byte{p: body}
		}
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := goodSpec()
			mutate(&s)

			_, err := Verify(s.bytes(t))

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidArchive)
		})
	}
}

func TestVerify_should_Reject_When_FirstMemberIsNotNamedMANIFEST(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("x")
	m := goodManifestJSON(FileEntry{Path: "fixtures/a.json", SHA256: sha(body), SizeBytes: 1})
	for _, mem := range []struct {
		name string
		data []byte
	}{{"decoy.json", m}, {"fixtures/a.json", body}} {
		h := regular(mem.name, mem.data)
		require.NoError(t, tw.WriteHeader(&h))
		_, err := tw.Write(mem.data)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	_, err := Verify(buf.Bytes())

	assert.ErrorIs(t, err, ErrInvalidArchive, "a valid manifest under another name must not be accepted")
}

func TestVerify_should_Reject_When_InputIsNotAGzipTarball(t *testing.T) {
	_, err := Verify([]byte("not an archive"))

	assert.ErrorIs(t, err, ErrInvalidArchive)
}
