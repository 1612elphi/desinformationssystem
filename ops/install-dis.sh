#!/usr/bin/env bash
# Copy the dis binary out of the image to $APP/bin/dis for the host-run jobs
# (enrich-claude, vod, geo-backfill). Re-run after every image rebuild.
set -euo pipefail

APP=/home/ruby/desinformationssystem
IMAGE=desinformationssystem:latest

mkdir -p "$APP/bin"
id=$(docker create "$IMAGE")
trap 'docker rm "$id" >/dev/null' EXIT
docker cp "$id:/usr/local/bin/dis" "$APP/bin/dis.new"
mv "$APP/bin/dis.new" "$APP/bin/dis"
"$APP/bin/dis" init >/dev/null
echo "[install-dis] $APP/bin/dis"
