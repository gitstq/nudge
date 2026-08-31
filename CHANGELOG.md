# Changelog

All notable changes to nudge are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/) and this project uses
[Semantic Versioning](https://semver.org/).

## [v1.0.0] - 2026-08-31

### Added
- Self-hosted notification inbox shipped as a single zero-dependency Go binary
- Canonical JSON publish API (`POST /api/v1/notify`) with topic/level/tags/url
- ntfy.sh-compatible publish endpoint (`POST /<topic>`, X-* headers & query)
- W3C Web Push delivery: in-tree VAPID (ES256) and RFC 8291/8188 aes128gcm
  payload encryption, pure standard library
- Installable PWA inbox: live SSE stream, filters, read state, device enroll
- Per-topic publish keys (SHA-256 hashed at rest, raw token shown once)
- Outbound fan-out channels: Discord, Slack, Telegram, ntfy, generic webhook
- Crash-safe append-only WAL storage with snapshot compaction and retention
- Built-in `nudge send` CLI and `nudge keys` VAPID generator
- Dockerfile (distroless, non-root) and docker-compose.yml
- Cross-platform builds for linux/darwin/windows × amd64/arm64 with checksums
- Unit + race tests, HTTP integration tests, binary e2e smoke and an
  independent Python cross-language Web Push known-answer test
- Trilingual documentation: English, 简体中文, 繁體中文
