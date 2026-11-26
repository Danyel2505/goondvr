#!/bin/sh
# Starts the detector on this computer (Linux / macOS). The first time it creates a private Python
# environment, installs what it needs and downloads the base model. Extra options go to server.py:
#   ./start.sh --device cpu        ./start.sh --port 8100 --threads 4
set -e
cd "$(dirname "$0")"
[ -d .venv ] || python3 -m venv .venv
. .venv/bin/activate
if [ ! -f .venv/ready ]; then
  pip install -q numpy Pillow onnx
  # a graphics card only if one is there; the server still checks it works and is faster than the CPU
  if command -v nvidia-smi >/dev/null 2>&1; then pip install -q onnxruntime-gpu || pip install -q onnxruntime; else pip install -q onnxruntime; fi
  touch .venv/ready
fi
[ -f models/_base/mobilenetv2_features.onnx ] || python fetch_models.py
exec python server.py "$@"
