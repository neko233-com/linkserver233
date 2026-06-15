# linkserver233

> **Self-hosted, agent-first link server.** Long links, short links, custom path
> mapping, expiration, one-time links, password protection, and a clean JSON
> API — in a single Go 1.26 binary with zero runtime dependencies.

[![ci](https://github.com/neko233-com/linkserver233/actions/workflows/ci.yml/badge.svg)](https://github.com/neko233-com/linkserver233/actions/workflows/ci.yml)
[![release](https://github.com/neko233-com/linkserver233/actions/workflows/release.yml/badge.svg)](https://github.com/neko233-com/linkserver233/actions/workflows/release.yml)

- 📚 Docs site: https://neko233-com.github.io/linkserver233/
- 🤖 Agent guide: [AGENTS.md](AGENTS.md) · machine index: [llms.txt](llms.txt)

## Features

- **Short links** — auto-generated, collision-resistant short codes.
- **Custom paths** — multi-segment vanity URLs like `docs/latest`.
- **Long-link storage** — store and resolve arbitrarily long targets.
- **Expiration** — relative (`30d`, `24h`, `1d12h`) or absolute RFC3339 TTLs, on by default.
- **One-time / click limits** — atomic `max_clicks` budgets (burn-after-reading).
- **Password protection** — PBKDF2-hashed secrets with an agent-friendly bypass.
- **Security** — SSRF/open-redirect protection, per-client rate limiting, constant-time token auth.
- **Analytics** — click counts, last-visited timestamps, and an aggregate stats endpoint.
- **Tags, search & pagination** — manage large catalogs.
- **Bulk import/export** — move catalogs between instances.
- **Auto-purge** — background janitor removes expired links.
- **Agent-first** — `linkserver233 agent`, `GET /agent`, `GET /llms.txt`, stable JSON.
- **One-click install** — `curl | bash` / `irm | iex`, modeled on `neko233-com/unicli`.

## Installation

### One-Click Install (Recommended)

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/neko233-com/linkserver233/main/scripts/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/neko233-com/linkserver233/main/scripts/install.ps1 | iex
```

**Windows (CMD)**

```cmd
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/neko233-com/linkserver233/main/scripts/install.ps1 | iex"
```

**With a specific version**

```bash
curl -fsSL https://raw.githubusercontent.com/neko233-com/linkserver233/main/scripts/install.sh | bash -s -- v1.0.0
```

### From source

```bash
go install github.com/neko233-com/linkserver233/cmd/linkserver233@latest
```

### Pre-built binaries

Download from [GitHub Releases](https://github.com/neko233-com/linkserver233/releases).

## Quick start

```bash
# Run (every link expires in 30 days by default)
linkserver233 --addr :8080 --base-url https://go.example.com

# Shorten a long URL
curl -X POST http://127.0.0.1:8080/api/v1/links \
  -H 'Content-Type: application/json' \
  -d '{"target_url":"https://example.com/some/really/long/url"}'

# Custom multi-segment path
curl -X POST http://127.0.0.1:8080/api/v1/links \
  -H 'Content-Type: application/json' \
  -d '{"path":"docs/latest","target_url":"https://example.com/products/docs"}'

# One-time, expiring, password-protected vanity link
curl -X POST http://127.0.0.1:8080/api/v1/links \
  -H 'Content-Type: application/json' \
  -d '{"path":"launch","target_url":"https://example.com/launch","expires_in":"24h","max_clicks":1,"password":"s3cret"}'

# Visit a protected link (agent style)
curl -i 'http://127.0.0.1:8080/launch?pw=s3cret'
```

## Configuration

All options are available as flags or environment variables.

| Flag | Env | Default | Description |
| --- | --- | --- | --- |
| `--addr` | `LINKSERVER_ADDR` | `:8080` | Listen address. |
| `--data` | `LINKSERVER_DATA` | `data/links.json` | Persistence file. |
| `--base-url` | `LINKSERVER_BASE_URL` | _(empty)_ | Public base URL in API responses. |
| `--admin-token` | `LINKSERVER_ADMIN_TOKEN` | _(empty)_ | Bearer token required for `/api/v1/*`. |
| `--code-length` | `LINKSERVER_CODE_LENGTH` | `7` | Generated short-code length. |
| `--default-ttl` | `LINKSERVER_DEFAULT_TTL` | `30d` | Default link lifetime (`0` to disable). |
| `--max-ttl` | `LINKSERVER_MAX_TTL` | `0` | Maximum link lifetime (`0` = unlimited). |
| `--require-expiry` | `LINKSERVER_REQUIRE_EXPIRY` | `false` | Reject links that would never expire. |
| `--allow-private-targets` | `LINKSERVER_ALLOW_PRIVATE_TARGETS` | `false` | Allow private/internal redirect targets. |
| `--rate-limit` | `LINKSERVER_RATE_LIMIT_PER_MIN` | `120` | Per-client requests/min (`0` to disable). |
| `--rate-limit-burst` | `LINKSERVER_RATE_LIMIT_BURST` | `60` | Per-client burst allowance. |
| `--janitor-interval` | `LINKSERVER_JANITOR_INTERVAL` | `5m` | Expired-link purge interval (`0` to disable). |

For production, set `LINKSERVER_ADMIN_TOKEN` and `LINKSERVER_BASE_URL`.

## Security & time limits

- **Time limits are on by default.** Links expire after `--default-ttl` (30 days)
  unless you set an explicit `expires_in`/`expires_at`, or disable the default
  with `--default-ttl 0`. Use `--max-ttl` to cap lifetimes and `--require-expiry`
  to forbid permanent links entirely.
- **SSRF / open-redirect protection.** Redirect targets pointing at loopback,
  private, link-local, CGNAT, or cloud-metadata addresses (e.g.
  `169.254.169.254`) are rejected. Override with `--allow-private-targets` for
  trusted internal deployments.
- **Password protection.** Per-link passwords are stored as salted
  PBKDF2-HMAC-SHA256. Visitors supply the password via `?pw=` or the
  `X-Link-Password` header; it is stripped before merging query params into the
  target.
- **Rate limiting.** A per-client token-bucket limiter guards every endpoint and
  returns `429` with `Retry-After` when exceeded.
- **Token auth.** When `--admin-token` is set, all `/api/v1/*` calls require
  `Authorization: Bearer <token>` (compared in constant time). Public redirects
  remain open.

## API reference

Authentication (when a token is configured): `Authorization: Bearer <token>`.

### `POST /api/v1/links`

```json
{
  "path": "optional/custom/path",
  "target_url": "https://example.com/long/url",
  "description": "optional",
  "redirect_status": 302,
  "tags": ["docs"],
  "password": "optional",
  "max_clicks": 0,
  "expires_in": "24h",
  "expires_at": "2026-01-01T00:00:00Z",
  "enabled": true
}
```

- Omit `path` to auto-generate a short code.
- `redirect_status` supports `301`, `302` (default), `307`, `308`.
- Provide either `expires_in` (relative) or `expires_at` (absolute); omit both to
  use the server default TTL.
- The visit's query string is appended to the target URL.

### `GET /api/v1/links`

Filter and paginate: `?status=active&tag=docs&q=guide&limit=50&offset=0`. Returns
`{ "items": [...], "total": n, "limit": l, "offset": o }`.

### `GET /api/v1/links/{path}` · `PUT /api/v1/links/{path}` · `DELETE /api/v1/links/{path}`

Show, update, or delete a single link. `PUT` accepts the same fields as create
plus `clear_expiry`; a `null` `password` keeps it, `""` removes it.

### `GET /api/v1/stats`

Aggregate counts (active/expired/disabled/exhausted), total clicks, and top links.

### `POST /api/v1/import`

```json
{ "replace": false, "links": [ { "path": "a", "target_url": "https://..." } ] }
```

`replace: true` swaps the whole catalog; otherwise existing paths are skipped.

### Redirect & meta

- `GET /{path}` — public redirect (`410` expired/exhausted, `404` unknown/disabled, `401` if a password is required).
- `GET /healthz` — health check.
- `GET /agent` — agent usage guide · `GET /llms.txt` — machine-readable index.

## Agent-first

linkserver233 ships a stable, agent-friendly contract:

- `linkserver233 agent` prints the guide; `GET /agent` serves it over HTTP.
- `GET /llms.txt` and [AGENTS.md](AGENTS.md) describe the decision tree and fields.
- Every link object always includes `short_url`, `status`, `expires_at`, and
  `remaining_clicks`, so agents never have to scrape HTML.

## CI/CD & releases

- **CI** ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs gofmt,
  `go vet`, `go build`, and `go test -race -cover` on every push and PR.
- **Release** ([`.github/workflows/release.yml`](.github/workflows/release.yml))
  triggers on `v*` tags, cross-compiles binaries, and publishes them with
  `gh release create`. Cut one with:

  ```bash
  scripts/release.sh v1.0.0      # or: scripts\release.ps1 v1.0.0
  ```

- **Docs** ([`.github/workflows/pages.yml`](.github/workflows/pages.yml)) deploys
  `docs/` to GitHub Pages. Enable Pages → "GitHub Actions" in repository settings.

Release artifacts:

- `linkserver233-linux-amd64`, `linkserver233-linux-arm64`
- `linkserver233-darwin-amd64`, `linkserver233-darwin-arm64`
- `linkserver233-windows-amd64.exe`, `linkserver233-windows-arm64.exe`
- `SHA256SUMS`

## Development

```bash
go run ./cmd/linkserver233 --addr :8080
gofmt -w cmd internal && go vet ./... && go test ./...
```

## License

[MIT](LICENSE)
