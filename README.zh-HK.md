[English](README.md) | [中文(简体)](README.zh-CN.md) | 中文(繁體) | [日本語](README.ja.md)

# HinaTracer

<img src="resources/icon.png" width="96" height="96" alt="HinaTracer 圖示" />

適用於 Windows、macOS 與 Linux 的原生桌面網路工具：持續 Ping（ICMP / TCP）、路由追蹤 / MTR 與 IP 查詢，每個位址都附帶位置、地區與 ASN 資訊。

## 功能

- **Ping 監測**：同時監測多個目標，即時顯示成功率、丟包率，以及最近 / 平均 / 最小 / 最大 / 中位延遲。可個別啟用或停用、設定別名、檢視詳情，支援逐一新增或批次匯入。
- **TCP Ping**：為目標加上連接埠（如 `example.com:443`），即改為測量 TCP 連線耗時，而非傳送 ICMP。
- **路由追蹤 / MTR**：每個追蹤各佔一個分頁。MTR 模式持續探測每一跳，可一鍵全部開始或停止，任一跳點都能一鍵加入 Ping 監測。
- **IP 查詢**：輸入 IP 或網域（每行一筆），網域會解析出全部 IPv4 / IPv6 位址，並對每個位址快速測一次延遲。
- **地區與 ASN 資訊**：每個位址顯示位置、地區（附旗幟）、ASN / 網路名稱及 ASN 註冊地區，支援 IPv4 與 IPv6。
- **好用的表格**：任意欄位排序，可調整欄寬與順序（重新啟動後保留），支援多選並以右鍵批次操作。
- **檢查更新**：可於啟動時檢查 GitHub Releases（僅正式版，或包含預覽版），並可自動下載、驗證與安裝新版本。
- **語言與主題**：中文(简体)、中文(繁體)、日本語、English，並支援自訂語言包；淺色、深色或跟隨系統。

## 下載

請前往 **[GitHub Releases](https://github.com/hinasatou/hinatracer/releases)** 下載最新版本。

| 平台 | 檔案 | 說明 |
|------|------|------|
| Windows (x64) | `HinaTracer-vX.Y.Z-windows-amd64-setup.exe` | 安裝程式（建議），為目前使用者安裝，毋須系統管理員權限 |
| Windows (x64) | `HinaTracer-vX.Y.Z-windows-amd64.zip` | 免安裝版，解壓後執行 `HinaTracer.exe` |
| macOS (Apple 晶片) | `HinaTracer-vX.Y.Z-macos-arm64.zip` | 內含 `HinaTracer.app` |
| macOS (Intel) | `HinaTracer-vX.Y.Z-macos-amd64.zip` | 內含 `HinaTracer.app` |
| Linux (x64) | `HinaTracer-vX.Y.Z-linux-amd64.deb` | 適用於 Debian / Ubuntu 及其衍生版 |
| Linux (x64) | `HinaTracer-vX.Y.Z-linux-amd64.tar.gz` | 免安裝執行檔，附桌面項目與圖示 |
| 全部 | `SHA256SUMS.txt` | 用於驗證下載檔案 |

## 平台說明

**Linux：Ping 權限。** 若 Ping 因權限失敗，可允許非特權 ICMP，或為程式授予原始通訊端權限：

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"   # 重新開機前有效
sudo setcap cap_net_raw+ep /path/to/hinatracer              # 或一次性設定
```

**macOS：首次開啟。** 應用程式未經 Apple 簽署，可能被 Gatekeeper 阻擋。在 `HinaTracer.app` 上按右鍵選擇 **打開**，或執行：

```bash
xattr -dr com.apple.quarantine /Applications/HinaTracer.app
```

## 資料檔

位置、地區與 ASN 資訊來自三份免費資料檔。在 **設定** 中按 **下載/更新全部** 即可一次取得（也可以指定既有檔案）。缺少資料檔時程式照常運作，只是相關欄位顯示「—」。

| 檔案 | 用途 | 來源 |
|------|------|------|
| `qqwry.ipdb` | 位置 | [nmgliangwei/qqwry.ipdb](https://github.com/nmgliangwei/qqwry.ipdb) |
| `Country.mmdb` | 地區與旗幟 | [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip) |
| `ip2asn-combined.tsv.gz` | ASN、網路名稱與 ASN 地區 | [iptoasn.com](https://iptoasn.com/) |

這些檔案由各自的專案提供，並受其各自的授權條款約束。

## 使用提示

- **目標格式**：`example.com`、`1.1.1.1`、`2606:4700:4700::1111` 使用 ICMP；`example.com:443`、`1.1.1.1:443`、`[2606:4700:4700::1111]:443` 使用 TCP Ping。帶連接埠的 IPv6 位址須加方括號。
- **批次匯入**（Ping 監測 → **批次匯入**）：每行一個目標，可在逗號後加別名：`host,alias` 或 `host:port,alias`。以 `#` 開頭的行會被略過。
- **多選**：Ctrl+點擊（macOS 為 ⌘+點擊）加入或取消一列，Shift+點擊選取一段範圍，Ctrl+A / ⌘+A 全選。多選時右鍵選單只顯示適用於全部所選列的操作，例如啟用 / 停用 / 刪除或新增到 Ping。
- **詳情**：按兩下某列或按 Enter 開啟詳情視窗。
- **隨處發起追蹤**：在 Ping 目標或 IP 查詢結果上按右鍵即可追蹤；TCP 目標會追蹤到對應主機。

## 設定與日誌

設定、目標、追蹤分頁與欄位配置儲存在 `config.json`。程式當機時會在同一資料夾寫入 `crash.log`。

| 系統 | 資料夾 |
|------|--------|
| Windows | `%APPDATA%\hinatracer\` |
| macOS | `~/Library/Application Support/hinatracer/` |
| Linux | `~/.config/hinatracer/` |

## 自訂語言包

將 `*.json` 語言檔放入上述設定資料夾下的 `lang` 資料夾，或程式所在位置的 `lang` 資料夾，即可在 **設定 → 語言** 中選擇。與內建語言包 `code` 相同時會覆蓋對應文字，缺少的文字會改用英文。可參考 [`i18n/`](i18n/) 中的內建語言包：

```json
{
  "name": "English",
  "code": "en",
  "strings": {
    "nav.ping": "Ping Monitor"
  }
}
```

## 從原始碼建置

需要 [`go.mod`](go.mod) 中標示的 Go 版本，毋須 CGO。

```bash
go build .                # 為目前平台建置
go tool mygo build        # 在 build/ 中產生封裝好的應用程式
```

## 授權條款

[PolyForm Noncommercial License 1.0.0](LICENSE)：可免費用於非商業用途，不允許商業使用。第三方資料檔受其各自的授權條款約束。
