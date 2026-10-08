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
	"sync"
	"time"
)

const (
	// keyEnv, when set, holds the client's key instead of keyFile.
	keyEnv     = "JRDWP_KEY"
	secretSize = 32
	nonceSize  = 16
	// tokenMaxSkew is how far, in seconds, a token's timestamp may be from the
	// server's clock in either direction.
	tokenMaxSkew = 60
	// maxTokenLen is the longest valid token: a signed int64 timestamp, the
	// nonce and the MAC in hex, and two dots. Anything longer is rejected before
	// parsing, since the header comes from a peer that hasn't authenticated yet.
	maxTokenLen = 20 + 1 + 2*nonceSize + 1 + 2*sha256.Size
	// replayCacheSize bounds how many unexpired tokens the server remembers. A
	// debugger needs a handful; when full, new connections are refused rather
	// than forgetting tokens that could still be replayed.
	replayCacheSize = 4096
)

var (
	errTokenMalformed  = errors.New("malformed token")
	errTokenInvalid    = errors.New("invalid token")
	errTokenExpired    = errors.New("token expired or from the future, check client and server clocks")
	errTokenReplayed   = errors.New("token already used")
	errReplayCacheFull = errors.New("too many recent connections, try again in a minute")
)

// tokenID identifies a token for replay detection. It holds decoded values, so
// different spellings of one token map to the same ID.
type tokenID struct {
	unix  int64
	nonce [nonceSize]byte
}

// newSecret returns a random key. The server makes a new one on every start.
func newSecret() []byte {
	secret := make([]byte, secretSize)
	rand.Read(secret) // never fails since Go 1.24
	return secret
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

// readSecret returns the key from the JRDWP_KEY environment variable if it is
// set, even to an empty value, otherwise from the file at path.
func readSecret(path string) ([]byte, error) {
	if text, ok := os.LookupEnv(keyEnv); ok {
		return parseSecret(text, keyEnv)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSecret(string(data), path)
}

// parseSecret decodes a key. The error names where the key came from but never
// includes it.
func parseSecret(text, source string) ([]byte, error) {
	secret, err := hex.DecodeString(strings.TrimSpace(text))
	if err != nil || len(secret) != secretSize {
		return nil, fmt.Errorf("bad key in %s, copy it again from a server running this version", source)
	}
	return secret, nil
}

// makeToken returns "<unix seconds>.<hex nonce>.<hex HMAC>". The HMAC covers the
// JDWP port, so a token only opens the port it was made for, and the random
// nonce makes every token unique, so the server can refuse replays.
func makeToken(secret []byte, jdwpPort int, now time.Time) string {
	nonce := make([]byte, nonceSize)
	rand.Read(nonce) // never fails since Go 1.24
	ts := strconv.FormatInt(now.Unix(), 10)
	nonceHex := hex.EncodeToString(nonce)
	return ts + "." + nonceHex + "." + hex.EncodeToString(tokenMAC(secret, ts, nonceHex, jdwpPort))
}

// verifyToken checks a token's format, MAC and age, and returns its ID for
// replay detection.
func verifyToken(secret []byte, token string, jdwpPort int, now time.Time) (tokenID, error) {
	if len(token) > maxTokenLen {
		return tokenID{}, errTokenMalformed
	}
	ts, rest, _ := strings.Cut(token, ".")
	nonceHex, macHex, _ := strings.Cut(rest, ".")
	if len(nonceHex) != 2*nonceSize || len(macHex) != 2*sha256.Size {
		return tokenID{}, errTokenMalformed
	}
	var id tokenID
	var err error
	if id.unix, err = strconv.ParseInt(ts, 10, 64); err != nil {
		return tokenID{}, errTokenMalformed
	}
	if _, err := hex.Decode(id.nonce[:], []byte(nonceHex)); err != nil {
		return tokenID{}, errTokenMalformed
	}
	mac, err := hex.DecodeString(macHex)
	if err != nil {
		return tokenID{}, errTokenMalformed
	}
	if !hmac.Equal(mac, tokenMAC(secret, ts, nonceHex, jdwpPort)) {
		return tokenID{}, errTokenInvalid
	}
	if expired(id.unix, now) || id.unix > now.Unix()+tokenMaxSkew {
		return tokenID{}, errTokenExpired
	}
	return id, nil
}

// expired reports whether a token made at unix is too old to accept. It
// compares instead of subtracting, because unix comes from the peer and could
// overflow.
func expired(unix int64, now time.Time) bool {
	return unix < now.Unix()-tokenMaxSkew
}

func tokenMAC(secret []byte, ts, nonceHex string, jdwpPort int) []byte {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s|%s|%d", ts, nonceHex, jdwpPort)
	return mac.Sum(nil)
}

// replayCache remembers accepted tokens until they expire, so each one opens at
// most one connection. The zero value is ready to use.
type replayCache struct {
	mu   sync.Mutex
	seen map[tokenID]struct{}
}

// add records a verified token. Check and insert happen under one lock, so two
// simultaneous copies of a token can't both get through.
func (c *replayCache) add(id tokenID, now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.seen[id]; ok {
		return errTokenReplayed
	}
	if len(c.seen) >= replayCacheSize {
		for old := range c.seen {
			if expired(old.unix, now) {
				delete(c.seen, old)
			}
		}
	}
	if len(c.seen) >= replayCacheSize {
		return errReplayCacheFull
	}
	if c.seen == nil {
		c.seen = make(map[tokenID]struct{})
	}
	c.seen[id] = struct{}{}
	return nil
}
