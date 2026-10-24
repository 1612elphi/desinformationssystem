#!/usr/bin/env bash
# Nightly geo tagging: street mentions + primary Stadtteil over enriched docs,
# then the data-quality report. Driven by dis-geo-backfill.service/.timer.
#
# host network reaches switchboard
#
# Idempotent: dis geo-backfill processes rows with geo_status not
# 'ok', so the first run drains the enriched backlog and later runs pick up only
# new documents. Scheduled after the enrich window so summaries exist.
set -euo pipefail

APP=/home/ruby/desinformationssystem
IMAGE=desinformationssystem:latest
TYPESAFE_BASE_URL="${TYPESAFE_BASE_URL:-http://localhost:3990/typesafe}"

log() { echo "[geo-backfill] $*"; }
dis() {
  docker run --rm --user 1000:1000 --network host -v "$APP":/app -w /app \
    -e TYPESAFE_BASE_URL="$TYPESAFE_BASE_URL" "$IMAGE" "$@"
}

log "tagging streets + Stadtteil over enriched docs (via $TYPESAFE_BASE_URL)"
dis geo-backfill

log "data quality (roll-call consistency + orphan files):"
dis data-quality
