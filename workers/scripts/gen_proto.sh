#!/usr/bin/env bash
set -euo pipefail
# Generate Python gRPC stubs for notify.proto into workers/notify/
cd "$(dirname "$0")/.."

uv run python -m grpc_tools.protoc \
  -I../proto \
  --python_out=. \
  --grpc_python_out=. \
  notify/notify.proto

echo "Generated: workers/notify/notify_pb2.py, workers/notify/notify_pb2_grpc.py"
