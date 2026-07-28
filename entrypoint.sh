#!/usr/bin/env bash
# Dispatch by ROLE: web | mcp | scraper.
# The scraper runs once at startup, then daily at SCRAPE_HOUR:SCRAPE_MINUTE.
set -euo pipefail

ROLE="${ROLE:-web}"
WEB_PORT="${WEB_PORT:-3650}"
SCRAPE_HOUR="${SCRAPE_HOUR:-3}"
SCRAPE_MINUTE="${SCRAPE_MINUTE:-30}"
# Ingestion engine for the scraper role: html (scraper.py, the proven default)
# or oparl (oparl.py, the OParl API ingester — see docs/oparl-migration.md).
INGEST="${INGEST:-html}"

case "$ROLE" in
  web)
    exec uvicorn web:app --host 0.0.0.0 --port "$WEB_PORT"
    ;;
  mcp)
    exec python mcp_server.py
    ;;
  ticker)
    exec python ticker.py
    ;;
  scraper)
    # This branch keeps bash as PID 1, which by default ignores SIGTERM — so a
    # `docker stop` would hang out the full grace period and then SIGKILL a
    # possibly mid-scrape python. Run children in the background and forward
    # TERM/INT so shutdown is prompt and logged.
    child=""
    on_term() {
      echo "[scraper] SIGTERM — stopping"
      [ -n "$child" ] && kill "$child" 2>/dev/null
      exit 0
    }
    trap on_term TERM INT

    case "$INGEST" in
      oparl) SCRAPE_CMD="oparl.py" ;;
      html)  SCRAPE_CMD="scraper.py" ;;
      *) echo "unknown INGEST: $INGEST (want html|oparl)" >&2; exit 1 ;;
    esac

    run_scrape() {
      python "$SCRAPE_CMD" & child=$!
      wait "$child" || echo "[scraper] run failed; continuing"
      child=""
    }

    echo "[scraper] initial run (ingest=${INGEST}, months=${BACKFILL_MONTHS:-12})"
    run_scrape
    while true; do
      # seconds until the next SCRAPE_HOUR:SCRAPE_MINUTE
      sleep_secs=$(python - "$SCRAPE_HOUR" "$SCRAPE_MINUTE" <<'PY'
import sys, datetime
h, m = int(sys.argv[1]), int(sys.argv[2])
now = datetime.datetime.now()
nxt = now.replace(hour=h, minute=m, second=0, microsecond=0)
if nxt <= now:
    nxt += datetime.timedelta(days=1)
print(int((nxt - now).total_seconds()))
PY
)
      echo "[scraper] sleeping ${sleep_secs}s until next run at ${SCRAPE_HOUR}:${SCRAPE_MINUTE}"
      sleep "$sleep_secs" & child=$!
      wait "$child" || true
      child=""
      echo "[scraper] scheduled run $(date -Is)"
      run_scrape
    done
    ;;
  *)
    echo "unknown ROLE: $ROLE" >&2
    exit 1
    ;;
esac
