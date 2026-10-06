English | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-HK.md) | [日本語](README.ja.md)

# HinaTracer

<img src="resources/icon.png" width="64" height="64" alt="HinaTracer icon" />

Cross-platform native desktop network diagnostics: **Ping monitoring**, **Traceroute** (MTR), and **IP Lookup**, built with [mygo](https://github.com/egoist/mygo) v0.2.9. Annotates hops/hosts with **qqwry.ipdb** (IPIP format), Loyalsoldier GeoIP region flags (embedded flagcdn PNGs, emoji fallback), and **iptoasn ASN** (number, org, registry region). Supports **IPv4 / IPv6**.

## Features

1. **Ping Monitor** (default page) — Periodic ICMP probes with success/failure counts, success rate, latency last/avg/min/max/median, and timestamps. Columns: alias, host, rDNS, location, region, ASN. Batch import (`host` or `host,alias` per line), enable/disable per target, context menu (copy, alias, enable/disable, details, traceroute, delete). Double-click a row for details.
2. **Traceroute** — Chrome-style vertical tabs on the left (status dot, title, close, context menu, resizable strip). Default **MTR**: discover the path, then probe each hop on an interval; can switch to one-shot traceroute. Multi-tab concurrent traces (host/alias/MTR/interval/order/active tab persisted). **Start all / Stop all** for every tab. Hop table aligns with Ping stats columns. Right-click a hop to copy, open hop details, or add the IP to Ping.
3. **IP Lookup** — Enter an IP (v4/v6) or domain (batch: one per line). Domains resolve all A/AAAA asynchronously; each IP shows rDNS, location (qqwry.ipdb), region + flag (GeoIP), ASN + ASN region. Context menu: copy, add IP/domain to Ping, open Traceroute.
4. **Settings** — Language (简体中文 / 繁體中文 / 日本語 / English + user packs), theme (light / dark / system), data file paths, **one-click download/update** of all three data files (atomic replace + hot-reload), auto-start Ping / Trace on launch.
5. **About** — App name, version, GitHub link.
6. **Config & crash log** — See [Config & crash log](#config--crash-log).

## Download

Pre-built binaries are published on **[GitHub Releases](https://github.com/hinasatou/hinatracer/releases)** (draft until published). Typical assets for tag `vX.Y.Z`:

| Asset | Contents |
|-------|----------|
| `HinaTracer-vX.Y.Z-windows-amd64.zip` | `HinaTracer.exe` (GUI), `LICENSE`, `README.md` |
| `HinaTracer-vX.Y.Z-linux-amd64.tar.gz` | binary, `.desktop`, icon, `LICENSE`, `README.md` |
| `HinaTracer-vX.Y.Z-linux-amd64.deb` | Debian package from mygo |
| `HinaTracer-vX.Y.Z-macos-arm64.zip` / `…-macos-amd64.zip` | `HinaTracer.app` plus `LICENSE`, `README.md` |
| `SHA256SUMS.txt` | SHA-256 checksums of the files above |

## Platforms

| Platform | Notes | ICMP |
|----------|--------|------|
| **Windows 10/11** x64 | No WebView2 / CGO | `IcmpSendEcho` / `Icmp6SendEcho2` (admin usually not required) |
| **macOS** 12+ (amd64 / arm64) | Native UI (no WebView) | Unprivileged `udp4`/`udp6` ICMP datagram preferred; raw when privileged; `ping` fallback |
| **Linux** amd64 (GNOME/KDE/wlroots, etc.) | Native UI (no WebView / no mandatory GTK) | Same as macOS. Unprivileged ICMP needs `net.ipv4.ping_group_range` allowing your user (often already set) |

### Linux ping permissions

If unprivileged ICMP fails:

```bash
# Temporary (resets on reboot)
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"

# Or grant cap_net_raw to the binary after install
sudo setcap cap_net_raw+ep /path/to/hinatracer
```

### macOS Gatekeeper

Unsigned builds may be blocked on first open: allow under **System Settings → Privacy & Security**, or right-click → **Open**. Signed/notarized builds are recommended for distribution.

## Data files

Set paths in **Settings**, or use **Download / update all** to fetch them automatically (overwrites a configured path, otherwise saves under the config directory and fills the path). Missing files show “—” for location / region / ASN. Downloads follow redirects, use `http.ProxyFromEnvironment` (on Windows, Go reads proxy from env vars only), write to a temp file, validate, then atomically replace.

| Data | Source | Direct URL used by the app |
|------|--------|--------|
| **qqwry.ipdb** | [nmgliangwei/qqwry.ipdb](https://github.com/nmgliangwei/qqwry.ipdb) | `https://cdn.jsdelivr.net/npm/qqwry.ipdb/qqwry.ipdb` |
| **Country.mmdb** | [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip/releases) | `https://github.com/Loyalsoldier/geoip/releases/latest/download/Country.mmdb` |
| **ip2asn-combined.tsv.gz** | [iptoasn.com](https://iptoasn.com/data/ip2asn-combined.tsv.gz) | `https://iptoasn.com/data/ip2asn-combined.tsv.gz` |

## Language packs

Built-in: Simplified Chinese, Traditional Chinese, Japanese, English. Switch in Settings (applies immediately).

On first run (no `language` in config), the app detects the OS UI language and picks the best matching pack, then saves it.

Custom packs: put `*.json` in `%APPDATA%\hinatracer\lang\` (or `lang\` next to the exe / `.app`). Same `code` merges/overrides built-in strings and appears in the language list.

```json
{
  "name": "English",
  "code": "en",
  "strings": {
    "nav.ping": "Ping Monitor",
    "ping.add": "Add"
  }
}
```

Missing keys fall back to English, then Simplified Chinese. Full keys: `i18n/*.json`.

## Build from source

Requirements: Go **1.27.1+** (mygo v0.2.9; `GOTOOLCHAIN=go1.27.1` works). **No CGO**; cross-compile from any host.

```bash
# Packaged builds (Windows exe / macOS .app / Linux binary)
go tool mygo build
go tool mygo build -platform windows/amd64
go tool mygo build -platform darwin/universal
go tool mygo build -platform linux/amd64

# Direct go build
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -H windowsgui" -o build/hinatracer.exe .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/hinatracer-debug.exe .
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/hinatracer-linux-amd64 .
GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/hinatracer-darwin-amd64 .
GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/hinatracer-darwin-arm64 .

go test ./...
go test -race ./...
go vet ./...
```

Windows file icon: committed `rsrc_windows_amd64.syso` is linked automatically by `go build`. Regenerate with `go run scripts/mkicon.go -in <src.jpg>` then `bash scripts/genwinres.sh` (or `go generate`). `mygo build` also embeds `resources/icon.png` per `mygo.json`.

GitHub Actions (`.github/workflows/build.yml`) runs tests, builds with mygo on Windows / Linux / macOS for `v*` tags, packages the archives above, and attaches them to a **draft** Release with `SHA256SUMS.txt`.

## Config & crash log

Config path: `os.UserConfigDir()/hinatracer/config.json`

| OS | Config directory |
|----|------------------|
| Windows | `%APPDATA%\hinatracer\` |
| macOS | `~/Library/Application Support/hinatracer/` |
| Linux | `~/.config/hinatracer/` |

`crash.log` in the same directory records UI/update panics for diagnosis.

## License

[PolyForm Noncommercial License 1.0.0](LICENSE). Noncommercial use is permitted (personal, research, nonprofit/educational and similar purposes as defined in the license). **Commercial use is not permitted.** Third-party data files and flagcdn assets remain under their own licenses.
