package services

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Harness for Story 5.2.1a: loads the shared ADR-001 conformance vectors and the
// shared allowlist fixture. Codec cases are added by Task 5.2.1a; until the codec
// exists this file only checks that the vectors are self-consistent.

const transportFixtureDir = "../../web-app/src/lib/transport/__fixtures__"

type unaryVectorFrame struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Fields json.RawMessage `json:"fields"`
	Hex    string          `json:"hex"`
}

type unaryVectors struct {
	MaxFrameBytes int                `json:"maxFrameBytes"`
	Types         map[string]byte    `json:"types"`
	Frames        []unaryVectorFrame `json:"frames"`
	Reassembly    []struct {
		Name       string   `json:"name"`
		ID         uint32   `json:"id"`
		Frames     []string `json:"frames"`
		PayloadHex string   `json:"payloadHex"`
		TotalLen   int      `json:"totalLen"`
	} `json:"reassembly"`
	Invalid []struct {
		Name   string `json:"name"`
		Hex    string `json:"hex"`
		Expect string `json:"expect"`
	} `json:"invalid"`
}

type unaryAllowlist struct {
	WsUnaryAllowed []string `json:"wsUnaryAllowed"`
	ClientHeaders  []string `json:"clientHeaders"`
	MaxInFlight    int      `json:"maxInFlight"`
}

func loadFixture(t *testing.T, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(transportFixtureDir, name))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, into))
}

func TestUnaryFrameVectors_ShouldBeSelfConsistent(t *testing.T) {
	var v unaryVectors
	loadFixture(t, "unary-mux-vectors.json", &v)
	require.Equal(t, 262144, v.MaxFrameBytes)
	require.NotEmpty(t, v.Frames)

	byName := map[string][]byte{}
	for _, f := range v.Frames {
		raw, err := hex.DecodeString(f.Hex)
		require.NoError(t, err, f.Name)
		require.GreaterOrEqual(t, len(raw), 5, f.Name)
		assert.Equal(t, v.Types[f.Type], raw[0], "%s: type byte", f.Name)
		var fields struct {
			ID uint32 `json:"id"`
		}
		require.NoError(t, json.Unmarshal(f.Fields, &fields), f.Name)
		assert.Equal(t, fields.ID, binary.BigEndian.Uint32(raw[1:5]), "%s: id", f.Name)
		byName[f.Name] = raw
	}

	for _, r := range v.Reassembly {
		payload, err := hex.DecodeString(r.PayloadHex)
		require.NoError(t, err, r.Name)
		assert.Len(t, payload, r.TotalLen, r.Name)
		for _, n := range r.Frames {
			raw, ok := byName[n]
			require.True(t, ok, "%s references unknown frame %s", r.Name, n)
			assert.Equal(t, r.ID, binary.BigEndian.Uint32(raw[1:5]), "%s/%s id", r.Name, n)
		}
	}
	for _, iv := range v.Invalid {
		_, err := hex.DecodeString(iv.Hex)
		assert.NoError(t, err, iv.Name)
		assert.Equal(t, "malformed", iv.Expect, iv.Name)
	}
}

func TestUnaryAllowlistFixture_ShouldLoad(t *testing.T) {
	var a unaryAllowlist
	loadFixture(t, "ws-unary-allowlist.json", &a)
	assert.Len(t, a.WsUnaryAllowed, 9)
	assert.Equal(t, []string{"Connect-Protocol-Version"}, a.ClientHeaders)
	assert.Equal(t, 6, a.MaxInFlight)
}
