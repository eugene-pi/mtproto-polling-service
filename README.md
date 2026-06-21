# mtproto-polling-service

A small Go service that always keeps a **working Telegram MTProto proxy** on hand
and serves it over a local HTTP endpoint. It runs as a **Windows service** or as
an ordinary **console application** from the same binary.

Implements [issue #1](https://github.com/eugene-pi/mtproto-polling-service/issues/1).

## How it works

1. If there is no currently-valid proxy, the service downloads the public proxy
   list from
   [`SoliSpirit/mtproto/all_proxies.txt`](https://github.com/SoliSpirit/mtproto/blob/master/all_proxies.txt).
2. It checks the proxies **in parallel** and serves the **first usable** one. A
   proxy is usable when it passes both stages:
   1. **TCP connect** — a TCP connection to `server:port` succeeds (a fast
      filter).
   2. **Telegram check** — a real Telegram client (via
      [`gotd/td`](https://github.com/gotd/td)) connects to a Telegram data
      center *through* the proxy, completes the MTProto handshake and makes one
      unauthenticated call. This proves a Telegram client can actually use the
      proxy. It uses an in-memory session, so no account, phone number or login
      code is involved — only an `api_id`/`api_hash`.
3. If **none** of the proxies work, it waits **30 minutes** and then checks
   whether the published list has changed:
   - if it **changed**, it re-checks the fresh list;
   - if it is **unchanged**, it waits another 30 minutes and checks again.
4. Once a working proxy is found it is served until it stops responding, at which
   point the search starts over.

Update detection uses an HTTP conditional request (`ETag`) when the server
supports it, falling back to a SHA-256 comparison of the file contents.

## Telegram API credentials (required)

The second-stage check needs a Telegram `api_id`/`api_hash`. Get them for free
from [my.telegram.org](https://my.telegram.org) → *API development tools*, then
provide them via environment variables (preferred) or flags:

```console
$env:TG_API_ID  = "123456"
$env:TG_API_HASH = "0123456789abcdef0123456789abcdef"
```

The service refuses to run without them. For **service mode**, set them as
**machine** environment variables so the service account can read them — they are
deliberately *not* stored in the service definition.

## Config file (handy for local debugging)

Instead of env vars or flags you can put settings in a JSON file. Copy the
example and edit it:

```console
copy config.example.json config.json
```

`config.json` is loaded automatically from the working directory when present
(or point at one with `-config path\to\file.json`). It can hold any option,
including the credentials:

```json
{
  "tgApiId": 123456,
  "tgApiHash": "0123456789abcdef0123456789abcdef",
  "httpAddr": "127.0.0.1:8080",
  "pollInterval": "30m",
  "dialTimeout": "5s",
  "concurrency": 200
}
```

Durations are strings like `"30m"` / `"5s"`. Settings are resolved with the
precedence **flag > environment variable > config file > built-in default**, so
a flag always overrides the file. `config.json` is git-ignored (it may contain
your credentials); `config.example.json` is committed as a template.

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

Set the credentials, then run the binary; it runs interactively and logs to the
console:

```console
$env:TG_API_ID = "123456"; $env:TG_API_HASH = "..."
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
| `-verify-timeout` | `15s` | Timeout for the Telegram client verification of a proxy. |
| `-tg-api-id` | `$TG_API_ID` | Telegram `api_id` (required). |
| `-tg-api-hash` | `$TG_API_HASH` | Telegram `api_hash` (required). |

## Test

```console
go test ./...
```
