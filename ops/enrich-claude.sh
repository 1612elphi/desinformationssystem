#!/usr/bin/env bash
# Nightly bulk enrichment via headless Claude Code (Sonnet). Driven by
# dis-enrich-claude.service/.timer.
#
# Runs on the HOST, not in a container: it needs the `claude` CLI and the
# subscription credential in ~/.claude, neither of which the image has.
# Therefore it runs as ruby (not root) so it reads the right ~/.claude and
# leaves the DB and its -wal/-shm files ruby-owned.
#
# Resumable by design — work is selected by enrich_status, so a run cut short
# by plan limits simply resumes on the next firing.
set -euo pipefail

APP=/home/ruby/desinformationssystem
export DB_PATH="$APP/data/desinformationssystem.db"
export PATH="/home/ruby/.local/bin:$PATH"

# Concurrent `claude -p` invocations, documents per invocation, and the
# wall-clock budget for one night. Tune via the systemd unit.
export ENRICH_CONCURRENCY="${ENRICH_CONCURRENCY:-6}"
export BATCH_DOCS="${BATCH_DOCS:-25}"
BUDGET="${ENRICH_MAX_SECONDS:-18000}"   # 5h: 05:00 -> 10:00

log() { echo "[enrich-claude] $*"; }

cd "$APP"
log "backlog before:"; python3 enrich_claude.py --count

log "starting (concurrency=$ENRICH_CONCURRENCY batch=$BATCH_DOCS budget=${BUDGET}s)"
python3 enrich_claude.py --max-seconds "$BUDGET"

log "backlog after:"; python3 enrich_claude.py --count
