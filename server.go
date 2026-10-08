package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

const (
	headerToken    = "jrdwptoken"
	headerPort     = "jdwpport"
	jvmDialTimeout = 10 * time.Second
)

const goodbye = `
 _________________________________________
< Bug's life was short, long live Gopher! >
 -----------------------------------------
        \   ^__^
         \  (oo)\_______
            (__)\       )\/\
                ||----w |
                ||     ||
`

// Requests are authenticated by token, and browsers can't set the token header,
// so the default Origin check adds nothing. It would also reject clients behind
// a proxy that rewrites Host.
var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// server authenticates websocket requests and tunnels each one to a JVM port.
type server struct {
	jvmHost      string
	allowedPorts []int
	secret       []byte
}

func runServer(conf config) error {
	// Bind first: a server that can't start must not replace a running server's key.
	ln, err := net.Listen("tcp", net.JoinHostPort(conf.bindHost, strconv.Itoa(conf.bindPort)))
	if err != nil {
		return err
	}
	defer ln.Close()

	secret, err := newSecret()
	if err != nil {
		return err
	}
	if err := writeSecret(keyFile, secret); err != nil {
		return err
	}
	log.Printf("wrote a new key to %s, copy it to the client's working directory", keyFile)

	mux := http.NewServeMux()
	mux.Handle(conf.wsPath, &server{jvmHost: conf.serverHost, allowedPorts: conf.allowedPorts, secret: secret})
	log.Printf("serving websocket on %s%s", ln.Addr(), conf.wsPath)

	time.AfterFunc(time.Duration(conf.serverDeadline)*time.Minute, func() {
		log.Print(goodbye)
		os.Exit(0)
	})
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	return httpServer.Serve(ln)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	port, err := s.authorize(r)
	if err != nil {
		log.Printf("rejected %s: %v", r.RemoteAddr, err)
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	jvmAddr := net.JoinHostPort(s.jvmHost, strconv.Itoa(port))
	jvm, err := net.DialTimeout("tcp", jvmAddr, jvmDialTimeout)
	if err != nil {
		log.Printf("can't connect to JVM at %s: %v", jvmAddr, err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has already replied with an HTTP error.
		log.Printf("websocket upgrade from %s failed: %v", r.RemoteAddr, err)
		jvm.Close()
		return
	}
	log.Printf("%s attached to JVM at %s", r.RemoteAddr, jvmAddr)
	pipe(ws, jvm)
}

func (s *server) authorize(r *http.Request) (int, error) {
	port, err := strconv.Atoi(r.Header.Get(headerPort))
	if err != nil {
		return 0, fmt.Errorf("bad %s header: %w", headerPort, err)
	}
	if !slices.Contains(s.allowedPorts, port) {
		return 0, fmt.Errorf("JDWP port %d is not allowed", port)
	}
	if err := verifyToken(s.secret, r.Header.Get(headerToken), port, time.Now()); err != nil {
		return 0, err
	}
	return port, nil
}
