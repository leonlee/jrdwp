package main

import (
	"bytes"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("test-secret-test-secret-test-sec")

// fakeJVM writes greeting to each connection, then echoes. closed gets a value
// whenever a connection ends.
type fakeJVM struct {
	port   int
	closed chan struct{}
}

func startJVM(t *testing.T, greeting string) *fakeJVM {
	t.Helper()
	ln := listen(t)
	jvm := &fakeJVM{port: ln.Addr().(*net.TCPAddr).Port, closed: make(chan struct{}, 100)}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() {
					conn.Close()
					jvm.closed <- struct{}{}
				}()
				if _, err := io.WriteString(conn, greeting); err == nil {
					io.Copy(conn, conn)
				}
			}()
		}
	}()
	return jvm
}

// startServer returns the websocket URL of a jrdwp server.
func startServer(t *testing.T, allowedPorts []int) string {
	t.Helper()
	srv := httptest.NewServer(&server{jvmHost: "127.0.0.1", allowedPorts: allowedPorts, secret: testSecret})
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// startClient returns the address where a debugger connects to a jrdwp client.
func startClient(t *testing.T, wsURL string, jdwpPort int) string {
	t.Helper()
	ln := listen(t)
	go (&client{url: wsURL, jdwpPort: jdwpPort, secret: testSecret}).serve(ln)
	return ln.Addr().String()
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

func dialDebugger(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	return conn
}

func TestRoundTrip(t *testing.T) {
	jvm := startJVM(t, "")
	conn := dialDebugger(t, startClient(t, startServer(t, []int{jvm.port}), jvm.port))

	payload := make([]byte, 200_000)
	rand.Read(payload)
	go conn.Write(payload)
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("payload changed in transit")
	}
}

func TestConcurrentSessionsReachTheirOwnPort(t *testing.T) {
	jvmA, jvmB := startJVM(t, "A"), startJVM(t, "B")
	wsURL := startServer(t, []int{jvmA.port, jvmB.port})
	addrA, addrB := startClient(t, wsURL, jvmA.port), startClient(t, wsURL, jvmB.port)

	var conns []net.Conn
	var wants []string
	for i := 0; i < 10; i++ {
		conns = append(conns, dialDebugger(t, addrA), dialDebugger(t, addrB))
		wants = append(wants, "A", "B")
	}
	for i, conn := range conns {
		got := make([]byte, 1)
		if _, err := io.ReadFull(conn, got); err != nil {
			t.Fatal(err)
		}
		if string(got) != wants[i] {
			t.Errorf("session %d reached JVM %s, want %s", i, got, wants[i])
		}
	}
}

func TestRejectsUnauthorized(t *testing.T) {
	jvm := startJVM(t, "")
	wsURL := startServer(t, []int{jvm.port})
	clients := map[string]*client{
		"wrong secret":     {url: wsURL, jdwpPort: jvm.port, secret: []byte("wrong")},
		"port not allowed": {url: wsURL, jdwpPort: jvm.port + 1, secret: testSecret},
	}
	for name, c := range clients {
		if _, err := c.dial(); err == nil || !strings.Contains(err.Error(), "403") {
			t.Errorf("%s: got %v, want 403", name, err)
		}
	}
}

func TestJVMDownDisconnectsDebugger(t *testing.T) {
	ln := listen(t)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	wsURL := startServer(t, []int{port})

	if _, err := (&client{url: wsURL, jdwpPort: port, secret: testSecret}).dial(); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("got %v, want 502", err)
	}
	conn := dialDebugger(t, startClient(t, wsURL, port))
	if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("debugger read got %v, want EOF", err)
	}
}

func TestFailedUpgradeClosesJVMConnection(t *testing.T) {
	jvm := startJVM(t, "")
	wsURL := startServer(t, []int{jvm.port})

	// Authorized, but not a websocket handshake.
	req, err := http.NewRequest(http.MethodGet, "http"+strings.TrimPrefix(wsURL, "ws"), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(headerToken, makeToken(testSecret, jvm.port, time.Now()))
	req.Header.Set(headerPort, strconv.Itoa(jvm.port))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %s, want 400", resp.Status)
	}

	select {
	case <-jvm.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("JVM connection left open after failed upgrade")
	}
}

func TestServerBindFailureKeepsKey(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	if err := os.WriteFile(keyFile, []byte("live\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	busy := listen(t)
	conf := config{bindHost: "127.0.0.1", bindPort: busy.Addr().(*net.TCPAddr).Port, wsPath: "/jrdwp", serverDeadline: 1}
	if err := runServer(conf); err == nil {
		t.Fatal("runServer started on a busy port")
	}
	if got, _ := os.ReadFile(keyFile); string(got) != "live\n" {
		t.Errorf("key file is %q, want the running server's key kept", got)
	}
}
