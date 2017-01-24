package server

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/leonlee/jrdwp/common"
)

type WSServer struct {
	port      int
	path      string
	origin    string
	client    *TCPClient
	key       *rsa.PrivateKey
	jdwpPorts []int
	jdwpPort  int
}

func NewWSServer(path string, origin string, port int, jdwpPorts []int, client *TCPClient) *WSServer {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return &WSServer{
		port:      port,
		path:      path,
		origin:    origin,
		client:    client,
		jdwpPorts: jdwpPorts,
	}
}

func (server *WSServer) Start() {
	if err := initKey(server); err != nil {
		log.Fatalln("can't start ws server", err.Error())
	}

	addr := fmt.Sprintf(":%d", server.port)
	log.Printf("starting ws server: %s\n", addr)
	log.Printf("ws path: %s\n", server.path)
	server.listen(addr)

	defer func() {
		if err := recover(); err != nil {
			log.Println("recovering from:", err)
			server.listen(addr)
		}
	}()
}

func initKey(server *WSServer) error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Println("can't generate key", err.Error())
		return err
	}
	server.key = privateKey

	publicKeyBytes, err := common.PublicKeyToBytes(&privateKey.PublicKey)
	if err != nil {
		log.Println("can't convert public key to bytes", err.Error())
		return err
	}
	log.Printf("generated public key:\n\n%s\n", publicKeyBytes)

	err = ioutil.WriteFile(common.PublicKeyPath(), publicKeyBytes, 0655)
	if err != nil {
		log.Println("can't write public key", common.PublicKeyPath(), err.Error())
		return err
	}

	return nil
}

func (server *WSServer) listen(addr string) {
	http.HandleFunc(server.path, server.onMessage)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func (server *WSServer) onMessage(w http.ResponseWriter, request *http.Request) {
	var conn *websocket.Conn
	var clientConn *net.TCPConn

	defer closeOnFail(conn, clientConn)

	var upgrader = websocket.Upgrader{
		CheckOrigin: func(req *http.Request) bool {
			return server.verifyRequest(req)
		},
	}

	conn, err := upgrader.Upgrade(w, request, nil)
	if err != nil {
		log.Panicln("upgrade:", err)
	}

	clientConn, err = server.client.Connect(server.jdwpPort)
	if err != nil {
		log.Panicln("can't connect to jvm:", err)
	}

	common.InitTCPConn(clientConn)

	go send(conn, clientConn)
	go receive(conn, clientConn)
}

func closeOnFail(conn *websocket.Conn, clientConn *net.TCPConn) {
	if err := recover(); err != nil {
		log.Println("recover from", err)
		if conn != nil {
			conn.Close()
		}
		if clientConn != nil {
			clientConn.Close()
		}
	}
}

func (server *WSServer) verifyRequest(req *http.Request) bool {
	port, err := strconv.Atoi(req.Header.Get(common.HeaderPort))
	if err != nil {
		log.Println("bad port", err.Error())
		return false
	}
	log.Println("got port", port)

	if !server.allowJdwpPort(port) {
		log.Println("not allowed port", port)
		return false
	}

	token := req.Header.Get(common.HeaderToken)
	if common.VerifyToken(server.key, token) {
		return true
	} else {
		log.Println("bad token", token)
		return false
	}
}

func (server *WSServer) allowJdwpPort(port int) bool {
	for _, jdwpPort := range server.jdwpPorts {
		if port == jdwpPort {
			server.jdwpPort = port
			return true
		}
	}
	return false
}

func send(conn *websocket.Conn, clientConn *net.TCPConn) {
	defer cleanConn(conn, clientConn)

	var buffer []byte
	var err error
	for {
		if clientConn == nil || conn == nil {
			return
		}

		_, buffer, err = conn.ReadMessage()
		if err != nil {
			log.Printf("read ws failed: %v\n", err.Error())
			return
		}

		_, err = clientConn.Write(buffer)
		if err != nil {
			log.Printf("write tcp failed: %v\n", err.Error())
			return
		}
	}
}

func receive(conn *websocket.Conn, clientConn *net.TCPConn) {
	defer cleanConn(conn, clientConn)

	buffer := make([]byte, 256)
	var read = 0
	var err error
	for {
		if clientConn == nil || conn == nil {
			return
		}

		read, err = clientConn.Read(buffer)
		if err != nil {
			log.Printf("read tcp failed: %v\n", err.Error())
			return
		}

		err = conn.WriteMessage(websocket.BinaryMessage, buffer[:read])
		if err != nil {
			log.Printf("write ws failed: %v\n", err.Error())
			return
		}
	}
}

func cleanConn(conn *websocket.Conn, clientConn *net.TCPConn) {
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
