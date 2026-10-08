package main

import (
	"io"
	"log"
	"net"

	"github.com/gorilla/websocket"
)

// pipe relays bytes between a websocket and a TCP connection until either side
// fails, then closes both. It returns once both directions have stopped.
func pipe(ws *websocket.Conn, tcp net.Conn) {
	errs := make(chan error, 2)
	go func() { errs <- wsToTCP(ws, tcp) }()
	go func() { errs <- tcpToWS(tcp, ws) }()

	err := <-errs
	ws.Close()
	tcp.Close()
	<-errs
	log.Printf("connection %s closed: %v", tcp.RemoteAddr(), err)
}

// wsToTCP streams each message rather than buffering it whole, so a peer can't
// make us allocate an arbitrarily large message.
func wsToTCP(ws *websocket.Conn, tcp net.Conn) error {
	for {
		_, msg, err := ws.NextReader()
		if err != nil {
			return err
		}
		if _, err := io.Copy(tcp, msg); err != nil {
			return err
		}
	}
}

func tcpToWS(tcp net.Conn, ws *websocket.Conn) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := tcp.Read(buf)
		if n > 0 {
			if err := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
}
