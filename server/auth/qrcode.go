package auth

import (
	"fmt"
	"os"

	qrcode "github.com/skip2/go-qrcode"
)

// QRSize is the pixel dimension of the generated QR code PNG.
const QRSize = 256

// GenerateQRPNG returns a PNG-encoded QR code for the given URL.
func GenerateQRPNG(url string) ([]byte, error) {
	png, err := qrcode.Encode(url, qrcode.Medium, QRSize)
	if err != nil {
		return nil, fmt.Errorf("generate QR PNG: %w", err)
	}
	return png, nil
}

// PrintQRToTerminal prints an ASCII-art QR code to stderr so the operator can
// scan it directly from the terminal.
func PrintQRToTerminal(url string) error {
	q, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return fmt.Errorf("create QR: %w", err)
	}
	fmt.Fprintln(os.Stderr, "\n── Scan this QR code with your phone to set up remote access ──")
	fmt.Fprintln(os.Stderr, q.ToSmallString(false))
	fmt.Fprintf(os.Stderr, "Setup URL: %s\n\n", url)
	return nil
}
