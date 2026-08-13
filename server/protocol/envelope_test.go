package protocol

import (
	"bytes"
	"testing"
)

func TestCreateEnvelope(t *testing.T) {
	tests := []struct {
		name  string
		flags byte
		data  []byte
		want  []byte
	}{
		{
			name:  "simple message",
			flags: 0x00,
			data:  []byte("test"),
			want:  []byte{0x00, 0x00, 0x00, 0x00, 0x04, 't', 'e', 's', 't'},
		},
		{
			name:  "empty data",
			flags: 0x00,
			data:  []byte{},
			want:  []byte{0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:  "with end stream flag",
			flags: EndStreamFlag,
			data:  []byte("end"),
			want:  []byte{0x02, 0x00, 0x00, 0x00, 0x03, 'e', 'n', 'd'},
		},
		{
			name:  "with compressed flag",
			flags: CompressedFlag,
			data:  []byte{0x01, 0x02},
			want:  []byte{0x01, 0x00, 0x00, 0x00, 0x02, 0x01, 0x02},
		},
		{
			name:  "256 byte message (test big-endian encoding)",
			flags: 0x00,
			data:  make([]byte, 256),
			want:  append([]byte{0x00, 0x00, 0x00, 0x01, 0x00}, make([]byte, 256)...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CreateEnvelope(tt.flags, tt.data)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("CreateEnvelope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    *Envelope
		wantErr bool
	}{
		{
			name:  "valid envelope",
			input: []byte{0x00, 0x00, 0x00, 0x00, 0x05, 'h', 'e', 'l', 'l', 'o'},
			want: &Envelope{
				Flags:  0x00,
				Length: 5,
				Data:   []byte("hello"),
			},
		},
		{
			name:  "envelope with flags",
			input: []byte{0x03, 0x00, 0x00, 0x00, 0x03, 'a', 'b', 'c'},
			want: &Envelope{
				Flags:  0x03,
				Length: 3,
				Data:   []byte("abc"),
			},
		},
		{
			name:    "envelope too short",
			input:   []byte{0x00, 0x00},
			wantErr: true,
		},
		{
			name:    "incomplete data",
			input:   []byte{0x00, 0x00, 0x00, 0x00, 0x05, 'h', 'i'},
			wantErr: true,
		},
		{
			name:  "empty data",
			input: []byte{0x00, 0x00, 0x00, 0x00, 0x00},
			want: &Envelope{
				Flags:  0x00,
				Length: 0,
				Data:   []byte{},
			},
		},
		{
			name:  "big-endian length decoding",
			input: append([]byte{0x00, 0x00, 0x00, 0x01, 0x00}, make([]byte, 256)...),
			want: &Envelope{
				Flags:  0x00,
				Length: 256,
				Data:   make([]byte, 256),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := ParseEnvelope(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEnvelope() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Flags != tt.want.Flags {
					t.Errorf("ParseEnvelope() flags = %v, want %v", got.Flags, tt.want.Flags)
				}
				if got.Length != tt.want.Length {
					t.Errorf("ParseEnvelope() length = %v, want %v", got.Length, tt.want.Length)
				}
				if !bytes.Equal(got.Data, tt.want.Data) {
					t.Errorf("ParseEnvelope() data = %v, want %v", got.Data, tt.want.Data)
				}
			}
		})
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	// Test that envelope creation and parsing are inverses
	testCases := []struct {
		name  string
		flags byte
		data  []byte
	}{
		{"simple text", 0x00, []byte("hello world")},
		{"empty data", 0x01, []byte{}},
		{"terminal escape codes", 0x02, []byte("\x1b[31mred text\x1b[0m")},
		{"binary data", 0x00, []byte{0x00, 0xFF, 0x1B, 0x5B}},
		{"1KB data", 0x00, make([]byte, 1024)},
		{"protobuf-like data", 0x00, []byte{0x0a, 0x0b, 0x6e, 0x65, 0x77, 0x2d, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			envelope := CreateEnvelope(tc.flags, tc.data)
			parsed, remaining, err := ParseEnvelope(envelope)

			if err != nil {
				t.Errorf("Failed to parse created envelope: %v", err)
			}
			if len(remaining) != 0 {
				t.Errorf("Unexpected remaining data: %d bytes", len(remaining))
			}
			if parsed.Flags != tc.flags {
				t.Errorf("Flags mismatch: got %v, want %v", parsed.Flags, tc.flags)
			}
			if !bytes.Equal(parsed.Data, tc.data) {
				t.Errorf("Data mismatch: got %v, want %v", parsed.Data, tc.data)
			}
		})
	}
}

func TestParseEnvelopeWithRemaining(t *testing.T) {
	// Test parsing when there's data after the envelope
	input := []byte{
		0x00, 0x00, 0x00, 0x00, 0x03, 'a', 'b', 'c', // First envelope
		0x00, 0x00, 0x00, 0x00, 0x02, 'x', 'y', // Second envelope
	}

	env1, remaining1, err := ParseEnvelope(input)
	if err != nil {
		t.Fatalf("Failed to parse first envelope: %v", err)
	}
	if !bytes.Equal(env1.Data, []byte("abc")) {
		t.Errorf("First envelope data = %v, want %v", env1.Data, []byte("abc"))
	}
	if len(remaining1) != 7 {
		t.Errorf("Remaining after first envelope = %d bytes, want 7", len(remaining1))
	}

	env2, remaining2, err := ParseEnvelope(remaining1)
	if err != nil {
		t.Fatalf("Failed to parse second envelope: %v", err)
	}
	if !bytes.Equal(env2.Data, []byte("xy")) {
		t.Errorf("Second envelope data = %v, want %v", env2.Data, []byte("xy"))
	}
	if len(remaining2) != 0 {
		t.Errorf("Remaining after second envelope = %d bytes, want 0", len(remaining2))
	}
}

func TestEnvelopeFlags(t *testing.T) {
	tests := []struct {
		name         string
		flags        byte
		isCompressed bool
		isEndStream  bool
	}{
		{"no flags", 0x00, false, false},
		{"compressed only", CompressedFlag, true, false},
		{"end stream only", EndStreamFlag, false, true},
		{"both flags", CompressedFlag | EndStreamFlag, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := &Envelope{Flags: tt.flags}
			if env.IsCompressed() != tt.isCompressed {
				t.Errorf("IsCompressed() = %v, want %v", env.IsCompressed(), tt.isCompressed)
			}
			if env.IsEndStream() != tt.isEndStream {
				t.Errorf("IsEndStream() = %v, want %v", env.IsEndStream(), tt.isEndStream)
			}
		})
	}
}
