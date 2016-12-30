package common

import (
	"log"
	"path"
	"strconv"
	"strings"
)

const (
	//HeaderToken security token
	HeaderToken = "JRDWP_TOKEN"
	//HeaderPort jdwp port
	HeaderPort = "JDWP_PORT"
	//PublicKeyDir default client key path
	PublicKeyDir = "."
	//PublicKeyFile filename
	PublicKeyFile = ".jrdwp_key"
)

//PublicKeyPath concat file path of client key
func PublicKeyPath() string {
	return path.Join(PublicKeyDir, PublicKeyFile)
}

//SplitToInt split comma delimited string to int array
func SplitToInt(commaDelimitedString string) []int {
	ports := []int{}

	for _, token := range strings.Split(commaDelimitedString, ",") {
		port, err := strconv.Atoi(string(strings.TrimSpace(token)))
		if err != nil {
			log.Fatalln("bad integer", token, err.Error)
		} else {
			ports = append(ports, port)
		}
	}

	return ports
}
