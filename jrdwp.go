package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
)

const (
	modeClient = "client"
	modeServer = "server"
	// keyFile holds the shared key: the server writes it, the client reads it.
	keyFile = ".jrdwp_key"
)

type config struct {
	mode           string
	bindHost       string
	bindPort       int
	serverHost     string
	serverPort     int
	wsScheme       string
	wsPath         string
	wsOrigin       string
	jdwpPort       int
	allowedPorts   []int
	serverDeadline int // minutes
}

func main() {
	conf, err := parseFlags()
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("starting jrdwp: %+v", conf)

	if conf.mode == modeServer {
		err = runServer(conf)
	} else {
		err = runClient(conf)
	}
	log.Fatalln(err)
}

func parseFlags() (config, error) {
	var conf config
	var allowedPorts string
	flag.StringVar(&conf.mode, "mode", modeClient, `jrdwp mode, "client" or "server"`)
	flag.StringVar(&conf.bindHost, "bind-host", "127.0.0.1", "bind host, use 0.0.0.0 to listen on all interfaces")
	flag.IntVar(&conf.bindPort, "bind-port", 9876, "bind port")
	flag.StringVar(&conf.serverHost, "server-host", "", "client: jrdwp server host; server: JVM host")
	flag.IntVar(&conf.serverPort, "server-port", 9877, "client: jrdwp server port")
	flag.StringVar(&conf.wsScheme, "ws-scheme", "ws", `client: "ws" or "wss"`)
	flag.StringVar(&conf.wsPath, "ws-path", "jrdwp", "websocket path")
	flag.StringVar(&conf.wsOrigin, "ws-origin", "", "client: Origin header to send, optional")
	flag.IntVar(&conf.jdwpPort, "jdwp-port", 0, "client: JDWP port of the remote JVM, required")
	flag.StringVar(&allowedPorts, "allowed-jdwp-ports", "", `server: allowed JDWP ports like "5005,5006", required`)
	flag.IntVar(&conf.serverDeadline, "server-deadline", 60, "server: shut down after this many minutes")
	flag.Parse()

	if conf.wsPath == "" {
		return conf, errors.New("ws-path is required")
	}
	conf.wsPath = "/" + strings.TrimPrefix(conf.wsPath, "/")

	switch conf.mode {
	case modeClient:
		if conf.wsScheme != "ws" && conf.wsScheme != "wss" {
			return conf, fmt.Errorf("bad ws-scheme %q", conf.wsScheme)
		}
		if !validPort(conf.jdwpPort) {
			return conf, errors.New("jdwp-port is required")
		}
	case modeServer:
		ports, err := parsePorts(allowedPorts)
		if err != nil {
			return conf, err
		}
		conf.allowedPorts = ports
		if conf.serverDeadline <= 0 {
			return conf, errors.New("server-deadline must be positive")
		}
	default:
		return conf, fmt.Errorf("bad mode %q", conf.mode)
	}
	return conf, nil
}

func parsePorts(text string) ([]int, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("allowed-jdwp-ports is required")
	}
	var ports []int
	for _, field := range strings.Split(text, ",") {
		port, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || !validPort(port) {
			return nil, fmt.Errorf("bad port %q in allowed-jdwp-ports", field)
		}
		ports = append(ports, port)
	}
	return ports, nil
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}
