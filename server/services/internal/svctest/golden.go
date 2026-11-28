package svctest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// CanonicalProto renders msg as JSON with stable key order and spacing.
func CanonicalProto(t testing.TB, msg proto.Message) string {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	require.NoError(t, err)
	var v any
	require.NoError(t, json.Unmarshal(raw, &v))
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return string(out)
}

// CanonicalError renders an RPC error as its connect code and message.
func CanonicalError(err error) string {
	if err == nil {
		return "ok"
	}
	var connErr *connect.Error
	if errors.As(err, &connErr) {
		return connErr.Code().String() + ": " + connErr.Message()
	}
	return "error: " + err.Error()
}

// CompareGolden compares lines with the file at path, rewriting it when update is set.
func CompareGolden(t testing.TB, path string, lines []string, update bool) {
	t.Helper()
	got := strings.Join(lines, "\n") + "\n"
	if update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o600))
	}
	want, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
