[English](README.md) | [中文(简体)](README.zh-CN.md) | [中文(繁體)](README.zh-HK.md) | 日本語

# HinaTracer

<img src="resources/icon.png" width="96" height="96" alt="HinaTracer アイコン" />

Windows・macOS・Linux 向けのネイティブなデスクトップ用ネットワークツールです。継続的な Ping（ICMP / TCP）、traceroute / MTR、IP 検索ができ、各アドレスの位置・地域・ASN 情報も表示します。

## 機能

- **Ping 監視**：複数のターゲットを同時に監視し、成功率・パケット損失率と、直近 / 平均 / 最小 / 最大 / 中央値の遅延をリアルタイムに表示。ターゲットごとの有効化・無効化、別名、詳細表示に対応し、1 件ずつの追加も一括インポートもできます。
- **TCP Ping**：ターゲットにポートを付ける（例：`example.com:443`）と、ICMP の代わりに TCP 接続時間を測定します。
- **ルート追跡 / MTR**：追跡ごとにタブが分かれます。MTR モードでは各ホップを継続的にプローブし、全タブの一括開始・停止も可能です。ホップはワンクリックで Ping 監視に追加できます。
- **IP 検索**：IP またはドメインを 1 行に 1 件入力。ドメインはすべての IPv4 / IPv6 アドレスに解決され、各アドレスの遅延も簡易測定します。
- **地域と ASN 情報**：各アドレスの位置、地域（旗アイコン付き）、ASN / ネットワーク名、ASN の登録地域を表示。IPv4 と IPv6 の両方に対応しています。
- **使いやすい表**：任意の列で並べ替え、列幅と列順の変更（次回起動時も保持）、複数行を選択して右クリックでまとめて操作できます。
- **アップデート確認**：起動時に GitHub Releases を確認し（正式版のみ、またはプレビュー版を含む）、新バージョンのダウンロード・検証・インストールまで行えます。
- **WebUI**：ブラウザーから全機能を利用。デスクトップ版から起動するほか、ウィンドウなし（`--web`）でも動作し、アクセスパスワードを設定できます。
- **言語とテーマ**：中文(简体)、中文(繁體)、日本語、English に加え、独自の言語パックも利用可能。ライト / ダーク / システムに合わせるの 3 テーマ。

## ダウンロード

最新版は **[GitHub Releases](https://github.com/hinasatou/hinatracer/releases)** から入手できます。

| プラットフォーム | ファイル | 備考 |
|------------------|----------|------|
| Windows (x64) | `HinaTracer-vX.Y.Z-windows-amd64-setup.exe` | インストーラー（推奨）。現在のユーザーにインストールされ、管理者権限は不要 |
| Windows (x64) | `HinaTracer-vX.Y.Z-windows-amd64.zip` | ポータブル版。展開して `HinaTracer.exe` を実行 |
| macOS (Apple シリコン) | `HinaTracer-vX.Y.Z-macos-arm64.zip` | `HinaTracer.app` を同梱 |
| macOS (Intel) | `HinaTracer-vX.Y.Z-macos-amd64.zip` | `HinaTracer.app` を同梱 |
| Linux (x64) | `HinaTracer-vX.Y.Z-linux-amd64.deb` | Debian / Ubuntu とその派生ディストリビューション向け |
| Linux (x64) | `HinaTracer-vX.Y.Z-linux-amd64.tar.gz` | ポータブル版バイナリ（デスクトップエントリとアイコン付き） |
| 共通 | `SHA256SUMS.txt` | ダウンロードの検証用チェックサム |

## プラットフォームごとの注意

**Linux：Ping の権限。** 権限エラーで Ping が失敗する場合は、非特権 ICMP を許可するか、バイナリに raw ソケットの権限を付与してください。

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"   # 再起動まで有効
sudo setcap cap_net_raw+ep /path/to/hinatracer              # または一度だけ設定
```

**macOS：初回起動。** アプリは Apple の署名を受けていないため、Gatekeeper にブロックされることがあります。`HinaTracer.app` を右クリックして **開く** を選ぶか、次を実行してください。

```bash
xattr -dr com.apple.quarantine /Applications/HinaTracer.app
```

## データファイル

位置・地域・ASN の情報は 3 つの無料データファイルから取得します。**設定** の **すべてダウンロード/更新** をクリックすれば一度にまとめて取得できます（手元のファイルを指定することも可能）。データファイルがなくてもアプリは動作しますが、該当する列は「—」と表示されます。

| ファイル | 用途 | 提供元 |
|----------|------|--------|
| `qqwry.ipdb` | 位置 | [nmgliangwei/qqwry.ipdb](https://github.com/nmgliangwei/qqwry.ipdb) |
| `Country.mmdb` | 地域と旗アイコン | [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip) |
| `ip2asn-combined.tsv.gz` | ASN・ネットワーク名・ASN の地域 | [iptoasn.com](https://iptoasn.com/) |

これらのファイルは各プロジェクトが提供しており、それぞれのライセンスに従います。

## 使い方のヒント

- **ターゲットの書式**：`example.com`、`1.1.1.1`、`2606:4700:4700::1111` は ICMP、`example.com:443`、`1.1.1.1:443`、`[2606:4700:4700::1111]:443` は TCP Ping になります。ポート付きの IPv6 アドレスは角括弧で囲んでください。
- **一括インポート**（Ping 監視 → **一括インポート**）：1 行に 1 ターゲット。カンマの後に別名を付けられます：`host,alias` または `host:port,alias`。`#` で始まる行は無視されます。
- **複数選択**：Ctrl+クリック（macOS では ⌘+クリック）で行を追加・解除、Shift+クリックで範囲選択、Ctrl+A / ⌘+A ですべて選択。複数選択時の右クリックメニューには、有効化 / 無効化 / 削除や Ping への追加など、選択したすべての行に使える操作だけが表示されます。
- **詳細**：行をダブルクリックするか Enter キーで詳細ウィンドウを開きます。
- **どこからでもルート追跡**：Ping のターゲットや IP 検索の結果を右クリックして追跡できます。TCP ターゲットはホストまでを追跡します。

## WebUI（ブラウザーで使う）

デスクトップ版のすべての機能（Ping 監視、経路追跡 / MTR、IP 照会、データファイルのダウンロード、設定）を Web ページからも使えます。Web ページとデスクトップ画面は同じ設定と実行中のタスクを共有するため、片方で追加した対象はすぐもう片方にも表示されます。

**デスクトップ版から起動**：**WebUI** ページでバインド IP とポートを指定し、**起動** をクリックします。開くべきアドレスが表示され、コピーやブラウザーで開くことができます。「HinaTracer 起動時に Web サーバーを自動起動」にチェックを入れると毎回自動で起動します。

**ウィンドウなしで実行**（サーバーや常時稼働の PC 向け）：

```
hinatracer --web                                # 保存済みのアドレス（既定 127.0.0.1:8790）
hinatracer --web --listen 0.0.0.0:8790          # 他の端末からも接続可（パスワード必須）
hinatracer --web --web-host 127.0.0.1 --web-port 9000
hinatracer --help
```

アドレスはターミナルに表示され、`config.json` と同じフォルダーの `web.log` にも記録されます。Ctrl+C で停止します。

**アクセス保護**

- 既定のアドレス `127.0.0.1` はこの PC からのみ開けます。
- 他の端末から接続させる場合（例：`0.0.0.0`）は、先に WebUI ページで**アクセスパスワード**を設定してください。パスワードがないとこの種のアドレスでは待ち受けません。ウィンドウなしモードでは環境変数 `HINATRACER_WEB_PASSWORD` でも設定できます。
- パスワードを設定すると、ページを開くときにログインが必要です。パスワードを変更すると、すべてのブラウザーがログアウトされます。
- 通信は HTTP です。信頼できないネットワークでは `127.0.0.1` のまま使うか、HTTPS のリバースプロキシ経由で利用してください。

## ゲストモード

**WebUI** ページでアクセスパスワードを設定したあと、**ゲストモードを許可** にチェックを入れます。ログイン画面に **ゲストとしてログイン** が表示されます。ゲストはパスワード不要ですべての結果を閲覧できますが、変更は一切できません（ターゲットの追加・削除、開始・停止、設定変更、データのダウンロード、アップデートは不可）。ファイルパスやアクセス設定も表示されません。画面上部には **ゲスト** バッジが表示されます。ゲストモードをオフにするとゲストはログアウトされます。

## API トークン

**WebUI** ページ（デスクトップ版、またはブラウザーの設定ページ）でトークンを作成します。名前を入力し、権限を選びます：

- **管理者**：すべて閲覧・変更できます。
- **ゲスト**：閲覧のみ。

トークンは**一度だけ**表示されるので、すぐにコピーしてください。保存されるのはトークンの指紋だけなので、なくした場合は失効させて作り直してください。

## AI クライアント（MCP）

WebUI の実行中、HinaTracer は MCP サーバーとしても動作します。MCP 対応の AI アシスタントが監視結果を読み取り、管理者トークンならターゲットや経路追跡の管理もできます。アドレスは WebUI ページに表示されます：

```
http://127.0.0.1:8790/mcp
```

AI クライアントにリモート（HTTP）MCP サーバーとして追加し、`Authorization` ヘッダーに API トークンを付けます。例：

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

ゲストトークンでは閲覧のみです。アクセスパスワード未設定のときは、このコンピューター上のクライアントはトークンなしで接続できます。

## コマンドライン

HinaTracer（または `hinatracer --web`）の実行中は、ターミナルから操作できます：

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

コマンドは保存済みの WebUI アドレスに接続します。別のコンピューターには `--server http://ホスト:ポート`（または環境変数 `HINATRACER_SERVER`）、API トークンは `--token トークン`（または `HINATRACER_TOKEN`）で指定します。既定は表形式で、`--json` を付けると JSON で出力します。

## 監視履歴と CSV エクスポート

Ping ターゲット（または traceroute のホップ）の詳細ウィンドウを開き、遅延グラフ上で範囲を選びます：**回ごと**、**分ごと**、**時間ごと**、**日ごと**、**週ごと**、**月ごと**。集計範囲では平均を線、最小–最大を帯、パケットロス率を赤い棒で表示します。

**設定 → 監視履歴** で **監視履歴をディスクに保存** を有効にし、保存日数（既定 90）を設定します。Ping と HTTP 監視の履歴と表の統計が対象です。データはデータディレクトリの `history.db`（SQLite）に保存され、古い生データは自動で分・時間単位に集計されます。オフのときはメモリ上の最近のデータだけを使います。

詳細の **CSV をエクスポート**（または `GET /api/history.csv`、`hinatracer ping history <id> --csv`）は列 `time,target,alias,ip,samples,avg,min,max,loss%` を書き出します。

保存を有効にすると、Ping と HTTP の一覧の累計統計（回数、成功率、失敗率、最新/平均/最小/最大）が再起動後も残り、そのまま数え続けます。再起動後の中央値は直近 1000 件の成功サンプルから求めます。このとき**統計リセット**は保存済みの統計を完全に消去するため、先に確認が表示されます（WebUI も同様）。コマンドラインでは `--yes`、AI クライアントは `confirm` が必要です。Traceroute の各ホップは保存しません。

## HTTP 監視

「HTTP 監視」ページ（Ping 監視の下）は Web サイトや API を定期的にチェックします。

- URL を 1 つずつ追加するか、「一括追加」で 1 行に 1 つの URL を入力します（空白の後に別名を書けます）。
- 対象ごとに設定できます（右クリック → 編集）：メソッド GET / HEAD / POST / PUT / PATCH / DELETE / OPTIONS またはカスタム（POST・PUT・PATCH・DELETE とカスタムはリクエストボディと Content-Type を指定可能）、間隔（既定 30 秒）、タイムアウト、期待するステータスコード（例 `200-299,301` や `2xx`。空欄は 200–399）、ページ先頭 1 MB に対する文字列または正規表現の一致（反転可）、カスタムヘッダー、リダイレクトに従う、TLS 証明書エラーを無視（既定はオフ）。
- HinaTracer を開いても HTTP 監視は自動では始まりません。自動で始めるには **設定 → 起動オプション** で **起動時に HTTP 監視を開始** をオンにします（既定はオフ、Ping と同じ）。
- WebUI では列見出しの右端をドラッグして列幅を変更でき、ブラウザが表ごとに記憶します（右端をダブルクリックで元に戻す）。
- 一覧は Ping の一覧と同じです：IP、位置、地域、ASN、ASN 地域、成功/失敗回数、成功率と失敗率、最新/平均/最小/最大/中央値の応答時間、ステータスコード、最終チェック、最終成功と最終失敗。ヘッダーで並べ替え、Ctrl/Shift で複数選択（右クリックでまとめて操作）、ドラッグで並べ替え、列幅は記憶されます。行をダブルクリック（または Enter）で DNS / 接続 / TLS / TTFB の時間、証明書の残り日数、推移グラフ（全範囲、CSV エクスポート）を表示します。保存が有効なら履歴は Ping と同じくディスクに保存されます。
- ヘッダーの値はログに書かれず、ゲストにも表示されません。
- 同じ機能は WebUI、AI クライアント（MCP）、コマンドライン（`hinatracer http list`、`http add <url> ...`。`hinatracer help` 参照）でも使えます。

## リモートモード（デスクトップアプリをクライアントに）

設定 →「データソース」→「リモート WebUI」で、このウィンドウにこの PC ではなく別の HinaTracer（例：サーバーパッケージ）を表示・操作させられます。

- WebUI の URL を入力し、管理者パスワード、API トークン、またはゲスト（読み取り専用）でログインします。
- パスワードは API トークンの取得に一度だけ使い、保存しません。トークンは本人だけが読める設定ファイルに保存されます。
- リモートモード中はローカルの監視が止まり、ローカル WebUI ページは無効になります。上部に接続状態が表示され、切断時は自動で再接続します。
- 各ページはローカルと同じように使えます：Ping（追加、インポート、編集、並べ替え、詳細グラフ、CSV）、Traceroute タブ、IP 検索、HTTP。管理者の場合、設定に「リモートサーバー設定」（サーバーの言語、テーマ、データファイルとダウンロード、起動オプション、履歴、更新、WebUI の待ち受けアドレス、ゲストモード、API トークン）も表示されます。データソース、テーマ、このコンピューターの言語はローカルのみです。ゲストはすべて読み取り専用です。
- 「ローカル」→「ローカルに戻す」で元に戻せます。

## サーバーパッケージ（Debian/Ubuntu）

WebUI だけを systemd で動かすヘッドレス導入：

```bash
sudo apt install ./HinaTracer-v<ver>-linux-amd64-server.deb
```

`/usr/bin/hinatracer`、ユニット `hinatracer.service`（ユーザー `hinatracer`、`CAP_NET_RAW`、強化）、既定設定 `/etc/hinatracer/config.json`（conffile）、データ `/var/lib/hinatracer` を入れます。既定は `http://127.0.0.1:8790/` です。

アクセスパスワードの設定（ループバック以外へバインドする前に必須）：

```bash
sudo -u hinatracer hinatracer web passwd --config /etc/hinatracer/config.json
sudo systemctl restart hinatracer
```

サーバーモードでは自動インストールしません。パッケージマネージャで更新してください。パッケージなしでも `hinatracer --web --server-mode --config /path/to/config.json` で起動できます。

## ブラウザごとの言語

WebUI の訪問者（ゲスト含む）は表示言語を選べます。選択はブラウザの `localStorage` だけに保存され、サーバー設定には書き込みません。管理者は従来どおりアプリ全体の言語を変えられ、**このブラウザのみ** も選べます。

## 設定とログ

設定、ターゲット、追跡タブ、列のレイアウトは `config.json` に保存されます。アプリがクラッシュした場合は同じフォルダーに `crash.log` が書き出されます。

| OS | フォルダー |
|----|------------|
| Windows | `%APPDATA%\hinatracer\` |
| macOS | `~/Library/Application Support/hinatracer/` |
| Linux | `~/.config/hinatracer/` |

## カスタム言語パック

`*.json` の言語ファイルを、上記の設定フォルダー内の `lang` フォルダー、またはアプリ実行ファイルと同じ場所の `lang` フォルダーに置くと、**設定 → 言語** で選べるようになります。組み込みパックと同じ `code` を指定するとその文字列を上書きし、足りない文字列は英語で表示されます。[`i18n/`](i18n/) の組み込みパックをひな形にしてください。

```json
{
  "name": "English",
  "code": "en",
  "strings": {
    "nav.ping": "Ping Monitor"
  }
}
```

## ソースからビルド

[`go.mod`](go.mod) に記載された Go のバージョンが必要です。CGO は不要です。

```bash
go build .                # 現在のプラットフォーム向けにビルド
go tool mygo build        # パッケージ化したアプリを build/ に生成
```

## ライセンス

[MIT ライセンス](LICENSE)。サードパーティのデータファイルはそれぞれのライセンスに従います。
