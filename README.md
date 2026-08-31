<div align="center">

<img src="internal/api/webassets/icon.svg" width="84" alt="nudge logo" />

# nudge

### 🔔 A tiny, self-hosted notification inbox for developers

**Your app says something happened. Nudge makes sure it reaches every screen you own — no cloud relay, no native app, no account system.**

🌐 **Languages:** [English](README.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md)

[![Release](https://img.shields.io/github/v/release/gitstq/nudge?style=flat-square)](https://github.com/gitstq/nudge/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Platforms](https://img.shields.io/badge/platforms-linux%20%7C%20macos%20%7C%20windows-54968b?style=flat-square)](#-package--deploy)
[![Zero deps](https://img.shields.io/badge/dependencies-zero-success?style=flat-square)](go.mod)

### ⬇️ Download: **[Latest Release (v1.0.0)](https://github.com/gitstq/nudge/releases/latest)** — Linux · macOS · Windows, no runtime required

</div>

---

## 🎉 Introduction

**nudge** is a self-hosted service that turns a one-line HTTP call into a
notification on your phone, laptop and team channels. Cron jobs, CI pipelines,
backup scripts, long-running training jobs — anything that finishes (or fails)
while you look away — can poke you with a single `curl`.

It was inspired by the pain point behind this week's trending developer
notification projects: existing self-hosted options either lock you into one
vendor push network (and force you to build/sign a native app), or require a
database stack and a hosted relay. **nudge is different:**

- 🌍 **Open standards, every device.** Delivery uses the W3C **Web Push**
  protocol (VAPID, RFC 8030/8291/8188) plus an installable **PWA**. Android,
  desktop Chrome/Edge/Firefox and iOS (installed to Home Screen) all work —
  there is no native app to compile and no APNs `.p8` to configure.
- 🪶 **One binary, zero dependencies.** Pure Go standard library — the Web
  Push ECDH/HKDF/aes128gcm crypto and ES256 VAPID JWTs are implemented
  in-tree. No cgo, no SQLite, no npm build step; storage is an append-only
  log in one directory you can back up with `cp`.
- 📡 **Multi-channel fan-out.** Browser push, a live SSE web inbox, and
  outbound **Discord / Slack / Telegram / ntfy / generic JSON webhooks** from
  the same event.
- 🔌 **ntfy.sh-compatible publish API.** Scripts written for ntfy work
  unchanged: `curl -d "done" http://nudge/topic`.
- 🛡️ **Private by design.** Per-topic publish keys (only SHA-256 hashes are
  stored), per-IP rate limiting, body-size caps, no telemetry, no phone-home.

> 💡 Inspiration: nudge references the *product problem* of recently trending
> self-hosted notifier projects, then re-imagines it around open web standards.
> Every line — crypto, storage, server, PWA — is original.

## ✨ Features

- 📥 **Unified inbox** — topic/level/tag organization, unread state, filters,
  one-click "mark all read" and bulk clear.
- 🔔 **Real Web Push** — standards-compliant `aes128gcm` payload encryption
  (cross-verified against an independent Python implementation in CI).
- 🖥️ **Live web inbox** — Server-Sent Events stream, zero WebSocket hassle,
  friendly to plain reverse proxies.
- 🔑 **Scoped publish keys** — bind a key to one topic (deploy key per job),
  or issue wildcard keys; raw tokens are shown **once**.
- 🚦 **Four severity levels** — `info / success / warning / error`, with
  color-coded cards and push icons.
- 🌊 **Fan-out channels** — Discord embeds, Slack blocks, Telegram messages,
  ntfy relays and arbitrary webhooks, each with per-topic filters and a test
  button.
- 🧩 **Two ways to publish** — canonical JSON API *and* ntfy-compatible
  headers/query/raw-body API.
- 💻 **Built-in CLI** — the same binary is a sender: `nudge send --title …`.
- 🔁 **Crash-safe persistence** — write-ahead log + snapshotting; restart and
  your inbox is intact.
- 🐳 **Tiny deployment** — ~9 MB static binary or distroless Docker image.
- 🧪 **Tested** — unit tests with the race detector, HTTP integration tests,
  an end-to-end binary smoke test, and a cross-language crypto KAT.

## 🚀 Quick Start

### Requirements

| | Version / note |
| --- | --- |
| Go (build from source) | 1.22+ |
| Run from binary | **none** — static, no runtime needed |
| OS / arch | Linux, macOS, Windows · amd64 / arm64 |
| Browser push | any Chromium / Firefox / Edge; iOS via "Add to Home Screen" |

### Option A — download a binary

```bash
# Linux / macOS (auto-detects OS and arch)
curl -fsSL https://raw.githubusercontent.com/gitstq/nudge/main/scripts/install.sh | sh

# …or grab an archive from Releases and:
tar xzf nudge_v1.0.0_linux_amd64.tar.gz
NUDGE_DATA_DIR=./data ./nudge
```

### Option B — Docker

```bash
docker compose up -d
# open http://localhost:8080
```

### Option C — build from source

```bash
git clone https://github.com/gitstq/nudge.git
cd nudge
go run . serve --addr :8080 --data ./data
```

On first boot nudge prints an admin token (also saved as `data/admin.token`)
and generates a VAPID key pair (`data/vapid.json`). Open the printed URL, log
in with the admin token, and you are in.

### Send your first notification in 30 seconds

```bash
# 1. create a publish key (UI: "Publish keys" tab), then:
curl -X POST http://localhost:8080/api/v1/notify \
  -H "Authorization: Bearer nudg_k_xxxx" \
  -H "Content-Type: application/json" \
  -d '{"topic":"backups","title":"Backup OK ✅","body":"nightly dump finished in 42s","level":"success"}'
```

Or with the ntfy-compatible shorthand:

```bash
curl -X POST -H "Authorization: Bearer nudg_k_xxxx" \
  -H "X-Title: Backup OK" --data "nightly dump finished in 42s" \
  http://localhost:8080/backups
```

Or with the built-in CLI:

```bash
nudge send --server http://localhost:8080 --token nudg_k_xxxx \
  --topic backups --title "Backup OK ✅" --body "finished in 42s" --level success
```

## 📖 Usage Guide

### Canonical publish API

`POST /api/v1/notify` · header `Authorization: Bearer <publish-key or admin>`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `topic` | string | no | Channel name, default `default` |
| `title` | string | one of title/body | Short headline |
| `body` | string | one of title/body | Longer text |
| `level` | string | no | `info` (default) · `success` · `warning` · `error` |
| `tags` | string[] | no | Free-form labels |
| `url` | string | no | Action link opened on notification click |

### ntfy-compatible API

`POST|PUT /<topic>` — raw request body is the message; supported hints:

| Source | Keys |
| --- | --- |
| Headers | `X-Title`, `X-Priority` (1–5), `X-Tags`, `X-Click` |
| Query | `?title=&priority=&tags=&click=&message=` |

### Admin API overview

| Method & path | Purpose |
| --- | --- |
| `GET /api/v1/events?topic=&unread=1&limit=` | List events (newest first) |
| `POST /api/v1/events/read` | `{"ids":[…]}` or `{"all":true}` |
| `DELETE /api/v1/events/{id}` | Delete one |
| `POST /api/v1/events/clear` | Delete all |
| `GET /api/v1/stream` | SSE live feed (`?topic=` optional) |
| `GET/POST/DELETE /api/v1/devices…` | Manage browser subscriptions |
| `GET/POST/DELETE /api/v1/keys…` | Manage publish keys |
| `GET/POST/DELETE /api/v1/channels…` | Manage outbound channels |
| `POST /api/v1/channels/{id}/test` | Send a test message to a channel |
| `GET /api/v1/stats` · `GET /healthz` · `GET /api/v1/vapid-public` | Ops |

### Enabling push on a device

1. Open the nudge page in the browser and **install it as an app**
   (Chromium: install icon; iOS Safari: Share → Add to Home Screen).
2. Log in → **Devices** → **Enable push notifications** → allow the permission.
3. Publish an event; it appears as a system notification even when the tab is
   closed. *(Screenshot/demo GIF placeholder: `docs/demo-push.gif`)*

### Typical scenarios

- **Cron / backups:** append a `curl` at the end of a script; `level=error`
  on failure, `success` otherwise.
- **CI/CD pipelines:** notify on deploy or flaky-test failure without
  installing a vendor CLI.
- **Long ML jobs:** `python train.py && nudge send --title done || nudge send --level error …`.
- **Home lab:** pair with Uptime Kuma-style webhooks; fan out to Discord *and*
  your phone simultaneously.

### Configuration reference

| Env variable | Default | Meaning |
| --- | --- | --- |
| `NUDGE_ADDR` | `:8080` | Listen address |
| `NUDGE_DATA_DIR` | `./data` | WAL/snapshot/state directory |
| `NUDGE_BASE_URL` | _(empty)_ | Public URL for links |
| `NUDGE_ADMIN_TOKEN` | _generated_ | Fixed admin token (otherwise auto-created) |
| `NUDGE_VAPID_SUBJECT` | `mailto:admin@nudge.local` | VAPID contact claim |
| `NUDGE_MAX_EVENTS` | `5000` | Retention cap (oldest evicted) |
| `NUDGE_MAX_AGE` | `0` (off) | Event TTL, e.g. `720h` |
| `NUDGE_RATE_PER_MIN` | `120` | Per-IP request cap, `0` disables |
| `NUDGE_MAX_BODY_BYTES` | `65536` | Inbound body cap |

### Reverse proxy notes

SSE needs buffering disabled: for nginx add `proxy_buffering off;`,
`proxy_read_timeout 1h;` and HTTP/1.1. Web Push requires HTTPS, so put nudge
behind TLS (Caddy does this automatically).

## 💡 Design & Roadmap

### Why this shape

- **Open standards beat vendor lock-in.** Web Push with VAPID works on every
  mainstream browser platform; the push service (FCM/Mozilla/Apple web push)
  carries only encrypted payloads that **only your browser can decrypt**.
- **A directory, not a database.** Notification volume is append-heavy and
  small; a WAL + in-memory index gives durability with zero ops and trivial
  backups, while snapshotting keeps replay tails short.
- **Standard library only.** Auditing a notifier should be easy: no supply
  chain tree, reproducible offline builds, ~9 MB binaries.
- **Same binary, client and server.** Scripts on hosts need no extra tool.

### Roadmap

- [ ] v1.1 — scheduled/quiet hours, event search, JSONL export
- [ ] v1.2 — OpenMetrics `/metrics` endpoint, Prometheus alertmanager webhook receiver
- [ ] v1.3 — Web Push message receipts & per-device delivery history UI
- [ ] Ideas welcome: Mattermost/Matrix/Bark channels, OIDC login, multi-user

## 📦 Package & Deploy

### Cross-platform build

```bash
# builds linux/darwin/windows × amd64/arm64 + build/checksums.txt
VERSION=v1.0.0 bash scripts/build.sh
```

Release artifacts are static (`CGO_ENABLED=0`). **Test matrix:** linux/amd64
is fully runtime-tested in CI (unit + e2e smoke); linux/arm64, darwin
(amd64/arm64) and windows (amd64/arm64) are cross-compiled and
architecture-verified on every release.

### Verify a download

```bash
sha256sum -c checksums.txt --ignore-missing
```

### systemd unit (example)

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

## 🤝 Contributing

Issues and PRs are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for
the commit convention (Angular Commits), code style (`gofmt`, standard
library only) and the test commands expected before opening a PR:

```bash
gofmt -w .
go vet ./...
go test -race ./...
bash tests/e2e_smoke.sh
```

## 📄 License

Released under the [MIT License](LICENSE) © 2026 gitstq.
