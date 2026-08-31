<div align="center">

<img src="internal/api/webassets/icon.svg" width="84" alt="nudge 商標" />

# nudge

### 🔔 一個極小、可自行架設的開發者通知收件匣

**當你的應用程式發生了什麼事，nudge 會把訊息送到你手上的每一塊螢幕——沒有雲端中繼、不用原生 App、也沒有帳號系統。**

🌐 **語言切換：** [English](README.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md)

[![Release](https://img.shields.io/github/v/release/gitstq/nudge?style=flat-square)](https://github.com/gitstq/nudge/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Platforms](https://img.shields.io/badge/%E5%B9%B3%E5%8F%B0-linux%20%7C%20macos%20%7C%20windows-54968b?style=flat-square)](#-打包與部署)
[![Zero deps](https://img.shields.io/badge/%E9%9B%B6%E4%BE%9D%E8%B3%9D-zero-success?style=flat-square)](go.mod)

### ⬇️ 下載：**[最新 Release（v1.0.0）](https://github.com/gitstq/nudge/releases/latest)** —— Linux · macOS · Windows，免執行階段

</div>

---

## 🎉 專案介紹

**nudge** 是一套自行架設的服務：只要一行 HTTP 呼叫，就能把通知同時送到手機、筆電與團隊頻道。排程工作、CI 管線、備份腳本、長時間執行的訓練任務——任何會在你轉身之後跑完（或失敗）的事情，都能用一條 `curl` 輕輕「戳」你一下。

它的出發點，來自本週竄紅的「開發者自架通知」類專案所反映的真實痛點：現有方案不是把你綁在單一廠商的推播網路上（還得自己編譯、簽署原生 App），就是得依賴資料庫堆疊與代管中繼。**nudge 走了一條不一樣的路：**

- 🌍 **開放標準，所有裝置通用。** 派送採用 W3C **Web Push** 協定（VAPID，RFC 8030/8291/8188）加上可安裝的 **PWA**。Android、桌面版 Chrome/Edge/Firefox、iOS（加入主畫面後）全都能用——**不必編譯原生 App，也不必設定 APNs 的 `.p8` 憑證**。
- 🪶 **單一執行檔，零第三方相依。** 純 Go 標準函式庫實作，就連 Web Push 的 ECDH/HKDF/aes128gcm 加密與 ES256 的 VAPID JWT 都是專案內自研；沒有 cgo、沒有 SQLite、沒有 npm 建置步驟；儲存層只不過是一個資料夾裡的追加式日誌，用 `cp` 就能備份。
- 📡 **多頻道廣發。** 同一則事件同時送往瀏覽器推播、SSE 即時網頁收件匣，並外發到 **Discord / Slack / Telegram / ntfy / 通用 JSON Webhook**。
- 🔌 **相容 ntfy.sh 釋出協定。** 原本寫給 ntfy 的腳本無須改動：`curl -d "done" http://nudge/topic`。
- 🛡️ **隱私優先。** 依主題簽發獨立發布金鑰（磁碟上只存 SHA-256 雜湊）、依 IP 限流、請求體大小上限、零遙測、零對外連線。

> 💡 靈感來源：nudge 只參考近期熱門自架通知器所要解決的**產品問題**，再以開放 Web 標準重新設計；加密、儲存、伺服器與 PWA 的每一行程式皆為原創。

## ✨ 核心特性

- 📥 **統一收件匣**：以主題 / 等級 / 標籤分類，具備未讀狀態、篩選、一鍵全部已讀與批次清除。
- 🔔 **正規 Web Push**：符合標準的 `aes128gcm` 承載加密，CI 中以**獨立的 Python 實作進行交叉解密驗證**。
- 🖥️ **即時網頁收件匣**：採用 SSE（Server-Sent Events），不需要 WebSocket，一般反向代理即可承載。
- 🔑 **具範圍的發布金鑰**：可將金鑰綁定單一主題（每個工作一把部署金鑰），也能簽發萬用金鑰；明文權杖**只顯示一次**。
- 🚦 **四種嚴重度**：`info / success / warning / error`，卡片配色與推播圖示互相對應。
- 🌊 **外發頻道**：Discord Embed、Slack 訊息、Telegram、ntfy 中繼與任意 Webhook，皆可依主題篩選並一鍵測試。
- 🧩 **兩種發布方式**：標準 JSON API，以及 ntfy 風格的標頭 / 查詢參數 / 原始請求體 API。
- 💻 **內建 CLI**：同一支執行檔也是傳送端：`nudge send --title …`。
- 🔁 **崩潰安全的持久化**：預寫式日誌（WAL）加上快照壓縮，重啟後收件匣完好如初。
- 🐳 **極小部署**：約 9MB 靜態執行檔，或 distroless 容器映像。
- 🧪 **完整測試**：競態偵測單元測試、HTTP 整合測試、真實執行檔端對端冒煙測試，以及跨語言加密已知答案驗證。

## 🚀 快速開始

### 環境需求

| 項目 | 版本 / 備註 |
| --- | --- |
| 原始碼建置 | Go 1.22+ |
| 執行擋執行 | **不需要任何執行階段**，靜態編譯 |
| 作業系統 / 架構 | Linux、macOS、Windows · amd64 / arm64 |
| 瀏覽器推播 | Chromium / Firefox / Edge；iOS 需「加入主畫面」 |

### 方式 A：下載執行檔

```bash
# Linux / macOS 自動判斷系統與架構
curl -fsSL https://raw.githubusercontent.com/gitstq/nudge/main/scripts/install.sh | sh

# 或從 Releases 下載壓縮檔後：
tar xzf nudge_v1.0.0_linux_amd64.tar.gz
NUDGE_DATA_DIR=./data ./nudge
```

### 方式 B：Docker

```bash
docker compose up -d
# 開啟 http://localhost:8080
```

### 方式 C：從原始碼執行

```bash
git clone https://github.com/gitstq/nudge.git
cd nudge
go run . serve --addr :8080 --data ./data
```

首次啟動會印出管理員權杖（同時存於 `data/admin.token`），並自動產生 VAPID 金鑰對（`data/vapid.json`）。開啟顯示的網址，以管理員權杖登入即可。

### 30 秒發出第一則通知

```bash
# 1. 在介面「發布金鑰」頁建立一把金鑰，接著：
curl -X POST http://localhost:8080/api/v1/notify \
  -H "Authorization: Bearer nudg_k_xxxx" \
  -H "Content-Type: application/json" \
  -d '{"topic":"backups","title":"備份完成 ✅","body":"夜間備份耗時 42 秒","level":"success"}'
```

ntfy 相容簡寫：

```bash
curl -X POST -H "Authorization: Bearer nudg_k_xxxx" \
  -H "X-Title: 備份完成" --data "夜間備份耗時 42 秒" \
  http://localhost:8080/backups
```

內建命令列：

```bash
nudge send --server http://localhost:8080 --token nudg_k_xxxx \
  --topic backups --title "備份完成 ✅" --body "耗時 42 秒" --level success
```

## 📖 詳細使用指南

### 標準發布 API

`POST /api/v1/notify`，標頭 `Authorization: Bearer <發布金鑰或管理員權杖>`

| 欄位 | 型別 | 必填 | 說明 |
| --- | --- | --- | --- |
| `topic` | string | 否 | 頻道名稱，預設 `default` |
| `title` | string | 與 body 至少一個 | 簡短標題 |
| `body` | string | 與 title 至少一個 | 內文 |
| `level` | string | 否 | `info`（預設）· `success` · `warning` · `error` |
| `tags` | string[] | 否 | 自由標籤 |
| `url` | string | 否 | 點擊通知時開啟的動作連結 |

### ntfy 相容 API

`POST|PUT /<topic>`：請求體原文即訊息內容，並支援以下提示欄位：

| 來源 | 欄位 |
| --- | --- |
| 要求標頭 | `X-Title`、`X-Priority`（1–5）、`X-Tags`、`X-Click` |
| 查詢參數 | `?title=&priority=&tags=&click=&message=` |

### 管理 API 總覽

| 方法與路徑 | 用途 |
| --- | --- |
| `GET /api/v1/events?topic=&unread=1&limit=` | 查詢事件（最新在前） |
| `POST /api/v1/events/read` | `{"ids":[…]}` 或 `{"all":true}` |
| `DELETE /api/v1/events/{id}` | 刪除單一則 |
| `POST /api/v1/events/clear` | 清空全部 |
| `GET /api/v1/stream` | SSE 即時串流（可選 `?topic=`） |
| `GET/POST/DELETE /api/v1/devices…` | 瀏覽器訂閱管理 |
| `GET/POST/DELETE /api/v1/keys…` | 發布金鑰管理 |
| `GET/POST/DELETE /api/v1/channels…` | 外發頻道管理 |
| `POST /api/v1/channels/{id}/test` | 對頻道發送測試訊息 |
| `GET /api/v1/stats` · `GET /healthz` · `GET /api/v1/vapid-public` | 維運介面 |

### 在裝置上開啟推播

1. 用瀏覽器開啟 nudge 頁面並**安裝成應用程式**（Chromium 點安裝圖示；iOS 在 Safari 用分享 → 加入主畫面）。
2. 登入後進入「裝置」頁 → **開啟瀏覽器推播** → 允許通知權限。
3. 之後即使分頁關閉，事件仍會以系統通知彈出。（展示動圖預留位置：`docs/demo-push.gif`）

### 常見使用情境

- **排程 / 備份**：在腳本結尾加上 `curl`，失敗送 `error`、成功送 `success`。
- **CI/CD 管線**：部署完成或測試失敗即通知，不必安裝廠商 CLI。
- **長時間訓練**：`python train.py && nudge send --title 訓練完成 || nudge send --level error …`。
- **家庭實驗室**：與 Uptime Kuma 類 Webhook 聯動，一則事件同時進 Discord 與手機。

### 設定項說明

| 環境變數 | 預設值 | 意義 |
| --- | --- | --- |
| `NUDGE_ADDR` | `:8080` | 監聽位址 |
| `NUDGE_DATA_DIR` | `./data` | 日誌 / 快照 / 狀態目錄 |
| `NUDGE_BASE_URL` | _（空）_ | 對外網址，用於組連結 |
| `NUDGE_ADMIN_TOKEN` | _自動產生_ | 固定管理員權杖，未設定則首次啟動產生 |
| `NUDGE_VAPID_SUBJECT` | `mailto:admin@nudge.local` | VAPID 聯絡聲明 |
| `NUDGE_MAX_EVENTS` | `5000` | 事件保留上限，超出淘汰最舊者 |
| `NUDGE_MAX_AGE` | `0`（關閉） | 事件存活時間，例如 `720h` |
| `NUDGE_RATE_PER_MIN` | `120` | 單 IP 每分鐘請求上限，`0` 關閉 |
| `NUDGE_MAX_BODY_BYTES` | `65536` | 請求體大小上限 |

### 反向代理提醒

SSE 必須關閉緩衝：nginx 請加上 `proxy_buffering off;`、`proxy_read_timeout 1h;` 並使用 HTTP/1.1。Web Push 需要 HTTPS，請將 nudge 放在 TLS 之後（Caddy 可自動簽證）。

## 💡 設計思路與迭代規劃

### 為什麼這樣設計

- **開放標準勝過廠商綁定。** 以 VAPID 為基礎的 Web Push 在所有主流瀏覽器平台通用；推播服務（FCM / Mozilla / Apple Web Push）只轉送**唯有你的瀏覽器才能解密**的密文承載。
- **用資料夾取代資料庫。** 通知是「追加多、體積小」的資料，WAL 加上記憶體索引既持久、又零維運、好備份，快照機制讓重啟回放始終很短。
- **只用標準函式庫。** 通知器應該容易審計：沒有供應鏈依賴、可離線重複建置、執行檔約 9MB。
- **用戶端與伺服器一體。** 主機上的腳本不必再安裝第二套工具。

### 迭代路線

- [ ] v1.1：排程靜音 / 勿擾時段、事件搜尋、JSONL 匯出
- [ ] v1.2：OpenMetrics `/metrics` 指標、Prometheus Alertmanager Webhook 接收
- [ ] v1.3：Web Push 回執與各裝置投遞紀錄介面
- [ ] 歡迎共創：Mattermost / Matrix / Bark 頻道、OIDC 登入、多使用者

## 📦 打包與部署指南

### 跨平台建置

```bash
# 建置 linux/darwin/windows × amd64/arm64，並產生 build/checksums.txt
VERSION=v1.0.0 bash scripts/build.sh
```

發布產物皆為靜態編譯（`CGO_ENABLED=0`）。**測試矩陣：** linux/amd64 於 CI 完整執行執行期測試（單元 + 端對端冒煙）；linux/arm64、darwin（amd64/arm64）、windows（amd64/arm64）每次發布皆交叉編譯並完成架構校驗。

### 驗證下載完整性

```bash
sha256sum -c checksums.txt --ignore-missing
```

### systemd 服務範例

```ini
[Unit]
Description=nudge notification inbox
After=network.target

[Service]
WorkingDirectory=/opt/nudge
ExecStart=/opt/nudge/nudge serve
Environment=NUDGE_ADDR=:8080 NUDGE_DATA_DIR=/opt/nudge/data
Restart=always
User=nobody

[Install]
WantedBy=multi-user.target
```

## 🤝 貢獻指南

歡迎 Issue 與 PR！提交前請閱讀 [CONTRIBUTING.md](CONTRIBUTING.md)：遵循 Angular 提交規範、保持 `gofmt`、堅持標準函式庫，並通過以下檢查：

```bash
gofmt -w .
go vet ./...
go test -race ./...
bash tests/e2e_smoke.sh
```

## 📄 開源授權

以 [MIT 授權條款](LICENSE) 釋出 © 2026 gitstq。
