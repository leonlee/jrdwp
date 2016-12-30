package client

import (
	"fmt"
	"log"
	"net"

	"github.com/gorilla/websocket"
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
func (server *TCPServer) Start() {
	addr := fmt.Sprintf(":%d", server.port)

	err := server.listen(addr)
	if err != nil {
		log.Fatal(err)
	}

}

func (server *TCPServer) listen(addr string) error {
	log.Printf("starting tcp server: %s\n", addr)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
		return err
	}
	defer server.recoverListen(listener, addr)

	return server.accept(listener)
}

func (server *TCPServer) recoverListen(listener net.Listener, addr string) {
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

func (server *TCPServer) accept(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			return err
		}
		server.handle(conn)
	}
}

func (server *TCPServer) handle(conn net.Conn) {
	var clientConn *websocket.Conn

	defer func() {
		if err := recover(); err != nil {
			log.Println("got err in handle", err)

			if conn != nil {
				conn.Close()
			}
			if clientConn != nil {
				clientConn.Close()
			}
		}
	}()

	log.Printf("got tcp connection from %s.\r\n", conn.RemoteAddr())

	clientConn, err := server.client.Connect()
	if err != nil {
		log.Printf("can't connect to server %s", err.Error())
	}

	go send(conn, clientConn)
	go receive(conn, clientConn)
}

func send(conn net.Conn, clientConn *websocket.Conn) {
	defer closeConn(conn, clientConn)

	buffer := make([]byte, 256)
	read := 0
	var err error
	for {
		if clientConn == nil || conn == nil {
			return
		}

		//conn.SetReadDeadline(time.Now().Add(time.Second * 50))
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

func receive(conn net.Conn, clientConn *websocket.Conn) {
	defer closeConn(conn, clientConn)

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

		//conn.SetWriteDeadline(time.Now().Add(time.Second * 50))
		_, err := conn.Write(buffer)
		if err != nil {
			log.Printf("write tcp failed: %v\n", err.Error())
			return
		}
	}
}

func closeConn(conn net.Conn, clientConn *websocket.Conn) {
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
