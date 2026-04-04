package services

import (
	"context"
	"net"
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

	"connectrpc.com/connect"
)

func TestValidateLocalhostOrigin(t *testing.T) {
	tests := []struct {
		name          string
		headers       map[string]string
		peerAddr      string
		expectedError bool
	}{
		{
			name:          "Localhost IPv4",
			headers:       map[string]string{},
			peerAddr:      "127.0.0.1:12345",
			expectedError: false,
		},
		{
			name: "Spoofed X-Real-IP - From External",
			headers: map[string]string{
				"X-Real-IP": "127.0.0.1",
			},
			peerAddr:      "192.168.1.1:12345",
			expectedError: true,
		},
		{
			name: "Spoofed X-Forwarded-For - From External",
			headers: map[string]string{
				"X-Forwarded-For": "127.0.0.1, 192.168.1.1",
			},
			peerAddr:      "192.168.1.1:12345",
			expectedError: true,
		},
		{
			name:          "External IP",
			headers:       map[string]string{},
			peerAddr:      "192.168.1.1:12345",
			expectedError: true,
		},
		{
			name:          "IPv6 Localhost",
			headers:       map[string]string{},
			peerAddr:      "[::1]:12345",
			expectedError: false,
		},
		{
			name:          "Missing IP",
			headers:       map[string]string{},
			peerAddr:      "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := connect.NewRequest(&sessionv1.SendNotificationRequest{})
			for k, v := range tt.headers {
				req.Header().Set(k, v)
			}

			ctx := context.Background()
			if tt.peerAddr != "" {
				ctx = connect.WithPeer(ctx, connect.Peer{
					Addr: tt.peerAddr,
				})
			}

			err := validateLocalhostOrigin(ctx, req)
			if (err != nil) != tt.expectedError {
				t.Errorf("validateLocalhostOrigin() error = %v, expectedError %v", err, tt.expectedError)
			}
		})
	}
}
