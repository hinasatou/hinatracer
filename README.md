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
- **WebUI**: use every feature from a browser, from the desktop app or with no window at all (`--web`), with an optional access password.
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

Either way traceroute / MTR shows every hop: with the capability (or as root, or the server package's service) HinaTracer uses a raw ICMP socket; without it, it uses the unprivileged ICMP socket and reads the routers' replies from the socket's error queue.

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

## WebUI (use HinaTracer in a browser)

Everything in the desktop app (Ping monitoring, traceroute / MTR, IP lookup, data file downloads and settings) is also available as a web page. The web page and the desktop window share the same settings and the same running tasks, so a target you add in one appears in the other at once.

**From the desktop app**: open the **WebUI** page, choose the bind IP and port, and click **Start**. The address to open is shown there; you can copy it or open it in your browser. Tick *Start the web server when HinaTracer opens* to have it running every time.

**Without a window** (for a server or an always-on PC):

```
hinatracer --web                                # uses the saved address, default 127.0.0.1:8790
hinatracer --web --listen 0.0.0.0:8790          # reachable from other devices (password required)
hinatracer --web --web-host 127.0.0.1 --web-port 9000
hinatracer --help
```

The address is printed in the terminal and written to `web.log` next to `config.json`. Press Ctrl+C to stop.

**Access protection**

- The default address `127.0.0.1` can only be opened on this computer.
- To let other devices connect (for example `0.0.0.0`), set an **access password** on the WebUI page first; HinaTracer will not listen on such an address without one. In no-window mode you can instead set the `HINATRACER_WEB_PASSWORD` environment variable.
- With a password set, the page asks you to sign in. Changing the password signs out every browser.
- The page uses plain HTTP. On networks you don't trust, keep it on `127.0.0.1` or put it behind an HTTPS reverse proxy.

## Guest mode

On the **WebUI** page, after setting an access password, tick **Allow guest mode**. The sign-in page then also shows **Sign in as guest**: guests need no password and can look at everything, but cannot change anything (no adding or deleting targets, no starting or stopping, no settings, downloads or updates), and they do not see file paths or access settings. A **Guest** badge is shown at the top. Turning guest mode off signs guests out.

## API tokens

Create tokens on the **WebUI** page (in the desktop app or under Settings in the browser): give it a name and pick a role:

- **Admin**: can view and change everything.
- **Guest**: view only.

The token is shown **only once**; copy it right away. Only a fingerprint of it is stored, so a lost token cannot be recovered: revoke it and create a new one.

## AI clients (MCP)

While the WebUI is running, HinaTracer is also an MCP server, so AI assistants that support MCP can read your monitoring results and, with an admin token, manage targets and traceroutes for you. The address is shown on the WebUI page:

```
http://127.0.0.1:8790/mcp
```

Add it to your AI client as a remote (HTTP) MCP server with an API token in the `Authorization` header, for example:

```json
{
  "mcpServers": {
    "hinatracer": {
      "type": "http",
      "url": "http://127.0.0.1:8790/mcp",
      "headers": { "Authorization": "Bearer htk_your_token_here" }
    }
  }
}
```

With a guest token the assistant can only read. When no access password is set, clients on this computer can connect without a token.

## Command line

With HinaTracer (or `hinatracer --web`) running, you can control it from a terminal:

```
hinatracer ping list
hinatracer ping add 1.1.1.1 --alias cf
hinatracer ping disable cf
hinatracer trace new 8.8.8.8 --start
hinatracer trace show 8.8.8.8
hinatracer lookup 8.8.8.8 example.com
hinatracer settings get
hinatracer settings set theme=dark autoStartPing=true
hinatracer ping list --json
hinatracer api GET /api/state
hinatracer help
```

By default the commands connect to the saved WebUI address. Use `--server http://HOST:PORT` (or the `HINATRACER_SERVER` environment variable) for another machine and `--token TOKEN` (or `HINATRACER_TOKEN`) for the API token. Tables are printed by default; add `--json` for JSON.

## Monitoring history and CSV export

Open the detail window of a ping target (or a traceroute hop). Above the latency chart, choose a range: **Per probe**, **Per minute**, **Per hour**, **Per day**, **Per week** or **Per month**. Aggregated ranges show the average as a line, the min–max band, and loss % as red bars.

On **Settings → Monitoring history**, tick **Save monitoring history to disk** and set how many days to keep (default 90). This covers both Ping and HTTP monitoring: their history and the table statistics. Data is stored as SQLite next to your other data files (`history.db`). Old raw samples are rolled up into minute/hour buckets automatically. When saving is off, only recent in-memory data is used.

**Export CSV** in the detail window (or `GET /api/history.csv`, or `hinatracer ping history <id> --csv`) writes columns `time,target,alias,ip,samples,avg,min,max,loss%`.

When saving is on, the Ping and HTTP tables also keep their totals (counts, success and failure rates, last/avg/min/max) across restarts and keep counting. The median after a restart is taken from the latest 1000 successful samples. **Reset Stats** then clears the saved totals for good, so the app asks first (in the WebUI too); on the command line add `--yes`, and AI clients must pass `confirm`. Traceroute hops are never kept.

## HTTP monitoring

The **HTTP Monitor** page (below Ping Monitor) checks websites and APIs on a schedule.

- Add one URL, or use **Batch add** with one URL per line (an alias may follow after a space).
- Each target has its own settings (right-click → Edit): method GET / HEAD / POST / PUT / PATCH / DELETE / OPTIONS or a custom one (POST, PUT, PATCH, DELETE and custom methods can send a request body with a Content-Type of your choice), interval (default 30 s), timeout, expected status codes (for example `200-299,301` or `2xx`; empty means 200–399), a text or regular-expression match on the first 1 MB of the page (can be inverted), custom request headers, follow redirects, and ignore TLS certificate errors (off by default).
- HTTP monitoring does not start by itself when HinaTracer opens unless **Settings → Startup → Auto-start HTTP monitoring on launch** is ticked (off by default, like Ping).
- In the WebUI, drag the right edge of a column header to change its width; the browser remembers it per table (double-click the edge to reset).
- The table has the same columns and handling as the Ping table: IP, location, region, ASN and ASN region, success/failure counts, success and failure rates, last/avg/min/max/median response time, status code, last check, last success and last failure. Click headers to sort, Ctrl/Shift-click to select several rows (right-click for actions on all of them), drag to reorder, and column widths are remembered. Double-click a row (or press Enter) for DNS / connect / TLS / TTFB timing, certificate days left and the trend chart (all ranges, CSV export). History is saved together with ping history when **Save monitoring history to disk** is on.
- Header values are never written to logs and are hidden from guests.
- The same functions are available in the WebUI, to AI clients (MCP) and on the command line (`hinatracer http list`, `http add <url> ...`, see `hinatracer help`).
- In every table (Ping, HTTP, Traceroute hops, IP lookup — desktop app, remote mode and WebUI), Ctrl+A selects all rows and the right-click menu has **Copy column**: pick a column to copy that value of every selected row, one per line, in the order shown (empty values are left out).
- **Desktop alerts** (Settings → Desktop alerts, desktop app only, all off by default): send a system notification when a target goes down (a set number of failures in a row (default 3)) or, optionally, recovers — bursts are grouped into one notification; show a tray icon (show window, start/stop all Ping or HTTP monitoring, quit) and optionally minimize to the tray when closing the window. Works with a remote data source too.
- In remote data-source mode, Settings clearly separates **Local** settings (this computer only) from **Remote** server settings; local-only options that only apply after switching back are in a collapsed group.

## Remote mode (desktop app as a client)

Settings → **Data source** → **Remote WebUI** lets this window show and control another HinaTracer (for example the server package) instead of this computer.

- Enter the WebUI address and sign in with the admin password, an API token, or as a guest (read-only).
- The password is only used once to create an API token; it is never saved. The token is kept in your config file, which only your user can read.
- While remote mode is on, local monitoring stops and the local WebUI page is disabled. The connection status is shown at the top and the app reconnects by itself.
- Every page works as it does locally: Ping (add, import, edit, reorder, detail charts, CSV), Traceroute tabs, IP lookup and HTTP. As admin, Settings also shows **Remote server settings** (language, theme, data files and downloads, startup, history, updates, WebUI listen address, guest mode and API tokens of the server). Data source, theme and this computer's language stay local. Guests see everything read-only.
- **Connect automatically at startup** (Settings → Data source, shown after connecting, off by default): the app starts in remote mode and connects with the saved token; if the server is unreachable it shows the status, keeps retrying, and offers **Switch back to local**.
- Switch back with **Local** → **Switch back to local**.

## Server package (Debian/Ubuntu)

For a headless install that only runs the WebUI under systemd:

```bash
sudo apt install ./HinaTracer-v<ver>-linux-amd64-server.deb
```

This installs `/usr/bin/hinatracer`, the unit `hinatracer.service` (user `hinatracer`, `CAP_NET_RAW`, hardened), default config `/etc/hinatracer/config.json` (conffile) and data under `/var/lib/hinatracer`. It listens on `http://127.0.0.1:8790/` by default.

Set an access password (required before binding a non-loopback address):

```bash
sudo -u hinatracer hinatracer web passwd --config /etc/hinatracer/config.json
sudo systemctl restart hinatracer
```

Self-update install is disabled in server mode; use the package manager to upgrade. You can also run `hinatracer --web --server-mode --config /path/to/config.json` without the package.

## Per-browser language

Any WebUI visitor (including guests) can pick a language; it is stored only in the browser (`localStorage`) and never writes the server config. Admins can still change the app-wide language, or tick **This browser only**.

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

[MIT License](LICENSE). Third-party data files are subject to their own licenses.
