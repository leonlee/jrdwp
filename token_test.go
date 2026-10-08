package main

import (
	"bytes"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestVerifyToken(t *testing.T) {
	secret := bytes.Repeat([]byte{7}, secretSize)
	now := time.Unix(1_700_000_000, 0)
	fresh := makeToken(secret, 5005, now)
	nonce := strings.Repeat("ab", nonceSize)
	signed := func(ts string) string {
		return ts + "." + nonce + "." + hex.EncodeToString(tokenMAC(secret, ts, nonce, 5005))
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
		{"other secret", makeToken(bytes.Repeat([]byte{8}, secretSize), 5005, now), 5005, errTokenInvalid},
		{"v0.2.0 format", "1700000000." + hex.EncodeToString(tokenMAC(secret, "1700000000", "", 5005)), 5005, errTokenMalformed},
		{"short nonce", "1700000000.abcd." + hex.EncodeToString(tokenMAC(secret, "1700000000", "abcd", 5005)), 5005, errTokenMalformed},
		{"empty", "", 5005, errTokenMalformed},
		{"garbage", "not.a.token", 5005, errTokenMalformed},
		{"padded timestamp", signed("0000000000000000001700000000"), 5005, errTokenMalformed},
		{"short MAC", fresh[:len(fresh)-2], 5005, errTokenMalformed},
		{"extra field", fresh + ".00", 5005, errTokenMalformed},
		{"many dots", strings.Repeat(".", 1<<20), 5005, errTokenMalformed},
	}
	for _, tt := range tests {
		if _, got := verifyToken(secret, tt.token, tt.port, now); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}

	// A header near 1 MiB from an unauthenticated peer must be rejected without
	// allocating in proportion to its size.
	huge := strings.Repeat(".", 1<<20)
	if n := testing.AllocsPerRun(10, func() { verifyToken(secret, huge, 5005, now) }); n > 0 {
		t.Errorf("rejecting a 1 MiB token allocates %v times, want 0", n)
	}

	again := makeToken(secret, 5005, now)
	if again == fresh {
		t.Error("two tokens made in the same second are identical")
	}
	// Hex decoding ignores case, so an upper-cased MAC still verifies. It must
	// map to the same ID, or it would dodge the replay cache.
	freshID, _ := verifyToken(secret, fresh, 5005, now)
	ts, nonceAndMAC, _ := strings.Cut(fresh, ".")
	nonceHex, macHex, _ := strings.Cut(nonceAndMAC, ".")
	upperID, err := verifyToken(secret, ts+"."+nonceHex+"."+strings.ToUpper(macHex), 5005, now)
	if err != nil || upperID != freshID {
		t.Errorf("upper-cased MAC: got %v, %v, want the original token's ID", upperID, err)
	}
}

func TestReplayCache(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	id := func(unix int64, n int) tokenID {
		var nonce [nonceSize]byte
		nonce[0], nonce[1] = byte(n), byte(n>>8)
		return tokenID{unix: unix, nonce: nonce}
	}

	var c replayCache
	if err := c.add(id(now.Unix(), 0), now); err != nil {
		t.Fatal(err)
	}
	if err := c.add(id(now.Unix(), 0), now); err != errTokenReplayed {
		t.Errorf("replay: got %v, want %v", err, errTokenReplayed)
	}
	if err := c.add(id(now.Unix(), 1), now); err != nil {
		t.Errorf("second nonce in the same second: %v", err)
	}

	for n := 2; n < replayCacheSize; n++ {
		if err := c.add(id(now.Unix(), n), now); err != nil {
			t.Fatal(err)
		}
	}
	// Full of tokens that are still valid at the inclusive expiry boundary.
	boundary := now.Add(tokenMaxSkew * time.Second)
	if err := c.add(id(boundary.Unix(), 0), boundary); err != errReplayCacheFull {
		t.Errorf("full cache: got %v, want %v", err, errReplayCacheFull)
	}
	if err := c.add(id(now.Unix(), 0), boundary); err != errTokenReplayed {
		t.Errorf("replay at the expiry boundary: got %v, want %v", err, errTokenReplayed)
	}
	// One second later all of them have expired and make room.
	later := boundary.Add(time.Second)
	if err := c.add(id(later.Unix(), 0), later); err != nil {
		t.Errorf("after expiry: %v", err)
	}
}

func TestReplayCacheConcurrentDuplicates(t *testing.T) {
	var c replayCache
	now := time.Now()
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.add(tokenID{unix: now.Unix()}, now) == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if n := accepted.Load(); n != 1 {
		t.Errorf("%d copies of one token accepted, want 1", n)
	}
}

func TestReadSecretFromEnv(t *testing.T) {
	secret := newSecret()
	t.Setenv(keyEnv, hex.EncodeToString(secret)+"\n")
	got, err := readSecret(filepath.Join(t.TempDir(), "missing"))
	if err != nil || !bytes.Equal(got, secret) {
		t.Errorf("read %x, %v, want %x", got, err, secret)
	}

	for _, bad := range []string{"", "not-hex", "abcd"} {
		t.Setenv(keyEnv, bad)
		_, err := readSecret(filepath.Join(t.TempDir(), "missing"))
		if err == nil || !strings.Contains(err.Error(), keyEnv) {
			t.Errorf("%s=%q: got %v, want an error naming %s", keyEnv, bad, err, keyEnv)
		} else if bad != "" && strings.Contains(err.Error(), bad) {
			t.Errorf("error repeats the key: %v", err)
		}
	}
}

func TestWriteSecretReplacesKeyFile(t *testing.T) {
	t.Setenv(keyEnv, "") // restores any value set outside the test
	os.Unsetenv(keyEnv)
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

	secret := newSecret()
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
