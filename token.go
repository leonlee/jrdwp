package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// tokenMaxSkew is how far, in seconds, a token's timestamp may be from the
// server's clock in either direction.
const tokenMaxSkew = 60

var (
	errTokenMalformed = errors.New("malformed token")
	errTokenInvalid   = errors.New("invalid token")
	errTokenExpired   = errors.New("token expired or from the future, check client and server clocks")
)

// newSecret returns a random key. The server makes a new one on every start.
func newSecret() ([]byte, error) {
	secret := make([]byte, 32)
	_, err := rand.Read(secret)
	return secret, err
}

// writeSecret replaces the key file with a new owner-only file. Rewriting it in
// place would let anyone still holding the old file open read the new key.
func writeSecret(path string, secret []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // fails harmlessly once renamed
	_, err = tmp.WriteString(hex.EncodeToString(secret) + "\n")
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func readSecret(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	secret, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(secret) == 0 {
		return nil, fmt.Errorf("bad key in %s, copy it again from a server running this version", path)
	}
	return secret, nil
}

// makeToken returns "<unix seconds>.<hex HMAC>". The HMAC covers the JDWP port,
// so a token only opens the port it was made for. Anyone who sees a token can
// replay it until it expires, so use wss on untrusted networks.
func makeToken(secret []byte, jdwpPort int, now time.Time) string {
	ts := strconv.FormatInt(now.Unix(), 10)
	return ts + "." + hex.EncodeToString(tokenMAC(secret, ts, jdwpPort))
}

func verifyToken(secret []byte, token string, jdwpPort int, now time.Time) error {
	ts, mac, ok := strings.Cut(token, ".")
	if !ok {
		return errTokenMalformed
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errTokenMalformed
	}
	got, err := hex.DecodeString(mac)
	if err != nil {
		return errTokenMalformed
	}
	if !hmac.Equal(got, tokenMAC(secret, ts, jdwpPort)) {
		return errTokenInvalid
	}
	// Bounds instead of subtraction: unix comes from the peer and could overflow.
	if unix < now.Unix()-tokenMaxSkew || unix > now.Unix()+tokenMaxSkew {
		return errTokenExpired
	}
	return nil
}

func tokenMAC(secret []byte, ts string, jdwpPort int) []byte {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%d", ts, jdwpPort)
	return mac.Sum(nil)
}
