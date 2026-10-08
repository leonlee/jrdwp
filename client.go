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

// client accepts debugger connections and tunnels each one to the jrdwp server.
type client struct {
	url      string
	origin   string
	jdwpPort int
	secret   []byte
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
	c := &client{url: wsURL.String(), origin: conf.wsOrigin, jdwpPort: conf.jdwpPort, secret: secret}

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
	pipe(ws, conn)
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
