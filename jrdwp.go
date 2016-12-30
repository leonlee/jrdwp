package main

import (
	"crypto/rsa"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"

	"github.com/leonlee/jrdwp/client"
	"github.com/leonlee/jrdwp/common"
	"github.com/leonlee/jrdwp/server"
)

const (
	//ModeClient client mode
	ModeClient = "client"
	//ModeServer server mode
	ModeServer = "server"
	//PortListen default port of tcp server on client or ws server on remote server
	PortListen = 9876
	//PortServer default port of ws server on client or JDWP port on remote server
	PortServer = 9877
	//WsPath default websocket server path
	WsPath = "jrdwp"
	//DefaultJdwpPorts default enabled ports
	DefaultJdwpPorts = "5005"
)

//Config configuration struct
type Config struct {
	Mode             *string        `json:"mode"`
	BindHost         *string        `json:"bindHost"`
	BindPort         *int           `json:"bindPort"`
	ServerHost       *string        `json:"serverHost"`
	ServerPort       *int           `json:"serverPort"`
	WsPath           *string        `json:"wsPath"`
	WsOrigin         *string        `json:"wsOrigin"`
	AllowedJdwpPorts []int          `json:"allowedJdwpPorts"`
	JdwpPort         *int           `json:"jdwpPort"`
	PublicKey        *rsa.PublicKey `json:"clientKey"`
}

func (conf *Config) String() string {
	confJSON, err := json.Marshal(*conf)
	if err != nil {
		return err.Error()
	}
	return string(confJSON)
}

func main() {
	conf := parseFlags()
	start(conf)
}

func loadKey(conf *Config) {
	bytes, err := ioutil.ReadFile(common.PublicKeyPath())
	if err != nil {
		log.Fatalln("Can't reade public key", err.Error())
	}

	conf.PublicKey = common.ParsePublicKey(bytes)

	publicKeyBytes, err := common.PublicKeyToBytes(conf.PublicKey)
	if err != nil {
		log.Fatalln("can't parse public key", err.Error())
	}

	log.Printf("loaded key\n\n%s\n", string(publicKeyBytes))
}

func parseFlags() *Config {
	log.Printf("initializing with %v ...", flag.Args())

	conf := &Config{}
	conf.Mode = flag.String("mode", ModeClient, "jrdwp mode, \"client\" or \"server\"")
	conf.BindHost = flag.String("bind-host", "", "bind host, default ''")
	conf.BindPort = flag.Int("bind-port", PortListen, "bind port, default 9876")
	conf.ServerHost = flag.String("server-host", "", "server host")
	conf.ServerPort = flag.Int("server-port", PortServer, "server port, default 9877")
	conf.WsPath = flag.String("ws-path", WsPath, "websocket server path")
	conf.WsOrigin = flag.String("ws-origin", "", "websocket request origin header")
	conf.JdwpPort = flag.Int("jdwp-port", -1, "jdwp port of remote application")
	jdwpPortsText := flag.String("allowed-jdwp-ports", "", "allowed jdwp ports likes: \"5005,5006\"")
	flag.Parse()

	if *conf.Mode == ModeServer {
		conf.AllowedJdwpPorts = common.SplitToInt(*jdwpPortsText)
	}

	if *conf.WsPath == "" {
		log.Fatal("invalid ws-path")
	}

	if *conf.WsOrigin == "" {
		log.Fatal("invalid ws-origin")
	}

	log.Printf("initialized by %s \n", conf)

	return conf
}

func start(conf *Config) {
	log.Println("starting jrdwp...")

	if *conf.Mode == ModeClient {
		startClient(conf)
	} else if *conf.Mode == ModeServer {
		startServer(conf)
	} else {
		log.Fatalf("bad mode %s\n", *conf.Mode)
	}

	log.Printf("jrdwp started in %v mode\n", conf.Mode)
}

func startClient(conf *Config) {
	loadKey(conf)
	wsClient := client.NewWSClient(*conf.ServerHost, *conf.ServerPort, *conf.WsPath, *conf.WsOrigin, *conf.JdwpPort, conf.PublicKey)
	tcpServer := client.NewTCPServer(wsClient, *conf.BindPort)
	tcpServer.Start()
}

func startServer(conf *Config) {
	tcpClient := server.NewTCPClient(*conf.ServerHost)
	wsServer := server.NewWSServer(*conf.WsPath, *conf.WsOrigin, *conf.BindPort, conf.AllowedJdwpPorts, tcpClient)
	wsServer.Start()
}
