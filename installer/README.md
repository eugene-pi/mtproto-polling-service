# Windows installer

The [`Build Windows Installer`](../.github/workflows/release-installer.yml) workflow
builds an MSI from [`mtproto-polling-service.wxs`](mtproto-polling-service.wxs)
and, on a version tag, attaches it to a GitHub Release.

The installer contains **no secrets** — the Telegram credentials are supplied at
**install time**, so the MSI is safe to publish publicly (e.g. from a public
repo's Releases).

## Releasing

Push a version tag:

```console
git tag v1.0.0
git push origin v1.0.0
```

The workflow builds `MTProtoPollingService-1.0.0.msi` and publishes it on the
Release for that tag. It also uploads the MSI as a build artifact on every run,
including manual runs (**Actions → Build Windows Installer → Run workflow**,
which takes a version input). No repository secrets are required.

## Installing

From an **elevated** prompt, pass the Telegram credentials as properties:

```console
msiexec /i MTProtoPollingService-1.0.0.msi TG_API_ID=12345 TG_API_HASH=your_api_hash
```

If `TG_API_ID` / `TG_API_HASH` are omitted, the installer stops with a message
explaining the required command line (a dialog in interactive mode, or a logged
error under `/qn`). Add `/qn` for a silent install or `/l*v install.log` to log.

## What the MSI does

- Installs `mtproto-polling-service.exe` to `C:\Program Files\MTProto Polling Service\`.
- Sets machine-wide `TG_API_ID` / `TG_API_HASH` environment variables from the
  supplied properties (handy for console mode and other tools; removed on
  uninstall).
- Installs the **MTProtoPollingService** Windows service (LocalSystem, automatic
  start) and starts it. Credentials are also passed as service arguments so the
  service starts reliably without waiting for the environment change to
  propagate.
- Uninstall stops and removes the service and the environment variables.

## Building locally

```console
go build -trimpath -o installer/mtproto-polling-service.exe .
dotnet tool install --global wix --version 5.0.2
wix build installer/mtproto-polling-service.wxs -arch x64 -bindpath installer ^
  -d Version=0.0.0 -o installer/MTProtoPollingService-0.0.0.msi
```
