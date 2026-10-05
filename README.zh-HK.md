[English](README.md) | [简体中文](README.zh-CN.md) | 繁體中文 | [日本語](README.ja.md)

# HinaTracer

<img src="resources/icon.png" width="64" height="64" alt="HinaTracer 圖示" />

跨平台原生桌面網絡診斷工具：**Ping 監測**與**路由追蹤**（MTR），建基於 [mygo](https://github.com/egoist/mygo) v0.2.9。結合 **qqwry.ipdb**（ipip 格式）、Loyalsoldier GeoIP 地區旗幟（內嵌 flagcdn PNG，emoji 回退）與 **iptoasn ASN**（編號、組織、註冊地區）標註跳點/主機。支援 **IPv4 / IPv6**。

## 功能

1. **Ping 監測**（預設頁）— 按間隔持續探測；統計成功/失敗、成功率、延遲分位數與時間戳。表格含別名、主機、rDNS、位置、地區、ASN。支援批量匯入（每行 `host` 或 `host,alias`）、單目標啟用/停用；右鍵複製/改別名/啟用停用/詳情/查 traceroute/刪除；雙擊開啟詳情。
2. **路由追蹤** — 左側 Chrome 風格垂直頁籤（狀態點、標題、關閉、右鍵選單、可調寬度）。預設 **MTR**：發現路徑後按間隔持續探測每一跳；可改為單次 traceroute。多頁籤並發。支援**全部開始 / 全部停止**。跳點表列與 Ping 對齊。右鍵可複製、查看跳點詳情、將 IP 加到 Ping。
3. **設定** — 語言（簡體/香港繁體/日本語/English，可載入自訂包）、主題、資料檔路徑、啟動時自動監測。
4. **關於** — 應用名稱、版本與 GitHub 連結。
5. **設定檔與崩潰日誌** — 見下文。

## 下載

預編譯包見 **[GitHub Releases](https://github.com/hinasatou/hinatracer/releases)**。

## 平台

| 平台 | 說明 | ICMP |
|------|------|------|
| **Windows 10/11** x64 | 無需 WebView2 / CGO | `IcmpSendEcho` / `Icmp6SendEcho2`（一般無需管理員） |
| **macOS** 12+（amd64 / arm64） | 原生 UI | 優先無特權 ICMP datagram；特權時 raw；回退 `ping` |
| **Linux** amd64 | 原生 UI（無強制 GTK） | 同上。無特權 ICMP 需 `net.ipv4.ping_group_range` 允許目前使用者 |

### Linux ping 權限

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"
sudo setcap cap_net_raw+ep /path/to/hinatracer
```

### macOS Gatekeeper

未簽名應用首次開啟可能被攔截：在「系統設定 → 隱私權與安全性」中允許，或右鍵「打開」。正式發佈建議簽名與公證。

## 資料檔

在**設定**中指定路徑。缺失時位置/地區/ASN 顯示為「—」。

| 資料 | 來源 | 說明 |
|------|------|------|
| **qqwry.ipdb** | [nmgliangwei/qqwry.ipdb](https://github.com/nmgliangwei/qqwry.ipdb) | ipip 格式（IPv4/IPv6）。CDN：`https://cdn.bili33.top/gh/nmgliangwei/qqwry.ipdb@main/qqwry.ipdb` |
| **Country.mmdb** | [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip/releases) | MaxMind 地區庫（地區旗幟）。CDN：`https://cdn.jsdelivr.net/gh/Loyalsoldier/geoip@release/Country.mmdb` |
| **ip2asn-combined.tsv.gz** | [iptoasn.com](https://iptoasn.com/data/ip2asn-combined.tsv.gz) | ASN 號、組織、註冊地區（IPv4+IPv6）。**.gz 可直接使用**；亦支援未壓縮 `.tsv` |

## 語言包

內建：簡體中文、香港繁體、日本語、English。設定中切換即時生效。

首次執行（設定檔沒有 `language` 欄位）會按作業系統介面語言自動配對語言包並寫入設定。

自訂：將 `*.json` 放入 `%APPDATA%\hinatracer\lang\`（或 exe / `.app` 旁的 `lang\`）。完整 key 見 `i18n/*.json`。

## 從原始碼建置

需要 Go **1.27.1+**（mygo v0.2.9；可用 `GOTOOLCHAIN=go1.27.1`）。**無需 CGO**，可交叉編譯。

```bash
go tool mygo build
go tool mygo build -platform windows/amd64
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -H windowsgui" -o build/hinatracer.exe .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/hinatracer-debug.exe .
go test ./... && go vet ./...
```

Windows 檔案圖示：倉庫已提交 `rsrc_windows_amd64.syso`，`go build` 會自動連結。再生：`go run scripts/mkicon.go -in <src.jpg>` 後 `bash scripts/genwinres.sh`。

## 設定檔與崩潰日誌

路徑：`os.UserConfigDir()/hinatracer/config.json`

| 系統 | 設定目錄 |
|------|----------|
| Windows | `%APPDATA%\hinatracer\` |
| macOS | `~/Library/Application Support/hinatracer/` |
| Linux | `~/.config/hinatracer/` |

同目錄下的 `crash.log` 記錄介面/更新相關 panic，方便排查。

## 授權條款

採用 [PolyForm Noncommercial License 1.0.0](LICENSE)。允許非商業用途（個人、研究、非營利/教育等，以授權條款正文為準）。**禁止商業用途。** 第三方資料檔與 flagcdn 資源仍遵循其各自授權。
