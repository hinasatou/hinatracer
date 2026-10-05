[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-HK.md) | 日本語

# HinaTracer

<img src="resources/icon.png" width="64" height="64" alt="HinaTracer アイコン" />

クロスプラットフォームのネイティブ UI ネットワーク診断ツール：**Ping 監視**と**Traceroute**（MTR）。[mygo](https://github.com/egoist/mygo) v0.2.9 製。**qqwry.ipdb**（IPIP 形式）、Loyalsoldier GeoIP の地域フラグ（埋め込み flagcdn PNG、絵文字フォールバック）、**iptoasn ASN**（番号・組織・登録地域）でホップ/ホストを注釈。**IPv4 / IPv6** 対応。

## 機能

1. **Ping 監視**（デフォルト）— 間隔付き ICMP。成功/失敗、成功率、遅延の last/avg/min/max/median、タイムスタンプ。別名・ホスト・rDNS・位置・地域・ASN。一括インポート（行ごと `host` または `host,alias`）、有効/無効、コンテキストメニュー、ダブルクリックで詳細。
2. **Traceroute** — 左の Chrome 風縦タブ（状態ドット、タイトル、閉じる、右クリック、幅変更）。デフォルト **MTR**。複数タブ同時実行。**すべて開始 / すべて停止**対応。ホップ表は Ping と同様の統計列。右クリックでコピー、ホップ詳細、Ping への追加。
3. **設定** — 言語（簡体/香港繁体/日本語/English＋カスタム）、テーマ、データファイルパス、起動時自動開始。
4. **About** — アプリ名・バージョン・GitHub。
5. **設定とクラッシュログ** — 下記参照。

## ダウンロード

バイナリは **[GitHub Releases](https://github.com/hinasatou/hinatracer/releases)** で公開しています。

## プラットフォーム

| プラットフォーム | 備考 | ICMP |
|------------------|------|------|
| **Windows 10/11** x64 | WebView2 / CGO 不要 | `IcmpSendEcho` / `Icmp6SendEcho2` |
| **macOS** 12+（amd64 / arm64） | ネイティブ UI | 非特権 ICMP datagram 優先。raw / `ping` フォールバック |
| **Linux** amd64 | ネイティブ UI（必須 GTK なし） | 同上。非特権 ICMP には `net.ipv4.ping_group_range` が必要 |

### Linux の ping 権限

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"
sudo setcap cap_net_raw+ep /path/to/hinatracer
```

### macOS Gatekeeper

未署名アプリは初回にブロックされることがあります。「システム設定 → プライバシーとセキュリティ」で許可するか、右クリック →「開く」。配布時は署名・公証を推奨。

## データファイル

**設定**でパスを指定。欠落時は位置/地域/ASN が「—」。

| データ | 入手先 | 備考 |
|--------|--------|------|
| **qqwry.ipdb** | [nmgliangwei/qqwry.ipdb](https://github.com/nmgliangwei/qqwry.ipdb) | IPIP 形式（IPv4/IPv6） |
| **Country.mmdb** | [Loyalsoldier/geoip](https://github.com/Loyalsoldier/geoip/releases) | 地域フラグ用 MaxMind DB |
| **ip2asn-combined.tsv.gz** | [iptoasn.com](https://iptoasn.com/data/ip2asn-combined.tsv.gz) | ASN・組織・登録地域。**.gz をそのまま利用可**（`.tsv` も可） |

## 言語パック

組み込み：簡体中国語、香港繁体、日本語、English。設定ですぐ切替。

初回起動（設定に `language` がない場合）は OS の UI 言語を検出し、最適なパックを選んで保存します。

カスタム：`*.json` を `%APPDATA%\hinatracer\lang\`（または exe / `.app` 横の `lang\`）へ。キー一覧は `i18n/*.json`。

## ソースからビルド

Go **1.27.1+**（mygo v0.2.9；`GOTOOLCHAIN=go1.27.1` 可）。**CGO 不要**。

```bash
go tool mygo build
go tool mygo build -platform windows/amd64
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -H windowsgui" -o build/hinatracer.exe .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/hinatracer-debug.exe .
go test ./... && go vet ./...
```

Windows のファイルアイコン用 `rsrc_windows_amd64.syso` はリポジトリに含まれ、`go build` で自動リンクされます。再生成：`go run scripts/mkicon.go -in <src.jpg>` の後 `bash scripts/genwinres.sh`。

## 設定とクラッシュログ

パス：`os.UserConfigDir()/hinatracer/config.json`

| OS | 設定ディレクトリ |
|----|------------------|
| Windows | `%APPDATA%\hinatracer\` |
| macOS | `~/Library/Application Support/hinatracer/` |
| Linux | `~/.config/hinatracer/` |

同じディレクトリの `crash.log` に UI/更新の panic を記録します。

## ライセンス

[PolyForm Noncommercial License 1.0.0](LICENSE) を採用しています。非営利目的の利用（個人・研究・非営利／教育など。詳細はライセンス本文）は許可されます。**商用利用は許可されません。** サードパーティのデータファイルおよび flagcdn 資産は各ライセンスに従います。
