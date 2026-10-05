[English](README.md) | 简体中文 | [繁體中文](README.zh-HK.md) | [日本語](README.ja.md)

# HinaTracer

<img src="resources/icon.png" width="64" height="64" alt="HinaTracer 图标" />

跨平台原生桌面网络诊断工具：**Ping 监测**与**路由追踪**（MTR），基于 [mygo](https://github.com/egoist/mygo) v0.2.9。结合 **qqwry.ipdb**（ipip 格式）、Loyalsoldier GeoIP 地区旗帜（内嵌 flagcdn PNG，emoji 回退）与 **iptoasn ASN**（编号、组织、注册地区）标注跳点/主机。支持 **IPv4 / IPv6**。

## 功能

1. **Ping 监测**（默认页）— 按间隔持续探测；统计成功/失败、成功率、延迟分位数与时间戳。表格含别名、主机、rDNS、位置、地区、ASN。支持批量导入（每行 `host` 或 `host,alias`）、单目标启用/禁用；右键复制/改别名/启用禁用/详情/查 traceroute/删除；双击打开详情。
2. **路由追踪** — 左侧 Chrome 风格垂直页签（状态点、标题、关闭、右键菜单、可调宽度）。默认 **MTR**：发现路径后按间隔持续探测每一跳；可改为单次 traceroute。多页签并发。支持**全部开始 / 全部停止**。跳点表列与 Ping 对齐。右键可复制、查看跳点详情、将 IP 添加到 Ping。
3. **设置** — 语言（简体/香港繁体/日本語/English，可加载自定义包）、主题、数据文件路径、启动时自动监测。
4. **关于** — 应用名、版本与 GitHub 链接。
5. **配置与崩溃日志** — 见下文。

## 下载

预编译包见 **[GitHub Releases](https://github.com/hinasatou/hinatracer/releases)**（先以草稿创建，需手动发布）。标签 `vX.Y.Z` 常见产物：

| 文件 | 内容 |
|------|------|
| `HinaTracer-vX.Y.Z-windows-amd64.zip` | `HinaTracer.exe`（GUI）、`LICENSE`、`README.md` |
| `HinaTracer-vX.Y.Z-linux-amd64.tar.gz` | 二进制、`.desktop`、图标、`LICENSE`、`README.md` |
| `HinaTracer-vX.Y.Z-linux-amd64.deb` | mygo 生成的 deb |
| `HinaTracer-vX.Y.Z-macos-arm64.zip` / `…-macos-amd64.zip` | `HinaTracer.app` 以及 `LICENSE`、`README.md` |
| `SHA256SUMS.txt` | 上述文件的 SHA-256 校验和 |

## 平台

| 平台 | 说明 | ICMP |
|------|------|------|
| **Windows 10/11** x64 | 无需 WebView2 / CGO | `IcmpSendEcho` / `Icmp6SendEcho2`（一般无需管理员） |
| **macOS** 12+（amd64 / arm64） | 原生 UI | 优先无特权 ICMP 数据报套接字；特权时 raw；回退 `ping` |
| **Linux** amd64 | 原生 UI（无强制 GTK） | 同上。无特权 ICMP 需 `net.ipv4.ping_group_range` 允许当前用户 |

### Linux ping 权限

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"
sudo setcap cap_net_raw+ep /path/to/hinatracer
```

### macOS Gatekeeper

未签名应用首次打开可能被拦截：在「系统设置 → 隐私与安全性」中允许，或右键「打开」。正式分发建议签名与公证。

## 数据文件

在**设置**中指定路径。缺失时位置/地区/ASN 显示为「—」。

| 数据 | 来源 | 说明 |
|------|------|------|
| **qqwry.ipdb** | [nmgliangwei/qqwry.ipdb](https://github.com/nmgliangwei/qqwry.ipdb) | ipip 格式（IPv4/IPv6）。CDN：`https://cdn.bili33.top/gh/nmgliangwei/qqwry.ipdb@main/qqwry.ipdb` |
| **Country.mmdb** | [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip/releases) | MaxMind 地区库（地区旗帜）。CDN：`https://cdn.jsdelivr.net/gh/Loyalsoldier/geoip@release/Country.mmdb` |
| **ip2asn-combined.tsv.gz** | [iptoasn.com](https://iptoasn.com/data/ip2asn-combined.tsv.gz) | ASN 号、组织、注册地区（IPv4+IPv6）。**.gz 可直接使用**；也支持未压缩 `.tsv` |

## 语言包

内置：简体中文、香港繁体、日本語、English。设置中切换立即生效。

首次运行（配置中无 `language` 字段）会按操作系统界面语言自动匹配语言包并写入配置。

自定义：将 `*.json` 放入 `%APPDATA%\hinatracer\lang\`（或 exe / `.app` 旁的 `lang\`）。同 code 会合并/覆盖内置字符串。完整 key 见 `i18n/*.json`。

## 从源码构建

需要 Go **1.27.1+**（mygo v0.2.9；可用 `GOTOOLCHAIN=go1.27.1`）。**无需 CGO**，可交叉编译。

```bash
go tool mygo build
go tool mygo build -platform windows/amd64
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -H windowsgui" -o build/hinatracer.exe .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/hinatracer-debug.exe .
go test ./... && go vet ./...
```

Windows 文件图标：仓库已提交 `rsrc_windows_amd64.syso`，`go build` 会自动链接。再生：`go run scripts/mkicon.go -in <src.jpg>` 后 `bash scripts/genwinres.sh`。

## 配置与崩溃日志

路径：`os.UserConfigDir()/hinatracer/config.json`

| 系统 | 配置目录 |
|------|----------|
| Windows | `%APPDATA%\hinatracer\` |
| macOS | `~/Library/Application Support/hinatracer/` |
| Linux | `~/.config/hinatracer/` |

同目录下的 `crash.log` 记录界面/更新相关 panic，便于排查。

GitHub Actions（`.github/workflows/build.yml`）在 `v*` 标签上跑测试、用 mygo 构建 Windows / Linux / macOS，打包上述产物，并以**草稿** Release 附带 `SHA256SUMS.txt`。

## 许可证

采用 [PolyForm Noncommercial License 1.0.0](LICENSE)。允许非商业用途（个人、研究、非营利/教育等，以许可证正文为准）。**禁止商业用途。** 第三方数据文件与 flagcdn 资源仍遵循其各自许可。
