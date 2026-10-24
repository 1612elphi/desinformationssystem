#!/usr/bin/env bash
# re-run after image rebuild
set -euo pipefail

APP=/home/ruby/desinformationssystem
IMAGE=desinformationssystem:latest

mkdir -p "$APP/bin"
id=$(docker create "$IMAGE")
trap 'docker rm "$id" >/dev/null' EXIT
docker cp "$id:/usr/local/bin/dis" "$APP/bin/dis.new"
mv "$APP/bin/dis.new" "$APP/bin/dis"
DB_PATH="$APP/data/desinformationssystem.db" "$APP/bin/dis" init >/dev/null
echo "[install-dis] $APP/bin/dis"
