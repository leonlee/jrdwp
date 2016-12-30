package server

import (
	"fmt"
	"log"
	"net"
)

type TCPClient struct {
	server string
}

func NewTCPClient(server string) *TCPClient {
	return &TCPClient{
		server: server,
	}
}

func (client *TCPClient) Connect(port int) (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", client.server, port)
	log.Printf("connecting tcp server: %s\n", addr)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Panicf("can't connect to %s:%d : %v", client.server, port, err.Error())
	}

	return conn, err
}
