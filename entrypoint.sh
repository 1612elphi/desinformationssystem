#!/usr/bin/env bash
# Dispatch by ROLE: web | mcp | scraper.
# The scraper runs once at startup, then daily at SCRAPE_HOUR:SCRAPE_MINUTE.
set -euo pipefail

ROLE="${ROLE:-web}"
WEB_PORT="${WEB_PORT:-3650}"
SCRAPE_HOUR="${SCRAPE_HOUR:-3}"
SCRAPE_MINUTE="${SCRAPE_MINUTE:-30}"

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
    echo "[scraper] initial run (months=${BACKFILL_MONTHS:-12})"
    python scraper.py || echo "[scraper] initial run failed (will retry on schedule)"
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
      sleep "$sleep_secs"
      echo "[scraper] scheduled run $(date -Is)"
      python scraper.py || echo "[scraper] run failed; continuing"
    done
    ;;
  *)
    echo "unknown ROLE: $ROLE" >&2
    exit 1
    ;;
esac
