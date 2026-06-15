# Contributing to linkserver233

Thanks for your interest in improving linkserver233.

## Prerequisites

- Go 1.26+

## Development

```bash
# Run the server locally
go run ./cmd/linkserver233 --addr :8080 --base-url http://127.0.0.1:8080

# Format, vet, and test
gofmt -w cmd internal
go vet ./...
go test ./...

# Race + coverage (matches CI; needs a C toolchain)
CGO_ENABLED=1 go test -race -cover ./...
```

CI runs gofmt, `go vet`, `go build`, and `go test -race` on every push and pull
request. Please make sure these pass before opening a PR.

## Project structure

```
linkserver233/
├── cmd/linkserver233/      # CLI entry point (serve, agent, version)
├── internal/
│   ├── agentdocs/          # embedded agent guide and llms.txt
│   ├── buildinfo/          # version/commit/build metadata
│   ├── config/             # flags + environment parsing
│   ├── link/               # link model, validation, duration parsing
│   ├── ratelimit/          # token-bucket limiter
│   ├── security/           # PBKDF2 passwords + SSRF target checks
│   ├── server/             # HTTP handlers and middleware
│   └── store/              # file-backed persistence
├── scripts/                # install.sh, install.ps1, release.sh, release.ps1
├── docs/                   # GitHub Pages site
└── .github/workflows/      # ci.yml, release.yml, pages.yml
```

## Releases

Maintainers cut a release with `scripts/release.sh vX.Y.Z` (or
`scripts\release.ps1`). Pushing a `v*` tag triggers the release workflow, which
builds cross-platform binaries and publishes them with `gh release create`.

## License

By contributing you agree that your contributions are licensed under the MIT License.
