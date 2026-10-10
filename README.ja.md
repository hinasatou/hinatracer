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

[PolyForm Noncommercial License 1.0.0](LICENSE)：非商用であれば無料で利用できますが、商用利用は認められていません。サードパーティのデータファイルはそれぞれのライセンスに従います。
