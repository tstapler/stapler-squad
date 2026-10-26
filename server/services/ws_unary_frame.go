package services

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// Unary mux wire format (ADR-001, project_plans/frontend-responsiveness/decisions).
// One WebSocket binary message is: type u8 | request id u32 | type-specific body,
// all integers big-endian. Shared conformance vectors live in
// web-app/src/lib/transport/__fixtures__/unary-mux-vectors.json.

// unaryMaxFrameBytes bounds a whole encoded frame (type + id + body).
const unaryMaxFrameBytes = 256 * 1024

// unaryReadLimitBytes is the gorilla hard backstop: exceeding it kills the socket
// with close 1009, so it only guards protocol violations.
const unaryReadLimitBytes = 4 * unaryMaxFrameBytes

type unaryFrameType uint8

const (
	unaryFrameRequest  unaryFrameType = 1
	unaryFrameResponse unaryFrameType = 2
	unaryFrameError    unaryFrameType = 3
	unaryFrameCancel   unaryFrameType = 4
	unaryFramePing     unaryFrameType = 5
	unaryFramePong     unaryFrameType = 6
	unaryFrameChunk    unaryFrameType = 7
)

const unaryFrameHeaderLen = 5

// errMalformedUnaryFrame is returned by decodeUnaryFrame for any structurally
// invalid frame; the bridge closes the socket with 1003.
var errMalformedUnaryFrame = errors.New("malformed unary frame")

// unaryRequestMeta is the REQUEST meta JSON.
type unaryRequestMeta struct {
	Headers   map[string]string `json:"headers"`
	TimeoutMs int64             `json:"timeoutMs"`
}

// unaryResponseMeta is the RESPONSE meta JSON (first frame only).
type unaryResponseMeta struct {
	Headers  map[string]string `json:"headers"`
	Trailers map[string]string `json:"trailers"`
}

// unaryErrorBody is the ERROR frame JSON; Code is a Connect code string.
type unaryErrorBody struct {
	Code     string            `json:"code"`
	Message  string            `json:"message"`
	Details  json.RawMessage   `json:"details,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Trailers map[string]string `json:"trailers,omitempty"`
}

// unaryFrame is a decoded frame; only the fields for Type are set.
type unaryFrame struct {
	Type unaryFrameType
	ID   uint32

	Method      string           // REQUEST
	RequestMeta unaryRequestMeta // REQUEST

	TotalLen     uint32            // RESPONSE
	ResponseMeta unaryResponseMeta // RESPONSE
	Fin          bool              // RESPONSE, CHUNK

	Body  []byte         // REQUEST proto, RESPONSE first slice, CHUNK slice
	Error unaryErrorBody // ERROR
}

// peekUnaryFrameID reads the request id without validating the rest, so an
// oversize or malformed frame can still be answered for its id.
func peekUnaryFrameID(msg []byte) (uint32, bool) {
	if len(msg) < unaryFrameHeaderLen {
		return 0, false
	}
	return binary.BigEndian.Uint32(msg[1:unaryFrameHeaderLen]), true
}

// decodeUnaryFrame parses one frame. It never panics on arbitrary input and does
// not enforce unaryMaxFrameBytes (the caller does, so it can answer for the id).
// Returned Body slices alias msg.
func decodeUnaryFrame(msg []byte) (unaryFrame, error) {
	id, ok := peekUnaryFrameID(msg)
	if !ok {
		return unaryFrame{}, fmt.Errorf("%w: %d bytes, need header", errMalformedUnaryFrame, len(msg))
	}
	f := unaryFrame{Type: unaryFrameType(msg[0]), ID: id}
	body := msg[unaryFrameHeaderLen:]
	var err error
	switch f.Type {
	case unaryFrameRequest:
		err = decodeUnaryRequest(&f, body)
	case unaryFrameResponse:
		err = decodeUnaryResponse(&f, body)
	case unaryFrameChunk:
		if len(body) < 1 {
			return unaryFrame{}, fmt.Errorf("%w: chunk without fin byte", errMalformedUnaryFrame)
		}
		f.Fin = body[0] != 0
		f.Body = body[1:]
	case unaryFrameError:
		if jerr := json.Unmarshal(body, &f.Error); jerr != nil {
			return unaryFrame{}, fmt.Errorf("%w: error body: %v", errMalformedUnaryFrame, jerr)
		}
	case unaryFrameCancel, unaryFramePing, unaryFramePong:
	default:
		return unaryFrame{}, fmt.Errorf("%w: unknown type %d", errMalformedUnaryFrame, msg[0])
	}
	if err != nil {
		return unaryFrame{}, err
	}
	return f, nil
}

// readLenPrefixed reads a u16-length-prefixed field and returns it plus the rest.
func readLenPrefixed(b []byte, what string) (field, rest []byte, err error) {
	if len(b) < 2 {
		return nil, nil, fmt.Errorf("%w: %s length truncated", errMalformedUnaryFrame, what)
	}
	n := int(binary.BigEndian.Uint16(b))
	if len(b)-2 < n {
		return nil, nil, fmt.Errorf("%w: %s length %d overruns frame", errMalformedUnaryFrame, what, n)
	}
	return b[2 : 2+n], b[2+n:], nil
}

func decodeUnaryRequest(f *unaryFrame, body []byte) error {
	method, rest, err := readLenPrefixed(body, "method")
	if err != nil {
		return err
	}
	meta, rest, err := readLenPrefixed(rest, "meta")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(meta, &f.RequestMeta); err != nil {
		return fmt.Errorf("%w: request meta: %v", errMalformedUnaryFrame, err)
	}
	f.Method = string(method)
	f.Body = rest
	return nil
}

func decodeUnaryResponse(f *unaryFrame, body []byte) error {
	if len(body) < 5 {
		return fmt.Errorf("%w: response head truncated", errMalformedUnaryFrame)
	}
	f.TotalLen = binary.BigEndian.Uint32(body)
	f.Fin = body[4] != 0
	meta, rest, err := readLenPrefixed(body[5:], "meta")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(meta, &f.ResponseMeta); err != nil {
		return fmt.Errorf("%w: response meta: %v", errMalformedUnaryFrame, err)
	}
	if f.Fin && uint32(len(rest)) != f.TotalLen {
		return fmt.Errorf("%w: fin response carries %d bytes, totalLen %d", errMalformedUnaryFrame, len(rest), f.TotalLen)
	}
	f.Body = rest
	return nil
}

func unaryHeader(t unaryFrameType, id uint32, extra int) []byte {
	b := make([]byte, unaryFrameHeaderLen, unaryFrameHeaderLen+extra)
	b[0] = byte(t)
	binary.BigEndian.PutUint32(b[1:], id)
	return b
}

func appendLenPrefixed(b, field []byte) []byte {
	b = binary.BigEndian.AppendUint16(b, uint16(len(field)))
	return append(b, field...)
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

// encodeUnaryRequest is used by tests and Go clients; the server only decodes it.
func encodeUnaryRequest(id uint32, method string, meta unaryRequestMeta, body []byte) ([]byte, error) {
	if len(method) > 0xFFFF {
		return nil, fmt.Errorf("method too long: %d", len(method))
	}
	if meta.Headers == nil {
		meta.Headers = map[string]string{}
	}
	mj, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	if len(mj) > 0xFFFF {
		return nil, fmt.Errorf("meta too long: %d", len(mj))
	}
	b := unaryHeader(unaryFrameRequest, id, 4+len(method)+len(mj)+len(body))
	b = appendLenPrefixed(b, []byte(method))
	b = appendLenPrefixed(b, mj)
	return append(b, body...), nil
}

// encodeUnaryResponse builds the RESPONSE head; slice is the first payload slice.
func encodeUnaryResponse(id, totalLen uint32, fin bool, meta unaryResponseMeta, slice []byte) ([]byte, error) {
	if meta.Headers == nil {
		meta.Headers = map[string]string{}
	}
	if meta.Trailers == nil {
		meta.Trailers = map[string]string{}
	}
	mj, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	if len(mj) > 0xFFFF {
		return nil, fmt.Errorf("meta too long: %d", len(mj))
	}
	b := unaryHeader(unaryFrameResponse, id, 8+len(mj)+len(slice))
	b = binary.BigEndian.AppendUint32(b, totalLen)
	b = append(b, boolByte(fin))
	b = appendLenPrefixed(b, mj)
	return append(b, slice...), nil
}

func encodeUnaryChunk(id uint32, fin bool, slice []byte) []byte {
	b := unaryHeader(unaryFrameChunk, id, 1+len(slice))
	b = append(b, boolByte(fin))
	return append(b, slice...)
}

func encodeUnaryError(id uint32, e unaryErrorBody) ([]byte, error) {
	ej, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return append(unaryHeader(unaryFrameError, id, len(ej)), ej...), nil
}

// encodeUnaryControl encodes the bodiless CANCEL, PING and PONG frames.
func encodeUnaryControl(t unaryFrameType, id uint32) []byte {
	return unaryHeader(t, id, 0)
}

func (t unaryFrameType) String() string {
	switch t {
	case unaryFrameRequest:
		return "REQUEST"
	case unaryFrameResponse:
		return "RESPONSE"
	case unaryFrameError:
		return "ERROR"
	case unaryFrameCancel:
		return "CANCEL"
	case unaryFramePing:
		return "PING"
	case unaryFramePong:
		return "PONG"
	case unaryFrameChunk:
		return "CHUNK"
	}
	return "UNKNOWN(" + strconv.Itoa(int(t)) + ")"
}
