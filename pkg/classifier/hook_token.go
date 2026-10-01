package classifier

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// HookTokenFile is the per-user secret, under the state directory, shared by the server and
// ssq-hooks. It is never sent over the wire: each side proves knowledge of it with an HMAC, so
// a stray listener on the server's port cannot forge a decision (AutoAllow is the zero value)
// and an arbitrary local caller cannot write analytics rows.
const HookTokenFile = "hook_token"

// HookSigHeader carries the request body HMAC.
const HookSigHeader = "X-SSQ-Hook-Sig"

const hookTokenBytes = 32

// LoadOrCreateHookToken returns the token in dir, creating it (dir 0700, file 0600) if absent.
// Server side.
func LoadOrCreateHookToken(dir string) ([]byte, error) {
	path := filepath.Join(dir, HookTokenFile)
	tok, err := ReadHookToken(dir)
	if err == nil {
		return tok, nil
	}
	// A file that exists but is unusable (partial write, wrong mode) would otherwise leave the
	// endpoint disabled until someone deletes it by hand.
	if !os.IsNotExist(err) {
		if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
			return nil, fmt.Errorf("replace unusable %s: %w", path, rmErr)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	raw := make([]byte, hookTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate hook token: %w", err)
	}
	// O_EXCL so two starting processes cannot each write a different token.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- fixed filename under the state dir.
	if err != nil {
		if os.IsExist(err) {
			return ReadHookToken(dir)
		}
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := f.WriteString(hex.EncodeToString(raw)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close %s: %w", path, err)
	}
	return ReadHookToken(dir)
}

// ReadHookToken reads the token in dir; it fails if the file is missing, malformed, or readable
// by group/other (a token others can read authenticates nothing). Client side.
func ReadHookToken(dir string) ([]byte, error) {
	path := filepath.Join(dir, HookTokenFile)
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s has permissions %v, want 0600", path, fi.Mode().Perm())
	}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed filename under the state dir.
	if err != nil {
		return nil, err
	}
	tok, err := hex.DecodeString(string(data))
	if err != nil || len(tok) != hookTokenBytes {
		return nil, fmt.Errorf("%s is malformed", path)
	}
	return tok, nil
}

func hmacHex(token []byte, parts ...string) string {
	m := hmac.New(sha256.New, token)
	for _, p := range parts {
		// Length-prefix each part so ("ab","c") and ("a","bc") cannot collide.
		m.Write([]byte(strconv.Itoa(len(p)) + ":"))
		m.Write([]byte(p))
	}
	return hex.EncodeToString(m.Sum(nil))
}

// SignRequestBody returns the HMAC the client puts in HookSigHeader.
func SignRequestBody(token, body []byte) string {
	return hmacHex(token, "req", string(body))
}

// VerifyRequestBody checks a HookSigHeader value in constant time.
func VerifyRequestBody(token, body []byte, sig string) bool {
	return hmac.Equal([]byte(SignRequestBody(token, body)), []byte(sig))
}

// ResponseProof binds the server's answer to the client's nonce, so a recorded or forged
// response cannot be replayed for another request or another decision.
func ResponseProof(token []byte, nonce string, r RemoteClassifyResponse) string {
	return hmacHex(token, "resp", nonce, strconv.Itoa(r.Version), r.ConfigDir,
		strconv.Itoa(int(r.Result.Decision)), strconv.Itoa(int(r.Result.RiskLevel)), r.Result.RuleID,
		r.Result.RuleName, r.Result.Source, r.Result.Reason, r.Result.Alternative)
}

// VerifyResponseProof checks r.Proof against nonce in constant time.
func VerifyResponseProof(token []byte, nonce string, r RemoteClassifyResponse) bool {
	return hmac.Equal([]byte(ResponseProof(token, nonce, r)), []byte(r.Proof))
}
