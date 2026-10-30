#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
version="${1:-dev}"
case "$version" in *[!A-Za-z0-9._-]*|'') printf 'Invalid release version\n' >&2; exit 1;; esac
mkdir -p dist
for arch in amd64 arm64; do
  directory="dist/opnsense-ip-invites_${version}_linux_${arch}"
  mkdir -p "$directory"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -tags production \
    -ldflags "-s -w -X main.version=$version" -o "$directory/opnsense-ip-invites" ./cmd/opnsense-ip-invites
  cp examples/config.yaml "$directory/config.example.yaml"
  cp README.md LICENSE "$directory/"
  cp -R docs "$directory/"
  cp deploy/opnsense-ip-invites.service "$directory/"
  tar -czf "$directory.tar.gz" -C dist "$(basename "$directory")"
done
(cd dist && sha256sum "opnsense-ip-invites_${version}_linux_amd64.tar.gz" "opnsense-ip-invites_${version}_linux_arm64.tar.gz" > "opnsense-ip-invites_${version}_checksums.txt")
