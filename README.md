English | [中文(简体)](README.zh-CN.md) | [中文(繁體)](README.zh-HK.md) | [日本語](README.ja.md)

# HinaTracer

<img src="resources/icon.png" width="96" height="96" alt="HinaTracer icon" />

A native desktop network toolkit for Windows, macOS and Linux: continuous ping (ICMP and TCP), traceroute / MTR and IP lookup, with location, region and ASN details for every address.

## Features

- **Ping monitoring**: watch many targets at once with live success rate, packet loss, and last / average / min / max / median latency. Each target can be enabled or disabled, renamed with an alias and opened in a details window. Targets can be added one at a time or imported in bulk.
- **TCP ping**: give a target a port (`example.com:443`) and HinaTracer measures the TCP connection time instead of sending ICMP echoes.
- **Traceroute / MTR**: each trace runs in its own tab. MTR mode keeps probing every hop, and you can start or stop all tabs at once. A hop can be sent to Ping monitoring with one click.
- **IP lookup**: look up IPs or domains (one per line). Domains resolve to all of their IPv4 and IPv6 addresses, and each address gets a quick latency check.
- **Region and ASN info**: every address shows its location, region with flag, ASN / network name and the ASN's registered region. IPv4 and IPv6 are both supported.
- **Convenient tables**: sort by any column, resize and reorder columns (remembered between launches), select several rows and right-click for quick actions.
- **Update checker**: optionally checks GitHub Releases at startup (stable releases only, or including pre-releases) and can download, verify and install the new version for you.
- **Languages and themes**: 中文(简体), 中文(繁體), 日本語 and English, plus your own language packs; light, dark or follow-system theme.

## Download

Get the latest version from **[GitHub Releases](https://github.com/hinasatou/hinatracer/releases)**.

| Platform | File | Notes |
|----------|------|-------|
| Windows (x64) | `HinaTracer-vX.Y.Z-windows-amd64-setup.exe` | Installer (recommended); installs for the current user, no admin rights needed |
| Windows (x64) | `HinaTracer-vX.Y.Z-windows-amd64.zip` | Portable; unzip and run `HinaTracer.exe` |
| macOS (Apple silicon) | `HinaTracer-vX.Y.Z-macos-arm64.zip` | Contains `HinaTracer.app` |
| macOS (Intel) | `HinaTracer-vX.Y.Z-macos-amd64.zip` | Contains `HinaTracer.app` |
| Linux (x64) | `HinaTracer-vX.Y.Z-linux-amd64.deb` | For Debian / Ubuntu and derivatives |
| Linux (x64) | `HinaTracer-vX.Y.Z-linux-amd64.tar.gz` | Portable binary with desktop entry and icon |
| All | `SHA256SUMS.txt` | Checksums to verify your download |

## Platform notes

**Linux: ping permissions.** If pings fail with a permission error, allow unprivileged ICMP or give the binary the raw-socket capability:

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"   # until reboot
sudo setcap cap_net_raw+ep /path/to/hinatracer              # or this, once
```

**macOS: first launch.** The app is not signed by Apple, so Gatekeeper may block it. Right-click `HinaTracer.app` and choose **Open**, or run:

```bash
xattr -dr com.apple.quarantine /Applications/HinaTracer.app
```

## Data files

Location, region and ASN details come from three free data files. Open **Settings** and click **Download / update all** to fetch all three in one go (you can also point to files you already have). Without them the app still works, but those columns show “—”.

| File | Used for | Source |
|------|----------|--------|
| `qqwry.ipdb` | Location | [nmgliangwei/qqwry.ipdb](https://github.com/nmgliangwei/qqwry.ipdb) |
| `Country.mmdb` | Region and flag | [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip) |
| `ip2asn-combined.tsv.gz` | ASN, network name and ASN region | [iptoasn.com](https://iptoasn.com/) |

These files are provided by their respective projects and are subject to their own licenses.

## Usage tips

- **Target formats**: `example.com`, `1.1.1.1` and `2606:4700:4700::1111` use ICMP. `example.com:443`, `1.1.1.1:443` and `[2606:4700:4700::1111]:443` use TCP ping. IPv6 addresses with a port must be in brackets.
- **Batch import** (Ping Monitor → **Batch Import**): one target per line, optionally followed by an alias: `host,alias` or `host:port,alias`. Lines starting with `#` are ignored.
- **Multi-select**: Ctrl+click (⌘+click on macOS) adds or removes a row, Shift+click selects a range, and Ctrl+A / ⌘+A selects all. With several rows selected, the right-click menu shows only the actions that make sense for all of them, such as enable / disable / delete or add to Ping.
- **Details**: double-click a row or press Enter to open its details window.
- **Traceroute from anywhere**: right-click a Ping target or IP lookup result to trace it. TCP targets are traced to the host.

## Configuration and logs

Settings, targets, trace tabs and column layout are stored in `config.json`. If the app crashes, a `crash.log` is written to the same folder.

| OS | Folder |
|----|--------|
| Windows | `%APPDATA%\hinatracer\` |
| macOS | `~/Library/Application Support/hinatracer/` |
| Linux | `~/.config/hinatracer/` |

## Custom language packs

Put a `*.json` language file into a `lang` folder inside the configuration folder above, or next to the app executable. It then appears in **Settings → Language**. A pack with the same `code` as a built-in one overrides its strings, and any missing strings fall back to English. Use the built-in packs in [`i18n/`](i18n/) as a template:

```json
{
  "name": "English",
  "code": "en",
  "strings": {
    "nav.ping": "Ping Monitor"
  }
}
```

## Build from source

Requires the Go version listed in [`go.mod`](go.mod). CGO is not needed.

```bash
go build .                # build for the current platform
go tool mygo build        # build packaged apps into build/
```

## License

[PolyForm Noncommercial License 1.0.0](LICENSE): free for noncommercial use; commercial use is not permitted. Third-party data files are subject to their own licenses.
