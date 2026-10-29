package services

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Harness for Story 5.2.1a: loads the shared ADR-001 conformance vectors and the
// shared allowlist fixture, then drives the codec (ws_unary_frame.go) with them.

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
	BatchAllowed   []string `json:"batchAllowed"`
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

type vectorFields struct {
	ID       uint32          `json:"id"`
	Method   string          `json:"method"`
	Meta     json.RawMessage `json:"meta"`
	BodyHex  string          `json:"bodyHex"`
	TotalLen uint32          `json:"totalLen"`
	Fin      uint8           `json:"fin"`
	JSON     json.RawMessage `json:"json"`
}

func TestUnaryFrameCodec_ShouldDecodeAndReencodeEveryVector(t *testing.T) {
	var v unaryVectors
	loadFixture(t, "unary-mux-vectors.json", &v)
	for _, vf := range v.Frames {
		t.Run(vf.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(vf.Hex)
			require.NoError(t, err)
			var fl vectorFields
			require.NoError(t, json.Unmarshal(vf.Fields, &fl))
			body, err := hex.DecodeString(fl.BodyHex)
			require.NoError(t, err)

			got, err := decodeUnaryFrame(raw)
			require.NoError(t, err)
			assert.Equal(t, unaryFrameType(v.Types[vf.Type]), got.Type)
			assert.Equal(t, fl.ID, got.ID)

			var again []byte
			switch got.Type {
			case unaryFrameRequest:
				assert.Equal(t, fl.Method, got.Method)
				assert.True(t, bytes.Equal(body, got.Body))
				var meta unaryRequestMeta
				require.NoError(t, json.Unmarshal(fl.Meta, &meta))
				assert.Equal(t, meta, got.RequestMeta)
				again, err = encodeUnaryRequest(got.ID, got.Method, got.RequestMeta, got.Body)
			case unaryFrameResponse:
				assert.Equal(t, fl.TotalLen, got.TotalLen)
				assert.Equal(t, fl.Fin == 1, got.Fin)
				assert.True(t, bytes.Equal(body, got.Body))
				again, err = encodeUnaryResponse(got.ID, got.TotalLen, got.Fin, got.ResponseMeta, got.Body)
			case unaryFrameChunk:
				assert.Equal(t, fl.Fin == 1, got.Fin)
				assert.True(t, bytes.Equal(body, got.Body))
				again = encodeUnaryChunk(got.ID, got.Fin, got.Body)
			case unaryFrameError:
				again, err = encodeUnaryError(got.ID, got.Error)
			default:
				again = encodeUnaryControl(got.Type, got.ID)
			}
			require.NoError(t, err)
			assert.Equal(t, vf.Hex, hex.EncodeToString(again), "re-encode must match the vector bytes")
		})
	}
}

func TestUnaryFrameCodec_ShouldRejectInvalidVectors_As_Malformed(t *testing.T) {
	var v unaryVectors
	loadFixture(t, "unary-mux-vectors.json", &v)
	for _, iv := range v.Invalid {
		raw, err := hex.DecodeString(iv.Hex)
		require.NoError(t, err)
		_, err = decodeUnaryFrame(raw)
		assert.True(t, errors.Is(err, errMalformedUnaryFrame), "%s: got %v", iv.Name, err)
	}
}

func TestUnaryFrameCodec_ShouldReassembleChunkedVector(t *testing.T) {
	var v unaryVectors
	loadFixture(t, "unary-mux-vectors.json", &v)
	byName := map[string][]byte{}
	for _, vf := range v.Frames {
		byName[vf.Name], _ = hex.DecodeString(vf.Hex)
	}
	for _, r := range v.Reassembly {
		var payload []byte
		var total uint32
		for i, n := range r.Frames {
			f, err := decodeUnaryFrame(byName[n])
			require.NoError(t, err, r.Name)
			if i == 0 {
				total = f.TotalLen
			}
			payload = append(payload, f.Body...)
		}
		want, _ := hex.DecodeString(r.PayloadHex)
		assert.Equal(t, want, payload, r.Name)
		assert.EqualValues(t, total, len(payload), r.Name)
	}
}

func TestPeekUnaryFrameID_ShouldNeedFullHeader(t *testing.T) {
	_, ok := peekUnaryFrameID([]byte{1, 0, 0, 0})
	assert.False(t, ok)
	id, ok := peekUnaryFrameID([]byte{9, 0, 0, 1, 2, 0xff})
	assert.True(t, ok)
	assert.EqualValues(t, 0x102, id)
}

func TestUnaryFrameCodec_ShouldRejectBadMetaJSON(t *testing.T) {
	good, err := encodeUnaryRequest(1, "/a.B/C", unaryRequestMeta{}, nil)
	require.NoError(t, err)
	// Corrupt the meta JSON's opening brace: header(5)+len(2)+method(6)+len(2).
	bad := append([]byte(nil), good...)
	bad[5+2+6+2] = '['
	_, err = decodeUnaryFrame(bad)
	assert.ErrorIs(t, err, errMalformedUnaryFrame)
}

func FuzzDecodeUnaryFrame(f *testing.F) {
	var v unaryVectors
	raw, err := os.ReadFile(filepath.Join(transportFixtureDir, "unary-mux-vectors.json"))
	require.NoError(f, err)
	require.NoError(f, json.Unmarshal(raw, &v))
	for _, vf := range v.Frames {
		b, _ := hex.DecodeString(vf.Hex)
		f.Add(b)
	}
	for _, iv := range v.Invalid {
		b, _ := hex.DecodeString(iv.Hex)
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, msg []byte) {
		fr, err := decodeUnaryFrame(msg) // must not panic
		if err != nil {
			assert.ErrorIs(t, err, errMalformedUnaryFrame)
			return
		}
		assert.Equal(t, msg[0], byte(fr.Type))
	})
}
