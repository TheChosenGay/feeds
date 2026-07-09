FROM ghcr.io/astral-sh/uv:python3.13-alpine

WORKDIR /app

# Copy proto definitions for code generation
COPY proto/ /proto/
COPY workers/ ./

# Generate Python gRPC stubs from notify.proto
RUN uv pip install --system grpcio-tools protobuf && \
    python -m grpc_tools.protoc \
      -I/proto \
      --python_out=. \
      --grpc_python_out=. \
      /proto/notify/notify.proto && \
    uv pip uninstall --system grpcio-tools protobuf

# Install runtime deps only
RUN uv sync --no-dev

ENV PYTHONPATH=/app

CMD ["uv", "run", "python", "fanout/main.py"]
