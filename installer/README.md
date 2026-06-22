# Windows installer

The [`Build Windows Installer`](../.github/workflows/release-installer.yml) workflow
builds an MSI from [`mtproto-polling-service.wxs`](mtproto-polling-service.wxs)
and, on a version tag, attaches it to a GitHub Release.

## Prerequisites — repository secrets

Set these under **Settings → Secrets and variables → Actions**:

| Secret | Value |
| ------ | ----- |
| `TG_API_ID` | Telegram `api_id` (numeric) |
| `TG_API_HASH` | Telegram `api_hash` (hex string) |

> ⚠️ These values are **baked into the MSI** (as machine environment variables and
> as service arguments). Anyone who obtains the installer can read them — keep the
> Release/artifact private.

## Releasing

Push a version tag:

```console
git tag v1.0.0
git push origin v1.0.0
```

The workflow builds `MTProtoPollingService-1.0.0.msi` and publishes it on the
Release for that tag. It also uploads the MSI as a build artifact on every run,
including manual runs (**Actions → Build Windows Installer → Run workflow**,
which takes a version input).

## What the MSI does

- Installs `mtproto-polling-service.exe` to `C:\Program Files\MTProto Polling Service\`.
- Sets machine-wide `TG_API_ID` / `TG_API_HASH` environment variables (handy for
  console mode and other tools; removed on uninstall).
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
  -d Version=0.0.0 -d TgApiId=12345 -d TgApiHash=deadbeef ^
  -o installer/MTProtoPollingService-0.0.0.msi
```
