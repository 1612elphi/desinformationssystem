# bun's node shim hangs
FROM node:26.6.0-trixie-slim AS ui
COPY --from=oven/bun:1.4.4-debian /usr/local/bin/bun /usr/local/bin/bun
WORKDIR /ui
COPY ui/package.json ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY ui/ ./
RUN bun run build

FROM golang:1.26.10-trixie AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY geo/ geo/
RUN CGO_ENABLED=0 go build -trimpath -ldflags=-s -o /dis ./cmd/dis

FROM debian:trixie-slim AS app

# ocr: tesseract-ocr tesseract-ocr-deu ocrmypdf
RUN apt-get update \
    && apt-get install -y --no-install-recommends poppler-utils ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# /app is bind-mounted
COPY --from=go /dis /usr/local/bin/dis
COPY --from=ui /ui/dist /usr/local/share/dis/ui

WORKDIR /app
RUN mkdir -p /app/data/pdfs \
    && useradd -u 1000 -m appuser \
    && chown -R 1000:1000 /app

USER 1000:1000
ENV DB_PATH=/app/data/desinformationssystem.db \
    PDF_DIR=/app/data/pdfs \
    WEB_DIST=/usr/local/share/dis/ui

ENTRYPOINT ["dis"]
