# jrdwp

**J**ava **R**emote **D**ebugging through **W**ebSocket **P**roxy: attach your IDE's debugger to a JVM that you can only reach over HTTP(S).

jrdwp wraps JDWP, the Java debug protocol, in a WebSocket so it can pass through an ordinary reverse proxy such as nginx. It started as an open-source take on Microsoft's [azure-websites-java-remote-debugging](https://github.com/Azure/azure-websites-java-remote-debugging), which published only the client. jrdwp has both the client and the server.

```text
IDE --TCP--> jrdwp client ==WebSocket (ws/wss)==> nginx --> jrdwp server --TCP--> JVM
             localhost:9876                                 127.0.0.1:9877        127.0.0.1:5005
```

## When to use it

jrdwp fits when:

- the JVM is reachable only through an HTTP(S) reverse proxy: no SSH, no Kubernetes access, no cloud tunnel, and perhaps only a corporate HTTP proxy on your side;
- you want debug access narrower than SSH: only the JDWP ports you allow, no shell, a new key on every start, single-use tokens, and a server that exits after a set time.

Often something else is simpler:

| Your setup | Use instead |
|---|---|
| Kubernetes | `kubectl port-forward` |
| You can SSH to the host | `ssh -L 5005:127.0.0.1:5005 host` |
| AWS, on a host running the SSM agent | Session Manager port forwarding (`AWS-StartPortForwardingSession`) |
| A GCP VM | `gcloud compute ssh VM --tunnel-through-iap -- -L 5005:127.0.0.1:5005` |
| Azure App Service on Linux | [`az webapp create-remote-connection`](https://learn.microsoft.com/en-us/azure/app-service/configure-linux-open-ssh-session), then SSH port forwarding through that tunnel |
| You need a general TCP-over-WebSocket tunnel | [wstunnel](https://github.com/erebe/wstunnel), [chisel](https://github.com/jpillora/chisel), [`cloudflared access tcp`](https://developers.cloudflare.com/cloudflare-one/access-controls/applications/non-http/cloudflared-authentication/arbitrary-tcp/) |
| You're diagnosing production and can't pause threads | [Arthas](https://github.com/alibaba/arthas), JFR, async-profiler |

> [!WARNING]
> A breakpoint suspends threads in the target JVM, and debug access lets the debugger run code in it. Think twice before debugging a production service.

## Download

Prebuilt binaries for Linux, macOS and Windows are on the [releases page](https://github.com/leonlee/jrdwp/releases), named `jrdwp_<os>_<arch>[.exe]`. Rename yours to `jrdwp`. To build from source, see [Building](#building).

## Try it locally

With a JVM waiting for a debugger on port 5005 (see [step 1](#1-start-the-jvm-with-jdwp)):

```bash
make build
./start-server   # jrdwp server on 127.0.0.1:8877, writes .jrdwp_key
./start-client   # in a second terminal: jrdwp client on 127.0.0.1:8876
```

Then attach your IDE's remote debugger to `localhost:8876`.

## Set it up

### 1. Start the JVM with JDWP

Bind JDWP to `127.0.0.1` so the only way in is through jrdwp:

```bash
java -agentlib:jdwp=transport=dt_socket,server=y,suspend=n,address=127.0.0.1:5005 -jar app.jar
```

### 2. Route a WebSocket path to jrdwp

For [nginx 1.3.13 or later](https://nginx.org/en/docs/http/websocket.html) (any reverse proxy with WebSocket support works):

```nginx
location /jrdwp {
  proxy_pass http://127.0.0.1:9877;
  proxy_http_version 1.1;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection "upgrade";
  # The client pings every 30s, which covers the default 60s idle timeout.
  # This is a fallback for long sessions paused at a breakpoint.
  proxy_read_timeout 3600s;
}
```

### 3. Start the jrdwp server next to the JVM

```bash
./jrdwp -mode server -bind-port 9877 -server-host 127.0.0.1 -allowed-jdwp-ports 5005
```

On every start the server writes a new random key to `.jrdwp_key` (mode 0600) in its working directory. It shuts down after 60 minutes; change that with `-server-deadline`.

### 4. Give the key to the client

Copy `.jrdwp_key` to the client's working directory, or put its contents in the client's `JRDWP_KEY` environment variable, which takes precedence over the file. Copy it again after every server restart. The key is never logged. Anyone who has it can open a debug session, so treat it like a password.

### 5. Start the jrdwp client on your machine

```bash
./jrdwp -mode client -server-host java.remote.com -server-port 443 -ws-scheme wss -jdwp-port 5005
```

If nginx doesn't serve TLS, use `-ws-scheme ws -server-port 80`, but only on a network you trust. Behind a corporate proxy, the client honors `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`.

### 6. Attach your debugger

Point a remote JVM debug configuration in IntelliJ IDEA, Eclipse or NetBeans at `localhost:9876`.

```text
 _________________
< Enjoy yourself! >
 -----------------
        \   ^__^
         \  (oo)\_______
            (__)\       )\/\
                ||----w |
                ||     ||
```

## Options

| Flag | Mode | Default | Description |
|---|---|---|---|
| `-mode` | both | `client` | `client` or `server` |
| `-bind-host` | both | `127.0.0.1` | Address to listen on. `0.0.0.0` listens on all interfaces. |
| `-bind-port` | both | `9876` | Port to listen on |
| `-ws-path` | both | `jrdwp` | WebSocket path |
| `-server-host` | both | this host | Client: jrdwp server host. Server: JVM host. |
| `-server-port` | client | `9877` | jrdwp server port |
| `-ws-scheme` | client | `ws` | `ws` or `wss` |
| `-ws-origin` | client | none | `Origin` header to send |
| `-jdwp-port` | client | required | JDWP port of the remote JVM |
| `-allowed-jdwp-ports` | server | required | JDWP ports clients may open, like `5005,5006` |
| `-server-deadline` | server | `60` | Minutes until the server shuts down |
| `-version` | both | | Print the version and exit |

The client reads its key from `JRDWP_KEY` if set, otherwise from `.jrdwp_key`.

## Security

- Both sides listen on `127.0.0.1` by default. Keep the JVM's JDWP port on `127.0.0.1` too.
- The server opens only the ports in `-allowed-jdwp-ports`.
- The server makes a new random key on every start and shuts down after `-server-deadline` minutes.
- Every connection carries an HMAC-SHA256 token bound to its JDWP port. A token is valid for 60 seconds either side of the server's clock, so keep clocks in sync, and it opens only one connection.
- Use `wss` on untrusted networks. Plain `ws` exposes the debug traffic, and whoever uses a captured token first gets the connection.

## Building

Requires Go 1.26 or later.

```bash
make test      # run tests with the race detector
make build     # ./jrdwp for this platform
make release   # dist/jrdwp_<os>_<arch>[.exe] for Linux, macOS and Windows
make linux     # ./jrdwp.bin for linux/amd64
make windows   # ./jrdwp.exe for windows/386
./jrdwp -version
```

## Upgrading from v0.2.0 or earlier

- The key file and token format changed. Upgrade the client and server together, and copy the new `.jrdwp_key` (or set `JRDWP_KEY`) after the server starts.
- Both sides now listen on `127.0.0.1` by default. Pass `-bind-host 0.0.0.0` if the server is reached without a proxy on the same host.
- `-ws-origin` is now optional, and the server ignores it.
- Building from source requires Go 1.26 or later.
- `make release` now builds every platform into `dist/` as `jrdwp_<os>_<arch>[.exe]` instead of producing `jrdwp`, `jrdwp.bin` and `jrdwp.exe`. Use `make build`, `make linux` (`jrdwp.bin`) and `make windows` (`jrdwp.exe`) for the old outputs. The `xbuild` target is gone.
