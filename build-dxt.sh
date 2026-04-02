#!/usr/bin/env bash
set -euo pipefail

NAME="cubbit-mcp"
VERSION=$(jq -r .version manifest.json)
DIST="dist"

echo "Building $NAME v$VERSION"
mkdir -p "$DIST"

# --- Cross-compile ---
PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "windows/amd64"
)

for PLATFORM in "${PLATFORMS[@]}"; do
  OS="${PLATFORM%/*}"
  ARCH="${PLATFORM#*/}"
  BINARY="$NAME"
  [[ "$OS" == "windows" ]] && BINARY="$NAME.exe"

  echo "  Compiling $OS/$ARCH..."
  GOOS="$OS" GOARCH="$ARCH" CGO_ENABLED=1 go build -ldflags="-s -w" -o "$DIST/$OS-$ARCH/$BINARY" .
done

# --- Bundle .dxt for each platform ---
# A .dxt is a zip file: binary + manifest.json at root
for PLATFORM in "${PLATFORMS[@]}"; do
  OS="${PLATFORM%/*}"
  ARCH="${PLATFORM#*/}"
  BINARY="$NAME"
  [[ "$OS" == "windows" ]] && BINARY="$NAME.exe"

  DXT_FILE="$DIST/${NAME}-${VERSION}-${OS}-${ARCH}.dxt"
  echo "  Bundling $DXT_FILE..."

  TMPDIR=$(mktemp -d)
  cp "$DIST/$OS-$ARCH/$BINARY" "$TMPDIR/$BINARY"
  cp manifest.json "$TMPDIR/manifest.json"
  (cd "$TMPDIR" && zip -q "$OLDPWD/$DXT_FILE" "$BINARY" manifest.json)
  rm -rf "$TMPDIR"
done

echo ""
echo "Done. Artifacts in $DIST/:"
ls -lh "$DIST/"*.dxt
