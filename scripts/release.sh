#!/usr/bin/env bash
set -euo pipefail

# Cut a release: validate, tag, and push. The `release` GitHub Actions workflow
# then builds cross-platform binaries and publishes them with `gh release create`.
#
# Usage: scripts/release.sh v1.0.0

VERSION="${1:-}"
REMOTE="${2:-origin}"

if [ -z "$VERSION" ]; then
  echo "usage: scripts/release.sh <version> [remote]" >&2
  echo "example: scripts/release.sh v1.0.0" >&2
  exit 1
fi

case "$VERSION" in
  v*) ;;
  *) VERSION="v$VERSION" ;;
esac

if ! printf '%s' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
  echo "error: version must look like v1.2.3" >&2
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "error: working tree is not clean; commit or stash first" >&2
  exit 1
fi

echo "Running tests..."
go test ./...

if git rev-parse "$VERSION" >/dev/null 2>&1; then
  echo "error: tag $VERSION already exists" >&2
  exit 1
fi

echo "Tagging $VERSION..."
git tag -a "$VERSION" -m "linkserver233 $VERSION"
git push "$REMOTE" "$VERSION"

echo "Pushed $VERSION. The release workflow will build and publish binaries."
if command -v gh >/dev/null 2>&1; then
  echo "Watching release workflow (Ctrl-C to stop)..."
  gh run watch --exit-status || true
fi
