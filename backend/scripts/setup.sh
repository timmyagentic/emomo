#!/bin/bash

set -e

echo "=== Emomo Setup Script ==="

# Check if .env exists
if [ ! -f .env ]; then
    echo "Creating .env from .env.example..."
    cp .env.example .env
    echo "Please edit .env to add your API keys before running the services."
fi

# Create data directory
mkdir -p data

# Build binaries
echo "Building API server..."
go build -o api ./cmd/api

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Next steps:"
echo "1. Edit .env to add your API keys (OPENAI_API_KEY, JINA_API_KEY)"
echo "2. Start API + logging (Docker Compose): docker compose -f ../deployments/docker-compose.yml up -d"
echo "   - Logs only: docker compose -f ../deployments/docker-compose.yml up -d alloy"
echo "3. Put static images under ./data/memes, then run: ./scripts/import-data.sh -p ./data/memes"
echo "4. Start API server (if not using Docker Compose): ./api"
