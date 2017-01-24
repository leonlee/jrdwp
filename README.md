# jrdwp
**J**ava **R**emote **D**ebugging through **W**ebsocket **P**roxy is a proxy for Java remote debugging. It likes Microsoft's [azure-websites-java-remote-debugging] (https://github.com/Azure/azure-websites-java-remote-debugging), but includes all of client and server side implementation(**azure-websites-java-remote-debugging repo** only published client side, the serverside is not opensource now).

# Prerequisites:
* Enable websocket endpoint (nginx version >= 1.3)
* JDWP compatible debugger like Eclipse/Netbeans

# Compiling & Building
```bash
#build according to development platform
make build 
#build with GOOS=linux GOARCH=amd64 CGO_ENABLED=0 for linux platform
make linux
#build with GOOS=windows GOARCH=386 CGO_ENABLED=0 for windows platform
make windows
```

# Usage
## Enable webosocket (nginx)
```nginx
#place before "server"
map $http_upgrade $connection_upgrade {
  default upgrade;
  ''      close;
}

#add websocket location likes:
location /jrdwp {
  proxy_pass http://localhost:9877;
  proxy_http_version 1.1;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection "upgrade";
}
```

## Start Java application with JDWP
```bash
java -agentlib:jdwp=transport=dt_socket,server=y,suspend=n,address=127.0.0.1:5005 -jar foo.jar
```

## [Start jrdwp server on remote host] (start-server)
```bash
./jrdwp -mode server -bind-port 9877 -server-host 127.0.0.1  -allowed-jdwp-ports "5005" -ws-origin http://java.remote.com/
```

## Copy public key from remote host
jrdwp server will generate .jrdwp_key under the working directory, please copy it's content and save as .jrdwp_key to jrdwp client working directory.

## [Start jrdwp client on local box] (start-client)
```bash
./jrdwp -mode=client -bind-port=9876 -server-host=java.remote.com -server-port=80 -ws-origin=http://java.remote.com/ -jdwp-port=5006 -ws-path=jrdwp
```
## Open IDEA/Eclipse to connect to jrdwp client on localhost:9876
```bash
 _________________
< Enjoy yourself! >
 -----------------
        \   ^__^
         \  (oo)\_______
            (__)\       )\/\
                ||----w |
                ||     ||
```

# Options
## Flags of jrdwp server
```bash
    -mode string
        jrdwp mode, "client" or "server" (default "client")
    -bind-port int
        bind port, default 9876 (default 9876)
    -allowed-jdwp-ports string
        allowed jdwp ports likes: "5005,5006"
    -server-host string
        jdwp server host, default ''
    -ws-origin string
        websocket request origin header
    -ws-path string
        websocket server path (default "jrdwp")
    -server-deadline int
    	  server deadline in minutes that server will shutdown on deadline, default 60 minutes (default 60)
```

## Flags of jrdwp client
```bash
    -mode string
        jrdwp mode, "client" or "server" (default "client")
    -bind-host string
        bind host, default ''
    -bind-port int
        bind port, default 9876 (default 9876)
    -server-host string
        remote server host
    -server-port int
        remote server port, default 9877 (default 9877)
    -ws-origin string
        websocket request origin header
    -ws-path string
        websocket server path (default "jrdwp")
    -jdwp-port int
        jdwp port of remote application (default -1)
```

# Security
* changes public key on server starting, verify token according to timestamp
* specify "allowed-jdwp-ports" to prevent unexpected intrusions
* bind jdwp ports to locally ports to prevent ports leaks
