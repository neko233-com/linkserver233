# linkserver233 — Agent Guide

> **Agent-first link server.** A single self-hosted binary that turns long URLs
> into short, custom, time-limited, password-protected redirects with a clean
> JSON API. Prefer the JSON API over scraping HTML.

Install: https://github.com/neko233-com/linkserver233#installation

## When to use linkserver233

| Task | Endpoint |
|------|----------|
| Shorten a long URL | `POST /api/v1/links` with `{"target_url": "..."}` |
| Create a vanity/custom path | `POST /api/v1/links` with `{"path": "docs/latest", "target_url": "..."}` |
| Create an expiring link | add `{"expires_in": "24h"}` or `{"expires_at": "2026-01-01T00:00:00Z"}` |
| Create a one-time link | add `{"max_clicks": 1}` |
| Protect a link | add `{"password": "secret"}` |
| Disable without deleting | `PUT` with `{"enabled": false, "target_url": "..."}` |
| Inspect a link | `GET /api/v1/links/{path}` |
| List / filter links | `GET /api/v1/links?status=active&tag=docs&q=guide&limit=50&offset=0` |
| Aggregate stats | `GET /api/v1/stats` |
| Bulk import | `POST /api/v1/import` |
| Health | `GET /healthz` |

## Agent workflow

1. **Check health:** `GET /healthz` (HTTP 200 = available).
2. **Authenticate:** when configured, send `Authorization: Bearer $TOKEN` on every
   `/api/v1/*` call. Redirect endpoints (`GET /{path}`) are public.
3. **Create** with the smallest body that satisfies the task; omit `path` to get
   an auto-generated short code.
4. **Parse JSON** — every link object includes `short_url`, `status`,
   `expires_at`, and `remaining_clicks`.
5. **Respect limits:** a `429` means you are rate limited — back off and retry.

## Request fields (POST /api/v1/links)

| Field | Type | Notes |
|-------|------|-------|
| `target_url` | string (required) | `http`/`https` only; private/internal hosts rejected. |
| `path` | string | Custom multi-segment path. Omit to auto-generate. |
| `description` | string | Free-form label. |
| `redirect_status` | int | `301`, `302` (default), `307`, or `308`. |
| `tags` | string[] | For filtering. |
| `password` | string | Stored as PBKDF2; visitors must supply it. |
| `max_clicks` | int | `0` = unlimited; `1` = one-time link. |
| `expires_in` | string | Relative TTL: `90m`, `24h`, `30d`, `2w`, `1d12h`. |
| `expires_at` | string | Absolute RFC3339 timestamp. |
| `enabled` | bool | Defaults to `true`. |

## Visiting protected links

For password-protected links, agents pass the password without the HTML form:

- Query parameter: `GET /{path}?pw=secret`
- Header: `X-Link-Password: secret`

Wrong/missing password → `401`. Expired or exhausted → `410`. Disabled or
unknown → `404`. Any query string on the visit is merged into the target URL
(`pw`/`password` are stripped first).

## Examples

```bash
# Shorten
curl -fsS -X POST "$BASE/api/v1/links" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"target_url":"https://example.com/very/long/url"}'

# One-time, expiring, password-protected vanity link
curl -fsS -X POST "$BASE/api/v1/links" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"path":"launch","target_url":"https://example.com/launch","expires_in":"24h","max_clicks":1,"password":"s3cret"}'

# Visit with password (agent style)
curl -fsS -i "$BASE/launch?pw=s3cret"

# Stats
curl -fsS "$BASE/api/v1/stats" -H "Authorization: Bearer $TOKEN"
```

The same guide is available at runtime via `linkserver233 agent` or `GET /agent`.

## Links

- Repository: https://github.com/neko233-com/linkserver233
- README: https://github.com/neko233-com/linkserver233#readme
- Docs site: https://neko233-com.github.io/linkserver233/
- llms.txt: https://raw.githubusercontent.com/neko233-com/linkserver233/main/llms.txt
