#requires -version 5.1
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$Version,

    [Parameter(Position = 1)]
    [string]$Remote = 'origin'
)

$ErrorActionPreference = 'Stop'

# Cut a release: validate, tag, and push. The `release` GitHub Actions workflow
# then builds cross-platform binaries and publishes them with `gh release create`.
#
# Usage: scripts\release.ps1 v1.0.0

if ($Version -notmatch '^[vV]') {
    $Version = "v$Version"
}
if ($Version -cnotmatch '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$') {
    throw "Version must look like v1.2.3"
}

if (git status --porcelain) {
    throw "Working tree is not clean; commit or stash first"
}

Write-Host "Running tests..."
go test ./...
if ($LASTEXITCODE -ne 0) {
    throw "Tests failed"
}

git rev-parse $Version 2>$null
if ($LASTEXITCODE -eq 0) {
    throw "Tag $Version already exists"
}

Write-Host "Tagging $Version..."
git tag -a $Version -m "linkserver233 $Version"
git push $Remote $Version

Write-Host "Pushed $Version. The release workflow will build and publish binaries."
if (Get-Command gh -ErrorAction SilentlyContinue) {
    Write-Host "Watching release workflow (Ctrl-C to stop)..."
    gh run watch --exit-status
}
