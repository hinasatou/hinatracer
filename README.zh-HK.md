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
- **WebUI**：在瀏覽器中使用全部功能，可從桌面版啟動，也可無視窗執行（`--web`），支援訪問密碼。
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

兩種方式下路由追蹤 / MTR 都能顯示每一跳：有該權限（或以 root、伺服器安裝套件的服務執行）時使用原始 ICMP 通訊端；沒有時使用非特權 ICMP 通訊端，並從通訊端的錯誤佇列讀取各路由器的回應。

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

## WebUI（在瀏覽器中使用）

桌面版的全部功能（Ping 監測、路由追蹤 / MTR、IP 查詢、資料檔下載和設定）都可以在網頁中使用。網頁與桌面視窗共用同一份設定和正在執行的工作，在一端新增的目標會立即出現在另一端。

**在桌面版中啟動**：開啟 **WebUI** 頁面，設定綁定 IP 和連接埠，按 **啟動**。頁面上會顯示訪問地址，可以複製或直接在瀏覽器中開啟。勾選「開啟 HinaTracer 時自動啟動 Web 服務」後，每次開啟程式都會自動啟動。

**無視窗執行**（適合伺服器或常開的電腦）：

```
hinatracer --web                                # 使用已儲存的地址，預設 127.0.0.1:8790
hinatracer --web --listen 0.0.0.0:8790          # 允許其他裝置訪問（需要密碼）
hinatracer --web --web-host 127.0.0.1 --web-port 9000
hinatracer --help
```

訪問地址會顯示在終端機中，並寫入 `config.json` 所在目錄的 `web.log`。按 Ctrl+C 停止。

**訪問保護**

- 預設地址 `127.0.0.1` 只能在本機開啟。
- 若要讓其他裝置訪問（例如 `0.0.0.0`），請先在 WebUI 頁面設定**訪問密碼**；未設定密碼時不會監聽這類地址。無視窗模式下也可以用環境變數 `HINATRACER_WEB_PASSWORD` 設定密碼。
- 設定密碼後，開啟網頁需要先登入；修改密碼會讓所有瀏覽器登出。
- 網頁使用 HTTP。在不信任的網絡中，請保持 `127.0.0.1`，或透過 HTTPS 反向代理訪問。

## 訪客模式

在 **WebUI** 頁面設定存取密碼後，勾選 **允許訪客模式**。登入頁會多出 **以訪客身份登入** 按鈕：訪客無需密碼，可以查看所有監測結果，但不能作任何修改（不能新增或刪除目標、開始或停止、修改設定、下載資料或更新），也看不到檔案路徑和存取設定。頁面頂部會顯示 **訪客** 標記。關閉訪客模式後，訪客會被登出。

## API 令牌

在 **WebUI** 頁面（桌面版，或網頁的設定頁）建立令牌：填寫名稱並選擇權限：

- **管理員**：可以查看和修改所有內容。
- **訪客**：只能查看。

令牌**只顯示一次**，請立即複製保存。程式只保存令牌的指紋，遺失後無法找回，只能撤銷後重新建立。

## AI 客戶端（MCP）

WebUI 運行時，HinaTracer 同時是一個 MCP 伺服器，支援 MCP 的 AI 助手可以讀取監測結果；使用管理員令牌時，還可以替你管理目標和路由追蹤。地址顯示在 WebUI 頁面上：

```
http://127.0.0.1:8790/mcp
```

在 AI 客戶端中把它新增為遠端（HTTP）MCP 伺服器，並在 `Authorization` 請求標頭中帶上 API 令牌，例如：

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

使用訪客令牌時，AI 助手只能查看。未設定存取密碼時，本機的客戶端無需令牌即可連接。

## 命令列

HinaTracer（或 `hinatracer --web`）運行時，可以在終端中控制它：

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

命令預設連接已儲存的 WebUI 地址。用 `--server http://主機:連接埠`（或環境變數 `HINATRACER_SERVER`）連接其他電腦，用 `--token 令牌`（或 `HINATRACER_TOKEN`）指定 API 令牌。預設輸出表格，加上 `--json` 輸出 JSON。

## 監測歷史與 CSV 匯出

開啟 Ping 目標（或路由躍點）的詳情視窗，在延遲圖上方選擇範圍：**按次**、**按分鐘**、**按小時**、**按天**、**按週**、**按月**。彙總範圍會顯示平均值曲線、最小–最大陰影帶，以及紅色丟包率柱。

在 **設定 → 監測歷史** 勾選 **儲存監測歷史到磁碟**，並設定保留日數（預設 90）。包括 Ping 與 HTTP 監測的歷史和表格統計。資料以 SQLite 保存在資料目錄的 `history.db`。舊的原始取樣會自動彙總為分鐘／小時資料。關閉儲存時只使用記憶體中最近的資料。

詳情視窗的 **匯出 CSV**（或 `GET /api/history.csv`，或 `hinatracer ping history <id> --csv`）會寫出欄位：`time,target,alias,ip,samples,avg,min,max,loss%`。

開啟儲存後，Ping 與 HTTP 列表的累計統計（次數、成功率、失敗率、最近/平均/最小/最大）在重新啟動後保留並繼續累計；重新啟動後的中位值取自最近 1000 個成功樣本。此時**重設統計**會永久清除已儲存的統計，因此會先彈出確認（WebUI 亦然）；命令列需加 `--yes`，AI 用戶端需傳 `confirm`。Traceroute 各跳不儲存。

## HTTP 監測

「HTTP 監測」頁（在 Ping 監測下方）按週期檢查網站和 API。

- 可新增單個 URL，或用「批次新增」每行一個 URL（空格後可寫別名）。
- 每個目標可單獨設定（右鍵 → 編輯）：方法 GET / HEAD / POST / PUT / PATCH / DELETE / OPTIONS 或自訂（POST、PUT、PATCH、DELETE 及自訂方法可傳送請求內容，並可指定 Content-Type）、間隔（預設 30 秒）、逾時、期望狀態碼（如 `200-299,301` 或 `2xx`，留空為 200–399）、對頁面前 1 MB 的文字或正規表示式比對（可反向）、自訂請求標頭、跟隨重新導向、忽略 TLS 憑證錯誤（預設關閉）。
- 開啟 HinaTracer 時 HTTP 監測預設不會自動開始；如需自動開始，請在 **設定 → 啟動選項** 勾選 **啟動時自動開始 HTTP 監測**（預設關閉，與 Ping 一致）。
- 在 WebUI 中拖動欄標題右邊緣可調整欄寬，瀏覽器會按表格分別記住（雙擊邊緣還原預設）。
- 列表與 Ping 列表相同：IP、位置、地區、ASN、ASN 地區、成功/失敗次數、成功率與失敗率、最近/平均/最小/最大/中位值回應時間、狀態碼、最近檢測、最近成功與最近失敗。可點表頭排序、Ctrl/Shift 多選（右鍵對所選批次操作）、拖曳排序，並記住欄寬。按兩下一行（或按 Enter）查看 DNS / 連線 / TLS / 首位元組耗時、憑證剩餘天數與趨勢圖（所有範圍，可匯出 CSV）。開啟儲存後歷史與 Ping 一樣寫入磁碟。
- 請求標頭的值不會寫入日誌，訪客也看不到。
- 同樣的功能在 WebUI、AI 用戶端（MCP）和命令列中都可用（`hinatracer http list`、`http add <url> ...`，詳見 `hinatracer help`）。
- 所有表格（Ping、HTTP、路由追蹤各跳、IP 查詢；桌面版、遠端模式和 WebUI）都支援 Ctrl+A 全選；右鍵選單中的 **複製欄** 可選一欄，把所選各行該欄的值按目前顯示順序複製，每行一個（空值會略過）。
- **桌面提醒**（設定 → 桌面提醒，僅桌面版，預設全部關閉）：目標斷線（連續失敗達到設定次數，預設 3 次）或恢復（可選）時發送系統通知，短時間內的多條會合併為一條；顯示系統匣圖示（顯示視窗、全部開始/停止 Ping 或 HTTP 監測、結束），並可在關閉視窗時最小化到系統匣。使用遠端資料來源時同樣有效。
- 使用遠端資料來源時，設定頁會清楚區分 **本機** 設定（只影響這部電腦）和 **遠端** 伺服器設定；只在切回本機資料來源後才生效的本機選項放在摺疊的分組裡。

## 遠端模式（桌面程式作為用戶端）

設定 →「資料來源」→「遠端 WebUI」可讓本視窗顯示並控制另一台 HinaTracer（例如伺服器套件），而不是本機。

- 填寫 WebUI 位址，用管理員密碼、API 權杖或訪客（唯讀）登入。
- 密碼只用於一次性換取 API 權杖，不會被儲存；權杖儲存在僅目前使用者可讀的設定檔中。
- 遠端模式下本機監測停止，本機 WebUI 頁停用；頂部顯示連線狀態，斷線會自動重新連線。
- 各頁面與本機使用一致：Ping（新增、匯入、編輯、排序、詳情圖表、CSV）、Traceroute 分頁、IP 查詢與 HTTP。管理員在設定中亦會看到「遠端伺服器設定」（伺服器的語言、主題、資料檔與下載、啟動選項、歷史、更新、WebUI 監聽位址、訪客模式與 API 權杖）。資料來源、主題和本機語言只影響本機。訪客一切唯讀。
- **啟動時自動連接遠端 WebUI**（設定 → 資料來源，連接成功後顯示，預設關閉）：程式啟動時直接進入遠端模式並用已儲存的權杖連接；伺服器連不上時顯示狀態並持續重試，也可以一鍵切換回本機。
- 在「本機」→「切換回本機」即可恢復。

## 伺服器套件（Debian/Ubuntu）

僅執行 WebUI、由 systemd 管理的無介面安裝：

```bash
sudo apt install ./HinaTracer-v<ver>-linux-amd64-server.deb
```

會安裝 `/usr/bin/hinatracer`、單元 `hinatracer.service`（使用者 `hinatracer`，`CAP_NET_RAW`，強化）、預設設定 `/etc/hinatracer/config.json`（conffile）以及資料目錄 `/var/lib/hinatracer`。預設監聽 `http://127.0.0.1:8790/`。

設定存取密碼（綁定非迴路位址前必須）：

```bash
sudo -u hinatracer hinatracer web passwd --config /etc/hinatracer/config.json
sudo systemctl restart hinatracer
```

伺服器模式不會自動安裝更新，請用套件管理器升級。也可以不安裝套件直接執行：`hinatracer --web --server-mode --config /path/to/config.json`。

## 依瀏覽器選擇語言

任何 WebUI 訪客（包括訪客模式）都可以選擇介面語言；只保存在本瀏覽器（`localStorage`），不會寫入伺服器設定。管理員仍可改全域語言，或勾選 **僅此瀏覽器**。

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

[MIT 授權條款](LICENSE)。第三方資料檔受其各自的授權條款約束。
