# agent-gh-repo-token

**Coding Agent 用に、repository 単位で scope を絞った GitHub App installation token を発行する CLI。**

長期間有効な PAT を渡す代わりに、worker が作業を始めるたびに

- 必要な repo 1 つだけ
- 必要な permission だけ (`contents:write`, `pull_requests:write` 等)
- 1 時間で自動失効

の installation token を mint して渡すことで、漏洩リスクを最小化します。

```bash
agent-gh-repo-token --repo td72/foo
# → ghs_xxxx... (1時間有効、td72/foo の指定 permission だけ)
```

## なぜこれが必要か

Coding Agent (Claude Code, Cursor, Aider 等) に GitHub アクセスを与える典型的な方法は:

| 方法 | 問題点 |
|---|---|
| Classic PAT | scope が粗く `repo` で全 repo に効く、長期間有効 |
| Fine-grained PAT | repo 単位に絞れるが Web UI でしか発行できず、自動化困難・長期有効 |
| `gh auth login` | ホスト全体で 1 token、OAuth scope は粗い、ユーザ全 repo に効く |

このツールは GitHub App + installation token のフローを CLI で完結させ、

- repo 単位 scope (mint 時に `repositories=[...]` で API レベルで絞る)
- permission 単位 scope (mint 時に `permissions={...}` で App 上限から削る)
- 自動失効 (GitHub の仕様で 1 時間)

を Coding Agent に渡すための薄いラッパーを提供します。

## インストール

単一バイナリを GitHub Releases から取得します (ランタイム依存なし)。

```bash
# install スクリプト (OS/arch を自動判定して /usr/local/bin に配置)
curl -fsSL https://raw.githubusercontent.com/td72/agent-gh-repo-token/main/scripts/install.sh | sh
```

手動で落とす場合は対象 asset を直接ダウンロード:

| OS / arch | asset 名 |
|---|---|
| macOS (Apple Silicon) | `agent-gh-repo-token-darwin-arm64` |
| Linux (x86_64) | `agent-gh-repo-token-linux-amd64` |
| Linux (arm64) | `agent-gh-repo-token-linux-arm64` |

```bash
curl -fsSL -o agent-gh-repo-token \
  https://github.com/td72/agent-gh-repo-token/releases/latest/download/agent-gh-repo-token-darwin-arm64
chmod +x agent-gh-repo-token && sudo mv agent-gh-repo-token /usr/local/bin/
```

Go が入っていれば `go install` でも可:

```bash
go install github.com/td72/agent-gh-repo-token@latest
```

### 前提ツール

- `op` (1Password CLI) — App private key 等の保管に使用。Vault 認証済みであること。
- (ソースからビルドする場合のみ) Go — バージョンは [`mise.toml`](./mise.toml) で固定
- (将来) 他の secret backend を追加予定

## セットアップ (1 回だけ)

### 1. GitHub App を作る

<https://github.com/settings/apps/new>

| 設定項目 | 推奨値 |
|---|---|
| GitHub App name | `agent-gh-repo-token` 等 (グローバル一意) |
| Homepage URL | リポジトリ URL でOK |
| Webhook | **Active のチェックを外す** |
| Identifying and authorizing users (OAuth 系) | **触らない** — Callback URL は空、各チェックも全てオフ |
| Repository permissions | Contents / Pull requests を Read and write (worker が使う分だけ) |
| Where can this GitHub App be installed? | Only on this account (個人用途なら) |

ここで付与した Repository permissions が **トークンの上限**です。config の `permissions`
はこの範囲内でしか絞れない (超えると mint 時に弾かれる) ので、read-only worker に
`issues` / `actions` 等の `read` を渡したいなら、App 側にもその権限を足しておきます。

> このツールは App 自身として動く **installation token** だけを使い、ユーザーの
> OAuth (user-to-server) トークンは使いません。そのため OAuth 系の設定は不要です。
>
> mint 時に repo・permission を最小化できるのは installation token だけで、
> user-to-server token は「ユーザーの権限 ∩ App install 範囲」までしか絞れません
> (= `gh auth login` 相当)。本ツールが前者を使うのはこの粒度のためです。

作成後:
1. **App ID** をメモ (6〜7桁の整数)
2. **Generate a private key** → `.pem` がダウンロードされる

### 2. 対象 repo に install

<https://github.com/settings/apps/YOUR-APP-NAME/installations> から `Install` を押し:
- **Only select repositories** で対象 repo を選択

完了後の URL `https://github.com/settings/installations/<INSTALLATION_ID>` の数字が **installation_id**。

### 3. 1Password item に保存

App ごとに 1 つ item を作り、`app_id` / `installation_id` / `private_key` を保存します
(`private_key` は `.pem` の中身 = `-----BEGIN ...` から `-----END ...` まで)。private key
は**改行を含む PEM** なので保存方法に注意。方法は2通り:

**推奨: SSH Key アイテム** — 鍵を multiline でネイティブに扱うため改行が壊れません。

1. **SSH Key** カテゴリの item を作り `.pem` をインポート (GitHub App の鍵は RSA なので可)
2. カスタムフィールド `app_id` / `installation_id` を追加

ツールは SSHKEY フィールドを検出して `op read` で取得します。1Password は SSH Key
アイテムでも元の PEM (`-----BEGIN RSA PRIVATE KEY-----`, PKCS#1) を返すのでそのまま使えます。

**別解: login item のテキストフィールド** — UI で password フィールドに貼ると改行が
潰れるので、必ず CLI で `.pem` から流し込みます:

```bash
op item create \
  --category=login \
  --title="agent-gh-repo-token" \
  --vault=Personal \
  app_id="1234567" \
  installation_id="78901234" \
  private_key="$(cat ~/Downloads/agent-gh-repo-token.YYYY-MM-DD.private-key.pem)"
```

どちらの場合も item を `op://<vault>/<item>` として次の `credentials` から参照します。

> vault 名はアカウント種別で変わります。個人 (Individual / Families) は既定で
> `Personal`、**1Password Business は `Employee`**。`--vault` と `op://<vault>/...`
> は自分の vault 名に合わせてください (以下の例は `Personal`)。

### 4. `~/.config/agent-gh-repo-token/repos.toml` を書く

```toml
# org / user 単位のデフォルト
["github.com/td72"]
credentials = "op://Personal/agent-gh-repo-token"
permissions = { contents = "write", pull_requests = "write" }

# repo 単位の上書き (必要なものだけ書く): read-only な worker
["github.com/td72/secret-stuff"]
permissions = { contents = "read", pull_requests = "read", issues = "read" }
```

`permissions` はあくまで **例**です。挙動は:

- **書かない** → App に付与された全権限のトークンが出る
- **書く** → そこへ絞り込む。ただし**絞れるのは App 自身に付与した権限の範囲内だけ**
  (App が持たない権限を要求すると GitHub が弾く)

read-only 用途なら `contents` に加え `pull_requests` / `issues` / `actions` /
`checks` / `statuses` / `security_events` の `read` も有用 (使う分だけ App 側にも
付与しておく)。config のキーは `<host>/<owner>[/<repo>]` 形式 (host 必須)。詳細は
[`examples/repos.toml`](./examples/repos.toml) 参照。

## 使い方

```bash
agent-gh-repo-token --repo td72/foo
# → ghs_xxxx... (stdout に token のみ)

# host は省略すると github.com。GHES なら明示する
agent-gh-repo-token --repo github.com/td72/foo
agent-gh-repo-token --repo ghe.corp/team/foo

# 別 config を使う
agent-gh-repo-token --repo td72/foo --config /path/to/repos.toml
```

`--repo` は `gh` CLI と同じ `[<host>/]<owner>/<repo>` 形式。host を省略すると
`github.com` を補完します。

### 終了コードの設計

| Code | 意味 |
|---|---|
| 0 | token を stdout に出力 |
| 2 | config ファイルが存在しない |
| 3 | repo に該当する entry が無い |
| 4 | 1Password の item 取得に失敗 |
| 5 | 必須フィールド (app_id / installation_id / private_key) が欠落 |
| 6 | JWT 生成または GitHub API 呼び出しに失敗 |

呼び出し側 (Coding Agent runner 等) は、非ゼロ終了時は warning ログだけ出して
GitHub 認証なしで続行する設計を想定。

## Config の解決規則

`agent-gh-repo-token --repo github.com/td72/foo` を実行した場合 (host 省略時は
`github.com` 補完後に照合):

```
1. ["github.com/td72/foo"]   を引く  (repo-level)
2. ["github.com/td72"]       を引く  (org/user-level)
3. 上記を shallow merge し、repo > org で per-key 上書き
4. どちらも無ければ exit 3
```

`credentials` から取った App の認証情報は config の値で上書き可能 (例外的なケース用)。

### 例: 1 App を別 org でも使う

td72 の App を kiconia でも install してもらった場合 (App を public 化していれば可):

```toml
["github.com/td72"]
credentials     = "op://Personal/agent-gh-repo-token"
permissions     = { contents = "write", pull_requests = "write" }

["github.com/kiconia"]
credentials     = "op://Personal/agent-gh-repo-token"  # App 自体は同じ
installation_id = 99999999                             # でも install は別
permissions     = { contents = "write" }
```

`installation_id` を config 側で書けば 1Password の値を上書きします。

## Coding Agent への組み込み例

### bash で sandbox 環境変数 / secret として渡す

```bash
# origin リモートから owner/repo を取り出す (host は --repo 側で github.com 補完)
repo=$(git remote get-url origin | sed -E 's#.*github.com[:/]([^/]+/[^/.]+).*#\1#')

if token=$(agent-gh-repo-token --repo "$repo" 2>/dev/null); then
  # Docker Sandboxes に secret として渡す
  echo "$token" | sbx secret set "$sandbox_name" github
fi
```

### Docker run の env として渡す

```bash
token=$(agent-gh-repo-token --repo "$repo")
docker run --rm -e GH_TOKEN="$token" my-coding-agent-image
```

### [wt-worker](https://github.com/td72/claude-code-tools/tree/main/wt-worker) からの利用

wt-worker plugin が標準で対応 (PATH に `agent-gh-repo-token` があれば自動で使用)。

## アーキテクチャ

```
┌──────────────────────────────────────────────────────────┐
│ agent-gh-repo-token --repo github.com/td72/foo           │
│   │                                                      │
│   ├─ 1. ~/.config/agent-gh-repo-token/repos.toml を読む  │
│   │     repo > org でマージ                              │
│   │                                                      │
│   ├─ 2. op item get op://Personal/agent-gh-repo-token    │
│   │     → app_id, installation_id, private_key           │
│   │                                                      │
│   ├─ 3. App private key で JWT 生成 (RS256, 有効 10 分)  │
│   │                                                      │
│   ├─ 4. POST /app/installations/{id}/access_tokens       │
│   │     body: { repositories: ["foo"],                   │
│   │             permissions: { ... } }                   │
│   │                                                      │
│   └─ 5. stdout に token を出力 (有効 1 時間)             │
└──────────────────────────────────────────────────────────┘
```

## 設計指針

### 「最小限」を維持する

- config schema は最小: `credentials` + `permissions` の 2 キーが基本
- 1 ツール 1 目的: token を 1 つ stdout に出すだけ。secret store への書き込み等は呼び出し側
- 失敗時は素直に non-zero exit (silent fallback はしない)
- 標準出力には token 以外を一切混ぜない (script 化が容易)

### 拡張ポイント (将来)

- secret backend の plugin 化 (env var, file, AWS Secrets Manager, Vault 等)
- `--permissions` フラグで config を上書き
- token cache (1時間以内の同じ要求は cache を返す)
- `--format json` で expires_at 等もまとめて返す

## 開発

ツールチェインは [mise](https://mise.jdx.dev/) で固定 (`mise.toml` / `mise.lock`)。

```bash
mise install      # mise.lock 固定の Go を入れる
mise run test     # go test ./...
mise run vet      # go vet ./...
mise run build    # ./agent-gh-repo-token を生成
mise run dist     # dist/ に各 OS/arch 向けバイナリをクロスコンパイル
```

`v*` タグを push すると [`.github/workflows/release.yml`](./.github/workflows/release.yml) が
全 OS/arch のバイナリをビルドして GitHub Releases に添付します。

## 関連

- [GitHub Apps: Authenticating as an installation](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-as-a-github-app-installation)
- [1Password CLI](https://developer.1password.com/docs/cli/)
- [wt-worker](https://github.com/td72/claude-code-tools/tree/main/wt-worker) — このツールを使う Coding Agent runner

## ライセンス

MIT
