package client

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/leonlee/jrdwp/common"
)

var (
	//ErrConnFailed connection failure
	ErrConnFailed = errors.New("can't connect to ws server")
)

//WSClient websocket client
type WSClient struct {
	host     string
	port     int
	path     string
	origin   string
	scheme   string
	key      *rsa.PublicKey
	jdwpPort int
}

//NewWSClient create WsClient
func NewWSClient(host string, port int, path string, origin string, jdwpPort int, key *rsa.PublicKey) *WSClient {
	return &WSClient{
		host:     host,
		port:     port,
		path:     path,
		origin:   origin,
		scheme:   "ws",
		key:      key,
		jdwpPort: jdwpPort,
	}
}

//Connect connect to remote websocket server
func (client *WSClient) Connect() (*websocket.Conn, error) {
	url := url.URL{
		Scheme: client.scheme,
		Host:   fmt.Sprintf("%s:%d", client.host, client.port),
		Path:   client.path,
	}

	log.Printf("connecting ws server: %s\n", url.String())

	header := client.assembleHeader()

	conn, _, err := websocket.DefaultDialer.Dial(url.String(), header)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	return conn, err
}

func (client *WSClient) assembleHeader() http.Header {
	header := http.Header{}
	token := common.GenerateKey(client.key)
	header.Add(common.HeaderToken, token)
	header.Add(common.HeaderPort, strconv.Itoa(client.jdwpPort))

	return header
}
