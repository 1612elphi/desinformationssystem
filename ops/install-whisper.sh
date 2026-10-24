#!/usr/bin/env bash
# Build whisper.cpp with CUDA for the Tesla P4 (compute capability 6.1) and
# fetch the models dis vod reads from data/whisper/. Needs cmake, a C++
# compiler and the CUDA toolkit.
set -euo pipefail

APP=/home/ruby/desinformationssystem
VERSION=v1.9.1
SRC="${WHISPER_SRC:-/home/ruby/src/whisper.cpp}"
MODELS="$APP/data/whisper"
HF=https://huggingface.co

if [ ! -d "$SRC" ]; then
  git clone --depth 1 --branch "$VERSION" https://github.com/ggml-org/whisper.cpp "$SRC"
fi
git -C "$SRC" fetch --depth 1 origin tag "$VERSION"
git -C "$SRC" checkout -q "$VERSION"
cmake -S "$SRC" -B "$SRC/build" -DGGML_CUDA=1 -DCMAKE_CUDA_ARCHITECTURES=61 -DCMAKE_BUILD_TYPE=Release
cmake --build "$SRC/build" -j --target whisper-cli
install -D "$SRC/build/bin/whisper-cli" /home/ruby/.local/bin/whisper-cli

mkdir -p "$MODELS"
[ -f "$MODELS/ggml-large-v3-turbo.bin" ] || curl -fL -o "$MODELS/ggml-large-v3-turbo.bin" \
  "$HF/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin"
[ -f "$MODELS/ggml-silero-v5.1.2.bin" ] || curl -fL -o "$MODELS/ggml-silero-v5.1.2.bin" \
  "$HF/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin"
echo "[install-whisper] whisper-cli $VERSION, models in $MODELS"
