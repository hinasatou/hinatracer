[English](README.md) | 中文(简体) | [中文(繁體)](README.zh-HK.md) | [日本語](README.ja.md)

# HinaTracer

<img src="resources/icon.png" width="96" height="96" alt="HinaTracer 图标" />

适用于 Windows、macOS 和 Linux 的原生桌面网络工具：持续 Ping（ICMP / TCP）、路由追踪 / MTR 与 IP 查询，每个地址都附带位置、地区和 ASN 信息。

## 功能

- **Ping 监测**：同时监测多个目标，实时显示成功率、丢包率，以及最近 / 平均 / 最小 / 最大 / 中位延迟。可单独启用或禁用、设置别名、查看详情，支持逐个添加或批量导入。
- **TCP Ping**：为目标加上端口（如 `example.com:443`），即改为测量 TCP 连接耗时，而非发送 ICMP。
- **路由追踪 / MTR**：每个追踪独占一个页签。MTR 模式持续探测每一跳，可一键全部开始或停止，任意跳点可一键加入 Ping 监测。
- **IP 查询**：输入 IP 或域名（每行一条），域名会解析出全部 IPv4 / IPv6 地址，并对每个地址快速测一次延迟。
- **地区与 ASN 信息**：每个地址显示位置、地区（带旗帜）、ASN / 网络名称及 ASN 注册地区，支持 IPv4 和 IPv6。
- **好用的表格**：任意列排序，可调整列宽和顺序（重启后保留），支持多选并通过右键批量操作。
- **检查更新**：可在启动时检查 GitHub Releases（仅正式版，或包含预览版），并可自动下载、校验和安装新版本。
- **WebUI**：在浏览器中使用全部功能，可从桌面版启动，也可无窗口运行（`--web`），支持访问密码。
- **语言与主题**：中文(简体)、中文(繁體)、日本語、English，并支持自定义语言包；浅色、深色或跟随系统。

## 下载

请前往 **[GitHub Releases](https://github.com/hinasatou/hinatracer/releases)** 下载最新版本。

| 平台 | 文件 | 说明 |
|------|------|------|
| Windows (x64) | `HinaTracer-vX.Y.Z-windows-amd64-setup.exe` | 安装程序（推荐），为当前用户安装，无需管理员权限 |
| Windows (x64) | `HinaTracer-vX.Y.Z-windows-amd64.zip` | 便携版，解压后运行 `HinaTracer.exe` |
| macOS (Apple 芯片) | `HinaTracer-vX.Y.Z-macos-arm64.zip` | 内含 `HinaTracer.app` |
| macOS (Intel) | `HinaTracer-vX.Y.Z-macos-amd64.zip` | 内含 `HinaTracer.app` |
| Linux (x64) | `HinaTracer-vX.Y.Z-linux-amd64.deb` | 适用于 Debian / Ubuntu 及其衍生版 |
| Linux (x64) | `HinaTracer-vX.Y.Z-linux-amd64.tar.gz` | 便携二进制，附桌面入口和图标 |
| 全部 | `SHA256SUMS.txt` | 用于校验下载文件 |

## 平台说明

**Linux：Ping 权限。** 如果 Ping 因权限失败，可允许非特权 ICMP，或为程序授予原始套接字权限：

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"   # 重启前有效
sudo setcap cap_net_raw+ep /path/to/hinatracer              # 或者一次性设置
```

**macOS：首次打开。** 应用未经 Apple 签名，可能被 Gatekeeper 拦截。右键点击 `HinaTracer.app` 选择 **打开**，或执行：

```bash
xattr -dr com.apple.quarantine /Applications/HinaTracer.app
```

## 数据文件

位置、地区和 ASN 信息来自三份免费数据文件。在 **设置** 中点击 **下载/更新全部** 即可一次获取（也可以指定已有文件）。缺少数据文件时程序照常运行，只是相关列显示“—”。

| 文件 | 用途 | 来源 |
|------|------|------|
| `qqwry.ipdb` | 位置 | [nmgliangwei/qqwry.ipdb](https://github.com/nmgliangwei/qqwry.ipdb) |
| `Country.mmdb` | 地区与旗帜 | [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip) |
| `ip2asn-combined.tsv.gz` | ASN、网络名称与 ASN 地区 | [iptoasn.com](https://iptoasn.com/) |

这些文件由各自项目提供，遵循其各自的许可协议。

## 使用提示

- **目标格式**：`example.com`、`1.1.1.1`、`2606:4700:4700::1111` 使用 ICMP；`example.com:443`、`1.1.1.1:443`、`[2606:4700:4700::1111]:443` 使用 TCP Ping。带端口的 IPv6 地址须加方括号。
- **批量导入**（Ping 监测 → **批量导入**）：每行一个目标，可在逗号后加别名：`host,alias` 或 `host:port,alias`。以 `#` 开头的行会被忽略。
- **多选**：Ctrl+单击（macOS 为 ⌘+单击）添加或取消一行，Shift+单击选择一段范围，Ctrl+A / ⌘+A 全选。多选时右键菜单只显示适用于全部所选行的操作，如启用 / 禁用 / 删除或添加到 Ping。
- **详情**：双击某行或按 Enter 打开详情窗口。
- **随处发起追踪**：在 Ping 目标或 IP 查询结果上右键即可追踪；TCP 目标会追踪到对应主机。

## WebUI（在浏览器中使用）

桌面版的全部功能（Ping 监测、路由追踪 / MTR、IP 查询、数据文件下载和设置）都可以在网页中使用。网页与桌面窗口共用同一份设置和正在运行的任务，在一端添加的目标会立即出现在另一端。

**在桌面版中启动**：打开 **WebUI** 页面，设置绑定 IP 和端口，点击 **启动**。页面上会显示访问地址，可以复制或直接在浏览器中打开。勾选「打开 HinaTracer 时自动启动 Web 服务」后，每次打开程序都会自动启动。

**无窗口运行**（适合服务器或常开的电脑）：

```
hinatracer --web                                # 使用保存的地址，默认 127.0.0.1:8790
hinatracer --web --listen 0.0.0.0:8790          # 允许其他设备访问（需要密码）
hinatracer --web --web-host 127.0.0.1 --web-port 9000
hinatracer --help
```

访问地址会显示在终端中，并写入 `config.json` 所在目录的 `web.log`。按 Ctrl+C 停止。

**访问保护**

- 默认地址 `127.0.0.1` 只能在本机打开。
- 若要让其他设备访问（例如 `0.0.0.0`），请先在 WebUI 页面设置**访问密码**；未设置密码时不会监听这类地址。无窗口模式下也可以用环境变量 `HINATRACER_WEB_PASSWORD` 设置密码。
- 设置密码后，打开网页需要先登录；修改密码会让所有浏览器退出登录。
- 网页使用 HTTP。在不信任的网络中，请保持 `127.0.0.1`，或通过 HTTPS 反向代理访问。

## 访客模式

在 **WebUI** 页面设置访问密码后，勾选 **允许访客模式**。登录页会多出 **以访客身份登录** 按钮：访客无需密码，可以查看所有监测结果，但不能做任何修改（不能添加或删除目标、开始或停止、修改设置、下载数据或更新），也看不到文件路径和访问设置。页面顶部会显示 **访客** 标记。关闭访客模式后，访客会被退出登录。

## API 令牌

在 **WebUI** 页面（桌面版，或网页的设置页）创建令牌：填写名称并选择权限：

- **管理员**：可以查看和修改所有内容。
- **访客**：只能查看。

令牌**只显示一次**，请立即复制保存。程序只保存令牌的指纹，丢失后无法找回，只能吊销后重新创建。

## AI 客户端（MCP）

WebUI 运行时，HinaTracer 同时是一个 MCP 服务器，支持 MCP 的 AI 助手可以读取监测结果；使用管理员令牌时，还可以替你管理目标和路由追踪。地址显示在 WebUI 页面上：

```
http://127.0.0.1:8790/mcp
```

在 AI 客户端中把它添加为远程（HTTP）MCP 服务器，并在 `Authorization` 请求头中带上 API 令牌，例如：

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

使用访客令牌时，AI 助手只能查看。未设置访问密码时，本机的客户端无需令牌即可连接。

## 命令行

HinaTracer（或 `hinatracer --web`）运行时，可以在终端中控制它：

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

命令默认连接保存的 WebUI 地址。用 `--server http://主机:端口`（或环境变量 `HINATRACER_SERVER`）连接其他电脑，用 `--token 令牌`（或 `HINATRACER_TOKEN`）指定 API 令牌。默认输出表格，加上 `--json` 输出 JSON。

## Ping 历史与 CSV 导出

打开 Ping 目标（或路由跳）的详情窗口，在延迟图上方选择范围：**按次**、**按分钟**、**按小时**、**按天**、**按周**、**按月**。汇总范围会显示平均值曲线、最小–最大阴影带，以及红色丢包率柱。

在 **设置 → Ping 历史** 中勾选 **保存 Ping 历史到磁盘**，并设置保留天数（默认 90）。数据以 SQLite 保存在数据目录的 `history.db`。旧的原始采样会自动汇总为分钟/小时数据。关闭保存时只使用内存中最近的数据。

详情窗口的 **导出 CSV**（或 `GET /api/history.csv`，或 `hinatracer ping history <id> --csv`）会写出列：`time,target,alias,ip,samples,avg,min,max,loss%`。

开启保存后，Ping 与 HTTP 列表的累计统计（次数、成功率、失败率、最近/平均/最小/最大）在重启后保留并继续累计；重启后的中位值取自最近 1000 个成功样本。此时**重置统计**会永久清除已保存的统计，因此会先弹出确认（WebUI 同样如此）；命令行需加 `--yes`，AI 客户端需传 `confirm`。Traceroute 各跳不保存。

## HTTP 监测

「HTTP 监测」页（在 Ping 监测下方）按周期检查网站和 API。

- 可添加单个 URL，或用「批量添加」每行一个 URL（空格后可写别名）。
- 每个目标可单独设置（右键 → 编辑）：方法 GET / HEAD / POST / PUT / PATCH / DELETE / OPTIONS 或自定义（POST、PUT、PATCH、DELETE 和自定义方法可发送请求体，并可指定 Content-Type）、间隔（默认 30 秒）、超时、期望状态码（如 `200-299,301` 或 `2xx`，留空为 200–399）、对页面前 1 MB 的文本或正则匹配（可反向）、自定义请求头、跟随重定向、忽略 TLS 证书错误（默认关闭）。
- 在 WebUI 中拖动列标题右边缘可调整列宽，浏览器会按表格分别记住（双击边缘恢复默认）。
- 列表与 Ping 列表相同：IP、位置、地区、ASN、ASN 地区、成功/失败次数、成功率与失败率、最近/平均/最小/最大/中位值响应时间、状态码、最近检测、最近成功与最近失败。可点表头排序、Ctrl/Shift 多选（右键对所选批量操作）、拖动排序，并记住列宽。双击一行（或按 Enter）查看 DNS / 连接 / TLS / 首字节耗时、证书剩余天数与趋势图（所有范围，可导出 CSV）。开启保存后历史与 Ping 一样写入磁盘。
- 请求头的值不会写入日志，访客也看不到。
- 同样的功能在 WebUI、AI 客户端（MCP）和命令行中都可用（`hinatracer http list`、`http add <url> ...`，详见 `hinatracer help`）。

## 远程模式（桌面程序作为客户端）

设置 →「数据源」→「远程 WebUI」可让本窗口显示并控制另一台 HinaTracer（例如服务器软件包），而不是本机。

- 填写 WebUI 地址，用管理员密码、API 令牌或访客（只读）登录。
- 密码只用于一次性换取 API 令牌，不会被保存；令牌保存在仅当前用户可读的配置文件中。
- 远程模式下本机监测停止，本机 WebUI 页停用；顶部显示连接状态，断线会自动重连。
- 各页面与本机使用一致：Ping（添加、导入、编辑、排序、详情图表、CSV）、Traceroute 标签页、IP 查询与 HTTP。管理员在设置中还会看到「远程服务器设置」（服务器的语言、主题、数据文件与下载、启动选项、历史、更新、WebUI 监听地址、访客模式与 API 令牌）。数据源、主题和本机语言只影响本机。访客一切只读。
- 在「本地」→「切换回本地」即可恢复。

## 服务器软件包（Debian/Ubuntu）

仅运行 WebUI、由 systemd 托管的无界面安装：

```bash
sudo apt install ./HinaTracer-v<ver>-linux-amd64-server.deb
```

会安装 `/usr/bin/hinatracer`、单元 `hinatracer.service`（用户 `hinatracer`，`CAP_NET_RAW`，加固）、默认配置 `/etc/hinatracer/config.json`（conffile）以及数据目录 `/var/lib/hinatracer`。默认监听 `http://127.0.0.1:8790/`。

设置访问密码（绑定非回环地址前必须）：

```bash
sudo -u hinatracer hinatracer web passwd --config /etc/hinatracer/config.json
sudo systemctl restart hinatracer
```

服务器模式不会自动安装更新，请用软件包管理器升级。也可以不装包直接运行：`hinatracer --web --server-mode --config /path/to/config.json`。

## 按浏览器选择语言

任何 WebUI 访客（包括访客模式）都可以选择界面语言；只保存在本浏览器（`localStorage`），不会写入服务器配置。管理员仍可改全局语言，或勾选 **仅此浏览器**。

## 配置与日志

设置、目标、追踪页签和列布局保存在 `config.json` 中。程序崩溃时会在同一目录写入 `crash.log`。

| 系统 | 目录 |
|------|------|
| Windows | `%APPDATA%\hinatracer\` |
| macOS | `~/Library/Application Support/hinatracer/` |
| Linux | `~/.config/hinatracer/` |

## 自定义语言包

将 `*.json` 语言文件放入上述配置目录下的 `lang` 文件夹，或程序所在目录的 `lang` 文件夹，即可在 **设置 → 语言** 中选择。与内置语言包 `code` 相同时会覆盖对应文本，缺失的文本回退为英文。可参考 [`i18n/`](i18n/) 中的内置语言包：

```json
{
  "name": "English",
  "code": "en",
  "strings": {
    "nav.ping": "Ping Monitor"
  }
}
```

## 从源码构建

需要 [`go.mod`](go.mod) 中注明的 Go 版本，无需 CGO。

```bash
go build .                # 为当前平台构建
go tool mygo build        # 在 build/ 中生成打包好的应用
```

## 许可证

[MIT 许可证](LICENSE)。第三方数据文件遵循其各自的许可协议。
