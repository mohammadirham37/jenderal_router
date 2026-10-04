#!/usr/bin/env bash
# build.sh — lint, test, dan build binary linux amd64+arm64 (tanpa CGO).
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-dev}"

# UI SvelteKit: build bila Node tersedia; otherwise pakai dist yang ter-commit
if command -v npm >/dev/null 2>&1 && [ -d web ]; then
  echo ">> build UI (SvelteKit)"
  ( cd web && npm ci --no-audit --no-fund && npm run build )
  rm -rf internal/web/dist
  cp -r web/build internal/web/dist
else
  echo ">> npm tidak tersedia — memakai internal/web/dist yang ter-commit"
fi

echo ">> go vet"
CGO_ENABLED=0 go vet ./...

echo ">> go test ./..."
CGO_ENABLED=0 go test ./...

OUT=dist
rm -rf "$OUT"
mkdir -p "$OUT"
for arch in amd64 arm64; do
  echo ">> build linux/${arch}"
  CGO_ENABLED=0 GOOS=linux GOARCH=$arch \
    go build -trimpath \
      -ldflags "-s -w -X github.com/jenderal/jenderalrouter/internal/api.Version=${VERSION}" \
      -o "$OUT/jenderalrouter-linux-$arch" ./cmd/jenderalrouter
  tar -czf "$OUT/jenderalrouter-linux-$arch.tar.gz" -C "$OUT" "jenderalrouter-linux-$arch"
done

echo ">> selesai:"
ls -la "$OUT"
