package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeContextLines(t *testing.T) {
	tests := []struct {
		name    string
		in      int32
		want    int
		wantErr bool
	}{
		{"default", 0, 0, false},
		{"whole file", -1, -1, false},
		{"literal", 13, 13, false},
		{"at cap", maxContextLines, maxContextLines, false},
		{"above cap means whole file", maxContextLines + 1, -1, false},
		{"max int32 means whole file", 1<<31 - 1, -1, false},
		{"below -1 is invalid", -2, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeContextLines(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
