# jrdwp
**J**ava **R**emote **D**ebugging through **W**ebsocket **P**roxy is a proxy for Java remote debugging. It likes Microsoft's [azure-websites-java-remote-debugging] (https://github.com/Azure/azure-websites-java-remote-debugging), but includes all of client and server side implementation(**azure-websites-java-remote-debugging repo** only published client side, the serverside is not opensource now).

# Prerequisites:
* Enable websocket endpoint (nginx version >= 1.3)
* JDWP compatible debugger like Eclipse/Netbeans

# Downloads
https://github.com/leonlee/jrdwp/releases

# Compiling & Building
Requires Go 1.26 or later.
```bash
#run tests
make test
#build according to development platform
make build 
#build with GOOS=linux GOARCH=amd64 CGO_ENABLED=0 for linux platform
make linux
#build with GOOS=windows GOARCH=386 CGO_ENABLED=0 for windows platform
make windows
#build linux, macOS and windows binaries into dist/
make release
#print the version stamped from git, e.g. v0.3.0
./jrdwp -version
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
  proxy_pass http://127.0.0.1:9877;
  proxy_http_version 1.1;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection "upgrade";
  #the client pings every 30s, which covers the default 60s; this is a fallback
  #for a session paused at a breakpoint, which sends no JDWP data
  proxy_read_timeout 3600s;
}
```

## Start Java application with JDWP
```bash
java -agentlib:jdwp=transport=dt_socket,server=y,suspend=n,address=127.0.0.1:5005 -jar foo.jar
```

## [Start jrdwp server on remote host] (start-server)
```bash
./jrdwp -mode server -bind-port 9877 -server-host 127.0.0.1  -allowed-jdwp-ports "5005"
```

## Copy the key from remote host
On every start, jrdwp server writes a new random key to .jrdwp_key (mode 0600) in its working directory. Copy that file to the jrdwp client's working directory, or put its contents in the client's `JRDWP_KEY` environment variable, which takes precedence over the file. The key is not printed to the log, and anyone who has it can open a debug session, so treat it like a password.

## [Start jrdwp client on local box] (start-client)
```bash
./jrdwp -mode=client -bind-port=9876 -server-host=java.remote.com -server-port=80 -ws-origin=http://java.remote.com/ -jdwp-port=5005 -ws-path=jrdwp
```
If nginx serves TLS, use `-ws-scheme=wss -server-port=443` so the token and debug traffic are encrypted.
Behind a corporate proxy, the client honors `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`.

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
    -bind-host string
        bind host, use 0.0.0.0 to listen on all interfaces (default "127.0.0.1")
    -bind-port int
        bind port (default 9876)
    -allowed-jdwp-ports string
        allowed JDWP ports like "5005,5006", required
    -server-host string
        JVM host (default "", which is localhost)
    -ws-path string
        websocket path (default "jrdwp")
    -server-deadline int
        shut down after this many minutes (default 60)
    -version
        print version and exit
```

## Flags of jrdwp client
```bash
    -mode string
        jrdwp mode, "client" or "server" (default "client")
    -bind-host string
        bind host, use 0.0.0.0 to listen on all interfaces (default "127.0.0.1")
    -bind-port int
        bind port (default 9876)
    -server-host string
        jrdwp server host
    -server-port int
        jrdwp server port (default 9877)
    -ws-scheme string
        "ws" or "wss" (default "ws")
    -ws-path string
        websocket path (default "jrdwp")
    -ws-origin string
        Origin header to send, optional
    -jdwp-port int
        JDWP port of the remote JVM, required
    -version
        print version and exit
```
The client reads its key from `JRDWP_KEY` if set, otherwise from .jrdwp_key.

# Security
* the server makes a new random key on every start and shuts down after `-server-deadline` minutes
* every connection carries an HMAC-SHA256 token bound to the JDWP port, valid for 60 seconds either side of the server's clock (keep clocks in sync)
* each token opens one connection: the server refuses a token it has already accepted
* still use `-ws-scheme wss` on untrusted networks: plain ws exposes the debug traffic, and whoever uses a captured token first gets the connection
* specify "allowed-jdwp-ports" to prevent unexpected intrusions
* both sides listen on 127.0.0.1 by default; bind the JVM's JDWP port to 127.0.0.1 too

# Upgrading from v0.2.0 or earlier
* The key file and token format changed. Upgrade client and server together, and copy the new .jrdwp_key (or set `JRDWP_KEY`) after the server starts.
* Both sides now listen on 127.0.0.1 by default. Pass `-bind-host 0.0.0.0` if the server is reached without a proxy on the same host.
* `-ws-origin` is now optional, and the server ignores it.
* Building from source requires Go 1.26 or later.
* `make release` now builds every platform into `dist/` as `jrdwp_<os>_<arch>[.exe]` instead of producing `jrdwp`, `jrdwp.bin` and `jrdwp.exe`. Use `make build`, `make linux` (`jrdwp.bin`) and `make windows` (`jrdwp.exe`) for the old outputs. The `xbuild` target is gone.
