#!/usr/bin/env bash
# Bounded nightly PDF backfill for seeded rows. Driven by
# dis-pdf-backfill.service/.timer.
#
# Deliberately time-boxed rather than run-to-completion: the ~47k backlog at
# the 1.5s politeness throttle is several days of fetching, and a bounded
# nightly window keeps sustained load off web1 while still draining steadily.
# Re-running is always safe — it only picks rows still missing a cached PDF.
set -euo pipefail

APP=/home/ruby/desinformationssystem
IMAGE=desinformationssystem:latest
# Default: 4 hours a night. Override in the unit or the environment.
BUDGET="${BACKFILL_MAX_SECONDS:-14400}"

log() { echo "[pdf-backfill] $*"; }

log "starting, budget ${BUDGET}s"
docker run --rm --user 1000:1000 \
  -v "$APP":/app -w /app \
  --entrypoint python "$IMAGE" \
  backfill_pdfs.py --max-seconds "$BUDGET"

log "remaining:"
docker run --rm --user 1000:1000 -v "$APP":/app -w /app \
  --entrypoint python "$IMAGE" backfill_pdfs.py --count
