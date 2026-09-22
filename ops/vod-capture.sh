#!/usr/bin/env bash
# Hourly: download every finished livestream from the city's YouTube channel.
# The city keeps council VODs up for only 24 hours, so this job must stay cheap
# and independent of transcription (dis-vod-transcribe runs vod.py separately).
#
# The channel also carries unrelated city events (congresses run 8h), hence the title filter.
# live_status flips is_live -> post_live (YouTube still processing) -> was_live;
# only was_live is complete, so the filter waits out processing by itself.
# --download-archive makes re-runs idempotent.
set -uo pipefail

APP=/home/ruby/desinformationssystem
VENV="${VOD_VENV:-/home/ruby/.venvs/dis-vod}"
CHANNEL=https://www.youtube.com/channel/UCi-k4SVGHlSigCtILwczY-Q/streams
OUT="$APP/data/vod"

log() { echo "[vod-capture] $*"; }

mkdir -p "$OUT"
# YouTube extraction breaks every few weeks; a stale yt-dlp would miss the 24h window.
"$VENV/bin/pip" install -q -U yt-dlp || log "yt-dlp self-update failed, using installed version"

out=$("$VENV/bin/yt-dlp" --no-update -i \
  --playlist-end 10 \
  --match-filters "live_status=was_live & title~='(?i)gemeinderat|haushalt|sitzung'" \
  --download-archive "$OUT/archive.txt" \
  -f "bv*[height<=720]+ba/b[height<=720]" --merge-output-format mkv \
  --write-info-json --no-write-playlist-metafiles --no-progress \
  -o "$OUT/%(release_timestamp>%Y-%m-%d)s_%(id)s.%(ext)s" \
  "$CHANNEL" 2>&1)
rc=$?
echo "$out"

# Scheduled (upcoming) streams always error during extraction; anything else is real.
if [ $rc -ne 0 ] && echo "$out" | grep '^ERROR:' | grep -vqE 'will begin in|Premieres in|This live event'; then
  log "yt-dlp failed (exit $rc)"
  exit 1
fi
log "done"
