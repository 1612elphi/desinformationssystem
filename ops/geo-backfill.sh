#!/usr/bin/env bash
# Nightly geo tagging: street mentions + primary Stadtteil over enriched docs,
# then the data-quality report. Driven by dis-geo-backfill.service/.timer.
#
# Runs on the HOST like enrich-claude (stdlib-only Python, no image needed) and
# routes Jev calls through switchboard, which holds the TypeSafe key — so no key
# lives here. Idempotent: geo_backfill.py processes rows with geo_status not
# 'ok', so the first run drains the enriched backlog and later runs pick up only
# new documents. Scheduled after the enrich window so summaries exist.
set -euo pipefail

APP=/home/ruby/desinformationssystem
export DB_PATH="$APP/data/desinformationssystem.db"
export TYPESAFE_BASE_URL="${TYPESAFE_BASE_URL:-http://localhost:3990/typesafe}"

log() { echo "[geo-backfill] $*"; }

cd "$APP"
log "tagging streets + Stadtteil over enriched docs (via $TYPESAFE_BASE_URL)"
python3 geo_backfill.py

log "data quality (roll-call consistency + orphan files):"
python3 data_quality.py
