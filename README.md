# mtproto-polling-service

A small Go service that always keeps a **working Telegram MTProto proxy** on hand
and serves it over a local HTTP endpoint. It runs as a **Windows service** or as
an ordinary **console application** from the same binary.

Implements [issue #1](https://github.com/eugene-pi/mtproto-polling-service/issues/1).

## How it works

1. If there is no currently-valid proxy, the service downloads the public proxy
   list from
   [`SoliSpirit/mtproto/all_proxies.txt`](https://github.com/SoliSpirit/mtproto/blob/master/all_proxies.txt).
2. It checks the proxies **in parallel** and serves the **first one that is
   connectable** (a TCP connection to `server:port` succeeds).
3. If **none** of the proxies work, it waits **30 minutes** and then checks
   whether the published list has changed:
   - if it **changed**, it re-checks the fresh list;
   - if it is **unchanged**, it waits another 30 minutes and checks again.
4. Once a working proxy is found it is served until it stops responding, at which
   point the search starts over.

Update detection uses an HTTP conditional request (`ETag`) when the server
supports it, falling back to a SHA-256 comparison of the file contents.

## HTTP API

| Method & path | Description |
| ------------- | ----------- |
| `GET /proxy`   | The current working proxy as JSON, or `503` if none is available yet. |
| `GET /healthz` | Liveness/readiness info. |

Example:

```console
$ curl http://127.0.0.1:8080/proxy
{
  "server": "example.com",
  "port": 443,
  "secret": "ee...",
  "url": "https://t.me/proxy?server=example.com&port=443&secret=ee..."
}
```

## Build

```console
go build -o mtproto-polling-service.exe .
```

## Run as a console app

Just run the binary; it runs interactively and logs to the console:

```console
mtproto-polling-service.exe
```

## Run as a Windows service

Install / control the service from an **elevated** prompt:

```console
mtproto-polling-service.exe -service install
mtproto-polling-service.exe -service start
mtproto-polling-service.exe -service stop
mtproto-polling-service.exe -service uninstall
```

The flags passed at install time are baked into the service definition, so
configure it like this:

```console
mtproto-polling-service.exe -http-addr 127.0.0.1:9000 -poll-interval 30m -service install
```

## Configuration flags

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `-service` | _(none)_ | Service control action: `install`, `uninstall`, `start`, `stop`, `restart`. |
| `-list-url` | public list | URL of the proxy list to poll. |
| `-http-addr` | `127.0.0.1:8080` | Address for the local proxy API. |
| `-poll-interval` | `30m` | Wait between checks when no proxy works. |
| `-retry-interval` | `1m` | Backoff when the list cannot be downloaded. |
| `-validate-interval` | `2m` | How often the current proxy is re-verified. |
| `-dial-timeout` | `5s` | Per-proxy TCP connect timeout. |
| `-concurrency` | `200` | Max proxies dialed in parallel. |

## Test

```console
go test ./...
```
