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

func (client *TCPClient) Connect(port int) (*net.TCPConn, error) {
	addr := fmt.Sprintf("%s:%d", client.server, port)
	log.Printf("connecting tcp server: %s\n", addr)

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		log.Panicln("can't resolve addr", addr, err.Error())
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		log.Panicf("can't connect to %s:%d : %v", client.server, port, err.Error())
	}

	return conn, err
}
