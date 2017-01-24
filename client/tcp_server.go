package client

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/gorilla/websocket"
	"github.com/leonlee/jrdwp/common"
)

//TCPServer for listenning JDWP client
type TCPServer struct {
	client *WSClient
	port   int
}

//NewTCPServer create new server
func NewTCPServer(wsClient *WSClient, port int) *TCPServer {
	return &TCPServer{
		client: wsClient,
		port:   port,
	}
}

//Start start tcp server
func (server *TCPServer) Start() error {
	addr := fmt.Sprintf(":%d", server.port)
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		log.Println("can't resolve addr", addr, err.Error())
		return err
	}

	err = server.listen(tcpAddr)
	if err != nil {
		log.Println(err)
		return err
	}

	return nil
}

func (server *TCPServer) listen(addr *net.TCPAddr) error {
	log.Printf("starting tcp server: %s\n", addr.String())

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		log.Println(err)
		return err
	}

	defer server.recoverListen(listener, addr)

	return server.accept(listener)
}

func (server *TCPServer) recoverListen(listener *net.TCPListener, addr *net.TCPAddr) {
	err := recover()
	if err != nil {
		log.Printf("got panic by: %v\n", err)
	}

	if listener != nil {
		log.Println("closing broken listener")
		listener.Close()
	}

	server.listen(addr)
}

func (server *TCPServer) accept(listener *net.TCPListener) error {
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Println(err)
			return err
		}
		server.handle(conn)
	}
}

func (server *TCPServer) handle(conn *net.TCPConn) {
	var clientConn *websocket.Conn
	defer closeOnFail(conn, clientConn)

	log.Printf("got tcp connection from %s.\r\n", conn.RemoteAddr())

	clientConn, err := server.client.Connect()
	if err != nil {
		log.Printf("can't connect to server %s", err.Error())
	}

	common.InitTCPConn(conn)

	go send(conn, clientConn)
	go receive(conn, clientConn)
}

func closeOnFail(conn *net.TCPConn, clientConn *websocket.Conn) {
	if err := recover(); err != nil {
		log.Println("got err in handle", err)

		if conn != nil {
			conn.Close()
		}
		if clientConn != nil {
			clientConn.Close()
		}
	}

}

func send(conn *net.TCPConn, clientConn *websocket.Conn) {
	defer cleanConn(conn, clientConn)

	buffer := make([]byte, 256)
	read := 0
	var err error
	for {
		if clientConn == nil || conn == nil {
			return
		}

		conn.SetReadDeadline(time.Now().Add(common.DeadlineDuration))
		read, err = conn.Read(buffer)
		if err != nil {
			log.Printf("read tcp fialed: %v\n", err.Error())
			return
		}

		err = clientConn.WriteMessage(websocket.BinaryMessage, buffer[:read])
		if err != nil {
			log.Printf("write ws failed: %v\n", err.Error())
			return
		}
	}
}

func receive(conn *net.TCPConn, clientConn *websocket.Conn) {
	defer cleanConn(conn, clientConn)

	var buffer []byte
	var err error
	for {
		if clientConn == nil || conn == nil {
			return
		}

		_, buffer, err = clientConn.ReadMessage()
		if err != nil {
			log.Printf("read ws fialed: %v\n", err.Error())
			return
		}

		conn.SetWriteDeadline(time.Now().Add(common.DeadlineDuration))
		_, err := conn.Write(buffer)
		if err != nil {
			log.Printf("write tcp failed: %v\n", err.Error())
			return
		}
	}
}

func cleanConn(conn *net.TCPConn, clientConn *websocket.Conn) {
	log.Println("connection was disconnected")

	if err := recover(); err != nil {
		log.Println("recover from", err)
	}

	if conn != nil {
		conn.Close()
	}
	if clientConn != nil {
		clientConn.Close()
	}
}
