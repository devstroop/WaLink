# Scope: whatsmeow Submodule Update to Latest + WaLink Debts

> Branch: `chore/scope-whatsmeow-debts` from `develop` `bbaef2b`

## 1. whatsmeow Submodule

**Current:** `fc65416c22c47e15be1daf2e2d62deaba6881591` (fork `devstroop/whatsmeow`, sync with `tulir/whatsmeow` at `fc65416`)
**Latest upstream:** `0fadda796019293764d0c2f6f08cb9453ef3eaaa` (`tulir/main` HEAD, also `devstroop/whatsmeow` HEAD)
**Delta:** 30 commits (`git -C whatsmeow log --oneline fc65416..0fadda7`)

### Key upstream changes

| Commit | Area | Impact |
|--------|------|--------|
| `4650ea9` | deps: bump Go to 1.26 | WaLink `go.mod:3` `go 1.25` must bump to `1.26`, `toolchain go1.26` |
| `33cfac5` `d1cc3c0` | ci: staticcheck, goimports | WaLink CI already hardened but should align |
| `72f22e6` `6eefbff` `b06ae6e` `a23afe3` `e277b76` `39b719b` | proto: v104363→v104564 | Regenerate, check `proto/wa*` handling in `internal/service/account.go:1244` `groupInfoToModel`, `message.go:202` |
| `197e617` `8d023aa` | user: UpdateBlocklist LIDs | `internal/handler/whatsapp.go:380` `ListContacts`, `internal/service/account.go:1277` `CheckContacts` may need LID handling |
| `662b012` | user: SetStatusMessage mex query | `internal/service/account.go:1233` `SetStatusMessage` breaking change |
| `e277b76` `39b719b` `8b4a8ba` | group: create/capability | `internal/handler/whatsapp.go:476` `CreateGroup`, `internal/service/account.go:813` |
| `0dcf1f5` | group: remove create_key | Remove `create_key` param if present |
| `1494ba7` | presence: remove from attr | `internal/service/account.go:1110` `SendPresence`, `whatsapp.go:620` |
| `4fa3462` `72f22e6` | client/socket | `internal/service/account.go:118` `Connect`, `prepareClient` |
| `fb386f1` `b06ae6e` | deps: update | `golang.org/x/crypto 0.48→0.50`, `x/net 0.50`, `sys 0.41`, `text 0.34` align with WaLink `go.mod:10` `0.49` |

### Update steps (scope)

1. **Fetch & checkout** (worktree `chore/scope-whatsmeow`):
   ```bash
   git -C whatsmeow fetch origin
   git -C whatsmeow checkout 0fadda7
   git add whatsmeow
   go mod edit -go=1.26
   go get go.mau.fi/whatsmeow@latest  # via replace => ./whatsmeow, but update go.sum
   go mod tidy
   ```
2. **Fix breaking changes**:
   - `group.go:39` create params, `group: remove create_key`
   - `user.go:662` SetStatusMessage new mex
   - `presence.go:1494` from attr removal
   - `client.go:4fa34` handler queue
   - `download.go:72` unencrypted media keys
3. **Test**: `WALINK_TEST_DSN=... go test -p 1 ./... -count=1` (85% database, 80% middleware, 100% config already), plus integration `tests` with `-coverpkg`
4. **Docker**: `docker compose build && docker compose up -d && curl /api/health`
5. **Push**: `git push origin chore/scope-whatsmeow-debts` + PR to `develop`

### Risk

- **HIGH**: Go 1.26 bump requires CI `actions/setup-go@v6` with `go-version: "1.26"` (`ci.yml:28`, `release.yml:15`) — currently 1.25
- **MED**: Proto bumps may change `internal/service/account.go:1969` `classifyMessage`, `storeMessage`
- **LOW**: Dep updates are minor

## 2. WaLink Debts

### Docs debt

- `ARCHITECTURE.md:27` says `SQLite` but `internal/database/database.go:107` uses `postgres` (`lib/pq`), `docker-compose.yml:3` `postgres:16-alpine`. Fix docs to `PostgreSQL`.
- `ARCHITECTURE.md:42` `database.go` comment `SQLite` outdated.
- `CONFIGURATION.md:109` says `modernc.org/sqlite` — remove.
- `ISSUES.md:1` dated March 18, 2026 — update after coverage work (`feat/coverage-improvements` `e1d2a09`).

### Code debt

- `go.mod:3` `go 1.25` behind whatsmeow `1.26` (toolchain), `golang.org/x/*` behind (crypto 0.49 vs 0.50, net 0.51 vs 0.50, sys 0.42 vs 0.41)
- `internal/service/account.go:30` `go.mau.fi/whatsmeow` replace via `go.mod:42` — keep but ensure `whatsmeow/go.mod` toolchain matches
- `internal/web/*:0%` coverage (5674 LOC), `internal/agent:0%` (1022 LOC), `internal/mcpserver:0%` (1004 LOC) — from audit `internal/database 17%→85%` done, remaining 0% packages need follow-up `feat/web-tests`
- `internal/handler/whatsapp.go:620` `SendPresence` 10% coverage — add tests
- `TODO` in `whatsmeow/*` (upstream) — not WaLink debt, but 30+ TODOs in `whatsmeow/appstate.go:TODO`, `whatsmeow/message.go:TODO` — track upstream

### Workflow debt (done in `fix/workflow-hardening` `63d9095`)

- `ci.yml:5` `branches: [master]` fixed → `main,develop` + `workflow_dispatch`
- Pin SHAs `checkout@93cb6ef`, `setup-go@924ae3`, `upload-artifact@b7c566`, `golangci@1481404`, `gh-release@c95fe14`
- `permissions: contents: read` per-job, `concurrency`, `timeout-minutes`

### Infra debt

- `docker-compose.yml:3` `postgres:16-alpine` vs `internal/database` expects `postgres:18` (we use 18.6 locally at `/tmp/pg-root`) — align tag
- No `dependabot.yml` for automated updates — add `.github/dependabot.yml`
- No `whatsmeow` update automation — add weekly `git submodule update --remote`

## 3. Estimate

- **whatsmeow update**: 1–2 days (Go bump + proto + 5 breaking handlers + test)
- **docs fix**: 0.5 day
- **web/agent/mcp coverage**: 3–5 days (follow-up)
- **total**: 5–7 days

## 4. Acceptance

- `go vet ./...` PASS, `go test -p 1 ./internal/... ./tests` PASS (85%+), `docker compose up -d && curl /api/health` → `{"status":"ok"}`
- `git submodule status` shows `0fadda7`
- `go.mod` `go 1.26`, `ARCHITECTURE.md` says PostgreSQL
