<div align="center">

<img src="internal/api/webassets/icon.svg" width="84" alt="nudge 图标" />

# nudge

### 🔔 一个极小、可自托管的开发者通知收件箱

**你的应用只要发生了某件事，nudge 就能把它送到你的每一块屏幕——没有云中转、没有原生 App、没有账号体系。**

🌐 **语言切换：** [English](README.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md)

[![Release](https://img.shields.io/github/v/release/gitstq/nudge?style=flat-square)](https://github.com/gitstq/nudge/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Platforms](https://img.shields.io/badge/%E5%B9%B3%E5%8F%B0-linux%20%7C%20macos%20%7C%20windows-54968b?style=flat-square)](#-打包与部署)
[![Zero deps](https://img.shields.io/badge/%E9%9B%B6%E4%BE%9D%E8%B5%96-zero-success?style=flat-square)](go.mod)

</div>

---

## 🎉 项目介绍

**nudge** 是一个自托管服务：一行 HTTP 调用，就能把通知同时送到你的手机、电脑和团队频道。定时任务、CI 流水线、备份脚本、长时间训练任务——任何会在你移开视线后结束（或失败）的东西，都可以用一条 `curl` 轻轻「戳」你一下。

它来自本周热门的「开发者自托管通知」类项目所暴露的真实痛点：现有方案要么把你锁死在某一家厂商的推送网络里（还要自己编译签名原生 App），要么依赖数据库栈和托管中转。**nudge 选择了另一条路：**

- 🌍 **开放标准，全设备通用。** 投递基于 W3C **Web Push** 协议（VAPID，RFC 8030/8291/8188）与可安装的 **PWA**。安卓、桌面 Chrome/Edge/Firefox、iOS（添加到主屏幕后）全部可用——**无需编译原生 App，也无需配置 APNs 的 `.p8` 凭证**。
- 🪶 **单二进制，零三方依赖。** 纯 Go 标准库实现，连 Web Push 的 ECDH/HKDF/aes128gcm 加密与 ES256 的 VAPID JWT 都在仓库内自研；无 cgo、无 SQLite、无 npm 构建步骤；存储只是一个目录里的追加式日志，`cp` 即可备份。
- 📡 **多通道扇出。** 同一条事件同时走浏览器推送、SSE 实时网页收件箱，以及外发到 **Discord / Slack / Telegram / ntfy / 通用 JSON Webhook**。
- 🔌 **兼容 ntfy.sh 发布协议。** 为 ntfy 写的脚本无需改动：`curl -d "done" http://nudge/topic`。
- 🛡️ **隐私优先。** 按主题签发独立发布密钥（落盘只存 SHA-256 哈希）、按 IP 限流、请求体大小上限、零遥测、零外联。

> 💡 灵感来源：nudge 仅参考近期热门自托管通知器所解决的**产品问题**，再用开放 Web 标准重新设计；加密、存储、服务端与 PWA 的每一行代码均为原创自研。

## ✨ 核心特性

- 📥 **统一收件箱**：主题 / 级别 / 标签组织，未读状态、筛选、一键全部已读与批量清空。
- 🔔 **真正的 Web Push**：符合标准的 `aes128gcm` 载荷加密，CI 中用**独立的 Python 实现交叉解密验证**。
- 🖥️ **实时网页收件箱**：基于 SSE（Server-Sent Events），无需 WebSocket，普通反向代理即可承载。
- 🔑 **作用域发布密钥**：可把密钥绑定到单一主题（每个任务一把部署密钥），也可签发通配密钥；明文令牌**只展示一次**。
- 🚦 **四级严重度**：`info / success / warning / error`，卡片颜色与推送图标一一对应。
- 🌊 **外发通道**：Discord Embed、Slack 消息、Telegram、ntfy 中转与任意 Webhook，支持按主题过滤与一键测试。
- 🧩 **两种发布姿势**：规范 JSON 接口 + ntfy 风格的请求头 / 查询参数 / 原始体接口。
- 💻 **内置命令行**：同一个二进制也是发送端：`nudge send --title …`。
- 🔁 **崩溃安全持久化**：预写日志（WAL）+ 快照压缩，重启后收件箱完整无损。
- 🐳 **极小部署**：约 9MB 静态二进制，或 distroless 容器镜像。
- 🧪 **充分自测**：竞态检测单元测试、HTTP 集成测试、真实二进制端到端冒烟、跨语言加密已知答案验证。

## 🚀 快速开始

### 环境要求

| 项 | 版本 / 说明 |
| --- | --- |
| 源码构建 | Go 1.22+ |
| 二进制运行 | **无需任何运行时**，静态编译 |
| 系统 / 架构 | Linux、macOS、Windows · amd64 / arm64 |
| 浏览器推送 | Chromium / Firefox / Edge；iOS 需「添加到主屏幕」 |

### 方式 A：下载二进制

```bash
# Linux / macOS 自动识别系统与架构
curl -fsSL https://raw.githubusercontent.com/gitstq/nudge/main/scripts/install.sh | sh

# 或在 Releases 下载压缩包后：
tar xzf nudge_v1.0.0_linux_amd64.tar.gz
NUDGE_DATA_DIR=./data ./nudge
```

### 方式 B：Docker

```bash
docker compose up -d
# 打开 http://localhost:8080
```

### 方式 C：源码运行

```bash
git clone https://github.com/gitstq/nudge.git
cd nudge
go run . serve --addr :8080 --data ./data
```

首次启动会打印管理员令牌（同时保存在 `data/admin.token`），并自动生成 VAPID 密钥对（`data/vapid.json`）。打开对应网址，用管理员令牌登录即可。

### 30 秒发出第一条通知

```bash
# 1. 在界面「发布密钥」页创建一把密钥，然后：
curl -X POST http://localhost:8080/api/v1/notify \
  -H "Authorization: Bearer nudg_k_xxxx" \
  -H "Content-Type: application/json" \
  -d '{"topic":"backups","title":"备份完成 ✅","body":"夜间备份耗时 42 秒","level":"success"}'
```

ntfy 兼容简写：

```bash
curl -X POST -H "Authorization: Bearer nudg_k_xxxx" \
  -H "X-Title: 备份完成" --data "夜间备份耗时 42 秒" \
  http://localhost:8080/backups
```

内置命令行：

```bash
nudge send --server http://localhost:8080 --token nudg_k_xxxx \
  --topic backups --title "备份完成 ✅" --body "耗时 42 秒" --level success
```

## 📖 详细使用指南

### 规范发布接口

`POST /api/v1/notify`，请求头 `Authorization: Bearer <发布密钥或管理员令牌>`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `topic` | string | 否 | 频道名，默认 `default` |
| `title` | string | 与 body 至少一个 | 短标题 |
| `body` | string | 与 title 至少一个 | 正文 |
| `level` | string | 否 | `info`（默认）· `success` · `warning` · `error` |
| `tags` | string[] | 否 | 自由标签 |
| `url` | string | 否 | 点击通知跳转的动作链接 |

### ntfy 兼容接口

`POST|PUT /<topic>`：请求体原文即消息内容，支持以下提示：

| 来源 | 字段 |
| --- | --- |
| 请求头 | `X-Title`、`X-Priority`（1–5）、`X-Tags`、`X-Click` |
| 查询参数 | `?title=&priority=&tags=&click=&message=` |

### 管理接口一览

| 方法与路径 | 作用 |
| --- | --- |
| `GET /api/v1/events?topic=&unread=1&limit=` | 查询事件（最新在前） |
| `POST /api/v1/events/read` | `{"ids":[…]}` 或 `{"all":true}` |
| `DELETE /api/v1/events/{id}` | 删除单条 |
| `POST /api/v1/events/clear` | 清空全部 |
| `GET /api/v1/stream` | SSE 实时流（可选 `?topic=`） |
| `GET/POST/DELETE /api/v1/devices…` | 浏览器订阅管理 |
| `GET/POST/DELETE /api/v1/keys…` | 发布密钥管理 |
| `GET/POST/DELETE /api/v1/channels…` | 外发通道管理 |
| `POST /api/v1/channels/{id}/test` | 向通道发送测试消息 |
| `GET /api/v1/stats` · `GET /healthz` · `GET /api/v1/vapid-public` | 运维接口 |

### 在设备上开启推送

1. 用浏览器打开 nudge 页面并**安装为应用**（Chromium 点安装图标；iOS 在 Safari 分享 → 添加到主屏幕）。
2. 登录后进入「设备」页 → **开启浏览器推送** → 允许通知权限。
3. 之后即使标签页关闭，事件也会以系统通知形式弹出。（演示动图占位：`docs/demo-push.gif`）

### 典型使用场景

- **定时任务 / 备份**：脚本结尾追加 `curl`，失败发 `error`、成功发 `success`。
- **CI/CD 流水线**：部署完成或用例失败即通知，无需安装厂商 CLI。
- **长耗时训练**：`python train.py && nudge send --title 训练完成 || nudge send --level error …`。
- **家庭实验室**：与 Uptime Kuma 类 Webhook 联动，一条事件同时进 Discord 和手机。

### 配置项说明

| 环境变量 | 默认值 | 含义 |
| --- | --- | --- |
| `NUDGE_ADDR` | `:8080` | 监听地址 |
| `NUDGE_DATA_DIR` | `./data` | 日志 / 快照 / 状态目录 |
| `NUDGE_BASE_URL` | _（空）_ | 对外访问地址，用于拼接链接 |
| `NUDGE_ADMIN_TOKEN` | _自动生成_ | 固定管理员令牌，不设则首次启动生成 |
| `NUDGE_VAPID_SUBJECT` | `mailto:admin@nudge.local` | VAPID 联系声明 |
| `NUDGE_MAX_EVENTS` | `5000` | 事件保留上限，超出淘汰最旧 |
| `NUDGE_MAX_AGE` | `0`（关闭） | 事件存活时长，如 `720h` |
| `NUDGE_RATE_PER_MIN` | `120` | 单 IP 每分钟请求上限，`0` 关闭 |
| `NUDGE_MAX_BODY_BYTES` | `65536` | 请求体大小上限 |

### 反向代理提示

SSE 需要关闭缓冲：nginx 请加 `proxy_buffering off;`、`proxy_read_timeout 1h;` 并使用 HTTP/1.1。Web Push 要求 HTTPS，请把 nudge 放在 TLS 之后（Caddy 可自动签证书）。

## 💡 设计思路与迭代规划

### 为什么这样设计

- **开放标准优于厂商锁定。** 基于 VAPID 的 Web Push 在所有主流浏览器平台通用；推送服务（FCM / Mozilla / Apple Web Push）只转发**只有你的浏览器能解密**的密文载荷。
- **用目录代替数据库。** 通知是「追加多、体量小」的数据，WAL + 内存索引既保证持久，又零运维、易备份，快照机制让重启重放始终很短。
- **只用标准库。** 通知器应当易于审计：没有依赖供应链、可离线可复现构建、二进制约 9MB。
- **客户端与服务端同体。** 主机上的脚本不需要再装第二个工具。

### 迭代路线

- [ ] v1.1：定时静默 / 免打扰时段、事件搜索、JSONL 导出
- [ ] v1.2：OpenMetrics `/metrics` 指标、Prometheus Alertmanager Webhook 接收
- [ ] v1.3：Web Push 回执与每设备投递历史界面
- [ ] 欢迎共创：Mattermost / Matrix / Bark 通道、OIDC 登录、多用户

## 📦 打包与部署指南

### 跨平台构建

```bash
# 构建 linux/darwin/windows × amd64/arm64，并生成 build/checksums.txt
VERSION=v1.0.0 bash scripts/build.sh
```

发布产物全部静态编译（`CGO_ENABLED=0`）。**测试矩阵说明：** linux/amd64 在 CI 中完成完整运行时测试（单元 + 端到端冒烟）；linux/arm64、darwin（amd64/arm64）、windows（amd64/arm64）每次发布均交叉编译并做架构校验。

### 校验下载完整性

```bash
sha256sum -c checksums.txt --ignore-missing
```

### systemd 服务示例

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

## 🤝 贡献指南

欢迎 Issue 与 PR！提交前请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)：遵循 Angular 提交规范、保持 `gofmt`、坚持标准库实现，并跑通以下检查：

```bash
gofmt -w .
go vet ./...
go test -race ./...
bash tests/e2e_smoke.sh
```

## 📄 开源协议

基于 [MIT 协议](LICENSE) 开源 © 2026 gitstq。
