package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// keepAliveInterval keeps sessions idle at a breakpoint alive through
	// proxies that drop connections after 60 idle seconds, the nginx and AWS
	// load balancer default.
	keepAliveInterval = 30 * time.Second
	pingTimeout       = 10 * time.Second
)

// client accepts debugger connections and tunnels each one to the jrdwp server.
type client struct {
	url          string
	origin       string
	jdwpPort     int
	secret       []byte
	pingInterval time.Duration
}

func runClient(conf config) error {
	secret, err := readSecret(keyFile)
	if err != nil {
		return err
	}
	wsURL := url.URL{
		Scheme: conf.wsScheme,
		Host:   net.JoinHostPort(conf.serverHost, strconv.Itoa(conf.serverPort)),
		Path:   conf.wsPath,
	}
	c := &client{
		url:          wsURL.String(),
		origin:       conf.wsOrigin,
		jdwpPort:     conf.jdwpPort,
		secret:       secret,
		pingInterval: keepAliveInterval,
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(conf.bindHost, strconv.Itoa(conf.bindPort)))
	if err != nil {
		return err
	}
	log.Printf("waiting for debugger on %s, tunneling to %s", ln.Addr(), c.url)
	return c.serve(ln)
}

func (c *client) serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go c.handle(conn)
	}
}

func (c *client) handle(conn net.Conn) {
	log.Printf("debugger connected from %s", conn.RemoteAddr())
	ws, err := c.dial()
	if err != nil {
		log.Printf("can't connect to %s: %v", c.url, err)
		conn.Close()
		return
	}
	stop := keepAlive(ws, c.pingInterval)
	pipe(ws, conn)
	stop()
}

// keepAlive pings the server every interval until stop is called. The server's
// default ping handler answers with a pong, so both directions see traffic. A
// failed ping closes ws, which ends the session. stop waits for the pinging
// goroutine to exit.
func keepAlive(ws *websocket.Conn, interval time.Duration) (stop func()) {
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(pingTimeout)); err != nil {
					log.Printf("keepalive ping failed: %v", err)
					ws.Close()
					return
				}
			}
		}
	}()
	return func() {
		close(done)
		<-exited
	}
}

func (c *client) dial() (*websocket.Conn, error) {
	header := http.Header{}
	header.Set(headerToken, makeToken(c.secret, c.jdwpPort, time.Now()))
	header.Set(headerPort, strconv.Itoa(c.jdwpPort))
	if c.origin != "" {
		header.Set("Origin", c.origin)
	}
	ws, resp, err := websocket.DefaultDialer.Dial(c.url, header)
	if err != nil && resp != nil {
		return nil, fmt.Errorf("%w (%s)", err, resp.Status)
	}
	return ws, err
}
