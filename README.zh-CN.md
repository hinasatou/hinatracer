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

[PolyForm Noncommercial License 1.0.0](LICENSE)：可免费用于非商业用途，不允许商业使用。第三方数据文件遵循其各自的许可协议。
