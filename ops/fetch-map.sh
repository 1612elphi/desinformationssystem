#!/usr/bin/env bash
set -euo pipefail

APP=/home/ruby/desinformationssystem
VERSION=1.31.2
PMTILES="$APP/bin/pmtiles"

if [ ! -x "$PMTILES" ]; then
  arch=$(uname -m)
  [ "$arch" = aarch64 ] && arch=arm64
  mkdir -p "$APP/bin"
  curl -fsSL "https://github.com/protomaps/go-pmtiles/releases/download/v$VERSION/go-pmtiles_${VERSION}_Linux_$arch.tar.gz" \
    | tar -xz -C "$APP/bin" pmtiles
fi

PMTILES="$PMTILES" node "$APP/scripts/map/fetch-tiles.mjs" "$APP/data/map/karlsruhe.pmtiles"
echo "[fetch-map] $APP/data/map/karlsruhe.pmtiles"
