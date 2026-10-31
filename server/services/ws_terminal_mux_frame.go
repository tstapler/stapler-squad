package services

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

// Terminal mux wire format (ADR-033). One binary WebSocket message is
// type u8 | stream id u32 | body, big-endian. Shared conformance vectors live in
// web-app/src/lib/transport/__fixtures__/terminal-mux-vectors.json.

// TerminalMuxPath is the mux upgrade endpoint.
const TerminalMuxPath = "/api/ws/terminal-mux"

const (
	muxHeaderLen = 5
	// muxChunkBytes is the largest DATA payload; bigger WebSocket messages are split so
	// one terminal's large frame never delays another stream by more than one chunk.
	muxChunkBytes = 16 << 10
	// muxStreamWindow is each stream's initial send credit (server -> client output).
	muxStreamWindow = 256 << 10
	// muxMaxStreamsPerSocket bounds streams on one socket; OPEN beyond it is refused with 1013.
	muxMaxStreamsPerSocket = 64
	// muxReadLimitBytes is the gorilla backstop; a frame is at most muxChunkBytes plus headers.
	muxReadLimitBytes = 4 * muxChunkBytes
)

type muxFrameType uint8

const (
	muxFrameHello  muxFrameType = 1
	muxFrameOpen   muxFrameType = 2
	muxFrameData   muxFrameType = 3
	muxFrameCredit muxFrameType = 4
	muxFrameClose  muxFrameType = 5
	muxFramePing   muxFrameType = 6
	muxFramePong   muxFrameType = 7
	muxFrameConfig muxFrameType = 8
)

var errMalformedMuxFrame = errors.New("malformed terminal mux frame")

// muxConfig is the HELLO/CONFIG body. PoolSize is the server's advertised socket count.
type muxConfig struct {
	PoolSize   int `json:"poolSize"`
	Window     int `json:"window"`
	Chunk      int `json:"chunk"`
	MaxStreams int `json:"maxStreams"`
}

// muxFrame is a decoded frame; only the fields for Type are set.
type muxFrame struct {
	Type    muxFrameType
	ID      uint32
	Kind    int  // DATA: websocket message type (1 text, 2 binary)
	Fin     bool // DATA
	Payload []byte
	Credit  uint32 // CREDIT
	Code    uint16 // CLOSE
	Reason  string // CLOSE
	Config  muxConfig
}

func muxHeader(t muxFrameType, id uint32, bodyLen int) []byte {
	b := make([]byte, muxHeaderLen, muxHeaderLen+bodyLen)
	b[0] = byte(t)
	binary.BigEndian.PutUint32(b[1:], id)
	return b
}

func encodeMuxData(id uint32, kind int, fin bool, payload []byte) []byte {
	b := muxHeader(muxFrameData, id, 2+len(payload))
	finByte := byte(0)
	if fin {
		finByte = 1
	}
	b = append(b, byte(kind), finByte)
	return append(b, payload...)
}

func encodeMuxCredit(id uint32, credit int) []byte {
	return binary.BigEndian.AppendUint32(muxHeader(muxFrameCredit, id, 4), uint32(credit))
}

func encodeMuxClose(id uint32, code uint16, reason string) []byte {
	b := muxHeader(muxFrameClose, id, 2+len(reason))
	b = binary.BigEndian.AppendUint16(b, code)
	return append(b, reason...)
}

func encodeMuxEmpty(t muxFrameType, id uint32) []byte { return muxHeader(t, id, 0) }

func encodeMuxConfig(t muxFrameType, c muxConfig) []byte {
	body, _ := json.Marshal(c) // fixed struct of ints: cannot fail
	return append(muxHeader(t, 0, len(body)), body...)
}

// decodeMuxFrame parses one frame without panicking on arbitrary input. Payload aliases msg.
func decodeMuxFrame(msg []byte) (muxFrame, error) {
	if len(msg) < muxHeaderLen {
		return muxFrame{}, fmt.Errorf("%w: %d bytes, need header", errMalformedMuxFrame, len(msg))
	}
	f := muxFrame{Type: muxFrameType(msg[0]), ID: binary.BigEndian.Uint32(msg[1:muxHeaderLen])}
	body := msg[muxHeaderLen:]
	switch f.Type {
	case muxFrameData:
		if len(body) < 2 || (body[0] != 1 && body[0] != 2) {
			return muxFrame{}, fmt.Errorf("%w: bad DATA header", errMalformedMuxFrame)
		}
		f.Kind, f.Fin, f.Payload = int(body[0]), body[1] != 0, body[2:]
	case muxFrameCredit:
		if len(body) != 4 {
			return muxFrame{}, fmt.Errorf("%w: CREDIT body %d bytes", errMalformedMuxFrame, len(body))
		}
		f.Credit = binary.BigEndian.Uint32(body)
	case muxFrameClose:
		if len(body) < 2 {
			return muxFrame{}, fmt.Errorf("%w: CLOSE body %d bytes", errMalformedMuxFrame, len(body))
		}
		f.Code, f.Reason = binary.BigEndian.Uint16(body), string(body[2:])
	case muxFrameHello, muxFrameConfig:
		if err := json.Unmarshal(body, &f.Config); err != nil {
			return muxFrame{}, fmt.Errorf("%w: config: %v", errMalformedMuxFrame, err)
		}
	case muxFrameOpen, muxFramePing, muxFramePong:
	default:
		return muxFrame{}, fmt.Errorf("%w: unknown type %d", errMalformedMuxFrame, msg[0])
	}
	return f, nil
}
