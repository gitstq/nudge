# Contributing to nudge

Thanks for taking the time to improve nudge! 🦞

## Ground rules

- **Standard library only.** New third-party Go modules require a strong
  reason and maintainer agreement — zero dependencies is a core feature.
- Keep the single-binary promise: the PWA is embedded with `go:embed`, there
  is no frontend build step and no external CDN.
- Be kind; discuss larger changes in an issue first.

## Development setup

```bash
git clone https://github.com/gitstq/nudge.git
cd nudge
go run . serve --data ./tmp-data      # run locally
```

Required checks before opening a PR:

```bash
gofmt -w .
go vet ./...
go test -race ./...
bash tests/e2e_smoke.sh
```

If you touch the Web Push crypto (`internal/push`), also refresh and run the
cross-language known-answer test:

```bash
UPDATE_VECTOR=1 go test ./internal/push/ -run TestDeterministicVector
python3 tests/kat_webpush.py   # needs: pip install cryptography
```

## Commit convention

We follow the [Angular Convention](https://www.conventionalcommits.org/):

- `feat: add telegram channel fan-out`
- `fix: survive a truncated WAL tail on startup`
- `docs: clarify nginx SSE buffering`
- `refactor: split channel payload builders`
- `test: cover topic-scoped key enforcement`
- `chore: bump ci setup-go action`

## Pull request checklist

- [ ] Code is `gofmt`-clean and `go vet ./...` passes
- [ ] New behavior has tests; `go test -race ./...` is green
- [ ] User-facing changes are reflected in **all three** READMEs
      (`README.md`, `README.zh-CN.md`, `README.zh-TW.md`)
- [ ] No secrets, data files or build artifacts committed
- [ ] Commit messages follow the convention above

## Reporting issues

Please include: nudge version (`nudge version`), OS/arch, reproduction
steps, expected vs actual behavior, and (redacted) logs from the server.

## Security

Do not open a public issue for vulnerabilities — email the maintainer via the
address in `NUDGE_VAPID_SUBJECT` defaults or use GitHub private advisory.
