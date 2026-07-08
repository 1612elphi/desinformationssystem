# ---- Stage 1: build the Carbon (React/Vite) SPA -------------------------
FROM node:20-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- Stage 2: Python runtime (API + MCP + scraper) ----------------------
FROM python:3.12-slim AS app

# poppler-utils provides pdftotext. For ENABLE_OCR=1 also add:
#   tesseract-ocr tesseract-ocr-deu ocrmypdf
RUN apt-get update \
    && apt-get install -y --no-install-recommends poppler-utils ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY db.py scraper.py enrich.py submitter.py vote_parse.py ticker.py web.py mcp_server.py entrypoint.sh ./
COPY --from=web /web/dist ./web/dist

RUN chmod +x entrypoint.sh \
    && mkdir -p /app/data/pdfs \
    && useradd -u 1000 -m appuser \
    && chown -R 1000:1000 /app

USER 1000:1000
ENV DB_PATH=/app/data/desinformationssystem.db \
    PDF_DIR=/app/data/pdfs \
    PYTHONUNBUFFERED=1

ENTRYPOINT ["./entrypoint.sh"]
