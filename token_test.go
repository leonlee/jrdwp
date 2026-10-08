package main

import (
	"bytes"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVerifyToken(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_700_000_000, 0)
	fresh := makeToken(secret, 5005, now)
	signed := func(ts string) string {
		return ts + "." + hex.EncodeToString(tokenMAC(secret, ts, 5005))
	}

	tests := []struct {
		name  string
		token string
		port  int
		want  error
	}{
		{"fresh", fresh, 5005, nil},
		{"60s old", makeToken(secret, 5005, now.Add(-60*time.Second)), 5005, nil},
		{"60s ahead", makeToken(secret, 5005, now.Add(60*time.Second)), 5005, nil},
		{"61s old", makeToken(secret, 5005, now.Add(-61*time.Second)), 5005, errTokenExpired},
		{"61s ahead", makeToken(secret, 5005, now.Add(61*time.Second)), 5005, errTokenExpired},
		{"a year ahead", makeToken(secret, 5005, now.AddDate(1, 0, 0)), 5005, errTokenExpired},
		{"max int64 timestamp", signed("9223372036854775807"), 5005, errTokenExpired},
		{"min int64 timestamp", signed("-9223372036854775808"), 5005, errTokenExpired},
		{"other port", fresh, 5006, errTokenInvalid},
		{"other secret", makeToken([]byte("other"), 5005, now), 5005, errTokenInvalid},
		{"empty", "", 5005, errTokenMalformed},
		{"garbage", "not.a-token", 5005, errTokenMalformed},
	}
	for _, tt := range tests {
		if got := verifyToken(secret, tt.token, tt.port, now); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestWriteSecretReplacesKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, keyFile)
	old := "-----BEGIN PUBLIC KEY-----\n"
	if err := os.WriteFile(path, []byte(old), 0o655); err != nil {
		t.Fatal(err)
	}
	if _, err := readSecret(path); err == nil {
		t.Error("read a key file from an older version, want error")
	}
	// Another user opened the old world-readable file and kept it open.
	stale, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Close()

	secret, err := newSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeSecret(path, secret); err != nil {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(stale); string(got) != old {
		t.Errorf("old handle reads %q, want the old contents", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("%d files in key dir, want 1 (temp file left behind?)", len(entries))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("key file mode %o, want 600", mode)
	}
	got, err := readSecret(path)
	if err != nil || !bytes.Equal(got, secret) {
		t.Errorf("read back %x, %v, want %x", got, err, secret)
	}
}
