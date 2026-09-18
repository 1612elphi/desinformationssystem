#!/usr/bin/env bash
# Nightly geo tagging: street mentions + primary Stadtteil over enriched docs,
# then the data-quality report. Driven by dis-geo-backfill.service/.timer.
#
# Runs in the image (like pdf-backfill) — pure Python, needs only the DB and the
# TypeSafe key. Idempotent: geo_backfill.py processes rows with geo_status not
# 'ok', so the first run drains the ~17k enriched backlog and later runs pick up
# only new documents. Scheduled after the enrich window so summaries exist.
set -euo pipefail

APP=/home/ruby/desinformationssystem
IMAGE=desinformationssystem:latest
: "${TYPESAFE_API_KEY:?set TYPESAFE_API_KEY (from switchboard) in the unit environment}"

log() { echo "[geo-backfill] $*"; }

log "tagging streets + Stadtteil over enriched docs"
docker run --rm --user 1000:1000 -v "$APP":/app -w /app \
  -e TYPESAFE_API_KEY --entrypoint python "$IMAGE" geo_backfill.py

log "data quality (roll-call consistency + orphan files):"
docker run --rm --user 1000:1000 -v "$APP":/app -w /app \
  --entrypoint python "$IMAGE" data_quality.py
