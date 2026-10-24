#!/usr/bin/env bash
# One-shot bulk seed of the Desinformationssystem archive from the
# karlsruhe-oparl-syndication dump. Driven by dis-seed.service/.timer.
#
# Runs the container as uid 1000 deliberately: the DB and its -wal/-shm
# side files must stay ruby-owned, or the uid-1000 service containers lose
# write access to their own database afterwards.
set -euo pipefail

APP=/home/ruby/desinformationssystem
DUMP=/home/ruby/oparl-syndication/docs
IMAGE=desinformationssystem:latest
DB="$APP/data/desinformationssystem.db"
BACKUP="$APP/data/backups"
STAMP="$(date +%Y%m%d-%H%M%S)"

log() { echo "[seed-once] $*"; }

[ -d "$DUMP" ] || { log "dump missing: $DUMP"; exit 1; }
[ -f "$DB" ]   || { log "db missing: $DB"; exit 1; }

mkdir -p "$BACKUP"
log "backing up DB -> $BACKUP/pre-seed-$STAMP.db"
# consistent live snapshot
docker run --rm --user 1000:1000 -v "$APP":/app -w /app \
  "$IMAGE" backup "/app/data/backups/pre-seed-$STAMP.db"
ls -lh "$BACKUP/pre-seed-$STAMP.db"

# The nightly scrape (03:30) and the seed would otherwise contend for the
# write lock across tens of thousands of transactions. One skipped scrape is
# cheaper than a half-applied seed.
log "stopping desinfo-scraper"
docker stop desinfo-scraper >/dev/null 2>&1 || log "  (was not running)"

restore_scraper() {
  log "restarting desinfo-scraper"
  docker start desinfo-scraper >/dev/null 2>&1 || log "  (failed to start)"
}
trap restore_scraper EXIT

log "seeding from $DUMP"
docker run --rm --user 1000:1000 \
  -v "$APP":/app -v "$DUMP":/dump:ro -w /app \
  "$IMAGE" seed --dir /dump

log "seed finished; pending PDF backlog:"
docker run --rm --user 1000:1000 -v "$APP":/app -w /app \
  "$IMAGE" pdf-backfill --count

log "done"
