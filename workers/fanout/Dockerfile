FROM ghcr.io/astral-sh/uv:python3.13-alpine

WORKDIR /app

# Install deps first (layer caching)
COPY workers/pyproject.toml workers/uv.lock ./
RUN uv sync --no-dev

# Copy all worker code
COPY workers/ ./

ENV PYTHONPATH=/app

CMD ["uv", "run", "python", "fanout/main.py"]
